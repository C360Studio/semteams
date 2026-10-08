// Rule-fired control loops (OpenSpec work-board-read-only, design D3 / task 3.5).
//
// A "control" is a loop a rule spawned on a RUN entity (for example the ops
// observer fired by a run reaching a terminal phase). The frozen runtime
// records it as a loop entity whose `agent.loop.task` is
// `rule-<firing entity id>-<unix nanos>`. The runs lens attaches such loops to
// the coordinator card of the run that fired them instead of rendering them as
// top-level cards, and explains each one from the graph facts below.
//
// What the runtime does NOT record is the identity of the rule itself (#298),
// so "rule" is always rendered as unknown. Nothing here guesses it.
//
// Everything in this module is pure (no fetch, no stores) so the parsing and
// the "fact at spawn" selection are unit-testable on their own.

import type { AgentLoop, AgentLoopState } from "./agent";
import type { RawTriple } from "$lib/services/runStatusApi";

export const RUN_ENTITY_INFIX = ".chain.agent.execution.";
export const LOOP_ENTITY_INFIX = ".agentic-loop.agent.execution.";
export const LOOP_TASK_PREDICATE = "agent.loop.task";

/** A rule-fired loop, as read from its `agent.loop.task` provenance triple. */
export interface Control {
  /** Bare loop id: the last dotted segment of the loop entity id. */
  loopId: string;
  loopEntityId: string;
  /** The RUN entity whose change fired the rule. */
  firingEntityId: string;
  /** The run instance: the last dotted segment of the firing entity id. */
  runInstance: string;
  /** When the rule spawned the loop (millisecond precision). */
  spawnedAt: Date;
  /**
   * The spawn time at full nanosecond precision. Lifecycle transitions are
   * stamped to the nanosecond; ordering them against a millisecond Date
   * would put a transition that landed a microsecond AFTER the spawn on the
   * wrong side of it.
   */
  spawnedAtNanos: bigint;
}

/** A control attached to a task, with the live loop for state and outcome. */
export interface TaskControl extends Control {
  loop: AgentLoop;
}

export interface ControlsSnapshot {
  controlsByRun: Record<string, Control[]>;
  controlLoopIds: Set<string>;
  /**
   * True when the provenance read hit its limit. The scan applies the limit
   * before any filtering, so a full page means rule-fired loops may be missing
   * and every control-derived display must say so.
   */
  truncated: boolean;
}

export const EMPTY_CONTROLS: ControlsSnapshot = {
  controlsByRun: {},
  controlLoopIds: new Set<string>(),
  truncated: false,
};

function lastSegment(entityId: string): string {
  return entityId.slice(entityId.lastIndexOf(".") + 1);
}

// `.+` is greedy, so the match splits at the LAST `-<digits>`; the firing
// entity id itself ends in a UUID and may contain hyphens and digits.
const RULE_TASK_ID = /^rule-(.+)-(\d+)$/;

/**
 * Parse one `agent.loop.task` triple. Returns null unless it names a loop
 * spawned by a rule firing on a RUN entity. Dispatch-spawned loops
 * (`dispatch-<hash>`) and chain children whose firing entity is another LOOP
 * (already attached through `parent_loop_id`) are deliberately not controls.
 */
export function parseControlTriple(triple: RawTriple): Control | null {
  if (triple.predicate !== LOOP_TASK_PREDICATE) return null;
  if (!triple.subject.includes(LOOP_ENTITY_INFIX)) return null;
  const match = RULE_TASK_ID.exec(triple.object);
  if (!match) return null;
  const [, firingEntityId, nanos] = match;
  if (!firingEntityId.includes(RUN_ENTITY_INFIX)) return null;
  const loopId = lastSegment(triple.subject);
  const runInstance = lastSegment(firingEntityId);
  if (!loopId || !runInstance) return null;
  const spawnedAtNanos = BigInt(nanos);
  return {
    loopId,
    loopEntityId: triple.subject,
    firingEntityId,
    runInstance,
    spawnedAt: new Date(Number(spawnedAtNanos / 1_000_000n)),
    spawnedAtNanos,
  };
}

/**
 * Fold a page of `agent.loop.task` triples into the per-run control index.
 * `limit` is the page size that was requested: a result of that length is
 * treated as truncated.
 */
