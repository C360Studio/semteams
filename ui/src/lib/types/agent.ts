export type AgentLoopState =
  | "exploring"
  | "planning"
  | "architecting"
  | "executing"
  | "reviewing"
  | "paused"
  | "awaiting_approval"
  | "complete"
  | "success"
  | "failed"
  | "error"
  | "cancelled"
  | "truncated";
// Terminal-state aliases ("success"/"error"/"truncated") come from upstream
// dispatch writing the loop's `outcome` value into the `state` field at
// completion (loop_tracker.UpdateState in agentic-dispatch). Until upstream
// normalises that, the UI accepts both vocabularies.

export type ActiveLoopState =
  | "exploring"
  | "planning"
  | "architecting"
  | "executing"
  | "reviewing";

export function isActiveState(state: AgentLoopState): state is ActiveLoopState {
  return [
    "exploring",
    "planning",
    "architecting",
    "executing",
    "reviewing",
  ].includes(state);
}

export interface AgentLoop {
  loop_id: string;
  task_id: string;
  state: AgentLoopState;
  role: string;
  iterations: number;
  max_iterations: number;
  user_id: string;
  channel_type: string;
  parent_loop_id: string;
  outcome: string;
  error: string;
}

// WireLoop is the actual JSON shape SSE delivers — looser than AgentLoop
// because two upstream producers disagree on the field names. The KV
// watcher emits `id`; the dispatch LoopTracker emits `loop_id`. Several
// fields are absent (`omitempty`) when empty. Treat WireLoop as an
// untrusted input and use `normalizeWireLoop` to produce an AgentLoop.
//
// The index signature catches producer-specific fields we don't model
// (e.g. `result`, `tokens_in`, `model`, `prompt` on COMPLETE_<id>
// records). Listed fields are read by normalizeWireLoop; everything else
// rides along untyped.
export interface WireLoop {
  id?: string;
  loop_id?: string;
  task_id?: string;
  state?: string;
  role?: string;
  iterations?: number;
  max_iterations?: number;
  user_id?: string;
  channel_type?: string;
  parent_loop_id?: string;
  outcome?: string;
  error?: string;
  [k: string]: unknown;
}

export interface WireActivityEnvelope {
  type: string;
  loop_id: string;
  timestamp?: string;
  data?: WireLoop;
}

// normalizeWireLoop takes an SSE envelope and produces an AgentLoop, or
// null if the record should be ignored. Drops COMPLETE_<id> ghost records
// (dispatch publishes these alongside the real loop entry — they carry
// outcome/result but no `state` field, so they're not loops in their own
// right). Reads the loop id from either `data.loop_id` or `data.id`.
export function normalizeWireLoop(env: WireActivityEnvelope): AgentLoop | null {
  if (env.loop_id?.startsWith("COMPLETE_")) return null;
  const w = env.data;
  if (!w) return null;
  const id = w.loop_id || w.id;
  if (!id) return null;
  return {
    loop_id: id,
    task_id: w.task_id ?? "",
    state: (w.state as AgentLoopState) ?? "exploring",
    role: w.role ?? "",
    iterations: w.iterations ?? 0,
    max_iterations: w.max_iterations ?? 0,
    user_id: w.user_id ?? "",
    channel_type: w.channel_type ?? "",
    parent_loop_id: w.parent_loop_id ?? "",
    outcome: w.outcome ?? "",
    error: w.error ?? "",
  };
}

export type ControlSignal =
  | "pause"
  | "resume"
  | "cancel"
  | "approve"
  | "reject"
  | "feedback"
  | "retry";

export type AgentActivityEventType =
  | "connected"
  | "sync_complete"
  | "loop_update"
  | "heartbeat";

export interface LoopUpdateEvent {
  loop_id: string;
  task_id: string;
  state: AgentLoopState;
  role: string;
  iterations: number;
  max_iterations: number;
  user_id: string;
  channel_type: string;
  parent_loop_id: string;
  outcome: string;
  error: string;
}

export interface SignalRequest {
  type: ControlSignal;
  reason?: string;
}

export interface SignalResponse {
  loop_id: string;
  signal: string;
  status: string;
}

export interface TrajectoryEntry {
  loop_id: string;
  role: string;
  iterations: number;
  outcome: string;
  duration_ms: number;
  token_usage?: {
    input_tokens: number;
    output_tokens: number;
  };
  tool_calls?: TrajectoryToolCall[];
}

export interface TrajectoryToolCall {
  name: string;
  args: Record<string, unknown>;
  result?: string;
  error?: string;
  duration_ms?: number;
}
