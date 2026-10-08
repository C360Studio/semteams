import { describe, it, expect, vi, beforeEach } from "vitest";
import { controlsStore, CONTROLS_READ_LIMIT } from "./controlsStore.svelte";
import { getTriples } from "$lib/services/runStatusApi";
import type { RawTriple } from "$lib/services/runStatusApi";

vi.mock("$lib/services/runStatusApi");
const mockGetTriples = vi.mocked(getTriples);

const PREFIX = "c360.semteams-bootstrap-e2e-6c50f3";
const RUN_ENTITY = `${PREFIX}.chain.agent.execution.run-1`;
const NANOS = "1791480613893413505";

function taskTriple(loopId: string, object: string): RawTriple {
  return {
    subject: `${PREFIX}.agentic-loop.agent.execution.${loopId}`,
    predicate: "agent.loop.task",
    object,
  };
}

beforeEach(async () => {
  controlsStore.stop(); // reset the in-flight guard between tests
  mockGetTriples.mockReset();
  mockGetTriples.mockResolvedValue([]);
  await controlsStore.pollOnce(); // clear any snapshot a previous test left
  mockGetTriples.mockClear(); // ...without counting that cleanup read
});

describe("controlsStore", () => {
  it("polls agent.loop.task with the documented limit", async () => {
    await controlsStore.pollOnce();
    expect(mockGetTriples).toHaveBeenLastCalledWith(
      expect.objectContaining({ predicate: "agent.loop.task", limit: 500 }),
    );
    expect(CONTROLS_READ_LIMIT).toBe(500);
  });

  it("exposes controls by run, the control loop ids, and the truncation flag", async () => {
    mockGetTriples.mockResolvedValue([
      taskTriple("observer-1", `rule-${RUN_ENTITY}-${NANOS}`),
      taskTriple("dispatch-1", "dispatch-abc"),
    ]);
    await controlsStore.pollOnce();

    expect(controlsStore.controlsByRun["run-1"].map((c) => c.loopId)).toEqual(["observer-1"]);
    expect(controlsStore.controlLoopIds.has("observer-1")).toBe(true);
    expect(controlsStore.controlLoopIds.has("dispatch-1")).toBe(false);
    expect(controlsStore.truncated).toBe(false);
  });

  it("treats a result that fills the limit as truncated", async () => {
    const full = Array.from({ length: CONTROLS_READ_LIMIT }, (_, i) =>
      taskTriple(`loop-${i}`, `dispatch-${i}`),
    );
    mockGetTriples.mockResolvedValue(full);
    await controlsStore.pollOnce();
    expect(controlsStore.truncated).toBe(true);
  });

  it("drops a control that disappears from the graph", async () => {
    mockGetTriples.mockResolvedValue([taskTriple("observer-1", `rule-${RUN_ENTITY}-${NANOS}`)]);
    await controlsStore.pollOnce();
    expect(controlsStore.controlLoopIds.size).toBe(1);

    mockGetTriples.mockResolvedValue([]);
    await controlsStore.pollOnce();
    expect(controlsStore.controlLoopIds.size).toBe(0);
  });

  it("keeps the same snapshot object when nothing changed (no needless re-derivation)", async () => {
    mockGetTriples.mockResolvedValue([taskTriple("observer-1", `rule-${RUN_ENTITY}-${NANOS}`)]);
    await controlsStore.pollOnce();
    const first = controlsStore.controlsByRun;
    await controlsStore.pollOnce();
    expect(controlsStore.controlsByRun).toBe(first);
  });

  it("ignores a concurrent pollOnce while one is in flight", async () => {
    let resolveFirst!: (v: RawTriple[]) => void;
    mockGetTriples.mockReturnValue(new Promise<RawTriple[]>((r) => (resolveFirst = r)));

    const p1 = controlsStore.pollOnce();
    const p2 = controlsStore.pollOnce();
    expect(mockGetTriples).toHaveBeenCalledTimes(1);

    resolveFirst([]);
    await Promise.all([p1, p2]);
  });

  it("captures a poll error and keeps the last good snapshot", async () => {
    mockGetTriples.mockResolvedValue([taskTriple("observer-1", `rule-${RUN_ENTITY}-${NANOS}`)]);
    await controlsStore.pollOnce();

    mockGetTriples.mockRejectedValue(new Error("triples endpoint down"));
    await expect(controlsStore.pollOnce()).resolves.toBeUndefined();
    expect(controlsStore.lastError).toBe("triples endpoint down");
    expect(controlsStore.controlLoopIds.has("observer-1")).toBe(true);

    mockGetTriples.mockResolvedValue([]);
    await controlsStore.pollOnce();
    expect(controlsStore.lastError).toBeNull();
  });

  it("aborts the in-flight read on stop() without reporting an error, and polls again afterwards", async () => {
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

    mockGetTriples.mockReset();
    mockGetTriples.mockResolvedValue([]);
    await controlsStore.pollOnce();
    expect(mockGetTriples).toHaveBeenCalledTimes(1);
  });
});
