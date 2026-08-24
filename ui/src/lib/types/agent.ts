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

  // Pending-approval snapshot — populated when state === "awaiting_approval".
  // Mirrors upstream agentic.PendingApprovalState (semstreams beta.19+).
  // Cleared when the loop resumes after an ApprovalResponse.
  pending_approval?: PendingApproval;
}

// PendingApproval mirrors agentic.PendingApprovalState from semstreams.
// Wire shape is set by the upstream JSON tags on the Go struct.
export interface PendingApproval {
  call_id: string;
  tool_name: string;
  arguments?: Record<string, unknown>;
  reason?: string;
  /** RFC3339 timestamp when the gating rejection arrived. */
  requested_at: string;
  /**
   * Auto-reject deadline as a nanosecond integer. Go's time.Duration
   * has no custom JSON marshaller, so it serialises as int64
   * (nanoseconds). Zero / absent means wait indefinitely. Convert to
   * seconds at the render site: `Math.round(timeout / 1e9)`.
   */
  timeout?: number;
  trace_id?: string;
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
  pending_approval?: PendingApproval;
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
    ...(w.pending_approval !== undefined && {
      pending_approval: w.pending_approval,
    }),
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

// ApprovalDecision values match the upstream agentic.ApprovalDecision*
// constants ("approve" / "reject" / "modify"). Wire format on
// POST /teams-dispatch/loops/{id}/approval.
export type ApprovalDecision = "approve" | "reject" | "modify";

export interface ApprovalRequest {
  decision: ApprovalDecision;
  /** Required when decision === "modify"; ignored otherwise. */
  modified_arguments?: Record<string, unknown>;
  /** Free-form reason; surfaced in the audit trail. */
  reason?: string;
  /**
   * Body fallback for the approver identity. The product middleware
   * lifts X-User-Id into ctx, which dominates this field. Sending
   * both keeps the request resolvable even if the middleware is not
   * deployed (e.g., during a partial rollout).
   */
  user_id?: string;
}

export interface ApprovalAcceptResponse {
  loop_id: string;
  decision: ApprovalDecision;
  accepted: boolean;
  message?: string;
  /** RFC3339 server-side acceptance timestamp. */
  timestamp: string;
}

// ---------------------------------------------------------------------------
// Loop trajectory — the GraphQL `trajectory(loopId, limit, cursor)` field on
// graph-gateway (semstreams beta.160). Replaces the deleted REST
// /teams-loop/trajectories[/{loopId}] endpoints, which this app no longer
// calls. Every fact is bounded, privacy-scrubbed metadata reconstructed from
// an immutable fact log: NO response text, NO tool arguments, NO tool
// results ride along — only kind/status/timing/token counts and short
// previews (model name, tool name, provider). A `StorageReference` in
// `evidence` points at the full body when one was captured, but this app
// does not fetch evidence stores (out of scope — see agentApi.ts comments).
//
// Field list verified against
// `gateway/graph-gateway/component.go` (trajectoryTypeDef /
// trajectoryTotalsTypeDef / trajectoryFactTypeDef) and the wire structs in
// `agentic/trajectory_fact.go` + `agentic/trajectory_query.go` at
// v1.0.0-beta.160.
// ---------------------------------------------------------------------------

// TrajectoryFactKind is the closed v1 observation vocabulary (see
// agentic.TrajectoryKind upstream). Typed as a union for authoring
// ergonomics in narrative-building code — the field itself widens
// gracefully at runtime since this is a compile-time-only guarantee.
export type TrajectoryFactKind =
  | "loop.started"
  | "model.requested"
  | "model.completed"
  | "tool.requested"
  | "tool.completed"
  | "context.compacted"
  | "loop.terminal";

export type TrajectoryFactPhase =
  | "loop_start"
  | "model_request"
  | "model_result"
  | "tool_request"
  | "tool_result"
  | "compaction"
  | "terminal";

// TrajectoryStatus is bounded *display* metadata upstream — not loop
// authority. "requested" facts precede their paired "completed" fact;
// "failed"/"cancelled" mark a completed observation that did not succeed.
export type TrajectoryFactStatus =
  | "requested"
  | "completed"
  | "failed"
  | "cancelled";

// TrajectoryStorageReference mirrors upstream message.StorageReference —
// a pointer into evidence storage, never the body itself.
export interface TrajectoryStorageReference {
  storage_instance?: string;
  key?: string;
  content_type?: string;
  /** Byte size hint; 0/absent means unknown. */
  size?: number;
}

export interface TrajectoryFact {
  schema_version?: string;
  loop_digest?: string;
  attempt_id?: string;
  attempt_ordinal?: number;
  kind: TrajectoryFactKind;
  source_kind?: string;
  source_correlation?: string;
  /** 0 for facts recorded before the first iteration (e.g. loop.started). */
  causal_iteration: number;
  causal_phase: TrajectoryFactPhase;
  causal_ordinal: number;
  /** RFC3339 timestamp. */
  observed_at?: string;
  elapsed_ms?: number;
  status?: TrajectoryFactStatus;
  tokens_in?: number;
  tokens_out?: number;
  message_count?: number;
  tool_count?: number;
  url_count?: number;
  /** Model name preview, e.g. "gemini-2.5-flash" — not response content. */
  model_preview?: string;
  provider_preview?: string;
  /** Tool name preview, e.g. "decide" — not call arguments or result. */
  tool_preview?: string;
  /** The loop's role at observation time, e.g. "coordinator". */
  capability_preview?: string;
  error_category?: string;
  evidence_digest?: string;
  evidence_size?: number;
  /** Present only when `evidence_capture === "stored"`. */
  evidence?: TrajectoryStorageReference;
  evidence_capture?: "none" | "stored" | "missing";
  evidence_failure?: string;
}

// TrajectoryObservedTotals summarizes only the facts returned in ONE page —
// it is not a running total across the whole loop. agentApi.ts sums pages
// together when it walks next_cursor so the merged trajectory's totals
// describe every fact it fetched.
export interface TrajectoryObservedTotals {
  facts: number;
  tokens_in: number;
  tokens_out: number;
  elapsed_ms: number;
  message_count: number;
  tool_count: number;
  url_count: number;
  model_requests: number;
  model_completions: number;
  tool_requests: number;
  tool_completions: number;
  context_compactions: number;
  terminal_observations: number;
  requested_observations: number;
  completed_observations: number;
  failed_observations: number;
  cancelled_observations: number;
}

// LoopTrajectory mirrors the GraphQL `Trajectory` type / upstream
// agentic.TrajectoryPage. `next_cursor` is only meaningful on a single raw
// page fetch; agentApi.ts's public getTrajectory/getLoopTrajectory walk
// cursors internally and return the merged result (next_cursor present only
// if pagination stopped early via the fact cap or a cursor-repeat guard).
export interface LoopTrajectory {
  schema_version: string;
  loop_id: string;
  coverage: string;
  observed_totals: TrajectoryObservedTotals;
  terminal_observed: boolean;
  facts: TrajectoryFact[];
  next_cursor?: string;
}
