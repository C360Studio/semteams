import { Page } from "@playwright/test";

/**
 * Test helper utilities for flow setup and teardown
 * Used by E2E tests to create flows via backend API
 */

/**
 * Flow node structure for E2E test setup
 */
export interface TestFlowNode {
  id: string;
  type: string;
  name: string;
  config: Record<string, unknown>;
}

/**
 * Flow connection structure for E2E test setup
 */
export interface TestFlowConnection {
  id: string;
  source_node_id: string;
  source_port: string;
  target_node_id: string;
  target_port: string;
}

/**
 * Backend flow response structure
 */
export interface FlowResponse {
  id: string;
  name: string;
  description?: string;
  version: number;
  runtime_state: string;
  nodes: TestFlowNode[];
  connections: TestFlowConnection[];
  created_at: string;
  updated_at: string;
  last_modified: string;
}

export interface FlowSetupOptions {
  name?: string;
  description?: string;
  nodes?: TestFlowNode[];
  connections?: TestFlowConnection[];
}

export interface TestFlowResult {
  id: string;
  url: string;
  flow: FlowResponse;
}

/**
 * Create a test flow via backend API
 *
 * @param page - Playwright Page object
 * @param options - Optional flow configuration
 * @returns Flow ID, URL, and full flow object
 *
 * @example
 * // Create empty flow
 * const { url, id } = await createTestFlow(page);
 *
 * @example
 * // Create flow with components
 * const { url, id } = await createTestFlow(page, {
 *   nodes: [{ id: 'node-1', type: 'udp-input', name: 'udp 1', config: {} }]
 * });
 */
export async function createTestFlow(
  page: Page,
  options?: FlowSetupOptions,
): Promise<TestFlowResult> {
  const timestamp = Date.now();
  const flowData = {
    name: options?.name || `E2E Test Flow ${timestamp}`,
    description:
      options?.description ||
      `Created by E2E test at ${new Date(timestamp).toISOString()}`,
    nodes: options?.nodes || [],
    connections: options?.connections || [],
  };

  const response = await page.request.post("/flowbuilder/flows", {
    data: flowData,
  });

  if (!response.ok()) {
    const body = await response.text();
    throw new Error(
      `Failed to create test flow: ${response.status()} ${response.statusText()}\n${body}`,
    );
  }

  const flow = await response.json();

  return {
    id: flow.id,
    url: `/flows/${flow.id}`,
    flow,
  };
}

/**
 * Delete a test flow via backend API
 *
 * @param page - Playwright Page object
 * @param flowId - Flow ID to delete
 *
 * @example
 * await deleteTestFlow(page, flowId);
 */
export async function deleteTestFlow(
  page: Page,
  flowId: string,
): Promise<void> {
  const response = await page.request.delete(`/flowbuilder/flows/${flowId}`);

  if (!response.ok() && response.status() !== 404) {
    // 404 is OK (flow already deleted or never existed)
    const body = await response.text();
    console.warn(
      `Warning: Failed to delete test flow ${flowId}: ${response.status()} ${response.statusText()}\n${body}`,
    );
  }
}

/**
 * Create a flow with validation errors for testing
 * Creates a flow with unconfigured components that will trigger validation errors
 *
 * @param page - Playwright Page object
 * @returns Flow with validation errors
 */
export async function createFlowWithValidationErrors(
  page: Page,
): Promise<TestFlowResult> {
  return createTestFlow(page, {
    name: "E2E Test Flow - With Validation Errors",
    nodes: [
      {
        id: "node-1",
        type: "udp-input",
        name: "udp 1",
        config: {}, // Unconfigured - will cause validation error
      },
      {
        id: "node-2",
        type: "robotics-processor",
        name: "Robotics Processor 1",
        config: {}, // Unconfigured - will cause validation error
      },
    ],
  });
}
