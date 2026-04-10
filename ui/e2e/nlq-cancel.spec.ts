import { expect, test } from "@playwright/test";
import { createRunningFlow, deleteTestFlow } from "./helpers/runtime-helpers";

// ---------------------------------------------------------------------------
// GraphQL response fixtures
// ---------------------------------------------------------------------------

const MOCK_PATH_SEARCH_RESPONSE = {
  data: {
    pathSearch: {
      entities: [
        {
          id: "c360.ops.robotics.gcs.drone.001",
          triples: [
            {
              subject: "c360.ops.robotics.gcs.drone.001",
              predicate: "core.property.name",
              object: "Drone 001",
            },
          ],
        },
      ],
      edges: [],
    },
  },
};

const MOCK_GLOBAL_SEARCH_RESPONSE = {
  data: {
    globalSearch: {
      entities: [
        {
          id: "c360.ops.robotics.gcs.fleet.alpha",
          triples: [
            {
              subject: "c360.ops.robotics.gcs.fleet.alpha",
              predicate: "core.property.name",
              object: "Alpha Fleet",
            },
          ],
        },
      ],
      community_summaries: [],
      relationships: [],
      count: 1,
      duration_ms: 12,
    },
  },
};

// ---------------------------------------------------------------------------
// Helper: switch to DataView with mocked pathSearch
// ---------------------------------------------------------------------------

async function enterDataView(
  page: import("@playwright/test").Page,
): Promise<void> {
  await page.route("**/graphql", (route) => {
    const body = route.request().postDataJSON();
    if (body?.query?.includes("pathSearch")) {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(MOCK_PATH_SEARCH_RESPONSE),
      });
    } else {
      // Fallthrough for other queries (e.g. globalSearch handled per-test)
      route.continue();
    }
  });

  await page.click('[data-testid="view-switch-data"]');
  await expect(page.locator('[data-testid="data-view"]')).toBeVisible({
    timeout: 5000,
  });
  // Wait for initial pathSearch to complete (loading overlay gone)
  await expect(page.locator(".loading-overlay")).not.toBeVisible({
    timeout: 5000,
  });
}

// ---------------------------------------------------------------------------
// NLQ Cancellation UX E2E Tests
// ---------------------------------------------------------------------------

