import { test, expect } from "@playwright/test";
import { spawn } from "node:child_process";
import { attachJourneyEvidenceReport } from "./e2e_report";

/**
 * Journey: approved OpenSpec artifact -> operator/proof gates ->
 * dev-from-task projection -> Ralph -> CBG.
 *
 * This intentionally uses a tiny one-task health endpoint spec. The
 * load-bearing proof is the routing and authority chain:
 *
 *   create_change author emits change.add-health-endpoint.*
 *   reviewer approves -> agent.run.outcome=success
 *   UI approval exposes /implement-spec add-health-endpoint
 *   test seeds only external proof.* inventory facts
 *   operator submits /implement-spec from the selected run
 *   command runs the proof analyzer and records the explicit handoff request
 *   proof-readiness rules stamp proof_readiness.implementation_ready
 *   dev-from-task/02 wakes coordinator
 *   project_spec_tasks stamps plan.* + plan.done_authority.*
 *   coordinator emits dev_via_test with subtopics=["task-0"]
 *   Ralph executes directly; Lisa is skipped
 *   CBG final gate approves
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
  source?: string;
  timestamp?: string;
  confidence?: number;
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

// PARKED (ADR-058): this journey exercises a dev-side pack that is unwired
// from the bootstrap pending the canonical-predicate migration. Re-enable by
// restoring test.describe when the pack is re-authored and re-wired.
test.describe.skip("OpenSpec -> dev-from-task -> dev-via-test demo", () => {
  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok(), "Backend not healthy — stack not running?").toBe(true);
  });

  test.setTimeout(140_000);

  test("approved spec handoff projects tasks and dispatches Ralph without Lisa", async ({
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
        "Draft an OpenSpec change for a tiny Go health endpoint: GET /health should return HTTP 200 with JSON status ok. Keep it to one implementation task with a focused Go test.",
      );
    await page.getByTestId("send-button").click();

    const createLoops = await pollUntil(
      async () => {
        const list = await fetchLoops(request);
        const author = list.filter((l) => l.role === "author-create-change");
        const reviewer = list.filter((l) => l.role === "reviewer-create-change");
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
    expect(createLoops, "create-change chain did not settle").toBeTruthy();

    const changeIntent = await fetchTriples(request, {
      predicate: "change.add-health-endpoint.proposal.intent",
      limit: 5,
    });
    expect(
      changeIntent.length,
      "emit_change must stamp change.add-health-endpoint.* on the run entity",
    ).toBeGreaterThanOrEqual(1);
    const runEntityId = String(changeIntent[0]?.subject ?? "");
    const runId = bareIdAfter(runEntityId, RUN_INFIX);
    expect(runId, `could not extract run id from ${runEntityId}`).toBeTruthy();
    await seedProofInventory(runEntityId);
    await expectEventuallyTriple(request, {
      subject: runEntityId,
      predicate: "proof.claim.health.endpoint.status",
      object: "accepted",
    });

    const authorLoop = createLoops!.find(
      (l) => l.role === "author-create-change",
    );
    const authorLoopId = String(authorLoop?.loop_id ?? "");
    expect(authorLoopId, "expected author-create-change loop id").not.toBe("");

    await page.goto(`/?task=${runId}`);
    await expect(page.getByTestId("task-detail-panel")).toBeVisible({
      timeout: 10_000,
    });
    const healthPanel = page.getByTestId("run-health-panel");
    await expect(healthPanel).toBeVisible({ timeout: 15_000 });

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

    await expect(page.getByTestId("openspec-review-panel")).toBeVisible();
    await expect(page.getByTestId("openspec-title")).toHaveText(
      "add-health-endpoint",
    );
    await page.getByTestId("openspec-approve").click();
    await expect(page.getByTestId("openspec-review-state")).toHaveAttribute(
      "data-state",
      "approved",
    );
    await expect(page.getByTestId("openspec-implementation-command")).toHaveText(
      "/implement-spec add-health-endpoint",
    );

    await page.getByTestId("chat-input").fill("/implement-spec add-health-endpoint");
    await page.getByTestId("send-button").click();
    await expectEventuallyTriple(request, {
      subject: runEntityId,
      predicate: "formal_claims.status",
      object: "passed",
    });
    await expectEventuallyTriple(request, {
      subject: runEntityId,
      predicate: "formal_claims.analyzer.version",
      object: "go-native-v1",
    });
    await expectEventuallyTriple(request, {
      subject: runEntityId,
      predicate: "proof_readiness.implementation_ready",
      object: "true",
    });
    await expectEventuallyPredicate(request, {
      subject: runEntityId,
      predicate: "dev_from_task.requested",
    });
    await expectEventuallyTriple(request, {
      subject: runEntityId,
      predicate: "dev_from_task.requested_change_slug",
      object: "add-health-endpoint",
    });
    await expect(healthPanel).toContainText(/Working|Complete/, {
      timeout: 30_000,
    });
    await expect(healthPanel).not.toContainText("Implement Task request");

    await page.getByTestId("tab-evidence").click();
    await page.getByRole("button", { name: "Refresh evidence" }).click();
    const evidence = page.getByTestId("run-evidence-triples");
    await expect(evidence).toContainText("formal_claims.status");
    await expect(evidence).toContainText("proof_readiness.route");
    await expect(evidence).toContainText("dev_from_task.requested");
    await page.getByTestId("tab-activity").click();

    const settled = await pollUntil(
      async () => {
        const list = await fetchLoops(request);
        const ralph = list.filter((l) => l.role === "dev-via-test-execute");
        const cbg = list.filter((l) => l.role === "reviewer-dev-via-test");
        const allTerminal = list.every((l) => TERMINAL.has(l.state ?? ""));
        if (ralph.length >= 1 && cbg.length >= 1 && allTerminal) {
          const acts = await fetchTriples(request, {
            predicate: "coordinator.decision.next_action",
            limit: 100,
          });
          const respondCount = acts.filter(
            (t) => String(t.object ?? "") === "respond_direct",
          ).length;
          if (respondCount >= 2) return list;
        }
        return null;
      },
      { timeoutMs: 90_000 },
    );
    expect(
      settled,
      "expected dev-from-task -> Ralph -> CBG chain to settle",
    ).toBeTruthy();
    await expect(healthPanel).toHaveAttribute("aria-label", "Run health: Complete", {
      timeout: 30_000,
    });
    await expect(healthPanel).toContainText("Accepted");
    await expect(healthPanel).toContainText(
      "Review the final artifact or export evidence",
    );

    const failed = settled!.filter((l) =>
      ["failed", "error", "cancelled", "truncated"].includes(l.state ?? ""),
    );
    expect(failed.map((l) => `${l.role}:${l.state}`)).toEqual([]);

    const byRole = (r: string) => settled!.filter((l) => l.role === r).length;
    expect(byRole("dev-via-test-plan"), "Lisa must be skipped").toBe(0);
    expect(byRole("dev-via-test-execute"), "one Ralph task executor").toBe(1);
    expect(byRole("reviewer-dev-via-test"), "one CBG final gate").toBe(1);

    const requiredTriples = await fetchTriples(request, {
      subject: runEntityId,
      limit: 500,
    });
    expect(valueOf(requiredTriples, "dev_from_task.started")).toBe("true");
    expect(valueOf(requiredTriples, "plan.source_change_slug")).toBe(
      "add-health-endpoint",
    );
    expect(valueOf(requiredTriples, "plan.done_authority.policy")).toBe(
      "approved_openspec_change",
    );
    expect(valueOf(requiredTriples, "plan.done_authority.source_change")).toBe(
      "change.add-health-endpoint",
    );
    expect(valueOf(requiredTriples, "plan.done_authority.final_gate")).toBe(
      "reviewer-dev-via-test",
    );
    expect(valueOf(requiredTriples, "plan.task.task-0.status")).toBe("ready");
    expect(valueOf(requiredTriples, "plan.integration_test_command")).toBe(
      "go test ./internal/health",
    );

    const actionTriples = await fetchTriples(request, {
      predicate: "coordinator.decision.next_action",
      limit: 100,
    });
    const actions = new Set(actionTriples.map((t) => String(t.object ?? "")));
    for (const want of [
      "create_change",
      "drafted",
      "dev_via_test",
      "measured",
      "dev_via_test_finalize",
      "approved",
      "respond_direct",
    ]) {
      expect(actions.has(want), `missing action ${want}`).toBe(true);
    }

    const report = await attachJourneyEvidenceReport({
      journeyName: "spec-to-dev-demo",
      fixture: "spec-to-dev-demo.yaml",
      config: "e2e-flow-bootstrap.json",
      request,
      testInfo,
      runIds: [runId],
      runEntityIds: [runEntityId],
      observations: {
        authorLoopId,
        handoffCommand: "/implement-spec add-health-endpoint",
        proofAnalyzerDrivenByCommand: true,
        lisaSkipped: true,
      },
    });
    expect(
      report.artifacts.tool_calls.some(
        (call) => call.tool === "project_spec_tasks",
      ),
      "journey report must include the live project_spec_tasks projection",
    ).toBe(true);
  });
});

async function fetchLoops(
  request: import("@playwright/test").APIRequestContext,
): Promise<Loop[]> {
  const resp = await request.get("/teams-dispatch/loops");
  if (!resp.ok()) {
    throw new Error(`/teams-dispatch/loops returned ${resp.status()}`);
  }
  return (await resp.json()) as Loop[];
}

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

async function seedProofInventory(runEntityId: string): Promise<void> {
  const now = new Date().toISOString();
  const expiresAt = new Date(Date.now() + 60 * 60 * 1000).toISOString();
  const mk = (predicate: string, object: unknown): Triple => ({
    subject: runEntityId,
    predicate,
    object,
    source: "e2e-spec-to-dev-demo",
    timestamp: now,
    confidence: 1,
  });
  await publishNatsRequestless("graph.mutation.triple.add_batch", {
    triples: [
      mk("proof.claim.health.endpoint.status", "accepted"),
      mk(
        "proof.claim.health.endpoint.statement",
        "GET /health returns HTTP 200 with JSON status ok.",
      ),
      mk("proof.claim.health.endpoint.requires", ["go_health_endpoint_test"]),
      mk("proof.claim.health.endpoint.task_refs", ["change.add-health-endpoint.task.0"]),

      mk("proof.dependency.go_health_endpoint_test.kind", "test"),
      mk(
        "proof.dependency.go_health_endpoint_test.description",
        "Focused Go test proves the health endpoint contract.",
      ),
      mk("proof.dependency.go_health_endpoint_test.required_for", ["health.endpoint"]),
      mk("proof.dependency.go_health_endpoint_test.status", "ready"),
      mk("proof.dependency.go_health_endpoint_test.profile_ref", "go.unit-test@v1"),

      mk("proof.harness_profile.go.unit-test@v1.status", "ready"),
      mk("proof.harness_profile.go.unit-test@v1.version", "v1"),
      mk("proof.harness_profile.go.unit-test@v1.team", "local-go"),
      mk("proof.harness_profile.go.unit-test@v1.claims_supported", ["health.endpoint"]),
      mk("proof.harness_profile.go.unit-test@v1.dependencies", [
        "go_health_endpoint_test",
      ]),
      mk("proof.harness_profile.go.unit-test@v1.readiness_probes", ["health_smoke"]),
      mk("proof.harness_profile.go.unit-test@v1.smoke_command", "go test ./internal/health"),
      mk("proof.harness_profile.go.unit-test@v1.artifacts", ["go-test.log"]),
      mk("proof.harness_profile.go.unit-test@v1.renderer", "local"),
      mk("proof.harness_profile.go.unit-test@v1.ttl_seconds", "3600"),

      mk("proof.readiness.health_smoke.profile_ref", "go.unit-test@v1"),
      mk("proof.readiness.health_smoke.status", "passed"),
      mk("proof.readiness.health_smoke.smoke_status", "passed"),
      mk("proof.readiness.health_smoke.expires_at", expiresAt),
      mk("proof.readiness.health_smoke.evidence", ["health_endpoint_test_log"]),

      mk("proof.evidence.health_endpoint_test_log.kind", "test"),
      mk("proof.evidence.health_endpoint_test_log.producer", "e2e-spec-to-dev-demo"),
      mk("proof.evidence.health_endpoint_test_log.command", "go test ./internal/health"),
      mk("proof.evidence.health_endpoint_test_log.created_at", now),
      mk("proof.evidence.health_endpoint_test_log.covers", ["health.endpoint"]),
    ],
  });
}

async function publishNatsRequestless(
  subject: string,
  payload: unknown,
): Promise<void> {
  const body = JSON.stringify(payload);
  const bytes = Buffer.byteLength(body);
  const protocol =
    `CONNECT {"verbose":false,"pedantic":false,"name":"spec-to-dev-demo"}\r\n` +
    `PUB ${subject} ${bytes}\r\n${body}\r\nPING\r\n`;

  await new Promise<void>((resolve, reject) => {
    const child = spawn(
      "docker",
      ["exec", "-i", "semteams-ui-agentic-nats", "sh", "-c", "nc -w 2 localhost 4222"],
      { stdio: ["pipe", "pipe", "pipe"] },
    );
    let stderr = "";
    const timer = setTimeout(() => {
      child.kill("SIGKILL");
      reject(new Error("timed out publishing NATS seed triples"));
    }, 5_000);

    child.stderr.on("data", (chunk) => {
      stderr += String(chunk);
    });
    child.on("error", (err) => {
      clearTimeout(timer);
      reject(err);
    });
    child.on("close", (code) => {
      clearTimeout(timer);
      if (code === 0) resolve();
      else reject(new Error(`docker exec nc exited ${code}: ${stderr}`));
    });
    child.stdin.end(protocol);
  });
}

async function expectEventuallyTriple(
  request: import("@playwright/test").APIRequestContext,
  want: { subject: string; predicate: string; object: unknown },
): Promise<void> {
  const found = await pollUntil(
    async () => {
      const triples = await fetchTriples(request, {
        subject: want.subject,
        predicate: want.predicate,
        limit: 20,
      });
      return triples.some((t) => t.object === want.object) ? true : null;
    },
    { timeoutMs: 10_000 },
  );
  expect(
    found,
    `expected ${want.subject} ${want.predicate} ${String(want.object)}`,
  ).toBe(true);
}

async function expectEventuallyPredicate(
  request: import("@playwright/test").APIRequestContext,
  want: { subject: string; predicate: string },
): Promise<void> {
  const found = await pollUntil(
    async () => {
      const triples = await fetchTriples(request, {
        subject: want.subject,
        predicate: want.predicate,
        limit: 20,
      });
      return triples.length > 0 ? true : null;
    },
    { timeoutMs: 10_000 },
  );
  expect(found, `expected ${want.subject} ${want.predicate}`).toBe(true);
}

function valueOf(triples: Triple[], predicate: string): string | undefined {
  const triple = triples.find((t) => t.predicate === predicate);
  if (!triple) return undefined;
  return String(triple.object ?? "");
}

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
