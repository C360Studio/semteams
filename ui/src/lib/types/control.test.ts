import { describe, it, expect } from "vitest";
import type { RawTriple } from "$lib/services/runStatusApi";
import type { AgentLoop } from "./agent";
import {
  EMPTY_CONTROLS,
  ENTITY_NOT_READABLE,
  MEMBERSHIP_TRUNCATED,
  attachControls,
  buildControlsSnapshot,
  controlOutcome,
  controlsSignature,
  deriveControlEvidence,
  groupLifecycleTransitions,
  membershipFromLoopTriples,
  parseControlCandidate,
  parseRfc3339Nanos,
  transitionPrecedingSpawn,
  type Control,
  type ControlCandidate,
} from "./control";

// Shapes below are copied from a measured run on the e2e stack (frozen
// semstreams 8b99efe, 2026-10-08): the platform segment carries hyphens and a
// framework-minted suffix, and nanos are a Unix-epoch nanosecond timestamp.
const PREFIX = "c360.semteams-bootstrap-e2e-6c50f3";
const RUN = "b5a2d6ec-f67d-4cc7-ab5c-b40e8223a43e";
const OBSERVER = "8b9f1444-166f-4da3-a1b5-2718995c4b23";
const RUN_ENTITY = `${PREFIX}.chain.agent.execution.${RUN}`;
const NANOS = "1791480613893413505"; // 2026-10-08T17:30:13.893413505Z

function loopEntity(loopId: string): string {
  return `${PREFIX}.agentic-loop.agent.execution.${loopId}`;
}

function control(overrides: Partial<Control> = {}): Control {
  return {
    loopId: OBSERVER,
    loopEntityId: loopEntity(OBSERVER),
    firingEntityId: RUN_ENTITY,
    runInstance: RUN,
    spawnedAt: new Date(1791480613893),
    spawnedAtNanos: BigInt(NANOS),
    ...overrides,
  };
}

function loop(loopId: string, overrides: Partial<AgentLoop> = {}): AgentLoop {
  return {
    loop_id: loopId,
    task_id: "",
    state: "executing",
    role: "ops-chain-observer",
    iterations: 1,
    max_iterations: 5,
    user_id: "",
    channel_type: "",
    parent_loop_id: "",
    outcome: "",
    error: "",
    ...overrides,
  };
}

// One lifecycle transition is five triples that share a triple timestamp.
function transition(
  at: string,
  from: string,
  to: string,
  source: string,
  note: string,
): RawTriple[] {
  const base = { subject: RUN_ENTITY, timestamp: at };
  return [
    { ...base, predicate: "lifecycle.transition.at", object: at },
    { ...base, predicate: "lifecycle.transition.from", object: from },
    { ...base, predicate: "lifecycle.transition.to", object: to },
    { ...base, predicate: "lifecycle.transition.source", object: source },
    { ...base, predicate: "lifecycle.transition.note", object: note },
  ];
}

const RUN_HISTORY: RawTriple[] = [
  ...transition("2026-10-08T17:30:13.516406629Z", "", "dispatched", "framework", "created"),
  ...transition(
    "2026-10-08T17:30:13.629098755Z",
    "dispatched",
    "executing",
    "rule",
    "child task handed off (rule.task.spawned confirmed) — run is now executing",
  ),
  ...transition(
    "2026-10-08T17:30:13.846566421Z",
    "executing",
    "completed",
    "rule",
    "chain reached its success terminal (reviewer/CBG approved) — run completed",
  ),
];

