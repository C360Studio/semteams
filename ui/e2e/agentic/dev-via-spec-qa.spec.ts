import { test, expect } from "@playwright/test";

/**
 * Journey: Dev-via-Spec needs_clarification recovery (ADR-041 MVP)
 *
 * Extends dev-via-spec.spec.ts with a complete ADR-039 Phase 1 Shape A
 * supersession cycle. The baseline Loops A–G mirror dev-via-spec.spec.ts
 * (golden path through to reviewer-qa) except Loop G's reviewer-qa
 * terminates with decide(needs_clarification) instead of accept.
 * Rule 03 fires → spawn researcher-plan; the full MVP chain re-runs
 * (Loops H–N) and the recovery reviewer-qa accepts. Chain settles at
 * fourteen loops.
 *
 * Sequence (fourteen loops):
 *
 *   Loops A–F: identical to dev-via-spec.spec.ts (researcher-plan →
 *              researcher-gather → researcher-synthesize →
 *              researcher-architect → reviewer-spec → builder).
 *
 *   Loop G: reviewer-qa (rule 04 dev-via-spec spawn) →
 *           read_loop_result → decide(needs_clarification,
 *           reason="evidence summary is (no checks)")
 *           → rule 03 → spawn researcher-plan (recovery)
 *
 *   Loops H–N: full MVP chain re-runs with the architect's spec
 *              now carrying checks[] (one evidence rule binding the
 *              packet decoder). reviewer-spec approves the recovery
 *              spec, builder runs tests_passing, reviewer-qa accepts.
 *              No further rule fires.
 *
 * What this proves:
 *   - Rule 03 (configs/rules/dev-via-spec/03-needs-clarification-to-
 *     researcher.json) fires on coordinator.next_action=
 *     needs_clarification and spawns researcher-plan with
 *     properties.recovery="needs_clarification".
 *   - phasevalidator allows the recovery cycle: plan→gather (counter
 *     bumps 1→2), gather→synthesize (counter 1→2 at cap),
 *     synthesize→architect (counter 1→2 at cap).
 *   - phasevalidator.SpecModeGate forwards
 *     dev_via_spec.artifact.{path,slug} on BOTH the original and
 *     recovery reviewer-spec entities.
 *   - phasevalidator.QAModeGate stamps proceed on BOTH the original
 *     and recovery builder entities.
 *   - Recovery reviewer-qa's accept terminates cleanly (rule 03 does
 *     NOT fire on accept).
 *   - Total chain: fourteen loops.
 *
 * What this does NOT prove:
 *   - Substantive evidence grading. The evidence summary in the
 *     reviewer-qa prompt is a stub under mock-LLM. Real evidence-
 *     rendering plumbing lands in smoke #21 (Phase 4).
 *   - Real builder running mvn test. Smoke #21 exercises this on
 *     real LLM.
 *
 * Required fixture: test/fixtures/journeys/dev-via-spec-qa.yaml
 * Required compose profile: sandbox (builder bootstrap_workspace +
 * bash routing).
 */

