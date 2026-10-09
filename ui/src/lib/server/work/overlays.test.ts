// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";
import type { LinkedRun } from "$lib/types/work";
import {
  NOT_YET_STAMPED_REASON,
  PROMPT_BINDING_UNMATCHED_REASON,
  READ_TIMEOUT_MS,
  resolveLinkedRuns,
  VERIFICATION_UNKNOWN_REASON,
} from "./overlays";
import type { RunsResolution } from "./overlays";
import { fakeBackend, requestedUrls, stubBackend, triple } from "./testkit";
import type { FakeTriple, FakeWorld } from "./testkit";
import type { LinkedRunBinding } from "./source";

const HOST = "backend:8080";
const PROMPT =
  "Compare MQTT vs NATS for IoT edge deployments — which has lower latency on constrained ARM devices?";
const LOOP_ID = "loop_7b3b867e";
const LOOP_ENTITY = `c360.semteams-e2e.agentic-loop.agent.execution.${LOOP_ID}`;
const RUN = `c360.semteams-e2e.chain.agent.execution.${LOOP_ID}`;
const DEAD_RUN = "c360.semteams.chain.agent.execution.00000000-0000-4000-8000-00000000dead";

const byPrompt: LinkedRunBinding = { by: "coordinator_prompt", equals: PROMPT };
const byRun = (value: string): LinkedRunBinding => ({ by: "run_entity_id", value });

/** Graph facts for a coordinator found by prompt and the run it minted. */
function coordinatorFacts(extra: FakeTriple[] = [], opts: { stamped?: boolean } = {}): FakeTriple[] {
  const stamped = opts.stamped ?? true;
  return [
    triple(LOOP_ENTITY, "agent.loop.description", PROMPT),
    triple(LOOP_ENTITY, "agent.loop.role", "coordinator"),
    triple(LOOP_ENTITY, "agent.loop.run", LOOP_ID),
    ...(stamped ? [triple(LOOP_ENTITY, "agent.run.entity-id", RUN)] : []),
    ...extra,
  ];
}

const completedRun = (): FakeTriple[] => [
  triple(RUN, "agent.run.phase", "completed"),
  triple(RUN, "agent.run.outcome", "success"),
  triple(RUN, "agent.run.handoff", LOOP_ID),
];

/**
 * The contract: "no linked run" (`complete` with no runs) is only the answer for
 * an item that declares no bindings. A declared binding that found nothing
 * could not be proven empty, so it must surface as a gap, never as that answer.
 */
function expectNoLinkedRunOnlyForEmptyBindings(resolution: RunsResolution, bindings: LinkedRunBinding[]) {
  if (resolution.lookup === "complete" && resolution.runs.length === 0) {
    expect(bindings, "complete with no runs for a declared binding").toEqual([]);
  }
}

async function resolve(bindings: LinkedRunBinding[], world: FakeWorld) {
  const fetchMock = stubBackend(fakeBackend(world));
  const resolution = await resolveLinkedRuns(bindings, HOST);
  expectNoLinkedRunOnlyForEmptyBindings(resolution, bindings);
  return { resolution, fetchMock };
}

