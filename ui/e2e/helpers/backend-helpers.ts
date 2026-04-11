import type { Page } from "@playwright/test";
import type { FlowResponse } from "./flow-setup";

/**
 * Backend helper utilities for E2E tests
 * Provides functions to interact with backend APIs
 */

/**
 * Flow summary structure from /flowbuilder/flows endpoint
 */
export interface FlowSummary {
  id: string;
  name: string;
  description?: string;
  runtime_state: string;
  created_at: string;
  updated_at: string;
}

/**
 * Component type structure from /components/types endpoint
 */
export interface ComponentType {
  id: string;
  name: string;
  type: string;
  protocol: string;
  category: string;
  description: string;
  schema: Record<string, unknown>;
}

/**
 * Health response structure from /health endpoint
 */
export interface HealthResponse {
  status: string;
  version?: string;
}

/**
 * Verify backend health
 *
 * @param page - Playwright Page object
 * @returns true if backend is healthy, false otherwise
 */
export async function verifyBackendHealth(page: Page): Promise<boolean> {
  try {
    const response = await page.request.get("/health");
    return response.ok();
  } catch {
    return false;
  }
}

/**
 * List all flows from backend
 *
 * @param page - Playwright Page object
 * @returns Array of flow summaries
 * @throws Error if request fails or response is malformed
 */
export async function listFlows(page: Page): Promise<FlowSummary[]> {
  const response = await page.request.get("/flowbuilder/flows");

  if (!response.ok()) {
    const body = await response.text();
    throw new Error(
      `Failed to list flows: ${response.status()} ${response.statusText()}\n${body}`,
    );
  }

  try {
    const data = await response.json();
    return data.flows || [];
  } catch {
    throw new Error(`Failed to parse flows response from ${response.url()}`);
  }
}

/**
 * Get a specific flow by ID
 *
 * @param page - Playwright Page object
 * @param flowId - Flow ID to retrieve
 * @returns Flow object
 * @throws Error if flowId is empty or request fails
 */
export async function getFlow(
  page: Page,
  flowId: string,
): Promise<FlowResponse> {
  if (!flowId || flowId.trim() === "") {
    throw new Error("Flow ID cannot be empty");
  }

  const response = await page.request.get(`/flowbuilder/flows/${flowId}`);

  if (!response.ok()) {
    const body = await response.text();
    throw new Error(
      `Failed to get flow ${flowId}: ${response.status()} ${response.statusText()}\n${body}`,
    );
  }

  return await response.json();
}

/**
 * Delete all flows with "E2E Test" in their name
 * Used for test cleanup
 *
 * @param page - Playwright Page object
 * @returns Promise that resolves when cleanup is complete
 */
export async function cleanupAllTestFlows(page: Page): Promise<void> {
  const flows = await listFlows(page);
  const testFlows = flows.filter((flow) => flow.name.includes("E2E Test"));

  const deletePromises = testFlows.map(async (flow) => {
    try {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const response = await (page.request.delete as any)(
        `/flowbuilder/flows/${flow.id}`,
        {
          method: "DELETE",
        },
      );

      // Treat 404 as success (already deleted)
      if (!response.ok() && response.status() !== 404) {
        console.warn(`Failed to delete flow ${flow.id}: ${response.status()}`);
      }
    } catch (error) {
      console.warn(`Error deleting flow ${flow.id}:`, error);
    }
  });

  await Promise.all(deletePromises);
}

/**
 * Get all component types from backend
 *
 * @param page - Playwright Page object
 * @returns Array of component types
 * @throws Error if request fails
 */
export async function getComponentTypes(page: Page): Promise<ComponentType[]> {
  const response = await page.request.get("/components/types");

  if (!response.ok()) {
    const body = await response.text();
    throw new Error(
      `Failed to get component types: ${response.status()} ${response.statusText()}\n${body}`,
    );
  }

  return await response.json();
}

/**
 * Wait for a flow to reach a specific runtime state
 * Polls the backend until the flow reaches the expected state or timeout occurs
 *
 * @param page - Playwright Page object
 * @param flowId - Flow ID to monitor
 * @param expectedState - Expected runtime state to wait for
 * @param timeout - Maximum time to wait in milliseconds (default: 30000)
 * @throws Error if timeout occurs or flow not found
 */
export async function waitForFlowState(
  page: Page,
  flowId: string,
  expectedState: string,
  timeout: number = 30000,
): Promise<void> {
  const startTime = Date.now();
  const pollInterval = 500; // Poll every 500ms

  while (Date.now() - startTime < timeout) {
    const flow = await getFlow(page, flowId);

    if (flow.runtime_state === expectedState) {
      return;
    }

    // Wait before next poll
    await new Promise((resolve) => setTimeout(resolve, pollInterval));
  }

  throw new Error(
    `Timeout waiting for flow ${flowId} to reach state ${expectedState} (timeout: ${timeout}ms)`,
  );
}
