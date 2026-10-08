// Controls store — a membership cache for rule-fired loops (design D3 / task 3.5).
//
// A loop whose `task_id` is `rule-<run entity>-<nanos>` is only a control
// CANDIDATE: the autoresearch pack fires its propose/synthesize WORK loops on
// the run entity the same way, and those are run members that must stay on the
// board. What separates the two is `agent.loop.run` on the loop's own entity
// (present for a member, absent for a `run_scope: none` control such as the ops
// observer). The loops REST and SSE do not carry it, so each candidate's entity
// is read from GET /graph/triples ONCE and the answer is cached per loop id: a
// resolved loop is never read again. Reads that fail or are cut off leave the
// candidate unknown, to be retried with a bounded backoff. An unknown candidate
// is left exactly where it was, a top-level card.
//
// Candidates come from agentStore on every tick (same cadence and abort
// discipline as runStatus), so a loop that appears between ticks is classified
// by the next one.

import { getTriples } from "$lib/services/runStatusApi";
import {
  EMPTY_CONTROLS,
  buildControlsSnapshot,
  controlsSignature,
  membershipFromLoopTriples,
  parseControlCandidate,
  type Control,
  type ControlCandidate,
  type ControlsSnapshot,
  type MembershipRead,
} from "$lib/types/control";
import type { AgentLoop } from "$lib/types/agent";
import { agentStore } from "./agentStore.svelte";

export const POLL_INTERVAL_MS = 2500;
/** A loop entity has a dozen or so triples; a page this full is treated as cut off. */
export const MEMBERSHIP_READ_LIMIT = 200;
export const MAX_READ_CONCURRENCY = 4;
/** The first read plus its retries. After this the candidate stays unknown. */
export const MAX_ATTEMPTS = 5;
/** Retry n waits RETRY_BASE_MS * 2^(n-1): 2s, 4s, 8s, 16s. */
export const RETRY_BASE_MS = 2000;

interface Tracked {
  control: Control;
  status: "pending" | "member" | "control" | "unknown";
  /** Failed reads so far. */
  attempts: number;
  nextAttemptAt: number;
  reason: string;
  /** The last failure was a real error or truncation, not a not-yet-readable entity. */
  hard: boolean;
}

function isAbort(err: unknown): boolean {
  return err instanceof DOMException && err.name === "AbortError";
}

