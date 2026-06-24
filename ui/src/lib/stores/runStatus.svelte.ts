// Run-status polling store
// Polls /graph/triples for run-level pause markers (ADR-053 Phase 4b-2 / 4c).
//
// A "run" parks in `awaiting_approval` when:
//   - a gated tool call fires (4c tool-gate):      agent.run.approval_pending
//   - a recovery coordinator calls ask_user (4b-2): agent.run.clarification_pending
//
// The front-door coordinator's loop (the kanban card) is typically already
// `complete` when this happens — so agentStore's SSE stream does not surface
// the pause. This store polls the triple endpoint and exposes the pause data
// so the board can surface a "Waiting on you" affordance.
//
// Design: pause source of truth = marker PRESENCE, not agent.run.phase.
// Rules 11/13 REMOVE the marker as part of the resume transition, so a
// present marker means "human needed now". We do not fetch agent.run.phase
// at all.
//
// MARKER OBJECT FORMS ARE ASYMMETRIC (verified against semstreams beta.106):
//   - agent.run.approval_pending.object  = the FULL 6-part loop entity ref
//     (cmd/semteams/approvalpause/pauser.go stamps TryLoopExecutionEntityID).
//   - agent.run.clarification_pending.object = the BARE loop UUID
//     (configs/rules/agent-run/07,08 stamp `$entity.instance`, which
//     execution_context.go documents as "the bare loop UUID").
// Both are audit-only to the backend (rules key on predicate PRESENCE, not the
// object), so the backend never noticed the inconsistency — but a consumer that
// reads the object (this store, to recover the loop id for the reply/approval
// anchor) must normalize. `toBareLoopId` accepts either form. This asymmetry is
// a backend-cleanup candidate (normalize 07/08 to `$entity.id`); tracked for the
// consumer-tester feedback pass. The clarification-resume e2e proves the correct
// anchor is the BARE loop id (in_reply_to), extracted from coordinator.user_question.
//
// Poll interval: 2500ms by default. An AbortController + in-flight guard prevent
// concurrent and orphaned fetches (mirrors systemStatus). Errors are captured in
// lastError but never thrown, to keep the reactive graph stable.

import { SvelteMap } from "svelte/reactivity";
import { getTriples } from "$lib/services/runStatusApi";
import type { RawTriple } from "$lib/services/runStatusApi";
import type { RunPause } from "$lib/types/task";
import {
  deriveGraphRunHealthFacts,
  RUN_HEALTH_PREDICATES,
  type GraphRunHealthFacts,
} from "$lib/utils/runHealth";

export interface RunStatus {
  runId: string;
  pause: RunPause | null;
  healthFacts: GraphRunHealthFacts | null;
}

const RUN_INFIX = ".agent.chain.execution.";
const LOOP_INFIX = ".agent.agentic-loop.execution.";
const POLL_INTERVAL_MS = 2500;

/** Extract the bare id after a fixed entity-id infix. Returns "" if not found. */
function bareIdAfter(entityId: string, infix: string): string {
  const idx = entityId.indexOf(infix);
  return idx === -1 ? "" : entityId.slice(idx + infix.length);
}

/**
 * Normalize a loop reference to its BARE id, accepting either a full 6-part
 * loop entity ref (`…agent.agentic-loop.execution.<id>`) or an already-bare id.
 * The two run-pause markers stamp different forms (see file header); the reply
 * and approval anchors both want the bare id, so callers normalize here.
 */
function toBareLoopId(loopRef: string): string {
  return loopRef.includes(LOOP_INFIX) ? bareIdAfter(loopRef, LOOP_INFIX) : loopRef;
}

function runEntityIdsFromTriples(...batches: RawTriple[][]): string[] {
  const seen: Record<string, true> = {};
  for (const batch of batches) {
    for (const triple of batch) {
      if (!triple.subject.includes(RUN_INFIX)) continue;
      if (!bareIdAfter(triple.subject, RUN_INFIX)) continue;
      seen[triple.subject] = true;
    }
  }
  return Object.keys(seen);
}

function dedupeTriples(triples: RawTriple[]): RawTriple[] {
  const seen: Record<string, true> = {};
  const out: RawTriple[] = [];
  for (const triple of triples) {
    const key = [
      triple.subject,
      triple.predicate,
      triple.object,
      triple.timestamp ?? "",
    ].join("\u0000");
    if (seen[key]) continue;
    seen[key] = true;
    out.push(triple);
  }
  return out;
}

