# Runtime Visualization Panel - E2E Test Plan

**Date**: 2025-11-17
**Status**: Planning
**Related BD Issue**: semstreams-8cj7

## Overview

End-to-end test plan for the Runtime Visualization Panel feature using Playwright. These tests validate the complete user workflow from flow deployment through runtime monitoring.

## Test Strategy

### Approach

- **User-Centric**: Test from user's perspective, not implementation
- **Integration Focus**: Validate UI + backend API integration
- **Realistic Scenarios**: Use actual flow deployment, not just UI mocking
- **Error Resilience**: Test both success and failure paths

### Test Environment

- **Backend**: Running semstreams backend (via docker-compose)
- **Flow**: Simple test flow (UDP source → processor → NATS sink)
- **Mock Data**: Backend provides mock runtime data for testing
- **Isolation**: Each test deploys its own flow, cleans up after

## Test Coverage

### Phase 1: Basic Panel (5 tests)

#### Test 1.1: Toggle button appears when flow is running

```typescript
test("should show debug toggle button when flow is running", async ({
  page,
}) => {
  // Deploy a test flow
  await deployTestFlow(page);

  // Start the flow
  await page.getByRole("button", { name: "Start" }).click();

  // Debug button should appear
  await expect(page.getByRole("button", { name: /Debug/i })).toBeVisible();
});
```

#### Test 1.2: Panel slides up when debug button clicked

```typescript
test("should open runtime panel when debug button clicked", async ({
  page,
}) => {
  await deployAndStartFlow(page);

  // Click debug button
  await page.getByRole("button", { name: /Debug/i }).click();

  // Panel should be visible
  await expect(page.getByTestId("runtime-panel")).toBeVisible();

  // Should show "Runtime Debugging" title
  await expect(page.getByText("Runtime Debugging")).toBeVisible();
});
```

#### Test 1.3: Canvas height adjusts when panel opens

```typescript
test("should adjust canvas height when panel opens", async ({ page }) => {
  await deployAndStartFlow(page);

  // Get canvas height before opening panel
  const canvasBefore = page.locator(".canvas-container");
  const heightBefore = await canvasBefore.evaluate((el) => el.offsetHeight);

  // Open panel
  await page.getByRole("button", { name: /Debug/i }).click();
  await page.waitForTimeout(300); // Wait for animation

  // Canvas should be shorter
  const heightAfter = await canvasBefore.evaluate((el) => el.offsetHeight);
  expect(heightAfter).toBeLessThan(heightBefore);
});
```

#### Test 1.4: Close button closes panel

```typescript
test("should close panel when close button clicked", async ({ page }) => {
  await deployAndStartFlow(page);
  await openRuntimePanel(page);

  // Click close button
  await page.getByRole("button", { name: "Close runtime panel" }).click();

  // Panel should be hidden
  await expect(page.getByTestId("runtime-panel")).not.toBeVisible();
});
```

#### Test 1.5: Esc key closes panel

```typescript
test("should close panel when Esc key pressed", async ({ page }) => {
  await deployAndStartFlow(page);
  await openRuntimePanel(page);

  // Press Escape
  await page.keyboard.press("Escape");

  // Panel should be hidden
  await expect(page.getByTestId("runtime-panel")).not.toBeVisible();
});
```

---

### Phase 2: Logs Tab (8 tests)

#### Test 2.1: Logs tab is active by default

```typescript
test("should show Logs tab as active by default", async ({ page }) => {
  await deployAndStartFlow(page);
  await openRuntimePanel(page);

  // Logs tab should be active
  const logsTab = page.getByTestId("tab-logs");
  await expect(logsTab).toHaveAttribute("aria-selected", "true");

  // Logs panel should be visible
  await expect(page.getByTestId("logs-panel")).toBeVisible();
});
```

#### Test 2.2: Log entries appear when flow is running

