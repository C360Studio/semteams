import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/svelte";
import userEvent from "@testing-library/user-event";
import SlashCommandMenu from "./SlashCommandMenu.svelte";
import type { SlashCommand } from "$lib/types/slashCommand";

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

function makeCommand(
  name: string,
  aliases: string[],
  description: string,
  availableOn: ("flow-builder" | "data-view")[] = ["flow-builder", "data-view"],
): SlashCommand {
  return {
    name,
    aliases,
    description,
    usage: `/${name} [args]`,
    intent: "general" as const,
    availableOn,
    parse: (args: string) => ({
      intent: "general" as const,
      content: args,
      params: {},
    }),
  };
}

const searchCommand = makeCommand(
  "search",
  ["s", "find"],
  "Search the knowledge graph",
);
const flowCommand = makeCommand(
  "flow",
  ["f", "create"],
  "Create or modify a flow",
  ["flow-builder"],
);
const explainCommand = makeCommand(
  "explain",
  ["e", "what"],
  "Explain an entity or component",
);
const debugCommand = makeCommand("debug", ["d"], "Diagnose runtime issues", [
  "flow-builder",
]);
const healthCommand = makeCommand(
  "health",
  ["h", "status"],
  "Show system health summary",
);

const allCommands: SlashCommand[] = [
  searchCommand,
  flowCommand,
  explainCommand,
  debugCommand,
  healthCommand,
];

// ---------------------------------------------------------------------------
// Rendering
// ---------------------------------------------------------------------------

