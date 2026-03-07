import { expect, test, type Request } from "@playwright/test";
import { createRunningFlow, deleteTestFlow } from "./helpers/runtime-helpers";

/**
 * DataView GraphQL Integration - E2E Tests
 *
 * IMPORTANT: Read docs/testing/E2E_INFRASTRUCTURE.md before modifying
 *
 * These tests verify the DataView component's GraphQL integration with the backend.
 * Tests cover:
 * 1. Loading data when switching to Data view
 * 2. Loading state overlay
 * 3. Error handling and retry functionality
 * 4. Refresh functionality
 *
 * The DataView uses GraphQL queries to fetch knowledge graph data from the backend.
 * All tests create a running flow to enable the ViewSwitcher.
 */

test.describe("DataView GraphQL Integration", () => {
  let flowId: string;

  test.beforeEach(async ({ page }) => {
    // Create and start a flow (required for ViewSwitcher to be visible)
    const setup = await createRunningFlow(page);
    flowId = setup.flowId;

    // Navigate to flow page
    await page.goto(setup.url);

    // Wait for canvas to be visible and stable
    await expect(page.locator("#flow-canvas")).toBeVisible();
    await page.waitForLoadState("networkidle");
  });

  test.afterEach(async ({ page }) => {
    // Clean up test flow
    if (flowId) {
      await deleteTestFlow(page, flowId);
    }
  });

  test("should load DataView when switching to Data mode", async ({ page }) => {
    // Track GraphQL requests
    const graphqlRequests: Request[] = [];
    page.on("request", (req) => {
      if (req.url().includes("/graphql")) {
        graphqlRequests.push(req);
      }
    });

    // Verify ViewSwitcher is visible (only shows when flow is running)
    await expect(page.locator('[data-testid="view-switcher"]')).toBeVisible();

    // Click Data button
    await page.click('[data-testid="view-switch-data"]');

    // Verify DataView is visible
    await expect(page.locator('[data-testid="data-view"]')).toBeVisible({
      timeout: 5000,
    });

    // Verify GraphQL request was made
    expect(graphqlRequests.length).toBeGreaterThan(0);

    // Verify the request contains the expected pathSearch query
    const graphqlRequest = graphqlRequests[0];
    const postData = graphqlRequest.postDataJSON();
    expect(postData.query).toContain("pathSearch");
    expect(postData.variables).toHaveProperty("startEntity", "*");
    expect(postData.variables).toHaveProperty("maxDepth", 2);
    expect(postData.variables).toHaveProperty("maxNodes", 50);
  });

  test("should show loading state while fetching data", async ({ page }) => {
    // Intercept GraphQL request and delay response
    let resolveGraphQL: (value: unknown) => void;
    const graphQLPromise = new Promise((resolve) => {
      resolveGraphQL = resolve;
    });

    await page.route("**/graphql", async (route) => {
      // Wait before fulfilling the request
      await graphQLPromise;

      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            pathSearch: {
              entities: [],
              edges: [],
            },
          },
        }),
      });
    });

    // Click Data button
    await page.click('[data-testid="view-switch-data"]');

    // Verify loading overlay is visible
    const loadingOverlay = page.locator(".loading-overlay");
    await expect(loadingOverlay).toBeVisible();
    await expect(loadingOverlay).toContainText("Loading graph data...");

    // Resolve the GraphQL request
    resolveGraphQL!(null);

    // Wait for loading to disappear
    await expect(loadingOverlay).not.toBeVisible({ timeout: 3000 });
  });

  test("should handle GraphQL errors and show error banner", async ({
    page,
  }) => {
    // Intercept GraphQL request and return error
    await page.route("**/graphql", (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          errors: [{ message: "Graph service unavailable" }],
        }),
      });
    });

    // Click Data button
    await page.click('[data-testid="view-switch-data"]');

    // Wait for DataView to be visible
    await expect(page.locator('[data-testid="data-view"]')).toBeVisible();

    // Verify error banner is visible
    const errorBanner = page.locator(".error-banner");
    await expect(errorBanner).toBeVisible({ timeout: 3000 });
    await expect(errorBanner).toContainText("Graph service unavailable");

    // Verify retry button is present
    const retryButton = errorBanner.locator(".retry-button");
    await expect(retryButton).toBeVisible();
    await expect(retryButton).toContainText("Retry");
  });

  test("should handle HTTP error responses", async ({ page }) => {
    // Intercept GraphQL request and return HTTP 500 error
    await page.route("**/graphql", (route) => {
      route.fulfill({
        status: 500,
        contentType: "application/json",
        body: JSON.stringify({ error: "Internal server error" }),
      });
    });

    // Click Data button
    await page.click('[data-testid="view-switch-data"]');

    // Wait for DataView to be visible
    await expect(page.locator('[data-testid="data-view"]')).toBeVisible();

    // Verify error banner is visible with appropriate message
    const errorBanner = page.locator(".error-banner");
    await expect(errorBanner).toBeVisible({ timeout: 3000 });
    // The component shows "pathSearch failed: Internal Server Error" for HTTP errors
    await expect(errorBanner).toContainText("pathSearch failed");
  });

  test("should handle network errors", async ({ page }) => {
    // Intercept GraphQL request and abort (simulates network failure)
    await page.route("**/graphql", (route) => {
      route.abort("failed");
    });

    // Click Data button
    await page.click('[data-testid="view-switch-data"]');

    // Wait for DataView to be visible
    await expect(page.locator('[data-testid="data-view"]')).toBeVisible();

    // Verify error banner shows connection error
    const errorBanner = page.locator(".error-banner");
    await expect(errorBanner).toBeVisible({ timeout: 3000 });
    await expect(errorBanner).toContainText(
      "Unable to connect to graph service",
    );
  });

  test("should retry loading data when retry button is clicked", async ({
    page,
  }) => {
    let requestCount = 0;

    // Mock: first request fails, second succeeds
    await page.route("**/graphql", (route) => {
      requestCount++;

      if (requestCount === 1) {
        // First request: return error
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            errors: [{ message: "Temporary failure" }],
          }),
        });
      } else {
        // Second request: return success
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            data: {
              pathSearch: {
                entities: [],
                edges: [],
              },
            },
          }),
        });
      }
    });

    // Click Data button
    await page.click('[data-testid="view-switch-data"]');

    // Wait for error to appear
    const errorBanner = page.locator(".error-banner");
    await expect(errorBanner).toBeVisible({ timeout: 3000 });

    // Click retry button
    const retryButton = errorBanner.locator(".retry-button");
    await retryButton.click();

    // Verify second request was made
    expect(requestCount).toBe(2);

    // Verify error banner disappears after successful retry
    await expect(errorBanner).not.toBeVisible({ timeout: 3000 });
  });

  test("should reload data when refresh button is clicked", async ({
    page,
  }) => {
    let requestCount = 0;

    // Track all GraphQL requests
    await page.route("**/graphql", (route) => {
      requestCount++;

      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            pathSearch: {
              entities: [],
              edges: [],
            },
          },
        }),
      });
    });

    // Click Data button
    await page.click('[data-testid="view-switch-data"]');

    // Wait for initial load
    await expect(page.locator('[data-testid="data-view"]')).toBeVisible();
    await page.waitForTimeout(500);

    const initialRequestCount = requestCount;
    expect(initialRequestCount).toBeGreaterThan(0);

    // Find and click refresh button (toolbar button with refresh icon)
    const refreshButton = page.locator(".toolbar-button[title='Refresh data']");
    await expect(refreshButton).toBeVisible();
    await refreshButton.click();

    // Wait for new request to complete
    await page.waitForTimeout(500);

    // Verify a new request was made
    expect(requestCount).toBe(initialRequestCount + 1);
  });

  test("should disable refresh button while loading", async ({ page }) => {
    // Intercept GraphQL request and delay response
    let resolveGraphQL: (value: unknown) => void;
    const graphQLPromise = new Promise((resolve) => {
      resolveGraphQL = resolve;
    });

    await page.route("**/graphql", async (route) => {
      await graphQLPromise;

      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            pathSearch: {
              entities: [],
              edges: [],
            },
          },
        }),
      });
    });

    // Click Data button
    await page.click('[data-testid="view-switch-data"]');

    // Wait for DataView to be visible
    await expect(page.locator('[data-testid="data-view"]')).toBeVisible();

    // Verify refresh button is disabled while loading
    const refreshButton = page.locator(".toolbar-button[title='Refresh data']");
    await expect(refreshButton).toBeVisible();
    await expect(refreshButton).toBeDisabled();

    // Resolve the GraphQL request
    resolveGraphQL!(null);

    // Wait for loading to complete
    await page.waitForTimeout(500);

    // Verify refresh button is enabled after loading
    await expect(refreshButton).not.toBeDisabled();
  });

  test("should switch back to Flow view from Data view", async ({ page }) => {
    // Click Data button to switch to Data view
    await page.click('[data-testid="view-switch-data"]');

    // Verify DataView is visible
    await expect(page.locator('[data-testid="data-view"]')).toBeVisible();

    // Click Flow button to switch back
    await page.click('[data-testid="view-switch-flow"]');

    // Verify canvas is visible again
    await expect(page.locator("#flow-canvas")).toBeVisible();

    // Verify DataView is no longer visible
    await expect(page.locator('[data-testid="data-view"]')).not.toBeVisible();
  });
});
