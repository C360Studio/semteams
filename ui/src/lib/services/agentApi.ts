// Agent API client
// Handles communication with teams-dispatch and teams-loop backend services

import type {
  AgentLoop,
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
