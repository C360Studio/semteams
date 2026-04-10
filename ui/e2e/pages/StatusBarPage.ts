import { Page, Locator, expect } from "@playwright/test";

/**
 * Page Object Model for the Status Bar
 * Encapsulates interactions with the runtime status and deployment controls
 */
export class StatusBarPage {
  constructor(private page: Page) {}

  // Locators
  get statusBar(): Locator {
    return this.page.locator(".status-bar");
  }

  get runtimeState(): Locator {
    return this.page.locator(".runtime-state");
  }

  get deployButton(): Locator {
    return this.page.locator('button:has-text("Deploy")');
  }

  get stopButton(): Locator {
    return this.page.locator('button:has-text("Stop")');
  }

  get startButton(): Locator {
    return this.page.locator('button:has-text("Start")');
  }

  get statusMessage(): Locator {
    return this.page.locator(".status-message");
  }

  get lastSaved(): Locator {
    return this.page.locator(".last-saved");
  }

  // Actions
  async clickDeploy(): Promise<void> {
    // Wait for button to be stable before clicking
    await this.deployButton.waitFor({ state: "visible" });
    await this.deployButton.click({ timeout: 10000 });
  }

  async clickStart(): Promise<void> {
    await this.startButton.click();
  }

  async clickStop(): Promise<void> {
    await this.stopButton.click();
  }

  async getRuntimeState(): Promise<string> {
    return (await this.runtimeState.getAttribute("data-state")) || "";
  }

  async getStatusMessage(): Promise<string> {
    return (await this.statusMessage.textContent()) || "";
  }

  // Assertions
  async expectStatusBarVisible(): Promise<void> {
    await expect(this.statusBar).toBeVisible();
  }

  async expectRuntimeState(
    state: "not_deployed" | "deployed_stopped" | "running" | "error",
  ): Promise<void> {
    await expect(this.runtimeState).toHaveAttribute("data-state", state);
  }

  async expectDeployButtonVisible(): Promise<void> {
    await expect(this.deployButton).toBeVisible();
  }

  async expectDeployButtonEnabled(): Promise<void> {
    await expect(this.deployButton).toBeEnabled();
  }

  async expectDeployButtonDisabled(): Promise<void> {
    await expect(this.deployButton).toBeDisabled();
  }

  async expectStopButtonVisible(): Promise<void> {
    await expect(this.stopButton).toBeVisible();
  }

  async expectStopButtonEnabled(): Promise<void> {
    await expect(this.stopButton).toBeEnabled();
  }

  async expectStopButtonDisabled(): Promise<void> {
    await expect(this.stopButton).toBeDisabled();
  }

  async expectStartButtonVisible(): Promise<void> {
    await expect(this.startButton).toBeVisible();
  }

  async expectStartButtonEnabled(): Promise<void> {
    await expect(this.startButton).toBeEnabled();
  }

  async expectStartButtonDisabled(): Promise<void> {
    await expect(this.startButton).toBeDisabled();
  }

  async expectStatusMessage(message: string): Promise<void> {
    await expect(this.statusMessage).toContainText(message);
  }

  async expectLastSavedVisible(): Promise<void> {
    await expect(this.lastSaved).toBeVisible();
  }

  async expectStateTransition(
    fromState: string,
    toState: string,
    timeoutMs: number = 5000,
  ): Promise<void> {
    // Wait for state to change from initial state
    await expect(this.runtimeState).not.toHaveAttribute(
      "data-state",
      fromState,
      { timeout: timeoutMs },
    );
    // Verify it reached the expected target state
    await expect(this.runtimeState).toHaveAttribute("data-state", toState);
  }
}
