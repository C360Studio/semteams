import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/svelte";
import userEvent from "@testing-library/user-event";
import TaskDetailPanel from "./TaskDetailPanel.svelte";
import type { TaskInfo } from "$lib/types/task";
import type { AgentLoop } from "$lib/types/agent";

// Mock agentApi — covers both TaskDetailPanel's sendSignal calls and
// TrajectoryViewer's getTrajectory/getTrajectories calls.
vi.mock("$lib/services/agentApi", () => ({
  agentApi: {
    sendSignal: vi.fn().mockResolvedValue({ status: "sent" }),
    getTrajectory: vi.fn().mockResolvedValue({
      loop_id: "",
      role: "",
      iterations: 0,
      outcome: "",
      duration_ms: 0,
    }),
    getTrajectories: vi.fn().mockResolvedValue([]),
  },
}));

import { agentApi } from "$lib/services/agentApi";

function makeLoop(overrides: Partial<AgentLoop> = {}): AgentLoop {
  return {
    loop_id: "loop_001",
    task_id: "task_001",
    state: "executing",
    role: "general",
    iterations: 3,
    max_iterations: 10,
    user_id: "user-1",
    channel_type: "web",
    parent_loop_id: "",
    outcome: "",
    error: "",
    ...overrides,
  };
}

