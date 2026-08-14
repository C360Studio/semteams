import { test, expect } from "@playwright/test";
import { attachJourneyEvidenceReport } from "./e2e_report";

/**
 * Journey: MAVLink-hard OpenSpec production.
 *
 * This is a black-box demo-MVP journey. It drives SemTeams through the public
 * UI chat path, then reads loops, graph facts, UI state, and downloads for
 * assertions. It does not seed NATS, graph storage, lifecycle state, proof
 * facts, or private KV buckets.
 */

interface Loop {
  loop_id?: string;
  role?: string | null;
  state?: string;
}

interface Triple {
  subject?: string;
  predicate?: string;
  object?: unknown;
}

const TERMINAL = new Set([
  "complete",
  "success",
  "failed",
  "error",
  "cancelled",
  "truncated",
]);
const RUN_INFIX = ".agent.chain.execution.";
const SLUG = "mavlink-hard-osh-mavsdk-csapi";

// PARKED (ADR-058): this journey exercises a dev-side pack that is unwired
// from the bootstrap pending the canonical-predicate migration. Re-enable by
// restoring test.describe when the pack is re-authored and re-wired.
test.describe.skip("MAVLink-hard OpenSpec handoff", () => {
  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok(), "Backend not healthy; stack not running?").toBe(true);
  });

  test.setTimeout(90_000);

  test("mavlink-hard prompt produces a reviewed and exportable OpenSpec handoff", async ({
    page,
    request,
  }, testInfo) => {
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
        [
          "SemSpec mavlink-hard handoff: create an OpenSpec change for extending the OpenSensorHub MAVSDK addon",
          "so a PX4/MAVLink vehicle is exposed through OGC Connected Systems API.",
          "The spec must name harness/readiness dependencies before implementation; do not implement code yet.",
        ].join(" "),
      );
    await page.getByTestId("send-button").click();

    const loops = await pollUntil(
      async () => {
        const resp = await request.get("/teams-dispatch/loops");
        if (!resp.ok()) return null;
        const list = (await resp.json()) as Loop[];
        const author = list.filter((l) => l.role === "author-create-change");
        const reviewer = list.filter(
          (l) => l.role === "reviewer-create-change",
        );
        const allTerminal = list.every((l) => TERMINAL.has(l.state ?? ""));
        if (author.length >= 1 && reviewer.length >= 1 && allTerminal) {
          const acts = await fetchTriples(request, {
            predicate: "coordinator.decision.next_action",
            limit: 50,
          });
          if (acts.some((t) => String(t.object ?? "") === "respond_direct")) {
            return list;
          }
        }
        return null;
      },
      { timeoutMs: 70_000 },
    );

    expect(
      loops,
      "expected the create-change chain to settle for MAVLink-hard",
    ).toBeTruthy();
    const settled = loops!;
    const failed = settled.filter((l) =>
      ["failed", "error", "cancelled", "truncated"].includes(l.state ?? ""),
    );
    expect(
      failed.map((l) => `${l.role}:${l.state}`),
      "no loop should fail while producing the spec handoff",
    ).toEqual([]);

    const byRole = (r: string) => settled.filter((l) => l.role === r).length;
    expect(byRole("author-create-change"), "one author").toBe(1);
    expect(byRole("reviewer-create-change"), "one reviewer").toBe(1);
    expect(byRole("coordinator"), "coordinator wake-up").toBeGreaterThanOrEqual(
      1,
    );

    const authorLoop = settled.find((l) => l.role === "author-create-change");
    const authorLoopId = String(authorLoop?.loop_id ?? "");
    expect(authorLoopId, "expected author loop id").not.toBe("");

    const actions = new Set(
      (
        await fetchTriples(request, {
          predicate: "coordinator.decision.next_action",
          limit: 50,
        })
      ).map((t) => String(t.object ?? "")),
    );
    for (const want of [
      "create_change",
      "drafted",
      "approved",
      "respond_direct",
    ]) {
      expect(
        actions.has(want),
        `expected coordinator.decision.next_action=${want}; saw ${[...actions].sort().join(", ")}`,
      ).toBe(true);
    }

    const changeIntent = await fetchTriples(request, {
      predicate: `change.${SLUG}.proposal.intent`,
      limit: 5,
    });
    expect(
      changeIntent.length,
      `expected change.${SLUG}.proposal.intent stamped by emit_change`,
    ).toBeGreaterThanOrEqual(1);
    const changeSubject = String(changeIntent[0]?.subject ?? "");
    expect(changeSubject).toContain("agent.chain.execution.");
    expect(changeSubject).not.toContain("agent.loop.");

    const acceptance = await fetchTriples(request, {
      predicate: `change.${SLUG}.acceptance_command`,
      limit: 5,
    });
    expect(acceptance.map((t) => String(t.object ?? ""))).toContain(
      "task harness:mavlink:smoke && ./gradlew :osh-addons:mavsdk:test",
    );

    const runPhases = await pollUntil(
      async () => {
        const phases = await fetchTriples(request, {
          predicate: "agent.run.phase",
          limit: 20,
        });
        const objs = phases
          .filter((t) =>
            String(t.subject ?? "").includes("agent.chain.execution."),
          )
          .map((t) => String(t.object));
        if (objs.includes("completed") || objs.includes("failed")) return objs;
        return null;
      },
      { timeoutMs: 30_000 },
    );
    expect(runPhases).toContain("completed");
    expect(runPhases).not.toContain("failed");

    const runId = bareIdAfter(changeSubject, RUN_INFIX);
    expect(
      runId,
      `could not extract run id from ${changeSubject}`,
    ).toBeTruthy();

    await page.goto(`/?task=${runId}`);
    await expect(page.getByTestId("task-detail-panel")).toBeVisible({
      timeout: 10_000,
    });

    const healthPanel = page.getByTestId("run-health-panel");
    await expect(healthPanel).toBeVisible({ timeout: 30_000 });
    await expect(healthPanel).toHaveAttribute(
      "aria-label",
      /Run health: (Working|Complete)/,
    );
    await expect(healthPanel).toContainText("Next");
    await expect(healthPanel).toContainText("Evidence");

    const authorChild = page.locator(
      `[data-testid='child-item'][data-loop-id='${authorLoopId}']`,
    );
    await expect(authorChild).toBeVisible({ timeout: 10_000 });
    await authorChild.click();

    const emitChangeStep = page
      .getByTestId("story-step")
      .filter({ hasText: /used emit_change/ })
      .first();
    await expect(emitChangeStep).toBeVisible({ timeout: 15_000 });
    await emitChangeStep.click();

    await expect(page.getByTestId("artifact-card")).toBeVisible();
    await expect(page.getByTestId("artifact-tool-name")).toHaveText(
      "emit_change",
    );
    await expect(page.getByTestId("openspec-review-panel")).toBeVisible();
    await expect(page.getByTestId("openspec-title")).toHaveText(SLUG);

    const preview = page.getByTestId("openspec-preview");
    await expect(preview).toContainText(`# OpenSpec change: ${SLUG}`);
    await expect(preview).toContainText("OpenSensorHub");
    await expect(preview).toContainText("MAVSDK");
    await expect(preview).toContainText("Connected Systems API");
    await expect(preview).toContainText("PX4 SITL");
    await expect(preview).toContainText("Harness readiness gate");
    await expect(preview).toContainText("full acceptance gate");
    await expect(preview).toContainText(
      "Code implementation before harness readiness",
    );

    await page.getByTestId("openspec-approve").click();
    await expect(page.getByTestId("openspec-review-state")).toHaveAttribute(
      "data-state",
      "approved",
    );
    await expect(
      page.getByTestId("openspec-implementation-handoff"),
    ).toContainText("Approved spec handoff");
    await expect(
      page.getByTestId("openspec-implementation-command"),
    ).toHaveText(`/implement-spec ${SLUG}`);

    const docDownload = page.waitForEvent("download");
    await page.getByTestId("openspec-download").click();
    const doc = await docDownload;
    expect(doc.suggestedFilename()).toBe(`${SLUG}.md`);

    const folderDownload = page.waitForEvent("download");
    await page.getByTestId("openspec-download-folder").click();
    const folder = await folderDownload;
    expect(folder.suggestedFilename()).toBe(`${SLUG}.openspec.zip`);

    const report = await attachJourneyEvidenceReport({
      journeyName: "mavlink-hard-spec",
      fixture: "mavlink-hard-spec.yaml",
      config: "e2e-flow-bootstrap.json",
      request,
      testInfo,
      runIds: [runId],
      runEntityIds: [changeSubject],
      observations: {
        evidenceTier: "black-box",
        authorLoopId,
        runPhases,
        slug: SLUG,
      },
      artifactOutputs: [
        {
          name: doc.suggestedFilename(),
          kind: "openspec-single-document",
          contentType: "text/markdown",
          path: await downloadPath(doc),
          metadata: { slug: SLUG, source: "openspec-download" },
        },
        {
          name: folder.suggestedFilename(),
          kind: "openspec-folder-archive",
          contentType: "application/zip",
          path: await downloadPath(folder),
          metadata: { slug: SLUG, source: "openspec-download-folder" },
        },
      ],
    });
    expect(
      report.artifacts.explicit_outputs.map((artifact) => artifact.kind),
    ).toEqual(
      expect.arrayContaining([
        "openspec-single-document",
        "openspec-folder-archive",
      ]),
    );
    expect(
      report.artifacts.tool_calls.some((call) => call.tool === "emit_change"),
      "journey report must include the live emit_change artifact call",
    ).toBe(true);
  });
});

