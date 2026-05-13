import { test, expect } from "@playwright/test";

/**
 * Journey: Dev-via-Spec golden path (ADR-041 MVP role compression)
 *
 * Validates the dev-via-spec arc end-to-end on mock-LLM with the
 * compressed MVP roster (researcher-{plan,gather,synthesize,architect}
 * + reviewer-spec + builder + reviewer-qa). Every gate approves on
 * first pass; the recovery path (reviewer-qa needs_clarification via
 * rule 03) is exercised by the sibling dev-via-spec-qa.spec.ts.
 *
 * Sequence (seven loops):
 *
 *   Loop A: researcher-plan (dispatch) → emit_plan → decide(gather)
 *           → rule 04 → spawn researcher-gather
 *
 *   Loop B: researcher-gather → read_loop_result → query_entity →
 *           decide(synthesize) → rule 05 → spawn researcher-synthesize
 *
 *   Loop C: researcher-synthesize → read_loop_result →
 *           emit_research_artifact (rev 1) → decide(architect)
 *           → rule 06 → spawn researcher-architect
 *
 *   Loop D: researcher-architect → read_loop_result →
 *           emit_dev_via_spec_artifact → decide(emit)
 *           → rule 01b → spawn reviewer-spec
 *
 *   Loop E: reviewer-spec → read_loop_result → decide(approved)
 *           → phasevalidator.SpecModeGate forwards
 *             dev_via_spec.artifact.{path,slug} → rule 02 →
 *             spawn builder
 *
 *   Loop F: builder → bootstrap_workspace → bash ×2 →
 *           builder_decide(tests_passing)
 *           → phasevalidator.QAModeGate → rule 04 →
 *             spawn reviewer-qa
 *
 *   Loop G: reviewer-qa → read_loop_result → decide(accept)
 *           → terminal. No rule fires on action=accept.
 *
 * Validates:
 *   - All seven loops appear with the expected role distribution.
 *   - Rules 04/05/06 fire in order on the researcher chain
 *     (phase-transition forward edges).
 *   - Rule 01b fires on the researcher-architect's decide(emit).
 *   - phasevalidator.SpecModeGate stamps chain.spec_mode_gate.proceed
 *     after the reviewer-spec approves, triggering rule 02 → builder.
 *   - phasevalidator.QAModeGate stamps chain.qa_mode_gate.proceed
 *     after the builder's tests_passing, triggering rule 04 →
 *     reviewer-qa.
 *   - The MVP chain runs autonomously: no human approval gate
 *     (substrate-mutation is out of MVP scope per ADR-041 §1).
 *   - Final state: all seven loops complete; no eighth loop appears
 *     (reviewer-qa's accept does NOT trigger rule 03).
 *
 * Required fixture: test/fixtures/journeys/dev-via-spec.yaml
 * Required compose profile: sandbox (builder bootstrap_workspace +
 * bash routing).
 */

