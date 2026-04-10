import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/svelte";
import type { TrajectoryEntry } from "$lib/types/agent";

// Mock agentApi — must be before component import
const mockGetTrajectory = vi.fn();
vi.mock("$lib/services/agentApi", () => ({
  agentApi: {
    getTrajectory: (...args: unknown[]) => mockGetTrajectory(...args),
  },
}));

import TrajectoryViewer from "./TrajectoryViewer.svelte";

// ---------------------------------------------------------------------------
// Test data factory
// ---------------------------------------------------------------------------

function makeTrajectory(
  overrides: Partial<TrajectoryEntry> = {},
): TrajectoryEntry {
  return {
    loop_id: "loop-abc123",
    role: "architect",
    iterations: 5,
    outcome: "complete",
    duration_ms: 12345,
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
    mockGetTrajectory.mockResolvedValue(makeTrajectory());
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
  it("shows trajectory role after load", async () => {
    mockGetTrajectory.mockResolvedValue(makeTrajectory({ role: "builder" }));
    render(TrajectoryViewer, { props: { loopId: "loop-abc123" } });

    await waitFor(() => {
      expect(screen.getByRole("heading", { level: 3 })).toHaveTextContent(
        "builder",
      );
    });
  });

  it("shows trajectory outcome", async () => {
    mockGetTrajectory.mockResolvedValue(
      makeTrajectory({ outcome: "complete" }),
    );
    render(TrajectoryViewer, { props: { loopId: "loop-abc123" } });

    await waitFor(() => {
      expect(screen.getByTestId("trajectory-outcome")).toHaveTextContent(
        "complete",
      );
    });
  });

  it("shows trajectory duration", async () => {
    mockGetTrajectory.mockResolvedValue(makeTrajectory({ duration_ms: 9876 }));
    render(TrajectoryViewer, { props: { loopId: "loop-abc123" } });

    await waitFor(() => {
      expect(screen.getByTestId("trajectory-viewer")).toHaveTextContent(
        "9876ms",
      );
    });
  });

  it("shows trajectory iterations", async () => {
    mockGetTrajectory.mockResolvedValue(makeTrajectory({ iterations: 7 }));
    render(TrajectoryViewer, { props: { loopId: "loop-abc123" } });

    await waitFor(() => {
      expect(screen.getByTestId("trajectory-viewer")).toHaveTextContent(
        "7 iterations",
      );
    });
  });
});

// ---------------------------------------------------------------------------
// Token usage
// ---------------------------------------------------------------------------

describe("TrajectoryViewer — token usage", () => {
  it("shows token usage when present", async () => {
    mockGetTrajectory.mockResolvedValue(
      makeTrajectory({
        token_usage: { input_tokens: 1500, output_tokens: 800 },
      }),
    );
    render(TrajectoryViewer, { props: { loopId: "loop-abc123" } });

    await waitFor(() => {
      const tokenUsage = screen.getByTestId("token-usage");
      expect(tokenUsage).toHaveTextContent("1,500");
      expect(tokenUsage).toHaveTextContent("800");
    });
  });

  it("does not show token usage when absent", async () => {
    mockGetTrajectory.mockResolvedValue(
      makeTrajectory({ token_usage: undefined }),
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
  it("shows tool calls list with count", async () => {
    mockGetTrajectory.mockResolvedValue(
      makeTrajectory({
        tool_calls: [
          { name: "search", args: { query: "test" } },
          { name: "lookup", args: { id: "123" } },
        ],
      }),
    );
    render(TrajectoryViewer, { props: { loopId: "loop-abc123" } });

    await waitFor(() => {
      const toolCalls = screen.getByTestId("tool-calls");
      expect(toolCalls).toHaveTextContent("Tool Calls (2)");
    });
  });

  it("shows tool call name", async () => {
    mockGetTrajectory.mockResolvedValue(
      makeTrajectory({
        tool_calls: [{ name: "graph_search", args: { query: "foo" } }],
      }),
    );
    render(TrajectoryViewer, { props: { loopId: "loop-abc123" } });

    await waitFor(() => {
      const entry = screen.getByTestId("tool-call-entry");
      expect(entry).toHaveTextContent("graph_search");
    });
  });

  it("shows tool call arguments in details", async () => {
    mockGetTrajectory.mockResolvedValue(
      makeTrajectory({
        tool_calls: [
          { name: "search", args: { query: "test query", limit: 10 } },
        ],
      }),
    );
    render(TrajectoryViewer, { props: { loopId: "loop-abc123" } });

    await waitFor(() => {
      const entry = screen.getByTestId("tool-call-entry");
      expect(entry).toHaveTextContent("test query");
    });
  });

  it("shows tool call result when present", async () => {
    mockGetTrajectory.mockResolvedValue(
      makeTrajectory({
        tool_calls: [
          {
            name: "search",
            args: {},
            result: "Found 3 matches",
          },
        ],
      }),
    );
    render(TrajectoryViewer, { props: { loopId: "loop-abc123" } });

    await waitFor(() => {
      const entry = screen.getByTestId("tool-call-entry");
      expect(entry).toHaveTextContent("Found 3 matches");
    });
  });

  it("shows tool call error when present", async () => {
    mockGetTrajectory.mockResolvedValue(
      makeTrajectory({
        tool_calls: [
          {
            name: "search",
            args: {},
            error: "Connection refused",
          },
        ],
      }),
    );
    render(TrajectoryViewer, { props: { loopId: "loop-abc123" } });

    await waitFor(() => {
      const errorEl = screen.getByTestId("tool-call-error");
      expect(errorEl).toHaveTextContent("Connection refused");
    });
  });

  it("shows tool call duration when present", async () => {
    mockGetTrajectory.mockResolvedValue(
      makeTrajectory({
        tool_calls: [
          {
            name: "search",
            args: {},
            duration_ms: 250,
          },
        ],
      }),
    );
    render(TrajectoryViewer, { props: { loopId: "loop-abc123" } });

    await waitFor(() => {
      const entry = screen.getByTestId("tool-call-entry");
      expect(entry).toHaveTextContent("250ms");
    });
  });

  it("does not show tool calls section when no tool calls", async () => {
    mockGetTrajectory.mockResolvedValue(
      makeTrajectory({ tool_calls: undefined }),
    );
    render(TrajectoryViewer, { props: { loopId: "loop-abc123" } });

    await waitFor(() => {
      expect(screen.getByTestId("trajectory-outcome")).toBeInTheDocument();
    });
    expect(screen.queryByTestId("tool-calls")).not.toBeInTheDocument();
  });

  it("does not show tool calls section when tool calls array is empty", async () => {
    mockGetTrajectory.mockResolvedValue(makeTrajectory({ tool_calls: [] }));
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
    mockGetTrajectory.mockResolvedValue(makeTrajectory());
    render(TrajectoryViewer, { props: { loopId: "my-loop-id-999" } });

    await waitFor(() => {
      expect(mockGetTrajectory).toHaveBeenCalledWith("my-loop-id-999");
    });
  });

  it("re-fetches when loopId prop changes", async () => {
    mockGetTrajectory.mockResolvedValue(makeTrajectory());
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