test.describe("NLQ Cancellation UX", () => {
  let flowId: string;

  test.beforeEach(async ({ page }) => {
    const setup = await createRunningFlow(page);
    flowId = setup.flowId;
    await page.goto(setup.url);
    await expect(page.locator("#flow-canvas")).toBeVisible();
    await page.waitForLoadState("networkidle");
  });

  test.afterEach(async ({ page }) => {
    if (flowId) {
      await deleteTestFlow(page, flowId);
    }
  });

  // -------------------------------------------------------------------------
  // NlqSearchBar presence
  // -------------------------------------------------------------------------

  test("should show NlqSearchBar search input in DataView", async ({
    page,
  }) => {
    await enterDataView(page);

    const searchInput = page.getByRole("textbox", {
      name: /search knowledge graph/i,
    });
    await expect(searchInput).toBeVisible();
  });

  // -------------------------------------------------------------------------
  // Loading indicator and cancel button
  // -------------------------------------------------------------------------

  test("should show loading indicator and cancel button while globalSearch is in flight", async ({
    page,
  }) => {
    // Hold the globalSearch response until we verify the loading state
    let resolveSearch: (() => void) | null = null;

    await page.route("**/graphql", async (route) => {
      const body = route.request().postDataJSON();
      if (body?.query?.includes("pathSearch")) {
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(MOCK_PATH_SEARCH_RESPONSE),
        });
      } else if (body?.query?.includes("globalSearch")) {
        // Hold this response until test releases it
        await new Promise<void>((resolve) => {
          resolveSearch = resolve;
        });
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(MOCK_GLOBAL_SEARCH_RESPONSE),
        });
      } else {
        route.continue();
      }
    });

    await page.click('[data-testid="view-switch-data"]');
    await expect(page.locator('[data-testid="data-view"]')).toBeVisible({
      timeout: 5000,
    });
    await expect(page.locator(".loading-overlay")).not.toBeVisible({
      timeout: 5000,
    });

    // Submit a search query
    const searchInput = page.getByRole("textbox", {
      name: /search knowledge graph/i,
    });
    await searchInput.fill("show all drones");
    await searchInput.press("Enter");

    // Loading indicator should appear
    await expect(
      page.locator('[data-testid="nlq-loading-indicator"]'),
    ).toBeVisible({ timeout: 3000 });

    // Cancel button should appear alongside loading indicator
    await expect(page.locator('[data-testid="nlq-cancel-button"]')).toBeVisible(
      { timeout: 1000 },
    );

    // Release the held response
    resolveSearch?.();

    // Loading indicator and cancel button should disappear after completion
    await expect(
      page.locator('[data-testid="nlq-loading-indicator"]'),
    ).not.toBeVisible({ timeout: 5000 });
    await expect(
      page.locator('[data-testid="nlq-cancel-button"]'),
    ).not.toBeVisible({ timeout: 1000 });
  });

  // -------------------------------------------------------------------------
  // Cancel clears loading state without showing an error
  // -------------------------------------------------------------------------

  test("should hide loading indicator after cancel and show no error", async ({
    page,
  }) => {
    let resolveSearch: (() => void) | null = null;

    await page.route("**/graphql", async (route) => {
      const body = route.request().postDataJSON();
      if (body?.query?.includes("pathSearch")) {
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(MOCK_PATH_SEARCH_RESPONSE),
        });
      } else if (body?.query?.includes("globalSearch")) {
        await new Promise<void>((resolve) => {
          resolveSearch = resolve;
        });
        route.abort("aborted");
      } else {
        route.continue();
      }
    });

    await page.click('[data-testid="view-switch-data"]');
    await expect(page.locator('[data-testid="data-view"]')).toBeVisible({
      timeout: 5000,
    });
    await expect(page.locator(".loading-overlay")).not.toBeVisible({
      timeout: 5000,
    });

    // Submit search
    const searchInput = page.getByRole("textbox", {
      name: /search knowledge graph/i,
    });
    await searchInput.fill("cancel this search");
    await searchInput.press("Enter");

    // Wait for cancel button
    await expect(page.locator('[data-testid="nlq-cancel-button"]')).toBeVisible(
      { timeout: 3000 },
    );

    // Click cancel — this aborts the in-flight request
    await page.locator('[data-testid="nlq-cancel-button"]').click();

    // Release held request so the route handler can complete
    resolveSearch?.();

    // Loading indicator should disappear
    await expect(
      page.locator('[data-testid="nlq-loading-indicator"]'),
    ).not.toBeVisible({ timeout: 3000 });

    // No error alert should appear after cancel
    await expect(page.getByRole("alert")).not.toBeVisible({ timeout: 1000 });
  });

  // -------------------------------------------------------------------------
  // Elapsed timer increments during long-running searches
  // -------------------------------------------------------------------------

  test("should show elapsed time counter during in-flight search", async ({
    page,
  }) => {
    let resolveSearch: (() => void) | null = null;

    await page.route("**/graphql", async (route) => {
      const body = route.request().postDataJSON();
      if (body?.query?.includes("pathSearch")) {
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(MOCK_PATH_SEARCH_RESPONSE),
        });
      } else if (body?.query?.includes("globalSearch")) {
        await new Promise<void>((resolve) => {
          resolveSearch = resolve;
        });
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(MOCK_GLOBAL_SEARCH_RESPONSE),
        });
      } else {
        route.continue();
      }
    });

    await page.click('[data-testid="view-switch-data"]');
    await expect(page.locator('[data-testid="data-view"]')).toBeVisible({
      timeout: 5000,
    });
    await expect(page.locator(".loading-overlay")).not.toBeVisible({
      timeout: 5000,
    });

    // Submit search
    const searchInput = page.getByRole("textbox", {
      name: /search knowledge graph/i,
    });
    await searchInput.fill("slow query");
    await searchInput.press("Enter");

    // Elapsed timer element should appear
    await expect(page.locator('[data-testid="nlq-elapsed-time"]')).toBeVisible({
      timeout: 3000,
    });

    // Initial value should be "0s"
    await expect(page.locator('[data-testid="nlq-elapsed-time"]')).toHaveText(
      "0s",
    );

    // Release the search
    resolveSearch?.();

    // Timer should disappear after completion
    await expect(
      page.locator('[data-testid="nlq-elapsed-time"]'),
    ).not.toBeVisible({ timeout: 3000 });
  });

  // -------------------------------------------------------------------------
  // Successful search shows "Back to browse" button
  // -------------------------------------------------------------------------

  test("should show Back to browse button after successful globalSearch", async ({
    page,
  }) => {
    await page.route("**/graphql", (route) => {
      const body = route.request().postDataJSON();
      if (body?.query?.includes("pathSearch")) {
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(MOCK_PATH_SEARCH_RESPONSE),
        });
      } else if (body?.query?.includes("globalSearch")) {
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(MOCK_GLOBAL_SEARCH_RESPONSE),
        });
      } else {
        route.continue();
      }
    });

    await page.click('[data-testid="view-switch-data"]');
    await expect(page.locator('[data-testid="data-view"]')).toBeVisible({
      timeout: 5000,
    });
    await expect(page.locator(".loading-overlay")).not.toBeVisible({
      timeout: 5000,
    });

    // Submit search
    const searchInput = page.getByRole("textbox", {
      name: /search knowledge graph/i,
    });
    await searchInput.fill("fleet alpha");
    await searchInput.press("Enter");

    // Wait for search to complete — "Back to browse" signals search mode
    await expect(
      page.getByRole("button", { name: /back to browse/i }),
    ).toBeVisible({ timeout: 5000 });
  });

  // -------------------------------------------------------------------------
  // Back to browse exits search mode and reloads graph
  // -------------------------------------------------------------------------

  test("should exit search mode and reload graph when Back to browse is clicked", async ({
    page,
  }) => {
    let requestCount = 0;

    await page.route("**/graphql", (route) => {
      const body = route.request().postDataJSON();
      if (body?.query?.includes("pathSearch")) {
        requestCount++;
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(MOCK_PATH_SEARCH_RESPONSE),
        });
      } else if (body?.query?.includes("globalSearch")) {
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(MOCK_GLOBAL_SEARCH_RESPONSE),
        });
      } else {
        route.continue();
      }
    });

    await page.click('[data-testid="view-switch-data"]');
    await expect(page.locator('[data-testid="data-view"]')).toBeVisible({
      timeout: 5000,
    });
    await expect(page.locator(".loading-overlay")).not.toBeVisible({
      timeout: 5000,
    });

    // Initial pathSearch should have been called once
    expect(requestCount).toBe(1);

    // Submit search to enter search mode
    const searchInput = page.getByRole("textbox", {
      name: /search knowledge graph/i,
    });
    await searchInput.fill("fleet alpha");
    await searchInput.press("Enter");

    await expect(
      page.getByRole("button", { name: /back to browse/i }),
    ).toBeVisible({ timeout: 5000 });

    // Click Back to browse
    await page.getByRole("button", { name: /back to browse/i }).click();

    // Should exit search mode (Back to browse disappears)
    await expect(
      page.getByRole("button", { name: /back to browse/i }),
    ).not.toBeVisible({ timeout: 3000 });

    // pathSearch should have been called a second time to reload browse data
    expect(requestCount).toBe(2);

    // Search input should be cleared
    await expect(searchInput).toHaveValue("");
  });

  // -------------------------------------------------------------------------
  // Error handling: globalSearch failure shows alert
  // -------------------------------------------------------------------------

  test("should show error alert when globalSearch fails", async ({ page }) => {
    await page.route("**/graphql", (route) => {
      const body = route.request().postDataJSON();
      if (body?.query?.includes("pathSearch")) {
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(MOCK_PATH_SEARCH_RESPONSE),
        });
      } else if (body?.query?.includes("globalSearch")) {
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            errors: [{ message: "NLQ classifier unavailable" }],
          }),
        });
      } else {
        route.continue();
      }
    });

    await page.click('[data-testid="view-switch-data"]');
    await expect(page.locator('[data-testid="data-view"]')).toBeVisible({
      timeout: 5000,
    });
    await expect(page.locator(".loading-overlay")).not.toBeVisible({
      timeout: 5000,
    });

    // Submit search
    const searchInput = page.getByRole("textbox", {
      name: /search knowledge graph/i,
    });
    await searchInput.fill("trigger error");
    await searchInput.press("Enter");

    // Error alert should appear
    await expect(page.getByRole("alert")).toBeVisible({ timeout: 5000 });

    // DataView itself should still be intact
    await expect(page.locator('[data-testid="data-view"]')).toBeVisible();
  });
});
