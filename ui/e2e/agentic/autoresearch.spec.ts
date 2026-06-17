import { test, expect } from "@playwright/test";
import { assertAnchorBornFirst, RUN_ANCHOR } from "./born_first";

/**
 * Journey: autoresearch pack (Karpathy propose/execute iteration
 * loop) mock-LLM journey. First Playwright coverage of the pack.
 *
 *   coordinator(dispatch) → request_sandbox → decide(autoresearch)
 *                                       [rule 01 → baseline + markers]
 *   baseline → emit_autoresearch_baseline(cap=2) → decide(propose)
 *                                       [rule 05 iter 1 → propose]
 *   propose → decide(measure)           [rule 03 → execute]
 *   execute → emit_measurement(kept) → decide(measured)
 *                                       [rule 04a → rule 05 iter 2 → propose]
 *   …(second iteration)… → rule 05 iter 3 > cap → synthesize
 *   synthesize → emit_artifact → decide(emit)   [rule 07 → reviewer]
 *   reviewer → decide(approved)         [rule 08 → coordinator]
 *   coordinator → decide(respond_direct) → user.response.*
 *
 * Load-bearing assertions: the rule-05 presence-marker loop ran
 * exactly cap=2 iterations (2 propose + 2 execute), best.value was
 * promoted below baseline (the empirical keep), and the chain
 * terminated with a delivered reply. A regression in the iteration
 * dispatch (rule 05), the cap gate, or best-promotion (rule 04c)
 * fails here.
 *
 * Required fixture: test/fixtures/journeys/autoresearch.yaml
 * Required config:  configs/e2e-flow-bootstrap.json (autoresearch
 *   rules wired into rule-processor).
 * Required env:     SEMTEAMS_SANDBOX_RUNNER=mock.
 */

interface Loop {
  loop_id?: string;
  role?: string | null;
  state?: string;
}
interface Triple {
  subject?: string;
  predicate?: string;
  object?: unknown;
}

const TERMINAL = new Set([
  "complete",
  "success",
  "failed",
  "error",
  "cancelled",
  "truncated",
]);

