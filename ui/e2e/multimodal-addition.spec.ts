import { expect, test } from "@playwright/test";
import { FlowCanvasPage } from "./pages/FlowCanvasPage";
import { ComponentPalettePage } from "./pages/ComponentPalettePage";
import { FlowListPage } from "./pages/FlowListPage";

/**
 * Multi-Modal Component Addition Tests
 * Verifies double-click and keyboard component addition works
 */

test.describe("Multi-Modal Component Addition", () => {
  test("Double-click adds component to canvas", async ({ page }) => {
    // Navigate to flow list and create new flow
    const flowList = new FlowListPage(page);
    await flowList.goto();
    await page.waitForLoadState("networkidle");

    await flowList.clickCreateNewFlow();
    await page.waitForURL(/\/flows\/.+/);

    const canvas = new FlowCanvasPage(page);
    await canvas.expectCanvasLoaded();

    // Verify canvas starts empty
    await expect(canvas.nodes).toHaveCount(0);

    // Add component via double-click
    const palette = new ComponentPalettePage(page);
    await palette.addComponentToCanvas("UDP Input");

    // Verify node appears
    await expect(canvas.nodes).toHaveCount(1);

    // Add second component
    await palette.addComponentToCanvas("iot_sensor");

    // Verify both nodes appear
    await expect(canvas.nodes).toHaveCount(2);
  });

  test("Keyboard (Enter) adds component to canvas", async ({ page }) => {
    const flowList = new FlowListPage(page);
    await flowList.goto();
    await page.waitForLoadState("networkidle");

    await flowList.clickCreateNewFlow();
    await page.waitForURL(/\/flows\/.+/);

    const canvas = new FlowCanvasPage(page);
    await canvas.expectCanvasLoaded();

    await expect(canvas.nodes).toHaveCount(0);

    // Add component via keyboard
    const palette = new ComponentPalettePage(page);
    await palette.addComponentToCanvasViaKeyboard("UDP Input");

    // Verify node appears
    await expect(canvas.nodes).toHaveCount(1);
  });

  test("Added components persist after save", async ({ page }) => {
    const flowList = new FlowListPage(page);
    await flowList.goto();
    await page.waitForLoadState("networkidle");

    await flowList.clickCreateNewFlow();
    await page.waitForURL(/\/flows\/.+/);
    const flowUrl = page.url();

    const canvas = new FlowCanvasPage(page);
    await canvas.expectCanvasLoaded();

    // Add components
    const palette = new ComponentPalettePage(page);
    await palette.addComponentToCanvas("UDP Input");
    await palette.addComponentToCanvas("iot_sensor");

    await expect(canvas.nodes).toHaveCount(2);

    // Save
    await canvas.clickSaveButton();

    // Navigate away and back
    await page.goto("/");
    await page.waitForLoadState("networkidle");

    // Return to flow
    await page.goto(flowUrl);
    await page.waitForLoadState("networkidle");

    // Verify nodes persisted
    await expect(canvas.nodes).toHaveCount(2);
  });
});
