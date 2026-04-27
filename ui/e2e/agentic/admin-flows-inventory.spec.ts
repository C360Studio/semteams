import { test, expect } from "@playwright/test";

/**
 * Journey: Admin flows inventory is read-only
 *
 * Goal: After bffa800 (drop the flow editor) the `/admin/flows` page
 * is a read-only inventory of deployed flows. Coordinator-authored
 * changes are the path forward; humans approve via the existing
 * approval gate, not by editing JSON.
 *
 * Validates:
 *   - The admin landing exposes a "Flows" card pointing at /admin/flows.
 *   - Clicking the card lands on /admin/flows.
 *   - The flows page renders with subtitle copy that names the
 *     read-only contract ("Coordinator authors changes…").
 *   - No Create / Edit / Delete affordances are present (regression
 *     guard against re-introducing the editor).
 *
 * Required config: any working backend (deep-research is fine).
 *
 * Run via:
 *   task test:e2e:agentic:admin-flows-inventory
 */

test.describe("Admin flows inventory", () => {
  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok(), "Backend not healthy — stack not running?").toBe(true);
  });

  test("/admin → Flows card → read-only inventory at /admin/flows", async ({
    page,
  }) => {
    await page.goto("/admin");

    await expect(page.getByTestId("admin-page")).toBeVisible();

    const flowsCard = page.getByTestId("admin-card-flows");
    await expect(flowsCard).toBeVisible();
    await expect(flowsCard).toContainText("Flows");

    await flowsCard.click();
    await expect(page).toHaveURL(/\/admin\/flows$/);

    const flowsPage = page.getByTestId("flows-page");
    await expect(flowsPage).toBeVisible();

    // Read-only contract is named in the page subtitle. The exact
    // wording is a stable user-facing claim — if it changes, the
    // editor regression guard below loses meaning.
    await expect(flowsPage).toContainText(
      "Read-only inventory",
    );
    await expect(flowsPage).toContainText(
      "Flows are managed by the coordinator",
    );

    // Either flow rows render, the empty-state message does, OR an
    // error banner appears (e.g. when this stack doesn't include the
    // flow-builder component — common on lean per-scenario configs).
    // None of those paths should expose an editor.
    const flowList = page.getByTestId("flow-list");
    const empty = flowsPage.locator(".empty-state");
    const errorBanner = page.getByTestId("error-banner");
    const haveAny =
      (await flowList.count()) > 0 ||
      (await empty.count()) > 0 ||
      (await errorBanner.count()) > 0;
    expect(
      haveAny,
      "expected flow-list, empty-state, or error-banner — got none",
    ).toBe(true);

    // Regression guard — these affordances were removed in bffa800.
    // None of them must reappear without an explicit product-shape
    // decision.
    await expect(
      page.getByRole("button", { name: /create.*flow/i }),
    ).toHaveCount(0);
    await expect(
      page.getByRole("button", { name: /^new flow$/i }),
    ).toHaveCount(0);
    await expect(
      page.getByRole("link", { name: /create.*flow/i }),
    ).toHaveCount(0);
    await expect(
      page.getByRole("button", { name: /^delete$/i }),
    ).toHaveCount(0);
    await expect(
      page.getByRole("button", { name: /^edit$/i }),
    ).toHaveCount(0);
  });
});