describe("parseControlCandidate", () => {
  const observer = (taskId: string, overrides: Partial<AgentLoop> = {}) =>
    loop(OBSERVER, { task_id: taskId, ...overrides });

  it("derives the candidate from a loop whose task id is a rule firing on a run entity", () => {
    const parsed = parseControlCandidate(observer(`rule-${RUN_ENTITY}-${NANOS}`));
    expect(parsed).toMatchObject({
      loopId: OBSERVER,
      // Rebuilt from the firing entity's org.platform: the loop entity is not read to find it.
      loopEntityId: loopEntity(OBSERVER),
      firingEntityId: RUN_ENTITY,
      runInstance: RUN,
    });
  });

  it("converts the nanosecond spawn time to a Date and keeps full precision", () => {
    const parsed = parseControlCandidate(observer(`rule-${RUN_ENTITY}-${NANOS}`))!;
    expect(parsed.spawnedAt.toISOString()).toBe("2026-10-08T17:30:13.893Z");
    // 19 digits overflow a double; the exact value must survive.
    expect(parsed.spawnedAtNanos).toBe(1791480613893413505n);
  });

  it("parses a firing id whose last UUID group is all digits", () => {
    // The nanos split must land on the stamp, not inside the run id.
    const digitsRun = "b5a2d6ec-f67d-4cc7-ab5c-123456789012";
    const firing = `${PREFIX}.chain.agent.execution.${digitsRun}`;
    const parsed = parseControlCandidate(observer(`rule-${firing}-${NANOS}`))!;
    expect(parsed.firingEntityId).toBe(firing);
    expect(parsed.runInstance).toBe(digitsRun);
    expect(parsed.spawnedAtNanos).toBe(1791480613893413505n);
  });

  it("does not mistake an all-digit last UUID group for the stamp when the stamp is missing", () => {
    const firing = `${PREFIX}.chain.agent.execution.b5a2d6ec-f67d-4cc7-ab5c-123456789012`;
    expect(parseControlCandidate(observer(`rule-${firing}`))).toBeNull();
  });

  it("ignores dispatch-spawned loops and loops with no task id", () => {
    expect(parseControlCandidate(observer("dispatch-9f2c1a"))).toBeNull();
    expect(parseControlCandidate(observer(""))).toBeNull();
  });

  it("ignores chain children whose firing entity is a loop (already attached by parent_loop_id)", () => {
    const firingLoop = loopEntity("ce821ec0-b64b-4626-9fe1-0a027b997dbb");
    expect(parseControlCandidate(observer(`rule-${firingLoop}-${NANOS}`))).toBeNull();
  });

  it("ignores a rule task id that does not end in nanos", () => {
    expect(parseControlCandidate(observer(`rule-${RUN_ENTITY}`))).toBeNull();
  });

  it("ignores a loop that already has a parent", () => {
    expect(
      parseControlCandidate(observer(`rule-${RUN_ENTITY}-${NANOS}`, { parent_loop_id: RUN })),
    ).toBeNull();
  });
});

describe("membershipFromLoopTriples", () => {
  const entity = (predicate: string, object = "x"): RawTriple => ({
    subject: loopEntity(OBSERVER),
    predicate,
    object,
  });
  // The ops observer's measured loop-entity triples: no agent.loop.run.
  const OBSERVER_TRIPLES = [
    entity("agent.loop.task", `rule-${RUN_ENTITY}-${NANOS}`),
    entity("agent.loop.role", "ops-chain-observer"),
    entity("agent.lineage.root"),
    entity("agent.loop.outcome", "success"),
    entity("coordinator.decision.next-action", "observed"),
  ];

  it("a loop with agent.loop.run is a run member", () => {
    expect(
      membershipFromLoopTriples([...OBSERVER_TRIPLES, entity("agent.loop.run", RUN)], 200),
    ).toEqual({ status: "member" });
  });

  it("a complete read without agent.loop.run is a control", () => {
    expect(membershipFromLoopTriples(OBSERVER_TRIPLES, 200)).toEqual({ status: "control" });
  });

  it("a truncated read is unknown, never a control", () => {
    expect(membershipFromLoopTriples(OBSERVER_TRIPLES, OBSERVER_TRIPLES.length)).toEqual({
      status: "unknown",
      reason: MEMBERSHIP_TRUNCATED,
      hard: true,
    });
  });

  it("agent.loop.run is conclusive even on a truncated page", () => {
    const page = [entity("agent.loop.run", RUN), entity("agent.loop.role", "researcher")];
    expect(membershipFromLoopTriples(page, 2)).toEqual({ status: "member" });
  });

  it("an entity that is not readable yet is unknown, not a control", () => {
    // An empty or partial read must not be cached as 'not a member': that would
    // fold a work loop (and hide its descendants) behind a coordinator card.
    expect(membershipFromLoopTriples([], 200)).toEqual({
      status: "unknown",
      reason: ENTITY_NOT_READABLE,
      hard: false,
    });
    expect(membershipFromLoopTriples([entity("agent.loop.role", "researcher")], 200)).toMatchObject({
      status: "unknown",
    });
  });
});

