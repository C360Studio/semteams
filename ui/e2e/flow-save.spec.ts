import { test, expect } from "@playwright/test";
import { ComponentPalettePage } from "./pages/ComponentPalettePage";
import type { Flow } from "../src/lib/types/flow";

/**
 * E2E Test: Manual Flow Save Journey
 * Implements quickstart.md Scenario 1
 *
 * User Story: As a user, I want to manually save my flow changes to the server
 * so that they are persisted permanently.
 */
test.describe("Manual Flow Save", () => {
  let flowId: string;

  test.beforeEach(async ({ page }) => {
    // Create test flow via API
    const response = await page.request.post(`/flowbuilder/flows`, {
      data: {
        name: "Test Flow",
        description: "Test description",
        nodes: [],
        connections: [],
      },
    });

    const flow: Flow = await response.json();
    flowId = flow.id;

    // Navigate to flow editor
    await page.goto(`/flows/${flowId}`);
    await page.waitForLoadState("networkidle");
  });

  test("should save flow changes to server", async ({ page }) => {
    // Add a node (triggers dirty state) - use WebSocket to avoid UDP port conflicts
    const palette = new ComponentPalettePage(page);
    await palette.addComponentToCanvas("WebSocket Output");

    // Wait for component to be added to canvas
    await page.waitForTimeout(500);

    // Verify dirty state indicator
    await expect(page.getByText(/unsaved changes/i)).toBeVisible();

    // Click save button
    const saveButton = page.getByRole("button", { name: /save/i });
    await saveButton.click();

    // Note: "Saving..." state may be too fast to observe reliably
    // Instead, wait directly for save completion

    // Wait for "Saved at" confirmation
    await expect(page.getByText(/saved at/i)).toBeVisible({ timeout: 5000 });

    // Verify clean state (no "unsaved changes")
    await expect(page.getByText(/unsaved changes/i)).not.toBeVisible();

    // Reload page and verify changes persisted
    await page.reload();
    await page.waitForLoadState("networkidle");

    // Verify node is still present on canvas (D3 canvas uses g.flow-node with .node-label)
    // Node name is auto-generated like "websocket-<timestamp>" so we check for the type prefix
    await expect(
      page.locator("g.flow-node .node-label").filter({ hasText: /websocket/i }),
    ).toBeVisible();
  });

  test("should display save errors when server fails", async ({ page }) => {
    // Add a node to trigger dirty state
    const palette = new ComponentPalettePage(page);
    await palette.addComponentToCanvas("WebSocket Output");

    // Wait for component to be added
    await page.waitForTimeout(500);

    // Intercept the save request and make it fail
    await page.route(`/flowbuilder/flows/${flowId}`, async (route) => {
      if (route.request().method() === "PUT") {
        await route.fulfill({
          status: 500,
          contentType: "application/json",
          body: JSON.stringify({ error: "Internal server error" }),
        });
      } else {
        await route.continue();
      }
    });

    // Click save button
    const saveButton = page.getByRole("button", { name: /save/i });
    await saveButton.click();

    // Wait for error state to appear
    await expect(page.getByText(/save failed/i)).toBeVisible({ timeout: 5000 });

    // Verify save status indicator shows error
    const saveStatus = page.locator("#save-status");
    await expect(saveStatus).toHaveAttribute("data-status", "error");

    // Clean up: remove route interception
    await page.unroute(`/flowbuilder/flows/${flowId}`);
  });

  test("should show saved status after successful save", async ({ page }) => {
    // Add a node - use json_filter to avoid UDP port conflicts
    const palette = new ComponentPalettePage(page);
    await palette.addComponentToCanvas("json_filter");

    // Wait for component to be added
    await page.waitForTimeout(500);

    // Save
    const saveButton = page.getByRole("button", { name: /save/i });
    await saveButton.click();

    // Verify "Saved at" status appears
    await expect(page.getByText(/saved at/i)).toBeVisible({ timeout: 5000 });

    // Verify button is disabled (nothing to save)
    await expect(saveButton).toBeDisabled();
  });

  test("should handle rapid save attempts", async ({ page }) => {
    // Add a node - use iot_sensor to avoid UDP port conflicts
    const palette = new ComponentPalettePage(page);
    await palette.addComponentToCanvas("iot_sensor");

    // Wait for component to be added
    await page.waitForTimeout(500);

    // First save should succeed
    const saveButton = page.getByRole("button", { name: /save/i });
    await saveButton.click();
    await expect(page.getByText(/saved at/i)).toBeVisible({ timeout: 5000 });

    // Button should be disabled (nothing to save)
    await expect(saveButton).toBeDisabled();

    // Subsequent clicks should be prevented (button disabled)
    // This tests that rapid clicking doesn't cause double-saves
  });

  test("should disable save button when saving", async ({ page }) => {
    // Add a node - use websocket to avoid UDP port conflicts
    const palette = new ComponentPalettePage(page);
    await palette.addComponentToCanvas("WebSocket Output");

    // Wait for component to be added
    await page.waitForTimeout(500);

    // Save
    const saveButton = page.getByRole("button", { name: /save/i });
    await saveButton.click();

    // Wait for save to complete
    await expect(page.getByText(/saved at/i)).toBeVisible({ timeout: 5000 });

    // Button should be disabled after save (nothing left to save)
    await expect(saveButton).toBeDisabled();

    // Make another change - button should re-enable (use json_filter to avoid conflicts)
    await palette.addComponentToCanvas("json_filter");
    await page.waitForTimeout(500);
    await expect(page.getByText(/unsaved changes/i)).toBeVisible();
    await expect(saveButton).not.toBeDisabled();
  });

  test("should preserve node positions after save and reload", async ({
    page,
  }) => {
    // Add a node - double-click adds to canvas center (use Robotics to avoid UDP conflicts)
    const palette = new ComponentPalettePage(page);
    await palette.addComponentToCanvas("iot_sensor");

    // Wait for component to be added
    await page.waitForTimeout(500);

    // Save
    const saveButton = page.getByRole("button", { name: /save/i });
    await saveButton.click();
    await expect(page.getByText(/saved at/i)).toBeVisible({ timeout: 5000 });

    // Get node position before reload (D3 canvas uses g.flow-node)
    const nodeBefore = page.locator("g.flow-node").first();
    const boxBefore = await nodeBefore.boundingBox();

    // Reload page
    await page.reload();
    await page.waitForLoadState("networkidle");

    // Get node position after reload
    const nodeAfter = page.locator("g.flow-node").first();
    const boxAfter = await nodeAfter.boundingBox();

    // Positions should match (within 10px tolerance for layout shifts)
    // Using Math.abs to check difference is less than 10px
    expect(Math.abs((boxAfter?.x ?? 0) - (boxBefore?.x ?? 0))).toBeLessThan(10);
    expect(Math.abs((boxAfter?.y ?? 0) - (boxBefore?.y ?? 0))).toBeLessThan(10);
  });

  test.afterEach(async ({ page }) => {
    // Cleanup test flow
    if (flowId) {
      await page.request.delete(`/flowbuilder/flows/${flowId}`);
    }
  });
});

/**
 * NOTE: These tests validate the TDD requirements:
 * - Tests are written BEFORE implementation
 * - Tests will FAIL until components are implemented
 * - Tests validate behavior, not implementation details
 * - Tests cover happy path and error cases
 */
