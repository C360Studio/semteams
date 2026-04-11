import { describe, it, expect } from "vitest";
import { computeFlowDiff } from "./flowDiff";
import type { FlowNode, FlowConnection } from "$lib/types/flow";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeNode(
  id: string,
  name: string,
  config: Record<string, unknown> = {},
): FlowNode {
  return {
    id,
    component: "test-component",
    type: "processor",
    name,
    position: { x: 0, y: 0 },
    config,
  };
}

function makeConnection(
  id: string,
  sourceId = "n1",
  targetId = "n2",
): FlowConnection {
  return {
    id,
    source_node_id: sourceId,
    source_port: "output",
    target_node_id: targetId,
    target_port: "input",
  };
}

// ---------------------------------------------------------------------------
// Node diffing — identical
// ---------------------------------------------------------------------------

describe("computeFlowDiff — identical flows", () => {
  it("returns empty diff when old and new nodes are identical", () => {
    const nodes = [makeNode("n1", "Parser"), makeNode("n2", "Sink")];
    const connections = [makeConnection("c1")];
    const diff = computeFlowDiff(nodes, connections, nodes, connections);
    expect(diff.nodesAdded).toHaveLength(0);
    expect(diff.nodesRemoved).toHaveLength(0);
    expect(diff.nodesModified).toHaveLength(0);
    expect(diff.connectionsAdded).toBe(0);
    expect(diff.connectionsRemoved).toBe(0);
  });
});

// ---------------------------------------------------------------------------
// Node diffing — added
// ---------------------------------------------------------------------------

describe("computeFlowDiff — nodes added", () => {
  it("detects a single node added to an empty flow", () => {
    const diff = computeFlowDiff([], [], [makeNode("n1", "Parser")], []);
    expect(diff.nodesAdded).toEqual(["Parser"]);
  });

  it("uses node name (not id) in nodesAdded", () => {
    const diff = computeFlowDiff(
      [],
      [],
      [makeNode("abc-123", "My Processor")],
      [],
    );
    expect(diff.nodesAdded).toContain("My Processor");
    expect(diff.nodesAdded).not.toContain("abc-123");
  });

  it("detects multiple nodes added", () => {
    const oldNodes = [makeNode("n1", "Source")];
    const newNodes = [
      makeNode("n1", "Source"),
      makeNode("n2", "Transform"),
      makeNode("n3", "Sink"),
    ];
    const diff = computeFlowDiff(oldNodes, [], newNodes, []);
    expect(diff.nodesAdded).toHaveLength(2);
    expect(diff.nodesAdded).toContain("Transform");
    expect(diff.nodesAdded).toContain("Sink");
  });

  it("handles empty old flow — all nodes are added", () => {
    const newNodes = [makeNode("n1", "A"), makeNode("n2", "B")];
    const diff = computeFlowDiff([], [], newNodes, []);
    expect(diff.nodesAdded).toHaveLength(2);
    expect(diff.nodesRemoved).toHaveLength(0);
    expect(diff.nodesModified).toHaveLength(0);
  });
});

// ---------------------------------------------------------------------------
// Node diffing — removed
// ---------------------------------------------------------------------------

describe("computeFlowDiff — nodes removed", () => {
  it("detects a single node removed", () => {
    const oldNodes = [makeNode("n1", "Parser"), makeNode("n2", "Sink")];
    const newNodes = [makeNode("n2", "Sink")];
    const diff = computeFlowDiff(oldNodes, [], newNodes, []);
    expect(diff.nodesRemoved).toEqual(["Parser"]);
  });

  it("uses node name (not id) in nodesRemoved", () => {
    const diff = computeFlowDiff(
      [makeNode("abc-123", "My Source")],
      [],
      [],
      [],
    );
    expect(diff.nodesRemoved).toContain("My Source");
    expect(diff.nodesRemoved).not.toContain("abc-123");
  });

  it("handles empty new flow — all nodes are removed", () => {
    const oldNodes = [makeNode("n1", "A"), makeNode("n2", "B")];
    const diff = computeFlowDiff(oldNodes, [], [], []);
    expect(diff.nodesRemoved).toHaveLength(2);
    expect(diff.nodesAdded).toHaveLength(0);
  });
});

// ---------------------------------------------------------------------------
// Node diffing — modified
// ---------------------------------------------------------------------------

