import { describe, it, expect, vi, beforeEach } from "vitest";
import type { AgentLoop } from "$lib/types/agent";
import type { RawTriple } from "$lib/services/runStatusApi";

// taskStore reads the selection from the URL; no test here exercises it.
vi.mock("$app/state", () => ({
  page: { url: new URL("http://localhost/"), state: {} },
}));
vi.mock("$app/navigation", () => ({ replaceState: vi.fn() }));
vi.mock("$lib/services/runStatusApi");

import { getTriples } from "$lib/services/runStatusApi";
import { agentStore } from "./agentStore.svelte";
import { controlsStore } from "./controlsStore.svelte";
import { taskStore } from "./taskStore.svelte";

const mockGetTriples = vi.mocked(getTriples);

const PREFIX = "c360.semteams-bootstrap-e2e-6c50f3";
const NANOS = "1791480613893413505";

function ruleTaskId(runInstance: string): string {
  return `rule-${PREFIX}.chain.agent.execution.${runInstance}-${NANOS}`;
}

function loop(loopId: string, overrides: Partial<AgentLoop> = {}): AgentLoop {
  return {
    loop_id: loopId,
    task_id: "",
    state: "complete",
    role: "coordinator",
    iterations: 1,
    max_iterations: 10,
    user_id: "u",
    channel_type: "web",
    parent_loop_id: "",
    outcome: "",
    error: "",
    ...overrides,
  };
}

/** A loop a rule fired on the run entity of `runInstance`: the shape of both controls and autoresearch work loops. */
function ruleFired(loopId: string, runInstance: string, overrides: Partial<AgentLoop> = {}): AgentLoop {
  return loop(loopId, { task_id: ruleTaskId(runInstance), ...overrides });
}

// What the runtime wrote to the loop entity: a run member carries agent.loop.run, a run_scope: none control does not.
function entityTriples(loopId: string, member: boolean): RawTriple[] {
  const subject = `${PREFIX}.agentic-loop.agent.execution.${loopId}`;
  const predicates = ["agent.loop.task", "agent.loop.role", ...(member ? ["agent.loop.run"] : ["agent.lineage.root"])];
  return predicates.map((predicate) => ({ subject, predicate, object: "x" }));
}

/** Serve loop entities (true = run member, false = control) and classify every candidate. */
async function classify(served: Record<string, boolean>) {
  mockGetTriples.mockImplementation(async (params) => {
    const id = params.subject?.slice(params.subject.lastIndexOf(".") + 1) ?? "";
    return id in served ? entityTriples(id, served[id]) : [];
  });
  await controlsStore.pollOnce();
}

beforeEach(() => {
  agentStore.reset();
  localStorage.clear();
  controlsStore.stop();
  controlsStore.reset();
  mockGetTriples.mockReset();
  mockGetTriples.mockResolvedValue([]);
});