/**
 * Pure helper: derive the run-status map from three triple arrays.
 * Exported so tests can drive the parse logic without timers or fetch mocks.
 *
 * @param approvalTriples   - triples with predicate `agent.run.approval_pending`
 *                            (object = full loop entity ref)
 * @param clarTriples       - triples with predicate `agent.run.clarification_pending`
 *                            (object = bare loop UUID)
 * @param questionTriples   - triples with predicate `coordinator.user_question`
 *                            (subject = full asking-loop entity ref, object = prose)
 * @returns SvelteMap<bare runId, RunStatus>. Callers ITERATE this map (copy its
 *          contents into reactive state); they must NOT bind to it directly.
 */
export function deriveRunStatuses(
  approvalTriples: RawTriple[],
  clarTriples: RawTriple[],
  questionTriples: RawTriple[],
  healthTriples: RawTriple[] = [],
): SvelteMap<string, RunStatus> {
  const healthFacts = deriveGraphRunHealthFacts([
    ...approvalTriples,
    ...clarTriples,
    ...healthTriples,
  ]);

  // Plain objects used here (not Map/Set) to satisfy the
  // svelte/prefer-svelte-reactivity rule that applies file-wide to
  // .svelte.ts files. These are local intermediaries in a pure function,
  // not reactive state — but the rule fires on all `new Map()` in the file.
  const approvalByRunEntity: Record<string, string> = {};
  for (const t of approvalTriples) {
    if (t.subject.includes(RUN_INFIX)) {
      approvalByRunEntity[t.subject] = t.object;
    }
  }

  const clarByRunEntity: Record<string, string> = {};
  for (const t of clarTriples) {
    if (t.subject.includes(RUN_INFIX)) {
      clarByRunEntity[t.subject] = t.object;
    }
  }

  // Map from BARE asking-loop id → question prose. The coordinator.user_question
  // subject is the FULL asking-loop entity ref, so we key by its bare id to join
  // against the clarification marker's bare object. (Matches the extraction the
  // clarification-resume e2e spec performs against the real backend.)
  const questionByBareLoop: Record<string, string> = {};
  for (const t of questionTriples) {
    const bareLoop = toBareLoopId(t.subject);
    if (bareLoop) questionByBareLoop[bareLoop] = t.object;
  }

  // Use SvelteMap (extends Map) so the lint rule is satisfied even in the
  // pure-function path. The store's pollOnce() consumes this as a plain
  // Map-compatible iterable, so the reactive wrapping is harmless here.
  const result = new SvelteMap<string, RunStatus>();

  // Union of all run entities seen in either marker
  const allRunEntityIds = [
    ...Object.keys(approvalByRunEntity),
    ...Object.keys(clarByRunEntity).filter(
      (k) => !(k in approvalByRunEntity),
    ),
    ...[...healthFacts.values()]
      .map((facts) => facts.runEntityId)
      .filter((id) => !(id in approvalByRunEntity) && !(id in clarByRunEntity)),
  ];

  for (const runEntityId of allRunEntityIds) {
    const bareRunId = bareIdAfter(runEntityId, RUN_INFIX);
    if (!bareRunId) continue;

    if (runEntityId in approvalByRunEntity) {
      // tool_gate: object is the FULL loop entity ref → normalize to bare.
      const gatedLoopId = toBareLoopId(approvalByRunEntity[runEntityId]);
      result.set(bareRunId, {
        runId: bareRunId,
        pause: { cause: "tool_gate", gatedLoopId },
        healthFacts: healthFacts.get(bareRunId) ?? null,
      });
    } else if (runEntityId in clarByRunEntity) {
      // clarification: object is the BARE loop UUID (already the reply anchor).
      // Normalize defensively in case the backend asymmetry is later fixed to
      // stamp the full ref. The question joins on this bare id.
      const askingLoopId = toBareLoopId(clarByRunEntity[runEntityId]);
      const question = questionByBareLoop[askingLoopId] ?? "";
      result.set(bareRunId, {
        runId: bareRunId,
        pause: { cause: "clarification", askingLoopId, question },
        healthFacts: healthFacts.get(bareRunId) ?? null,
      });
    } else {
      result.set(bareRunId, {
        runId: bareRunId,
        pause: null,
        healthFacts: healthFacts.get(bareRunId) ?? null,
      });
    }
  }

  return result;
}

