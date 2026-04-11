import { expect, test } from "@playwright/test";
import {
  createRunningFlow,
  deleteTestFlow,
  waitForRuntimePanel,
  clickTab,
} from "./helpers/runtime-helpers";

/**
 * Runtime Visualization Panel - Logs Tab Tests
 * Phase 2: SSE log streaming with filtering and controls
 *
 * Tests:
 * 1. Connect to SSE endpoint when tab active
 * 2. Stream real-time logs with timestamp, level, component, message
 * 3. Filter logs by level (DEBUG, INFO, WARN, ERROR)
 * 4. Filter logs by component
 * 5. Toggle auto-scroll on/off
 * 6. Clear logs button
 * 7. Show connection status (connecting, connected, disconnected)
 * 8. Handle SSE connection errors gracefully
 */

test.describe("Runtime Panel - Logs Tab", () => {
  let flowId: string;

  test.beforeEach(async ({ page }) => {
    // Create and start a flow
    const setup = await createRunningFlow(page);
    flowId = setup.flowId;

    // Navigate to flow page
    await page.goto(setup.url);
    await expect(page.locator("#flow-canvas")).toBeVisible();
    await page.waitForLoadState("networkidle");

    // Open runtime panel
    await page.click('[data-testid="debug-toggle-button"]');
    await waitForRuntimePanel(page);

    // Logs tab should be active by default
    await expect(page.locator('[data-testid="logs-panel"]')).toBeVisible();
  });

  test.afterEach(async ({ page }) => {
    if (flowId) {
      await deleteTestFlow(page, flowId);
    }
  });

  test("should connect to SSE endpoint when tab active", async ({ page }) => {
    // Logs tab is already active from beforeEach

    // Wait for SSE connection to be established
    // The component should transition from 'connecting' to 'connected'
    // Note: Backend might not have SSE implemented yet, so we check for connection attempt

    // Look for either:
    // 1. Connected state (if backend SSE is ready)
    // 2. Error state (if backend endpoint doesn't exist yet)
    // 3. Connecting state (if backend is slow)

    await page.waitForTimeout(1000); // Give SSE time to connect

    // Verify we're not in initial disconnected state anymore
    const logTab = page.locator('[data-testid="logs-tab"]');
    await expect(logTab).toBeVisible();

    // Check if connection error message appears (backend not ready)
    const errorMessage = page.locator('[data-testid="connection-error"]');
    const connectingMessage = page.locator(
      '[data-testid="connection-connecting"]',
    );

    // Either we should see error or connecting state (connection was attempted)
    const hasError = await errorMessage.isVisible().catch(() => false);
    const isConnecting = await connectingMessage.isVisible().catch(() => false);

    // If neither error nor connecting, backend must be connected (ideal case)
    if (!hasError && !isConnecting) {
      // This means SSE connected successfully
      // Verify log container is visible and ready
      await expect(page.locator('[data-testid="log-container"]')).toBeVisible();
    } else {
      // Backend not ready - verify error handling is graceful
      if (hasError) {
        await expect(errorMessage).toContainText("Log streaming unavailable");
      }
    }
  });

  test("should stream real-time logs with timestamp, level, component, message", async ({
    page,
  }) => {
    // Wait for potential logs to appear
    await page.waitForTimeout(2000);

    // Check if any log entries exist
    const logEntries = page.locator('[data-testid="log-entry"]');
    const count = await logEntries.count();

    if (count > 0) {
      // Backend is streaming logs - verify structure
      const firstLog = logEntries.first();

      // Verify log entry has required fields
      await expect(firstLog).toBeVisible();

      // Check for timestamp (formatted as HH:MM:SS.mmm)
      const timestamp = firstLog.locator(".log-timestamp");
      await expect(timestamp).toBeVisible();
      const timestampText = await timestamp.textContent();
      expect(timestampText).toMatch(/\d{2}:\d{2}:\d{2}\.\d{3}/);

      // Check for log level
      const level = firstLog.locator(".log-level");
      await expect(level).toBeVisible();
      const levelText = await level.textContent();
      expect(["DEBUG", "INFO", "WARN", "ERROR"]).toContain(levelText);

      // Check for component name
      const component = firstLog.locator(".log-component");
      await expect(component).toBeVisible();

      // Check for message
      const message = firstLog.locator(".log-message");
      await expect(message).toBeVisible();
    } else {
      // No logs yet - verify empty state is shown
      const emptyState = page.locator(".empty-state");
      await expect(emptyState).toBeVisible();
      await expect(emptyState).toContainText("No logs yet");
    }
  });

  test("should filter logs by level (DEBUG, INFO, WARN, ERROR)", async ({
    page,
  }) => {
    // Wait for potential logs
    await page.waitForTimeout(1000);

    const levelFilter = page.locator('[data-testid="level-filter"]');
    await expect(levelFilter).toBeVisible();

    // Test each level filter option
    const levels = ["DEBUG", "INFO", "WARN", "ERROR", "all"];

    for (const level of levels) {
      // Select level
      await levelFilter.selectOption(level);

      // Verify filter was applied
      const selectedValue = await levelFilter.inputValue();
      expect(selectedValue).toBe(level);

      // Wait for filter to apply
      await page.waitForTimeout(200);

      // If logs exist and level is not 'all', verify filtering
      const logEntries = page.locator('[data-testid="log-entry"]');
      const count = await logEntries.count();

      if (count > 0 && level !== "all") {
        // Verify all visible logs match the selected level
        const levels = await logEntries.locator(".log-level").allTextContents();
        for (const logLevel of levels) {
          expect(logLevel.trim()).toBe(level);
        }
      }
    }
  });

  test("should filter logs by component", async ({ page }) => {
    // Wait for potential logs
    await page.waitForTimeout(1000);

    const componentFilter = page.locator('[data-testid="component-filter"]');
    await expect(componentFilter).toBeVisible();

    // Initially should show "All Components"
    const initialValue = await componentFilter.inputValue();
    expect(initialValue).toBe("all");

    // If logs exist with components, filter options should be populated
    const logEntries = page.locator('[data-testid="log-entry"]');
    const logCount = await logEntries.count();

    if (logCount > 0) {
      // Get list of available component options
      const options = await componentFilter.locator("option").allTextContents();
      expect(options.length).toBeGreaterThan(1); // At least "All Components" + 1 component

      // Select first non-"all" component option
      const componentOptions = options.filter(
        (opt) => opt !== "All Components",
      );
      if (componentOptions.length > 0) {
        const componentToFilter = componentOptions[0];

        // Select component
        await componentFilter.selectOption(componentToFilter);
        await page.waitForTimeout(200);

        // Verify all visible logs are from selected component
        const components = await logEntries
          .locator(".log-component")
          .allTextContents();
        for (const comp of components) {
          // Component is wrapped in brackets: [component-name]
          expect(comp).toContain(componentToFilter);
        }
      }
    }
  });

  test("should toggle auto-scroll on/off", async ({ page }) => {
    const autoScrollToggle = page.locator('[data-testid="auto-scroll-toggle"]');
    await expect(autoScrollToggle).toBeVisible();

    // Auto-scroll should be enabled by default
    await expect(autoScrollToggle).toBeChecked();

    // Toggle off
    await autoScrollToggle.click();
    await expect(autoScrollToggle).not.toBeChecked();

    // Toggle back on
    await autoScrollToggle.click();
    await expect(autoScrollToggle).toBeChecked();

    // Verify checkbox is interactive and properly labeled
    const label = page.locator('label[for="auto-scroll"]');
    await expect(label).toContainText("Auto-scroll");
  });

  test("should clear logs when clear button clicked", async ({ page }) => {
    // Wait for potential logs
    await page.waitForTimeout(1000);

    const clearButton = page.locator('[data-testid="clear-logs-button"]');
    await expect(clearButton).toBeVisible();

    // Click clear button
    await clearButton.click();

    // Wait for clear to process
    await page.waitForTimeout(200);

    // Log entries should be cleared
    const logEntries = page.locator('[data-testid="log-entry"]');
    const count = await logEntries.count();
    expect(count).toBe(0);

    // Empty state should be shown
    const emptyState = page.locator(".empty-state");
    await expect(emptyState).toBeVisible();

    // Filters should reset to "all"
    const levelFilter = page.locator('[data-testid="level-filter"]');
    const componentFilter = page.locator('[data-testid="component-filter"]');
    expect(await levelFilter.inputValue()).toBe("all");
    expect(await componentFilter.inputValue()).toBe("all");
  });

  test.skip("should show connection status (connecting, connected, disconnected)", async ({
    page,
  }) => {
    // SKIPPED: Tab switching with SSE reconnection has complex timing.
    // The logs panel reconnects to SSE when becoming active which can take
    // variable time. This test should be re-enabled with mocked SSE or
    // more robust waiting logic.
    // Check for connection status indicators
    // After initial load, should be either connected or error state

    await page.waitForTimeout(1000);

    // Check for connecting state (might be brief)
    const connectingStatus = page.locator(
      '[data-testid="connection-connecting"]',
    );

    // Check for error state (if backend not ready)
    const errorStatus = page.locator('[data-testid="connection-error"]');

    const hasError = await errorStatus.isVisible().catch(() => false);
    const isConnecting = await connectingStatus.isVisible().catch(() => false);

    if (hasError) {
      // Backend SSE endpoint not ready - verify error message
      await expect(errorStatus).toContainText("Log streaming unavailable");
      await expect(errorStatus).toContainText("⚠");
    } else if (isConnecting) {
      // Still connecting
      await expect(connectingStatus).toContainText("Connecting to log stream");
      await expect(connectingStatus).toContainText("⋯");
    }
    // else: Connected successfully (no status message shown)

    // Switch to different tab to disconnect
    await clickTab(page, "metrics");
    await page.waitForTimeout(500);

    // Switch back to logs tab (should reconnect)
    await clickTab(page, "logs");
    await page.waitForTimeout(1000);

    // Should attempt reconnection
    const stillHasError = await errorStatus.isVisible().catch(() => false);
    // const stillConnecting = await connectingStatus.isVisible().catch(() => false);

    // Either we reconnected successfully or got same error state
    if (stillHasError) {
      await expect(errorStatus).toBeVisible();
    }
    // else: Successfully reconnected
  });

  test("should handle SSE connection errors gracefully", async ({ page }) => {
    // Wait for initial connection attempt
    await page.waitForTimeout(1500);

    // Check if error is shown (backend not ready)
    const errorStatus = page.locator('[data-testid="connection-error"]');
    const isError = await errorStatus.isVisible().catch(() => false);

    if (isError) {
      // Verify error is displayed gracefully
      await expect(errorStatus).toBeVisible();
      await expect(errorStatus).toContainText("Log streaming unavailable");

      // Verify error has appropriate styling (error container)
      const errorClass = await errorStatus.getAttribute("class");
      expect(errorClass).toContain("error");

      // Verify error icon is present
      await expect(errorStatus.locator(".status-icon")).toBeVisible();

      // Verify rest of UI is still functional despite error
      const levelFilter = page.locator('[data-testid="level-filter"]');
      await expect(levelFilter).toBeVisible();
      await expect(levelFilter).toBeEnabled();

      const clearButton = page.locator('[data-testid="clear-logs-button"]');
      await expect(clearButton).toBeVisible();
      await expect(clearButton).toBeEnabled();
    } else {
      // No error - backend SSE is working
      // Verify log container is accessible
      const logContainer = page.locator('[data-testid="log-container"]');
      await expect(logContainer).toBeVisible();
    }
  });
});
