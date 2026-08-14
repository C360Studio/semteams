import { describe, it, expect, vi, beforeEach } from "vitest";
import { deriveRunStatuses, runStatus } from "./runStatus.svelte";
import { getTriples } from "$lib/services/runStatusApi";
import type { RawTriple } from "$lib/services/runStatusApi";
import { RUN_HEALTH_PREDICATES } from "$lib/utils/runHealth";

vi.mock("$lib/services/runStatusApi");
const mockGetTriples = vi.mocked(getTriples);

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

const ORG = "c360.semteams";

function runEntity(runId: string): string {
  return `${ORG}.agent.chain.execution.${runId}`;
}

function loopEntity(loopId: string): string {
  return `${ORG}.agent.agentic-loop.execution.${loopId}`;
}

// approval_pending.object is the FULL 6-part loop entity ref — the
// cmd/semteams/approvalpause subscriber stamps TryLoopExecutionEntityID(loopID).
// Contrast clarTriple (bare). The store normalizes both via toBareLoopId.
function approvalTriple(runId: string, gatedLoopId: string): RawTriple {
  return {
    subject: runEntity(runId),
    predicate: "agent.run.approval-pending",
    object: loopEntity(gatedLoopId),
  };
}

// NOTE: clarification_pending.object is the BARE loop UUID — configs/rules/
// agent-run/07,08 stamp `$entity.instance` (the bare id per
// execution_context.go), NOT the full entity ref. This asymmetry vs
// approval_pending (full ref, see approvalTriple) is the real backend shape
// the store must normalize. Pinned by "marker object forms are asymmetric".
function clarTriple(runId: string, askingLoopId: string): RawTriple {
  return {
    subject: runEntity(runId),
    predicate: "agent.run.clarification-pending",
    object: askingLoopId,
  };
}

function questionTriple(askingLoopId: string, question: string): RawTriple {
  return {
    subject: loopEntity(askingLoopId),
    predicate: "coordinator.clarification.question",
    object: question,
  };
}

// ---------------------------------------------------------------------------
// deriveRunStatuses — pure parse logic
// ---------------------------------------------------------------------------

