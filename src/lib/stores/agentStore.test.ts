import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import type { AgentLoop } from "$lib/types/agent";

// ---------------------------------------------------------------------------
// Mock EventSource
// ---------------------------------------------------------------------------

type EventSourceListener = (event: MessageEvent) => void;

class MockEventSource {
  static instances: MockEventSource[] = [];

  url: string;
  readyState = 0;
  onerror: ((event: Event) => void) | null = null;

  private listeners: Map<string, EventSourceListener[]> = new Map();

  constructor(url: string) {
    this.url = url;
    MockEventSource.instances.push(this);
  }

  addEventListener(type: string, listener: EventSourceListener) {
    if (!this.listeners.has(type)) {
      this.listeners.set(type, []);
    }
    this.listeners.get(type)!.push(listener);
  }

  removeEventListener(type: string, listener: EventSourceListener) {
    const list = this.listeners.get(type);
    if (list) {
      const idx = list.indexOf(listener);
      if (idx >= 0) list.splice(idx, 1);
    }
  }

  close() {
    this.readyState = 2;
  }

  // Test helper: simulate a named SSE event
  simulateEvent(type: string, data: unknown) {
    const event = new MessageEvent(type, {
      data: typeof data === "string" ? data : JSON.stringify(data),
    });
    const list = this.listeners.get(type) || [];
    for (const listener of list) {
      listener(event);
    }
  }

