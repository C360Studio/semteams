import { test, expect } from "@playwright/test";

/**
 * Journey: ADR-053 Phase 4c PR-2 — the tool-gate RESUME. The resume twin of
 * approval-pause.
 *
 *   (pause half — identical to approval-pause)
 *   recovery coordinator → tool_call(create_rule)  loop parks in awaiting_approval;
 *                                                   approvalpause stamps
 *                                                   agent.run.approval_pending;
 *                                                   agent-run/12 → run awaiting_approval
 *
 *   (resume half — this journey)
 *   POST /teams-dispatch/loops/{id}/approval {decision:"approve"}
 *      → dispatch publishes agent.approval_response.<id>
 *      → teams-loop (agent.approval_response input port) un-parks the loop
 *      → approvalpause stamps agent.run.approval_resumed
 *      → agent-run/13 → run awaiting_approval→executing + clears BOTH markers
 *
 * DECISIVE assertions:
 *   - agent.run.approval_pending is CLEARED on the run entity (rule 13 ran),
 *   - run is `executing`, NOT awaiting_approval (and not completed/failed),
 *   - agent.run.approval_resumed is CLEARED too (removed last),
 *   - after a settle window the run has NOT bounced back to awaiting_approval
 *     (exercises rule 12's approval_resumed length_eq 0 guard live).
 *
 * NOTE (per the upstream-fragility flag): this drives the thinly-tested upstream
 * approval RESUME path for real — GetPendingApprovalCallID (the dispatch must have
 * consumed agent.approval_pending), publishApprovalResponse, ResolveApprovalIfPending,
 * and the ApprovalFilter's ApprovedBy bypass. A gap there surfaces as a 409 on the
 * POST or a run that never resumes.
 *
 * Required fixture: test/fixtures/journeys/approval-resume.yaml
 * Required config: configs/e2e-flow-bootstrap.json (approval_required:[create_rule],
 *   agent-run/12+13, the agent.approval_pending + agent.approval_response ports)
 */

const RUN_INFIX = "agent.chain.execution.";
const LOOP_INFIX = "agent.agentic-loop.execution.";

