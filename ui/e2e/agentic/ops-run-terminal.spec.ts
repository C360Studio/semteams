import { test, expect } from "@playwright/test";

/**
 * Journey: ops agent, run-terminal observation (ADR-027, re-wired)
 *
 * Replaces ops-agent-baseline.spec.ts, which had been a standing RED: it
 * required `configs/e2e-ops-observer.json` (deleted in ADR-042 MVP-7), expected
 * a role `ops-analyst` that has no persona, and expected the observe rule to
 * carry `fire_every_n_events: 20` — a field that does not gate `publish_agent`
 * at all (semstreams#1007). It also fired 20 serial research prompts to reach
 * an event count that no longer means anything.
 *
 * What this proves instead:
 *
 *   One research chain runs to a terminal run phase. The ops rule
 *   (configs/rules/ops/01-run-terminal-observe.json) fires on the RUN entity —
 *   not on any role — and spawns exactly one ops-chain-observer, which emits
 *   diagnoses and terminates with decide(action="observed").
 *
 * Why assert what it asserts:
 *
 *   - ONE observer, not one per completed loop. This is the assertion that
 *     would have caught the `fire_every_n_events` misunderstanding: the
 *     retired progress-observer rule declared a throttle that does nothing, so
 *     it would have spawned an LLM loop per worker-loop completion. Cadence is
 *     set by trigger scope, and this is where that contract is pinned.
 *
 *   - Diagnosis triples actually land. `emit_diagnosis` was silently emitting
 *     nothing before upstream gh#390 — it appended to an entity that was never
 *     born — and that failure class is invisible everywhere else, because the
 *     loop still completes and the tool still reports success.
 *
 *   - Predicates share a subject. Asserting "some ops.diagnosis.* triples
 *     exist" would pass on a partial write. Asserting that finding,
 *     recommendation, and confidence sit on the SAME minted entity is what
 *     proves the atomic entity.create.
 *
 *   - The observer carries no `agent.run.*` triple. That is the structural
 *     proof of `run_scope: "none"` — the observer must not become a member of
 *     the run it observes, or its own terminal re-enters the run family.
 *     Without this the isolation is an argument rather than a fact.
 *
 * What it deliberately does NOT assert (mock-LLM theatre — the fixture wrote
 * it, so asserting it tests the fixture author): finding text, whether it
 * names a threshold or reads sensibly, confidence calibration, severity
 * semantics (the executor clamps anything outside {info, warn, critical} to
 * info), hydration discipline, or that evidence entity IDs resolve — the
 * executor validates only that the list is non-empty and never dereferences
 * it, so asserting resolvability would assert a property the system lacks.
 *
 * Required fixture: test/fixtures/journeys/ops-run-terminal.yaml
 * Required config:  configs/e2e-flow-bootstrap.json (ops rule wired)
 *
 * Run via: task ui:test:e2e:agentic:ops-agent
 */

const CHAIN_TIMEOUT_MS = 120_000;
const OPS_TIMEOUT_MS = 90_000;

type Loop = { loop_id: string; role?: string; state?: string };

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

async function fetchTriples(
  request: import("@playwright/test").APIRequestContext,
  params: { subject?: string; predicate?: string; limit?: number },
): Promise<Array<{ subject?: string; predicate?: string; object?: unknown }>> {
  const query = new URLSearchParams();
  if (params.subject) query.set("subject", params.subject);
  if (params.predicate) query.set("predicate", params.predicate);
  if (params.limit) query.set("limit", String(params.limit));
  const resp = await request.get(`/graph/triples?${query.toString()}`);
  if (!resp.ok()) {
    throw new Error(
      `GET /graph/triples?${query.toString()} returned ${resp.status()}`,
    );
  }
  return (await resp.json()) as Array<{
    subject?: string;
    predicate?: string;
    object?: unknown;
  }>;
}