function createRunStatusStore() {
  const statuses = new SvelteMap<string, RunStatus>();
  let lastError = $state<string | null>(null);
  let intervalId: ReturnType<typeof setInterval> | null = null;
  let fetching = false;
  // The in-flight poll's abort controller, so stop() can cancel a pending
  // fetch and the finally-block runs promptly (resetting `fetching`). Without
  // this, a stop() during a hung fetch would leave `fetching` stuck true and a
  // later start() would never poll again. Mirrors systemStatus.
  let currentAbort: AbortController | null = null;

  async function pollOnce(): Promise<void> {
    if (fetching) return;
    fetching = true;
    const ctrl = new AbortController();
    currentAbort = ctrl;
    try {
      const [approvalTriples, clarTriples, questionTriples, ...healthBatches] = await Promise.all([
        getTriples({ predicate: "agent.run.approval_pending", limit: 100, signal: ctrl.signal }),
        getTriples({ predicate: "agent.run.clarification_pending", limit: 100, signal: ctrl.signal }),
        getTriples({ predicate: "coordinator.user_question", limit: 100, signal: ctrl.signal }),
        ...RUN_HEALTH_PREDICATES.map((predicate) =>
          getTriples({ predicate, limit: 100, signal: ctrl.signal }),
        ),
      ]);
      const healthTriples = healthBatches.flat();
      const runEntityIds = runEntityIdsFromTriples(
        approvalTriples,
        clarTriples,
        healthTriples,
      );
      const subjectBatches = runEntityIds.length > 0
        ? await Promise.all(
            runEntityIds.map((subject) =>
              getTriples({ subject, limit: 500, signal: ctrl.signal }),
            ),
          )
        : [];

      const derived = deriveRunStatuses(
        approvalTriples,
        clarTriples,
        questionTriples,
        dedupeTriples([...healthTriples, ...subjectBatches.flat()]),
      );

      // Replace map contents to exactly the set of currently-paused runs.
      // Runs that resolved drop out; newly-paused runs appear.
      const toDelete: string[] = [];
      for (const key of statuses.keys()) {
        if (!derived.has(key)) toDelete.push(key);
      }
      for (const key of toDelete) statuses.delete(key);
      for (const [key, val] of derived) {
        statuses.set(key, val);
      }
      lastError = null;
    } catch (err) {
      // An intentional stop()-abort is not a failure — leave last-good state
      // and don't surface it. Any other error is captured (not thrown) so a
      // poll failure neither poisons the reactive graph nor logs noise.
      if (!(err instanceof DOMException && err.name === "AbortError")) {
        lastError = err instanceof Error ? err.message : String(err);
      }
    } finally {
      if (currentAbort === ctrl) currentAbort = null;
      fetching = false;
    }
  }

  return {
    /** Start polling. Idempotent — no-op if already running. */
    start() {
      if (intervalId !== null) return;
      void pollOnce(); // immediate first tick
      intervalId = setInterval(() => void pollOnce(), POLL_INTERVAL_MS);
    },

    /** Stop polling, cancel any in-flight fetch, and clear the interval. */
    stop() {
      if (intervalId !== null) {
        clearInterval(intervalId);
        intervalId = null;
      }
      currentAbort?.abort();
      currentAbort = null;
      fetching = false; // safety reset so a later start() always polls
    },

    /** Get the RunStatus for a bare runId, or undefined if not paused. */
    get(runId: string): RunStatus | undefined {
      return statuses.get(runId);
    },

    /** All currently-paused run statuses. */
    getList(): RunStatus[] {
      return [...statuses.values()];
    },

    /** Count of runs currently paused and waiting on the operator. */
    get pausedCount(): number {
      return statuses.size;
    },

    /**
     * Last poll error message, or null if the most recent poll succeeded.
     * Captured (not thrown) so the reactive graph stays stable. NOT yet
     * surfaced in the UI — a poll-failure indicator in the nav is a deferred
     * follow-up slice; exposed here so that slice has the signal ready.
     */
    get lastError(): string | null {
      return lastError;
    },

    /**
     * Run a single poll tick directly. Exposed for unit tests so they can
     * drive parse logic via injected fetch mocks without real timers.
     */
    pollOnce,
  };
}

export const runStatus = createRunStatusStore();