```typescript
test("should display log entries from running flow", async ({ page }) => {
  await deployAndStartFlow(page);
  await openRuntimePanel(page);

  // Wait for logs to appear (backend should emit test logs)
  await expect(page.getByTestId("log-entry")).toHaveCount(3, {
    timeout: 10000,
  });

  // Should show timestamp, level, component, message
  const firstLog = page.getByTestId("log-entry").first();
  await expect(firstLog).toContainText(/\d{2}:\d{2}:\d{2}/); // Timestamp
  await expect(firstLog).toContainText(/INFO|DEBUG|WARN|ERROR/); // Level
});
```

#### Test 2.3: Filter logs by level

```typescript
test("should filter logs by level", async ({ page }) => {
  await deployAndStartFlow(page);
  await openRuntimePanel(page);
  await waitForLogs(page, 5);

  // Select ERROR level filter
  await page.getByTestId("level-filter").selectOption("ERROR");

  // Only ERROR logs should be visible
  const logs = await page.getByTestId("log-entry").all();
  for (const log of logs) {
    await expect(log).toContainText("ERROR");
  }
});
```

#### Test 2.4: Filter logs by component

```typescript
test("should filter logs by component", async ({ page }) => {
  await deployAndStartFlow(page);
  await openRuntimePanel(page);
  await waitForLogs(page, 5);

  // Get available component from dropdown
  const componentFilter = page.getByTestId("component-filter");
  await componentFilter.selectOption({ index: 1 }); // First non-"all" option
  const selectedComponent = await componentFilter.inputValue();

  // Only logs from selected component should be visible
  const logs = await page.getByTestId("log-entry").all();
  for (const log of logs) {
    await expect(log).toContainText(selectedComponent);
  }
});
```

#### Test 2.5: Auto-scroll works

```typescript
test("should auto-scroll to bottom when new logs arrive", async ({ page }) => {
  await deployAndStartFlow(page);
  await openRuntimePanel(page);

  // Auto-scroll should be enabled by default
  await expect(page.getByTestId("auto-scroll-toggle")).toBeChecked();

  // Wait for logs to appear
  await waitForLogs(page, 3);

  // Log container should be scrolled to bottom
  const logContainer = page.getByTestId("log-container");
  const isAtBottom = await logContainer.evaluate((el) => {
    return Math.abs(el.scrollHeight - el.scrollTop - el.clientHeight) < 10;
  });
  expect(isAtBottom).toBe(true);
});
```

#### Test 2.6: Disable auto-scroll maintains scroll position

```typescript
test("should maintain scroll position when auto-scroll disabled", async ({
  page,
}) => {
  await deployAndStartFlow(page);
  await openRuntimePanel(page);
  await waitForLogs(page, 5);

  // Scroll to top
  const logContainer = page.getByTestId("log-container");
  await logContainer.evaluate((el) => (el.scrollTop = 0));

  // Disable auto-scroll
  await page.getByTestId("auto-scroll-toggle").uncheck();

  // Wait for more logs
  await page.waitForTimeout(2000);

  // Should still be at top
  const scrollTop = await logContainer.evaluate((el) => el.scrollTop);
  expect(scrollTop).toBeLessThan(50);
});
```

#### Test 2.7: Clear logs button works

```typescript
test("should clear all logs when clear button clicked", async ({ page }) => {
  await deployAndStartFlow(page);
  await openRuntimePanel(page);
  await waitForLogs(page, 5);

  // Click clear button
  await page.getByTestId("clear-logs-button").click();

  // No logs should be visible
  await expect(page.getByTestId("log-entry")).toHaveCount(0);

  // Should show empty state
  await expect(page.getByText(/No logs yet/i)).toBeVisible();
});
```

#### Test 2.8: Error state when backend unavailable

```typescript
test("should show error when log streaming unavailable", async ({ page }) => {
  // Stop backend before opening panel
  await stopBackend();

  await deployAndStartFlow(page);
  await openRuntimePanel(page);

  // Should show error message
  await expect(page.getByText(/Log streaming unavailable/i)).toBeVisible();

  // Restart backend for cleanup
  await startBackend();
});
```

---

### Phase 3: Metrics Tab (7 tests)

