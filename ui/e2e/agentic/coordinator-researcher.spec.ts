import { test, expect } from "@playwright/test";

/**
 * Journey: Coordinator → research-only chain → coordinator return path
 *
 * Slice 1c Piece 2 rewrite. Exercises the full Slice 1 + 1b + 1c
 * wiring end-to-end on mock-LLM:
 *
 *   Loop 0: coordinator (dispatch) → decide(delegate_research)
 *           → chainmode.Stamper writes chain.mode.classification="research_only"
 *           → rule 01 fires → spawn researcher-plan
 *
 *   Loop A: researcher-plan → emit_plan → decide(gather)
 *           → rule 04 fires → spawn researcher-gather
 *
 *   Loop B: researcher-gather → read_loop_result → query_entity →
 *           decide(synthesize) → rule 05 fires → spawn researcher-synthesize
 *
 *   Loop C: researcher-synthesize → read_loop_result →
 *           emit_research_artifact (rev 1 FULL) → decide(emit)
 *           → rule 01a fires → spawn reviewer-research
 *
 *   Loop D: reviewer-research → read_loop_result → decide(approved)
 *           → chain.TerminalStamper writes chain.terminal.* cluster
 *           → rule 04a fires → spawn coordinator (wake-up)
 *
 *   Loop E: coordinator (wake-up) → read_loop_result on Loop D →
 *           decide(respond_direct) → rule 03b fires → publish on
 *           user.response.* + stamp coordinator.user_reply triple.
 *           Terminal — no further rules fire.
 *
 * Validates:
 *   - All SIX loops reach terminal complete state. Recovery rule 02
 *     is loaded but does NOT fire (reviewer-research approves rev 1
 *     directly — recovery is exercised by the sibling
 *     research-mode-transition.spec.ts journey).
 *   - Role distribution proves every rule fired in order:
 *       2 × coordinator       (Loop 0 dispatch + Loop E rule 04a wake-up)
 *       1 × researcher-plan   (Loop A — rule 01 spawn)
 *       1 × researcher-gather (Loop B — rule 04 spawn)
 *       1 × researcher-synthesize (Loop C — rule 05 spawn)
 *       1 × reviewer-research (Loop D — rule 01a spawn)
 *   - Wire-shape: Loop 0 is dispatch-spawned (default_role=coordinator
 *     resolves it; LoopInfo.role is empty on the wire per
 *     [[feedback_loopinfo_role_omitempty]]); every other loop carries an
 *     explicit role from its publish_agent spawn.
 *   - No approval gate on the research arc.
 *   - No seventh loop appears (settle assertion against accidental
 *     re-fire of rule 04a or rule 02 on the approved terminal).
 *
 * Required fixture: test/fixtures/journeys/coordinator-researcher.yaml
 * Required config: configs/e2e-coordinator.json (Slice 1c upgrade
 *   loads the full coordinator + research-mode-transition rule set).
 * Compose profile: none beyond the agentic-e2e baseline.
 */

