import { describe, it, expect, vi } from "vitest";
import { render, screen, within } from "@testing-library/svelte";
import userEvent from "@testing-library/user-event";
import ChatMessageList from "./ChatMessageList.svelte";
import type { ChatMessage, FlowAttachment } from "$lib/types/chat";

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

function makeMessage(
  overrides: Partial<ChatMessage> & { id: string },
): ChatMessage {
  return {
    role: "user",
    content: "Hello",
    timestamp: new Date("2026-03-08T12:00:00Z"),
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// Presence and data-testid
// ---------------------------------------------------------------------------

describe("ChatMessageList — presence", () => {
  it("should have data-testid='chat-message-list'", () => {
    render(ChatMessageList, { props: { messages: [] } });

    expect(screen.getByTestId("chat-message-list")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Empty state
// ---------------------------------------------------------------------------

describe("ChatMessageList — empty state", () => {
  it("should show empty state when messages array is empty", () => {
    render(ChatMessageList, { props: { messages: [] } });

    expect(screen.getByTestId("chat-empty-state")).toBeInTheDocument();
  });

  it("should NOT show empty state when messages are present", () => {
    render(ChatMessageList, {
      props: {
        messages: [makeMessage({ id: "1", content: "First" })],
      },
    });

    expect(screen.queryByTestId("chat-empty-state")).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Rendering messages
// ---------------------------------------------------------------------------

describe("ChatMessageList — renders messages", () => {
  it("should render all messages in order", () => {
    render(ChatMessageList, {
      props: {
        messages: [
          makeMessage({ id: "1", content: "First message" }),
          makeMessage({ id: "2", content: "Second message" }),
          makeMessage({ id: "3", content: "Third message" }),
        ],
      },
    });

    const list = screen.getByTestId("chat-message-list");
    const messages = within(list).getAllByTestId("chat-message");

    expect(messages).toHaveLength(3);
    expect(messages[0]).toHaveTextContent("First message");
    expect(messages[1]).toHaveTextContent("Second message");
    expect(messages[2]).toHaveTextContent("Third message");
  });

  it("should render a single message correctly", () => {
    render(ChatMessageList, {
      props: {
        messages: [makeMessage({ id: "1", content: "Only message" })],
      },
    });

    expect(screen.getAllByTestId("chat-message")).toHaveLength(1);
    expect(screen.getByText("Only message")).toBeInTheDocument();
  });

  it("should render messages with different roles", () => {
    render(ChatMessageList, {
      props: {
        messages: [
          makeMessage({ id: "1", role: "user", content: "User says" }),
          makeMessage({
            id: "2",
            role: "assistant",
            content: "Assistant says",
          }),
          makeMessage({ id: "3", role: "system", content: "System says" }),
        ],
      },
    });

    expect(screen.getByTestId("chat-message-user")).toBeInTheDocument();
    expect(screen.getByTestId("chat-message-assistant")).toBeInTheDocument();
    expect(screen.getByTestId("chat-message-system")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Streaming message
// ---------------------------------------------------------------------------

describe("ChatMessageList — streaming message", () => {
  it("should show streaming message when isStreaming=true and streamingContent is non-empty", () => {
    render(ChatMessageList, {
      props: {
        messages: [],
        isStreaming: true,
        streamingContent: "Generating response...",
      },
    });

    expect(screen.getByTestId("chat-message-streaming")).toBeInTheDocument();
    expect(screen.getByTestId("chat-message-streaming")).toHaveTextContent(
      "Generating response...",
    );
  });

  it("should NOT show streaming message when isStreaming=false", () => {
    render(ChatMessageList, {
      props: {
        messages: [],
        isStreaming: false,
        streamingContent: "Some content",
      },
    });

    expect(
      screen.queryByTestId("chat-message-streaming"),
    ).not.toBeInTheDocument();
  });

  it("should NOT show streaming message when streamingContent is empty string", () => {
    render(ChatMessageList, {
      props: {
        messages: [],
        isStreaming: true,
        streamingContent: "",
      },
    });

    expect(
      screen.queryByTestId("chat-message-streaming"),
    ).not.toBeInTheDocument();
  });

  it("should show streaming message at the bottom after existing messages", () => {
    render(ChatMessageList, {
      props: {
        messages: [
          makeMessage({ id: "1", content: "First" }),
          makeMessage({ id: "2", content: "Second" }),
        ],
        isStreaming: true,
        streamingContent: "Streaming...",
      },
    });

    const list = screen.getByTestId("chat-message-list");
    const allMessages = within(list).getAllByTestId(/^chat-message/);

    // Streaming message should be last
    const lastMessage = allMessages[allMessages.length - 1];
    expect(lastMessage).toHaveAttribute(
      "data-testid",
      "chat-message-streaming",
    );
  });
});

// ---------------------------------------------------------------------------
// onApplyFlow passthrough
// ---------------------------------------------------------------------------

describe("ChatMessageList — onApplyFlow passthrough", () => {
  it("should pass onApplyFlow through to ChatMessage components", async () => {
    const onApplyFlow = vi.fn();
    const user = userEvent.setup();
    // Migration: flow is now in attachments[] as a FlowAttachment
    const flowAttachment: FlowAttachment = {
      kind: "flow",
      flow: { nodes: [], connections: [] },
    };

    render(ChatMessageList, {
      props: {
        messages: [
          makeMessage({
            id: "msg-apply",
            role: "assistant",
            content: "Here is your flow",
            attachments: [flowAttachment],
          }),
        ],
        onApplyFlow,
      },
    });

    await user.click(screen.getByTestId("apply-flow-button"));

    expect(onApplyFlow).toHaveBeenCalledOnce();
    expect(onApplyFlow).toHaveBeenCalledWith("msg-apply");
  });
});
