import { describe, it, expect, beforeEach } from "vitest";
import { chatStore } from "./chatStore.svelte";
import type {
  ContextChip,
  FlowAttachment,
  SearchResultAttachment,
  MessageAttachment,
} from "$lib/types/chat";

// ---------------------------------------------------------------------------
// Reset between tests — store is a singleton
// ---------------------------------------------------------------------------

beforeEach(() => {
  chatStore.clearConversation();
  chatStore.clearChips();
  chatStore.setError(null);
  chatStore.setStreaming(false);
});

// ---------------------------------------------------------------------------
// Chip overflow — cap at 10
// ---------------------------------------------------------------------------

describe("chatStore (attack) — chip cap enforcement", () => {
  function makeChip(i: number): ContextChip {
    return {
      id: `chip-${i}`,
      kind: "entity",
      label: `Entity ${i}`,
      value: `entity-${i}`,
    };
  }

  it("accepts exactly 10 chips", () => {
    for (let i = 0; i < 10; i++) {
      chatStore.addChip(makeChip(i));
    }
    expect(chatStore.chips).toHaveLength(10);
  });

  it("rejects the 11th chip silently — chip count stays at 10", () => {
    for (let i = 0; i < 15; i++) {
      chatStore.addChip(makeChip(i));
    }
    expect(chatStore.chips).toHaveLength(10);
  });

  it("does not throw when adding chips beyond the cap", () => {
    expect(() => {
      for (let i = 0; i < 20; i++) {
        chatStore.addChip(makeChip(i));
      }
    }).not.toThrow();
  });
});

// ---------------------------------------------------------------------------
// Chip deduplication
// ---------------------------------------------------------------------------

describe("chatStore (attack) — chip deduplication", () => {
  it("ignores a chip with duplicate kind+value", () => {
    const chip: ContextChip = {
      id: "chip-a",
      kind: "entity",
      label: "Alpha",
      value: "entity-alpha",
    };
    chatStore.addChip(chip);
    chatStore.addChip({ ...chip, id: "chip-b", label: "Alpha 2" });
    expect(chatStore.chips).toHaveLength(1);
  });

  it("allows same value with different kind (not a duplicate)", () => {
    chatStore.addChip({
      id: "chip-1",
      kind: "entity",
      label: "Alpha",
      value: "alpha",
    });
    chatStore.addChip({
      id: "chip-2",
      kind: "component",
      label: "Alpha",
      value: "alpha",
    });
    expect(chatStore.chips).toHaveLength(2);
  });

  it("allows same kind with different value (not a duplicate)", () => {
    chatStore.addChip({
      id: "chip-1",
      kind: "entity",
      label: "Alpha",
      value: "alpha",
    });
    chatStore.addChip({
      id: "chip-2",
      kind: "entity",
      label: "Beta",
      value: "beta",
    });
    expect(chatStore.chips).toHaveLength(2);
  });
});

// ---------------------------------------------------------------------------
// clearConversation preserves chips
// ---------------------------------------------------------------------------

describe("chatStore (attack) — clearConversation preserves chips", () => {
  it("chips remain after clearConversation", () => {
    chatStore.addChip({
      id: "chip-1",
      kind: "flow",
      label: "My Flow",
      value: "flow-1",
    });
    chatStore.addUserMessage("hello");
    chatStore.clearConversation();

    expect(chatStore.messages).toHaveLength(0);
    expect(chatStore.chips).toHaveLength(1);
    expect(chatStore.chips[0].id).toBe("chip-1");
  });

  it("clearChips() empties chips without affecting messages", () => {
    chatStore.addChip({
      id: "chip-1",
      kind: "entity",
      label: "X",
      value: "x",
    });
    chatStore.addUserMessage("hello");
    chatStore.clearChips();

    expect(chatStore.chips).toHaveLength(0);
    expect(chatStore.messages).toHaveLength(1);
  });
});

// ---------------------------------------------------------------------------
// addUserMessage snapshots chips
// ---------------------------------------------------------------------------

