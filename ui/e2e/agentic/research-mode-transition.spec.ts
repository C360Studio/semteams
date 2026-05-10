import { test, expect } from "@playwright/test";

/**
 * Journey: Research Mode Transition (ADR-040 reshape)
 *
 * Validates the new source-curator flow end-to-end on mock-LLM:
 *
 *   Loop A: researcher (default_role, revision=1) → query_entity →
 *           emit_research_artifact (revision=1, empty actors,
 *           open_gaps names corpus gap) → completion
 *           → outcome=success → rule_01a fires → spawn reviewer
 *
 *   Loop B: research-reviewer (pass 1) → read_loop_result →
 *           decide(insufficient, reason recommends add_source_repo)
 *           → rule_02 (spawn-curator) fires → spawn source-curator
 *
 *   Loop C: source-curator (substrate-mutating) → read_loop_result →
 *           add_source_repo (approval-gated) → [test approves] →
 *           query_entity (verifies indexed) → emit_curator_artifact
 *           (added_sources + verified_entity_ids) →
 *           decide(action="indexed") → rule_02b fires → spawn researcher
 *
 *   Loop D: researcher (recovery=curator_indexed) → read_loop_result
 *           (consumes curator artifact) → query_entity (augmented
 *           corpus) → emit_research_artifact (revision=2, full
 *           actors/integration_points/tasks; substrate_mutations=[]
 *           — researcher does NOT mutate under ADR-040) → completion
 *           → outcome=success → rule_01a fires → spawn reviewer
 *
 *   Loop E: research-reviewer (pass 2) → read_loop_result →
 *           decide(approved) → rule_03 fires → spawn dev-via-spec-
 *           planner (the persona-swap mode-transition)
 *
 *   Loop F: dev-via-spec-planner (R3.3 stub) → read_loop_result →
 *           completion (ack message) → terminal. No rule fires.
 *
 * Validates:
 *   - All six loops appear with the expected role distribution
 *     (2 researcher + 1 source-curator + 2 research-reviewer + 1
 *     dev-via-spec-planner) — proves rule_01a (×2), rule_02
 *     (spawn-curator), rule_02b (curator-indexed → researcher), and
 *     rule_03 all fired in order. ADR-040 collapses the OLD 2×
 *     researcher-with-source-acquisition (substrate-mod + settling
 *     pass) into 1× source-curator + 1× re-query researcher.
 *   - approval_required gate fires for add_source_repo on Loop C
 *     (the curator's substrate-mutating call) — only Loop C; Loop D
 *     re-query has no add_source_repo, proving the ADR-040 separation
 *     of substrate-mutation (curator) from query (researcher).
 *   - The mode-transition fires: rule_03 spawns dev-via-spec-planner,
 *     which terminates cleanly.
 *   - Final state: all six loops complete; no seventh loop appears.
 *
 * Required fixture: test/fixtures/journeys/research-mode-transition.yaml
 * Required compose profile: semsource (the SemSource container is
 * required for the curator's substrate-modifying add_source_repo call).
 */