describe("taskStore — rule-fired controls (design D3)", () => {
  it("does not render the ops observer as a top-level card; it hangs off its run's coordinator", async () => {
    agentStore.updateLoop(loop("coord-1"));
    agentStore.updateLoop(ruleFired("observer-1", "coord-1", { role: "ops-chain-observer", state: "executing" }));
    await classify({ "observer-1": false });

    expect(taskStore.tasks.map((t) => t.id)).toEqual(["coord-1"]);
    const [task] = taskStore.tasks;
    expect(task.controls.map((c) => c.loopId)).toEqual(["observer-1"]);
    expect(task.controls[0]).toMatchObject({
      runInstance: "coord-1",
      loop: { loop_id: "observer-1", role: "ops-chain-observer", state: "executing" },
    });
  });

  it("attaches each control to the coordinator whose run fired it", async () => {
    agentStore.updateLoop(loop("coord-a"));
    agentStore.updateLoop(loop("coord-b"));
    agentStore.updateLoop(ruleFired("observer-a", "coord-a", { role: "ops-chain-observer" }));
    agentStore.updateLoop(ruleFired("observer-b", "coord-b", { role: "ops-chain-observer" }));
    await classify({ "observer-a": false, "observer-b": false });

    const byId = Object.fromEntries(taskStore.tasks.map((t) => [t.id, t.controls.map((c) => c.loopId)]));
    expect(byId).toEqual({ "coord-a": ["observer-a"], "coord-b": ["observer-b"] });
  });

  it("keeps a run member that a rule fired on the run entity on the board, with its descendants reachable", async () => {
    // autoresearch/05 fires its propose loop on the run entity: rule-<run entity>-<nanos>,
    // no parent_loop_id. It is the run's work, so it must not be folded into the coordinator
    // (that would hide `execute-1` and freeze the column).
    agentStore.updateLoop(loop("coord-1"));
    agentStore.updateLoop(ruleFired("propose-1", "coord-1", { role: "researcher", state: "complete" }));
    agentStore.updateLoop(
      loop("execute-1", { parent_loop_id: "propose-1", role: "executor", state: "awaiting_approval" }),
    );
    await classify({ "propose-1": true });

    expect(taskStore.tasks.map((t) => t.id).sort()).toEqual(["coord-1", "propose-1"]);
    const coord = taskStore.tasks.find((t) => t.id === "coord-1")!;
    const propose = taskStore.tasks.find((t) => t.id === "propose-1")!;
    expect(coord.controls).toEqual([]);
    expect(propose.childLoops.map((l) => l.loop_id)).toEqual(["execute-1"]);
    // The card follows its live descendant instead of freezing.
    expect(propose.column).toBe("needs_you");
    expect(coord.column).toBe("done");
  });

  it("leaves a candidate whose membership is unknown exactly where it was: a top-level card", async () => {
    agentStore.updateLoop(loop("coord-1"));
    agentStore.updateLoop(ruleFired("propose-1", "coord-1", { role: "researcher" }));
    agentStore.updateLoop(loop("execute-1", { parent_loop_id: "propose-1", role: "executor" }));
    mockGetTriples.mockRejectedValue(new Error("triples endpoint down"));
    await controlsStore.pollOnce();

    expect(taskStore.tasks.map((t) => t.id).sort()).toEqual(["coord-1", "propose-1"]);
    expect(taskStore.tasks.find((t) => t.id === "propose-1")!.childLoops.map((l) => l.loop_id)).toEqual([
      "execute-1",
    ]);
    expect(taskStore.tasks.find((t) => t.id === "coord-1")!.controls).toEqual([]);
  });

  it("keeps a resolved control as a top-level card while its coordinator is absent, then folds it in", async () => {
    agentStore.updateLoop(ruleFired("observer-1", "coord-1", { role: "ops-chain-observer" }));
    await classify({ "observer-1": false });

    // No coordinator yet: nothing to attach to, so it is an ordinary card.
    expect(taskStore.tasks.map((t) => t.id)).toEqual(["observer-1"]);

    agentStore.updateLoop(loop("coord-1"));
    expect(taskStore.tasks.map((t) => t.id)).toEqual(["coord-1"]);
    expect(taskStore.tasks[0].controls.map((c) => c.loopId)).toEqual(["observer-1"]);
  });

  it("attaches a control fired on a nested run to the top-level task that owns it", async () => {
    // research/06 spawns a child coordinator; research/01 (run_scope: new) opens a run
    // whose instance is that child's loop id. The observer fires on that nested run.
    agentStore.updateLoop(loop("coord-1"));
    agentStore.updateLoop(loop("child-coord", { parent_loop_id: "coord-1" }));
    agentStore.updateLoop(loop("worker", { parent_loop_id: "child-coord", role: "researcher" }));
    agentStore.updateLoop(ruleFired("observer-nested", "child-coord", { role: "ops-chain-observer" }));
    await classify({ "observer-nested": false });

    expect(taskStore.tasks.map((t) => t.id)).toEqual(["coord-1"]);
    const [task] = taskStore.tasks;
    expect(task.controls.map((c) => c.loopId)).toEqual(["observer-nested"]);
    expect(task.controls[0].runInstance).toBe("child-coord");
    expect(task.childLoops.map((l) => l.loop_id)).toEqual(["child-coord", "worker"]);
  });

  it("leaves non-candidate top-level loops exactly as they were", async () => {
    agentStore.updateLoop(loop("coord-1"));
    // Dispatch-spawned (front-door) and chain-child task ids are not candidates.
    agentStore.updateLoop(loop("front-door", { state: "executing", task_id: "dispatch-9f2c1a" }));
    agentStore.updateLoop(
      loop("child-1", {
        parent_loop_id: "coord-1",
        role: "researcher",
        task_id: `rule-${PREFIX}.agentic-loop.agent.execution.coord-1-${NANOS}`,
      }),
    );
    await classify({});

    expect(mockGetTriples).not.toHaveBeenCalled();
    expect(taskStore.tasks.map((t) => t.id).sort()).toEqual(["coord-1", "front-door"]);
    const coord = taskStore.tasks.find((t) => t.id === "coord-1")!;
    expect(coord.childLoops.map((l) => l.loop_id)).toEqual(["child-1"]);
    expect(coord.controls).toEqual([]);
  });

  it("shows a loop as a card until the graph says it is a control, then folds it away", async () => {
    // Membership is only knowable from the graph, which arrives after the loop list.
    agentStore.updateLoop(loop("coord-1"));
    agentStore.updateLoop(ruleFired("observer-1", "coord-1", { role: "ops-chain-observer" }));
    expect(taskStore.tasks.map((t) => t.id).sort()).toEqual(["coord-1", "observer-1"]);

    await classify({ "observer-1": false });
    expect(taskStore.tasks.map((t) => t.id)).toEqual(["coord-1"]);
  });

  it("does not let a control change the coordinator's column", async () => {
    agentStore.updateLoop(loop("coord-1", { state: "complete" }));
    agentStore.updateLoop(ruleFired("observer-1", "coord-1", { state: "failed", role: "ops-chain-observer" }));
    await classify({ "observer-1": false });

    const [task] = taskStore.tasks;
    expect(task.column).toBe("done");
    expect(task.childLoops).toEqual([]);
    expect(task.controls[0].loop.state).toBe("failed");
  });

  it("marks the owning task incomplete while a candidate on its run cannot be classified, and clears it on recovery", async () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    try {
      agentStore.updateLoop(loop("coord-1"));
      agentStore.updateLoop(ruleFired("observer-1", "coord-1", { role: "ops-chain-observer" }));
      mockGetTriples.mockRejectedValue(new Error("triples endpoint down"));
      await controlsStore.pollOnce();

      const coord = () => taskStore.tasks.find((t) => t.id === "coord-1")!;
      expect(coord().controlsTruncated).toBe(true);
      expect(taskStore.tasks.find((t) => t.id === "observer-1")).toBeDefined();

      vi.setSystemTime(Date.now() + 10_000);
      await classify({ "observer-1": false });
      expect(coord().controlsTruncated).toBe(false);
      expect(coord().controls.map((c) => c.loopId)).toEqual(["observer-1"]);
    } finally {
      vi.useRealTimers();
    }
  });
});

