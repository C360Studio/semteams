import type { Page, Locator } from "@playwright/test";

/**
 * Page Object for NavigationDialog component
 * Handles interactions with the custom navigation warning dialog
 */
export class NavigationDialogPage {
  readonly page: Page;
  readonly dialog: Locator;
  readonly title: Locator;
  readonly message: Locator;
  readonly saveButton: Locator;
  readonly discardButton: Locator;
  readonly cancelButton: Locator;

  constructor(page: Page) {
    this.page = page;
    this.dialog = page.locator('[role="dialog"]');
    this.title = this.dialog.locator("h2");
    this.message = this.dialog.locator("p");
    this.saveButton = this.dialog.getByRole("button", {
      name: /save changes/i,
    });
    this.discardButton = this.dialog.getByRole("button", {
      name: /discard changes/i,
    });
    this.cancelButton = this.dialog.getByRole("button", { name: /^cancel$/i });
  }

  /**
   * Wait for dialog to appear
   */
  async waitForDialog(timeout = 5000): Promise<void> {
    await this.dialog.waitFor({ state: "visible", timeout });
  }

  /**
   * Check if dialog is visible
   */
  async isVisible(): Promise<boolean> {
    return await this.dialog.isVisible();
  }

  /**
   * Get dialog message text
   */
  async getMessage(): Promise<string> {
    return (await this.message.textContent()) || "";
  }

  /**
   * Click "Save Changes" button
   */
  async clickSave(): Promise<void> {
    await this.saveButton.click();
  }

  /**
   * Click "Discard Changes" button
   */
  async clickDiscard(): Promise<void> {
    await this.discardButton.click();
  }

  /**
   * Click "Cancel" button
   */
  async clickCancel(): Promise<void> {
    await this.cancelButton.click();
  }

  /**
   * Press Escape key to close dialog
   */
  async pressEscape(): Promise<void> {
    await this.page.keyboard.press("Escape");
  }
}