test.describe("Coordinator → Researcher → Coordinator return (Slice 1c)", () => {
  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok(), "Backend not healthy — stack not running?").toBe(true);
  });

  // Six loops, autonomous chain (no approval gate, no recovery). 3
  // minutes covers mock-LLM scheduling jitter; budget kept generous
  // to absorb cold-start latency on slower CI.
  test.setTimeout(180_000);

  test("user prompt → full research-only chain → coordinator delivers reply", async ({
    page,
    request,
  }) => {
    // -----------------------------------------------------------------
    // Step 1 — open the Board so the SSE activity stream is connected
    // before any loops appear.
    // -----------------------------------------------------------------
    await page.goto("/");

    await expect(page.getByTestId("connection-status")).toHaveAttribute(
      "data-summary",
      "healthy",
      { timeout: 15000 },
    );
    await expect(page.getByTestId("kanban-board")).toBeVisible();

    // -----------------------------------------------------------------
    // Step 2 — send a research-worthy prompt. The coordinator's decide
    // tool will classify it as delegate_research and the rule chain
    // takes over.
    // -----------------------------------------------------------------
    await page.getByTestId("chat-input").fill(
      "Compare MQTT vs NATS for IoT edge deployments — which has lower latency on constrained ARM devices?",
    );
    await page.getByTestId("send-button").click();

    // -----------------------------------------------------------------
    // Step 3 — wait for all SIX loops to reach terminal complete
    // state. Rules involved:
    //   01 (coordinator → researcher-plan)
    //   04 (plan → gather)
    //   05 (gather → synthesize)
    //   01a (synthesize → reviewer-research)
    //   04a (reviewer-research approved → wake-up coordinator)
    //   03b (wake-up coordinator respond_direct → publish prose; no spawn)
    // Recovery rule 02 is loaded but does not fire (reviewer approves
    // rev 1 directly).
    // -----------------------------------------------------------------
    const allTerminal = await pollUntil(async () => {
      const resp = await request.get("/teams-dispatch/loops");
      if (!resp.ok()) return null;
      const list = (await resp.json()) as Array<{ state: string }>;
      if (list.length !== 6) return null;
      return list.every((l) => l.state === "complete") ? list : null;
    }, { timeoutMs: 120_000 });

    expect(
      allTerminal,
      "expected all 6 loops to reach terminal complete state (coordinator dispatch + researcher chain ×4 + wake-up coordinator)",
    ).toBeTruthy();

    // -----------------------------------------------------------------
    // Step 4 — role distribution proves every rule fired in order.
    //
    // Wire-shape note: dispatch-spawned loops ride on
    // dispatch.default_role and don't get their role stamped back
    // onto the LoopInfo wire JSON; rule-spawned loops do. So Loop 0
    // (the dispatch coordinator) has empty role on the wire, and the
    // wake-up coordinator (Loop E — rule 04a publish_agent) carries
    // role="coordinator" explicitly.
    //
    // Expected role distribution (after default_role resolution to
    // "coordinator" for the empty-role dispatch loop):
    //   2 × coordinator       (Loop 0 dispatch + Loop E rule 04a)
    //   1 × researcher-plan   (Loop A — rule 01)
    //   1 × researcher-gather (Loop B — rule 04)
    //   1 × researcher-synthesize (Loop C — rule 05)
    //   1 × reviewer-research (Loop D — rule 01a)
    // -----------------------------------------------------------------
    const finalLoops = await request
      .get("/teams-dispatch/loops")
      .then((r) => r.json()) as Array<{
        role: string;
        state: string;
        pending_approval?: { tool_name?: string } | null;
      }>;

    const dispatchLoops = finalLoops.filter((l) => !l.role);
    expect(
      dispatchLoops.length,
      `expected exactly 1 dispatch-spawned loop with empty role (Loop 0); got ${dispatchLoops.length}`,
    ).toBe(1);

    const expectedDefaultRole = "coordinator";
    const roles = finalLoops.map((l) => l.role || expectedDefaultRole);
    const coordinatorCount = roles.filter((r) => r === "coordinator").length;
    const planCount = roles.filter((r) => r === "researcher-plan").length;
    const gatherCount = roles.filter((r) => r === "researcher-gather").length;
    const synthesizeCount = roles.filter((r) => r === "researcher-synthesize").length;
    const reviewerCount = roles.filter((r) => r === "reviewer-research").length;

    expect(
      coordinatorCount,
      `expected 2 coordinator loops (Loop 0 dispatch + Loop E rule 04a wake-up), got roles=${JSON.stringify(roles)}. Missing → rule 04a (research-terminal-to-coordinator) did not fire on reviewer-research(approved).`,
    ).toBe(2);
    expect(
      planCount,
      `expected 1 researcher-plan loop (Loop A via rule 01), got roles=${JSON.stringify(roles)}`,
    ).toBe(1);
    expect(
      gatherCount,
      `expected 1 researcher-gather loop (Loop B via rule 04), got roles=${JSON.stringify(roles)}. Missing → rule 04 (phase-transition-to-gather) did not fire.`,
    ).toBe(1);
    expect(
      synthesizeCount,
      `expected 1 researcher-synthesize loop (Loop C via rule 05), got roles=${JSON.stringify(roles)}. Missing → rule 05 (phase-transition-to-synthesize) did not fire.`,
    ).toBe(1);
    expect(
      reviewerCount,
      `expected 1 reviewer-research loop (Loop D via rule 01a), got roles=${JSON.stringify(roles)}. Missing → rule 01a (researcher-synthesize-to-reviewer-research) did not fire.`,
    ).toBe(1);

    // -----------------------------------------------------------------
    // Step 5 — no approval gate. The Slice 1c return path is fully
    // autonomous: chain reaches terminal → coordinator wakes →
    // delivers reply. No human-in-the-loop step.
    // -----------------------------------------------------------------
    const stillPendingAny = finalLoops.find(
      (l) => l.pending_approval != null,
    );
    expect(
      stillPendingAny,
      "no loop should ever require approval — Slice 1c return path is autonomous",
    ).toBeUndefined();

    // -----------------------------------------------------------------
    // Step 6 — settle assertion: no seventh loop appears. Two
    // regressions this catches:
    //   (a) rule 04a accidentally re-firing on its own wake-up
    //       coordinator's terminal — would loop infinitely;
    //   (b) rule 02 mis-firing on reviewer-research approved (rather
    //       than insufficient) — would spawn a recovery
    //       researcher-plan and produce 7+ loops.
    // -----------------------------------------------------------------
    await new Promise((r) => setTimeout(r, 2000));
    const settledList = await request
      .get("/teams-dispatch/loops")
      .then((r) => r.json()) as unknown[];
    expect(
      settledList.length,
      "no seventh loop should appear — rule 04a must not re-fire on the wake-up coordinator's terminal, and rule 02 must not fire on approved (only on insufficient)",
    ).toBe(6);
  });
});

/**
 * pollUntil — race a polling fn against a deadline.
 * Returns the fn's value on success, or null on deadline.
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
