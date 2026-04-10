/**
 * Flow History Store Test Suite
 *
 * Tests for flow history management store.
 * Implements undo/redo functionality for flow canvas operations.
 *
 * Following TDD principles:
 * - Test push operation (add flow state to history)
 * - Test pop/undo operation (retrieve previous state)
 * - Test stack size limit enforcement
 * - Test clear operation
 * - Test edge cases and error conditions
 */

import { describe, it, expect, beforeEach } from "vitest";
import type { Flow } from "$lib/types/flow";
import { createFlowHistoryStore } from "$lib/stores/flowHistory.svelte";

/**
 * Flow history store interface
 *
 * Manages undo/redo history for flow canvas operations.
 * Stores snapshots of flow state with configurable stack size limit.
 */
interface FlowHistoryStore {
  /** Current history stack */
  history: Flow[];

  /** Current position in history (for redo support) */
  currentIndex: number;

  /** Maximum history stack size */
  maxSize: number;

  /** Push new flow state to history */
  push: (flow: Flow) => void;

  /** Pop/undo to previous state */
  pop: () => Flow | undefined;

  /** Redo to next state (if available) */
  redo: () => Flow | undefined;

  /** Get current flow state */
  getCurrent: () => Flow | undefined;

  /** Check if undo is available */
  canUndo: () => boolean;

  /** Check if redo is available */
  canRedo: () => boolean;

  /** Clear history */
  clear: () => void;

  /** Get history size */
  size: () => number;
}

/**
 * Note: Mock implementation removed - now using actual implementation
 * from $lib/stores/flowHistory.svelte imported above
 */

// Helper function to create test flow
function createTestFlow(id: string, name: string, nodeCount = 0): Flow {
  const nodes = Array.from({ length: nodeCount }, (_, i) => ({
    id: `node-${i + 1}`,
    component: "test-component",
    type: "processor",
    name: `Node ${i + 1}`,
    position: { x: 100 * (i + 1), y: 100 },
    config: {},
  }));

  return {
    id,
    name,
    version: 1,
    nodes,
    connections: [],
    runtime_state: "not_deployed",
    created_at: "2025-01-01T00:00:00Z",
    updated_at: "2025-01-01T00:00:00Z",
    last_modified: "2025-01-01T00:00:00Z",
  };
}

describe("Flow History Store - Initialization", () => {
  it("should create empty history store", () => {
    const store = createFlowHistoryStore();

    expect(store.history).toEqual([]);
    expect(store.currentIndex).toBe(-1);
    expect(store.size()).toBe(0);
  });

  it("should use default max size of 10", () => {
    const store = createFlowHistoryStore();

    expect(store.maxSize).toBe(10);
  });

  it("should accept custom max size", () => {
    const store = createFlowHistoryStore(20);

    expect(store.maxSize).toBe(20);
  });

  it("should start with no undo/redo available", () => {
    const store = createFlowHistoryStore();

    expect(store.canUndo()).toBe(false);
    expect(store.canRedo()).toBe(false);
  });

  it("should return undefined for getCurrent on empty store", () => {
    const store = createFlowHistoryStore();

    expect(store.getCurrent()).toBeUndefined();
  });
});