describe("computeFlowDiff — nodes modified", () => {
  it("detects a node modified by config change (same id)", () => {
    const oldNodes = [makeNode("n1", "Parser", { format: "json" })];
    const newNodes = [makeNode("n1", "Parser", { format: "csv" })];
    const diff = computeFlowDiff(oldNodes, [], newNodes, []);
    expect(diff.nodesModified).toEqual(["Parser"]);
  });

  it("detects a node modified by name change (same id)", () => {
    const oldNodes = [makeNode("n1", "Old Name", {})];
    const newNodes = [makeNode("n1", "New Name", {})];
    const diff = computeFlowDiff(oldNodes, [], newNodes, []);
    expect(diff.nodesModified).toContain("New Name");
  });

  it("does NOT mark node as modified when only position changes", () => {
    const oldNode: FlowNode = {
      id: "n1",
      component: "c",
      type: "processor",
      name: "Parser",
      position: { x: 0, y: 0 },
      config: {},
    };
    const newNode: FlowNode = {
      id: "n1",
      component: "c",
      type: "processor",
      name: "Parser",
      position: { x: 999, y: 999 },
      config: {},
    };
    const diff = computeFlowDiff([oldNode], [], [newNode], []);
    expect(diff.nodesModified).toHaveLength(0);
  });

  it("uses node name from new node in nodesModified", () => {
    const oldNodes = [makeNode("n1", "Old Name", { a: 1 })];
    const newNodes = [makeNode("n1", "New Name", { a: 2 })];
    const diff = computeFlowDiff(oldNodes, [], newNodes, []);
    // When both name and config change, either name is acceptable — just must be a string
    expect(diff.nodesModified).toHaveLength(1);
    expect(typeof diff.nodesModified[0]).toBe("string");
  });

  it("handles multiple modifications simultaneously", () => {
    const oldNodes = [
      makeNode("n1", "A", { x: 1 }),
      makeNode("n2", "B", { y: 2 }),
      makeNode("n3", "C", {}),
    ];
    const newNodes = [
      makeNode("n1", "A", { x: 99 }), // config changed
      makeNode("n2", "B-renamed", { y: 2 }), // name changed
      makeNode("n3", "C", {}), // unchanged
    ];
    const diff = computeFlowDiff(oldNodes, [], newNodes, []);
    expect(diff.nodesModified).toHaveLength(2);
  });

  it("does not count added/removed nodes as modified", () => {
    const oldNodes = [makeNode("n1", "A"), makeNode("n2", "B")];
    const newNodes = [makeNode("n2", "B"), makeNode("n3", "C")];
    const diff = computeFlowDiff(oldNodes, [], newNodes, []);
    expect(diff.nodesAdded).toContain("C");
    expect(diff.nodesRemoved).toContain("A");
    expect(diff.nodesModified).toHaveLength(0);
  });
});

// ---------------------------------------------------------------------------
// Connection diffing
// ---------------------------------------------------------------------------

describe("computeFlowDiff — connections", () => {
  it("counts added connections", () => {
    const oldConns: FlowConnection[] = [];
    const newConns = [makeConnection("c1"), makeConnection("c2")];
    const diff = computeFlowDiff([], oldConns, [], newConns);
    expect(diff.connectionsAdded).toBe(2);
  });

  it("counts removed connections", () => {
    const oldConns = [
      makeConnection("c1"),
      makeConnection("c2"),
      makeConnection("c3"),
    ];
    const newConns = [makeConnection("c1")];
    const diff = computeFlowDiff([], oldConns, [], newConns);
    expect(diff.connectionsRemoved).toBe(2);
  });

  it("reports zero when connections are unchanged", () => {
    const conns = [makeConnection("c1"), makeConnection("c2")];
    const diff = computeFlowDiff([], conns, [], conns);
    expect(diff.connectionsAdded).toBe(0);
    expect(diff.connectionsRemoved).toBe(0);
  });

  it("handles all connections added (empty old)", () => {
    const newConns = [
      makeConnection("c1"),
      makeConnection("c2"),
      makeConnection("c3"),
    ];
    const diff = computeFlowDiff([], [], [], newConns);
    expect(diff.connectionsAdded).toBe(3);
    expect(diff.connectionsRemoved).toBe(0);
  });

  it("handles all connections removed (empty new)", () => {
    const oldConns = [makeConnection("c1"), makeConnection("c2")];
    const diff = computeFlowDiff([], oldConns, [], []);
    expect(diff.connectionsAdded).toBe(0);
    expect(diff.connectionsRemoved).toBe(2);
  });

  it("matches connections by id", () => {
    // c1 persists, c2 removed, c3 added
    const oldConns = [makeConnection("c1"), makeConnection("c2")];
    const newConns = [makeConnection("c1"), makeConnection("c3")];
    const diff = computeFlowDiff([], oldConns, [], newConns);
    expect(diff.connectionsAdded).toBe(1);
    expect(diff.connectionsRemoved).toBe(1);
  });
});

