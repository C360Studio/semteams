import { test, expect } from "@playwright/test";

/**
 * Journey: Dev-via-Spec (R3.3 of ADR-031)
 *
 * Builds on R3.2.2's research-mode-transition: the same six-loop
 * research arc completes (researcher + reviewer + source-acquisition
 * + reviewer + settling pass + reviewer-approve), then the
 * stabilisation rule (R3.2.2 rule_03) spawns the dev-via-spec-planner.
 * From there the dev-via-spec internal rule chain
 * (configs/rules/dev-via-spec/ rules 01/03/05) drives the four-role
 * handoff to architect terminal. This spec covers the *golden
 * path* — every gate (reviewer + challenger) approves on first pass.
 * The retry rules (02 reviewer-rejected and 04 challenger-concerns)
 * are exercised structurally by the rule's max_iterations metadata
 * and persona-content discipline; covering them under e2e is a
 * follow-up if R3.4's OSH journey reveals real-LLM retry behaviour
 * worth pinning.
 *
 * Sequence (matches dev-via-spec.yaml — ten loops, twenty-eight LLM
 * round-trips):
 *
 *   Loop A — researcher (default_role, revision=1) → query_entity →
 *            emit_research_artifact (revision=1, empty) → completion
 *            → outcome=success → rule_01a fires → spawn reviewer
 *
 *   Loop B — research-reviewer (pass 1) → read_loop_result →
 *            decide(insufficient, recommends add_source_repo) →
 *            rule_02 fires → spawn researcher-with-source-acquisition
 *
 *   Loop C — researcher-with-source-acquisition (revision=2) →
 *            read_loop_result → add_source_repo (approval-gated) →
 *            [test approves] → query_entity →
 *            emit_research_artifact (revision=2, full + 1 mutation)
 *            → completion → outcome=success → rule_01b fires
 *
 *   Loop D — research-reviewer (pass 2 — stabilisation gate) →
 *            read_loop_result → decide(insufficient, "awaiting
 *            stabilisation") → rule_02 fires → spawn another
 *            researcher-with-source-acquisition
 *
 *   Loop E — researcher-with-source-acquisition (revision=3,
 *            settling — no new mutations) → read_loop_result →
 *            query_entity → emit_research_artifact (revision=3,
 *            substrate_mutations carried forward) → completion
 *            → outcome=success → rule_01b fires
 *
 *   Loop F — research-reviewer (pass 3) → read_loop_result →
 *            decide(approved) → rule_03 fires (research-mode-
 *            transition) → spawn dev-via-spec-planner
 *
 *   Loop G — dev-via-spec-planner (R3.3 first pass, real persona) →
 *            read_loop_result → decide(planned, reason=<plan>)
 *            → coordinator.next_action=planned → dev-via-spec
 *            rule_01 fires → spawn dev-via-spec-reviewer
 *
 *   Loop H — dev-via-spec-reviewer (first pass, golden path) →
 *            read_loop_result → decide(approved)
 *            → coordinator.next_action=approved → dev-via-spec
 *            rule_03 fires → spawn dev-via-spec-challenger
 *
 *   Loop I — dev-via-spec-challenger (first pass, golden path) →
 *            read_loop_result → decide(accept)
 *            → coordinator.next_action=accept → dev-via-spec
 *            rule_05 fires → spawn dev-via-spec-architect
 *
 *   Loop J — dev-via-spec-architect (terminal) →
 *            read_loop_result × 3 (challenger / planner /
 *            research-reviewer) →
 *            decide(seed_requirements_emitted) → terminal.
 *            No rule fires.
 *
 * Validates:
 *   - All ten loops appear with the expected role distribution
 *     (1 researcher + 2 researcher-with-source-acquisition + 3
 *     research-reviewer + 1 dev-via-spec-planner + 1 dev-via-spec-
 *     reviewer + 1 dev-via-spec-challenger + 1 dev-via-spec-
 *     architect) — proves rules 01a, 02 (×2), 01b (×2), 03,
 *     dev-via-spec rules 01, 03, 05 all fired in order.
 *   - approval_required gate fires for add_source_repo on Loop C.
 *   - The dev-via-spec chain runs autonomously after the research
 *     arc settles (no second approval gate during dev-via-spec).
 *   - Final state: all ten loops complete; no eleventh loop
 *     appears (architect terminal does not re-trigger any rule).
 *
 * Required fixture: test/fixtures/journeys/dev-via-spec.yaml
 * Required compose profile: semsource (the SemSource container is
 * required for the substrate-modifying retry pass).
 */

