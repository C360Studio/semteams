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
    name: "evidence comparison routes to research",
    prompt:
      "Compare MQTT and NATS for constrained IoT edge deployments. I need the practical tradeoffs and evidence, not code.",
    action: "research",
  },
  {
    name: "OpenSpec authoring routes to create_change",
    prompt:
      "Create an OpenSpec change for adding Idempotency-Key support to POST /jobs. Include acceptance criteria and affected API contracts.",
    action: "create_change",
  },
  {
    name: "bounded scalar optimization routes to autoresearch",
    prompt:
      "Optimize `go test ./...` wallclock. Metric: trailing ok-line seconds, lower is better. Command: `go test ./...`. Surface: `./...`. Cap: 2 iterations. Guardrail: all tests must keep passing.",
    action: "autoresearch",
    requiresSandbox: true,
  },
  {
    name: "verifiable implementation routes to dev_via_test",
    prompt:
      "Build a Go HTTP service that decodes MAVLink HEARTBEAT frames with github.com/bluenviron/gomavlib and serves the latest at GET /heartbeat as JSON, with unit tests.",
    action: "dev_via_test",
    requiresSandbox: true,
  },
  {
    name: "vague optimization asks the user",
    prompt: "Make this project faster and better.",
    action: "ask_user",
  },
  {
    name: "simple taxonomy question responds directly",
    prompt: "What coordinator routing actions are available? Answer briefly.",
    action: "respond_direct",
  },
];

test.describe("coordinator routing matrix", () => {
  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok(), "Backend not healthy - stack not running?").toBe(true);
  });

  test.setTimeout(90_000);

  test("representative prompts choose the expected coordinator action", async ({
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

      const newLoop = await waitForNewLoop(request, beforeLoopIds);
      expect(newLoop, `${c.name}: expected a new coordinator loop`).toBeTruthy();

      const terminal = await waitForLoopTerminal(request, String(newLoop!.loop_id));
      expect(
        terminal,
        `${c.name}: coordinator loop ${newLoop!.loop_id} did not reach a terminal state`,
      ).toBeTruthy();
      expect(
        LOOP_FAILURE.has(terminal!.state ?? ""),
        `${c.name}: coordinator loop failed with state ${terminal!.state}`,
      ).toBe(false);

      const action = await pollUntil(async () => {
        const actions = await fetchTriples(request, {
          predicate: "coordinator.decision.next_action",
          limit: 500,
        });
        return (
          actions.find(
            (triple) =>
              subjectIncludesLoop(triple, String(newLoop!.loop_id)) &&
              String(triple.object ?? "") === c.action,
          ) ?? null
        );
      }, { timeoutMs: 10_000 });

      expect(
        action,
        `${c.name}: expected coordinator.decision.next_action=${c.action} on loop ${newLoop!.loop_id}`,
      ).toBeTruthy();

      if (c.requiresSandbox) {
        await expectSandboxPreflightAttempted(
          request,
          String(newLoop!.loop_id),
          c.name,
        );
      }

      await expectOnlyOneLoopSpawned(
        request,
        beforeLoopIds,
        String(newLoop!.loop_id),
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

async function expectOnlyOneLoopSpawned(
  request: import("@playwright/test").APIRequestContext,
  beforeLoopIds: Set<string | undefined>,
  expectedLoopId: string,
  caseName: string,
): Promise<void> {
  await new Promise((resolve) => setTimeout(resolve, 2_000));
  const loops = await fetchLoops(request);
  const spawned = loops.filter((loop) => !beforeLoopIds.has(loop.loop_id));

  expect(
    spawned.map((loop) => loop.loop_id),
    `${caseName}: routing-only config must not spawn any loop beyond the dispatch coordinator`,
  ).toEqual([expectedLoopId]);
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
    `${caseName}: coordinator should query sandbox readiness before choosing the action token`,
  ).toContain("query_sandbox_attestation");
  expect(
    loopCalls ?? [],
    `${caseName}: coordinator should request sandbox provisioning before choosing the action token`,
  ).toContain("request_sandbox");
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

function subjectIncludesLoop(triple: Triple, loopId: string): boolean {
  return String(triple.subject ?? "").includes(loopId);
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
