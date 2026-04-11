import { Page, expect } from "@playwright/test";

/**
 * Runtime Panel Test Helpers
 * Utilities for E2E testing of Runtime Visualization Panel
 */

export interface RunningFlowSetup {
  flowId: string;
  url: string;
}

/**
 * Create a flow, deploy it, and start it (full runtime setup)
 *
 * @param page - Playwright Page object
 * @returns Flow ID and URL
 *
 * @example
 * const { flowId, url } = await createRunningFlow(page);
 * await page.goto(url);
 */
export async function createRunningFlow(page: Page): Promise<RunningFlowSetup> {
  const timestamp = Date.now();
  const flowData = {
    name: `E2E Runtime Test Flow ${timestamp}`,
    description: `Runtime testing flow created at ${new Date(timestamp).toISOString()}`,
    nodes: [
      {
        id: "udp-input-1",
        type: "udp",
        name: "udp",
        config: {},
        position: { x: 100, y: 100 },
      },
    ],
    connections: [],
  };

  // Create flow
  const createResponse = await page.request.post("/flowbuilder/flows", {
    data: flowData,
  });

  if (!createResponse.ok()) {
    const body = await createResponse.text();
    throw new Error(
      `Failed to create test flow: ${createResponse.status()} ${createResponse.statusText()}\n${body}`,
    );
  }

  const flow = await createResponse.json();
  const flowId = flow.id;

  // Deploy flow (backend uses /deployment/{id}/deploy)
  const deployResponse = await page.request.post(
    `/flowbuilder/deployment/${flowId}/deploy`,
  );
  if (!deployResponse.ok()) {
    const body = await deployResponse.text();
    throw new Error(
      `Failed to deploy flow: ${deployResponse.status()}\n${body}`,
    );
  }

  // Start flow (backend uses /deployment/{id}/start)
  const startResponse = await page.request.post(
    `/flowbuilder/deployment/${flowId}/start`,
  );
  if (!startResponse.ok()) {
    const body = await startResponse.text();
    throw new Error(`Failed to start flow: ${startResponse.status()}\n${body}`);
  }

  return {
    flowId,
    url: `/flows/${flowId}`,
  };
}

/**
 * Wait for runtime panel to be visible and animated
 *
 * @param page - Playwright Page object
 * @param timeout - Optional timeout in milliseconds
 *
 * @example
 * await waitForRuntimePanel(page);
 */
export async function waitForRuntimePanel(
  page: Page,
  timeout: number = 5000,
): Promise<void> {
  await expect(page.locator('[data-testid="runtime-panel"]')).toBeVisible({
    timeout,
  });

  // Wait for slide-up animation to complete (300ms from RuntimePanel.svelte)
  await page.waitForTimeout(350);
}

/**
 * Wait for specific tab content to be visible
 *
 * @param page - Playwright Page object
 * @param tabName - Tab to wait for ('logs' | 'messages' | 'metrics' | 'health')
 * @param timeout - Optional timeout in milliseconds
 *
 * @example
 * await waitForTabContent(page, 'metrics');
 */
export async function waitForTabContent(
  page: Page,
  tabName: "logs" | "messages" | "metrics" | "health",
  timeout: number = 5000,
): Promise<void> {
  await expect(page.locator(`[data-testid="${tabName}-panel"]`)).toBeVisible({
    timeout,
  });
}

/**
 * Click tab and wait for content to load
 *
 * @param page - Playwright Page object
 * @param tabName - Tab to click
 *
 * @example
 * await clickTab(page, 'metrics');
 */
export async function clickTab(
  page: Page,
  tabName: "logs" | "messages" | "metrics" | "health",
): Promise<void> {
  // Click the tab button
  await page.click(`[data-testid="tab-${tabName}"]`);
  // Small wait for state to update and re-render
  await page.waitForTimeout(100);
  await waitForTabContent(page, tabName);
}

/**
 * Mock metrics endpoint response
 * NOTE: Use sparingly - prefer testing against real backend
 *
 * @param page - Playwright Page object
 * @param flowId - Flow ID
 * @param data - Metrics response data
 *
 * @example
 * await mockMetricsResponse(page, flowId, {
 *   timestamp: new Date().toISOString(),
 *   components: [
 *     { name: 'udp-input-1', throughput: 100, errorRate: 0, status: 'healthy' }
 *   ]
 * });
 */
export async function mockMetricsResponse(
  page: Page,
  flowId: string,
  data: {
    timestamp: string;
    components: Array<{
      name: string;
      throughput: number;
      errorRate: number;
      status: "healthy" | "degraded" | "error";
      cpu?: number;
      memory?: number;
    }>;
  },
): Promise<void> {
  await page.route(`**/flowbuilder/flows/${flowId}/runtime/metrics`, (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(data),
    }),
  );
}

