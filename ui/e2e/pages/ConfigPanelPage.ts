import { Page, Locator, expect } from "@playwright/test";

/**
 * Page Object Model for the Component Configuration
 *
 * After the D3 canvas refactor, configuration is now in EditComponentModal.
 * The flow is:
 * 1. Click Edit button (⚙️) in sidebar component list
 * 2. EditComponentModal opens
 * 3. Configure and click Save
 */
export class ConfigPanelPage {
  constructor(private page: Page) {}

  // Locators - config is now in EditComponentModal dialog
  get panel(): Locator {
    return this.page.locator('[role="dialog"]');
  }

  get dialogTitle(): Locator {
    return this.panel.locator("#dialog-title");
  }

  get closeButton(): Locator {
    return this.panel.locator('button[aria-label="Close dialog"]');
  }

  get saveButton(): Locator {
    // EditComponentModal uses just "Save" button
    return this.panel.locator('button:has-text("Save")');
  }

  get cancelButton(): Locator {
    return this.panel.locator('button:has-text("Cancel")');
  }

  get deleteButton(): Locator {
    return this.panel.locator('button:has-text("Delete")');
  }

  get nameInput(): Locator {
    return this.panel.locator('input[name="name"]');
  }

  // Config section in modal
  get configSection(): Locator {
    return this.panel.locator(".config-section");
  }

  // Schema form locators (inside modal)
  get schemaForm(): Locator {
    return this.panel.locator("form");
  }

  get basicConfigSection(): Locator {
    return this.panel.locator(".config-section");
  }

  // Get form field by name (matches ID attribute or name attribute)
  getFormField(fieldName: string): Locator {
    return this.panel.locator(
      `[id="config.${fieldName}"], [name="config.${fieldName}"]`,
    );
  }

  getFieldLabel(fieldName: string): Locator {
    return this.panel.locator(`label[for="config.${fieldName}"]`);
  }

  // Actions
  async fillFormField(fieldName: string, value: string): Promise<void> {
    const field = this.getFormField(fieldName);
    await field.fill(value);
  }

  async fillName(value: string): Promise<void> {
    await this.nameInput.fill(value);
  }

  async clickSave(): Promise<void> {
    await this.saveButton.click();
  }

  async clickCancel(): Promise<void> {
    await this.cancelButton.click();
  }

  async clickClose(): Promise<void> {
    await this.closeButton.click();
  }

  async getFieldValue(fieldName: string): Promise<string> {
    const field = this.getFormField(fieldName);
    return await field.inputValue();
  }

  // Assertions
  async expectPanelVisible(): Promise<void> {
    await expect(this.panel).toBeVisible({ timeout: 5000 });
  }

  async expectPanelHidden(): Promise<void> {
    await expect(this.panel).not.toBeVisible();
  }

  async expectComponentTitle(typeOrName: string): Promise<void> {
    // EditComponentModal title is "Edit: {name}" format
    // Can match either the full title or just a substring
    await expect(this.dialogTitle).toContainText(typeOrName);
  }

  async expectSchemaFormVisible(): Promise<void> {
    await expect(this.schemaForm).toBeVisible();
  }

  async expectFieldVisible(fieldName: string): Promise<void> {
    const field = this.getFormField(fieldName);
    await expect(field).toBeVisible();
  }

  async expectFieldValue(
    fieldName: string,
    expectedValue: string,
  ): Promise<void> {
    const field = this.getFormField(fieldName);
    await expect(field).toHaveValue(expectedValue);
  }

  async expectSaveEnabled(): Promise<void> {
    await expect(this.saveButton).toBeEnabled();
  }

  async expectSaveDisabled(): Promise<void> {
    await expect(this.saveButton).toBeDisabled();
  }

  async expectBasicSectionVisible(): Promise<void> {
    await expect(this.basicConfigSection).toBeVisible();
  }

  // ==========================================
  // PortConfigEditor Support Methods
  // ==========================================

  // Locators for PortConfigEditor (inside modal)
  get portConfigEditor(): Locator {
    return this.panel.locator(".port-config-editor");
  }

  get addInputPortButton(): Locator {
    return this.panel.locator('button:has-text("Add Input Port")');
  }

  get addOutputPortButton(): Locator {
    return this.panel.locator('button:has-text("Add Output Port")');
  }

  getPortSection(direction: "input" | "output"): Locator {
    const heading = direction === "input" ? "Input Ports" : "Output Ports";
    return this.panel.locator(`.port-section:has(h4:has-text("${heading}"))`);
  }

  getPortItem(direction: "input" | "output", index: number): Locator {
    const section = this.getPortSection(direction);
    return section.locator(".port-item").nth(index);
  }

  getPortFieldInput(
    direction: "input" | "output",
    portIndex: number,
    fieldName: string,
  ): Locator {
    // ID format: "ports-input-0-subject" or "ports-output-0-subject"
    return this.panel.locator(`#ports-${direction}-${portIndex}-${fieldName}`);
  }

  getRemovePortButton(direction: "input" | "output", index: number): Locator {
    const portItem = this.getPortItem(direction, index);
    return portItem.locator('button:has-text("Remove")');
  }

  // Actions for PortConfigEditor
  async clickAddInputPort(): Promise<void> {
    await this.addInputPortButton.click();
  }

  async clickAddOutputPort(): Promise<void> {
    await this.addOutputPortButton.click();
  }

  async fillPortField(
    direction: "input" | "output",
    portIndex: number,
    fieldName: string,
    value: string,
  ): Promise<void> {
    const field = this.getPortFieldInput(direction, portIndex, fieldName);
    await field.fill(value);
  }

  async removePort(
    direction: "input" | "output",
    portIndex: number,
  ): Promise<void> {
    const removeButton = this.getRemovePortButton(direction, portIndex);
    await removeButton.click();
  }

  async getPortFieldValue(
    direction: "input" | "output",
    portIndex: number,
    fieldName: string,
  ): Promise<string> {
    const field = this.getPortFieldInput(direction, portIndex, fieldName);
    return await field.inputValue();
  }

  // Assertions for PortConfigEditor
  async expectPortConfigEditorVisible(): Promise<void> {
    await expect(this.portConfigEditor).toBeVisible();
  }

  async expectPortCount(
    direction: "input" | "output",
    count: number,
  ): Promise<void> {
    const section = this.getPortSection(direction);
    const portItems = section.locator(".port-item");
    await expect(portItems).toHaveCount(count);
  }

  async expectPortFieldValue(
    direction: "input" | "output",
    portIndex: number,
    fieldName: string,
    expectedValue: string,
  ): Promise<void> {
    const field = this.getPortFieldInput(direction, portIndex, fieldName);
    await expect(field).toHaveValue(expectedValue);
  }

  async expectPortFieldVisible(
    direction: "input" | "output",
    portIndex: number,
    fieldName: string,
  ): Promise<void> {
    const field = this.getPortFieldInput(direction, portIndex, fieldName);
    await expect(field).toBeVisible();
  }

  async expectEmptyPortsMessage(direction: "input" | "output"): Promise<void> {
    const section = this.getPortSection(direction);
    const emptyState = section.locator(".empty-state");
    await expect(emptyState).toBeVisible();
    const text =
      direction === "input"
        ? "No input ports configured"
        : "No output ports configured";
    await expect(emptyState).toContainText(text);
  }
}