describe("taskStore — which loops are worth a #ref", () => {
  it("withholds a ref from a control that folded into its run's card, and from a candidate still being classified", async () => {
    agentStore.updateLoop(loop("coord-1"));
    const observer = ruleFired("observer-1", "coord-1", { role: "ops-chain-observer" });
    agentStore.updateLoop(observer);

    // Not classified yet: it may fold away, so it does not spend a number.
    expect(taskStore.isRefEligible(observer)).toBe(false);

    await classify({ "observer-1": false });
    expect(taskStore.tasks.map((t) => t.id)).toEqual(["coord-1"]);
    expect(taskStore.isRefEligible(observer)).toBe(false);
    expect(taskStore.isRefEligible(loop("coord-1"))).toBe(true);
  });

  it("gives a ref to a resolved control that stays a card because its run's loop is not on the board", async () => {
    const observer = ruleFired("observer-1", "coord-1", { role: "ops-chain-observer" });
    agentStore.updateLoop(observer);
    await classify({ "observer-1": false });

    expect(taskStore.tasks.map((t) => t.id)).toEqual(["observer-1"]);
    expect(taskStore.isRefEligible(observer)).toBe(true);
  });

  it("gives a ref to a resolved control that stays a card because it has children of its own", async () => {
    agentStore.updateLoop(loop("coord-1"));
    const observer = ruleFired("observer-1", "coord-1", { role: "ops-chain-observer" });
    agentStore.updateLoop(observer);
    agentStore.updateLoop(loop("sub-1", { parent_loop_id: "observer-1", role: "researcher" }));
    await classify({ "observer-1": false });

    expect(taskStore.tasks.map((t) => t.id).sort()).toEqual(["coord-1", "observer-1"]);
    expect(taskStore.isRefEligible(observer)).toBe(true);
  });

  it("gives a ref to a run member and to a candidate whose classification was given up on", async () => {
    agentStore.updateLoop(loop("coord-1"));
    const member = ruleFired("propose-1", "coord-1", { role: "researcher" });
    agentStore.updateLoop(member);
    await classify({ "propose-1": true });
    expect(taskStore.isRefEligible(member)).toBe(true);

    const lost = ruleFired("lost-1", "coord-1", { role: "ops-chain-observer" });
    agentStore.updateLoop(lost);
    mockGetTriples.mockRejectedValue(new Error("triples endpoint down"));
    await controlsStore.pollOnce();
    expect(taskStore.isRefEligible(lost)).toBe(true);
  });

  it("never gives a ref to a sub-task", () => {
    expect(taskStore.isRefEligible(loop("child-1", { parent_loop_id: "coord-1" }))).toBe(false);
  });
});
