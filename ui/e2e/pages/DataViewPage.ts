import { Page, Locator, expect } from "@playwright/test";

/**
 * Page Object Model for the DataView component
 * Encapsulates interactions with the knowledge graph data view, including the
 * Sigma canvas, graph filters, detail panel, and view switcher controls.
 */
export class DataViewPage {
  constructor(private page: Page) {}

  // ---------------------------------------------------------------------------
  // View switcher locators
  // ---------------------------------------------------------------------------

  get viewSwitcher(): Locator {
    return this.page.locator('[data-testid="view-switcher"]');
  }

  get dataViewButton(): Locator {
    return this.page.locator('[data-testid="view-switch-data"]');
  }

  get flowViewButton(): Locator {
    return this.page.locator('[data-testid="view-switch-flow"]');
  }

  // ---------------------------------------------------------------------------
  // DataView container and canvas
  // ---------------------------------------------------------------------------

  get dataView(): Locator {
    return this.page.locator('[data-testid="data-view"]');
  }

  get sigmaCanvas(): Locator {
    return this.page.locator('[data-testid="sigma-canvas"]');
  }

  get sigmaContainer(): Locator {
    return this.page.locator(".sigma-container");
  }

  // ---------------------------------------------------------------------------
  // Stats, filters, and panels
  // ---------------------------------------------------------------------------

  get graphStats(): Locator {
    return this.page.locator(".graph-stats");
  }

  get graphFilters(): Locator {
    return this.page.locator('[data-testid="graph-filters"]');
  }

  get detailPanel(): Locator {
    return this.page.locator('[data-testid="graph-detail-panel"]');
  }

  get detailPanelEmpty(): Locator {
    return this.page.locator('[data-testid="graph-detail-panel-empty"]');
  }

  // ---------------------------------------------------------------------------
  // Loading, error, and toolbar
  // ---------------------------------------------------------------------------

  get loadingOverlay(): Locator {
    return this.page.locator(".loading-overlay");
  }

  get errorBanner(): Locator {
    return this.page.locator(".error-banner");
  }

  get refreshButton(): Locator {
    return this.page.locator(".toolbar-button[title='Refresh data']");
  }

  // ---------------------------------------------------------------------------
  // Filter inputs
  // ---------------------------------------------------------------------------

  get entitySearch(): Locator {
    return this.page.locator('[data-testid="entity-search"]');
  }

  get confidenceSlider(): Locator {
    return this.page.locator('[data-testid="confidence-slider"]');
  }

  // ---------------------------------------------------------------------------
  // Dynamic filter chip locators
  // ---------------------------------------------------------------------------

  /**
   * Return a locator for the type-filter chip matching the given entity type.
   * Example: getTypeFilterChip("function") → [data-testid="type-filter-function"]
   */
  getTypeFilterChip(type: string): Locator {
    return this.page.locator(`[data-testid="type-filter-${type}"]`);
  }

  /**
   * Return a locator for the domain-filter chip matching the given domain.
   * Example: getDomainFilterChip("code") → [data-testid="domain-filter-code"]
   */
  getDomainFilterChip(domain: string): Locator {
    return this.page.locator(`[data-testid="domain-filter-${domain}"]`);
  }

  // ---------------------------------------------------------------------------
  // Action methods
  // ---------------------------------------------------------------------------

  /**
   * Switch to Data view. Waits for the view switcher to be visible, clicks the
   * Data button, then waits for the data-view container to become visible.
   */
  async switchToDataView(): Promise<void> {
    await this.viewSwitcher.waitFor({ state: "visible" });
    await this.dataViewButton.click();
    await expect(this.dataView).toBeVisible({ timeout: 5000 });
  }

  /**
   * Switch back to Flow view. Clicks the Flow button and waits for the canvas.
   */
  async switchToFlowView(): Promise<void> {
    await this.flowViewButton.click();
    await expect(this.page.locator("#flow-canvas")).toBeVisible();
  }

  /**
   * Wait for the graph to finish loading by waiting for the loading overlay to
   * disappear.
   */
  async waitForGraphLoaded(timeout: number = 8000): Promise<void> {
    await expect(this.loadingOverlay).not.toBeVisible({ timeout });
  }

  /**
   * Click the Refresh data toolbar button.
   */
  async clickRefresh(): Promise<void> {
    await this.refreshButton.click();
  }

  /**
   * Parse and return the entity count from the graph-stats overlay.
   * The stats overlay renders two spans: "{N} entities" and "{N} relationships".
   */
  async getEntityCount(): Promise<number> {
    const text = await this.graphStats.locator("span").first().textContent();
    const match = text?.match(/(\d+)/);
    return match ? parseInt(match[1], 10) : 0;
  }

  /**
   * Parse and return the relationship count from the graph-stats overlay.
   */
  async getRelationshipCount(): Promise<number> {
    const text = await this.graphStats.locator("span").last().textContent();
    const match = text?.match(/(\d+)/);
    return match ? parseInt(match[1], 10) : 0;
  }

  /**
   * Programmatically select an entity via the __e2eSelectEntity test seam
   * exposed by SigmaCanvas. This bypasses WebGL canvas click coordinates.
   */
  async selectEntity(entityId: string): Promise<void> {
    await this.page.evaluate((id) => window.__e2eSelectEntity?.(id), entityId);
  }

  // ---------------------------------------------------------------------------
  // Assertion helpers
  // ---------------------------------------------------------------------------

  async expectDataViewVisible(): Promise<void> {
    await expect(this.dataView).toBeVisible({ timeout: 5000 });
  }

  async expectDataViewHidden(): Promise<void> {
    await expect(this.dataView).not.toBeVisible();
  }

  async expectSigmaCanvasVisible(): Promise<void> {
    await expect(this.sigmaCanvas).toBeVisible();
  }

  async expectDetailPanelVisible(): Promise<void> {
    await expect(this.detailPanel).toBeVisible({ timeout: 3000 });
  }

  async expectDetailPanelEmpty(): Promise<void> {
    await expect(this.detailPanelEmpty).toBeVisible();
    await expect(this.detailPanel).not.toBeAttached();
  }

  async expectErrorBannerVisible(): Promise<void> {
    await expect(this.errorBanner).toBeVisible({ timeout: 3000 });
  }

  async expectErrorBannerHidden(): Promise<void> {
    await expect(this.errorBanner).not.toBeVisible({ timeout: 3000 });
  }

  async expectRefreshButtonEnabled(): Promise<void> {
    await expect(this.refreshButton).not.toBeDisabled();
  }

  async expectRefreshButtonDisabled(): Promise<void> {
    await expect(this.refreshButton).toBeDisabled();
  }
}
