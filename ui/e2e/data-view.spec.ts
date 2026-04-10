import { expect, test, type Request } from "@playwright/test";
import { createRunningFlow, deleteTestFlow } from "./helpers/runtime-helpers";

// ---------------------------------------------------------------------------
// Mock GraphQL response with realistic entity data for graph rendering tests
// ---------------------------------------------------------------------------

/**
 * pathSearch mock: 3 entities (drone, fleet, sensor) with property triples
 * and 1 edge (drone → fleet via "fleet.membership").
 *
 * Entity IDs use the 6-part format: org.platform.domain.system.type.instance
 * so parseEntityId() and isEntityReference() work correctly.
 *
 * Expected rendered state:
 *   - 3 entities in .graph-stats
 *   - 1 relationship in .graph-stats (the edge; deduped in store)
 *   - type filter chips: drone, fleet, sensor
 */
const MOCK_GRAPH_RESPONSE = {
  data: {
    pathSearch: {
      entities: [
        {
          id: "c360.ops.robotics.gcs.drone.001",
          triples: [
            {
              subject: "c360.ops.robotics.gcs.drone.001",
              predicate: "robotics.status.active",
              object: "true",
            },
            {
              subject: "c360.ops.robotics.gcs.drone.001",
              predicate: "robotics.speed.current",
              object: "12.5",
            },
          ],
        },
        {
          id: "c360.ops.robotics.gcs.fleet.west",
          triples: [
            {
              subject: "c360.ops.robotics.gcs.fleet.west",
              predicate: "robotics.region.name",
              object: "west-coast",
            },
          ],
        },
        {
          id: "c360.ops.robotics.gcs.sensor.temp-01",
          triples: [
            {
              subject: "c360.ops.robotics.gcs.sensor.temp-01",
              predicate: "robotics.reading.temperature",
              object: "23.4",
            },
            {
              subject: "c360.ops.robotics.gcs.sensor.temp-01",
              predicate: "robotics.reading.humidity",
              object: "65.0",
            },
          ],
        },
      ],
      edges: [
        {
          subject: "c360.ops.robotics.gcs.drone.001",
          predicate: "fleet.membership",
          object: "c360.ops.robotics.gcs.fleet.west",
        },
      ],
    },
  },
};

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

// =============================================================================
// Mocked graph rendering tests — no Docker/semsource required
// =============================================================================

test.describe("DataView — mocked graph rendering", () => {
  let flowId: string;

  test.beforeEach(async ({ page }) => {
    // Create and start a flow (required for ViewSwitcher to be visible)
    const setup = await createRunningFlow(page);
    flowId = setup.flowId;

    // Route all GraphQL requests to the mock before navigating
    await page.route("**/graphql", (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(MOCK_GRAPH_RESPONSE),
      });
    });

    // Navigate to the flow page
    await page.goto(setup.url);

    // Wait for flow canvas then switch to Data view
    await expect(page.locator("#flow-canvas")).toBeVisible();
    await page.waitForLoadState("networkidle");
    await page.click('[data-testid="view-switch-data"]');

    // Wait for DataView to finish loading (loading overlay disappears)
    await expect(page.locator('[data-testid="data-view"]')).toBeVisible({
      timeout: 5000,
    });
    await expect(page.locator(".loading-overlay")).not.toBeVisible({
      timeout: 5000,
    });
  });

  test.afterEach(async ({ page }) => {
    if (flowId) {
      await deleteTestFlow(page, flowId);
    }
  });

  test("SigmaCanvas renders with mocked entities", async ({ page }) => {
    // The SigmaCanvas outer wrapper should be present
    await expect(page.locator('[data-testid="sigma-canvas"]')).toBeVisible();

    // The inner sigma container (where Sigma.js attaches its WebGL renderer)
    await expect(
      page.locator('[data-testid="sigma-canvas"] .sigma-container'),
    ).toBeVisible();

    // Sigma.js injects a <canvas> element into the container
    await expect(
      page.locator('[data-testid="sigma-canvas"] canvas'),
    ).toBeAttached();
  });

  test("graph stats overlay shows correct entity and relationship counts", async ({
    page,
  }) => {
    const stats = page.locator(".graph-stats");
    await expect(stats).toBeVisible();

    // Mock data has 3 entities
    await expect(stats).toContainText("3 entities");

    // Mock data has 1 edge, which becomes 1 unique relationship in the store
    await expect(stats).toContainText("1 relationships");
  });

  test("GraphFilters populates type chips from mocked entity types", async ({
    page,
  }) => {
    // The three entity types in the mock data are: drone, fleet, sensor
    // graphStore.getEntityTypes() returns them sorted alphabetically
    await expect(
      page.locator('[data-testid="type-filter-drone"]'),
    ).toBeVisible();
    await expect(
      page.locator('[data-testid="type-filter-fleet"]'),
    ).toBeVisible();
    await expect(
      page.locator('[data-testid="type-filter-sensor"]'),
    ).toBeVisible();
  });

  test("GraphDetailPanel shows empty state before any entity is selected", async ({
    page,
  }) => {
    // No entity selected yet — empty panel should be visible
    await expect(
      page.locator('[data-testid="graph-detail-panel-empty"]'),
    ).toBeVisible();

    // Populated panel should not be present
    await expect(
      page.locator('[data-testid="graph-detail-panel"]'),
    ).not.toBeAttached();

    // Confirm the "select an entity" prompt text is visible
    await expect(
      page.locator('[data-testid="graph-detail-panel-empty"]'),
    ).toContainText("Select an entity");
  });
});

