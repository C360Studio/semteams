import { test, expect } from "@playwright/test";

/**
 * Journey: Dev-via-Spec QA-reviewer + ADR-039 Phase 1 recovery cycle
 *
 * Extends dev-via-spec.spec.ts with the post-build review hop AND the
 * full ADR-039 Phase 1 Shape A supersession recovery cycle when the
 * qa-reviewer rejects with needs_clarification. The first eleven loops
 * are identical to dev-via-spec.spec.ts. The difference at the tail:
 *
 *   - Loop K (dev-via-spec-builder) terminates with
 *     builder_decide(tests_passing) instead of needs_clarification.
 *     The mock-LLM does not actually run tests; the builder_decide
 *     tool emits coordinator.next_action=tests_passing
 *     unconditionally given the args, which triggers rule 07
 *     (R3.7.2.k′: builder→qa-reviewer).
 *
 *   - Loop L (dev-via-spec-qa-reviewer) reads the builder's terminal
 *     via read_loop_result and emits decide(needs_clarification).
 *     Per j′ 10-evaluation-contract.md Rule 3 this is the
 *     persona-correct verdict for the stub state where the spawn-rule
 *     prompt embeds a hardcoded "(stub)" evidence block.
 *
 *   - Loop M (dev-via-spec-architect, recovery) — ADR-039 Phase 1
 *     rule 09 fires on the qa-reviewer's needs_clarification and
 *     spawns a fresh architect. Per the rule's prompt, the recovery
 *     architect reads the qa-reviewer's terminal AND the prior
 *     research artifact, then re-emits the spec WITH evidence rules
 *     in checks (the gap qa-reviewer flagged). decide(tasks_emitted)
 *     re-fires rule 06 → recovery builder.
 *
 *   - Loop N (dev-via-spec-builder, recovery) — rule 06 re-spawn.
 *     bootstrap_workspace + bash + builder_decide(tests_passing).
 *     Rule 07 re-fires on tests_passing → recovery qa-reviewer.
 *
 *   - Loop O (dev-via-spec-qa-reviewer, recovery) — clean terminal.
 *     decide(action="accept") on the recovery spec's checks. No
 *     further rule fires (rule 09 only fires on needs_clarification).
 *     Chain settles at fifteen loops; ADR-039 Phase 1 Shape A
 *     supersession cycle complete end-to-end.
 *
 * What this proves:
 *   - Rule 07 (configs/rules/dev-via-spec/07-builder-decide-to-
 *     qa-reviewer.json) fires on coordinator.next_action=tests_passing
 *     and spawns the qa-reviewer role.
 *   - The qa-reviewer's persona (R3.7.2.j′) loads at boot.
 *   - ADR-039 Phase 1 rule 09 fires on qa-reviewer's needs_clarification
 *     and spawns the recovery architect with lineage.researcher forwarded.
 *   - Recovery architect's tasks_emitted re-fires rule 06 (verifies the
 *     "supersession via new spawn" mechanism — ADR-039 Shape A — works
 *     end-to-end without mutating prior loop entities).
 *   - Recovery builder's tests_passing re-fires rule 07.
 *   - Recovery qa-reviewer's accept terminates cleanly (rule 09 does
 *     NOT fire on accept).
 *   - Total chain: fifteen loops (eleven baseline + qa-reviewer +
 *     three-loop recovery cycle).
 *
 * What this does NOT prove:
 *   - Substantive evidence grading. The evidence summary in the
 *     spawn rule is a literal stub. Real evidence-rendering plumbing
 *     (rule action / tool / preprocessor) is deferred to a
 *     follow-on slice; project_smoke7_open_plumbing.md §2 records
 *     the lean choice.
 *   - Real builder running mvn test. Smoke #7 (R3.7.2.l′) is where
 *     real-LLM exercise lands.
 *
 * Required fixture: test/fixtures/journeys/dev-via-spec-qa.yaml
 * Required compose profiles: semsource (substrate-modifying retry
 * pass) and sandbox (R3.6.2 builder loop calls bootstrap_workspace
 * + bash via the sandbox container).
 */

