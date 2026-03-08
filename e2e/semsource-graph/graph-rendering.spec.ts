/**
 * Graph Rendering - SemSource Integration E2E Tests
 *
 * Validates that semsource entities appear in the DataView after ingestion
 * through the full pipeline:
 *
 *   semsource → WebSocket → semstreams backend → ENTITY_STATES KV → GraphQL → UI
 *
 * Prerequisites:
 *   - Docker Compose profile "semsource" must be active
 *   - Backend must be configured with e2e-with-semsource.json
 *   - Run via: task test:e2e:semsource-graph
 *     (or: COMPOSE_PROFILES=semsource npx playwright test e2e/semsource-graph/)
 *
 * All tests use polling/waiting. Never assume entities exist immediately after
 * the backend starts — semsource ingestion is asynchronous.
 */

import { test, expect } from "@playwright/test";
import {
  KNOWN_ENTITIES,
  SEMSOURCE_ENTITY_PREFIX,
  SEMSOURCE_ENTITY_TYPES,
  setupDataViewWithSemsource,
  deleteTestFlow,
} from "./helpers/semsource-helpers";

test.describe("Graph Rendering - SemSource Entities", () => {
  let flowId: string;

  test.beforeEach(async ({ page }) => {
    // setupDataViewWithSemsource polls GraphQL until entities appear, then
    // creates a running flow and navigates to the Data view.
    flowId = await setupDataViewWithSemsource(page, 3);
  });

  test.afterEach(async ({ page }) => {
    if (flowId) {
      await deleteTestFlow(page, flowId);
    }
  });

  test("semsource entities load from GraphQL after ingestion", async ({
    page,
  }) => {
    // DataView is already open (setupDataViewWithSemsource navigated there).
    // The graph-stats overlay in SigmaCanvas shows the live entity count.
    const statsOverlay = page.locator(".graph-stats");
    await expect(statsOverlay).toBeVisible({ timeout: 10000 });

    // At least some entities must be rendered — exact count depends on timing
    // but we know at least 3 semsource entities reached the backend.
    const entityCountText = await statsOverlay
      .locator("span")
      .first()
      .textContent();
    const match = entityCountText?.match(/(\d+)\s+entities/);
    expect(match).not.toBeNull();
    const count = parseInt(match![1], 10);
    expect(count).toBeGreaterThan(0);
  });

  test("entity count is in expected range for the fixture", async ({
    page,
  }) => {
    // The fixture produces 8-15 entities (AST + docs + config handlers).
    // We verify this via GraphQL directly so we are not dependent on WebGL
    // rendering being complete.
    const response = await page.request.post("/graphql", {
      data: {
        query: `query { pathSearch(startEntity: "*", maxDepth: 2, maxNodes: 50) { entities { id } } }`,
        variables: {},
      },
    });

    expect(response.ok()).toBe(true);
    const body = await response.json();
    const allEntities: Array<{ id: string }> =
      body?.data?.pathSearch?.entities ?? [];
    const semsourceEntities = allEntities.filter((e) =>
      e.id.startsWith(SEMSOURCE_ENTITY_PREFIX),
    );

    expect(semsourceEntities.length).toBeGreaterThanOrEqual(3);
    // The fixture is small — guard against runaway ingestion too
    expect(semsourceEntities.length).toBeLessThanOrEqual(50);
  });

  test("entity types include function, type, document, and module", async ({
    page,
  }) => {
    const response = await page.request.post("/graphql", {
      data: {
        query: `query {
          pathSearch(startEntity: "*", maxDepth: 2, maxNodes: 50) {
            entities {
              id
              triples { predicate object }
            }
          }
        }`,
        variables: {},
      },
    });

    expect(response.ok()).toBe(true);
    const body = await response.json();
    const entities: Array<{ id: string }> =
      body?.data?.pathSearch?.entities ?? [];

    const semsourceIds = entities
      .map((e) => e.id)
      .filter((id) => id.startsWith(SEMSOURCE_ENTITY_PREFIX));

    // The 5th segment of the 6-part ID is the entity type.
    // e.g. "e2e.semsource.code.go.function.main" → type = "function"
    const observedTypes = new Set(
      semsourceIds.map((id) => id.split(".")[4]).filter(Boolean),
    );

    for (const expectedType of SEMSOURCE_ENTITY_TYPES) {
      expect(
        observedTypes.has(expectedType),
        `Expected entity type "${expectedType}" in observed types: ${[...observedTypes].join(", ")}`,
      ).toBe(true);
    }
  });

  test("known entity IDs from the fixture are present in GraphQL results", async ({
    page,
  }) => {
    const response = await page.request.post("/graphql", {
      data: {
        query: `query { pathSearch(startEntity: "*", maxDepth: 2, maxNodes: 50) { entities { id } } }`,
        variables: {},
      },
    });

    expect(response.ok()).toBe(true);
    const body = await response.json();
    const entityIds = new Set<string>(
      (body?.data?.pathSearch?.entities ?? []).map((e: { id: string }) => e.id),
    );

    // The most important known entities — assert a reasonable subset
    expect(
      entityIds.has(KNOWN_ENTITIES.mainFunc),
      `Expected "${KNOWN_ENTITIES.mainFunc}" in graph`,
    ).toBe(true);
    expect(
      entityIds.has(KNOWN_ENTITIES.handlerType),
      `Expected "${KNOWN_ENTITIES.handlerType}" in graph`,
    ).toBe(true);
    expect(
      entityIds.has(KNOWN_ENTITIES.readme),
      `Expected "${KNOWN_ENTITIES.readme}" in graph`,
    ).toBe(true);
  });

  test("relationship triples are present between semsource entities", async ({
    page,
  }) => {
    const response = await page.request.post("/graphql", {
      data: {
        query: `query {
          pathSearch(startEntity: "*", maxDepth: 2, maxNodes: 50) {
            entities {
              id
              triples { subject predicate object }
            }
          }
        }`,
        variables: {},
      },
    });

    expect(response.ok()).toBe(true);
    const body = await response.json();
    const entities: Array<{
      id: string;
      triples: Array<{ subject: string; predicate: string; object: string }>;
    }> = body?.data?.pathSearch?.entities ?? [];

    const semsourceEntities = entities.filter((e) =>
      e.id.startsWith(SEMSOURCE_ENTITY_PREFIX),
    );

    const allTriples = semsourceEntities.flatMap((e) => e.triples ?? []);
    expect(
      allTriples.length,
      "Expected semsource entities to have relationship triples",
    ).toBeGreaterThan(0);
  });

  test("SigmaCanvas renders and is visible in the Data view", async ({
    page,
  }) => {
    // Verify the Sigma canvas container is rendered and visible
    const sigmaCanvas = page.locator('[data-testid="sigma-canvas"]');
    await expect(sigmaCanvas).toBeVisible({ timeout: 10000 });

    // The inner sigma-container div is where Sigma.js mounts its WebGL canvas
    const sigmaContainer = sigmaCanvas.locator(".sigma-container");
    await expect(sigmaContainer).toBeVisible();

    // Sigma appends a <canvas> element inside the container
    const canvas = sigmaContainer.locator("canvas");
    await expect(canvas).toBeVisible({ timeout: 5000 });
  });

  test("graph stats overlay shows entity and relationship counts", async ({
    page,
  }) => {
    const statsOverlay = page.locator(".graph-stats");
    await expect(statsOverlay).toBeVisible({ timeout: 10000 });

    const spans = statsOverlay.locator("span");
    await expect(spans).toHaveCount(2);

    // First span: "{N} entities"
    await expect(spans.first()).toContainText("entities");
    // Second span: "{N} relationships"
    await expect(spans.last()).toContainText("relationships");
  });

  test("GraphFilters panel is visible alongside the canvas", async ({
    page,
  }) => {
    await expect(page.locator('[data-testid="graph-filters"]')).toBeVisible();
  });

  test("GraphDetailPanel shows empty state before any node is selected", async ({
    page,
  }) => {
    const emptyPanel = page.locator('[data-testid="graph-detail-panel-empty"]');
    await expect(emptyPanel).toBeVisible();
    await expect(emptyPanel).toContainText("Select an entity to view details");
  });
});