async function fetchTriples(
  request: import("@playwright/test").APIRequestContext,
  params: { subject?: string; predicate?: string; limit?: number },
): Promise<Triple[]> {
  const query = new URLSearchParams();
  if (params.subject) query.set("subject", params.subject);
  if (params.predicate) query.set("predicate", params.predicate);
  if (params.limit) query.set("limit", String(params.limit));
  const resp = await request.get(`/graph/triples?${query.toString()}`);
  if (!resp.ok()) {
    throw new Error(
      `GET /graph/triples?${query.toString()} returned ${resp.status()}: ${await resp.text()}`,
    );
  }
  return (await resp.json()) as Triple[];
}

function bareIdAfter(entityId: string, infix: string): string {
  const idx = entityId.indexOf(infix);
  return idx === -1 ? "" : entityId.slice(idx + infix.length);
}

async function downloadPath(
  download: import("@playwright/test").Download,
): Promise<string | null> {
  try {
    return await download.path();
  } catch {
    return null;
  }
}

async function pollUntil<T>(
  fn: () => Promise<T | null>,
  opts: { timeoutMs: number; intervalMs?: number },
): Promise<T | null> {
  const deadline = Date.now() + opts.timeoutMs;
  const interval = opts.intervalMs ?? 300;
  while (Date.now() < deadline) {
    const result = await fn();
    if (result != null) return result;
    await new Promise((r) => setTimeout(r, interval));
  }
  return null;
}
