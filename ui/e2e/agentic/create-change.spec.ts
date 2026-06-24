import { test, expect } from "@playwright/test";

/**
 * Journey: ADR-057 §D5 create_change pack (author / reviewer) mock-LLM
 * journey. The first end-to-end coverage of the create-change journey —
 * validates the orchestration through the e2e substrate:
 *
 *   coordinator(dispatch) → decide(create_change)  [rule 01 → author]
 *   author-create-change → emit_change (real executor stamps
 *     change.add-mfa.*) → decide(drafted)          [rule 02 → reviewer]
 *   reviewer-create-change → decide(approved)
 *     [rule 03 → stamp agent.run.outcome=success + coordinator wake-up]
 *   coordinator(wake-up) → decide(respond_direct)  → user.response.*
 *
 * The load-bearing assertions: the author loop spawns and emit_change
 * STAMPS a schema-valid change.add-mfa.* on the run entity (the executor
 * rejects a non-compliant payload — TestCreateChangeFixtureEmitChangeValid
 * pins the payload shape at `go test` time; this proves it ran live in the
 * chain), the reviewer gate spawns + approves, the run reaches `completed`,
 * and the chain terminates with a delivered reply. A regression that drops
 * a rule, mis-routes a token, or breaks the emit_change run-anchor wiring
 * fails here.
 *
 * Required fixture: test/fixtures/journeys/create-change.yaml
 * Required config:  configs/e2e-flow-bootstrap.json (create-change rules
 *   wired into rule-processor rules_files).
 * No sandbox: the author authors greenfield (its bash tool is unused on
 *   this fixture), so no SEMTEAMS_SANDBOX_RUNNER is needed.
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

test.describe("ADR-057 — create_change pack mock-LLM journey", () => {
  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok(), "Backend not healthy — stack not running?").toBe(true);
  });

  // 4-loop chain (coordinator dispatch + author + reviewer + coordinator
  // wake-up) on the mock-LLM. Generous budget for mock scheduling.
  test.setTimeout(90_000);

  test("spec-authoring prompt → author emits change → reviewer approves → reply", async ({
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

    // A spec-authoring ask — the coordinator's decision-contract routes
    // this to create_change (the deliverable is the spec, not running code).
    await page.getByTestId("chat-input").fill(
      "Draft a spec change to add multi-factor authentication (TOTP) to our auth system: users enroll an authenticator, and login requires a valid code for MFA-enabled accounts.",
    );
    await page.getByTestId("send-button").click();

    // Poll until the chain has settled: author + reviewer present, every
    // loop terminal, and the final respond_direct delivered (the wake-up
    // lags the reviewer's approval by a beat).
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
      "expected the create-change chain to settle with author + reviewer, all terminal",
    ).toBeTruthy();
    const settled = loops!;

    // No loop failed — the whole chain converged.
    const failed = settled.filter((l) =>
      ["failed", "error", "cancelled", "truncated"].includes(l.state ?? ""),
    );
    expect(
      failed.map((l) => `${l.role}:${l.state}`),
      "no create-change loop should have failed",
    ).toEqual([]);

    // Role distribution: exactly the pack's shape — 1 author, 1 reviewer,
    // ≥1 coordinator wake-up (the front-door dispatch loop's role is
    // omitempty per feedback_loopinfo_role_omitempty, so floor at 1 to
    // catch a missing wake-up).
    const byRole = (r: string) => settled.filter((l) => l.role === r).length;
    expect(byRole("author-create-change"), "one author").toBe(1);
    expect(byRole("reviewer-create-change"), "one reviewer gate").toBe(1);
    expect(
      byRole("coordinator"),
      "≥1 coordinator wake-up (rule 03 → respond_direct)",
    ).toBeGreaterThanOrEqual(1);
    const authorLoop = settled.find((l) => l.role === "author-create-change");
    const authorLoopId = String(authorLoop?.loop_id ?? "");
    expect(authorLoopId, "expected an author-create-change loop id").not.toBe(
      "",
    );

    // The chain flowed through the pack's distinct tokens.
    const actionTriples = await fetchTriples(request, {
      predicate: "coordinator.decision.next_action",
      limit: 50,
    });
    const actions = new Set(actionTriples.map((t) => String(t.object ?? "")));
    for (const want of [
      "create_change", // front-door dispatch
      "drafted", // author terminal
      "approved", // reviewer gate verdict
      "respond_direct", // final delivery
    ]) {
      expect(
        actions.has(want),
        `expected a coordinator.decision.next_action=${want} triple — saw: ${[...actions].sort().join(", ")}`,
      ).toBe(true);
    }

    // The decisive proof: emit_change ran live in the author loop and
    // stamped a schema-valid change on the run entity. change.add-mfa.* is
    // stamped ONLY by emit_change (the executor rejects a non-compliant
    // payload), so its presence proves the author→tool→graph path worked.
    const changeIntent = await fetchTriples(request, {
      predicate: "change.add-mfa.proposal.intent",
      limit: 5,
    });
    expect(
      changeIntent.length,
      "expected a change.add-mfa.proposal.intent triple — emit_change must have stamped the change on the run entity",
    ).toBeGreaterThanOrEqual(1);
    // Stamped on the run entity (agent.chain.execution.<runID>), not a loop.
    const changeSubject = String(changeIntent[0]?.subject ?? "");
    expect(changeSubject, "change.* must land on the run entity").toContain(
      "agent.chain.execution.",
    );
    expect(
      changeSubject,
      "change.* must land on the run entity, not a loop entity",
    ).not.toContain("agent.loop.");

    // Terminal: rule 03b published the user reply on respond_direct.
    const resp = await request.get(
      "/message-logger/entries?subject_prefix=dispatch.user.response&limit=10",
    );
    expect(resp.ok(), "/message-logger/entries non-OK").toBe(true);
    const payloads = (await resp.json()) as Array<{ subject: string }>;
    expect(
      payloads.length,
      "expected a user.response.* publish (coordinator wake-up respond_direct)",
    ).toBeGreaterThanOrEqual(1);

    // ADR-053 Phase 4a — the run reached `completed`. Rule 03 stamps
    // agent.run.outcome=success on the run entity, and agent-run/03
    // transitions executing→completed. `failed` here = the D3 zombie guard
    // raced, or a pack loop failed.
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
    expect(
      runPhases,
      "agent.run.phase never reached a terminal on the run entity (run stuck in dispatched/executing)",
    ).toBeTruthy();
    expect(runPhases, "run must reach completed (ADR-053 Phase 4a)").toContain(
      "completed",
    );
    expect(
      runPhases,
      "run must NOT be failed (D3 race / wrong terminal)",
    ).not.toContain("failed");

    // -----------------------------------------------------------------
    // UI coverage for OpenSpec review/export + run health (OpenSpec change
    // 6.2). The backend assertions above prove the chain ran; this drives
    // the operator surface that answers "is this thing working?" and lets a
    // human review/edit/export the generated spec before it is handed off.
    // -----------------------------------------------------------------
    const runId = bareIdAfter(changeSubject, RUN_INFIX);
    expect(
      runId,
      `could not extract the front-door run id from ${changeSubject}`,
    ).toBeTruthy();

    const runCard = page.locator(
      `[data-testid='task-card'][data-task-id='${runId}']`,
    );
    await expect(
      runCard,
      `run card ${runId} did not render in the board after create-change settled`,
    ).toBeVisible({ timeout: 30_000 });

    await page.goto(`/?task=${runId}`);
    await expect(page.getByTestId("task-detail-panel")).toBeVisible({
      timeout: 10_000,
    });

    const healthPanel = page.getByTestId("run-health-panel");
    await expect(
      healthPanel,
      "run health summary did not render in the detail panel",
    ).toBeVisible({ timeout: 30_000 });
    await expect(healthPanel).toHaveAttribute(
      "aria-label",
      /Run health: (Working|Complete)/,
    );
    await expect(healthPanel).toContainText("Next");
    await expect(healthPanel).toContainText("Evidence");
    await expect(healthPanel).toContainText("Active loops");

    const authorChild = page.locator(
      `[data-testid='child-item'][data-loop-id='${authorLoopId}']`,
    );
    await expect(
      authorChild,
      `author child row ${authorLoopId} did not appear under the run card`,
    ).toBeVisible({ timeout: 10_000 });
    await authorChild.click();

    const emitChangeStep = page
      .getByTestId("story-step")
      .filter({ hasText: /used emit_change/ })
      .first();
    await expect(
      emitChangeStep,
      "expected the author trajectory to include the live emit_change call",
    ).toBeVisible({ timeout: 15_000 });
    await emitChangeStep.click();

    await expect(page.getByTestId("artifact-card")).toBeVisible();
    await expect(page.getByTestId("artifact-tool-name")).toHaveText(
      "emit_change",
    );
    await expect(page.getByTestId("openspec-review-panel")).toBeVisible();
    await expect(page.getByTestId("openspec-title")).toHaveText("add-mfa");
    await expect(page.getByTestId("openspec-preview")).toContainText(
      "# OpenSpec change: add-mfa",
    );
    await expect(page.getByTestId("openspec-preview")).toContainText(
      "<!-- openspec/changes/add-mfa/specs/auth/spec.md -->",
    );

    await page.getByTestId("openspec-edit").click();
    await expect(page.getByTestId("openspec-editor")).toHaveValue(
      /# OpenSpec change: add-mfa/,
    );
    await page
      .getByTestId("openspec-editor")
      .fill("# Edited OpenSpec\n\nManual reviewer note.\n");
    await page.getByTestId("openspec-save-edit").click();
    await expect(page.getByTestId("openspec-export-state")).toHaveText(
      "Draft updated",
    );
    await expect(page.getByTestId("openspec-preview")).toContainText(
      "Manual reviewer note.",
    );

    await page.getByTestId("openspec-approve").click();
    await expect(page.getByTestId("openspec-review-state")).toHaveAttribute(
      "data-state",
      "approved",
    );

    const docDownload = page.waitForEvent("download");
    await page.getByTestId("openspec-download").click();
    expect((await docDownload).suggestedFilename()).toBe("add-mfa.md");
    await expect(page.getByTestId("openspec-export-state")).toHaveText(
      "Document downloaded",
    );

    const folderDownload = page.waitForEvent("download");
    await page.getByTestId("openspec-download-folder").click();
    expect((await folderDownload).suggestedFilename()).toBe(
      "add-mfa.openspec-folder.json",
    );
    await expect(page.getByTestId("openspec-export-state")).toHaveText(
      "Folder manifest downloaded",
    );
  });
});

// NOTE: pollUntil/fetchTriples + the Loop/Triple/TERMINAL shapes are
// duplicated across sandbox-mvp/dev-via-test/autoresearch/create-change
// specs. Follow-up: extract to ui/e2e/helpers/agentic.ts (go-reviewer
// 2026-06-06 flagged the same on dev-via-test).
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

/** Extract the bare id after a fixed entity-ID infix. */
function bareIdAfter(entityId: string, infix: string): string {
  const idx = entityId.indexOf(infix);
  return idx === -1 ? "" : entityId.slice(idx + infix.length);
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