export function buildControlsSnapshot(
  triples: RawTriple[],
  limit: number,
): ControlsSnapshot {
  const byLoop = new Map<string, Control>();
  for (const triple of triples) {
    const control = parseControlTriple(triple);
    if (control) byLoop.set(control.loopId, control);
  }
  const controlsByRun: Record<string, Control[]> = {};
  for (const control of byLoop.values()) {
    (controlsByRun[control.runInstance] ??= []).push(control);
  }
  for (const controls of Object.values(controlsByRun)) {
    controls.sort((a, b) =>
      a.spawnedAtNanos === b.spawnedAtNanos
        ? a.loopId.localeCompare(b.loopId)
        : a.spawnedAtNanos < b.spawnedAtNanos
          ? -1
          : 1,
    );
  }
  return {
    controlsByRun,
    controlLoopIds: new Set(byLoop.keys()),
    truncated: triples.length >= limit,
  };
}

/** Stable identity for change detection, so an unchanged poll is not a reactive write. */
export function controlsSignature(snapshot: ControlsSnapshot): string {
  const rows = Object.entries(snapshot.controlsByRun)
    .flatMap(([run, controls]) => controls.map((c) => `${run}:${c.loopId}:${c.spawnedAtNanos}`))
    .sort();
  return `${snapshot.truncated ? "T" : "F"}|${rows.join(",")}`;
}

// ---------------------------------------------------------------------------
// Attachment to tasks
// ---------------------------------------------------------------------------

/**
 * Split the loop list into board-level loops and control loops. A top-level
 * loop (no parent) whose id is a known control is NOT a board card: it is
 * returned in `controlLoops` so it can be attached to its coordinator (and
 * stays off the board in the meantime if that coordinator has not appeared).
 * Controls that already carry a `parent_loop_id` are left alone.
 */
export function splitControlLoops(
  loops: AgentLoop[],
  controlLoopIds: Set<string>,
): { topLevel: AgentLoop[]; controlLoops: AgentLoop[] } {
  const topLevel: AgentLoop[] = [];
  const controlLoops: AgentLoop[] = [];
  for (const loop of loops) {
    if (loop.parent_loop_id) continue;
    if (controlLoopIds.has(loop.loop_id)) controlLoops.push(loop);
    else topLevel.push(loop);
  }
  return { topLevel, controlLoops };
}

/** Attach the live loops to the controls of one run. Controls without a live loop are omitted. */
export function controlsForRun(
  runInstance: string,
  controlsByRun: Record<string, Control[]>,
  controlLoops: AgentLoop[],
): TaskControl[] {
  const known = controlsByRun[runInstance];
  if (!known?.length) return [];
  const out: TaskControl[] = [];
  for (const control of known) {
    const loop = controlLoops.find((l) => l.loop_id === control.loopId);
    if (loop) out.push({ ...control, loop });
  }
  return out;
}

export type ControlOutcome = "accepted" | "applied" | "rejected";

/**
 * Outcome as the control loop's own state (design D3): the loop existing means
 * the engine accepted the spawn; completing means it was applied; failing
 * means it was rejected. `error` is the same terminal vocabulary as `failed`
 * (see agent.ts); any other state (running, cancelled, truncated) stays
 * "accepted" and the caller shows the raw state beside it.
 */
export function controlOutcome(state: AgentLoopState): ControlOutcome {
  if (state === "complete" || state === "success") return "applied";
  if (state === "failed" || state === "error") return "rejected";
  return "accepted";
}

// ---------------------------------------------------------------------------
// Explained-row evidence (read on demand from the graph)
// ---------------------------------------------------------------------------

/** A value read from the graph, or an explicit unknown with the reason it is unknown. */
export type Known<T> =
  | { status: "known"; value: T }
  | { status: "unknown"; reason: string };

export const READ_TRUNCATED = "read truncated";

export interface LifecycleTransition {
  /** Nanoseconds since the epoch, from `lifecycle.transition.at`. */
  atNanos: bigint;
  from: string;
  to: string;
  source: string;
  note: string;
}

export interface ControlEvidence {
  /** The firing run's lifecycle transition that precedes the spawn. */
  fact: Known<LifecycleTransition>;
  nextAction: Known<string>;
  reason: Known<string>;
  outcome: Known<string>;
  description: Known<string>;
}

const RFC3339 =
  /^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(?:\.(\d{1,9})\d*)?(Z|[+-]\d{2}:\d{2})$/;

/** Parse an RFC 3339 timestamp (Go RFC3339Nano) to nanoseconds, or null. */
export function parseRfc3339Nanos(value: string | undefined): bigint | null {
  if (!value) return null;
  const match = RFC3339.exec(value);
  if (!match) return null;
  const [, base, fraction = "", zone] = match;
  const seconds = Date.parse(`${base}${zone}`);
  if (Number.isNaN(seconds)) return null;
  return BigInt(seconds) * 1_000_000n + BigInt(fraction.padEnd(9, "0"));
}

