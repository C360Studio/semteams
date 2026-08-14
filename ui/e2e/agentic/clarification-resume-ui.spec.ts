import { test, expect } from "@playwright/test";

/**
 * Journey: ADR-053 Phase 4b-2 — the run-level "waiting on you" UI affordance
 * (the deferred "B2 UI slice"). This is the UI-driven twin of
 * clarification-resume.spec.ts: that spec POSTs the reply via the raw dispatch
 * API; THIS spec drives the resume entirely through the operator-facing UI
 * (the RunWaitingSection reply box), proving the board surfaces a paused run
 * and the reply box resumes it.
 *
 * WHY this exists beyond the API spec — it exercises three things the
 * API-driven spec and the green unit tests CANNOT:
 *
 *   1. SURFACE: the front-door coordinator loop is `complete` when a descendant
 *      pauses, so the SSE stream never shows the pause. The runStatus poll of
 *      /graph/triples must surface it as a "Answer needed" badge on the card
 *      (deriveTaskInfo forces the needs_you column) and a RunWaitingSection in
 *      the detail panel. runID === the front-door coordinator's loop_id
 *      (verified: rule-engine mints the run with agentrun.Mint(…, firingLoopID)
 *      and task.RunID = firingLoopID), so the pause lands on the right card.
 *
 *   2. WIRE-SHAPE (the load-bearing assertion): the question prose rendered in
 *      the panel proves deriveRunStatuses joined coordinator.clarification.question to
 *      the run via the BARE asking-loop id. The first UI draft ran bareIdAfter()
 *      on agent.run.clarification-pending.object — but that object is the BARE
 *      loop UUID ($entity.instance), not a full entity ref, so the join MISSED
 *      and the in_reply_to anchor was "". The unit tests masked this by assuming
 *      full entity ids for both markers; only a real-backend journey catches it.
 *      If the question text is the fallback copy, or the reply never resumes the
 *      run, the bare/full normalization regressed.
 *
 *   3. RESUME via UI: clicking Send posts the operator's answer through
 *      agentApi.sendMessage(content, { runId, inReplyTo }) where inReplyTo is
 *      the bare asking-loop id taken from the runStatus pause. beta.106 stamps
 *      agent.run.entity-id + agent.loop.reply-to at spawn → agent-run/10→11
 *      resume the run (awaiting_approval→executing + clear both markers). The
 *      affordance then unmounts on the next poll.
 *
 * The run resumes to `executing` and stays there (a plain coordinator
 * respond_direct stamps no agent.run.outcome — the documented incomplete-run
 * case). The journey proves the RESUME MECHANIC + the UI surface, not work
 * completion.
 *
 * Mock serves responses in GLOBAL ORDER (clarification-resume.yaml), so the
 * typed reply content is free — the 4th response (respond_direct) is served to
 * the resumed coordinator regardless. Reused fixture: clarification-resume.yaml.
 * Config: configs/e2e-flow-bootstrap.json (interactive; rules 07/09/10/11).
 */

const RUN_PREFIX = ".agent.chain.execution.";

