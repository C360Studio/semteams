import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/svelte";
import userEvent from "@testing-library/user-event";
import ChatToolbar from "./ChatToolbar.svelte";

// ---------------------------------------------------------------------------
// Presence and data-testid
// ---------------------------------------------------------------------------

describe("ChatToolbar — presence", () => {
  it("should have data-testid='chat-toolbar'", () => {
    render(ChatToolbar, {
      props: {
        onExportJson: vi.fn(),
        onNewChat: vi.fn(),
      },
    });

    expect(screen.getByTestId("chat-toolbar")).toBeInTheDocument();
  });

  it("should render Export button when onExportJson provided", () => {
    render(ChatToolbar, {
      props: {
        onExportJson: vi.fn(),
        onNewChat: vi.fn(),
      },
    });

    expect(screen.getByTestId("chat-export-json")).toBeInTheDocument();
  });

  it("should NOT render Export button when onExportJson not provided", () => {
    render(ChatToolbar, {
      props: {
        onNewChat: vi.fn(),
      },
    });

    expect(screen.queryByTestId("chat-export-json")).not.toBeInTheDocument();
  });

  it("should render New Chat button", () => {
    render(ChatToolbar, {
      props: {
        onNewChat: vi.fn(),
      },
    });

    expect(screen.getByTestId("chat-new-chat")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Callbacks
// ---------------------------------------------------------------------------

describe("ChatToolbar — callbacks", () => {
  it("should call onExportJson when Export is clicked", async () => {
    const onExportJson = vi.fn();
    const user = userEvent.setup();

    render(ChatToolbar, {
      props: {
        onExportJson,
        onNewChat: vi.fn(),
      },
    });

    await user.click(screen.getByTestId("chat-export-json"));

    expect(onExportJson).toHaveBeenCalledOnce();
  });

  it("should call onNewChat when New is clicked", async () => {
    const onNewChat = vi.fn();
    const user = userEvent.setup();

    render(ChatToolbar, {
      props: {
        onNewChat,
      },
    });

    await user.click(screen.getByTestId("chat-new-chat"));

    expect(onNewChat).toHaveBeenCalledOnce();
  });
});

// ---------------------------------------------------------------------------
// Accessibility
// ---------------------------------------------------------------------------

describe("ChatToolbar — accessibility", () => {
  it("Export button should have an accessible name", () => {
    render(ChatToolbar, {
      props: {
        onExportJson: vi.fn(),
        onNewChat: vi.fn(),
      },
    });

    expect(screen.getByTestId("chat-export-json")).toHaveAccessibleName();
  });

  it("New Chat button should have an accessible name", () => {
    render(ChatToolbar, {
      props: {
        onNewChat: vi.fn(),
      },
    });

    expect(screen.getByTestId("chat-new-chat")).toHaveAccessibleName();
  });

  it("toolbar buttons should be focusable", () => {
    render(ChatToolbar, {
      props: {
        onExportJson: vi.fn(),
        onNewChat: vi.fn(),
      },
    });

    const buttons = [
      screen.getByTestId("chat-export-json"),
      screen.getByTestId("chat-new-chat"),
    ];

    for (const button of buttons) {
      expect(button).not.toBeDisabled();
      button.focus();
      expect(document.activeElement).toBe(button);
    }
  });
});
