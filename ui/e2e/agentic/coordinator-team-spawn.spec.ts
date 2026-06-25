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
    name: "research prompt spawns the research planner",
    prompt:
      "Compare MQTT and NATS for constrained IoT edge deployments. I need the practical tradeoffs and evidence, not code.",
    action: "research",
    role: "researcher-research-plan",
  },
  {
    name: "OpenSpec prompt spawns the change author",
    prompt:
      "Create an OpenSpec change for adding Idempotency-Key support to POST /jobs. Include acceptance criteria and affected API contracts.",
    action: "create_change",
    role: "author-create-change",
  },
  {
    name: "bounded optimization prompt spawns autoresearch baseline",
    prompt:
      "Optimize `go test ./...` wallclock. Metric: trailing ok-line seconds, lower is better. Command: `go test ./...`. Surface: `./...`. Cap: 2 iterations. Guardrail: all tests must keep passing.",
    action: "autoresearch",
    role: "autoresearch-baseline",
  },
  {
    name: "build-with-tests prompt spawns Lisa",
    prompt:
      "Build a Go HTTP service that decodes MAVLink HEARTBEAT frames with github.com/bluenviron/gomavlib and serves the latest at GET /heartbeat as JSON, with unit tests.",
    action: "dev_via_test",
    role: "dev-via-test-plan",
  },
];

test.describe("coordinator team handoff matrix", () => {
  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok(), "Backend not healthy - stack not running?").toBe(true);
  });

  test.setTimeout(90_000);

  test("representative category prompts spawn the expected first role", async ({
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

      const coordinatorLoop = await waitForLoopWithDecision(
        request,
        beforeLoopIds,
        c.action,
      );
      expect(
        coordinatorLoop,
        `${c.name}: expected a new front-door coordinator loop`,
      ).toBeTruthy();

      const coordinatorTerminal = await waitForLoopTerminal(
        request,
        String(coordinatorLoop!.loop_id),
      );
      expect(
        coordinatorTerminal,
        `${c.name}: coordinator loop ${coordinatorLoop!.loop_id} did not reach terminal state`,
      ).toBeTruthy();
      expect(
        LOOP_FAILURE.has(coordinatorTerminal!.state ?? ""),
        `${c.name}: coordinator loop failed with state ${coordinatorTerminal!.state}`,
      ).toBe(false);

      const roleLoop = await waitForRoleLoop(request, beforeLoopIds, c.role);
      expect(
        roleLoop,
        `${c.name}: expected ${c.role} to spawn from coordinator action ${c.action}`,
      ).toBeTruthy();

      const roleTerminal = await waitForLoopTerminal(
        request,
        String(roleLoop!.loop_id),
      );
      expect(
        roleTerminal,
        `${c.name}: first role ${c.role} did not terminate under the handoff-only fixture`,
      ).toBeTruthy();
      expect(
        LOOP_FAILURE.has(roleTerminal!.state ?? ""),
        `${c.name}: first role ${c.role} failed with state ${roleTerminal!.state}`,
      ).toBe(false);

      await expectOnlyCoordinatorAndFirstRoleSpawned(
        request,
        beforeLoopIds,
        String(coordinatorLoop!.loop_id),
        String(roleLoop!.loop_id),
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

async function waitForLoopWithDecision(
  request: import("@playwright/test").APIRequestContext,
  beforeLoopIds: Set<string | undefined>,
  action: string,
): Promise<Loop | null> {
  return pollUntil(async () => {
    const [loops, actions] = await Promise.all([
      fetchLoops(request),
      fetchTriples(request, {
        predicate: "coordinator.decision.next_action",
        limit: 500,
      }),
    ]);
    return (
      loops
        .filter((loop) => !beforeLoopIds.has(loop.loop_id))
        .find((loop) =>
          actions.some(
            (triple) =>
              subjectIncludesLoop(triple, String(loop.loop_id)) &&
              String(triple.object ?? "") === action,
          ),
        ) ?? null
    );
  }, { timeoutMs: 20_000 });
}

async function waitForRoleLoop(
  request: import("@playwright/test").APIRequestContext,
  beforeLoopIds: Set<string | undefined>,
  role: string,
): Promise<Loop | null> {
  return pollUntil(async () => {
    const loops = await fetchLoops(request);
    return (
      loops.find(
        (loop) => !beforeLoopIds.has(loop.loop_id) && loop.role === role,
      ) ?? null
    );
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

async function expectOnlyCoordinatorAndFirstRoleSpawned(
  request: import("@playwright/test").APIRequestContext,
  beforeLoopIds: Set<string | undefined>,
  coordinatorLoopId: string,
  roleLoopId: string,
  caseName: string,
): Promise<void> {
  await new Promise((resolve) => setTimeout(resolve, 2_000));
  const loops = await fetchLoops(request);
  const spawnedIds = loops
    .filter((loop) => !beforeLoopIds.has(loop.loop_id))
    .map((loop) => String(loop.loop_id))
    .sort();

  expect(
    spawnedIds,
    `${caseName}: handoff-only config must spawn only the coordinator and expected first role`,
  ).toEqual([coordinatorLoopId, roleLoopId].sort());
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