test.describe("Dev-via-Spec QA-reviewer (R3.7.2.k′)", () => {
  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok()).toBe(true);
  });

  // Fifteen loops: twelve baseline + three recovery (architect, builder,
  // qa-reviewer accept). Allow 8 minutes — dev-via-spec.spec.ts's 360s
  // baseline plus ~30s for the original qa-reviewer plus ~90s for the
  // three-loop recovery cycle (recovery architect ~10 round-trips,
  // recovery builder ~4, recovery qa-reviewer ~2; mock-LLM is fast but
  // each loop adds rule-engine + graph-write hops).
  test.setTimeout(480_000);

  test("research → stabilisation → dev-via-spec planner / reviewer / challenger / architect / builder / qa-reviewer", async ({
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
      { timeout: 10000 },
    );

    await expect(page.getByTestId("kanban-board")).toBeVisible();

    // -----------------------------------------------------------------
    // Step 2 — type the bounded prompt. Identical to dev-via-spec's
    // prompt; the same six-loop research arc + dev-via-spec chain
    // executes ahead of the qa-reviewer hop.
    // -----------------------------------------------------------------
    const chatInput = page.getByTestId("chat-input");
    await chatInput.fill(
      "Identify the actor types in OSH's driver framework. The OSH core repo is at https://github.com/sensorhub-tools/osh-core; if it isn't already indexed in our research SemSource, register it.",
    );
    await page.getByTestId("send-button").click();

    // -----------------------------------------------------------------
    // Step 3 — wait for THREE loops (researcher A, reviewer B,
    // researcher-with-source C). Loop C will pause for approval.
    // -----------------------------------------------------------------
    const threeLoops = await pollUntil(async () => {
      const resp = await request.get("/teams-dispatch/loops");
      if (!resp.ok()) return null;
      const list = (await resp.json()) as Array<{
        loop_id: string;
        role: string;
        state: string;
      }>;
      return list.length >= 3 ? list : null;
    }, { timeoutMs: 60000 });

    expect(
      threeLoops,
      "expected 3 loops before first approval (researcher + reviewer + researcher-with-source)",
    ).toBeTruthy();

    // -----------------------------------------------------------------
    // Step 4 — find the first awaiting-approval loop (Loop C).
    // -----------------------------------------------------------------
    const loopCId = await pollUntil(async () => {
      const resp = await request.get("/teams-dispatch/loops");
      if (!resp.ok()) return null;
      const list = (await resp.json()) as Array<{
        loop_id: string;
        state: string;
        pending_approval?: { tool_name?: string } | null;
      }>;
      const awaiting = list.find((l) => l.pending_approval != null);
      return awaiting?.loop_id ?? null;
    }, { timeoutMs: 60000 });

    expect(loopCId, "expected Loop C to surface pending_approval").toBeTruthy();

    // Kanban awaiting-approval surface skipped intentionally — the
    // kanban only walks ONE level of children (deriveTaskInfo in
    // ui/src/lib/types/task.ts). Loop C's awaiting_approval is on a
    // grandchild, so the root task's effectiveColumn never cascades to
    // needs_you. The detail-panel navigation below uses Loop C's
    // loop_id directly. The API poll in Step 4 is the load-bearing
    // "chain reached awaiting_approval" signal.

    // -----------------------------------------------------------------
    // Step 5+6 — approve via API (POST /teams-dispatch/loops/{id}/approval).
    //
    // The UI-driven approval flow does not work for chain journeys —
    // the detail panel only surfaces PendingApprovalSection when the
    // selected task's primaryLoop has pending_approval, but the
    // dispatch-root task is the primary while the awaiting_approval
    // loop is a child / grandchild (TaskDetailPanel.svelte:218).
    // Same fix as ui/e2e/agentic/dev-via-spec.spec.ts. Surfacing
    // child-loop approval through the parent's detail panel is a
    // separate UI effort.
    //
    // The API path is what agentApi.submitApproval calls under the
    // hood (ADR-030 X-User-Id middleware contract), so this still
    // exercises the approval-gate plumbing end-to-end. Chain after
    // approval is autonomous: research arc completes (Loops C through
    // F), stabilisation rule spawns the dev-via-spec-planner (Loop
    // G), the dev-via-spec rules drive the four-role chain through
    // architect (Loop J) and builder (Loop K), then rule 07
    // (R3.7.2.k′) spawns the qa-reviewer (Loop L) on the builder's
    // tests_passing terminal.
    const approvalResp = await request.post(
      `/teams-dispatch/loops/${loopCId}/approval`,
      {
        data: { decision: "approve", user_id: "e2e-test-user" },
        headers: { "X-User-Id": "e2e-test-user" },
      },
    );
    expect(
      approvalResp.ok(),
      `expected /loops/${loopCId}/approval to accept; got ${approvalResp.status()}`,
    ).toBe(true);

    // -----------------------------------------------------------------
    // Step 7 — intermediate checkpoint: wait for FIVE loops (A, B,
    // C-complete, D-complete, E-spawned). Mirrors dev-via-spec.spec.ts
    // Step 7 — gives a fast-fail signal if the approval click didn't
    // unblock the chain, before we wait the full 240s for all-terminal.
    // -----------------------------------------------------------------
    const fiveLoops = await pollUntil(async () => {
      const resp = await request.get("/teams-dispatch/loops");
      if (!resp.ok()) return null;
      const list = (await resp.json()) as Array<{
        loop_id: string;
        role: string;
        state: string;
      }>;
      return list.length >= 5 ? list : null;
    }, { timeoutMs: 60000 });

    expect(
      fiveLoops,
      "expected 5 loops after first approval (A, B, C, D, E spawned by stabilisation rejection — same intermediate state as dev-via-spec.spec.ts)",
    ).toBeTruthy();

    // -----------------------------------------------------------------
    // Step 8 — wait for ALL FIFTEEN loops to reach terminal complete
    // state. Critical assertions: dev-via-spec rules 01/03/05/06/07
    // fire in order, ending with qa-reviewer terminal at needs_clarification.
    // ADR-039 Phase 1 rule 09 then fires + spawns the recovery architect;
    // rules 06/07 re-fire as the recovery builder/qa-reviewer roll
    // through to the recovery accept terminal.
    // -----------------------------------------------------------------
    const allTerminal = await pollUntil(async () => {
      const resp = await request.get("/teams-dispatch/loops");
      if (!resp.ok()) return null;
      const list = (await resp.json()) as Array<{ state: string }>;
      if (list.length !== 15) return null;
      return list.every((l) => l.state === "complete") ? list : null;
    }, { timeoutMs: 360000 });

    expect(
      allTerminal,
      "expected all 15 loops to reach terminal complete state (12 baseline + 3-loop ADR-039 Phase 1 recovery cycle)",
    ).toBeTruthy();

    // -----------------------------------------------------------------
    // Step 9 — role distribution proves every rule fired in order.
    //
    // Wire-shape note: dispatch-spawned loops ride on
    // dispatch.default_role and don't get their role stamped back
    // onto the LoopInfo wire JSON; rule-spawned loops do. So Loop A's
    // role field is empty; we resolve it via the dispatch default.
    //
    // Expected roles (after default_role resolution):
    //   1 × researcher                          (Loop A — dispatch)
    //   2 × researcher-with-source-acquisition  (Loops C, E — research rule 02 ×2)
    //   3 × research-reviewer                   (Loops B, D, F — research rules 01a/01b)
    //   1 × dev-via-spec-planner                (Loop G — research rule 03)
    //   1 × dev-via-spec-reviewer               (Loop H — dev-via-spec rule 01)
    //   1 × dev-via-spec-challenger             (Loop I — dev-via-spec rule 03)
    //   2 × dev-via-spec-architect              (Loop J — dev-via-spec rule 05;
    //                                            Loop M — ADR-039 rule 09 recovery)
    //   2 × dev-via-spec-builder                (Loop K — dev-via-spec rule 06;
    //                                            Loop N — recovery via re-fire of rule 06)
    //   2 × dev-via-spec-qa-reviewer            (Loop L — dev-via-spec rule 07;
    //                                            Loop O — recovery via re-fire of rule 07)
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
      `expected exactly 1 dispatch-spawned loop with empty role (Loop A); got ${dispatchLoops.length}`,
    ).toBe(1);

    const expectedDefaultRole = "researcher";
    const roles = finalLoops.map((l) => l.role || expectedDefaultRole);

    const researcherCount = roles.filter((r) => r === "researcher").length;
    const sourceAcqCount = roles.filter(
      (r) => r === "researcher-with-source-acquisition",
    ).length;
    const reviewerCount = roles.filter((r) => r === "research-reviewer").length;
    const plannerCount = roles.filter((r) => r === "dev-via-spec-planner").length;
    const devReviewerCount = roles.filter(
      (r) => r === "dev-via-spec-reviewer",
    ).length;
    const challengerCount = roles.filter(
      (r) => r === "dev-via-spec-challenger",
    ).length;
    const architectCount = roles.filter(
      (r) => r === "dev-via-spec-architect",
    ).length;
    const builderCount = roles.filter(
      (r) => r === "dev-via-spec-builder",
    ).length;
    const qaReviewerCount = roles.filter(
      (r) => r === "dev-via-spec-qa-reviewer",
    ).length;

    expect(
      researcherCount,
      `expected 1 researcher loop, got roles=${JSON.stringify(roles)}`,
    ).toBe(1);
    expect(
      sourceAcqCount,
      `expected 2 researcher-with-source-acquisition loops, got roles=${JSON.stringify(roles)}`,
    ).toBe(2);
    expect(
      reviewerCount,
      `expected 3 research-reviewer loops, got roles=${JSON.stringify(roles)}`,
    ).toBe(3);
    expect(
      plannerCount,
      `expected 1 dev-via-spec-planner loop. Missing → R3.2.2 stabilisation rule did not fire. roles=${JSON.stringify(roles)}`,
    ).toBe(1);
    expect(
      devReviewerCount,
      `expected 1 dev-via-spec-reviewer loop. Missing → planner's decide(planned) did not propagate. roles=${JSON.stringify(roles)}`,
    ).toBe(1);
    expect(
      challengerCount,
      `expected 1 dev-via-spec-challenger loop. Missing → reviewer's decide(approved) did not propagate. roles=${JSON.stringify(roles)}`,
    ).toBe(1);
    expect(
      architectCount,
      `expected 2 dev-via-spec-architect loops (Loop J via rule 05 + Loop M via ADR-039 rule 09 recovery). Missing → challenger's decide(accept) didn't propagate, OR rule 09 didn't fire on qa-reviewer's needs_clarification. roles=${JSON.stringify(roles)}`,
    ).toBe(2);
    expect(
      builderCount,
      `expected 2 dev-via-spec-builder loops (Loop K via rule 06 + Loop N via rule 06 re-fire on recovery architect's tasks_emitted). Missing → original or recovery architect's decide(tasks_emitted) didn't propagate. roles=${JSON.stringify(roles)}`,
    ).toBe(2);
    expect(
      qaReviewerCount,
      `expected 2 dev-via-spec-qa-reviewer loops (Loop L via rule 07 + Loop O via rule 07 re-fire on recovery builder's tests_passing). Missing → original or recovery builder didn't emit tests_passing OR rule 07 didn't load. roles=${JSON.stringify(roles)}`,
    ).toBe(2);

    // -----------------------------------------------------------------
    // Step 10 — only Loop C should ever have required approval. The
    // dev-via-spec chain has no approval-gated tools.
    // -----------------------------------------------------------------
    const stillPendingAny = finalLoops.find(
      (l) => l.pending_approval != null,
    );
    expect(
      stillPendingAny,
      "no loop should remain in pending_approval after the chain settles",
    ).toBeUndefined();

    // -----------------------------------------------------------------
    // Step 11 — settle assertion: no sixteenth loop appears. The
    // recovery qa-reviewer's decide(accept) terminal MUST NOT re-fire
    // any rule. Specifically: rule 09 only fires on needs_clarification
    // (not accept). A sixteenth loop would indicate either rule 09
    // mis-fired on accept (regression) or some other rule unexpectedly
    // matches the qa-reviewer role.
    // -----------------------------------------------------------------
    await new Promise((r) => setTimeout(r, 2000));
    const settledList = await request
      .get("/teams-dispatch/loops")
      .then((r) => r.json()) as unknown[];
    expect(
      settledList.length,
      "recovery qa-reviewer's accept terminal must not spawn a sixteenth loop",
    ).toBe(15);

    // -----------------------------------------------------------------
    // Step 12 — verify research.artifact.v1 publishes (Loops A/C/E)
    // and dev_via_spec.artifact.v1 publish (Loop J architect emit).
    // -----------------------------------------------------------------
    const messageLoggerResp = await request.get(
      "/message-logger/entries?limit=10000",
    );
    expect(
      messageLoggerResp.ok(),
      "message-logger /entries endpoint should be reachable",
    ).toBe(true);
    const entries = (await messageLoggerResp.json()) as Array<{
      subject: string;
    }>;
    const artifactSubjects = entries
      .map((e) => e.subject)
      .filter((s) => s.startsWith("research.artifact."));
    expect(
      artifactSubjects.length,
      `expected at least 3 research.artifact.<loop_id> publishes (one per researcher pass), got ${artifactSubjects.length}: ${JSON.stringify(artifactSubjects)}`,
    ).toBeGreaterThanOrEqual(3);

    const specArtifactSubjects = entries
      .map((e) => e.subject)
      .filter((s) => s.startsWith("dev_via_spec.artifact."));
    // Two architect emits (Loop J first-pass + Loop M recovery v2 with checks).
    expect(
      specArtifactSubjects.length,
      `expected ≥2 dev_via_spec.artifact.<loop_id> publishes (Loop J first pass + Loop M ADR-039 recovery), got ${specArtifactSubjects.length}: ${JSON.stringify(specArtifactSubjects)}`,
    ).toBeGreaterThanOrEqual(2);

    // ADR-038 PR C Phase C5: planner emits dev_via_spec.plan.<loop_id>
    // (Loop G); challenger emits dev_via_spec.consensus.<loop_id>
    // (Loop I, accept branch only). Catches wire-format drift between
    // persona prose, tool schema, and payload shape under mock-llm —
    // smoke #8 is the substance gate, this is the cheap insurance.
    const planSubjects = entries
      .map((e) => e.subject)
      .filter((s) => s.startsWith("dev_via_spec.plan."));
    expect(
      planSubjects.length,
      `expected at least 1 dev_via_spec.plan.<loop_id> publish from the planner's emit_plan, got ${planSubjects.length}: ${JSON.stringify(planSubjects)}`,
    ).toBeGreaterThanOrEqual(1);

    const consensusSubjects = entries
      .map((e) => e.subject)
      .filter((s) => s.startsWith("dev_via_spec.consensus."));
    expect(
      consensusSubjects.length,
      `expected at least 1 dev_via_spec.consensus.<loop_id> publish from the challenger's emit_consensus, got ${consensusSubjects.length}: ${JSON.stringify(consensusSubjects)}`,
    ).toBeGreaterThanOrEqual(1);

    // -----------------------------------------------------------------
    // Step 13 — terminal-state checks. All architect / builder /
    // qa-reviewer loops (both first-pass AND recovery) must be complete.
    // -----------------------------------------------------------------
    const architectLoops = finalLoops.filter(
      (l) => l.role === "dev-via-spec-architect",
    );
    expect(
      architectLoops.length,
      "expected 2 architect loops (first pass + recovery)",
    ).toBe(2);
    for (const a of architectLoops) {
      expect(a.state).toBe("complete");
    }

    const builderLoops = finalLoops.filter(
      (l) => l.role === "dev-via-spec-builder",
    );
    expect(
      builderLoops.length,
      "expected 2 builder loops (first pass + recovery)",
    ).toBe(2);
    for (const b of builderLoops) {
      expect(b.state).toBe("complete");
    }

    const qaReviewerLoops = finalLoops.filter(
      (l) => l.role === "dev-via-spec-qa-reviewer",
    );
    expect(
      qaReviewerLoops.length,
      "expected 2 qa-reviewer loops (first pass needs_clarification + recovery accept)",
    ).toBe(2);
    for (const q of qaReviewerLoops) {
      expect(q.state).toBe("complete");
    }
  });
});

async function pollUntil<T>(
  check: () => Promise<T | null>,
  options: { timeoutMs?: number; intervalMs?: number } = {},
): Promise<T | null> {
  const timeout = options.timeoutMs ?? 30000;
  const interval = options.intervalMs ?? 250;
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    const result = await check();
    if (result !== null && result !== undefined) {
      return result;
    }
    await new Promise((r) => setTimeout(r, interval));
  }
  return null;
}