#### Test 3.1: Switch to Metrics tab

```typescript
test("should switch to Metrics tab when clicked", async ({ page }) => {
  await deployAndStartFlow(page);
  await openRuntimePanel(page);

  // Click Metrics tab
  await page.getByTestId("tab-metrics").click();

  // Metrics tab should be active
  await expect(page.getByTestId("tab-metrics")).toHaveAttribute(
    "aria-selected",
    "true",
  );

  // Metrics panel should be visible
  await expect(page.getByTestId("metrics-panel")).toBeVisible();
});
```

#### Test 3.2: Metrics table displays component data

```typescript
test("should display metrics table with component data", async ({ page }) => {
  await deployAndStartFlow(page);
  await openRuntimePanel(page);
  await page.getByTestId("tab-metrics").click();

  // Wait for metrics to load
  await expect(page.getByTestId("metrics-row")).toHaveCount(3, {
    timeout: 10000,
  });

  // Table should have correct columns
  await expect(
    page.getByRole("columnheader", { name: "Component" }),
  ).toBeVisible();
  await expect(
    page.getByRole("columnheader", { name: "Msg/sec" }),
  ).toBeVisible();
  await expect(
    page.getByRole("columnheader", { name: "Errors/sec" }),
  ).toBeVisible();
  await expect(
    page.getByRole("columnheader", { name: "Status" }),
  ).toBeVisible();
});
```

#### Test 3.3: Metrics update at configured interval

```typescript
test("should update metrics at configured interval", async ({ page }) => {
  await deployAndStartFlow(page);
  await openRuntimePanel(page);
  await page.getByTestId("tab-metrics").click();

  // Get initial last updated timestamp
  const lastUpdated1 = await page.getByTestId("last-updated").textContent();

  // Wait 2+ seconds (default interval)
  await page.waitForTimeout(2500);

  // Timestamp should have changed
  const lastUpdated2 = await page.getByTestId("last-updated").textContent();
  expect(lastUpdated2).not.toBe(lastUpdated1);
});
```

#### Test 3.4: Refresh rate selector changes interval

```typescript
test("should change polling interval when refresh rate changed", async ({
  page,
}) => {
  await deployAndStartFlow(page);
  await openRuntimePanel(page);
  await page.getByTestId("tab-metrics").click();

  // Change to 1 second interval
  await page.getByTestId("refresh-rate-selector").selectOption("1000");

  // Wait for initial update
  await page.waitForTimeout(100);
  const time1 = await page.getByTestId("last-updated").textContent();

  // Wait 1+ second
  await page.waitForTimeout(1200);

  // Should have updated
  const time2 = await page.getByTestId("last-updated").textContent();
  expect(time2).not.toBe(time1);
});
```

#### Test 3.5: Manual refresh mode shows button

```typescript
test("should show manual refresh button in manual mode", async ({ page }) => {
  await deployAndStartFlow(page);
  await openRuntimePanel(page);
  await page.getByTestId("tab-metrics").click();

  // Change to manual mode
  await page.getByTestId("refresh-rate-selector").selectOption("manual");

  // Manual refresh button should appear
  await expect(page.getByTestId("manual-refresh-button")).toBeVisible();
});
```

#### Test 3.6: Manual refresh button fetches new data

```typescript
test("should fetch new metrics when manual refresh clicked", async ({
  page,
}) => {
  await deployAndStartFlow(page);
  await openRuntimePanel(page);
  await page.getByTestId("tab-metrics").click();

  // Switch to manual mode
  await page.getByTestId("refresh-rate-selector").selectOption("manual");

  const time1 = await page.getByTestId("last-updated").textContent();
  await page.waitForTimeout(500);

  // Click manual refresh
  await page.getByTestId("manual-refresh-button").click();

  // Timestamp should update
  await expect(page.getByTestId("last-updated")).not.toHaveText(time1);
});
```

#### Test 3.7: Status indicators show correct colors