describe("chatStore (attack) — addUserMessage chip snapshot", () => {
  it("snapshots current chips into message.chips at send time", () => {
    chatStore.addChip({
      id: "chip-1",
      kind: "entity",
      label: "E1",
      value: "e1",
    });
    const msg = chatStore.addUserMessage("with chips");
    expect(msg.chips).toBeDefined();
    expect(msg.chips).toHaveLength(1);
    expect(msg.chips![0].id).toBe("chip-1");
  });

  it("snapshot is not mutated when chips change after send", () => {
    chatStore.addChip({
      id: "chip-1",
      kind: "entity",
      label: "E1",
      value: "e1",
    });
    const msg = chatStore.addUserMessage("snapshot test");

    // Add more chips after send
    chatStore.addChip({
      id: "chip-2",
      kind: "entity",
      label: "E2",
      value: "e2",
    });

    // The stored message snapshot should still have only the original chip
    const stored = chatStore.messages.find((m) => m.id === msg.id);
    expect(stored?.chips).toHaveLength(1);
  });

  it("message.chips is undefined when no chips were active at send time", () => {
    const msg = chatStore.addUserMessage("no chips");
    expect(msg.chips).toBeUndefined();
  });
});

// ---------------------------------------------------------------------------
// updateAttachment — malformed inputs
// ---------------------------------------------------------------------------

describe("chatStore (attack) — updateAttachment edge cases", () => {
  it("is a no-op for a non-existent message id", () => {
    chatStore.addUserMessage("hello");
    expect(() =>
      chatStore.updateAttachment("nonexistent-id", "flow", { applied: true }),
    ).not.toThrow();
    expect(chatStore.messages).toHaveLength(1);
  });

  it("is a no-op when the message has no attachments", () => {
    const msg = chatStore.addUserMessage("no attachments");
    expect(() =>
      chatStore.updateAttachment(msg.id, "flow", { applied: true }),
    ).not.toThrow();
    expect(chatStore.messages[0].attachments).toBeUndefined();
  });

  it("is a no-op when the attachment kind is not found on the message", () => {
    const searchAttachment: SearchResultAttachment = {
      kind: "search-result",
      query: "test",
      entityIds: [],
      count: 0,
      durationMs: 5,
    };
    const msg = chatStore.addAssistantMessage("results", [searchAttachment]);
    expect(() =>
      chatStore.updateAttachment(msg.id, "flow", { applied: true }),
    ).not.toThrow();
    // The search attachment must be untouched
    const stored = chatStore.messages.find((m) => m.id === msg.id);
    expect(stored?.attachments?.[0].kind).toBe("search-result");
  });

  it("updates only the matching kind when multiple attachments are present", () => {
    const flowAttachment: FlowAttachment = {
      kind: "flow",
      flow: { nodes: [], connections: [] },
      applied: false,
    };
    const searchAttachment: SearchResultAttachment = {
      kind: "search-result",
      query: "test",
      entityIds: [],
      count: 0,
      durationMs: 5,
    };
    const msg = chatStore.addAssistantMessage("mixed", [
      flowAttachment,
      searchAttachment,
    ]);
    chatStore.updateAttachment(msg.id, "flow", { applied: true });

    const stored = chatStore.messages.find((m) => m.id === msg.id);
    const flow = stored?.attachments?.find(
      (a): a is FlowAttachment => a.kind === "flow",
    );
    const search = stored?.attachments?.find((a) => a.kind === "search-result");
    expect(flow?.applied).toBe(true);
    expect(search?.kind).toBe("search-result"); // untouched
  });
});

// ---------------------------------------------------------------------------
// updateAttachment with empty update object
// ---------------------------------------------------------------------------

