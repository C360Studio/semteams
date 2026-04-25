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
