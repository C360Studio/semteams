import { test, expect } from "@playwright/test";
import {
  attachJourneyEvidenceReport,
  type JourneyReport,
} from "./e2e_report";

interface Triple {
  subject?: string;
  predicate?: string;
  object?: unknown;
}

type JsonRecord = Record<string, unknown>;

const CONFIG = "flow-bootstrap-gemini-smoke.gen.json";
const RUN_INFIX = ".agent.chain.execution.";
const LOOP_INFIX = ".agent.agentic-loop.execution.";

const PROMPT = `Draft a reviewed OpenSpec change, not code, for an existing Go HTTP API.

Feature: add Idempotency-Key support to POST /jobs.

Requirements:
- Clients MAY send an Idempotency-Key header when creating a job.
- If the same key and same request body are retried within 24 hours, the API SHALL return the original response without creating a second job.
- If the same key is reused with a different request body, the API SHALL return 409.
- Keep scope limited to server-side request handling and storage; no UI, billing, or background worker changes.

The output should be an OpenSpec change with requirements, Given/When/Then scenarios, implementation tasks, target files, and Go test commands.`;

// PARKED (ADR-058): this journey exercises a dev-side pack that is unwired
// from the bootstrap pending the canonical-predicate migration. Re-enable by
// restoring test.describe when the pack is re-authored and re-wired.
test.describe.skip("ADR-057 / OpenSpec 6.1 — create_change Gemini smoke", () => {
  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok(), "Backend not healthy -- stack not running?").toBe(true);
  });

  test.setTimeout(720_000);

  test("Gemini routes spec prompt, emits reviewed OpenSpec change, and reports model/artifact evidence", async ({
    page,
    request,
  }, testInfo) => {
    let report: JourneyReport | undefined;
    const runIds: string[] = [];
    const runEntityIds: string[] = [];
    const observations: JsonRecord = {
      prompt: "idempotency-key-post-jobs",
    };

    try {
      await page.goto("/");
      await expect(page.getByTestId("connection-status")).toHaveAttribute(
        "data-summary",
        "healthy",
        { timeout: 15_000 },
      );
      await expect(page.getByTestId("kanban-board")).toBeVisible();

      await page.getByTestId("chat-input").fill(PROMPT);
      await page.getByTestId("send-button").click();

      const runContext = await pollUntil(
        async () => {
          const triples = await fetchTriples(request, { limit: 10_000 });
          const intents = triples.filter(
            (triple) =>
              String(triple.predicate ?? "").startsWith("change.") &&
              String(triple.predicate ?? "").endsWith(".proposal.intent"),
          );
          const intent = intents.find((triple) =>
            /idempotency|jobs/i.test(String(triple.object ?? "")),
          );
          if (!intent) return null;

          const runEntity = String(intent.subject ?? "");
          if (!runEntity.includes(RUN_INFIX)) return null;

          const runId = bareIdAfter(runEntity, RUN_INFIX);
          const loopSubjects = loopSubjectsForRun(triples, runEntity);
          return {
            runEntity,
            runId,
            loopSubjects,
            frontDoorLoopSubject: frontDoorLoopSubjectForRun(runEntity),
            intent,
          };
        },
        { timeoutMs: 300_000, intervalMs: 2_000 },
      );

      expect(
        runContext,
        "emit_change must stamp a change.<slug>.proposal.intent triple for the new run",
      ).toBeTruthy();
      expect(runContext!.runId, `could not extract run id from ${runContext!.runEntity}`).toBeTruthy();

      runIds.push(runContext!.runId);
      runEntityIds.push(runContext!.runEntity);
      observations.runId = runContext!.runId;
      observations.changeIntentPredicate = runContext!.intent.predicate;

      const settled = await pollUntil(
        async () => {
          const triples = await fetchTriples(request, { limit: 10_000 });
          const loopSubjects = loopSubjectsForRun(
            triples,
            runContext!.runEntity,
          );
          const scopedSubjects = unique([
            ...loopSubjects,
            runContext!.frontDoorLoopSubject,
          ]);

          const roles = valuesForSubjects(
            triples,
            scopedSubjects,
            "agent.loop.role",
          );
          const actions = valuesForSubjects(
            triples,
            scopedSubjects,
            "coordinator.decision.next_action",
          );
          const outcomes = valuesForSubjects(
            triples,
            scopedSubjects,
            "agent.loop.outcome",
          );
          const runPhases = valuesForSubjects(
            triples,
            [runContext!.runEntity],
            "agent.run.phase",
          );

          if (
            outcomes.some((outcome) =>
              ["failed", "error", "cancelled", "truncated"].includes(outcome),
            ) ||
            runPhases.includes("failed")
          ) {
            return { roles, actions, outcomes, runPhases, loopSubjects };
          }

          const expectedRoles = [
            "coordinator",
            "author-create-change",
            "reviewer-create-change",
          ];
          const expectedActions = [
            "create_change",
            "drafted",
            "approved",
            "respond_direct",
          ];
          if (
            expectedRoles.every((role) => roles.includes(role)) &&
            expectedActions.every((action) => actions.includes(action)) &&
            runPhases.includes("completed")
          ) {
            return { roles, actions, outcomes, runPhases, loopSubjects };
          }

          return null;
        },
        { timeoutMs: 300_000, intervalMs: 2_000 },
      );

      expect(
        settled,
        "expected the scoped create-change run to route, author, review, render, and complete",
      ).toBeTruthy();

      expect(
        settled!.outcomes.filter((outcome) => outcome !== "success"),
        "no create-change smoke loop should fail",
      ).toEqual([]);

      expect(
        settled!.runPhases,
        "run must reach completed after reviewer approval",
      ).toContain("completed");
      expect(settled!.runPhases, "run must not be failed").not.toContain(
        "failed",
      );
      expect(
        settled!.roles,
        "scoped run must include coordinator, author, and reviewer roles",
      ).toEqual(
        expect.arrayContaining([
          "coordinator",
          "author-create-change",
          "reviewer-create-change",
        ]),
      );
      expect(
        settled!.actions,
        "scoped run must include the create_change/drafted/approved/respond_direct control tokens",
      ).toEqual(
        expect.arrayContaining([
          "create_change",
          "drafted",
          "approved",
          "respond_direct",
        ]),
      );

      report = await attachJourneyEvidenceReport({
        journeyName: "create-change-gemini-smoke",
        fixture: "none",
        config: CONFIG,
        request,
        testInfo,
        runIds,
        runEntityIds,
        observations,
      });

      expect(
        report.models.resolved.length,
        "journey report must resolve model requests from the Gemini-only config",
      ).toBeGreaterThan(0);
      expect(
        report.models.resolved.map((model) => model.provider),
        "Gemini smoke must not resolve Anthropic/OpenAI fallback endpoints",
      ).toEqual(expect.arrayContaining(["gemini"]));
      expect(
        report.models.resolved.every((model) => model.provider === "gemini"),
        "every resolved model endpoint must be Gemini for this smoke",
      ).toBe(true);
      expect(
        report.models.resolved.map((model) => model.model ?? ""),
        "resolved model ids must be Gemini model ids",
      ).toEqual(expect.arrayContaining([expect.stringContaining("gemini")]));

      const toolNames = report.artifacts.tool_calls.map((call) => call.tool);
      expect(toolNames, "author must call emit_change").toContain("emit_change");
      expect(toolNames, "wake-up coordinator must call render_openspec").toContain(
        "render_openspec",
      );

      const emitChangeArgs = report.artifacts.tool_calls.find((call) => {
        if (call.tool !== "emit_change" || !call.argument_keys.includes("tasks")) {
          return false;
        }
        return /Idempotency-Key|POST \/jobs/i.test(JSON.stringify(call.arguments));
      })?.arguments;
      expect(
        emitChangeArgs,
        "expected structured emit_change arguments",
      ).toBeTruthy();
      const emitted = JSON.stringify(emitChangeArgs);
      expect(emitted).toContain("Idempotency-Key");
      expect(emitted).toContain("POST /jobs");
      expect(emitted).toContain("409");
      expect(emitted).toMatch(/24|86400/);
      expect(emitted).toMatch(/Given|GIVEN/);
      expect(emitted).toMatch(/When|WHEN/);
      expect(emitted).toMatch(/Then|THEN/);
      expect(emitted).toMatch(/go test/);

      const args = asRecord(emitChangeArgs);
      expect(
        Array.isArray(args?.tasks) && args.tasks.length > 0,
        "emitted change must include implementation tasks",
      ).toBe(true);
      const taskJson = JSON.stringify(args?.tasks);
      expect(taskJson, "tasks must name target files").toContain("target");
      expect(taskJson, "tasks must name Go test commands").toMatch(/go test/);
    } finally {
      if (!report) {
        try {
          await attachJourneyEvidenceReport({
            journeyName: "create-change-gemini-smoke",
            fixture: "none",
            config: CONFIG,
            request,
            testInfo,
            runIds,
            runEntityIds,
            observations,
          });
        } catch (err) {
          await testInfo.attach("journey-report-error.txt", {
            body: err instanceof Error ? err.stack ?? err.message : String(err),
            contentType: "text/plain",
          });
        }
      }
    }
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

function frontDoorLoopSubjectForRun(runEntity: string): string {
  const idx = runEntity.indexOf(RUN_INFIX);
  if (idx === -1) return "";
  return `${runEntity.slice(0, idx)}${LOOP_INFIX}${bareIdAfter(
    runEntity,
    RUN_INFIX,
  )}`;
}

function loopSubjectsForRun(triples: Triple[], runEntity: string): string[] {
  return unique(
    triples
      .filter(
        (triple) =>
          triple.predicate === "agent.run.entity_id" &&
          triple.object === runEntity &&
          typeof triple.subject === "string",
      )
      .map((triple) => triple.subject as string),
  );
}

function valuesForSubjects(
  triples: Triple[],
  subjects: string[],
  predicate: string,
): string[] {
  const subjectSet = new Set(subjects);
  return unique(
    triples
      .filter(
        (triple) =>
          triple.predicate === predicate &&
          typeof triple.subject === "string" &&
          subjectSet.has(triple.subject),
      )
      .map((triple) => String(triple.object ?? "")),
  );
}

async function pollUntil<T>(
  fn: () => Promise<T | null>,
  opts: { timeoutMs: number; intervalMs?: number },
): Promise<T | null> {
  const deadline = Date.now() + opts.timeoutMs;
  const interval = opts.intervalMs ?? 1_000;
  while (Date.now() < deadline) {
    const result = await fn();
    if (result != null) return result;
    await new Promise((resolve) => setTimeout(resolve, interval));
  }
  return null;
}

function asRecord(value: unknown): JsonRecord | undefined {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    return undefined;
  }
  return value as JsonRecord;
}

function unique<T>(values: T[]): T[] {
  return [...new Set(values)];
}
