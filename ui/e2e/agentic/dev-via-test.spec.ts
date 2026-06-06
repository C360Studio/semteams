import { test, expect } from "@playwright/test";

/**
 * Journey: ADR-044 dev-via-test pack (Lisa / Ralph / CBG) mock-LLM
 * journey. The first end-to-end coverage of the pack that shipped in
 * PR #196 — validates the full orchestration through the e2e
 * substrate:
 *
 *   coordinator(dispatch) → request_sandbox (MockRunner Ready)
 *     → decide(dev_via_test)            [rule 01 → Lisa]
 *   Lisa(dev-via-test-plan) → emit_dev_via_test_plan → decide(planned)
 *                                       [rule 02 → CBG plan-review]
 *   CBG(plan-review) → decide(plan_approved)
 *                                       [rule 02b → coordinator]
 *   coordinator → decide(dev_via_test, subtopics=[task])
 *                                       [rule 03 → Ralph]
 *   Ralph(dev-via-test-execute) → emit_measurement(pass) → decide(measured)
 *                                       [rule 04a + 05 → coordinator]
 *   coordinator → decide(dev_via_test_finalize)
 *                                       [rule 06 → CBG work-gate]
 *   CBG(work-gate) → decide(approved)   [rule 07a → final coordinator]
 *   coordinator → decide(respond_direct) → user.response.*
 *
 * The load-bearing assertions: BOTH CBG gates spawn the
 * reviewer-dev-via-test role (plan + work), they emit the DISTINCT
 * tokens (plan_approved vs approved) that keep the two gates' routing
 * disjoint (Slice 6), and the chain terminates with a delivered
 * reply. A regression that collapses the gates, drops a rule, or
 * mis-routes a token fails here.
 *
 * Required fixture: test/fixtures/journeys/dev-via-test.yaml
 * Required config:  configs/e2e-flow-bootstrap.json (dev-via-test
 *   rules wired into rule-processor rules_files).
 * Required env:     SEMTEAMS_SANDBOX_RUNNER=mock on the backend.
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

test.describe("ADR-044 — dev-via-test pack mock-LLM journey", () => {
  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok(), "Backend not healthy — stack not running?").toBe(true);
  });

  // 8-loop chain (4 coordinator + Lisa + Ralph + 2 CBG gates) on the
  // mock-LLM. Generous budget for mock scheduling + the MockRunner
  // request_sandbox round-trip.
  test.setTimeout(120_000);

  test("build-with-tests prompt → plan gate → Ralph → work gate → reply", async ({
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

    // A build-with-verifiable-acceptance ask — the coordinator's
    // decision-contract routes this to dev_via_test (after sandbox).
    await page.getByTestId("chat-input").fill(
      "Add a Go HTTP service that decodes MAVLink HEARTBEAT frames and serves the latest at GET /heartbeat as JSON, using the gomavlib library — with unit tests that assert the parsed fields.",
    );
    await page.getByTestId("send-button").click();

    // Poll until the full chain has settled: both CBG gates present +
    // Lisa + Ralph, every loop terminal.
    const loops = await pollUntil(async () => {
      const resp = await request.get("/teams-dispatch/loops");
      if (!resp.ok()) return null;
      const list = (await resp.json()) as Loop[];
      const cbg = list.filter((l) => l.role === "reviewer-dev-via-test");
      const lisa = list.filter((l) => l.role === "dev-via-test-plan");
      const ralph = list.filter((l) => l.role === "dev-via-test-execute");
      const allTerminal = list.every((l) => TERMINAL.has(l.state ?? ""));
      // Both gates (plan + work) + planner + executor + settled.
      if (cbg.length >= 2 && lisa.length >= 1 && ralph.length >= 1 && allTerminal) {
        // Don't settle until the final coordinator wake-up has
        // delivered — the work-gate going terminal precedes
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
      "expected the dev-via-test chain to settle with 2 CBG gates + Lisa + Ralph, all terminal",
    ).toBeTruthy();
    const settled = loops!;

    // No loop failed — the whole chain converged.
    const failed = settled.filter((l) =>
      ["failed", "error", "cancelled", "truncated"].includes(l.state ?? ""),
    );
    expect(
      failed.map((l) => `${l.role}:${l.state}`),
      "no dev-via-test loop should have failed",
    ).toEqual([]);

    // Role distribution: exactly the pack's shape — 1 planner, 1
    // executor (single-task plan), 2 reviewer gates, ≥3 coordinator
    // (dispatch + 2 between-task + final).
    const byRole = (r: string) => settled.filter((l) => l.role === r).length;
    expect(byRole("dev-via-test-plan"), "one Lisa (planner)").toBe(1);
    expect(byRole("dev-via-test-execute"), "one Ralph (1-task plan)").toBe(1);
    expect(
      byRole("reviewer-dev-via-test"),
      "TWO CBG gates — plan-review at chain-start + work-gate at chain-end",
    ).toBe(2);

    // The decisive proof: the chain flowed through BOTH gates with the
    // DISTINCT per-phase verdict tokens (Slice 6 disjoint routing).
    // Collect every coordinator.decision.next_action stamped.
    const actionTriples = await fetchTriples(request, {
      predicate: "coordinator.decision.next_action",
      limit: 50,
    });
    const actions = new Set(actionTriples.map((t) => String(t.object ?? "")));
    for (const want of [
      "dev_via_test", // initial dispatch + walker dispatch
      "planned", // Lisa terminal
      "plan_approved", // PLAN gate verdict (distinct token)
      "measured", // Ralph converged
      "dev_via_test_finalize", // walk → CBG work-gate
      "approved", // WORK gate verdict (distinct token)
      "respond_direct", // final delivery
    ]) {
      expect(
        actions.has(want),
        `expected a coordinator.decision.next_action=${want} triple — saw: ${[...actions].sort().join(", ")}`,
      ).toBe(true);
    }

    // Sandbox was provisioned for the pack (request_sandbox → MockRunner).
    const ready = await fetchTriples(request, {
      predicate: "sandbox.attestation.ready",
      limit: 5,
    });
    expect(
      ready.length,
      "expected a sandbox.attestation.ready triple — the coordinator provisions a sandbox before dispatching dev_via_test",
    ).toBeGreaterThanOrEqual(1);

    // Terminal: rule 03b published the user reply on respond_direct.
    const resp = await request.get(
      "/message-logger/entries?subject_prefix=dispatch.user.response&limit=10",
    );
    expect(resp.ok(), "/message-logger/entries non-OK").toBe(true);
    const payloads = (await resp.json()) as Array<{ subject: string }>;
    expect(
      payloads.length,
      "expected a user.response.* publish (final coordinator respond_direct)",
    ).toBeGreaterThanOrEqual(1);
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