describe("buildControlsSnapshot", () => {
  const ctl = (loopId: string, run: string, nanos = BigInt(NANOS)) =>
    control({ loopId, loopEntityId: loopEntity(loopId), runInstance: run, spawnedAtNanos: nanos });
  const candidates: ControlCandidate[] = [
    { control: ctl("obs", RUN), membership: { status: "control" } },
    { control: ctl("work", RUN), membership: { status: "member" } },
    { control: ctl("pending", RUN), membership: { status: "pending" } },
    { control: ctl("quiet", RUN), membership: { status: "unknown", reason: "x", surfaced: false } },
    { control: ctl("loud", "run-2"), membership: { status: "unknown", reason: "y", surfaced: true } },
  ];

  it("indexes resolved controls by run and sorts the other candidates into their sets", () => {
    const snapshot = buildControlsSnapshot(candidates);
    expect(Object.keys(snapshot.controlsByRun)).toEqual([RUN]);
    expect(snapshot.controlsByRun[RUN].map((c) => c.loopId)).toEqual(["obs"]);
    expect([...snapshot.controlLoopIds]).toEqual(["obs"]);
    expect([...snapshot.memberLoopIds]).toEqual(["work"]);
    // Pending and not-yet-worth-reporting candidates are in no set: they stay cards.
    expect([...snapshot.unclassifiedLoopIds]).toEqual(["loud"]);
    expect([...snapshot.unclassifiedRuns]).toEqual(["run-2"]);
  });

  it("orders a run's controls by spawn time", () => {
    const snapshot = buildControlsSnapshot([
      { control: ctl("late", RUN, BigInt(NANOS) + 1000n), membership: { status: "control" } },
      { control: ctl("early", RUN, BigInt(NANOS) - 1000n), membership: { status: "control" } },
    ]);
    expect(snapshot.controlsByRun[RUN].map((c) => c.loopId)).toEqual(["early", "late"]);
  });

  it("gives an unchanged read an unchanged signature, and a changed one a different signature", () => {
    const a = buildControlsSnapshot(candidates);
    const b = buildControlsSnapshot([...candidates].reverse());
    expect(controlsSignature(a)).toBe(controlsSignature(b));
    expect(controlsSignature(a)).not.toBe(controlsSignature(buildControlsSnapshot(candidates.slice(1))));
    expect(controlsSignature(a)).not.toBe(controlsSignature(EMPTY_CONTROLS));
  });
});

describe("attachControls", () => {
  const coordinator = loop(RUN, { role: "coordinator", state: "complete" });
  const observer = loop(OBSERVER, { task_id: `rule-${RUN_ENTITY}-${NANOS}` });
  const snapshotOf = (...candidates: ControlCandidate[]) => buildControlsSnapshot(candidates);
  const resolved = (c: Control): ControlCandidate => ({ control: c, membership: { status: "control" } });

  it("folds a resolved control into the task that owns its run", () => {
    const { topLevel, controlsByTask, incompleteTaskIds } = attachControls(
      [coordinator, observer],
      snapshotOf(resolved(control())),
    );
    expect(topLevel.map((l) => l.loop_id)).toEqual([RUN]);
    expect(controlsByTask[RUN].map((c) => c.loopId)).toEqual([OBSERVER]);
    expect(controlsByTask[RUN][0].loop).toBe(observer);
    expect(incompleteTaskIds.size).toBe(0);
  });

  it("leaves a control as a top-level card while its run's loop is not on the board", () => {
    const { topLevel, controlsByTask } = attachControls([observer], snapshotOf(resolved(control())));
    expect(topLevel.map((l) => l.loop_id)).toEqual([OBSERVER]);
    expect(controlsByTask).toEqual({});
  });

  it("attaches a control fired on a nested run to the top-level task that owns it", () => {
    // research/06 -> child coordinator -> research/01 (run_scope: new): the run
    // instance is the child coordinator's id, a descendant of the top-level loop.
    const child = loop("child-coord", { parent_loop_id: RUN, role: "coordinator" });
    const grandchild = loop("grand", { parent_loop_id: "child-coord" });
    const nested = control({ runInstance: "child-coord" });
    const { topLevel, controlsByTask } = attachControls(
      [coordinator, child, grandchild, observer],
      snapshotOf(resolved(nested)),
    );
    expect(topLevel.map((l) => l.loop_id)).toEqual([RUN]);
    expect(controlsByTask[RUN].map((c) => c.loopId)).toEqual([OBSERVER]);
  });

  it("never folds a loop that has children, nor one that already has a parent", () => {
    const withChild = loop("child-of-observer", { parent_loop_id: OBSERVER });
    const kept = attachControls([coordinator, observer, withChild], snapshotOf(resolved(control())));
    expect(kept.topLevel.map((l) => l.loop_id)).toEqual([RUN, OBSERVER]);
    expect(kept.controlsByTask).toEqual({});

    const parented = loop(OBSERVER, { parent_loop_id: RUN });
    const alone = attachControls([coordinator, parented], snapshotOf(resolved(control())));
    expect(alone.topLevel.map((l) => l.loop_id)).toEqual([RUN]);
    expect(alone.controlsByTask).toEqual({});
  });

  it("does not fold a run member, or a candidate that is still unknown", () => {
    const member: ControlCandidate = { control: control(), membership: { status: "member" } };
    const unknown: ControlCandidate = {
      control: control(),
      membership: { status: "unknown", reason: "x", surfaced: false },
    };
    for (const candidate of [member, unknown]) {
      const { topLevel, controlsByTask } = attachControls([coordinator, observer], snapshotOf(candidate));
      expect(topLevel.map((l) => l.loop_id)).toEqual([RUN, OBSERVER]);
      expect(controlsByTask).toEqual({});
    }
  });

  it("flags the owning task incomplete when a candidate on its runs is unclassified", () => {
    const child = loop("child-coord", { parent_loop_id: RUN });
    const lost: ControlCandidate = {
      control: control({ loopId: "lost", runInstance: "child-coord" }),
      membership: { status: "unknown", reason: "HTTP 500", surfaced: true },
    };
    const { incompleteTaskIds } = attachControls([coordinator, child], snapshotOf(lost));
    expect([...incompleteTaskIds]).toEqual([RUN]);
  });

  it("orders a task's controls by spawn time across its runs", () => {
    const child = loop("child-coord", { parent_loop_id: RUN });
    const later = loop("later");
    const laterControl = control({ loopId: "later", runInstance: "child-coord", spawnedAtNanos: BigInt(NANOS) + 5n });
    const earlierControl = control();
    const { controlsByTask } = attachControls(
      [coordinator, child, later, observer],
      snapshotOf(resolved(laterControl), resolved(earlierControl)),
    );
    expect(controlsByTask[RUN].map((c) => c.loopId)).toEqual([OBSERVER, "later"]);
  });
});

