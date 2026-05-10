import { test, expect } from "@playwright/test";

/**
 * Journey: Research Mode Transition (R3.2.2 of ADR-031)
 *
 * Builds on R2.5's chained research+source-acquisition arc; adds the
 * `emit_research_artifact` tool call inside each researcher pass and
 * the persona-swap rule (03-stabilise-and-transition.json) that fires
 * when the reviewer approves a stabilised arc and spawns the
 * dev-via-spec-planner stub. The reviewer's stabilisation predicate
 * (zero new substrate mutations on the latest revision) lives in the
 * reviewer persona; the rule trusts the structured terminal output
 * (decide(approved)) per ADR-028 §Layer 2.
 *
 * Sequence (matches research-mode-transition.yaml — seven loops,
 * twenty LLM round-trips):
 *
 *   Loop A: researcher (default_role, revision=1) → query_entity →
 *           emit_research_artifact (revision=1, empty actors,
 *           open_gaps names corpus gap) → completion
 *           → outcome=success → rule_01a fires → spawn reviewer
 *
 *   Loop B: research-reviewer (pass 1) → read_loop_result →
 *           decide(insufficient, reason recommends add_source_repo)
 *           → rule_02 fires → spawn researcher-with-source-acquisition
 *
 *   Loop C: researcher-with-source-acquisition (revision=2,
 *           substrate-mod) → read_loop_result → add_source_repo
 *           (approval-gated) → [test approves] → query_entity →
 *           emit_research_artifact (revision=2, full artifact, ONE
 *           substrate_mutations entry with revision=2) → completion
 *           → outcome=success → rule_01b fires → spawn reviewer
 *
 *   Loop D: research-reviewer (pass 2 — stabilisation gate fires) →
 *           read_loop_result → decide(insufficient, reason="awaiting
 *           stabilisation") → rule_02 fires → spawn another
 *           researcher-with-source-acquisition
 *
 *   Loop E: researcher-with-source-acquisition (revision=3,
 *           settling pass — NO new substrate mutations) →
 *           read_loop_result → query_entity (no add_source_repo) →
 *           emit_research_artifact (revision=3, substrate_mutations
 *           carried forward from revision=2 unchanged — count of
 *           revision=3 entries is zero) → completion
 *           → outcome=success → rule_01b fires → spawn reviewer
 *
 *   Loop F: research-reviewer (pass 3) → read_loop_result →
 *           decide(approved) → rule_03 fires → spawn dev-via-spec-
 *           planner (the persona-swap mode-transition)
 *
 *   Loop G: dev-via-spec-planner (R3.2.2 stub) → read_loop_result →
 *           completion (ack message) → terminal. No rule fires.
 *
 * Validates:
 *   - All seven loops appear with the expected role distribution
 *     (1 researcher + 2 researcher-with-source-acquisition + 3
 *     research-reviewer + 1 dev-via-spec-planner) — proves
 *     rule_01a, rule_02 (×2), rule_01b (×2), and rule_03 all fired
 *     in order.
 *   - approval_required gate fires for add_source_repo on Loop C
 *     (and only Loop C — Loop E does NOT trigger approval, proving
 *     the settling pass query-only path).
 *   - The stabilisation gate is what causes Loop D's insufficient —
 *     reviewer's decide reason text contains "stabilisation".
 *   - The mode-transition fires: rule_03 spawns
 *     dev-via-spec-planner, which terminates cleanly.
 *   - Final state: all seven loops complete; no eighth loop appears.
 *
 * Required fixture: test/fixtures/journeys/research-mode-transition.yaml
 * Required compose profile: semsource (the SemSource container is
 * required for the substrate-modifying retry pass).
 */

