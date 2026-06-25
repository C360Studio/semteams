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

const TERMINAL = new Set([
  "complete",
  "success",
  "failed",
  "error",
  "cancelled",
  "truncated",
]);

test.describe("OpenSpec 6.4 - autoresearch metric guardrails", () => {
  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok(), "Backend not healthy - stack not running?").toBe(true);
  });

  test.setTimeout(120_000);

  test("refuses vague objectives, then rejects guardrail-breaking metric wins", async ({
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
      .fill("Make the developer experience better.");
    await page.getByTestId("send-button").click();

    const refusalLoops = await pollUntil(async () => {
      const loops = await fetchLoops(request);
      if (loops.length !== 1) return null;
      return loops.every((loop) => TERMINAL.has(loop.state ?? ""))
        ? loops
        : null;
    }, { timeoutMs: 40_000 });
    expect(refusalLoops, "vague prompt should settle as one coordinator loop").toBeTruthy();

    let actions = await fetchTriples(request, {
      predicate: "coordinator.decision.next_action",
      limit: 50,
    });
    expect(values(actions), "vague prompt should ask for scalar framing").toContain(
      "ask_user",
    );
    expect(
      refusalLoops!.some((loop) => String(loop.role ?? "").startsWith("autoresearch")),
      "vague/non-scalar objective must not spawn the autoresearch arc",
    ).toBe(false);

    let replies = await fetchUserResponses(request);
    expect(replies.length, "ask_user should publish a user response").toBeGreaterThan(0);

    await page.getByTestId("chat-input").fill(
      "Optimize `go test ./...` wallclock. Metric: trailing ok-line seconds, lower is better. Command: `go test ./...`. Surface: `test/helpers/` and `internal/testutil/`. Cap: 2 iterations. Guardrail: all tests must keep passing; do not keep changes that skip coverage or fail the command.",
    );
    await page.getByTestId("send-button").click();

    const settled = await pollUntil(async () => {
      const loops = await fetchLoops(request);
      const n = (role: string) => loops.filter((loop) => loop.role === role).length;
      const allTerminal = loops.every((loop) => TERMINAL.has(loop.state ?? ""));
      if (
        n("autoresearch-baseline") >= 1 &&
        n("autoresearch-propose") >= 2 &&
        n("autoresearch-execute") >= 2 &&
        n("autoresearch-synthesize") >= 1 &&
        n("reviewer-autoresearch") >= 1 &&
        allTerminal
      ) {
        const actionTriples = await fetchTriples(request, {
          predicate: "coordinator.decision.next_action",
          limit: 100,
        });
        if (values(actionTriples).includes("respond_direct")) return loops;
      }
      return null;
    }, { timeoutMs: 100_000 });

    expect(
      settled,
      "scalar guarded prompt should run autoresearch to reviewed response",
    ).toBeTruthy();

    const loops = settled!;
    const failed = loops.filter((loop) =>
      ["failed", "error", "cancelled", "truncated"].includes(loop.state ?? ""),
    );
    expect(
      failed.map((loop) => `${loop.role}:${loop.state}`),
      "no loop should fail; guardrail failure is a measured/crashed iteration, not a chain crash",
    ).toEqual([]);

    const countRole = (role: string) => loops.filter((loop) => loop.role === role).length;
    expect(countRole("autoresearch-baseline"), "exactly one baseline").toBe(1);
    expect(countRole("autoresearch-propose"), "cap=2 -> two propose loops").toBe(2);
    expect(countRole("autoresearch-execute"), "cap=2 -> two execute loops").toBe(2);
    expect(countRole("autoresearch-synthesize"), "exactly one synthesize").toBe(1);
    expect(countRole("reviewer-autoresearch"), "exactly one reviewer").toBe(1);

    actions = await fetchTriples(request, {
      predicate: "coordinator.decision.next_action",
      limit: 100,
    });
    expect(values(actions)).toEqual(
      expect.arrayContaining([
        "ask_user",
        "autoresearch",
        "propose",
        "measure",
        "measured",
        "emit",
        "approved",
        "respond_direct",
      ]),
    );

    const measurementOutcomes = await fetchTriples(request, {
      predicate: "autoresearch.measurement.outcome",
      limit: 20,
    });
    expect(values(measurementOutcomes)).toEqual(
      expect.arrayContaining(["kept", "crashed"]),
    );
    expect(
      values(measurementOutcomes).filter((outcome) => outcome === "kept").length,
      "only the passing improvement should be kept",
    ).toBe(1);
    expect(
      values(measurementOutcomes).filter((outcome) => outcome === "crashed").length,
      "the apparent metric win that broke the pass gate should be recorded",
    ).toBe(1);

    const crashed = await measurementByOutcome(request, "crashed");
    expect(crashed, "expected a crashed measurement entity").toBeTruthy();
    expect(Number(crashed!.value), "crashed metric was numerically better").toBeCloseTo(
      0.1,
      5,
    );
    expect(String(crashed!.pass), "crashed measurement pass flag").toBe("false");
    expect(
      String(crashed!.stderrTail ?? ""),
      "guardrail violation should be audit evidence on the crashed measurement",
    ).toContain("guardrail violation");

    const best = await fetchTriples(request, {
      predicate: "autoresearch.best.value",
      limit: 10,
    });
    expect(best.length, "best.value should remain a single replace-owned scalar").toBe(1);
    expect(
      Number(best[0]?.object),
      "best.value must preserve the passing 1.00 result and reject failed 0.10",
    ).toBeCloseTo(1.0, 5);
    expect(Number(best[0]?.object)).not.toBeCloseTo(0.1, 5);

    const completedArtifact = await fetchTriples(request, {
      predicate: "autoresearch.artifact.iterations_completed",
      limit: 10,
    });
    expect(values(completedArtifact)).toContain("2");
    const keptArtifact = await fetchTriples(request, {
      predicate: "autoresearch.artifact.iterations_kept",
      limit: 10,
    });
    expect(values(keptArtifact)).toContain("1");
    const bestExperimentArtifact = await fetchTriples(request, {
      predicate: "autoresearch.artifact.best_experiment_id",
      limit: 10,
    });
    expect(values(bestExperimentArtifact)).toContain("iteration-1");

    replies = await fetchUserResponses(request);
    expect(
      replies.length,
      "ask_user + final respond_direct should both publish user responses",
    ).toBeGreaterThanOrEqual(2);
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

async function fetchUserResponses(
  request: import("@playwright/test").APIRequestContext,
): Promise<Array<{ subject: string }>> {
  const resp = await request.get(
    "/message-logger/entries?subject_prefix=dispatch.user.response&limit=20",
  );
  if (!resp.ok()) {
    throw new Error(`/message-logger/entries returned ${resp.status()}: ${await resp.text()}`);
  }
  return (await resp.json()) as Array<{ subject: string }>;
}

async function measurementByOutcome(
  request: import("@playwright/test").APIRequestContext,
  outcome: string,
): Promise<
  | {
      subject: string;
      value?: unknown;
      pass?: unknown;
      stderrTail?: unknown;
    }
  | undefined
> {
  const outcomes = await fetchTriples(request, {
    predicate: "autoresearch.measurement.outcome",
    limit: 20,
  });
  const match = outcomes.find((triple) => String(triple.object ?? "") === outcome);
  if (!match?.subject) return undefined;
  const triples = await fetchTriples(request, { subject: match.subject, limit: 100 });
  const byPredicate = new Map(triples.map((triple) => [triple.predicate, triple.object]));
  return {
    subject: match.subject,
    value: byPredicate.get("autoresearch.measurement.value"),
    pass: byPredicate.get("autoresearch.measurement.pass"),
    stderrTail: byPredicate.get("autoresearch.measurement.stderr_tail"),
  };
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
    await new Promise((resolve) => setTimeout(resolve, interval));
  }
  return null;
}

function values(triples: Triple[]): string[] {
  return triples.map((triple) => String(triple.object ?? ""));
}