/** `count` triples on `subject` that carry no fact the overlays read. */
const filler = (subject: string, count: number): FakeTriple[] =>
  Array.from({ length: count }, (_, i) => triple(subject, `test.filler.p${i}`, "x"));

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("coordinator_prompt binding", () => {
  it("resolves through graph facts to a complete, fully known run", async () => {
    const { resolution, fetchMock } = await resolve([byPrompt], {
      triples: [...coordinatorFacts(), ...completedRun()],
      loops: { [LOOP_ID]: { loop_id: LOOP_ID, role: "coordinator", state: "complete", outcome: "success" } },
    });

    expect(resolution.lookup).toBe("complete");
    expect(resolution.reason).toBeUndefined();
    expect(resolution.runs).toEqual<LinkedRun[]>([
      {
        run_entity_id: RUN,
        coordinator_loop_id: LOOP_ID,
        lookup: "complete",
        execution_stage: { state: "known", value: "completed" },
        needs_you: { state: "known", value: false },
        verification: { state: "unknown", reason: VERIFICATION_UNKNOWN_REASON },
      },
    ]);

    // The loops list carries no prompt, metadata or parent on this runtime, so
    // it must never be listed; the per-loop read is for state only.
    const urls = requestedUrls(fetchMock);
    expect(urls.some((u) => u.pathname === "/teams-dispatch/loops")).toBe(false);
    expect(urls.filter((u) => u.pathname.startsWith("/teams-dispatch/loops/")).map((u) => u.pathname)).toEqual([
      `/teams-dispatch/loops/${LOOP_ID}`,
    ]);
    const listing = urls.find((u) => u.searchParams.get("predicate") === "agent.loop.description");
    expect(listing?.searchParams.get("object")).toBe(PROMPT);
    expect(listing?.searchParams.get("limit")).toBe("50");
  });

  it("is a linked run with stage unknown ('run entity not yet stamped') when the coordinator has no run anchor yet", async () => {
    const { resolution } = await resolve([byPrompt], {
      triples: coordinatorFacts([], { stamped: false }),
      loops: { [LOOP_ID]: { loop_id: LOOP_ID, state: "exploring" } },
    });

    expect(resolution.lookup).toBe("partial");
    expect(resolution.reason).toContain(NOT_YET_STAMPED_REASON);
    expect(resolution.runs).toHaveLength(1);
    expect(resolution.runs[0]).toMatchObject({
      run_entity_id: null,
      coordinator_loop_id: LOOP_ID,
      lookup: "partial",
      execution_stage: { state: "unknown", reason: NOT_YET_STAMPED_REASON },
      // The loop is readable and idle, but run markers are not: not knowable as false.
      needs_you: { state: "unknown" },
    });
  });

  it("still reports needs-you true from the loop alone while the run is not yet stamped", async () => {
    const { resolution } = await resolve([byPrompt], {
      triples: coordinatorFacts([], { stamped: false }),
      loops: { [LOOP_ID]: { loop_id: LOOP_ID, state: "awaiting_approval" } },
    });
    expect(resolution.runs[0].needs_you).toMatchObject({ state: "known", value: true });
    expect(resolution.runs[0].execution_stage.state).toBe("unknown");
  });

  it("matches only coordinators: a researcher with the same description is not the work item's run, and the binding is partial", async () => {
    const researcher = "c360.semteams-e2e.agentic-loop.agent.execution.loop_researcher";
    const { resolution } = await resolve([byPrompt], {
      triples: [
        triple(researcher, "agent.loop.description", PROMPT),
        triple(researcher, "agent.loop.role", "researcher-research-gather"),
        triple(researcher, "agent.run.entity-id", RUN),
      ],
    });
    // Not "no linked run": a declared binding that matched no coordinator is a gap.
    expect(resolution).toEqual({ lookup: "partial", reason: PROMPT_BINDING_UNMATCHED_REASON, runs: [] });
  });

  it("is partial, not 'no linked run', when no loop has that prompt (the frozen triples read answers [] when it cannot read)", async () => {
    const { resolution } = await resolve([byPrompt], { triples: [] });
    expect(resolution).toEqual({
      lookup: "partial",
      reason: "prompt binding matched no coordinator loop",
      runs: [],
    });
  });

  it("keeps the unmatched binding visible next to a binding that did resolve", async () => {
    const { resolution } = await resolve(
      [{ by: "coordinator_prompt", equals: "a prompt nothing ran" }, byRun(RUN)],
      { triples: completedRun(), loops: { [LOOP_ID]: { state: "complete" } } },
    );
    expect(resolution.lookup).toBe("partial");
    expect(resolution.reason).toBe(PROMPT_BINDING_UNMATCHED_REASON);
    expect(resolution.runs.map((r) => r.run_entity_id)).toEqual([RUN]);
  });

  it("is partial when the prompt listing is cut off at its limit, even if the coordinator was found", async () => {
    const others = Array.from({ length: 49 }, (_, i) => {
      const entity = `c360.semteams-e2e.agentic-loop.agent.execution.loop_other_${i}`;
      return triple(entity, "agent.loop.description", PROMPT);
    });
    const { resolution } = await resolve([byPrompt], {
      // The coordinator is inside the first 50; the 51st would not be.
      triples: [...coordinatorFacts(), ...completedRun(), ...others, triple(`${LOOP_ENTITY}x`, "agent.loop.description", PROMPT)],
      loops: { [LOOP_ID]: { loop_id: LOOP_ID, state: "complete" } },
    });
    expect(resolution.lookup).toBe("partial");
    expect(resolution.reason).toContain("read truncated at 50 triples");
    // What was read is still reported.
    expect(resolution.runs.map((r) => r.run_entity_id)).toEqual([RUN]);
  });

  it("is complete (not truncated) when the prompt listing is under its limit", async () => {
    const { resolution } = await resolve([byPrompt], {
      triples: [...coordinatorFacts(), ...completedRun()],
      loops: { [LOOP_ID]: { loop_id: LOOP_ID, state: "complete" } },
    });
    expect(resolution.lookup).toBe("complete");
  });

  it("is partial, without inventing a run, when a loop's own facts are cut off at their limit", async () => {
    const { resolution } = await resolve([byPrompt], {
      // The role and run anchor sit beyond the 200th triple, so this loop can be
      // neither claimed as the coordinator nor ruled out as a researcher.
      triples: [
        triple(LOOP_ENTITY, "agent.loop.description", PROMPT),
        ...filler(LOOP_ENTITY, 199),
        triple(LOOP_ENTITY, "agent.loop.role", "coordinator"),
        triple(LOOP_ENTITY, "agent.run.entity-id", RUN),
      ],
    });
    expect(resolution.lookup).toBe("partial");
    expect(resolution.reason).toContain(`loop facts for ${LOOP_ID}: read truncated at 200 triples`);
    expect(resolution.runs).toEqual([]);
  });

  it("prefers the coordinator's own run anchor over an inherited one", async () => {
    const inherited = "c360.semteams-e2e.chain.agent.execution.older-run";
    const { resolution } = await resolve([byPrompt], {
      triples: [
        ...coordinatorFacts([triple(LOOP_ENTITY, "agent.run.entity-id", inherited)]).filter(
          // inherited anchor first, own anchor second, as a recovery coordinator carries them
          (t) => !(t.predicate === "agent.run.entity-id" && t.object === RUN),
        ),
        triple(LOOP_ENTITY, "agent.run.entity-id", RUN),
        ...completedRun(),
      ],
      loops: { [LOOP_ID]: { loop_id: LOOP_ID, state: "complete" } },
    });
    expect(resolution.runs).toHaveLength(1);
    expect(resolution.runs[0].run_entity_id).toBe(RUN);
  });

  it("reports failed when the prompt lookup itself cannot be read", async () => {
    stubBackend(() => new Response("boom", { status: 500 }));
    const resolution = await resolveLinkedRuns([byPrompt], HOST);
    expectNoLinkedRunOnlyForEmptyBindings(resolution, [byPrompt]);
    expect(resolution.lookup).toBe("failed");
    expect(resolution.reason).toContain("GET /graph/triples answered 500");
    expect(resolution.runs).toEqual([]);
  });
});

