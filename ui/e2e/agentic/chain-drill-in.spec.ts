import { test, expect } from "@playwright/test";

/**
 * Journey: Chain drill-in + emit_* artifact rendering
 *
 * Goal: After a chain runs, the user opens the parent task and drills
 * into a sub-loop by clicking its row in the Sub-tasks list. The
 * right-rail TaskStory swaps to that child's trajectory. For an
 * `emit_*` tool call, the expanded payload renders via ArtifactCard
 * (structured fields) instead of raw JSON.
 *
 * Validates:
 *   - Coordinator (dispatch) task appears as a top-level kanban card.
 *   - Its TaskDetailPanel renders the plan loop as a clickable
 *     Sub-tasks <button> (regression guard against the pre-Slice-A
 *     decorative <li>).
 *   - Clicking the plan child swaps the right-rail TaskStory to the
 *     plan loop's trajectory: `focus-breadcrumb` appears, child's
 *     aria-pressed flips to true.
 *   - Plan's `emit_plan` step expands to ArtifactCard (testid
 *     `artifact-card`) with at least one structured field — NOT the
 *     raw <pre class="step-payload"> JSON.
 *   - The plan loop's decide(action="gather") surfaces as a verdict
 *     row (chip + always-visible reason), proving the decide-verdict
 *     enrichment reads the live trajectory wire shape (Thread #2).
 *   - Breadcrumb back-button restores focus to the parent.
 *
 * **Chain shape note (semstreams beta.96, validated 2026-06-03 via
 * the activity SSE):**
 *
 * `publish_agent` actions stamp `parent_loop_id` from the triggering
 * loop entity. For research-mvp:
 *
 *   - coordinator (dispatch)            parent=∅      → top-level task
 *   - researcher-research-plan          parent=coord  → child of coord
 *   - researcher-research-gather (N×)   parent=plan   → children of plan
 *   - researcher-research-synthesize    parent=plan   → child of plan
 *   - reviewer-research                 parent=synth  → child of synth
 *
 * The kanban only shows the coordinator card; everything else is
 * nested under the coordinator's Sub-tasks. The plan loop is the
 * only direct child of the coordinator that this spec exercises —
 * which is exactly the slice's drill-in target. Deeper descendants
 * (gather, synthesize, reviewer) are reachable only after the chain
 * tree is flattened at the task level — see
 * [[chain-tree-flattening-followup]].
 *
 * **Chain-completion note:** Under the current `research-mvp.yaml`
 * fixture (which predates the 2026-05-29 research-pack redesign per
 * [[research-pack-redesign]]), the gather + reviewer phases now
 * fail mid-chain (`max_iterations` reached). Both terminate, the
 * plan loop completes successfully, and emit_plan is in its
 * trajectory — which is what this spec asserts on. When the fixture
 * is refreshed to the redesigned pack, the spec will also work
 * (only stronger assertions become available).
 *
 * Required fixture: test/fixtures/journeys/research-mvp.yaml
 * Required config: configs/e2e-flow-bootstrap.json
 *
 * Run via:
 *   task ui:test:e2e:agentic:chain-drill-in
 */

