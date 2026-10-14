import { test, expect } from "@playwright/test";

/**
 * Journey: ADR-053 Phase 4c PR-2 — the tool-gate RESUME. The resume twin of
 * approval-pause.
 *
 *   (pause half — identical to approval-pause)
 *   recovery coordinator → tool_call(create_rule)  loop parks in awaiting_approval;
 *                                                   approvalpause stamps
 *                                                   agent.run.approval-pending +
 *                                                   agent.run.approval-outstanding=1;
 *                                                   agent-run/12 → run awaiting_approval
 *
 *   (resume half — this journey)
 *   POST /teams-dispatch/loops/{id}/approval {decision:"approve", execution_id}
 *      → dispatch publishes agent.approval_response.<id>
 *      → teams-loop (agent.approval_response input port) un-parks the loop
 *      → approvalpause appends [loopId, executionId] to agent.run.approval-answered
 *        and updates agent.run.approval-outstanding=0
 *      → agent-run/13 → run awaiting_approval→executing while preserving append-only
 *        approval receipts
 *
 * DECISIVE assertions:
 *   - agent.run.approval-outstanding reaches 0 on the run entity,
 *   - agent.run.approval-answered records the matching [loopId, executionId] receipt,
 *   - run is `executing`, NOT awaiting_approval (and not completed/failed),
 *   - after a settle window the run has NOT bounced back to awaiting_approval.
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

const RUN_INFIX = "chain.agent.execution.";

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
    // Step 2 — send a research prompt (drives the pause half).
    // -----------------------------------------------------------------
    await page.getByTestId("chat-input").fill(
      "Research MAVLink v2 HEARTBEAT frame staleness thresholds over UDP and summarize alerting defaults.",
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

    const pendingBefore = await runEntityTriples(request, "agent.run.approval-pending");
    expect(
      pendingBefore.length,
      "expected agent.run.approval-pending on the run entity before the approval (the 4c pause marker).",
    ).toBeGreaterThanOrEqual(1);

    // -----------------------------------------------------------------
    // Step 4 — the gated loop is the recovery coordinator. Its bare loop_id and
    // execution_id are encoded as a JSON [loopId, executionId] tuple in the
    // agent.run.approval-pending triple's OBJECT (recorded by approvalpause).
    // -----------------------------------------------------------------
    const rawPendingTuple = String(pendingBefore[0].object ?? "null");
    const parsedTuple = JSON.parse(rawPendingTuple) as unknown;
    expect(
      Array.isArray(parsedTuple) &&
        parsedTuple.length === 2 &&
        typeof parsedTuple[0] === "string" &&
        typeof parsedTuple[1] === "string",
      `could not parse [loopId, executionId] tuple from approval-pending object ${rawPendingTuple}`,
    ).toBe(true);
    const [gatedLoopId, pendingExecutionId] = parsedTuple as [string, string];

    // -----------------------------------------------------------------
    // Step 5 — POST the operator's APPROVAL through the real dispatch endpoint.
    // This publishes agent.approval_response.<loopId>; the loop un-parks (re-dispatches
    // create_rule with ApprovedBy → ApprovalFilter bypass) and the approvalpause
    // subscriber records the approval-answered receipt and updates approval-outstanding=0.
    // The dispatch consumes agent.approval_pending ASYNC, so retry on 409 ("loop not
    // awaiting approval") until it has registered the pending execution_id.
    // -----------------------------------------------------------------
    const approvalStatus = await pollUntil(async () => {
      const resp = await request.post(
        `/teams-dispatch/loops/${gatedLoopId}/approval`,
        {
          data: {
            decision: "approve",
            execution_id: pendingExecutionId,
            user_id: "e2e-operator",
          },
        },
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
    // Step 6 — DECISIVE: the run RESUMES. approvalpause appends the answered
    // tuple to agent.run.approval-answered and updates agent.run.approval-outstanding
    // to 0, driving agent-run/13's awaiting_approval→executing transition.
    // -----------------------------------------------------------------
    const resumed = await pollUntil(async () => {
      const [outstanding, phases] = await Promise.all([
        runEntityTriples(request, "agent.run.approval-outstanding"),
        runEntityTriples(request, "agent.run.phase"),
      ]);
      const outstandingZero = outstanding.some((t) => Number(t.object) === 0);
      const isExecuting = phases.some((t) => String(t.object) === "executing");
      return outstandingZero && isExecuting ? true : null;
    }, { timeoutMs: 60_000 });
    expect(
      resumed,
      "run never resumed to executing with approval-outstanding=0 after approval (approvalpause → agent-run/13). Current phases: " +
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
      "run must NOT still be awaiting_approval after approval-outstanding reached 0 (resume incomplete). Got: " +
        JSON.stringify(afterPhases),
    ).not.toContain("awaiting_approval");

    // -----------------------------------------------------------------
    // Step 8 — append-only approval receipt: agent.run.approval-answered records
    // the matching [loopId, executionId] tuple and agent.run.approval-pending
    // remains preserved as audit history.
    // -----------------------------------------------------------------
    const answeredMarkers = await runEntityTriples(request, "agent.run.approval-answered");
    expect(
      answeredMarkers.map((t) => String(t.object)),
      "agent.run.approval-answered must record the answered [loopId, executionId] tuple",
    ).toContain(rawPendingTuple);

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
    const pendingSettled = await runEntityTriples(request, "agent.run.approval-pending");
    expect(
      pendingSettled.length,
      "agent.run.approval-pending count changed after resume — unexpected re-gate.",
    ).toBe(1);
    const outstandingSettled = await runEntityTriples(request, "agent.run.approval-outstanding");
    expect(
      outstandingSettled.map((t) => Number(t.object)),
      "agent.run.approval-outstanding must remain 0 after the settle window.",
    ).toContain(0);
  });
});

/** Triples for `predicate` that live on a run entity (the chain.agent.execution entity). */
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