  // Test helper: simulate an error
  simulateError() {
    if (this.onerror) {
      this.onerror(new Event("error"));
    }
  }
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("agentStore", () => {
  let agentStore: typeof import("$lib/stores/agentStore.svelte").agentStore;

  const originalEventSource = globalThis.EventSource;

  beforeEach(async () => {
    MockEventSource.instances = [];
    // @ts-expect-error - Replacing EventSource with mock
    globalThis.EventSource = MockEventSource;
    vi.useFakeTimers();

    // Dynamic import to get a fresh module for each test
    vi.resetModules();
    const mod = await import("./agentStore.svelte");
    agentStore = mod.agentStore;
  });

  afterEach(() => {
    agentStore.reset();
    globalThis.EventSource = originalEventSource;
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  // Helper to create a mock loop
  const createMockLoop = (overrides?: Partial<AgentLoop>): AgentLoop => ({
    loop_id: "loop-1",
    task_id: "task-1",
    state: "exploring",
    role: "architect",
    iterations: 1,
    max_iterations: 10,
    user_id: "user-1",
    channel_type: "chat",
    parent_loop_id: "",
    outcome: "",
    error: "",
    ...overrides,
  });

  // =========================================================================
  // connect / disconnect
  // =========================================================================

  describe("connect", () => {
    it("should create an EventSource with the correct URL", () => {
      agentStore.connect();

      expect(MockEventSource.instances).toHaveLength(1);
      expect(MockEventSource.instances[0].url).toBe(
        "/agentic-dispatch/activity",
      );
    });

    it("should be idempotent — calling twice does not create two EventSources", () => {
      agentStore.connect();
      agentStore.connect();

      expect(MockEventSource.instances).toHaveLength(1);
    });
  });

  describe("disconnect", () => {
    it("should close the EventSource", () => {
      agentStore.connect();
      const es = MockEventSource.instances[0];

      agentStore.disconnect();

      expect(es.readyState).toBe(2); // CLOSED
    });

    it("should set connected to false", () => {
      agentStore.connect();
      MockEventSource.instances[0].simulateEvent("connected", {});
      expect(agentStore.connected).toBe(true);

      agentStore.disconnect();
      expect(agentStore.connected).toBe(false);
    });

    it("should be safe to call when not connected", () => {
      expect(() => agentStore.disconnect()).not.toThrow();
    });
  });

  // =========================================================================
  // SSE event handling
  // =========================================================================

  describe("SSE events", () => {
    it("should set connected=true on 'connected' event", () => {
      agentStore.connect();
      const es = MockEventSource.instances[0];

      expect(agentStore.connected).toBe(false);

      es.simulateEvent("connected", {});

      expect(agentStore.connected).toBe(true);
      expect(agentStore.error).toBeNull();
    });

    it("should populate loops on 'sync_complete' event", () => {
      agentStore.connect();
      const es = MockEventSource.instances[0];
      const loops = [
        createMockLoop({ loop_id: "loop-1" }),
        createMockLoop({ loop_id: "loop-2", role: "builder" }),
      ];

      es.simulateEvent("sync_complete", loops);

      expect(agentStore.loops.size).toBe(2);
      expect(agentStore.getLoop("loop-1")?.role).toBe("architect");
      expect(agentStore.getLoop("loop-2")?.role).toBe("builder");
    });

    it("should update a loop on 'loop_update' event", () => {
      agentStore.connect();
      const es = MockEventSource.instances[0];
      const loop = createMockLoop();

      es.simulateEvent("loop_update", loop);

      expect(agentStore.loops.size).toBe(1);
      expect(agentStore.getLoop("loop-1")).toEqual(loop);
    });

    it("should update existing loop on repeated 'loop_update' events", () => {
      agentStore.connect();
      const es = MockEventSource.instances[0];

      es.simulateEvent("loop_update", createMockLoop({ iterations: 1 }));
      expect(agentStore.getLoop("loop-1")?.iterations).toBe(1);

      es.simulateEvent("loop_update", createMockLoop({ iterations: 3 }));
      expect(agentStore.getLoop("loop-1")?.iterations).toBe(3);
    });

    it("should ignore malformed loop_update data", () => {
      agentStore.connect();
      const es = MockEventSource.instances[0];

      // Send invalid JSON string
      const event = new MessageEvent("loop_update", {
        data: "not valid json!!!",
      });
      const listeners = (
        es as unknown as { listeners: Map<string, EventSourceListener[]> }
      ).listeners;
      const list = listeners?.get("loop_update") || [];
      for (const l of list) {
        l(event);
      }

      expect(agentStore.loops.size).toBe(0);
    });

    it("should ignore malformed sync_complete data", () => {
      agentStore.connect();
      const es = MockEventSource.instances[0];

      // Send non-array JSON
      es.simulateEvent("sync_complete", { not: "an array" });

      expect(agentStore.loops.size).toBe(0);
    });
  });

  // =========================================================================
  // SSE error + reconnect
  // =========================================================================

  describe("SSE error handling", () => {
    it("should set connected=false on error", () => {
      agentStore.connect();
      const es = MockEventSource.instances[0];
      es.simulateEvent("connected", {});
      expect(agentStore.connected).toBe(true);

      es.simulateError();

      expect(agentStore.connected).toBe(false);
    });

    it("should attempt reconnection with exponential backoff", () => {
      agentStore.connect();
      const es = MockEventSource.instances[0];

      // Trigger error
      es.simulateError();

      // No new EventSource yet (waiting for timer)
      expect(MockEventSource.instances).toHaveLength(1);

      // Advance past first reconnect delay (1000ms)
      vi.advanceTimersByTime(1000);

      // Now a second EventSource should have been created
      expect(MockEventSource.instances).toHaveLength(2);
      expect(MockEventSource.instances[1].url).toBe(
        "/agentic-dispatch/activity",
      );
    });

    it("should reset reconnect attempts on successful connection", () => {
      agentStore.connect();
      const es1 = MockEventSource.instances[0];

      // Trigger error and reconnect
      es1.simulateError();
      vi.advanceTimersByTime(1000);

      // Second EventSource connects successfully
      const es2 = MockEventSource.instances[1];
      es2.simulateEvent("connected", {});

      expect(agentStore.connected).toBe(true);

      // Trigger another error — should use base delay (1000ms) again, not 2000ms
      es2.simulateError();
      vi.advanceTimersByTime(1000);
      expect(MockEventSource.instances).toHaveLength(3);
    });

    it("should stop reconnecting after max attempts", () => {
      agentStore.connect();

      // Exhaust all 5 reconnect attempts
      for (let i = 0; i < 5; i++) {
        const es =
          MockEventSource.instances[MockEventSource.instances.length - 1];
        es.simulateError();
        const delay = 1000 * Math.pow(2, i);
        vi.advanceTimersByTime(delay);
      }

      // One more error — should NOT create another EventSource
      const lastEs =
        MockEventSource.instances[MockEventSource.instances.length - 1];
      lastEs.simulateError();
      vi.advanceTimersByTime(100000);

      // Should have: 1 original + 5 reconnects = 6 total
      expect(MockEventSource.instances).toHaveLength(6);
      expect(agentStore.error).toContain("Failed to connect after 5 attempts");
    });

    it("should cancel pending reconnect on disconnect", () => {
      agentStore.connect();
      const es = MockEventSource.instances[0];

      es.simulateError();
      // Reconnect is scheduled but not fired yet

      agentStore.disconnect();

      // Advance past the reconnect delay
      vi.advanceTimersByTime(5000);

      // Should NOT have created a new EventSource
      expect(MockEventSource.instances).toHaveLength(1);
    });
  });

  // =========================================================================
  // Loop tracking: updateLoop, removeLoop
  // =========================================================================

  describe("loop tracking", () => {
    it("should add a loop via updateLoop", () => {
      const loop = createMockLoop();
      agentStore.updateLoop(loop);

      expect(agentStore.loops.size).toBe(1);
      expect(agentStore.getLoop("loop-1")).toEqual(loop);
    });

    it("should update an existing loop via updateLoop", () => {
      agentStore.updateLoop(createMockLoop({ iterations: 1 }));
      agentStore.updateLoop(createMockLoop({ iterations: 5 }));

      expect(agentStore.loops.size).toBe(1);
      expect(agentStore.getLoop("loop-1")?.iterations).toBe(5);
    });

    it("should remove a loop via removeLoop", () => {
      agentStore.updateLoop(createMockLoop());
      expect(agentStore.loops.size).toBe(1);

      agentStore.removeLoop("loop-1");
      expect(agentStore.loops.size).toBe(0);
      expect(agentStore.getLoop("loop-1")).toBeUndefined();
    });

    it("should handle removing nonexistent loop without error", () => {
      expect(() => agentStore.removeLoop("nonexistent")).not.toThrow();
    });
  });

  // =========================================================================
  // Derived getters
  // =========================================================================

  describe("derived getters", () => {
    it("loopsList returns all loops as an array", () => {
      agentStore.updateLoop(createMockLoop({ loop_id: "loop-1" }));
      agentStore.updateLoop(
        createMockLoop({ loop_id: "loop-2", role: "builder" }),
      );

      expect(agentStore.loopsList).toHaveLength(2);
    });

    it("activeLoops returns only loops in active states", () => {
      agentStore.updateLoop(
        createMockLoop({ loop_id: "a", state: "exploring" }),
      );
      agentStore.updateLoop(
        createMockLoop({ loop_id: "b", state: "planning" }),
      );
      agentStore.updateLoop(
        createMockLoop({ loop_id: "c", state: "complete" }),
      );
      agentStore.updateLoop(createMockLoop({ loop_id: "d", state: "failed" }));
      agentStore.updateLoop(
        createMockLoop({ loop_id: "e", state: "executing" }),
      );

      expect(agentStore.activeLoops).toHaveLength(3);
      expect(agentStore.activeLoops.map((l) => l.loop_id).sort()).toEqual([
        "a",
        "b",
        "e",
      ]);
    });

    it("awaitingApproval returns only loops in awaiting_approval state", () => {
      agentStore.updateLoop(
        createMockLoop({ loop_id: "a", state: "exploring" }),
      );
      agentStore.updateLoop(
        createMockLoop({ loop_id: "b", state: "awaiting_approval" }),
      );
      agentStore.updateLoop(
        createMockLoop({ loop_id: "c", state: "awaiting_approval" }),
      );

      expect(agentStore.awaitingApproval).toHaveLength(2);
      expect(agentStore.awaitingApproval.map((l) => l.loop_id).sort()).toEqual([
        "b",
        "c",
      ]);
    });

    it("getLoop returns undefined for unknown loop", () => {
      expect(agentStore.getLoop("unknown")).toBeUndefined();
    });
  });

  // =========================================================================
  // reset
  // =========================================================================

  describe("reset", () => {
    it("should clear all state and disconnect", () => {
      agentStore.connect();
      const es = MockEventSource.instances[0];
      es.simulateEvent("connected", {});
      agentStore.updateLoop(createMockLoop());

      agentStore.reset();

      expect(agentStore.connected).toBe(false);
      expect(agentStore.error).toBeNull();
      expect(agentStore.loops.size).toBe(0);
      expect(agentStore.loopsList).toHaveLength(0);
      expect(es.readyState).toBe(2); // CLOSED
    });

    it("should allow reconnecting after reset", () => {
      agentStore.connect();
      agentStore.reset();
      agentStore.connect();

      expect(MockEventSource.instances).toHaveLength(2);
    });
  });
});