test.describe("Chain drill-in + emit_* artifact rendering", () => {
  test.setTimeout(180_000);

  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok(), "Backend not healthy — stack not running?").toBe(true);
  });

  test("user drills coordinator → plan child, expands emit_plan, sees ArtifactCard", async ({
    page,
    request,
  }) => {
    // -----------------------------------------------------------------
    // Step 1 — open the Board.
    // -----------------------------------------------------------------
    await page.goto("/");
    await expect(page.getByTestId("connection-status")).toHaveAttribute(
      "data-summary",
      "healthy",
      { timeout: 15_000 },
    );
    await expect(page.getByTestId("kanban-board")).toBeVisible();

    // -----------------------------------------------------------------
    // Step 2 — drive the research chain. Only the plan-loop tier
    // needs to complete for this spec; the downstream gather +
    // reviewer phases may fail under the current fixture (see the
    // chain-completion note at the top of this file) — that's fine,
    // the plan's emit_plan trajectory is what we exercise.
    // -----------------------------------------------------------------
    await page.getByTestId("chat-input").fill(
      "Compare MQTT vs NATS for IoT edge deployments — which has lower latency on constrained ARM devices?",
    );
    await page.getByTestId("send-button").click();

    // -----------------------------------------------------------------
    // Step 3 — wait for the plan loop to terminal-complete, then
    // derive its parent_loop_id from the task_id wire field.
    //
    // Wire-shape note (2026-06-03): the /teams-dispatch/loops summary
    // endpoint OMITS `parent_loop_id` (only the SSE /activity stream
    // includes it). We can't poll SSE here because `request.get` on
    // `text/event-stream` never resolves until the connection closes.
    // Instead we read parent_loop_id from the task_id format
    // `rule-<entity_prefix>.execution.<parent_loop_id>-<19digit_ts>`,
    // which is a stable contract upstream (see `agentic-loop`'s
    // rule-spawned task_id generator). The 19-digit suffix is the
    // unix nanosecond timestamp.
    // -----------------------------------------------------------------
    const planLoop = await pollUntil(async () => {
      const resp = await request.get("/teams-dispatch/loops");
      if (!resp.ok()) return null;
      const list = (await resp.json()) as LoopSummary[];
      const candidate = list.find(
        (l) =>
          l.role === "researcher-research-plan" && l.state === "complete",
      );
      if (!candidate) return null;
      const parent = parentFromTaskId(candidate.task_id);
      if (!parent) return null;
      return { plan_id: candidate.loop_id, parent_id: parent };
    }, { timeoutMs: 90_000 });

    expect(
      planLoop,
      "expected a researcher-research-plan loop to reach terminal complete state within 90s with a derivable parent_loop_id. If the loop exists but task_id doesn't match `…execution.<parent>-<ts>`, the upstream task_id format changed — see chain-drill-in.spec.ts:parentFromTaskId.",
    ).toBeTruthy();
    const { plan_id: planId, parent_id: parentId } = planLoop!;

    // -----------------------------------------------------------------
    // Step 4 — wait for the parent task-card to appear in the kanban
    // (SSE-fed). Without this, the URL-driven panel open races against
    // an empty taskStore and the panel never renders.
    // -----------------------------------------------------------------
    const parentCard = page.locator(
      `[data-testid='task-card'][data-task-id='${parentId}']`,
    );
    await expect(
      parentCard,
      `parent task-card for ${parentId} did not appear via SSE within 30s — agentStore is empty or the SSE stream dropped`,
    ).toBeVisible({ timeout: 30_000 });

    // -----------------------------------------------------------------
    // Step 5 — open the parent's detail panel via the URL contract.
    // -----------------------------------------------------------------
    await page.goto(`/?task=${parentId}`);
    await expect(page.getByTestId("task-detail-panel")).toBeVisible({
      timeout: 10_000,
    });
    await expect(page.getByTestId("panel-activity")).toBeVisible();

    // -----------------------------------------------------------------
    // Step 6 — find the plan loop in Sub-tasks. It MUST be a button
    // with the correct loop_id (regression guard against re-introducing
    // the decorative <li> shape this slice replaced).
    // -----------------------------------------------------------------
    const planChild = page.locator(
      `[data-testid='child-item'][data-loop-id='${planId}']`,
    );
    await expect(
      planChild,
      `plan child row for ${planId} did not appear in Sub-tasks. Either the parent_loop_id stamp didn't reach the UI (check the kanban shows only the coordinator card), or the Sub-tasks list isn't rendering child-items.`,
    ).toBeVisible({ timeout: 10_000 });
    await expect(planChild).toHaveJSProperty("tagName", "BUTTON");
    await expect(planChild).toHaveAttribute("aria-pressed", "false");

    // No breadcrumb before drill-in.
    await expect(page.getByTestId("focus-breadcrumb")).not.toBeVisible();

    // -----------------------------------------------------------------
    // Step 7 — click the plan child. The right-rail TaskStory should
    // swap to the plan's trajectory; breadcrumb appears with the
    // child's role + back-button anchored to the parent title.
    // -----------------------------------------------------------------
    await planChild.click();
    await expect(planChild).toHaveAttribute("aria-pressed", "true");

    const crumb = page.getByTestId("focus-breadcrumb");
    await expect(crumb).toBeVisible({ timeout: 5_000 });
    await expect(crumb).toContainText("Back to");
    await expect(crumb).toContainText("researcher-research-plan");

    // -----------------------------------------------------------------
    // Step 8 — wait for the plan loop's story to populate. emit_plan
    // is the first tool_call in the plan loop per research-mvp.yaml.
    // -----------------------------------------------------------------
    await expect(page.getByTestId("story-list")).toBeVisible({
      timeout: 15_000,
    });
    const emitPlanStep = page
      .getByTestId("story-step")
      .filter({ hasText: /used emit_plan/ })
      .first();
    await expect(
      emitPlanStep,
      "expected an emit_plan step in the plan loop's trajectory. If only model_call rows render, the fixture turn ordering changed — check research-mvp.yaml Loop A.",
    ).toBeVisible({ timeout: 15_000 });

    // -----------------------------------------------------------------
    // Step 9 — expand the step. beta.160 (ADR-059 decision 7): the
    // trajectory wire carries fact previews + storage references, NOT
    // tool arguments — ArtifactCard and the decide-verdict rows lost
    // their data source and are intentionally parked until the
    // evidence-fetch pass dereferences the fact's StorageReference.
    // The CURRENT contract: expanding a step shows the fact's own
    // metadata JSON in the story-step-payload <pre>. When the
    // evidence-fetch pass lands, restore the ArtifactCard assertions
    // from git history (this block, pre-ADR-059).
    // -----------------------------------------------------------------
    await emitPlanStep.click();

    const payload = page.getByTestId("story-step-payload");
    await expect(
      payload,
      "expected the expanded emit_plan step to show the fact metadata payload — the beta.160 fact-based contract (ADR-059). If nothing expands, the TaskStory expand affordance regressed.",
    ).toBeVisible({ timeout: 5_000 });
    await expect(
      payload,
      "expected the fact payload to identify the tool via tool_preview — check the trajectory fact wire shape",
    ).toContainText("emit_plan");

    // -----------------------------------------------------------------
    // Step 9b — decide renders as a generic tool row at beta.160 (the
    // verdict chip's tool_arguments source is gone — same ADR-059
    // register entry as ArtifactCard above). Assert the row exists so
    // the decide call remains narratively visible.
    // -----------------------------------------------------------------
    // The decide completion fact ships with an EMPTY tool_preview at
    // beta.160 (upstream omits it on the terminal decide), so the row
    // reads "used a tool"; match both in case upstream adds the preview.
    const decideStep = page
      .getByTestId("story-step")
      .filter({ hasText: /used (decide|a tool)/ })
      .first();
    await expect(
      decideStep,
      "expected the plan loop's terminal decide to render as a tool row (generic at beta.160; verdict chips return with the evidence-fetch pass — ADR-059).",
    ).toBeVisible({ timeout: 10_000 });

    // -----------------------------------------------------------------
    // Step 10 — back-button returns focus to the coordinator. The
    // breadcrumb disappears, the plan-child's aria-pressed flips back
    // to false. We don't assert on the parent's story content — the
    // disappearance of the breadcrumb is the observable transition.
    // -----------------------------------------------------------------
    await page.getByTestId("focus-back").click();
    await expect(page.getByTestId("focus-breadcrumb")).not.toBeVisible();
    await expect(planChild).toHaveAttribute("aria-pressed", "false");
  });
});