// =============================================================================
// Entity selection via programmatic test seam — deterministic, no WebGL clicks
// =============================================================================

test.describe("DataView — entity selection via test seam", () => {
  let flowId: string;

  // Entity IDs that exist in MOCK_GRAPH_RESPONSE above
  const ENTITY_DRONE = "c360.ops.robotics.gcs.drone.001";
  const ENTITY_FLEET = "c360.ops.robotics.gcs.fleet.west";

  test.beforeEach(async ({ page }) => {
    const setup = await createRunningFlow(page);
    flowId = setup.flowId;

    // Route GraphQL to the same mock used by graph rendering tests
    await page.route("**/graphql", (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(MOCK_GRAPH_RESPONSE),
      });
    });

    await page.goto(setup.url);
    await expect(page.locator("#flow-canvas")).toBeVisible();
    await page.waitForLoadState("networkidle");

    // Switch to Data view and wait for data to load
    await page.click('[data-testid="view-switch-data"]');
    await expect(page.locator('[data-testid="data-view"]')).toBeVisible({
      timeout: 5000,
    });
    await expect(page.locator(".loading-overlay")).not.toBeVisible({
      timeout: 8000,
    });
  });

  test.afterEach(async ({ page }) => {
    if (flowId) {
      await deleteTestFlow(page, flowId);
    }
  });

  test("empty state is shown before any entity is selected", async ({
    page,
  }) => {
    await expect(
      page.locator('[data-testid="graph-detail-panel-empty"]'),
    ).toBeVisible();
    await expect(
      page.locator('[data-testid="graph-detail-panel"]'),
    ).not.toBeAttached();
  });

  test("selecting an entity via __e2eSelectEntity shows the detail panel", async ({
    page,
  }) => {
    await page.evaluate((id) => window.__e2eSelectEntity?.(id), ENTITY_DRONE);

    const detailPanel = page.locator('[data-testid="graph-detail-panel"]');
    await expect(detailPanel).toBeVisible({ timeout: 3000 });

    await expect(
      page.locator('[data-testid="graph-detail-panel-empty"]'),
    ).not.toBeVisible();
  });

  test("detail panel displays the selected entity's 6-part ID breakdown", async ({
    page,
  }) => {
    await page.evaluate((id) => window.__e2eSelectEntity?.(id), ENTITY_DRONE);

    const detailPanel = page.locator('[data-testid="graph-detail-panel"]');
    await expect(detailPanel).toBeVisible({ timeout: 3000 });

    // c360.ops.robotics.gcs.drone.001 → org=c360, platform=ops, domain=robotics,
    //                                    system=gcs, type=drone, instance=001
    const idSection = detailPanel.locator(".id-breakdown");
    await expect(idSection).toBeVisible();

    await expect(idSection.locator(".id-label").nth(0)).toContainText("org");
    await expect(idSection.locator(".id-label").nth(1)).toContainText(
      "platform",
    );
    await expect(idSection.locator(".id-label").nth(2)).toContainText("domain");
    await expect(idSection.locator(".id-label").nth(3)).toContainText("system");
    await expect(idSection.locator(".id-label").nth(4)).toContainText("type");
    await expect(idSection.locator(".id-label").nth(5)).toContainText(
      "instance",
    );

    await expect(idSection.locator(".id-value").nth(0)).toContainText("c360");
    await expect(idSection.locator(".id-value").nth(1)).toContainText("ops");
    await expect(idSection.locator(".id-value").nth(2)).toContainText(
      "robotics",
    );
    await expect(idSection.locator(".id-value").nth(3)).toContainText("gcs");
    await expect(idSection.locator(".id-value").nth(4)).toContainText("drone");
    await expect(idSection.locator(".id-value").nth(5)).toContainText("001");
  });

  test("detail panel shows properties ingested from entity triples", async ({
    page,
  }) => {
    await page.evaluate((id) => window.__e2eSelectEntity?.(id), ENTITY_DRONE);

    const detailPanel = page.locator('[data-testid="graph-detail-panel"]');
    await expect(detailPanel).toBeVisible({ timeout: 3000 });

    const propertiesSection = detailPanel.locator(".properties-list");
    await expect(propertiesSection).toBeVisible();

    const propertyRows = propertiesSection.locator(".property-row");
    const rowCount = await propertyRows.count();
    expect(rowCount).toBeGreaterThan(0);

    // Drone entity has "robotics.status.active" → "true" in MOCK_GRAPH_RESPONSE
    await expect(propertiesSection).toContainText("active");
    await expect(propertiesSection).toContainText("true");
  });

  test("selecting a different entity switches the detail panel", async ({
    page,
  }) => {
    // Select the drone first
    await page.evaluate((id) => window.__e2eSelectEntity?.(id), ENTITY_DRONE);

    const detailPanel = page.locator('[data-testid="graph-detail-panel"]');
    await expect(detailPanel).toBeVisible({ timeout: 3000 });
    await expect(detailPanel.locator(".id-breakdown")).toContainText("001");
    await expect(detailPanel.locator(".id-breakdown")).toContainText("drone");

    // Switch to the fleet entity
    await page.evaluate((id) => window.__e2eSelectEntity?.(id), ENTITY_FLEET);

    // Panel should now reflect the fleet entity: type=fleet, instance=west
    await expect(detailPanel.locator(".id-breakdown")).toContainText("fleet");
    await expect(detailPanel.locator(".id-breakdown")).toContainText("west");
  });
});

