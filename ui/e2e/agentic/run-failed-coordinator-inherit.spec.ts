import { test, expect } from "@playwright/test";

/**
 * Journey: ADR-053 Phase 4b-1a — an INHERIT-anchor recovery coordinator that
 * fails while executing drives the run executing→failed via rule 05 (the
 * agent.run.entity-id branch).
 *
 * The anchor_inherit sibling of run-failed-coordinator.spec.ts (which exercises
 * the THREADED branch → rule 06). It closes the empirical gap the 4b-1a review
 * flagged: the inherit coordinator-failure path had no end-to-end mock journey.
 *
 *   coordinator(dispatch) → decide(dev_via_test)            run becomes `executing`
 *   first-pass Lisa       → decide(needs_clarification)      rule 02f spawns the
 *                                                            recovery coordinator
 *                                                            (DEFAULT run_scope,
 *                                                            does NOT thread lineage)
 *   recovery coordinator  → WEDGED (repeating               outcome=failed
 *                            read_loop_result, never          (max_iterations)
 *                            decides)
 *
 * Because rule 02f does NOT thread run-loop-entity-id, the recovery coordinator
 * carries the INHERITED agent.run.entity-id but NO agent.lineage.run-loop-entity-id —
 * so only agent-run/05 (length_eq 0) can fire (agent-run/06 needs length_gt 0).
 * agent-run/04-executing-to-failed then drives the transition.
 *
 * The decisive assertions match the threaded sibling (agent.run.phase==failed,
 * last_transition_from==executing) — the inherit-vs-threaded distinction is
 * structural (02f threads nothing → rule 06 cannot fire), so a run reaching
 * failed here is necessarily the rule-05 path.
 *
 * Required fixture: test/fixtures/journeys/run-failed-coordinator-inherit.yaml
 * Required config: configs/e2e-flow-bootstrap.json (rule 02f + agent-run/05)
 */

test.describe("ADR-053 Phase 4b-1a — inherit-anchor recovery coordinator fails → executing→failed", () => {
  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok(), "Backend not healthy — stack not running?").toBe(true);
  });

  // coordinator + Lisa + the wedged recovery coordinator (8 mock round-trips for
  // the wedge) before the run settles. 2.5 minutes covers mock jitter + cold start.
  test.setTimeout(150_000);

  test("inherit recovery coordinator wedged while executing → run transitions executing→failed via rule 05", async ({
    page,
    request,
  }) => {
    // -----------------------------------------------------------------
    // Step 1 — open the Board so the SSE stream connects before loops appear.
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
    // action="dev_via_test"; rule 01 spawns first-pass Lisa, who escalates
    // needs_clarification → rule 02f spawns the inherit recovery coordinator,
    // which is wedged.
    // -----------------------------------------------------------------
    await page.getByTestId("chat-input").fill(
      "Build a Go HTTP service that decodes MAVLink v2 HEARTBEAT frames over UDP and serves the latest at GET /heartbeat as JSON.",
    );
    await page.getByTestId("send-button").click();

    // -----------------------------------------------------------------
    // Step 3 — wait for the run entity to reach a TERMINAL phase. Settle on
    // failed OR completed so a regression that wrongly completes is caught.
    // -----------------------------------------------------------------
    const runPhases = await pollUntil(async () => {
      const phases = await fetchTriples(request, {
        predicate: "agent.run.phase",
        limit: 20,
      });
      const objs = phases
        .filter((t) => String(t.subject ?? "").includes("agent.chain.execution."))
        .map((t) => String(t.object));
      if (objs.includes("failed") || objs.includes("completed")) return objs;
      return null;
    }, { timeoutMs: 120_000 });

    expect(
      runPhases,
      "agent.run.phase never reached a terminal on the run entity (run stuck in dispatched/executing — the inherit recovery-coordinator failure did not drive executing→failed)",
    ).toBeTruthy();
    expect(
      runPhases,
      "run must reach failed (ADR-053 4b-1a inherit branch: the recovery coordinator inherits agent.run.entity-id → agent-run/05 stamps agent.run.outcome=failed → agent-run/04 executing→failed). Got: " +
        JSON.stringify(runPhases),
    ).toContain("failed");
    expect(
      runPhases,
      "run must NOT complete — Lisa escalated needs_clarification and the recovery coordinator failed; no reviewer approved",
    ).not.toContain("completed");

    // -----------------------------------------------------------------
    // Step 4 — DECISIVE: the transition is executing→failed (rule 04 driven by
    // rule 05's agent.run.entity-id stamp), NOT D3's dispatched→failed.
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
      "the failed transition must come FROM executing (agent-run/04 driven by agent-run/05's agent.run.entity-id stamp) — a from-phase of 'dispatched' would mean the D3 zombie guard fired instead. Got: " +
        JSON.stringify(fromPhases),
    ).toContain("executing");

    // -----------------------------------------------------------------
    // Step 5 — evidence the inherit path ran: a dev-via-test-plan (Lisa) loop
    // exists AND the rule-02f recovery coordinator (role=="coordinator",
    // distinct from the dispatch coordinator by STATE — the dispatch coordinator
    // reached "complete", the recovery coordinator is non-complete) was spawned
    // and wedged. Polled (the dispatch tracker is a separate subscriber from the
    // rule engine — avoids a lagging-tracker flake under CI load).
    // -----------------------------------------------------------------
    const loopsEvidence = await pollUntil(async () => {
      const loops = (await request
        .get("/teams-dispatch/loops")
        .then((r) => r.json())) as Array<{ role: string; state: string }>;
      const lisa = loops.find((l) => l.role === "dev-via-test-plan");
      const recovery = loops.find(
        (l) => l.role === "coordinator" && l.state !== "complete",
      );
      return lisa && recovery ? { loops, lisa, recovery } : null;
    }, { timeoutMs: 15_000 });

    expect(
      loopsEvidence,
      "expected a dev-via-test-plan (Lisa) loop (which escalated needs_clarification → rule 02f) AND a non-complete coordinator-role loop (the rule-02f inherit recovery coordinator, wedged to a max_iterations failure). Their absence means rule 02f never spawned the inherit recovery coordinator.",
    ).toBeTruthy();
  });
});

/**
 * fetchTriples — read graph triples via GET /graph/triples. Mirrors the helper
 * in run-failed-coordinator.spec.ts.
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