describe("controlOutcome", () => {
  it.each([
    ["exploring", "accepted"],
    ["executing", "accepted"],
    ["awaiting_approval", "accepted"],
    ["cancelled", "accepted"],
    ["complete", "applied"],
    ["success", "applied"],
    ["failed", "rejected"],
    ["error", "rejected"],
  ] as const)("%s loop is %s", (state, expected) => {
    expect(controlOutcome(state)).toBe(expected);
  });
});

describe("parseRfc3339Nanos", () => {
  it("keeps nanosecond precision and trims-zero fractions", () => {
    expect(parseRfc3339Nanos("2026-10-08T17:30:13.893413505Z")).toBe(1791480613893413505n);
    expect(parseRfc3339Nanos("2026-10-08T17:30:13.5Z")).toBe(1791480613500000000n);
    expect(parseRfc3339Nanos("2026-10-08T17:30:13Z")).toBe(1791480613000000000n);
  });

  it("honours a zone offset and rejects garbage", () => {
    expect(parseRfc3339Nanos("2026-10-08T18:30:13+01:00")).toBe(1791480613000000000n);
    expect(parseRfc3339Nanos("not a time")).toBeNull();
    expect(parseRfc3339Nanos(undefined)).toBeNull();
  });
});

describe("groupLifecycleTransitions", () => {
  it("groups the five triples of one transition by their shared timestamp", () => {
    const transitions = groupLifecycleTransitions(RUN_HISTORY);
    expect(transitions).toHaveLength(3);
    expect(transitions[1]).toEqual({
      atNanos: 1791480613629098755n,
      from: "dispatched",
      to: "executing",
      source: "rule",
      note: "child task handed off (rule.task.spawned confirmed) — run is now executing",
    });
  });

  it("does not mix values across transitions even when triples arrive interleaved", () => {
    const interleaved = [...RUN_HISTORY].reverse();
    const transitions = groupLifecycleTransitions(interleaved);
    expect(transitions.map((t) => t.to)).toEqual(["dispatched", "executing", "completed"]);
    expect(transitions[2].source).toBe("rule");
    expect(transitions[0].source).toBe("framework");
  });

  it("ignores non-transition triples and groups it cannot place in time", () => {
    const noise: RawTriple = { subject: RUN_ENTITY, predicate: "agent.run.phase", object: "completed", timestamp: "2026-10-08T17:30:13.846568005Z" };
    const unplaceable: RawTriple = { subject: RUN_ENTITY, predicate: "lifecycle.transition.to", object: "x" };
    expect(groupLifecycleTransitions([noise, unplaceable])).toEqual([]);
  });
});

