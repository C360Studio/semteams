import { test, expect } from "@playwright/test";

/**
 * Journey: ADR-053 Phase 4c PR-1 — an in-run loop's approval_required tool-gate
 * pauses the run executing→awaiting_approval (the run-phase reflection of the
 * loop-level approval gate, ADR-030).
 *
 *   coordinator(dispatch) → decide(dev_via_test)        run becomes `executing`
 *   first-pass Lisa       → decide(needs_clarification)  rule 02f spawns the
 *                                                        recovery coordinator
 *                                                        (inherit anchor:
 *                                                         agent.run.entity_id)
 *   recovery coordinator  → tool_call(create_rule)       create_rule ∈
 *                                                        approval_required → the
 *                                                        loop parks in
 *                                                        awaiting_approval, emits
 *                                                        agent.approval_pending.<id>;
 *                                                        the approvalpause subscriber
 *                                                        stamps
 *                                                        agent.run.approval_pending
 *                                                        on the run entity
 *   agent-run/12 (approval_pending + phase==executing) → awaiting_approval
 *
 * The run is honestly PAUSED on a human approval. PR-1 has no resume — the run parks
 * in awaiting_approval (resume is PR-2). The decisive assertions:
 *   - agent.run.phase reaches awaiting_approval (NOT completed/failed),
 *   - agent.run.approval_pending is set on the run entity (the 4c marker),
 *   - agent.run.clarification_pending is NOT (proves rule 12 drove it, not 4b-2's 09),
 *   - agent.run.last_transition_from == "executing" (rule 12 drove it).
 *
 * NOTE (per the upstream-fragility flag): the approval path — gateForApproval's
 * agent.approval_pending publish, the ApprovalFilter, BeginAwaitingApproval — has
 * thin real-world exercise. This journey is the decisive end-to-end proof that the
 * gated tool actually parks the loop and emits the event our subscriber consumes.
 *
 * Required fixture: test/fixtures/journeys/approval-pause.yaml
 * Required config: configs/e2e-flow-bootstrap.json (approval_required:[create_rule],
 *   agent-run/12, the agent.approval_pending teams-loop output port)
 */

test.describe("ADR-053 Phase 4c — in-run tool-gate pauses the run (executing→awaiting_approval)", () => {
  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok(), "Backend not healthy — stack not running?").toBe(true);
  });

  test.setTimeout(120_000);

  test("recovery coordinator gated create_rule → run transitions executing→awaiting_approval", async ({
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
    // spawns the recovery coordinator, which calls the gated create_rule.
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
      "agent.run.phase never reached a settled phase on the run entity (run stuck in dispatched/executing — the gated create_rule did not drive executing→awaiting_approval)",
    ).toBeTruthy();
    expect(
      runPhases,
      "run must reach awaiting_approval (ADR-053 4c: approvalpause stamps approval_pending → agent-run/12 executing→awaiting_approval). Got: " +
        JSON.stringify(runPhases),
    ).toContain("awaiting_approval");
    expect(
      runPhases,
      "run must NOT complete — the gated tool never executed; no reviewer approved",
    ).not.toContain("completed");
    expect(
      runPhases,
      "run must NOT fail — an approval gate is a pause, not a failure",
    ).not.toContain("failed");

    // -----------------------------------------------------------------
    // Step 4 — DECISIVE: the transition is executing→awaiting_approval (rule 12),
    // not dispatched→X. agent.run.last_transition_from records the from-phase.
    // -----------------------------------------------------------------
    const fromPhases = await pollUntil(async () => {
      const triples = await fetchTriples(request, {
        predicate: "agent.run.last_transition_from",
        limit: 20,
      });
      const objs = triples
        .filter((t) => String(t.subject ?? "").includes("agent.chain.execution."))
        .map((t) => String(t.object));
      return objs.length > 0 ? objs : null;
    }, { timeoutMs: 15_000 });
    expect(
      fromPhases,
      "agent.run.last_transition_from missing on the run entity",
    ).toBeTruthy();
    expect(
      fromPhases,
      "the awaiting_approval transition must come FROM executing (agent-run/12). Got: " +
        JSON.stringify(fromPhases),
    ).toContain("executing");

    // -----------------------------------------------------------------
    // Step 5 — 4c marker present: agent.run.approval_pending on the run entity
    // (stamped by the approvalpause subscriber). This is what distinguishes a 4c
    // tool-gate pause from a 4b-2 clarification pause.
    // -----------------------------------------------------------------
    const approvalMarkers = await pollUntil(async () => {
      const triples = await fetchTriples(request, {
        predicate: "agent.run.approval_pending",
        limit: 20,
      });
      const onRun = triples.filter((t) =>
        String(t.subject ?? "").includes("agent.chain.execution."),
      );
      return onRun.length > 0 ? onRun : null;
    }, { timeoutMs: 15_000 });
    expect(
      approvalMarkers,
      "expected agent.run.approval_pending on the run entity (approvalpause subscriber) — the marker that drives agent-run/12 and distinguishes a 4c tool-gate from a 4b-2 clarification pause.",
    ).toBeTruthy();

    // -----------------------------------------------------------------
    // Step 6 — NEGATIVE: this is NOT a 4b-2 clarification pause. No
    // agent.run.clarification_pending should appear on the run entity (the
    // recovery coordinator called a gated tool, it did not decide(ask_user)).
    // -----------------------------------------------------------------
    const clarificationMarkers = await fetchTriples(request, {
      predicate: "agent.run.clarification_pending",
      limit: 20,
    });
    expect(
      clarificationMarkers.filter((t) =>
        String(t.subject ?? "").includes("agent.chain.execution."),
      ).length,
      "agent.run.clarification_pending must be ABSENT — a 4c tool-gate pause must not be mistaken for (or cross-resumed with) a 4b-2 clarification pause.",
    ).toBe(0);
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
