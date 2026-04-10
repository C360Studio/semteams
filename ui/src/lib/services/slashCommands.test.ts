import { describe, it, expect } from "vitest";
import {
  parseSlashCommand,
  getCommandsForPage,
  filterCommands,
} from "./slashCommands";
import type { PageKind } from "$lib/types/slashCommand";

// ---------------------------------------------------------------------------
// parseSlashCommand — basic recognition
// ---------------------------------------------------------------------------

describe("parseSlashCommand — not a slash command", () => {
  it("returns null for plain text", () => {
    expect(parseSlashCommand("hello world", "flow-builder")).toBeNull();
  });

  it("returns null for empty string", () => {
    expect(parseSlashCommand("", "flow-builder")).toBeNull();
  });

  it("returns null for whitespace-only input", () => {
    expect(parseSlashCommand("   ", "flow-builder")).toBeNull();
  });

  it("returns null for unrecognized command /unknown", () => {
    expect(parseSlashCommand("/unknown foo bar", "flow-builder")).toBeNull();
  });

  it("returns null for /unknown on data-view", () => {
    expect(parseSlashCommand("/unknown", "data-view")).toBeNull();
  });

  it("returns null when recognized command is not available on page", () => {
    // /flow is only available on flow-builder, not data-view
    expect(
      parseSlashCommand("/flow create a pipeline", "data-view"),
    ).toBeNull();
  });

  it("returns null when /debug used on data-view (debug is flow-builder only)", () => {
    expect(parseSlashCommand("/debug check nats", "data-view")).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// parseSlashCommand — all 5 commands by primary name
// ---------------------------------------------------------------------------

describe("parseSlashCommand — primary command names", () => {
  it("/search returns match with search intent", () => {
    const result = parseSlashCommand(
      "/search drones in sector 7",
      "flow-builder",
    );
    expect(result).not.toBeNull();
    expect(result!.command.name).toBe("search");
    expect(result!.result.intent).toBe("search");
  });

  it("/flow returns match with flow-create intent on flow-builder", () => {
    const result = parseSlashCommand(
      "/flow build a NATS pipeline",
      "flow-builder",
    );
    expect(result).not.toBeNull();
    expect(result!.command.name).toBe("flow");
    expect(result!.result.intent).toBe("flow-create");
  });

  it("/explain returns match with explain intent", () => {
    const result = parseSlashCommand("/explain this component", "flow-builder");
    expect(result).not.toBeNull();
    expect(result!.command.name).toBe("explain");
    expect(result!.result.intent).toBe("explain");
  });

  it("/debug returns match with debug intent on flow-builder", () => {
    const result = parseSlashCommand(
      "/debug check the processor",
      "flow-builder",
    );
    expect(result).not.toBeNull();
    expect(result!.command.name).toBe("debug");
    expect(result!.result.intent).toBe("debug");
  });

  it("/health returns match with health intent", () => {
    const result = parseSlashCommand("/health", "flow-builder");
    expect(result).not.toBeNull();
    expect(result!.command.name).toBe("health");
    expect(result!.result.intent).toBe("health");
  });
});

// ---------------------------------------------------------------------------
// parseSlashCommand — aliases
// ---------------------------------------------------------------------------

describe("parseSlashCommand — aliases", () => {
  const aliasTestCases: Array<{
    input: string;
    page: PageKind;
    expectedCommandName: string;
    expectedIntent: string;
    label: string;
  }> = [
    {
      input: "/s drones",
      page: "flow-builder",
      expectedCommandName: "search",
      expectedIntent: "search",
      label: "/s alias for search",
    },
    {
      input: "/find robots",
      page: "data-view",
      expectedCommandName: "search",
      expectedIntent: "search",
      label: "/find alias for search",
    },
    {
      input: "/f add a http input",
      page: "flow-builder",
      expectedCommandName: "flow",
      expectedIntent: "flow-create",
      label: "/f alias for flow",
    },
    {
      input: "/create nats pipeline",
      page: "flow-builder",
      expectedCommandName: "flow",
      expectedIntent: "flow-create",
      label: "/create alias for flow",
    },
    {
      input: "/e this node",
      page: "flow-builder",
      expectedCommandName: "explain",
      expectedIntent: "explain",
      label: "/e alias for explain",
    },
    {
      input: "/what is this",
      page: "data-view",
      expectedCommandName: "explain",
      expectedIntent: "explain",
      label: "/what alias for explain",
    },
    {
      input: "/d check processor",
      page: "flow-builder",
      expectedCommandName: "debug",
      expectedIntent: "debug",
      label: "/d alias for debug",
    },
    {
      input: "/h",
      page: "flow-builder",
      expectedCommandName: "health",
      expectedIntent: "health",
      label: "/h alias for health",
    },
    {
      input: "/status",
      page: "data-view",
      expectedCommandName: "health",
      expectedIntent: "health",
      label: "/status alias for health",
    },
  ];

  it.each(aliasTestCases)(
    "$label",
    ({ input, page, expectedCommandName, expectedIntent }) => {
      const result = parseSlashCommand(input, page);
      expect(result).not.toBeNull();
      expect(result!.command.name).toBe(expectedCommandName);
      expect(result!.result.intent).toBe(expectedIntent);
    },
  );
});

// ---------------------------------------------------------------------------
// parseSlashCommand — query extraction
// ---------------------------------------------------------------------------

describe("parseSlashCommand — query extraction", () => {
  it("extracts query after command name", () => {
    const result = parseSlashCommand(
      "/search drones in sector 7",
      "flow-builder",
    );
    expect(result!.result.content).toBe("drones in sector 7");
  });

  it("extracts empty string when no args given", () => {
    const result = parseSlashCommand("/search", "flow-builder");
    expect(result).not.toBeNull();
    // content may be empty string or a default; it should not contain the command name
    expect(result!.result.content).not.toContain("/search");
  });

  it("extracts multi-word query after /flow", () => {
    const result = parseSlashCommand(
      "/flow build a NATS pipeline with HTTP input",
      "flow-builder",
    );
    expect(result!.result.content).toBe(
      "build a NATS pipeline with HTTP input",
    );
  });

  it("extracts content from /explain with entity ID", () => {
    const result = parseSlashCommand(
      "/explain c360.ops.robotics.gcs.drone.001",
      "data-view",
    );
    expect(result!.result.content).toBe("c360.ops.robotics.gcs.drone.001");
  });

  it("/health extracts no meaningful query (health has no args)", () => {
    const result = parseSlashCommand("/health", "flow-builder");
    expect(result).not.toBeNull();
    // result.content is some non-null string (the default message)
    expect(typeof result!.result.content).toBe("string");
  });
});

// ---------------------------------------------------------------------------
// parseSlashCommand — case insensitivity
// ---------------------------------------------------------------------------

describe("parseSlashCommand — case insensitive", () => {
  it("matches /SEARCH as search", () => {
    const result = parseSlashCommand("/SEARCH drones", "flow-builder");
    expect(result).not.toBeNull();
    expect(result!.command.name).toBe("search");
  });

  it("matches /Search (mixed case) as search", () => {
    const result = parseSlashCommand("/Search robots", "data-view");
    expect(result).not.toBeNull();
    expect(result!.command.name).toBe("search");
  });

  it("matches /FLOW on flow-builder", () => {
    const result = parseSlashCommand("/FLOW add a sink", "flow-builder");
    expect(result).not.toBeNull();
    expect(result!.command.name).toBe("flow");
  });

  it("matches /Health on data-view", () => {
    const result = parseSlashCommand("/Health", "data-view");
    expect(result).not.toBeNull();
    expect(result!.command.name).toBe("health");
  });
});

// ---------------------------------------------------------------------------
// parseSlashCommand — extra whitespace handling
// ---------------------------------------------------------------------------

describe("parseSlashCommand — whitespace handling", () => {
  it("trims leading whitespace before the slash", () => {
    const result = parseSlashCommand("   /search drones", "flow-builder");
    expect(result).not.toBeNull();
    expect(result!.command.name).toBe("search");
  });

  it("handles multiple spaces between command and args", () => {
    const result = parseSlashCommand(
      "/search   lots of spaces",
      "flow-builder",
    );
    expect(result).not.toBeNull();
    // content should have the args trimmed of leading spaces
    expect(result!.result.content).not.toMatch(/^\s/);
  });

  it("handles trailing whitespace after command with no args", () => {
    const result = parseSlashCommand("/health   ", "flow-builder");
    expect(result).not.toBeNull();
    expect(result!.command.name).toBe("health");
  });
});

// ---------------------------------------------------------------------------
// parseSlashCommand — page availability
// ---------------------------------------------------------------------------

describe("parseSlashCommand — page availability", () => {
  const availabilityTestCases: Array<{
    input: string;
    page: PageKind;
    shouldMatch: boolean;
    label: string;
  }> = [
    {
      input: "/search test",
      page: "flow-builder",
      shouldMatch: true,
      label: "/search on flow-builder",
    },
    {
      input: "/search test",
      page: "data-view",
      shouldMatch: true,
      label: "/search on data-view",
    },
    {
      input: "/flow build something",
      page: "flow-builder",
      shouldMatch: true,
      label: "/flow on flow-builder",
    },
    {
      input: "/flow build something",
      page: "data-view",
      shouldMatch: false,
      label: "/flow on data-view (not available)",
    },
    {
      input: "/explain this",
      page: "flow-builder",
      shouldMatch: true,
      label: "/explain on flow-builder",
    },
    {
      input: "/explain this",
      page: "data-view",
      shouldMatch: true,
      label: "/explain on data-view",
    },
    {
      input: "/debug processor",
      page: "flow-builder",
      shouldMatch: true,
      label: "/debug on flow-builder",
    },
    {
      input: "/debug processor",
      page: "data-view",
      shouldMatch: false,
      label: "/debug on data-view (not available)",
    },
    {
      input: "/health",
      page: "flow-builder",
      shouldMatch: true,
      label: "/health on flow-builder",
    },
    {
      input: "/health",
      page: "data-view",
      shouldMatch: true,
      label: "/health on data-view",
    },
  ];

  it.each(availabilityTestCases)("$label", ({ input, page, shouldMatch }) => {
    const result = parseSlashCommand(input, page);
    if (shouldMatch) {
      expect(result).not.toBeNull();
    } else {
      expect(result).toBeNull();
    }
  });
});

// ---------------------------------------------------------------------------
// getCommandsForPage
// ---------------------------------------------------------------------------

describe("getCommandsForPage — flow-builder", () => {
  // Updated count: 5 original + 4 agent-control commands (approve, reject, pause, resume) = 9
  it("returns all 9 commands for flow-builder", () => {
    const commands = getCommandsForPage("flow-builder");
    expect(commands).toHaveLength(9);
  });

  it("includes search on flow-builder", () => {
    const commands = getCommandsForPage("flow-builder");
    expect(commands.some((c) => c.name === "search")).toBe(true);
  });

  it("includes flow on flow-builder", () => {
    const commands = getCommandsForPage("flow-builder");
    expect(commands.some((c) => c.name === "flow")).toBe(true);
  });

  it("includes explain on flow-builder", () => {
    const commands = getCommandsForPage("flow-builder");
    expect(commands.some((c) => c.name === "explain")).toBe(true);
  });

  it("includes debug on flow-builder", () => {
    const commands = getCommandsForPage("flow-builder");
    expect(commands.some((c) => c.name === "debug")).toBe(true);
  });

  it("includes health on flow-builder", () => {
    const commands = getCommandsForPage("flow-builder");
    expect(commands.some((c) => c.name === "health")).toBe(true);
  });

  it("every returned command has availableOn including 'flow-builder'", () => {
    const commands = getCommandsForPage("flow-builder");
    for (const cmd of commands) {
      expect(cmd.availableOn).toContain("flow-builder");
    }
  });
});

describe("getCommandsForPage — data-view", () => {
  // Updated count: 4 original + 4 agent-control commands (approve, reject, pause, resume) = 8
  it("returns 8 commands for data-view (no /flow, no /debug)", () => {
    const commands = getCommandsForPage("data-view");
    expect(commands).toHaveLength(8);
  });

  it("includes search on data-view", () => {
    const commands = getCommandsForPage("data-view");
    expect(commands.some((c) => c.name === "search")).toBe(true);
  });

  it("does NOT include flow on data-view", () => {
    const commands = getCommandsForPage("data-view");
    expect(commands.some((c) => c.name === "flow")).toBe(false);
  });

  it("includes explain on data-view", () => {
    const commands = getCommandsForPage("data-view");
    expect(commands.some((c) => c.name === "explain")).toBe(true);
  });

  it("does NOT include debug on data-view", () => {
    const commands = getCommandsForPage("data-view");
    expect(commands.some((c) => c.name === "debug")).toBe(false);
  });

  it("includes health on data-view", () => {
    const commands = getCommandsForPage("data-view");
    expect(commands.some((c) => c.name === "health")).toBe(true);
  });

  it("every returned command has availableOn including 'data-view'", () => {
    const commands = getCommandsForPage("data-view");
    for (const cmd of commands) {
      expect(cmd.availableOn).toContain("data-view");
    }
  });
});

// ---------------------------------------------------------------------------
// filterCommands
// ---------------------------------------------------------------------------

describe("filterCommands — empty filter returns all available", () => {
  it("returns all flow-builder commands when filter is empty string", () => {
    const all = getCommandsForPage("flow-builder");
    const filtered = filterCommands("", "flow-builder");
    expect(filtered).toHaveLength(all.length);
  });

  it("returns all data-view commands when filter is empty string", () => {
    const all = getCommandsForPage("data-view");
    const filtered = filterCommands("", "data-view");
    expect(filtered).toHaveLength(all.length);
  });
});

describe("filterCommands — matching by name prefix", () => {
  it("'se' matches /search on flow-builder", () => {
    const filtered = filterCommands("se", "flow-builder");
    expect(filtered.some((c) => c.name === "search")).toBe(true);
  });

  it("'fl' matches /flow on flow-builder", () => {
    const filtered = filterCommands("fl", "flow-builder");
    expect(filtered.some((c) => c.name === "flow")).toBe(true);
  });

  it("'he' matches /health", () => {
    const filtered = filterCommands("he", "flow-builder");
    expect(filtered.some((c) => c.name === "health")).toBe(true);
  });

  it("'search' (exact full name) matches /search", () => {
    const filtered = filterCommands("search", "data-view");
    expect(filtered.some((c) => c.name === "search")).toBe(true);
  });

  it("'xyz' matches nothing", () => {
    const filtered = filterCommands("xyz", "flow-builder");
    expect(filtered).toHaveLength(0);
  });
});

describe("filterCommands — page constraint respected", () => {
  it("'fl' on data-view returns empty (no /flow there)", () => {
    const filtered = filterCommands("fl", "data-view");
    expect(filtered.some((c) => c.name === "flow")).toBe(false);
  });

  it("'de' on data-view returns empty (no /debug there)", () => {
    const filtered = filterCommands("de", "data-view");
    expect(filtered.some((c) => c.name === "debug")).toBe(false);
  });
});

describe("filterCommands — matching by alias prefix", () => {
  it("'fi' matches /search via 'find' alias on flow-builder", () => {
    const filtered = filterCommands("fi", "flow-builder");
    expect(filtered.some((c) => c.name === "search")).toBe(true);
  });

  it("'st' matches /health via 'status' alias on data-view", () => {
    const filtered = filterCommands("st", "data-view");
    expect(filtered.some((c) => c.name === "health")).toBe(true);
  });

  it("'cr' matches /flow via 'create' alias on flow-builder", () => {
    const filtered = filterCommands("cr", "flow-builder");
    expect(filtered.some((c) => c.name === "flow")).toBe(true);
  });

  it("'wh' matches /explain via 'what' alias on flow-builder", () => {
    const filtered = filterCommands("wh", "flow-builder");
    expect(filtered.some((c) => c.name === "explain")).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// Phase 4 — agent-control slash commands: /approve, /reject, /pause, /resume
// ---------------------------------------------------------------------------

describe("parseSlashCommand — /approve", () => {
  it("/approve loop123 returns agent-control intent with loopId", () => {
    const result = parseSlashCommand("/approve loop123", "flow-builder");
    expect(result).not.toBeNull();
    expect(result!.command.name).toBe("approve");
    expect(result!.result.intent).toBe("agent-control");
    expect(result!.result.params).toMatchObject({
      action: "approve",
      loopId: "loop123",
    });
  });

  it("/approve with no args has undefined loopId", () => {
    const result = parseSlashCommand("/approve", "flow-builder");
    expect(result).not.toBeNull();
    expect(result!.result.intent).toBe("agent-control");
    expect(result!.result.params).toMatchObject({
      action: "approve",
      loopId: undefined,
    });
  });

  it("/yes resolves to approve via alias", () => {
    const result = parseSlashCommand("/yes loop123", "data-view");
    expect(result).not.toBeNull();
    expect(result!.command.name).toBe("approve");
    expect(result!.result.intent).toBe("agent-control");
    expect(result!.result.params).toMatchObject({
      action: "approve",
      loopId: "loop123",
    });
  });

  it("/ok resolves to approve via alias", () => {
    const result = parseSlashCommand("/ok loop456", "flow-builder");
    expect(result).not.toBeNull();
    expect(result!.command.name).toBe("approve");
    expect(result!.result.params).toMatchObject({
      action: "approve",
      loopId: "loop456",
    });
  });

  it("/approve is available on both pages", () => {
    const fb = parseSlashCommand("/approve loop1", "flow-builder");
    const dv = parseSlashCommand("/approve loop1", "data-view");
    expect(fb).not.toBeNull();
    expect(dv).not.toBeNull();
  });
});

describe("parseSlashCommand — /reject", () => {
  it("/reject loop123 bad idea parses loopId and reason", () => {
    const result = parseSlashCommand(
      "/reject loop123 bad idea",
      "flow-builder",
    );
    expect(result).not.toBeNull();
    expect(result!.command.name).toBe("reject");
    expect(result!.result.intent).toBe("agent-control");
    expect(result!.result.params).toMatchObject({
      action: "reject",
      loopId: "loop123",
      reason: "bad idea",
    });
  });

  it("/reject loop123 with no reason has undefined reason", () => {
    const result = parseSlashCommand("/reject loop123", "flow-builder");
    expect(result).not.toBeNull();
    expect(result!.result.params).toMatchObject({
      action: "reject",
      loopId: "loop123",
      reason: undefined,
    });
  });

  it("/reject with no args has undefined loopId and reason", () => {
    const result = parseSlashCommand("/reject", "data-view");
    expect(result).not.toBeNull();
    expect(result!.result.params).toMatchObject({
      action: "reject",
      loopId: undefined,
      reason: undefined,
    });
  });

  it("/no resolves to reject via alias", () => {
    const result = parseSlashCommand("/no loop123", "data-view");
    expect(result).not.toBeNull();
    expect(result!.command.name).toBe("reject");
    expect(result!.result.intent).toBe("agent-control");
  });

  it("/deny resolves to reject via alias", () => {
    const result = parseSlashCommand("/deny loop789 not ready", "flow-builder");
    expect(result).not.toBeNull();
    expect(result!.command.name).toBe("reject");
    expect(result!.result.params).toMatchObject({
      action: "reject",
      loopId: "loop789",
      reason: "not ready",
    });
  });

  it("/reject is available on both pages", () => {
    const fb = parseSlashCommand("/reject loop1", "flow-builder");
    const dv = parseSlashCommand("/reject loop1", "data-view");
    expect(fb).not.toBeNull();
    expect(dv).not.toBeNull();
  });
});

describe("parseSlashCommand — /pause", () => {
  it("/pause loop123 returns agent-control intent", () => {
    const result = parseSlashCommand("/pause loop123", "flow-builder");
    expect(result).not.toBeNull();
    expect(result!.command.name).toBe("pause");
    expect(result!.result.intent).toBe("agent-control");
    expect(result!.result.params).toMatchObject({
      action: "pause",
      loopId: "loop123",
    });
  });

  it("/pause is available on both pages", () => {
    const fb = parseSlashCommand("/pause loop1", "flow-builder");
    const dv = parseSlashCommand("/pause loop1", "data-view");
    expect(fb).not.toBeNull();
    expect(dv).not.toBeNull();
  });
});

describe("parseSlashCommand — /resume", () => {
  it("/resume loop123 returns agent-control intent", () => {
    const result = parseSlashCommand("/resume loop123", "flow-builder");
    expect(result).not.toBeNull();
    expect(result!.command.name).toBe("resume");
    expect(result!.result.intent).toBe("agent-control");
    expect(result!.result.params).toMatchObject({
      action: "resume",
      loopId: "loop123",
    });
  });

  it("/resume is available on both pages", () => {
    const fb = parseSlashCommand("/resume loop1", "flow-builder");
    const dv = parseSlashCommand("/resume loop1", "data-view");
    expect(fb).not.toBeNull();
    expect(dv).not.toBeNull();
  });
});

describe("getCommandsForPage — includes agent-control commands", () => {
  it("flow-builder includes approve, reject, pause, resume", () => {
    const commands = getCommandsForPage("flow-builder");
    const names = commands.map((c) => c.name);
    expect(names).toContain("approve");
    expect(names).toContain("reject");
    expect(names).toContain("pause");
    expect(names).toContain("resume");
  });

  it("data-view includes approve, reject, pause, resume", () => {
    const commands = getCommandsForPage("data-view");
    const names = commands.map((c) => c.name);
    expect(names).toContain("approve");
    expect(names).toContain("reject");
    expect(names).toContain("pause");
    expect(names).toContain("resume");
  });
});

describe("filterCommands — agent-control commands", () => {
  it("'ap' matches /approve on flow-builder", () => {
    const filtered = filterCommands("ap", "flow-builder");
    expect(filtered.some((c) => c.name === "approve")).toBe(true);
  });

  it("'re' matches /reject and /resume on flow-builder", () => {
    const filtered = filterCommands("re", "flow-builder");
    expect(filtered.some((c) => c.name === "reject")).toBe(true);
    expect(filtered.some((c) => c.name === "resume")).toBe(true);
  });

  it("'pa' matches /pause on data-view", () => {
    const filtered = filterCommands("pa", "data-view");
    expect(filtered.some((c) => c.name === "pause")).toBe(true);
  });

  it("'ye' matches /approve via 'yes' alias", () => {
    const filtered = filterCommands("ye", "flow-builder");
    expect(filtered.some((c) => c.name === "approve")).toBe(true);
  });

  it("'de' matches /reject via 'deny' alias on data-view", () => {
    const filtered = filterCommands("de", "data-view");
    expect(filtered.some((c) => c.name === "reject")).toBe(true);
  });
});