/**
 * Mock health endpoint response
 * NOTE: Use sparingly - prefer testing against real backend
 *
 * @param page - Playwright Page object
 * @param flowId - Flow ID
 * @param data - Health response data
 *
 * @example
 * await mockHealthResponse(page, flowId, {
 *   summary: { running: 1, degraded: 0, error: 0 },
 *   components: [
 *     { name: 'udp-input-1', status: 'running', uptime: 120, lastActivity: new Date().toISOString() }
 *   ]
 * });
 */
export async function mockHealthResponse(
  page: Page,
  flowId: string,
  data: {
    summary: { running: number; degraded: number; error: number };
    components: Array<{
      name: string;
      status: "running" | "degraded" | "error" | "stopped";
      uptime: number;
      lastActivity: string;
      details?: string;
    }>;
  },
): Promise<void> {
  await page.route(`**/flowbuilder/flows/${flowId}/runtime/health`, (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(data),
    }),
  );
}

/**
 * Mock messages endpoint response
 * NOTE: Use sparingly - prefer testing against real backend
 *
 * @param page - Playwright Page object
 * @param flowId - Flow ID
 * @param data - Messages response data
 *
 * @example
 * await mockMessagesResponse(page, flowId, {
 *   messages: [
 *     { timestamp: new Date().toISOString(), source: 'udp-input-1', destination: 'processor-1', messageType: 'data', size: 1024 }
 *   ]
 * });
 */
export async function mockMessagesResponse(
  page: Page,
  flowId: string,
  data: {
    messages: Array<{
      timestamp: string;
      source: string;
      destination: string;
      messageType: string;
      size: number;
    }>;
  },
): Promise<void> {
  await page.route(`**/flowbuilder/flows/${flowId}/runtime/messages`, (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(data),
    }),
  );
}

/**
 * Mock SSE log stream endpoint
 * NOTE: This is complex - prefer testing against real backend
 * This helper is provided for reference but E2E tests should use real SSE
 *
 * @param page - Playwright Page object
 * @param flowId - Flow ID
 * @param logs - Array of log entries to stream
 *
 * @example
 * // This is challenging to implement in Playwright - better to test real backend
 * await mockLogsSSE(page, flowId, [
 *   { timestamp: new Date().toISOString(), level: 'INFO', component: 'udp-input-1', message: 'Started' }
 * ]);
 */
export async function mockLogsSSE(
  page: Page,
  flowId: string,
  logs: Array<{
    timestamp: string;
    level: "DEBUG" | "INFO" | "WARN" | "ERROR";
    component: string;
    message: string;
  }>,
): Promise<void> {
  // SSE mocking is complex in Playwright
  // For E2E tests, prefer testing against real backend
  // This is a simplified mock that won't stream properly
  let logIndex = 0;

  await page.route(`**/flowbuilder/flows/${flowId}/runtime/logs`, (route) => {
    if (logIndex < logs.length) {
      const log = logs[logIndex++];
      route.fulfill({
        status: 200,
        contentType: "text/event-stream",
        body: `event: log\ndata: ${JSON.stringify(log)}\n\n`,
      });
    } else {
      route.fulfill({
        status: 200,
        contentType: "text/event-stream",
        body: "",
      });
    }
  });
}

/**
 * Get current canvas height (useful for verifying panel resize)
 *
 * @param page - Playwright Page object
 * @returns Canvas height in pixels
 *
 * @example
 * const initialHeight = await getCanvasHeight(page);
 * // ... open panel
 * const newHeight = await getCanvasHeight(page);
 * expect(newHeight).toBeLessThan(initialHeight);
 */
export async function getCanvasHeight(page: Page): Promise<number> {
  const canvas = page.locator("#flow-canvas");
  const box = await canvas.boundingBox();
  return box?.height || 0;
}

/**
 * Stop a running flow via backend API
 *
 * @param page - Playwright Page object
 * @param flowId - Flow ID to stop
 * @returns true if stop succeeded, false if endpoint not ready
 *
 * @example
 * const stopped = await stopFlow(page, flowId);
 * if (stopped) {
 *   await waitForFlowState(page, flowId, 'stopped', 10000);
 * }
 */
export async function stopFlow(page: Page, flowId: string): Promise<boolean> {
  const response = await page.request.post(
    `/flowbuilder/deployment/${flowId}/stop`,
  );

  if (!response.ok()) {
    const body = await response.text();
    console.warn(
      `Warning: Failed to stop flow ${flowId}: ${response.status()} ${response.statusText()}\n${body}`,
    );
    return false;
  }

  return true;
}

/**
 * Start a deployed flow via backend API
 *
 * @param page - Playwright Page object
 * @param flowId - Flow ID to start
 *
 * @example
 * await startFlow(page, flowId);
 */
export async function startFlow(page: Page, flowId: string): Promise<void> {
  const response = await page.request.post(
    `/flowbuilder/deployment/${flowId}/start`,
  );

  if (!response.ok()) {
    const body = await response.text();
    throw new Error(
      `Failed to start flow ${flowId}: ${response.status()} ${response.statusText()}\n${body}`,
    );
  }
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
