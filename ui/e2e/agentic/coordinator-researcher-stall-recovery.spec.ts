import { test, expect } from "@playwright/test";

/**
 * Journey: Coordinator → research_only chain → synthesize drifts to
 * architect → chainstall recovery → second arc converges → wake-up
 * coordinator delivers user reply.
 *
 * Slice 2 Piece 2. Builds on coordinator-researcher.spec.ts's happy
 * path by injecting a deliberate synthesize→architect drift on the
 * first arc and asserting that:
 *
 *   - cmd/semteams/phasevalidator rejects the drift with mode_mismatch
 *     (stamps chain.phase_transition.{rejected,reject_reason} on the
 *     Chain 1 chain entity).
 *   - cmd/semteams/chainstall observes the rejection, increments the
 *     chain.rejection.recovery_count cap, stamps the chain.stall.*
 *     audit cluster on the Chain 1 chain entity, and writes the
 *     per-loop chain.stall.proceed sentinel on the Loop C producer.
 *   - rule 05 (configs/rules/coordinator/05-chain-stall-to-coordinator.json)
 *     fires on Loop C's chain.stall.proceed=="true" and spawns a fresh
 *     coordinator wake-up (Loop D coord-2).
 *   - Loop D coord-2 re-dispatches via decide(delegate_research) →
 *     rule 01 → researcher-plan-2 → research arc retries → reviewer-
 *     research approves → rule 04a wakes Loop E coord-3 → rule 03b
 *     publishes user reply.
 *   - Chainstall is idempotent under at-least-once NATS redelivery: no
 *     eleventh loop appears even if the agent.complete event for Loop C
 *     replays (the chain.stall.producer_loop_id guard short-circuits).
 *
 * Expected loops (10 total):
 *
 *   1. Loop 0 (dispatch)        coordinator      decide(delegate_research)
 *   2. Loop A                   researcher-plan-1 emit_plan + decide(gather)
 *   3. Loop B                   researcher-gather-1 read + query + decide(synthesize)
 *   4. Loop C                   researcher-synthesize-1 emit_research_artifact + decide(architect)
 *      ← chainstall fires; rule 05 spawns Loop D coord-2
 *   5. Loop D                   coordinator      read + decide(delegate_research)
 *   6. Loop A'                  researcher-plan-2 emit_plan + decide(gather)
 *   7. Loop B'                  researcher-gather-2 read + query + decide(synthesize)
 *   8. Loop C'                  researcher-synthesize-2 emit_research_artifact + decide(emit)
 *   9. Loop D'                  reviewer-research read + decide(approved)
 *      ← TerminalStamper fires; rule 04a spawns Loop E coord-3
 *  10. Loop E                   coordinator      read + decide(respond_direct)
 *
 * Required fixture: test/fixtures/journeys/coordinator-researcher-stall-recovery.yaml
 * Required config:  configs/e2e-coordinator.json (Slice 2 Piece 1 wires
 *                   rule 05 — `05-chain-stall-to-coordinator.json` —
 *                   into rules_files; no further config change needed).
 * Compose profile:  none beyond the agentic-e2e baseline.
 */

