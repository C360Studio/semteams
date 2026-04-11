import { test } from "@playwright/test";
import { FlowCanvasPage } from "./pages/FlowCanvasPage";
import { ComponentPalettePage } from "./pages/ComponentPalettePage";
import { StatusBarPage } from "./pages/StatusBarPage";
import { FlowListPage } from "./pages/FlowListPage";

/**
 * Runtime Control E2E Tests
 * Tests scenarios 15-18 from spec.md
 *
 * REQUIRES: Backend ComponentManager must be functional
 */

test.describe("Runtime Control", () => {
  test('Scenario 15: Deploy flow → Status becomes "deployed_stopped"', async ({
    page,
  }) => {
    // Navigate to flow list
    const flowList = new FlowListPage(page);
    await flowList.goto();
    await page.waitForLoadState("networkidle");

    // Create new flow
    await flowList.clickCreateNewFlow();
    await page.waitForURL(/\/flows\/.+/);

    // Wait for canvas to load
    const canvas = new FlowCanvasPage(page);
    await canvas.expectCanvasLoaded();

    // Add udp component (valid minimal flow - will deploy with warning for no subscribers)
    const palette = new ComponentPalettePage(page);
    await palette.addComponentToCanvas("UDP Input");

    // Save the flow
    await canvas.clickSaveButton();
    await page.waitForTimeout(1000);

    // Verify initial runtime state is not_deployed
    const statusBar = new StatusBarPage(page);
    await statusBar.expectStatusBarVisible();
    await statusBar.expectRuntimeState("not_deployed");

    // Click deploy button
    await statusBar.expectDeployButtonEnabled();
    await statusBar.clickDeploy();

    // Wait for deployment to complete
    await page.waitForTimeout(2000);

    // Verify runtime state changed to deployed_stopped
    await statusBar.expectRuntimeState("deployed_stopped");

    // Verify Start button is now available (flow deployed but not running)
    await statusBar.expectStartButtonVisible();
  });

  test('Scenario 16: Start flow → Status becomes "running"', async ({
    page,
  }) => {
    // Navigate to flow list
    const flowList = new FlowListPage(page);
    await flowList.goto();
    await page.waitForLoadState("networkidle");

    // Create and deploy a flow
    await flowList.clickCreateNewFlow();
    await page.waitForURL(/\/flows\/.+/);

    const canvas = new FlowCanvasPage(page);
    await canvas.expectCanvasLoaded();

    // Add udp component (valid minimal flow - will deploy with warning for no subscribers)
    const palette = new ComponentPalettePage(page);
    await palette.addComponentToCanvas("UDP Input");

    // Save and deploy
    await canvas.clickSaveButton();
    await page.waitForTimeout(1000);

    const statusBar = new StatusBarPage(page);
    await statusBar.clickDeploy();
    await page.waitForTimeout(2000);
    await statusBar.expectRuntimeState("deployed_stopped");

    // Start the flow (button changes from "Deploy" to "Start" after deployment)
    await statusBar.clickStart();
    await page.waitForTimeout(2000);

    // Verify runtime state is now running
    await statusBar.expectRuntimeState("running");

    // Verify Stop button is enabled
    await statusBar.expectStopButtonEnabled();
  });

  test('Scenario 17: Stop flow → Status becomes "deployed_stopped"', async ({
    page,
  }) => {
    // Navigate to flow list
    const flowList = new FlowListPage(page);
    await flowList.goto();
    await page.waitForLoadState("networkidle");

    // Create, deploy, and start a flow
    await flowList.clickCreateNewFlow();
    await page.waitForURL(/\/flows\/.+/);

    const canvas = new FlowCanvasPage(page);
    await canvas.expectCanvasLoaded();

    // Add udp component (valid minimal flow - will deploy with warning for no subscribers)
    const palette = new ComponentPalettePage(page);
    await palette.addComponentToCanvas("UDP Input");

    // Save and deploy
    await canvas.clickSaveButton();
    await page.waitForTimeout(1000);

    const statusBar = new StatusBarPage(page);
    await statusBar.clickDeploy();
    await page.waitForTimeout(2000);

    // Start flow
    await statusBar.clickStart();
    await page.waitForTimeout(2000);
    await statusBar.expectRuntimeState("running");

    // Stop the flow
    await statusBar.clickStop();

    // Wait for state transition
    await page.waitForTimeout(2000);

    // Verify runtime state is deployed_stopped
    await statusBar.expectRuntimeState("deployed_stopped");

    // Verify Start button is available again (not Deploy)
    await statusBar.expectStartButtonEnabled();
  });

  test("Scenario 18: Edit running flow → Blocked with message", async ({
    page,
  }) => {
    // Navigate to flow list
    const flowList = new FlowListPage(page);
    await flowList.goto();
    await page.waitForLoadState("networkidle");

    // Create and start a flow
    await flowList.clickCreateNewFlow();
    await page.waitForURL(/\/flows\/.+/);

    const canvas = new FlowCanvasPage(page);
    await canvas.expectCanvasLoaded();

    // Add udp component (valid minimal flow - will deploy with warning for no subscribers)
    const palette = new ComponentPalettePage(page);
    await palette.addComponentToCanvas("UDP Input");

    // Save and deploy
    await canvas.clickSaveButton();
    await page.waitForTimeout(1000);

    const statusBar = new StatusBarPage(page);
    await statusBar.clickDeploy();
    await page.waitForTimeout(2000);

    // Start flow
    await statusBar.clickStart();
    await page.waitForTimeout(2000);
    await statusBar.expectRuntimeState("running");

    // Try to add a new component while running
    // This should be blocked or show a warning message
    await palette.addComponentToCanvas("Object Store");

    // Verify error message or blocking behavior
    // Implementation may vary - could show:
    // - Alert dialog
    // - Toast notification
    // - Status bar message
    // - Component doesn't appear on canvas
    await statusBar.expectStatusMessage("Cannot edit running flow");

    // Alternative: Verify component wasn't added
    // await expect(canvas.nodes).toHaveCount(2); // Still only 2 nodes

    // Alternative: Verify canvas is in read-only mode
    // const canvasElement = canvas.canvas;
    // await expect(canvasElement).toHaveAttribute('data-readonly', 'true');
  });
});
