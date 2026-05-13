import { test, expect } from "@playwright/test";

/**
 * Journey: Research Mode Transition (ADR-041 MVP role compression)
 *
 * Validates the research-only arc end-to-end on mock-LLM with the
 * compressed MVP roster (researcher-{plan,gather,synthesize} +
 * reviewer-research). Reviewer rejects rev 1, recoverycounter writes
 * chain.recovery.proceed, rule 02 spawns a fresh researcher-plan,
 * the chain re-runs through gather/synthesize, reviewer approves
 * rev 2. Terminal at reviewer-research.
 *
 *   Loop A: researcher-plan (dispatch) → emit_plan → decide(gather)
 *           → rule 04 → spawn researcher-gather
 *
 *   Loop B: researcher-gather → read_loop_result → query_entity →
 *           decide(synthesize) → rule 05 → spawn researcher-synthesize
 *
 *   Loop C: researcher-synthesize → read_loop_result →
 *           emit_research_artifact (rev 1 empty) → decide(emit)
 *           → rule 01a → spawn reviewer-research
 *
 *   Loop D: reviewer-research → read_loop_result →
 *           decide(insufficient) → recoverycounter +
 *           rule 02 → spawn researcher-plan (recovery)
 *
 *   Loop E: researcher-plan (recovery) → read_loop_result →
 *           decide(gather) → rule 04 → spawn researcher-gather
 *
 *   Loop F: researcher-gather → read_loop_result → query_entity →
 *           decide(synthesize) → rule 05 → spawn researcher-synthesize
 *
 *   Loop G: researcher-synthesize → read_loop_result →
 *           emit_research_artifact (rev 2 full) → decide(emit)
 *           → rule 01a → spawn reviewer-research
 *
 *   Loop H: reviewer-research → read_loop_result → decide(approved)
 *           → terminal. No rule fires.
 *
 * Validates:
 *   - All eight loops appear with the expected role distribution
 *     (2 researcher-plan + 2 researcher-gather + 2 researcher-synthesize
 *     + 2 reviewer-research) — proves rules 04 (×2), 05 (×2), 01a
 *     (×2), and rule 02 all fired in order. ADR-041 absorbs the
 *     legacy curator + planner triangle into researcher-plan +
 *     reviewer-research.
 *   - The MVP chain runs autonomously: no human approval gate
 *     (substrate-mutation is out of MVP scope per ADR-041 §1).
 *   - Final state: all eight loops complete; no ninth loop appears.
 *
 * Required fixture: test/fixtures/journeys/research-mode-transition.yaml
 * Compose profile: none beyond agentic-e2e baseline (the curator's
 * semsource dependency was retired with the MVP roster).
 */