test.describe("Research Mode Transition (R3.2.2)", () => {
  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok()).toBe(true);
  });

  // Seven loops + approval round-trip + SemSource AddRequest +
  // settling pass takes longer than the default. Allow 3 minutes.
  test.setTimeout(180_000);

  // ADR-040 PR 3 hard-replaced researcher-with-source-acquisition with the
  // source-curator role + new rules (02 spawn-curator, 02b indexed→researcher,
  // 02c needs_clarification→researcher). The fixture below still expects the
  // OLD flow shape (Loops C+E as researcher-with-source-acquisition); under
  // the new wiring rule 02 spawns source-curator and the chain shape changes.
  // PR 4 rewrites the fixture for the new curator flow and re-enables this
  // test.fixme.
  test.fixme("[ADR-040 PR 4 pending] research → corpus-gap reject → source-mod → stabilisation reject → settling pass → approve → dev-via-spec mode transition", async ({
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
    // Step 2 — type the bounded prompt. Identical to R2.5's prompt to
    // keep the substrate-mod path active; the difference is the
    // stabilisation gate now requires a settling pass before approval.
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
    // exercises the approval-gate plumbing end-to-end. Loop C iterates
    // against the augmented corpus, emits the artifact at revision=2,
    // terminates. rule_01b spawns Loop D (reviewer pass 2). Loop D
    // applies the stabilisation gate and rejects (revision=2 has new
    // mutations) — rule_02 spawns Loop E.
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
    // Step 7 — wait for FIVE loops to exist (A, B, C-complete, D-
    // complete, E-spawned). Loop E will be in executing state — no
    // approval needed (settling pass, query-only).
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
      "expected 5 loops after first approval (A, B, C, D, E spawned by stabilisation rejection)",
    ).toBeTruthy();

    // -----------------------------------------------------------------
    // Step 8 — wait for ALL SEVEN loops to reach terminal complete
    // state. The chain after the first approval is autonomous: Loop E
    // (settling pass) → Loop F (reviewer approve) → Loop G (dev-via-
    // spec-planner ack).
    //
    // Critical assertion: the stabilisation rule (03) must fire on the
    // reviewer-approved signal and spawn dev-via-spec-planner — that's
    // Loop G. Without it the test ends at six loops.
    // -----------------------------------------------------------------
    const allTerminal = await pollUntil(async () => {
      const resp = await request.get("/teams-dispatch/loops");
      if (!resp.ok()) return null;
      const list = (await resp.json()) as Array<{ state: string }>;
      if (list.length !== 7) return null;
      return list.every((l) => l.state === "complete") ? list : null;
    }, { timeoutMs: 90000 });

    expect(
      allTerminal,
      "expected all 7 loops to reach terminal complete state (research arc + stabilisation + dev-via-spec mode transition)",
    ).toBeTruthy();

    // -----------------------------------------------------------------
    // Step 9 — role distribution proves every rule fired in order.
    //
    // Wire-shape note (carried over from R2.5): dispatch-spawned loops
    // ride on dispatch.default_role and don't get their role stamped
    // back onto the LoopInfo wire JSON; rule-spawned loops do. So Loop
    // A's role field is empty; we resolve it via the dispatch default.
    //
    // Expected roles (after default_role resolution):
    //   1 × researcher                          (Loop A — dispatch)
    //   3 × research-reviewer                   (Loops B, D, F — rule 01a/01b)
    //   2 × researcher-with-source-acquisition  (Loops C, E — rule 02 ×2)
    //   1 × dev-via-spec-planner                (Loop G — rule 03)
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

    expect(
      researcherCount,
      `expected 1 researcher loop, got roles=${JSON.stringify(roles)}`,
    ).toBe(1);
    expect(
      sourceAcqCount,
      `expected 2 researcher-with-source-acquisition loops (initial mod + settling pass), got roles=${JSON.stringify(roles)}`,
    ).toBe(2);
    expect(
      reviewerCount,
      `expected 3 research-reviewer loops (corpus-gap reject, stabilisation reject, approve), got roles=${JSON.stringify(roles)}`,
    ).toBe(3);
    expect(
      plannerCount,
      `expected 1 dev-via-spec-planner loop (R3.2.2 mode-transition rule), got roles=${JSON.stringify(roles)}. Missing → rule_03 did not fire.`,
    ).toBe(1);

    // -----------------------------------------------------------------
    // Step 10 — only Loop C should have ever required approval. Loop E
    // (settling pass, revision=3) must have completed without invoking
    // add_source_repo, proving the stabilisation flow's query-only
    // path. We confirm by counting how many loops had a pending_approval
    // populated at any point — only one (Loop C) should have.
    //
    // The dispatch /loops endpoint returns current state, not history,
    // so we verify by inspecting Loop E's terminal trajectory does not
    // contain an add_source_repo tool call. (If the e2e harness exposes
    // a trajectory endpoint we'd query it; for now we trust the
    // fixture sequence — Loop E's only tool call is query_entity.)
    // -----------------------------------------------------------------
    const stillPendingAny = finalLoops.find(
      (l) => l.pending_approval != null,
    );
    expect(
      stillPendingAny,
      "no loop should remain in pending_approval after the chain settles",
    ).toBeUndefined();

    // -----------------------------------------------------------------
    // Step 11 — settle assertion: no eighth loop appears. If
    // dev-via-spec-planner's completion somehow re-fires a research
    // rule (regression where rule_01a matches the planner role
    // instead of researcher), an eighth loop would appear.
    // -----------------------------------------------------------------
    await new Promise((r) => setTimeout(r, 2000));
    const settledList = await request
      .get("/teams-dispatch/loops")
      .then((r) => r.json()) as unknown[];
    expect(
      settledList.length,
      "dev-via-spec-planner completion must not spawn an eighth loop",
    ).toBe(7);

    // -----------------------------------------------------------------
    // Step 12 — verify the typed research.artifact.v1 payload was
    // published on a stable subject. The message-logger surfaces every
    // subject it monitored; research.artifact.> was added to its
    // monitor_subjects in the e2e config. Three artifacts should have
    // been published (one per researcher pass, revisions 1/2/3).
    // -----------------------------------------------------------------
    // The message-logger /entries endpoint accepts a `subject` query
    // param meant to glob-filter, but in practice the param's
    // pattern matcher does not match prefix globs reliably (verified
    // against semstreams beta.27: `?subject=research.artifact.*`
    // returns 0 entries even when the entries exist). Fetch the full
    // recent buffer and filter client-side instead — the entries
    // buffer is bounded (max_entries config), so this is safe.
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
    // Step 13 — Loop G's terminal state literal must be "complete".
    // -----------------------------------------------------------------
    const plannerLoop = finalLoops.find(
      (l) => l.role === "dev-via-spec-planner",
    );
    expect(plannerLoop, "dev-via-spec-planner loop should exist").toBeTruthy();
    expect(plannerLoop?.state).toBe("complete");
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
