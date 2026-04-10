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

describe("graphology-adapter", () => {
  describe("syncStoreToGraph", () => {
    it("should add nodes with correct attributes", () => {
      const graph = new Graph();
      const entities = [
        makeEntity("c360.ops.robotics.gcs.drone.001"),
        makeEntity("c360.ops.robotics.gcs.fleet.west"),
      ];

      syncStoreToGraph(graph, entities, []);

      expect(graph.order).toBe(2);
      expect(graph.hasNode("c360.ops.robotics.gcs.drone.001")).toBe(true);
      expect(graph.hasNode("c360.ops.robotics.gcs.fleet.west")).toBe(true);

      const attrs = graph.getNodeAttributes("c360.ops.robotics.gcs.drone.001");
      expect(attrs.label).toBe("001");
      expect(attrs.entityType).toBe("drone");
      expect(attrs.domain).toBe("robotics");
      expect(typeof attrs.color).toBe("string");
      expect(typeof attrs.size).toBe("number");
      expect(typeof attrs.x).toBe("number");
      expect(typeof attrs.y).toBe("number");
    });

    it("should add edges with correct attributes", () => {
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

      expect(graph.size).toBe(1);
      const edgeAttrs = graph.getEdgeAttributes(rel.id);
      expect(edgeAttrs.label).toBe("current");
      expect(edgeAttrs.type).toBe("arrow");
      expect(typeof edgeAttrs.color).toBe("string");
    });

    it("should clear previous data on sync", () => {
      const graph = new Graph();
      const entities1 = [makeEntity("c360.ops.robotics.gcs.drone.001")];
      syncStoreToGraph(graph, entities1, []);
      expect(graph.order).toBe(1);

      const entities2 = [makeEntity("c360.ops.robotics.gcs.fleet.west")];
      syncStoreToGraph(graph, entities2, []);
      expect(graph.order).toBe(1);
      expect(graph.hasNode("c360.ops.robotics.gcs.fleet.west")).toBe(true);
      expect(graph.hasNode("c360.ops.robotics.gcs.drone.001")).toBe(false);
    });

    it("should preserve existing node positions on re-sync", () => {
      const graph = new Graph();
      const entity = makeEntity("c360.ops.robotics.gcs.drone.001");
      syncStoreToGraph(graph, [entity], []);

      // Set a known position (simulating FA2 output)
      graph.setNodeAttribute(entity.id, "x", 42);
      graph.setNodeAttribute(entity.id, "y", 99);

      // Re-sync with same entity
      syncStoreToGraph(graph, [entity], []);

      expect(graph.getNodeAttribute(entity.id, "x")).toBe(42);
      expect(graph.getNodeAttribute(entity.id, "y")).toBe(99);
    });

    it("should assign random positions for new nodes during re-sync", () => {
      const graph = new Graph();
      const entity1 = makeEntity("c360.ops.robotics.gcs.drone.001");
      syncStoreToGraph(graph, [entity1], []);

      const entity2 = makeEntity("c360.ops.robotics.gcs.fleet.west");
      syncStoreToGraph(graph, [entity1, entity2], []);

      // entity2 is new — should have a position (random, but present)
      expect(typeof graph.getNodeAttribute(entity2.id, "x")).toBe("number");
      expect(typeof graph.getNodeAttribute(entity2.id, "y")).toBe("number");
    });

    it("should skip edges where source or target node is missing", () => {
      const graph = new Graph();
      const rel = makeRelationship(
        "c360.ops.robotics.gcs.drone.001",
        "fleet.membership.current",
        "c360.ops.robotics.gcs.fleet.missing",
      );
      const entities = [makeEntity("c360.ops.robotics.gcs.drone.001")];

      syncStoreToGraph(graph, entities, [rel]);

      expect(graph.order).toBe(1);
      expect(graph.size).toBe(0);
    });

    it("should handle empty inputs", () => {
      const graph = new Graph();
      syncStoreToGraph(graph, [], []);
      expect(graph.order).toBe(0);
      expect(graph.size).toBe(0);
    });

    it("should scale node size with connections", () => {
      const graph = new Graph();
      const rel1 = makeRelationship(
        "c360.ops.robotics.gcs.drone.001",
        "a.b.c",
        "c360.ops.robotics.gcs.fleet.west",
      );
      const rel2 = makeRelationship(
        "c360.ops.robotics.gcs.drone.001",
        "x.y.z",
        "c360.ops.robotics.gcs.fleet.west",
      );
      const connected = makeEntity(
        "c360.ops.robotics.gcs.drone.001",
        [rel1, rel2],
        [],
      );
      const isolated = makeEntity("c360.ops.robotics.gcs.fleet.west", [], []);

      syncStoreToGraph(graph, [connected, isolated], []);

      const connectedSize = graph.getNodeAttribute(connected.id, "size");
      const isolatedSize = graph.getNodeAttribute(isolated.id, "size");
      expect(connectedSize).toBeGreaterThan(isolatedSize);
    });
  });

  describe("addToGraph", () => {
    it("should add new nodes without removing existing ones", () => {
      const graph = new Graph();
      const entities1 = [makeEntity("c360.ops.robotics.gcs.drone.001")];
      syncStoreToGraph(graph, entities1, []);

      const entities2 = [makeEntity("c360.ops.robotics.gcs.fleet.west")];
      addToGraph(graph, entities2, []);

      expect(graph.order).toBe(2);
      expect(graph.hasNode("c360.ops.robotics.gcs.drone.001")).toBe(true);
      expect(graph.hasNode("c360.ops.robotics.gcs.fleet.west")).toBe(true);
    });

    it("should not duplicate existing nodes", () => {
      const graph = new Graph();
      const entity = makeEntity("c360.ops.robotics.gcs.drone.001");
      syncStoreToGraph(graph, [entity], []);

      addToGraph(graph, [entity], []);

      expect(graph.order).toBe(1);
    });

    it("should not duplicate existing edges", () => {
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

      addToGraph(graph, [], [rel]);

      expect(graph.size).toBe(1);
    });

    it("should position new nodes near existing neighbors", () => {
      const graph = new Graph();
      const existingEntity = makeEntity("c360.ops.robotics.gcs.drone.001");
      syncStoreToGraph(graph, [existingEntity], []);

      // Set known position for existing node
      graph.setNodeAttribute(existingEntity.id, "x", 50);
      graph.setNodeAttribute(existingEntity.id, "y", 50);

      const rel = makeRelationship(
        "c360.ops.robotics.gcs.fleet.west",
        "has.member",
        "c360.ops.robotics.gcs.drone.001",
      );
      const newEntity = makeEntity(
        "c360.ops.robotics.gcs.fleet.west",
        [rel],
        [],
      );
      addToGraph(graph, [newEntity], [rel]);

      const x = graph.getNodeAttribute(newEntity.id, "x");
      const y = graph.getNodeAttribute(newEntity.id, "y");
      // Should be near 50,50 (within jitter range of 20)
      expect(x).toBeGreaterThan(30);
      expect(x).toBeLessThan(70);
      expect(y).toBeGreaterThan(30);
      expect(y).toBeLessThan(70);
    });
  });
});
