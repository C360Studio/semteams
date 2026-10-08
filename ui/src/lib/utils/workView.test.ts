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

  it("reports the run count as incomplete when the lookup was partial", () => {
    const summary = summarizeItemOverlays(
      loaded([makeRun({ lookup: "partial" })], { lookup: "partial", reason: "one run unreadable" }),
    );

    expect(summary.linkedRuns).toEqual({ state: "known", value: 1, reason: "one run unreadable" });
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