```typescript
test("should show status indicators with correct colors", async ({ page }) => {
  await deployAndStartFlow(page);
  await openRuntimePanel(page);
  await page.getByTestId("tab-metrics").click();
  await waitForMetrics(page);

  // At least one healthy component should have green indicator
  const healthyIndicator = page.getByTestId("status-indicator").first();
  const color = await healthyIndicator.evaluate(
    (el) => window.getComputedStyle(el).color,
  );

  // Should be green (success color from design system)
  // RGB values may vary, but should be greenish
  expect(color).toMatch(/rgb\(.*,.*,.*\)/);
});
```

---

### Phase 4: Health Tab (9 tests)

#### Test 4.1: Switch to Health tab

```typescript
test("should switch to Health tab when clicked", async ({ page }) => {
  await deployAndStartFlow(page);
  await openRuntimePanel(page);

  // Click Health tab
  await page.getByTestId("tab-health").click();

  // Health tab should be active
  await expect(page.getByTestId("tab-health")).toHaveAttribute(
    "aria-selected",
    "true",
  );

  // Health panel should be visible
  await expect(page.getByTestId("health-panel")).toBeVisible();
});
```

#### Test 4.2: System health summary displays

```typescript
test("should display system health summary", async ({ page }) => {
  await deployAndStartFlow(page);
  await openRuntimePanel(page);
  await page.getByTestId("tab-health").click();

  // Health summary should be visible
  await expect(page.getByTestId("health-summary")).toBeVisible();

  // Should show component count
  await expect(page.getByText(/\d+\/\d+ components healthy/i)).toBeVisible();

  // Should show status indicator
  await expect(page.getByTestId("overall-status-indicator")).toBeVisible();
});
```

#### Test 4.3: Component health table displays

```typescript
test("should display component health table", async ({ page }) => {
  await deployAndStartFlow(page);
  await openRuntimePanel(page);
  await page.getByTestId("tab-health").click();

  // Wait for health data
  await expect(page.getByTestId("health-row")).toHaveCount(3, {
    timeout: 10000,
  });

  // Table should have correct columns
  await expect(
    page.getByRole("columnheader", { name: "Component" }),
  ).toBeVisible();
  await expect(
    page.getByRole("columnheader", { name: "Status" }),
  ).toBeVisible();
  await expect(
    page.getByRole("columnheader", { name: "Uptime" }),
  ).toBeVisible();
  await expect(
    page.getByRole("columnheader", { name: "Last Activity" }),
  ).toBeVisible();
});
```

#### Test 4.4: Uptime counter updates every second

```typescript
test("should update uptime counters every second", async ({ page }) => {
  await deployAndStartFlow(page);
  await openRuntimePanel(page);
  await page.getByTestId("tab-health").click();
  await waitForHealth(page);

  // Get initial uptime
  const uptime1 = await page
    .getByTestId("health-row")
    .first()
    .locator(".uptime-cell")
    .textContent();

  // Wait 2 seconds
  await page.waitForTimeout(2000);

  // Uptime should have increased
  const uptime2 = await page
    .getByTestId("health-row")
    .first()
    .locator(".uptime-cell")
    .textContent();

  expect(uptime2).not.toBe(uptime1);
  // Should be in HH:MM:SS format
  expect(uptime2).toMatch(/\d{2}:\d{2}:\d{2}/);
});
```

#### Test 4.5: Last activity shows relative time

```typescript
test("should show last activity as relative time", async ({ page }) => {
  await deployAndStartFlow(page);
  await openRuntimePanel(page);
  await page.getByTestId("tab-health").click();
  await waitForHealth(page);

  // Last activity should be relative
  const lastActivity = await page
    .getByTestId("health-row")
    .first()
    .locator(".last-activity-cell")
    .textContent();

  expect(lastActivity).toMatch(/\d+ (second|minute|hour)s? ago/);
});
```

#### Test 4.6: Stale components are highlighted

