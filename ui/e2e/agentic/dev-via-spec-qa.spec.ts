import { test, expect } from "@playwright/test";

/**
 * Journey: Dev-via-Spec QA-reviewer (R3.7.2.k′ of ADR-034)
 *
 * Extends dev-via-spec.spec.ts with the post-build review hop. The
 * first eleven loops are identical to dev-via-spec.spec.ts. The
 * difference at the tail:
 *
 *   - Loop K (dev-via-spec-builder) terminates with
 *     builder_decide(tests_passing) instead of needs_clarification.
 *     The mock-LLM does not actually run tests; the builder_decide
 *     tool emits coordinator.next_action=tests_passing
 *     unconditionally given the args, which triggers rule 07
 *     (R3.7.2.k′: builder→qa-reviewer).
 *
 *   - Loop L (dev-via-spec-qa-reviewer, NEW) reads the builder's
 *     terminal via read_loop_result and emits
 *     decide(needs_clarification). Per j′ 10-evaluation-contract.md
 *     Rule 3 this is the persona-correct verdict for the R3.7.2.k′
 *     stub state where the spawn-rule prompt embeds a hardcoded
 *     "(stub)" evidence block.
 *
 * What this proves:
 *   - Rule 07 (configs/rules/dev-via-spec/07-builder-decide-to-
 *     qa-reviewer.json) fires on coordinator.next_action=tests_passing
 *     and spawns the qa-reviewer role.
 *   - The qa-reviewer's persona (R3.7.2.j′) loads at boot and the
 *     loop reaches terminal complete state.
 *   - Total chain: twelve loops, eleven up to and including builder
 *     (identical to dev-via-spec.spec.ts) + one qa-reviewer.
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

  // Twelve loops + approval round-trip + SemSource AddRequest +
  // settling pass + four-role dev-via-spec chain + builder +
  // qa-reviewer. Allow 6.5 minutes — 30s headroom over
  // dev-via-spec.spec.ts's 360s for the extra qa-reviewer round-trip
  // (read_loop_result placeholder + decide; ~3-5s on mock-LLM).
  test.setTimeout(390_000);

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

    await expect(
      page.locator("[data-testid='task-card'] [data-state='awaiting_approval']"),
    ).toBeVisible({ timeout: 30000 });

    // -----------------------------------------------------------------
    // Step 5 — open Loop C's detail panel via URL state.
    // -----------------------------------------------------------------
    await page.goto(`/?task=${loopCId}`);
    await expect(page.getByTestId("connection-status")).toHaveAttribute(
      "data-summary",
      "healthy",
      { timeout: 10000 },
    );
    await expect(page.getByTestId("task-detail-panel")).toBeVisible();

    await expect(page.getByTestId("pending-approval-section")).toBeVisible();
    await expect(page.getByTestId("approval-tool-name")).toHaveText(
      "add_source_repo",
    );
    await expect(page.getByTestId("approval-args-display")).toContainText(
      "sensorhub-tools/osh-core",
    );

    // -----------------------------------------------------------------
    // Step 6 — approve. Chain after the first approval is autonomous:
    // research arc completes (Loops C through F), stabilisation rule
    // spawns the dev-via-spec-planner (Loop G), the dev-via-spec rules
    // drive the four-role chain through architect (Loop J) and builder
    // (Loop K), then rule 07 (R3.7.2.k′) spawns the qa-reviewer
    // (Loop L) on the builder's tests_passing terminal.
    // -----------------------------------------------------------------
    await page.getByTestId("approval-approve").click();

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
    // Step 8 — wait for ALL TWELVE loops to reach terminal complete
    // state. Critical assertions: dev-via-spec rules 01/03/05/06/07
    // must fire in order, ending with qa-reviewer terminal.
    // -----------------------------------------------------------------
    const allTerminal = await pollUntil(async () => {
      const resp = await request.get("/teams-dispatch/loops");
      if (!resp.ok()) return null;
      const list = (await resp.json()) as Array<{ state: string }>;
      if (list.length !== 12) return null;
      return list.every((l) => l.state === "complete") ? list : null;
    }, { timeoutMs: 270000 });

    expect(
      allTerminal,
      "expected all 12 loops to reach terminal complete state (research arc + stabilisation + dev-via-spec planner / reviewer / challenger / architect / builder / qa-reviewer)",
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
    //   1 × dev-via-spec-architect              (Loop J — dev-via-spec rule 05)
    //   1 × dev-via-spec-builder                (Loop K — dev-via-spec rule 06)
    //   1 × dev-via-spec-qa-reviewer            (Loop L — dev-via-spec rule 07)
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
      `expected 1 dev-via-spec-architect loop. Missing → challenger's decide(accept) did not propagate. roles=${JSON.stringify(roles)}`,
    ).toBe(1);
    expect(
      builderCount,
      `expected 1 dev-via-spec-builder loop. Missing → architect's decide(seed_requirements_emitted) did not propagate. roles=${JSON.stringify(roles)}`,
    ).toBe(1);
    expect(
      qaReviewerCount,
      `expected 1 dev-via-spec-qa-reviewer loop (rule 07: builder→qa-reviewer fires on tests_passing). Missing → builder did not emit coordinator.next_action=tests_passing OR rule 07 did not load. roles=${JSON.stringify(roles)}`,
    ).toBe(1);

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
    // Step 11 — settle assertion: no thirteenth loop appears. If the
    // qa-reviewer's decide(needs_clarification) somehow re-fires a
    // dev-via-spec rule (regression where rule 07 or another rule
    // matches the qa-reviewer role), a thirteenth loop would appear.
    // -----------------------------------------------------------------
    await new Promise((r) => setTimeout(r, 2000));
    const settledList = await request
      .get("/teams-dispatch/loops")
      .then((r) => r.json()) as unknown[];
    expect(
      settledList.length,
      "qa-reviewer terminal must not spawn a thirteenth loop",
    ).toBe(12);

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
    expect(
      specArtifactSubjects.length,
      `expected at least 1 dev_via_spec.artifact.<loop_id> publish from the architect, got ${specArtifactSubjects.length}`,
    ).toBeGreaterThanOrEqual(1);

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
      `expected at least 1 dev_via_spec.plan.<loop_id> publish from the planner's emit_plan, got ${planSubjects.length}`,
    ).toBeGreaterThanOrEqual(1);

    const consensusSubjects = entries
      .map((e) => e.subject)
      .filter((s) => s.startsWith("dev_via_spec.consensus."));
    expect(
      consensusSubjects.length,
      `expected at least 1 dev_via_spec.consensus.<loop_id> publish from the challenger's emit_consensus, got ${consensusSubjects.length}`,
    ).toBeGreaterThanOrEqual(1);

    // -----------------------------------------------------------------
    // Step 13 — terminal-state checks for the new tail. Builder must
    // be complete (rule 07 fired only because builder reached
    // terminal) and qa-reviewer must be complete (rule 07 spawned it
    // and it terminated cleanly).
    // -----------------------------------------------------------------
    const builderLoop = finalLoops.find(
      (l) => l.role === "dev-via-spec-builder",
    );
    expect(builderLoop, "dev-via-spec-builder loop should exist").toBeTruthy();
    expect(builderLoop?.state).toBe("complete");

    const qaReviewerLoop = finalLoops.find(
      (l) => l.role === "dev-via-spec-qa-reviewer",
    );
    expect(
      qaReviewerLoop,
      "dev-via-spec-qa-reviewer loop should exist (rule 07 fired on builder tests_passing)",
    ).toBeTruthy();
    expect(qaReviewerLoop?.state).toBe("complete");
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
