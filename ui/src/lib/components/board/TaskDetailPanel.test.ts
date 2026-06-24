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
    sendMessage: vi.fn().mockResolvedValue({ content: "ok" }),
    sendSignal: vi.fn().mockResolvedValue({
      loop_id: "loop_001",
      signal: "pause",
      status: "sent",
    }),
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

vi.mock("$lib/services/runStatusApi", () => ({
  getTriples: vi.fn().mockResolvedValue([]),
}));

vi.mock("$lib/services/messageLoggerApi", () => ({
  messageLoggerApi: {
    fetchEntries: vi.fn().mockResolvedValue([]),
  },
  entryMentionsLoop: vi.fn(() => false),
  classifyEntry: vi.fn(() => "other"),
}));

import { agentApi } from "$lib/services/agentApi";
import { getTriples } from "$lib/services/runStatusApi";
import {
  classifyEntry,
  entryMentionsLoop,
  messageLoggerApi,
} from "$lib/services/messageLoggerApi";

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
    runPause: null,
    runHealth: null,
    ...overrides,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(agentApi.sendMessage).mockResolvedValue({ content: "ok" });
  vi.mocked(agentApi.sendSignal).mockResolvedValue({
    loop_id: "loop_001",
    signal: "pause",
    status: "sent",
  });
  vi.mocked(agentApi.submitApproval).mockResolvedValue({
    loop_id: "loop_001",
    decision: "approve",
    accepted: true,
    timestamp: "2026-04-29T11:00:00Z",
  });
  vi.mocked(agentApi.getTrajectory).mockResolvedValue({
    loop_id: "",
    role: "",
    iterations: 0,
    outcome: "",
    duration_ms: 0,
  });
  vi.mocked(agentApi.getTrajectories).mockResolvedValue([]);
  vi.mocked(agentApi.getLoopTrajectory).mockImplementation((loopId: string) =>
    Promise.resolve({
      loop_id: loopId,
      start_time: "2026-06-03T00:00:00Z",
      steps: [],
    }),
  );
  vi.mocked(getTriples).mockResolvedValue([]);
  vi.mocked(messageLoggerApi.fetchEntries).mockResolvedValue([]);
  vi.mocked(entryMentionsLoop).mockReturnValue(false);
  vi.mocked(classifyEntry).mockReturnValue("other");
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

  describe("run-level waiting affordance (ADR-053 4b-2 / 4c)", () => {
    it("renders RunWaitingSection when the run is paused (clarification)", () => {
      // The front-door loop is typically `complete` when a descendant pauses,
      // so the run-pause surfaces via task.runPause, not the loop state.
      render(TaskDetailPanel, {
        props: {
          task: makeTask({
            state: "complete",
            runPause: {
              cause: "clarification",
              askingLoopId: "loop-ask-1",
              question: "Which region?",
            },
          }),
        },
      });

      expect(screen.getByTestId("run-waiting-section")).toBeInTheDocument();
      expect(screen.getByTestId("run-reply-input")).toBeInTheDocument();
      expect(screen.getByTestId("run-question")).toHaveTextContent("Which region?");
    });

    it("renders RunWaitingSection when the run is paused (tool_gate)", () => {
      render(TaskDetailPanel, {
        props: {
          task: makeTask({
            state: "complete",
            runPause: { cause: "tool_gate", gatedLoopId: "loop-gated-1" },
          }),
        },
      });

      expect(screen.getByTestId("run-waiting-section")).toBeInTheDocument();
      expect(screen.getByTestId("run-approve")).toBeInTheDocument();
      expect(screen.getByTestId("run-reject")).toBeInTheDocument();
    });

    it("does NOT render RunWaitingSection when runPause is null", () => {
      render(TaskDetailPanel, { props: { task: makeTask({ runPause: null }) } });

      expect(screen.queryByTestId("run-waiting-section")).not.toBeInTheDocument();
    });
  });

  describe("run health", () => {
    it("renders the run health gate, next action, evidence freshness, and active loop count", () => {
      render(TaskDetailPanel, {
        props: {
          task: makeTask({
            runHealth: {
              state: "working",
              label: "Working",
              currentGate: "reviewer-dev-via-test reviewing",
              nextAction: "Wait for the current loop or gate to emit evidence",
              detail: "reviewer-dev-via-test is reviewing.",
              evidenceFreshness: "fresh",
              activeLoopCount: 1,
              signals: [],
            },
          }),
        },
      });

      const panel = screen.getByTestId("run-health-panel");
      expect(panel).toHaveTextContent("Working");
      expect(panel).toHaveTextContent("reviewer-dev-via-test reviewing");
      expect(panel).toHaveTextContent("Wait for the current loop or gate to emit evidence");
      expect(panel).toHaveTextContent("fresh");
      expect(panel).toHaveTextContent("1");
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
    it("renders Activity and Evidence tabs", () => {
      render(TaskDetailPanel, { props: { task: makeTask() } });
      expect(screen.getByTestId("tab-activity")).toBeInTheDocument();
      expect(screen.getByTestId("tab-evidence")).toBeInTheDocument();
      expect(screen.queryByTestId("tab-entities")).not.toBeInTheDocument();
      expect(screen.queryByTestId("tab-logs")).not.toBeInTheDocument();
      expect(screen.queryByTestId("tab-trace")).not.toBeInTheDocument();
    });

    it("Activity is the default active tab", () => {
      render(TaskDetailPanel, { props: { task: makeTask() } });
      expect(screen.getByTestId("tab-activity")).toHaveAttribute(
        "aria-selected",
        "true",
      );
      expect(screen.getByTestId("panel-activity")).toBeInTheDocument();
      expect(screen.queryByTestId("panel-evidence")).not.toBeInTheDocument();
    });

    it("clicking a tab switches the visible panel", async () => {
      const user = userEvent.setup();
      render(TaskDetailPanel, { props: { task: makeTask() } });

      await user.click(screen.getByTestId("tab-evidence"));

      expect(screen.getByTestId("tab-evidence")).toHaveAttribute(
        "aria-selected",
        "true",
      );
      expect(screen.getByTestId("panel-evidence")).toBeInTheDocument();
      expect(screen.getByTestId("run-evidence-panel")).toBeInTheDocument();
      expect(screen.queryByTestId("panel-activity")).not.toBeInTheDocument();
    });

    it("Evidence tab keeps raw receipts behind the summary", async () => {
      const user = userEvent.setup();
      render(TaskDetailPanel, {
        props: {
          task: makeTask({
            runHealth: {
              state: "working",
              label: "Working",
              runEntityId: "c360.semteams.agent.chain.execution.loop_001",
              currentGate: "coordinator executing",
              nextAction: "Wait for evidence",
              detail: "Coordinator is executing.",
              evidenceFreshness: "fresh",
              activeLoopCount: 1,
              signals: [],
            },
          }),
        },
      });

      await user.click(screen.getByTestId("tab-evidence"));
      expect(screen.getByText("Raw trajectory")).toBeInTheDocument();
      expect(screen.getByText("Message log")).toBeInTheDocument();
      expect(screen.getByText("Run graph triples")).toBeInTheDocument();
    });
  });
});
