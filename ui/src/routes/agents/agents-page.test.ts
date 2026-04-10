import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, within } from "@testing-library/svelte";
import userEvent from "@testing-library/user-event";

// Mock agentStore — must be before component import
vi.mock("$lib/stores/agentStore.svelte", () => {
  // The mock store uses plain object properties that tests can mutate
  const store = {
    connected: false,
    loops: new Map(),
    loopsList: [] as import("$lib/types/agent").AgentLoop[],
    activeLoops: [] as import("$lib/types/agent").AgentLoop[],
    awaitingApproval: [] as import("$lib/types/agent").AgentLoop[],
    getLoop: (id: string) => store.loops.get(id),
  };
  return { agentStore: store };
});

// Mock agentApi
vi.mock("$lib/services/agentApi", () => ({
  agentApi: {
    sendSignal: vi
      .fn()
      .mockResolvedValue({ loop_id: "", signal: "", status: "ok" }),
    getTrajectory: vi.fn().mockResolvedValue({
      loop_id: "loop-123",
      role: "architect",
      iterations: 5,
      outcome: "complete",
      duration_ms: 1000,
    }),
  },
}));

// Mock isActiveState to match real implementation
vi.mock("$lib/types/agent", async () => {
  const actual =
    await vi.importActual<typeof import("$lib/types/agent")>(
      "$lib/types/agent",
    );
  return actual;
});

import AgentsPage from "./+page.svelte";
import { agentStore } from "$lib/stores/agentStore.svelte";
import { agentApi } from "$lib/services/agentApi";
import type { AgentLoop } from "$lib/types/agent";

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

function makeLoop(overrides: Partial<AgentLoop> = {}): AgentLoop {
  return {
    loop_id: "loop-abc123def456",
    task_id: "task-001",
    state: "exploring",
    role: "architect",
    iterations: 2,
    max_iterations: 10,
    user_id: "user-1",
    channel_type: "web",
    parent_loop_id: "",
    outcome: "",
    error: "",
    ...overrides,
  };
}

function setLoops(loops: AgentLoop[]) {
  const store = agentStore as unknown as {
    loopsList: AgentLoop[];
    activeLoops: AgentLoop[];
    awaitingApproval: AgentLoop[];
  };
  store.loopsList = loops;
  store.activeLoops = loops.filter((l) =>
    [
      "exploring",
      "planning",
      "architecting",
      "executing",
      "reviewing",
    ].includes(l.state),
  );
  store.awaitingApproval = loops.filter((l) => l.state === "awaiting_approval");
}

function setConnected(value: boolean) {
  (agentStore as unknown as { connected: boolean }).connected = value;
}

beforeEach(() => {
  vi.clearAllMocks();
  setConnected(false);
  setLoops([]);
});

// ---------------------------------------------------------------------------
// Page title and header
// ---------------------------------------------------------------------------

describe("Agents page — header", () => {
  it("renders page title 'Agents'", () => {
    render(AgentsPage);
    expect(screen.getByRole("heading", { name: "Agents" })).toBeInTheDocument();
  });

  it("renders back link to homepage", () => {
    render(AgentsPage);
    const backLink = screen.getByRole("link", { name: /graph/i });
    expect(backLink).toHaveAttribute("href", "/");
  });
});

// ---------------------------------------------------------------------------
// Connection status
// ---------------------------------------------------------------------------

describe("Agents page — connection status", () => {
  it("shows 'Disconnected' when not connected", () => {
    setConnected(false);
    render(AgentsPage);
    const status = screen.getByTestId("connection-status");
    expect(status).toHaveTextContent("Disconnected");
    expect(status).toHaveAttribute("data-connected", "false");
  });

  it("shows 'Connected' when connected", () => {
    setConnected(true);
    render(AgentsPage);
    const status = screen.getByTestId("connection-status");
    expect(status).toHaveTextContent("Connected");
    expect(status).toHaveAttribute("data-connected", "true");
  });
});

// ---------------------------------------------------------------------------
// Filter tabs
// ---------------------------------------------------------------------------

describe("Agents page — filter tabs", () => {
  it("renders all filter tabs", () => {
    render(AgentsPage);
    const tabs = screen.getByTestId("filter-tabs");
    expect(within(tabs).getByTestId("filter-all")).toBeInTheDocument();
    expect(within(tabs).getByTestId("filter-active")).toBeInTheDocument();
    expect(within(tabs).getByTestId("filter-paused")).toBeInTheDocument();
    expect(
      within(tabs).getByTestId("filter-awaiting_approval"),
    ).toBeInTheDocument();
    expect(within(tabs).getByTestId("filter-complete")).toBeInTheDocument();
    expect(within(tabs).getByTestId("filter-failed")).toBeInTheDocument();
  });

  it("'All' tab is active by default", () => {
    render(AgentsPage);
    const allTab = screen.getByTestId("filter-all");
    expect(allTab).toHaveClass("active");
  });

  it("displays friendly label for awaiting_approval", () => {
    render(AgentsPage);
    expect(screen.getByTestId("filter-awaiting_approval")).toHaveTextContent(
      "Awaiting Approval",
    );
  });
});

