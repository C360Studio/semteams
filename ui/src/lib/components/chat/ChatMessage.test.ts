import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/svelte";
import userEvent from "@testing-library/user-event";
import ChatMessage from "./ChatMessage.svelte";
import type {
  ChatMessage as ChatMessageType,
  FlowAttachment,
  FlowDiff,
} from "$lib/types/chat";

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

function makeMessage(
  overrides: Partial<ChatMessageType> = {},
): ChatMessageType {
  return {
    id: "msg-1",
    role: "user",
    content: "Hello there",
    timestamp: new Date("2026-03-08T12:00:00Z"),
    ...overrides,
  };
}

const sampleDiff: FlowDiff = {
  nodesAdded: ["WebhookInput", "JsonProcessor"],
  nodesRemoved: ["OldOutput"],
  nodesModified: ["KafkaOutput"],
  connectionsAdded: 2,
  connectionsRemoved: 1,
};

// ---------------------------------------------------------------------------
// Rendering — content and data-testid
// ---------------------------------------------------------------------------

describe("ChatMessage — rendering", () => {
  it("should render message content", () => {
    render(ChatMessage, {
      props: { message: makeMessage({ content: "Build me a pipeline" }) },
    });

    expect(screen.getByText("Build me a pipeline")).toBeInTheDocument();
  });

  it("should have data-testid='chat-message'", () => {
    render(ChatMessage, { props: { message: makeMessage() } });

    expect(screen.getByTestId("chat-message")).toBeInTheDocument();
  });

  it("should have data-testid='chat-message-user' for user messages", () => {
    render(ChatMessage, {
      props: { message: makeMessage({ role: "user" }) },
    });

    expect(screen.getByTestId("chat-message-user")).toBeInTheDocument();
  });

  it("should have data-testid='chat-message-assistant' for assistant messages", () => {
    render(ChatMessage, {
      props: {
        message: makeMessage({ role: "assistant", content: "Here you go" }),
      },
    });

    expect(screen.getByTestId("chat-message-assistant")).toBeInTheDocument();
  });

  it("should have data-testid='chat-message-system' for system messages", () => {
    render(ChatMessage, {
      props: {
        message: makeMessage({ role: "system", content: "Session started" }),
      },
    });

    expect(screen.getByTestId("chat-message-system")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Apply to Canvas button
// ---------------------------------------------------------------------------

describe("ChatMessage — Apply to Canvas button", () => {
  // Migration: flow is now in message.attachments as a FlowAttachment.
  // applied is a field on FlowAttachment, not on ChatMessage.

  it("should show 'Apply to Canvas' button when assistant message has a flow", () => {
    const flowAttachment: FlowAttachment = {
      kind: "flow",
      flow: { nodes: [], connections: [] },
    };
    render(ChatMessage, {
      props: {
        message: makeMessage({
          role: "assistant",
          attachments: [flowAttachment],
        }),
        onApplyFlow: vi.fn(),
      },
    });

    expect(screen.getByTestId("apply-flow-button")).toBeInTheDocument();
    expect(screen.getByTestId("apply-flow-button")).toHaveTextContent(
      /apply to canvas/i,
    );
  });

  it("should NOT show apply button for user messages even when flow is present", () => {
    const flowAttachment: FlowAttachment = {
      kind: "flow",
      flow: { nodes: [], connections: [] },
    };
    render(ChatMessage, {
      props: {
        message: makeMessage({
          role: "user",
          attachments: [flowAttachment],
        }),
      },
    });

    expect(screen.queryByTestId("apply-flow-button")).not.toBeInTheDocument();
  });

  it("should NOT show apply button for system messages even when flow is present", () => {
    const flowAttachment: FlowAttachment = {
      kind: "flow",
      flow: { nodes: [], connections: [] },
    };
    render(ChatMessage, {
      props: {
        message: makeMessage({
          role: "system",
          attachments: [flowAttachment],
        }),
      },
    });

    expect(screen.queryByTestId("apply-flow-button")).not.toBeInTheDocument();
  });

  it("should NOT show apply button when assistant message has no flow", () => {
    render(ChatMessage, {
      props: {
        message: makeMessage({ role: "assistant" }),
      },
    });

    expect(screen.queryByTestId("apply-flow-button")).not.toBeInTheDocument();
  });

  it("should call onApplyFlow with message id when 'Apply to Canvas' is clicked", async () => {
    const onApplyFlow = vi.fn();
    const user = userEvent.setup();
    const flowAttachment: FlowAttachment = {
      kind: "flow",
      flow: { nodes: [], connections: [] },
    };

    render(ChatMessage, {
      props: {
        message: makeMessage({
          id: "msg-42",
          role: "assistant",
          attachments: [flowAttachment],
        }),
        onApplyFlow,
      },
    });

    await user.click(screen.getByTestId("apply-flow-button"));

    expect(onApplyFlow).toHaveBeenCalledOnce();
    expect(onApplyFlow).toHaveBeenCalledWith("msg-42");
  });

  it("should show 'Applied' and be disabled when message.applied is true", () => {
    // Migration: applied is now on FlowAttachment, not top-level ChatMessage.
    const flowAttachment: FlowAttachment = {
      kind: "flow",
      flow: { nodes: [], connections: [] },
      applied: true,
    };
    render(ChatMessage, {
      props: {
        message: makeMessage({
          role: "assistant",
          attachments: [flowAttachment],
        }),
        onApplyFlow: vi.fn(),
      },
    });

    const button = screen.getByTestId("apply-flow-button");
    expect(button).toHaveTextContent(/applied/i);
    expect(button).toBeDisabled();
  });

  it("should NOT be disabled when message.applied is false", () => {
    // Migration: applied is now on FlowAttachment, not top-level ChatMessage.
    const flowAttachment: FlowAttachment = {
      kind: "flow",
      flow: { nodes: [], connections: [] },
      applied: false,
    };
    render(ChatMessage, {
      props: {
        message: makeMessage({
          role: "assistant",
          attachments: [flowAttachment],
        }),
        onApplyFlow: vi.fn(),
      },
    });

    const button = screen.getByTestId("apply-flow-button");
    expect(button).not.toBeDisabled();
  });
});

// ---------------------------------------------------------------------------
// FlowDiffSummary integration
// ---------------------------------------------------------------------------

describe("ChatMessage — FlowDiffSummary", () => {
  // Migration: flowDiff is now a field on FlowAttachment, not top-level ChatMessage.
  // FlowDiffSummary is rendered when a FlowAttachment in attachments[] has a diff field.

  it("should render FlowDiffSummary when message has flowDiff", () => {
    const flowAttachment: FlowAttachment = {
      kind: "flow",
      flow: { nodes: [], connections: [] },
      diff: sampleDiff,
    };
    render(ChatMessage, {
      props: {
        message: makeMessage({
          role: "assistant",
          attachments: [flowAttachment],
        }),
      },
    });

    expect(screen.getByTestId("flow-diff-summary")).toBeInTheDocument();
  });

  it("should NOT render FlowDiffSummary when message has no flowDiff", () => {
    render(ChatMessage, {
      props: {
        message: makeMessage({ role: "assistant" }),
      },
    });

    expect(screen.queryByTestId("flow-diff-summary")).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Table-driven: role × data-testid mapping
// ---------------------------------------------------------------------------

describe("ChatMessage — role-based data-testid", () => {
  const roleCases = [
    {
      role: "user" as const,
      expectedTestId: "chat-message-user",
      absentTestId: "chat-message-assistant",
    },
    {
      role: "assistant" as const,
      expectedTestId: "chat-message-assistant",
      absentTestId: "chat-message-user",
    },
    {
      role: "system" as const,
      expectedTestId: "chat-message-system",
      absentTestId: "chat-message-user",
    },
  ];

  it.each(roleCases)(
    "role '$role' renders with correct data-testid",
    ({ role, expectedTestId, absentTestId }) => {
      render(ChatMessage, {
        props: { message: makeMessage({ role, content: `${role} message` }) },
      });

      expect(screen.getByTestId(expectedTestId)).toBeInTheDocument();
      expect(screen.queryByTestId(absentTestId)).not.toBeInTheDocument();
    },
  );
});