describe("deriveRunStatuses", () => {
  it("returns an empty map when all triple arrays are empty", () => {
    const result = deriveRunStatuses([], [], []);
    expect(result.size).toBe(0);
  });

  // -------------------------------------------------------------------------
  // tool_gate (approval_pending)
  // -------------------------------------------------------------------------

  it("classifies approval_pending as tool_gate pause", () => {
    const approval = [approvalTriple("run-abc", "loop-gated-1")];
    const result = deriveRunStatuses(approval, [], []);

    expect(result.size).toBe(1);
    const status = result.get("run-abc");
    expect(status).toBeDefined();
    expect(status!.runId).toBe("run-abc");
    expect(status!.pause).toEqual({
      cause: "tool_gate",
      gatedLoopId: "loop-gated-1",
    });
  });

  it("parses the bare gated-loop id from a full entity id", () => {
    // Full org-qualified entity id — bareIdAfter must strip correctly.
    const result = deriveRunStatuses(
      [approvalTriple("run-xyz", "loop-gate-42")],
      [],
      [],
    );
    expect(result.get("run-xyz")!.pause).toMatchObject({
      cause: "tool_gate",
      gatedLoopId: "loop-gate-42",
    });
  });

  // -------------------------------------------------------------------------
  // clarification (clarification_pending + coordinator.clarification.question)
  // -------------------------------------------------------------------------

  it("classifies clarification_pending as clarification pause with question", () => {
    const clar = [clarTriple("run-def", "loop-asking-2")];
    const questions = [questionTriple("loop-asking-2", "What timeout should I use?")];
    const result = deriveRunStatuses([], clar, questions);

    const status = result.get("run-def");
    expect(status).toBeDefined();
    expect(status!.pause).toEqual({
      cause: "clarification",
      askingLoopId: "loop-asking-2",
      question: "What timeout should I use?",
    });
  });

  it("falls back to empty string when question triple is absent", () => {
    const clar = [clarTriple("run-nogq", "loop-noquestion")];
    const result = deriveRunStatuses([], clar, []); // no question triples

    expect(result.get("run-nogq")!.pause).toEqual({
      cause: "clarification",
      askingLoopId: "loop-noquestion",
      question: "",
    });
  });

  it("correlates question to the right asking loop when multiple clarifications exist", () => {
    const clar = [
      clarTriple("run-1", "loop-ask-a"),
      clarTriple("run-2", "loop-ask-b"),
    ];
    const questions = [
      questionTriple("loop-ask-a", "Question A?"),
      questionTriple("loop-ask-b", "Question B?"),
    ];
    const result = deriveRunStatuses([], clar, questions);

    expect(result.get("run-1")!.pause).toMatchObject({
      cause: "clarification",
      question: "Question A?",
    });
    expect(result.get("run-2")!.pause).toMatchObject({
      cause: "clarification",
      question: "Question B?",
    });
  });

  // -------------------------------------------------------------------------
  // Both markers present — prefer tool_gate
  // -------------------------------------------------------------------------

  it("prefers tool_gate when both markers are present for the same run", () => {
    const approval = [approvalTriple("run-both", "loop-gated-x")];
    const clar = [clarTriple("run-both", "loop-asking-x")];
    const questions = [questionTriple("loop-asking-x", "Question?")];
    const result = deriveRunStatuses(approval, clar, questions);

    // tool_gate takes precedence per spec disambiguation
    expect(result.get("run-both")!.pause!.cause).toBe("tool_gate");
  });

  // -------------------------------------------------------------------------
  // Absent = not paused
  // -------------------------------------------------------------------------

  it("excludes a run entity id that lacks both markers", () => {
    // question triple exists but no clarification or approval marker
    const questions = [questionTriple("loop-orphan", "orphan question")];
    const result = deriveRunStatuses([], [], questions);
    expect(result.size).toBe(0);
  });

  it("skips triples whose subject does not contain the run entity infix", () => {
    // A triple that happens to carry approval_pending but on a NON-run entity
    const badTriple: RawTriple = {
      subject: `${ORG}.agent.agentic-loop.execution.some-loop`,
      predicate: "agent.run.approval-pending",
      object: loopEntity("gate-99"),
    };
    const result = deriveRunStatuses([badTriple], [], []);
    expect(result.size).toBe(0);
  });

  // -------------------------------------------------------------------------
  // Multiple runs in parallel
  // -------------------------------------------------------------------------

  it("handles multiple paused runs simultaneously", () => {
    const approval = [approvalTriple("run-gate", "loop-gate-1")];
    const clar = [clarTriple("run-clar", "loop-clar-1")];
    const questions = [questionTriple("loop-clar-1", "How many retries?")];
    const result = deriveRunStatuses(approval, clar, questions);

    expect(result.size).toBe(2);
    expect(result.get("run-gate")!.pause!.cause).toBe("tool_gate");
    expect(result.get("run-clar")!.pause!.cause).toBe("clarification");
  });

  it("attaches graph health facts for runs with lifecycle/proof triples", () => {
    const health: RawTriple[] = [
      {
        subject: runEntity("run-health"),
        predicate: "agent.run.phase",
        object: "executing",
      },
      {
        subject: runEntity("run-health"),
        predicate: "proof_readiness.route",
        object: "test_harness",
      },
    ];
    const result = deriveRunStatuses([], [], [], health);

    const status = result.get("run-health");
    expect(status).toBeDefined();
    expect(status!.pause).toBeNull();
    expect(status!.healthFacts).toMatchObject({
      phase: "executing",
      proofReadinessRoute: "test_harness",
    });
  });

  it("derives freshness from subject-scoped proof triples", () => {
    const health: RawTriple[] = [
      {
        subject: runEntity("run-freshness"),
        predicate: "agent.run.phase",
        object: "executing",
      },
      {
        subject: runEntity("run-freshness"),
        predicate: "proof.readiness.sitl.status",
        object: "stale",
      },
    ];
    const result = deriveRunStatuses([], [], [], health);

    expect(result.get("run-freshness")!.healthFacts).toMatchObject({
      phase: "executing",
      evidenceFreshness: "stale",
      staleEvidenceCount: 1,
    });
  });

  // -------------------------------------------------------------------------
  // Bare-id extraction edge cases
  // -------------------------------------------------------------------------

  it("skips a run entity id that contains the infix but yields an empty bare id", () => {
    // Subject ends right at the infix (nothing after it)
    const badRunEntity: RawTriple = {
      subject: `${ORG}.agent.chain.execution.`,
      predicate: "agent.run.approval-pending",
      object: loopEntity("loop-x"),
    };
    const result = deriveRunStatuses([badRunEntity], [], []);
    // bareIdAfter returns "" → skipped
    expect(result.size).toBe(0);
  });

  // -------------------------------------------------------------------------
  // Marker-object asymmetry — the regression that the green unit tests masked.
  // clarification_pending stamps the BARE loop id ($entity.instance); the UI
  // must NOT re-strip it (that yielded "" → empty in_reply_to → run never
  // resumed). approval_pending stamps the FULL ref. Both must normalize to the
  // same bare id and the question must still join.
  // -------------------------------------------------------------------------

  it("recovers the bare asking-loop id from a clarification marker that is already bare", () => {
    // Real backend shape: object is the bare UUID, question subject is full.
    const clar = [clarTriple("run-z", "loop-bare-9")];
    const questions = [questionTriple("loop-bare-9", "Which region?")];
    const result = deriveRunStatuses([], clar, questions);

    const pause = result.get("run-z")!.pause!;
    expect(pause).toEqual({
      cause: "clarification",
      askingLoopId: "loop-bare-9", // bare, non-empty → valid in_reply_to anchor
      question: "Which region?", // joined via bare id, not the full subject
    });
  });

  it("normalizes a clarification marker even if the backend later stamps the full ref", () => {
    // Defensive: if the 07/08 asymmetry is fixed upstream to stamp $entity.id,
    // toBareLoopId still recovers the bare id and the question still joins.
    const clarFullRef: RawTriple = {
      subject: runEntity("run-fwd"),
      predicate: "agent.run.clarification-pending",
      object: loopEntity("loop-fwd-1"), // FULL ref (hypothetical future shape)
    };
    const questions = [questionTriple("loop-fwd-1", "Proceed?")];
    const result = deriveRunStatuses([], [clarFullRef], questions);

    expect(result.get("run-fwd")!.pause).toEqual({
      cause: "clarification",
      askingLoopId: "loop-fwd-1",
      question: "Proceed?",
    });
  });
});

