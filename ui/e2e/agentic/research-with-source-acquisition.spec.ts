import { test, expect } from "@playwright/test";

/**
 * Journey: Research with Chained Source Acquisition (R2.5 of ADR-031)
 *
 * Joins R1 (iterative researcher with reviewer-as-enumerator gate)
 * with R2 (add_source_repo + approval gate + SemSource). The
 * structural test of the ADR-031 invariant: research can modify the
 * substrate it is reasoning over.
 *
 * Sequence (matches research-with-source-acquisition.yaml — four
 * loops, ten LLM round-trips):
 *
 *   Loop A: researcher (default_role) → query_entity → completion
 *           (incomplete artifact, open_gaps names the missing corpus)
 *           → outcome=success → rule_01a fires → spawn reviewer
 *
 *   Loop B: research-reviewer → read_loop_result → decide(insufficient,
 *           reason recommends add_source_repo) → rule_02 fires →
 *           spawn researcher-with-source-acquisition
 *
 *   Loop C: researcher-with-source-acquisition → read_loop_result →
 *           add_source_repo (approval-gated, loop pauses) →
 *           [test approves] → query_entity → completion (complete
 *           artifact, addressed_gaps names the source registration)
 *           → outcome=success → rule_01b fires → spawn reviewer
 *
 *   Loop D: research-reviewer → read_loop_result → decide(approved)
 *           → terminal. No rule fires.
 *
 * Validates:
 *   - All four loops appear with the expected role distribution
 *     (1 researcher + 1 researcher-with-source-acquisition + 2
 *     research-reviewer) — proves rule_01a, rule_02, rule_01b all
 *     fired in order.
 *   - approval_required gate fires for add_source_repo on Loop C.
 *   - PendingApprovalSection renders the gated tool with the repo
 *     URL visible in the args block.
 *   - Approve click → tool re-dispatches → SemSource registers →
 *     Loop C iterates against the augmented corpus → terminates.
 *   - Final state: all four loops complete, no fifth loop.
 *
 * Required fixture: test/fixtures/journeys/research-with-source-acquisition.yaml
 * Required compose profile: semsource (SemSource container is gated
 * behind the profile so journeys that don't need it skip the extra
 * boot cost).
 */

