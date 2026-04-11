// Attack tests by Reviewer Agent.
// These test adversarial inputs and edge cases.

import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/svelte";
import userEvent from "@testing-library/user-event";
import SlashCommandMenu from "./SlashCommandMenu.svelte";
import type { SlashCommand } from "$lib/types/slashCommand";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeCommand(
  name: string,
  aliases: string[] = [],
  description = "A command",
): SlashCommand {
  return {
    name,
    aliases,
    description,
    usage: `/${name} [args]`,
    intent: "general",
    availableOn: ["flow-builder", "data-view"],
    parse: (args: string) => ({
      intent: "general",
      content: args,
      params: {},
    }),
  };
}

// ---------------------------------------------------------------------------
// Empty command list
// ---------------------------------------------------------------------------

describe("SlashCommandMenu — empty commands array", () => {
  it("renders without throwing when commands is empty", () => {
    expect(() =>
      render(SlashCommandMenu, {
        props: {
          commands: [],
          filter: "",
          onSelect: vi.fn(),
          onDismiss: vi.fn(),
        },
      }),
    ).not.toThrow();
  });

  it("renders menu container when commands is empty", () => {
    render(SlashCommandMenu, {
      props: {
        commands: [],
        filter: "",
        onSelect: vi.fn(),
        onDismiss: vi.fn(),
      },
    });
    expect(screen.getByTestId("slash-command-menu")).toBeInTheDocument();
  });

  it("shows no options when commands is empty", () => {
    render(SlashCommandMenu, {
      props: {
        commands: [],
        filter: "",
        onSelect: vi.fn(),
        onDismiss: vi.fn(),
      },
    });
    expect(screen.queryAllByRole("option")).toHaveLength(0);
  });
});

// ---------------------------------------------------------------------------
// Large command lists
// ---------------------------------------------------------------------------

