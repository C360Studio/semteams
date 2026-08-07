import type {
  SlashCommand,
  SlashCommandMatch,
  PageKind,
} from "$lib/types/slashCommand";

// ---------------------------------------------------------------------------
// Command registry — core navigation/search commands plus governed agent
// control shortcuts. Slash commands are UI shortcuts only: mutating commands
// resolve to the same agent-control action envelope as visible controls.
// ---------------------------------------------------------------------------

function firstArg(args: string): string | undefined {
  const trimmed = args.trim();
  if (!trimmed) return undefined;
  return trimmed.split(/\s+/, 1)[0];
}

function argsAfterFirst(args: string): string | undefined {
  const trimmed = args.trim();
  if (!trimmed) return undefined;
  const spaceIndex = trimmed.search(/\s/);
  if (spaceIndex === -1) return undefined;
  const rest = trimmed.slice(spaceIndex + 1).trim();
  return rest || undefined;
}

export const COMMANDS: SlashCommand[] = [
  {
    name: "research",
    aliases: [],
    description: "Ask the coordinator to validate and route a research prompt",
    usage: "/research <question>",
    intent: "general",
    availableOn: ["flow-builder", "data-view"],
    parse: (args: string) => ({
      intent: "general",
      content: `/research ${args}`.trim(),
      params: { requestedTeam: "research", prompt: args },
    }),
  },
  // The create-change and dev-via-test team commands are parked with their
  // category packs pending the canonical-predicate migration (ADR-058);
  // restore them here when the packs are re-wired.
  {
    name: "optimize",
    aliases: ["autoresearch"],
    description:
      "Ask the coordinator to validate and route a metric optimization prompt",
    usage: "/optimize <metric target>",
    intent: "general",
    availableOn: ["flow-builder", "data-view"],
    parse: (args: string) => ({
      intent: "general",
      content: `/optimize ${args}`.trim(),
      params: { requestedTeam: "autoresearch", prompt: args },
    }),
  },
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
  {
    name: "approve",
    aliases: ["yes", "ok"],
    description: "Approve an agent's pending action",
    usage: "/approve [loop-id]",
    intent: "agent-control",
    availableOn: ["flow-builder", "data-view"],
    parse: (args: string) => ({
      intent: "agent-control",
      content: `/approve ${args}`.trim(),
      params: { action: "approve", loopId: args.trim() || undefined },
    }),
  },
  {
    name: "reject",
    aliases: ["no", "deny"],
    description: "Reject an agent's pending action",
    usage: "/reject [loop-id] [reason]",
    intent: "agent-control",
    availableOn: ["flow-builder", "data-view"],
    parse: (args: string) => {
      const parts = args.trim().split(/\s+/);
      const loopId = parts[0] || undefined;
      const reason = parts.slice(1).join(" ") || undefined;
      return {
        intent: "agent-control",
        content: `/reject ${args}`.trim(),
        params: { action: "reject", loopId, reason },
      };
    },
  },
  {
    name: "pause",
    aliases: [],
    description: "Pause an active agent loop",
    usage: "/pause <loop-id>",
    intent: "agent-control",
    availableOn: ["flow-builder", "data-view"],
    parse: (args: string) => ({
      intent: "agent-control",
      content: `/pause ${args}`.trim(),
      params: { action: "pause", loopId: args.trim() },
    }),
  },
  {
    name: "resume",
    aliases: [],
    description: "Resume a paused agent loop",
    usage: "/resume <loop-id>",
    intent: "agent-control",
    availableOn: ["flow-builder", "data-view"],
    parse: (args: string) => ({
      intent: "agent-control",
      content: `/resume ${args}`.trim(),
      params: { action: "resume", loopId: args.trim() },
    }),
  },
  {
    name: "export-spec",
    aliases: ["spec-export"],
    description: "Export an OpenSpec change artifact",
    usage: "/export-spec <change-slug> [folder|document]",
    intent: "agent-control",
    availableOn: ["flow-builder", "data-view"],
    parse: (args: string) => {
      const changeSlug = firstArg(args);
      const requestedFormat = argsAfterFirst(args);
      const format =
        requestedFormat === "document" || requestedFormat === "folder"
          ? requestedFormat
          : "folder";
      return {
        intent: "agent-control",
        content: `/export-spec ${args}`.trim(),
        params: {
          action: "export-spec",
          changeSlug,
          format,
          requestedFormat,
        },
      };
    },
  },
  {
    name: "implement-spec",
    aliases: ["dev-via-spec"],
    description:
      "Start governed implementation for an approved OpenSpec change",
    usage: "/implement-spec <change-slug>",
    intent: "agent-control",
    availableOn: ["flow-builder", "data-view"],
    parse: (args: string) => ({
      intent: "agent-control",
      content: `/implement-spec ${args}`.trim(),
      params: {
        action: "implement-spec",
        changeSlug: firstArg(args),
      },
    }),
  },
  {
    name: "run-status",
    aliases: ["run"],
    description: "Show the current governed run status",
    usage: "/run-status [run-id]",
    intent: "agent-control",
    availableOn: ["flow-builder", "data-view"],
    parse: (args: string) => ({
      intent: "agent-control",
      content: `/run-status ${args}`.trim(),
      params: {
        action: "run-status",
        runId: firstArg(args),
      },
    }),
  },
  {
    name: "evidence",
    aliases: ["proof"],
    description: "Show proof evidence for a run, claim, or dependency",
    usage: "/evidence [run-id|claim-id|dependency-id]",
    intent: "agent-control",
    availableOn: ["flow-builder", "data-view"],
    parse: (args: string) => ({
      intent: "agent-control",
      content: `/evidence ${args}`.trim(),
      params: {
        action: "evidence",
        target: firstArg(args),
      },
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