// ---------------------------------------------------------------------------
// Empty state
// ---------------------------------------------------------------------------

describe("Agents page — empty state", () => {
  it("shows empty state when no loops exist", () => {
    setLoops([]);
    render(AgentsPage);
    expect(screen.getByTestId("empty-state")).toBeInTheDocument();
    expect(screen.getByTestId("empty-state")).toHaveTextContent(
      "No agent loops",
    );
  });

  it("shows empty state when filtered list is empty", async () => {
    const user = userEvent.setup();
    setLoops([makeLoop({ state: "exploring" })]);
    render(AgentsPage);

    // Click "Failed" filter — no loops match
    await user.click(screen.getByTestId("filter-failed"));
    expect(screen.getByTestId("empty-state")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Loops table
// ---------------------------------------------------------------------------

describe("Agents page — loops table", () => {
  it("renders the loops table when loops exist", () => {
    setLoops([makeLoop()]);
    render(AgentsPage);
    expect(screen.getByTestId("loops-table")).toBeInTheDocument();
  });

  it("displays loop ID truncated to 12 characters", () => {
    setLoops([makeLoop({ loop_id: "loop-abc123def456789" })]);
    render(AgentsPage);
    const row = screen.getByTestId("loop-row");
    expect(row).toHaveTextContent("loop-abc123d");
  });

  it("displays loop state as badge text", () => {
    setLoops([makeLoop({ state: "exploring" })]);
    render(AgentsPage);
    const row = screen.getByTestId("loop-row");
    expect(row).toHaveTextContent("exploring");
  });

  it("replaces underscores with spaces in state badge", () => {
    setLoops([makeLoop({ state: "awaiting_approval" })]);
    render(AgentsPage);
    const row = screen.getByTestId("loop-row");
    expect(row).toHaveTextContent("awaiting approval");
  });

  it("displays role", () => {
    setLoops([makeLoop({ role: "builder" })]);
    render(AgentsPage);
    expect(screen.getByTestId("loop-row")).toHaveTextContent("builder");
  });

  it("displays progress as iterations/max_iterations", () => {
    setLoops([makeLoop({ iterations: 3, max_iterations: 10 })]);
    render(AgentsPage);
    expect(screen.getByTestId("loop-row")).toHaveTextContent("3/10");
  });

  it("displays user ID", () => {
    setLoops([makeLoop({ user_id: "alice" })]);
    render(AgentsPage);
    expect(screen.getByTestId("loop-row")).toHaveTextContent("alice");
  });

  it("renders multiple loops as separate rows", () => {
    setLoops([
      makeLoop({ loop_id: "loop-001", role: "architect" }),
      makeLoop({ loop_id: "loop-002", role: "builder" }),
    ]);
    render(AgentsPage);
    const rows = screen.getAllByTestId("loop-row");
    expect(rows).toHaveLength(2);
  });
});

// ---------------------------------------------------------------------------
// Action buttons — state-dependent
// ---------------------------------------------------------------------------

describe("Agents page — action buttons", () => {
  it.each([
    "exploring" as const,
    "planning" as const,
    "executing" as const,
    "reviewing" as const,
    "architecting" as const,
  ])("active state '%s' shows Pause and Cancel buttons", (state) => {
    setLoops([makeLoop({ state })]);
    render(AgentsPage);
    const row = screen.getByTestId("loop-row");
    expect(
      within(row).getByRole("button", { name: "Pause" }),
    ).toBeInTheDocument();
    expect(
      within(row).getByRole("button", { name: "Cancel" }),
    ).toBeInTheDocument();
  });

  it("paused state shows Resume and Cancel buttons", () => {
    setLoops([makeLoop({ state: "paused" })]);
    render(AgentsPage);
    const row = screen.getByTestId("loop-row");
    expect(
      within(row).getByRole("button", { name: "Resume" }),
    ).toBeInTheDocument();
    expect(
      within(row).getByRole("button", { name: "Cancel" }),
    ).toBeInTheDocument();
  });

  it("awaiting_approval state shows Approve and Reject buttons", () => {
    setLoops([makeLoop({ state: "awaiting_approval" })]);
    render(AgentsPage);
    const row = screen.getByTestId("loop-row");
    expect(
      within(row).getByRole("button", { name: "Approve" }),
    ).toBeInTheDocument();
    expect(
      within(row).getByRole("button", { name: "Reject" }),
    ).toBeInTheDocument();
  });

  // Updated: complete/failed now have a "View" trajectory button (Phase 8).
  // Only "cancelled" has zero action buttons.
  it.each(["complete" as const, "failed" as const])(
    "terminal state '%s' shows only View trajectory button",
    (state) => {
      setLoops([makeLoop({ loop_id: "loop-term", state })]);
      render(AgentsPage);
      const row = screen.getByTestId("loop-row");
      const buttons = within(row).queryAllByRole("button");
      expect(buttons).toHaveLength(1);
      expect(buttons[0]).toHaveTextContent("View");
    },
  );

  it("terminal state 'cancelled' shows no action buttons", () => {
    setLoops([makeLoop({ state: "cancelled" })]);
    render(AgentsPage);
    const row = screen.getByTestId("loop-row");
    expect(within(row).queryAllByRole("button")).toHaveLength(0);
  });
});

// ---------------------------------------------------------------------------
// Action button click handlers — sends signals via agentApi
// ---------------------------------------------------------------------------

describe("Agents page — signal sending", () => {
  it("Pause button sends 'pause' signal", async () => {
    const user = userEvent.setup();
    setLoops([makeLoop({ loop_id: "loop-123", state: "exploring" })]);
    render(AgentsPage);

    await user.click(screen.getByRole("button", { name: "Pause" }));
    expect(agentApi.sendSignal).toHaveBeenCalledWith("loop-123", "pause");
  });

  it("Cancel button sends 'cancel' signal", async () => {
    const user = userEvent.setup();
    setLoops([makeLoop({ loop_id: "loop-123", state: "exploring" })]);
    render(AgentsPage);

    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(agentApi.sendSignal).toHaveBeenCalledWith("loop-123", "cancel");
  });

  it("Resume button sends 'resume' signal", async () => {
    const user = userEvent.setup();
    setLoops([makeLoop({ loop_id: "loop-123", state: "paused" })]);
    render(AgentsPage);

    await user.click(screen.getByRole("button", { name: "Resume" }));
    expect(agentApi.sendSignal).toHaveBeenCalledWith("loop-123", "resume");
  });

  it("Approve button sends 'approve' signal", async () => {
    const user = userEvent.setup();
    setLoops([makeLoop({ loop_id: "loop-123", state: "awaiting_approval" })]);
    render(AgentsPage);

    await user.click(screen.getByRole("button", { name: "Approve" }));
    expect(agentApi.sendSignal).toHaveBeenCalledWith("loop-123", "approve");
  });

  it("Reject button sends 'reject' signal", async () => {
    const user = userEvent.setup();
    setLoops([makeLoop({ loop_id: "loop-123", state: "awaiting_approval" })]);
    render(AgentsPage);

    await user.click(screen.getByRole("button", { name: "Reject" }));
    expect(agentApi.sendSignal).toHaveBeenCalledWith("loop-123", "reject");
  });
});

// ---------------------------------------------------------------------------
// Filter tabs — filtering behavior
// ---------------------------------------------------------------------------

describe("Agents page — filtering", () => {
  const allLoops = [
    makeLoop({ loop_id: "loop-active", state: "exploring", role: "architect" }),
    makeLoop({ loop_id: "loop-paused", state: "paused", role: "builder" }),
    makeLoop({
      loop_id: "loop-approval",
      state: "awaiting_approval",
      role: "reviewer",
    }),
    makeLoop({ loop_id: "loop-complete", state: "complete", role: "debugger" }),
    makeLoop({ loop_id: "loop-failed", state: "failed", role: "planner" }),
  ];

  it("'All' tab shows all loops", () => {
    setLoops(allLoops);
    render(AgentsPage);
    expect(screen.getAllByTestId("loop-row")).toHaveLength(5);
  });

  it("'Active' tab shows only active state loops", async () => {
    const user = userEvent.setup();
    setLoops(allLoops);
    render(AgentsPage);

    await user.click(screen.getByTestId("filter-active"));
    const rows = screen.getAllByTestId("loop-row");
    expect(rows).toHaveLength(1);
    expect(rows[0]).toHaveTextContent("architect");
  });

  it("'Paused' tab shows only paused loops", async () => {
    const user = userEvent.setup();
    setLoops(allLoops);
    render(AgentsPage);

    await user.click(screen.getByTestId("filter-paused"));
    const rows = screen.getAllByTestId("loop-row");
    expect(rows).toHaveLength(1);
    expect(rows[0]).toHaveTextContent("builder");
  });

  it("'Awaiting Approval' tab shows only awaiting_approval loops", async () => {
    const user = userEvent.setup();
    setLoops(allLoops);
    render(AgentsPage);

    await user.click(screen.getByTestId("filter-awaiting_approval"));
    const rows = screen.getAllByTestId("loop-row");
    expect(rows).toHaveLength(1);
    expect(rows[0]).toHaveTextContent("reviewer");
  });

  it("'Complete' tab shows only complete loops", async () => {
    const user = userEvent.setup();
    setLoops(allLoops);
    render(AgentsPage);

    await user.click(screen.getByTestId("filter-complete"));
    const rows = screen.getAllByTestId("loop-row");
    expect(rows).toHaveLength(1);
    expect(rows[0]).toHaveTextContent("debugger");
  });

  it("'Failed' tab shows only failed loops", async () => {
    const user = userEvent.setup();
    setLoops(allLoops);
    render(AgentsPage);

    await user.click(screen.getByTestId("filter-failed"));
    const rows = screen.getAllByTestId("loop-row");
    expect(rows).toHaveLength(1);
    expect(rows[0]).toHaveTextContent("planner");
  });

  it("switching tabs updates the active tab style", async () => {
    const user = userEvent.setup();
    setLoops(allLoops);
    render(AgentsPage);

    expect(screen.getByTestId("filter-all")).toHaveClass("active");
    expect(screen.getByTestId("filter-active")).not.toHaveClass("active");

    await user.click(screen.getByTestId("filter-active"));
    expect(screen.getByTestId("filter-active")).toHaveClass("active");
    expect(screen.getByTestId("filter-all")).not.toHaveClass("active");
  });
});

// ---------------------------------------------------------------------------
// Trajectory viewer integration
// ---------------------------------------------------------------------------

describe("Agents page — trajectory viewer", () => {
  it("shows View button for complete loops", () => {
    setLoops([makeLoop({ loop_id: "loop-done", state: "complete" })]);
    render(AgentsPage);
    expect(screen.getByTestId("view-trajectory-loop-done")).toHaveTextContent(
      "View",
    );
  });

  it("shows View button for failed loops", () => {
    setLoops([makeLoop({ loop_id: "loop-err", state: "failed" })]);
    render(AgentsPage);
    expect(screen.getByTestId("view-trajectory-loop-err")).toHaveTextContent(
      "View",
    );
  });

  it("does not show View button for active loops", () => {
    setLoops([makeLoop({ loop_id: "loop-run", state: "exploring" })]);
    render(AgentsPage);
    expect(
      screen.queryByTestId("view-trajectory-loop-run"),
    ).not.toBeInTheDocument();
  });

  it("does not show View button for paused loops", () => {
    setLoops([makeLoop({ loop_id: "loop-p", state: "paused" })]);
    render(AgentsPage);
    expect(
      screen.queryByTestId("view-trajectory-loop-p"),
    ).not.toBeInTheDocument();
  });

  it("expands trajectory viewer when View is clicked", async () => {
    const user = userEvent.setup();
    setLoops([makeLoop({ loop_id: "loop-done", state: "complete" })]);
    render(AgentsPage);

    expect(screen.queryByTestId("trajectory-row")).not.toBeInTheDocument();
    await user.click(screen.getByTestId("view-trajectory-loop-done"));
    expect(screen.getByTestId("trajectory-row")).toBeInTheDocument();
  });

  it("changes View button to Hide when expanded", async () => {
    const user = userEvent.setup();
    setLoops([makeLoop({ loop_id: "loop-done", state: "complete" })]);
    render(AgentsPage);

    const btn = screen.getByTestId("view-trajectory-loop-done");
    expect(btn).toHaveTextContent("View");
    await user.click(btn);
    expect(btn).toHaveTextContent("Hide");
  });

  it("collapses trajectory viewer when Hide is clicked", async () => {
    const user = userEvent.setup();
    setLoops([makeLoop({ loop_id: "loop-done", state: "complete" })]);
    render(AgentsPage);

    const btn = screen.getByTestId("view-trajectory-loop-done");
    await user.click(btn); // expand
    expect(screen.getByTestId("trajectory-row")).toBeInTheDocument();

    await user.click(btn); // collapse
    expect(screen.queryByTestId("trajectory-row")).not.toBeInTheDocument();
  });

  it("renders TrajectoryViewer inside expanded row", async () => {
    const user = userEvent.setup();
    setLoops([makeLoop({ loop_id: "loop-done", state: "complete" })]);
    render(AgentsPage);

    await user.click(screen.getByTestId("view-trajectory-loop-done"));
    expect(screen.getByTestId("trajectory-viewer")).toBeInTheDocument();
  });
});