test.describe("Research Mode Transition (ADR-041 MVP)", () => {
  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok()).toBe(true);
  });

  // Eight loops, autonomous chain (no approval gate). 3 minutes
  // covers mock-LLM scheduling jitter; budget kept generous to
  // absorb cold-start latency on slower CI.
  test.setTimeout(180_000);

  test("research → corpus-gap reject → recovery → approve (ADR-041 MVP chain)", async ({
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
    // Step 2 — type the bounded prompt. ADR-041 absorbs the legacy
    // "register source" step into the model floor; the MVP researcher
    // chain queries the corpus directly without an explicit curator
    // role.
    // -----------------------------------------------------------------
    const chatInput = page.getByTestId("chat-input");
    await chatInput.fill(
      "Identify the actor types in OSH's driver framework.",
    );
    await page.getByTestId("send-button").click();

    // -----------------------------------------------------------------
    // Step 3 — wait for ALL EIGHT loops to reach terminal complete
    // state. The MVP chain is autonomous: dispatch researcher-plan,
    // through gather/synthesize/reviewer-research, recovery via
    // rule 02, second pass through to reviewer-research approve.
    //
    // Critical assertions: rule 02 (recovery), rules 04+05 (×2 each
    // forward edges), rule 01a (×2 terminal hand-off) all fire in
    // order. Without rule 02, the chain ends at six loops with no
    // recovery; without rule 01a (×2), the chain ends prematurely
    // at researcher-synthesize.
    // -----------------------------------------------------------------
    const allTerminal = await pollUntil(async () => {
      const resp = await request.get("/teams-dispatch/loops");
      if (!resp.ok()) return null;
      const list = (await resp.json()) as Array<{ state: string }>;
      if (list.length !== 8) return null;
      return list.every((l) => l.state === "complete") ? list : null;
    }, { timeoutMs: 120000 });

    expect(
      allTerminal,
      "expected all 8 loops to reach terminal complete state (researcher-plan/gather/synthesize ×2 chain + reviewer-research ×2)",
    ).toBeTruthy();

    // -----------------------------------------------------------------
    // Step 4 — role distribution proves every rule fired in order.
    //
    // Wire-shape note: dispatch-spawned loops ride on
    // dispatch.default_role and don't get their role stamped back
    // onto the LoopInfo wire JSON; rule-spawned loops do. So Loop A's
    // role field is empty; we resolve it via the dispatch default.
    //
    // Expected roles (after default_role resolution, ADR-041 MVP):
    //   2 × researcher-plan       (Loop A — dispatch; Loop E — rule 02 recovery)
    //   2 × researcher-gather     (Loops B, F — rule 04 ×2)
    //   2 × researcher-synthesize (Loops C, G — rule 05 ×2)
    //   2 × reviewer-research     (Loops D, H — rule 01a ×2)
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

    // Phase 3 wires dispatch.default_role to "researcher-plan"; until
    // then this spec is gated by that config update (the rules expect
    // a researcher-* prefixed role to fire phasevalidator).
    const expectedDefaultRole = "researcher-plan";
    const roles = finalLoops.map((l) => l.role || expectedDefaultRole);
    const planCount = roles.filter((r) => r === "researcher-plan").length;
    const gatherCount = roles.filter((r) => r === "researcher-gather").length;
    const synthesizeCount = roles.filter((r) => r === "researcher-synthesize").length;
    const reviewerCount = roles.filter((r) => r === "reviewer-research").length;

    expect(
      planCount,
      `expected 2 researcher-plan loops (Loop A dispatch + Loop E rule 02 recovery), got roles=${JSON.stringify(roles)}`,
    ).toBe(2);
    expect(
      gatherCount,
      `expected 2 researcher-gather loops (Loops B + F via rule 04 ×2), got roles=${JSON.stringify(roles)}. Missing → rule 04 (phase-transition-to-gather) did not fire as expected.`,
    ).toBe(2);
    expect(
      synthesizeCount,
      `expected 2 researcher-synthesize loops (Loops C + G via rule 05 ×2), got roles=${JSON.stringify(roles)}. Missing → rule 05 (phase-transition-to-synthesize) did not fire as expected.`,
    ).toBe(2);
    expect(
      reviewerCount,
      `expected 2 reviewer-research loops (Loops D + H via rule 01a ×2), got roles=${JSON.stringify(roles)}. Missing → rule 01a (researcher-synthesize-to-reviewer-research) did not fire as expected.`,
    ).toBe(2);

    // -----------------------------------------------------------------
    // Step 5 — no approval gate on the MVP research chain. The
    // legacy curator's add_source_repo + approval round-trip was
    // retired under ADR-041 §1 (substrate-mutation is out of MVP
    // scope).
    // -----------------------------------------------------------------
    const stillPendingAny = finalLoops.find(
      (l) => l.pending_approval != null,
    );
    expect(
      stillPendingAny,
      "no loop should ever require approval — MVP research chain is autonomous",
    ).toBeUndefined();

    // -----------------------------------------------------------------
    // Step 6 — settle assertion: no ninth loop appears. If
    // reviewer-research's decide(approved) somehow re-fires a rule
    // (regression where rule 02 mis-fires on approved), a ninth loop
    // would appear.
    // -----------------------------------------------------------------
    await new Promise((r) => setTimeout(r, 2000));
    const settledList = await request
      .get("/teams-dispatch/loops")
      .then((r) => r.json()) as unknown[];
    expect(
      settledList.length,
      "reviewer-research approved terminal must not spawn a ninth loop",
    ).toBe(8);
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
