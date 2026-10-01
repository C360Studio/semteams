// Poll graph run status. Approval history is append-only: exact JSON
// [bareLoopID, executionID] tuples in approval-pending minus approval-answered
// identify unresolved gates. Historical receipts alone never imply a pause.
// Clarification still uses its removable bare-loop marker. The loop record's
// pending_approval remains the authority for rendering and submitting a gate.

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

const RUN_INFIX = ".chain.agent.execution.";
const LOOP_INFIX = ".agentic-loop.agent.execution.";
const POLL_INTERVAL_MS = 2500;

/** Extract the bare id after a fixed entity-id infix. Returns "" if not found. */
function bareIdAfter(entityId: string, infix: string): string {
  const idx = entityId.indexOf(infix);
  return idx === -1 ? "" : entityId.slice(idx + infix.length);
}

/**
 * Normalize a loop reference to its BARE id, accepting either a full 6-part
 * loop entity ref (`…agentic-loop.agent.execution.<id>`) or an already-bare id.
 * Clarification markers and question subjects use these two forms. Approval
 * tuples are parsed separately and must never pass through this helper.
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

type ApprovalIdentity = [loopId: string, executionId: string];

function approvalIdentity(object: string): ApprovalIdentity | null {
  try {
    const value: unknown = JSON.parse(object);
    if (!Array.isArray(value) || value.length !== 2
      || value.some((part) => typeof part !== "string" || part.trim().length === 0)
      || value[0].includes(".")) return null;
    return value as ApprovalIdentity;
  } catch {
    return null;
  }
}

/** Derive current pauses; approval history is a set difference, never last-write-wins. */
export function deriveRunStatuses(
  approvalTriples: RawTriple[],
  clarTriples: RawTriple[],
  questionTriples: RawTriple[],
  healthTriples: RawTriple[] = [],
  answeredTriples: RawTriple[] = [],
): SvelteMap<string, RunStatus> {
  const healthFacts = deriveGraphRunHealthFacts([
    ...approvalTriples,
    ...clarTriples,
    ...answeredTriples,
    ...healthTriples,
  ]);

  // Plain objects used here (not Map/Set) to satisfy the
  // svelte/prefer-svelte-reactivity rule that applies file-wide to
  // .svelte.ts files. These are local intermediaries in a pure function,
  // not reactive state — but the rule fires on all `new Map()` in the file.
  const approvals = [...approvalTriples, ...healthTriples.filter((t) => t.predicate === "agent.run.approval-pending")];
  const answers = [...answeredTriples, ...healthTriples.filter((t) => t.predicate === "agent.run.approval-answered")];
  const answeredByRun: Record<string, Record<string, true>> = {};
  for (const t of answers) {
    const identity = approvalIdentity(t.object);
    if (!t.subject.includes(RUN_INFIX) || !identity) continue;
    (answeredByRun[t.subject] ??= {})[JSON.stringify(identity)] = true;
  }
  const pendingByRun: Record<string, Record<string, ApprovalIdentity>> = {};
  for (const t of approvals) {
    const identity = approvalIdentity(t.object);
    if (!t.subject.includes(RUN_INFIX) || !identity) continue;
    const key = JSON.stringify(identity);
    if (!answeredByRun[t.subject]?.[key]) (pendingByRun[t.subject] ??= {})[key] = identity;
  }
  const approvalByRunEntity: Record<string, ApprovalIdentity> = {};
  for (const [run, identities] of Object.entries(pendingByRun)) {
    // Stable selection if several loops in the same run need approval. Answering
    // one leaves the other tuple available on the following poll.
    const key = Object.keys(identities).sort()[0];
    if (key) approvalByRunEntity[run] = identities[key];
  }

  const clarByRunEntity: Record<string, string> = {};
  for (const t of clarTriples) {
    if (t.subject.includes(RUN_INFIX)) {
      clarByRunEntity[t.subject] = t.object;
    }
  }

  // Map from BARE asking-loop id → question prose. The coordinator.clarification.question
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

    const facts = healthFacts.get(bareRunId) ?? null;
    if (facts && ["completed", "failed", "cancelled"].includes(facts.phase)) {
      result.set(bareRunId, { runId: bareRunId, pause: null, healthFacts: facts });
    } else if (runEntityId in approvalByRunEntity) {
      const [gatedLoopId, executionId] = approvalByRunEntity[runEntityId];
      result.set(bareRunId, {
        runId: bareRunId,
        pause: { cause: "tool_gate", gatedLoopId, executionId },
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
      const [approvalTriples, answeredTriples, clarTriples, questionTriples, ...healthBatches] = await Promise.all([
        getTriples({ predicate: "agent.run.approval-pending", limit: 100, signal: ctrl.signal }),
        getTriples({ predicate: "agent.run.approval-answered", limit: 100, signal: ctrl.signal }),
        getTriples({ predicate: "agent.run.clarification-pending", limit: 100, signal: ctrl.signal }),
        getTriples({ predicate: "coordinator.clarification.question", limit: 100, signal: ctrl.signal }),
        ...RUN_HEALTH_PREDICATES.map((predicate) =>
          getTriples({ predicate, limit: 100, signal: ctrl.signal }),
        ),
      ]);
      const healthTriples = healthBatches.flat();
      const runEntityIds = runEntityIdsFromTriples(
        approvalTriples,
        answeredTriples,
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
        answeredTriples,
      );

      // Replace the observed run status, including resumed runs whose
      // approval receipts remain in graph history.
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

    /** Get the status for an observed bare runId. */
    get(runId: string): RunStatus | undefined {
      return statuses.get(runId);
    },

    /** All observed run statuses, including historical runs. */
    getList(): RunStatus[] {
      return [...statuses.values()];
    },

    /** Count of runs currently paused and waiting on the operator. */
    get pausedCount(): number {
      return [...statuses.values()].filter((status) => status.pause !== null).length;
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
