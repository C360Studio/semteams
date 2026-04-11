import { test, expect } from "@playwright/test";
import { FlowListPage } from "./pages/FlowListPage";
import { FlowCanvasPage } from "./pages/FlowCanvasPage";
import { ComponentPalettePage } from "./pages/ComponentPalettePage";
import { ConfigPanelPage } from "./pages/ConfigPanelPage";

/**
 * Port Configuration E2E Tests (Schema Tag System)
 *
 * Tests the new port-based configuration architecture where components
 * use PortConfigEditor instead of flat fields (port, bind, subject).
 *
 * After the D3 canvas refactor, configuration is done via EditComponentModal
 * accessed by clicking the Edit (⚙️) button in the sidebar.
 *
 * Tests udp component which has:
 * - Ports field (type:ports)
 * - Input ports: Network UDP socket configuration
 * - Output ports: NATS subject for publishing
 */

test.describe.skip("Port Configuration (Schema Tag System)", () => {
  let flowList: FlowListPage;
  let canvas: FlowCanvasPage;
  let palette: ComponentPalettePage;
  let configPanel: ConfigPanelPage;

  test.beforeEach(async ({ page }) => {
    // Initialize Page Object Models
    flowList = new FlowListPage(page);
    canvas = new FlowCanvasPage(page);
    palette = new ComponentPalettePage(page);
    configPanel = new ConfigPanelPage(page);

    // Navigate to flow list and create new flow
    await flowList.goto();
    await page.waitForLoadState("networkidle");

    // Create a new flow
    await flowList.clickCreateNewFlow();
    await page.waitForURL(/\/flows\/.+/);

    // Wait for canvas to load
    await canvas.expectCanvasLoaded();
  });

  test("Scenario 1: Open UDP component → PortConfigEditor visible", async ({
    page,
  }) => {
    // Add UDP component to canvas
    await palette.addComponentToCanvas("UDP Input");
    await page.waitForTimeout(500);

    // Click Edit button in sidebar to open config modal
    const nodeName = await canvas.getFirstNodeName();
    await canvas.clickEditButton(nodeName);

    // Wait for config modal
    await configPanel.expectPanelVisible();
    await configPanel.expectComponentTitle("UDP Input");

    // Should display PortConfigEditor (not flat fields)
    await configPanel.expectPortConfigEditorVisible();

    // Should have sections for input and output ports
    await expect(page.locator('h4:has-text("Input Ports")')).toBeVisible();
    await expect(page.locator('h4:has-text("Output Ports")')).toBeVisible();

    // Should have add buttons
    await expect(configPanel.addInputPortButton).toBeVisible();
    await expect(configPanel.addOutputPortButton).toBeVisible();
  });

  test("Scenario 2: Add input port → Port appears in list", async ({
    page,
  }) => {
    // Add and configure UDP component
    await palette.addComponentToCanvas("UDP Input");
    await page.waitForTimeout(500);

    // Click Edit button in sidebar to open config modal
    const nodeName = await canvas.getFirstNodeName();
    await canvas.clickEditButton(nodeName);
    await configPanel.expectPanelVisible();

    // Initially should have no ports (or empty state)
    await configPanel.expectEmptyPortsMessage("input");

    // Add an input port
    await configPanel.clickAddInputPort();

    // Should now have 1 input port
    await configPanel.expectPortCount("input", 1);

    // Port should have editable fields visible
    await configPanel.expectPortFieldVisible("input", 0, "subject");
  });

  test("Scenario 3: Add output port → Port appears in list", async ({
    page,
  }) => {
    // Add and configure UDP component
    await palette.addComponentToCanvas("UDP Input");
    await page.waitForTimeout(500);

    // Click Edit button in sidebar to open config modal
    const nodeName = await canvas.getFirstNodeName();
    await canvas.clickEditButton(nodeName);
    await configPanel.expectPanelVisible();

    // Initially should have no output ports
    await configPanel.expectEmptyPortsMessage("output");

    // Add an output port
    await configPanel.clickAddOutputPort();

    // Should now have 1 output port
    await configPanel.expectPortCount("output", 1);

    // Port should have editable fields visible
    await configPanel.expectPortFieldVisible("output", 0, "subject");
  });

  test("Scenario 4: Edit port subject → Value updates", async ({ page }) => {
    // Add and configure UDP component
    await palette.addComponentToCanvas("UDP Input");
    await page.waitForTimeout(500);

    // Click Edit button in sidebar to open config modal
    const nodeName = await canvas.getFirstNodeName();
    await canvas.clickEditButton(nodeName);
    await configPanel.expectPanelVisible();

    // Add an output port
    await configPanel.clickAddOutputPort();
    await configPanel.expectPortCount("output", 1);

    // Edit the subject field
    await configPanel.fillPortField(
      "output",
      0,
      "subject",
      "events.udp.custom",
    );

    // Wait for input to update
    await page.waitForTimeout(100);

    // Verify the value persisted
    await configPanel.expectPortFieldValue(
      "output",
      0,
      "subject",
      "events.udp.custom",
    );
  });

  test("Scenario 5: Remove port → Port disappears from list", async ({
    page,
  }) => {
    // Add and configure UDP component
    await palette.addComponentToCanvas("UDP Input");
    await page.waitForTimeout(500);

    // Click Edit button in sidebar to open config modal
    const nodeName = await canvas.getFirstNodeName();
    await canvas.clickEditButton(nodeName);
    await configPanel.expectPanelVisible();

    // Add two output ports
    await configPanel.clickAddOutputPort();
    await configPanel.clickAddOutputPort();
    await configPanel.expectPortCount("output", 2);

    // Remove the first port
    await configPanel.removePort("output", 0);

    // Should now have 1 port
    await configPanel.expectPortCount("output", 1);
  });

  test("Scenario 6: Configure ports → Save → Values persist", async ({
    page,
  }) => {
    // Add and configure UDP component
    await palette.addComponentToCanvas("UDP Input");
    await page.waitForTimeout(500);

    // Click Edit button in sidebar to open config modal
    const nodeName = await canvas.getFirstNodeName();
    await canvas.clickEditButton(nodeName);
    await configPanel.expectPanelVisible();

    // Add and configure an output port
    await configPanel.clickAddOutputPort();
    await configPanel.fillPortField(
      "output",
      0,
      "subject",
      "events.test.output",
    );

    // Save configuration
    await configPanel.clickSave();

    // Modal should close
    await page.waitForTimeout(500);

    // Flow should be marked dirty
    await canvas.expectSaveStatus("dirty");

    // Reopen config modal to verify persistence
    await canvas.clickEditButton(nodeName);
    await configPanel.expectPanelVisible();

    // Port configuration should be persisted
    await configPanel.expectPortCount("output", 1);
    await configPanel.expectPortFieldValue(
      "output",
      0,
      "subject",
      "events.test.output",
    );
  });

  test("Scenario 7: Multiple ports → All editable independently", async ({
    page,
  }) => {
    // Add and configure UDP component
    await palette.addComponentToCanvas("UDP Input");
    await page.waitForTimeout(500);

    // Click Edit button in sidebar to open config modal
    const nodeName = await canvas.getFirstNodeName();
    await canvas.clickEditButton(nodeName);
    await configPanel.expectPanelVisible();

    // Add multiple output ports
    await configPanel.clickAddOutputPort();
    await configPanel.clickAddOutputPort();
    await configPanel.clickAddOutputPort();
    await configPanel.expectPortCount("output", 3);

    // Edit each port with different subject
    await configPanel.fillPortField("output", 0, "subject", "events.port1");
    await configPanel.fillPortField("output", 1, "subject", "events.port2");
    await configPanel.fillPortField("output", 2, "subject", "events.port3");

    // Verify all values are independent
    await configPanel.expectPortFieldValue(
      "output",
      0,
      "subject",
      "events.port1",
    );
    await configPanel.expectPortFieldValue(
      "output",
      1,
      "subject",
      "events.port2",
    );
    await configPanel.expectPortFieldValue(
      "output",
      2,
      "subject",
      "events.port3",
    );
  });

  test("Scenario 8: Full user journey with port configuration", async ({
    page,
  }) => {
    // 1. Add UDP component
    await palette.addComponentToCanvas("UDP Input");
    await page.waitForTimeout(500);

    // 2. Open configuration via Edit button
    const nodeName = await canvas.getFirstNodeName();
    await canvas.clickEditButton(nodeName);
    await configPanel.expectPanelVisible();

    // 3. Add and configure input port (UDP socket)
    await configPanel.clickAddInputPort();
    await configPanel.fillPortField(
      "input",
      0,
      "subject",
      "udp://0.0.0.0:14550",
    );

    // 4. Add and configure output port (NATS subject)
    await configPanel.clickAddOutputPort();
    await configPanel.fillPortField(
      "output",
      0,
      "subject",
      "telemetry.mavlink",
    );

    // 5. Save configuration
    await configPanel.clickSave();
    await page.waitForTimeout(500);

    // 6. Verify flow is dirty (needs saving)
    await canvas.expectSaveStatus("dirty");

    // 7. Save the flow
    await canvas.clickSaveButton();
    await page.waitForTimeout(1000);

    // 8. Verify flow is now clean
    await canvas.expectSaveStatus("clean");

    // 9. Reopen config to verify everything persisted
    await canvas.clickEditButton(nodeName);
    await configPanel.expectPanelVisible();

    // 10. Verify input port persisted
    await configPanel.expectPortCount("input", 1);
    await configPanel.expectPortFieldValue(
      "input",
      0,
      "subject",
      "udp://0.0.0.0:14550",
    );

    // 11. Verify output port persisted
    await configPanel.expectPortCount("output", 1);
    await configPanel.expectPortFieldValue(
      "output",
      0,
      "subject",
      "telemetry.mavlink",
    );
  });

  test("Scenario 9: Cancel without saving → Changes discarded", async ({
    page,
  }) => {
    // Add UDP component
    await palette.addComponentToCanvas("UDP Input");
    await page.waitForTimeout(500);

    // Click Edit button in sidebar to open config modal
    const nodeName = await canvas.getFirstNodeName();
    await canvas.clickEditButton(nodeName);
    await configPanel.expectPanelVisible();

    // Add a port
    await configPanel.clickAddOutputPort();
    await configPanel.fillPortField("output", 0, "subject", "events.temporary");

    // Cancel without saving
    await configPanel.clickCancel();
    await page.waitForTimeout(500);

    // Reopen config modal
    await canvas.clickEditButton(nodeName);
    await configPanel.expectPanelVisible();

    // Changes should be discarded - no ports configured
    await configPanel.expectEmptyPortsMessage("output");
  });

  test("Scenario 10: Schema shows ports type (not flat fields)", async ({
    page,
  }) => {
    // Add UDP component
    await palette.addComponentToCanvas("UDP Input");
    await page.waitForTimeout(500);

    // Click Edit button in sidebar to open config modal
    const nodeName = await canvas.getFirstNodeName();
    await canvas.clickEditButton(nodeName);
    await configPanel.expectPanelVisible();

    // Should display schema-driven form
    await configPanel.expectSchemaFormVisible();

    // Should display PortConfigEditor (type:ports)
    await configPanel.expectPortConfigEditorVisible();

    // Should NOT have flat fields like 'port', 'bind', 'subject'
    const flatPortField = page.locator("input#port");
    await expect(flatPortField).not.toBeVisible();

    const flatBindField = page.locator("input#bind");
    await expect(flatBindField).not.toBeVisible();

    const flatSubjectField = page.locator("input#subject");
    await expect(flatSubjectField).not.toBeVisible();
  });
});
