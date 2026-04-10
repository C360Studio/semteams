import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/svelte";
import FlowDiffSummary from "./FlowDiffSummary.svelte";
import type { FlowDiff } from "$lib/types/chat";

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

function makeDiff(overrides: Partial<FlowDiff> = {}): FlowDiff {
  return {
    nodesAdded: [],
    nodesRemoved: [],
    nodesModified: [],
    connectionsAdded: 0,
    connectionsRemoved: 0,
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// Presence
// ---------------------------------------------------------------------------

describe("FlowDiffSummary — presence", () => {
  it("should have data-testid='flow-diff-summary'", () => {
    render(FlowDiffSummary, { props: { diff: makeDiff() } });

    expect(screen.getByTestId("flow-diff-summary")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Empty diff
// ---------------------------------------------------------------------------

describe("FlowDiffSummary — empty diff", () => {
  it("should show 'No changes' for an empty diff", () => {
    render(FlowDiffSummary, { props: { diff: makeDiff() } });

    expect(screen.getByTestId("flow-diff-summary")).toHaveTextContent(
      /no changes/i,
    );
  });
});

// ---------------------------------------------------------------------------
// Added nodes
// ---------------------------------------------------------------------------

describe("FlowDiffSummary — added nodes", () => {
  it("should show added node names", () => {
    render(FlowDiffSummary, {
      props: {
        diff: makeDiff({ nodesAdded: ["WebhookInput", "JsonProcessor"] }),
      },
    });

    const summary = screen.getByTestId("flow-diff-summary");
    expect(summary).toHaveTextContent("WebhookInput");
    expect(summary).toHaveTextContent("JsonProcessor");
  });

  it("should NOT show 'No changes' when nodes are added", () => {
    render(FlowDiffSummary, {
      props: { diff: makeDiff({ nodesAdded: ["NewNode"] }) },
    });

    expect(screen.getByTestId("flow-diff-summary")).not.toHaveTextContent(
      /no changes/i,
    );
  });

  it("should handle a single added node", () => {
    render(FlowDiffSummary, {
      props: { diff: makeDiff({ nodesAdded: ["KafkaInput"] }) },
    });

    expect(screen.getByTestId("flow-diff-summary")).toHaveTextContent(
      "KafkaInput",
    );
  });
});

// ---------------------------------------------------------------------------
// Removed nodes
// ---------------------------------------------------------------------------

describe("FlowDiffSummary — removed nodes", () => {
  it("should show removed node names", () => {
    render(FlowDiffSummary, {
      props: {
        diff: makeDiff({ nodesRemoved: ["OldOutput", "LegacyFilter"] }),
      },
    });

    const summary = screen.getByTestId("flow-diff-summary");
    expect(summary).toHaveTextContent("OldOutput");
    expect(summary).toHaveTextContent("LegacyFilter");
  });

  it("should handle removals-only diff", () => {
    render(FlowDiffSummary, {
      props: { diff: makeDiff({ nodesRemoved: ["DeadNode"] }) },
    });

    expect(screen.getByTestId("flow-diff-summary")).not.toHaveTextContent(
      /no changes/i,
    );
    expect(screen.getByTestId("flow-diff-summary")).toHaveTextContent(
      "DeadNode",
    );
  });
});

// ---------------------------------------------------------------------------
// Modified nodes
// ---------------------------------------------------------------------------

describe("FlowDiffSummary — modified nodes", () => {
  it("should show modified node names", () => {
    render(FlowDiffSummary, {
      props: { diff: makeDiff({ nodesModified: ["KafkaOutput"] }) },
    });

    expect(screen.getByTestId("flow-diff-summary")).toHaveTextContent(
      "KafkaOutput",
    );
  });
});

// ---------------------------------------------------------------------------
// Connection counts
// ---------------------------------------------------------------------------

describe("FlowDiffSummary — connection counts", () => {
  it("should show connectionsAdded count when greater than zero", () => {
    render(FlowDiffSummary, {
      props: { diff: makeDiff({ connectionsAdded: 3 }) },
    });

    expect(screen.getByTestId("flow-diff-summary")).toHaveTextContent("3");
  });

  it("should show connectionsRemoved count when greater than zero", () => {
    render(FlowDiffSummary, {
      props: { diff: makeDiff({ connectionsRemoved: 2 }) },
    });

    expect(screen.getByTestId("flow-diff-summary")).toHaveTextContent("2");
  });

  it("should handle connections-only diff (no node changes)", () => {
    render(FlowDiffSummary, {
      props: {
        diff: makeDiff({ connectionsAdded: 1, connectionsRemoved: 1 }),
      },
    });

    expect(screen.getByTestId("flow-diff-summary")).not.toHaveTextContent(
      /no changes/i,
    );
  });
});

// ---------------------------------------------------------------------------
// Table-driven: partial diffs
// ---------------------------------------------------------------------------

describe("FlowDiffSummary — table-driven partial diffs", () => {
  const cases = [
    {
      description: "additions only",
      diff: makeDiff({ nodesAdded: ["NodeA", "NodeB"] }),
      expectNoChanges: false,
      expectTexts: ["NodeA", "NodeB"],
    },
    {
      description: "removals only",
      diff: makeDiff({ nodesRemoved: ["NodeX"] }),
      expectNoChanges: false,
      expectTexts: ["NodeX"],
    },
    {
      description: "modifications only",
      diff: makeDiff({ nodesModified: ["NodeM"] }),
      expectNoChanges: false,
      expectTexts: ["NodeM"],
    },
    {
      description: "connections only",
      diff: makeDiff({ connectionsAdded: 4 }),
      expectNoChanges: false,
      expectTexts: ["4"],
    },
    {
      description: "empty — no changes",
      diff: makeDiff(),
      expectNoChanges: true,
      expectTexts: [],
    },
    {
      description: "mixed add + remove + modify + connections",
      diff: makeDiff({
        nodesAdded: ["Alpha"],
        nodesRemoved: ["Beta"],
        nodesModified: ["Gamma"],
        connectionsAdded: 2,
        connectionsRemoved: 1,
      }),
      expectNoChanges: false,
      expectTexts: ["Alpha", "Beta", "Gamma"],
    },
  ];

  it.each(cases)("$description", ({ diff, expectNoChanges, expectTexts }) => {
    render(FlowDiffSummary, { props: { diff } });

    const summary = screen.getByTestId("flow-diff-summary");

    if (expectNoChanges) {
      expect(summary).toHaveTextContent(/no changes/i);
    } else {
      expect(summary).not.toHaveTextContent(/no changes/i);
    }

    for (const text of expectTexts) {
      expect(summary).toHaveTextContent(text);
    }
  });
});
