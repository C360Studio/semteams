import { describe, it, expect } from "vitest";
import type { AgentLoop, AgentLoopState } from "./agent";
import {
  loopStateToColumn,
  deriveTaskInfo,
  COLUMNS,
  type TaskColumn,
} from "./task";

// ---------------------------------------------------------------------------
// Helper: minimal AgentLoop factory
// ---------------------------------------------------------------------------

function makeLoop(overrides: Partial<AgentLoop> = {}): AgentLoop {
  return {
    loop_id: "loop_001",
    task_id: "task_001",
    state: "executing",
    role: "general",
    iterations: 0,
    max_iterations: 10,
    user_id: "user-1",
    channel_type: "web",
    parent_loop_id: "",
    outcome: "",
    error: "",
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// loopStateToColumn — exhaustive mapping
// ---------------------------------------------------------------------------

describe("loopStateToColumn", () => {
  const expectations: [AgentLoopState, TaskColumn][] = [
    ["exploring", "thinking"],
    ["planning", "thinking"],
    ["architecting", "thinking"],
    ["executing", "executing"],
    ["reviewing", "executing"],
    ["awaiting_approval", "needs_you"],
    ["paused", "needs_you"],
    ["complete", "done"],
    ["success", "done"],
    ["truncated", "done"],
    ["failed", "failed"],
    ["error", "failed"],
    ["cancelled", "failed"],
  ];

  it.each(expectations)(
    "maps %s → %s",
    (state, expectedColumn) => {
      expect(loopStateToColumn(state)).toBe(expectedColumn);
    },
  );

  it("covers all 10 AgentLoopState values", () => {
    const allStates: AgentLoopState[] = [
      "exploring",
      "planning",
      "architecting",
      "executing",
      "reviewing",
      "paused",
      "awaiting_approval",
      "complete",
      "failed",
      "cancelled",
    ];
    for (const state of allStates) {
      expect(loopStateToColumn(state)).toBeDefined();
    }
    expect(allStates).toHaveLength(10);
  });
});

// ---------------------------------------------------------------------------
// COLUMNS metadata
// ---------------------------------------------------------------------------

describe("COLUMNS", () => {
  it("has 5 column definitions in display order", () => {
    expect(COLUMNS).toHaveLength(5);
    expect(COLUMNS.map((c) => c.id)).toEqual([
      "thinking",
      "executing",
      "needs_you",
      "done",
      "failed",
    ]);
  });

  it("marks Failed as hidden by default", () => {
    const failed = COLUMNS.find((c) => c.id === "failed");
    expect(failed?.defaultVisible).toBe(false);
  });

  it("marks all other columns as visible by default", () => {
    const visible = COLUMNS.filter((c) => c.id !== "failed");
    for (const col of visible) {
      expect(col.defaultVisible).toBe(true);
    }
  });
});

// ---------------------------------------------------------------------------
// deriveTaskInfo — task derivation from loop + children
// ---------------------------------------------------------------------------

describe("deriveTaskInfo", () => {
  it("creates a TaskInfo from a top-level loop with no children", () => {
    const loop = makeLoop({
      loop_id: "loop_abc",
      task_id: "task_abc",
      state: "executing",
      role: "editor",
      iterations: 3,
      max_iterations: 10,
    });

    const task = deriveTaskInfo(loop, []);

    expect(task.id).toBe("loop_abc");
    expect(task.title).toBe("task_abc");
    expect(task.column).toBe("executing");
    expect(task.state).toBe("executing");
    expect(task.role).toBe("editor");
    expect(task.iterations).toBe(3);
    expect(task.maxIterations).toBe(10);
    expect(task.primaryLoop).toBe(loop);
    expect(task.childLoops).toHaveLength(0);
    expect(task.childNeedsAttention).toBe(false);
    expect(task.childAttentionCount).toBe(0);
  });

  it("title preference: prompt → task_id → truncated loop_id", () => {
    // Best case: the user's prompt arrived (via dispatch's COMPLETE_<id>
    // merge in agentStore) and becomes the human-readable title.
    const withPrompt = deriveTaskInfo(
      makeLoop({
        task_id: "uuid-not-pretty",
        loop_id: "loop_xyz",
        prompt: "compare mqtt vs nats",
      }),
      [],
    );
    expect(withPrompt.title).toBe("compare mqtt vs nats");

    // Pre-completion / no prompt yet → fall back to task_id (still
    // ugly, but stable).
    const withTaskId = deriveTaskInfo(
      makeLoop({ task_id: "my-task", loop_id: "loop_long_id_1234" }),
      [],
    );
    expect(withTaskId.title).toBe("my-task");

    // No task_id either → short slice of loop_id.
    const withoutTaskId = deriveTaskInfo(
      makeLoop({ task_id: "", loop_id: "loop_long_id_1234" }),
      [],
    );
    expect(withoutTaskId.title).toBe("loop_long_id");
  });

  it("title strips a leading slash command", () => {
    // The slash command itself is captured in role/state already.
    // "/research foo bar" → "foo bar"; lookup is friendlier without it.
    const t = deriveTaskInfo(
      makeLoop({ prompt: "/research compare mqtt vs nats" }),
      [],
    );
    expect(t.title).toBe("compare mqtt vs nats");
  });

  it("title collapses whitespace and truncates with ellipsis at 80 chars", () => {
    const longPrompt =
      "this is a really long prompt that goes on and on and on without any meaningful content but exceeds the maximum title length we allow";
    const t = deriveTaskInfo(makeLoop({ prompt: longPrompt }), []);
    expect(t.title.length).toBeLessThanOrEqual(80);
    expect(t.title.endsWith("…")).toBe(true);

    const messy = deriveTaskInfo(
      makeLoop({ prompt: "  hello\n\nworld\t  test  " }),
      [],
    );
    expect(messy.title).toBe("hello world test");
  });

  it("title falls through prompt:'' → task_id when prompt is empty", () => {
    // Belt-and-suspenders: if the wire format ever sends prompt:"",
    // we should NOT show an empty card title.
    const t = deriveTaskInfo(
      makeLoop({ prompt: "", task_id: "real-task-id" }),
      [],
    );
    expect(t.title).toBe("real-task-id");
  });

  it("column follows primary loop state when no children", () => {
    const thinking = deriveTaskInfo(makeLoop({ state: "planning" }), []);
    expect(thinking.column).toBe("thinking");

    const done = deriveTaskInfo(makeLoop({ state: "complete" }), []);
    expect(done.column).toBe("done");
  });

  describe("child loop urgency aggregation", () => {
    it("needs_you child overrides parent executing state", () => {
      const parent = makeLoop({ state: "executing", loop_id: "parent" });
      const child = makeLoop({
        state: "awaiting_approval",
        loop_id: "child",
        parent_loop_id: "parent",
      });

      const task = deriveTaskInfo(parent, [child]);

      expect(task.column).toBe("needs_you");
      expect(task.childNeedsAttention).toBe(true);
      expect(task.childAttentionCount).toBe(1);
    });

    it("failed child overrides parent thinking state", () => {
      const parent = makeLoop({ state: "exploring", loop_id: "parent" });
      const child = makeLoop({
        state: "failed",
        loop_id: "child",
        parent_loop_id: "parent",
      });

      const task = deriveTaskInfo(parent, [child]);

      expect(task.column).toBe("failed");
      expect(task.childNeedsAttention).toBe(false);
    });

    it("needs_you wins over failed when both are present", () => {
      const parent = makeLoop({ state: "complete", loop_id: "parent" });
      const failedChild = makeLoop({
        state: "failed",
        loop_id: "child1",
        parent_loop_id: "parent",
      });
      const approvalChild = makeLoop({
        state: "awaiting_approval",
        loop_id: "child2",
        parent_loop_id: "parent",
      });

      const task = deriveTaskInfo(parent, [failedChild, approvalChild]);

      expect(task.column).toBe("needs_you");
      expect(task.childAttentionCount).toBe(1);
    });

    it("done children don't override parent executing state", () => {
      const parent = makeLoop({ state: "executing", loop_id: "parent" });
      const child = makeLoop({
        state: "complete",
        loop_id: "child",
        parent_loop_id: "parent",
      });

      const task = deriveTaskInfo(parent, [child]);

      expect(task.column).toBe("executing");
    });

    it("counts multiple children needing attention", () => {
      const parent = makeLoop({ state: "executing", loop_id: "parent" });
      const child1 = makeLoop({
        state: "awaiting_approval",
        loop_id: "child1",
        parent_loop_id: "parent",
      });
      const child2 = makeLoop({
        state: "paused",
        loop_id: "child2",
        parent_loop_id: "parent",
      });
      const child3 = makeLoop({
        state: "complete",
        loop_id: "child3",
        parent_loop_id: "parent",
      });

      const task = deriveTaskInfo(parent, [child1, child2, child3]);

      expect(task.childNeedsAttention).toBe(true);
      // awaiting_approval maps to needs_you, paused maps to needs_you
      expect(task.childAttentionCount).toBe(2);
    });
  });

  it("preserves child loops array on the TaskInfo", () => {
    const parent = makeLoop({ loop_id: "parent" });
    const children = [
      makeLoop({ loop_id: "c1", parent_loop_id: "parent" }),
      makeLoop({ loop_id: "c2", parent_loop_id: "parent" }),
    ];

    const task = deriveTaskInfo(parent, children);

    expect(task.childLoops).toHaveLength(2);
    expect(task.childLoops[0].loop_id).toBe("c1");
    expect(task.childLoops[1].loop_id).toBe("c2");
  });
});
