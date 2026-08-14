import { test, expect } from "@playwright/test";

/**
 * Journey: emitted research artifact -> coordinator-routed follow-up prompt.
 *
 * This is intentionally a UI handoff journey, not another routing matrix.
 * It proves that a human can reach a real emitted research artifact from the
 * task detail panel, attach it to any public team starter, edit the prompt,
 * and send the combined prompt through the coordinator front door.
 *
 * Required fixture: test/fixtures/journeys/research-mvp.yaml
 * Required config: configs/e2e-flow-bootstrap.json
 *
 * Run via:
 *   task ui:test:e2e:agentic:artifact-context-handoff
 */

// PARKED at beta.160 (ADR-059 decision 7): this journey's entire user
// story — ArtifactCard title/content, the handoff panel, the
// use-in-team buttons, the artifact context chip — renders from
// tool_arguments, which left the trajectory wire (facts carry previews
// + StorageReferences, not bodies). The flow is a REAL product
// casualty of the accepted regression, not a spec drift: un-skip when
// the evidence-fetch pass dereferences the fact's StorageReference and
// ArtifactCard renders again. Restore assertions as-is — they pin the
// correct UX.
test.describe.skip("Artifact context handoff", () => {
  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok(), "Backend not healthy - stack not running?").toBe(true);
  });

  test.setTimeout(180_000);

  test("research artifact can seed an editable coordinator-routed team prompt", async ({
    page,
    request,
  }) => {
    await page.goto("/");
    await expect(page.getByTestId("connection-status")).toHaveAttribute(
      "data-summary",
      "healthy",
      { timeout: 15_000 },
    );
    await expect(page.getByTestId("kanban-board")).toBeVisible();

    await page
      .getByTestId("chat-input")
      .fill(
        "Compare MQTT vs NATS for IoT edge deployments - which has lower latency on constrained ARM devices?",
      );
    await page.getByTestId("send-button").click();

    const loopRefs = await waitForResearchArtifactLoop(request);
    expect(
      loopRefs,
      "expected the research synthesize loop to complete with derivable plan + parent task ids",
    ).toBeTruthy();
    const { parentId, synthesizeId } = loopRefs!;

    const parentCard = page.locator(
      `[data-testid='task-card'][data-task-id='${parentId}']`,
    );
    await expect(
      parentCard,
      `parent task-card for ${parentId} did not appear via SSE`,
    ).toBeVisible({ timeout: 30_000 });

    await page.goto(`/?task=${parentId}`);
    await expect(page.getByTestId("task-detail-panel")).toBeVisible({
      timeout: 10_000,
    });

    const synthesizeChild = page.locator(
      `[data-testid='child-item'][data-loop-id='${synthesizeId}']`,
    );
    await expect(
      synthesizeChild,
      "expected descendant synthesize loop to be reachable from the parent task detail panel",
    ).toBeVisible({ timeout: 10_000 });
    await expect(synthesizeChild).toContainText(
      "researcher-research-synthesize",
    );

    await synthesizeChild.click();
    await expect(synthesizeChild).toHaveAttribute("aria-pressed", "true");
    await expect(page.getByTestId("focus-breadcrumb")).toContainText(
      "researcher-research-synthesize",
    );

    const emitResearchStep = page
      .getByTestId("story-step")
      .filter({ hasText: /used emit_research_artifact/ })
      .first();
    await expect(
      emitResearchStep,
      "expected the synthesize story to include the emitted research artifact",
    ).toBeVisible({ timeout: 15_000 });
    await emitResearchStep.click();

    await expect(page.getByTestId("artifact-card")).toBeVisible();
    await expect(page.getByTestId("artifact-tool-name")).toHaveText(
      "emit_research_artifact",
    );
    await expect(page.getByTestId("artifact-title")).toHaveText(
      "MQTT vs NATS for IoT edge",
    );

    await expect(page.getByTestId("artifact-handoff-panel")).toBeVisible();
    await expect(page.getByTestId("artifact-copy")).toBeVisible();
    await expect(page.getByTestId("artifact-use-research")).toBeVisible();
    await expect(page.getByTestId("artifact-use-spec")).toBeVisible();
    await expect(page.getByTestId("artifact-use-optimize")).toBeVisible();
    await expect(page.getByTestId("artifact-use-build")).toBeVisible();

    const seededPrompt =
      "/create-change Use the attached artifact as context to ";
    const chatInput = page.getByTestId("chat-input");

    await page.getByTestId("artifact-use-spec").click();
    await expect(chatInput).toHaveValue(seededPrompt);
    await expect(page.getByTestId("artifact-context-chip")).toContainText(
      "MQTT vs NATS for IoT edge",
    );

    await page.getByLabel("Remove artifact context").click();
    await expect(page.getByTestId("artifact-context-chip")).not.toBeVisible();
    await expect(chatInput).toHaveValue(seededPrompt);

    await page.getByTestId("artifact-use-spec").click();
    await expect(page.getByTestId("artifact-context-chip")).toContainText(
      "MQTT vs NATS for IoT edge",
    );
    await chatInput.fill(
      `${seededPrompt}draft an OpenSpec change for selecting an edge pub/sub broker policy.`,
    );

    let capturedContent = "";
    await page.route("**/teams-dispatch/message", async (route) => {
      const body = JSON.parse(route.request().postData() ?? "{}") as {
        content?: string;
      };
      capturedContent = body.content ?? "";
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ content: "accepted" }),
      });
    });

    const followupRequest = page.waitForRequest(
      (req) =>
        req.method() === "POST" &&
        req.url().includes("/teams-dispatch/message") &&
        (req.postData() ?? "").includes("/create-change Use"),
    );
    await page.getByTestId("send-button").click();
    await followupRequest;

    expect(capturedContent).toContain(
      "/create-change Use the attached artifact as context to draft an OpenSpec change",
    );
    expect(capturedContent).toContain(
      "Artifact context: MQTT vs NATS for IoT edge",
    );
    expect(capturedContent).toContain("Artifact tool: emit_research_artifact");
    expect(capturedContent).toContain(
      "Choose MQTT for sub-millisecond latency budgets",
    );
    expect(capturedContent).toContain(
      "Choose NATS when the workload mixes pub/sub + request-reply",
    );
    await expect(page.getByTestId("artifact-context-chip")).not.toBeVisible();
  });
});

interface LoopSummary {
  loop_id: string;
  role: string;
  state: string;
  task_id: string;
}

async function waitForResearchArtifactLoop(
  request: import("@playwright/test").APIRequestContext,
): Promise<{ parentId: string; synthesizeId: string } | null> {
  return pollUntil(
    async () => {
      const resp = await request.get("/teams-dispatch/loops");
      if (!resp.ok()) return null;
      const loops = (await resp.json()) as LoopSummary[];
      const synthesize = loops.find(
        (loop) =>
          loop.role === "researcher-research-synthesize" &&
          loop.state === "complete",
      );
      if (!synthesize) return null;

      const planId = parentFromTaskId(synthesize.task_id);
      if (!planId) return null;
      const plan = loops.find((loop) => loop.loop_id === planId);
      if (!plan) return null;

      const parentId = parentFromTaskId(plan.task_id);
      if (!parentId) return null;
      return { parentId, synthesizeId: synthesize.loop_id };
    },
    { timeoutMs: 120_000 },
  );
}

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
