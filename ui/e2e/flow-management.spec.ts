import { expect, test } from "@playwright/test";
import { FlowListPage } from "./pages/FlowListPage";
import { FlowCanvasPage } from "./pages/FlowCanvasPage";

/**
 * Flow Management E2E Tests
 * Tests scenarios 1-3 from spec.md
 */

test.describe("Flow Management", () => {
  test("Scenario 1: Load homepage and see flow list", async ({ page }) => {
    const flowList = new FlowListPage(page);

    await flowList.goto();
    await page.waitForLoadState("networkidle");

    await expect(flowList.flowList).toBeVisible();
    await expect(flowList.createButton).toBeVisible();
  });

  test('Scenario 2: Click "Create New Flow" and navigate to canvas', async ({
    page,
  }) => {
    const flowList = new FlowListPage(page);

    await flowList.goto();
    await page.waitForLoadState("networkidle");

    // Click create button and wait for SvelteKit client-side navigation
    await flowList.clickCreateNewFlow();
    await page.waitForURL(/\/flows\/.+/);

    // Verify we're on the canvas page
    const canvas = new FlowCanvasPage(page);
    await expect(canvas.backButton).toBeVisible();
    await canvas.expectCanvasLoaded();
  });

  test("Scenario 3: Click back button returns to flow list", async ({
    page,
  }) => {
    const flowList = new FlowListPage(page);
    const canvas = new FlowCanvasPage(page);

    await flowList.goto();
    await page.waitForLoadState("networkidle");

    // Create a flow
    await flowList.clickCreateNewFlow();
    await page.waitForURL(/\/flows\/.+/);

    // Verify we're on canvas page
    await canvas.expectCanvasLoaded();

    // Click back button
    await canvas.clickBackButton();
    await page.waitForURL("/");

    // Should see flow list again
    await expect(flowList.flowList).toBeVisible();
    await expect(flowList.createButton).toBeVisible();
  });
});