describe("Flow History Store - Push Operation", () => {
  let store: FlowHistoryStore;

  beforeEach(() => {
    store = createFlowHistoryStore();
  });

  describe("add flow states", () => {
    it("should add flow state to history", () => {
      const flow = createTestFlow("flow-1", "Test Flow");

      store.push(flow);

      expect(store.size()).toBe(1);
      expect(store.history[0]).toEqual(flow);
    });

    it("should update current index when pushing", () => {
      const flow1 = createTestFlow("flow-1", "Flow 1");

      store.push(flow1);

      expect(store.currentIndex).toBe(0);
    });

    it("should add multiple flow states", () => {
      const flow1 = createTestFlow("flow-1", "Flow 1");
      const flow2 = createTestFlow("flow-2", "Flow 2");
      const flow3 = createTestFlow("flow-3", "Flow 3");

      store.push(flow1);
      store.push(flow2);
      store.push(flow3);

      expect(store.size()).toBe(3);
      expect(store.currentIndex).toBe(2);
    });

    it("should store flow snapshots independently", () => {
      const flow1 = createTestFlow("flow-1", "Flow 1", 1);
      const flow2 = createTestFlow("flow-1", "Flow 1", 2);

      store.push(flow1);
      store.push(flow2);

      expect(store.history[0].nodes).toHaveLength(1);
      expect(store.history[1].nodes).toHaveLength(2);
    });

    it("should enable undo after first push", () => {
      const flow1 = createTestFlow("flow-1", "Flow 1");
      const flow2 = createTestFlow("flow-2", "Flow 2");

      store.push(flow1);
      expect(store.canUndo()).toBe(false); // No previous state

      store.push(flow2);
      expect(store.canUndo()).toBe(true); // Can undo to flow1
    });
  });

  describe("history branching", () => {
    it("should remove forward history when pushing after undo", () => {
      const flow1 = createTestFlow("flow-1", "Flow 1");
      const flow2 = createTestFlow("flow-2", "Flow 2");
      const flow3 = createTestFlow("flow-3", "Flow 3");

      store.push(flow1);
      store.push(flow2);
      store.push(flow3);

      // Undo twice
      store.pop();
      store.pop();

      expect(store.currentIndex).toBe(0);
      expect(store.size()).toBe(3);

      // Push new state - should remove flow2 and flow3
      const flow4 = createTestFlow("flow-4", "Flow 4");
      store.push(flow4);

      expect(store.size()).toBe(2);
      expect(store.history[0]).toEqual(flow1);
      expect(store.history[1]).toEqual(flow4);
      expect(store.canRedo()).toBe(false);
    });

    it("should create new branch from middle of history", () => {
      const flows = [
        createTestFlow("flow-1", "Flow 1"),
        createTestFlow("flow-2", "Flow 2"),
        createTestFlow("flow-3", "Flow 3"),
        createTestFlow("flow-4", "Flow 4"),
      ];

      flows.forEach((f) => store.push(f));

      // Undo to middle
      store.pop();
      store.pop();

      // Create new branch
      const branchFlow = createTestFlow("flow-branch", "Branch Flow");
      store.push(branchFlow);

      expect(store.size()).toBe(3);
      expect(store.history[2].id).toBe("flow-branch");
    });
  });
});

describe("Flow History Store - Pop/Undo Operation", () => {
  let store: FlowHistoryStore;

  beforeEach(() => {
    store = createFlowHistoryStore();
  });

  describe("retrieve previous states", () => {
    it("should return previous flow state", () => {
      const flow1 = createTestFlow("flow-1", "Flow 1");
      const flow2 = createTestFlow("flow-2", "Flow 2");

      store.push(flow1);
      store.push(flow2);

      const previous = store.pop();

      expect(previous).toEqual(flow1);
    });

    it("should update current index when popping", () => {
      const flow1 = createTestFlow("flow-1", "Flow 1");
      const flow2 = createTestFlow("flow-2", "Flow 2");

      store.push(flow1);
      store.push(flow2);

      store.pop();

      expect(store.currentIndex).toBe(0);
    });

    it("should allow multiple pops", () => {
      const flow1 = createTestFlow("flow-1", "Flow 1");
      const flow2 = createTestFlow("flow-2", "Flow 2");
      const flow3 = createTestFlow("flow-3", "Flow 3");

      store.push(flow1);
      store.push(flow2);
      store.push(flow3);

      const state2 = store.pop();
      const state1 = store.pop();

      expect(state2).toEqual(flow2);
      expect(state1).toEqual(flow1);
      expect(store.currentIndex).toBe(0);
    });

    it("should return undefined when at start of history", () => {
      const flow1 = createTestFlow("flow-1", "Flow 1");

      store.push(flow1);

      const result = store.pop();

      expect(result).toBeUndefined();
      expect(store.currentIndex).toBe(0);
    });

    it("should return undefined on empty history", () => {
      const result = store.pop();

      expect(result).toBeUndefined();
    });

    it("should update canUndo after popping", () => {
      const flow1 = createTestFlow("flow-1", "Flow 1");
      const flow2 = createTestFlow("flow-2", "Flow 2");
      const flow3 = createTestFlow("flow-3", "Flow 3");

      store.push(flow1);
      store.push(flow2);
      store.push(flow3);

      expect(store.canUndo()).toBe(true);

      store.pop();
      expect(store.canUndo()).toBe(true);

      store.pop();
      expect(store.canUndo()).toBe(false);
    });
  });
});

