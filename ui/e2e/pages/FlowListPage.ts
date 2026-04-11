import { Page, Locator, expect } from "@playwright/test";

/**
 * Page Object Model for the Flow List homepage
 * Encapsulates interactions with the flow list page
 */
export class FlowListPage {
  constructor(private page: Page) {}

  // Locators
  get flowList(): Locator {
    return this.page.locator(".flow-list");
  }

  get createButton(): Locator {
    return this.page.locator('button:has-text("Create New Flow")');
  }

  get flowItems(): Locator {
    return this.page.locator(".flow-item");
  }

  // Actions
  async goto(): Promise<void> {
    await this.page.goto("/");
    // Wait for page to be fully loaded and hydrated
    await this.page.waitForLoadState("networkidle");
    // Wait for the create button to be visible - ensures SvelteKit hydration is complete
    await this.createButton.waitFor({ state: "visible", timeout: 10000 });
  }

  getFlowByName(name: string): Locator {
    return this.page.locator(`.flow-item:has-text("${name}")`);
  }

  async clickCreateNewFlow(): Promise<void> {
    // Ensure button is visible and clickable before attempting click
    await this.createButton.waitFor({ state: "visible", timeout: 10000 });
    await this.createButton.click();
  }

  async deleteFlow(name: string): Promise<void> {
    const flow = this.getFlowByName(name);
    const deleteButton = flow.locator('button[aria-label="Delete flow"]');
    await deleteButton.click();
  }

  // Assertions
  async expectFlowInList(name: string): Promise<void> {
    const flow = this.getFlowByName(name);
    await expect(flow).toBeVisible();
  }

  async expectFlowCount(count: number): Promise<void> {
    await expect(this.flowItems).toHaveCount(count);
  }
}
