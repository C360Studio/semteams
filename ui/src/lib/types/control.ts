// Rule-fired control loops (OpenSpec work-board-read-only, design D3 / task 3.5).
//
// A "control" is a loop a rule spawned on a RUN entity that is NOT a member of
// that run: for example the ops observer fired by a run reaching a terminal
// phase (`run_scope: none`). Two facts define it:
//
//   1. its task id is `rule-<firing entity id>-<unix nanos>` and the firing
//      entity is a run entity (`AgentLoop.task_id`, already on the loops REST
//      and the SSE stream), and
//   2. its loop entity carries no `agent.loop.run` triple. A loop that does is
//      a member of that run (the autoresearch pack's propose/synthesize loops
//      are fired on the run entity too, but they are the run's work, not
//      controls, and their descendants hang off them through `parent_loop_id`).
//
// Fact 1 only makes a loop a CANDIDATE. Fact 2 needs one graph read per
// candidate (controlsStore) and is cached. Until a candidate is resolved as a
// non-member it stays exactly where it was: a top-level card.
//
// What the runtime does NOT record is the identity of the rule itself (#298),
// so "rule" is always rendered as unknown. Nothing here guesses it.
//
// Everything in this module is pure (no fetch, no stores) so the parsing, the
// membership rule and the attachment are unit-testable on their own.

import type { AgentLoop, AgentLoopState } from "./agent";
import type { RawTriple } from "$lib/services/runStatusApi";

export const RUN_ENTITY_INFIX = ".chain.agent.execution.";
export const LOOP_ENTITY_INFIX = ".agentic-loop.agent.execution.";
/** Present on every loop entity from its spawn-identity snapshot. */
export const LOOP_TASK_PREDICATE = "agent.loop.task";
/** Present on a loop entity exactly when the loop is a member of a run. */
export const LOOP_RUN_PREDICATE = "agent.loop.run";

/** A rule-fired loop on a run entity (a control candidate, or a resolved control). */
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

function lastSegment(entityId: string): string {
  return entityId.slice(entityId.lastIndexOf(".") + 1);
}

// `.+` is greedy, so the match splits at the LAST `-<digits>`; the firing
// entity id itself ends in a UUID and may contain hyphens and digits. A UUID's
// last group is 12 characters, so requiring 16+ digits for the nanosecond
// stamp (a Unix-nanos value has 19) keeps an all-digit last group from being
// mistaken for the stamp when the real one is missing.
const RULE_TASK_ID = /^rule-(.+)-(\d{16,})$/;

/**
 * Parse a loop as a control CANDIDATE: its `task_id` names a rule firing on a
 * RUN entity. Returns null for anything else: dispatch-spawned loops
 * (`dispatch-<hash>`), chain children whose firing entity is another LOOP
 * (already attached through `parent_loop_id`), and loops that already carry a
 * parent. Whether the candidate is a control or a run member is NOT known here.
 */
export function parseControlCandidate(loop: AgentLoop): Control | null {
  if (loop.parent_loop_id) return null;
  const match = RULE_TASK_ID.exec(loop.task_id ?? "");
  if (!match) return null;
  const [, firingEntityId, nanos] = match;
  const split = firingEntityId.indexOf(RUN_ENTITY_INFIX);
  if (split === -1) return null;
  const runInstance = lastSegment(firingEntityId);
  if (!loop.loop_id || !runInstance) return null;
  const spawnedAtNanos = BigInt(nanos);
  return {
    loopId: loop.loop_id,
    // The loop entity lives under the same org.platform as the run that fired it.
    loopEntityId: `${firingEntityId.slice(0, split)}${LOOP_ENTITY_INFIX}${loop.loop_id}`,
    firingEntityId,
    runInstance,
    spawnedAt: new Date(Number(spawnedAtNanos / 1_000_000n)),
    spawnedAtNanos,
  };
}

