import { test, expect } from "@playwright/test";

/**
 * Journey: ADR-057 §D5 create_change pack — the REJECT-RECOVERY path
 * (rule 04). The companion to create-change.spec.ts (the happy approve
 * path); this one exercises the reviewer→coordinator recovery the happy
 * path never reaches, giving the rule-04 / anchor_inherit recovery spawn
 * live coverage (go-reviewer Nit 2 on the journey-config slice).
 *
 *   coordinator decide(create_change)            [rule 01 → author1, run R1]
 *   author1 → emit_change → decide(drafted)      [rule 02 → reviewer1]
 *   reviewer1 → decide(rejected)                 [rule 04 → recovery coordinator]
 *   recovery coordinator → decide(create_change) [rule 01 → author2, run R2]
 *   author2 → emit_change → decide(drafted)      [rule 02 → reviewer2]
 *   reviewer2 → decide(approved)   [rule 03 → R2 success + wake-up coordinator]
 *   wake-up coordinator → decide(respond_direct) → user.response.*
 *
 * Load-bearing assertions: TWO author + TWO reviewer loops (the re-draft
 * happened), ≥2 coordinator loops (recovery + wake-up — the happy path has
 * only the wake-up), the `rejected` token (reviewer1 → rule 04), and a run
 * reaches `completed` (R2). A regression that drops rule 04, fences rule 01
 * against the recovery coordinator's re-dispatch, or mis-routes the reject
 * token fails here.
 *
 * Recovery semantics note: the ORIGINAL run R1 stays `executing` — the
 * rejected path stamps no run outcome; R1 is superseded (a fresh run R2),
 * not completed (the cancel/timeout reaper is deferred). So this asserts a
 * run COMPLETED (R2), not that all runs reach a terminal.
 *
 * Required fixture: test/fixtures/journeys/create-change-recovery.yaml
 * Required config:  configs/e2e-flow-bootstrap.json.
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

// PARKED (ADR-058): this journey exercises a dev-side pack that is unwired
// from the bootstrap pending the canonical-predicate migration. Re-enable by
// restoring test.describe when the pack is re-authored and re-wired.
test.describe.skip("ADR-057 — create_change reject-recovery mock-LLM journey", () => {
  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok(), "Backend not healthy — stack not running?").toBe(true);
  });

  // 6-loop chain (front-door + 2 authors + 2 reviewers + recovery + wake-up,
  // front-door role omitempty). Generous budget for the re-dispatch round-trip.
  test.setTimeout(120_000);

  test("rejected change → coordinator re-dispatch → fresh author → approved → reply", async ({
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
      "Draft a spec change to add multi-factor authentication (TOTP) to our auth system: enroll an authenticator, require a code at login.",
    );
    await page.getByTestId("send-button").click();

    // Poll until the re-dispatch has fully settled: 2 authors + 2 reviewers,
    // every loop terminal, and the final respond_direct delivered.
    const loops = await pollUntil(
      async () => {
        const resp = await request.get("/teams-dispatch/loops");
        if (!resp.ok()) return null;
        const list = (await resp.json()) as Loop[];
        const authors = list.filter((l) => l.role === "author-create-change");
        const reviewers = list.filter(
          (l) => l.role === "reviewer-create-change",
        );
        const allTerminal = list.every((l) => TERMINAL.has(l.state ?? ""));
        if (authors.length >= 2 && reviewers.length >= 2 && allTerminal) {
          const acts = await fetchTriples(request, {
            predicate: "coordinator.decision.next_action",
            limit: 50,
          });
          if (acts.some((t) => String(t.object ?? "") === "respond_direct")) {
            return list;
          }
        }
        return null;
      },
      { timeoutMs: 90_000 },
    );

    expect(
      loops,
      "expected the recovery chain to settle with 2 authors + 2 reviewers, all terminal",
    ).toBeTruthy();
    const settled = loops!;

    // No loop failed — the recovery converged voluntarily (rejected is a
    // success-state decide, not a loop failure).
    const failed = settled.filter((l) =>
      ["failed", "error", "cancelled", "truncated"].includes(l.state ?? ""),
    );
    expect(
      failed.map((l) => `${l.role}:${l.state}`),
      "no create-change loop should have failed",
    ).toEqual([]);

    // Exactly two of each — the re-draft produced a second author + reviewer.
    // (Exact, not floor: a double-firing rule 01/02 would over-count.)
    const byRole = (r: string) => settled.filter((l) => l.role === r).length;
    expect(byRole("author-create-change"), "two authors (draft + re-draft)").toBe(2);
    expect(byRole("reviewer-create-change"), "two reviewer gates (reject + approve)").toBe(2);
    // ≥2 coordinator: the recovery coordinator (rule 04) + the wake-up (rule 03).
    // The happy path has only 1 — this floor distinguishes the recovery path.
    // (front-door dispatch loop's role is omitempty per feedback_loopinfo_role_omitempty.)
    expect(
      byRole("coordinator"),
      "≥2 coordinator wake-ups (recovery + final) — the recovery path's signature",
    ).toBeGreaterThanOrEqual(2);

    // The decisive tokens: the chain flowed through reject → re-dispatch → approve.
    const actionTriples = await fetchTriples(request, {
      predicate: "coordinator.decision.next_action",
      limit: 50,
    });
    const actions = new Set(actionTriples.map((t) => String(t.object ?? "")));
    for (const want of [
      "create_change", // front-door dispatch + recovery re-dispatch
      "drafted", // both authors
      "rejected", // reviewer1 → rule 04 (the recovery trigger)
      "approved", // reviewer2 → rule 03
      "respond_direct", // final delivery
    ]) {
      expect(
        actions.has(want),
        `expected a coordinator.decision.next_action=${want} triple — saw: ${[...actions].sort().join(", ")}`,
      ).toBe(true);
    }

    // emit_change ran live on the re-dispatched run too: change.add-mfa.* is
    // stamped on a run entity (both R1 and R2 stamp it — distinct entities, no
    // double-stamp collision, which is the supersession design).
    const changeIntent = await fetchTriples(request, {
      predicate: "change.add-mfa.proposal.intent",
      limit: 10,
    });
    expect(
      changeIntent.length,
      "expected change.add-mfa.proposal.intent — emit_change must have stamped on the run entity",
    ).toBeGreaterThanOrEqual(1);
    expect(
      changeIntent.every((t) =>
        String(t.subject ?? "").includes("agent.chain.execution."),
      ),
      "every change.* must land on a run entity, not a loop",
    ).toBe(true);

    // Terminal: the wake-up coordinator's respond_direct published the reply.
    const resp = await request.get(
      "/message-logger/entries?subject_prefix=dispatch.user.response&limit=10",
    );
    expect(resp.ok(), "/message-logger/entries non-OK").toBe(true);
    const payloads = (await resp.json()) as Array<{ subject: string }>;
    expect(
      payloads.length,
      "expected a user.response.* publish (wake-up coordinator respond_direct)",
    ).toBeGreaterThanOrEqual(1);

    // The re-dispatched run R2 reached `completed` (rule 03 stamped success).
    // R1 (the rejected original) stays `executing` — accepted recovery
    // semantics (superseded, not completed) — so assert completed is PRESENT,
    // and that nothing failed.
    const runPhases = await pollUntil(
      async () => {
        const phases = await fetchTriples(request, {
          predicate: "agent.run.phase",
          limit: 20,
        });
        const objs = phases
          .filter((t) =>
            String(t.subject ?? "").includes("agent.chain.execution."),
          )
          .map((t) => String(t.object));
        if (objs.includes("completed") || objs.includes("failed")) return objs;
        return null;
      },
      { timeoutMs: 30_000 },
    );
    expect(
      runPhases,
      "no run reached a terminal phase (the re-dispatched run R2 should complete)",
    ).toBeTruthy();
    expect(
      runPhases,
      "the re-dispatched run must reach completed (ADR-053 Phase 4a)",
    ).toContain("completed");
    expect(runPhases, "no run should be failed").not.toContain("failed");
  });
});

// NOTE: pollUntil/fetchTriples + the Loop/Triple/TERMINAL shapes are
// duplicated across the agentic specs (see create-change.spec.ts). Follow-up:
// extract to ui/e2e/helpers/agentic.ts.
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