test.describe("ADR-053 Phase 4b-2 — operator resumes a paused run THROUGH the UI affordance", () => {
  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok(), "Backend not healthy — stack not running?").toBe(true);
  });

  test.setTimeout(180_000);

  test("paused run surfaces a RunWaitingSection; the reply box resumes it", async ({
    page,
    request,
  }) => {
    // -----------------------------------------------------------------
    // Step 1 — open the Board so the SSE stream + runStatus poll start.
    // -----------------------------------------------------------------
    await page.goto("/");
    await expect(page.getByTestId("connection-status")).toHaveAttribute(
      "data-summary",
      "healthy",
      { timeout: 15_000 },
    );
    await expect(page.getByTestId("kanban-board")).toBeVisible();

    // -----------------------------------------------------------------
    // Step 2 — send the dev-build prompt → first-pass Lisa escalates →
    // recovery coordinator asks the user → the run parks in awaiting_approval.
    // -----------------------------------------------------------------
    await page.getByTestId("chat-input").fill(
      "Build a Go HTTP service that decodes MAVLink v2 HEARTBEAT frames over UDP and serves the latest at GET /heartbeat as JSON.",
    );
    await page.getByTestId("send-button").click();

    // -----------------------------------------------------------------
    // Step 3 — wait (via the backend) for the run to PARK. The UI poll lags
    // this by up to one interval; gating the UI assertions on the backend
    // phase first keeps them from racing the very first poll tick.
    // -----------------------------------------------------------------
    const pausedPhase = await pollUntil(async () => {
      const phases = await runPhaseTriples(request);
      return phases.some((t) => t.object === "awaiting_approval") ? phases : null;
    }, { timeoutMs: 90_000 });
    expect(
      pausedPhase,
      "run never reached awaiting_approval — the recovery coordinator's ask_user did not pause the run. Cannot test the UI affordance.",
    ).toBeTruthy();

    const runEntityId = String(pausedPhase![0].subject ?? "");
    const runId = bareIdAfter(runEntityId, RUN_PREFIX);
    expect(runId, `could not extract bare runID from ${runEntityId}`).toBeTruthy();

    // -----------------------------------------------------------------
    // Step 4 — SURFACE: the board shows the "Answer needed" badge on the
    // paused run's card (runStatus poll → deriveTaskInfo forces needs_you).
    // -----------------------------------------------------------------
    await expect(
      page.getByTestId("run-waiting-badge").first(),
      "no run-waiting badge appeared — runStatus did not surface the run-level pause on the board (poll/parse/column-forcing regression)",
    ).toBeVisible({ timeout: 30_000 });
    await expect(page.getByTestId("run-waiting-badge").first()).toHaveText(
      "Answer needed",
    );

    // -----------------------------------------------------------------
    // Step 5 — open the paused run's detail panel deterministically via the
    // URL selection param (?task=<loop_id>); runID === the card's loop_id.
    // -----------------------------------------------------------------
    await page.goto(`/?task=${runId}`);
    await expect(page.getByTestId("connection-status")).toHaveAttribute(
      "data-summary",
      "healthy",
      { timeout: 15_000 },
    );

    const section = page.getByTestId("run-waiting-section");
    await expect(
      section,
      "RunWaitingSection did not render in the detail panel for the paused run",
    ).toBeVisible({ timeout: 30_000 });

    // WIRE-SHAPE PROOF: the real question prose rendered → deriveRunStatuses
    // joined coordinator.clarification.question to the run via the bare asking-loop id.
    // The fixture question contains "MAVLink dialect"; the fallback copy does
    // not. A miss here means the bare/full marker normalization regressed.
    await expect(
      page.getByTestId("run-question"),
      "the asking question did not render — the user_question→run join missed (bare/full asking-loop id regression)",
    ).toContainText("MAVLink dialect");

    // -----------------------------------------------------------------
    // Step 6 — RESUME via the UI: type the answer and Send. agentApi.sendMessage
    // posts run_id + in_reply_to (the bare asking-loop id from the runStatus
    // pause). The content is free — the mock serves responses in global order.
    // -----------------------------------------------------------------
    await page.getByTestId("run-reply-input").fill(
      "Listen on 0.0.0.0:14550 and decode the common MAVLink dialect. Proceed.",
    );
    await page.getByTestId("run-reply-send").click();

    // The optimistic confirmation appears immediately (before the poll unmounts
    // the section), so the operator knows the answer registered.
    await expect(page.getByTestId("run-reply-confirm")).toBeVisible({
      timeout: 10_000,
    });

    // -----------------------------------------------------------------
    // Step 7 — DECISIVE (backend): the run RESUMES. agent-run/11 clears
    // clarification_pending as part of awaiting_approval→executing. A
    // never-resumed run keeps pending set forever (in_reply_to was ""); a
    // working reply clears it.
    // -----------------------------------------------------------------
    const resumed = await pollUntil(async () => {
      const pending = await runEntityTriples(request, "agent.run.clarification-pending");
      return pending.length === 0 ? true : null;
    }, { timeoutMs: 60_000 });
    expect(
      resumed,
      "agent.run.clarification-pending was never cleared after the UI reply — the resume did not fire. The reply likely carried an empty in_reply_to (bare/full asking-loop id regression). Current phases: " +
        JSON.stringify((await runPhaseTriples(request)).map((t) => t.object)),
    ).toBeTruthy();

    const afterPhases = (await runPhaseTriples(request)).map((t) => t.object);
    expect(
      afterPhases,
      "after resume the run must be `executing` (agent-run/11). Got: " +
        JSON.stringify(afterPhases),
    ).toContain("executing");
    expect(afterPhases).not.toContain("awaiting_approval");

    // -----------------------------------------------------------------
    // Step 8 — SURFACE clears: the next poll drops the marker, so the
    // RunWaitingSection unmounts and the badge disappears. The affordance is
    // self-clearing — the operator is not left with a stale "waiting" surface.
    // -----------------------------------------------------------------
    await expect(
      page.getByTestId("run-waiting-section"),
      "RunWaitingSection did not unmount after resume — runStatus kept a stale pause (the poll should drop the cleared marker)",
    ).toHaveCount(0, { timeout: 30_000 });
    await expect(page.getByTestId("run-waiting-badge")).toHaveCount(0);
  });
});

/** All agent.run.phase triples that live on a run (chain.execution) entity. */
async function runPhaseTriples(
  request: import("@playwright/test").APIRequestContext,
): Promise<Array<{ subject?: string; object?: unknown }>> {
  const phases = await fetchTriples(request, { predicate: "agent.run.phase", limit: 20 });
  return phases.filter((t) => String(t.subject ?? "").includes(RUN_PREFIX));
}

/** Triples for `predicate` that live on a run (chain.execution) entity. */
async function runEntityTriples(
  request: import("@playwright/test").APIRequestContext,
  predicate: string,
): Promise<Array<{ subject?: string; object?: unknown }>> {
  const triples = await fetchTriples(request, { predicate, limit: 20 });
  return triples.filter((t) => String(t.subject ?? "").includes(RUN_PREFIX));
}

/** Extract the bare id after a fixed entity-ID infix. */
function bareIdAfter(entityId: string, infix: string): string {
  const idx = entityId.indexOf(infix);
  return idx === -1 ? "" : entityId.slice(idx + infix.length);
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
    throw new Error(`GET /graph/triples?${query.toString()} returned ${resp.status()}`);
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
