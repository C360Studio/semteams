import { test, expect } from "@playwright/test";

/**
 * Journey: Team action chips
 *
 * Goal: The ChatBar exposes the public team chips that pre-fill the
 * input with a coordinator-routed slash-command hint. The chips are
 * only visible on the empty-state (no task selected, no `/` typed)
 * and preserve any text the user has already typed when switching
 * teams.
 *
 * The live chip set is Research + Optimize. The Spec and Build chips
 * are parked with their category packs (ADR-058) and must NOT render
 * until the packs are re-authored and re-wired.
 *
 * Validates:
 *   - Chips render when no task is selected; parked chips do not.
 *   - Click → input value = "/research " (or "/optimize ").
 *   - Click a different team while text is present → the prefix
 *     swaps, the user's text survives.
 *   - Typing a slash command hides the chip row; clearing restores it.
 *
 * Required config: any working backend that boots the page; this spec
 * doesn't actually send a message.
 *
 * Run via:
 *   task test:e2e:agentic:action-chips
 */

test.describe("Action chips — team hints", () => {
  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok(), "Backend not healthy — stack not running?").toBe(true);
  });

  test("clicking Research / Optimize pre-fills input with the team hint", async ({
    page,
  }) => {
    await page.goto("/");

    await expect(page.getByTestId("connection-status")).toHaveAttribute(
      "data-summary",
      "healthy",
      { timeout: 15000 },
    );

    // Chip row is visible on the empty-state homepage.
    const chipRow = page.getByTestId("action-chips");
    await expect(chipRow).toBeVisible();
    await expect(page.getByTestId("action-chip-research")).toBeVisible();
    await expect(page.getByTestId("action-chip-optimize")).toBeVisible();
    // Parked with their packs (ADR-058).
    await expect(page.getByTestId("action-chip-spec")).toHaveCount(0);
    await expect(page.getByTestId("action-chip-build")).toHaveCount(0);

    const input = page.getByTestId("chat-input");

    // Empty input + Research click → "/research " prefix, focus is on
    // the input so the user can keep typing.
    await page.getByTestId("action-chip-research").click();
    await expect(input).toHaveValue("/research ");
    await expect(input).toBeFocused();

    // Type a query, then swap to Optimize — the prefix should be
    // replaced (not duplicated) and the query should survive.
    await input.pressSequentially("survey design alternatives");
    await expect(input).toHaveValue("/research survey design alternatives");
    await page.getByTestId("action-chip-optimize").click();
    await expect(input).toHaveValue("/optimize survey design alternatives");

    // -----------------------------------------------------------------
    // Typing a slash command hides the chip row (showingSlash), the
    // same visibility rule that applies when a task is selected.
    // -----------------------------------------------------------------
    await input.fill("/approve");
    await expect(chipRow).toBeHidden();

    // Clearing the slash → chips return.
    await input.fill("");
    await expect(chipRow).toBeVisible();
  });
});
