import { describe, expect, it } from "vitest";
import {
  guardNone,
  repositoryIds,
  RUNS_LOADING,
  RUNS_NOT_LOADED,
  slug,
  summarizeItemOverlays,
} from "./workView";
import type { LinkedRunsState } from "./workView";
import { makePortfolio, makeRun } from "../../test-utils/work";

const loaded = (runs: ReturnType<typeof makeRun>[], overrides: Partial<Extract<LinkedRunsState, { status: "loaded" }>> = {}) =>
  ({ status: "loaded", lookup: "complete", runs, ...overrides }) as LinkedRunsState;

describe("summarizeItemOverlays", () => {
  it("is unknown with a reason for every overlay until runs are loaded, never none", () => {
    for (const state of [undefined, { status: "idle" } as const]) {
      const summary = summarizeItemOverlays(state);
      for (const overlay of Object.values(summary)) {
        expect(overlay).toEqual({ state: "unknown", reason: RUNS_NOT_LOADED });
      }
    }
    expect(summarizeItemOverlays({ status: "loading" }).executionStage).toEqual({
      state: "unknown",
      reason: RUNS_LOADING,
    });
  });

  it("carries a failed read as unknown with its message", () => {
    const summary = summarizeItemOverlays({ status: "error", code: "HTTP_500", message: "boom" });

    expect(summary.needsYou).toEqual({ state: "unknown", reason: "linked runs unavailable: boom" });
  });

  it("is none only for a complete lookup that found no runs", () => {
    const summary = summarizeItemOverlays(loaded([]));

    expect(Object.values(summary).every((overlay) => overlay.state === "none")).toBe(true);
  });

  it.each(["partial", "failed", "unsupported"] as const)("is unknown, not none, for an empty %s lookup", (lookup) => {
    const summary = summarizeItemOverlays(loaded([], { lookup, reason: "no writer wired" }));

    expect(summary.executionStage).toEqual({ state: "unknown", reason: "no writer wired" });
    expect(summary.linkedRuns.state).toBe("unknown");
  });

  it("passes a single run's overlays through, reason included", () => {
    const run = makeRun({
      needs_you: { state: "known", value: true, reason: "coordinator loop is awaiting approval" },
    });
    const summary = summarizeItemOverlays(loaded([run]));

    expect(summary.executionStage).toEqual({ state: "known", value: "completed" });
    expect(summary.needsYou).toEqual(run.needs_you);
    expect(summary.linkedRuns).toEqual({ state: "known", value: 1 });
  });

  it("joins distinct stages across runs and lets one waiting run settle needs-you", () => {
    const summary = summarizeItemOverlays(
      loaded([
        makeRun({ execution_stage: { state: "known", value: "completed" } }),
        makeRun({
          execution_stage: { state: "known", value: "failed" },
          needs_you: { state: "known", value: true, reason: "approval outstanding" },
        }),
        makeRun({
          execution_stage: { state: "known", value: "completed" },
          needs_you: { state: "unknown", reason: "loop unreadable" },
        }),
      ]),
    );

    expect(summary.executionStage).toEqual({ state: "known", value: "completed, failed" });
    expect(summary.needsYou).toEqual({ state: "known", value: true, reason: "approval outstanding" });
    expect(summary.linkedRuns).toEqual({ state: "known", value: 3 });
  });

  it("is unknown when any run's value is unknown and nothing decisive is known", () => {
    const summary = summarizeItemOverlays(
      loaded([
        makeRun({ needs_you: { state: "known", value: false } }),
        makeRun({ needs_you: { state: "unknown", reason: "loop unreadable" } }),
      ]),
    );

    expect(summary.needsYou).toEqual({ state: "unknown", reason: "1 of 2 linked runs unknown: loop unreadable" });
  });

  describe("when the lookup did not complete but some runs came back", () => {
    const partial = (runs: ReturnType<typeof makeRun>[], overrides: Partial<Extract<LinkedRunsState, { status: "loaded" }>> = {}) =>
      summarizeItemOverlays(loaded(runs, { lookup: "partial", reason: "one run unreadable", ...overrides }));
    const incomplete = "at least 1 run; lookup partial: one run unreadable";

    it("does not report no-needs-you as known, and carries the lookup reason", () => {
      const summary = partial([makeRun({ needs_you: { state: "known", value: false } })]);

      expect(summary.needsYou).toEqual({ state: "unknown", reason: incomplete });
    });

    it("reports the run count, stage and verification as unknown, not as known", () => {
      const summary = partial([makeRun({ verification: { state: "known", value: "approved" } })]);

      expect(summary.linkedRuns).toEqual({ state: "unknown", reason: incomplete });
      expect(summary.executionStage).toEqual({ state: "unknown", reason: incomplete });
      expect(summary.verification).toEqual({ state: "unknown", reason: incomplete });
    });

    it("keeps a known needs-you of true: one run waiting on the operator stays true whatever else is missing", () => {
      const waiting = { state: "known", value: true, reason: "coordinator loop is awaiting approval" } as const;
      const summary = partial([makeRun({ needs_you: waiting })]);

      expect(summary.needsYou).toEqual(waiting);
      expect(summary.linkedRuns.state).toBe("unknown");
    });

    it("names the run count in the reason", () => {
      const summary = partial([makeRun(), makeRun({ run_entity_id: "acme.platform.chain.agent.execution.run-2" })]);

      expect(summary.linkedRuns).toEqual({
        state: "unknown",
        reason: "at least 2 runs; lookup partial: one run unreadable",
      });
    });

    it.each(["failed", "unsupported"] as const)("treats a %s lookup the same way", (lookup) => {
      const summary = partial([makeRun()], { lookup, reason: undefined });

      expect(summary.needsYou).toEqual({ state: "unknown", reason: `at least 1 run; lookup ${lookup}` });
    });
  });
});

describe("guardNone", () => {
  it("keeps none only for a complete lookup", () => {
    expect(guardNone({ state: "none" }, "complete")).toEqual({ state: "none" });
    expect(guardNone({ state: "none" }, "partial")).toEqual({ state: "unknown", reason: "lookup partial" });
  });

  it("leaves known and unknown alone", () => {
    const known = { state: "known", value: "x" } as const;
    expect(guardNone(known, "partial")).toBe(known);
  });
});

describe("portfolio helpers", () => {
  it("lists each configured repository once, in order", () => {
    const portfolio = makePortfolio(["acme/widgets", "acme/gadgets", "acme/widgets"]);

    expect(repositoryIds(portfolio.programs)).toEqual(["acme/widgets", "acme/gadgets"]);
  });

  it("slugs column names for test ids", () => {
    expect(slug("In Progress")).toBe("in-progress");
    expect(slug("No Status")).toBe("no-status");
    expect(slug(" Done! ")).toBe("done");
  });
});
