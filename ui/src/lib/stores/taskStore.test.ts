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

// The runtime records a rule-spawned loop as agent.loop.task = rule-<firing run entity>-<nanos>.
function controlTriple(controlLoopId: string, runInstance: string): RawTriple {
  return {
    subject: `${PREFIX}.agentic-loop.agent.execution.${controlLoopId}`,
    predicate: "agent.loop.task",
    object: `rule-${PREFIX}.chain.agent.execution.${runInstance}-${NANOS}`,
  };
}

async function learnControls(...triples: RawTriple[]) {
  mockGetTriples.mockResolvedValue(triples);
  await controlsStore.pollOnce();
}

beforeEach(async () => {
  agentStore.reset();
  localStorage.clear();
  controlsStore.stop();
  mockGetTriples.mockReset();
  await learnControls(); // clear controls a previous test learned
});

describe("taskStore — rule-fired controls (design D3)", () => {
  it("does not render the ops observer as a top-level card; it hangs off its run's coordinator", async () => {
    agentStore.updateLoop(loop("coord-1"));
    agentStore.updateLoop(loop("observer-1", { role: "ops-chain-observer", state: "executing" }));
    await learnControls(controlTriple("observer-1", "coord-1"));

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
    agentStore.updateLoop(loop("observer-a", { role: "ops-chain-observer" }));
    agentStore.updateLoop(loop("observer-b", { role: "ops-chain-observer" }));
    await learnControls(
      controlTriple("observer-a", "coord-a"),
      controlTriple("observer-b", "coord-b"),
    );

    const byId = Object.fromEntries(taskStore.tasks.map((t) => [t.id, t.controls.map((c) => c.loopId)]));
    expect(byId).toEqual({ "coord-a": ["observer-a"], "coord-b": ["observer-b"] });
  });

  it("keeps a control off the board until its coordinator appears, then surfaces it", async () => {
    agentStore.updateLoop(loop("observer-1", { role: "ops-chain-observer" }));
    await learnControls(controlTriple("observer-1", "coord-1"));

    // No coordinator yet: the control must not leak onto the board as a card.
    expect(taskStore.tasks).toEqual([]);

    agentStore.updateLoop(loop("coord-1"));
    expect(taskStore.tasks.map((t) => t.id)).toEqual(["coord-1"]);
    expect(taskStore.tasks[0].controls.map((c) => c.loopId)).toEqual(["observer-1"]);
  });

  it("leaves non-control top-level loops exactly as they were", async () => {
    agentStore.updateLoop(loop("coord-1"));
    // Dispatch-spawned (front-door) and chain-child task ids are not controls.
    agentStore.updateLoop(loop("front-door", { state: "executing" }));
    agentStore.updateLoop(loop("child-1", { parent_loop_id: "coord-1", role: "researcher" }));
    await learnControls(
      {
        subject: `${PREFIX}.agentic-loop.agent.execution.front-door`,
        predicate: "agent.loop.task",
        object: "dispatch-9f2c1a",
      },
      {
        subject: `${PREFIX}.agentic-loop.agent.execution.child-1`,
        predicate: "agent.loop.task",
        object: `rule-${PREFIX}.agentic-loop.agent.execution.coord-1-${NANOS}`,
      },
    );

    expect(taskStore.tasks.map((t) => t.id).sort()).toEqual(["coord-1", "front-door"]);
    const coord = taskStore.tasks.find((t) => t.id === "coord-1")!;
    expect(coord.childLoops.map((l) => l.loop_id)).toEqual(["child-1"]);
    expect(coord.controls).toEqual([]);
  });

  it("shows a loop as a card until the graph says it is a control, then folds it away", async () => {
    // A control is only knowable from the graph, which arrives after the loop list.
    agentStore.updateLoop(loop("coord-1"));
    agentStore.updateLoop(loop("observer-1", { role: "ops-chain-observer" }));
    expect(taskStore.tasks.map((t) => t.id).sort()).toEqual(["coord-1", "observer-1"]);

    await learnControls(controlTriple("observer-1", "coord-1"));
    expect(taskStore.tasks.map((t) => t.id)).toEqual(["coord-1"]);
  });

  it("does not let a control change the coordinator's column", async () => {
    agentStore.updateLoop(loop("coord-1", { state: "complete" }));
    agentStore.updateLoop(loop("observer-1", { state: "failed", role: "ops-chain-observer" }));
    await learnControls(controlTriple("observer-1", "coord-1"));

    const [task] = taskStore.tasks;
    expect(task.column).toBe("done");
    expect(task.childLoops).toEqual([]);
    expect(task.controls[0].loop.state).toBe("failed");
  });

  it("passes the truncation flag to every task so control-derived displays can say incomplete", async () => {
    agentStore.updateLoop(loop("coord-1"));
    const full = Array.from({ length: 500 }, (_, i): RawTriple => ({
      subject: `${PREFIX}.agentic-loop.agent.execution.l${i}`,
      predicate: "agent.loop.task",
      object: `dispatch-${i}`,
    }));
    await learnControls(...full);

    expect(taskStore.tasks[0].controlsTruncated).toBe(true);

    await learnControls();
    expect(taskStore.tasks[0].controlsTruncated).toBe(false);
  });
});
