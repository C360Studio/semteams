// Flow API client
// Handles CRUD operations for flows

import type { Flow } from "$lib/types/flow";

const API_BASE = "/flowbuilder/flows";

export interface CreateFlowRequest {
  name: string;
  description?: string;
}

export interface FlowListResponse {
  flows: Flow[];
}

export class FlowApiError extends Error {
  constructor(
    message: string,
    public statusCode: number,
    public details?: unknown,
  ) {
    super(message);
    this.name = "FlowApiError";
  }
}

/**
 * Backend flow response structure (may have null/undefined arrays)
 */
interface BackendFlowResponse {
  id: string;
  name: string;
  description?: string;
  version: number;
  runtime_state: string;
  nodes?: unknown[] | null;
  connections?: unknown[] | null;
  deployed_at?: string;
  started_at?: string;
  stopped_at?: string;
  created_at: string;
  updated_at: string;
  created_by?: string;
  last_modified: string;
}

/**
 * Normalize flow data from backend
 * Ensures nodes and connections are always arrays (never null/undefined)
 */
function normalizeFlow(flow: BackendFlowResponse): Flow {
  return {
    ...flow,
    nodes: (flow.nodes || []) as Flow["nodes"],
    connections: (flow.connections || []) as Flow["connections"],
    runtime_state: flow.runtime_state as Flow["runtime_state"],
  };
}

export const flowApi = {
  /**
   * Create a new flow
   */
  async createFlow(request: CreateFlowRequest): Promise<Flow> {
    const response = await fetch(API_BASE, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(request),
    });

    if (!response.ok) {
      const error = await response.json().catch(() => ({}));
      throw new FlowApiError(
        `Failed to create flow: ${response.statusText}`,
        response.status,
        error,
      );
    }

    const flow = await response.json();
    return normalizeFlow(flow);
  },

  /**
   * List all flows
   */
  async listFlows(): Promise<Flow[]> {
    const response = await fetch(API_BASE);

    if (!response.ok) {
      throw new FlowApiError(
        `Failed to list flows: ${response.statusText}`,
        response.status,
      );
    }

    const data: FlowListResponse = await response.json();
    return data.flows.map(normalizeFlow);
  },

  /**
   * Get flow by ID
   */
  async getFlow(id: string): Promise<Flow> {
    const response = await fetch(`${API_BASE}/${id}`);

    if (!response.ok) {
      throw new FlowApiError(
        `Failed to get flow: ${response.statusText}`,
        response.status,
      );
    }

    const flow = await response.json();
    return normalizeFlow(flow);
  },

  /**
   * Update flow
   * Returns 409 if version conflict detected (optimistic concurrency)
   */
  async updateFlow(id: string, flow: Flow): Promise<Flow> {
    const response = await fetch(`${API_BASE}/${id}`, {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(flow),
    });

    if (!response.ok) {
      const error = await response.json().catch(() => ({}));
      throw new FlowApiError(
        `Failed to update flow: ${response.statusText}`,
        response.status,
        error,
      );
    }

    const updatedFlow = await response.json();
    return normalizeFlow(updatedFlow);
  },

  /**
   * Delete flow
   */
  async deleteFlow(id: string): Promise<void> {
    const response = await fetch(`${API_BASE}/${id}`, {
      method: "DELETE",
    });

    if (!response.ok) {
      throw new FlowApiError(
        `Failed to delete flow: ${response.statusText}`,
        response.status,
      );
    }
  },
};
