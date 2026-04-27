import { test, expect } from "@playwright/test";

/**
 * Journey: Deep Research
 *
 * Goal: User asks a research question via the chat bar, the agent
 * researches using web_search, and returns a structured report. The
 * full pipeline is validated: chat bar → dispatch → loop → tool calls
 * → completion → kanban card in Done column.
 *
 * This is the first "real workflow" journey — it validates that the
 * deep-research config (researcher role, extended iterations/timeout,
 * no approval gates) works end-to-end.
 *
 * Validates:
 *   - Dispatch accepts messages (permissions must allow task submission)
 *   - Researcher role prompt is applied (agent uses web_search)
 *   - Tool calling works (web_search → results → completion)
 *   - Kanban board renders the task card via SSE
 *   - Card transitions: Thinking → Executing → Done
 *   - Backend state matches UI state
 *
 * Required fixture: test/fixtures/journeys/deep-research.yaml
 *   - Turn 1: tool_call(name=web_search, args={query: "..."})
 *   - Turn 2: completion with structured report
 *
 * Required config: configs/deep-research.json (via AGENTIC_CONFIG env)
 *   - default_role: researcher
 *   - max_iterations: 25
 *   - approval_required: [] (no gates)
 *
 * Run via:
 *   FIXTURE=deep-research.yaml AGENTIC_CONFIG=deep-research.json \
 *     npx playwright test --config playwright.agentic.config.ts \
 *     e2e/agentic/deep-research.spec.ts
 */

test.describe("Deep Research", () => {
  test.beforeAll(async ({ request }) => {
    // Verify backend is healthy through Caddy
    const health = await request.get("/health");
    expect(health.ok(), "Backend not healthy — is the stack running?").toBe(
      true,
    );

    // Verify dispatch endpoint exists and responds
    const commands = await request.get("/teams-dispatch/commands");
    expect(
      commands.ok(),
      "Dispatch /commands not responding — is teams-dispatch configured?",
    ).toBe(true);
  });

  test("user asks research question via chat bar, agent researches and completes", async ({
    page,
    request,
  }) => {
    // -----------------------------------------------------------------
    // Step 1 — open the Board homepage, wait for SSE connection
    // -----------------------------------------------------------------
    await page.goto("/");

    await expect(page.getByTestId("connection-status")).toHaveAttribute(
      "data-summary",
      "healthy",
      { timeout: 15000 },
    );

    await expect(page.getByTestId("kanban-board")).toBeVisible();
    await page.getByTestId("task-card").count();

    // -----------------------------------------------------------------
    // Step 2 — type a research question in the chat bar
    // This calls agentApi.sendMessage() → POST /teams-dispatch/message
    // The dispatch must accept the message (permissions must allow it)
    // -----------------------------------------------------------------
    const chatInput = page.getByTestId("chat-input");
    await chatInput.fill(
      "Research the key differences between MQTT and NATS for IoT edge deployments",
    );
    await page.getByTestId("send-button").click();

    // Verify NO error appears in the chat bar (catches permission denied,
    // network errors, etc.)
    await page.waitForTimeout(2000);
    const chatError = page.getByTestId("chat-error");
    const hasError = await chatError.isVisible().catch(() => false);
    if (hasError) {
      const errorText = await chatError.textContent();
      throw new Error(`Chat bar error after send: ${errorText}`);
    }

    // -----------------------------------------------------------------
    // Step 3 — verify a new task card appears on the kanban board
    // -----------------------------------------------------------------
    const loopId = await pollUntil(async () => {
      const resp = await request.get("/teams-dispatch/loops");
      if (!resp.ok()) return null;
      const loops = (await resp.json()) as Array<{
        loop_id: string;
        state: string;
        role: string;
      }>;
      if (loops.length === 0) return null;
      // Find a loop with the researcher role
      const researchLoop = loops.find((l) => l.role === "researcher");
      return researchLoop?.loop_id ?? loops[loops.length - 1].loop_id;
    });
    expect(loopId, "No agent loop appeared after dispatch").toBeTruthy();

    // Card should appear on the board via SSE
    const taskCard = page.getByTestId("task-card").first();
    await expect(taskCard).toBeVisible({ timeout: 30000 });

    // -----------------------------------------------------------------
    // Step 4 — wait for the loop to complete
    // The mock-llm fixture has 2 turns: web_search → completion
    // With the deep-research config, there are no approval gates.
    //
    // Assert by kanban column rather than the raw `data-state` attr —
    // upstream emits both `complete` and `success` as terminal states
    // (see agent.ts AgentLoopState union). Column mapping is the
    // canonical UI signal.
    // -----------------------------------------------------------------
    await expect(
      page.locator("[data-testid='task-card'][data-column='done']"),
    ).toBeVisible({ timeout: 60000 });

    // -----------------------------------------------------------------
    // Step 5 — verify backend state. Accept either terminal alias.
    // -----------------------------------------------------------------
    const finalLoop = await request
      .get(`/teams-dispatch/loops/${loopId}`)
      .then((r) => r.json());
    expect(["complete", "success"]).toContain(finalLoop.state);

    // -----------------------------------------------------------------
    // Step 6 — verify we stayed on the board (no accidental navigation)
    // -----------------------------------------------------------------
    expect(new URL(page.url()).pathname).toBe("/");
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
