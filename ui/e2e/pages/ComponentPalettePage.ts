import { Page, Locator, expect } from "@playwright/test";

/**
 * Page Object Model for the Component Palette
 *
 * After the D3 canvas refactor, the palette is now embedded in the AddComponentModal.
 * The flow is:
 * 1. Click "+ Add" button in sidebar (opens modal)
 * 2. Select component type from palette in modal
 * 3. Configure and click "Add Component"
 */
export class ComponentPalettePage {
  constructor(private page: Page) {}

  // Locators - palette is inside modal dialog
  get modal(): Locator {
    return this.page.locator('[role="dialog"]');
  }

  get palette(): Locator {
    return this.modal.locator(".component-palette");
  }

  get componentCards(): Locator {
    return this.modal.locator(".component-card");
  }

  get categories(): Locator {
    return this.modal.locator(".category");
  }

  get searchInput(): Locator {
    return this.modal.locator('input[placeholder*="Search"]');
  }

  get addButton(): Locator {
    return this.page.locator('button:has-text("+ Add")');
  }

  get addComponentButton(): Locator {
    return this.modal.locator('button:has-text("Add Component")');
  }

  // Actions
  getComponentByName(name: string): Locator {
    return this.modal.locator(`.component-card:has-text("${name}")`);
  }

  getCategoryByName(name: string): Locator {
    // Match category header specifically to avoid matching component card text
    return this.modal.locator(`.category-header:has-text("${name}")`);
  }

  /**
   * Open the Add Component modal
   */
  async openAddModal(): Promise<void> {
    await this.addButton.click();
    await expect(this.modal).toBeVisible();
    await expect(this.palette).toBeVisible();
  }

  async clickComponent(name: string): Promise<void> {
    const component = this.getComponentByName(name);
    await component.click();
  }

  /**
   * Add component to canvas via the modal flow
   * Opens modal, selects component type via double-click, configures, and confirms
   */
  async addComponentToCanvas(componentName: string): Promise<void> {
    // Step 1: Open the Add Component modal
    await this.openAddModal();

    // Step 2: Double-click on the component card to select type
    // ComponentPalette triggers onAddComponent via double-click or Enter
    const component = this.getComponentByName(componentName);
    await expect(component).toBeVisible({ timeout: 5000 });
    await component.dblclick();

    // Step 3: Wait for config form and click Add Component
    await expect(this.addComponentButton).toBeVisible({ timeout: 2000 });
    await this.addComponentButton.click();

    // Step 4: Wait for modal to close
    await expect(this.modal).toBeHidden({ timeout: 2000 });
  }

  /**
   * Add component to canvas via keyboard (accessibility testing)
   * Opens modal and uses keyboard to select and add
   */
  async addComponentToCanvasViaKeyboard(componentName: string): Promise<void> {
    // Open modal
    await this.addButton.focus();
    await this.page.keyboard.press("Enter");
    await expect(this.modal).toBeVisible();

    // Find and select component with keyboard
    const component = this.getComponentByName(componentName);
    await component.focus();
    await this.page.keyboard.press("Enter");

    // Confirm with keyboard
    await this.addComponentButton.focus();
    await this.page.keyboard.press("Enter");

    // Wait for modal to close
    await expect(this.modal).toBeHidden({ timeout: 2000 });
  }

  async searchComponent(query: string): Promise<void> {
    await this.searchInput.fill(query);
  }

  async clickCategory(name: string): Promise<void> {
    const category = this.getCategoryByName(name);
    await category.click();
  }

  // Assertions
  async expectPaletteVisible(): Promise<void> {
    await expect(this.palette).toBeVisible();
  }

  async expectComponentInPalette(name: string): Promise<void> {
    const component = this.getComponentByName(name);
    await expect(component).toBeVisible();
  }

  async expectComponentCount(count: number): Promise<void> {
    await expect(this.componentCards).toHaveCount(count);
  }

  async expectCategoryVisible(name: string): Promise<void> {
    const category = this.getCategoryByName(name);
    await expect(category).toBeVisible();
  }

  async expectCategoryExpanded(name: string): Promise<void> {
    const category = this.getCategoryByName(name);
    await expect(category).toHaveAttribute("aria-expanded", "true");
  }
}
