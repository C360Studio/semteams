import { test, expect } from "@playwright/test";

/**
 * Journey: ADR-053 Phase 3b — dev-via-test PLAN-RETRY (re-plan) path on
 * mock-LLM. The happy-path journey (dev-via-test.spec.ts) approves the
 * plan first pass and never exercises the re-plan loop. This journey
 * drives ONE plan_rejected_retry so the chain flows through:
 *
 *   Lisa v1 → decide(planned)              [rule 02 first-pass → CBG plan-review]
 *   CBG #1  → decide(plan_rejected_retry)  [rule 02c stamp + 02d driver re-dispatch]
 *   Lisa v2 (re-plan) → decide(planned)    [rule 02g RE-PLAN re-source → CBG plan-review]
 *   CBG #2  → decide(plan_approved)        [rule 02b → coordinator → Ralph → work gate]
 *
 * This is the e2e regression guard for the go-reviewer C1 fix (the
 * rule-02 split). The mock ignores tool reads, so it validates that
 * rule 02g FIRES on the re-plan Lisa (without 02g, re-plan Lisa's
 * `planned` terminal matches NO rule — rule 02 is excluded by its
 * `lineage.run-loop-entity-id length_eq 0` first-pass fence — so the
 * chain would wedge with no second plan-review, no plan_approved, no
 * Ralph, no reply). The anchor-source correctness (02g reads
 * lineage.run-loop-entity-id, not the absent agent.run.entity_id) is
 * pinned by the contract test TestDevViaTestPack_02g_ReplanReSource;
 * here we prove the wiring + routing end-to-end.
 *
 * Load-bearing assertions: TWO dev-via-test-plan loops (the re-plan
 * happened), THREE reviewer-dev-via-test loops (plan-review #1 reject +
 * plan-review #2 re-gate via 02g + work-gate), the plan_rejected_retry
 * AND a subsequent plan_approved verdict, and a delivered reply.
 *
 * Required fixture: test/fixtures/journeys/dev-via-test-replan.yaml
 * Required config:  configs/e2e-flow-bootstrap.json (incl. rule 02g).
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

test.describe("ADR-053 Phase 3b — dev-via-test re-plan (02g) mock-LLM journey", () => {
  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok(), "Backend not healthy — stack not running?").toBe(true);
  });

  // Re-plan adds a CBG plan-review #2 + a second Lisa over the happy
  // path's 8 loops. Generous budget for mock scheduling + the
  // MockRunner request_sandbox round-trip + the extra hop.
  test.setTimeout(150_000);

  test("plan_rejected_retry → re-plan → 02g re-gate → approve → reply", async ({
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
      "Add a Go HTTP service that decodes MAVLink HEARTBEAT frames and serves the latest at GET /heartbeat as JSON, using the gomavlib library — with unit tests that assert the parsed fields.",
    );
    await page.getByTestId("send-button").click();

    // Poll until the re-plan chain settles: 2 Lisa + 3 CBG (reject +
    // re-gate + work) + Ralph, all terminal, and the final reply sent.
    const loops = await pollUntil(async () => {
      const resp = await request.get("/teams-dispatch/loops");
      if (!resp.ok()) return null;
      const list = (await resp.json()) as Loop[];
      const cbg = list.filter((l) => l.role === "reviewer-dev-via-test");
      const lisa = list.filter((l) => l.role === "dev-via-test-plan");
      const ralph = list.filter((l) => l.role === "dev-via-test-execute");
      const allTerminal = list.every((l) => TERMINAL.has(l.state ?? ""));
      // Re-plan shape: 2 planners (v1 + re-plan), 3 reviewers (plan #1 +
      // plan #2 re-gate + work), ≥1 executor, all settled.
      if (
        cbg.length >= 3 &&
        lisa.length >= 2 &&
        ralph.length >= 1 &&
        allTerminal
      ) {
        const acts = await fetchTriples(request, {
          predicate: "coordinator.decision.next_action",
          limit: 50,
        });
        if (acts.some((t) => String(t.object ?? "") === "respond_direct")) {
          return list;
        }
      }
      return null;
    }, { timeoutMs: 130_000 });

    expect(
      loops,
      "expected the re-plan chain to settle with 2 Lisa + 3 CBG + Ralph, all terminal",
    ).toBeTruthy();
    const settled = loops!;

    const failed = settled.filter((l) =>
      ["failed", "error", "cancelled", "truncated"].includes(l.state ?? ""),
    );
    expect(
      failed.map((l) => `${l.role}:${l.state}`),
      "no dev-via-test loop should have failed",
    ).toEqual([]);

    const byRole = (r: string) => settled.filter((l) => l.role === r).length;
    // TWO planners — proof the re-plan (rule 02d re-dispatch) happened.
    expect(
      byRole("dev-via-test-plan"),
      "TWO Lisa loops — first plan + the re-plan driven by rule 02d",
    ).toBe(2);
    // THREE reviewer gates — plan-review #1 (reject) + plan-review #2
    // (the re-gate spawned by rule 02g) + the chain-end work-gate. If
    // rule 02g didn't fire, the re-plan Lisa's `planned` terminal would
    // match no rule and the chain would wedge at 1 reviewer.
    expect(
      byRole("reviewer-dev-via-test"),
      "THREE CBG gates — plan-review reject + plan-review re-gate (rule 02g) + work-gate",
    ).toBe(3);
    expect(byRole("dev-via-test-execute"), "one Ralph (1-task plan)").toBe(1);

    // The verdict trail proves the reject→re-gate→approve flow.
    const actionTriples = await fetchTriples(request, {
      predicate: "coordinator.decision.next_action",
      limit: 50,
    });
    const actions = new Set(actionTriples.map((t) => String(t.object ?? "")));
    for (const want of [
      "dev_via_test", // dispatch + walker
      "planned", // Lisa terminal (both passes)
      "plan_rejected_retry", // PLAN gate bounce → rule 02c/02d
      "plan_approved", // re-gate verdict (rule 02g → CBG #2 → rule 02b)
      "measured", // Ralph converged
      "dev_via_test_finalize", // walk → work gate
      "approved", // WORK gate verdict
      "respond_direct", // final delivery
    ]) {
      expect(
        actions.has(want),
        `expected coordinator.decision.next_action=${want} — saw: ${[...actions].sort().join(", ")}`,
      ).toBe(true);
    }

    // The plan-retry marker was stamped on the run entity (rule 02c).
    const findings = await fetchTriples(request, {
      predicate: "dev_via_test.plan.retry.finding",
      limit: 10,
    });
    expect(
      findings.length,
      "expected a dev_via_test.plan.retry.finding triple (rule 02c stamped CBG's re-plan finding on the run entity)",
    ).toBeGreaterThanOrEqual(1);
    // It lands on the chain.execution run entity (ADR-053 Phase 3b),
    // NOT a coordinator agentic-loop entity.
    expect(
      String(findings[0]?.subject ?? ""),
      "plan.retry.finding must be stamped on the agent.chain.execution run entity",
    ).toContain("agent.chain.execution.");

    // Terminal: the final coordinator delivered the reply.
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
