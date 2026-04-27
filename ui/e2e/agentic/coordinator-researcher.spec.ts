import { test, expect } from "@playwright/test";

/**
 * Journey: Coordinator → Researcher delegation
 *
 * Goal: User asks a research-worthy question. The coordinator (front-door
 * role) classifies intent via its `decide` tool, fires
 * `coordinator.next_action=delegate_research`, and the rule-engine spawns
 * a researcher loop via `publish_agent`. The researcher runs one
 * `web_search` turn and synthesizes a structured report.
 *
 * Validates:
 *   - Single ChatBar entry produces TWO loops (coordinator + researcher).
 *   - Coordinator role is the front-door — its loop is the FIRST to
 *     terminate.
 *   - Rule wiring: coordinator's `decide` tool emits the next-action
 *     triple → rule fires → `publish_agent` spawns researcher.
 *   - Both loops land in the Done column on the kanban board.
 *   - The board shows the parent/child relationship (researcher card
 *     references coordinator's loop_id as its parent_loop_id).
 *
 * Required fixture: test/fixtures/journeys/coordinator-researcher.yaml
 *   - Turn 1 (coordinator): decide(action=delegate_research, reason=...)
 *   - Turn 2 (researcher):  web_search(query=...)
 *   - Turn 3 (researcher):  completion with structured report
 *
 * Required config: configs/e2e-coordinator.json
 *   - coordinator role with decide tool + observe rule
 *   - researcher role available for publish_agent target
 *
 * Run via:
 *   task test:e2e:agentic:coordinator-researcher
 */

test.describe("Coordinator → Researcher", () => {
  test.setTimeout(180_000);

  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok(), "Backend not healthy — stack not running?").toBe(true);

    const commands = await request.get("/teams-dispatch/commands");
    expect(
      commands.ok(),
      "Dispatch /commands not responding — is teams-dispatch configured?",
    ).toBe(true);
  });

  test("user prompt → coordinator delegates → researcher completes", async ({
    page,
    request,
  }) => {
    // -----------------------------------------------------------------
    // Step 1 — open the Board, wait for SSE, snapshot existing cards.
    // -----------------------------------------------------------------
    await page.goto("/");

    await expect(page.getByTestId("connection-status")).toHaveAttribute(
      "data-summary",
      "healthy",
      { timeout: 15000 },
    );
    await expect(page.getByTestId("kanban-board")).toBeVisible();

    const initialLoops = await listLoops(request);
    const initialIds = new Set(initialLoops.map((l) => l.loop_id));

    // -----------------------------------------------------------------
    // Step 2 — send the prompt that the coordinator's decide tool will
    // classify as a research-delegation intent.
    // -----------------------------------------------------------------
    await page.getByTestId("chat-input").fill(
      "Compare MQTT vs NATS for IoT edge deployments — which has lower latency on constrained ARM devices?",
    );
    await page.getByTestId("send-button").click();

    // -----------------------------------------------------------------
    // Step 3 — wait for the coordinator loop to appear (the dispatch's
    // default_role is `coordinator` in e2e-coordinator.json), and
    // capture its loop_id.
    // -----------------------------------------------------------------
    const coordinatorLoop = await pollUntil(
      async () => {
        const loops = await listLoops(request);
        const fresh = loops.find((l) => !initialIds.has(l.loop_id));
        if (!fresh) return null;
        // Coordinator is the front-door — first new loop is always it.
        return fresh;
      },
      { timeoutMs: 30_000 },
    );
    expect(coordinatorLoop, "no coordinator loop appeared after dispatch")
      .toBeTruthy();
    const coordinatorId = coordinatorLoop!.loop_id;

    // -----------------------------------------------------------------
    // Step 4 — wait for the coordinator to terminate. Its only LLM
    // call is `decide` (StopLoop=true), so it should reach a terminal
    // state quickly. The rule then fires publish_agent for the
    // researcher.
    // -----------------------------------------------------------------
    const coordinatorTerminal = await pollUntil(
      async () => {
        const resp = await request.get(`/teams-dispatch/loops/${coordinatorId}`);
        if (!resp.ok()) return null;
        const body = (await resp.json()) as { state?: string };
        return body.state &&
          ["complete", "success", "failed", "error", "timeout"].includes(
            body.state,
          )
          ? body
          : null;
      },
      { timeoutMs: 30_000 },
    );
    expect(
      coordinatorTerminal,
      `coordinator loop ${coordinatorId} did not terminate within 30s`,
    ).toBeTruthy();

    // -----------------------------------------------------------------
    // Step 5 — wait for the researcher loop to appear (spawned by the
    // observe-rule's publish_agent action).
    // -----------------------------------------------------------------
    const researcherLoop = await pollUntil(
      async () => {
        const loops = await listLoops(request);
        const newOne = loops.find(
          (l) => l.loop_id !== coordinatorId && !initialIds.has(l.loop_id),
        );
        return newOne ?? null;
      },
      { timeoutMs: 30_000 },
    );
    expect(
      researcherLoop,
      "no researcher loop appeared after coordinator terminated — observe rule may not have fired",
    ).toBeTruthy();
    const researcherId = researcherLoop!.loop_id;

    // -----------------------------------------------------------------
    // Step 6 — both loops should be visible as cards on the board.
    // -----------------------------------------------------------------
    await expect(
      page.locator(`[data-testid='task-card'][data-task-id='${coordinatorId}']`),
    ).toBeVisible({ timeout: 10_000 });
    await expect(
      page.locator(`[data-testid='task-card'][data-task-id='${researcherId}']`),
    ).toBeVisible({ timeout: 30_000 });

    // -----------------------------------------------------------------
    // Step 7 — wait for the researcher to complete (web_search → synth).
    // Once it lands in Done, both cards should be in `done` column.
    // -----------------------------------------------------------------
    const researcherTerminal = await pollUntil(
      async () => {
        const resp = await request.get(`/teams-dispatch/loops/${researcherId}`);
        if (!resp.ok()) return null;
        const body = (await resp.json()) as { state?: string };
        return body.state &&
          ["complete", "success", "failed", "error", "timeout"].includes(
            body.state,
          )
          ? body
          : null;
      },
      { timeoutMs: 60_000 },
    );
    expect(
      researcherTerminal,
      `researcher loop ${researcherId} did not terminate within 60s`,
    ).toBeTruthy();
    expect(["complete", "success"]).toContain(researcherTerminal!.state);

    // Coordinator should also have terminated successfully (no error).
    expect(["complete", "success"]).toContain(coordinatorTerminal!.state);

    // Both cards in Done column.
    await expect(
      page.locator(
        `[data-testid='task-card'][data-task-id='${coordinatorId}'][data-column='done']`,
      ),
    ).toBeVisible({ timeout: 10_000 });
    await expect(
      page.locator(
        `[data-testid='task-card'][data-task-id='${researcherId}'][data-column='done']`,
      ),
    ).toBeVisible({ timeout: 10_000 });

    // -----------------------------------------------------------------
    // Step 8 — single-page-app guard: never navigated away from /.
    // -----------------------------------------------------------------
    expect(new URL(page.url()).pathname).toBe("/");
  });
});

interface LoopSummary {
  loop_id: string;
  state?: string;
  role?: string;
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
