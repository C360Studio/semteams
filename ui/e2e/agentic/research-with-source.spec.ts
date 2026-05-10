import { test, expect } from "@playwright/test";

/**
 * Journey: Research with Source Acquisition
 *
 * R2 slice 2 of the ADR-031 phasing. End-to-end exercise of the
 * approval-gated add_source_repo tool: an agent proposes adding a
 * GitHub repo to SemSource, the loop pauses on the approval gate
 * (per ADR-030), a human approves via PendingApprovalSection, the
 * tool re-dispatches with ApprovedBy, SemSource accepts the
 * AddRequest and replies, the loop completes.
 *
 * Validates:
 *   - The product-shell-local add_source_repo tool registers correctly
 *     (env-driven SEMTEAMS_SEMSOURCE_NAMESPACES + DEFAULT_NAMESPACE)
 *   - approval_required gate fires for add_source_repo
 *   - PendingApprovalSection renders the gated tool with the
 *     repository URL visible in the args block
 *   - Approve click → POST /teams-dispatch/loops/{id}/approval
 *   - Tool re-dispatches against the running SemSource container,
 *     publishes AddRequest on graph.ingest.add.research, parses
 *     AddReply
 *   - Loop terminates outcome=success
 *
 * Required fixture: test/fixtures/journeys/research-with-source.yaml.
 * Required docker-compose profile: semsource (SemSource container is
 * gated behind the profile so journeys that don't need it skip the
 * extra boot cost).
 */

test.describe("Research with Source Acquisition", () => {
  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok()).toBe(true);
  });

  test("source-registrar proposes add_source_repo, human approves, SemSource registers", async ({
    page,
    request,
  }) => {
    // -----------------------------------------------------------------
    // Step 1 — open the Board so the SSE activity stream is connected
    // before the loop spawns. PendingApprovalSection assertions in
    // later steps need the kanban / detail-panel surface to be live.
    // -----------------------------------------------------------------
    await page.goto("/");

    await expect(page.getByTestId("connection-status")).toHaveAttribute(
      "data-summary",
      "healthy",
      { timeout: 10000 },
    );

    await expect(page.getByTestId("kanban-board")).toBeVisible();

    // -----------------------------------------------------------------
    // Step 2 — chat: ask the source-registrar to register the OSH core
    // repo. The dispatch's default_role is `source-registrar`; the
    // mock-llm fixture's first response is the add_source_repo
    // tool_call (approval-gated).
    // -----------------------------------------------------------------
    const chatInput = page.getByTestId("chat-input");
    await chatInput.fill(
      "Add https://github.com/sensorhub-tools/osh-core to our research SemSource so the researcher can query it.",
    );
    await page.getByTestId("send-button").click();

    // -----------------------------------------------------------------
    // Step 3 — capture the loop_id and wait for awaiting_approval.
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

    // The kanban groups child loops under the dispatch-root task; child
    // awaiting_approval cascades the parent to the needs_you column.
    // (The StateBadge data-state stays on the primary loop's state.)
    await expect(
      page.locator("[data-testid='task-card'][data-column='needs_you']"),
    ).toBeVisible({ timeout: 30000 });

    // -----------------------------------------------------------------
    // Step 4 — open the detail panel via URL state (the canonical
    // selection path; taskCard.click races replaceState). Re-assert
    // the SSE connection after the navigation: the goto reloads the
    // page, dropping the activity stream — without a fresh
    // healthy-status assertion the detail panel can render with stale
    // pending_approval state.
    // -----------------------------------------------------------------
    await page.goto(`/?task=${loopId}`);
    await expect(page.getByTestId("connection-status")).toHaveAttribute(
      "data-summary",
      "healthy",
      { timeout: 10000 },
    );
    await expect(page.getByTestId("task-detail-panel")).toBeVisible();

    // -----------------------------------------------------------------
    // Step 5 — PendingApprovalSection renders the gated tool with the
    // url visible in the formatted args. Tool name surfaces as
    // `add_source_repo`.
    // -----------------------------------------------------------------
    await expect(page.getByTestId("pending-approval-section")).toBeVisible();
    await expect(page.getByTestId("approval-tool-name")).toHaveText(
      "add_source_repo",
    );
    await expect(page.getByTestId("approval-args-display")).toContainText(
      "sensorhub-tools/osh-core",
    );

    // -----------------------------------------------------------------
    // Step 6 — approve. The component POSTs to
    // /teams-dispatch/loops/{id}/approval; dispatch publishes the
    // ApprovalResponse; agentic-loop re-dispatches the tool with
    // ApprovedBy stamped; the tool publishes AddRequest to
    // graph.ingest.add.research; SemSource replies; the agent receives
    // the success result and the next mock-llm turn emits a
    // completion message that terminates the loop.
    // -----------------------------------------------------------------
    await page.getByTestId("approval-approve").click();

    // -----------------------------------------------------------------
    // Step 7 — wait for the loop to reach terminal state. Use
    // page.locator on the kanban column rather than role-based
    // matching since the canonical signal is the card's data-column
    // attribute.
    // -----------------------------------------------------------------
    await expect(
      page.locator("[data-testid='task-card'][data-column='done']"),
    ).toBeVisible({ timeout: 30000 });

    const finalLoop = await request
      .get(`/teams-dispatch/loops/${loopId}`)
      .then((r) => r.json());
    // Upstream LoopStateComplete = "complete" — the only legitimate
    // terminal-state literal at the loop level. "success" is an
    // outcome alias, not a state, and was a defensive fallback in
    // earlier specs; tighten here so a regression that lands a
    // different state literal doesn't slip through.
    expect(finalLoop.state).toBe("complete");
  });
});

async function pollUntil<T>(
  check: () => Promise<T | null>,
  options: { timeoutMs?: number; intervalMs?: number } = {},
): Promise<T | null> {
  const timeout = options.timeoutMs ?? 30000;
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
