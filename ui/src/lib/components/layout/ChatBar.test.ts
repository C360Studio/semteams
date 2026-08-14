import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/svelte";
import userEvent from "@testing-library/user-event";
import { chatHandoff } from "$lib/stores/chatHandoff.svelte";
import ChatBar from "./ChatBar.svelte";

// ---------------------------------------------------------------------------
// Mock agentApi
// ---------------------------------------------------------------------------

vi.mock("$lib/services/agentApi", () => ({
  agentApi: {
    sendMessage: vi.fn().mockResolvedValue({ content: "ok" }),
    sendSignal: vi.fn().mockResolvedValue({ status: "sent" }),
  },
}));

// Mock taskStore with controllable selectedTask + attention queue.
// vi.mock hoists the entire call (including factory body) to the top
// of the file, before const declarations — so direct references to
// outer vars inside the factory hit the temporal dead zone. Use
// vi.hoisted for all the controllable spies; getters in the factory
// body are still safe (they run lazily on access).
const {
  mockSelectedTask,
  mockNeedsYou,
  mockNeedsAttentionCount,
  mockSelectTask,
} = vi.hoisted(() => ({
  mockSelectedTask: vi.fn().mockReturnValue(undefined),
  mockNeedsYou: vi.fn().mockReturnValue([]),
  mockNeedsAttentionCount: vi.fn().mockReturnValue(0),
  mockSelectTask: vi.fn(),
}));

vi.mock("$lib/stores/taskStore.svelte", () => ({
  taskStore: {
    get selectedTask() {
      return mockSelectedTask();
    },
    get byColumn() {
      return {
        needs_you: mockNeedsYou(),
        thinking: [],
        executing: [],
        done: [],
        failed: [],
      };
    },
    get needsAttentionCount() {
      return mockNeedsAttentionCount();
    },
    selectTask: (id: string) => mockSelectTask(id),
    deselectTask: vi.fn(),
  },
}));

import { agentApi } from "$lib/services/agentApi";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function selectedTask(overrides: Record<string, unknown> = {}) {
  return {
    id: "loop_test",
    title: "Test Task",
    state: "executing",
    column: "executing",
    role: "general",
    iterations: 1,
    maxIterations: 10,
    primaryLoop: {},
    childLoops: [],
    childNeedsAttention: false,
    childAttentionCount: 0,
    ...overrides,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  chatHandoff.clearAll();
  mockSelectedTask.mockReturnValue(undefined);
  mockNeedsYou.mockReturnValue([]);
  mockNeedsAttentionCount.mockReturnValue(0);
});

// ---------------------------------------------------------------------------
// Rendering
// ---------------------------------------------------------------------------