/** What one read of a candidate's loop entity says about it. */
export type MembershipRead =
  | { status: "member" }
  | { status: "control" }
  | {
      status: "unknown";
      reason: string;
      /**
       * True for a read that failed or was cut off: worth telling the operator.
       * False for an entity that is simply not readable yet, which resolves
       * itself and is only worth mentioning once the retries run out.
       */
      hard: boolean;
    };

export const MEMBERSHIP_TRUNCATED = "membership read truncated";
export const ENTITY_NOT_READABLE = "loop entity not readable yet";

/**
 * Decide membership from one read of the candidate's loop entity, limited to
 * `limit` triples. `agent.loop.run` anywhere in the page is conclusive even
 * from a truncated page. Its absence only counts when the page is complete AND
 * shows the entity's spawn-identity snapshot (`agent.loop.task`): an empty or
 * partial read must never be cached as "not a member", because that would fold
 * a work loop and its descendants away.
 */
export function membershipFromLoopTriples(
  triples: RawTriple[],
  limit: number,
): MembershipRead {
  if (triples.some((t) => t.predicate === LOOP_RUN_PREDICATE)) return { status: "member" };
  if (triples.length >= limit) {
    return { status: "unknown", reason: MEMBERSHIP_TRUNCATED, hard: true };
  }
  if (!triples.some((t) => t.predicate === LOOP_TASK_PREDICATE)) {
    return { status: "unknown", reason: ENTITY_NOT_READABLE, hard: false };
  }
  return { status: "control" };
}

/** A candidate's membership as the store tracks it. */
export type ControlMembership =
  | { status: "pending" }
  | { status: "member" }
  | { status: "control" }
  | { status: "unknown"; reason: string; surfaced: boolean };

export interface ControlCandidate {
  control: Control;
  membership: ControlMembership;
}

/**
 * The resolved view of every candidate. Pending candidates and unknown ones
 * that are not yet worth reporting appear in none of the sets: they stay where
 * they were, as top-level cards.
 */
export interface ControlsSnapshot {
  /** Resolved controls (non-members), grouped by the run instance that fired them. */
  controlsByRun: Record<string, Control[]>;
  controlLoopIds: Set<string>;
  /** Candidates resolved as run members: ordinary work loops, never folded. */
  memberLoopIds: Set<string>;
  /**
   * Candidates whose membership could not be read, after a failed read or once
   * their retries ran out. They stay top-level cards and the runs they fired
   * on may be missing controls.
   */
  unclassifiedLoopIds: Set<string>;
  unclassifiedRuns: Set<string>;
}

export const EMPTY_CONTROLS: ControlsSnapshot = {
  controlsByRun: {},
  controlLoopIds: new Set<string>(),
  memberLoopIds: new Set<string>(),
  unclassifiedLoopIds: new Set<string>(),
  unclassifiedRuns: new Set<string>(),
};

function compareSpawn(a: Control, b: Control): number {
  if (a.spawnedAtNanos === b.spawnedAtNanos) return a.loopId.localeCompare(b.loopId);
  return a.spawnedAtNanos < b.spawnedAtNanos ? -1 : 1;
}

/** Fold the tracked candidates into the snapshot taskStore and the layout read. */
export function buildControlsSnapshot(candidates: Iterable<ControlCandidate>): ControlsSnapshot {
  const controlsByRun: Record<string, Control[]> = {};
  const controlLoopIds = new Set<string>();
  const memberLoopIds = new Set<string>();
  const unclassifiedLoopIds = new Set<string>();
  const unclassifiedRuns = new Set<string>();
  for (const { control, membership } of candidates) {
    if (membership.status === "control") {
      (controlsByRun[control.runInstance] ??= []).push(control);
      controlLoopIds.add(control.loopId);
    } else if (membership.status === "member") {
      memberLoopIds.add(control.loopId);
    } else if (membership.status === "unknown" && membership.surfaced) {
      unclassifiedLoopIds.add(control.loopId);
      unclassifiedRuns.add(control.runInstance);
    }
  }
  for (const controls of Object.values(controlsByRun)) controls.sort(compareSpawn);
  return { controlsByRun, controlLoopIds, memberLoopIds, unclassifiedLoopIds, unclassifiedRuns };
}

