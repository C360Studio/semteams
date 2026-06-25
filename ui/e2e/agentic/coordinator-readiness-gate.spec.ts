import { test, expect } from "@playwright/test";

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

interface MessageEntry {
  subject?: string;
  raw_data?: unknown;
  payload?: unknown;
}

const LOOP_TERMINAL = new Set([
  "complete",
  "success",
  "failed",
  "failure",
  "error",
  "cancelled",
  "truncated",
  "timeout",
]);
const LOOP_FAILURE = new Set([
  "failed",
  "failure",
  "error",
  "cancelled",
  "truncated",
  "timeout",
]);

const CASES = [
  {
    name: "public-network optimization waits for sandbox admission",
    prompt:
      "Optimize the Svelte Playwright smoke that fetches public browser dependencies. Metric: wallclock seconds, lower is better. Command: `npm run test:e2e`. Surface: `ui/`. Cap: 2 iterations. This requires public network access.",
    expectedAdmission: "admission_pending",
    expectedReady: false,
    expectedTerminal: false,
    forbiddenAction: "autoresearch",
    forbiddenRole: "autoresearch-baseline",
  },
  {
    name: "Go public-network implementation surfaces terminal admission denial",
    prompt:
      "Build a Go service that fetches a live public API during its acceptance test, prove it with `task test`, and use public network access.",
    expectedAdmission: "denied",
    expectedReady: false,
    expectedTerminal: true,
    forbiddenAction: "dev_via_test",
    forbiddenRole: "dev-via-test-plan",
  },
];

test.describe("coordinator readiness gate", () => {
  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok(), "Backend not healthy - stack not running?").toBe(true);
  });

  test.setTimeout(90_000);

  test("non-ready sandbox admission fails closed before execution routing", async ({
    request,
  }) => {
    for (const c of CASES) {
      const beforeLoops = await fetchLoops(request);
      const beforeLoopIds = new Set(beforeLoops.map((loop) => loop.loop_id));

      const sent = await request.post("/teams-dispatch/message", {
        data: { content: c.prompt },
      });
      expect(
        sent.ok(),
        `${c.name}: dispatch rejected message ${sent.status()} ${await sent.text()}`,
      ).toBe(true);

      const loop = await waitForNewLoop(request, beforeLoopIds);
      expect(loop, `${c.name}: expected a new coordinator loop`).toBeTruthy();
      const loopId = String(loop!.loop_id);

      const terminal = await waitForLoopTerminal(request, loopId);
      expect(
        terminal,
        `${c.name}: coordinator loop ${loopId} did not reach terminal state`,
      ).toBeTruthy();
      expect(
        LOOP_FAILURE.has(terminal!.state ?? ""),
        `${c.name}: coordinator loop failed with state ${terminal!.state}`,
      ).toBe(false);

      await expectSandboxPreflightAttempted(request, loopId, c.name);
      await expectSandboxAttestation(
        request,
        loopId,
        {
          admission: c.expectedAdmission,
          ready: c.expectedReady,
          terminal: c.expectedTerminal,
        },
        c.name,
      );
      await expectCoordinatorAction(request, loopId, "respond_direct", c.name);
      await expectCoordinatorActionAbsent(
        request,
        loopId,
        c.forbiddenAction,
        c.name,
      );
      await expectRoleNotSpawned(
        request,
        beforeLoopIds,
        c.forbiddenRole,
        c.name,
      );
    }
  });
});

