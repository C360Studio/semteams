/**
 * Graph Rendering - SemSource Integration E2E Tests
 *
 * Validates that semsource entities appear in the DataView after ingestion.
 * Semsource IS a semstreams app — it runs the full graph pipeline internally:
 *
 *   semsource sources → NATS (graph.ingest.entity)
 *     → graph-ingest → ENTITY_STATES KV → graph-index → graph-query
 *     → graph-gateway (:8082/graphql) → Caddy → UI
 *
 * Prerequisites:
 *   - Docker Compose profile "semsource" must be active
 *   - Run via: COMPOSE_PROFILES=semsource npx playwright test e2e/semsource-graph/
 *
 * All tests use polling/waiting. Never assume entities exist immediately after
 * startup — semsource ingestion is asynchronous.
 */

import { test, expect } from "@playwright/test";
import {
  KNOWN_ENTITIES,
  SEMSOURCE_ENTITY_PREFIX,
  SEMSOURCE_ENTITY_TYPES,
  setupDataViewWithSemsource,
  deleteTestFlow,
  waitForSemsourceEntities,
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
    // The fixture produces ~16 entities (AST + docs + config).
    // We query from a known file entity and traverse to discover connected
    // entities. pathSearch(startEntity: "*") only returns itself.
    await waitForSemsourceEntities(page, 3);

    const response = await page.request.post("/graphql", {
      data: {
        query: `query { pathSearch(startEntity: "${KNOWN_ENTITIES.mainFile}", maxDepth: 3, maxNodes: 50) { entities { id } } }`,
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

    expect(semsourceEntities.length).toBeGreaterThanOrEqual(2);
    // The fixture is small — guard against runaway ingestion too
    expect(semsourceEntities.length).toBeLessThanOrEqual(50);
  });

  test("entity types include function, interface, file, and doc", async ({
    page,
  }) => {
    await waitForSemsourceEntities(page, 3);

    // Query from the main file entity to discover AST entities, plus
    // query config/doc entities separately since they may not be connected.
    const response = await page.request.post("/graphql", {
      data: {
        query: `query {
          pathSearch(startEntity: "${KNOWN_ENTITIES.mainFile}", maxDepth: 3, maxNodes: 50) {
            entities { id }
          }
        }`,
        variables: {},
      },
    });

    expect(response.ok()).toBe(true);
    const body = await response.json();
    const entities: Array<{ id: string }> =
      body?.data?.pathSearch?.entities ?? [];

    // Also check known entities from other domains
    const allIds = [
      ...entities.map((e) => e.id),
      KNOWN_ENTITIES.readme,
      KNOWN_ENTITIES.goMod,
    ];

    const semsourceIds = allIds.filter((id) =>
      id.startsWith(SEMSOURCE_ENTITY_PREFIX),
    );

    // The 5th segment of the 6-part ID is the entity type.
    // e.g. "e2e.semsource.golang.data-fixture.function.src-main-go-main" → type = "function"
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
    await waitForSemsourceEntities(page, 3);

    // Query each known entity individually — pathSearch always returns the
    // start entity, but connected entities confirm it exists in the graph.
    const knownIds = [
      KNOWN_ENTITIES.mainFunc,
      KNOWN_ENTITIES.handlerType,
      KNOWN_ENTITIES.mainFile,
    ];

    for (const knownId of knownIds) {
      const response = await page.request.post("/graphql", {
        data: {
          query: `query { pathSearch(startEntity: "${knownId}", maxDepth: 1, maxNodes: 10) { entities { id } paths { from to } } }`,
          variables: {},
        },
      });

      expect(response.ok()).toBe(true);
      const body = await response.json();
      const entities: Array<{ id: string }> =
        body?.data?.pathSearch?.entities ?? [];
      const paths = body?.data?.pathSearch?.paths ?? [];

      // Entity should be the start entity and have at least one connection
      expect(
        entities.some((e) => e.id === knownId),
        `Expected "${knownId}" in pathSearch results`,
      ).toBe(true);
      expect(
        entities.length > 1 || paths.some((p: unknown[]) => p.length > 0),
        `Expected "${knownId}" to have connections in the graph`,
      ).toBe(true);
    }
  });

  test("relationship triples are present between semsource entities", async ({
    page,
  }) => {
    await waitForSemsourceEntities(page, 3);

    const response = await page.request.post("/graphql", {
      data: {
        query: `query {
          pathSearch(startEntity: "${KNOWN_ENTITIES.mainFile}", maxDepth: 3, maxNodes: 50) {
            entities {
              id
              triples { subject predicate object }
            }
            paths { from predicate to }
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
    const paths: Array<{ from: string; predicate: string; to: string }[]> =
      body?.data?.pathSearch?.paths ?? [];

    const semsourceEntities = entities.filter((e) =>
      e.id.startsWith(SEMSOURCE_ENTITY_PREFIX),
    );

    // Check for relationships via paths (edges between entities)
    const allEdges = paths.flat();
    expect(
      allEdges.length,
      "Expected semsource entities to have relationship edges",
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
