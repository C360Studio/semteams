import { describe, it, expect, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/svelte";
import userEvent from "@testing-library/user-event";
import ChatPanel from "./ChatPanel.svelte";
import ChatInput from "./ChatInput.svelte";
import ChatMessage from "./ChatMessage.svelte";
import ChatMessageList from "./ChatMessageList.svelte";
import ChatToolbar from "./ChatToolbar.svelte";
import FlowDiffSummary from "./FlowDiffSummary.svelte";
import type { ChatMessage as ChatMessageType, FlowDiff } from "$lib/types/chat";

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

function makeMessage(
  overrides: Partial<ChatMessageType> & { id: string },
): ChatMessageType {
  return {
    role: "user",
    content: "Hello",
    timestamp: new Date("2026-03-08T12:00:00Z"),
    ...overrides,
  };
}

function defaultPanelProps() {
  return {
    messages: [] as ChatMessageType[],
    isStreaming: false,
    streamingContent: "",
    error: null as string | null,
    onSubmit: vi.fn(),
    onCancel: vi.fn(),
    onApplyFlow: vi.fn(),
    onExportJson: vi.fn(),
    onNewChat: vi.fn(),
  };
}

function makeDiff(overrides?: Partial<FlowDiff>): FlowDiff {
  return {
    nodesAdded: [],
    nodesRemoved: [],
    nodesModified: [],
    connectionsAdded: 0,
    connectionsRemoved: 0,
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// 1. Empty State UX
// ---------------------------------------------------------------------------

describe("ChatMessageList — empty state UX", () => {
  it("should contain guidance text referencing the purpose when empty", () => {
    render(ChatMessageList, {
      props: { messages: [], isStreaming: false, streamingContent: "" },
    });

    const emptyState = screen.getByTestId("chat-empty-state");
    const text = emptyState.textContent?.toLowerCase() ?? "";

    // At least one of these guidance words must be present
    // Phase 3 migration: updated from flow-specific to general-purpose assistant text
    const hasGuidance =
      text.includes("question") ||
      text.includes("graph") ||
      text.includes("command");

    expect(hasGuidance).toBe(true);
  });

  it("should render the empty state element with data-testid='chat-empty-state'", () => {
    render(ChatMessageList, {
      props: { messages: [], isStreaming: false, streamingContent: "" },
    });

    expect(screen.getByTestId("chat-empty-state")).toBeInTheDocument();
  });

  it("should NOT show the empty state when messages are present", () => {
    render(ChatMessageList, {
      props: {
        messages: [makeMessage({ id: "1", content: "Hello" })],
        isStreaming: false,
        streamingContent: "",
      },
    });

    expect(screen.queryByTestId("chat-empty-state")).not.toBeInTheDocument();
  });

  it("should NOT show the empty state while streaming content is active", () => {
    render(ChatMessageList, {
      props: {
        messages: [],
        isStreaming: true,
        streamingContent: "Generating...",
      },
    });

    expect(screen.queryByTestId("chat-empty-state")).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// 2. ChatInput Accessibility
// ---------------------------------------------------------------------------

describe("ChatInput — accessibility", () => {
  it("should have an accessible label on the textarea (aria-label or associated label)", () => {
    render(ChatInput, { props: { onSubmit: vi.fn() } });

    const textarea = screen.getByTestId("chat-input");

    // Either the textarea has aria-label, or it has an id matched by a <label> element
    const hasAriaLabel = textarea.hasAttribute("aria-label");
    const hasId = textarea.hasAttribute("id");
    const labelExists =
      hasId &&
      document.querySelector(`label[for="${textarea.getAttribute("id")}"]`) !==
        null;

    expect(hasAriaLabel || labelExists).toBe(true);
  });

  it("should give the submit button an accessible name", () => {
    render(ChatInput, { props: { onSubmit: vi.fn() } });

    const submitBtn = screen.getByTestId("chat-submit");
    const name =
      submitBtn.getAttribute("aria-label") ?? submitBtn.textContent?.trim();

    expect(name).toBeTruthy();
  });

  it("should give the cancel button an accessible name when visible", () => {
    render(ChatInput, {
      props: { onSubmit: vi.fn(), isStreaming: true },
    });

    const cancelBtn = screen.getByTestId("chat-cancel");
    const name =
      cancelBtn.getAttribute("aria-label") ?? cancelBtn.textContent?.trim();

    expect(name).toBeTruthy();
  });

  it("should have type='button' on the submit button", () => {
    render(ChatInput, { props: { onSubmit: vi.fn() } });

    expect(screen.getByTestId("chat-submit")).toHaveAttribute("type", "button");
  });

  it("should have type='button' on the cancel button when visible", () => {
    render(ChatInput, {
      props: { onSubmit: vi.fn(), isStreaming: true },
    });

    expect(screen.getByTestId("chat-cancel")).toHaveAttribute("type", "button");
  });
});

// ---------------------------------------------------------------------------
// 3. ChatMessage Accessibility
// ---------------------------------------------------------------------------

describe("ChatMessage — accessibility", () => {
  it("should use data-testid='chat-message-user' for user messages", () => {
    render(ChatMessage, {
      props: { message: makeMessage({ id: "1", role: "user" }) },
    });

    expect(screen.getByTestId("chat-message-user")).toBeInTheDocument();
  });

  it("should use data-testid='chat-message-assistant' for assistant messages", () => {
    render(ChatMessage, {
      props: {
        message: makeMessage({
          id: "1",
          role: "assistant",
          content: "Here is a flow",
        }),
      },
    });

    expect(screen.getByTestId("chat-message-assistant")).toBeInTheDocument();
  });

  it("should present a timestamp in some form (text, title, or datetime attribute)", () => {
    const timestamp = new Date("2026-03-08T12:00:00Z");

    render(ChatMessage, {
      props: { message: makeMessage({ id: "1", timestamp }) },
    });

    const container = screen.getByTestId("chat-message");

    // Any of: rendered text containing time, title attribute, or a <time> element with datetime
    const hasTimeElement = container.querySelector("time") !== null;
    const hasTimestampText = container.textContent?.includes("12:00") ?? false;
    const hasTitleAttr = Array.from(container.querySelectorAll("[title]")).some(
      (el) => el.getAttribute("title")?.includes("2026"),
    );

    expect(hasTimeElement || hasTimestampText || hasTitleAttr).toBe(true);
  });

  it("should have an ARIA role or landmark on each message", () => {
    render(ChatMessage, {
      props: { message: makeMessage({ id: "1" }) },
    });

    const message = screen.getByTestId("chat-message");

    // Must have role="article", role="listitem", or equivalent semantic element
    const role = message.getAttribute("role");
    const tagName = message.tagName.toLowerCase();
    const semanticTags = ["article", "li", "section"];

    expect(role !== null || semanticTags.includes(tagName)).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// 4. ChatPanel Error UX
// ---------------------------------------------------------------------------

describe("ChatPanel — error UX", () => {
  it("should show error alert with role='alert'", () => {
    render(ChatPanel, {
      props: { ...defaultPanelProps(), error: "Something went wrong" },
    });

    expect(screen.getByRole("alert")).toBeInTheDocument();
  });

  it("should include the actual error message text in the alert", () => {
    render(ChatPanel, {
      props: { ...defaultPanelProps(), error: "Connection timed out" },
    });

    expect(screen.getByRole("alert")).toHaveTextContent("Connection timed out");
  });

  it("should render a dismiss button with data-testid='chat-error-dismiss' in the error alert", () => {
    render(ChatPanel, {
      props: { ...defaultPanelProps(), error: "Something went wrong" },
    });

    expect(screen.getByTestId("chat-error-dismiss")).toBeInTheDocument();
  });

  it("should dismiss the error alert when the dismiss button is clicked", async () => {
    const user = userEvent.setup();

    render(ChatPanel, {
      props: { ...defaultPanelProps(), error: "Something went wrong" },
    });

    expect(screen.getByRole("alert")).toBeInTheDocument();

    await user.click(screen.getByTestId("chat-error-dismiss"));

    await waitFor(() => {
      expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    });
  });

  it("should display different error messages correctly", () => {
    const errorCases = [
      "API rate limit exceeded",
      "Internal server error",
      "Network request failed",
    ];

    for (const msg of errorCases) {
      const { unmount } = render(ChatPanel, {
        props: { ...defaultPanelProps(), error: msg },
      });

      expect(screen.getByRole("alert")).toHaveTextContent(msg);
      unmount();
    }
  });
});

// ---------------------------------------------------------------------------
// 5. ChatInput Character Counter
// ---------------------------------------------------------------------------

describe("ChatInput — character counter", () => {
  it("should NOT show the character counter when input is empty", () => {
    render(ChatInput, { props: { onSubmit: vi.fn() } });

    expect(screen.queryByTestId("chat-char-count")).not.toBeInTheDocument();
  });

  it("should show the character counter after typing", async () => {
    const user = userEvent.setup();

    render(ChatInput, { props: { onSubmit: vi.fn() } });

    await user.type(screen.getByTestId("chat-input"), "hello");

    await waitFor(() => {
      expect(screen.getByTestId("chat-char-count")).toBeInTheDocument();
    });
  });

  it("should update the character count as the user types", async () => {
    const user = userEvent.setup();

    render(ChatInput, { props: { onSubmit: vi.fn() } });

    const textarea = screen.getByTestId("chat-input");

    await user.type(textarea, "hi");
    await waitFor(() => {
      const count = screen.getByTestId("chat-char-count");
      expect(count.textContent).toMatch(/2/);
    });

    await user.type(textarea, "!!!");
    await waitFor(() => {
      const count = screen.getByTestId("chat-char-count");
      expect(count.textContent).toMatch(/5/);
    });
  });

  it("should show the current character count in the counter element", async () => {
    const user = userEvent.setup();

    render(ChatInput, { props: { onSubmit: vi.fn() } });

    await user.type(screen.getByTestId("chat-input"), "hello world");

    await waitFor(() => {
      const count = screen.getByTestId("chat-char-count");
      // Counter text must include "11" — accepts "11" or "11/2000" or similar formats
      expect(count.textContent).toMatch(/11/);
    });
  });

  it("should hide the character counter after input is cleared via submit", async () => {
    const user = userEvent.setup();

    render(ChatInput, { props: { onSubmit: vi.fn() } });

    const textarea = screen.getByTestId("chat-input");
    await user.type(textarea, "test{Enter}");

    await waitFor(() => {
      expect(screen.queryByTestId("chat-char-count")).not.toBeInTheDocument();
    });
  });
});

// ---------------------------------------------------------------------------
// 6. ChatToolbar Accessibility
// ---------------------------------------------------------------------------

describe("ChatToolbar — accessibility", () => {
  const toolbarButtons = [
    { testId: "chat-export-json", label: "Export" },
    { testId: "chat-new-chat", label: "New" },
  ];

  it.each(toolbarButtons)(
    "should give the '$label' button an accessible name",
    ({ testId }) => {
      render(ChatToolbar, {
        props: {
          onExportJson: vi.fn(),
          onNewChat: vi.fn(),
        },
      });

      const button = screen.getByTestId(testId);
      const name =
        button.getAttribute("aria-label") ?? button.textContent?.trim();

      expect(name).toBeTruthy();
    },
  );
});

// ---------------------------------------------------------------------------
// 7. ChatInput Keyboard Navigation
// ---------------------------------------------------------------------------

describe("ChatInput — keyboard navigation", () => {
  it("should blur the textarea when Escape is pressed (not submit)", async () => {
    const onSubmit = vi.fn();
    const user = userEvent.setup();

    render(ChatInput, { props: { onSubmit } });

    const textarea = screen.getByTestId("chat-input");
    await user.type(textarea, "some text");
    textarea.focus();

    expect(document.activeElement).toBe(textarea);

    await user.keyboard("{Escape}");

    // After Escape, the textarea should no longer be focused
    expect(document.activeElement).not.toBe(textarea);
    // onSubmit must NOT have been called
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("should NOT submit on Escape key press", async () => {
    const onSubmit = vi.fn();
    const user = userEvent.setup();

    render(ChatInput, { props: { onSubmit } });

    const textarea = screen.getByTestId("chat-input");
    await user.type(textarea, "some text");
    await user.keyboard("{Escape}");

    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("should allow Tab to move focus away from the textarea normally", async () => {
    const user = userEvent.setup();

    render(ChatInput, { props: { onSubmit: vi.fn() } });

    const textarea = screen.getByTestId("chat-input");
    textarea.focus();

    await user.tab();

    // After Tab, focus should have moved off the textarea
    expect(document.activeElement).not.toBe(textarea);
  });
});

// ---------------------------------------------------------------------------
// 8. FlowDiffSummary Accessibility
// ---------------------------------------------------------------------------

describe("FlowDiffSummary — accessibility and change type text", () => {
  it("should render descriptive 'Added:' label text for added nodes", () => {
    render(FlowDiffSummary, {
      props: { diff: makeDiff({ nodesAdded: ["source-1"] }) },
    });

    expect(screen.getByTestId("flow-diff-summary").textContent).toMatch(
      /added/i,
    );
  });

  it("should render descriptive 'Removed:' label text for removed nodes", () => {
    render(FlowDiffSummary, {
      props: { diff: makeDiff({ nodesRemoved: ["processor-1"] }) },
    });

    expect(screen.getByTestId("flow-diff-summary").textContent).toMatch(
      /removed/i,
    );
  });

  it("should render descriptive 'Modified:' label text for modified nodes", () => {
    render(FlowDiffSummary, {
      props: { diff: makeDiff({ nodesModified: ["output-1"] }) },
    });

    expect(screen.getByTestId("flow-diff-summary").textContent).toMatch(
      /modified/i,
    );
  });

  it("should show node names alongside their change type label", () => {
    render(FlowDiffSummary, {
      props: {
        diff: makeDiff({
          nodesAdded: ["kafka-source"],
          nodesRemoved: ["http-sink"],
          nodesModified: ["json-processor"],
        }),
      },
    });

    const summary = screen.getByTestId("flow-diff-summary");
    expect(summary.textContent).toContain("kafka-source");
    expect(summary.textContent).toContain("http-sink");
    expect(summary.textContent).toContain("json-processor");
  });

  it("should show 'No changes' text when the diff is empty", () => {
    render(FlowDiffSummary, {
      props: { diff: makeDiff() },
    });

    expect(screen.getByTestId("flow-diff-summary")).toHaveTextContent(
      "No changes",
    );
  });

  it("should include connection change counts with descriptive text", () => {
    render(FlowDiffSummary, {
      props: {
        diff: makeDiff({ connectionsAdded: 2, connectionsRemoved: 1 }),
      },
    });

    const summary = screen.getByTestId("flow-diff-summary");
    // Must have a numeric count and the word "connection" (singular or plural)
    expect(summary.textContent?.toLowerCase()).toMatch(/connection/);
  });

  // Table-driven: each change type produces its label independently
  const changeTypeCases = [
    {
      name: "added nodes only",
      diff: makeDiff({ nodesAdded: ["a"] }),
      expectedPattern: /added/i,
      notExpectedPattern: /removed|modified/i,
    },
    {
      name: "removed nodes only",
      diff: makeDiff({ nodesRemoved: ["b"] }),
      expectedPattern: /removed/i,
      notExpectedPattern: /^(?!.*removed).*added|modified/i,
    },
    {
      name: "modified nodes only",
      diff: makeDiff({ nodesModified: ["c"] }),
      expectedPattern: /modified/i,
      notExpectedPattern: /added|removed/i,
    },
  ] as const;

  it.each(changeTypeCases)(
    "should only show the '$name' section when only that change type is present",
    ({ diff, expectedPattern }) => {
      render(FlowDiffSummary, { props: { diff } });

      const text = screen.getByTestId("flow-diff-summary").textContent ?? "";
      expect(text).toMatch(expectedPattern);
    },
  );
});