function makeTask(overrides: Partial<TaskInfo> = {}): TaskInfo {
  return {
    id: "loop_001",
    shortRef: null,
    aliases: [],
    titleEdited: false,
    title: "Test Task",
    column: "executing",
    state: "executing",
    role: "general",
    iterations: 3,
    maxIterations: 10,
    primaryLoop: makeLoop(),
    childLoops: [],
    childNeedsAttention: false,
    childAttentionCount: 0,
    ...overrides,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("TaskDetailPanel", () => {
  it("renders task title and state", () => {
    render(TaskDetailPanel, { props: { task: makeTask({ title: "My Task", state: "planning" }) } });

    expect(screen.getByText("My Task")).toBeInTheDocument();
  });

  it("renders role and iteration count", () => {
    render(TaskDetailPanel, {
      props: { task: makeTask({ role: "editor", iterations: 5, maxIterations: 20 }) },
    });

    expect(screen.getByText("editor")).toBeInTheDocument();
    expect(screen.getByText("5/20 iterations")).toBeInTheDocument();
  });

  it("renders truncated ID with ellipsis", () => {
    render(TaskDetailPanel, { props: { task: makeTask({ id: "loop_abcdef123456" }) } });

    // ID is now displayed without the "ID: " label (the monospace
    // styling + ellipsis says it's an identifier).
    expect(screen.getByText("loop_abcdef1…")).toBeInTheDocument();
  });

  it("renders #N short ref in header when present", () => {
    render(TaskDetailPanel, { props: { task: makeTask({ shortRef: 42 }) } });
    expect(screen.getByTestId("header-ref")).toHaveTextContent("#42");
  });

  it("does NOT render short ref when shortRef is null", () => {
    render(TaskDetailPanel, { props: { task: makeTask({ shortRef: null }) } });
    expect(screen.queryByTestId("header-ref")).not.toBeInTheDocument();
  });

  it("fires onClose when close button is clicked", async () => {
    const onClose = vi.fn();
    const user = userEvent.setup();
    render(TaskDetailPanel, { props: { task: makeTask(), onClose } });

    await user.click(screen.getByLabelText("Close detail panel"));

    expect(onClose).toHaveBeenCalledOnce();
  });

  describe("action buttons per state", () => {
    it("shows Pause + Cancel for active states", () => {
      render(TaskDetailPanel, { props: { task: makeTask({ state: "executing" }) } });

      expect(screen.getByText("Pause")).toBeInTheDocument();
      expect(screen.getByText("Cancel")).toBeInTheDocument();
    });

    it("shows Resume + Cancel for paused state", () => {
      render(TaskDetailPanel, { props: { task: makeTask({ state: "paused" }) } });

      expect(screen.getByText("Resume")).toBeInTheDocument();
      expect(screen.getByText("Cancel")).toBeInTheDocument();
    });

    it("shows Approve + Reject for awaiting_approval state", () => {
      render(TaskDetailPanel, { props: { task: makeTask({ state: "awaiting_approval" }) } });

      expect(screen.getByText("Approve")).toBeInTheDocument();
      expect(screen.getByText("Reject")).toBeInTheDocument();
    });

    it("shows no action buttons for complete state", () => {
      render(TaskDetailPanel, { props: { task: makeTask({ state: "complete" }) } });

      expect(screen.queryByText("Pause")).not.toBeInTheDocument();
      expect(screen.queryByText("Approve")).not.toBeInTheDocument();
    });
  });

  describe("signal dispatch", () => {
    it("calls sendSignal on Approve click", async () => {
      const user = userEvent.setup();
      render(TaskDetailPanel, {
        props: { task: makeTask({ id: "loop_xyz", state: "awaiting_approval" }) },
      });

      await user.click(screen.getByText("Approve"));

      expect(agentApi.sendSignal).toHaveBeenCalledWith("loop_xyz", "approve");
    });

    it("shows error when signal fails", async () => {
      vi.mocked(agentApi.sendSignal).mockRejectedValueOnce(new Error("Signal failed"));
      const user = userEvent.setup();
      render(TaskDetailPanel, {
        props: { task: makeTask({ state: "awaiting_approval" }) },
      });

      await user.click(screen.getByText("Approve"));

      expect(screen.getByRole("alert")).toBeInTheDocument();
      expect(screen.getByText("Signal failed")).toBeInTheDocument();
    });
  });

  describe("child loops", () => {
    it("renders child loop list when children exist", () => {
      const children = [
        makeLoop({ loop_id: "c1", state: "executing", role: "researcher" }),
        makeLoop({ loop_id: "c2", state: "complete", role: "editor" }),
      ];
      render(TaskDetailPanel, {
        props: { task: makeTask({ childLoops: children }) },
      });

      // "Sub-loops" reads better than "Child Loops" in the user-facing
      // copy — implementation detail in the rename.
      expect(screen.getByText("Sub-loops (2)")).toBeInTheDocument();
      expect(screen.getByText("researcher")).toBeInTheDocument();
      expect(screen.getByText("editor")).toBeInTheDocument();
    });

    it("hides child loop section when no children", () => {
      render(TaskDetailPanel, { props: { task: makeTask() } });

      expect(screen.queryByText(/Sub-loops/)).not.toBeInTheDocument();
    });
  });

  describe("tabs", () => {
    it("renders all four tabs", () => {
      render(TaskDetailPanel, { props: { task: makeTask() } });
      expect(screen.getByTestId("tab-activity")).toBeInTheDocument();
      expect(screen.getByTestId("tab-trace")).toBeInTheDocument();
      expect(screen.getByTestId("tab-entities")).toBeInTheDocument();
      expect(screen.getByTestId("tab-logs")).toBeInTheDocument();
    });

    it("Activity is the default active tab", () => {
      render(TaskDetailPanel, { props: { task: makeTask() } });
      expect(screen.getByTestId("tab-activity")).toHaveAttribute(
        "aria-selected",
        "true",
      );
      expect(screen.getByTestId("panel-activity")).toBeInTheDocument();
      expect(screen.queryByTestId("panel-trace")).not.toBeInTheDocument();
    });

    it("clicking a tab switches the visible panel", async () => {
      const user = userEvent.setup();
      render(TaskDetailPanel, { props: { task: makeTask() } });

      await user.click(screen.getByTestId("tab-trace"));

      expect(screen.getByTestId("tab-trace")).toHaveAttribute(
        "aria-selected",
        "true",
      );
      expect(screen.getByTestId("panel-trace")).toBeInTheDocument();
      expect(screen.queryByTestId("panel-activity")).not.toBeInTheDocument();
    });

    it("placeholder tabs surface what's coming, don't pretend to deliver", async () => {
      const user = userEvent.setup();
      render(TaskDetailPanel, { props: { task: makeTask() } });

      await user.click(screen.getByTestId("tab-entities"));
      expect(screen.getByText("Scoped knowledge graph")).toBeInTheDocument();

      await user.click(screen.getByTestId("tab-logs"));
      expect(screen.getByText("Filtered logs")).toBeInTheDocument();
    });
  });
});
