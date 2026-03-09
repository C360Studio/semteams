import type {
  SlashCommand,
  SlashCommandMatch,
  PageKind,
} from "$lib/types/slashCommand";

// ---------------------------------------------------------------------------
// Command registry — all 5 slash commands
// ---------------------------------------------------------------------------

export const COMMANDS: SlashCommand[] = [
  {
    name: "search",
    aliases: ["s", "find"],
    description: "Search the knowledge graph",
    usage: "/search <query>",
    intent: "search",
    availableOn: ["flow-builder", "data-view"],
    parse: (args: string) => ({
      intent: "search",
      content: args,
      params: { query: args },
    }),
  },
  {
    name: "flow",
    aliases: ["f", "create"],
    description: "Create or modify a flow",
    usage: "/flow <description>",
    intent: "flow-create",
    availableOn: ["flow-builder"],
    parse: (args: string) => ({
      intent: "flow-create",
      content: args,
      params: { description: args },
    }),
  },
  {
    name: "explain",
    aliases: ["e", "what"],
    description: "Explain an entity or component",
    usage: "/explain <entity>",
    intent: "explain",
    availableOn: ["flow-builder", "data-view"],
    parse: (args: string) => ({
      intent: "explain",
      content: args,
      params: { target: args },
    }),
  },
  {
    name: "debug",
    aliases: ["d"],
    description: "Diagnose runtime issues",
    usage: "/debug <query>",
    intent: "debug",
    availableOn: ["flow-builder"],
    parse: (args: string) => ({
      intent: "debug",
      content: args,
      params: { query: args },
    }),
  },
  {
    name: "health",
    aliases: ["h", "status"],
    description: "Show system health summary",
    usage: "/health",
    intent: "health",
    availableOn: ["flow-builder", "data-view"],
    parse: (_args: string) => ({
      intent: "health",
      content: "Show system health",
      params: {},
    }),
  },
  {
    name: "query",
    aliases: ["q"],
    description: "Query entities and relationships in the data view",
    usage: "/query <expression>",
    intent: "general",
    availableOn: ["data-view"],
    parse: (args: string) => ({
      intent: "general",
      content: args,
      params: { query: args },
    }),
  },
];

// ---------------------------------------------------------------------------
// getCommandsForPage — filter COMMANDS by page availability
// ---------------------------------------------------------------------------

export function getCommandsForPage(page: PageKind): SlashCommand[] {
  return COMMANDS.filter((cmd) => cmd.availableOn.includes(page));
}

// ---------------------------------------------------------------------------
// filterCommands — match by name prefix or alias prefix
// ---------------------------------------------------------------------------

export function filterCommands(
  partial: string,
  page: PageKind,
): SlashCommand[] {
  const available = getCommandsForPage(page);
  if (!partial) return available;

  const lower = partial.toLowerCase();
  return available.filter(
    (cmd) =>
      cmd.name.startsWith(lower) ||
      cmd.aliases.some((alias) => alias.startsWith(lower)),
  );
}

// ---------------------------------------------------------------------------
// parseSlashCommand — parse a full user input string
// Returns a SlashCommandMatch if the input is a recognized slash command
// available on the given page, or null otherwise.
// ---------------------------------------------------------------------------

export function parseSlashCommand(
  input: string,
  page: PageKind,
): SlashCommandMatch | null {
  const trimmed = input.trim();
  if (!trimmed.startsWith("/")) return null;

  // Split on first whitespace: ["/commandName", "rest of args"]
  const withoutSlash = trimmed.slice(1); // e.g. "search drones in sector 7"
  const spaceIndex = withoutSlash.search(/\s/);
  const token =
    spaceIndex === -1 ? withoutSlash : withoutSlash.slice(0, spaceIndex);
  const rawArgs =
    spaceIndex === -1 ? "" : withoutSlash.slice(spaceIndex + 1).trimStart();

  const lowerToken = token.toLowerCase();

  // Find the command by primary name or alias (case-insensitive)
  const cmd = COMMANDS.find(
    (c) => c.name === lowerToken || c.aliases.includes(lowerToken),
  );

  if (!cmd) return null;

  // Check page availability
  if (!cmd.availableOn.includes(page)) return null;

  const result = cmd.parse(rawArgs);
  return { command: cmd, result };
}
