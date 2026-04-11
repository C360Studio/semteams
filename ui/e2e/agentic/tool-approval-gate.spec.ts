import { test, expect } from "@playwright/test";

/**
 * Journey: Tool Approval Gate
 *
 * Goal: An agent proposes a high-risk tool (`create_rule`), the loop pauses
 * for human approval, the user approves via the /agents page, the tool
 * executes, and the loop completes.
 *
 * Validates:
 *   - Phase 4 backend HITL gate (RequiresApproval enforcement in
 *     agentic-loop's ApprovalFilter)
 *   - HTTP signal endpoint accepting `approve` (PR closing the Phase 4 gap)
 *   - /agents page rendering loops with `awaiting_approval` state
 *   - /agents page Approve button → POST /agentic-dispatch/loops/{id}/signal
 *   - SSE activity stream → agentStore → reactive UI state updates
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
 * Or via the task wrapper:
 *   task ui:test:e2e:agentic:tool-approval-gate
 *
 * NOTE on the entry point: The UI does not currently expose a way to start
 * an agent loop from a user action — `agentApi.sendMessage()` exists but
 * has zero production callers. This spec triggers the loop directly via
 * the backend HTTP endpoint (POST /agentic-dispatch/message) using the
 * Playwright `request` fixture, then exercises the rest of the journey
 * through the UI. When the UI gains a chat-driven entry point, this
 * setup step can be replaced with the corresponding UI action.
 */

test.describe("Tool Approval Gate", () => {
  test.beforeAll(async ({ request }) => {
    // Sanity check: backend is reachable through Caddy.
    const health = await request.get("/health");
    expect(health.ok()).toBe(true);
  });

  test("agent proposes create_rule, user approves, loop completes", async ({
    page,
    request,
  }) => {
    // -----------------------------------------------------------------
    // Step 1 — trigger the agent loop via the backend dispatch endpoint.
    // The mock-llm fixture (tool-approval-gate.yaml) is loaded at stack
    // startup, so the LLM will respond deterministically with the
    // create_rule tool call on the first turn.
    // -----------------------------------------------------------------
    const dispatch = await request.post("/agentic-dispatch/message", {
      headers: { "Content-Type": "application/json" },
      data: {
        content:
          "Add a rule that alerts when environmental sensor temperature exceeds 100",
      },
    });
    expect(
      dispatch.ok(),
      `dispatch POST failed: ${dispatch.status()} ${await dispatch.text()}`,
    ).toBe(true);

    // -----------------------------------------------------------------
    // Step 2 — find the loop that just got created. The dispatch
    // response shape varies; the simplest portable approach is to list
    // loops and pick the most recent one. We poll because loop creation
    // is async (NATS round-trip + state machine init).
    // -----------------------------------------------------------------
    const loopId = await pollUntil(async () => {
      const resp = await request.get("/agentic-dispatch/loops");
      if (!resp.ok()) return null;
      const loops = (await resp.json()) as Array<{
        loop_id: string;
        state: string;
      }>;
      // Return the loop_id of the newest non-terminal loop, or any
      // loop if there's only one.
      if (loops.length === 0) return null;
      return loops[loops.length - 1].loop_id;
    });
    expect(loopId, "no agent loop appeared after dispatch").toBeTruthy();

    // -----------------------------------------------------------------
    // Step 3 — open the /agents page and wait for the loop to appear in
    // `awaiting_approval` state. This is the moment the UI shows the
    // human-in-the-loop gate.
    // -----------------------------------------------------------------
    await page.goto("/agents");

    // The agents page subscribes to /agentic-dispatch/activity SSE on
    // mount. Wait for the connection-status indicator to flip to
    // connected before asserting on loop rows.
    await expect(page.getByTestId("connection-status")).toHaveAttribute(
      "data-connected",
      "true",
      { timeout: 10000 },
    );

    // Wait for our loop's row to appear and reach awaiting_approval.
    // The loop_id is rendered truncated (12 chars) in the table, so we
    // match on a substring.
    const loopRow = page
      .getByTestId("loop-row")
      .filter({ hasText: loopId!.slice(0, 12) });
    await expect(loopRow).toBeVisible({ timeout: 30000 });
    await expect(loopRow.locator("span.state-badge")).toHaveText(
      "awaiting approval",
      { timeout: 30000 },
    );

    // -----------------------------------------------------------------
    // Step 4 — click Approve. This calls
    // POST /agentic-dispatch/loops/{id}/signal with {type: "approve"}
    // via agentApi.sendSignal — see ui/src/routes/agents/+page.svelte
    // line 117.
    // -----------------------------------------------------------------
    await loopRow.getByRole("button", { name: "Approve" }).click();

    // -----------------------------------------------------------------
    // Step 5 — verify the loop transitions to complete. The backend
    // executes the create_rule tool, feeds the result back to the LLM,
    // mock-llm returns the second fixture entry (completion), and the
    // loop's state machine moves to `complete`.
    // -----------------------------------------------------------------
    await expect(loopRow.locator("span.state-badge")).toHaveText("complete", {
      timeout: 30000,
    });

    // -----------------------------------------------------------------
    // Step 6 — backend-state assertion. The UI showing "complete" is
    // necessary but not sufficient — also verify the canonical source
    // of truth (the loop entity in the agentic-dispatch HTTP API)
    // agrees.
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