test.describe("Dev-via-Spec (R3.3)", () => {
  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok()).toBe(true);
  });

  // Ten loops + approval round-trip + SemSource AddRequest +
  // settling pass + four-role dev-via-spec chain. Allow 4.5
  // minutes to match the sum of inner poll budgets (60 + 60 + 60
  // + 150) and maintain headroom comparable to R3.2.2's
  // 180s-for-7-loops baseline.
  test.setTimeout(270_000);

  test("research → stabilisation → dev-via-spec planner / reviewer / challenger / architect", async ({
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
    // Step 2 — type the bounded prompt. Identical to R3.2.2's prompt;
    // the same six-loop research arc executes ahead of the dev-via-spec
    // handoff.
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
    // Step 6 — approve. The chain after the first approval is
    // autonomous: research arc completes (Loops C through F), the
    // stabilisation rule spawns the dev-via-spec-planner (Loop G),
    // and the dev-via-spec rules drive the four-role chain through
    // architect terminal (Loop J).
    // -----------------------------------------------------------------
    await page.getByTestId("approval-approve").click();

    // -----------------------------------------------------------------
    // Step 7 — intermediate checkpoint: wait for FIVE loops (A, B,
    // C-complete, D-complete, E-spawned). Mirrors R3.2.2's spec
    // Step 7 — gives us a fast-fail signal if the approval click
    // didn't unblock the chain, before we wait the full 150s for
    // all-terminal.
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
      "expected 5 loops after first approval (A, B, C, D, E spawned by stabilisation rejection — same intermediate state as R3.2.2)",
    ).toBeTruthy();

    // -----------------------------------------------------------------
    // Step 8 — wait for ALL TEN loops to reach terminal complete
    // state. Critical assertions: dev-via-spec rules 01/03/05 must
    // fire in order, ending with architect terminal.
    // -----------------------------------------------------------------
    const allTerminal = await pollUntil(async () => {
      const resp = await request.get("/teams-dispatch/loops");
      if (!resp.ok()) return null;
      const list = (await resp.json()) as Array<{ state: string }>;
      if (list.length !== 10) return null;
      return list.every((l) => l.state === "complete") ? list : null;
    }, { timeoutMs: 150000 });

    expect(
      allTerminal,
      "expected all 10 loops to reach terminal complete state (research arc + stabilisation + dev-via-spec planner / reviewer / challenger / architect)",
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
    //   1 × dev-via-spec-architect        (Loop J — dev-via-spec rule 05)
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
      `expected 1 dev-via-spec-planner loop (research-mode-transition rule_03 → planner). Missing → R3.2.2 stabilisation rule did not fire. roles=${JSON.stringify(roles)}`,
    ).toBe(1);
    expect(
      devReviewerCount,
      `expected 1 dev-via-spec-reviewer loop (dev-via-spec rule 01: planner→reviewer). Missing → planner's decide(planned) did not propagate. roles=${JSON.stringify(roles)}`,
    ).toBe(1);
    expect(
      challengerCount,
      `expected 1 dev-via-spec-challenger loop (dev-via-spec rule 03: reviewer→challenger). Missing → reviewer's decide(approved) did not propagate. roles=${JSON.stringify(roles)}`,
    ).toBe(1);
    expect(
      architectCount,
      `expected 1 dev-via-spec-architect loop (dev-via-spec rule 05: challenger→architect). Missing → challenger's decide(accept) did not propagate. roles=${JSON.stringify(roles)}`,
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
    // Step 11 — settle assertion: no eleventh loop appears. If
    // architect's decide(seed_requirements_emitted) somehow
    // re-fires a dev-via-spec rule (regression where rule 01 or 03
    // matches the architect role instead of planner/reviewer),
    // an eleventh loop would appear.
    // -----------------------------------------------------------------
    await new Promise((r) => setTimeout(r, 2000));
    const settledList = await request
      .get("/teams-dispatch/loops")
      .then((r) => r.json()) as unknown[];
    expect(
      settledList.length,
      "architect terminal must not spawn an eleventh loop",
    ).toBe(10);

    // -----------------------------------------------------------------
    // Step 12 — verify the typed research.artifact.v1 payload was
    // published per R3.2.1 — three artifacts (one per researcher pass,
    // revisions 1/2/3). The dev-via-spec chain doesn't publish typed
    // artifacts — its terminal output is the architect's
    // decide(seed_requirements_emitted) reason field, retained on
    // the AGENT_LOOPS bucket.
    // -----------------------------------------------------------------
    const messageLoggerResp = await request.get(
      "/message-logger/entries?limit=1000",
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

    // -----------------------------------------------------------------
    // Step 13 — Loop J's terminal state literal must be "complete".
    // -----------------------------------------------------------------
    const architectLoop = finalLoops.find(
      (l) => l.role === "dev-via-spec-architect",
    );
    expect(
      architectLoop,
      "dev-via-spec-architect loop should exist",
    ).toBeTruthy();
    expect(architectLoop?.state).toBe("complete");
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