describe("run_entity_id binding", () => {
  it("is partial, naming the id, when the run entity has no facts (a real dangling reference)", async () => {
    const { resolution } = await resolve([byRun(DEAD_RUN)], { triples: [] });

    expect(resolution.lookup).toBe("partial");
    expect(resolution.reason).toContain(DEAD_RUN);
    expect(resolution.runs).toHaveLength(1);
    expect(resolution.runs[0]).toMatchObject({
      run_entity_id: DEAD_RUN,
      coordinator_loop_id: null,
      lookup: "partial",
      execution_stage: { state: "unknown" },
      needs_you: { state: "unknown" },
      verification: { state: "unknown" },
    });
    expect(resolution.runs[0].reason).toContain(DEAD_RUN);
  });

  it("takes the coordinator loop id from the run's agent.run.handoff (bare id)", async () => {
    const { resolution } = await resolve([byRun(RUN)], {
      triples: completedRun(),
      loops: { [LOOP_ID]: { loop_id: LOOP_ID, state: "complete" } },
    });
    expect(resolution.lookup).toBe("complete");
    expect(resolution.runs[0]).toMatchObject({ run_entity_id: RUN, coordinator_loop_id: LOOP_ID });
  });

  it("normalizes a full loop entity ref in agent.run.handoff to the bare id", async () => {
    const { resolution } = await resolve([byRun(RUN)], {
      triples: [triple(RUN, "agent.run.phase", "executing"), triple(RUN, "agent.run.handoff", LOOP_ENTITY)],
      loops: { [LOOP_ID]: { loop_id: LOOP_ID, state: "exploring" } },
    });
    expect(resolution.runs[0].coordinator_loop_id).toBe(LOOP_ID);
    expect(resolution.runs[0].execution_stage).toEqual({ state: "known", value: "executing" });
  });

  it("without a handoff the loop is unreadable: stage still known, needs-you unknown, lookup partial", async () => {
    const { resolution } = await resolve([byRun(RUN)], {
      triples: [triple(RUN, "agent.run.phase", "dispatched")],
    });
    expect(resolution.lookup).toBe("partial");
    expect(resolution.runs[0].execution_stage).toEqual({ state: "known", value: "dispatched" });
    expect(resolution.runs[0].needs_you.state).toBe("unknown");
    expect(resolution.runs[0].coordinator_loop_id).toBeNull();
  });

  it.each(["dispatched", "executing", "awaiting_approval", "completed", "failed", "cancelled"] as const)(
    "reads agent.run.phase %s",
    async (phase) => {
      const { resolution } = await resolve([byRun(RUN)], {
        triples: [triple(RUN, "agent.run.phase", phase), triple(RUN, "agent.run.handoff", LOOP_ID)],
        loops: { [LOOP_ID]: { loop_id: LOOP_ID, state: "complete" } },
      });
      expect(resolution.runs[0].execution_stage).toEqual({ state: "known", value: phase });
    },
  );

  it("keeps a run in awaiting_approval known and complete: a live phase is not a gap", async () => {
    const { resolution } = await resolve([byRun(RUN)], {
      triples: [
        triple(RUN, "agent.run.phase", "awaiting_approval"),
        triple(RUN, "agent.run.handoff", LOOP_ID),
        triple(RUN, "agent.run.approval-outstanding", 1),
      ],
      loops: { [LOOP_ID]: { loop_id: LOOP_ID, state: "awaiting_approval" } },
    });
    expect(resolution.lookup).toBe("complete");
    expect(resolution.reason).toBeUndefined();
    expect(resolution.runs[0].execution_stage).toEqual({ state: "known", value: "awaiting_approval" });
    expect(resolution.runs[0].needs_you).toMatchObject({ state: "known", value: true });
  });

  it("is partial, with stage unknown and the truncation reason, when the run's facts are cut off at their limit", async () => {
    const { resolution } = await resolve([byRun(RUN)], {
      // Phase and handoff sit beyond the 200th triple: last-wins over a cut-short
      // read could report a stale phase, and absence of a marker proves nothing.
      triples: [...filler(RUN, 200), ...completedRun()],
      loops: { [LOOP_ID]: { loop_id: LOOP_ID, state: "complete" } },
    });
    expect(resolution.lookup).toBe("partial");
    expect(resolution.reason).toContain(`run facts for ${RUN}: read truncated at 200 triples`);
    expect(resolution.runs[0].execution_stage).toMatchObject({ state: "unknown" });
    expect(resolution.runs[0].needs_you.state).toBe("unknown");
  });

  it("still reports needs-you true from a marker inside a truncated read (true is definitive)", async () => {
    const { resolution } = await resolve([byRun(RUN)], {
      triples: [triple(RUN, "agent.run.clarification-pending", LOOP_ID), ...filler(RUN, 199), ...completedRun()],
    });
    expect(resolution.lookup).toBe("partial");
    expect(resolution.runs[0].needs_you).toMatchObject({ state: "known", value: true });
    expect(resolution.runs[0].execution_stage.state).toBe("unknown");
  });

  it("is complete when the run's facts stop just short of the limit", async () => {
    const { resolution } = await resolve([byRun(RUN)], {
      triples: [...filler(RUN, 196), ...completedRun()],
      loops: { [LOOP_ID]: { loop_id: LOOP_ID, state: "complete" } },
    });
    expect(resolution.lookup).toBe("complete");
  });

  it("treats a phase outside the known set, or a missing phase, as unknown rather than guessing", async () => {
    const odd = await resolve([byRun(RUN)], {
      triples: [triple(RUN, "agent.run.phase", "paused"), triple(RUN, "agent.run.handoff", LOOP_ID)],
      loops: { [LOOP_ID]: { state: "complete" } },
    });
    expect(odd.resolution.runs[0].execution_stage.state).toBe("unknown");
    expect(odd.resolution.runs[0].execution_stage.reason).toContain("paused");
    expect(odd.resolution.lookup).toBe("partial");

    const missing = await resolve([byRun(RUN)], {
      triples: [triple(RUN, "agent.run.handoff", LOOP_ID)],
      loops: { [LOOP_ID]: { state: "complete" } },
    });
    expect(missing.resolution.runs[0].execution_stage.reason).toContain("agent.run.phase");
  });
});

