import type { ChatIntent } from "./chat";

// ---------------------------------------------------------------------------
// PageKind — the two pages that support slash commands
// ---------------------------------------------------------------------------

export type PageKind = "flow-builder" | "data-view";

// ---------------------------------------------------------------------------
// SlashCommandResult — what a command's parse() returns
// ---------------------------------------------------------------------------

export interface SlashCommandResult {
  intent: ChatIntent;
  content: string;
  params: Record<string, unknown>;
}

// ---------------------------------------------------------------------------
// SlashCommand — a registered command definition
// ---------------------------------------------------------------------------

export interface SlashCommand {
  name: string;
  aliases: string[];
  description: string;
  usage: string;
  intent: ChatIntent;
  availableOn: PageKind[];
  parse: (args: string) => SlashCommandResult;
}

// ---------------------------------------------------------------------------
// SlashCommandMatch — a resolved command paired with its parsed result
// ---------------------------------------------------------------------------

export interface SlashCommandMatch {
  command: SlashCommand;
  result: SlashCommandResult;
}
