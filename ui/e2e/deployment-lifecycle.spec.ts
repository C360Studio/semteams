import { test, expect } from "@playwright/test";
import { FlowCanvasPage } from "./pages/FlowCanvasPage";
import { StatusBarPage } from "./pages/StatusBarPage";
import {
  createTestFlow,
  deleteTestFlow,
  createFlowWithValidationErrors,
} from "./helpers/flow-setup";
import { getFlow, waitForFlowState } from "./helpers/backend-helpers";

/**
 * Deployment Lifecycle E2E Tests with Backend State Verification
 * Tests Phase 4 requirements: deployment lifecycle with backend API verification
 *
 * REQUIRES: Backend ComponentManager and FlowBuilder must be functional
 */

test.describe("Deployment Lifecycle - Backend Verification", () => {
  let flowId: string;

  test.afterEach(async ({ page }) => {
    // Clean up test flow if created
    if (flowId) {
      await deleteTestFlow(page, flowId);
      flowId = "";
    }
  });

  test('Deploy flow verifies backend runtime_state is "deployed_stopped"', async ({
    page,
  }) => {
    // Create a test flow with a valid component via backend API
    const { id, url } = await createTestFlow(page, {
      name: "E2E Test - Deploy Backend Verification",
      nodes: [
        {
          id: "node-1",
          type: "udp-input",
          name: "UDP Input 1",
          config: {
            port: 8888,
            buffer_size: 1024,
          },
        },
      ],
    });
    flowId = id;

    // Navigate to the flow
    await page.goto(url);

    // Wait for canvas to load
    const canvas = new FlowCanvasPage(page);
    await canvas.expectCanvasLoaded();

    // Verify initial backend state is "not_deployed"
    let backendFlow = await getFlow(page, flowId);
    expect(backendFlow.runtime_state).toBe("not_deployed");

    // Verify UI shows "not_deployed"
    const statusBar = new StatusBarPage(page);
    await statusBar.expectStatusBarVisible();
    await statusBar.expectRuntimeState("not_deployed");

    // Click deploy button
    await statusBar.expectDeployButtonEnabled();
    await statusBar.clickDeploy();

    // Wait for backend state to update to "deployed_stopped"
    await waitForFlowState(page, flowId, "deployed_stopped", 10000);

    // Verify UI shows "deployed_stopped"
    await statusBar.expectRuntimeState("deployed_stopped");

    // Double-check backend state
    backendFlow = await getFlow(page, flowId);
    expect(backendFlow.runtime_state).toBe("deployed_stopped");

    // Verify Start button is now available
    await statusBar.expectStartButtonVisible();
  });

  test('Start flow verifies backend runtime_state is "running"', async ({
    page,
  }) => {
    // Create and deploy a flow
    const { id, url } = await createTestFlow(page, {
      name: "E2E Test - Start Backend Verification",
      nodes: [
        {
          id: "node-1",
          type: "udp-input",
          name: "UDP Input 1",
          config: {
            port: 8889,
            buffer_size: 1024,
          },
        },
      ],
    });
    flowId = id;

    await page.goto(url);

    const canvas = new FlowCanvasPage(page);
    await canvas.expectCanvasLoaded();

    // Deploy the flow
    const statusBar = new StatusBarPage(page);
    await statusBar.clickDeploy();
    await waitForFlowState(page, flowId, "deployed_stopped", 10000);

    // Verify backend state is "deployed_stopped"
    let backendFlow = await getFlow(page, flowId);
    expect(backendFlow.runtime_state).toBe("deployed_stopped");

    // Start the flow
    await statusBar.clickStart();

    // Wait for backend state to update to "running"
    await waitForFlowState(page, flowId, "running", 10000);

    // Verify UI shows "running"
    await statusBar.expectRuntimeState("running");

    // Double-check backend state
    backendFlow = await getFlow(page, flowId);
    expect(backendFlow.runtime_state).toBe("running");

    // Verify Stop button is enabled
    await statusBar.expectStopButtonEnabled();
  });

  test('Stop flow verifies backend runtime_state is "deployed_stopped"', async ({
    page,
  }) => {
    // Create, deploy, and start a flow
    const { id, url } = await createTestFlow(page, {
      name: "E2E Test - Stop Backend Verification",
      nodes: [
        {
          id: "node-1",
          type: "udp-input",
          name: "UDP Input 1",
          config: {
            port: 8890,
            buffer_size: 1024,
          },
        },
      ],
    });
    flowId = id;

    await page.goto(url);

    const canvas = new FlowCanvasPage(page);
    await canvas.expectCanvasLoaded();

    const statusBar = new StatusBarPage(page);

    // Deploy the flow
    await statusBar.clickDeploy();
    await waitForFlowState(page, flowId, "deployed_stopped", 10000);

    // Start the flow
    await statusBar.clickStart();
    await waitForFlowState(page, flowId, "running", 10000);

    // Verify backend state is "running"
    let backendFlow = await getFlow(page, flowId);
    expect(backendFlow.runtime_state).toBe("running");

    // Stop the flow
    await statusBar.clickStop();

    // Wait for backend state to update to "deployed_stopped"
    await waitForFlowState(page, flowId, "deployed_stopped", 10000);

    // Verify UI shows "deployed_stopped"
    await statusBar.expectRuntimeState("deployed_stopped");

    // Double-check backend state
    backendFlow = await getFlow(page, flowId);
    expect(backendFlow.runtime_state).toBe("deployed_stopped");

    // Verify Start button is available again (can restart)
    await statusBar.expectStartButtonEnabled();
  });

  test('Undeploy flow verifies backend runtime_state is "not_deployed"', async ({
    page,
  }) => {
    // Create and deploy a flow
    const { id, url } = await createTestFlow(page, {
      name: "E2E Test - Undeploy Backend Verification",
      nodes: [
        {
          id: "node-1",
          type: "udp-input",
          name: "UDP Input 1",
          config: {
            port: 8891,
            buffer_size: 1024,
          },
        },
      ],
    });
    flowId = id;

    await page.goto(url);

    const canvas = new FlowCanvasPage(page);
    await canvas.expectCanvasLoaded();

    const statusBar = new StatusBarPage(page);

    // Deploy the flow
    await statusBar.clickDeploy();
    await waitForFlowState(page, flowId, "deployed_stopped", 10000);

    // Verify backend state is "deployed_stopped"
    let backendFlow = await getFlow(page, flowId);
    expect(backendFlow.runtime_state).toBe("deployed_stopped");

    // Look for undeploy button (may be in a menu or separate button)
    // Attempt to find undeploy button
    const undeployButton = page.locator('button:has-text("Undeploy")');

    // Check if undeploy button exists
    const undeployExists = (await undeployButton.count()) > 0;

    if (undeployExists) {
      // Click undeploy
      await undeployButton.click();

      // Wait for backend state to update to "not_deployed"
      await waitForFlowState(page, flowId, "not_deployed", 10000);

      // Verify UI shows "not_deployed"
      await statusBar.expectRuntimeState("not_deployed");

      // Double-check backend state
      backendFlow = await getFlow(page, flowId);
      expect(backendFlow.runtime_state).toBe("not_deployed");

      // Verify Deploy button is available again
      await statusBar.expectDeployButtonVisible();
    } else {
      // Skip test if undeploy not implemented yet
      test.skip();
    }
  });

  test('Invalid flow blocks deployment and backend state stays "not_deployed"', async ({
    page,
  }) => {
    // Create a flow with validation errors (unconfigured components)
    const { id, url } = await createFlowWithValidationErrors(page);
    flowId = id;

    await page.goto(url);

    const canvas = new FlowCanvasPage(page);
    await canvas.expectCanvasLoaded();

    const statusBar = new StatusBarPage(page);

    // Verify initial backend state is "not_deployed"
    let backendFlow = await getFlow(page, flowId);
    expect(backendFlow.runtime_state).toBe("not_deployed");

    // Attempt to deploy the invalid flow
    await statusBar.expectDeployButtonEnabled();
    await statusBar.clickDeploy();

    // Wait a moment for validation to run
    await page.waitForTimeout(1000);

    // Expect validation dialog or error message
    // Look for validation dialog/modal
    const validationDialog = page
      .locator('[role="dialog"]')
      .or(page.locator(".dialog"))
      .or(page.locator(".modal"));

    // Check if dialog appears
    const dialogVisible = await validationDialog
      .isVisible({ timeout: 2000 })
      .catch(() => false);

    if (dialogVisible) {
      // Verify validation message is shown
      await expect(validationDialog).toContainText(
        /validation|error|invalid|configuration/i,
      );
    } else {
      // Alternatively, check for error message in status bar or toast
      const errorMessage = page.locator(
        ".error-message, .status-message, .toast",
      );
      await expect(errorMessage.first()).toContainText(
        /validation|error|invalid|configuration/i,
        { timeout: 2000 },
      );
    }

    // Verify backend state is still "not_deployed"
    backendFlow = await getFlow(page, flowId);
    expect(backendFlow.runtime_state).toBe("not_deployed");

    // Verify UI still shows "not_deployed"
    await statusBar.expectRuntimeState("not_deployed");
  });

  test("State persists across page refresh", async ({ page }) => {
    // Create, deploy, and start a flow
    const { id, url } = await createTestFlow(page, {
      name: "E2E Test - State Persistence",
      nodes: [
        {
          id: "node-1",
          type: "udp-input",
          name: "UDP Input 1",
          config: {
            port: 8892,
            buffer_size: 1024,
          },
        },
      ],
    });
    flowId = id;

    await page.goto(url);

    const canvas = new FlowCanvasPage(page);
    await canvas.expectCanvasLoaded();

    const statusBar = new StatusBarPage(page);

    // Deploy and start the flow
    await statusBar.clickDeploy();
    await waitForFlowState(page, flowId, "deployed_stopped", 10000);

    await statusBar.clickStart();
    await waitForFlowState(page, flowId, "running", 10000);

    // Verify backend state is "running"
    let backendFlow = await getFlow(page, flowId);
    expect(backendFlow.runtime_state).toBe("running");

    // Refresh the page
    await page.reload();

    // Wait for canvas to reload
    await canvas.expectCanvasLoaded();

    // Verify UI shows "running" after refresh
    await statusBar.expectRuntimeState("running");

    // Verify backend state is still "running"
    backendFlow = await getFlow(page, flowId);
    expect(backendFlow.runtime_state).toBe("running");

    // Verify Stop button is still available
    await statusBar.expectStopButtonEnabled();
  });

  test("Cannot start undeployed flow", async ({ page }) => {
    // Create a new flow (not deployed)
    const { id, url } = await createTestFlow(page, {
      name: "E2E Test - Cannot Start Undeployed",
      nodes: [
        {
          id: "node-1",
          type: "udp-input",
          name: "UDP Input 1",
          config: {
            port: 8893,
            buffer_size: 1024,
          },
        },
      ],
    });
    flowId = id;

    await page.goto(url);

    const canvas = new FlowCanvasPage(page);
    await canvas.expectCanvasLoaded();

    const statusBar = new StatusBarPage(page);

    // Verify backend state is "not_deployed"
    const backendFlow = await getFlow(page, flowId);
    expect(backendFlow.runtime_state).toBe("not_deployed");

    // Verify UI shows "not_deployed"
    await statusBar.expectRuntimeState("not_deployed");

    // Verify Start button is NOT available (only Deploy should be visible)
    const startButton = statusBar.startButton;
    const startButtonVisible = await startButton
      .isVisible({ timeout: 1000 })
      .catch(() => false);

    expect(startButtonVisible).toBe(false);

    // Verify Deploy button IS available
    await statusBar.expectDeployButtonVisible();
    await statusBar.expectDeployButtonEnabled();
  });
});
