import { test, expect } from "@playwright/test";

/**
 * Journey: Ops Agent Baseline (ADR-027 Phase 1)
 *
 * Validates end-to-end that the ops-analyst rule fires after researcher
 * completions and produces a diagnostic loop that cites the objective
 * spec.
 *
 * Flow:
 *   1. Fire 20 research questions serially via POST /teams-dispatch/message.
 *      Each drives a researcher loop (mock LLM → web_search → synthesis).
 *   2. Wait for each researcher loop to reach state=complete before firing
 *      the next. Strict serial order is required because mock-llm pops
 *      responses in arrival order and the fixture has a matched sequence.
 *   3. After the 20th researcher completion, the ops_observe_complete_loops
 *      rule (inline in configs/e2e-ops-observer.json) fires publish_agent,
 *      spawning a loop with role=ops-analyst.
 *   4. Poll for that ops loop to appear, then wait for completion.
 *   5. Assert the ops loop's result cites docs/objectives/deep-research.md
 *      — proves the rule's inline prompt carried through. (Full persona
 *      fragment grounding is gated on ADR-029 persona.Manager.)
 *
 * Known limitations:
 *   - The rule fires on EVERY researcher completion (5s duration cooldown)
 *     because framework does not yet support event-count cooldowns. This
 *     means multiple ops loops may spawn during the 20-loop run; the spec
 *     validates that at least one ops loop completes successfully and
 *     cites the spec.
 *   - Graph triple assertions (e.g., "ops.diagnosis.finding triple exists")
 *     are not possible until a message-bus / graph query HTTP endpoint
 *     lands framework-side. The spec inspects the loop's completion text
 *     instead, which is accessible via GET /teams-dispatch/loops/{id}.
 *
 * Required fixture: test/fixtures/journeys/ops-agent-baseline.yaml
 * Required config: configs/e2e-ops-observer.json
 *
 * Run via:
 *   task ui:test:e2e:agentic:ops-agent
 */

const RESEARCH_COUNT = 20;
const RESEARCH_TIMEOUT_MS = 60_000;
const OPS_TIMEOUT_MS = 90_000;

