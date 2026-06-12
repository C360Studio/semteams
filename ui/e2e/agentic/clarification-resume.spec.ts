import { test, expect } from "@playwright/test";

/**
 * Journey: ADR-053 Phase 4b-2 PR-2 — an in-run clarification PAUSE is RESUMED by
 * the operator's reply, driving the run awaiting_approval→executing.
 *
 * This is the resume twin of ask-user-pause.spec.ts. The pause half is identical:
 *
 *   coordinator(dispatch) → decide(dev_via_test)        run becomes `executing`
 *   first-pass Lisa       → decide(needs_clarification)  rule 02f spawns the
 *                                                        inherit recovery
 *                                                        coordinator
 *   recovery coordinator  → decide(ask_user)             coordinator/03-ask-user
 *                                                        stamps user_question;
 *                                                        agent-run/07 stamps
 *                                                        clarification_pending;
 *                                                        agent-run/09 →
 *                                                        awaiting_approval
 *
 * Resume half (NEW — ACTIVE on semstreams beta.106 / #256):
 *
 *   POST /teams-dispatch/message { content, run_id, in_reply_to }
 *        run_id      = the paused run's bare runID    (re-attaches → agent.run.entity_id)
 *        in_reply_to = the asking loop's bare loop-id (marks reply → agent.loop.reply_to)
 *   → buildSpawnIdentityTriples stamps BOTH on the resumed coordinator loop at
 *     spawn (before any LLM call) →
 *        agent-run/10 → agent.run.clarification_resumed
 *        agent-run/11 → awaiting_approval→executing + clear BOTH markers
 *   → resumed coordinator decide(respond_direct) answers directly (the loop
 *     completes; the run stays `executing`).
 *
 * The resumed coordinator answering DIRECTLY does NOT complete the run — no rule
 * stamps agent.run.outcome=success on a plain coordinator respond_direct (only the
 * per-pack reviewer-approved paths do). So the run resumes to `executing` and stays
 * there (the original dev_via_test work was abandoned when the recovery coordinator
 * asked instead of planning — the documented incomplete-run case owned by the future
 * cancel/timeout slice). That is fine: this journey proves the RUN-PHASE RESUME
 * MECHANIC, not work completion.
 *
 * DECISIVE proof of resume is durable + race-free: the run that REACHED
 * awaiting_approval (asserted before the reply) LEAVES it to `executing` with BOTH
 * clarification markers CLEARED — only agent-run/11 does that. A never-resumed run
 * (rules 10/11 not firing) stays stuck in awaiting_approval with pending set; a
 * pause↔resume bounce (rule 09/11 guard broken) flaps back to awaiting_approval.
 * A settle window re-asserts the run stays `executing` (no bounce).
 * last_transition_from is deliberately NOT the gate — it is fragile across the
 * spawn/transition race; the markers-cleared + phase==executing end state is airtight.
 *
 * Reply path is driven via the dispatch /message endpoint directly (the same
 * direct-API pattern tool-approval-gate uses) — no UI reply affordance exists yet
 * (that is the deferred B2 UI slice), and no triple seeding (the resumed loop
 * carries the real wire anchors threaded by beta.106).
 *
 * Required fixture: test/fixtures/journeys/clarification-resume.yaml
 * Required config: configs/e2e-flow-bootstrap.json (interactive; rules 07/09/10/11)
 */

const RUN_PREFIX = ".agent.chain.execution.";
const LOOP_PREFIX = ".agent.agentic-loop.execution.";