function createControlsStore() {
  // $state.raw: the snapshot is replaced wholesale, never mutated in place, and
  // it holds Sets and BigInts that a deep proxy would only get in the way of.
  let snapshot = $state.raw<ControlsSnapshot>(EMPTY_CONTROLS);
  let signature = controlsSignature(EMPTY_CONTROLS);
  let lastError = $state<string | null>(null);
  // The cache, not reactive state (consumers read `snapshot`), so a plain
  // record rather than a SvelteMap. No prototype, so a loop id can never collide
  // with an inherited key.
  let tracked: Record<string, Tracked> = Object.create(null);
  let intervalId: ReturnType<typeof setInterval> | null = null;
  let fetching = false;
  // The in-flight poll's abort controller, so stop() can cancel pending
  // fetches and the finally-block runs promptly. Mirrors runStatus.
  let currentAbort: AbortController | null = null;

  /** Real failures and exhausted retries are worth telling the operator about. */
  function surfaced(t: Tracked): boolean {
    return t.status === "unknown" && (t.hard || t.attempts >= MAX_ATTEMPTS);
  }

  function discover(loops: AgentLoop[]): void {
    for (const loop of loops) {
      if (loop.loop_id in tracked) continue;
      const control = parseControlCandidate(loop);
      if (control) {
        tracked[loop.loop_id] = {
          control,
          status: "pending",
          attempts: 0,
          nextAttemptAt: 0,
          reason: "",
          hard: false,
        };
      }
    }
  }

  function isDue(t: Tracked, now: number): boolean {
    return (
      (t.status === "pending" || t.status === "unknown") &&
      t.attempts < MAX_ATTEMPTS &&
      t.nextAttemptAt <= now
    );
  }

  function record(t: Tracked, read: MembershipRead): void {
    if (read.status === "member" || read.status === "control") {
      t.status = read.status;
      t.reason = "";
      t.hard = false;
      return;
    }
    fail(t, read.reason, read.hard);
  }

  function fail(t: Tracked, reason: string, hard: boolean): void {
    t.status = "unknown";
    t.attempts += 1;
    t.reason = reason;
    t.hard = hard;
    t.nextAttemptAt = Date.now() + RETRY_BASE_MS * 2 ** (t.attempts - 1);
  }

  async function readOne(t: Tracked, signal: AbortSignal): Promise<void> {
    try {
      const triples = await getTriples({
        subject: t.control.loopEntityId,
        limit: MEMBERSHIP_READ_LIMIT,
        signal,
      });
      record(t, membershipFromLoopTriples(triples, MEMBERSHIP_READ_LIMIT));
    } catch (err) {
      // An abort means stop() moved on: it is not a failed read and must not
      // spend one of the candidate's attempts.
      if (isAbort(err)) throw err;
      fail(t, err instanceof Error ? err.message : String(err), true);
    }
  }

  async function readAll(due: Tracked[], signal: AbortSignal): Promise<void> {
    let next = 0;
    const lane = async () => {
      while (next < due.length) await readOne(due[next++], signal);
    };
    await Promise.all(
      Array.from({ length: Math.min(MAX_READ_CONCURRENCY, due.length) }, lane),
    );
  }

  function publish(): void {
    const candidates: ControlCandidate[] = [];
    let error: string | null = null;
    for (const t of Object.values(tracked)) {
      const isSurfaced = surfaced(t);
      if (isSurfaced) error = t.reason;
      candidates.push({
        control: t.control,
        membership:
          t.status === "unknown"
            ? { status: "unknown", reason: t.reason, surfaced: isSurfaced }
            : { status: t.status },
      });
    }
    // Only write when something changed: a write re-derives every task card.
    const next = buildControlsSnapshot(candidates);
    const nextSignature = controlsSignature(next);
    if (nextSignature !== signature) {
      signature = nextSignature;
      snapshot = next;
    }
    lastError = error;
  }

  async function pollOnce(): Promise<void> {
    if (fetching) return;
    fetching = true;
    const ctrl = new AbortController();
    currentAbort = ctrl;
    try {
      discover(agentStore.loopsList);
      const now = Date.now();
      await readAll(
        Object.values(tracked).filter((t) => isDue(t, now)),
        ctrl.signal,
      );
      publish();
    } catch (err) {
      // An intentional stop()-abort is not a failure. Reads never throw
      // anything else (readOne records it), so there is nothing to report.
      if (!isAbort(err)) throw err;
    } finally {
      if (currentAbort === ctrl) {
        currentAbort = null;
        fetching = false;
      }
    }
  }

  return {
    /** Start polling. Idempotent — no-op if already running. */
    start() {
      if (intervalId !== null) return;
      void pollOnce(); // immediate first tick
      intervalId = setInterval(() => void pollOnce(), POLL_INTERVAL_MS);
    },

    /** Stop polling, cancel any in-flight reads, and clear the interval. */
    stop() {
      if (intervalId !== null) {
        clearInterval(intervalId);
        intervalId = null;
      }
      currentAbort?.abort();
      currentAbort = null;
      fetching = false; // safety reset so a later start() always polls
    },

    /** Forget every cached answer (unit tests; a fresh session). */
    reset() {
      tracked = Object.create(null);
      signature = controlsSignature(EMPTY_CONTROLS);
      snapshot = EMPTY_CONTROLS;
      lastError = null;
    },

    /** The resolved view: controls, run members and unclassified candidates. */
    get snapshot(): ControlsSnapshot {
      return snapshot;
    },

    /** Resolved controls grouped by the run instance (a loop id) that fired them. */
    get controlsByRun(): Record<string, Control[]> {
      return snapshot.controlsByRun;
    },

    /** Bare loop ids of every resolved control. */
    get controlLoopIds(): Set<string> {
      return snapshot.controlLoopIds;
    },

    /**
     * True while a top-level loop might still turn out to be a control: it is a
     * candidate that has not been resolved as a run member and has not given up
     * being classified. Callers use it to avoid spending a #ref on a loop that
     * will probably not be a card.
     */
    mayBeControl(loop: AgentLoop): boolean {
      if (parseControlCandidate(loop) === null) return false;
      return !snapshot.memberLoopIds.has(loop.loop_id) && !snapshot.unclassifiedLoopIds.has(loop.loop_id);
    },

    /** Candidates that could not be classified (failed read or retries exhausted). */
    get unclassifiedCount(): number {
      return snapshot.unclassifiedLoopIds.size;
    },

    /** Why the most recent unclassified candidate could not be read, or null when none are. */
    get lastError(): string | null {
      return lastError;
    },

    /** Run a single poll tick directly (unit tests drive this with a mocked getTriples). */
    pollOnce,
  };
}

export const controlsStore = createControlsStore();
