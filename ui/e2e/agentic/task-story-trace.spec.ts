import { test, expect } from "@playwright/test";

/**
 * Journey: Right-rail story + raw activity
 *
 * Goal: After a researcher loop completes, the user opens the task
 * detail panel and reads the AI's actions as a plain-language story
 * (TaskStory). The wire-level message log lives behind a "Show raw
 * activity" disclosure (TaskTrace) for the few users who want it.
 *
 * Validates:
 *   - TaskDetailPanel opens on task-card click.
 *   - Activity tab is the default (per recent merge of Trace into
 *     Activity, see commit 1026ad5).
 *   - TaskStory renders one row per trajectory step (model_call +
 *     tool_call), pulled from /teams-loop/trajectories/<loop_id>.
 *   - The "Show raw activity" disclosure opens TaskTrace, which polls
 *     /message-logger/entries and renders rows mentioning the loop_id.
 *
 * Required fixture: test/fixtures/journeys/deep-research.yaml
 *   (Reused — any 2-turn fixture with one tool_call yields enough
 *   trajectory steps + log entries to validate.)
 *
 * Required config: configs/e2e-deep-research.json
 *
 * Run via:
 *   task test:e2e:agentic:task-story-trace
 */

test.describe("Right-rail story + raw activity", () => {
  test.setTimeout(120_000);

  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok(), "Backend not healthy — stack not running?").toBe(true);
  });

  test("completed task exposes story rows + raw-activity drawer", async ({
    page,
    request,
  }) => {
    // -----------------------------------------------------------------
    // Step 1 — open the Board, send a research question.
    // -----------------------------------------------------------------
    await page.goto("/");

    await expect(page.getByTestId("connection-status")).toHaveAttribute(
      "data-summary",
      "healthy",
      { timeout: 15000 },
    );
    await expect(page.getByTestId("kanban-board")).toBeVisible();

    const initialIds = new Set(
      (await listLoops(request)).map((l) => l.loop_id),
    );

    await page.getByTestId("chat-input").fill(
      "Research how MQTT differs from NATS at the edge.",
    );
    await page.getByTestId("send-button").click();

    const loopId = await pollUntil(async () => {
      const fresh = (await listLoops(request)).find(
        (l) => !initialIds.has(l.loop_id),
      );
      return fresh?.loop_id ?? null;
    }, { timeoutMs: 30_000 });
    expect(loopId, "no loop appeared after dispatch").toBeTruthy();

    // -----------------------------------------------------------------
    // Step 2 — wait for the loop to finish so the trajectory has
    // model_call + tool_call entries to story-render.
    // -----------------------------------------------------------------
    const card = page.locator(
      `[data-testid='task-card'][data-task-id='${loopId}']`,
    );
    await expect(card).toBeVisible({ timeout: 30_000 });
    await expect(
      page.locator(
        `[data-testid='task-card'][data-task-id='${loopId}'][data-column='done']`,
      ),
    ).toBeVisible({ timeout: 60_000 });

    // -----------------------------------------------------------------
    // Step 3 — open the detail panel. Selection lives in the URL
    // (?task=<id>) since 065784c — drive via page.goto rather than
    // clicking, since the click path's replaceState can race the
    // assertion.
    // -----------------------------------------------------------------
    await page.goto(`/?task=${loopId}`);
    await expect(page.getByTestId("task-detail-panel")).toBeVisible({
      timeout: 10_000,
    });

    // Activity tab is the default after the Trace-into-Activity merge.
    // The story view is what user-facing taxonomy calls "the trace" —
    // the panel-activity testid + task-story testid live there.
    await expect(page.getByTestId("panel-activity")).toBeVisible();
    await expect(page.getByTestId("task-story")).toBeVisible();

    // -----------------------------------------------------------------
    // Step 4 — story-list should populate. Trajectory polling cadence
    // is 3s (TaskStory.svelte POLL_INTERVAL_MS); be generous.
    // -----------------------------------------------------------------
    const storyList = page.getByTestId("story-list");
    await expect(storyList).toBeVisible({ timeout: 15_000 });
    // At least one step (model_call). The 2-turn deep-research fixture
    // produces model_call → tool_call → model_call, so expect ≥ 2 steps.
    await expect(page.getByTestId("story-step")).toHaveCount(
      await page.getByTestId("story-step").count(),
    );
    expect(await page.getByTestId("story-step").count()).toBeGreaterThanOrEqual(2);

    // The first step is the user-prompt narrative bookend.
    await expect(page.getByTestId("story-line-asked")).toBeVisible();

    // -----------------------------------------------------------------
    // Step 5 — open the "Show raw activity" disclosure. Behind it lives
    // TaskTrace — a polled view of /message-logger/entries scoped to
    // this loop.
    // -----------------------------------------------------------------
    const rawToggle = page.getByTestId("raw-toggle");
    await expect(rawToggle).toBeVisible();
    await rawToggle.click();

    const trace = page.getByTestId("task-trace");
    await expect(trace).toBeVisible();
    // First poll runs immediately (TaskTrace's $effect on mount); allow
    // a short window for at least one entry mentioning this loop to
    // arrive.
    await expect(page.getByTestId("trace-row").first()).toBeVisible({
      timeout: 10_000,
    });
  });
});

interface LoopSummary {
  loop_id: string;
  state?: string;
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
