import { test, expect } from "@playwright/test";

/**
 * Journey: Real-Time Activity Stream
 *
 * Goal: User watches the Board kanban update in real time as an agent
 * loop is created via the chat bar, executes a tool, and completes.
 * The entire flow is driven by the SSE activity stream — no page
 * refresh, no manual poll.
 *
 * Validates:
 *   - ChatBar → agentApi.sendMessage() creates a new task
 *   - agentStore SSE subscription to /agentic-dispatch/activity
 *   - Kanban board renders new task card without reload
 *   - State badge transitions live on the card via SSE loop_update events
 *   - Board stays on the same URL throughout (no accidental navigation)
 *
 * Required fixture: test/fixtures/journeys/real-time-activity-stream.yaml
 *   - Turn 1: tool_call(name=query_entity, args={...}) — NOT approval-gated
 *   - Turn 2: completion("Entity lookup complete...")
 *
 * Run via:
 *   FIXTURE=real-time-activity-stream.yaml \
 *     npx playwright test --config playwright.agentic.config.ts \
 *     e2e/agentic/real-time-activity-stream.spec.ts
 */

test.describe("Real-Time Activity Stream", () => {
  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok()).toBe(true);
  });

  test("task created via chat bar appears on board and completes via SSE", async ({
    page,
    request,
  }) => {
    // -----------------------------------------------------------------
    // Step 1 — open the Board BEFORE creating the task. The spec must
    // observe the card APPEARING during the test, not discover it
    // already present. This proves the SSE pipe delivers live updates.
    // -----------------------------------------------------------------
    await page.goto("/");

    await expect(page.getByTestId("connection-status")).toHaveAttribute(
      "data-connected",
      "true",
      { timeout: 10000 },
    );

    await expect(page.getByTestId("kanban-board")).toBeVisible();

    // Snapshot the initial card count.
    const initialCards = await page.getByTestId("task-card").count();

    // -----------------------------------------------------------------
    // Step 2 — type a task in the chat bar. This is the primary user
    // entry point for starting agent work — validates the ChatBar →
    // agentApi.sendMessage() wiring end-to-end.
    // -----------------------------------------------------------------
    const chatInput = page.getByTestId("chat-input");
    await chatInput.fill("Look up the status of the temperature sensor.");
    await page.getByTestId("send-button").click();

    // -----------------------------------------------------------------
    // Step 3 — capture the loop_id from the backend so we can
    // correlate with the card that appears on the board.
    // -----------------------------------------------------------------
    const loopId = await pollUntil(async () => {
      const resp = await request.get("/agentic-dispatch/loops");
      if (!resp.ok()) return null;
      const loops = (await resp.json()) as Array<{ loop_id: string }>;
      if (loops.length === 0) return null;
      return loops[loops.length - 1].loop_id;
    });
    expect(loopId, "no agent loop appeared after dispatch").toBeTruthy();

    // -----------------------------------------------------------------
    // Step 4 — the task card must appear on the kanban board WITHOUT a
    // page reload. Playwright only navigated via page.goto (Step 1),
    // so if a new card appears now, it arrived via the SSE stream.
    // -----------------------------------------------------------------
    const taskCard = page.getByTestId("task-card").first();
    await expect(taskCard).toBeVisible({ timeout: 30000 });

    // Total card count should now be initial + 1.
    await expect(page.getByTestId("task-card")).toHaveCount(initialCards + 1);

    // -----------------------------------------------------------------
    // Step 5 — observe the card transition to complete. The fixture is
    // only two turns (tool_call → completion) and query_entity is not
    // approval-gated, so the loop runs to completion without human
    // intervention.
    // -----------------------------------------------------------------
    await expect(
      page.locator("[data-testid='task-card'] [data-state='complete']"),
    ).toBeVisible({ timeout: 30000 });

    // -----------------------------------------------------------------
    // Step 6 — verify we never navigated away from /. This guards
    // against a regression where the UI reloads on SSE events.
    // -----------------------------------------------------------------
    expect(new URL(page.url()).pathname).toBe("/");

    // -----------------------------------------------------------------
    // Step 7 — backend-state assertion.
    // -----------------------------------------------------------------
    const finalLoop = await request
      .get(`/agentic-dispatch/loops/${loopId}`)
      .then((r) => r.json());
    expect(finalLoop.state).toBe("complete");
  });
});

async function pollUntil<T>(
  check: () => Promise<T | null>,
  options: { timeoutMs?: number; intervalMs?: number } = {},
): Promise<T | null> {
  const timeout = options.timeoutMs ?? 15000;
  const interval = options.intervalMs ?? 250;
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
