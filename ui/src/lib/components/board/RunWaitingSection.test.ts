import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/svelte";
import userEvent from "@testing-library/user-event";
import RunWaitingSection from "./RunWaitingSection.svelte";
import type { RunPause } from "$lib/types/task";

// ---------------------------------------------------------------------------
// Mock agentApi — we don't want real network calls in unit tests
// ---------------------------------------------------------------------------

const mockSendMessage = vi.fn();
const mockSubmitApproval = vi.fn();

vi.mock("$lib/services/agentApi", () => ({
  agentApi: {
    sendMessage: (...args: unknown[]) => mockSendMessage(...args),
    submitApproval: (...args: unknown[]) => mockSubmitApproval(...args),
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

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const CLARIFICATION_PAUSE: RunPause = {
  cause: "clarification",
  askingLoopId: "loop-ask-42",
  question: "What timeout should I use?",
};

const CLARIFICATION_NO_QUESTION: RunPause = {
  cause: "clarification",
  askingLoopId: "loop-ask-99",
  question: "",
};

const TOOL_GATE_PAUSE: RunPause = {
  cause: "tool_gate",
  gatedLoopId: "loop-gated-7",
};

beforeEach(() => {
  mockSendMessage.mockReset();
  mockSubmitApproval.mockReset();
});

// ---------------------------------------------------------------------------
// Clarification path
// ---------------------------------------------------------------------------

describe("RunWaitingSection — clarification", () => {
  it("renders the question prose", () => {
    render(RunWaitingSection, {
      props: { runId: "run-abc", pause: CLARIFICATION_PAUSE },
    });

    expect(screen.getByTestId("run-question")).toHaveTextContent(
      "What timeout should I use?",
    );
  });

  it("falls back to default copy when question is empty", () => {
    render(RunWaitingSection, {
      props: { runId: "run-abc", pause: CLARIFICATION_NO_QUESTION },
    });

    expect(screen.getByTestId("run-question")).toHaveTextContent(
      "The agent needs your input to continue.",
    );
  });

  it("renders the textarea and Send button", () => {
    render(RunWaitingSection, {
      props: { runId: "run-abc", pause: CLARIFICATION_PAUSE },
    });

    expect(screen.getByTestId("run-reply-input")).toBeInTheDocument();
    expect(screen.getByTestId("run-reply-send")).toBeInTheDocument();
  });

  it("calls sendMessage with runId + inReplyTo on Send", async () => {
    mockSendMessage.mockResolvedValueOnce({ content: "ok" });
    const user = userEvent.setup();

    render(RunWaitingSection, {
      props: { runId: "run-abc", pause: CLARIFICATION_PAUSE },
    });

    await user.type(screen.getByTestId("run-reply-input"), "Use 30 seconds");
    await user.click(screen.getByTestId("run-reply-send"));

    expect(mockSendMessage).toHaveBeenCalledWith("Use 30 seconds", {
      runId: "run-abc",
      inReplyTo: "loop-ask-42",
    });
  });

  it("clears the textarea on success", async () => {
    mockSendMessage.mockResolvedValueOnce({ content: "ok" });
    const user = userEvent.setup();

    render(RunWaitingSection, {
      props: { runId: "run-abc", pause: CLARIFICATION_PAUSE },
    });

    const input = screen.getByTestId("run-reply-input");
    await user.type(input, "30 seconds");
    await user.click(screen.getByTestId("run-reply-send"));

    await waitFor(() => {
      expect((input as HTMLTextAreaElement).value).toBe("");
    });
  });

  it("does not call sendMessage when reply is whitespace-only", async () => {
    const user = userEvent.setup();

    render(RunWaitingSection, {
      props: { runId: "run-abc", pause: CLARIFICATION_PAUSE },
    });

    await user.type(screen.getByTestId("run-reply-input"), "   ");
    // Send button should be disabled (trim-empty guard)
    expect(screen.getByTestId("run-reply-send")).toBeDisabled();
  });

  it("shows inline error when sendMessage throws", async () => {
    mockSendMessage.mockRejectedValueOnce(new Error("Network failure"));
    const user = userEvent.setup();

    render(RunWaitingSection, {
      props: { runId: "run-abc", pause: CLARIFICATION_PAUSE },
    });

    await user.type(screen.getByTestId("run-reply-input"), "Answer");
    await user.click(screen.getByTestId("run-reply-send"));

    await waitFor(() => {
      expect(screen.getByTestId("run-reply-error")).toBeInTheDocument();
      expect(screen.getByTestId("run-reply-error")).toHaveTextContent("Network failure");
    });
  });

  it("disables Send while in-flight", async () => {
    let resolveMsg!: (v: { content: string }) => void;
    mockSendMessage.mockReturnValueOnce(
      new Promise<{ content: string }>((r) => { resolveMsg = r; }),
    );
    const user = userEvent.setup();

    render(RunWaitingSection, {
      props: { runId: "run-abc", pause: CLARIFICATION_PAUSE },
    });

    await user.type(screen.getByTestId("run-reply-input"), "Answer");
    await user.click(screen.getByTestId("run-reply-send"));

    await waitFor(() => {
      expect(screen.getByTestId("run-reply-send")).toBeDisabled();
    });

    resolveMsg({ content: "ok" });
  });

  it("sends on Enter (no Shift) — keyboard submit path", async () => {
    mockSendMessage.mockResolvedValueOnce({ content: "ok" });
    const user = userEvent.setup();

    render(RunWaitingSection, {
      props: { runId: "run-abc", pause: CLARIFICATION_PAUSE },
    });

    await user.type(screen.getByTestId("run-reply-input"), "30 seconds");
    await user.keyboard("{Enter}");

    await waitFor(() => {
      expect(mockSendMessage).toHaveBeenCalledWith("30 seconds", {
        runId: "run-abc",
        inReplyTo: "loop-ask-42",
      });
    });
  });

  it("does NOT send on Shift+Enter (newline, not submit)", async () => {
    const user = userEvent.setup();

    render(RunWaitingSection, {
      props: { runId: "run-abc", pause: CLARIFICATION_PAUSE },
    });

    await user.type(screen.getByTestId("run-reply-input"), "line one");
    await user.keyboard("{Shift>}{Enter}{/Shift}");

    expect(mockSendMessage).not.toHaveBeenCalled();
  });

  it("shows a confirmation after a successful reply (poll-lag gap)", async () => {
    mockSendMessage.mockResolvedValueOnce({ content: "ok" });
    const user = userEvent.setup();

    render(RunWaitingSection, {
      props: { runId: "run-abc", pause: CLARIFICATION_PAUSE },
    });

    await user.type(screen.getByTestId("run-reply-input"), "Answer");
    await user.click(screen.getByTestId("run-reply-send"));

    await waitFor(() => {
      expect(screen.getByTestId("run-reply-confirm")).toBeInTheDocument();
    });

    // Typing again dismisses the confirmation (fresh draft).
    await user.type(screen.getByTestId("run-reply-input"), "more");
    expect(screen.queryByTestId("run-reply-confirm")).not.toBeInTheDocument();
  });

  it("does not render approval section for clarification pause", () => {
    render(RunWaitingSection, {
      props: { runId: "run-abc", pause: CLARIFICATION_PAUSE },
    });

    expect(screen.queryByTestId("run-approval-section")).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Tool-gate path
// ---------------------------------------------------------------------------

describe("RunWaitingSection — tool_gate", () => {
  it("renders the approval section with Approve and Reject buttons", () => {
    render(RunWaitingSection, {
      props: { runId: "run-xyz", pause: TOOL_GATE_PAUSE },
    });

    expect(screen.getByTestId("run-approval-section")).toBeInTheDocument();
    expect(screen.getByTestId("run-approve")).toBeInTheDocument();
    expect(screen.getByTestId("run-reject")).toBeInTheDocument();
  });

  it("does not render the reply textarea for tool_gate pause", () => {
    render(RunWaitingSection, {
      props: { runId: "run-xyz", pause: TOOL_GATE_PAUSE },
    });

    expect(screen.queryByTestId("run-reply-input")).not.toBeInTheDocument();
  });

  it("calls submitApproval with approve + gatedLoopId on Approve", async () => {
    mockSubmitApproval.mockResolvedValueOnce({ accepted: true });
    const user = userEvent.setup();

    render(RunWaitingSection, {
      props: { runId: "run-xyz", pause: TOOL_GATE_PAUSE },
    });

    await user.click(screen.getByTestId("run-approve"));

    expect(mockSubmitApproval).toHaveBeenCalledWith("loop-gated-7", {
      decision: "approve",
    });
  });

  it("calls submitApproval with reject + gatedLoopId on Reject", async () => {
    mockSubmitApproval.mockResolvedValueOnce({ accepted: true });
    const user = userEvent.setup();

    render(RunWaitingSection, {
      props: { runId: "run-xyz", pause: TOOL_GATE_PAUSE },
    });

    await user.click(screen.getByTestId("run-reject"));

    expect(mockSubmitApproval).toHaveBeenCalledWith("loop-gated-7", {
      decision: "reject",
    });
  });

  it("includes reason in submitApproval body when reason is entered", async () => {
    mockSubmitApproval.mockResolvedValueOnce({ accepted: true });
    const user = userEvent.setup();

    render(RunWaitingSection, {
      props: { runId: "run-xyz", pause: TOOL_GATE_PAUSE },
    });

    await user.type(screen.getByRole("textbox", { name: /reason/i }), "Too broad");
    await user.click(screen.getByTestId("run-approve"));

    expect(mockSubmitApproval).toHaveBeenCalledWith("loop-gated-7", {
      decision: "approve",
      reason: "Too broad",
    });
  });

  it("disables buttons while in-flight", async () => {
    let resolveApproval!: (v: { accepted: boolean }) => void;
    mockSubmitApproval.mockReturnValueOnce(
      new Promise<{ accepted: boolean }>((r) => { resolveApproval = r; }),
    );
    const user = userEvent.setup();

    render(RunWaitingSection, {
      props: { runId: "run-xyz", pause: TOOL_GATE_PAUSE },
    });

    await user.click(screen.getByTestId("run-approve"));

    await waitFor(() => {
      expect(screen.getByTestId("run-approve")).toBeDisabled();
      expect(screen.getByTestId("run-reject")).toBeDisabled();
    });

    resolveApproval({ accepted: true });
  });

  it("shows inline error when submitApproval throws", async () => {
    mockSubmitApproval.mockRejectedValueOnce(new Error("Backend error"));
    const user = userEvent.setup();

    render(RunWaitingSection, {
      props: { runId: "run-xyz", pause: TOOL_GATE_PAUSE },
    });

    await user.click(screen.getByTestId("run-approve"));

    await waitFor(() => {
      expect(screen.getByTestId("run-approval-error")).toBeInTheDocument();
      expect(screen.getByTestId("run-approval-error")).toHaveTextContent("Backend error");
    });
  });

  it("shows a confirmation after a successful decision", async () => {
    mockSubmitApproval.mockResolvedValueOnce({ accepted: true });
    const user = userEvent.setup();

    render(RunWaitingSection, {
      props: { runId: "run-xyz", pause: TOOL_GATE_PAUSE },
    });

    await user.click(screen.getByTestId("run-approve"));

    await waitFor(() => {
      const confirm = screen.getByTestId("run-approval-confirm");
      expect(confirm).toBeInTheDocument();
      expect(confirm).toHaveTextContent("Approval submitted");
    });
  });

  it("shows 409 message when submitApproval throws AgentApiError with status 409", async () => {
    const { AgentApiError } = await import("$lib/services/agentApi");
    mockSubmitApproval.mockRejectedValueOnce(
      new AgentApiError("conflict", 409),
    );
    const user = userEvent.setup();

    render(RunWaitingSection, {
      props: { runId: "run-xyz", pause: TOOL_GATE_PAUSE },
    });

    await user.click(screen.getByTestId("run-reject"));

    await waitFor(() => {
      const err = screen.getByTestId("run-approval-error");
      expect(err).toBeInTheDocument();
      expect(err).toHaveTextContent("no longer pending");
    });
  });
});
