import { test, expect } from "@playwright/test";
import { assertAnchorBornFirst, RUN_ANCHOR, LOOP_ANCHOR } from "./born_first";

/**
 * Journey: ADR-042 MVP-5 — coordinator → research-category arc →
 * coordinator return path, end-to-end against the MVP roster.
 *
 * Validates the substrate-plus-overlays architecture introduced by
 * ADR-042 §Phase 2 redesign:
 *
 *   - MVP-1 (PR #172): substrate singletons wired by
 *     configs/flow-bootstrap.json (clone: e2e-flow-bootstrap.json).
 *   - MVP-2 (PR #173): category-keyed rule pack in
 *     configs/rules/research/.
 *   - MVP-3 (PR #174): named persona bundles
 *     researcher-research-{plan,gather,synthesize}.
 *   - MVP-4 (PR #175): coordinator persona emits
 *     decide(action="research") instead of legacy delegate_research.
 *   - MVP-5 (this PR): mock-LLM Playwright journey closing the loop.
 *
 * Loop sequence:
 *
 *   Loop 0: coordinator (dispatch) → decide(research)
 *           → rule 01 fires → spawn researcher-research-plan
 *
 *   Loop A: researcher-research-plan → emit_plan → decide(gather)
 *           → rule 02 fires → spawn researcher-research-gather
 *
 *   Loop B: researcher-research-gather → read_loop_result →
 *           web_search → decide(synthesize)
 *           → rule 03 fires → spawn researcher-research-synthesize
 *
 *   Loop C: researcher-research-synthesize → read_loop_result →
 *           emit_research_artifact (rev 1 FULL) → decide(emit)
 *           → rule 04 fires → spawn reviewer-research
 *
 *   Loop D: reviewer-research → read_loop_result → decide(approved)
 *           → rule 07 fires → spawn coordinator (wake-up)
 *
 *   Loop E: coordinator (wake-up) → read_loop_result on Loop D →
 *           decide(respond_direct) → typed dispatch on user.response.*
 *           + rule 03b stamps coordinator.clarification.reply triple.
 *           Terminal — no further rules fire.
 *
 * Validates:
 *   - All SIX loops reach terminal complete state. Recovery rules
 *     05 (reviewer-rejected) and 06 (needs-clarification) are
 *     loaded but do NOT fire (reviewer-research approves rev 1
 *     directly).
 *   - Role distribution proves every rule fired in order:
 *       2 × coordinator                   (Loop 0 + Loop E)
 *       1 × researcher-research-plan      (Loop A — rule 01)
 *       1 × researcher-research-gather    (Loop B — rule 02)
 *       1 × researcher-research-synthesize (Loop C — rule 03)
 *       1 × reviewer-research             (Loop D — rule 04)
 *   - Wire-shape: Loop 0 is dispatch-spawned (default_role=coordinator
 *     resolves it; LoopInfo.role is empty on the wire per
 *     [[feedback_loopinfo_role_omitempty]]); every other loop carries
 *     an explicit role from its publish_agent spawn.
 *   - No approval gate on the research arc.
 *   - No seventh loop appears (settle assertion against accidental
 *     re-fire of rule 07 on the wake-up coordinator's terminal, or
 *     rule 05 on the reviewer's approved terminal).
 *
 * Required fixture: test/fixtures/journeys/research-mvp.yaml
 * Required config: configs/e2e-flow-bootstrap.json (mock-LLM clone
 *   of flow-bootstrap.json — same rule + persona wiring).
 * Compose profile: none beyond the agentic-e2e baseline.
 */

