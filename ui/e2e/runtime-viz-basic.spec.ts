import { expect, test } from "@playwright/test";
import {
  createRunningFlow,
  deleteTestFlow,
  getCanvasHeight,
  waitForRuntimePanel,
} from "./helpers/runtime-helpers";

/**
 * Runtime Visualization Panel - Basic Functionality Tests
 *
 * IMPORTANT: Read docs/testing/E2E_INFRASTRUCTURE.md before modifying
 *
 * These tests run against a REAL backend via Docker (see Taskfile.yml)
 * - Do NOT mock /flowbuilder/* API calls
 * - Backend runs in Docker automatically (Playwright manages it)
 * - Use: task test:e2e:semstreams
 *
 * Runtime Visualization Panel - Basic Functionality Tests
 * Phase 1: Panel open/close interactions and layout behavior
 *
 * Tests:
 * 1. Toggle panel open/closed from StatusBar button
 * 2. Close panel with Esc key
 * 3. Close panel with X button
 * 4. Canvas height adjusts when panel opens
 * 5. Canvas height restores when panel closes
 */

test.describe("Runtime Panel - Basic Functionality", () => {
  let flowId: string;

  test.beforeEach(async ({ page }) => {
    // Create and start a flow for runtime testing
    const setup = await createRunningFlow(page);
    flowId = setup.flowId;

    // Navigate to flow page
    await page.goto(setup.url);

    // Wait for canvas to be visible and stable
    await expect(page.locator("#flow-canvas")).toBeVisible();
    await page.waitForLoadState("networkidle");
  });

  test.afterEach(async ({ page }) => {
    // Clean up test flow
    if (flowId) {
      await deleteTestFlow(page, flowId);
    }
  });

  test("should toggle runtime panel open/closed from StatusBar button", async ({
    page,
  }) => {
    // Verify panel is initially closed
    await expect(
      page.locator('[data-testid="runtime-panel"]'),
    ).not.toBeVisible();

    // Verify debug button is visible (only shows when flow is running)
    const debugButton = page.locator('[data-testid="debug-toggle-button"]');
    await expect(debugButton).toBeVisible();
    await expect(debugButton).toContainText("Debug");

    // Click debug button to open panel
    await debugButton.click();

    // Panel should slide up and be visible
    await waitForRuntimePanel(page);
    await expect(page.locator('[data-testid="runtime-panel"]')).toBeVisible();

    // Button should show "down" arrow when open
    await expect(debugButton).toContainText("▼");

    // Click again to close panel
    await debugButton.click();

    // Panel should close
    await expect(
      page.locator('[data-testid="runtime-panel"]'),
    ).not.toBeVisible();

    // Button should show "up" arrow when closed
    await expect(debugButton).toContainText("▲");
  });

  test("should close panel with Esc key", async ({ page }) => {
    // Open the panel
    await page.click('[data-testid="debug-toggle-button"]');
    await waitForRuntimePanel(page);

    // Panel should be visible
    await expect(page.locator('[data-testid="runtime-panel"]')).toBeVisible();

    // Press Escape key
    await page.keyboard.press("Escape");

    // Wait a moment for the close handler
    await page.waitForTimeout(100);

    // Panel should close
    await expect(
      page.locator('[data-testid="runtime-panel"]'),
    ).not.toBeVisible();
  });

  test("should close panel with X button", async ({ page }) => {
    // Open the panel
    await page.click('[data-testid="debug-toggle-button"]');
    await waitForRuntimePanel(page);

    // Panel should be visible
    await expect(page.locator('[data-testid="runtime-panel"]')).toBeVisible();

    // Click close button (X in header)
    const closeButton = page.locator(".runtime-panel .close-button");
    await expect(closeButton).toBeVisible();
    await closeButton.click();

    // Panel should close
    await expect(
      page.locator('[data-testid="runtime-panel"]'),
    ).not.toBeVisible();
  });

  test.skip("should adjust canvas height when panel opens", async ({
    page,
  }) => {
    // SKIPPED: Canvas height adjustment not yet implemented.
    // The runtime panel currently appears below the canvas without resizing it.
    // This test should be enabled once the layout is updated to resize canvas on panel open.

    // Get initial canvas height (before panel opens)
    const initialHeight = await getCanvasHeight(page);
    expect(initialHeight).toBeGreaterThan(0);

    // Open runtime panel
    await page.click('[data-testid="debug-toggle-button"]');
    await waitForRuntimePanel(page);

    // Wait for layout to settle
    await page.waitForTimeout(400);

    // Get new canvas height (after panel opens)
    const newHeight = await getCanvasHeight(page);

    // Canvas should be shorter (panel takes up bottom space)
    expect(newHeight).toBeLessThan(initialHeight);

    // Difference should be approximately the panel height (default 300px)
    // Allow some margin for browser rendering differences
    const heightDifference = initialHeight - newHeight;
    expect(heightDifference).toBeGreaterThan(250);
    expect(heightDifference).toBeLessThan(350);
  });

  test.skip("should restore canvas height when panel closes", async ({
    page,
  }) => {
    // SKIPPED: Canvas height adjustment not yet implemented.
    // See above test for details.

    // Get initial canvas height
    const initialHeight = await getCanvasHeight(page);

    // Open panel
    await page.click('[data-testid="debug-toggle-button"]');
    await waitForRuntimePanel(page);
    await page.waitForTimeout(400);

    // Verify canvas is shorter
    const reducedHeight = await getCanvasHeight(page);
    expect(reducedHeight).toBeLessThan(initialHeight);

    // Close panel
    await page.click('[data-testid="debug-toggle-button"]');
    await page.waitForTimeout(400);

    // Get final canvas height
    const finalHeight = await getCanvasHeight(page);

    // Canvas should return to original height (within small margin)
    expect(Math.abs(finalHeight - initialHeight)).toBeLessThan(10);
  });
});
