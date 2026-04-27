import { describe, it, expect } from "vitest";
import {
  entryMentionsLoop,
  classifyEntry,
  type MessageLogEntry,
} from "./messageLoggerApi";

function entry(overrides: Partial<MessageLogEntry> = {}): MessageLogEntry {
  return {
    sequence: 1,
    timestamp: "2026-04-27T10:00:00Z",
    subject: "test.subject",
    message_type: "raw",
    ...overrides,
  };
}

describe("entryMentionsLoop", () => {
  it("matches when the loop_id is in the subject", () => {
    const e = entry({ subject: "agent.request.loop_e0ce4dd7" });
    expect(entryMentionsLoop(e, "loop_e0ce4dd7")).toBe(true);
  });

  it("matches when the loop_id appears in raw_data", () => {
    // Graph mutations carry the loop reference inside the triple
    // payload, not in the NATS subject.
    const e = entry({
      subject: "graph.mutation.triple.add",
      raw_data: {
        triple: {
          subject:
            "c360.coordinator-001.agent.agentic-loop.execution.loop_e0ce4dd7",
          predicate: "agent.loop.has_step",
        },
      },
    });
    expect(entryMentionsLoop(e, "loop_e0ce4dd7")).toBe(true);
  });

  it("returns false for unrelated entries", () => {
    const e = entry({
      subject: "agent.request.loop_other",
      raw_data: { foo: "bar" },
    });
    expect(entryMentionsLoop(e, "loop_e0ce4dd7")).toBe(false);
  });

  it("returns false for empty loop id", () => {
    const e = entry({ subject: "agent.request.loop_e0ce4dd7" });
    expect(entryMentionsLoop(e, "")).toBe(false);
  });

  it("survives unserialisable raw_data without throwing", () => {
    // A circular structure can't be JSON.stringified — the helper
    // catches the error and falls through to "no match" rather than
    // crashing the trace tab.
    const circular: Record<string, unknown> = { name: "loopy" };
    circular.self = circular;
    const e = entry({ subject: "x", raw_data: circular });
    expect(entryMentionsLoop(e, "loop_e0ce4dd7")).toBe(false);
  });
});

describe("classifyEntry", () => {
  const cases: [string, ReturnType<typeof classifyEntry>][] = [
    ["agent.request.loop_x", "llm-request"],
    ["agent.response.loop_x:req:abc", "llm-response"],
    ["tool.execute.foo", "tool-execute"],
    ["tool.result.foo", "tool-result"],
    ["agent.task.loop_x", "lifecycle"],
    ["agent.complete.loop_x", "lifecycle"],
    ["graph.mutation.triple.add", "graph"],
    ["something.else.entirely", "other"],
  ];

  it.each(cases)("classifies %s as %s", (subject, expected) => {
    expect(classifyEntry(entry({ subject }))).toBe(expected);
  });
});
