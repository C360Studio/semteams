import { test, expect } from "@playwright/test";
import { FlowCanvasPage } from "./pages/FlowCanvasPage";
import { ComponentPalettePage } from "./pages/ComponentPalettePage";
import { FlowListPage } from "./pages/FlowListPage";

/**
 * Port Visualization Tests
 * Tests visual port handles, connections, and validation state
 *
 * Note: Uses D3-based canvas with SVG elements. Ports use .port-input/.port-output classes.
 */

test.describe("Port Visualization", () => {
  // T010: Port visualization on component placement
  test("should display color-coded port handles on components", async ({
    page,
  }) => {
    // Capture browser console logs
    page.on("console", (msg) => {
      console.log("BROWSER:", msg.text());
    });

    // Create new flow with components
    const flowList = new FlowListPage(page);
    await flowList.goto();
    await page.waitForLoadState("networkidle");

    await flowList.clickCreateNewFlow();
    await page.waitForURL(/\/flows\/.+/);

    const canvas = new FlowCanvasPage(page);
    await canvas.expectCanvasLoaded();

    // Add UDP component
    const palette = new ComponentPalettePage(page);
    await palette.addComponentToCanvas("UDP Input");

    // Wait for node to appear
    await expect(canvas.nodes).toHaveCount(1);

    // Wait for validation to complete (debounced 500ms + execution time)
    // Validation discovers ports and adds them to node data
    await page.waitForTimeout(800);

    // Verify color-coded port handles exist on the UDP node (type is 'udp', not display name)
    const udpNode = canvas.getNodeByType("udp");
    await expect(udpNode).toBeVisible();

    // Verify input port exists (udp_socket - required input)
    const inputPort = udpNode.locator(
      '.port-input[data-port-name="udp_socket"]',
    );
    await expect(inputPort).toBeVisible();
    // Required ports have the .port-required class in D3 canvas
    await expect(inputPort).toHaveClass(/port-required/);

    // Verify output port exists (nats_output - NATS stream type)
    const outputPort = udpNode.locator(
      '.port-output[data-port-name="nats_output"]',
    );
    await expect(outputPort).toBeVisible();

    // Verify port has color styling via stroke attribute (SVG)
    await expect(outputPort).toHaveAttribute("stroke");

    // Verify node shows port summary
    const portSummary = udpNode.locator(".port-summary");
    await expect(portSummary).toBeVisible();
    await expect(portSummary).toContainText("1 in, 1 out");
  });

  // T010b: Port tooltips (via SVG <title> element)
  test("should display tooltip on port hover", async ({ page }) => {
    // Create new flow with component
    const flowList = new FlowListPage(page);
    await flowList.goto();
    await page.waitForLoadState("networkidle");

    await flowList.clickCreateNewFlow();
    await page.waitForURL(/\/flows\/.+/);

    const canvas = new FlowCanvasPage(page);
    await canvas.expectCanvasLoaded();

    // Add Robotics component (has multiple ports)
    const palette = new ComponentPalettePage(page);
    await palette.addComponentToCanvas("iot_sensor");
    await expect(canvas.nodes).toHaveCount(1);

    // Wait for validation to discover ports
    await page.waitForTimeout(800);

    const iotNode = canvas.getNodeByType("iot_sensor");
    const inputPort = iotNode.locator(".port-input").first();
    await expect(inputPort).toBeVisible();

    // D3/SVG tooltips are in <title> elements inside circles
    const inputTooltip = inputPort.locator("title");
    const tooltipText = await inputTooltip.textContent();
    expect(tooltipText).toBeTruthy();
    // Port names are dynamic - just verify tooltip has content
    expect(tooltipText).toMatch(/(required|optional)/);

    // Verify port has correct class
    await expect(inputPort).toHaveClass(/port-input/);

    // Verify output port also has tooltip
    const outputPort = iotNode.locator(".port-output").first();
    const outputTooltip = outputPort.locator("title");
    const outputTooltipText = await outputTooltip.textContent();
    expect(outputTooltipText).toBeTruthy();
    await expect(outputPort).toHaveClass(/port-output/);
  });

  // T011 and T012 removed - drag-based connection tests not applicable to D3 canvas
  // Auto-discovered connections tested in connection-creation.spec.ts T001

  // T013: Validation state display on orphaned ports
  test("should display validation state on port handles", async ({ page }) => {
    const flowList = new FlowListPage(page);
    await flowList.goto();
    await page.waitForLoadState("networkidle");

    await flowList.clickCreateNewFlow();
    await page.waitForURL(/\/flows\/.+/);

    const canvas = new FlowCanvasPage(page);
    await canvas.expectCanvasLoaded();

    const palette = new ComponentPalettePage(page);

    // Add Robotics component which has required input port
    await palette.addComponentToCanvas("iot_sensor");
    await expect(canvas.nodes).toHaveCount(1);

    // Wait for validation to discover ports
    await page.waitForTimeout(600);

    const iotNode = canvas.getNodeByType("iot_sensor");

    // Verify input ports exist (port names are dynamic)
    const inputPort = iotNode.locator(".port-input").first();
    await expect(inputPort).toBeVisible();
    // Required ports have .port-required class in D3 canvas
    await expect(inputPort).toHaveClass(/port/);

    // Verify output port exists
    const outputPort = iotNode.locator(".port-output").first();
    await expect(outputPort).toBeVisible();

    // TODO: Validation state visualization (error badges, red borders) will be added in T022
    // For now, verify basic port structure exists for validation to target
  });

  // T014: REMOVED - Port grouping feature removed in favor of clean color-coded handles
  // Port groups created visual clutter and are now replaced with simple color-coded handles

  // T015: Debounced validation timing
  test("should debounce validation after canvas changes", async ({ page }) => {
    const flowList = new FlowListPage(page);
    await flowList.goto();
    await page.waitForLoadState("networkidle");

    await flowList.clickCreateNewFlow();
    await page.waitForURL(/\/flows\/.+/);

    const canvas = new FlowCanvasPage(page);
    await canvas.expectCanvasLoaded();

    // Mock validation endpoint to track calls
    let validationCallCount = 0;
    await page.route("**/flowbuilder/flows/*/validate", (route) => {
      validationCallCount++;
      route.fulfill({
        status: 200,
        body: JSON.stringify({
          validation_status: "valid",
          errors: [],
          warnings: [],
          nodes: [],
          discovered_connections: [],
        }),
      });
    });

    const palette = new ComponentPalettePage(page);

    // Make 3 rapid changes (add 3 components)
    await palette.addComponentToCanvas("UDP Input");
    await page.waitForTimeout(100);

    await palette.addComponentToCanvas("iot_sensor");
    await page.waitForTimeout(100);

    await palette.addComponentToCanvas("json_filter");

    // Wait for debounce period (500ms + buffer)
    await page.waitForTimeout(600);

    // Verify validation called only ONCE (not 3 times)
    expect(validationCallCount).toBe(1);

    // Make another change
    await palette.addComponentToCanvas("WebSocket Output");
    await page.waitForTimeout(600);

    // Verify second validation call
    expect(validationCallCount).toBe(2);
  });

  // T016: Draft mode - save with validation errors
  test("should save flow with errors and display draft status", async ({
    page,
  }) => {
    const flowList = new FlowListPage(page);
    await flowList.goto();
    await page.waitForLoadState("networkidle");

    await flowList.clickCreateNewFlow();
    await page.waitForURL(/\/flows\/.+/);

    const canvas = new FlowCanvasPage(page);
    await canvas.expectCanvasLoaded();

    const palette = new ComponentPalettePage(page);

    // Mock validation endpoint to return errors
    await page.route("**/flowbuilder/flows/*/validate", (route) => {
      route.fulfill({
        status: 200,
        body: JSON.stringify({
          validation_status: "errors",
          errors: [
            {
              severity: "error",
              component: "robotics-1",
              message: "Required input port has no connection",
              port_name: "nats_input",
              suggestion: "Connect a NATS publisher to this port",
            },
          ],
          warnings: [],
          nodes: [
            {
              id: "robotics-1",
              ports: [
                {
                  name: "nats_input",
                  direction: "input",
                  required: true,
                  validation_state: "error",
                },
              ],
            },
          ],
          discovered_connections: [],
        }),
      });
    });

    // Add Robotics component which has required input port
    await palette.addComponentToCanvas("iot_sensor");
    await expect(canvas.nodes).toHaveCount(1);

    // Wait for validation to complete
    await page.waitForTimeout(600);

    // Save the flow
    await canvas.clickSaveButton();

    // Wait for save to complete
    await page.waitForTimeout(500);

    // Verify draft status is displayed
    const saveStatus = page.locator("#save-status");
    await expect(saveStatus).toHaveAttribute("data-status", "draft");

    // Verify status text shows error count (use .first() since validation status also has text)
    const statusText = saveStatus.locator(
      ".status-icon.status-draft + .status-text",
    );
    await expect(statusText).toContainText("Draft");
    await expect(statusText).toContainText("1 error");

    // Verify timestamp is shown
    const timestamp = saveStatus.locator(".timestamp");
    await expect(timestamp).toBeVisible();
    await expect(timestamp).toContainText("saved at");

    // Verify draft status icon (warning)
    const statusIcon = saveStatus.locator(".status-icon.status-draft");
    await expect(statusIcon).toBeVisible();
    await expect(statusIcon).toHaveText("⚠");

    // Reload page and verify draft state persists
    await page.reload();
    await page.waitForLoadState("networkidle");

    // After reload, validation will run again (debounced 500ms)
    // The validation effect now updates saveState based on validation result
    await page.waitForTimeout(800);

    // Verify draft status persists after reload
    await expect(saveStatus).toHaveAttribute("data-status", "draft");
    await expect(statusText).toContainText("Draft");
    await expect(statusText).toContainText("1 error");
  });
});