describe("ChatBar — initial render", () => {
  it("renders input and send button", () => {
    render(ChatBar);

    expect(screen.getByTestId("chat-input")).toBeInTheDocument();
    expect(screen.getByTestId("send-button")).toBeInTheDocument();
  });

  it("shows 'What should the team work on?' placeholder when no task selected", () => {
    render(ChatBar);

    expect(screen.getByTestId("chat-input")).toHaveAttribute(
      "placeholder",
      "What should the team work on?",
    );
  });

  it("shows 'Reply to this task…' placeholder when a task is selected", () => {
    mockSelectedTask.mockReturnValue(selectedTask());
    render(ChatBar);

    expect(screen.getByTestId("chat-input")).toHaveAttribute(
      "placeholder",
      "Reply to this task…",
    );
  });

  it("shows the task chip when a task is selected", () => {
    mockSelectedTask.mockReturnValue(selectedTask());
    render(ChatBar);

    expect(screen.getByTestId("chat-context")).toBeInTheDocument();
    expect(screen.getByText("Test Task")).toBeInTheDocument();
  });

  it("does NOT show technical chrome (no Task: label, no state badge)", () => {
    // Per ui-redesign: the chip-shape says "this is a task ref" — a
    // "Task:" label is redundant. The state badge was duplicated from
    // the card the user just clicked. Both removed.
    mockSelectedTask.mockReturnValue(selectedTask({ state: "executing" }));
    render(ChatBar);

    expect(screen.queryByText("Task:")).not.toBeInTheDocument();
    // No state badge text either
    expect(screen.queryByText("executing")).not.toBeInTheDocument();
  });

  it("does NOT show slash hint chips by default", () => {
    // Slash hints are technical chrome that intimidates new users.
    // They surface only when the user is actively typing "/...".
    mockSelectedTask.mockReturnValue(selectedTask());
    render(ChatBar);

    expect(screen.queryByText("/research")).not.toBeInTheDocument();
    expect(screen.queryByText("/approve")).not.toBeInTheDocument();
    expect(screen.queryByText("/reject")).not.toBeInTheDocument();
  });

  it("does NOT show 'Type a task' hint", () => {
    // Removed — the placeholder text already says it.
    render(ChatBar);

    expect(
      screen.queryByText("Type a task to get started"),
    ).not.toBeInTheDocument();
  });

  it("shows slash hints only after the user types '/'", async () => {
    mockSelectedTask.mockReturnValue(selectedTask());
    const user = userEvent.setup();
    render(ChatBar);

    expect(screen.queryByText("/research")).not.toBeInTheDocument();
    expect(screen.queryByText("/approve")).not.toBeInTheDocument();

    await user.type(screen.getByTestId("chat-input"), "/");

    expect(screen.getByText("/research")).toBeInTheDocument();
    expect(screen.getByText("/optimize")).toBeInTheDocument();
    expect(screen.getByText("/approve")).toBeInTheDocument();
    expect(screen.getByText("/reject")).toBeInTheDocument();
    // Parked with their category packs (ADR-058).
    expect(screen.queryByText("/create-change")).not.toBeInTheDocument();
    expect(screen.queryByText("/dev-via-test")).not.toBeInTheDocument();
    expect(screen.queryByText("/implement-spec")).not.toBeInTheDocument();
  });

  it("shows only front-door team slash hints without a selected task", async () => {
    const user = userEvent.setup();
    render(ChatBar);

    await user.type(screen.getByTestId("chat-input"), "/");

    expect(screen.getByText("/research")).toBeInTheDocument();
    expect(screen.getByText("/optimize")).toBeInTheDocument();
    expect(screen.queryByText("/create-change")).not.toBeInTheDocument();
    expect(screen.queryByText("/dev-via-test")).not.toBeInTheDocument();
    expect(screen.queryByText("/approve")).not.toBeInTheDocument();
    expect(screen.queryByText("/implement-spec")).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Message dispatch (agentApi.sendMessage)
// ---------------------------------------------------------------------------

describe("ChatBar — message dispatch", () => {
  it("calls agentApi.sendMessage on submit", async () => {
    const user = userEvent.setup();
    render(ChatBar);

    const input = screen.getByTestId("chat-input");
    await user.type(input, "Add a rule for high temperature");
    await user.click(screen.getByTestId("send-button"));

    expect(agentApi.sendMessage).toHaveBeenCalledWith(
      "Add a rule for high temperature",
    );
  });

  it("clears input after successful send", async () => {
    const user = userEvent.setup();
    render(ChatBar);

    const input = screen.getByTestId("chat-input");
    await user.type(input, "Test message");
    await user.click(screen.getByTestId("send-button"));

    expect(input).toHaveValue("");
  });

  it("submits on Enter key (not Shift+Enter)", async () => {
    const user = userEvent.setup();
    render(ChatBar);

    const input = screen.getByTestId("chat-input");
    await user.type(input, "Test message");
    await user.keyboard("{Enter}");

    expect(agentApi.sendMessage).toHaveBeenCalledWith("Test message");
  });

  it("does not submit on Shift+Enter", async () => {
    const user = userEvent.setup();
    render(ChatBar);

    const input = screen.getByTestId("chat-input");
    await user.type(input, "Test message");
    await user.keyboard("{Shift>}{Enter}{/Shift}");

    expect(agentApi.sendMessage).not.toHaveBeenCalled();
  });

  it("does not submit empty input", async () => {
    const user = userEvent.setup();
    render(ChatBar);

    await user.click(screen.getByTestId("send-button"));

    expect(agentApi.sendMessage).not.toHaveBeenCalled();
  });

  it("does not submit whitespace-only input", async () => {
    const user = userEvent.setup();
    render(ChatBar);

    const input = screen.getByTestId("chat-input");
    await user.type(input, "   ");
    await user.click(screen.getByTestId("send-button"));

    expect(agentApi.sendMessage).not.toHaveBeenCalled();
  });

  it("shows error when sendMessage fails", async () => {
    vi.mocked(agentApi.sendMessage).mockRejectedValueOnce(
      new Error("Network error"),
    );
    const user = userEvent.setup();
    render(ChatBar);

    const input = screen.getByTestId("chat-input");
    await user.type(input, "Test");
    await user.click(screen.getByTestId("send-button"));

    expect(screen.getByTestId("chat-error")).toBeInTheDocument();
    expect(screen.getByText("Network error")).toBeInTheDocument();
  });

  it("dismisses error on button click", async () => {
    vi.mocked(agentApi.sendMessage).mockRejectedValueOnce(new Error("Fail"));
    const user = userEvent.setup();
    render(ChatBar);

    await user.type(screen.getByTestId("chat-input"), "Test");
    await user.click(screen.getByTestId("send-button"));

    expect(screen.getByTestId("chat-error")).toBeInTheDocument();

    await user.click(screen.getByLabelText("Dismiss error"));

    expect(screen.queryByTestId("chat-error")).not.toBeInTheDocument();
  });

  it("sends attached artifact context with the coordinator message", async () => {
    chatHandoff.attachArtifact({
      id: "artifact-1",
      title: "MQTT vs NATS",
      toolName: "emit_research_artifact",
      content: "# MQTT vs NATS\n\nUse MQTT for smaller edge devices.",
    });
    const user = userEvent.setup();
    render(ChatBar);

    expect(screen.getByTestId("artifact-context-chip")).toHaveTextContent(
      "MQTT vs NATS",
    );

    await user.type(
      screen.getByTestId("chat-input"),
      "/research draft requirements",
    );
    await user.click(screen.getByTestId("send-button"));

    expect(agentApi.sendMessage).toHaveBeenCalledWith(
      expect.stringContaining("/research draft requirements"),
    );
    expect(agentApi.sendMessage).toHaveBeenCalledWith(
      expect.stringContaining("Artifact context: MQTT vs NATS"),
    );
    expect(agentApi.sendMessage).toHaveBeenCalledWith(
      expect.stringContaining("Use MQTT for smaller edge devices."),
    );
    expect(
      screen.queryByTestId("artifact-context-chip"),
    ).not.toBeInTheDocument();
  });

  it("clears artifact context without clearing the typed prompt", async () => {
    chatHandoff.attachArtifact({
      id: "artifact-1",
      title: "MQTT vs NATS",
      toolName: "emit_research_artifact",
      content: "# MQTT vs NATS",
    });
    const user = userEvent.setup();
    render(ChatBar);

    await user.type(screen.getByTestId("chat-input"), "/research follow up");
    await user.click(screen.getByLabelText("Remove artifact context"));

    expect(
      screen.queryByTestId("artifact-context-chip"),
    ).not.toBeInTheDocument();
    expect(screen.getByTestId("chat-input")).toHaveValue("/research follow up");
  });
});

// ---------------------------------------------------------------------------
// Slash command routing (agentApi.sendSignal)
// ---------------------------------------------------------------------------

describe("ChatBar — slash commands", () => {
  it("/approve routes to sendSignal on selected task", async () => {
    mockSelectedTask.mockReturnValue(selectedTask({ id: "loop_xyz" }));
    const user = userEvent.setup();
    render(ChatBar);

    const input = screen.getByTestId("chat-input");
    await user.type(input, "/approve");
    await user.click(screen.getByTestId("send-button"));

    expect(agentApi.sendSignal).toHaveBeenCalledWith(
      "loop_xyz",
      "approve",
      undefined,
    );
  });

  it("/reject with reason passes the reason", async () => {
    mockSelectedTask.mockReturnValue(selectedTask({ id: "loop_xyz" }));
    const user = userEvent.setup();
    render(ChatBar);

    const input = screen.getByTestId("chat-input");
    await user.type(input, "/reject Too risky");
    await user.click(screen.getByTestId("send-button"));

    expect(agentApi.sendSignal).toHaveBeenCalledWith(
      "loop_xyz",
      "reject",
      "Too risky",
    );
  });

  it("/research routes through coordinator chat without a selected task", async () => {
    mockSelectedTask.mockReturnValue(undefined);
    const user = userEvent.setup();
    render(ChatBar);

    const input = screen.getByTestId("chat-input");
    await user.type(input, "/research compare MQTT and NATS");
    await user.click(screen.getByTestId("send-button"));

    expect(agentApi.sendMessage).toHaveBeenCalledWith(
      "/research compare MQTT and NATS",
    );
    expect(agentApi.sendSignal).not.toHaveBeenCalled();
  });

  it("slash command without selected task shows error", async () => {
    mockSelectedTask.mockReturnValue(undefined);
    const user = userEvent.setup();
    render(ChatBar);

    const input = screen.getByTestId("chat-input");
    await user.type(input, "/approve");
    await user.click(screen.getByTestId("send-button"));

    expect(agentApi.sendSignal).not.toHaveBeenCalled();
    expect(screen.getByTestId("chat-error")).toBeInTheDocument();
    expect(screen.getByText(/Select a task first/)).toBeInTheDocument();
  });

  it("/implement-spec is parked — it sends as ordinary coordinator chat", async () => {
    // The run-scoped /implement-spec command is parked with the
    // dev-from-task pack (ADR-058). The text still reaches the
    // coordinator as plain content, which answers honestly that the
    // capability is not wired in this deployment.
    mockSelectedTask.mockReturnValue(undefined);
    const user = userEvent.setup();
    render(ChatBar);

    const input = screen.getByTestId("chat-input");
    await user.type(input, "/implement-spec add-health-endpoint");
    await user.click(screen.getByTestId("send-button"));

    expect(agentApi.sendMessage).toHaveBeenCalledWith(
      "/implement-spec add-health-endpoint",
    );
    expect(agentApi.sendSignal).not.toHaveBeenCalled();
    expect(screen.queryByTestId("chat-error")).not.toBeInTheDocument();
  });

  it("slash command error does NOT leave input disabled (regression)", async () => {
    // Regression test for the sending-stuck-on-true blocker found by
    // svelte-reviewer: the early return for "no selected task" must
    // fire BEFORE sending=true, otherwise the input is permanently
    // disabled.
    mockSelectedTask.mockReturnValue(undefined);
    const user = userEvent.setup();
    render(ChatBar);

    const input = screen.getByTestId("chat-input");
    await user.type(input, "/approve");
    await user.click(screen.getByTestId("send-button"));

    // Input should NOT be disabled after the error
    expect(input).not.toBeDisabled();
    expect(screen.getByTestId("send-button")).not.toBeDisabled();
  });

  it("unknown slash-like text dispatches as regular message", async () => {
    const user = userEvent.setup();
    render(ChatBar);

    const input = screen.getByTestId("chat-input");
    await user.type(input, "/unknowncmd something");
    await user.click(screen.getByTestId("send-button"));

    expect(agentApi.sendMessage).toHaveBeenCalledWith("/unknowncmd something");
    expect(agentApi.sendSignal).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// Action chips (persona shortcuts + Approve next)
// ---------------------------------------------------------------------------

describe("ChatBar — action chips", () => {
  it("shows team chips on the empty state", () => {
    render(ChatBar);
    expect(screen.getByTestId("action-chip-research")).toBeInTheDocument();
    expect(screen.getByTestId("action-chip-optimize")).toBeInTheDocument();
    // Spec and Build chips are parked with their category packs (ADR-058).
    expect(screen.queryByTestId("action-chip-spec")).not.toBeInTheDocument();
    expect(screen.queryByTestId("action-chip-build")).not.toBeInTheDocument();
  });

  it("hides action chips when a task is selected", () => {
    mockSelectedTask.mockReturnValue(selectedTask());
    render(ChatBar);
    expect(screen.queryByTestId("action-chips")).not.toBeInTheDocument();
  });

  it("hides action chips when typing a slash command", async () => {
    const user = userEvent.setup();
    render(ChatBar);

    expect(screen.getByTestId("action-chips")).toBeInTheDocument();
    await user.type(screen.getByTestId("chat-input"), "/");
    expect(screen.queryByTestId("action-chips")).not.toBeInTheDocument();
  });

  it("clicking a team chip prefixes the input and focuses it", async () => {
    const user = userEvent.setup();
    render(ChatBar);

    await user.click(screen.getByTestId("action-chip-research"));

    const input = screen.getByTestId("chat-input") as HTMLInputElement;
    expect(input.value).toBe("/research ");
    expect(document.activeElement).toBe(input);
  });

  it("clicking a team chip replaces a legacy @team prefix", async () => {
    const user = userEvent.setup();
    render(ChatBar);

    await user.type(screen.getByTestId("chat-input"), "@research compare mqtt");

    let input = screen.getByTestId("chat-input") as HTMLInputElement;
    expect(input.value).toBe("@research compare mqtt");

    await user.click(screen.getByTestId("action-chip-optimize"));

    // Replaced @research with /optimize, kept the user's typed body.
    input = screen.getByTestId("chat-input") as HTMLInputElement;
    expect(input.value).toBe("/optimize compare mqtt");
  });

  it("does not show 'Approve next' when no tasks need attention", () => {
    mockNeedsAttentionCount.mockReturnValue(0);
    render(ChatBar);
    expect(
      screen.queryByTestId("action-chip-approve-next"),
    ).not.toBeInTheDocument();
  });

  it("shows 'Approve next' with a count when tasks need attention", () => {
    mockNeedsAttentionCount.mockReturnValue(2);
    render(ChatBar);
    const chip = screen.getByTestId("action-chip-approve-next");
    expect(chip).toBeInTheDocument();
    expect(chip).toHaveTextContent("Approve next");
    expect(chip).toHaveTextContent("2");
  });

  it("clicking 'Approve next' selects the first awaiting task", async () => {
    mockNeedsAttentionCount.mockReturnValue(1);
    mockNeedsYou.mockReturnValue([
      selectedTask({ id: "loop_waiting", state: "awaiting_approval" }),
    ]);
    const user = userEvent.setup();
    render(ChatBar);

    await user.click(screen.getByTestId("action-chip-approve-next"));

    expect(mockSelectTask).toHaveBeenCalledWith("loop_waiting");
  });
});