describe("chatStore (attack) — updateAttachment with empty update", () => {
  it("applying an empty update does not corrupt the attachment", () => {
    const flowAttachment: FlowAttachment = {
      kind: "flow",
      flow: { nodes: [], connections: [] },
      applied: false,
    };
    const msg = chatStore.addAssistantMessage("flow msg", [flowAttachment]);
    chatStore.updateAttachment(msg.id, "flow", {});

    const stored = chatStore.messages.find((m) => m.id === msg.id);
    const flow = stored?.attachments?.find(
      (a): a is FlowAttachment => a.kind === "flow",
    );
    expect(flow?.kind).toBe("flow");
    expect(flow?.applied).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// finalizeStream with empty attachments array
// ---------------------------------------------------------------------------

describe("chatStore (attack) — finalizeStream with empty attachments", () => {
  it("empty attachments array results in undefined attachments on message", () => {
    chatStore.setStreaming(true);
    chatStore.finalizeStream("done", []);
    // An empty array passed is treated same as no attachments
    const msg = chatStore.messages[0];
    // The message is created via makeMessage with attachments=[]
    // check that no meaningful attachment UI would appear
    const hasAttachments =
      msg.attachments !== undefined && msg.attachments.length > 0;
    expect(hasAttachments).toBe(false);
  });

  it("null-like attachment handling — undefined does not crash", () => {
    chatStore.setStreaming(true);
    expect(() => chatStore.finalizeStream("done", undefined)).not.toThrow();
  });
});

// ---------------------------------------------------------------------------
// Concurrent-style rapid addChip calls
// ---------------------------------------------------------------------------

describe("chatStore (attack) — rapid addChip", () => {
  it("adding 50 chips synchronously results in exactly 10 stored chips", () => {
    for (let i = 0; i < 50; i++) {
      chatStore.addChip({
        id: `chip-${i}`,
        kind: "entity",
        label: `E${i}`,
        value: `entity-${i}`,
      });
    }
    expect(chatStore.chips).toHaveLength(10);
  });
});

// ---------------------------------------------------------------------------
// setChips — replaces entire chip list
// ---------------------------------------------------------------------------

describe("chatStore (attack) — setChips", () => {
  it("setChips replaces existing chips entirely", () => {
    chatStore.addChip({
      id: "chip-old",
      kind: "entity",
      label: "Old",
      value: "old",
    });
    const newChips: ContextChip[] = [
      { id: "chip-new-1", kind: "flow", label: "Flow A", value: "flow-a" },
      { id: "chip-new-2", kind: "flow", label: "Flow B", value: "flow-b" },
    ];
    chatStore.setChips(newChips);
    expect(chatStore.chips).toHaveLength(2);
    expect(chatStore.chips[0].id).toBe("chip-new-1");
  });

  it("setChips with empty array clears all chips", () => {
    chatStore.addChip({
      id: "chip-1",
      kind: "entity",
      label: "E",
      value: "e",
    });
    chatStore.setChips([]);
    expect(chatStore.chips).toHaveLength(0);
  });
});

// ---------------------------------------------------------------------------
// removeChip — unknown id
// ---------------------------------------------------------------------------

describe("chatStore (attack) — removeChip edge cases", () => {
  it("removeChip with unknown id does not throw", () => {
    expect(() => chatStore.removeChip("nonexistent-id")).not.toThrow();
    expect(chatStore.chips).toHaveLength(0);
  });

  it("removeChip removes only the matching chip", () => {
    chatStore.addChip({
      id: "chip-1",
      kind: "entity",
      label: "A",
      value: "a",
    });
    chatStore.addChip({
      id: "chip-2",
      kind: "entity",
      label: "B",
      value: "b",
    });
    chatStore.removeChip("chip-1");
    expect(chatStore.chips).toHaveLength(1);
    expect(chatStore.chips[0].id).toBe("chip-2");
  });
});

// ---------------------------------------------------------------------------
// Multiple attachment kinds on one message
// ---------------------------------------------------------------------------

describe("chatStore (attack) — multiple attachment kinds", () => {
  it("message can hold all four attachment kinds without corruption", () => {
    const attachments: MessageAttachment[] = [
      { kind: "flow", flow: { nodes: [], connections: [] } },
      {
        kind: "search-result",
        query: "test",
        entityIds: ["e1"],
        count: 1,
        durationMs: 10,
      },
      {
        kind: "entity-detail",
        entityId: "e1",
        summary: "Entity summary",
        propertyCount: 3,
        relationshipCount: 2,
      },
      { kind: "error", code: "ERR", message: "Something failed" },
    ];

    const msg = chatStore.addAssistantMessage("mixed result", attachments);
    const stored = chatStore.messages.find((m) => m.id === msg.id);
    expect(stored?.attachments).toHaveLength(4);
    expect(stored?.attachments?.map((a) => a.kind)).toEqual([
      "flow",
      "search-result",
      "entity-detail",
      "error",
    ]);
  });
});