test.describe("ADR-053 Phase 4b-2 PR-2 — operator reply resumes a paused run (awaiting_approval→executing)", () => {
  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok(), "Backend not healthy — stack not running?").toBe(true);
  });

  test.setTimeout(150_000);

  test("clarification reply with run_id + in_reply_to resumes the run", async ({
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
    // Step 2 — send the dev-build prompt → first-pass Lisa escalates →
    // recovery coordinator asks the user → run pauses awaiting_approval.
    // -----------------------------------------------------------------
    await page.getByTestId("chat-input").fill(
      "Build a Go HTTP service that decodes MAVLink v2 HEARTBEAT frames over UDP and serves the latest at GET /heartbeat as JSON.",
    );
    await page.getByTestId("send-button").click();

    // -----------------------------------------------------------------
    // Step 3 — wait for the run to PARK in awaiting_approval (the pause).
    // -----------------------------------------------------------------
    const pausedPhase = await pollUntil(async () => {
      const phases = await runPhaseTriples(request);
      return phases.some((t) => t.object === "awaiting_approval") ? phases : null;
    }, { timeoutMs: 90_000 });
    expect(
      pausedPhase,
      "run never reached awaiting_approval — the recovery coordinator's ask_user did not pause the run (PR-1). Cannot test resume.",
    ).toBeTruthy();

    // -----------------------------------------------------------------
    // Step 4 — derive the reply anchors from the paused run's triples.
    //   run_id      = bare runID from the run entity ID.
    //   in_reply_to = bare loop-id of the loop carrying coordinator.user_question
    //                 (the recovery coordinator that asked).
    // -----------------------------------------------------------------
    // [0] is safe ONLY because this journey runs exactly one run — every
    // agent.run.phase triple on a run entity carries that run's subject, so
    // bareIdAfter yields the same runID from any of them. A multi-run regression
    // would silently pick an arbitrary run here.
    const runEntityId = String(pausedPhase![0].subject ?? "");
    const runId = bareIdAfter(runEntityId, RUN_PREFIX);
    expect(runId, `could not extract bare runID from ${runEntityId}`).toBeTruthy();

    const questions = await fetchTriples(request, {
      predicate: "coordinator.user_question",
      limit: 20,
    });
    expect(
      questions.length,
      "expected a coordinator.user_question triple (the asking loop). Zero → ask_user never fired.",
    ).toBeGreaterThanOrEqual(1);
    const askingLoopEntity = String(questions[0].subject ?? "");
    const askingLoopId = bareIdAfter(askingLoopEntity, LOOP_PREFIX);
    expect(
      askingLoopId,
      `could not extract bare loop-id from ${askingLoopEntity}`,
    ).toBeTruthy();

    // Pause is genuine: clarification_pending is set on the run entity.
    const pendingBefore = await runEntityTriples(request, "agent.run.clarification_pending");
    expect(
      pendingBefore.length,
      "expected agent.run.clarification_pending on the run entity before the reply (the pause marker).",
    ).toBeGreaterThanOrEqual(1);

    // -----------------------------------------------------------------
    // Step 5 — POST the operator's answer through the REAL dispatch reply
    // path with the resumable-reply anchors (semstreams#256 / beta.106).
    // No reply_to → a fresh coordinator loop re-attached to the run via run_id
    // and marked as a reply via in_reply_to (NOT a re-entry of the completed
    // asking loop). No triple seeding — the wire stamps agent.run.entity_id +
    // agent.loop.reply_to at spawn.
    // -----------------------------------------------------------------
    const reply = await request.post("/teams-dispatch/message", {
      data: {
        content:
          "Listen on 0.0.0.0:14550 and decode the common MAVLink dialect. Proceed.",
        run_id: runId,
        in_reply_to: askingLoopId,
      },
    });
    expect(
      reply.ok(),
      `dispatch /message reply returned ${reply.status()} (run_id=${runId}, in_reply_to=${askingLoopId})`,
    ).toBe(true);

    // -----------------------------------------------------------------
    // Step 6 — DECISIVE: the run RESUMES. agent-run/11 clears clarification_pending
    // as part of the awaiting_approval→executing transition, so polling until
    // pending is GONE on the run entity is the durable proof rule 11 ran. A
    // never-resumed run keeps pending set forever; a bounce re-sets it.
    // -----------------------------------------------------------------
    const resumed = await pollUntil(async () => {
      const pending = await runEntityTriples(request, "agent.run.clarification_pending");
      return pending.length === 0 ? true : null;
    }, { timeoutMs: 60_000 });
    expect(
      resumed,
      "agent.run.clarification_pending was never cleared after the reply — the resume (agent-run/10→11) did not fire. Run still parked. Current phases: " +
        JSON.stringify((await runPhaseTriples(request)).map((t) => t.object)),
    ).toBeTruthy();

    // -----------------------------------------------------------------
    // Step 7 — the run left awaiting_approval to `executing` (rule 11's transition).
    // It does NOT complete — a plain coordinator respond_direct stamps no
    // agent.run.outcome, so executing is the correct stable post-resume phase.
    // -----------------------------------------------------------------
    const afterPhases = (await runPhaseTriples(request)).map((t) => t.object);
    expect(
      afterPhases,
      "after resume the run must be `executing` (agent-run/11 awaiting_approval→executing). Got: " +
        JSON.stringify(afterPhases),
    ).toContain("executing");
    expect(
      afterPhases,
      "run must NOT still be awaiting_approval after pending cleared (resume incomplete). Got: " +
        JSON.stringify(afterPhases),
    ).not.toContain("awaiting_approval");

    // -----------------------------------------------------------------
    // Step 8 — the reply loop carried the real wire anchor: agent.loop.reply_to
    // was stamped (proves beta.106 threaded in_reply_to → buildSpawnIdentityTriples),
    // AND it points at the asking loop's entity (proves the reply referenced the
    // RIGHT loop, not just that some reply triple exists). buildSpawnIdentityTriples
    // reconstructs the object as LoopExecutionEntityID(in_reply_to), which equals
    // the asking loop's entity ID (the coordinator.user_question subject).
    // -----------------------------------------------------------------
    const replyTo = await fetchTriples(request, {
      predicate: "agent.loop.reply_to",
      limit: 20,
    });
    expect(
      replyTo.length,
      "expected an agent.loop.reply_to triple on the resumed loop (semstreams#256 / beta.106 typed reply identity). Zero → the reply path did not thread in_reply_to.",
    ).toBeGreaterThanOrEqual(1);
    expect(
      replyTo.map((t) => String(t.object)),
      "agent.loop.reply_to must point at the asking loop's entity (" +
        askingLoopEntity +
        ") — proves the reply referenced the loop that asked, not an arbitrary loop. Got: " +
        JSON.stringify(replyTo.map((t) => t.object)),
    ).toContain(askingLoopEntity);

    // -----------------------------------------------------------------
    // Step 9 — rule 11 also cleared clarification_resumed (removed last). Required
    // so a FUTURE clarification on this run can pause again (rule 09's guard).
    // -----------------------------------------------------------------
    const resumedMarker = await runEntityTriples(request, "agent.run.clarification_resumed");
    expect(
      resumedMarker.length,
      "agent.run.clarification_resumed must be CLEARED after resume (agent-run/11 removes it last). Still present → marker-clear incomplete; a second clarification could never pause.",
    ).toBe(0);

    // -----------------------------------------------------------------
    // Step 10 — NO bounce: settle, then confirm the run is STILL executing and
    // has NOT flapped back to awaiting_approval (exercises the rule 09/11
    // bounce-proof mutual-exclusion guard live).
    // -----------------------------------------------------------------
    await new Promise((r) => setTimeout(r, 5_000));
    const settledPhases = (await runPhaseTriples(request)).map((t) => t.object);
    expect(
      settledPhases,
      "run bounced back to awaiting_approval after resume (rule 09/11 guard regression). Settled phases: " +
        JSON.stringify(settledPhases),
    ).not.toContain("awaiting_approval");
    expect(
      settledPhases,
      "run must remain `executing` after the settle window (no re-pause). Got: " +
        JSON.stringify(settledPhases),
    ).toContain("executing");
    const pendingSettled = await runEntityTriples(request, "agent.run.clarification_pending");
    expect(
      pendingSettled.length,
      "agent.run.clarification_pending re-appeared after resume — pause↔resume bounce.",
    ).toBe(0);
  });
});

/** All agent.run.phase triples that live on a run entity. */
async function runPhaseTriples(
  request: import("@playwright/test").APIRequestContext,
): Promise<Array<{ subject?: string; object?: unknown }>> {
  const phases = await fetchTriples(request, { predicate: "agent.run.phase", limit: 20 });
  return phases.filter((t) => String(t.subject ?? "").includes(RUN_PREFIX));
}

/** Triples for `predicate` that live on a run entity (the chain.execution entity). */
async function runEntityTriples(
  request: import("@playwright/test").APIRequestContext,
  predicate: string,
): Promise<Array<{ subject?: string; object?: unknown }>> {
  const triples = await fetchTriples(request, { predicate, limit: 20 });
  return triples.filter((t) => String(t.subject ?? "").includes(RUN_PREFIX));
}

/** Extract the bare id after a fixed entity-ID infix (e.g. ".agent.chain.execution."). */
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