describe("needs-you overlay", () => {
  const world = (loop: Record<string, unknown> | number | undefined, extra: FakeTriple[] = []): FakeWorld => ({
    triples: [...completedRun(), ...extra],
    loops: loop === undefined ? {} : { [LOOP_ID]: loop },
  });

  it("is known false only when the loop and the run markers were both read and none hold", async () => {
    const { resolution } = await resolve([byRun(RUN)], world({ loop_id: LOOP_ID, state: "exploring" }));
    expect(resolution.runs[0].needs_you).toEqual({ state: "known", value: false });
  });

  it("is true when the coordinator loop state is awaiting_approval", async () => {
    const { resolution } = await resolve([byRun(RUN)], world({ loop_id: LOOP_ID, state: "awaiting_approval" }));
    expect(resolution.runs[0].needs_you).toMatchObject({ state: "known", value: true });
  });

  it("is true when pending_approval is present on the loop", async () => {
    const { resolution } = await resolve(
      [byRun(RUN)],
      world({ loop_id: LOOP_ID, state: "exploring", pending_approval: { call_id: "c1", execution_id: "e1", tool_name: "bash" } }),
    );
    expect(resolution.runs[0].needs_you).toMatchObject({ state: "known", value: true });
    expect(resolution.runs[0].needs_you.reason).toContain("pending approval");
  });

  it("is true when the run carries agent.run.clarification-pending", async () => {
    const { resolution } = await resolve(
      [byRun(RUN)],
      world({ loop_id: LOOP_ID, state: "exploring" }, [triple(RUN, "agent.run.clarification-pending", LOOP_ID)]),
    );
    expect(resolution.runs[0].needs_you).toMatchObject({ state: "known", value: true });
    expect(resolution.runs[0].needs_you.reason).toContain("clarification");
  });

  it.each([
    ["1", true],
    [2, true],
    ["0", false],
    [0, false],
  ])("treats agent.run.approval-outstanding %j as needs-you %s", async (count, expected) => {
    const { resolution } = await resolve(
      [byRun(RUN)],
      world({ loop_id: LOOP_ID, state: "exploring" }, [triple(RUN, "agent.run.approval-outstanding", count)]),
    );
    expect(resolution.runs[0].needs_you).toMatchObject({ state: "known", value: expected });
  });

  it("is unknown, not false, when the loop read is a 404", async () => {
    const { resolution } = await resolve([byRun(RUN)], world(undefined));
    expect(resolution.runs[0].needs_you.state).toBe("unknown");
    expect(resolution.runs[0].needs_you.reason).toContain(`/teams-dispatch/loops/${LOOP_ID}`);
    expect(resolution.runs[0].needs_you.reason).toContain("404");
    // The run's own facts were fine.
    expect(resolution.runs[0].execution_stage).toEqual({ state: "known", value: "completed" });
    expect(resolution.lookup).toBe("partial");
  });

  it("is unknown when the loop read answers 503", async () => {
    const { resolution } = await resolve([byRun(RUN)], world(503));
    expect(resolution.runs[0].needs_you.state).toBe("unknown");
    expect(resolution.lookup).toBe("partial");
  });

  it("still reports true if a marker holds even though the loop read failed (true is definitive)", async () => {
    const { resolution } = await resolve(
      [byRun(RUN)],
      world(undefined, [triple(RUN, "agent.run.clarification-pending", LOOP_ID)]),
    );
    expect(resolution.runs[0].needs_you).toMatchObject({ state: "known", value: true });
  });
});