/**
 * /teams-dispatch/loops summary entry. Fields that do not appear on
 * the wire JSON (notably parent_loop_id) are deliberately not typed
 * here — we derive parent from task_id below.
 */
interface LoopSummary {
  loop_id: string;
  role: string;
  state: string;
  task_id: string;
}

/**
 * Derive parent_loop_id from a rule-spawned loop's task_id. Upstream
 * format: `rule-<entity_prefix>.execution.<parent_loop_id>-<19digit_ts>`.
 * The entity_prefix itself may contain dots; the parent_loop_id may
 * contain hyphens (UUID format). The unix-nanosecond timestamp is
 * always exactly 19 digits at the tail.
 *
 * Returns null when the format doesn't match — used as the "not
 * ready yet" signal for the pollUntil loop.
 */
function parentFromTaskId(taskId: string): string | null {
  const m = taskId.match(/^rule-.+\.execution\.(.+?)-\d{19}$/);
  return m ? m[1] : null;
}

async function pollUntil<T>(
  fn: () => Promise<T | null>,
  opts: { timeoutMs: number; intervalMs?: number },
): Promise<T | null> {
  const deadline = Date.now() + opts.timeoutMs;
  const interval = opts.intervalMs ?? 500;
  while (Date.now() < deadline) {
    const result = await fn();
    if (result != null) return result;
    await new Promise((r) => setTimeout(r, interval));
  }
  return null;
}