describe("SlashCommandMenu — rendering", () => {
  it("renders the menu container", () => {
    render(SlashCommandMenu, {
      props: {
        commands: allCommands,
        filter: "",
        onSelect: vi.fn(),
        onDismiss: vi.fn(),
      },
    });
    expect(screen.getByTestId("slash-command-menu")).toBeInTheDocument();
  });

  it("renders all commands when filter is empty", () => {
    render(SlashCommandMenu, {
      props: {
        commands: allCommands,
        filter: "",
        onSelect: vi.fn(),
        onDismiss: vi.fn(),
      },
    });

    expect(screen.getByText("/search")).toBeInTheDocument();
    expect(screen.getByText("/flow")).toBeInTheDocument();
    expect(screen.getByText("/explain")).toBeInTheDocument();
    expect(screen.getByText("/debug")).toBeInTheDocument();
    expect(screen.getByText("/health")).toBeInTheDocument();
  });

  it("renders the command name with / prefix", () => {
    render(SlashCommandMenu, {
      props: {
        commands: [searchCommand],
        filter: "",
        onSelect: vi.fn(),
        onDismiss: vi.fn(),
      },
    });
    expect(screen.getByText(/\/search/)).toBeInTheDocument();
  });

  it("renders the command description", () => {
    render(SlashCommandMenu, {
      props: {
        commands: [searchCommand],
        filter: "",
        onSelect: vi.fn(),
        onDismiss: vi.fn(),
      },
    });
    expect(screen.getByText("Search the knowledge graph")).toBeInTheDocument();
  });

  it("renders an empty state when no commands provided", () => {
    render(SlashCommandMenu, {
      props: {
        commands: [],
        filter: "",
        onSelect: vi.fn(),
        onDismiss: vi.fn(),
      },
    });
    // Menu should exist but have no command items
    const menu = screen.getByTestId("slash-command-menu");
    expect(menu).toBeInTheDocument();
    // No command names rendered
    expect(screen.queryByText(/\/search/)).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Filtering
// ---------------------------------------------------------------------------

describe("SlashCommandMenu — filtering by name prefix", () => {
  it("shows only matching commands when filter is 'se'", () => {
    render(SlashCommandMenu, {
      props: {
        commands: allCommands,
        filter: "se",
        onSelect: vi.fn(),
        onDismiss: vi.fn(),
      },
    });
    expect(screen.getByText(/\/search/)).toBeInTheDocument();
    expect(screen.queryByText(/\/flow/)).not.toBeInTheDocument();
  });

  it("shows only matching commands when filter is 'he'", () => {
    render(SlashCommandMenu, {
      props: {
        commands: allCommands,
        filter: "he",
        onSelect: vi.fn(),
        onDismiss: vi.fn(),
      },
    });
    expect(screen.getByText(/\/health/)).toBeInTheDocument();
    expect(screen.queryByText(/\/search/)).not.toBeInTheDocument();
  });

  it("shows no commands when filter matches nothing", () => {
    render(SlashCommandMenu, {
      props: {
        commands: allCommands,
        filter: "xyz",
        onSelect: vi.fn(),
        onDismiss: vi.fn(),
      },
    });
    expect(screen.queryByText(/\/search/)).not.toBeInTheDocument();
    expect(screen.queryByText(/\/flow/)).not.toBeInTheDocument();
    expect(screen.queryByText(/\/health/)).not.toBeInTheDocument();
  });

  it("shows all commands when filter is empty string", () => {
    render(SlashCommandMenu, {
      props: {
        commands: allCommands,
        filter: "",
        onSelect: vi.fn(),
        onDismiss: vi.fn(),
      },
    });
    // All 5 command names should be present
    expect(screen.getByText(/\/search/)).toBeInTheDocument();
    expect(screen.getByText(/\/flow/)).toBeInTheDocument();
    expect(screen.getByText(/\/explain/)).toBeInTheDocument();
    expect(screen.getByText(/\/debug/)).toBeInTheDocument();
    expect(screen.getByText(/\/health/)).toBeInTheDocument();
  });
});

describe("SlashCommandMenu — filtering by alias prefix", () => {
  it("shows /search when filter is 'fi' (matches 'find' alias)", () => {
    render(SlashCommandMenu, {
      props: {
        commands: allCommands,
        filter: "fi",
        onSelect: vi.fn(),
        onDismiss: vi.fn(),
      },
    });
    expect(screen.getByText(/\/search/)).toBeInTheDocument();
  });

  it("shows /health when filter is 'st' (matches 'status' alias)", () => {
    render(SlashCommandMenu, {
      props: {
        commands: allCommands,
        filter: "st",
        onSelect: vi.fn(),
        onDismiss: vi.fn(),
      },
    });
    expect(screen.getByText(/\/health/)).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Interaction: onSelect
// ---------------------------------------------------------------------------

describe("SlashCommandMenu — onSelect callback", () => {
  it("calls onSelect with the correct command when clicked", async () => {
    const onSelect = vi.fn();
    const user = userEvent.setup();

    render(SlashCommandMenu, {
      props: {
        commands: [searchCommand],
        filter: "",
        onSelect,
        onDismiss: vi.fn(),
      },
    });

    await user.click(screen.getByText(/\/search/));
    expect(onSelect).toHaveBeenCalledOnce();
    expect(onSelect).toHaveBeenCalledWith(searchCommand);
  });

  it("calls onSelect with the correct command when second item clicked", async () => {
    const onSelect = vi.fn();
    const user = userEvent.setup();

    render(SlashCommandMenu, {
      props: {
        commands: [searchCommand, healthCommand],
        filter: "",
        onSelect,
        onDismiss: vi.fn(),
      },
    });

    await user.click(screen.getByText(/\/health/));
    expect(onSelect).toHaveBeenCalledWith(healthCommand);
  });

  it("does not call onSelect when no commands are shown", async () => {
    const onSelect = vi.fn();
    const user = userEvent.setup();

    render(SlashCommandMenu, {
      props: {
        commands: allCommands,
        filter: "xyz",
        onSelect,
        onDismiss: vi.fn(),
      },
    });

    // No clickable items — onSelect should not have been called
    expect(onSelect).not.toHaveBeenCalled();
    void user; // suppress unused variable warning
  });

  it("calls onSelect once per click (not multiple times)", async () => {
    const onSelect = vi.fn();
    const user = userEvent.setup();

    render(SlashCommandMenu, {
      props: {
        commands: [searchCommand],
        filter: "",
        onSelect,
        onDismiss: vi.fn(),
      },
    });

    await user.click(screen.getByText(/\/search/));
    expect(onSelect).toHaveBeenCalledTimes(1);
  });
});

// ---------------------------------------------------------------------------
// Accessibility
// ---------------------------------------------------------------------------

describe("SlashCommandMenu — accessibility", () => {
  it("menu has role='listbox'", () => {
    render(SlashCommandMenu, {
      props: {
        commands: allCommands,
        filter: "",
        onSelect: vi.fn(),
        onDismiss: vi.fn(),
      },
    });
    expect(screen.getByRole("listbox")).toBeInTheDocument();
  });

  it("each command item has role='option'", () => {
    render(SlashCommandMenu, {
      props: {
        commands: [searchCommand, healthCommand],
        filter: "",
        onSelect: vi.fn(),
        onDismiss: vi.fn(),
      },
    });
    const options = screen.getAllByRole("option");
    expect(options).toHaveLength(2);
  });

  it("menu items are focusable (keyboard navigation support)", () => {
    render(SlashCommandMenu, {
      props: {
        commands: [searchCommand],
        filter: "",
        onSelect: vi.fn(),
        onDismiss: vi.fn(),
      },
    });
    const option = screen.getByRole("option");
    // Options should be buttons or have tabindex for keyboard access
    const tagName = option.tagName.toLowerCase();
    const tabindex = option.getAttribute("tabindex");
    expect(tagName === "button" || tabindex !== null).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// Command details display
// ---------------------------------------------------------------------------

describe("SlashCommandMenu — command detail display", () => {
  it("shows command description text", () => {
    render(SlashCommandMenu, {
      props: {
        commands: [explainCommand],
        filter: "",
        onSelect: vi.fn(),
        onDismiss: vi.fn(),
      },
    });
    expect(
      screen.getByText("Explain an entity or component"),
    ).toBeInTheDocument();
  });

  it("shows description for each command in list", () => {
    render(SlashCommandMenu, {
      props: {
        commands: [searchCommand, healthCommand],
        filter: "",
        onSelect: vi.fn(),
        onDismiss: vi.fn(),
      },
    });
    expect(screen.getByText("Search the knowledge graph")).toBeInTheDocument();
    expect(screen.getByText("Show system health summary")).toBeInTheDocument();
  });
});
