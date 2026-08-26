import { test, expect } from "@playwright/test";

/**
 * Journey: ADR-053 Phase 4b — run-level CLARIFICATION POLICY, AUTONOMOUS mode.
 *
 * Validates the framework decide-action gate (semstreams#239, beta.104) wired
 * into SemTeams via agentic-tools.restricted_decide_actions. The paired
 * Taskfile target boots a config derived from e2e-flow-bootstrap.json with
 * restricted_decide_actions: ["ask_user"] (autonomous mode).
 *
 * The FRONT-DOOR coordinator (which carries NO per-task action_allowlist — the
 * exact structural hole #239 closes) is fed two scripted responses: it first
 * tries decide(ask_user) — barred by the gate → ToolErrorInvalidArgs → the loop
 * re-picks — then decide(respond_direct), answering autonomously.
 *
 * Asserts the OUTCOME of the policy, not the mechanism:
 *   - coordinator.clarification.reply IS stamped  → the coordinator ANSWERED
 *     (03b-respond-direct fired) — it resolved without deferring to the user.
 *   - coordinator.clarification.question is ABSENT → 03-ask-user never fired — the
 *     ask_user decide was blocked, not delivered.
 *
 * Config: derived at run time from configs/e2e-flow-bootstrap.json.
 * Required fixture: test/fixtures/journeys/clarification-autonomous.yaml
 */

test.describe("ADR-053 Phase 4b — clarification policy (autonomous mode)", () => {
  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok(), "Backend not healthy — stack not running?").toBe(true);
  });

  // One loop; the coordinator does ask_user(rejected)→respond_direct. 90s
  // covers mock scheduling jitter + cold start.
  test.setTimeout(90_000);

  test("front-door ask_user is barred → coordinator answers via respond_direct", async ({
    page,
    request,
  }) => {
    // -----------------------------------------------------------------
    // Step 1 — open the Board so the SSE stream connects before the loop.
    // -----------------------------------------------------------------
    await page.goto("/");
    await expect(page.getByTestId("connection-status")).toHaveAttribute(
      "data-summary",
      "healthy",
      { timeout: 15000 },
    );
    await expect(page.getByTestId("kanban-board")).toBeVisible();

    // -----------------------------------------------------------------
    // Step 2 — send an ambiguous request. The mock scripts the coordinator to
    // try ask_user first; in autonomous mode the gate bars it.
    // -----------------------------------------------------------------
    await page.getByTestId("chat-input").fill(
      "Set it up the way we discussed — you know the one.",
    );
    await page.getByTestId("send-button").click();

    // -----------------------------------------------------------------
    // Step 3 — the coordinator ANSWERED: poll for a coordinator.clarification.reply
    // triple (stamped by 03b-respond-direct). Its presence proves the loop
    // re-picked respond_direct after the ask_user rejection and delivered.
    // -----------------------------------------------------------------
    const replies = await pollUntil(async () => {
      const t = await fetchTriples(request, {
        predicate: "coordinator.clarification.reply",
        limit: 20,
      });
      return t.length > 0 ? t : null;
    }, { timeoutMs: 60_000 });
    expect(
      replies,
      "no coordinator.clarification.reply triple — the coordinator never answered via respond_direct. Either the ask_user gate dead-ended the loop (regression: an off-policy decide must re-pick, not hang), or the wiring didn't apply restricted_decide_actions.",
    ).toBeTruthy();

    // -----------------------------------------------------------------
    // Step 4 — ask_user was BLOCKED, not delivered: there must be NO
    // coordinator.clarification.question triple (03-ask-user fires only on a SUCCESSFUL
    // decide(ask_user); the gate rejects before any decision triple is stamped).
    // -----------------------------------------------------------------
    const questions = await fetchTriples(request, {
      predicate: "coordinator.clarification.question",
      limit: 20,
    });
    expect(
      questions.length,
      `expected ZERO coordinator.clarification.question triples in autonomous mode (ask_user barred by restricted_decide_actions), got ${questions.length}: ${JSON.stringify(
        questions.map((q) => q.object),
      )}. A question triple means the front-door ask_user gate did NOT fire — the #239 front-door coverage is not wired.`,
    ).toBe(0);

    // -----------------------------------------------------------------
    // Step 5 — agentic-dispatch delivered the typed user response; rule 03b
    // separately retains the audit triple asserted above.
    // -----------------------------------------------------------------
    const resp = await request.get(
      "/message-logger/entries?limit=500&subject=user.response.*",
    );
    expect(resp.ok(), "/message-logger/entries returned non-OK").toBe(true);
    const payloads = (await resp.json()) as Array<{ subject: string }>;
    expect(
      payloads.length,
      "expected at least one user.response.* publish (the autonomous answer)",
    ).toBeGreaterThanOrEqual(1);
  });
});

/**
 * fetchTriples — read graph triples via GET /graph/triples.
 */
async function fetchTriples(
  request: import("@playwright/test").APIRequestContext,
  params: { subject?: string; predicate?: string; limit?: number },
): Promise<Array<{ subject?: string; object?: unknown }>> {
  const query = new URLSearchParams();
  if (params.subject) query.set("subject", params.subject);
  if (params.predicate) query.set("predicate", params.predicate);
  if (params.limit) query.set("limit", String(params.limit));
  const resp = await request.get(`/graph/triples?${query.toString()}`);
  if (!resp.ok()) {
    throw new Error(
      `GET /graph/triples?${query.toString()} returned ${resp.status()}`,
    );
  }
  return (await resp.json()) as Array<{ subject?: string; object?: unknown }>;
}

/**
 * pollUntil — race a polling fn against a deadline.
 */
async function pollUntil<T>(
  fn: () => Promise<T | null>,
  opts: { timeoutMs: number; intervalMs?: number },
): Promise<T | null> {
  const deadline = Date.now() + opts.timeoutMs;
  const interval = opts.intervalMs ?? 250;
  while (Date.now() < deadline) {
    const result = await fn();
    if (result != null) return result;
    await new Promise((r) => setTimeout(r, interval));
  }
  return null;
}