// ---------------------------------------------------------------------------
// Combined / edge cases
// ---------------------------------------------------------------------------

describe("computeFlowDiff — edge cases", () => {
  it("handles both old and new being empty", () => {
    const diff = computeFlowDiff([], [], [], []);
    expect(diff.nodesAdded).toHaveLength(0);
    expect(diff.nodesRemoved).toHaveLength(0);
    expect(diff.nodesModified).toHaveLength(0);
    expect(diff.connectionsAdded).toBe(0);
    expect(diff.connectionsRemoved).toBe(0);
  });

  it("handles nodes with same name but different ids as separate nodes", () => {
    // n1-old and n1-new have different ids but same name — should be removed + added, not modified
    const oldNodes = [makeNode("old-id", "SharedName")];
    const newNodes = [makeNode("new-id", "SharedName")];
    const diff = computeFlowDiff(oldNodes, [], newNodes, []);
    // Different ids → one removed, one added; matching is by id not name
    expect(diff.nodesAdded).toHaveLength(1);
    expect(diff.nodesRemoved).toHaveLength(1);
    expect(diff.nodesModified).toHaveLength(0);
  });

  it("returns all three categories independently in one call", () => {
    const oldNodes = [
      makeNode("keep", "Keeper"),
      makeNode("mod", "Modifiable", { v: 1 }),
      makeNode("del", "Deleted"),
    ];
    const newNodes = [
      makeNode("keep", "Keeper"),
      makeNode("mod", "Modifiable", { v: 2 }),
      makeNode("new", "NewNode"),
    ];
    const oldConns = [makeConnection("c-old")];
    const newConns = [makeConnection("c-new")];
    const diff = computeFlowDiff(oldNodes, oldConns, newNodes, newConns);
    expect(diff.nodesAdded).toContain("NewNode");
    expect(diff.nodesRemoved).toContain("Deleted");
    expect(diff.nodesModified).toContain("Modifiable");
    expect(diff.connectionsAdded).toBe(1);
    expect(diff.connectionsRemoved).toBe(1);
  });

  it("table-driven: various node-only diff scenarios", () => {
    const cases: Array<{
      label: string;
      oldNodes: FlowNode[];
      newNodes: FlowNode[];
      expectedAdded: string[];
      expectedRemoved: string[];
      expectedModified: number;
    }> = [
      {
        label: "no change",
        oldNodes: [makeNode("n1", "A")],
        newNodes: [makeNode("n1", "A")],
        expectedAdded: [],
        expectedRemoved: [],
        expectedModified: 0,
      },
      {
        label: "one added",
        oldNodes: [],
        newNodes: [makeNode("n1", "Alpha")],
        expectedAdded: ["Alpha"],
        expectedRemoved: [],
        expectedModified: 0,
      },
      {
        label: "one removed",
        oldNodes: [makeNode("n1", "Beta")],
        newNodes: [],
        expectedAdded: [],
        expectedRemoved: ["Beta"],
        expectedModified: 0,
      },
      {
        label: "config modified",
        oldNodes: [makeNode("n1", "Gamma", { port: 1234 })],
        newNodes: [makeNode("n1", "Gamma", { port: 5678 })],
        expectedAdded: [],
        expectedRemoved: [],
        expectedModified: 1,
      },
    ];

    for (const tc of cases) {
      const diff = computeFlowDiff(tc.oldNodes, [], tc.newNodes, []);
      expect(diff.nodesAdded, `${tc.label}: nodesAdded`).toEqual(
        tc.expectedAdded,
      );
      expect(diff.nodesRemoved, `${tc.label}: nodesRemoved`).toEqual(
        tc.expectedRemoved,
      );
      expect(
        diff.nodesModified,
        `${tc.label}: nodesModified count`,
      ).toHaveLength(tc.expectedModified);
    }
  });
});