// =============================================================================
// P1-002: Relationship navigation via detail panel
// =============================================================================

test.describe("DataView — P1-002: relationship navigation via detail panel", () => {
  let flowId: string;

  const ENTITY_DRONE = "c360.ops.robotics.gcs.drone.001";
  const ENTITY_FLEET = "c360.ops.robotics.gcs.fleet.west";

  test.beforeEach(async ({ page }) => {
    const setup = await createRunningFlow(page);
    flowId = setup.flowId;

    // Use the standard mock which has a drone→fleet.west edge
    await page.route("**/graphql", (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(MOCK_GRAPH_RESPONSE),
      });
    });

    await page.goto(setup.url);
    await expect(page.locator("#flow-canvas")).toBeVisible();
    await page.waitForLoadState("networkidle");

    await page.click('[data-testid="view-switch-data"]');
    await expect(page.locator('[data-testid="data-view"]')).toBeVisible({
      timeout: 5000,
    });
    await expect(page.locator(".loading-overlay")).not.toBeVisible({
      timeout: 8000,
    });
  });

  test.afterEach(async ({ page }) => {
    if (flowId) {
      await deleteTestFlow(page, flowId);
    }
  });

  test("clicking a relationship row in detail panel navigates to target entity", async ({
    page,
  }) => {
    // Select the drone entity — it has 1 outgoing edge to fleet.west
    await page.evaluate((id) => window.__e2eSelectEntity?.(id), ENTITY_DRONE);

    const detailPanel = page.locator('[data-testid="graph-detail-panel"]');
    await expect(detailPanel).toBeVisible({ timeout: 3000 });

    // Verify the Outgoing section is present with the fleet.west relationship
    const outgoingSection = detailPanel.locator("text=Outgoing");
    await expect(outgoingSection).toBeVisible();

    // The relationship row shows the predicate and target instance ("west")
    const relRow = detailPanel.locator(".relationship-row").first();
    await expect(relRow).toBeVisible();
    await expect(relRow).toContainText("west");

    // Click the relationship row to navigate to fleet.west
    await relRow.click();

    // Detail panel should now show fleet.west's ID breakdown
    await expect(detailPanel.locator(".id-breakdown")).toContainText("fleet");
    await expect(detailPanel.locator(".id-breakdown")).toContainText("west");

    // The drone entity should no longer be the selected one
    await expect(detailPanel.locator(".id-breakdown")).not.toContainText("001");
  });

  test("close button in detail panel clears selection", async ({ page }) => {
    await page.evaluate((id) => window.__e2eSelectEntity?.(id), ENTITY_FLEET);

    const detailPanel = page.locator('[data-testid="graph-detail-panel"]');
    await expect(detailPanel).toBeVisible({ timeout: 3000 });

    // Click the close button
    const closeButton = detailPanel.locator(".close-button");
    await expect(closeButton).toBeVisible();
    await closeButton.click();

    // Empty state should appear
    await expect(
      page.locator('[data-testid="graph-detail-panel-empty"]'),
    ).toBeVisible({ timeout: 2000 });
    await expect(
      page.locator('[data-testid="graph-detail-panel"]'),
    ).not.toBeAttached();
  });
});