describe("Flow History Store - Redo Operation", () => {
  let store: FlowHistoryStore;

  beforeEach(() => {
    store = createFlowHistoryStore();
  });

  describe("redo after undo", () => {
    it("should redo to next state", () => {
      const flow1 = createTestFlow("flow-1", "Flow 1");
      const flow2 = createTestFlow("flow-2", "Flow 2");

      store.push(flow1);
      store.push(flow2);

      store.pop(); // Back to flow1
      const redone = store.redo(); // Forward to flow2

      expect(redone).toEqual(flow2);
      expect(store.currentIndex).toBe(1);
    });

    it("should enable redo after undo", () => {
      const flow1 = createTestFlow("flow-1", "Flow 1");
      const flow2 = createTestFlow("flow-2", "Flow 2");

      store.push(flow1);
      store.push(flow2);

      expect(store.canRedo()).toBe(false);

      store.pop();
      expect(store.canRedo()).toBe(true);
    });

    it("should return undefined when at end of history", () => {
      const flow1 = createTestFlow("flow-1", "Flow 1");

      store.push(flow1);

      const result = store.redo();

      expect(result).toBeUndefined();
    });

    it("should allow multiple redos", () => {
      const flow1 = createTestFlow("flow-1", "Flow 1");
      const flow2 = createTestFlow("flow-2", "Flow 2");
      const flow3 = createTestFlow("flow-3", "Flow 3");

      store.push(flow1);
      store.push(flow2);
      store.push(flow3);

      store.pop();
      store.pop();

      const redo1 = store.redo();
      const redo2 = store.redo();

      expect(redo1).toEqual(flow2);
      expect(redo2).toEqual(flow3);
      expect(store.canRedo()).toBe(false);
    });
  });
});

describe("Flow History Store - Stack Size Limit", () => {
  describe("enforce maximum size", () => {
    it("should limit history to max size", () => {
      const store = createFlowHistoryStore(3);

      const flow1 = createTestFlow("flow-1", "Flow 1");
      const flow2 = createTestFlow("flow-2", "Flow 2");
      const flow3 = createTestFlow("flow-3", "Flow 3");
      const flow4 = createTestFlow("flow-4", "Flow 4");

      store.push(flow1);
      store.push(flow2);
      store.push(flow3);
      store.push(flow4);

      expect(store.size()).toBe(3);
    });

    it("should keep most recent states when exceeding limit", () => {
      const store = createFlowHistoryStore(3);

      const flow1 = createTestFlow("flow-1", "Flow 1");
      const flow2 = createTestFlow("flow-2", "Flow 2");
      const flow3 = createTestFlow("flow-3", "Flow 3");
      const flow4 = createTestFlow("flow-4", "Flow 4");

      store.push(flow1);
      store.push(flow2);
      store.push(flow3);
      store.push(flow4);

      // Should have flow2, flow3, flow4 (flow1 removed)
      expect(store.history[0]).toEqual(flow2);
      expect(store.history[1]).toEqual(flow3);
      expect(store.history[2]).toEqual(flow4);
    });

    it("should adjust current index when removing old states", () => {
      const store = createFlowHistoryStore(3);

      store.push(createTestFlow("flow-1", "Flow 1"));
      store.push(createTestFlow("flow-2", "Flow 2"));
      store.push(createTestFlow("flow-3", "Flow 3"));

      expect(store.currentIndex).toBe(2);

      store.push(createTestFlow("flow-4", "Flow 4"));

      expect(store.currentIndex).toBe(2); // Adjusted
      expect(store.getCurrent()?.id).toBe("flow-4");
    });

    it("should handle max size of 1", () => {
      const store = createFlowHistoryStore(1);

      const flow1 = createTestFlow("flow-1", "Flow 1");
      const flow2 = createTestFlow("flow-2", "Flow 2");

      store.push(flow1);
      store.push(flow2);

      expect(store.size()).toBe(1);
      expect(store.history[0]).toEqual(flow2);
    });

    it("should maintain functionality with large max size", () => {
      const store = createFlowHistoryStore(100);

      for (let i = 0; i < 50; i++) {
        store.push(createTestFlow(`flow-${i}`, `Flow ${i}`));
      }

      expect(store.size()).toBe(50);
      expect(store.currentIndex).toBe(49);
    });
  });
});

describe("Flow History Store - Clear Operation", () => {
  let store: FlowHistoryStore;

  beforeEach(() => {
    store = createFlowHistoryStore();

    // Add some history
    store.push(createTestFlow("flow-1", "Flow 1"));
    store.push(createTestFlow("flow-2", "Flow 2"));
    store.push(createTestFlow("flow-3", "Flow 3"));
  });

  it("should clear all history", () => {
    store.clear();

    expect(store.size()).toBe(0);
    expect(store.history).toEqual([]);
  });

  it("should reset current index", () => {
    expect(store.currentIndex).toBe(2);

    store.clear();

    expect(store.currentIndex).toBe(-1);
  });

  it("should disable undo after clear", () => {
    expect(store.canUndo()).toBe(true);

    store.clear();

    expect(store.canUndo()).toBe(false);
  });

  it("should disable redo after clear", () => {
    store.pop(); // Enable redo
    expect(store.canRedo()).toBe(true);

    store.clear();

    expect(store.canRedo()).toBe(false);
  });

  it("should return undefined for getCurrent after clear", () => {
    store.clear();

    expect(store.getCurrent()).toBeUndefined();
  });

  it("should allow new pushes after clear", () => {
    store.clear();

    const newFlow = createTestFlow("new-flow", "New Flow");
    store.push(newFlow);

    expect(store.size()).toBe(1);
    expect(store.getCurrent()).toEqual(newFlow);
  });
});

