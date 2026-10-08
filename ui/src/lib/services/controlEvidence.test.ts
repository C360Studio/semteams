import { describe, it, expect, vi, beforeEach } from "vitest";
import { loadControlEvidence, LOOP_READ_LIMIT, RUN_READ_LIMIT } from "./controlEvidence";
import { getTriples } from "./runStatusApi";
import type { RawTriple } from "./runStatusApi";
import type { Control } from "$lib/types/control";

vi.mock("./runStatusApi");
const mockGetTriples = vi.mocked(getTriples);

const PREFIX = "c360.semteams-bootstrap-e2e-6c50f3";
const RUN_ENTITY = `${PREFIX}.chain.agent.execution.run-1`;

function control(loopId: string, nanos = 1791480613893413505n): Control {
  return {
    loopId,
    loopEntityId: `${PREFIX}.agentic-loop.agent.execution.${loopId}`,
    firingEntityId: RUN_ENTITY,
    runInstance: "run-1",
    spawnedAt: new Date(Number(nanos / 1_000_000n)),
    spawnedAtNanos: nanos,
  };
}

function completedTransition(): RawTriple[] {
  const timestamp = "2026-10-08T17:30:13.846566421Z";
  const base = { subject: RUN_ENTITY, timestamp };
  return [
    { ...base, predicate: "lifecycle.transition.at", object: timestamp },
    { ...base, predicate: "lifecycle.transition.from", object: "executing" },
    { ...base, predicate: "lifecycle.transition.to", object: "completed" },
    { ...base, predicate: "lifecycle.transition.source", object: "rule" },
    { ...base, predicate: "lifecycle.transition.note", object: "run completed" },
  ];
}

beforeEach(() => {
  mockGetTriples.mockReset();
});

describe("loadControlEvidence", () => {
  it("reads the firing run entity once and each control loop entity once", async () => {
    mockGetTriples.mockImplementation((params) =>
      Promise.resolve(params.subject === RUN_ENTITY ? completedTransition() : []),
    );
    const a = control("loop-a");
    const b = control("loop-b");

    const result = await loadControlEvidence([a, b]);

    const subjects = mockGetTriples.mock.calls.map(([params]) => params.subject).sort();
    expect(subjects).toEqual([RUN_ENTITY, a.loopEntityId, b.loopEntityId].sort());
    expect(mockGetTriples).toHaveBeenCalledWith(expect.objectContaining({ subject: RUN_ENTITY, limit: RUN_READ_LIMIT }));
    expect(mockGetTriples).toHaveBeenCalledWith(expect.objectContaining({ subject: a.loopEntityId, limit: LOOP_READ_LIMIT }));
    expect(result["loop-a"].fact).toMatchObject({ status: "known", value: { to: "completed" } });
    expect(result["loop-b"].fact).toMatchObject({ status: "known", value: { to: "completed" } });
  });

  it("treats a run read that fills its limit as truncated: read truncated", async () => {
    const filler = Array.from({ length: RUN_READ_LIMIT }, (_, i): RawTriple => ({
      subject: RUN_ENTITY,
      predicate: "lifecycle.transition.note",
      object: `n${i}`,
      timestamp: `2026-10-08T17:30:13.${String(100000000 + i)}Z`,
    }));
    mockGetTriples.mockImplementation((params) =>
      Promise.resolve(params.subject === RUN_ENTITY ? filler : []),
    );

    const result = await loadControlEvidence([control("loop-a")]);

    expect(result["loop-a"].fact).toEqual({ status: "unknown", reason: "read truncated" });
  });

  it("turns a failed read into unknown fields instead of throwing", async () => {
    mockGetTriples.mockImplementation((params) =>
      params.subject === RUN_ENTITY
        ? Promise.reject(new Error("HTTP 503"))
        : Promise.resolve([]),
    );

    const result = await loadControlEvidence([control("loop-a")]);

    expect(result["loop-a"].fact).toEqual({ status: "unknown", reason: "read failed: HTTP 503" });
    expect(result["loop-a"].nextAction.status).toBe("unknown");
  });

  it("lets an abort unwind rather than caching it as an answer", async () => {
    mockGetTriples.mockRejectedValue(new DOMException("aborted", "AbortError"));
    await expect(loadControlEvidence([control("loop-a")])).rejects.toMatchObject({ name: "AbortError" });
  });

  it("passes the abort signal to every read", async () => {
    mockGetTriples.mockResolvedValue([]);
    const ctrl = new AbortController();
    await loadControlEvidence([control("loop-a")], ctrl.signal);
    for (const [params] of mockGetTriples.mock.calls) expect(params.signal).toBe(ctrl.signal);
  });
});
