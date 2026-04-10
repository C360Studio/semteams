import { expect, test } from "@playwright/test";
import { FlowCanvasPage } from "./pages/FlowCanvasPage";
import { ComponentPalettePage } from "./pages/ComponentPalettePage";
import { FlowListPage } from "./pages/FlowListPage";

/**
 * Component Palette E2E Tests
 * Tests scenarios 4-5 from spec.md
 */

test.describe("Component Palette", () => {
  test("Scenario 4: Open Add modal → See component palette with component types", async ({
    page,
  }) => {
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

    // Open the Add Component modal to see the palette
    const palette = new ComponentPalettePage(page);
    await palette.openAddModal();

    // Verify component palette is visible inside modal
    await palette.expectPaletteVisible();

    // Verify key component types are present (backend returns 22+ types)
    // Input types
    await palette.expectComponentInPalette("UDP Input");

    // Output types
    await palette.expectComponentInPalette("WebSocket Output");

    // Storage types
    await palette.expectComponentInPalette("Object Store");

    // Verify we have multiple component types loaded
    const componentCount = await palette.componentCards.count();
    expect(componentCount).toBeGreaterThanOrEqual(7);
  });

  test("Scenario 5: Open Add modal → See Input, Output, Processor, Storage categories", async ({
    page,
  }) => {
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

    // Open the Add Component modal to see the palette
    const palette = new ComponentPalettePage(page);
    await palette.openAddModal();

    // Verify palette is visible
    await palette.expectPaletteVisible();

    // Verify all 4 categories are present (categories are lowercase in HTML)
    await palette.expectCategoryVisible("input");
    await palette.expectCategoryVisible("output");
    await palette.expectCategoryVisible("processor");
    await palette.expectCategoryVisible("storage");
  });
});
