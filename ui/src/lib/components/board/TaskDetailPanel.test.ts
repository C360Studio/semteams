import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/svelte";
import userEvent from "@testing-library/user-event";
import TaskDetailPanel from "./TaskDetailPanel.svelte";
import type { TaskInfo } from "$lib/types/task";
import type { AgentLoop } from "$lib/types/agent";

// Mock agentApi — covers TaskDetailPanel's sendSignal calls,
// PendingApprovalSection's submitApproval calls, and TrajectoryViewer's
// getTrajectory/getTrajectories calls.
vi.mock("$lib/services/agentApi", () => ({
  agentApi: {
    sendSignal: vi.fn().mockResolvedValue({ status: "sent" }),
    submitApproval: vi.fn().mockResolvedValue({
      loop_id: "loop_001",
      decision: "approve",
      accepted: true,
      timestamp: "2026-04-29T11:00:00Z",
    }),
    getTrajectory: vi.fn().mockResolvedValue({
      loop_id: "",
      role: "",
      iterations: 0,
      outcome: "",
      duration_ms: 0,
    }),
    getTrajectories: vi.fn().mockResolvedValue([]),
    // TaskStory polls /teams-loop/trajectories/<loop_id>; record the
    // arg so the focused-loop tests can assert which loop's story is
    // being shown.
    getLoopTrajectory: vi.fn().mockImplementation((loopId: string) =>
      Promise.resolve({
        loop_id: loopId,
        start_time: "2026-06-03T00:00:00Z",
        steps: [],
      }),
    ),
    // TaskTrace polls /message-logger/entries; used by the raw-activity
    // disclosure inside TaskStory. Default to empty so the disclosure
    // doesn't blow up when opened.
    getMessages: vi.fn().mockResolvedValue([]),
  },
  AgentApiError: class AgentApiError extends Error {
    statusCode: number;
    constructor(message: string, statusCode: number) {
      super(message);
      this.name = "AgentApiError";
      this.statusCode = statusCode;
    }
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

    it("renders PendingApprovalSection and Cancel for awaiting_approval state", () => {
      render(TaskDetailPanel, {
        props: {
          task: makeTask({
            state: "awaiting_approval",
            primaryLoop: makeLoop({
              state: "awaiting_approval",
              pending_approval: {
                call_id: "call-1",
                tool_name: "create_rule",
                arguments: { name: "alert" },
                requested_at: "2026-04-29T11:00:00Z",
              },
            }),
          }),
        },
      });

      // Approve / Reject now live inside PendingApprovalSection — the
      // panel header surface only carries Cancel for this state.
      expect(screen.getByTestId("pending-approval-section")).toBeInTheDocument();
      expect(screen.getByTestId("approval-approve")).toBeInTheDocument();
      expect(screen.getByTestId("approval-reject")).toBeInTheDocument();
      expect(screen.getByText("Cancel")).toBeInTheDocument();
    });

    it("hides PendingApprovalSection when pending_approval is absent", () => {
      // Defensive: state is awaiting_approval but the dispatch hasn't
      // populated the snapshot yet (transient race). Render the row
      // without the section instead of crashing.
      render(TaskDetailPanel, {
        props: { task: makeTask({ state: "awaiting_approval" }) },
      });

      expect(
        screen.queryByTestId("pending-approval-section"),
      ).not.toBeInTheDocument();
      expect(screen.queryByTestId("approval-approve")).not.toBeInTheDocument();
    });

    it("shows no action buttons for complete state", () => {
      render(TaskDetailPanel, { props: { task: makeTask({ state: "complete" }) } });

      expect(screen.queryByText("Pause")).not.toBeInTheDocument();
      expect(screen.queryByTestId("approval-approve")).not.toBeInTheDocument();
    });
  });

  describe("signal dispatch", () => {
    it("approve click flows through submitApproval (not sendSignal)", async () => {
      const user = userEvent.setup();
      render(TaskDetailPanel, {
        props: {
          task: makeTask({
            id: "loop_xyz",
            state: "awaiting_approval",
            primaryLoop: makeLoop({
              loop_id: "loop_xyz",
              state: "awaiting_approval",
              pending_approval: {
                call_id: "call-xyz",
                tool_name: "create_rule",
                arguments: { name: "alert" },
                requested_at: "2026-04-29T11:00:00Z",
              },
            }),
          }),
        },
      });

      await user.click(screen.getByTestId("approval-approve"));

      expect(agentApi.submitApproval).toHaveBeenCalledWith(
        "loop_xyz",
        expect.objectContaining({ decision: "approve" }),
      );
      // Old broken signal path is gone.
      expect(agentApi.sendSignal).not.toHaveBeenCalled();
    });

    it("Cancel click still uses sendSignal during awaiting_approval", async () => {
      const user = userEvent.setup();
      render(TaskDetailPanel, {
        props: {
          task: makeTask({
            id: "loop_cancel",
            state: "awaiting_approval",
            primaryLoop: makeLoop({
              loop_id: "loop_cancel",
              state: "awaiting_approval",
              pending_approval: {
                call_id: "call-c",
                tool_name: "delete_rule",
                arguments: {},
                requested_at: "2026-04-29T11:00:00Z",
              },
            }),
          }),
        },
      });

      await user.click(screen.getByText("Cancel"));

      expect(agentApi.sendSignal).toHaveBeenCalledWith("loop_cancel", "cancel");
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

      // "Sub-tasks" reads less internal than "Sub-loops" / "Child
      // Loops" — the user thinks of them as tasks, not loops.
      expect(screen.getByText("Sub-tasks (2)")).toBeInTheDocument();
      expect(screen.getByText("researcher")).toBeInTheDocument();
      expect(screen.getByText("editor")).toBeInTheDocument();
    });

    it("hides child loop section when no children", () => {
      render(TaskDetailPanel, { props: { task: makeTask() } });

      expect(screen.queryByText(/Sub-tasks/)).not.toBeInTheDocument();
    });

    it("renders each child as a clickable button", () => {
      const children = [
        makeLoop({ loop_id: "c1", role: "researcher" }),
        makeLoop({ loop_id: "c2", role: "editor" }),
      ];
      render(TaskDetailPanel, {
        props: { task: makeTask({ childLoops: children }) },
      });

      const items = screen.getAllByTestId("child-item");
      expect(items).toHaveLength(2);
      // <button>, not <li>. Carries the loop_id for drill-in selection.
      items.forEach((el) => {
        expect(el.tagName).toBe("BUTTON");
      });
      expect(items[0]).toHaveAttribute("data-loop-id", "c1");
      expect(items[1]).toHaveAttribute("data-loop-id", "c2");
    });

    it("primary loop's trajectory is the default focus", async () => {
      render(TaskDetailPanel, {
        props: {
          task: makeTask({
            id: "loop_parent",
            primaryLoop: makeLoop({ loop_id: "loop_parent" }),
            childLoops: [makeLoop({ loop_id: "c1", role: "researcher" })],
          }),
        },
      });
      // TaskStory's $effect kicks the initial fetch with the primary
      // loop_id. No breadcrumb until a child is focused.
      await vi.waitFor(() => {
        expect(agentApi.getLoopTrajectory).toHaveBeenCalledWith("loop_parent");
      });
      expect(screen.queryByTestId("focus-breadcrumb")).not.toBeInTheDocument();
    });

    it("clicking a child swaps the right-rail to the child's trajectory", async () => {
      const user = userEvent.setup();
      render(TaskDetailPanel, {
        props: {
          task: makeTask({
            id: "loop_parent",
            primaryLoop: makeLoop({ loop_id: "loop_parent" }),
            childLoops: [
              makeLoop({ loop_id: "c1", role: "researcher-research-plan" }),
              makeLoop({ loop_id: "c2", role: "researcher-research-synthesize" }),
            ],
          }),
        },
      });

      const synth = screen
        .getAllByTestId("child-item")
        .find((el) => el.getAttribute("data-loop-id") === "c2");
      expect(synth).toBeDefined();
      await user.click(synth!);

      await vi.waitFor(() => {
        expect(agentApi.getLoopTrajectory).toHaveBeenCalledWith("c2");
      });
      // Visual confirmation: breadcrumb appears with the parent title +
      // current focus role.
      const crumb = screen.getByTestId("focus-breadcrumb");
      expect(crumb).toHaveTextContent(/Back to/);
      expect(crumb).toHaveTextContent("researcher-research-synthesize");
      // The focused child also wears a pressed state for screen readers.
      expect(synth).toHaveAttribute("aria-pressed", "true");
    });

    it("breadcrumb back-button returns focus to the primary loop", async () => {
      const user = userEvent.setup();
      render(TaskDetailPanel, {
        props: {
          task: makeTask({
            id: "loop_parent",
            primaryLoop: makeLoop({ loop_id: "loop_parent" }),
            childLoops: [makeLoop({ loop_id: "c1", role: "researcher" })],
          }),
        },
      });

      const child = screen.getByTestId("child-item");
      await user.click(child);
      await vi.waitFor(() => {
        expect(agentApi.getLoopTrajectory).toHaveBeenCalledWith("c1");
      });

      await user.click(screen.getByTestId("focus-back"));

      // After the back-click, the breadcrumb disappears and the panel
      // re-polls the primary loop. Use waitFor on the API call because
      // the polling refresh is asynchronous.
      await vi.waitFor(() => {
        const calls = (agentApi.getLoopTrajectory as ReturnType<typeof vi.fn>)
          .mock.calls;
        // Last call should be back to the primary.
        expect(calls[calls.length - 1][0]).toBe("loop_parent");
      });
      expect(screen.queryByTestId("focus-breadcrumb")).not.toBeInTheDocument();
      expect(child).toHaveAttribute("aria-pressed", "false");
    });

    it("focus does not leak across task changes", async () => {
      const user = userEvent.setup();
      // Initial: task A with one child.
      const { rerender } = render(TaskDetailPanel, {
        props: {
          task: makeTask({
            id: "loop_A",
            primaryLoop: makeLoop({ loop_id: "loop_A" }),
            childLoops: [makeLoop({ loop_id: "a_child", role: "researcher" })],
          }),
        },
      });

      await user.click(screen.getByTestId("child-item"));
      await vi.waitFor(() => {
        expect(agentApi.getLoopTrajectory).toHaveBeenCalledWith("a_child");
      });

      // Now switch to task B — A's focused child doesn't exist here.
      // The derived guard should fall back to B's primary, NOT keep
      // polling the stale a_child id.
      await rerender({
        task: makeTask({
          id: "loop_B",
          primaryLoop: makeLoop({ loop_id: "loop_B" }),
          childLoops: [makeLoop({ loop_id: "b_child", role: "editor" })],
        }),
      });

      await vi.waitFor(() => {
        const calls = (agentApi.getLoopTrajectory as ReturnType<typeof vi.fn>)
          .mock.calls;
        expect(calls[calls.length - 1][0]).toBe("loop_B");
      });
      expect(screen.queryByTestId("focus-breadcrumb")).not.toBeInTheDocument();
    });
  });

  describe("tabs", () => {
    it("renders three tabs (Activity is the only one with content today)", () => {
      // Trace tab was folded into Activity (under "Show raw activity"
      // toggle) — having two tabs for the same data confused users
      // who didn't know our internal taxonomy.
      render(TaskDetailPanel, { props: { task: makeTask() } });
      expect(screen.getByTestId("tab-activity")).toBeInTheDocument();
      expect(screen.getByTestId("tab-entities")).toBeInTheDocument();
      expect(screen.getByTestId("tab-logs")).toBeInTheDocument();
      expect(screen.queryByTestId("tab-trace")).not.toBeInTheDocument();
    });

    it("Activity is the default active tab", () => {
      render(TaskDetailPanel, { props: { task: makeTask() } });
      expect(screen.getByTestId("tab-activity")).toHaveAttribute(
        "aria-selected",
        "true",
      );
      expect(screen.getByTestId("panel-activity")).toBeInTheDocument();
      expect(screen.queryByTestId("panel-entities")).not.toBeInTheDocument();
    });

    it("clicking a tab switches the visible panel", async () => {
      const user = userEvent.setup();
      render(TaskDetailPanel, { props: { task: makeTask() } });

      await user.click(screen.getByTestId("tab-entities"));

      expect(screen.getByTestId("tab-entities")).toHaveAttribute(
        "aria-selected",
        "true",
      );
      expect(screen.getByTestId("panel-entities")).toBeInTheDocument();
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