describe("transitionPrecedingSpawn", () => {
  const transitions = groupLifecycleTransitions(RUN_HISTORY);

  it("picks the transition with the greatest at not after the spawn", () => {
    // The observer spawned at ...893, after the completed transition at ...846.
    expect(transitionPrecedingSpawn(transitions, 1791480613893413505n)?.to).toBe("completed");
    // A spawn between executing (…629) and completed (…846) is in the executing phase.
    expect(transitionPrecedingSpawn(transitions, 1791480613700000000n)?.to).toBe("executing");
  });

  it("includes a transition stamped exactly at the spawn", () => {
    expect(transitionPrecedingSpawn(transitions, 1791480613846566421n)?.to).toBe("completed");
  });

  it("is null when the spawn precedes every transition", () => {
    expect(transitionPrecedingSpawn(transitions, 1791480613000000000n)).toBeNull();
    expect(transitionPrecedingSpawn([], 1n)).toBeNull();
  });

  it("orders by nanoseconds, not milliseconds", () => {
    // Same millisecond: a transition 1ns AFTER the spawn must not count as preceding it.
    const sameMs = groupLifecycleTransitions(
      transition("2026-10-08T17:30:13.893413506Z", "executing", "completed", "rule", "late"),
    );
    expect(transitionPrecedingSpawn(sameMs, 1791480613893413505n)).toBeNull();
  });
});

describe("deriveControlEvidence", () => {
  const loopTriples: RawTriple[] = [
    { subject: loopEntity(OBSERVER), predicate: "agent.loop.description", object: "A run has reached a terminal phase.\n\n**Run entity:** x" },
    { subject: loopEntity(OBSERVER), predicate: "coordinator.decision.next-action", object: "observed" },
    { subject: loopEntity(OBSERVER), predicate: "coordinator.decision.reason", object: "Emitted two findings." },
    { subject: loopEntity(OBSERVER), predicate: "agent.loop.outcome", object: "success" },
  ];
  const reads = {
    run: RUN_HISTORY,
    runTruncated: false,
    loop: loopTriples,
    loopTruncated: false,
  };

  it("explains a finished control from the two reads", () => {
    const ev = deriveControlEvidence(control(), reads);
    expect(ev.fact).toMatchObject({ status: "known", value: { to: "completed", source: "rule" } });
    expect(ev.nextAction).toEqual({ status: "known", value: "observed" });
    expect(ev.reason).toEqual({ status: "known", value: "Emitted two findings." });
    expect(ev.outcome).toEqual({ status: "known", value: "success" });
    expect(ev.description.status).toBe("known");
  });

  it("reports unknown, with the reason, when the spawn precedes every transition", () => {
    const early = control({ spawnedAtNanos: 1791480613000000000n });
    expect(deriveControlEvidence(early, reads).fact).toEqual({
      status: "unknown",
      reason: "no lifecycle transition precedes the spawn",
    });
  });

  it("reports a decide that has not landed yet as not recorded, not as empty", () => {
    const ev = deriveControlEvidence(control(), { ...reads, loop: [] });
    expect(ev.nextAction).toEqual({ status: "unknown", reason: "not recorded yet" });
    expect(ev.reason).toEqual({ status: "unknown", reason: "not recorded yet" });
  });

  it("marks everything derived from a truncated read as unknown: read truncated", () => {
    const ev = deriveControlEvidence(control(), { ...reads, runTruncated: true, loopTruncated: true });
    for (const value of [ev.fact, ev.nextAction, ev.reason, ev.outcome, ev.description]) {
      expect(value).toEqual({ status: "unknown", reason: "read truncated" });
    }
  });

  it("degrades only the fields a failed read feeds", () => {
    const ev = deriveControlEvidence(control(), { ...reads, run: null, runError: "HTTP 500" });
    expect(ev.fact).toEqual({ status: "unknown", reason: "read failed: HTTP 500" });
    expect(ev.nextAction.status).toBe("known");
  });

  it("takes the latest value when a predicate holds several", () => {
    const history: RawTriple[] = [
      { subject: loopEntity(OBSERVER), predicate: "coordinator.decision.next-action", object: "retry", timestamp: "2026-10-08T17:30:13.900000000Z" },
      { subject: loopEntity(OBSERVER), predicate: "coordinator.decision.next-action", object: "observed", timestamp: "2026-10-08T17:30:13.923554963Z" },
    ];
    expect(deriveControlEvidence(control(), { ...reads, loop: history }).nextAction).toEqual({ status: "known", value: "observed" });
  });
});
