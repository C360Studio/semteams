import { describe, expect, it } from "vitest";
import {
  formatClaimBadge,
  formatGateBadge,
  formatReviewBadge,
  guardNone,
  matchesBatonFilter,
  parseBatonFilter,
  repositoryIds,
  RUNS_LOADING,
  RUNS_NOT_LOADED,
  slug,
  summarizeItemOverlays,
} from "./workView";
import type { LinkedRunsState } from "./workView";
import { makeItem, makePortfolio, makeRun } from "../../test-utils/work";

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

describe("baton formatters and slice filters", () => {
  it("formats claim, review, and gate overlays for badges while preserving unknown/none states", () => {
    expect(
      formatClaimBadge({
        state: "known",
        value: {
          kind: "agent-pr",
          holder: "claude",
          agent_vendor: "claude",
          pull_request: "c360studio/semteams#318",
          draft: true,
        },
      }),
    ).toEqual({ state: "known", value: "claude (#318 draft PR)" });

    expect(
      formatReviewBadge({
        state: "known",
        value: { pull_request: "c360studio/semteams#320", status: "stale", head_sha: "a1b2c3d" },
      }),
    ).toEqual({ state: "known", value: "stale" });

    expect(
      formatGateBadge({
        state: "known",
        value: [
          { kind: "needs-decision", summary: "status:needs-decision" },
          { kind: "openspec-hold", summary: "OpenSpec hold" },
        ],
      }),
    ).toEqual({ state: "known", value: "needs decision, openspec hold" });

    expect(formatReviewBadge({ state: "unknown", reason: "no PR records" })).toEqual({
      state: "unknown",
      reason: "no PR records",
    });
  });

  it("parses valid baton filters and defaults unknown values to all", () => {
    expect(parseBatonFilter("review")).toBe("review");
    expect(parseBatonFilter("ready-to-land")).toBe("ready-to-land");
    expect(parseBatonFilter("unknown")).toBe("all");
    expect(parseBatonFilter(null)).toBe("all");
  });

  it("matches items by baton slice without altering column placement", () => {
    const needsDecisionItem = makeItem({
      labels: ["status:needs-decision"],
      dependencies: [{ ref: "acme/widgets#2", source: "blocked-by" }],
      delivery: { state: "none" },
    });
    const staleReviewItem = makeItem({
      delivery: {
        state: "known",
        value: [
          {
            ref: "acme/widgets#10",
            state: "open",
            branch: "codex/fix",
            head_sha: "a1b2c3d",
            review_record: { sha: "9988776", verdict: "pass" },
          },
        ],
      },
    });
    const readyItem = makeItem({
      delivery: {
        state: "known",
        value: [
          {
            ref: "acme/widgets#11",
            state: "open",
            branch: "claude/ready",
            head_sha: "78fbc93",
            review_record: { sha: "78fbc93", verdict: "pass" },
          },
        ],
      },
    });

    expect(matchesBatonFilter(needsDecisionItem, "needs-decision")).toBe(true);
    expect(matchesBatonFilter(needsDecisionItem, "blocked")).toBe(true);
    expect(matchesBatonFilter(needsDecisionItem, "claimed")).toBe(false);

    expect(matchesBatonFilter(staleReviewItem, "claimed")).toBe(true);
    expect(matchesBatonFilter(staleReviewItem, "review")).toBe(true);
    expect(matchesBatonFilter(staleReviewItem, "ready-to-land")).toBe(false);

    expect(matchesBatonFilter(readyItem, "ready-to-land")).toBe(true);
    expect(matchesBatonFilter(readyItem, "review")).toBe(false);
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
