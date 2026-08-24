import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/svelte";
import type { LoopTrajectory, TrajectoryFact } from "$lib/types/agent";

// Mock agentApi — must be before component import
const mockGetTrajectory = vi.fn();
vi.mock("$lib/services/agentApi", () => ({
  agentApi: {
    getTrajectory: (...args: unknown[]) => mockGetTrajectory(...args),
  },
}));

import TrajectoryViewer from "./TrajectoryViewer.svelte";

// ---------------------------------------------------------------------------
// Test data factory — the beta.160 GraphQL trajectory(loopId) fact-log
// shape. See ui/src/lib/types/agent.ts for the full field contract.
// ---------------------------------------------------------------------------

function makeTrajectory(
  facts: TrajectoryFact[],
  overrides: Partial<LoopTrajectory> = {},
): LoopTrajectory {
  return {
    schema_version: "v1",
    loop_id: "loop-abc123",
    coverage: "observed",
    terminal_observed: false,
    observed_totals: {
      facts: facts.length,
      tokens_in: 0,
      tokens_out: 0,
      elapsed_ms: 0,
      message_count: 0,
      tool_count: 0,
      url_count: 0,
      model_requests: 0,
      model_completions: 0,
      tool_requests: 0,
      tool_completions: 0,
      context_compactions: 0,
      terminal_observations: 0,
      requested_observations: 0,
      completed_observations: 0,
      failed_observations: 0,
      cancelled_observations: 0,
    },
    facts,
    ...overrides,
  };
}

function startedFact(role: string): TrajectoryFact {
  return {
    kind: "loop.started",
    causal_iteration: 0,
    causal_phase: "loop_start",
    causal_ordinal: 0,
    capability_preview: role,
  };
}

function terminalFact(overrides: Partial<TrajectoryFact> = {}): TrajectoryFact {
  return {
    kind: "loop.terminal",
    causal_iteration: 5,
    causal_phase: "terminal",
    causal_ordinal: 0,
    status: "completed",
    elapsed_ms: 12345,
    ...overrides,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
});

// ---------------------------------------------------------------------------
// Rendering and data-testid
// ---------------------------------------------------------------------------