async function fetchLoops(
  request: import("@playwright/test").APIRequestContext,
): Promise<Loop[]> {
  const resp = await request.get("/teams-dispatch/loops");
  if (!resp.ok()) {
    throw new Error(`/teams-dispatch/loops returned ${resp.status()}: ${await resp.text()}`);
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

async function waitForNewLoop(
  request: import("@playwright/test").APIRequestContext,
  beforeLoopIds: Set<string | undefined>,
): Promise<Loop | null> {
  return pollUntil(async () => {
    const loops = await fetchLoops(request);
    return loops.find((loop) => !beforeLoopIds.has(loop.loop_id)) ?? null;
  }, { timeoutMs: 20_000 });
}

async function waitForLoopTerminal(
  request: import("@playwright/test").APIRequestContext,
  loopId: string,
): Promise<{ state?: string } | null> {
  return pollUntil(async () => {
    const resp = await request.get(`/teams-dispatch/loops/${loopId}`);
    if (!resp.ok()) return null;
    const body = (await resp.json()) as { state?: string };
    return LOOP_TERMINAL.has(body.state ?? "") ? body : null;
  }, { timeoutMs: 20_000 });
}

async function expectSandboxPreflightAttempted(
  request: import("@playwright/test").APIRequestContext,
  loopId: string,
  caseName: string,
): Promise<void> {
  const loopCalls = await pollUntil(async () => {
    const entries = await fetchMessageEntries(request, "tool.execute.", 500);
    const calls = entries
      .filter((entry) => entryBelongsToLoop(entry, loopId))
      .map(toolNameFromEntry)
      .filter((toolName): toolName is string => Boolean(toolName));

    if (
      calls.includes("query_sandbox_attestation") &&
      calls.includes("request_sandbox")
    ) {
      return calls;
    }
    return null;
  }, { timeoutMs: 10_000 });

  expect(
    loopCalls ?? [],
    `${caseName}: coordinator should query sandbox readiness before routing`,
  ).toContain("query_sandbox_attestation");
  expect(
    loopCalls ?? [],
    `${caseName}: coordinator should request sandbox provisioning before routing`,
  ).toContain("request_sandbox");
}

async function expectSandboxAttestation(
  request: import("@playwright/test").APIRequestContext,
  loopId: string,
  expected: { admission: string; ready: boolean; terminal: boolean },
  caseName: string,
): Promise<void> {
  const attestation = await pollUntil(async () => {
    const [admissionTriples, readyTriples, terminalTriples] = await Promise.all([
      fetchTriples(request, {
        predicate: "sandbox.attestation.admission",
        limit: 500,
      }),
      fetchTriples(request, {
        predicate: "sandbox.attestation.ready",
        limit: 500,
      }),
      fetchTriples(request, {
        predicate: "sandbox.attestation.terminal",
        limit: 500,
      }),
    ]);
    return coherentAttestationForLoop(
      [...admissionTriples, ...readyTriples, ...terminalTriples],
      loopId,
    );
  }, { timeoutMs: 10_000 });

  expect(
    attestation,
    `${caseName}: expected sandbox attestation triples on the attestation target entity for ${loopId}`,
  ).toBeTruthy();
  expect(
    String(attestation!.admission.object ?? ""),
    `${caseName}: unexpected sandbox admission outcome`,
  ).toBe(expected.admission);
  expect(
    boolish(attestation!.ready.object),
    `${caseName}: unexpected sandbox readiness`,
  ).toBe(String(expected.ready));
  expect(
    boolish(attestation!.terminal.object),
    `${caseName}: unexpected sandbox terminal flag`,
  ).toBe(String(expected.terminal));
}

async function expectCoordinatorAction(
  request: import("@playwright/test").APIRequestContext,
  loopId: string,
  action: string,
  caseName: string,
): Promise<void> {
  const found = await pollUntil(async () => {
    const actions = await fetchTriples(request, {
      predicate: "coordinator.decision.next_action",
      limit: 500,
    });
    return (
      actions.find(
        (triple) =>
          subjectIncludesLoop(triple, loopId) &&
          String(triple.object ?? "") === action,
      ) ?? null
    );
  }, { timeoutMs: 10_000 });

  expect(
    found,
    `${caseName}: expected coordinator.decision.next_action=${action}`,
  ).toBeTruthy();
}

async function expectCoordinatorActionAbsent(
  request: import("@playwright/test").APIRequestContext,
  loopId: string,
  action: string,
  caseName: string,
): Promise<void> {
  const actions = await fetchTriples(request, {
    predicate: "coordinator.decision.next_action",
    limit: 500,
  });
  const scopedActions = actions
    .filter((triple) => subjectIncludesLoop(triple, loopId))
    .map((triple) => String(triple.object ?? ""));
  expect(
    scopedActions,
    `${caseName}: non-ready sandbox must not release ${action}`,
  ).not.toContain(action);
}

async function expectRoleNotSpawned(
  request: import("@playwright/test").APIRequestContext,
  beforeLoopIds: Set<string | undefined>,
  role: string,
  caseName: string,
): Promise<void> {
  const spawnedRole = await pollUntil(async () => {
    const loops = await fetchLoops(request);
    return (
      loops
        .filter((loop) => !beforeLoopIds.has(loop.loop_id))
        .map((loop) => loop.role)
        .find((spawned) => spawned === role) ?? null
    );
  }, { timeoutMs: 2_000 });

  expect(
    spawnedRole,
    `${caseName}: non-ready sandbox must not spawn ${role}`,
  ).toBeNull();
}

async function fetchMessageEntries(
  request: import("@playwright/test").APIRequestContext,
  subjectPrefix: string,
  limit: number,
): Promise<MessageEntry[]> {
  const query = new URLSearchParams({
    subject_prefix: subjectPrefix,
    limit: String(limit),
  });
  const resp = await request.get(`/message-logger/entries?${query.toString()}`);
  if (!resp.ok()) {
    throw new Error(
      `/message-logger/entries?${query.toString()} returned ${resp.status()}: ${await resp.text()}`,
    );
  }
  return (await resp.json()) as MessageEntry[];
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

function coherentAttestationForLoop(
  triples: Triple[],
  loopId: string,
): { admission: Triple; ready: Triple; terminal: Triple } | null {
  const bySubject = new Map<
    string,
    Partial<{ admission: Triple; ready: Triple; terminal: Triple }>
  >();
  for (const triple of triples) {
    const subject = String(triple.subject ?? "");
    if (!subject.includes(loopId)) continue;

    const group = bySubject.get(subject) ?? {};
    switch (triple.predicate) {
      case "sandbox.attestation.admission":
        group.admission = triple;
        break;
      case "sandbox.attestation.ready":
        group.ready = triple;
        break;
      case "sandbox.attestation.terminal":
        group.terminal = triple;
        break;
    }
    bySubject.set(subject, group);
  }

  for (const group of bySubject.values()) {
    if (group.admission && group.ready && group.terminal) {
      return {
        admission: group.admission,
        ready: group.ready,
        terminal: group.terminal,
      };
    }
  }
  return null;
}

function subjectIncludesLoop(triple: Triple, loopId: string): boolean {
  return String(triple.subject ?? "").includes(loopId);
}

function boolish(value: unknown): string {
  if (typeof value === "boolean") return String(value);
  return String(value ?? "").toLowerCase();
}

function entryBelongsToLoop(entry: MessageEntry, loopId: string): boolean {
  const payload = entryPayload(entry);
  if (!payload) return false;
  if (payload.loop_id === loopId) return true;
  return asRecord(payload.metadata)?.loop_id === loopId;
}

function toolNameFromEntry(entry: MessageEntry): string | undefined {
  const payload = entryPayload(entry);
  const name = payload?.name;
  if (typeof name === "string") return name;
  if (entry.subject?.startsWith("tool.execute.")) {
    return entry.subject.slice("tool.execute.".length);
  }
  return undefined;
}

function entryPayload(entry: MessageEntry): Record<string, unknown> | undefined {
  const rawPayload = asRecord(asRecord(entry.raw_data)?.payload);
  return rawPayload ?? asRecord(entry.payload);
}

function asRecord(value: unknown): Record<string, unknown> | undefined {
  return value && typeof value === "object"
    ? (value as Record<string, unknown>)
    : undefined;
}
