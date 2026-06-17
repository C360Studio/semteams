import { test, expect } from "@playwright/test";
import { assertAnchorBornFirst, RUN_ANCHOR } from "./born_first";

/**
 * Journey: ADR-053 Phase 4a′ — executing→failed run transition.
 *
 * The merge gate for the failure edge. A coordinator dispatches research,
 * then the planner (researcher-research-plan) is WEDGED to an involuntary
 * failure (its fixture bucket serves a repeating non-terminal `scratchpad`
 * call, so it never decides and trips the loop's max_iterations →
 * OutcomeFailed). The substrate must drive the run to `failed`:
 *
 *   research/08-loop-failed-pause   → chain.paused.marker on the planner loop
 *   research/09-loop-failed-run-outcome → agent.run.outcome=failed on the run
 *                                          entity (anchor agent.run.entity_id)
 *   agent-run/04-executing-to-failed → executing→failed
 *
 * Why this proves rule 04 and NOT the upstream D3 zombie guard: D3 fires only
 * when the dispatch ROOT terminates while the run is still `dispatched`. Here
 * the dispatch coordinator completed successfully with a confirmed handoff
 * (the run is `executing`), and the NON-root planner loop is what fails. The
 * decisive assertion is agent.run.last_transition_from == "executing" — D3's
 * path would record a from-phase of "dispatched".
 *
 * Required fixture: test/fixtures/journeys/run-failed.yaml
 * Required config: configs/e2e-flow-bootstrap.json
 */

test.describe("ADR-053 Phase 4a′ — executing→failed run transition", () => {
  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok(), "Backend not healthy — stack not running?").toBe(true);
  });

  // Two loops; the planner wedges to max_iterations (8 mock round-trips)
  // before failing. 2 minutes covers mock scheduling jitter + cold start.
  test.setTimeout(120_000);

  test("planner fails while executing → run transitions executing→failed", async ({
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
    // Step 2 — send a research-worthy prompt. The coordinator classifies it
    // action="research"; research/01 spawns the planner, which is wedged.
    // -----------------------------------------------------------------
    await page.getByTestId("chat-input").fill(
      "Compare MQTT vs NATS for IoT edge deployments — which has lower latency on constrained ARM devices?",
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
    }, { timeoutMs: 90_000 });

    expect(
      runPhases,
      "agent.run.phase never reached a terminal on the run entity (run stuck in dispatched/executing — the planner failure did not drive executing→failed)",
    ).toBeTruthy();
    expect(
      runPhases,
      "run must reach failed (ADR-053 4a′: research/09 stamps agent.run.outcome=failed → agent-run/04 executing→failed). Got: " +
        JSON.stringify(runPhases),
    ).toContain("failed");
    expect(
      runPhases,
      "run must NOT complete — the planner loop failed, no reviewer ever approved",
    ).not.toContain("completed");

    // -----------------------------------------------------------------
    // Step 4 — DECISIVE: the transition is executing→failed (rule 04), NOT
    // D3's dispatched→failed. agent.run.last_transition_from records the
    // from-phase of the most recent transition (beta.103 REPLACEs it).
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
      "the failed transition must come FROM executing (rule 04, agent-run/04-executing-to-failed) — a from-phase of 'dispatched' would mean the D3 zombie guard fired on the dispatched root instead. Got: " +
        JSON.stringify(fromPhases),
    ).toContain("executing");

    // -----------------------------------------------------------------
    // Step 4b — BORN-FIRST gate (ADR-055/056 must-exist flip, semteams#222).
    // research/09 stamps agent.run.outcome onto the run anchor via the
    // agent.run.entity_id anchor. Prove that anchor was BORN-FIRST (by the
    // lifecycle Manager) and not auto-vivified by the marker: the same entity
    // must carry agent.run.phase. Fails under simulated auto-vivify (a stub
    // would carry only agent.run.outcome).
    // -----------------------------------------------------------------
    await assertAnchorBornFirst(request, {
      markerPredicate: "agent.run.outcome",
      markerObject: "failed",
      envelopePredicate: RUN_ANCHOR.envelope,
      anchorSubstr: RUN_ANCHOR.substr,
      label: "agent-run/09 run-outcome marker (run anchor)",
    });

    // -----------------------------------------------------------------
    // Step 5 — research/08-loop-failed-pause fired: chain.paused.marker is
    // stamped on the failed planner loop (the operator surface, which the
    // run-outcome rule is the companion to).
    // -----------------------------------------------------------------
    const pauseMarkers = await fetchTriples(request, {
      predicate: "chain.paused.marker",
      limit: 20,
    });
    expect(
      pauseMarkers.length,
      "expected a chain.paused.marker on the failed planner loop (research/08-loop-failed-pause). Zero → the loop-failed-pause rule did not fire on outcome=failed.",
    ).toBeGreaterThanOrEqual(1);

    // -----------------------------------------------------------------
    // Step 6 — exactly two loops: coordinator(dispatch, complete) + planner
    // (failed). No gather/synthesize/reviewer — the planner never decided
    // gather, so research/02 never fired.
    // -----------------------------------------------------------------
    const loops = (await request
      .get("/teams-dispatch/loops")
      .then((r) => r.json())) as Array<{ role: string; state: string }>;
    expect(
      loops.length,
      "expected exactly 2 loops (coordinator dispatch + wedged planner); a third loop means the planner spawned downstream despite failing. Got: " +
        JSON.stringify(loops.map((l) => ({ role: l.role || "(dispatch)", state: l.state }))),
    ).toBe(2);

    const planner = loops.find((l) => l.role === "researcher-research-plan");
    expect(
      planner,
      "expected one researcher-research-plan loop (the wedged planner)",
    ).toBeTruthy();
    expect(
      planner?.state,
      "the planner loop must be in a terminal non-complete state (it failed at max_iterations). Got: " +
        String(planner?.state),
    ).not.toBe("complete");
  });
});

/**
 * fetchTriples — read graph triples via GET /graph/triples. Mirrors the helper
 * in the other agentic journeys (ADR-053 run-phase assertions).
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
