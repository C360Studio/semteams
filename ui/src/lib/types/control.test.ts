import { describe, it, expect } from "vitest";
import type { RawTriple } from "$lib/services/runStatusApi";
import type { AgentLoop } from "./agent";
import {
  buildControlsSnapshot,
  controlOutcome,
  controlsForRun,
  controlsSignature,
  deriveControlEvidence,
  groupLifecycleTransitions,
  parseControlTriple,
  parseRfc3339Nanos,
  splitControlLoops,
  transitionPrecedingSpawn,
  type Control,
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

function taskTriple(loopId: string, object: string): RawTriple {
  return { subject: loopEntity(loopId), predicate: "agent.loop.task", object };
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

describe("parseControlTriple", () => {
  it("derives the control from a rule-spawned loop on a run entity", () => {
    const parsed = parseControlTriple(taskTriple(OBSERVER, `rule-${RUN_ENTITY}-${NANOS}`));
    expect(parsed).toMatchObject({
      loopId: OBSERVER,
      loopEntityId: loopEntity(OBSERVER),
      firingEntityId: RUN_ENTITY,
      runInstance: RUN,
    });
  });

  it("converts the nanosecond spawn time to a Date and keeps full precision", () => {
    const parsed = parseControlTriple(taskTriple(OBSERVER, `rule-${RUN_ENTITY}-${NANOS}`))!;
    expect(parsed.spawnedAt.toISOString()).toBe("2026-10-08T17:30:13.893Z");
    // 19 digits overflow a double; the exact value must survive.
    expect(parsed.spawnedAtNanos).toBe(1791480613893413505n);
  });

  it("ignores dispatch-spawned loops", () => {
    expect(parseControlTriple(taskTriple("loop-1", "dispatch-9f2c1a"))).toBeNull();
  });

  it("ignores chain children whose firing entity is a loop (already attached by parent_loop_id)", () => {
    const firingLoop = loopEntity("ce821ec0-b64b-4626-9fe1-0a027b997dbb");
    expect(parseControlTriple(taskTriple("child-1", `rule-${firingLoop}-${NANOS}`))).toBeNull();
  });

  it("ignores a rule task id that does not end in nanos", () => {
    expect(parseControlTriple(taskTriple(OBSERVER, `rule-${RUN_ENTITY}`))).toBeNull();
  });

  it("ignores other predicates and non-loop subjects", () => {
    const other: RawTriple = { ...taskTriple(OBSERVER, `rule-${RUN_ENTITY}-${NANOS}`), predicate: "agent.loop.role" };
    expect(parseControlTriple(other)).toBeNull();
    const nonLoop: RawTriple = { subject: RUN_ENTITY, predicate: "agent.loop.task", object: `rule-${RUN_ENTITY}-${NANOS}` };
    expect(parseControlTriple(nonLoop)).toBeNull();
  });
});

describe("buildControlsSnapshot", () => {
  const triples = [
    taskTriple(OBSERVER, `rule-${RUN_ENTITY}-${NANOS}`),
    taskTriple("dispatch-loop", "dispatch-abc"),
    taskTriple("chain-child", `rule-${loopEntity("parent")}-${NANOS}`),
  ];

  it("indexes controls by run instance and lists their loop ids", () => {
    const snapshot = buildControlsSnapshot(triples, 500);
    expect(Object.keys(snapshot.controlsByRun)).toEqual([RUN]);
    expect(snapshot.controlsByRun[RUN].map((c) => c.loopId)).toEqual([OBSERVER]);
    expect([...snapshot.controlLoopIds]).toEqual([OBSERVER]);
    expect(snapshot.truncated).toBe(false);
  });

  it("flags a read that filled its limit as truncated", () => {
    expect(buildControlsSnapshot(triples, 3).truncated).toBe(true);
    expect(buildControlsSnapshot(triples, 4).truncated).toBe(false);
  });

  it("orders a run's controls by spawn time", () => {
    const later = taskTriple("late", `rule-${RUN_ENTITY}-${BigInt(NANOS) + 1000n}`);
    const earlier = taskTriple("early", `rule-${RUN_ENTITY}-${BigInt(NANOS) - 1000n}`);
    const snapshot = buildControlsSnapshot([later, earlier], 500);
    expect(snapshot.controlsByRun[RUN].map((c) => c.loopId)).toEqual(["early", "late"]);
  });

  it("gives an unchanged read an unchanged signature, and a changed one a different signature", () => {
    const a = buildControlsSnapshot(triples, 500);
    const b = buildControlsSnapshot([...triples].reverse(), 500);
    expect(controlsSignature(a)).toBe(controlsSignature(b));
    expect(controlsSignature(a)).not.toBe(controlsSignature(buildControlsSnapshot(triples, 3)));
    expect(controlsSignature(a)).not.toBe(controlsSignature(buildControlsSnapshot([], 500)));
  });
});

describe("splitControlLoops / controlsForRun", () => {
  const coordinator = loop(RUN, { role: "coordinator" });
  const observer = loop(OBSERVER);
  const child = loop("child", { parent_loop_id: RUN });

  it("withholds known controls from the top level and keeps everything else", () => {
    const { topLevel, controlLoops } = splitControlLoops(
      [coordinator, observer, child],
      new Set([OBSERVER]),
    );
    expect(topLevel.map((l) => l.loop_id)).toEqual([RUN]);
    expect(controlLoops.map((l) => l.loop_id)).toEqual([OBSERVER]);
  });

  it("leaves a control that already has a parent alone", () => {
    const parented = loop(OBSERVER, { parent_loop_id: RUN });
    const { topLevel, controlLoops } = splitControlLoops([coordinator, parented], new Set([OBSERVER]));
    expect(topLevel.map((l) => l.loop_id)).toEqual([RUN]);
    expect(controlLoops).toEqual([]);
  });

  it("attaches live loops to the run's controls and omits controls with no live loop", () => {
    const byRun = { [RUN]: [control(), control({ loopId: "ghost", loopEntityId: loopEntity("ghost") })] };
    const attached = controlsForRun(RUN, byRun, [observer]);
    expect(attached.map((c) => c.loopId)).toEqual([OBSERVER]);
    expect(attached[0].loop).toBe(observer);
    expect(controlsForRun("other-run", byRun, [observer])).toEqual([]);
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
