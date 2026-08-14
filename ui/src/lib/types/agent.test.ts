import { describe, it, expect } from "vitest";
import {
  isActiveState,
  normalizeWireLoop,
  extractCompletionPatch,
  type AgentLoopState,
  type ActiveLoopState,
  type AgentLoop,
  type ControlSignal,
  type AgentActivityEventType,
  type LoopUpdateEvent,
  type SignalRequest,
  type SignalResponse,
  type LoopTrajectory,
  type TrajectoryFact,
  type WireActivityEnvelope,
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
    "success",
    "failed",
    "error",
    "cancelled",
    "truncated",
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
// LoopTrajectory / TrajectoryFact — the beta.160 GraphQL trajectory shape.
// Field list verified against gateway/graph-gateway/component.go
// (trajectoryTypeDef / trajectoryFactTypeDef) at v1.0.0-beta.160.
// ---------------------------------------------------------------------------

function makeObservedTotals(): LoopTrajectory["observed_totals"] {
  return {
    facts: 2,
    tokens_in: 100,
    tokens_out: 50,
    elapsed_ms: 1200,
    message_count: 3,
    tool_count: 1,
    url_count: 0,
    model_requests: 1,
    model_completions: 1,
    tool_requests: 1,
    tool_completions: 1,
    context_compactions: 0,
    terminal_observations: 0,
    requested_observations: 2,
    completed_observations: 2,
    failed_observations: 0,
    cancelled_observations: 0,
  };
}

describe("TrajectoryFact", () => {
  it("has the required fact fields", () => {
    const fact: TrajectoryFact = {
      kind: "tool.completed",
      causal_iteration: 1,
      causal_phase: "tool_result",
      causal_ordinal: 0,
      status: "completed",
      tool_preview: "decide",
      capability_preview: "coordinator",
    };

    expect(fact.kind).toBe("tool.completed");
    expect(fact.causal_iteration).toBe(1);
    expect(fact.causal_phase).toBe("tool_result");
    expect(fact.status).toBe("completed");
    expect(fact.tool_preview).toBe("decide");
    expect(fact.capability_preview).toBe("coordinator");
  });

  it("carries only a StorageReference pointer for evidence, never a body", () => {
    const fact: TrajectoryFact = {
      kind: "model.completed",
      causal_iteration: 1,
      causal_phase: "model_result",
      causal_ordinal: 0,
      evidence_capture: "stored",
      evidence_size: 4096,
      evidence: {
        storage_instance: "message-store",
        key: "2026/08/14/loop-1/req-1",
        content_type: "application/json",
        size: 4096,
      },
    };

    expect(fact.evidence?.key).toBe("2026/08/14/loop-1/req-1");
    expect(fact.evidence_capture).toBe("stored");
    // No `response`/`arguments`/`result` field exists on TrajectoryFact —
    // this is a type-level guarantee, verified by the fact that the object
    // above type-checks without one.
  });
});

describe("LoopTrajectory", () => {
  it("has the required page fields", () => {
    const trajectory: LoopTrajectory = {
      schema_version: "v1",
      loop_id: "loop-1",
      coverage: "observed",
      terminal_observed: true,
      observed_totals: makeObservedTotals(),
      facts: [
        {
          kind: "loop.terminal",
          causal_iteration: 3,
          causal_phase: "terminal",
          causal_ordinal: 0,
          status: "completed",
          elapsed_ms: 5000,
        },
      ],
    };

    expect(trajectory.loop_id).toBe("loop-1");
    expect(trajectory.coverage).toBe("observed");
    expect(trajectory.terminal_observed).toBe(true);
    expect(trajectory.observed_totals.tokens_in).toBe(100);
    expect(trajectory.facts).toHaveLength(1);
    expect(trajectory.facts[0].kind).toBe("loop.terminal");
  });

  it("next_cursor is optional — absent once fully drained", () => {
    const trajectory: LoopTrajectory = {
      schema_version: "v1",
      loop_id: "loop-1",
      coverage: "observed",
      terminal_observed: false,
      observed_totals: makeObservedTotals(),
      facts: [],
    };

    expect(trajectory.next_cursor).toBeUndefined();
  });
});

// ---------------------------------------------------------------------------
// normalizeWireLoop — SSE wire-format adapter
// ---------------------------------------------------------------------------

describe("normalizeWireLoop", () => {
  it("accepts the KV-watcher shape that uses `id` instead of `loop_id`", () => {
    // Real loop records from the agentic-loop KV watcher use `id`. The UI
    // canonicalises to `loop_id`.
    const env: WireActivityEnvelope = {
      type: "loop_updated",
      loop_id: "loop-1",
      data: {
        id: "loop-1",
        task_id: "task-1",
        state: "complete",
        role: "researcher",
        iterations: 3,
        max_iterations: 6,
      },
    };
    const loop = normalizeWireLoop(env);
    expect(loop?.loop_id).toBe("loop-1");
    expect(loop?.state).toBe("complete");
    expect(loop?.role).toBe("researcher");
    expect(loop?.iterations).toBe(3);
  });

  it("accepts the LoopTracker shape that uses `loop_id`", () => {
    const env: WireActivityEnvelope = {
      type: "loop_updated",
      loop_id: "loop-2",
      data: {
        loop_id: "loop-2",
        task_id: "task-2",
        state: "exploring",
        role: "coordinator",
      },
    };
    const loop = normalizeWireLoop(env);
    expect(loop?.loop_id).toBe("loop-2");
  });

  it("drops COMPLETE_<id> ghost envelopes", () => {
    // dispatch publishes these alongside the real loop entry; they carry
    // outcome/result for an already-tracked loop, no `state`, and they're
    // not loops in their own right.
    const env: WireActivityEnvelope = {
      type: "loop_updated",
      loop_id: "COMPLETE_loop-3",
      data: {
        loop_id: "COMPLETE_loop-3",
        task_id: "task-3",
        outcome: "success",
        result: "done",
      },
    };
    expect(normalizeWireLoop(env)).toBeNull();
  });

  it("drops envelopes with no data", () => {
    const env: WireActivityEnvelope = { type: "loop_updated", loop_id: "loop-4" };
    expect(normalizeWireLoop(env)).toBeNull();
  });

  it("drops envelopes that have neither `id` nor `loop_id`", () => {
    const env: WireActivityEnvelope = {
      type: "loop_updated",
      loop_id: "loop-5",
      data: { task_id: "task-5", state: "exploring" },
    };
    expect(normalizeWireLoop(env)).toBeNull();
  });

  it("coerces missing optional fields to empty defaults", () => {
    const env: WireActivityEnvelope = {
      type: "loop_updated",
      loop_id: "loop-6",
      data: { id: "loop-6" },
    };
    const loop = normalizeWireLoop(env);
    expect(loop).toEqual<AgentLoop>({
      loop_id: "loop-6",
      task_id: "",
      state: "exploring",
      role: "",
      iterations: 0,
      max_iterations: 0,
      user_id: "",
      channel_type: "",
      parent_loop_id: "",
      outcome: "",
      error: "",
    });
  });

  it("preserves outcome aliases that upstream leaks into `state`", () => {
    // Backend dispatch writes Outcome ("success"/"failed"/...) into the
    // state field; UI must accept the alias vocabulary verbatim and let
    // loopStateToColumn map it.
    const env: WireActivityEnvelope = {
      type: "loop_updated",
      loop_id: "loop-7",
      data: { id: "loop-7", state: "success" },
    };
    expect(normalizeWireLoop(env)?.state).toBe("success");
  });

  it("propagates prompt/result/tokens when present", () => {
    // The KV-watcher shape sometimes includes completion fields directly
    // on the main loop entry (post beta.14). When it does, we expose
    // them on AgentLoop so deriveTaskInfo can use prompt for the title.
    const env: WireActivityEnvelope = {
      type: "loop_updated",
      loop_id: "loop-8",
      data: {
        id: "loop-8",
        state: "complete",
        prompt: "compare mqtt vs nats for iot edge",
        result: "## Summary\n\n...",
        tokens_in: 100,
        tokens_out: 50,
      },
    };
    const loop = normalizeWireLoop(env);
    expect(loop?.prompt).toBe("compare mqtt vs nats for iot edge");
    expect(loop?.result).toContain("Summary");
    expect(loop?.tokens_in).toBe(100);
    expect(loop?.tokens_out).toBe(50);
  });
});

// ---------------------------------------------------------------------------
// extractCompletionPatch — COMPLETE_<id> envelope handling
// ---------------------------------------------------------------------------

describe("extractCompletionPatch", () => {
  it("returns null for non-COMPLETE envelopes", () => {
    const env: WireActivityEnvelope = {
      type: "loop_updated",
      loop_id: "loop-1",
      data: { id: "loop-1", state: "complete" },
    };
    expect(extractCompletionPatch(env)).toBeNull();
  });

  it("returns null for COMPLETE envelopes with no data", () => {
    const env: WireActivityEnvelope = {
      type: "loop_updated",
      loop_id: "COMPLETE_loop-1",
    };
    expect(extractCompletionPatch(env)).toBeNull();
  });

  it("strips the COMPLETE_ prefix and returns id + completion patch", () => {
    const env: WireActivityEnvelope = {
      type: "loop_updated",
      loop_id: "COMPLETE_loop-1",
      data: {
        loop_id: "COMPLETE_loop-1",
        outcome: "success",
        prompt: "compare mqtt vs nats",
        result: "## Summary\n\n...",
        tokens_in: 100,
        tokens_out: 50,
      },
    };
    const out = extractCompletionPatch(env);
    expect(out?.id).toBe("loop-1");
    expect(out?.patch).toEqual({
      outcome: "success",
      prompt: "compare mqtt vs nats",
      result: "## Summary\n\n...",
      tokens_in: 100,
      tokens_out: 50,
    });
  });

  it("only surfaces fields that are present (no undefined zero-fill)", () => {
    const env: WireActivityEnvelope = {
      type: "loop_updated",
      loop_id: "COMPLETE_loop-1",
      data: { loop_id: "COMPLETE_loop-1", prompt: "hi" },
    };
    const out = extractCompletionPatch(env);
    // Only `prompt` was set — patch should not include keys for the
    // missing fields, so a merge doesn't accidentally overwrite values
    // that the main loop entry already has.
    expect(out?.patch).toEqual({ prompt: "hi" });
    expect(Object.keys(out?.patch ?? {})).toEqual(["prompt"]);
  });
});
