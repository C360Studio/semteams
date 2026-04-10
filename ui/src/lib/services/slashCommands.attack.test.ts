// Attack tests by Reviewer Agent.
// These test adversarial inputs and edge cases.

import { describe, it, expect } from "vitest";
import {
  parseSlashCommand,
  filterCommands,
  getCommandsForPage,
} from "./slashCommands";

// ---------------------------------------------------------------------------
// Slash command injection — adversarial tokens
// ---------------------------------------------------------------------------

describe("parseSlashCommand — injection attacks", () => {
  it('ignores SQL-injection-style token: /search"; DROP TABLE--', () => {
    // The token after "/" is "search\";" — no command named that exists
    // Actually the token is everything up to the first space.
    // /search"; DROP TABLE-- → token = 'search";', not a registered name
    const result = parseSlashCommand('/search"; DROP TABLE--', "flow-builder");
    // 'search";' is not a registered command name so this should return null
    expect(result).toBeNull();
  });

  it("handles a slash followed only by special characters", () => {
    expect(() =>
      parseSlashCommand("/!@#$%^&*()", "flow-builder"),
    ).not.toThrow();
    expect(parseSlashCommand("/!@#$%^&*()", "flow-builder")).toBeNull();
  });

  it("handles a slash followed by a newline", () => {
    expect(() => parseSlashCommand("/\n", "flow-builder")).not.toThrow();
    expect(parseSlashCommand("/\n", "flow-builder")).toBeNull();
  });

  it("handles a slash followed by a tab character", () => {
    // '\tsearch' is not a command name
    expect(() => parseSlashCommand("/\tsearch", "flow-builder")).not.toThrow();
    expect(parseSlashCommand("/\tsearch", "flow-builder")).toBeNull();
  });

  it("handles null-byte in input without throwing", () => {
    expect(() =>
      parseSlashCommand("/search\x00malicious", "flow-builder"),
    ).not.toThrow();
  });

  it("handles input that is only a slash character", () => {
    expect(() => parseSlashCommand("/", "flow-builder")).not.toThrow();
    // "/" with no token — withoutSlash is "", no command matches
    expect(parseSlashCommand("/", "flow-builder")).toBeNull();
  });

  it("handles input that is slash followed by a space only", () => {
    expect(() => parseSlashCommand("/ ", "flow-builder")).not.toThrow();
    expect(parseSlashCommand("/ ", "flow-builder")).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// Malformed slash commands — partial, unicode, very long
// ---------------------------------------------------------------------------

describe("parseSlashCommand — malformed inputs", () => {
  it("returns null for empty string", () => {
    expect(parseSlashCommand("", "flow-builder")).toBeNull();
  });

  it("returns null for whitespace-only string", () => {
    expect(parseSlashCommand("   \t\n  ", "flow-builder")).toBeNull();
  });

  it("handles unicode command token without throwing", () => {
    expect(() =>
      parseSlashCommand("/séarch drones", "flow-builder"),
    ).not.toThrow();
    // 'séarch' is not a registered command
    expect(parseSlashCommand("/séarch drones", "flow-builder")).toBeNull();
  });

  it("handles emoji in command token without throwing", () => {
    expect(() => parseSlashCommand("/🔍 drones", "flow-builder")).not.toThrow();
    expect(parseSlashCommand("/🔍 drones", "flow-builder")).toBeNull();
  });

  it("handles a very long command token (10k chars) without throwing", () => {
    const longToken = "/".concat("a".repeat(10000));
    expect(() => parseSlashCommand(longToken, "flow-builder")).not.toThrow();
    expect(parseSlashCommand(longToken, "flow-builder")).toBeNull();
  });

  it("handles a very long args string without throwing", () => {
    const longArgs = "/search " + "x".repeat(100000);
    expect(() => parseSlashCommand(longArgs, "flow-builder")).not.toThrow();
    const result = parseSlashCommand(longArgs, "flow-builder");
    expect(result).not.toBeNull();
    expect(result!.command.name).toBe("search");
    // The args are preserved verbatim — no truncation applied
    expect(result!.result.content).toHaveLength(100000);
  });

  it("handles multiple slashes in sequence", () => {
    expect(() =>
      parseSlashCommand("//search drones", "flow-builder"),
    ).not.toThrow();
    // token is "/search" — not a registered name
    expect(parseSlashCommand("//search drones", "flow-builder")).toBeNull();
  });

  it("does not match when command name has embedded hyphen: /search-extra", () => {
    expect(
      parseSlashCommand("/search-extra drones", "flow-builder"),
    ).toBeNull();
  });

  it("preserves unicode args content correctly", () => {
    const result = parseSlashCommand("/search 드론 구역 7", "flow-builder");
    expect(result).not.toBeNull();
    expect(result!.result.content).toBe("드론 구역 7");
  });
});

// ---------------------------------------------------------------------------
// filterCommands — regex-special characters in partial
// ---------------------------------------------------------------------------

describe("filterCommands — regex-special characters in partial", () => {
  it("handles '.' in partial without treating it as regex wildcard", () => {
    // '.' would match everything if used as regex; filterCommands uses startsWith
    const result = filterCommands(".", "flow-builder");
    expect(result).toHaveLength(0);
  });

  it("handles '*' in partial without throwing", () => {
    expect(() => filterCommands("*", "flow-builder")).not.toThrow();
    expect(filterCommands("*", "flow-builder")).toHaveLength(0);
  });

  it("handles '(' in partial without throwing", () => {
    expect(() => filterCommands("(", "flow-builder")).not.toThrow();
    expect(filterCommands("(", "flow-builder")).toHaveLength(0);
  });

  it("handles '[' in partial without throwing", () => {
    expect(() => filterCommands("[", "flow-builder")).not.toThrow();
    expect(filterCommands("[", "flow-builder")).toHaveLength(0);
  });

  it("handles '^' in partial without throwing", () => {
    expect(() => filterCommands("^", "flow-builder")).not.toThrow();
    expect(filterCommands("^", "flow-builder")).toHaveLength(0);
  });

  it("handles '$' in partial without throwing", () => {
    expect(() => filterCommands("$", "flow-builder")).not.toThrow();
    expect(filterCommands("$", "flow-builder")).toHaveLength(0);
  });

  it("handles '+' in partial without throwing", () => {
    expect(() => filterCommands("+", "flow-builder")).not.toThrow();
    expect(filterCommands("+", "flow-builder")).toHaveLength(0);
  });

  it("handles '\\' in partial without throwing", () => {
    expect(() => filterCommands("\\", "flow-builder")).not.toThrow();
    expect(filterCommands("\\", "flow-builder")).toHaveLength(0);
  });

  it("handles very long partial string without throwing", () => {
    const longPartial = "s".repeat(10000);
    expect(() => filterCommands(longPartial, "flow-builder")).not.toThrow();
    // "ssss..." doesn't startsWith any command name since names are short
    expect(filterCommands(longPartial, "flow-builder")).toHaveLength(0);
  });

  it("handles emoji in partial without throwing", () => {
    expect(() => filterCommands("🔍", "flow-builder")).not.toThrow();
    expect(filterCommands("🔍", "flow-builder")).toHaveLength(0);
  });
});

// ---------------------------------------------------------------------------
// getCommandsForPage — registry invariants
// ---------------------------------------------------------------------------

describe("getCommandsForPage — registry invariants", () => {
  it("every command has a non-empty name", () => {
    const all = [
      ...getCommandsForPage("flow-builder"),
      ...getCommandsForPage("data-view"),
    ];
    for (const cmd of all) {
      expect(cmd.name.length).toBeGreaterThan(0);
    }
  });

  it("every command name is unique in the registry", () => {
    const fb = getCommandsForPage("flow-builder");
    const names = fb.map((c) => c.name);
    const unique = new Set(names);
    expect(unique.size).toBe(names.length);
  });

  it("every command has a parse function that returns a valid result without throwing", () => {
    const all = [
      ...getCommandsForPage("flow-builder"),
      ...getCommandsForPage("data-view"),
    ];
    const seen = new Set<string>();
    for (const cmd of all) {
      if (seen.has(cmd.name)) continue;
      seen.add(cmd.name);
      expect(() => cmd.parse("")).not.toThrow();
      const result = cmd.parse("");
      expect(result.intent).toBeTruthy();
      expect(typeof result.content).toBe("string");
      expect(typeof result.params).toBe("object");
    }
  });

  it("every command has a parse function that handles adversarial args without throwing", () => {
    const all = getCommandsForPage("flow-builder");
    const adversarialArgs = [
      "'; DROP TABLE users; --",
      "<script>alert('xss')</script>",
      "\x00\x01\x02",
      "a".repeat(100000),
      "\n\r\t",
    ];
    for (const cmd of all) {
      for (const args of adversarialArgs) {
        expect(() => cmd.parse(args)).not.toThrow();
      }
    }
  });
});
