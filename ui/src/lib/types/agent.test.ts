import { describe, it, expect } from "vitest";
import {
  isActiveState,
  type AgentLoopState,
  type ActiveLoopState,
  type AgentLoop,
  type ControlSignal,
  type AgentActivityEventType,
  type LoopUpdateEvent,
  type SignalRequest,
  type SignalResponse,
  type TrajectoryEntry,
  type TrajectoryToolCall,
} from "$lib/types/agent";

// ---------------------------------------------------------------------------
// isActiveState — type guard
// ---------------------------------------------------------------------------

describe("isActiveState", () => {
  const activeStates: ActiveLoopState[] = [
    "exploring",
    "planning",
    "architecting",
    "executing",
    "reviewing",
  ];

  it.each(activeStates)("returns true for active state '%s'", (state) => {
    expect(isActiveState(state)).toBe(true);
  });

  const inactiveStates: AgentLoopState[] = [
    "paused",
    "awaiting_approval",
    "complete",
    "failed",
    "cancelled",
  ];

  it.each(inactiveStates)("returns false for inactive state '%s'", (state) => {
    expect(isActiveState(state)).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// AgentLoop — struct shape
// ---------------------------------------------------------------------------

describe("AgentLoop", () => {
  it("has all required fields", () => {
    const loop: AgentLoop = {
      loop_id: "loop-1",
      task_id: "task-1",
      state: "exploring",
      role: "architect",
      iterations: 2,
      max_iterations: 10,
      user_id: "user-1",
      channel_type: "slack",
      parent_loop_id: "",
      outcome: "",
      error: "",
    };

    expect(loop.loop_id).toBe("loop-1");
    expect(loop.task_id).toBe("task-1");
    expect(loop.state).toBe("exploring");
    expect(loop.role).toBe("architect");
    expect(loop.iterations).toBe(2);
    expect(loop.max_iterations).toBe(10);
    expect(loop.user_id).toBe("user-1");
    expect(loop.channel_type).toBe("slack");
    expect(loop.parent_loop_id).toBe("");
    expect(loop.outcome).toBe("");
    expect(loop.error).toBe("");
  });
});

// ---------------------------------------------------------------------------
// ControlSignal — literal union
// ---------------------------------------------------------------------------

describe("ControlSignal", () => {
  const validSignals: ControlSignal[] = [
    "pause",
    "resume",
    "cancel",
    "approve",
    "reject",
    "feedback",
    "retry",
  ];

  it.each(validSignals)("'%s' is a valid ControlSignal", (signal) => {
    const req: SignalRequest = { type: signal };
    expect(req.type).toBe(signal);
  });
});

// ---------------------------------------------------------------------------
// AgentActivityEventType — literal union
// ---------------------------------------------------------------------------

describe("AgentActivityEventType", () => {
  const validTypes: AgentActivityEventType[] = [
    "connected",
    "sync_complete",
    "loop_update",
    "heartbeat",
  ];

  it.each(validTypes)("'%s' is a valid event type", (eventType) => {
    const t: AgentActivityEventType = eventType;
    expect(t).toBe(eventType);
  });
});

// ---------------------------------------------------------------------------
// LoopUpdateEvent — struct shape
// ---------------------------------------------------------------------------

describe("LoopUpdateEvent", () => {
  it("has all required fields matching AgentLoop shape", () => {
    const event: LoopUpdateEvent = {
      loop_id: "loop-1",
      task_id: "task-1",
      state: "awaiting_approval",
      role: "executor",
      iterations: 5,
      max_iterations: 20,
      user_id: "user-2",
      channel_type: "web",
      parent_loop_id: "loop-0",
      outcome: "",
      error: "",
    };

    expect(event.loop_id).toBe("loop-1");
    expect(event.state).toBe("awaiting_approval");
    expect(event.role).toBe("executor");
    expect(event.iterations).toBe(5);
    expect(event.max_iterations).toBe(20);
    expect(event.parent_loop_id).toBe("loop-0");
  });
});

// ---------------------------------------------------------------------------
// SignalRequest / SignalResponse
// ---------------------------------------------------------------------------

describe("SignalRequest", () => {
  it("has required type field", () => {
    const req: SignalRequest = { type: "approve" };
    expect(req.type).toBe("approve");
  });

  it("reason is optional", () => {
    const req: SignalRequest = { type: "reject", reason: "Unsafe operation" };
    expect(req.reason).toBe("Unsafe operation");

    const reqNoReason: SignalRequest = { type: "pause" };
    expect(reqNoReason.reason).toBeUndefined();
  });
});

describe("SignalResponse", () => {
  it("has loop_id, signal, and status", () => {
    const res: SignalResponse = {
      loop_id: "loop-1",
      signal: "approve",
      status: "accepted",
    };
    expect(res.loop_id).toBe("loop-1");
    expect(res.signal).toBe("approve");
    expect(res.status).toBe("accepted");
  });
});

// ---------------------------------------------------------------------------
// TrajectoryEntry / TrajectoryToolCall
// ---------------------------------------------------------------------------

describe("TrajectoryEntry", () => {
  it("has required fields", () => {
    const entry: TrajectoryEntry = {
      loop_id: "loop-1",
      role: "architect",
      iterations: 3,
      outcome: "success",
      duration_ms: 1500,
    };

    expect(entry.loop_id).toBe("loop-1");
    expect(entry.role).toBe("architect");
    expect(entry.iterations).toBe(3);
    expect(entry.outcome).toBe("success");
    expect(entry.duration_ms).toBe(1500);
  });

  it("token_usage is optional", () => {
    const entry: TrajectoryEntry = {
      loop_id: "loop-1",
      role: "executor",
      iterations: 1,
      outcome: "complete",
      duration_ms: 500,
      token_usage: { input_tokens: 1000, output_tokens: 200 },
    };

    expect(entry.token_usage?.input_tokens).toBe(1000);
    expect(entry.token_usage?.output_tokens).toBe(200);
  });

  it("tool_calls is optional", () => {
    const toolCall: TrajectoryToolCall = {
      name: "graph_search",
      args: { query: "drones" },
      result: "found 5 entities",
      duration_ms: 120,
    };

    const entry: TrajectoryEntry = {
      loop_id: "loop-1",
      role: "executor",
      iterations: 2,
      outcome: "success",
      duration_ms: 800,
      tool_calls: [toolCall],
    };

    expect(entry.tool_calls).toHaveLength(1);
    expect(entry.tool_calls![0].name).toBe("graph_search");
    expect(entry.tool_calls![0].result).toBe("found 5 entities");
  });
});

describe("TrajectoryToolCall", () => {
  it("has required name and args", () => {
    const tc: TrajectoryToolCall = {
      name: "entity_lookup",
      args: { id: "entity:person:alice:1" },
    };

    expect(tc.name).toBe("entity_lookup");
    expect(tc.args).toEqual({ id: "entity:person:alice:1" });
  });

  it("result and error are optional", () => {
    const success: TrajectoryToolCall = {
      name: "graph_search",
      args: {},
      result: "ok",
    };
    expect(success.result).toBe("ok");
    expect(success.error).toBeUndefined();

    const failure: TrajectoryToolCall = {
      name: "graph_search",
      args: {},
      error: "timeout",
    };
    expect(failure.error).toBe("timeout");
    expect(failure.result).toBeUndefined();
  });

  it("duration_ms is optional", () => {
    const tc: TrajectoryToolCall = {
      name: "health_check",
      args: {},
      duration_ms: 50,
    };
    expect(tc.duration_ms).toBe(50);
  });
});