/** Stable identity for change detection, so an unchanged poll is not a reactive write. */
export function controlsSignature(snapshot: ControlsSnapshot): string {
  const rows = [
    ...Object.entries(snapshot.controlsByRun).flatMap(([run, controls]) =>
      controls.map((c) => `C:${run}:${c.loopId}:${c.spawnedAtNanos}`),
    ),
    ...[...snapshot.memberLoopIds].map((id) => `M:${id}`),
    ...[...snapshot.unclassifiedLoopIds].map((id) => `U:${id}`),
  ].sort();
  return rows.join(",");
}

// ---------------------------------------------------------------------------
// Attachment to tasks
// ---------------------------------------------------------------------------

export interface ControlAttachment {
  /** The loops that are board cards: every top-level loop except the folded controls. */
  topLevel: AgentLoop[];
  /** Folded controls, keyed by the top-level loop (task id) they attach to. */
  controlsByTask: Record<string, TaskControl[]>;
  /**
   * Loop ids of the controls that were actually folded. A control the snapshot
   * resolved but this refused to fold (it has children, or its run's loop is not
   * on the board) is not in here: it is still a card.
   */
  foldedLoopIds: Set<string>;
  /** Tasks whose controls may be incomplete because a candidate on their runs is unclassified. */
  incompleteTaskIds: Set<string>;
}

/**
 * Fold resolved controls into the task that owns the run they fired on.
 *
 * A run instance is a loop id: the coordinator's, or a descendant coordinator's
 * for a nested run (`run_scope: new`). The owning task is that loop's top-level
 * ancestor, so a control fired on a nested run attaches to the same card. A
 * control stays a top-level card, unchanged, whenever it cannot be folded
 * safely: its run's loop is not on the board yet, it is itself an owner or
 * already has a parent, or another control owns the run.
 */
export function attachControls(loops: AgentLoop[], snapshot: ControlsSnapshot): ControlAttachment {
  const byId = new Map(loops.map((l) => [l.loop_id, l]));
  const parents = new Set<string>();
  for (const loop of loops) if (loop.parent_loop_id) parents.add(loop.parent_loop_id);

  const rootOf = (loopId: string): string | null => {
    const seen = new Set<string>();
    let current = byId.get(loopId);
    while (current) {
      if (seen.has(current.loop_id)) return null;
      seen.add(current.loop_id);
      if (!current.parent_loop_id) return current.loop_id;
      current = byId.get(current.parent_loop_id);
    }
    return null;
  };

  const folded = new Set<string>();
  const controlsByTask: Record<string, TaskControl[]> = {};
  for (const control of Object.values(snapshot.controlsByRun).flat()) {
    const loop = byId.get(control.loopId);
    // A loop with children is a parent of work, never a control to hide.
    if (!loop || loop.parent_loop_id || parents.has(loop.loop_id)) continue;
    const owner = rootOf(control.runInstance);
    if (!owner || owner === control.loopId || snapshot.controlLoopIds.has(owner)) continue;
    (controlsByTask[owner] ??= []).push({ ...control, loop });
    folded.add(control.loopId);
  }
  for (const attached of Object.values(controlsByTask)) attached.sort(compareSpawn);

  const incompleteTaskIds = new Set<string>();
  for (const run of snapshot.unclassifiedRuns) {
    const owner = rootOf(run);
    if (owner) incompleteTaskIds.add(owner);
  }

  return {
    topLevel: loops.filter((l) => !l.parent_loop_id && !folded.has(l.loop_id)),
    controlsByTask,
    foldedLoopIds: folded,
    incompleteTaskIds,
  };
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
