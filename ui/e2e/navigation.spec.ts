import { expect, test } from "@playwright/test";
import { FlowCanvasPage } from "./pages/FlowCanvasPage";
import { ComponentPalettePage } from "./pages/ComponentPalettePage";
import { FlowListPage } from "./pages/FlowListPage";
import { NavigationDialogPage } from "./pages/NavigationDialogPage";

/**
 * Navigation E2E Tests
 * Tests scenarios 19-20 from spec.md
 *
 * Tests SvelteKit SSR and client-side routing
 */

test.describe("Navigation and State Persistence", () => {
  test("Scenario 19: Click back button → Return to flow list", async ({
    page,
  }) => {
    // Navigate to flow list
    const flowList = new FlowListPage(page);
    await flowList.goto();
    await page.waitForLoadState("networkidle");

    // Verify we're on the flow list page
    await expect(flowList.flowList).toBeVisible();

    // Create new flow
    await flowList.clickCreateNewFlow();
    await page.waitForURL(/\/flows\/.+/);

    // Verify we're on the canvas page
    const canvas = new FlowCanvasPage(page);
    await canvas.expectCanvasLoaded();

    // Add a component (to verify state is not accidentally saved) - use json_filter
    const palette = new ComponentPalettePage(page);
    await palette.addComponentToCanvas("json_filter");
    await page.waitForTimeout(500);

    // DON'T save the flow

    // Click back button
    await canvas.clickBackButton();

    // Navigation guard should show dialog
    const dialog = new NavigationDialogPage(page);
    await dialog.waitForDialog();

    // Discard changes to allow navigation
    await dialog.clickDiscard();

    // Wait for navigation back to flow list
    await page.waitForURL("/");

    // Verify we're back on flow list page
    await expect(flowList.flowList).toBeVisible();
    await expect(flowList.createButton).toBeVisible();

    // Verify URL is correct
    expect(page.url()).toMatch(/\/$|\/$/);
  });

  test("Scenario 20: Reload page → Flow state preserved", async ({ page }) => {
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

    // Add multiple components (avoid UDP to prevent port conflicts)
    const palette = new ComponentPalettePage(page);
    await palette.addComponentToCanvas("WebSocket Output");
    await expect(canvas.nodes).toHaveCount(1);
    await palette.addComponentToCanvas("iot_sensor");
    await expect(canvas.nodes).toHaveCount(2);
    await palette.addComponentToCanvas("json_filter");
    await expect(canvas.nodes).toHaveCount(3);

    // Save the flow and wait for explicit "saved at" confirmation
    await canvas.clickSaveButton();

    // Wait for "saved at" to appear (ensures save fully completed and persisted to NATS KV)
    await expect(page.getByText(/saved at/i)).toBeVisible({ timeout: 5000 });

    // Verify save status is draft (components have no connections, so validation finds orphaned ports)
    await canvas.expectSaveStatus("draft");

    // Capture current URL
    const flowUrl = page.url();
    const flowId = flowUrl.match(/\/flows\/([^/]+)/)?.[1];
    expect(flowId).toBeTruthy();

    // Reload the page
    await page.reload();
    await page.waitForLoadState("networkidle");

    // Verify we're still on the same flow
    expect(page.url()).toBe(flowUrl);

    // Verify canvas loads
    await canvas.expectCanvasLoaded();

    // Verify all 3 components are still present
    await expect(canvas.nodes).toHaveCount(3);

    // Verify component types are preserved (use actual type IDs, not display names)
    const wsNode = canvas.getNodeByType("websocket");
    await expect(wsNode).toBeVisible();

    const iotNode = canvas.getNodeByType("iot_sensor");
    await expect(iotNode).toBeVisible();

    const jsonNode = canvas.getNodeByType("json_filter");
    await expect(jsonNode).toBeVisible();

    // Verify save status is draft (flow persisted, but has validation errors from orphaned ports)
    await canvas.expectSaveStatus("draft");
  });

  test("Scenario 20 (variant): Direct URL navigation → Flow loads correctly", async ({
    page,
  }) => {
    // First create a flow to get a valid ID
    const flowList = new FlowListPage(page);
    await flowList.goto();
    await page.waitForLoadState("networkidle");

    await flowList.clickCreateNewFlow();
    await page.waitForURL(/\/flows\/.+/);

    const canvas = new FlowCanvasPage(page);
    await canvas.expectCanvasLoaded();

    // Add a component and save
    const palette = new ComponentPalettePage(page);
    await palette.addComponentToCanvas("WebSocket Output");

    // Wait for component to be added to canvas
    await page.waitForTimeout(500);

    // Save and wait for completion
    await canvas.clickSaveButton();
    await expect(page.getByText(/saved at/i)).toBeVisible({ timeout: 5000 });

    // Capture the flow ID
    const flowUrl = page.url();
    const flowId = flowUrl.match(/\/flows\/([^/]+)/)?.[1];
    expect(flowId).toBeTruthy();

    // Navigate away
    await canvas.clickBackButton();
    await page.waitForURL("/");

    // Navigate directly to the flow using URL
    await page.goto(`/flows/${flowId}`);
    await page.waitForLoadState("networkidle");

    // Verify canvas loads with the saved component
    await canvas.expectCanvasLoaded();
    await expect(canvas.nodes).toHaveCount(1);

    const wsNode = canvas.getNodeByType("websocket");
    await expect(wsNode).toBeVisible();
  });
});