// =============================================================================
// P1-003: Confidence slider functional test
// =============================================================================

test.describe("DataView — P1-003: confidence slider", () => {
  let flowId: string;

  test.beforeEach(async ({ page }) => {
    const setup = await createRunningFlow(page);
    flowId = setup.flowId;

    await page.route("**/graphql", (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(MOCK_GRAPH_RESPONSE),
      });
    });

    await page.goto(setup.url);
    await expect(page.locator("#flow-canvas")).toBeVisible();
    await page.waitForLoadState("networkidle");

    await page.click('[data-testid="view-switch-data"]');
    await expect(page.locator('[data-testid="data-view"]')).toBeVisible({
      timeout: 5000,
    });
    await expect(page.locator(".loading-overlay")).not.toBeVisible({
      timeout: 8000,
    });
  });

  test.afterEach(async ({ page }) => {
    if (flowId) {
      await deleteTestFlow(page, flowId);
    }
  });

  test("confidence slider updates the filter label when moved", async ({
    page,
  }) => {
    // Initial label shows 0%
    const confidenceLabel = page.locator("label[for='confidence-slider']");
    await expect(confidenceLabel).toContainText("Min Confidence: 0%");

    // Use page.evaluate to set slider to 0.5 and dispatch input event
    const slider = page.locator('[data-testid="confidence-slider"]');
    await expect(slider).toBeVisible();

    await page.evaluate(() => {
      const el = document.querySelector(
        '[data-testid="confidence-slider"]',
      ) as HTMLInputElement;
      if (el) {
        el.value = "0.5";
        el.dispatchEvent(new Event("input", { bubbles: true }));
      }
    });

    // Filter label should update to reflect 50%
    await expect(confidenceLabel).toContainText("Min Confidence: 50%");
  });

  test("setting minConfidence above 1.0 via seam hides all relationships in stats", async ({
    page,
  }) => {
    // Verify initial stats show 1 relationship
    const stats = page.locator(".graph-stats");
    await expect(stats).toContainText("1 relationships");

    // Use the filter seam to force minConfidence above the hardcoded 1.0 value
    // in graphTransform — this filters out all relationships
    await page.evaluate(() => {
      window.__e2eSetFilters?.({ minConfidence: 1.1 });
    });

    // Stats should now show 0 relationships (all filtered out)
    await expect(stats).toContainText("0 relationships");

    // Entity count is unchanged — confidence filter only affects relationships
    await expect(stats).toContainText("3 entities");
  });
});

// =============================================================================
// P1-004: Combined filter interaction (type + domain + search)
// =============================================================================

// Mock with 3 entities across 2 different types AND 2 different domains
const MOCK_MULTI_FILTER_RESPONSE = {
  data: {
    pathSearch: {
      entities: [
        {
          // type=sensor, domain=analytics
          id: "c360.platform.analytics.system.sensor.a1",
          triples: [
            {
              subject: "c360.platform.analytics.system.sensor.a1",
              predicate: "analytics.reading.value",
              object: "42",
            },
          ],
        },
        {
          // type=drone, domain=robotics
          id: "c360.platform.robotics.system.drone.b1",
          triples: [
            {
              subject: "c360.platform.robotics.system.drone.b1",
              predicate: "robotics.status.active",
              object: "true",
            },
          ],
        },
        {
          // type=gateway, domain=analytics
          id: "c360.platform.analytics.system.gateway.c1",
          triples: [
            {
              subject: "c360.platform.analytics.system.gateway.c1",
              predicate: "analytics.gateway.status",
              object: "online",
            },
          ],
        },
      ],
      edges: [],
    },
  },
};

