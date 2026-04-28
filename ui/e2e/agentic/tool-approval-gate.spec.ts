import { test, expect } from "@playwright/test";

/**
 * Journey: Tool Approval Gate
 *
 * Goal: An agent proposes a high-risk tool (`create_rule`), the loop pauses
 * for human approval, the user approves via the Board's detail panel,
 * the tool executes, and the loop completes.
 *
 * Validates:
 *   - ChatBar → agentApi.sendMessage() wiring (new task creation)
 *   - Kanban board renders the task card via SSE activity stream
 *   - Task card transitions through state columns (Thinking → Needs You)
 *   - TaskDetailPanel shows Approve/Reject buttons for awaiting_approval
 *   - Approve signal → loop resumes → card moves to Done column
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
 */

test.describe("Tool Approval Gate", () => {
  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok()).toBe(true);
  });

  // Blocked on an upstream framework gap. The agentic-tools
  // ApprovalFilter rejects the create_rule call with the documented
  // `approval_required: ...` prefix, but agentic-loop never imports
  // IsApprovalRequired and never transitions the loop to
  // LoopStateAwaitingApproval. Confirmed on semstreams beta.15 AND
  // beta.16 — the prefix-detection wiring at the loop side is the
  // missing piece (the comment in approval_filter.go:11-12 is
  // aspirational). Until that lands, the loop treats the rejection
  // as a regular tool failure, feeds the error back to the LLM,
  // consumes the fixture's turn-2 completion, and terminates `success`
  // without ever pausing for human approval. Re-enable when the
  // upstream loop-side wiring exists.
  test.skip("user creates task via chat bar, approves tool, loop completes", async ({
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
    // correlate with the card that appears on the kanban board.
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
    // Step 4 — the task card should appear on the kanban board via SSE.
    // Wait for a card with the matching task_id (rendered as the card
    // title) or loop_id to become visible.
    // -----------------------------------------------------------------
    const taskCard = page
      .getByTestId("task-card")
      .first();
    await expect(taskCard).toBeVisible({ timeout: 30000 });

    // -----------------------------------------------------------------
    // Step 5 — wait for the card to reach awaiting_approval state.
    // The state badge text updates via SSE as the loop transitions.
    // -----------------------------------------------------------------
    await expect(
      page.locator("[data-testid='task-card'] [data-state='awaiting_approval']"),
    ).toBeVisible({ timeout: 30000 });

    // -----------------------------------------------------------------
    // Step 6 — click the card to open the detail panel, then click
    // Approve. The TaskDetailPanel shows Approve/Reject buttons when
    // the task is in awaiting_approval state.
    // -----------------------------------------------------------------
    await taskCard.click();

    await expect(page.getByTestId("task-detail-panel")).toBeVisible();

    await page.getByRole("button", { name: "Approve" }).click();

    // -----------------------------------------------------------------
    // Step 7 — verify the loop transitions to a terminal-success state.
    // Upstream dispatch emits both `complete` and `success` aliases on
    // completion; assert via the canonical kanban column instead of
    // the raw badge value.
    // -----------------------------------------------------------------
    await expect(
      page.locator("[data-testid='task-card'][data-column='done']"),
    ).toBeVisible({ timeout: 30000 });

    // -----------------------------------------------------------------
    // Step 8 — backend-state assertion. The canonical source of truth
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
