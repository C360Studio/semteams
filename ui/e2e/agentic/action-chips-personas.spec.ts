import { test, expect } from "@playwright/test";

/**
 * Journey: Persona action chips
 *
 * Goal: The ChatBar exposes Research / Plan / Implement chips that
 * pre-fill the input with an `@persona` prefix. The chips are only
 * visible on the empty-state (no task selected, no `/` typed) and
 * preserve any text the user has already typed when switching personas.
 *
 * Validates:
 *   - Chips render when no task is selected.
 *   - Click → input value = "@research " (or @plan/@implement).
 *   - Click a different persona while text is present → the prefix
 *     swaps, the user's text survives.
 *   - Selecting a task hides the chip row (the chips' affordance
 *     migrates to the right-rail action buttons).
 *
 * The "Approve next" chip is only rendered when at least one task is
 * waiting on the user (taskStore.needsAttentionCount > 0). That path
 * needs an approval-gated tool call, which currently depends on the
 * still-broken e2e-agentic.json (see project memory). We cover the
 * three persona chips here; approve-next gets its own spec once
 * tool-approval-gate boots end-to-end again.
 *
 * Required config: any working backend that boots the page; deep-research
 * is fine since this spec doesn't actually send a message.
 *
 * Run via:
 *   task test:e2e:agentic:action-chips
 */

test.describe("Action chips — persona prefixes", () => {
  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok(), "Backend not healthy — stack not running?").toBe(true);
  });

  test("clicking Research / Plan / Implement pre-fills input with @prefix", async ({
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
    await expect(page.getByTestId("action-chip-plan")).toBeVisible();
    await expect(page.getByTestId("action-chip-implement")).toBeVisible();

    const input = page.getByTestId("chat-input");

    // Empty input + Research click → "@research " prefix, focus is on
    // the input so the user can keep typing.
    await page.getByTestId("action-chip-research").click();
    await expect(input).toHaveValue("@research ");
    await expect(input).toBeFocused();

    // Type a query, then swap to Plan — the prefix should be replaced
    // (not duplicated) and the query should survive.
    await input.pressSequentially("survey design alternatives");
    await expect(input).toHaveValue("@research survey design alternatives");
    await page.getByTestId("action-chip-plan").click();
    await expect(input).toHaveValue("@plan survey design alternatives");

    // And again to Implement.
    await page.getByTestId("action-chip-implement").click();
    await expect(input).toHaveValue("@implement survey design alternatives");

    // -----------------------------------------------------------------
    // Selecting any task hides the chip row — the per-task affordances
    // live in the right-rail drilldown instead. Without an existing
    // task to click we can't drive this directly, so simulate by
    // typing a slash command (which also hides chips per the
    // showingSlash $derived).
    // -----------------------------------------------------------------
    await input.fill("/approve");
    await expect(chipRow).toBeHidden();

    // Clearing the slash → chips return.
    await input.fill("");
    await expect(chipRow).toBeVisible();
  });
});
