import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/svelte";
import userEvent from "@testing-library/user-event";
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

// Mock taskStore with controllable selectedTask
const mockSelectedTask = vi.fn().mockReturnValue(undefined);
vi.mock("$lib/stores/taskStore.svelte", () => ({
  taskStore: {
    get selectedTask() {
      return mockSelectedTask();
    },
    deselectTask: vi.fn(),
  },
}));

import { agentApi } from "$lib/services/agentApi";
import { taskStore } from "$lib/stores/taskStore.svelte";

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
  mockSelectedTask.mockReturnValue(undefined);
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

  it("shows 'What should I work on?' placeholder when no task selected", () => {
    render(ChatBar);

    expect(screen.getByTestId("chat-input")).toHaveAttribute(
      "placeholder",
      "What should I work on?",
    );
  });

  it("shows task context when a task is selected", () => {
    mockSelectedTask.mockReturnValue(selectedTask());
    render(ChatBar);

    expect(screen.getByTestId("chat-context")).toBeInTheDocument();
    expect(screen.getByText("Test Task")).toBeInTheDocument();
  });

  it("shows slash hint chips for selected task", () => {
    mockSelectedTask.mockReturnValue(selectedTask());
    render(ChatBar);

    expect(screen.getByText("/approve")).toBeInTheDocument();
    expect(screen.getByText("/reject")).toBeInTheDocument();
  });

  it("shows 'Type a task' hint when no task selected", () => {
    render(ChatBar);

    expect(screen.getByText("Type a task to get started")).toBeInTheDocument();
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
    vi.mocked(agentApi.sendMessage).mockRejectedValueOnce(
      new Error("Fail"),
    );
    const user = userEvent.setup();
    render(ChatBar);

    await user.type(screen.getByTestId("chat-input"), "Test");
    await user.click(screen.getByTestId("send-button"));

    expect(screen.getByTestId("chat-error")).toBeInTheDocument();

    await user.click(screen.getByLabelText("Dismiss error"));

    expect(screen.queryByTestId("chat-error")).not.toBeInTheDocument();
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

    expect(agentApi.sendMessage).toHaveBeenCalledWith(
      "/unknowncmd something",
    );
    expect(agentApi.sendSignal).not.toHaveBeenCalled();
  });
});
