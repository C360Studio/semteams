// Agent API client
// Handles communication with teams-dispatch and teams-loop backend services

import { userIdentity } from "$lib/stores/userIdentity.svelte";
import type {
  AgentLoop,
  ApprovalAcceptResponse,
  ApprovalRequest,
  ControlSignal,
  LoopTrajectory,
  SignalResponse,
  TrajectoryEntry,
} from "$lib/types/agent";

const DISPATCH_BASE = "/teams-dispatch";
const LOOP_BASE = "/teams-loop";

export class AgentApiError extends Error {
  constructor(
    message: string,
    public statusCode: number,
    public details?: unknown,
  ) {
    super(message);
    this.name = "AgentApiError";
  }
}

export const agentApi = {
  async sendMessage(content: string): Promise<{ content: string }> {
    const response = await fetch(`${DISPATCH_BASE}/message`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ content }),
    });
    if (!response.ok) {
      const error = await response.json().catch(() => ({}));
      throw new AgentApiError(
        `Failed to send message: ${response.statusText}`,
        response.status,
        error,
      );
    }
    return response.json();
  },

  async listLoops(): Promise<AgentLoop[]> {
    const response = await fetch(`${DISPATCH_BASE}/loops`);
    if (!response.ok) {
      throw new AgentApiError(
        `Failed to list loops: ${response.statusText}`,
        response.status,
      );
    }
    return response.json();
  },

  async getLoop(id: string): Promise<AgentLoop> {
    const response = await fetch(`${DISPATCH_BASE}/loops/${id}`);
    if (!response.ok) {
      throw new AgentApiError(
        `Failed to get loop: ${response.statusText}`,
        response.status,
      );
    }
    return response.json();
  },

  async sendSignal(
    id: string,
    type: ControlSignal,
    reason?: string,
  ): Promise<SignalResponse> {
    const response = await fetch(`${DISPATCH_BASE}/loops/${id}/signal`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ type, ...(reason ? { reason } : {}) }),
    });
    if (!response.ok) {
      const error = await response.json().catch(() => ({}));
      throw new AgentApiError(
        `Failed to send signal: ${response.statusText}`,
        response.status,
        error,
      );
    }
    return response.json();
  },

  /**
   * Submit a human approval response for a gated tool call. Drives
   * the semstreams beta.19 approval flow: the agent loop is paused
   * waiting for this call. The backend publishes an ApprovalResponse
   * on `agent.approval_response.<loop_id>` and the loop resumes.
   *
   * Sets the X-User-Id header from the userIdentity store so the
   * product middleware (cmd/semteams/middleware.go) lifts the value
   * into agentic-dispatch's identity ctx before resolution. The
   * backend's resolution order — ctx > body.user_id > "http-user" —
   * means a deployment without the middleware still resolves the
   * caller via the body fallback.
   *
   * Throws AgentApiError on non-2xx responses. Notable status codes:
   *   - 400: malformed body or unknown decision value
   *   - 404: loop unknown to dispatch (process restart, etc.)
   *   - 409: loop tracked but not awaiting approval (already resolved
   *          or never gated). Caller should refresh state.
   *   - 500: NATS publish failure; safe to retry.
   */
  async submitApproval(
    id: string,
    request: ApprovalRequest,
  ): Promise<ApprovalAcceptResponse> {
    const identity = userIdentity.value;
    // Treat an explicitly-empty user_id the same as absent: fall back
    // to the store value. Upstream IdentityFromRequest treats empty
    // body fields as "no claim" too, so an empty body would resolve
    // via ctx-or-default — but sending the empty string makes the
    // body and header disagree, which is noise.
    const callerUserID = request.user_id?.trim();
    const body: ApprovalRequest = {
      ...request,
      // Body fallback so the request resolves even if the middleware
      // is not deployed. Middleware-injected ctx still wins per
      // upstream IdentityFromRequest precedence.
      user_id: callerUserID && callerUserID !== "" ? callerUserID : identity,
    };
    const response = await fetch(`${DISPATCH_BASE}/loops/${id}/approval`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-User-Id": identity,
      },
      body: JSON.stringify(body),
    });
    if (!response.ok) {
      const error = await response.json().catch(() => ({}));
      throw new AgentApiError(
        `Failed to submit approval: ${response.statusText}`,
        response.status,
        error,
      );
    }
    return response.json();
  },

  async getTrajectories(): Promise<TrajectoryEntry[]> {
    const response = await fetch(`${LOOP_BASE}/trajectories`);
    if (!response.ok) {
      throw new AgentApiError(
        `Failed to get trajectories: ${response.statusText}`,
        response.status,
      );
    }
    return response.json();
  },

  async getTrajectory(loopId: string): Promise<TrajectoryEntry> {
    const response = await fetch(`${LOOP_BASE}/trajectories/${loopId}`);
    if (!response.ok) {
      throw new AgentApiError(
        `Failed to get trajectory: ${response.statusText}`,
        response.status,
      );
    }
    return response.json();
  },

  /**
   * Fetch the loop's full trajectory — the structured step sequence
   * (model_call / tool_call) that powers the story view.
   * Same endpoint as getTrajectory, but typed against the actual wire
   * shape rather than the legacy summary type.
   */
  async getLoopTrajectory(loopId: string): Promise<LoopTrajectory> {
    const response = await fetch(`${LOOP_BASE}/trajectories/${loopId}`);
    if (!response.ok) {
      throw new AgentApiError(
        `Failed to get loop trajectory: ${response.statusText}`,
        response.status,
      );
    }
    return response.json();
  },
};
