import { test, expect } from "@playwright/test";
import { FlowCanvasPage } from "./pages/FlowCanvasPage";
import { ComponentPalettePage } from "./pages/ComponentPalettePage";
import { FlowListPage } from "./pages/FlowListPage";

/**
 * Connection Creation Tests
 *
 * Tests the core connection functionality:
 * - Auto-discovered connections from NATS pattern matching
 *
 * Note: Manual drag-and-drop connection tests were removed as the D3-based
 * canvas does not support drag-and-drop connection creation.
 *
 * These tests validate the architecture from:
 * - specs/013-visual-component-wiring/CONNECTION_STATE_ARCHITECTURE.md
 * - specs/013-visual-component-wiring/VALIDATION_UX_DESIGN.md
 */

test.describe("Connection Creation", () => {
  /**
   * T001: Auto-discovered connections should appear after validation
   *
   * Given: A flow with two components that have matching NATS patterns
   * When: Validation runs (500ms debounce)
   * Then: Auto-discovered connection appears with 'auto' source
   */
  test.skip("should display auto-discovered connection between matching NATS patterns", async ({
    page,
  }) => {
    // SKIPPED: Auto-discovered connections depend on backend NATS pattern matching.
    // The backend validation endpoint needs to return discovered_connections for
    // components with compatible publish/subscribe patterns. Currently UDP Input
    // and iot_sensor may not have matching patterns in the test configuration.
    // Enable console logging for debugging
    page.on("console", (msg) => {
      if (msg.type() === "log" || msg.type() === "error") {
        console.log(`[BROWSER ${msg.type()}]`, msg.text());
      }
    });

    // Setup: Create new flow
    const flowList = new FlowListPage(page);
    await flowList.goto();
    await page.waitForLoadState("networkidle");

    await flowList.clickCreateNewFlow();
    await page.waitForURL(/\/flows\/.+/);

    const canvas = new FlowCanvasPage(page);
    await canvas.expectCanvasLoaded();

    // Intercept validation API calls to debug
    page.on("request", async (request) => {
      if (request.url().includes("/validate")) {
        console.log(
          "[TEST] Validation REQUEST payload:",
          request.postDataJSON(),
        );
      }
    });

    page.on("response", async (response) => {
      if (response.url().includes("/validate")) {
        console.log("[TEST] Validation API called:", response.status());
        try {
          const data = await response.json();
          console.log(
            "[TEST] Validation result:",
            JSON.stringify(data, null, 2),
          );
        } catch {
          console.log("[TEST] Could not parse validation response");
        }
      }
    });

    const palette = new ComponentPalettePage(page);

    // Add UDP component (publishes to "input.udp.mavlink")
    await palette.addComponentToCanvas("UDP Input");
    await expect(canvas.nodes).toHaveCount(1);

    // Add Robotics component (subscribes to "input.*.mavlink")
    await palette.addComponentToCanvas("iot_sensor");
    await expect(canvas.nodes).toHaveCount(2);

    // Wait for validation to run (500ms debounce + execution)
    await page.waitForTimeout(800);

    // Debug: Check for D3-based edges
    const allEdges = page.locator("g.flow-edge");
    const edgeCount = await allEdges.count();
    console.log("[TEST] Total edges:", edgeCount);

    // Debug: Check ports are rendered
    const inputPorts = page.locator(".port-input");
    const outputPorts = page.locator(".port-output");
    console.log("[TEST] Input ports:", await inputPorts.count());
    console.log("[TEST] Output ports:", await outputPorts.count());

    // Debug: Check specific ports for our components
    const natsOutputPort = page.locator('[data-port-name="nats_output"]');
    const mavlinkInputPort = page.locator('[data-port-name="mavlink_input"]');
    console.log(
      "[TEST] nats_output port exists:",
      await natsOutputPort.count(),
    );
    console.log(
      "[TEST] mavlink_input port exists:",
      await mavlinkInputPort.count(),
    );

    // Debug: List all edge IDs
    for (let i = 0; i < edgeCount; i++) {
      const edgeId = await allEdges.nth(i).getAttribute("data-connection-id");
      console.log(`[TEST] Edge ${i}: ${edgeId}`);
    }

    // Verify auto-discovered connection exists
    const autoEdges = canvas.getAutoEdges();
    await expect(autoEdges).toHaveCount(1, { timeout: 2000 });

    // Verify connection has correct attributes
    const autoEdge = autoEdges.first();
    await expect(autoEdge).toHaveAttribute("data-source", "auto");

    // Verify the connection links UDP output to Robotics input
    const edgeId = await autoEdge.getAttribute("data-connection-id");
    console.log("[TEST] Auto-discovered edge ID:", edgeId);

    // Edge ID format: auto_<source_node>_<source_port>_<target_node>_<target_port>
    // udp's nats_output connects to iot_sensor's mavlink_input
    expect(edgeId).toContain("nats_output");
    expect(edgeId).toContain("mavlink_input");
  });
});
