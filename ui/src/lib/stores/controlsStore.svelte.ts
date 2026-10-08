// Controls store — polls the graph for rule-fired loops (design D3 / task 3.5).
//
// `agent.loop.task` on a loop entity is `rule-<firing entity id>-<nanos>` when
// a rule spawned it. Neither the loops REST nor the SSE stream carries that, so
// the runs lens reads it from GET /graph/triples, the same path runStatus
// polls, with the same cadence and abort discipline. taskStore uses the result
// to keep control loops off the board as top-level cards and to attach each to
// the coordinator card of the run that fired it.

import { getTriples } from "$lib/services/runStatusApi";
import {
  EMPTY_CONTROLS,
  LOOP_TASK_PREDICATE,
  buildControlsSnapshot,
  controlsSignature,
  type Control,
  type ControlsSnapshot,
} from "$lib/types/control";

export const POLL_INTERVAL_MS = 2500;
export const CONTROLS_READ_LIMIT = 500;

function createControlsStore() {
  // $state.raw: the snapshot is replaced wholesale, never mutated in place, and
  // it holds a Set and BigInts that a deep proxy would only get in the way of.
  let snapshot = $state.raw<ControlsSnapshot>(EMPTY_CONTROLS);
  let signature = controlsSignature(EMPTY_CONTROLS);
  let lastError = $state<string | null>(null);
  let intervalId: ReturnType<typeof setInterval> | null = null;
  let fetching = false;
  // The in-flight poll's abort controller, so stop() can cancel a pending
  // fetch and the finally-block runs promptly (resetting `fetching`). Mirrors
  // runStatus.
  let currentAbort: AbortController | null = null;

  async function pollOnce(): Promise<void> {
    if (fetching) return;
    fetching = true;
    const ctrl = new AbortController();
    currentAbort = ctrl;
    try {
      const triples = await getTriples({
        predicate: LOOP_TASK_PREDICATE,
        limit: CONTROLS_READ_LIMIT,
        signal: ctrl.signal,
      });
      const next = buildControlsSnapshot(triples, CONTROLS_READ_LIMIT);
      // Only write when something changed: a write re-derives every task card.
      const nextSignature = controlsSignature(next);
      if (nextSignature !== signature) {
        signature = nextSignature;
        snapshot = next;
      }
      lastError = null;
    } catch (err) {
      // An intentional stop()-abort is not a failure. Any other error keeps the
      // last good snapshot and is captured, not thrown.
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

    /** Controls grouped by the run instance (= coordinator loop id) that fired them. */
    get controlsByRun(): Record<string, Control[]> {
      return snapshot.controlsByRun;
    },

    /** Bare loop ids of every known control. */
    get controlLoopIds(): Set<string> {
      return snapshot.controlLoopIds;
    },

    /** True when the read hit its limit: the control list may be incomplete. */
    get truncated(): boolean {
      return snapshot.truncated;
    },

    /** Last poll error, or null when the most recent poll succeeded. */
    get lastError(): string | null {
      return lastError;
    },

    /** Run a single poll tick directly (unit tests drive this with a mocked getTriples). */
    pollOnce,
  };
}

export const controlsStore = createControlsStore();