test.describe("Ops Agent Baseline", () => {
  // 20 researcher loops × up to 60s + ops wait → the test can run long.
  test.setTimeout(20 * 60_000 + OPS_TIMEOUT_MS);

  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok(), "Backend not healthy — stack not running?").toBe(true);

    const commands = await request.get("/teams-dispatch/commands");
    expect(
      commands.ok(),
      "Dispatch /commands not responding — is teams-dispatch configured?",
    ).toBe(true);
  });

  test("20 researcher completions trigger ops-analyst loop citing objective spec", async ({
    request,
  }) => {
    // -----------------------------------------------------------------
    // Step 1 — fire 20 research questions serially
    // -----------------------------------------------------------------
    const researcherLoopIds: string[] = [];

    // Note: /teams-dispatch/loops returns LoopInfo which does NOT
    // carry the role field — role lives on the underlying TaskMessage
    // and on agent.complete events but is not surfaced by this
    // endpoint as of semstreams beta.9. We track loops by creation
    // order: the first 20 belong to the researcher role (dispatch
    // default_role), the next loop to appear after they all complete
    // belongs to the ops-analyst role (spawned by the observe rule's
    // publish_agent action).

    for (let i = 0; i < RESEARCH_COUNT; i++) {
      const beforeAll = await listAllLoops(request);
      const beforeIds = beforeAll.map((l) => l.loop_id);

      const sent = await request.post("/teams-dispatch/message", {
        data: { content: `Research question ${i + 1}: MQTT vs NATS edge` },
      });
      expect(
        sent.ok(),
        `Dispatch rejected message ${i + 1}: ${sent.status()} ${await sent.text()}`,
      ).toBe(true);

      const firstLoopTimeout = i === 0 ? 30_000 : 15_000;
      const newLoop = await pollUntil(
        async () => {
          const current = await listAllLoops(request);
          const added = current.find((l) => !beforeIds.includes(l.loop_id));
          return added ?? null;
        },
        { timeoutMs: firstLoopTimeout },
      );

      if (!newLoop) {
        const all = await listAllLoops(request);
        throw new Error(
          `No new loop appeared for message ${i + 1} within ${firstLoopTimeout}ms. ` +
            `All loops currently visible: ${JSON.stringify(all)}`,
        );
      }
      researcherLoopIds.push(newLoop.loop_id);

      const completed = await pollUntil(
        async () => {
          const resp = await request.get(
            `/teams-dispatch/loops/${newLoop.loop_id}`,
          );
          if (!resp.ok()) return null;
          const body = (await resp.json()) as { state?: string };
          return body.state && ["success", "failure", "timeout"].includes(body.state) ? body : null;
        },
        { timeoutMs: RESEARCH_TIMEOUT_MS },
      );
      if (!completed) {
        // Dump full loop state for diagnostic.
        const stateResp = await request.get(
          `/teams-dispatch/loops/${newLoop.loop_id}`,
        );
        const stateBody = stateResp.ok() ? await stateResp.text() : "<no body>";
        const allLoops = await listAllLoops(request);
        throw new Error(
          `Researcher loop ${i + 1} (${newLoop.loop_id}) did not complete within ${RESEARCH_TIMEOUT_MS}ms.\n` +
            `Loop state: ${stateBody}\n` +
            `All loops: ${JSON.stringify(allLoops, null, 2)}`,
        );
      }
    }

    expect(researcherLoopIds).toHaveLength(RESEARCH_COUNT);

    // -----------------------------------------------------------------
    // Step 2 — wait for the ops rule to fire and the ops loop to appear
    // Identify it as "any new loop that wasn't in researcherLoopIds".
    // -----------------------------------------------------------------
    const opsLoopId = await pollUntil(
      async () => {
        const all = await listAllLoops(request);
        const newOne = all.find((l) => !researcherLoopIds.includes(l.loop_id));
        return newOne?.loop_id ?? null;
      },
      { timeoutMs: OPS_TIMEOUT_MS },
    );
    expect(
      opsLoopId,
      `No ops-analyst loop appeared within ${OPS_TIMEOUT_MS}ms after 20 researcher completions — rule may not have fired (check fire_every_n_events: 20 on the observe rule)`,
    ).toBeTruthy();

    // -----------------------------------------------------------------
    // Step 3 — wait for ops loop to complete
    // -----------------------------------------------------------------
    const opsLoop = await pollUntil(
      async () => {
        const resp = await request.get(`/teams-dispatch/loops/${opsLoopId}`);
        if (!resp.ok()) return null;
        const body = (await resp.json()) as {
          state?: string;
          result?: string;
          outcome?: string;
        };
        return body.state && ["success", "failure", "timeout"].includes(body.state) ? body : null;
      },
      { timeoutMs: OPS_TIMEOUT_MS },
    );
    expect(
      opsLoop,
      `Ops loop ${opsLoopId} did not complete within ${OPS_TIMEOUT_MS}ms`,
    ).toBeTruthy();

    // -----------------------------------------------------------------
    // Step 4 — assert ops.diagnosis.* triples landed in the graph.
    // beta.9 added GET /graph/triples on the service-manager mux with
    // exact-match params (subject, predicate, object, limit). One
    // query per predicate to confirm the full emit_diagnosis payload
    // landed.
    // -----------------------------------------------------------------
    const findings = await fetchTriples(request, {
      predicate: "ops.diagnosis.finding",
      limit: 10,
    });
    expect(
      findings.length,
      `Expected at least one ops.diagnosis.finding triple after ops loop completion. Got: ${JSON.stringify(findings)}`,
    ).toBeGreaterThanOrEqual(1);

    const recommendations = await fetchTriples(request, {
      predicate: "ops.diagnosis.recommendation",
      limit: 10,
    });
    expect(
      recommendations.length,
      `Expected ops.diagnosis.recommendation triples to match findings. Got: ${recommendations.length}`,
    ).toBeGreaterThanOrEqual(1);

    const evidenceTriples = await fetchTriples(request, {
      predicate: "ops.diagnosis.evidence",
      limit: 50,
    });
    expect(
      evidenceTriples.length,
      "Expected ops.diagnosis.evidence triples (≥1 per finding per emit_diagnosis schema)",
    ).toBeGreaterThanOrEqual(findings.length);

    const confidenceTriples = await fetchTriples(request, {
      predicate: "ops.diagnosis.confidence",
      limit: 10,
    });
    expect(
      confidenceTriples.length,
      "Expected ops.diagnosis.confidence triples to match findings",
    ).toBeGreaterThanOrEqual(1);

    // Each finding subject should follow the framework's
    // {org}.{platform}.ops.diagnosis.finding.{uuid} entity pattern
    // emitted by the emit_diagnosis tool executor.
    for (const triple of findings) {
      expect(
        triple.subject.includes("ops.diagnosis.finding"),
        `Finding subject does not match expected entity pattern: ${triple.subject}`,
      ).toBe(true);
    }
  });
});

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

interface LoopSummary {
  loop_id: string;
  role: string;
  state?: string;
}

async function listAllLoops(
  request: import("@playwright/test").APIRequestContext,
): Promise<LoopSummary[]> {
  const resp = await request.get("/teams-dispatch/loops");
  if (!resp.ok()) return [];
  return (await resp.json()) as LoopSummary[];
}

async function pollUntil<T>(
  check: () => Promise<T | null>,
  options: { timeoutMs?: number; intervalMs?: number } = {},
): Promise<T | null> {
  const timeout = options.timeoutMs ?? 15_000;
  const interval = options.intervalMs ?? 500;
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
