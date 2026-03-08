import type { Page } from "@playwright/test";
import {
  createRunningFlow,
  deleteTestFlow,
} from "../../helpers/runtime-helpers";

/**
 * Known entity IDs from the E2E fixture.
 *
 * These are deterministic because the fixture files are fixed and the entity
 * IDs derive from file paths, symbol names, and the "e2e" namespace configured
 * in semsource-e2e.json.
 *
 * See: e2e/fixtures/semsource/
 */
export const KNOWN_ENTITIES = {
  mainFunc: "e2e.semsource.code.go.function.main",
  handlerType: "e2e.semsource.code.go.type.Handler",
  readme: "e2e.semsource.docs.markdown.document.README",
  goMod: "e2e.semsource.config.go.module.fixture-project",
} as const;

export type KnownEntityKey = keyof typeof KNOWN_ENTITIES;

/**
 * Entity ID prefix for all semsource E2E fixture entities.
 * Used to filter GraphQL results to only those from semsource.
 */
export const SEMSOURCE_ENTITY_PREFIX = "e2e.semsource.";

/**
 * Expected entity type values produced by the semsource fixture.
 */
export const SEMSOURCE_ENTITY_TYPES = [
  "function",
  "type",
  "document",
  "module",
] as const;

/**
 * Expected domain values produced by the semsource fixture.
 */
export const SEMSOURCE_DOMAINS = ["code", "docs", "config"] as const;

/**
 * Wait for semsource entities to appear in the GraphQL backend.
 *
 * Polls the /graphql endpoint until at least `minEntities` entities with the
 * "e2e.semsource." prefix are returned, or until `timeout` ms elapses.
 *
 * Call this before asserting anything about graph content. Semsource emits
 * events asynchronously, and the backend processes them asynchronously; this
 * helper provides the necessary synchronisation point.
 *
 * @param page - Playwright Page object (used for page.request)
 * @param minEntities - Minimum number of semsource entities required (default: 3)
 * @param timeout - Maximum wait in milliseconds (default: 30000)
 */
export async function waitForSemsourceEntities(
  page: Page,
  minEntities: number = 3,
  timeout: number = 30000,
): Promise<void> {
  const startTime = Date.now();

  while (Date.now() - startTime < timeout) {
    try {
      const response = await page.request.post("/graphql", {
        data: {
          query: `query { pathSearch(startEntity: "*", maxDepth: 2, maxNodes: 50) { entities { id } } }`,
          variables: {},
        },
      });

      if (response.ok()) {
        const body = await response.json();
        const entities: Array<{ id: string }> =
          body?.data?.pathSearch?.entities ?? [];
        const semsourceEntities = entities.filter((e) =>
          e.id.startsWith(SEMSOURCE_ENTITY_PREFIX),
        );

        if (semsourceEntities.length >= minEntities) {
          return;
        }
      }
    } catch {
      // Network hiccup — keep polling
    }

    await page.waitForTimeout(1000);
  }

  throw new Error(
    `Timed out after ${timeout}ms waiting for ${minEntities} semsource entities (prefix: "${SEMSOURCE_ENTITY_PREFIX}")`,
  );
}

/**
 * Navigate to the Data view tab for a running flow.
 *
 * Waits for the ViewSwitcher to appear (only visible when a flow is running),
 * then clicks the Data button and waits for the DataView to be visible.
 *
 * @param page - Playwright Page object
 */
export async function navigateToDataView(page: Page): Promise<void> {
  await page
    .locator('[data-testid="view-switcher"]')
    .waitFor({ state: "visible", timeout: 10000 });
  await page.click('[data-testid="view-switch-data"]');
  await page
    .locator('[data-testid="data-view"]')
    .waitFor({ state: "visible", timeout: 5000 });
}

/**
 * Set up a running flow and navigate to its Data view, waiting for semsource
 * entities to be present in the GraphQL backend.
 *
 * Returns the flowId so the caller can clean it up in afterEach.
 *
 * @param page - Playwright Page object
 * @param minEntities - Minimum semsource entities to wait for (default: 3)
 * @returns flowId string
 */
export async function setupDataViewWithSemsource(
  page: Page,
  minEntities: number = 3,
): Promise<string> {
  // Wait for backend to have semsource data before creating the flow/navigating,
  // so the DataView loads with entities already present.
  await waitForSemsourceEntities(page, minEntities);

  const setup = await createRunningFlow(page);
  await page.goto(setup.url);
  await page.locator("#flow-canvas").waitFor({ state: "visible" });
  await page.waitForLoadState("networkidle");
  await navigateToDataView(page);

  return setup.flowId;
}

/**
 * Re-export deleteTestFlow for convenience — callers only need to import from
 * this module.
 */
export { deleteTestFlow };
