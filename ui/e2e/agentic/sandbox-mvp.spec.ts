import { test, expect } from "@playwright/test";

/**
 * Journey: ADR-043 PR 4.4 — sandbox tools mock-LLM journey.
 *
 * Validates the request_sandbox + query_sandbox_attestation
 * synchronous tool flow end-to-end at the Coordinator tier. ADR-043
 * three-layer model in action:
 *
 *   Loop 0 — coordinator (dispatch)
 *     1. tool_call: query_sandbox_attestation
 *        → chain entity has no prior attestation → miss
 *     2. tool_call: request_sandbox
 *        → SEMTEAMS_SANDBOX_RUNNER=mock wires MockRunner →
 *          Ready attestation; sandbox.attestation.* triples stamped
 *          on the chain entity
 *     3. tool_call: decide(action="respond_direct")
 *        → agentic-dispatch publishes the typed decision on user.response.*;
 *          rule coordinator/03b-respond-direct audits it. Terminal.
 *
 * Validates:
 *   - Both new tools reachable from the dispatch coordinator's
 *     default_tools (PR 4.2 default_tools extension)
 *   - MockRunner end-to-end Ready path
 *   - Single coordinator loop terminates with respond_direct;
 *     NO rule pack arc spawned (PR 4.2 retire validated)
 *   - user.response.* carries a registered agentic.user_response.v1 payload
 *
 * Required fixture: test/fixtures/journeys/sandbox-mvp.yaml
 * Required config: configs/e2e-flow-bootstrap.json.
 * Required env: SEMTEAMS_SANDBOX_RUNNER=mock on the backend
 *   service (see ui/docker-compose.agentic-e2e.yml).
 */

test.describe("ADR-043 PR 4.4 — Sandbox tools mock-LLM journey", () => {
  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok(), "Backend not healthy — stack not running?").toBe(true);
  });

  // Single coordinator loop with 3 tool calls. 90s budget covers
  // mock-LLM scheduling jitter + cold-start.
  test.setTimeout(90_000);

  test("user prompt → query+request_sandbox → coordinator delivers ready", async ({
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

    // Send a prompt the coordinator persona's "Provisioning a sandbox"
    // section would route through request_sandbox.
    await page.getByTestId("chat-input").fill(
      "Set up a Go backend sandbox so I can run task test — confirm the environment is ready before doing anything else.",
    );
    await page.getByTestId("send-button").click();

    // Wait for the single coordinator loop to reach terminal complete.
    // Under the mock-LLM fixture, this is one loop with three tool
    // calls: query_sandbox_attestation, request_sandbox, decide.
    const terminal = await pollUntil(async () => {
      const resp = await request.get("/teams-dispatch/loops");
      if (!resp.ok()) return null;
      const list = (await resp.json()) as Array<{ state: string }>;
      // Exactly ONE loop: the dispatch coordinator. No rule pack arc
      // spawns — request_sandbox is synchronous, no decide(action=
      // sandbox) routes a chain forward.
      if (list.length !== 1) return null;
      return list.every((l) => l.state === "complete") ? list : null;
    }, { timeoutMs: 60_000 });

    expect(
      terminal,
      "expected exactly 1 coordinator loop in terminal complete state",
    ).toBeTruthy();

    // user.response.* proves agentic-dispatch published the typed
    // respond_direct response and message-logger observed it. The prose is
    // also retained on the loop entity as coordinator.clarification.reply
    // (asserted below); semstreams#1090 tracks channel-ready delivery.
    const respResp = await request.get(
      "/message-logger/entries?subject=user.response.*&limit=10",
    );
    expect(respResp.ok(), "/message-logger/entries returned non-OK").toBe(true);
    const respPayloads = (await respResp.json()) as Array<{
      subject: string;
      message_type: string;
      raw_data?: { payload?: { content?: string } };
    }>;
    expect(
      respPayloads.length,
      "expected at least one typed user.response.* publish for respond_direct",
    ).toBeGreaterThanOrEqual(1);
    expect(respPayloads.every((entry) => entry.subject.startsWith("user.response."))).toBe(true);
    expect(respPayloads.every((entry) => entry.message_type === "agentic.user_response.v1")).toBe(true);
    expect(
      respPayloads.some((entry) => {
        const content = entry.raw_data?.payload?.content;
        return typeof content === "string" && content.includes('"action":"respond_direct"');
      }),
      "beta.161 carries the structured decide result in UserResponse.Content; semstreams#1090 tracks channel-ready reason extraction",
    ).toBe(true);

    // Per PR 4.4 finding M1: verify the structural attestation
    // triples were stamped. Absent triples mean the tool errored
    // before stamping (Runner failure, publisher drop, etc.) and the
    // loop-completed signal would have falsely indicated success.
    // sandbox.attestation.ready=true proves Manager.Request reached
    // its happy path (MockRunner → Ready → Attest → publisher).
    const readyTriples = await fetchTriples(request, {
      predicate: "sandbox.attestation.ready",
      limit: 10,
    });
    expect(
      readyTriples.length,
      "expected at least one sandbox.attestation.ready triple — proves request_sandbox reached Manager.Request + stamped via publisher; absence means tool errored before stamping or the publisher dropped the batch",
    ).toBeGreaterThanOrEqual(1);
    // Object can come back as bool or string "true" depending on
    // the graph-ingest path; accept either.
    const obj = readyTriples[0]?.object;
    const objStr = typeof obj === "boolean" ? String(obj) : String(obj ?? "");
    expect(
      objStr.toLowerCase(),
      `expected sandbox.attestation.ready=true; got ${objStr}`,
    ).toBe("true");

    // Sister assertion: the attestation profile matches the
    // mock-LLM fixture's request (go-backend@v1 — the BuiltinCatalog
    // canonical match for {languages: [go], tools: [task]}). Proves
    // the profile matcher fired correctly.
    const profileTriples = await fetchTriples(request, {
      predicate: "sandbox.attestation.profile",
      limit: 10,
    });
    expect(
      profileTriples.length,
      "expected sandbox.attestation.profile triple",
    ).toBeGreaterThanOrEqual(1);
    expect(
      profileTriples[0]?.object,
      "expected go-backend@v1 profile match for the fixture's requirements",
    ).toBe("go-backend@v1");

    // The user-facing prose lives on the dispatch coordinator loop
    // entity as coordinator.clarification.reply (the audit triple the rule
    // stamps from $entity.triple.coordinator.decision.reason).
    // Substring-check it carries the readiness signal.
    const replyTriples = await fetchTriples(request, {
      predicate: "coordinator.clarification.reply",
      limit: 10,
    });
    expect(
      replyTriples.length,
      "expected coordinator.clarification.reply triple (audit of respond_direct prose)",
    ).toBeGreaterThanOrEqual(1);
    const replyObj = String(replyTriples[0]?.object ?? "").toLowerCase();
    expect(
      replyObj,
      "expected coordinator.clarification.reply prose to mention readiness",
    ).toContain("ready");
  });
});

interface Triple {
  subject?: string;
  predicate?: string;
  object?: unknown;
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

/**
 * pollUntil — race a polling fn against a deadline.
 * Returns the fn's value on success, or null on deadline.
 */
async function pollUntil<T>(
  fn: () => Promise<T | null>,
  opts: { timeoutMs: number; intervalMs?: number },
): Promise<T | null> {
  const deadline = Date.now() + opts.timeoutMs;
  const interval = opts.intervalMs ?? 250;
  while (Date.now() < deadline) {
    const result = await fn();
    if (result != null) return result;
    await new Promise((r) => setTimeout(r, interval));
  }
  return null;
}