const TRANSITION_PREFIX = "lifecycle.transition.";

/**
 * Group a run entity's `lifecycle.transition.*` triples into transitions.
 * Each predicate holds several values over the run's life; the five triples of
 * one transition share the same triple `timestamp`, so that is the group key.
 * The transition's own time is its `.at` value, falling back to the group
 * timestamp; a group with neither is dropped because it cannot be placed.
 */
export function groupLifecycleTransitions(triples: RawTriple[]): LifecycleTransition[] {
  const groups = new Map<string, Record<string, string>>();
  for (const triple of triples) {
    if (!triple.predicate.startsWith(TRANSITION_PREFIX)) continue;
    const key = triple.timestamp ?? "";
    const fields = groups.get(key) ?? {};
    const field = triple.predicate.slice(TRANSITION_PREFIX.length);
    if (!(field in fields)) fields[field] = String(triple.object ?? "");
    groups.set(key, fields);
  }
  const transitions: LifecycleTransition[] = [];
  for (const [timestamp, fields] of groups) {
    const atNanos = parseRfc3339Nanos(fields.at) ?? parseRfc3339Nanos(timestamp);
    if (atNanos === null) continue;
    transitions.push({
      atNanos,
      from: fields.from ?? "",
      to: fields.to ?? "",
      source: fields.source ?? "",
      note: fields.note ?? "",
    });
  }
  return transitions.sort((a, b) => (a.atNanos < b.atNanos ? -1 : a.atNanos > b.atNanos ? 1 : 0));
}

/**
 * The transition in force when the rule spawned the loop: the one with the
 * greatest `at` that is not after the spawn. Null when the spawn precedes
 * every recorded transition.
 */
export function transitionPrecedingSpawn(
  transitions: LifecycleTransition[],
  spawnedAtNanos: bigint,
): LifecycleTransition | null {
  let found: LifecycleTransition | null = null;
  for (const transition of transitions) {
    if (transition.atNanos <= spawnedAtNanos) found = transition;
  }
  return found;
}

/** Most recent object of a single-valued predicate, or null when absent. */
function latestObject(triples: RawTriple[], predicate: string): string | null {
  let best: { object: string; at: bigint } | null = null;
  for (const triple of triples) {
    if (triple.predicate !== predicate) continue;
    const at = parseRfc3339Nanos(triple.timestamp) ?? 0n;
    if (best === null || at >= best.at) best = { object: String(triple.object ?? ""), at };
  }
  return best ? best.object : null;
}

function field(
  triples: RawTriple[],
  predicate: string,
  truncated: boolean,
  absent: string,
): Known<string> {
  // A truncated page may be missing the latest value of any predicate, so a
  // value read from it is not trustworthy either.
  if (truncated) return { status: "unknown", reason: READ_TRUNCATED };
  const value = latestObject(triples, predicate);
  return value === null || value === ""
    ? { status: "unknown", reason: absent }
    : { status: "known", value };
}

export interface ControlReads {
  /** Triples of the firing run entity, or null when that read failed. */
  run: RawTriple[] | null;
  runTruncated: boolean;
  runError?: string;
  /** Triples of the control loop entity, or null when that read failed. */
  loop: RawTriple[] | null;
  loopTruncated: boolean;
  loopError?: string;
}

/** Derive the explained-row fields from the two graph reads. */
export function deriveControlEvidence(
  control: Control,
  reads: ControlReads,
): ControlEvidence {
  let fact: Known<LifecycleTransition>;
  if (reads.run === null) {
    fact = { status: "unknown", reason: `read failed: ${reads.runError ?? "unknown error"}` };
  } else if (reads.runTruncated) {
    fact = { status: "unknown", reason: READ_TRUNCATED };
  } else {
    const transition = transitionPrecedingSpawn(
      groupLifecycleTransitions(reads.run),
      control.spawnedAtNanos,
    );
    fact = transition
      ? { status: "known", value: transition }
      : { status: "unknown", reason: "no lifecycle transition precedes the spawn" };
  }

  const loopTriples = reads.loop;
  const loopField = (predicate: string, absent: string): Known<string> =>
    loopTriples === null
      ? { status: "unknown", reason: `read failed: ${reads.loopError ?? "unknown error"}` }
      : field(loopTriples, predicate, reads.loopTruncated, absent);

  return {
    fact,
    nextAction: loopField("coordinator.decision.next-action", "not recorded yet"),
    reason: loopField("coordinator.decision.reason", "not recorded yet"),
    outcome: loopField("agent.loop.outcome", "not recorded yet"),
    description: loopField("agent.loop.description", "not recorded"),
  };
}
