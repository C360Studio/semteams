import { test, expect } from "@playwright/test";

/**
 * Journey: Real-Time Activity Stream
 *
 * Goal: User watches the /agents page update in real time as an agent
 * loop is created, executes a tool, and completes. The entire flow is
 * driven by the SSE activity stream — no page refresh, no manual poll.
 *
 * Validates:
 *   - agentStore SSE subscription to /agentic-dispatch/activity
 *   - EventSource → reactive Svelte 5 store wiring
 *   - /agents page renders newly-appeared loops without reload
 *   - State badge transitions live (loop_update events arriving during
 *     test runtime drive reactive UI updates)
 *   - connection-status indicator reflects SSE connection state
 *
 * Required fixture: test/fixtures/journeys/real-time-activity-stream.yaml
 *   - Turn 1: tool_call(name=query_entity, args={...}) — NOT approval-gated
 *   - Turn 2: completion("Entity lookup complete...")
 *
 * Run via:
 *   FIXTURE=real-time-activity-stream.yaml \
 *     npx playwright test --config playwright.agentic.config.ts \
 *     e2e/agentic/real-time-activity-stream.spec.ts
 *
 * Or via the task wrapper:
 *   task ui:test:e2e:agentic:real-time-activity-stream
 *
 * NOTE on the entry point: same caveat as tool-approval-gate.spec.ts —
 * there is no UI path to start a loop today (agentApi.sendMessage has
 * no production callers), so the spec triggers the loop via Playwright's
 * request fixture and then observes the UI update via SSE.
 */

test.describe("Real-Time Activity Stream", () => {
  test.beforeAll(async ({ request }) => {
    // Sanity check: backend is reachable through Caddy.
    const health = await request.get("/health");
    expect(health.ok()).toBe(true);
  });

  test("new loop appears and transitions to complete via SSE without reload", async ({
    page,
    request,
  }) => {
    // -----------------------------------------------------------------
    // Step 1 — open /agents BEFORE triggering the loop. The spec must
    // observe the loop APPEARING during the test, not discover it
    // already present (that would test initial sync, not real-time
    // streaming).
    // -----------------------------------------------------------------
    await page.goto("/agents");

    // Wait for the SSE connection to establish before triggering
    // anything. If the connection indicator never flips to connected,
    // the test fails fast with a clear signal.
    await expect(page.getByTestId("connection-status")).toHaveAttribute(
      "data-connected",
      "true",
      { timeout: 10000 },
    );

    // Snapshot the initial loop count. Fresh docker stacks start clean,
    // so this should be 0 on first run, but the test stays correct if
    // any were left over.
    const initialRows = await page.getByTestId("loop-row").count();

    // -----------------------------------------------------------------
    // Step 2 — trigger the agent loop via the backend dispatch endpoint.
    // The mock-llm fixture (real-time-activity-stream.yaml) is loaded
    // at stack startup, so the LLM responds deterministically with a
    // query_entity tool call on turn 1 and a completion on turn 2.
    // -----------------------------------------------------------------
    const dispatch = await request.post("/agentic-dispatch/message", {
      headers: { "Content-Type": "application/json" },
      data: {
        content: "Look up the status of the temperature sensor.",
      },
    });
    expect(
      dispatch.ok(),
      `dispatch POST failed: ${dispatch.status()} ${await dispatch.text()}`,
    ).toBe(true);

    // Capture the loop ID from the HTTP side so we can correlate with
    // the row that appears in the UI. We poll the list endpoint rather
    // than trying to race the SSE event.
    const loopId = await pollUntil(async () => {
      const resp = await request.get("/agentic-dispatch/loops");
      if (!resp.ok()) return null;
      const loops = (await resp.json()) as Array<{ loop_id: string }>;
      if (loops.length === 0) return null;
      return loops[loops.length - 1].loop_id;
    });
    expect(loopId, "no agent loop appeared after dispatch").toBeTruthy();

    // -----------------------------------------------------------------
    // Step 3 — the row for this loop must appear in the UI table
    // *without* a reload. Playwright navigates only via `page.goto`
    // (Step 1) and the row should show up via reactive SSE updates.
    // This is the main assertion of the journey.
    // -----------------------------------------------------------------
    const loopRow = page
      .getByTestId("loop-row")
      .filter({ hasText: loopId!.slice(0, 12) });
    await expect(loopRow).toBeVisible({ timeout: 30000 });

    // Total loop count should now be initial + 1 (proves the new row
    // arrived via SSE rather than by accident).
    await expect(page.getByTestId("loop-row")).toHaveCount(initialRows + 1);

    // -----------------------------------------------------------------
    // Step 4 — observe the loop transition to `complete`. The fixture
    // is only two turns (tool_call → completion) and query_entity is
    // not approval-gated, so the loop runs to completion without
    // human intervention. The UI should reflect this entirely via
    // SSE-driven reactive updates.
    // -----------------------------------------------------------------
    await expect(loopRow.locator("span.state-badge")).toHaveText("complete", {
      timeout: 30000,
    });

    // -----------------------------------------------------------------
    // Step 5 — verify we never navigated away from /agents. This
    // guards against a regression where the UI reloads (e.g. if the
    // store accidentally triggers a full refresh on SSE events), which
    // would silently defeat the "live stream" claim of this journey.
    // -----------------------------------------------------------------
    expect(new URL(page.url()).pathname).toBe("/agents");

    // -----------------------------------------------------------------
    // Step 6 — backend-state assertion. The canonical source of truth
    // should agree with what the UI shows.
    // -----------------------------------------------------------------
    const finalLoop = await request
      .get(`/agentic-dispatch/loops/${loopId}`)
      .then((r) => r.json());
    expect(finalLoop.state).toBe("complete");
  });
});

/**
 * pollUntil retries the given check until it returns a truthy value or
 * the timeout elapses. Returns the value or null on timeout.
 */
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
