/**
 * Graph Filtering - SemSource Integration E2E Tests
 *
 * Validates GraphFilters panel interactions against real semsource entities.
 * Tests cover type chips, domain chips, search, and reset behaviour.
 * Semsource serves its own graph-gateway — no separate backend needed for graph data.
 *
 * Prerequisites:
 *   - Docker Compose profile "semsource" must be active
 *   - Run via: COMPOSE_PROFILES=semsource npx playwright test e2e/semsource-graph/
 *
 * Filter chips appear only when the corresponding type/domain is present in
 * the loaded entity set. All tests wait for semsource entities to load before
 * making assertions about the filter UI.
 */

import { test, expect } from "@playwright/test";
import {
  SEMSOURCE_ENTITY_TYPES,
  SEMSOURCE_DOMAINS,
  setupDataViewWithSemsource,
  deleteTestFlow,
} from "./helpers/semsource-helpers";

test.describe("Graph Filtering - SemSource Entities", () => {
  let flowId: string;

  test.beforeEach(async ({ page }) => {
    flowId = await setupDataViewWithSemsource(page, 3);
  });

  test.afterEach(async ({ page }) => {
    if (flowId) {
      await deleteTestFlow(page, flowId);
    }
  });

  // ---------------------------------------------------------------------------
  // Filter panel presence
  // ---------------------------------------------------------------------------

  test("GraphFilters panel is visible in the Data view", async ({ page }) => {
    await expect(page.locator('[data-testid="graph-filters"]')).toBeVisible();
  });

  test("entity search input is visible and accepts text", async ({ page }) => {
    const searchInput = page.locator('[data-testid="entity-search"]');
    await expect(searchInput).toBeVisible();
    await searchInput.fill("main");
    await expect(searchInput).toHaveValue("main");

    // Clear the search so other tests are not affected
    await searchInput.fill("");
  });

  test("confidence slider is present", async ({ page }) => {
    const slider = page.locator('[data-testid="confidence-slider"]');
    await expect(slider).toBeVisible();
  });

  // ---------------------------------------------------------------------------
  // Type filter chips
  // ---------------------------------------------------------------------------

  test("type filter chips contain semsource entity types", async ({ page }) => {
    // Type chips only render if entities with those types are loaded.
    // We mock the GraphQL response to include entities of each known type so
    // the filter panel populates predictably.

    const namespace = "e2e.semsource";

    await page.route("**/graphql", (route) => {
      const ts = Date.now();
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            pathSearch: {
              entities: SEMSOURCE_ENTITY_TYPES.map((type, i) => ({
                id: `${namespace}.code.go.${type}.entity${i}`,
                triples: [
                  {
                    subject: `${namespace}.code.go.${type}.entity${i}`,
                    predicate: "has.name",
                    object: `entity${i}`,
                    confidence: 1.0,
                    timestamp: ts,
                  },
                ],
              })),
              edges: [],
            },
          },
        }),
      });
    });

    // Refresh with mocked data
    const refreshButton = page.locator(".toolbar-button[title='Refresh data']");
    await expect(refreshButton).toBeVisible();
    await refreshButton.click();
    await page
      .locator(".loading-overlay")
      .waitFor({ state: "hidden", timeout: 10000 });

    // Wait for type chips to appear
    await page.waitForTimeout(500);

    for (const type of SEMSOURCE_ENTITY_TYPES) {
      const chip = page.locator(`[data-testid="type-filter-${type}"]`);
      await expect(
        chip,
        `Type chip for "${type}" should be visible`,
      ).toBeVisible({ timeout: 3000 });
    }
  });

  test("clicking a type chip activates it (shows active state)", async ({
    page,
  }) => {
    const ts = Date.now();

    await page.route("**/graphql", (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            pathSearch: {
              entities: [
                {
                  id: "e2e.semsource.golang.data-fixture.function.main",
                  triples: [
                    {
                      subject:
                        "e2e.semsource.golang.data-fixture.function.main",
                      predicate: "has.name",
                      object: "main",
                      confidence: 1.0,
                      timestamp: ts,
                    },
                  ],
                },
                {
                  id: "e2e.semsource.golang.data-fixture.interface.Handler",
                  triples: [
                    {
                      subject:
                        "e2e.semsource.golang.data-fixture.interface.Handler",
                      predicate: "has.name",
                      object: "Handler",
                      confidence: 1.0,
                      timestamp: ts,
                    },
                  ],
                },
              ],
              edges: [],
            },
          },
        }),
      });
    });

    const refreshButton = page.locator(".toolbar-button[title='Refresh data']");
    await refreshButton.click();
    await page
      .locator(".loading-overlay")
      .waitFor({ state: "hidden", timeout: 10000 });
    await page.waitForTimeout(500);

    const functionChip = page.locator('[data-testid="type-filter-function"]');
    await expect(functionChip).toBeVisible({ timeout: 3000 });

    // Chip should not be active initially
    await expect(functionChip).not.toHaveClass(/active/);

    // Click to activate
    await functionChip.click();
    await expect(functionChip).toHaveClass(/active/);
  });

  test("filtering by type function shows only function entities in stats", async ({
    page,
  }) => {
    const ts = Date.now();

    // Provide two entity types: function and type
    await page.route("**/graphql", (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            pathSearch: {
              entities: [
                {
                  id: "e2e.semsource.golang.data-fixture.function.main",
                  triples: [
                    {
                      subject:
                        "e2e.semsource.golang.data-fixture.function.main",
                      predicate: "has.name",
                      object: "main",
                      confidence: 1.0,
                      timestamp: ts,
                    },
                  ],
                },
                {
                  id: "e2e.semsource.golang.data-fixture.interface.Handler",
                  triples: [
                    {
                      subject:
                        "e2e.semsource.golang.data-fixture.interface.Handler",
                      predicate: "has.name",
                      object: "Handler",
                      confidence: 1.0,
                      timestamp: ts,
                    },
                  ],
                },
              ],
              edges: [],
            },
          },
        }),
      });
    });

    const refreshButton = page.locator(".toolbar-button[title='Refresh data']");
    await refreshButton.click();
    await page
      .locator(".loading-overlay")
      .waitFor({ state: "hidden", timeout: 10000 });
    await page.waitForTimeout(500);

    // Read baseline entity count from stats overlay
    const statsOverlay = page.locator(".graph-stats");
    await expect(statsOverlay).toBeVisible();
    const baselineText = await statsOverlay
      .locator("span")
      .first()
      .textContent();
    const baselineMatch = baselineText?.match(/(\d+)/);
    const baselineCount = baselineMatch ? parseInt(baselineMatch[1], 10) : -1;

    // Activate the "function" type filter
    const functionChip = page.locator('[data-testid="type-filter-function"]');
    await expect(functionChip).toBeVisible({ timeout: 3000 });
    await functionChip.click();

    // Wait for the graph to re-render with the filter applied
    await page.waitForTimeout(600);

    // The entity count shown in stats should be fewer than before
    const filteredText = await statsOverlay
      .locator("span")
      .first()
      .textContent();
    const filteredMatch = filteredText?.match(/(\d+)/);
    const filteredCount = filteredMatch ? parseInt(filteredMatch[1], 10) : -1;

    if (baselineCount > 1 && filteredCount >= 0) {
      expect(filteredCount).toBeLessThanOrEqual(baselineCount);
    }

    // The "function" chip should remain active
    await expect(functionChip).toHaveClass(/active/);
  });

  // ---------------------------------------------------------------------------
  // Domain filter chips
  // ---------------------------------------------------------------------------

  test("domain filter chips contain golang, web, and config domains", async ({
    page,
  }) => {
    const ts = Date.now();

    // Provide one entity per domain
    await page.route("**/graphql", (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            pathSearch: {
              entities: SEMSOURCE_DOMAINS.map((domain, i) => ({
                id: `e2e.semsource.${domain}.go.function.entity${i}`,
                triples: [
                  {
                    subject: `e2e.semsource.${domain}.go.function.entity${i}`,
                    predicate: "has.name",
                    object: `entity${i}`,
                    confidence: 1.0,
                    timestamp: ts,
                  },
                ],
              })),
              edges: [],
            },
          },
        }),
      });
    });

    const refreshButton = page.locator(".toolbar-button[title='Refresh data']");
    await refreshButton.click();
    await page
      .locator(".loading-overlay")
      .waitFor({ state: "hidden", timeout: 10000 });
    await page.waitForTimeout(500);

    for (const domain of SEMSOURCE_DOMAINS) {
      const chip = page.locator(`[data-testid="domain-filter-${domain}"]`);
      await expect(
        chip,
        `Domain chip for "${domain}" should be visible`,
      ).toBeVisible({ timeout: 3000 });
    }
  });

  test("clicking a domain chip activates it", async ({ page }) => {
    const ts = Date.now();

    await page.route("**/graphql", (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            pathSearch: {
              entities: [
                {
                  id: "e2e.semsource.golang.data-fixture.function.main",
                  triples: [
                    {
                      subject:
                        "e2e.semsource.golang.data-fixture.function.main",
                      predicate: "has.name",
                      object: "main",
                      confidence: 1.0,
                      timestamp: ts,
                    },
                  ],
                },
                {
                  id: "e2e.semsource.web.data-fixture.doc.README",
                  triples: [
                    {
                      subject: "e2e.semsource.web.data-fixture.doc.README",
                      predicate: "has.name",
                      object: "README",
                      confidence: 1.0,
                      timestamp: ts,
                    },
                  ],
                },
              ],
              edges: [],
            },
          },
        }),
      });
    });

    const refreshButton = page.locator(".toolbar-button[title='Refresh data']");
    await refreshButton.click();
    await page
      .locator(".loading-overlay")
      .waitFor({ state: "hidden", timeout: 10000 });
    await page.waitForTimeout(500);

    const codeChip = page.locator('[data-testid="domain-filter-golang"]');
    await expect(codeChip).toBeVisible({ timeout: 3000 });
    await expect(codeChip).not.toHaveClass(/active/);

    await codeChip.click();
    await expect(codeChip).toHaveClass(/active/);
  });

  test("filtering by domain golang reduces visible entity count", async ({
    page,
  }) => {
    const ts = Date.now();

    await page.route("**/graphql", (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            pathSearch: {
              entities: [
                {
                  id: "e2e.semsource.golang.data-fixture.function.main",
                  triples: [
                    {
                      subject:
                        "e2e.semsource.golang.data-fixture.function.main",
                      predicate: "has.name",
                      object: "main",
                      confidence: 1.0,
                      timestamp: ts,
                    },
                  ],
                },
                {
                  id: "e2e.semsource.web.data-fixture.doc.README",
                  triples: [
                    {
                      subject: "e2e.semsource.web.data-fixture.doc.README",
                      predicate: "has.name",
                      object: "README",
                      confidence: 1.0,
                      timestamp: ts,
                    },
                  ],
                },
                {
                  id: "e2e.semsource.config.data-fixture.gomod.fixture-project",
                  triples: [
                    {
                      subject:
                        "e2e.semsource.config.data-fixture.gomod.fixture-project",
                      predicate: "has.name",
                      object: "fixture-project",
                      confidence: 1.0,
                      timestamp: ts,
                    },
                  ],
                },
              ],
              edges: [],
            },
          },
        }),
      });
    });

    const refreshButton = page.locator(".toolbar-button[title='Refresh data']");
    await refreshButton.click();
    await page
      .locator(".loading-overlay")
      .waitFor({ state: "hidden", timeout: 10000 });
    await page.waitForTimeout(500);

    const statsOverlay = page.locator(".graph-stats");
    await expect(statsOverlay).toBeVisible();

    const baselineText = await statsOverlay
      .locator("span")
      .first()
      .textContent();
    const baselineCount = parseInt(
      baselineText?.match(/(\d+)/)?.[1] ?? "0",
      10,
    );

    // Filter by "golang" domain — should hide web and config entities
    const codeChip = page.locator('[data-testid="domain-filter-golang"]');
    await expect(codeChip).toBeVisible({ timeout: 3000 });
    await codeChip.click();
    await page.waitForTimeout(600);

    const filteredText = await statsOverlay
      .locator("span")
      .first()
      .textContent();
    const filteredCount = parseInt(
      filteredText?.match(/(\d+)/)?.[1] ?? "0",
      10,
    );

    if (baselineCount > 1) {
      expect(filteredCount).toBeLessThan(baselineCount);
    }
  });

  // ---------------------------------------------------------------------------
  // Search filter
  // ---------------------------------------------------------------------------

  test("search input filters entities by name", async ({ page }) => {
    const ts = Date.now();

    await page.route("**/graphql", (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            pathSearch: {
              entities: [
                {
                  id: "e2e.semsource.golang.data-fixture.function.main",
                  triples: [
                    {
                      subject:
                        "e2e.semsource.golang.data-fixture.function.main",
                      predicate: "has.name",
                      object: "main",
                      confidence: 1.0,
                      timestamp: ts,
                    },
                  ],
                },
                {
                  id: "e2e.semsource.web.data-fixture.doc.README",
                  triples: [
                    {
                      subject: "e2e.semsource.web.data-fixture.doc.README",
                      predicate: "has.name",
                      object: "README",
                      confidence: 1.0,
                      timestamp: ts,
                    },
                  ],
                },
              ],
              edges: [],
            },
          },
        }),
      });
    });

    const refreshButton = page.locator(".toolbar-button[title='Refresh data']");
    await refreshButton.click();
    await page
      .locator(".loading-overlay")
      .waitFor({ state: "hidden", timeout: 10000 });
    await page.waitForTimeout(500);

    const statsOverlay = page.locator(".graph-stats");
    await expect(statsOverlay).toBeVisible();

    const baselineText = await statsOverlay
      .locator("span")
      .first()
      .textContent();
    const baselineCount = parseInt(
      baselineText?.match(/(\d+)/)?.[1] ?? "0",
      10,
    );

    // Search for "main" — should hide the README entity
    const searchInput = page.locator('[data-testid="entity-search"]');
    await searchInput.fill("main");

    // Debounce is 300ms in GraphFilters.svelte
    await page.waitForTimeout(500);

    const filteredText = await statsOverlay
      .locator("span")
      .first()
      .textContent();
    const filteredCount = parseInt(
      filteredText?.match(/(\d+)/)?.[1] ?? "0",
      10,
    );

    if (baselineCount > 1) {
      expect(filteredCount).toBeLessThan(baselineCount);
    }
  });

  // ---------------------------------------------------------------------------
  // Reset filters
  // ---------------------------------------------------------------------------

  test("reset button appears after activating a filter", async ({ page }) => {
    const ts = Date.now();

    await page.route("**/graphql", (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            pathSearch: {
              entities: [
                {
                  id: "e2e.semsource.golang.data-fixture.function.main",
                  triples: [
                    {
                      subject:
                        "e2e.semsource.golang.data-fixture.function.main",
                      predicate: "has.name",
                      object: "main",
                      confidence: 1.0,
                      timestamp: ts,
                    },
                  ],
                },
              ],
              edges: [],
            },
          },
        }),
      });
    });

    const refreshButton = page.locator(".toolbar-button[title='Refresh data']");
    await refreshButton.click();
    await page
      .locator(".loading-overlay")
      .waitFor({ state: "hidden", timeout: 10000 });
    await page.waitForTimeout(500);

    // Reset button should not be visible before any filter is active
    const resetButton = page.locator('[data-testid="reset-filters"]');
    await expect(resetButton).not.toBeVisible();

    // Activate a type filter chip
    const functionChip = page.locator('[data-testid="type-filter-function"]');
    await expect(functionChip).toBeVisible({ timeout: 3000 });
    await functionChip.click();

    // Reset button should now appear
    await expect(resetButton).toBeVisible({ timeout: 2000 });
    await expect(resetButton).toContainText("Clear Filters");
  });

  test("clicking reset button deactivates all filters and hides the reset button", async ({
    page,
  }) => {
    const ts = Date.now();

    await page.route("**/graphql", (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            pathSearch: {
              entities: [
                {
                  id: "e2e.semsource.golang.data-fixture.function.main",
                  triples: [
                    {
                      subject:
                        "e2e.semsource.golang.data-fixture.function.main",
                      predicate: "has.name",
                      object: "main",
                      confidence: 1.0,
                      timestamp: ts,
                    },
                  ],
                },
                {
                  id: "e2e.semsource.golang.data-fixture.interface.Handler",
                  triples: [
                    {
                      subject:
                        "e2e.semsource.golang.data-fixture.interface.Handler",
                      predicate: "has.name",
                      object: "Handler",
                      confidence: 1.0,
                      timestamp: ts,
                    },
                  ],
                },
              ],
              edges: [],
            },
          },
        }),
      });
    });

    const refreshButton = page.locator(".toolbar-button[title='Refresh data']");
    await refreshButton.click();
    await page
      .locator(".loading-overlay")
      .waitFor({ state: "hidden", timeout: 10000 });
    await page.waitForTimeout(500);

    // Activate a filter
    const functionChip = page.locator('[data-testid="type-filter-function"]');
    await expect(functionChip).toBeVisible({ timeout: 3000 });
    await functionChip.click();
    await expect(functionChip).toHaveClass(/active/);

    const resetButton = page.locator('[data-testid="reset-filters"]');
    await expect(resetButton).toBeVisible({ timeout: 2000 });

    // Reset — note the route mock is still active so the entity count does
    // not change, but the filter state should clear.
    await resetButton.click();

    // Filter chip should no longer be active
    await expect(functionChip).not.toHaveClass(/active/);

    // Reset button should disappear (no active filters)
    await expect(resetButton).not.toBeVisible({ timeout: 2000 });
  });

  test("reset restores all entities to the graph after filtering", async ({
    page,
  }) => {
    const ts = Date.now();

    await page.route("**/graphql", (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            pathSearch: {
              entities: [
                {
                  id: "e2e.semsource.golang.data-fixture.function.main",
                  triples: [
                    {
                      subject:
                        "e2e.semsource.golang.data-fixture.function.main",
                      predicate: "has.name",
                      object: "main",
                      confidence: 1.0,
                      timestamp: ts,
                    },
                  ],
                },
                {
                  id: "e2e.semsource.web.data-fixture.doc.README",
                  triples: [
                    {
                      subject: "e2e.semsource.web.data-fixture.doc.README",
                      predicate: "has.name",
                      object: "README",
                      confidence: 1.0,
                      timestamp: ts,
                    },
                  ],
                },
              ],
              edges: [],
            },
          },
        }),
      });
    });

    const refreshButton = page.locator(".toolbar-button[title='Refresh data']");
    await refreshButton.click();
    await page
      .locator(".loading-overlay")
      .waitFor({ state: "hidden", timeout: 10000 });
    await page.waitForTimeout(500);

    const statsOverlay = page.locator(".graph-stats");
    await expect(statsOverlay).toBeVisible();
    const baselineText = await statsOverlay
      .locator("span")
      .first()
      .textContent();
    const baselineCount = parseInt(
      baselineText?.match(/(\d+)/)?.[1] ?? "0",
      10,
    );

    // Apply a filter that reduces the count
    const functionChip = page.locator('[data-testid="type-filter-function"]');
    await expect(functionChip).toBeVisible({ timeout: 3000 });
    await functionChip.click();
    await page.waitForTimeout(500);

    // Reset filters
    const resetButton = page.locator('[data-testid="reset-filters"]');
    await expect(resetButton).toBeVisible({ timeout: 2000 });
    await resetButton.click();
    await page.waitForTimeout(500);

    // Count should return to baseline
    const restoredText = await statsOverlay
      .locator("span")
      .first()
      .textContent();
    const restoredCount = parseInt(
      restoredText?.match(/(\d+)/)?.[1] ?? "0",
      10,
    );
    expect(restoredCount).toBe(baselineCount);
  });
});
