import { test, expect } from "@playwright/test";
import { createTestFlow, deleteTestFlow } from "./helpers/flow-setup";
import { ComponentPalettePage } from "./pages/ComponentPalettePage";

/**
 * E2E Tests for Flow Validation UX Improvements
 * Feature 015: Improve Flow Validation UX
 *
 * Tests validate:
 * - Blank canvas shows backend validation result
 * - Validation status appears in top-right only
 * - Validation detail modal functionality
 * - Modal dismissal methods
 * - Runtime vs validation status separation
 */

test.describe("Flow Validation UX", () => {
  // Track created flows for cleanup
  let testFlowIds: string[] = [];

  // Cleanup after each test
  test.afterEach(async ({ page }) => {
    // Delete all flows created during test
    for (const flowId of testFlowIds) {
      await deleteTestFlow(page, flowId);
    }
    testFlowIds = [];
  });

  /**
   * T002: Blank canvas shows backend validation result (invalid - "Draft - 1 error")
   * Backend correctly identifies empty flows as invalid (cannot be deployed)
   */
  test("blank canvas shows backend validation result", async ({ page }) => {
    // Create empty flow
    const { url, id } = await createTestFlow(page, {
      name: "E2E Test - Blank Canvas Validation",
    });
    testFlowIds.push(id);

    // Navigate to flow
    await page.goto(url);
    await page.waitForLoadState("networkidle");

    // Wait for canvas to be visible
    await expect(page.locator("#flow-canvas")).toBeVisible();

    // Wait for validation to run (debounced)
    await page.waitForTimeout(700);

    // Verify top-right validation status shows backend result (invalid)
    const validationStatus = page.locator('[data-testid="validation-status"]');
    await expect(validationStatus).toBeVisible();
    await expect(validationStatus).toContainText("Draft");
    await expect(validationStatus).toContainText("error");

    // Verify status has error class
    await expect(validationStatus).toHaveClass(/error/);

    // Verify bottom status bar does NOT show validation status
    const statusBar = page.locator('[data-testid="status-bar"]');
    await expect(statusBar).not.toContainText("Draft");
    await expect(statusBar).not.toContainText("Valid");
  });

  /**
   * T003: Validation status appears in top-right only (not bottom bar)
   */
  test("validation status in top-right only", async ({ page }) => {
    // Create empty flow
    const { url, id } = await createTestFlow(page, {
      name: "E2E Test - Validation Status Location",
    });
    testFlowIds.push(id);

    await page.goto(url);
    await page.waitForLoadState("networkidle");

    // Wait for canvas to be visible
    await expect(page.locator("#flow-canvas")).toBeVisible();

    // Add unconfigured component to create validation error
    const palette = new ComponentPalettePage(page);
    await palette.addComponentToCanvas("UDP Input");

    // Wait for validation debounce (500ms + buffer)
    await page.waitForTimeout(700);

    // Verify top-right shows validation warnings
    const validationStatus = page.locator('[data-testid="validation-status"]');
    await expect(validationStatus).toBeVisible();
    await expect(validationStatus).toContainText("warning");

    // Verify bottom bar shows ONLY runtime status
    const statusBar = page.locator('[data-testid="status-bar"]');
    await expect(statusBar).toBeVisible();
    await expect(statusBar).toContainText(/Not Deployed|deployed/i);

    // Critical: Bottom bar should NOT show validation status
    await expect(statusBar).not.toContainText("Draft");
    await expect(statusBar).not.toContainText("Valid");
  });

  /**
   * T004: Clicking validation status opens detail modal
   */
  test("clicking validation status opens detail modal", async ({ page }) => {
    // Create empty flow
    const { url, id } = await createTestFlow(page, {
      name: "E2E Test - Validation Modal",
    });
    testFlowIds.push(id);

    await page.goto(url);

    // Wait for page load and add component with errors
    await page.waitForLoadState("networkidle");
    await expect(page.locator("#flow-canvas")).toBeVisible();
    const palette = new ComponentPalettePage(page);
    await palette.addComponentToCanvas("UDP Input");
    await page.waitForTimeout(700); // Validation debounce

    // Click validation status indicator
    const validationStatus = page.getByTestId("validation-status");
    await validationStatus.click();

    // Verify modal opens
    const modal = page.getByRole("dialog", { name: /validation/i });
    await expect(modal).toBeVisible();

    // Verify modal content structure (warnings for unconnected component)
    await expect(modal).toContainText(/warning/i);
    await expect(modal).toContainText(/udp|component/i);

    // Verify modal has backdrop
    const backdrop = page.locator('[data-testid="modal-backdrop"]');
    await expect(backdrop).toBeVisible();
  });

  /**
   * T005: Modal dismisses via click-outside and ESC key
   */
  test("modal dismisses via click-outside and ESC", async ({ page }) => {
    // Create empty flow
    const { url, id } = await createTestFlow(page, {
      name: "E2E Test - Modal Dismissal",
    });
    testFlowIds.push(id);

    await page.goto(url);
    await page.waitForLoadState("networkidle");

    // Wait for canvas to be visible
    await expect(page.locator("#flow-canvas")).toBeVisible();
    const palette = new ComponentPalettePage(page);
    await palette.addComponentToCanvas("UDP Input");
    await page.waitForTimeout(700);
    await page.getByTestId("validation-status").click();

    const modal = page.getByRole("dialog");
    await expect(modal).toBeVisible();

    // Test 1: Click outside (backdrop) dismisses modal
    await page
      .locator('[data-testid="modal-backdrop"]')
      .click({ position: { x: 10, y: 10 } });
    await expect(modal).not.toBeVisible();

    // Re-open modal for next test
    await page.getByTestId("validation-status").click();
    await expect(modal).toBeVisible();

    // Test 2: ESC key dismisses modal
    await page.keyboard.press("Escape");
    await expect(modal).not.toBeVisible();

    // Test 3: Focus returns to trigger element
    await page.getByTestId("validation-status").click();
    await page.keyboard.press("Escape");
    await expect(page.getByTestId("validation-status")).toBeFocused();
  });

  /**
   * T006: Modal shows correct component name for port warnings
   * Verifies component name resolution from port names
   */
  test("modal shows correct component name for warnings", async ({ page }) => {
    // Create empty flow
    const { url, id } = await createTestFlow(page, {
      name: "E2E Test - Component Name Resolution",
    });
    testFlowIds.push(id);

    await page.goto(url);
    await page.waitForLoadState("networkidle");

    // Wait for canvas to be visible
    await expect(page.locator("#flow-canvas")).toBeVisible();

    const palette = new ComponentPalettePage(page);

    // Add udp component
    await palette.addComponentToCanvas("UDP Input");

    // Add iot_sensor component (should create warning for unconnected port)
    await palette.addComponentToCanvas("iot_sensor");

    // Wait for validation to run
    await page.waitForTimeout(700);

    // Click validation status to open modal
    const validationStatus = page.getByTestId("validation-status");
    await validationStatus.click();

    // Verify modal opens
    const modal = page.getByRole("dialog", { name: /validation/i });
    await expect(modal).toBeVisible();

    // Critical: Modal should show component name (not "Flow") for port warnings
    // Look for the iot_sensor component name in the modal
    await expect(modal).toContainText(/iot_sensor/i);

    // Should NOT show "Flow" for component-specific port warnings
    // (Only flow-level issues should show "Flow")
    const modalText = await modal.textContent();
    const hasFlowLevelError = modalText?.includes(
      "flow must have at least one component",
    );
    if (!hasFlowLevelError) {
      // If there's no flow-level error, there should be no standalone "Flow" label
      const flowLabels = modalText?.match(/^Flow$/gm) || [];
      expect(flowLabels.length).toBe(0);
    }
  });

  /**
   * T007: Runtime status separate from validation status
   */
  test("runtime status separate from validation status", async ({ page }) => {
    // Create empty flow
    const { url, id } = await createTestFlow(page, {
      name: "E2E Test - Runtime vs Validation Status",
    });
    testFlowIds.push(id);

    await page.goto(url);
    await page.waitForLoadState("networkidle");

    // Wait for canvas to be visible
    await expect(page.locator("#flow-canvas")).toBeVisible();

    // Add component to create validation error
    const palette = new ComponentPalettePage(page);
    await palette.addComponentToCanvas("UDP Input");
    await page.waitForTimeout(700);

    // Verify top-right shows validation status
    const validationStatus = page.getByTestId("validation-status");
    await expect(validationStatus).toBeVisible();
    await expect(validationStatus).toContainText(/valid|draft|warning/i);

    // Verify bottom bar shows ONLY runtime status
    const statusBar = page.getByTestId("status-bar");
    await expect(statusBar).toBeVisible();
    await expect(statusBar).toContainText(
      /not deployed|deployed|running|stopped/i,
    );

    // Verify no validation terms in bottom bar
    await expect(statusBar).not.toContainText("draft");
    await expect(statusBar).not.toContainText("valid");
    await expect(statusBar).not.toContainText("error"); // validation errors

    // Verify visual distinction (different CSS classes)
    const validationClass = await validationStatus.getAttribute("class");
    const statusBarClass = await statusBar.getAttribute("class");
    expect(validationClass).not.toBe(statusBarClass);
  });
});