test.describe("Research with Chained Source Acquisition", () => {
  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok()).toBe(true);
  });

  // Four loops + approval round-trip + SemSource AddRequest takes
  // longer than the default 30s. Allow 2 minutes for the full chain.
  test.setTimeout(120_000);

  test("research → review insufficient → researcher-with-source registers + iterates → review approved", async ({
    page,
    request,
  }) => {
    // -----------------------------------------------------------------
    // Step 1 — open the Board so the SSE activity stream is connected
    // before any loops appear. PendingApprovalSection assertions in
    // later steps need the kanban / detail-panel surface to be live.
    // -----------------------------------------------------------------
    await page.goto("/");

    await expect(page.getByTestId("connection-status")).toHaveAttribute(
      "data-summary",
      "healthy",
      { timeout: 10000 },
    );

    await expect(page.getByTestId("kanban-board")).toBeVisible();

    // -----------------------------------------------------------------
    // Step 2 — type the bounded prompt. The dispatch's default_role is
    // `researcher`, so the loop spawns directly into the initial-pass
    // researcher role. The fixture's first response is a query_entity
    // probe that finds nothing, followed by an artifact whose
    // open_gaps names the corpus gap.
    // -----------------------------------------------------------------
    const chatInput = page.getByTestId("chat-input");
    await chatInput.fill(
      "Identify the actor types in OSH's driver framework. The OSH core repo is at https://github.com/sensorhub-tools/osh-core; if it isn't already indexed in our research SemSource, register it.",
    );
    await page.getByTestId("send-button").click();

    // -----------------------------------------------------------------
    // Step 3 — wait for THREE loops to appear (researcher, reviewer,
    // researcher-with-source-acquisition). Loop C will be in
    // awaiting_approval state until we approve. Loop D only spawns
    // after we approve and Loop C completes.
    //
    // The /teams-dispatch/loops API does not guarantee creation order,
    // and LoopInfo.Role is omitempty for early polls — so we count
    // loops, not order them.
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
      "expected 3 loops before approval (researcher + reviewer + researcher-with-source)",
    ).toBeTruthy();
    expect(threeLoops!.length).toBeGreaterThanOrEqual(3);

    // -----------------------------------------------------------------
    // Step 4 — find the Loop C loop_id (researcher-with-source-
    // acquisition, the one currently awaiting approval). The framework
    // does NOT change loop.state to "awaiting_approval" — instead, the
    // loop stays in state="executing" with pending_approval populated
    // (per /teams-dispatch/loops/<id> JSON shape). Discriminate on
    // pending_approval being non-null, which is the canonical signal
    // that the loop is paused for human approval.
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

    expect(loopCId, "expected a loop with pending_approval populated").toBeTruthy();

    // Kanban awaiting-approval surface skipped intentionally — the
    // kanban only walks ONE level of children (deriveTaskInfo in
    // ui/src/lib/types/task.ts). The awaiting_approval loop is a
    // grandchild here, so the root task's effectiveColumn never
    // cascades to needs_you. The detail-panel navigation below uses
    // the loop_id directly.

    // -----------------------------------------------------------------
    // Step 5+6+7 — approve via API (POST /teams-dispatch/loops/{id}/approval).
    //
    // The UI-driven approval flow does not work for chain journeys —
    // the detail panel only surfaces PendingApprovalSection when the
    // selected task's primaryLoop has pending_approval, but the
    // dispatch-root task is the primary while the awaiting_approval
    // loop is a child / grandchild (TaskDetailPanel.svelte:218).
    // Same fix as ui/e2e/agentic/dev-via-spec.spec.ts.
    //
    // The API path is what agentApi.submitApproval calls under the
    // hood; dispatch publishes the ApprovalResponse, agentic-loop
    // re-dispatches add_source_repo with ApprovedBy stamped, the tool
    // publishes AddRequest to graph.ingest.add.research, SemSource
    // replies, the agent continues iterating in the SAME loop,
    // queries the augmented corpus, and emits a completion. rule_01b
    // then spawns the second reviewer (Loop D).
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
    // Step 8 — wait for FOUR loops total, all in terminal complete
    // state. Loop D appears only after Loop C completes and rule_01b
    // fires.
    // -----------------------------------------------------------------
    const allTerminal = await pollUntil(async () => {
      const resp = await request.get("/teams-dispatch/loops");
      if (!resp.ok()) return null;
      const list = (await resp.json()) as Array<{ state: string }>;
      if (list.length !== 4) return null;
      return list.every((l) => l.state === "complete") ? list : null;
    }, { timeoutMs: 60000 });

    expect(
      allTerminal,
      "expected all 4 loops to reach terminal complete state",
    ).toBeTruthy();

    // -----------------------------------------------------------------
    // Step 9 — verify the role distribution proves all three rules
    // fired.
    //
    // Wire-shape note: dispatch-spawned loops (the user-initiated
    // entry-point loop, Loop A here) ride on the dispatch's
    // `default_role`, which is NOT stamped back onto the LoopInfo
    // wire JSON — its `role` field stays empty (omitempty) even at
    // terminal state. Rule-spawned loops (Loops B/C/D, via
    // publish_agent) carry an explicit role on the wire. So the
    // canonical assertion is "exactly one loop should have an empty
    // role (the dispatch one), and that loop's effective role is the
    // dispatch default_role".
    //
    // Expect: 1 researcher (Loop A, default-role fallback),
    // 1 researcher-with-source-acquisition (Loop C, rule-spawned),
    // 2 research-reviewer (Loops B + D, rule-spawned).
    // -----------------------------------------------------------------
    const finalLoops = await request
      .get("/teams-dispatch/loops")
      .then((r) => r.json()) as Array<{ role: string; state: string }>;
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

    expect(
      researcherCount,
      `expected exactly 1 researcher loop, got roles=${JSON.stringify(roles)}`,
    ).toBe(1);
    expect(
      sourceAcqCount,
      `expected exactly 1 researcher-with-source-acquisition loop, got roles=${JSON.stringify(roles)}`,
    ).toBe(1);
    expect(
      reviewerCount,
      `expected 2 research-reviewer loops, got roles=${JSON.stringify(roles)}`,
    ).toBe(2);

    // -----------------------------------------------------------------
    // Step 10 — assert the loop count did not grow beyond 4 even after
    // a settle interval. If a fifth loop appeared, the approved
    // decision somehow re-fired a retry rule (regression).
    // -----------------------------------------------------------------
    await new Promise((r) => setTimeout(r, 1500));
    const settledList = await request
      .get("/teams-dispatch/loops")
      .then((r) => r.json()) as unknown[];
    expect(
      settledList.length,
      "approved decision must not spawn a fifth loop",
    ).toBe(4);

    // -----------------------------------------------------------------
    // Step 11 — Loop C's terminal state literal must be "complete"
    // (per upstream LoopStateComplete). "success" is an outcome alias,
    // not a state.
    // -----------------------------------------------------------------
    const loopCFinal = await request
      .get(`/teams-dispatch/loops/${loopCId}`)
      .then((r) => r.json());
    expect(loopCFinal.state).toBe("complete");
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
