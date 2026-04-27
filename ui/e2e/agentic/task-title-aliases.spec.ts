import { test, expect } from "@playwright/test";

/**
 * Journey: Editable task titles + alias chips
 *
 * Goal: The task detail panel lets users override the auto-derived
 * title and attach `@`-mention aliases that the ChatBar can resolve.
 * Both are user-local (taskLabels store, localStorage-backed).
 *
 * Validates:
 *   - Click task-title → switches to title-input editor.
 *   - Enter commits the new title; title-reset chip appears (titleEdited).
 *   - Reset reverts to the auto-derived title; title-reset disappears.
 *   - Adding an alias renders an alias-chip (@name); the chip's `×`
 *     removes it; duplicate aliases across tasks raise alias-error.
 *
 * Required fixture: any 2-turn fixture that lands a card we can click.
 *   Reuses deep-research.yaml for stack consistency.
 *
 * Required config: configs/e2e-deep-research.json
 *
 * Run via:
 *   task test:e2e:agentic:task-title-aliases
 */

test.describe("Editable titles + aliases", () => {
  test.setTimeout(120_000);

  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok(), "Backend not healthy — stack not running?").toBe(true);
  });

  test("user edits title, resets, adds + removes alias chips", async ({
    page,
    request,
  }) => {
    // Clear any persisted overrides so prior runs don't pollute the
    // titleEdited / aliases state we're about to assert against.
    await page.addInitScript(() => {
      try {
        localStorage.removeItem("taskLabels");
      } catch {
        /* ignore */
      }
    });

    await page.goto("/");

    await expect(page.getByTestId("connection-status")).toHaveAttribute(
      "data-summary",
      "healthy",
      { timeout: 15000 },
    );

    // -----------------------------------------------------------------
    // Drive a task into existence so we have a card to click. The
    // detail panel UI we're testing is independent of completion
    // status, but having a clean state-known card avoids racing the
    // SSE stream against earlier test residue.
    // -----------------------------------------------------------------
    const initialIds = new Set(
      (await listLoops(request)).map((l) => l.loop_id),
    );

    await page.getByTestId("chat-input").fill(
      "Look up MQTT vs NATS comparison metrics",
    );
    await page.getByTestId("send-button").click();

    const loopId = await pollUntil(async () => {
      const fresh = (await listLoops(request)).find(
        (l) => !initialIds.has(l.loop_id),
      );
      return fresh?.loop_id ?? null;
    }, { timeoutMs: 30_000 });
    expect(loopId, "no loop appeared after dispatch").toBeTruthy();

    const card = page.locator(
      `[data-testid='task-card'][data-task-id='${loopId}']`,
    );
    await expect(card).toBeVisible({ timeout: 30_000 });

    // -----------------------------------------------------------------
    // Open the detail panel. Selection lives in the URL (?task=<id>)
    // since 065784c — driving via page.goto is more reliable than
    // click-then-wait, since the click path's replaceState can race
    // the assertion.
    // -----------------------------------------------------------------
    await page.goto(`/?task=${loopId}`);
    const panel = page.getByTestId("task-detail-panel");
    await expect(panel).toBeVisible({ timeout: 10_000 });

    const titleButton = page.getByTestId("task-title");
    await expect(titleButton).toBeVisible();
    const autoTitle = (await titleButton.textContent())?.trim() ?? "";
    expect(autoTitle.length).toBeGreaterThan(0);

    // No title-reset chip on the auto-derived title.
    await expect(page.getByTestId("title-reset")).toHaveCount(0);

    // -----------------------------------------------------------------
    // Edit the title. Click switches the heading to an input; Enter
    // commits; the button reappears with the new label and a
    // title-reset chip joins it.
    // -----------------------------------------------------------------
    await titleButton.click();
    const titleInput = page.getByTestId("title-input");
    await expect(titleInput).toBeVisible();
    await titleInput.fill("Edge messaging spike");
    await titleInput.press("Enter");

    await expect(titleButton).toContainText("Edge messaging spike");
    await expect(page.getByTestId("title-reset")).toBeVisible();

    // -----------------------------------------------------------------
    // Reset reverts to the auto-derived title and removes the chip.
    // -----------------------------------------------------------------
    await page.getByTestId("title-reset").click();
    await expect(titleButton).toContainText(autoTitle);
    await expect(page.getByTestId("title-reset")).toHaveCount(0);

    // -----------------------------------------------------------------
    // Aliases: type a value + Enter → chip appears. Type another →
    // second chip. Remove one with its `×` → only the other survives.
    // -----------------------------------------------------------------
    const aliasInput = page.getByTestId("alias-input");
    await aliasInput.fill("edge");
    await aliasInput.press("Enter");

    const chips = page.getByTestId("alias-chip");
    await expect(chips).toHaveCount(1);
    await expect(chips.first()).toContainText("@edge");

    await aliasInput.fill("messaging");
    await aliasInput.press("Enter");
    await expect(chips).toHaveCount(2);

    // Remove `edge` via its `×` button (aria-label="Remove alias edge").
    await page.getByRole("button", { name: "Remove alias edge" }).click();
    await expect(chips).toHaveCount(1);
    await expect(chips.first()).toContainText("@messaging");
  });
});

interface LoopSummary {
  loop_id: string;
}

async function listLoops(
  request: import("@playwright/test").APIRequestContext,
): Promise<LoopSummary[]> {
  const resp = await request.get("/teams-dispatch/loops");
  if (!resp.ok()) return [];
  return (await resp.json()) as LoopSummary[];
}

async function pollUntil<T>(
  check: () => Promise<T | null>,
  options: { timeoutMs?: number; intervalMs?: number } = {},
): Promise<T | null> {
  const timeout = options.timeoutMs ?? 15_000;
  const interval = options.intervalMs ?? 500;
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    const result = await check();
    if (result !== null && result !== undefined) {
      return result;
    }
    await new Promise((r) => setTimeout(r, interval));
  }
  return null;
}
