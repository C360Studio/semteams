import { test, expect } from "@playwright/test";

/**
 * Journey: ADR-053 Phase 4b-1a — a THREADED recovery coordinator that fails
 * while executing drives the run executing→failed via rule 06 (lineage anchor).
 *
 * The merge gate for the 4b-1a anchor-threading work. This is the
 * coordinator-failure sibling of run-failed.spec.ts (which wedges a research
 * ROLE → rule 04 via the agent.run.entity-id branch). Here:
 *
 *   coordinator(dispatch) → decide(dev_via_test)        run becomes `executing`
 *   first-pass Lisa       → emit_plan + decide(planned)  rule 02 spawns CBG
 *                                                         (threads run-loop-entity-id)
 *   CBG plan-review       → decide(plan_rejected)        rule 02e spawns the
 *                                                         recovery coordinator,
 *                                                         threading run-loop-entity-id
 *                                                         off the CBG's lineage
 *   recovery coordinator  → WEDGED (repeating            outcome=failed
 *                            read_loop_result, never      (max_iterations)
 *                            decides)
 *
 *   agent-run/06 (role==coordinator + outcome in [failed,truncated] +
 *                 agent.lineage.run-loop-entity-id length_gt 0)
 *     → agent.run.outcome=failed on the run entity (via the THREADED lineage
 *       anchor — the empirical proof it resolves, not a garbage literal)
 *   agent-run/04-executing-to-failed → executing→failed
 *
 * Why this proves rule 06 and NOT the upstream D3 zombie guard: D3 fires only
 * while phase==dispatched. Here the dispatch coordinator completed with a
 * confirmed handoff (the run is `executing`), and a later-spawned recovery
 * coordinator is what fails. The decisive assertion is
 * agent.run.last-transition-from == "executing".
 *
 * Required fixture: test/fixtures/journeys/run-failed-coordinator.yaml
 * Required config: configs/e2e-flow-bootstrap.json (rules 02e + agent-run/06)
 */

test.describe("ADR-053 Phase 4b-1a — threaded recovery coordinator fails → executing→failed", () => {
  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok(), "Backend not healthy — stack not running?").toBe(true);
  });

  // coordinator + Lisa + CBG + the wedged recovery coordinator (8 mock
  // round-trips for the wedge) before the run settles. 2.5 minutes covers mock
  // scheduling jitter + cold start.
  test.setTimeout(150_000);

  test("recovery coordinator wedged while executing → run transitions executing→failed via rule 06", async ({
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
    // action="dev_via_test"; rule 01 spawns Lisa; the plan-review gate rejects
    // and rule 02e spawns the (threaded) recovery coordinator, which is wedged.
    // -----------------------------------------------------------------
    await page.getByTestId("chat-input").fill(
      "Build a Go HTTP service that decodes MAVLink v2 HEARTBEAT frames over UDP and serves the latest at GET /heartbeat as JSON, using github.com/bluenviron/gomavlib.",
    );
    await page.getByTestId("send-button").click();

    // -----------------------------------------------------------------
    // Step 3 — wait for the run entity to reach a TERMINAL phase. Poll
    // agent.run.phase; settle on failed OR completed so a regression that
    // wrongly completes is caught (not just a timeout).
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
      "agent.run.phase never reached a terminal on the run entity (run stuck in dispatched/executing — the recovery-coordinator failure did not drive executing→failed)",
    ).toBeTruthy();
    expect(
      runPhases,
      "run must reach failed (ADR-053 4b-1a: rule 02e threads run-loop-entity-id onto the recovery coordinator → agent-run/06 stamps agent.run.outcome=failed → agent-run/04 executing→failed). Got: " +
        JSON.stringify(runPhases),
    ).toContain("failed");
    expect(
      runPhases,
      "run must NOT complete — no reviewer ever approved; the recovery coordinator failed",
    ).not.toContain("completed");

    // -----------------------------------------------------------------
    // Step 4 — DECISIVE: the transition is executing→failed (rule 04 driven by
    // rule 06's lineage-anchor stamp), NOT D3's dispatched→failed.
    // agent.run.last-transition-from records the from-phase (beta.103 REPLACEs
    // it). A from-phase of "dispatched" would mean D3 fired instead.
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
      "the failed transition must come FROM executing (agent-run/04 driven by agent-run/06's lineage-anchor stamp) — a from-phase of 'dispatched' would mean the D3 zombie guard fired on the dispatched root instead. Got: " +
        JSON.stringify(fromPhases),
    ).toContain("executing");

    // -----------------------------------------------------------------
    // Step 5 — evidence the 4b-1a threaded path actually ran: the plan-review
    // CBG escalated (a reviewer-dev-via-test loop exists) AND the rule-02e
    // recovery coordinator was spawned and did NOT complete (wedged to failure).
    //
    // The dispatch (front-door) coordinator ALSO has role=="coordinator" (the
    // config's default_role), so role alone does not distinguish them — the
    // discriminant is STATE: the dispatch coordinator reached "complete" (it
    // decided dev_via_test → OutcomeSuccess), while the rule-02e recovery
    // coordinator is non-complete (executing/failed). /loops is populated by a
    // separate dispatch subscriber from the rule engine that drives the run to
    // failed, so poll rather than read once (avoids a lagging-tracker flake
    // under CI load).
    // -----------------------------------------------------------------
    const loopsEvidence = await pollUntil(async () => {
      const loops = (await request
        .get("/teams-dispatch/loops")
        .then((r) => r.json())) as Array<{ role: string; state: string }>;
      const cbg = loops.find((l) => l.role === "reviewer-dev-via-test");
      const recovery = loops.find(
        (l) => l.role === "coordinator" && l.state !== "complete",
      );
      return cbg && recovery ? { loops, cbg, recovery } : null;
    }, { timeoutMs: 15_000 });

    expect(
      loopsEvidence,
      "expected a reviewer-dev-via-test loop (the plan-review CBG that rejected the plan → rule 02e) AND a non-complete coordinator-role loop (the rule-02e recovery coordinator, wedged to a max_iterations failure). Their absence means rule 02e never spawned the threaded recovery coordinator.",
    ).toBeTruthy();
  });
});

/**
 * fetchTriples — read graph triples via GET /graph/triples. Mirrors the helper
 * in run-failed.spec.ts (ADR-053 run-phase assertions).
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