test.describe("ADR-042 MVP-5 — Research category mock-LLM journey", () => {
  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok(), "Backend not healthy — stack not running?").toBe(true);
  });

  // Six loops, autonomous chain (no approval gate, no recovery). 3
  // minutes covers mock-LLM scheduling jitter; budget kept generous
  // to absorb cold-start latency on slower CI.
  test.setTimeout(180_000);

  test("user prompt → full research-category arc → coordinator delivers reply", async ({
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
    // tool will classify it as action="research" and the rule chain
    // takes over.
    // -----------------------------------------------------------------
    await page.getByTestId("chat-input").fill(
      "Compare MQTT vs NATS for IoT edge deployments — which has lower latency on constrained ARM devices?",
    );
    await page.getByTestId("send-button").click();

    // -----------------------------------------------------------------
    // Step 3 — wait for all SIX loops to reach terminal complete
    // state. Rules involved:
    //   01 (coordinator → researcher-research-plan)
    //   02 (plan → gather)
    //   03 (gather → synthesize)
    //   04 (synthesize → reviewer-research)
    //   07 (reviewer-research approved → wake-up coordinator)
    //   03b (wake-up coordinator respond_direct → audit prose; no spawn)
    // Recovery rules 05 + 06 are loaded but do not fire (reviewer
    // approves rev 1 directly).
    // -----------------------------------------------------------------
    const allTerminal = await pollUntil(async () => {
      const resp = await request.get("/teams-dispatch/loops");
      if (!resp.ok()) return null;
      const list = (await resp.json()) as Array<{ state: string }>;
      // 6 chain loops + 1 ops-chain-observer. The ops rule (ADR-027) fires on
      // the RUN entity reaching a terminal phase, so it spawns after the chain
      // settles and rides in the same loop list.
      if (list.length !== 7) return null;
      return list.every((l) => l.state === "complete") ? list : null;
    }, { timeoutMs: 120_000 });

    expect(
      allTerminal,
      "expected all 6 loops to reach terminal complete state (coordinator dispatch + research arc ×4 + wake-up coordinator)",
    ).toBeTruthy();

    // -----------------------------------------------------------------
    // Step 4 — role distribution proves every rule fired in order.
    //
    // Wire-shape note: dispatch-spawned loops ride on
    // dispatch.default_role and don't get their role stamped back
    // onto the LoopInfo wire JSON; rule-spawned loops do. So Loop 0
    // (the dispatch coordinator) has empty role on the wire, and the
    // wake-up coordinator (Loop E — rule 07 publish_agent) carries
    // role="coordinator" explicitly.
    //
    // Expected role distribution (after default_role resolution to
    // "coordinator" for the empty-role dispatch loop):
    //   2 × coordinator                    (Loop 0 dispatch + Loop E rule 07)
    //   1 × researcher-research-plan       (Loop A — rule 01)
    //   1 × researcher-research-gather     (Loop B — rule 02)
    //   1 × researcher-research-synthesize (Loop C — rule 03)
    //   1 × reviewer-research              (Loop D — rule 04)
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
    const planCount = roles.filter((r) => r === "researcher-research-plan").length;
    const gatherCount = roles.filter((r) => r === "researcher-research-gather").length;
    const synthesizeCount = roles.filter((r) => r === "researcher-research-synthesize").length;
    const reviewerCount = roles.filter((r) => r === "reviewer-research").length;

    expect(
      coordinatorCount,
      `expected 2 coordinator loops (Loop 0 dispatch + Loop E rule 07 wake-up), got roles=${JSON.stringify(roles)}. Missing → rule 07 (research/07-reviewer-approved-to-coordinator) did not fire on reviewer-research(approved).`,
    ).toBe(2);
    expect(
      planCount,
      `expected 1 researcher-research-plan loop (Loop A via rule 01), got roles=${JSON.stringify(roles)}. Missing → rule 01 (research/01-coordinator-research-spawn) did not fire on coordinator(research).`,
    ).toBe(1);
    expect(
      gatherCount,
      `expected 1 researcher-research-gather loop (Loop B via rule 02), got roles=${JSON.stringify(roles)}. Missing → rule 02 (research/02-plan-to-gather) did not fire.`,
    ).toBe(1);
    expect(
      synthesizeCount,
      `expected 1 researcher-research-synthesize loop (Loop C via rule 03), got roles=${JSON.stringify(roles)}. Missing → rule 03 (research/03-gather-to-synthesize) did not fire.`,
    ).toBe(1);
    expect(
      reviewerCount,
      `expected 1 reviewer-research loop (Loop D via rule 04), got roles=${JSON.stringify(roles)}. Missing → rule 04 (research/04-synthesize-to-reviewer) did not fire.`,
    ).toBe(1);

    // -----------------------------------------------------------------
    // Step 5 — no approval gate. The research arc is fully
    // autonomous: chain reaches terminal → coordinator wakes →
    // delivers reply. No human-in-the-loop step.
    // -----------------------------------------------------------------
    const stillPendingAny = finalLoops.find(
      (l) => l.pending_approval != null,
    );
    expect(
      stillPendingAny,
      "no loop should ever require approval — research-category arc is autonomous under the MVP roster",
    ).toBeUndefined();

    // -----------------------------------------------------------------
    // Step 6 — settle assertion: no seventh loop appears. Two
    // regressions this catches:
    //   (a) rule 07 accidentally re-firing on its own wake-up
    //       coordinator's terminal — would loop infinitely;
    //   (b) rule 05 mis-firing on reviewer-research approved (rather
    //       than insufficient) — would spawn a recovery
    //       researcher-research-plan and produce 7+ loops.
    // -----------------------------------------------------------------
    await new Promise((r) => setTimeout(r, 2000));
    const settledList = await request
      .get("/teams-dispatch/loops")
      .then((r) => r.json()) as unknown[];
    expect(
      settledList.length,
      "no eighth loop should appear — rule 07 must not re-fire on the wake-up coordinator's terminal, rule 05 must not fire on approved (only on insufficient), and the ops observer must fire exactly once per run rather than once per terminal loop",
    ).toBe(7);

    // The ops observer must be present and clean. A truncated one means the
    // mock served it an off-allowlist decide (its fixture bucket is missing or
    // its match fingerprint drifted), which would also mean it burned its full
    // iteration budget on every journey in the suite.
    const opsLoops = (settledList as Array<{ role?: string; state?: string }>)
      .filter((l) => l.role === "ops-chain-observer");
    expect(
      opsLoops.length,
      "expected exactly one ops-chain-observer loop — the ops rule fires once per run, on the run entity's terminal phase",
    ).toBe(1);
    expect(
      opsLoops[0]?.state,
      "ops-chain-observer must reach complete. `failed` with reason=max_iterations means it never called decide and burned its whole budget hydrating — that is how the framework surfaces exhaustion (NOT `truncated`), and it is what the real-LLM smoke on 2026-08-20 hit",
    ).toBe("complete");

    // -----------------------------------------------------------------
    // Step 7 — typed user-response publish proof. Loop-count assertions
    // alone would silently pass a regression where dispatch's typed
    // user.response output is misconfigured — the chain still terminates
    // with 6 loops, but no typed response is published. message-logger
    // captures the canonical user.response.<channel_type>.<channel_id>
    // publish; semstreams#1090 tracks channel-ready delivery.
    // -----------------------------------------------------------------
    const respResp = await request.get(
      "/message-logger/entries?limit=500&subject=user.response.*",
    );
    expect(respResp.ok(), "/message-logger/entries returned non-OK").toBe(true);
    const respPayloads = (await respResp.json()) as Array<{
      subject: string;
      message_type: string;
    }>;
    expect(
      respPayloads.length,
      "expected at least one typed dispatch.user.response publish. Zero entries means dispatch's typed user.response output is misconfigured.",
    ).toBeGreaterThanOrEqual(1);
    expect(respPayloads.every((entry) => entry.subject.startsWith("user.response."))).toBe(true);
    expect(respPayloads.every((entry) => entry.message_type === "agentic.user_response.v1")).toBe(true);

    // -----------------------------------------------------------------
    // Step 8 — ADR-053 Phase 4a: the run reached `completed`. Direct
    // agent.run.phase assertion on the run entity (the design spike's
    // merge gate §D/§H). Research is the loop-spawned-chain path: its
    // reviewer carries agent.run.entity-id, rule 07 stamps
    // agent.run.outcome=success, and rule 03 transitions executing→
    // completed. `failed` here would mean the D3 zombie guard raced the
    // dispatched→executing transition.
    // -----------------------------------------------------------------
    const runPhases = await pollUntil(async () => {
      const phases = await fetchTriples(request, {
        predicate: "agent.run.phase",
        limit: 20,
      });
      const objs = phases
        .filter((t) => String(t.subject ?? "").includes("agent.chain.execution."))
        .map((t) => String(t.object));
      if (objs.includes("completed") || objs.includes("failed")) return objs;
      return null;
    }, { timeoutMs: 30_000 });
    expect(
      runPhases,
      "agent.run.phase never reached a terminal on the run entity (run stuck in dispatched/executing)",
    ).toBeTruthy();
    expect(runPhases, "run must reach completed (ADR-053 Phase 4a)").toContain(
      "completed",
    );
    expect(
      runPhases,
      "run must NOT be failed (D3 race / wrong terminal)",
    ).not.toContain("failed");

    // BORN-FIRST gate (ADR-055/056 must-exist flip, semteams#222) — TWO anchors.
    // (1) Run anchor: research/07 stamps agent.run.outcome=success on the
    //     chain.execution run anchor; prove it was born by the lifecycle Manager
    //     (carries agent.run.phase), not auto-vivified by the outcome marker.
    await assertAnchorBornFirst(request, {
      markerPredicate: "agent.run.outcome",
      markerObject: "success",
      envelopePredicate: RUN_ANCHOR.envelope,
      anchorSubstr: RUN_ANCHOR.substr,
      label: "research/07 run-outcome marker (run anchor)",
    });
    // (2) Plan-loop anchor: research/03a stamps research.gather.completed-subtopic
    //     onto the PLANNER's loop entity (agent.lineage.plan-loop-entity-id →
    //     agent.agentic-loop.execution.*); prove that loop was born by the
    //     agentic-loop graph writer (carries agent.loop.role spawn-identity), not
    //     auto-vivified by the gather marker.
    await assertAnchorBornFirst(request, {
      markerPredicate: "research.gather.completed-subtopic",
      envelopePredicate: LOOP_ANCHOR.envelope,
      anchorSubstr: LOOP_ANCHOR.substr,
      label: "research/03a gather.completed_subtopic (plan-loop anchor)",
    });
  });
});

/**
 * fetchTriples — read graph triples via GET /graph/triples (ADR-053 Phase 4a
 * run-phase assertion). Mirrors the helper in the other agentic journeys.
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