test.describe("Dev-via-Spec golden path (ADR-041 MVP)", () => {
  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok()).toBe(true);
  });

  // Seven loops, autonomous chain. 4 minutes covers mock-LLM
  // scheduling jitter + sandbox cold-start latency on slower CI.
  test.setTimeout(240_000);

  test("plan → gather → synthesize → architect → reviewer-spec → builder → reviewer-qa (ADR-041 MVP)", async ({
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
    // Step 2 — type the bounded prompt. Same shape as research-mode-
    // transition.spec.ts; the MVP chain extends the research arc
    // through architect/reviewer-spec/builder/reviewer-qa rather than
    // terminating at reviewer-research.
    // -----------------------------------------------------------------
    const chatInput = page.getByTestId("chat-input");
    await chatInput.fill(
      "Build the OSH Meshtastic driver — IDriver interface backed by Meshtastic radio packets, exposing OGC CS observation endpoints.",
    );
    await page.getByTestId("send-button").click();

    // -----------------------------------------------------------------
    // Step 3 — wait for ALL SEVEN loops to reach terminal complete
    // state. The MVP chain is autonomous: dispatch researcher-plan
    // through architect terminal, reviewer-spec approve, builder
    // tests_passing, reviewer-qa accept.
    //
    // Critical assertions: rules 04/05/06 (phase-transition forward
    // edges) fire in order, rule 01b fires on architect terminal,
    // phasevalidator.SpecModeGate + QAModeGate stamp proceed
    // sentinels, rules 02 (reviewer-spec→builder) + 04 dev-via-spec
    // (builder→reviewer-qa) fire on their respective gate sentinels.
    // -----------------------------------------------------------------
    const allTerminal = await pollUntil(async () => {
      const resp = await request.get("/teams-dispatch/loops");
      if (!resp.ok()) return null;
      const list = (await resp.json()) as Array<{ state: string }>;
      if (list.length !== 7) return null;
      return list.every((l) => l.state === "complete") ? list : null;
    }, { timeoutMs: 180000 });

    expect(
      allTerminal,
      "expected all 7 loops to reach terminal complete state (researcher-plan/gather/synthesize/architect + reviewer-spec + builder + reviewer-qa)",
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
    //   1 × researcher-plan       (Loop A — dispatch)
    //   1 × researcher-gather     (Loop B — rule 04)
    //   1 × researcher-synthesize (Loop C — rule 05)
    //   1 × researcher-architect  (Loop D — rule 06)
    //   1 × reviewer-spec         (Loop E — rule 01b)
    //   1 × builder               (Loop F — rule 02 + spec_mode_gate)
    //   1 × reviewer-qa           (Loop G — rule 04 dev-via-spec +
    //                              qa_mode_gate)
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
      `expected 1 researcher-plan loop (Loop A dispatch), got roles=${JSON.stringify(roles)}`,
    ).toBe(1);
    expect(
      gatherCount,
      `expected 1 researcher-gather loop (Loop B via rule 04). Missing → rule 04 (phase-transition-to-gather) did not fire. roles=${JSON.stringify(roles)}`,
    ).toBe(1);
    expect(
      synthesizeCount,
      `expected 1 researcher-synthesize loop (Loop C via rule 05). Missing → rule 05 (phase-transition-to-synthesize) did not fire. roles=${JSON.stringify(roles)}`,
    ).toBe(1);
    expect(
      architectCount,
      `expected 1 researcher-architect loop (Loop D via rule 06). Missing → rule 06 (phase-transition-to-architect) did not fire. roles=${JSON.stringify(roles)}`,
    ).toBe(1);
    expect(
      reviewerSpecCount,
      `expected 1 reviewer-spec loop (Loop E via rule 01b). Missing → rule 01b (researcher-architect-to-reviewer-spec) did not fire. roles=${JSON.stringify(roles)}`,
    ).toBe(1);
    expect(
      builderCount,
      `expected 1 builder loop (Loop F via rule 02 + spec_mode_gate). Missing → reviewer-spec's decide(approved) did not propagate OR phasevalidator.SpecModeGate did not stamp chain.spec_mode_gate.proceed. roles=${JSON.stringify(roles)}`,
    ).toBe(1);
    expect(
      reviewerQaCount,
      `expected 1 reviewer-qa loop (Loop G via rule 04 dev-via-spec + qa_mode_gate). Missing → builder did not emit tests_passing OR phasevalidator.QAModeGate did not stamp chain.qa_mode_gate.proceed. roles=${JSON.stringify(roles)}`,
    ).toBe(1);

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
    // Step 6 — settle assertion: no eighth loop appears. reviewer-qa's
    // decide(accept) terminal MUST NOT re-fire any rule. Specifically:
    // rule 03 (needs_clarification → researcher-plan) only fires on
    // needs_clarification, NOT accept.
    // -----------------------------------------------------------------
    await new Promise((r) => setTimeout(r, 2000));
    const settledList = await request
      .get("/teams-dispatch/loops")
      .then((r) => r.json()) as unknown[];
    expect(
      settledList.length,
      "reviewer-qa accept terminal must not spawn an eighth loop",
    ).toBe(7);

    // -----------------------------------------------------------------
    // Step 7 — verify the typed research.artifact.v1 publish (Loop C
    // researcher-synthesize) and dev_via_spec.artifact.v1 publish
    // (Loop D researcher-architect). Catches wire-format drift
    // between persona prose, tool schema, and payload shape under
    // mock-llm — smoke #21 is the substance gate.
    //
    // limit=10000 (the message-logger's hard cap); the filter is
    // applied AFTER limit-truncation, so a smaller limit risks
    // evicting early-loop publishes before the assertion sees them.
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
      `expected ≥1 research.artifact.<loop_id> publish from researcher-synthesize, got ${researchArtifactSubjects.length}: ${JSON.stringify(researchArtifactSubjects)}`,
    ).toBeGreaterThanOrEqual(1);

    const specArtifactSubjects = entries
      .map((e) => e.subject)
      .filter((s) => s.startsWith("dev_via_spec.artifact."));
    expect(
      specArtifactSubjects.length,
      `expected ≥1 dev_via_spec.artifact.<loop_id> publish from researcher-architect, got ${specArtifactSubjects.length}: ${JSON.stringify(specArtifactSubjects)}`,
    ).toBeGreaterThanOrEqual(1);

    // ADR-038 PR C Phase C5: planner emits dev_via_spec.plan.<loop_id>.
    // researcher-plan is the MVP planner; emit_plan still publishes
    // the typed payload.
    const planSubjects = entries
      .map((e) => e.subject)
      .filter((s) => s.startsWith("dev_via_spec.plan."));
    expect(
      planSubjects.length,
      `expected ≥1 dev_via_spec.plan.<loop_id> publish from researcher-plan's emit_plan, got ${planSubjects.length}: ${JSON.stringify(planSubjects)}`,
    ).toBeGreaterThanOrEqual(1);

    // -----------------------------------------------------------------
    // Step 8 — verify the phasevalidator gates stamped their proceed
    // sentinels. chain.spec_mode_gate.proceed on the reviewer-spec
    // entity, chain.qa_mode_gate.proceed on the builder entity.
    // -----------------------------------------------------------------
    const specGateProceed = await fetchTriples(request, {
      predicate: "chain.spec_mode_gate.proceed",
      limit: 5,
    });
    expect(
      specGateProceed.length,
      `expected ≥1 chain.spec_mode_gate.proceed triple (SpecModeGate plumbing)`,
    ).toBeGreaterThanOrEqual(1);

    const qaGateProceed = await fetchTriples(request, {
      predicate: "chain.qa_mode_gate.proceed",
      limit: 5,
    });
    expect(
      qaGateProceed.length,
      `expected ≥1 chain.qa_mode_gate.proceed triple (QAModeGate plumbing)`,
    ).toBeGreaterThanOrEqual(1);
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

// fetchTriples queries the framework's GET /graph/triples endpoint
// (semstreams beta.9 added on the service-manager mux). Mirrors the
// helper in ops-agent-baseline.spec.ts; consider promoting both to a
// shared helper module if a third spec needs it.
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
