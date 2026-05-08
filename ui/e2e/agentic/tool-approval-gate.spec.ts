import { test, expect } from "@playwright/test";

/**
 * Journey: Tool Approval Gate
 *
 * Goal: An agent proposes a high-risk tool (`create_rule`), the loop pauses
 * for human approval, the user approves via PendingApprovalSection inside
 * TaskDetailPanel, the tool executes, and the loop completes.
 *
 * Validates:
 *   - ChatBar → agentApi.sendMessage() wiring (new task creation)
 *   - Kanban board renders the task card via SSE activity stream
 *   - Task card transitions through state columns (Thinking → Needs You)
 *   - PendingApprovalSection renders for awaiting_approval state with the
 *     gated tool's name + arguments + approve/reject controls
 *   - approval-approve click → POST /teams-dispatch/loops/{id}/approval →
 *     ApprovalResponse → loop resumes → card moves to Done column
 *   - Backend state matches UI state
 *
 * Required fixture: test/fixtures/journeys/tool-approval-gate.yaml
 *   - Turn 1: tool_call(name=create_rule, args={...})
 *   - Turn 2: completion("Rule created successfully...")
 *
 * Run via:
 *   FIXTURE=tool-approval-gate.yaml \
 *     npx playwright test --config playwright.agentic.config.ts \
 *     e2e/agentic/tool-approval-gate.spec.ts
 *
 * Backend wiring is feature-flag-free as of semstreams beta.22 + the
 * cmd/semteams/middleware.go X-User-Id lift. UI wiring landed in this
 * branch (feat/approval-flow-ui).
 */

test.describe("Tool Approval Gate", () => {
  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok()).toBe(true);
  });

  test("user creates task via chat bar, approves tool, loop completes", async ({
    page,
    request,
  }) => {
    // -----------------------------------------------------------------
    // Step 1 — open the Board homepage. The kanban and chat bar load,
    // agentStore connects via SSE.
    // -----------------------------------------------------------------
    await page.goto("/");

    await expect(page.getByTestId("connection-status")).toHaveAttribute(
      "data-summary",
      "healthy",
      { timeout: 10000 },
    );

    await expect(page.getByTestId("kanban-board")).toBeVisible();

    // -----------------------------------------------------------------
    // Step 2 — type a task in the chat bar. This calls
    // agentApi.sendMessage() → POST /teams-dispatch/message.
    // The mock-llm fixture responds with a create_rule tool call,
    // which triggers the approval gate.
    // -----------------------------------------------------------------
    const chatInput = page.getByTestId("chat-input");
    await chatInput.fill(
      "Add a rule that alerts when environmental sensor temperature exceeds 100",
    );
    await page.getByTestId("send-button").click();

    // -----------------------------------------------------------------
    // Step 3 — poll the backend to find the loop_id. We need this to
    // correlate with the card that appears on the kanban board AND to
    // drive selection via ?task=<id>.
    // -----------------------------------------------------------------
    const loopId = await pollUntil(async () => {
      const resp = await request.get("/teams-dispatch/loops");
      if (!resp.ok()) return null;
      const loops = (await resp.json()) as Array<{
        loop_id: string;
        state: string;
      }>;
      if (loops.length === 0) return null;
      return loops[loops.length - 1].loop_id;
    });
    expect(loopId, "no agent loop appeared after dispatch").toBeTruthy();

    // -----------------------------------------------------------------
    // Step 4 — wait for the card to reach awaiting_approval state.
    // SSE delivers the loop's state transition + pending_approval
    // snapshot via the activity stream.
    // -----------------------------------------------------------------
    // For top-level loops with their own awaiting_approval state, the
    // primary loop's state cascades to the needs_you column. Asserting
    // on data-column instead of the StateBadge data-state keeps the
    // selector consistent with chain journeys where the awaiting_approval
    // is on a child loop and only the column attribute reflects it.
    await expect(
      page.locator("[data-testid='task-card'][data-column='needs_you']"),
    ).toBeVisible({ timeout: 30000 });

    // -----------------------------------------------------------------
    // Step 5 — open the detail panel via URL state. Direct
    // page.goto?task=<id> is the canonical selection path —
    // taskCard.click() races replaceState and is flaky. See the
    // feedback_url_state_in_tests memory.
    // -----------------------------------------------------------------
    await page.goto(`/?task=${loopId}`);
    await expect(page.getByTestId("task-detail-panel")).toBeVisible();

    // -----------------------------------------------------------------
    // Step 6 — PendingApprovalSection renders the gated tool with
    // approve/reject controls. Verify the tool name surfaces
    // (the fixture configures `create_rule`).
    // -----------------------------------------------------------------
    await expect(page.getByTestId("pending-approval-section")).toBeVisible();
    await expect(page.getByTestId("approval-tool-name")).toHaveText(
      "create_rule",
    );

    // -----------------------------------------------------------------
    // Step 7 — click Approve. The component POSTs to
    // /teams-dispatch/loops/{id}/approval with X-User-Id from the
    // userIdentity store (default "ui-anonymous"). The dispatch
    // publishes ApprovalResponse on agent.approval_response.<loop_id>;
    // the agentic-loop resumes, runs the tool, takes the next mock-llm
    // turn, and terminates.
    // -----------------------------------------------------------------
    await page.getByTestId("approval-approve").click();

    // -----------------------------------------------------------------
    // Step 8 — verify the loop transitions to a terminal-success state.
    // Upstream dispatch emits both `complete` and `success` aliases on
    // completion; assert via the canonical kanban column.
    // -----------------------------------------------------------------
    await expect(
      page.locator("[data-testid='task-card'][data-column='done']"),
    ).toBeVisible({ timeout: 30000 });

    // -----------------------------------------------------------------
    // Step 9 — backend-state assertion. The canonical source of truth
    // should agree with what the UI shows. Accept either terminal alias.
    // -----------------------------------------------------------------
    const finalLoop = await request
      .get(`/teams-dispatch/loops/${loopId}`)
      .then((r) => r.json());
    expect(["complete", "success"]).toContain(finalLoop.state);
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