```typescript
test("should highlight components with stale activity", async ({ page }) => {
  // Deploy flow with intentionally stale component
  await deployFlowWithStaleComponent(page);
  await openRuntimePanel(page);
  await page.getByTestId("tab-health").click();
  await waitForHealth(page);

  // Find stale component row
  const staleRow = page.getByTestId("health-row").filter({
    hasText: /3\d+ seconds ago/,
  });

  // Should have stale styling
  await expect(staleRow.locator(".last-activity-cell")).toHaveClass(/stale/);
});
```

#### Test 4.7: Expand details for degraded component

```typescript
test("should expand details for component with issues", async ({ page }) => {
  // Deploy flow with degraded component
  await deployFlowWithDegradedComponent(page);
  await openRuntimePanel(page);
  await page.getByTestId("tab-health").click();
  await waitForHealth(page);

  // Find degraded component
  const degradedRow = page.getByTestId("health-row").filter({
    hasText: /Degraded/,
  });

  // Should have expand button
  const expandButton = degradedRow.getByTestId("expand-button");
  await expect(expandButton).toBeVisible();

  // Click to expand
  await expandButton.click();

  // Details should be visible
  await expect(page.getByTestId("details-row")).toBeVisible();
  await expect(page.getByText(/WARNING:/i)).toBeVisible();
});
```

#### Test 4.8: Collapse expanded details

```typescript
test("should collapse details when expand button clicked again", async ({
  page,
}) => {
  await deployFlowWithDegradedComponent(page);
  await openRuntimePanel(page);
  await page.getByTestId("tab-health").click();
  await waitForHealth(page);

  // Expand details
  const degradedRow = page.getByTestId("health-row").filter({
    hasText: /Degraded/,
  });
  await degradedRow.getByTestId("expand-button").click();
  await expect(page.getByTestId("details-row")).toBeVisible();

  // Click again to collapse
  await degradedRow.getByTestId("expand-button").click();

  // Details should be hidden
  await expect(page.getByTestId("details-row")).not.toBeVisible();
});
```

#### Test 4.9: Health data updates at 5-second interval

```typescript
test("should update health data every 5 seconds", async ({ page }) => {
  await deployAndStartFlow(page);
  await openRuntimePanel(page);
  await page.getByTestId("tab-health").click();
  await waitForHealth(page);

  // Get initial component count
  const summary1 = await page.getByTestId("health-summary").textContent();

  // Wait 5+ seconds
  await page.waitForTimeout(5500);

  // Summary should still be present (data refreshed)
  const summary2 = await page.getByTestId("health-summary").textContent();
  expect(summary2).toBeTruthy();
});
```

---

## Test Utilities

### Helper Functions

```typescript
// Deploy and start a test flow
async function deployAndStartFlow(page: Page): Promise<void> {
  await deployTestFlow(page);
  await page.getByRole("button", { name: "Start" }).click();
  await waitForFlowState(page, "running");
}

// Deploy a simple test flow
async function deployTestFlow(page: Page): Promise<void> {
  await page.goto("/flows/new");

  // Add components
  await addComponent(page, "udp-source", { port: 8080 });
  await addComponent(page, "json-processor", {});
  await addComponent(page, "nats-sink", { subject: "test" });

  // Connect components
  await connectComponents(page, "udp-source", "json-processor");
  await connectComponents(page, "json-processor", "nats-sink");

  // Deploy
  await page.getByRole("button", { name: "Deploy" }).click();
  await waitForFlowState(page, "deployed_stopped");
}

// Open the runtime panel
async function openRuntimePanel(page: Page): Promise<void> {
  await page.getByRole("button", { name: /Debug/i }).click();
  await expect(page.getByTestId("runtime-panel")).toBeVisible();
}

// Wait for logs to appear
async function waitForLogs(page: Page, minCount: number = 1): Promise<void> {
  await expect(page.getByTestId("log-entry")).toHaveCount(minCount, {
    timeout: 10000,
  });
}

// Wait for metrics to load
async function waitForMetrics(page: Page): Promise<void> {
  await expect(page.getByTestId("metrics-row")).toHaveCount(3, {
    timeout: 10000,
  });
}

// Wait for health data to load
async function waitForHealth(page: Page): Promise<void> {
  await expect(page.getByTestId("health-row")).toHaveCount(3, {
    timeout: 10000,
  });
}

// Deploy flow with degraded component (for testing)
async function deployFlowWithDegradedComponent(page: Page): Promise<void> {
  // Deploy test flow with configuration that causes degradation
  await deployTestFlow(page);
  // Backend should simulate degraded component
  await page.getByRole("button", { name: "Start" }).click();
  await waitForFlowState(page, "running");
}

// Deploy flow with stale component (for testing)
async function deployFlowWithStaleComponent(page: Page): Promise<void> {
  // Deploy test flow with component that doesn't send activity
  await deployTestFlow(page);
  await page.getByRole("button", { name: "Start" }).click();
  await waitForFlowState(page, "running");
  // Backend should simulate stale component (>30s no activity)
}
```