test.describe("ADR-053 Phase 4c — approval resumes the run (awaiting_approval→executing)", () => {
  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok(), "Backend not healthy — stack not running?").toBe(true);
  });

  test.setTimeout(120_000);

  test("operator approves gated create_rule → run resumes awaiting_approval→executing", async ({
    page,
    request,
  }) => {
    // -----------------------------------------------------------------
    // Step 1 — open the Board so the SSE stream connects.
    // -----------------------------------------------------------------
    await page.goto("/");
    await expect(page.getByTestId("connection-status")).toHaveAttribute(
      "data-summary",
      "healthy",
      { timeout: 15000 },
    );
    await expect(page.getByTestId("kanban-board")).toBeVisible();

    // -----------------------------------------------------------------
    // Step 2 — send a dev-build prompt (drives the pause half).
    // -----------------------------------------------------------------
    await page.getByTestId("chat-input").fill(
      "Build a Go HTTP service that decodes MAVLink v2 HEARTBEAT frames over UDP and serves the latest at GET /heartbeat as JSON.",
    );
    await page.getByTestId("send-button").click();

    // -----------------------------------------------------------------
    // Step 3 — wait for the run to PAUSE (awaiting_approval) with the 4c marker.
    // -----------------------------------------------------------------
    const paused = await pollUntil(async () => {
      const phases = await runEntityTriples(request, "agent.run.phase");
      return phases.some((t) => String(t.object) === "awaiting_approval")
        ? phases
        : null;
    }, { timeoutMs: 90_000 });
    expect(
      paused,
      "run never reached awaiting_approval — the gated create_rule did not pause the run (PR-1). Cannot test resume.",
    ).toBeTruthy();

    const pendingBefore = await runEntityTriples(request, "agent.run.approval_pending");
    expect(
      pendingBefore.length,
      "expected agent.run.approval_pending on the run entity before the approval (the 4c pause marker).",
    ).toBeGreaterThanOrEqual(1);

    // -----------------------------------------------------------------
    // Step 4 — the gated loop is the recovery coordinator (rule-spawned, so NOT in
    // the dispatch's /teams-dispatch/loops list). Its bare loop_id — what the approval
    // endpoint keys on — is the bare id of the approval_pending marker's OBJECT (the
    // gated loop entity the approvalpause subscriber recorded). The dispatch still
    // knows the pending CallID for it because it consumed agent.approval_pending
    // (SetPendingApproval handles the unknown-loop case).
    // -----------------------------------------------------------------
    // [0] is unambiguous ONLY because this journey fires exactly one gate → exactly
    // one approval_pending marker. A multi-gate journey would need to disambiguate
    // (/graph/triples has no ordering guarantee).
    const gatedLoopEntity = String(pendingBefore[0].object ?? "");
    const gatedLoopId = bareIdAfter(gatedLoopEntity, LOOP_INFIX);
    expect(
      gatedLoopId,
      `could not extract the gated loop id from the approval_pending object ${gatedLoopEntity}`,
    ).toBeTruthy();

    // -----------------------------------------------------------------
    // Step 5 — POST the operator's APPROVAL through the real dispatch endpoint.
    // This publishes agent.approval_response.<loopId>; the loop un-parks (re-dispatches
    // create_rule with ApprovedBy → ApprovalFilter bypass) and the approvalpause
    // subscriber stamps the run-phase resume marker. The dispatch consumes
    // agent.approval_pending ASYNC, so retry on 409 ("loop not awaiting approval")
    // until it has registered the pending CallID. A persistent 409 means the dispatch
    // never consumed agent.approval_pending (the stream-coverage wiring is broken).
    // -----------------------------------------------------------------
    const approvalStatus = await pollUntil(async () => {
      const resp = await request.post(
        `/teams-dispatch/loops/${gatedLoopId}/approval`,
        { data: { decision: "approve", user_id: "e2e-operator" } },
      );
      if (resp.ok()) return resp.status();
      if (resp.status() === 409) return null; // dispatch not ready yet — retry
      throw new Error(
        `approval POST returned ${resp.status()} for loop ${gatedLoopId} (non-409, non-OK — publish or handler failure): ${await resp.text()}`,
      );
    }, { timeoutMs: 30_000, intervalMs: 500 });
    expect(
      approvalStatus,
      `approval POST never succeeded for loop ${gatedLoopId} (persistent 409 — the dispatch never consumed agent.approval_pending; check AGENT-stream coverage of agent.approval_pending.*)`,
    ).toBeTruthy();

    // -----------------------------------------------------------------
    // Step 6 — DECISIVE: the run RESUMES. agent-run/13 clears approval_pending as
    // part of the awaiting_approval→executing transition, so polling until pending is
    // GONE on the run entity is the durable proof rule 13 ran. A never-resumed run
    // keeps pending set forever; a bounce re-sets it.
    // -----------------------------------------------------------------
    const resumed = await pollUntil(async () => {
      const pending = await runEntityTriples(request, "agent.run.approval_pending");
      return pending.length === 0 ? true : null;
    }, { timeoutMs: 60_000 });
    expect(
      resumed,
      "agent.run.approval_pending was never cleared after the approval — the resume (approvalpause → agent-run/13) did not fire. Run still parked. Current phases: " +
        JSON.stringify((await runEntityTriples(request, "agent.run.phase")).map((t) => t.object)),
    ).toBeTruthy();

    // -----------------------------------------------------------------
    // Step 7 — the run left awaiting_approval to `executing` (rule 13's transition).
    // It does NOT complete — a plain coordinator respond_direct stamps no
    // agent.run.outcome, so executing is the correct stable post-resume phase.
    // -----------------------------------------------------------------
    const afterPhases = (await runEntityTriples(request, "agent.run.phase")).map((t) => t.object);
    expect(
      afterPhases,
      "after resume the run must be `executing` (agent-run/13 awaiting_approval→executing). Got: " +
        JSON.stringify(afterPhases),
    ).toContain("executing");
    expect(
      afterPhases,
      "run must NOT still be awaiting_approval after pending cleared (resume incomplete). Got: " +
        JSON.stringify(afterPhases),
    ).not.toContain("awaiting_approval");

    // -----------------------------------------------------------------
    // Step 8 — rule 13 also cleared approval_resumed (removed last). Required so a
    // FUTURE tool-gate on this run can pause again (rule 12's guard).
    // -----------------------------------------------------------------
    const resumedMarker = await runEntityTriples(request, "agent.run.approval_resumed");
    expect(
      resumedMarker.length,
      "agent.run.approval_resumed must be CLEARED after resume (agent-run/13 removes it last). Still present → a second tool-gate could never pause.",
    ).toBe(0);

    // -----------------------------------------------------------------
    // Step 9 — NO bounce: settle, then confirm the run is STILL executing and has
    // NOT flapped back to awaiting_approval (exercises rule 12/13 bounce-proof guard).
    // -----------------------------------------------------------------
    await new Promise((r) => setTimeout(r, 5_000));
    const settledPhases = (await runEntityTriples(request, "agent.run.phase")).map((t) => t.object);
    expect(
      settledPhases,
      "run bounced back to awaiting_approval after resume (rule 12/13 guard regression). Settled phases: " +
        JSON.stringify(settledPhases),
    ).not.toContain("awaiting_approval");
    expect(
      settledPhases,
      "run must remain `executing` after the settle window (no re-pause). Got: " +
        JSON.stringify(settledPhases),
    ).toContain("executing");
    const pendingSettled = await runEntityTriples(request, "agent.run.approval_pending");
    expect(
      pendingSettled.length,
      "agent.run.approval_pending re-appeared after resume — pause↔resume bounce.",
    ).toBe(0);
  });
});

/** Extract the bare id after a fixed entity-ID infix (e.g. ".agent.agentic-loop.execution."). */
function bareIdAfter(entityId: string, infix: string): string {
  const idx = entityId.indexOf(infix);
  return idx === -1 ? "" : entityId.slice(idx + infix.length);
}

/** Triples for `predicate` that live on a run entity (the chain.execution entity). */
async function runEntityTriples(
  request: import("@playwright/test").APIRequestContext,
  predicate: string,
): Promise<Array<{ subject?: string; object?: unknown }>> {
  const triples = await fetchTriples(request, { predicate, limit: 20 });
  return triples.filter((t) => String(t.subject ?? "").includes(RUN_INFIX));
}

async function fetchTriples(
  request: import("@playwright/test").APIRequestContext,
  params: { subject?: string; predicate?: string; limit?: number },
): Promise<Array<{ subject?: string; object?: unknown }>> {
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
  return (await resp.json()) as Array<{ subject?: string; object?: unknown }>;
}

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