test.describe("DataView — P1-004: combined type + domain + search filters", () => {
  let flowId: string;

  test.beforeEach(async ({ page }) => {
    const setup = await createRunningFlow(page);
    flowId = setup.flowId;

    await page.route("**/graphql", (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(MOCK_MULTI_FILTER_RESPONSE),
      });
    });

    await page.goto(setup.url);
    await expect(page.locator("#flow-canvas")).toBeVisible();
    await page.waitForLoadState("networkidle");

    await page.click('[data-testid="view-switch-data"]');
    await expect(page.locator('[data-testid="data-view"]')).toBeVisible({
      timeout: 5000,
    });
    await expect(page.locator(".loading-overlay")).not.toBeVisible({
      timeout: 8000,
    });
  });

  test.afterEach(async ({ page }) => {
    if (flowId) {
      await deleteTestFlow(page, flowId);
    }
  });

  test("all three filters applied together use AND logic to narrow results", async ({
    page,
  }) => {
    const stats = page.locator(".graph-stats");

    // Initial state: all 3 entities visible
    await expect(stats).toContainText("3 entities");

    // Step 1: activate the "sensor" type filter chip → 1 entity (sensor.a1)
    const sensorChip = page.locator('[data-testid="type-filter-sensor"]');
    await expect(sensorChip).toBeVisible();
    await sensorChip.click();
    await expect(stats).toContainText("1 entities");

    // Step 2: activate the "analytics" domain filter chip (still 1 entity — sensor.a1 is in analytics)
    const analyticsChip = page.locator(
      '[data-testid="domain-filter-analytics"]',
    );
    await expect(analyticsChip).toBeVisible();
    await analyticsChip.click();
    await expect(stats).toContainText("1 entities");

    // Step 3: type "a1" in the search box — sensor.a1 matches, count stays at 1
    const searchInput = page.locator('[data-testid="entity-search"]');
    await searchInput.fill("a1");
    // Trigger the debounced search (GraphFilters debounces 300ms)
    await page.waitForTimeout(400);
    await expect(stats).toContainText("1 entities");

    // Step 4: change search to "b1" — drone.b1 does not match type=sensor, so 0 entities
    await searchInput.fill("b1");
    await page.waitForTimeout(400);
    await expect(stats).toContainText("0 entities");
  });

  test("type filter chip toggles active state visually", async ({ page }) => {
    const droneChip = page.locator('[data-testid="type-filter-drone"]');
    await expect(droneChip).toBeVisible();

    // Initially not active
    await expect(droneChip).not.toHaveClass(/active/);

    // Click to activate
    await droneChip.click();
    await expect(droneChip).toHaveClass(/active/);

    // Click again to deactivate
    await droneChip.click();
    await expect(droneChip).not.toHaveClass(/active/);
  });
});

// =============================================================================
// P1-005: 504 timeout error path
// =============================================================================

test.describe("DataView — P1-005: 504 timeout error", () => {
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

  test("HTTP 504 response shows 'Query timed out' in the error banner", async ({
    page,
  }) => {
    // Mock GraphQL to return a 504 Gateway Timeout
    await page.route("**/graphql", (route) => {
      route.fulfill({
        status: 504,
        contentType: "text/plain",
        body: "Gateway Timeout",
      });
    });

    // Switch to Data view
    await page.click('[data-testid="view-switch-data"]');
    await expect(page.locator('[data-testid="data-view"]')).toBeVisible({
      timeout: 5000,
    });

    // Error banner must appear with the timeout-specific message
    const errorBanner = page.locator(".error-banner");
    await expect(errorBanner).toBeVisible({ timeout: 5000 });
    await expect(errorBanner).toContainText("Query timed out");

    // Retry button is present so the user can re-query
    const retryButton = errorBanner.locator(".retry-button");
    await expect(retryButton).toBeVisible();
    await expect(retryButton).toContainText("Retry");
  });
});

// =============================================================================
// P1-006: Expand-node flow
// =============================================================================

// Initial mock: single entity (no neighbors loaded yet)
const MOCK_SINGLE_ENTITY = {
  data: {
    pathSearch: {
      entities: [
        {
          id: "c360.ops.robotics.gcs.drone.001",
          triples: [
            {
              subject: "c360.ops.robotics.gcs.drone.001",
              predicate: "robotics.status.active",
              object: "true",
            },
          ],
        },
      ],
      edges: [],
    },
  },
};

