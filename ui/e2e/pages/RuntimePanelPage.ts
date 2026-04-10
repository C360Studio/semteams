import { Page, Locator, expect } from "@playwright/test";

/**
 * Page Object Model for the Runtime Panel
 * Encapsulates interactions with the debug/runtime panel and its tabs
 * (Logs, Metrics, Health, Messages).
 */
export class RuntimePanelPage {
  constructor(private page: Page) {}

  // ---------------------------------------------------------------------------
  // Panel-level locators
  // ---------------------------------------------------------------------------

  get panel(): Locator {
    return this.page.locator('[data-testid="runtime-panel"]');
  }

  get debugToggle(): Locator {
    return this.page.locator('[data-testid="debug-toggle-button"]');
  }

  get closeButton(): Locator {
    return this.page.locator(".runtime-panel .close-button");
  }

  // ---------------------------------------------------------------------------
  // Tab button locators
  // ---------------------------------------------------------------------------

  get logsTab(): Locator {
    return this.page.locator('[data-testid="tab-logs"]');
  }

  get metricsTab(): Locator {
    return this.page.locator('[data-testid="tab-metrics"]');
  }

  get healthTab(): Locator {
    return this.page.locator('[data-testid="tab-health"]');
  }

  get messagesTab(): Locator {
    return this.page.locator('[data-testid="tab-messages"]');
  }

  // ---------------------------------------------------------------------------
  // Tab content panel locators
  // ---------------------------------------------------------------------------

  get logsPanel(): Locator {
    return this.page.locator('[data-testid="logs-panel"]');
  }

  get metricsPanel(): Locator {
    return this.page.locator('[data-testid="metrics-panel"]');
  }

  get healthPanel(): Locator {
    return this.page.locator('[data-testid="health-panel"]');
  }

  get messagesPanel(): Locator {
    return this.page.locator('[data-testid="messages-panel"]');
  }

  // ---------------------------------------------------------------------------
  // Action methods
  // ---------------------------------------------------------------------------

  /**
   * Open the runtime panel by clicking the debug toggle button, then wait for
   * the panel to become visible and for the slide-up animation to complete.
   */
  async open(): Promise<void> {
    await this.debugToggle.click();
    await this.waitForPanel();
  }

  /**
   * Close the runtime panel by clicking the X close button.
   */
  async close(): Promise<void> {
    await this.closeButton.click();
    await expect(this.panel).not.toBeVisible();
  }

  /**
   * Wait for the runtime panel to be visible.
   * Also waits for the 300ms slide-up animation defined in RuntimePanel.svelte.
   */
  async waitForPanel(timeout: number = 5000): Promise<void> {
    await expect(this.panel).toBeVisible({ timeout });
    // Allow the slide-up animation (300ms) to complete before interacting
    await this.page.waitForTimeout(350);
  }

  /**
   * Click one of the named tabs and wait for its content panel to appear.
   */
  async switchToTab(
    name: "logs" | "metrics" | "health" | "messages",
  ): Promise<void> {
    await this.page.click(`[data-testid="tab-${name}"]`);
    // Allow Svelte reactivity to update the rendered tab content
    await this.page.waitForTimeout(100);
    await expect(
      this.page.locator(`[data-testid="${name}-panel"]`),
    ).toBeVisible();
  }

  /**
   * Return whether the runtime panel is currently visible.
   */
  async isOpen(): Promise<boolean> {
    return await this.panel.isVisible().catch(() => false);
  }

  // ---------------------------------------------------------------------------
  // Assertion helpers
  // ---------------------------------------------------------------------------

  async expectPanelVisible(): Promise<void> {
    await expect(this.panel).toBeVisible();
  }

  async expectPanelHidden(): Promise<void> {
    await expect(this.panel).not.toBeVisible();
  }

  async expectLogsPanelVisible(): Promise<void> {
    await expect(this.logsPanel).toBeVisible();
  }

  async expectMetricsPanelVisible(): Promise<void> {
    await expect(this.metricsPanel).toBeVisible();
  }

  async expectHealthPanelVisible(): Promise<void> {
    await expect(this.healthPanel).toBeVisible();
  }

  async expectMessagesPanelVisible(): Promise<void> {
    await expect(this.messagesPanel).toBeVisible();
  }
}