test.describe("Research Mode Transition (ADR-040)", () => {
  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok()).toBe(true);
  });

  // Six loops + approval round-trip + SemSource AddRequest takes
  // longer than the default. Allow 3 minutes (was 7 loops pre-ADR-040;
  // budget kept the same to absorb mock-LLM scheduling jitter).
  test.setTimeout(180_000);

  test("research → corpus-gap reject → source-curator → re-query researcher → approve → dev-via-spec mode transition", async ({
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
    // Step 2 — type the bounded prompt. Identical to the pre-ADR-040
    // shape; the curator (not the researcher) handles the registration
    // path now.
    // -----------------------------------------------------------------
    const chatInput = page.getByTestId("chat-input");
    await chatInput.fill(
      "Identify the actor types in OSH's driver framework. The OSH core repo is at https://github.com/sensorhub-tools/osh-core; if it isn't already indexed in our research SemSource, register it.",
    );
    await page.getByTestId("send-button").click();

    // -----------------------------------------------------------------
    // Step 3 — wait for THREE loops (researcher A, reviewer B,
    // source-curator C). Loop C will pause for approval on the
    // curator's add_source_repo call.
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
      "expected 3 loops before first approval (researcher + reviewer + source-curator)",
    ).toBeTruthy();

    // -----------------------------------------------------------------
    // Step 4 — find the first awaiting-approval loop (Loop C — the
    // curator's add_source_repo).
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

    expect(loopCId, "expected Loop C (source-curator) to surface pending_approval").toBeTruthy();

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
    // Same UI-driven approval limitation as the pre-ADR-040 spec:
    // PendingApprovalSection only renders for the selected task's
    // primaryLoop, but the awaiting_approval loop here is a
    // grandchild of the dispatch root. We POST directly per the
    // contract X-User-Id middleware enforces (ADR-030). This still
    // exercises the approval-gate plumbing end-to-end. After approval,
    // the curator finishes its sequence (query_entity verifies
    // indexed → emit_curator_artifact → decide(action="indexed")) →
    // rule_02b spawns Loop D (re-query researcher).
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
    // Step 7 — wait for ALL SIX loops to reach terminal complete
    // state. The chain after the first approval is autonomous: Loop C
    // finishes the curator sequence → Loop D (re-query researcher) →
    // Loop E (reviewer approve) → Loop F (dev-via-spec-planner ack).
    //
    // Critical assertion: rule_03 must fire on the reviewer's approval
    // and spawn dev-via-spec-planner — that's Loop F. Without it the
    // test ends at five loops.
    // -----------------------------------------------------------------
    const allTerminal = await pollUntil(async () => {
      const resp = await request.get("/teams-dispatch/loops");
      if (!resp.ok()) return null;
      const list = (await resp.json()) as Array<{ state: string }>;
      if (list.length !== 6) return null;
      return list.every((l) => l.state === "complete") ? list : null;
    }, { timeoutMs: 90000 });

    expect(
      allTerminal,
      "expected all 6 loops to reach terminal complete state (research arc + curator + dev-via-spec mode transition)",
    ).toBeTruthy();

    // -----------------------------------------------------------------
    // Step 8 — role distribution proves every rule fired in order.
    //
    // Wire-shape note: dispatch-spawned loops ride on dispatch.default_role
    // and don't get their role stamped back onto the LoopInfo wire JSON;
    // rule-spawned loops do. So Loop A's role field is empty; we resolve
    // it via the dispatch default.
    //
    // Expected roles (after default_role resolution):
    //   2 × researcher           (Loop A — dispatch; Loop D — rule_02b post-curator re-query)
    //   2 × research-reviewer    (Loops B, E — rule_01a ×2)
    //   1 × source-curator       (Loop C — rule_02 spawn-curator)
    //   1 × dev-via-spec-planner (Loop F — rule_03)
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
    const curatorCount = roles.filter((r) => r === "source-curator").length;
    const reviewerCount = roles.filter((r) => r === "research-reviewer").length;
    const plannerCount = roles.filter((r) => r === "dev-via-spec-planner").length;

    expect(
      researcherCount,
      `expected 2 researcher loops (Loop A initial + Loop D re-query post-curator), got roles=${JSON.stringify(roles)}`,
    ).toBe(2);
    expect(
      curatorCount,
      `expected 1 source-curator loop (Loop C — rule_02 spawn-curator), got roles=${JSON.stringify(roles)}. Missing → rule_02 did not fire or fired to the wrong role.`,
    ).toBe(1);
    expect(
      reviewerCount,
      `expected 2 research-reviewer loops (corpus-gap reject, approve), got roles=${JSON.stringify(roles)}`,
    ).toBe(2);
    expect(
      plannerCount,
      `expected 1 dev-via-spec-planner loop (ADR-040 mode-transition rule_03), got roles=${JSON.stringify(roles)}. Missing → rule_03 did not fire.`,
    ).toBe(1);

    // -----------------------------------------------------------------
    // Step 9 — only Loop C should have ever required approval. Loop D
    // (re-query researcher) must have completed without invoking
    // add_source_repo, proving the ADR-040 separation of
    // substrate-mutation (curator) from query (researcher).
    //
    // The dispatch /loops endpoint returns current state, not history,
    // so we verify by confirming no loop is currently pending_approval.
    // The fixture sequence guarantees Loop D's only tool calls are
    // read_loop_result + query_entity + emit_research_artifact.
    // -----------------------------------------------------------------
    const stillPendingAny = finalLoops.find(
      (l) => l.pending_approval != null,
    );
    expect(
      stillPendingAny,
      "no loop should remain in pending_approval after the chain settles",
    ).toBeUndefined();

    // -----------------------------------------------------------------
    // Step 10 — settle assertion: no seventh loop appears. If
    // dev-via-spec-planner's completion somehow re-fires a research
    // rule (regression where rule_01a matches the planner role
    // instead of researcher), a seventh loop would appear.
    // -----------------------------------------------------------------
    await new Promise((r) => setTimeout(r, 2000));
    const settledList = await request
      .get("/teams-dispatch/loops")
      .then((r) => r.json()) as unknown[];
    expect(
      settledList.length,
      "dev-via-spec-planner completion must not spawn a seventh loop",
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