// SKIPPED 2026-05-16 — ADR-041 addendum 2026-05-16 §"Synthesize
// action_allowlist must be mode-aware" makes the drift scenario this
// journey exercises structurally impossible on real and mock LLMs
// alike. Rule 05a (research_only) spawns synthesize with
// action_allowlist = ["gather", "emit", "needs_clarification"] — the
// upstream decide tool's allowlist enforcement
// (semstreams/processor/agentic-tools/decide.go:219) returns
// InvalidArgs for decide(action="architect") on this loop, so the
// synthesize loop never terminates with the architect terminal,
// phasevalidator's modeGateRejects never fires, and chainstall has
// no mode_mismatch event to detect.
//
// The chainstall substrate (Slice 2 Piece 1, PR #159) is still
// load-bearing for:
//
//   1. Other reject reasons (phase_cap, invalid_edge,
//      missing_research_artifact, zero_tests_run, etc.) — detection
//      wiring per Finding 3 follow-up (issue #161).
//   2. Legacy chains without chain.mode (the validator defaults the
//      mirror to dev_via_spec, but legacy configs without a
//      coordinator front-door could still emit out-of-allowlist
//      actions if the rule defaults shifted).
//
// Unit-test coverage holds:
//
//   - cmd/semteams/chainstall/subscriber_test.go — detectRejection
//     + audit cluster + idempotency
//   - test/contract/chain_stall_routing_test.go — routing table
//     pins mode_mismatch → coordinator
//   - test/contract/adr041_persona_dirs_test.go
//     TestADR041_SynthesizeAllowlistMatchesMode — pins the rule split
//     + allowlist invariants that close the drift class this journey
//     used to exercise.
//
// The journey can be reworked when chainstall detection extends to
// phase_cap (Finding 3) — that reject reason CAN be reached even
// with the new allowlist enforcement, and exercises a different
// branch of the routing table (RouteHuman). For now the addendum
// removes the load-bearing case this journey was minted to cover;
// the unit + contract tests carry the structural regression
// guarantees forward.
test.describe.skip("Coordinator → Researcher → chainstall → Coordinator (Slice 2)", () => {
  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok(), "Backend not healthy — stack not running?").toBe(true);
  });

  // Ten loops, two LLM-driven arcs separated by a chainstall recovery.
  // 4 minutes covers mock-llm scheduling jitter + the rule-engine
  // round-trip for the chainstall → rule 05 wake-up; budget kept
  // generous to absorb cold-start latency on slower CI.
  test.setTimeout(240_000);

  test("synthesize drifts to architect → chainstall routes to coordinator → second arc converges", async ({
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
    // Step 2 — send the research-worthy prompt. The coordinator's
    // decide tool will classify it as delegate_research; the first
    // researcher-synthesize will drift to decide(architect); chainstall
    // will route the recovery to a second coordinator wake-up.
    // -----------------------------------------------------------------
    await page.getByTestId("chat-input").fill(
      "Compare MQTT vs NATS for IoT edge deployments — which has lower latency on constrained ARM devices?",
    );
    await page.getByTestId("send-button").click();

    // -----------------------------------------------------------------
    // Step 3 — wait for all TEN loops to reach terminal complete
    // state. Rules involved across the two arcs:
    //   01  (coordinator → researcher-plan)        ×2 (Loop A, Loop A')
    //   04  (plan → gather)                        ×2 (Loop B, Loop B')
    //   05p (gather → synthesize — phase transition) ×2 (Loop C, Loop C')
    //   01a (synthesize → reviewer-research)       ×1 (only Loop C'; Loop C
    //         was rejected by phasevalidator under chain.mode=research_only)
    //   05s (chain-stall → coordinator wake-up)    ×1 (Loop D from Loop C)
    //   04a (reviewer-research approved → coordinator wake-up) ×1 (Loop E)
    //   03b (wake-up coordinator respond_direct → publish prose) ×1 (Loop E)
    // -----------------------------------------------------------------
    const allTerminal = await pollUntil(async () => {
      const resp = await request.get("/teams-dispatch/loops");
      if (!resp.ok()) return null;
      const list = (await resp.json()) as Array<{ state: string }>;
      if (list.length !== 10) return null;
      return list.every((l) => l.state === "complete") ? list : null;
    }, { timeoutMs: 180_000 });

    expect(
      allTerminal,
      "expected all 10 loops to reach terminal complete state (coordinator dispatch + researcher chain×4 + chainstall wake-up + researcher chain×4 + chain-terminal wake-up). Missing → suspect chainstall did not stamp chain.stall.proceed on Loop C OR rule 05 (configs/rules/coordinator/05-chain-stall-to-coordinator.json) did not fire.",
    ).toBeTruthy();

    // -----------------------------------------------------------------
    // Step 4 — role distribution proves every rule fired in order.
    //
    // Wire-shape note (same as coordinator-researcher.spec.ts): dispatch-
    // spawned loops ride on dispatch.default_role and don't get role
    // stamped back onto the LoopInfo wire JSON. Loop 0 has empty role;
    // every rule-spawned loop carries an explicit role from its
    // publish_agent spawn.
    //
    // Expected role distribution (after default_role resolution to
    // "coordinator" for the empty-role dispatch loop):
    //   3 × coordinator           (Loop 0 dispatch + Loop D chainstall +
    //                              Loop E chain-terminal)
    //   2 × researcher-plan       (Loop A + Loop A')
    //   2 × researcher-gather     (Loop B + Loop B')
    //   2 × researcher-synthesize (Loop C + Loop C')
    //   1 × reviewer-research     (Loop D')
    // -----------------------------------------------------------------
    const finalLoops = await request
      .get("/teams-dispatch/loops")
      .then((r) => r.json()) as Array<{
        loop_id: string;
        role: string;
        state: string;
        pending_approval?: { tool_name?: string } | null;
      }>;

    const dispatchLoops = finalLoops.filter((l) => !l.role);
    expect(
      dispatchLoops.length,
      `expected exactly 1 dispatch-spawned loop with empty role (Loop 0); got ${dispatchLoops.length}. Wake-up coordinators (Loop D + Loop E) carry explicit role=coordinator from their publish_agent rules.`,
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
      `expected 3 coordinator loops (Loop 0 dispatch + Loop D chainstall wake-up + Loop E chain-terminal wake-up), got roles=${JSON.stringify(roles)}. Missing → rule 05 (chain-stall) OR rule 04a (chain-terminal) did not fire.`,
    ).toBe(3);
    expect(
      planCount,
      `expected 2 researcher-plan loops (Loop A + Loop A' across both arcs), got roles=${JSON.stringify(roles)}. Missing → rule 01 (coordinator-delegate-research) did not re-fire on the recovery coordinator.`,
    ).toBe(2);
    expect(
      gatherCount,
      `expected 2 researcher-gather loops (Loop B + Loop B'), got roles=${JSON.stringify(roles)}. Missing → rule 04 (phase-transition-to-gather) did not fire on the retry arc.`,
    ).toBe(2);
    expect(
      synthesizeCount,
      `expected 2 researcher-synthesize loops (Loop C + Loop C'), got roles=${JSON.stringify(roles)}. Missing → rule 05 (phase-transition-to-synthesize) did not fire on the retry arc.`,
    ).toBe(2);
    expect(
      reviewerCount,
      `expected 1 reviewer-research loop (Loop D' via rule 01a — only the second synthesize emits, the first is rejected by phasevalidator), got roles=${JSON.stringify(roles)}.`,
    ).toBe(1);

    // -----------------------------------------------------------------
    // Step 5 — no approval gate. Slice 2 Piece 1 routes mode_mismatch
    // stalls to the coordinator wake-up path (RouteCoordinator), NOT
    // the human-approval path. chain.stall.awaiting_human must NOT be
    // stamped on the chain entity for this fixture; pending_approval
    // must be null on every loop.
    // -----------------------------------------------------------------
    const stillPendingAny = finalLoops.find(
      (l) => l.pending_approval != null,
    );
    expect(
      stillPendingAny,
      "no loop should ever require approval — Slice 2 routes mode_mismatch to coordinator, not human",
    ).toBeUndefined();

    // -----------------------------------------------------------------
    // Step 6 — assert the chainstall audit cluster landed on the
    // Chain 1 chain entity.
    //
    // chain.rejection.recovery_count == "1" → exactly one stall cycle
    //   fired (Loop C drift); the retry arc did NOT stall.
    // chain.stall.routed_to == "coordinator" → RouteCoordinator was
    //   selected per reasonToRoute for mode_mismatch.
    // chain.stall.reason == "mode_mismatch" → the reject-reason token
    //   chainstall observed from phasevalidator.modeGateRejects.
    // chain.stall.producer_role == "researcher-synthesize" → the
    //   producer role chainstall fired on (the only role in
    //   roleAcceptingStall today).
    // -----------------------------------------------------------------
    const stallReasonTriples = await pollUntil(async () => {
      const triples = await fetchTriples(request, {
        predicate: "chain.stall.reason",
        object: "mode_mismatch",
        limit: 20,
      });
      return triples.length >= 1 ? triples : null;
    }, { timeoutMs: 30_000 });
    expect(
      stallReasonTriples,
      "expected ≥1 chain.stall.reason=='mode_mismatch' triple (chainstall.stampAuditCluster on chain entity + stampPerLoopSignals on producer loop entity). Missing → chainstall.detectRejection did not classify the synthesize→architect terminal under research_only as mode_mismatch.",
    ).toBeTruthy();

    const recoveryCountTriples = await fetchTriples(request, {
      predicate: "chain.rejection.recovery_count",
      object: "1",
      limit: 10,
    });
    expect(
      recoveryCountTriples.length,
      `expected exactly 1 chain.rejection.recovery_count=="1" triple (one stall cycle). Got ${recoveryCountTriples.length}: ${JSON.stringify(recoveryCountTriples)}. >1 → chainstall double-incremented (idempotency guard broken); 0 → chainstall did not stamp the cap counter.`,
    ).toBe(1);

    const routedToTriples = await fetchTriples(request, {
      predicate: "chain.stall.routed_to",
      object: "coordinator",
      limit: 10,
    });
    expect(
      routedToTriples.length,
      `expected exactly 1 chain.stall.routed_to=="coordinator" triple. Got ${routedToTriples.length}. mode_mismatch routes to coordinator per reasonToRoute; if 0 → chainstall took the human route (check chain.stall.awaiting_human) or cap escalation; if >1 → idempotency guard regression.`,
    ).toBe(1);

    const producerRoleTriples = await fetchTriples(request, {
      predicate: "chain.stall.producer_role",
      object: "researcher-synthesize",
      limit: 10,
    });
    expect(
      producerRoleTriples.length,
      "expected exactly 1 chain.stall.producer_role=='researcher-synthesize' triple — the producer role chainstall observed on the rejected loop completion.",
    ).toBe(1);

    // -----------------------------------------------------------------
    // Step 7 — assert chain.stall.exhausted is NOT stamped (single
    // stall cycle, cap=3). exhausted only fires when newCount > threshold.
    // -----------------------------------------------------------------
    const exhaustedTriples = await fetchTriples(request, {
      predicate: "chain.rejection.exhausted",
      object: "true",
      limit: 10,
    });
    expect(
      exhaustedTriples.length,
      "chain.rejection.exhausted must NOT be stamped — only one stall cycle, default threshold is 3. If stamped → chainstall.threshold defaulting wrong or counter math regression.",
    ).toBe(0);

    // -----------------------------------------------------------------
    // Step 8 — assert the human-route signals are NOT stamped.
    // mode_mismatch routes coordinator; awaiting_human is reserved for
    // RouteHuman reasons.
    // -----------------------------------------------------------------
    const awaitingHumanTriples = await fetchTriples(request, {
      predicate: "chain.stall.awaiting_human",
      object: "true",
      limit: 10,
    });
    expect(
      awaitingHumanTriples.length,
      "chain.stall.awaiting_human must NOT be stamped — mode_mismatch is a coordinator-route reason. If stamped → routing table regression.",
    ).toBe(0);

    // -----------------------------------------------------------------
    // Step 9 — assert the producer-loop sentinel landed. Rule 05's
    // condition is chain.stall.proceed=="true"; without this triple
    // the wake-up never fires.
    // -----------------------------------------------------------------
    const proceedTriples = await fetchTriples(request, {
      predicate: "chain.stall.proceed",
      object: "true",
      limit: 10,
    });
    expect(
      proceedTriples.length,
      "expected exactly 1 chain.stall.proceed=='true' triple — the per-loop sentinel chainstall.stampPerLoopSignals writes on the producer loop entity. Missing → rule 05 (chain-stall) had no condition to fire on.",
    ).toBe(1);

    // -----------------------------------------------------------------
    // Step 10 — assert the upstream phasevalidator rejection cluster
    // also landed. The two predicates (chain.phase_transition.* and
    // chain.stall.*) live in parallel on the same chain entity — one
    // is the structural-gate writer, the other is chainstall's audit.
    // -----------------------------------------------------------------
    const phaseRejectReasonTriples = await fetchTriples(request, {
      predicate: "chain.phase_transition.reject_reason",
      object: "mode_mismatch",
      limit: 10,
    });
    expect(
      phaseRejectReasonTriples.length,
      "expected exactly 1 chain.phase_transition.reject_reason=='mode_mismatch' (phasevalidator.modeGateRejects on Loop C). Missing → phasevalidator did not see the synthesize→architect drift under research_only.",
    ).toBe(1);

    // -----------------------------------------------------------------
    // Step 11 — assert chain.terminal.reached on Chain 2. The recovery
    // arc converges, reviewer-research approves, TerminalStamper writes
    // chain.terminal.* on Chain 2's chain entity, rule 04a spawns Loop E.
    // -----------------------------------------------------------------
    const terminalReachedTriples = await fetchTriples(request, {
      predicate: "chain.terminal.reached",
      object: "true",
      limit: 10,
    });
    expect(
      terminalReachedTriples.length,
      "expected exactly 1 chain.terminal.reached=='true' triple (TerminalStamper on Chain 2's reviewer-research approval; Chain 1 stalled at synthesize and never terminates). >1 → terminal stamper fired twice (regression); 0 → reviewer-research(approved) did not trigger the stamper.",
    ).toBe(1);

    // -----------------------------------------------------------------
    // Step 12 — assert chain.mode.classification=='research_only' on
    // both chain entities (Chain 1 from Loop 0 dispatch + Chain 2 from
    // Loop D wake-up dispatch). chainmode.Stamper fires on every
    // coordinator-completion with delegate_research/delegate_dev_chain.
    // -----------------------------------------------------------------
    const chainModeTriples = await fetchTriples(request, {
      predicate: "chain.mode.classification",
      object: "research_only",
      limit: 10,
    });
    expect(
      chainModeTriples.length,
      `expected ≥2 chain.mode.classification=='research_only' triples (Chain 1 from Loop 0 + Chain 2 from Loop D). Got ${chainModeTriples.length}. <2 → chainmode.Stamper did not fire on the wake-up coordinator's delegate_research terminal.`,
    ).toBeGreaterThanOrEqual(2);

    // -----------------------------------------------------------------
    // Step 13 — assert coordinator.user_reply audit triple stamped
    // (rule 03b on Loop E coord-3's respond_direct terminal). This is
    // the Slice 1c return-path's user-facing audit.
    // -----------------------------------------------------------------
    const userReplyTriples = await fetchTriples(request, {
      predicate: "coordinator.user_reply",
      limit: 10,
    });
    expect(
      userReplyTriples.length,
      "expected ≥1 coordinator.user_reply triple (rule 03b on Loop E coord-3). Missing → Slice 1c return path did not fire on the wake-up coordinator's respond_direct.",
    ).toBeGreaterThanOrEqual(1);

    // -----------------------------------------------------------------
    // Step 14 — settle assertion: no eleventh loop appears.
    //
    // Two regressions this catches:
    //   (a) chainstall's idempotency guard (chain.stall.producer_loop_id
    //       == ev.LoopID short-circuit) breaks under NATS at-least-once
    //       redelivery → would double-fire rule 05 and spawn a second
    //       coord-2 wake-up;
    //   (b) rule 05 (chain-stall) accidentally re-fires when the retry
    //       arc's researcher-synthesize-2 completes cleanly — would
    //       spawn a phantom wake-up coordinator on the happy path.
    // -----------------------------------------------------------------
    await new Promise((r) => setTimeout(r, 2000));
    const settledList = await request
      .get("/teams-dispatch/loops")
      .then((r) => r.json()) as unknown[];
    expect(
      settledList.length,
      "no eleventh loop should appear — chainstall's idempotency guard (producer_loop_id check) must prevent re-firing on redelivery, and rule 05 must not fire on the retry arc's clean synthesize.",
    ).toBe(10);
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

interface Triple {
  subject: string;
  predicate: string;
  object: string;
  source?: string;
  timestamp?: string;
}

/**
 * fetchTriples queries the framework's GET /graph/triples endpoint
 * (semstreams beta.9 added on the service-manager mux). Same helper now
 * duplicated across four specs (dev-via-spec, dev-via-spec-qa,
 * ops-agent-baseline, this file) — the threshold the original
 * "promote on a fourth spec" comment named has been crossed. Deferred
 * out of Piece 2 scope; the cleanup is a focused PR rather than mixed
 * into a journey fixture.
 */
async function fetchTriples(
  request: import("@playwright/test").APIRequestContext,
  params: { subject?: string; predicate?: string; object?: string; limit?: number },
): Promise<Triple[]> {
  const query = new URLSearchParams();
  if (params.subject) query.set("subject", params.subject);
  if (params.predicate) query.set("predicate", params.predicate);
  if (params.object) query.set("object", params.object);
  if (params.limit) query.set("limit", String(params.limit));

  const resp = await request.get(`/graph/triples?${query.toString()}`);
  if (!resp.ok()) {
    throw new Error(
      `GET /graph/triples?${query.toString()} returned ${resp.status()}: ${await resp.text()}`,
    );
  }
  return (await resp.json()) as Triple[];
}
