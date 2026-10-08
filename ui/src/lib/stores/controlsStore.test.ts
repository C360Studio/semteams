import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import {
  controlsStore,
  MAX_ATTEMPTS,
  MAX_READ_CONCURRENCY,
  MEMBERSHIP_READ_LIMIT,
  RETRY_BASE_MS,
} from "./controlsStore.svelte";
import { agentStore } from "./agentStore.svelte";
import { getTriples } from "$lib/services/runStatusApi";
import type { RawTriple } from "$lib/services/runStatusApi";
import type { AgentLoop } from "$lib/types/agent";

vi.mock("$lib/services/runStatusApi");
const mockGetTriples = vi.mocked(getTriples);

const PREFIX = "c360.semteams-bootstrap-e2e-6c50f3";
const RUN_ENTITY = `${PREFIX}.chain.agent.execution.run-1`;
const NANOS = "1791480613893413505";

function loopEntity(loopId: string): string {
  return `${PREFIX}.agentic-loop.agent.execution.${loopId}`;
}

function candidate(loopId: string, overrides: Partial<AgentLoop> = {}): AgentLoop {
  return {
    loop_id: loopId,
    task_id: `rule-${RUN_ENTITY}-${NANOS}`,
    state: "complete",
    role: "ops-chain-observer",
    iterations: 1,
    max_iterations: 5,
    user_id: "",
    channel_type: "",
    parent_loop_id: "",
    outcome: "",
    error: "",
    ...overrides,
  };
}

function entity(loopId: string, ...predicates: string[]): RawTriple[] {
  return predicates.map((predicate) => ({
    subject: loopEntity(loopId),
    predicate,
    object: predicate === "agent.loop.run" ? "run-1" : "x",
  }));
}

/** What the runtime wrote for a `run_scope: none` control such as the ops observer. */
const controlEntity = (id: string) => entity(id, "agent.loop.task", "agent.loop.role", "agent.lineage.root");
/** What it wrote for a member of a run, such as an autoresearch propose loop. */
const memberEntity = (id: string) => entity(id, "agent.loop.task", "agent.loop.role", "agent.loop.run");

function serve(byLoop: Record<string, RawTriple[]>) {
  mockGetTriples.mockImplementation(async (params) => {
    const id = params.subject?.slice(params.subject.lastIndexOf(".") + 1) ?? "";
    return byLoop[id] ?? [];
  });
}

beforeEach(() => {
  controlsStore.stop(); // reset the in-flight guard between tests
  controlsStore.reset();
  agentStore.reset();
  mockGetTriples.mockReset();
  mockGetTriples.mockResolvedValue([]);
});

afterEach(() => {
  vi.useRealTimers();
});