describe("TrajectoryViewer — structure", () => {
  it("renders with data-testid='trajectory-viewer'", async () => {
    mockGetTrajectory.mockResolvedValue(makeTrajectory([]));
    render(TrajectoryViewer, { props: { loopId: "loop-abc123" } });
    expect(screen.getByTestId("trajectory-viewer")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Loading state
// ---------------------------------------------------------------------------

describe("TrajectoryViewer — loading state", () => {
  it("shows loading state initially", () => {
    // Never-resolving promise to keep it in loading
    mockGetTrajectory.mockReturnValue(new Promise(() => {}));
    render(TrajectoryViewer, { props: { loopId: "loop-abc123" } });
    expect(screen.getByTestId("trajectory-loading")).toBeInTheDocument();
    expect(screen.getByTestId("trajectory-loading")).toHaveTextContent(
      "Loading trajectory...",
    );
  });
});

// ---------------------------------------------------------------------------
// Trajectory data display
// ---------------------------------------------------------------------------

describe("TrajectoryViewer — data display", () => {
  it("shows the role from the loop.started fact after load", async () => {
    mockGetTrajectory.mockResolvedValue(
      makeTrajectory([startedFact("builder"), terminalFact()], {
        terminal_observed: true,
      }),
    );
    render(TrajectoryViewer, { props: { loopId: "loop-abc123" } });

    await waitFor(() => {
      expect(screen.getByRole("heading", { level: 3 })).toHaveTextContent(
        "builder",
      );
    });
  });

  it("shows the terminal fact's status as the outcome", async () => {
    mockGetTrajectory.mockResolvedValue(
      makeTrajectory([terminalFact({ status: "completed" })], {
        terminal_observed: true,
      }),
    );
    render(TrajectoryViewer, { props: { loopId: "loop-abc123" } });

    await waitFor(() => {
      expect(screen.getByTestId("trajectory-outcome")).toHaveTextContent(
        "completed",
      );
    });
  });

  it("shows the terminal fact's elapsed_ms as the duration", async () => {
    mockGetTrajectory.mockResolvedValue(
      makeTrajectory([terminalFact({ elapsed_ms: 9876 })], {
        terminal_observed: true,
      }),
    );
    render(TrajectoryViewer, { props: { loopId: "loop-abc123" } });

    await waitFor(() => {
      expect(screen.getByTestId("trajectory-viewer")).toHaveTextContent(
        "9876ms",
      );
    });
  });

  it("shows the terminal fact's causal_iteration as the iteration count", async () => {
    mockGetTrajectory.mockResolvedValue(
      makeTrajectory([terminalFact({ causal_iteration: 7 })], {
        terminal_observed: true,
      }),
    );
    render(TrajectoryViewer, { props: { loopId: "loop-abc123" } });

    await waitFor(() => {
      expect(screen.getByTestId("trajectory-viewer")).toHaveTextContent(
        "7 iterations",
      );
    });
  });

  it("shows 'in progress' and 0 iterations before a terminal fact is observed", async () => {
    mockGetTrajectory.mockResolvedValue(makeTrajectory([startedFact("builder")]));
    render(TrajectoryViewer, { props: { loopId: "loop-abc123" } });

    await waitFor(() => {
      expect(screen.getByTestId("trajectory-outcome")).toHaveTextContent(
        "in progress",
      );
    });
    expect(screen.getByTestId("trajectory-viewer")).toHaveTextContent(
      "0 iterations",
    );
  });
});

// ---------------------------------------------------------------------------
// Token usage
// ---------------------------------------------------------------------------

describe("TrajectoryViewer — token usage", () => {
  it("shows token usage when present in observed_totals", async () => {
    mockGetTrajectory.mockResolvedValue(
      makeTrajectory([terminalFact()], {
        terminal_observed: true,
        observed_totals: {
          facts: 1,
          tokens_in: 1500,
          tokens_out: 800,
          elapsed_ms: 0,
          message_count: 0,
          tool_count: 0,
          url_count: 0,
          model_requests: 0,
          model_completions: 0,
          tool_requests: 0,
          tool_completions: 0,
          context_compactions: 0,
          terminal_observations: 1,
          requested_observations: 0,
          completed_observations: 1,
          failed_observations: 0,
          cancelled_observations: 0,
        },
      }),
    );
    render(TrajectoryViewer, { props: { loopId: "loop-abc123" } });

    await waitFor(() => {
      const tokenUsage = screen.getByTestId("token-usage");
      expect(tokenUsage).toHaveTextContent("1,500");
      expect(tokenUsage).toHaveTextContent("800");
    });
  });

  it("does not show token usage when observed_totals has zero tokens", async () => {
    mockGetTrajectory.mockResolvedValue(
      makeTrajectory([terminalFact()], { terminal_observed: true }),
    );
    render(TrajectoryViewer, { props: { loopId: "loop-abc123" } });

    await waitFor(() => {
      expect(screen.getByTestId("trajectory-outcome")).toBeInTheDocument();
    });
    expect(screen.queryByTestId("token-usage")).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Tool calls
// ---------------------------------------------------------------------------

describe("TrajectoryViewer — tool calls", () => {
  function toolFact(overrides: Partial<TrajectoryFact> = {}): TrajectoryFact {
    return {
      kind: "tool.completed",
      causal_iteration: 1,
      causal_phase: "tool_result",
      causal_ordinal: 0,
      status: "completed",
      ...overrides,
    };
  }

  it("shows tool calls list with count", async () => {
    mockGetTrajectory.mockResolvedValue(
      makeTrajectory([
        toolFact({ attempt_id: "a1", tool_preview: "search" }),
        toolFact({ attempt_id: "a2", tool_preview: "lookup" }),
      ]),
    );
    render(TrajectoryViewer, { props: { loopId: "loop-abc123" } });

    await waitFor(() => {
      const toolCalls = screen.getByTestId("tool-calls");
      expect(toolCalls).toHaveTextContent("Tool Calls (2)");
    });
  });

  it("shows tool call name from tool_preview", async () => {
    mockGetTrajectory.mockResolvedValue(
      makeTrajectory([toolFact({ tool_preview: "graph_search" })]),
    );
    render(TrajectoryViewer, { props: { loopId: "loop-abc123" } });

    await waitFor(() => {
      const entry = screen.getByTestId("tool-call-entry");
      expect(entry).toHaveTextContent("graph_search");
    });
  });

  it("does not show tool call arguments or results — only the wire allows", async () => {
    // beta.160: tool.completed carries no arguments/result content, only
    // metadata — this is a documented product-behavior change, not a bug.
    mockGetTrajectory.mockResolvedValue(
      makeTrajectory([toolFact({ tool_preview: "search" })]),
    );
    render(TrajectoryViewer, { props: { loopId: "loop-abc123" } });

    await screen.findByTestId("tool-call-entry");
    expect(screen.queryByText("Arguments")).not.toBeInTheDocument();
    expect(screen.queryByText("Result")).not.toBeInTheDocument();
  });

  it("shows tool call failure status and error_category when present", async () => {
    mockGetTrajectory.mockResolvedValue(
      makeTrajectory([
        toolFact({
          tool_preview: "search",
          status: "failed",
          error_category: "timeout",
        }),
      ]),
    );
    render(TrajectoryViewer, { props: { loopId: "loop-abc123" } });

    await waitFor(() => {
      const errorEl = screen.getByTestId("tool-call-error");
      expect(errorEl).toHaveTextContent("failed");
      expect(errorEl).toHaveTextContent("timeout");
    });
  });

  it("shows tool call duration when present", async () => {
    mockGetTrajectory.mockResolvedValue(
      makeTrajectory([toolFact({ tool_preview: "search", elapsed_ms: 250 })]),
    );
    render(TrajectoryViewer, { props: { loopId: "loop-abc123" } });

    await waitFor(() => {
      const entry = screen.getByTestId("tool-call-entry");
      expect(entry).toHaveTextContent("250ms");
    });
  });

  it("does not show the tool calls section when there are no tool.completed facts", async () => {
    mockGetTrajectory.mockResolvedValue(makeTrajectory([startedFact("builder")]));
    render(TrajectoryViewer, { props: { loopId: "loop-abc123" } });

    await waitFor(() => {
      expect(screen.getByTestId("trajectory-outcome")).toBeInTheDocument();
    });
    expect(screen.queryByTestId("tool-calls")).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Error state
// ---------------------------------------------------------------------------

describe("TrajectoryViewer — error state", () => {
  it("shows error state when API call fails", async () => {
    mockGetTrajectory.mockRejectedValue(new Error("Network error"));
    render(TrajectoryViewer, { props: { loopId: "loop-abc123" } });

    await waitFor(() => {
      expect(screen.getByTestId("trajectory-error")).toHaveTextContent(
        "Network error",
      );
    });
  });

  it("shows generic error for non-Error rejections", async () => {
    mockGetTrajectory.mockRejectedValue("unknown failure");
    render(TrajectoryViewer, { props: { loopId: "loop-abc123" } });

    await waitFor(() => {
      expect(screen.getByTestId("trajectory-error")).toHaveTextContent(
        "Failed to load trajectory",
      );
    });
  });
});

// ---------------------------------------------------------------------------
// API call behavior
// ---------------------------------------------------------------------------

describe("TrajectoryViewer — API calls", () => {
  it("fetches trajectory using the provided loopId", async () => {
    mockGetTrajectory.mockResolvedValue(makeTrajectory([]));
    render(TrajectoryViewer, { props: { loopId: "my-loop-id-999" } });

    await waitFor(() => {
      expect(mockGetTrajectory).toHaveBeenCalledWith("my-loop-id-999");
    });
  });

  it("re-fetches when loopId prop changes", async () => {
    mockGetTrajectory.mockResolvedValue(makeTrajectory([]));
    const { rerender } = render(TrajectoryViewer, {
      props: { loopId: "loop-1" },
    });

    await waitFor(() => {
      expect(mockGetTrajectory).toHaveBeenCalledWith("loop-1");
    });

    // Svelte 5 pattern: use rerender instead of $set
    await rerender({ loopId: "loop-2" });

    await waitFor(() => {
      expect(mockGetTrajectory).toHaveBeenCalledWith("loop-2");
    });
  });
});