---

## Backend Requirements

For these E2E tests to work, the backend needs to provide:

### 1. Test Flow Support

- Simple test flow that can be deployed via API
- Components: udp-source, json-processor, nats-sink
- Mock data generation for testing

### 2. Runtime Endpoints

**Logs Endpoint**: `GET /flows/{id}/runtime/logs` (SSE)

- Emit test log entries at regular intervals
- Include various log levels (DEBUG, INFO, WARN, ERROR)
- Include multiple component names

**Metrics Endpoint**: `GET /flows/{id}/runtime/metrics` (JSON)

- Return test metrics for 3 components
- Include throughput, error rates, status
- Vary metrics slightly on each poll

**Health Endpoint**: `GET /flows/{id}/runtime/health` (JSON)

- Return test health data for 3 components
- Include running/degraded/error states
- Include uptime and last activity
- Support mock degraded/stale components

### 3. Test Modes

**Environment Variable**: `E2E_TEST_MODE=true`

- Backend provides mock runtime data
- Predictable component names and values
- Controllable degraded/stale states

---

## Test Execution

### Run All Tests

```bash
npm run test:e2e
```

### Run Specific Phase

```bash
npm run test:e2e -- tests/runtime-viz-phase1.spec.ts
npm run test:e2e -- tests/runtime-viz-phase2.spec.ts
npm run test:e2e -- tests/runtime-viz-phase3.spec.ts
npm run test:e2e -- tests/runtime-viz-phase4.spec.ts
```

### Run in UI Mode (for debugging)

```bash
npm run test:e2e:ui
```

---

## Test File Organization

```
frontend/tests/
├── runtime-viz-phase1.spec.ts   # Basic panel tests (5 tests)
├── runtime-viz-phase2.spec.ts   # Logs tab tests (8 tests)
├── runtime-viz-phase3.spec.ts   # Metrics tab tests (7 tests)
├── runtime-viz-phase4.spec.ts   # Health tab tests (9 tests)
└── helpers/
    ├── flow-helpers.ts          # Flow deployment utilities
    ├── runtime-helpers.ts       # Runtime panel utilities
    └── backend-helpers.ts       # Backend control utilities
```

---

## Success Criteria

- [ ] All 29 E2E tests pass consistently
- [ ] Tests run in CI without flakiness
- [ ] Tests complete in < 5 minutes total
- [ ] No test-specific code in production components
- [ ] Backend test mode documented
- [ ] Tests are maintainable and clear

---

## Notes

- **Mock vs Real**: These tests use a real backend with test mode, not mocked APIs
- **Timing**: Use proper waits (`waitForSelector`, `toBeVisible`) not arbitrary timeouts
- **Cleanup**: Each test deploys its own flow and cleans up
- **Isolation**: Tests don't depend on each other
- **Realistic**: Tests validate actual user workflows, not just UI state

---

## Future Enhancements

- Test keyboard shortcuts (Ctrl+`)
- Test panel resizing (Phase 5)
- Test mobile responsive behavior
- Test with slow network conditions
- Test with high-frequency log generation
- Test with many components (20+)
- Visual regression testing for panel animations