describe("SlashCommandMenu — large command lists", () => {
  it("renders 500 commands without throwing", () => {
    const many = Array.from({ length: 500 }, (_, i) =>
      makeCommand(`command${i}`, [], `Description ${i}`),
    );
    expect(() =>
      render(SlashCommandMenu, {
        props: {
          commands: many,
          filter: "",
          onSelect: vi.fn(),
          onDismiss: vi.fn(),
        },
      }),
    ).not.toThrow();
  });

  it("renders 500 commands and the menu container is present", () => {
    const many = Array.from({ length: 500 }, (_, i) =>
      makeCommand(`cmd${i}`, [], `Desc ${i}`),
    );
    render(SlashCommandMenu, {
      props: {
        commands: many,
        filter: "",
        onSelect: vi.fn(),
        onDismiss: vi.fn(),
      },
    });
    expect(screen.getByTestId("slash-command-menu")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Adversarial filter values
// ---------------------------------------------------------------------------

describe("SlashCommandMenu — adversarial filter values", () => {
  const commands = [
    makeCommand("search", ["s", "find"], "Search"),
    makeCommand("health", ["h", "status"], "Health"),
  ];

  it("handles filter with regex special chars '.*' without throwing", () => {
    expect(() =>
      render(SlashCommandMenu, {
        props: {
          commands,
          filter: ".*",
          onSelect: vi.fn(),
          onDismiss: vi.fn(),
        },
      }),
    ).not.toThrow();
    // '.*' does not startsWith any command name
    expect(screen.queryAllByRole("option")).toHaveLength(0);
  });

  it("handles filter '[abc]' (regex char class) without throwing", () => {
    expect(() =>
      render(SlashCommandMenu, {
        props: {
          commands,
          filter: "[abc]",
          onSelect: vi.fn(),
          onDismiss: vi.fn(),
        },
      }),
    ).not.toThrow();
  });

  it("handles very long filter string without throwing", () => {
    expect(() =>
      render(SlashCommandMenu, {
        props: {
          commands,
          filter: "s".repeat(10000),
          onSelect: vi.fn(),
          onDismiss: vi.fn(),
        },
      }),
    ).not.toThrow();
  });

  it("handles emoji filter without throwing", () => {
    expect(() =>
      render(SlashCommandMenu, {
        props: {
          commands,
          filter: "🔍",
          onSelect: vi.fn(),
          onDismiss: vi.fn(),
        },
      }),
    ).not.toThrow();
    expect(screen.queryAllByRole("option")).toHaveLength(0);
  });
});

// ---------------------------------------------------------------------------
// Adversarial command content (XSS in name/description)
// ---------------------------------------------------------------------------

describe("SlashCommandMenu — adversarial command content", () => {
  it("renders XSS-laden description as text without executing it", () => {
    const xssCommand = makeCommand("safe", [], "<script>alert('xss')</script>");
    render(SlashCommandMenu, {
      props: {
        commands: [xssCommand],
        filter: "",
        onSelect: vi.fn(),
        onDismiss: vi.fn(),
      },
    });
    // The script tag text should appear as text content, not execute
    expect(
      screen.getByText("<script>alert('xss')</script>"),
    ).toBeInTheDocument();
  });

  it("renders HTML entity in description safely", () => {
    const htmlCmd = makeCommand("safe2", [], "Search &amp; filter");
    render(SlashCommandMenu, {
      props: {
        commands: [htmlCmd],
        filter: "",
        onSelect: vi.fn(),
        onDismiss: vi.fn(),
      },
    });
    // Should render as literal text, not decode HTML entities
    expect(screen.getByText("Search &amp; filter")).toBeInTheDocument();
  });

  it("handles command with empty aliases array without throwing", () => {
    const noAliasCmd = makeCommand("solo", [], "No aliases here");
    expect(() =>
      render(SlashCommandMenu, {
        props: {
          commands: [noAliasCmd],
          filter: "",
          onSelect: vi.fn(),
          onDismiss: vi.fn(),
        },
      }),
    ).not.toThrow();
  });
});

// ---------------------------------------------------------------------------
// Rapid interactions — no race conditions
// ---------------------------------------------------------------------------

describe("SlashCommandMenu — rapid interactions", () => {
  it("handles rapid clicks on the same item without calling onSelect multiple times spuriously", async () => {
    const onSelect = vi.fn();
    const user = userEvent.setup();

    const searchCommand = makeCommand("search", ["s", "find"], "Search");

    render(SlashCommandMenu, {
      props: {
        commands: [searchCommand],
        filter: "",
        onSelect,
        onDismiss: vi.fn(),
      },
    });

    const option = screen.getByRole("option");

    // Three sequential clicks
    await user.click(option);
    await user.click(option);
    await user.click(option);

    // Each click should fire exactly one onSelect call
    expect(onSelect).toHaveBeenCalledTimes(3);
    // All calls should be with the same command object
    expect(onSelect).toHaveBeenNthCalledWith(1, searchCommand);
    expect(onSelect).toHaveBeenNthCalledWith(2, searchCommand);
    expect(onSelect).toHaveBeenNthCalledWith(3, searchCommand);
  });

  it("calls onDismiss on Escape key without calling onSelect", async () => {
    const onSelect = vi.fn();
    const onDismiss = vi.fn();
    const user = userEvent.setup();

    const searchCommand = makeCommand("search", ["s"], "Search");

    render(SlashCommandMenu, {
      props: {
        commands: [searchCommand],
        filter: "",
        onSelect,
        onDismiss,
      },
    });

    const option = screen.getByRole("option");
    option.focus();
    await user.keyboard("{Escape}");

    expect(onDismiss).toHaveBeenCalledOnce();
    expect(onSelect).not.toHaveBeenCalled();
  });

  it("calls onSelect on Enter key press on a focused item", async () => {
    const onSelect = vi.fn();
    const user = userEvent.setup();

    const searchCommand = makeCommand("search", ["s"], "Search");

    render(SlashCommandMenu, {
      props: {
        commands: [searchCommand],
        filter: "",
        onSelect,
        onDismiss: vi.fn(),
      },
    });

    const option = screen.getByRole("option");
    option.focus();
    await user.keyboard("{Enter}");

    expect(onSelect).toHaveBeenCalledOnce();
    expect(onSelect).toHaveBeenCalledWith(searchCommand);
  });

  it("calls onSelect on Space key press on a focused item", async () => {
    const onSelect = vi.fn();
    const user = userEvent.setup();

    const searchCommand = makeCommand("search", ["s"], "Search");

    render(SlashCommandMenu, {
      props: {
        commands: [searchCommand],
        filter: "",
        onSelect,
        onDismiss: vi.fn(),
      },
    });

    const option = screen.getByRole("option");
    option.focus();
    await user.keyboard(" ");

    expect(onSelect).toHaveBeenCalledOnce();
  });
});

// ---------------------------------------------------------------------------
// Undefined/missing optional props
// ---------------------------------------------------------------------------

describe("SlashCommandMenu — missing optional props", () => {
  it("renders without onDismiss prop without throwing", () => {
    expect(() =>
      render(SlashCommandMenu, {
        props: {
          commands: [makeCommand("search", [], "Search")],
          filter: "",
          onSelect: vi.fn(),
          // onDismiss intentionally omitted
        },
      }),
    ).not.toThrow();
  });
});
