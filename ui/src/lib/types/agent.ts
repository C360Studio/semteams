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

  // Completion data — present once the loop terminates. Sourced from
  // the dispatch's COMPLETE_<id> envelope, merged onto the loop entry.
  // `prompt` is the field we mine for human-readable task titles.
  prompt?: string;
  result?: string;
  tokens_in?: number;
  tokens_out?: number;
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
  prompt?: string;
  result?: string;
  tokens_in?: number;
  tokens_out?: number;
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
    ...(w.prompt !== undefined && { prompt: w.prompt }),
    ...(w.result !== undefined && { result: w.result }),
    ...(w.tokens_in !== undefined && { tokens_in: w.tokens_in }),
    ...(w.tokens_out !== undefined && { tokens_out: w.tokens_out }),
  };
}

// extractCompletionPatch reads a "COMPLETE_<id>" envelope and produces
// a partial AgentLoop containing the completion fields (prompt, result,
// tokens_in, tokens_out, outcome) that the dispatch publishes
// separately from the main loop entry. Returns null if the envelope is
// not a completion record or has no usable id.
export function extractCompletionPatch(
  env: WireActivityEnvelope,
): { id: string; patch: Partial<AgentLoop> } | null {
  if (!env.loop_id?.startsWith("COMPLETE_")) return null;
  const id = env.loop_id.slice("COMPLETE_".length);
  const w = env.data;
  if (!id || !w) return null;
  const patch: Partial<AgentLoop> = {};
  if (w.prompt !== undefined) patch.prompt = w.prompt;
  if (w.result !== undefined) patch.result = w.result;
  if (w.tokens_in !== undefined) patch.tokens_in = w.tokens_in;
  if (w.tokens_out !== undefined) patch.tokens_out = w.tokens_out;
  if (w.outcome !== undefined) patch.outcome = w.outcome;
  return { id, patch };
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

// ---------------------------------------------------------------------------
// Loop trajectory — what /teams-loop/trajectories/<loop_id> actually returns.
// The "narrative" data source for the story view in TaskDetailPanel.
// Shape verified against the running e2e-coordinator stack 2026-04-27.
// ---------------------------------------------------------------------------

export type TrajectoryStepType = "model_call" | "tool_call";

export interface ModelCallStep {
  step_type: "model_call";
  timestamp: string;
  request_id: string;
  /** Populated only after the response arrives. */
  response?: string;
  tokens_in?: number;
  tokens_out?: number;
  /** Duration in milliseconds. */
  duration?: number;
  model?: string;
  provider?: string;
  /** Capability the model was called with — "coordinator", "researcher", … */
  capability?: string;
}

export interface ToolCallStep {
  step_type: "tool_call";
  timestamp: string;
  tool_name: string;
  tool_arguments?: Record<string, unknown>;
  tool_result?: string;
  tool_status?: string;
  duration?: number;
  provider?: string;
  capability?: string;
}

export type TrajectoryStep = ModelCallStep | ToolCallStep;

export interface LoopTrajectory {
  loop_id: string;
  start_time: string;
  end_time?: string;
  steps: TrajectoryStep[];
  outcome?: string;
  total_tokens_in?: number;
  total_tokens_out?: number;
  /** Total duration in milliseconds. Populated once the loop completes. */
  duration?: number;
}