describe("Flow History Store - Get Current State", () => {
  let store: FlowHistoryStore;

  beforeEach(() => {
    store = createFlowHistoryStore();
  });

  it("should return current flow state", () => {
    const flow1 = createTestFlow("flow-1", "Flow 1");
    const flow2 = createTestFlow("flow-2", "Flow 2");

    store.push(flow1);
    store.push(flow2);

    expect(store.getCurrent()).toEqual(flow2);
  });

  it("should update after push", () => {
    const flow1 = createTestFlow("flow-1", "Flow 1");
    const flow2 = createTestFlow("flow-2", "Flow 2");

    store.push(flow1);
    expect(store.getCurrent()).toEqual(flow1);

    store.push(flow2);
    expect(store.getCurrent()).toEqual(flow2);
  });

  it("should update after pop", () => {
    const flow1 = createTestFlow("flow-1", "Flow 1");
    const flow2 = createTestFlow("flow-2", "Flow 2");

    store.push(flow1);
    store.push(flow2);

    store.pop();

    expect(store.getCurrent()).toEqual(flow1);
  });

  it("should update after redo", () => {
    const flow1 = createTestFlow("flow-1", "Flow 1");
    const flow2 = createTestFlow("flow-2", "Flow 2");

    store.push(flow1);
    store.push(flow2);

    store.pop();
    store.redo();

    expect(store.getCurrent()).toEqual(flow2);
  });
});

describe("Flow History Store - Edge Cases", () => {
  it("should handle rapid push operations", () => {
    const store = createFlowHistoryStore(5);

    for (let i = 0; i < 20; i++) {
      store.push(createTestFlow(`flow-${i}`, `Flow ${i}`));
    }

    expect(store.size()).toBe(5);
    expect(store.history[4].id).toBe("flow-19");
  });

  it("should handle alternating push and pop", () => {
    const store = createFlowHistoryStore();

    const flow1 = createTestFlow("flow-1", "Flow 1");
    const flow2 = createTestFlow("flow-2", "Flow 2");

    store.push(flow1);
    store.push(flow2);
    store.pop();
    store.push(flow2);
    store.pop();

    expect(store.getCurrent()).toEqual(flow1);
  });

  it("should handle pop beyond history start", () => {
    const store = createFlowHistoryStore();

    const flow1 = createTestFlow("flow-1", "Flow 1");
    store.push(flow1);

    const result1 = store.pop();
    const result2 = store.pop();
    const result3 = store.pop();

    expect(result1).toBeUndefined();
    expect(result2).toBeUndefined();
    expect(result3).toBeUndefined();
    expect(store.currentIndex).toBe(0);
  });

  it("should handle redo beyond history end", () => {
    const store = createFlowHistoryStore();

    const flow1 = createTestFlow("flow-1", "Flow 1");
    store.push(flow1);

    const result1 = store.redo();
    const result2 = store.redo();

    expect(result1).toBeUndefined();
    expect(result2).toBeUndefined();
  });

  it("should handle complex undo/redo sequences", () => {
    const store = createFlowHistoryStore();

    const flows = [
      createTestFlow("flow-1", "Flow 1"),
      createTestFlow("flow-2", "Flow 2"),
      createTestFlow("flow-3", "Flow 3"),
      createTestFlow("flow-4", "Flow 4"),
      createTestFlow("flow-5", "Flow 5"),
    ];

    flows.forEach((f) => store.push(f));

    // Complex sequence: undo 3 times, redo 2 times, undo 1 time
    store.pop(); // to flow4
    store.pop(); // to flow3
    store.pop(); // to flow2
    store.redo(); // to flow3
    store.redo(); // to flow4
    store.pop(); // to flow3

    expect(store.getCurrent()?.id).toBe("flow-3");
    expect(store.canUndo()).toBe(true);
    expect(store.canRedo()).toBe(true);
  });

  it("should preserve flow object immutability", () => {
    const store = createFlowHistoryStore();

    const originalFlow = createTestFlow("flow-1", "Flow 1", 2);
    store.push(originalFlow);

    const retrieved = store.getCurrent();

    // Modify retrieved object
    if (retrieved) {
      retrieved.name = "Modified Name";
      retrieved.nodes[0].name = "Modified Node";
    }

    // Original should not be affected
    expect(originalFlow.name).toBe("Flow 1");
    expect(originalFlow.nodes[0].name).toBe("Node 1");
  });
});