describe("controlsStore — membership cache", () => {
  it("reads the candidate's own loop entity, with a bounded limit, and not the agent.loop.task predicate", async () => {
    agentStore.updateLoop(candidate("observer-1"));
    serve({ "observer-1": controlEntity("observer-1") });
    await controlsStore.pollOnce();

    expect(mockGetTriples).toHaveBeenCalledTimes(1);
    const params = mockGetTriples.mock.calls[0][0];
    expect(params.subject).toBe(loopEntity("observer-1"));
    expect(params.predicate).toBeUndefined();
    expect(params.limit).toBe(MEMBERSHIP_READ_LIMIT);
  });

  it("resolves a loop without agent.loop.run as a control, grouped by its run", async () => {
    agentStore.updateLoop(candidate("observer-1"));
    serve({ "observer-1": controlEntity("observer-1") });
    await controlsStore.pollOnce();

    expect(controlsStore.controlsByRun["run-1"].map((c) => c.loopId)).toEqual(["observer-1"]);
    expect(controlsStore.controlLoopIds.has("observer-1")).toBe(true);
    expect(controlsStore.unclassifiedCount).toBe(0);
    expect(controlsStore.lastError).toBeNull();
  });

  it("resolves a loop with agent.loop.run as a run member, not a control", async () => {
    agentStore.updateLoop(candidate("propose-1", { role: "researcher" }));
    serve({ "propose-1": memberEntity("propose-1") });
    await controlsStore.pollOnce();

    expect(controlsStore.controlLoopIds.size).toBe(0);
    expect(controlsStore.snapshot.memberLoopIds.has("propose-1")).toBe(true);
  });

  it("never reads a resolved loop again, and reads each loop id once", async () => {
    agentStore.updateLoop(candidate("observer-1"));
    agentStore.updateLoop(candidate("propose-1"));
    serve({ "observer-1": controlEntity("observer-1"), "propose-1": memberEntity("propose-1") });

    await controlsStore.pollOnce();
    await controlsStore.pollOnce();
    await controlsStore.pollOnce();

    const subjects = mockGetTriples.mock.calls.map(([p]) => p.subject).sort();
    expect(subjects).toEqual([loopEntity("observer-1"), loopEntity("propose-1")].sort());
  });

  it("ignores loops that are not candidates: dispatch, loop-fired children, and parented loops", async () => {
    agentStore.updateLoop(candidate("front-door", { task_id: "dispatch-9f2c1a" }));
    agentStore.updateLoop(
      candidate("child-1", { task_id: `rule-${PREFIX}.agentic-loop.agent.execution.coord-${NANOS}` }),
    );
    agentStore.updateLoop(candidate("parented", { parent_loop_id: "coord" }));
    await controlsStore.pollOnce();

    expect(mockGetTriples).not.toHaveBeenCalled();
  });

  it("leaves a candidate unknown when its read fails, and retries it after a backoff", async () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    agentStore.updateLoop(candidate("observer-1"));
    mockGetTriples.mockRejectedValue(new Error("triples endpoint down"));
    await controlsStore.pollOnce();

    expect(controlsStore.controlLoopIds.size).toBe(0);
    expect(controlsStore.snapshot.unclassifiedLoopIds.has("observer-1")).toBe(true);
    expect(controlsStore.lastError).toBe("triples endpoint down");

    // Not due yet: the next tick must not read again.
    await controlsStore.pollOnce();
    expect(mockGetTriples).toHaveBeenCalledTimes(1);

    serve({ "observer-1": controlEntity("observer-1") });
    vi.setSystemTime(Date.now() + RETRY_BASE_MS + 1);
    await controlsStore.pollOnce();

    expect(controlsStore.controlLoopIds.has("observer-1")).toBe(true);
    expect(controlsStore.lastError).toBeNull();
    expect(controlsStore.unclassifiedCount).toBe(0);
  });

  it("treats a truncated read as unknown, never as a control", async () => {
    agentStore.updateLoop(candidate("observer-1"));
    const full = Array.from({ length: MEMBERSHIP_READ_LIMIT }, (_, i): RawTriple => ({
      subject: loopEntity("observer-1"),
      predicate: i === 0 ? "agent.loop.task" : `p.${i}`,
      object: "x",
    }));
    mockGetTriples.mockResolvedValue(full);
    await controlsStore.pollOnce();

    expect(controlsStore.controlLoopIds.size).toBe(0);
    expect(controlsStore.snapshot.unclassifiedLoopIds.has("observer-1")).toBe(true);
    expect(controlsStore.lastError).toBe("membership read truncated");
  });

  it("does not cache an unreadable entity as a non-member, and stays quiet about it until retries run out", async () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    agentStore.updateLoop(candidate("observer-1"));
    mockGetTriples.mockResolvedValue([]); // not projected yet
    await controlsStore.pollOnce();

    expect(controlsStore.controlLoopIds.size).toBe(0);
    // A moment of graph lag is not worth a board-level notice.
    expect(controlsStore.snapshot.unclassifiedLoopIds.size).toBe(0);
    expect(controlsStore.lastError).toBeNull();

    // It is retried, and once the entity is readable it resolves.
    serve({ "observer-1": controlEntity("observer-1") });
    vi.setSystemTime(Date.now() + RETRY_BASE_MS + 1);
    await controlsStore.pollOnce();
    expect(controlsStore.controlLoopIds.has("observer-1")).toBe(true);
  });

  it("gives up after a bounded number of attempts and then reports the candidate unclassified", async () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    agentStore.updateLoop(candidate("observer-1"));
    mockGetTriples.mockResolvedValue([]); // never becomes readable

    for (let attempt = 0; attempt < MAX_ATTEMPTS + 3; attempt++) {
      await controlsStore.pollOnce();
      vi.setSystemTime(Date.now() + RETRY_BASE_MS * 2 ** attempt + 1);
    }

    expect(mockGetTriples).toHaveBeenCalledTimes(MAX_ATTEMPTS);
    expect(controlsStore.snapshot.unclassifiedLoopIds.has("observer-1")).toBe(true);
    expect(controlsStore.lastError).toBe("loop entity not readable yet");
  });

  it("bounds read concurrency", async () => {
    for (let i = 0; i < 10; i++) agentStore.updateLoop(candidate(`observer-${i}`));
    let inFlight = 0;
    let peak = 0;
    mockGetTriples.mockImplementation(async (params) => {
      inFlight++;
      peak = Math.max(peak, inFlight);
      await new Promise((r) => setTimeout(r, 5));
      inFlight--;
      return controlEntity(params.subject!.slice(params.subject!.lastIndexOf(".") + 1));
    });
    await controlsStore.pollOnce();

    expect(mockGetTriples).toHaveBeenCalledTimes(10);
    expect(peak).toBeGreaterThan(1);
    expect(peak).toBeLessThanOrEqual(MAX_READ_CONCURRENCY);
    expect(controlsStore.controlLoopIds.size).toBe(10);
  });

  it("keeps the same snapshot object when nothing changed (no needless re-derivation)", async () => {
    agentStore.updateLoop(candidate("observer-1"));
    serve({ "observer-1": controlEntity("observer-1") });
    await controlsStore.pollOnce();
    const first = controlsStore.snapshot;
    await controlsStore.pollOnce();
    expect(controlsStore.snapshot).toBe(first);
  });

  it("ignores a concurrent pollOnce while one is in flight", async () => {
    agentStore.updateLoop(candidate("observer-1"));
    let resolveFirst!: (v: RawTriple[]) => void;
    mockGetTriples.mockReturnValue(new Promise<RawTriple[]>((r) => (resolveFirst = r)));

    const p1 = controlsStore.pollOnce();
    const p2 = controlsStore.pollOnce();
    expect(mockGetTriples).toHaveBeenCalledTimes(1);

    resolveFirst(controlEntity("observer-1"));
    await Promise.all([p1, p2]);
  });

  it("can name the loops that may still be controls, so refs are not spent on them", async () => {
    const observer = candidate("observer-1");
    const propose = candidate("propose-1");
    const plain = candidate("plain", { task_id: "dispatch-1" });
    agentStore.updateLoop(observer);
    agentStore.updateLoop(propose);
    agentStore.updateLoop(plain);

    // Before any read: both candidates might be controls; a plain loop never is.
    expect(controlsStore.mayBeControl(observer)).toBe(true);
    expect(controlsStore.mayBeControl(propose)).toBe(true);
    expect(controlsStore.mayBeControl(plain)).toBe(false);

    serve({ "observer-1": controlEntity("observer-1"), "propose-1": memberEntity("propose-1") });
    await controlsStore.pollOnce();

    expect(controlsStore.mayBeControl(observer)).toBe(true);
    expect(controlsStore.mayBeControl(propose)).toBe(false); // a member is a card
  });

  it("aborts in-flight reads on stop() without reporting an error or spending an attempt, and polls again afterwards", async () => {
    agentStore.updateLoop(candidate("observer-1"));
    let signal: AbortSignal | undefined;
    mockGetTriples.mockImplementation((params) => {
      signal = params.signal;
      return new Promise<RawTriple[]>((_, reject) => {
        params.signal?.addEventListener("abort", () =>
          reject(new DOMException("aborted", "AbortError")),
        );
      });
    });

    const pending = controlsStore.pollOnce();
    controlsStore.stop();
    await pending;

    expect(signal?.aborted).toBe(true);
    expect(controlsStore.lastError).toBeNull();
    expect(controlsStore.unclassifiedCount).toBe(0);

    // The aborted read did not count: the very next poll reads the loop again.
    serve({ "observer-1": controlEntity("observer-1") });
    await controlsStore.pollOnce();
    expect(controlsStore.controlLoopIds.has("observer-1")).toBe(true);
  });
});