describe("failure handling", () => {
  it("degrades to failed, never none, when every read throws", async () => {
    stubBackend(() => {
      throw new Error("connect ECONNREFUSED");
    });
    const resolution = await resolveLinkedRuns([byRun(DEAD_RUN)], HOST);
    expectNoLinkedRunOnlyForEmptyBindings(resolution, [byRun(DEAD_RUN)]);
    expect(resolution.lookup).toBe("failed");
    expect(resolution.reason).toContain("ECONNREFUSED");
    // One failed read is reported once, not once per overlay.
    const parts = resolution.runs[0].reason?.split("; ") ?? [];
    expect(new Set(parts).size).toBe(parts.length);
    expect(resolution.runs).toHaveLength(1);
    expect(resolution.runs[0]).toMatchObject({
      lookup: "failed",
      execution_stage: { state: "unknown" },
      needs_you: { state: "unknown" },
    });
  });

  it("reports partial when one run resolves and another cannot be read", async () => {
    const other = `c360.semteams-e2e.chain.agent.execution.other`;
    const { resolution } = await resolve([byRun(RUN), byRun(other)], {
      triples: completedRun(),
      loops: { [LOOP_ID]: { state: "complete" } },
    });
    expect(resolution.lookup).toBe("partial");
    expect(resolution.runs.map((r) => r.lookup)).toEqual(["complete", "partial"]);
  });

  it("aborts a read after the timeout and reports it instead of hanging", async () => {
    vi.useFakeTimers();
    vi.stubGlobal(
      "fetch",
      vi.fn(
        (_input: RequestInfo | URL, init?: RequestInit) =>
          new Promise<Response>((_resolve, reject) => {
            init?.signal?.addEventListener("abort", () =>
              reject(Object.assign(new Error("This operation was aborted"), { name: "AbortError" })),
            );
          }),
      ),
    );
    const pending = resolveLinkedRuns([byRun(RUN)], HOST);
    await vi.advanceTimersByTimeAsync(READ_TIMEOUT_MS);
    const resolution = await pending;
    expect(resolution.lookup).toBe("failed");
    expect(resolution.reason).toContain(`timed out after ${READ_TIMEOUT_MS} ms`);
  });
});

describe("binding set", () => {
  it("the 'no linked run' assertion rejects a declared binding answered 'complete, no runs' (the B1 negative)", () => {
    expect(() => expectNoLinkedRunOnlyForEmptyBindings({ lookup: "complete", runs: [] }, [byPrompt])).toThrow();
    expect(() => expectNoLinkedRunOnlyForEmptyBindings({ lookup: "complete", runs: [] }, [])).not.toThrow();
    expect(() =>
      expectNoLinkedRunOnlyForEmptyBindings({ lookup: "partial", reason: "x", runs: [] }, [byPrompt]),
    ).not.toThrow();
  });

  it("makes no backend read for an empty binding list: complete, no runs, the one legal 'no linked run'", async () => {
    const { resolution, fetchMock } = await resolve([], {});
    expect(resolution).toEqual({ lookup: "complete", runs: [] });
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("lists a run once when two bindings resolve to it", async () => {
    const { resolution } = await resolve([byPrompt, byRun(RUN)], {
      triples: [...coordinatorFacts(), ...completedRun()],
      loops: { [LOOP_ID]: { state: "complete" } },
    });
    expect(resolution.runs).toHaveLength(1);
    expect(resolution.lookup).toBe("complete");
  });
});
