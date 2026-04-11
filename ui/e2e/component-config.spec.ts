import { test } from "@playwright/test";
import { FlowCanvasPage } from "./pages/FlowCanvasPage";
import { ComponentPalettePage } from "./pages/ComponentPalettePage";
import { ConfigPanelPage } from "./pages/ConfigPanelPage";
import { FlowListPage } from "./pages/FlowListPage";

/**
 * Component Configuration E2E Tests
 *
 * IMPORTANT: These tests verify the Edit button and EditComponentModal opening.
 * After the D3 canvas refactor, configuration is done via EditComponentModal
 * accessed by clicking the Edit (⚙️) button in the sidebar.
 *
 * For port-based configuration tests, see port-configuration.spec.ts
 */

test.describe("Component Configuration", () => {
  test("Scenario: Click Edit button → Config modal opens", async ({ page }) => {
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

    // Add a component to the canvas
    const palette = new ComponentPalettePage(page);
    await palette.addComponentToCanvas("UDP Input");
    await page.waitForTimeout(500);

    // Verify config panel is initially hidden
    const configPanel = new ConfigPanelPage(page);
    await configPanel.expectPanelHidden();

    // Get the node name from the sidebar and click its edit button
    const nodeName = await canvas.getFirstNodeName();
    await canvas.clickEditButton(nodeName);

    // Verify config modal opens
    await configPanel.expectPanelVisible();

    // Verify modal shows component info (title contains the type name)
    await configPanel.expectComponentTitle("udp");
  });

  // NOTE: Scenarios 10 & 11 removed - they tested flat fields (port, bind, subject)
  // which were replaced by PortConfigEditor in the schema tag system migration.
  // See port-configuration.spec.ts for port-based configuration tests.
});