test.describe("Dev-via-Spec QA recovery (ADR-041 MVP)", () => {
  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok()).toBe(true);
  });

  // Fourteen loops + recovery cycle, autonomous chain. 8 minutes
  // covers mock-LLM scheduling jitter + sandbox cold-start latency
  // on slower CI. Two bootstrap_workspace calls (rev 1 spec + rev 2
  // recovery spec) absorb additional latency.
  test.setTimeout(480_000);

  test("baseline chain → reviewer-qa needs_clarification → ADR-039 Phase 1 recovery cycle (ADR-041 MVP)", async ({
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
    // Step 2 — type the bounded prompt. Same shape as
    // dev-via-spec.spec.ts; the recovery cycle is internal to the
    // chain, triggered by reviewer-qa's needs_clarification verdict
    // on the rev 1 spec's empty checks[].
    // -----------------------------------------------------------------
    const chatInput = page.getByTestId("chat-input");
    await chatInput.fill(
      "Build the OSH Meshtastic driver — IDriver interface backed by Meshtastic radio packets, exposing OGC CS observation endpoints.",
    );
    await page.getByTestId("send-button").click();

    // -----------------------------------------------------------------
    // Step 3 — wait for ALL FOURTEEN loops to reach terminal complete
    // state. The MVP chain is autonomous: baseline through reviewer-qa
    // needs_clarification (Loops A–G), then ADR-039 Phase 1 Shape A
    // recovery cycle (Loops H–N).
    //
    // Critical assertions: rule 03 (needs_clarification →
    // researcher-plan) fires on Loop G's verdict; phasevalidator
    // honours the per-phase caps (gather=3 allows 2 fires;
    // synthesize=2 + architect=2 at cap on the recovery pass); rule
    // 04 dev-via-spec re-fires for the recovery reviewer-qa accept;
    // terminal accepts without re-firing rule 03.
    // -----------------------------------------------------------------
    const allTerminal = await pollUntil(async () => {
      const resp = await request.get("/teams-dispatch/loops");
      if (!resp.ok()) return null;
      const list = (await resp.json()) as Array<{ state: string }>;
      if (list.length !== 14) return null;
      return list.every((l) => l.state === "complete") ? list : null;
    }, { timeoutMs: 420000 });

    expect(
      allTerminal,
      "expected all 14 loops to reach terminal complete state (7 baseline + 7-loop ADR-039 Phase 1 recovery cycle)",
    ).toBeTruthy();

    // -----------------------------------------------------------------
    // Step 4 — role distribution proves every rule fired in order.
    //
    // Wire-shape note: dispatch-spawned loops ride on
    // dispatch.default_role; rule-spawned loops carry their role on
    // the LoopInfo wire JSON.
    //
    // Expected roles (after default_role resolution, ADR-041 MVP):
    //   2 × researcher-plan       (Loop A dispatch + Loop H rule 03 recovery)
    //   2 × researcher-gather     (Loops B, I via rule 04 ×2)
    //   2 × researcher-synthesize (Loops C, J via rule 05 ×2)
    //   2 × researcher-architect  (Loops D, K via rule 06 ×2)
    //   2 × reviewer-spec         (Loops E, L via rule 01b ×2)
    //   2 × builder               (Loops F, M via rule 02 + spec_mode_gate ×2)
    //   2 × reviewer-qa           (Loops G, N via rule 04 dev-via-spec + qa_mode_gate ×2)
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
    // then this spec is gated by that config update.
    const expectedDefaultRole = "researcher-plan";
    const roles = finalLoops.map((l) => l.role || expectedDefaultRole);

    const planCount = roles.filter((r) => r === "researcher-plan").length;
    const gatherCount = roles.filter((r) => r === "researcher-gather").length;
    const synthesizeCount = roles.filter((r) => r === "researcher-synthesize").length;
    const architectCount = roles.filter((r) => r === "researcher-architect").length;
    const reviewerSpecCount = roles.filter((r) => r === "reviewer-spec").length;
    const builderCount = roles.filter((r) => r === "builder").length;
    const reviewerQaCount = roles.filter((r) => r === "reviewer-qa").length;

    expect(
      planCount,
      `expected 2 researcher-plan loops (Loop A dispatch + Loop H rule 03 recovery), got roles=${JSON.stringify(roles)}`,
    ).toBe(2);
    expect(
      gatherCount,
      `expected 2 researcher-gather loops (Loops B + I via rule 04 ×2), got roles=${JSON.stringify(roles)}. Missing → rule 04 (phase-transition-to-gather) did not fire as expected.`,
    ).toBe(2);
    expect(
      synthesizeCount,
      `expected 2 researcher-synthesize loops (Loops C + J via rule 05 ×2). Cap is exactly 2 on the recovery pass. roles=${JSON.stringify(roles)}`,
    ).toBe(2);
    expect(
      architectCount,
      `expected 2 researcher-architect loops (Loops D + K via rule 06 ×2). Cap is exactly 2 on the recovery pass. roles=${JSON.stringify(roles)}`,
    ).toBe(2);
    expect(
      reviewerSpecCount,
      `expected 2 reviewer-spec loops (Loops E + L via rule 01b ×2). Missing → rule 01b did not fire on the recovery architect's emit. roles=${JSON.stringify(roles)}`,
    ).toBe(2);
    expect(
      builderCount,
      `expected 2 builder loops (Loops F + M via rule 02 + spec_mode_gate ×2). Missing → SpecModeGate did not stamp proceed on the recovery reviewer-spec OR rule 02 did not fire. roles=${JSON.stringify(roles)}`,
    ).toBe(2);
    expect(
      reviewerQaCount,
      `expected 2 reviewer-qa loops (Loops G + N via rule 04 dev-via-spec + qa_mode_gate ×2). Missing → QAModeGate did not stamp proceed on the recovery builder OR rule 04 dev-via-spec did not fire. roles=${JSON.stringify(roles)}`,
    ).toBe(2);

    // -----------------------------------------------------------------
    // Step 5 — no approval gate on the MVP chain. Substrate-mutation
    // is out of MVP scope per ADR-041 §1.
    // -----------------------------------------------------------------
    const stillPendingAny = finalLoops.find(
      (l) => l.pending_approval != null,
    );
    expect(
      stillPendingAny,
      "no loop should ever require approval — MVP dev-via-spec chain is autonomous",
    ).toBeUndefined();

    // -----------------------------------------------------------------
    // Step 6 — settle assertion: no fifteenth loop appears. The
    // recovery reviewer-qa's decide(accept) terminal MUST NOT re-fire
    // any rule. Specifically: rule 03 only fires on
    // needs_clarification, NOT accept. A fifteenth loop would indicate
    // either rule 03 mis-fired on accept (regression) or some other
    // rule unexpectedly matches the reviewer-qa role.
    // -----------------------------------------------------------------
    await new Promise((r) => setTimeout(r, 2000));
    const settledList = await request
      .get("/teams-dispatch/loops")
      .then((r) => r.json()) as unknown[];
    expect(
      settledList.length,
      "recovery reviewer-qa accept terminal must not spawn a fifteenth loop",
    ).toBe(14);

    // -----------------------------------------------------------------
    // Step 7 — verify the typed payload publishes:
    //   research.artifact ×2  (Loops C + J researcher-synthesize)
    //   dev_via_spec.artifact ×2 (Loops D + K researcher-architect)
    //   dev_via_spec.plan ×2 (Loops A + H researcher-plan emit_plan)
    // Catches wire-format drift between persona prose, tool schema,
    // and payload shape under mock-llm.
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

    const researchArtifactSubjects = entries
      .map((e) => e.subject)
      .filter((s) => s.startsWith("research.artifact."));
    expect(
      researchArtifactSubjects.length,
      `expected ≥2 research.artifact.<loop_id> publishes (Loop C + Loop J), got ${researchArtifactSubjects.length}: ${JSON.stringify(researchArtifactSubjects)}`,
    ).toBeGreaterThanOrEqual(2);

    const specArtifactSubjects = entries
      .map((e) => e.subject)
      .filter((s) => s.startsWith("dev_via_spec.artifact."));
    expect(
      specArtifactSubjects.length,
      `expected ≥2 dev_via_spec.artifact.<loop_id> publishes (Loop D rev 1 + Loop K recovery v2), got ${specArtifactSubjects.length}: ${JSON.stringify(specArtifactSubjects)}`,
    ).toBeGreaterThanOrEqual(2);

    const planSubjects = entries
      .map((e) => e.subject)
      .filter((s) => s.startsWith("dev_via_spec.plan."));
    expect(
      planSubjects.length,
      `expected ≥2 dev_via_spec.plan.<loop_id> publishes (Loop A dispatch + Loop H rule 03 recovery), got ${planSubjects.length}: ${JSON.stringify(planSubjects)}`,
    ).toBeGreaterThanOrEqual(2);

    // -----------------------------------------------------------------
    // Step 8 — verify the phasevalidator gates stamped their proceed
    // sentinels on BOTH the original and recovery passes.
    // -----------------------------------------------------------------
    const specGateProceed = await fetchTriples(request, {
      predicate: "chain.spec_mode_gate.proceed",
      limit: 5,
    });
    expect(
      specGateProceed.length,
      `expected ≥2 chain.spec_mode_gate.proceed triples (original Loop E + recovery Loop L)`,
    ).toBeGreaterThanOrEqual(2);

    const qaGateProceed = await fetchTriples(request, {
      predicate: "chain.qa_mode_gate.proceed",
      limit: 5,
    });
    expect(
      qaGateProceed.length,
      `expected ≥2 chain.qa_mode_gate.proceed triples (original Loop F + recovery Loop M)`,
    ).toBeGreaterThanOrEqual(2);

    // -----------------------------------------------------------------
    // Step 9 — terminal-state checks. All loops in each MVP role
    // (researcher-{plan,gather,synthesize,architect}, reviewer-{spec,qa},
    // builder) must be complete.
    // -----------------------------------------------------------------
    const reviewerQaLoops = finalLoops.filter(
      (l) => l.role === "reviewer-qa",
    );
    expect(
      reviewerQaLoops.length,
      "expected 2 reviewer-qa loops (first pass needs_clarification + recovery accept)",
    ).toBe(2);
    for (const q of reviewerQaLoops) {
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

interface Triple {
  subject: string;
  predicate: string;
  object: string;
  source?: string;
  timestamp?: string;
}

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