test.describe("Ops run-terminal observation", () => {
  test.setTimeout(CHAIN_TIMEOUT_MS + OPS_TIMEOUT_MS + 60_000);

  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok(), "Backend not healthy — stack not running?").toBe(true);
  });

  test("run reaches terminal → exactly one observer → diagnosis entities minted", async ({
    request,
  }) => {
    // -----------------------------------------------------------------
    // Step 1 — drive one research chain.
    // -----------------------------------------------------------------
    const sent = await request.post("/teams-dispatch/message", {
      data: {
        content:
          "Compare MQTT vs NATS for IoT edge deployments — which has lower latency on constrained ARM devices?",
      },
    });
    expect(
      sent.ok(),
      `dispatch rejected the prompt: ${sent.status()} ${await sent.text()}`,
    ).toBe(true);

    // -----------------------------------------------------------------
    // Step 2 — wait for the observer. It can only appear after the run
    // entity reaches a terminal phase, so its presence is itself proof
    // the run lifecycle closed.
    // -----------------------------------------------------------------
    const settled = await pollUntil(async () => {
      const resp = await request.get("/teams-dispatch/loops");
      if (!resp.ok()) return null;
      const list = (await resp.json()) as Loop[];
      const ops = list.filter((l) => l.role === "ops-chain-observer");
      if (ops.length === 0) return null;
      return ops.every((l) => l.state === "complete") ? list : null;
    }, { timeoutMs: CHAIN_TIMEOUT_MS + OPS_TIMEOUT_MS });

    expect(
      settled,
      "no completed ops-chain-observer appeared. Either the run never reached a terminal phase (check agent-run rules 03/04), or the ops rule did not fire on it (check configs/rules/ops/01-run-terminal-observe.json is in rules_files and its entity.pattern is the chain family)",
    ).toBeTruthy();
    const loops = settled!;

    // -----------------------------------------------------------------
    // Step 3 — EXACTLY one observer. Cadence is set by trigger scope:
    // one run terminal, one observer. More than one means the rule is
    // firing per loop rather than per run.
    // -----------------------------------------------------------------
    const observers = loops.filter((l) => l.role === "ops-chain-observer");
    expect(
      observers.length,
      `expected exactly 1 ops-chain-observer, got ${observers.length}. More than one means the rule is firing per completed loop rather than once per run — note that fire_every_n_events does NOT throttle publish_agent (semstreams#1007), so a throttle field would not save this.`,
    ).toBe(1);

    // -----------------------------------------------------------------
    // Step 4 — diagnosis entities were actually minted.
    // -----------------------------------------------------------------
    const findings = await pollUntil(async () => {
      const t = await fetchTriples(request, {
        predicate: "ops.diagnosis.finding",
        limit: 20,
      });
      return t.length > 0 ? t : null;
    }, { timeoutMs: 30_000 });

    expect(
      findings,
      "no ops.diagnosis.finding triples. The observer completed, so emit_diagnosis was called and reported success — this is the gh#390 shape, where findings were appended to an entity that was never born and silently vanished.",
    ).toBeTruthy();
    expect(findings!.length).toBeGreaterThanOrEqual(1);

    for (const f of findings!) {
      expect(
        f.subject,
        `finding subject ${f.subject} is not a minted ops diagnosis entity`,
      ).toContain(".ops.diagnosis.finding.");
    }

    // -----------------------------------------------------------------
    // Step 5 — the companion predicates share a subject with a finding.
    // "Some ops.diagnosis.* triples exist" would pass on a partial
    // write; same-subject is what proves the atomic entity.create.
    // -----------------------------------------------------------------
    const findingSubjects = new Set(findings!.map((f) => f.subject));
    for (const predicate of [
      "ops.diagnosis.recommendation",
      "ops.diagnosis.confidence",
      "ops.diagnosis.evidence",
    ]) {
      const triples = await fetchTriples(request, { predicate, limit: 20 });
      expect(
        triples.length,
        `expected at least one ${predicate} triple alongside the findings`,
      ).toBeGreaterThanOrEqual(1);
      const shared = triples.some((t) => findingSubjects.has(t.subject));
      expect(
        shared,
        `${predicate} landed on no subject that also carries ops.diagnosis.finding — the diagnosis entity was not written atomically`,
      ).toBe(true);
    }

    // -----------------------------------------------------------------
    // Step 6 — the observer is NOT a member of the run it observed.
    // This is the structural proof of run_scope: "none". If the observer
    // joined the run, its own terminal would re-enter the run family and
    // re-trigger observation.
    // -----------------------------------------------------------------
    const observerLoopID = observers[0].loop_id;
    const observerEntity = loops.find((l) => l.loop_id === observerLoopID);
    expect(observerEntity, "observer vanished from the loop list").toBeTruthy();

    const runTriples = await fetchTriples(request, {
      predicate: "agent.run.entity-id",
      limit: 50,
    });
    const observerCarriesRun = runTriples.some((t) =>
      typeof t.subject === "string" && t.subject.includes(observerLoopID),
    );
    expect(
      observerCarriesRun,
      "the ops observer carries an agent.run.entity-id triple, so it joined the run it was observing — run_scope: \"none\" is not taking effect, and the observer's own terminal can re-trigger observation",
    ).toBe(false);
  });
});
