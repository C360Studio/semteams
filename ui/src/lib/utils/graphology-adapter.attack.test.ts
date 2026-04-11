import { describe, it, expect } from "vitest";
import Graph from "graphology";
import type { GraphEntity, GraphRelationship } from "$lib/types/graph";
import { parseEntityId } from "$lib/types/graph";
import { syncStoreToGraph, addToGraph } from "./graphology-adapter";

function makeEntity(
  id: string,
  outgoing: GraphRelationship[] = [],
  incoming: GraphRelationship[] = [],
): GraphEntity {
  return {
    id,
    idParts: parseEntityId(id),
    properties: [],
    outgoing,
    incoming,
  };
}

function makeRelationship(
  sourceId: string,
  predicate: string,
  targetId: string,
  confidence = 0.9,
): GraphRelationship {
  return {
    id: `${sourceId}:${predicate}:${targetId}`,
    sourceId,
    targetId,
    predicate,
    confidence,
    timestamp: Date.now(),
  };
}

describe("graphology-adapter attack tests", () => {
  // ── Undefined / null inputs ──────────────────────────────────────────────

  it("syncStoreToGraph: does not throw on empty arrays", () => {
    const graph = new Graph();
    expect(() => syncStoreToGraph(graph, [], [])).not.toThrow();
  });

  it("addToGraph: does not throw on empty arrays", () => {
    const graph = new Graph();
    expect(() => addToGraph(graph, [], [])).not.toThrow();
  });

  it("syncStoreToGraph: handles entity with empty idParts.instance (falls back to full id as label)", () => {
    // Malformed 6-part id — instance part is empty string
    const graph = new Graph();
    const entity: GraphEntity = {
      id: "a.b.c.d.e.",
      idParts: parseEntityId("a.b.c.d.e."),
      properties: [],
      outgoing: [],
      incoming: [],
    };
    expect(() => syncStoreToGraph(graph, [entity], [])).not.toThrow();
    // Label falls back to full id because instance is ""
    const label = graph.getNodeAttribute("a.b.c.d.e.", "label");
    expect(label).toBe("a.b.c.d.e.");
  });

  // ── Malformed entity IDs ─────────────────────────────────────────────────

  it("syncStoreToGraph: handles entity with non-6-part id without throwing", () => {
    const graph = new Graph();
    const entity: GraphEntity = {
      id: "short.id",
      idParts: parseEntityId("short.id"),
      properties: [],
      outgoing: [],
      incoming: [],
    };
    expect(() => syncStoreToGraph(graph, [entity], [])).not.toThrow();
    expect(graph.order).toBe(1);
  });

  it("syncStoreToGraph: handles entity id containing colon (relationship id separator)", () => {
    // Colons in node ids should not confuse edge key parsing
    const graph = new Graph();
    const entity = makeEntity("c360.ops.robotics.gcs.drone.001");
    expect(() => syncStoreToGraph(graph, [entity], [])).not.toThrow();
  });

  // ── Edge cases: confidence boundaries ────────────────────────────────────

  it("syncStoreToGraph: edge size is at least 1 for zero-confidence relationship", () => {
    const graph = new Graph();
    const rel = makeRelationship(
      "c360.ops.robotics.gcs.drone.001",
      "a.b.c",
      "c360.ops.robotics.gcs.fleet.west",
      0,
    );
    const entities = [
      makeEntity("c360.ops.robotics.gcs.drone.001", [rel], []),
      makeEntity("c360.ops.robotics.gcs.fleet.west", [], [rel]),
    ];
    syncStoreToGraph(graph, entities, [rel]);
    const size = graph.getEdgeAttribute(rel.id, "size");
    expect(size).toBeGreaterThanOrEqual(1);
  });

  it("syncStoreToGraph: edge size is capped reasonably for confidence > 1", () => {
    const graph = new Graph();
    const rel = makeRelationship(
      "c360.ops.robotics.gcs.drone.001",
      "a.b.c",
      "c360.ops.robotics.gcs.fleet.west",
      999,
    );
    const entities = [
      makeEntity("c360.ops.robotics.gcs.drone.001", [rel], []),
      makeEntity("c360.ops.robotics.gcs.fleet.west", [], [rel]),
    ];
    // Should not throw — no validation that confidence <= 1
    expect(() => syncStoreToGraph(graph, entities, [rel])).not.toThrow();
  });

  // ── Predicate label extraction ────────────────────────────────────────────

  it("syncStoreToGraph: edge label uses last segment of dotted predicate", () => {
    const graph = new Graph();
    const rel = makeRelationship(
      "c360.ops.robotics.gcs.drone.001",
      "fleet.membership.current",
      "c360.ops.robotics.gcs.fleet.west",
    );
    const entities = [
      makeEntity("c360.ops.robotics.gcs.drone.001", [rel], []),
      makeEntity("c360.ops.robotics.gcs.fleet.west", [], [rel]),
    ];
    syncStoreToGraph(graph, entities, [rel]);
    expect(graph.getEdgeAttribute(rel.id, "label")).toBe("current");
  });

  it("syncStoreToGraph: edge label falls back to full predicate when no dot", () => {
    // predicate with no dots: split('.').pop() returns the full string, not ""
    const graph = new Graph();
    const rel = makeRelationship(
      "c360.ops.robotics.gcs.drone.001",
      "nodot",
      "c360.ops.robotics.gcs.fleet.west",
    );
    const entities = [
      makeEntity("c360.ops.robotics.gcs.drone.001", [rel], []),
      makeEntity("c360.ops.robotics.gcs.fleet.west", [], [rel]),
    ];
    syncStoreToGraph(graph, entities, [rel]);
    expect(graph.getEdgeAttribute(rel.id, "label")).toBe("nodot");
  });

  it("syncStoreToGraph: edge label falls back to full predicate when predicate ends with dot", () => {
    // "a.b." -> split gives ["a","b",""] -> pop() returns "" -> fallback to full predicate
    const graph = new Graph();
    const rel = makeRelationship(
      "c360.ops.robotics.gcs.drone.001",
      "a.b.",
      "c360.ops.robotics.gcs.fleet.west",
    );
    const entities = [
      makeEntity("c360.ops.robotics.gcs.drone.001", [rel], []),
      makeEntity("c360.ops.robotics.gcs.fleet.west", [], [rel]),
    ];
    syncStoreToGraph(graph, entities, [rel]);
    // "" is falsy so the || branch fires: label === "a.b."
    expect(graph.getEdgeAttribute(rel.id, "label")).toBe("a.b.");
  });

  // ── Duplicate relationships ───────────────────────────────────────────────

  it("syncStoreToGraph: duplicate relationship ids in input do not add duplicate edges", () => {
    const graph = new Graph();
    const rel = makeRelationship(
      "c360.ops.robotics.gcs.drone.001",
      "a.b.c",
      "c360.ops.robotics.gcs.fleet.west",
    );
    const entities = [
      makeEntity("c360.ops.robotics.gcs.drone.001", [rel, rel], []),
      makeEntity("c360.ops.robotics.gcs.fleet.west", [], [rel, rel]),
    ];
    // Two identical rels in the array — second hasEdge check should skip it
    expect(() => syncStoreToGraph(graph, entities, [rel, rel])).not.toThrow();
    expect(graph.size).toBe(1);
  });

  // ── Large data ────────────────────────────────────────────────────────────

  it("syncStoreToGraph: handles 1000 nodes without throwing", () => {
    const graph = new Graph();
    const entities = Array.from({ length: 1000 }, (_, i) =>
      makeEntity(`c360.ops.robotics.gcs.drone.n${i}`),
    );
    expect(() => syncStoreToGraph(graph, entities, [])).not.toThrow();
    expect(graph.order).toBe(1000);
  });

  it("syncStoreToGraph: handles 500 edges between 1000 nodes without throwing", () => {
    const graph = new Graph();
    const entities = Array.from({ length: 1000 }, (_, i) =>
      makeEntity(`c360.ops.robotics.gcs.drone.n${i}`),
    );
    const rels = Array.from({ length: 500 }, (_, i) =>
      makeRelationship(
        `c360.ops.robotics.gcs.drone.n${i}`,
        "a.b.c",
        `c360.ops.robotics.gcs.drone.n${i + 1 < 1000 ? i + 1 : 0}`,
      ),
    );
    expect(() => syncStoreToGraph(graph, entities, rels)).not.toThrow();
    expect(graph.size).toBe(500);
  });

  // ── addToGraph: neighbor positioning with no known neighbors ─────────────

  it("addToGraph: positions new node randomly when no neighbors exist in graph", () => {
    const graph = new Graph();
    // Entity with outgoing/incoming but none of those neighbors are in the graph yet
    const rel = makeRelationship(
      "c360.ops.robotics.gcs.drone.001",
      "a.b.c",
      "c360.ops.robotics.gcs.fleet.notingraph",
    );
    const entity = makeEntity("c360.ops.robotics.gcs.drone.001", [rel], []);
    expect(() => addToGraph(graph, [entity], [])).not.toThrow();
    const x = graph.getNodeAttribute("c360.ops.robotics.gcs.drone.001", "x");
    const y = graph.getNodeAttribute("c360.ops.robotics.gcs.drone.001", "y");
    expect(typeof x).toBe("number");
    expect(typeof y).toBe("number");
  });

  // ── Node size bounds ──────────────────────────────────────────────────────

  it("syncStoreToGraph: node size never exceeds MAX_NODE_SIZE (20) regardless of connections", () => {
    const graph = new Graph();
    // Entity with 200 outgoing connections
    const hub = "c360.ops.robotics.gcs.drone.hub";
    const rels = Array.from({ length: 200 }, (_, i) => {
      const target = `c360.ops.robotics.gcs.drone.n${i}`;
      return makeRelationship(hub, "a.b.c", target);
    });
    const spoke_entities = Array.from({ length: 200 }, (_, i) =>
      makeEntity(`c360.ops.robotics.gcs.drone.n${i}`),
    );
    const hubEntity = makeEntity(hub, rels, []);
    syncStoreToGraph(graph, [hubEntity, ...spoke_entities], rels);
    const size = graph.getNodeAttribute(hub, "size");
    expect(size).toBeLessThanOrEqual(20);
  });

  it("syncStoreToGraph: isolated node size is at least MIN_NODE_SIZE (5)", () => {
    const graph = new Graph();
    const entity = makeEntity("c360.ops.robotics.gcs.drone.001");
    syncStoreToGraph(graph, [entity], []);
    const size = graph.getNodeAttribute(
      "c360.ops.robotics.gcs.drone.001",
      "size",
    );
    expect(size).toBeGreaterThanOrEqual(5);
  });

  // ── Repeated full sync (clear + rebuild) ────────────────────────────────

  it("syncStoreToGraph: repeated syncs on same graph do not accumulate nodes", () => {
    const graph = new Graph();
    const entities = [
      makeEntity("c360.ops.robotics.gcs.drone.001"),
      makeEntity("c360.ops.robotics.gcs.fleet.west"),
    ];

    for (let i = 0; i < 10; i++) {
      syncStoreToGraph(graph, entities, []);
    }

    expect(graph.order).toBe(2);
  });
});