// ---------------------------------------------------------------------------
// Store poll lifecycle — the in-flight guard + error capture that the pure
// deriveRunStatuses tests can't reach. Drives pollOnce() directly with a
// mocked getTriples (no real timers).
// ---------------------------------------------------------------------------

describe("runStatus store — poll lifecycle", () => {
  beforeEach(() => {
    runStatus.stop(); // reset the in-flight guard between tests
    mockGetTriples.mockReset();
  });

  it("ignores a concurrent pollOnce while one is in flight (in-flight guard)", async () => {
    let resolveFirst!: (v: RawTriple[]) => void;
    const pending = new Promise<RawTriple[]>((r) => {
      resolveFirst = r;
    });
    mockGetTriples.mockReturnValue(pending);

    const p1 = runStatus.pollOnce();
    const p2 = runStatus.pollOnce(); // must no-op while p1 is in flight

    // Only the first poll's predicate fetches were issued — the second
    // call short-circuited on the guard.
    expect(mockGetTriples).toHaveBeenCalledTimes(3 + RUN_HEALTH_PREDICATES.length);

    resolveFirst([]);
    await Promise.all([p1, p2]);
  });

  it("captures a poll error in lastError without throwing", async () => {
    mockGetTriples.mockRejectedValue(new Error("triples endpoint down"));
    await expect(runStatus.pollOnce()).resolves.toBeUndefined();
    expect(runStatus.lastError).toBe("triples endpoint down");
  });

  it("clears lastError after a subsequent successful poll", async () => {
    mockGetTriples.mockRejectedValueOnce(new Error("transient"));
    await runStatus.pollOnce();
    expect(runStatus.lastError).toBe("transient");

    mockGetTriples.mockResolvedValue([]);
    await runStatus.pollOnce();
    expect(runStatus.lastError).toBeNull();
  });

  it("fetches full run-subject triples for dynamic proof evidence", async () => {
    const run = runEntity("run-dynamic");
    mockGetTriples.mockImplementation((params) => {
      if (params.predicate === "agent.run.phase") {
        return Promise.resolve([
          { subject: run, predicate: "agent.run.phase", object: "executing" },
        ]);
      }
      if (params.subject === run) {
        return Promise.resolve([
          { subject: run, predicate: "agent.run.phase", object: "executing" },
          { subject: run, predicate: "proof.readiness.sitl.status", object: "stale" },
        ]);
      }
      return Promise.resolve([]);
    });

    await runStatus.pollOnce();

    expect(mockGetTriples).toHaveBeenCalledWith(
      expect.objectContaining({ subject: run, limit: 500 }),
    );
    expect(runStatus.get("run-dynamic")?.healthFacts).toMatchObject({
      phase: "executing",
      evidenceFreshness: "stale",
      staleEvidenceCount: 1,
    });
  });
});