test.describe("autoresearch — propose/execute iteration mock-LLM journey", () => {
  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok(), "Backend not healthy — stack not running?").toBe(true);
  });

  test.setTimeout(120_000);

  test("optimize prompt → baseline → 2 kept iterations → approved reply", async ({
    page,
    request,
  }) => {
    await page.goto("/");
    await expect(page.getByTestId("connection-status")).toHaveAttribute(
      "data-summary",
      "healthy",
      { timeout: 15_000 },
    );
    await expect(page.getByTestId("kanban-board")).toBeVisible();

    await page.getByTestId("chat-input").fill(
      "Make `go test ./...` faster — measure the wallclock, propose changes, and keep the ones that lower it. Cap at 2 iterations.",
    );
    await page.getByTestId("send-button").click();

    // Poll until the iteration loop has run its course: baseline +
    // 2 propose + 2 execute + synthesize + reviewer, all terminal.
    const loops = await pollUntil(async () => {
      const resp = await request.get("/teams-dispatch/loops");
      if (!resp.ok()) return null;
      const list = (await resp.json()) as Loop[];
      const n = (r: string) => list.filter((l) => l.role === r).length;
      const allTerminal = list.every((l) => TERMINAL.has(l.state ?? ""));
      if (
        n("autoresearch-baseline") >= 1 &&
        n("autoresearch-propose") >= 2 &&
        n("autoresearch-execute") >= 2 &&
        n("autoresearch-synthesize") >= 1 &&
        n("reviewer-autoresearch") >= 1 &&
        allTerminal
      ) {
        // Don't settle until the wake-up coordinator has actually
        // delivered — reviewer going terminal precedes the final
        // respond_direct by a beat.
        const acts = await fetchTriples(request, {
          predicate: "coordinator.decision.next_action",
          limit: 50,
        });
        if (acts.some((t) => String(t.object ?? "") === "respond_direct")) {
          return list;
        }
      }
      return null;
    }, { timeoutMs: 100_000 });

    expect(
      loops,
      "expected the autoresearch chain to settle: baseline + 2 propose + 2 execute + synthesize + reviewer, all terminal",
    ).toBeTruthy();
    const settled = loops!;

    const failed = settled.filter((l) =>
      ["failed", "error", "cancelled", "truncated"].includes(l.state ?? ""),
    );
    expect(
      failed.map((l) => `${l.role}:${l.state}`),
      "no autoresearch loop should have failed",
    ).toEqual([]);

    // The cap gate held: EXACTLY 2 iterations (not 1, not 3+).
    const n = (r: string) => settled.filter((l) => l.role === r).length;
    expect(n("autoresearch-propose"), "cap=2 → exactly 2 propose loops").toBe(2);
    expect(n("autoresearch-execute"), "cap=2 → exactly 2 execute loops").toBe(2);
    // Exactly one baseline + one synthesize — a spurious second spawn
    // (rule misfire) would otherwise pass the >=1 poll gate.
    expect(n("autoresearch-baseline"), "exactly one baseline loop").toBe(1);
    expect(n("autoresearch-synthesize"), "exactly one synthesize loop").toBe(1);

    // The chain flowed through every stage's terminal token.
    const actionTriples = await fetchTriples(request, {
      predicate: "coordinator.decision.next_action",
      limit: 50,
    });
    const actions = new Set(actionTriples.map((t) => String(t.object ?? "")));
    for (const want of [
      "autoresearch",
      "propose",
      "measure",
      "measured",
      "emit",
      "approved",
      "respond_direct",
    ]) {
      expect(
        actions.has(want),
        `expected coordinator.decision.next_action=${want} — saw: ${[...actions].sort().join(", ")}`,
      ).toBe(true);
    }

    // Empirical keep: best.value promoted below baseline (1.20 → 0.85).
    // Proves rule 04c fired on the kept measurements.
    const best = await fetchTriples(request, {
      predicate: "autoresearch.best.value",
      limit: 10,
    });
    // EXACTLY one — rule 04c upserts (remove-then-add) best.value, so
    // a clean run leaves a single triple. Pinning length===1 (not >=1)
    // + reading best[0] means a regression to append-only behaviour
    // (stale 1.20 + new 0.85 both present) FAILS here instead of
    // passing vacuously off the most-recent element.
    expect(
      best.length,
      "expected exactly one autoresearch.best.value triple (rule 04c upsert)",
    ).toBe(1);
    const bestVal = Number(best[0]?.object);
    expect(
      bestVal,
      `expected best.value (${bestVal}) promoted below the 1.20 baseline`,
    ).toBeLessThan(1.2);

    // Terminal user reply published.
    const resp = await request.get(
      "/message-logger/entries?subject_prefix=dispatch.user.response&limit=10",
    );
    expect(resp.ok(), "/message-logger/entries non-OK").toBe(true);
    const payloads = (await resp.json()) as Array<{ subject: string }>;
    expect(
      payloads.length,
      "expected a user.response.* publish (coordinator respond_direct)",
    ).toBeGreaterThanOrEqual(1);

    // ADR-053 Phase 4a — the run reached `completed`, NOT `failed`/`dispatched`/
    // `executing`. This is the direct agent.run.phase assertion the design spike
    // names as the merge gate (§D/§H): mock's fast coordinator is the POSITIVE D3-
    // race detector. It is also the regression guard for go-reviewer C1 — the
    // autoresearch success stamp (rule 08, subject lineage.run-loop-entity-id)
    // only resolves because rule 07 threads run-loop-entity-id to the reviewer;
    // without it the run hangs in `executing` and this assertion goes red.
    const runPhases = await pollUntil(async () => {
      const phases = await fetchTriples(request, {
        predicate: "agent.run.phase",
        limit: 20,
      });
      const objs = phases
        .filter((t) => String(t.subject ?? "").includes("agent.chain.execution."))
        .map((t) => String(t.object));
      // Settle on a terminal phase (completed) or fail fast on failed.
      if (objs.includes("completed") || objs.includes("failed")) return objs;
      return null;
    }, { timeoutMs: 30_000 });
    expect(
      runPhases,
      "agent.run.phase never reached a terminal on the run entity (run stuck in dispatched/executing — handoff or success transition did not fire)",
    ).toBeTruthy();
    expect(
      runPhases,
      "run must reach completed (ADR-053 Phase 4a executing→completed)",
    ).toContain("completed");
    expect(
      runPhases,
      "run must NOT be failed — a failed run here means the D3 zombie guard raced the dispatched→executing transition",
    ).not.toContain("failed");

    // BORN-FIRST gate (ADR-055/056 must-exist flip, semteams#222). rule 04c
    // upserts autoresearch.best.value onto the run anchor (lineage.run-loop-entity-id
    // → agent.chain.execution.*). Prove that anchor was BORN-FIRST by the lifecycle
    // Manager (carries agent.run.phase), not auto-vivified by the best.value marker.
    // Fails under simulated auto-vivify (a stub would carry only best.value).
    await assertAnchorBornFirst(request, {
      markerPredicate: "autoresearch.best.value",
      envelopePredicate: RUN_ANCHOR.envelope,
      anchorSubstr: RUN_ANCHOR.substr,
      label: "autoresearch/04c best.value (run anchor)",
    });
  });
});

async function fetchTriples(
  request: import("@playwright/test").APIRequestContext,
  params: { subject?: string; predicate?: string; limit?: number },
): Promise<Triple[]> {
  const query = new URLSearchParams();
  if (params.subject) query.set("subject", params.subject);
  if (params.predicate) query.set("predicate", params.predicate);
  if (params.limit) query.set("limit", String(params.limit));
  const resp = await request.get(`/graph/triples?${query.toString()}`);
  if (!resp.ok()) {
    throw new Error(
      `GET /graph/triples?${query.toString()} returned ${resp.status()}: ${await resp.text()}`,
    );
  }
  return (await resp.json()) as Triple[];
}

async function pollUntil<T>(
  fn: () => Promise<T | null>,
  opts: { timeoutMs: number; intervalMs?: number },
): Promise<T | null> {
  const deadline = Date.now() + opts.timeoutMs;
  const interval = opts.intervalMs ?? 300;
  while (Date.now() < deadline) {
    const result = await fn();
    if (result != null) return result;
    await new Promise((r) => setTimeout(r, interval));
  }
  return null;
}
