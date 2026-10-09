import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/svelte";
import userEvent from "@testing-library/user-event";
import TaskDetailPanel from "./TaskDetailPanel.svelte";
import type { TaskInfo } from "$lib/types/task";
import type { AgentLoop, LoopTrajectory } from "$lib/types/agent";
import type { TaskControl } from "$lib/types/control";

// Function declarations are hoisted, so this is safely callable from the
// vi.mock factory below (which Vitest hoists above the imports, but only
// executes lazily on first import of the mocked module).
function makeEmptyTrajectory(loopId: string): LoopTrajectory {
  return {
    schema_version: "v1",
    loop_id: loopId,
    coverage: "observed",
    terminal_observed: false,
    observed_totals: {
      facts: 0,
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
    facts: [],
  };
}

// Mock agentApi — covers TaskDetailPanel's cancelLoop calls,
// PendingApprovalSection's submitApproval calls, and TaskStory /
// RunEvidencePanel's getLoopTrajectory calls.
vi.mock("$lib/services/agentApi", () => ({
  agentApi: {
    sendMessage: vi.fn().mockResolvedValue({ content: "ok" }),
    cancelLoop: vi.fn().mockResolvedValue({
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
    // TaskStory/RunEvidencePanel poll the GraphQL trajectory(loopId)
    // field (semstreams beta.160); record the arg so the focused-loop
    // tests can assert which loop's story is being shown.
    getLoopTrajectory: vi
      .fn()
      .mockImplementation((loopId: string) =>
        Promise.resolve(makeEmptyTrajectory(loopId)),
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
    controls: [],
    controlsTruncated: false,
    ...overrides,
  };
}

function makeControl(
  loopId: string,
  overrides: Partial<AgentLoop> = {},
  runInstance = "loop_001",
): TaskControl {
  const prefix = "c360.semteams-bootstrap-e2e-6c50f3";
  return {
    loopId,
    loopEntityId: `${prefix}.agentic-loop.agent.execution.${loopId}`,
    firingEntityId: `${prefix}.chain.agent.execution.${runInstance}`,
    runInstance,
    spawnedAt: new Date(1791480613893),
    spawnedAtNanos: 1791480613893413505n,
    loop: makeLoop({
      loop_id: loopId,
      role: "ops-chain-observer",
      state: "complete",
      ...overrides,
    }),
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(agentApi.sendMessage).mockResolvedValue({ content: "ok" });
  vi.mocked(agentApi.cancelLoop).mockResolvedValue({ content: "Cancellation requested" });
  vi.mocked(agentApi.submitApproval).mockResolvedValue({
    loop_id: "loop_001",
    decision: "approve",
    accepted: true,
    timestamp: "2026-04-29T11:00:00Z",
  });
  vi.mocked(agentApi.getLoopTrajectory).mockImplementation((loopId: string) =>
    Promise.resolve(makeEmptyTrajectory(loopId)),
  );
  vi.mocked(getTriples).mockResolvedValue([]);
  vi.mocked(messageLoggerApi.fetchEntries).mockResolvedValue([]);
  vi.mocked(entryMentionsLoop).mockReturnValue(false);
  vi.mocked(classifyEntry).mockReturnValue("other");
});

describe("TaskDetailPanel", () => {
  it("renders task title and state", () => {
    render(TaskDetailPanel, {
      props: { task: makeTask({ title: "My Task", state: "planning" }) },
    });

    expect(screen.getByText("My Task")).toBeInTheDocument();
  });

  it("renders role and iteration count", () => {
    render(TaskDetailPanel, {
      props: {
        task: makeTask({ role: "editor", iterations: 5, maxIterations: 20 }),
      },
    });

    expect(screen.getByText("editor")).toBeInTheDocument();
    expect(screen.getByText("5/20 iterations")).toBeInTheDocument();
  });

  it("renders truncated ID with ellipsis", () => {
    render(TaskDetailPanel, {
      props: { task: makeTask({ id: "loop_abcdef123456" }) },
    });

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
    it("shows Cancel without unsupported Pause for active states", () => {
      render(TaskDetailPanel, {
        props: { task: makeTask({ state: "executing" }) },
      });

      expect(screen.queryByText("Pause")).not.toBeInTheDocument();
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
                execution_id: "execution-pending",
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
      expect(
        screen.getByTestId("pending-approval-section"),
      ).toBeInTheDocument();
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
      render(TaskDetailPanel, {
        props: { task: makeTask({ state: "complete" }) },
      });

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
      expect(screen.getByTestId("run-question")).toHaveTextContent(
        "Which region?",
      );
    });

    it("renders RunWaitingSection when the run is paused (tool_gate)", () => {
      render(TaskDetailPanel, {
        props: {
          task: makeTask({
            state: "complete",
            runPause: { cause: "tool_gate", gatedLoopId: "loop-gated-1", executionId: "execution-pending" },
          }),
        },
      });

      expect(screen.getByTestId("run-waiting-section")).toBeInTheDocument();
      expect(screen.getByTestId("run-approve")).toBeInTheDocument();
      expect(screen.getByTestId("run-reject")).toBeInTheDocument();
    });

    it("does NOT render RunWaitingSection when runPause is null", () => {
      render(TaskDetailPanel, {
        props: { task: makeTask({ runPause: null }) },
      });

      expect(
        screen.queryByTestId("run-waiting-section"),
      ).not.toBeInTheDocument();
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
      expect(panel).toHaveTextContent(
        "Wait for the current loop or gate to emit evidence",
      );
      expect(panel).toHaveTextContent("fresh");
      expect(panel).toHaveTextContent("1");
    });
  });

  describe("signal dispatch", () => {
    it("approve click flows through submitApproval (not cancelLoop)", async () => {
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
                execution_id: "execution-pending",
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
      expect(agentApi.cancelLoop).not.toHaveBeenCalled();
    });

    it("Cancel click still uses cancelLoop during awaiting_approval", async () => {
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
                execution_id: "execution-pending",
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

      expect(agentApi.cancelLoop).toHaveBeenCalledWith("loop_cancel");
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
              makeLoop({
                loop_id: "c2",
                role: "researcher-research-synthesize",
              }),
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

    it("breadcrumb back-button hands keyboard focus to the sub-task that was focused", async () => {
      const user = userEvent.setup();
      render(TaskDetailPanel, {
        props: {
          task: makeTask({
            id: "loop_parent",
            primaryLoop: makeLoop({ loop_id: "loop_parent" }),
            childLoops: [
              makeLoop({ loop_id: "c1", role: "researcher" }),
              makeLoop({ loop_id: "c2", role: "reviewer" }),
            ],
          }),
        },
      });

      const second = screen.getAllByTestId("child-item")[1];
      await user.click(second);
      const back = await screen.findByTestId("focus-back");
      back.focus();
      await user.keyboard("{Enter}");

      await vi.waitFor(() => expect(screen.getAllByTestId("child-item")[1]).toHaveFocus());
    });

    it("names the back button by its visible text (WCAG 2.5.3)", async () => {
      const user = userEvent.setup();
      render(TaskDetailPanel, {
        props: {
          task: makeTask({ childLoops: [makeLoop({ loop_id: "c1", role: "researcher" })] }),
        },
      });

      await user.click(screen.getByTestId("child-item"));

      const back = await screen.findByTestId("focus-back");
      const visible = (back.textContent ?? "").replace(/\s+/g, " ").trim();
      expect(visible).toMatch(/^← Back to /);
      expect(back).toHaveAccessibleName(visible);
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

  describe("controls (design D3)", () => {
    it("hides the Controls section when the run fired none", () => {
      render(TaskDetailPanel, { props: { task: makeTask() } });

      expect(screen.queryByTestId("controls-section")).not.toBeInTheDocument();
      expect(screen.queryByText(/Controls \(/)).not.toBeInTheDocument();
    });

    it("lists controls beside, not inside, the sub-tasks", () => {
      render(TaskDetailPanel, {
        props: {
          task: makeTask({
            childLoops: [makeLoop({ loop_id: "c1", role: "researcher" })],
            controls: [makeControl("obs-1")],
          }),
        },
      });

      expect(screen.getByText("Sub-tasks (1)")).toBeInTheDocument();
      expect(screen.getByText("Controls (1)")).toBeInTheDocument();
      expect(screen.getAllByTestId("child-item")).toHaveLength(1);
      const rows = screen.getAllByTestId("control-row");
      expect(rows).toHaveLength(1);
      expect(rows[0].tagName).toBe("BUTTON");
      expect(rows[0]).toHaveAttribute("data-loop-id", "obs-1");
      // Text, not colour: state, role and the derived outcome are all spelled out.
      expect(rows[0]).toHaveTextContent("complete");
      expect(rows[0]).toHaveTextContent("ops-chain-observer");
      expect(rows[0]).toHaveTextContent("applied");
    });

    it("a control is not a sub-task and does not appear in the Sub-tasks count", () => {
      render(TaskDetailPanel, {
        props: { task: makeTask({ controls: [makeControl("obs-1")] }) },
      });

      expect(screen.queryByText(/Sub-tasks/)).not.toBeInTheDocument();
      expect(screen.queryByTestId("child-item")).not.toBeInTheDocument();
    });

    it("says the list may be incomplete when a rule-fired loop on the run could not be classified", () => {
      render(TaskDetailPanel, {
        props: {
          task: makeTask({ controls: [makeControl("obs-1")], controlsTruncated: true }),
        },
      });

      expect(screen.getByTestId("controls-truncated-note")).toHaveTextContent(
        "may be incomplete",
      );
    });

    it("can be focused from the keyboard, with a pressed state and the breadcrumb", async () => {
      const user = userEvent.setup();
      render(TaskDetailPanel, {
        props: {
          task: makeTask({
            id: "loop_001",
            controls: [makeControl("obs-1")],
          }),
        },
      });

      const row = screen.getByTestId("control-row");
      expect(row).toHaveAttribute("aria-pressed", "false");

      row.focus();
      expect(row).toHaveFocus();
      await user.keyboard("{Enter}");

      await vi.waitFor(() => {
        expect(agentApi.getLoopTrajectory).toHaveBeenCalledWith("obs-1");
      });
      expect(row).toHaveAttribute("aria-pressed", "true");
      const crumb = screen.getByTestId("focus-breadcrumb");
      expect(crumb).toHaveTextContent("ops-chain-observer");
      // The coordinator's Controls explanation belongs to the coordinator story.
      expect(screen.queryByTestId("story-controls")).not.toBeInTheDocument();

      await user.click(screen.getByTestId("focus-back"));
      expect(row).toHaveAttribute("aria-pressed", "false");
      expect(await screen.findByTestId("story-controls")).toBeInTheDocument();
    });

    it("also activates with Space", async () => {
      const user = userEvent.setup();
      render(TaskDetailPanel, {
        props: { task: makeTask({ controls: [makeControl("obs-1")] }) },
      });

      screen.getByTestId("control-row").focus();
      await user.keyboard(" ");

      await vi.waitFor(() => {
        expect(agentApi.getLoopTrajectory).toHaveBeenCalledWith("obs-1");
      });
    });

    it("explains the control in the story and focuses it from the story's identity button", async () => {
      const user = userEvent.setup();
      render(TaskDetailPanel, {
        props: { task: makeTask({ controls: [makeControl("obs-1")] }) },
      });

      const event = await screen.findByTestId("control-event");
      expect(event).toHaveAttribute("data-loop", "obs-1");
      expect(event).toHaveTextContent("rule: unknown");

      await user.click(screen.getByTestId("control-identity"));

      await vi.waitFor(() => {
        expect(agentApi.getLoopTrajectory).toHaveBeenCalledWith("obs-1");
      });
      expect(screen.getByTestId("control-row")).toHaveAttribute("aria-pressed", "true");
    });

    it("hands keyboard focus to the back button when the story's identity button is activated", async () => {
      // Focusing a control swaps the story for the control's own, which unmounts
      // the identity button that was just pressed. Focus must not fall to <body>.
      const user = userEvent.setup();
      render(TaskDetailPanel, {
        props: { task: makeTask({ controls: [makeControl("obs-1")] }) },
      });

      const identity = await screen.findByTestId("control-identity");
      identity.focus();
      expect(identity).toHaveFocus();
      await user.keyboard("{Enter}");

      await vi.waitFor(() => {
        expect(screen.getByTestId("focus-back")).toHaveFocus();
      });
      expect(screen.queryByTestId("control-identity")).not.toBeInTheDocument();

      // Back out with the keyboard: the coordinator's story returns.
      await user.keyboard("{Enter}");
      expect(await screen.findByTestId("story-controls")).toBeInTheDocument();
    });

    it("returns keyboard focus to the control's row when the back button is activated", async () => {
      // Going back unmounts the breadcrumb that held focus; the row for the loop
      // that was focused is the way back to where the operator was.
      const user = userEvent.setup();
      render(TaskDetailPanel, {
        props: { task: makeTask({ controls: [makeControl("obs-1"), makeControl("obs-2")] }) },
      });

      const identities = await screen.findAllByTestId("control-identity");
      identities[1].focus();
      await user.keyboard("{Enter}");
      await vi.waitFor(() => expect(screen.getByTestId("focus-back")).toHaveFocus());

      await user.keyboard("{Enter}");

      await vi.waitFor(() => {
        const rows = screen.getAllByTestId("control-row");
        expect(rows.find((row) => row.getAttribute("data-loop-id") === "obs-2")).toHaveFocus();
      });
      expect(screen.queryByTestId("focus-breadcrumb")).not.toBeInTheDocument();
    });

    it("keeps focus on the list row that was activated; only the story's button hands focus over", async () => {
      const user = userEvent.setup();
      render(TaskDetailPanel, {
        props: { task: makeTask({ controls: [makeControl("obs-1")] }) },
      });

      const row = screen.getByTestId("control-row");
      row.focus();
      await user.keyboard("{Enter}");
      await vi.waitFor(() => {
        expect(agentApi.getLoopTrajectory).toHaveBeenCalledWith("obs-1");
      });

      expect(row).toHaveFocus();
    });

    it("has one Controls heading in the tab panel; the story's group is named for what it holds", async () => {
      render(TaskDetailPanel, {
        props: { task: makeTask({ controls: [makeControl("obs-1")] }) },
      });

      expect(await screen.findByRole("heading", { name: "Controls fired on this run" })).toBeInTheDocument();
      expect(screen.getAllByRole("heading", { name: /^Controls/ })).toHaveLength(2);
      expect(screen.getByRole("heading", { name: "Controls (1)" })).toBeInTheDocument();
      expect(screen.queryByRole("heading", { name: "Controls" })).not.toBeInTheDocument();
    });

    it("falls back to the coordinator when the focused control is gone", async () => {
      const user = userEvent.setup();
      const { rerender } = render(TaskDetailPanel, {
        props: { task: makeTask({ id: "loop_001", controls: [makeControl("obs-1")] }) },
      });

      await user.click(screen.getByTestId("control-row"));
      await vi.waitFor(() => {
        expect(agentApi.getLoopTrajectory).toHaveBeenCalledWith("obs-1");
      });

      await rerender({ task: makeTask({ id: "loop_001", controls: [] }) });

      await vi.waitFor(() => {
        const calls = (agentApi.getLoopTrajectory as ReturnType<typeof vi.fn>).mock.calls;
        expect(calls[calls.length - 1][0]).toBe("loop_001");
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
              runEntityId: "c360.semteams.chain.agent.execution.loop_001",
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
