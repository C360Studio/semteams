import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { flushSync } from "svelte";
import type { AgentLoop } from "$lib/types/agent";

vi.mock("$lib/services/runStatusApi");

import { getTriples } from "$lib/services/runStatusApi";
import { agentStore } from "./agentStore.svelte";
import { controlsStore } from "./controlsStore.svelte";

const mockGetTriples = vi.mocked(getTriples);

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

beforeEach(() => {
  agentStore.reset();
  controlsStore.stop();
  controlsStore.reset();
  mockGetTriples.mockReset();
  mockGetTriples.mockResolvedValue([]);
});

afterEach(() => {
  controlsStore.stop();
});

// +layout.svelte starts the stores from ONE $effect and tears them all down (SSE
// included) in its cleanup. The first controls tick reads agentStore's loops
// synchronously, so if start() let that read be tracked, every loop update would
// re-run the effect: disconnect, reconnect, replay the stream, update again. The
// board then never settled on more than a loop or two.
describe("controlsStore.start() called from an effect", () => {
  it("does not make the calling effect depend on the loops it reads", () => {
    agentStore.updateLoop(loop("coord-1"));
    let starts = 0;
    let stops = 0;
    const dispose = $effect.root(() => {
      $effect(() => {
        starts++;
        controlsStore.start();
        return () => {
          stops++;
          controlsStore.stop();
        };
      });
    });
    try {
      flushSync();
      expect(starts).toBe(1);

      agentStore.updateLoop(loop("coord-2"));
      agentStore.updateLoop(loop("plan-1", { parent_loop_id: "coord-1" }));
      flushSync();

      expect(starts).toBe(1);
      expect(stops).toBe(0);
    } finally {
      dispose();
    }
  });
});
