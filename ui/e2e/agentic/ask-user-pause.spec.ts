import { test, expect } from "@playwright/test";

/**
 * Journey: ADR-053 Phase 4b-2 PR-1 — an in-run coordinator decide(ask_user)
 * pauses the run executing→awaiting_approval.
 *
 *   coordinator(dispatch) → decide(dev_via_test)        run becomes `executing`
 *   first-pass Lisa       → decide(needs_clarification)  rule 02f spawns the
 *                                                        recovery coordinator
 *                                                        (inherit anchor:
 *                                                         agent.run.entity-id)
 *   recovery coordinator  → decide(ask_user)             coordinator/03-ask-user
 *                                                        stamps user_question;
 *                                                        agent-run/07 stamps
 *                                                        agent.run.clarification-pending
 *                                                        on the run entity
 *   agent-run/09 (clarification_pending + phase==executing) → awaiting_approval
 *
 * The run is honestly PAUSED on a human reply. PR-1 has no resume — the run parks
 * in awaiting_approval (resume is PR-2). The decisive assertion is
 * agent.run.last-transition-from == "executing" (rule 09 drove it).
 *
 * Required fixture: test/fixtures/journeys/ask-user-pause.yaml
 * Required config: configs/e2e-flow-bootstrap.json (interactive mode; rules 07+09)
 */

test.describe("ADR-053 Phase 4b-2 — in-run ask_user pauses the run (executing→awaiting_approval)", () => {
  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok(), "Backend not healthy — stack not running?").toBe(true);
  });

  test.setTimeout(120_000);

  test("recovery coordinator decide(ask_user) → run transitions executing→awaiting_approval", async ({
    page,
    request,
  }) => {
    // -----------------------------------------------------------------
    // Step 1 — open the Board so the SSE stream connects.
    // -----------------------------------------------------------------
    await page.goto("/");
    await expect(page.getByTestId("connection-status")).toHaveAttribute(
      "data-summary",
      "healthy",
      { timeout: 15000 },
    );
    await expect(page.getByTestId("kanban-board")).toBeVisible();

    // -----------------------------------------------------------------
    // Step 2 — send a dev-build prompt. The coordinator classifies it
    // dev_via_test; first-pass Lisa escalates needs_clarification; rule 02f
    // spawns the recovery coordinator, which asks the user.
    // -----------------------------------------------------------------
    await page.getByTestId("chat-input").fill(
      "Build a Go HTTP service that decodes MAVLink v2 HEARTBEAT frames over UDP and serves the latest at GET /heartbeat as JSON.",
    );
    await page.getByTestId("send-button").click();

    // -----------------------------------------------------------------
    // Step 3 — wait for the run entity to reach a SETTLED phase. Poll
    // agent.run.phase; settle on awaiting_approval OR completed OR failed so a
    // regression that wrongly completes/fails is caught (not just a timeout).
    // -----------------------------------------------------------------
    const runPhases = await pollUntil(async () => {
      const phases = await fetchTriples(request, {
        predicate: "agent.run.phase",
        limit: 20,
      });
      const objs = phases
        .filter((t) => String(t.subject ?? "").includes("agent.chain.execution."))
        .map((t) => String(t.object));
      if (
        objs.includes("awaiting_approval") ||
        objs.includes("completed") ||
        objs.includes("failed")
      )
        return objs;
      return null;
    }, { timeoutMs: 90_000 });

    expect(
      runPhases,
      "agent.run.phase never reached a settled phase on the run entity (run stuck in dispatched/executing — the recovery coordinator's ask_user did not drive executing→awaiting_approval)",
    ).toBeTruthy();
    expect(
      runPhases,
      "run must reach awaiting_approval (ADR-053 4b-2: agent-run/07 stamps clarification_pending → agent-run/09 executing→awaiting_approval). Got: " +
        JSON.stringify(runPhases),
    ).toContain("awaiting_approval");
    expect(
      runPhases,
      "run must NOT complete — the recovery coordinator asked rather than answered; no reviewer approved",
    ).not.toContain("completed");
    expect(
      runPhases,
      "run must NOT fail — ask_user is a pause, not a failure",
    ).not.toContain("failed");

    // -----------------------------------------------------------------
    // Step 4 — DECISIVE: the transition is executing→awaiting_approval (rule 09),
    // not dispatched→X. agent.run.last-transition-from records the from-phase.
    // -----------------------------------------------------------------
    const fromPhases = await pollUntil(async () => {
      const triples = await fetchTriples(request, {
        predicate: "agent.run.last-transition-from",
        limit: 20,
      });
      const objs = triples
        .filter((t) => String(t.subject ?? "").includes("agent.chain.execution."))
        .map((t) => String(t.object));
      return objs.length > 0 ? objs : null;
    }, { timeoutMs: 15_000 });
    expect(
      fromPhases,
      "agent.run.last-transition-from missing on the run entity",
    ).toBeTruthy();
    expect(
      fromPhases,
      "the awaiting_approval transition must come FROM executing (agent-run/09). Got: " +
        JSON.stringify(fromPhases),
    ).toContain("executing");

    // -----------------------------------------------------------------
    // Step 5 — evidence the pause was caused by ask_user (not a 4c tool-gate):
    // the recovery coordinator stamped coordinator.clarification.question (03-ask-user),
    // and agent-run/07 stamped agent.run.clarification-pending on the run entity.
    // -----------------------------------------------------------------
    const questions = await fetchTriples(request, {
      predicate: "coordinator.clarification.question",
      limit: 20,
    });
    expect(
      questions.length,
      "expected a coordinator.clarification.question triple (the recovery coordinator's ask_user prose). Zero → ask_user never fired.",
    ).toBeGreaterThanOrEqual(1);

    const pendingMarkers = await fetchTriples(request, {
      predicate: "agent.run.clarification-pending",
      limit: 20,
    });
    expect(
      pendingMarkers.filter((t) =>
        String(t.subject ?? "").includes("agent.chain.execution."),
      ).length,
      "expected agent.run.clarification-pending on the run entity (agent-run/07) — the marker that distinguishes a 4b-2 clarification pause from a 4c tool-gate.",
    ).toBeGreaterThanOrEqual(1);
  });
});

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