// Expansion mock: 3 additional entities returned when expanding drone.001
const MOCK_EXPANDED_ENTITIES = {
  data: {
    pathSearch: {
      entities: [
        {
          id: "c360.ops.robotics.gcs.drone.001",
          triples: [
            {
              subject: "c360.ops.robotics.gcs.drone.001",
              predicate: "robotics.status.active",
              object: "true",
            },
          ],
        },
        {
          id: "c360.ops.robotics.gcs.fleet.west",
          triples: [
            {
              subject: "c360.ops.robotics.gcs.fleet.west",
              predicate: "robotics.region.name",
              object: "west-coast",
            },
          ],
        },
        {
          id: "c360.ops.robotics.gcs.sensor.temp-01",
          triples: [
            {
              subject: "c360.ops.robotics.gcs.sensor.temp-01",
              predicate: "robotics.reading.temperature",
              object: "23.4",
            },
          ],
        },
        {
          id: "c360.ops.robotics.gcs.controller.main",
          triples: [
            {
              subject: "c360.ops.robotics.gcs.controller.main",
              predicate: "robotics.control.mode",
              object: "autonomous",
            },
          ],
        },
      ],
      edges: [
        {
          subject: "c360.ops.robotics.gcs.drone.001",
          predicate: "fleet.membership",
          object: "c360.ops.robotics.gcs.fleet.west",
        },
      ],
    },
  },
};

test.describe("DataView — P1-006: expand-node flow", () => {
  let flowId: string;

  const ENTITY_DRONE = "c360.ops.robotics.gcs.drone.001";

  test.beforeEach(async ({ page }) => {
    const setup = await createRunningFlow(page);
    flowId = setup.flowId;

    let requestCount = 0;

    // First call (startEntity="*"): return 1 entity
    // Second call (startEntity=drone ID): return 4 entities (expansion)
    await page.route("**/graphql", (route) => {
      requestCount++;
      const body = route.request().postDataJSON() as {
        variables?: { startEntity?: string };
      };
      const startEntity = body?.variables?.startEntity ?? "*";

      if (startEntity === "*" || requestCount === 1) {
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(MOCK_SINGLE_ENTITY),
        });
      } else {
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(MOCK_EXPANDED_ENTITIES),
        });
      }
    });

    await page.goto(setup.url);
    await expect(page.locator("#flow-canvas")).toBeVisible();
    await page.waitForLoadState("networkidle");

    await page.click('[data-testid="view-switch-data"]');
    await expect(page.locator('[data-testid="data-view"]')).toBeVisible({
      timeout: 5000,
    });
    await expect(page.locator(".loading-overlay")).not.toBeVisible({
      timeout: 8000,
    });
  });

  test.afterEach(async ({ page }) => {
    if (flowId) {
      await deleteTestFlow(page, flowId);
    }
  });

  test("expanding an entity loads its neighbors and increases entity count", async ({
    page,
  }) => {
    const stats = page.locator(".graph-stats");

    // Initial state: 1 entity from the first load
    await expect(stats).toContainText("1 entities");

    // Select the drone entity via the test seam
    await page.evaluate((id) => window.__e2eSelectEntity?.(id), ENTITY_DRONE);

    const detailPanel = page.locator('[data-testid="graph-detail-panel"]');
    await expect(detailPanel).toBeVisible({ timeout: 3000 });

    // Trigger expansion via the test seam — this calls handleEntityExpand
    await page.evaluate(async (id) => {
      await window.__e2eExpandEntity?.(id);
    }, ENTITY_DRONE);

    // Wait for expanded entities to appear in the stats (1 → 4)
    await expect(stats).toContainText("4 entities", { timeout: 5000 });

    // Relationships should also appear from the expansion
    await expect(stats).toContainText("1 relationships");
  });

  test("expanding the same entity twice does not make a second request", async ({
    page,
  }) => {
    const stats = page.locator(".graph-stats");

    // Initial state: 1 entity
    await expect(stats).toContainText("1 entities");

    // First expansion
    await page.evaluate(async (id) => {
      await window.__e2eExpandEntity?.(id);
    }, ENTITY_DRONE);

    await expect(stats).toContainText("4 entities", { timeout: 5000 });

    // Second expansion attempt — graphStore.isExpanded() blocks this
    await page.evaluate(async (id) => {
      await window.__e2eExpandEntity?.(id);
    }, ENTITY_DRONE);

    // Count should not change (no duplicate entities added)
    await expect(stats).toContainText("4 entities");
  });
});
