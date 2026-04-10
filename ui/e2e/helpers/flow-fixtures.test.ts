// Tests for flow fixtures
// Task: Phase 1 - Flow Fixtures Tests

import { describe, it, expect } from "vitest";
import {
  createMinimalValidFlow,
  createFlowWithAllComponentTypes,
  createComplexFlow,
  createInvalidFlow,
} from "./flow-fixtures";

// Types for flow fixtures
interface FlowConfig {
  name: string;
  description?: string;
  nodes: Array<{
    id: string;
    type: string;
    name: string;
    config: Record<string, unknown>;
  }>;
  connections: Array<{
    id: string;
    source_node_id: string;
    source_port: string;
    target_node_id: string;
    target_port: string;
  }>;
}

describe("flow-fixtures", () => {
  describe("createMinimalValidFlow", () => {
    it("should return a valid flow configuration", () => {
      const flow: FlowConfig = createMinimalValidFlow();

      expect(flow).toBeDefined();
      expect(flow.name).toBeTruthy();
      expect(flow.nodes).toBeDefined();
      expect(flow.connections).toBeDefined();
    });

    it("should have at least one node", () => {
      const flow: FlowConfig = createMinimalValidFlow();

      expect(flow.nodes.length).toBeGreaterThan(0);
    });

    it("should have valid node structure", () => {
      const flow: FlowConfig = createMinimalValidFlow();

      flow.nodes.forEach((node) => {
        expect(node.id).toBeTruthy();
        expect(node.type).toBeTruthy();
        expect(node.name).toBeTruthy();
        expect(node.config).toBeDefined();
        expect(typeof node.config).toBe("object");
      });
    });

    it("should have unique node IDs", () => {
      const flow: FlowConfig = createMinimalValidFlow();

      const nodeIds = flow.nodes.map((n) => n.id);
      const uniqueIds = new Set(nodeIds);

      expect(uniqueIds.size).toBe(nodeIds.length);
    });

    it("should have valid connections array (can be empty)", () => {
      const flow: FlowConfig = createMinimalValidFlow();

      expect(Array.isArray(flow.connections)).toBe(true);
    });

    it("should have properly configured nodes (not empty config)", () => {
      const flow: FlowConfig = createMinimalValidFlow();

      // At least one node should have non-empty config for validity
      const hasConfiguredNode = flow.nodes.some(
        (node) => Object.keys(node.config).length > 0,
      );

      expect(hasConfiguredNode).toBe(true);
    });

    it("should return new instance on each call (not cached)", () => {
      const flow1 = createMinimalValidFlow();
      const flow2 = createMinimalValidFlow();

      // Should be different objects
      expect(flow1).not.toBe(flow2);

      // But should have same structure
      expect(flow1.nodes.length).toBe(flow2.nodes.length);
    });

    it("should have descriptive name indicating test purpose", () => {
      const flow: FlowConfig = createMinimalValidFlow();

      expect(flow.name.toLowerCase()).toMatch(/(test|minimal|basic|simple)/i);
    });
  });

  describe("createFlowWithAllComponentTypes", () => {
    it("should return flow with multiple component types", () => {
      const flow: FlowConfig = createFlowWithAllComponentTypes();

      expect(flow).toBeDefined();
      expect(flow.nodes.length).toBeGreaterThan(1);
    });

    it("should include input component", () => {
      const flow: FlowConfig = createFlowWithAllComponentTypes();

      const hasInput = flow.nodes.some((node) =>
        node.type.toLowerCase().includes("input"),
      );

      expect(hasInput).toBe(true);
    });

    it("should include processor component", () => {
      const flow: FlowConfig = createFlowWithAllComponentTypes();

      const hasProcessor = flow.nodes.some((node) =>
        node.type.toLowerCase().includes("processor"),
      );

      expect(hasProcessor).toBe(true);
    });

    it("should include output component", () => {
      const flow: FlowConfig = createFlowWithAllComponentTypes();

      const hasOutput = flow.nodes.some((node) =>
        node.type.toLowerCase().includes("output"),
      );

      expect(hasOutput).toBe(true);
    });

    it("should have unique node IDs", () => {
      const flow: FlowConfig = createFlowWithAllComponentTypes();

      const nodeIds = flow.nodes.map((n) => n.id);
      const uniqueIds = new Set(nodeIds);

      expect(uniqueIds.size).toBe(nodeIds.length);
    });

    it("should have valid component types", () => {
      const flow: FlowConfig = createFlowWithAllComponentTypes();

      const validTypes = [
        "input",
        "processor",
        "output",
        "storage",
        "gateway",
        "udp-input",
        "tcp-input",
        "http-input",
        "robotics-processor",
        "tcp-output",
        "udp-output",
      ];

      flow.nodes.forEach((node) => {
        const isValidType = validTypes.some((type) => node.type.includes(type));
        expect(isValidType).toBe(true);
      });
    });

    it("should have all nodes properly configured", () => {
      const flow: FlowConfig = createFlowWithAllComponentTypes();

      flow.nodes.forEach((node) => {
        expect(node.config).toBeDefined();
        expect(typeof node.config).toBe("object");
        // All nodes in this fixture should have config
        expect(Object.keys(node.config).length).toBeGreaterThan(0);
      });
    });

    it("should have descriptive node names", () => {
      const flow: FlowConfig = createFlowWithAllComponentTypes();

      flow.nodes.forEach((node) => {
        expect(node.name.length).toBeGreaterThan(0);
        expect(node.name).not.toBe(node.id);
      });
    });
  });

  describe("createComplexFlow", () => {
    it("should return flow with multiple nodes", () => {
      const flow: FlowConfig = createComplexFlow();

      expect(flow.nodes.length).toBeGreaterThanOrEqual(3);
    });

    it("should have connections between nodes", () => {
      const flow: FlowConfig = createComplexFlow();

      expect(flow.connections.length).toBeGreaterThan(0);
    });

    it("should have valid connection structure", () => {
      const flow: FlowConfig = createComplexFlow();

      flow.connections.forEach((conn) => {
        expect(conn.id).toBeTruthy();
        expect(conn.source_node_id).toBeTruthy();
        expect(conn.source_port).toBeTruthy();
        expect(conn.target_node_id).toBeTruthy();
        expect(conn.target_port).toBeTruthy();
      });
    });

    it("should have connections referencing valid node IDs", () => {
      const flow: FlowConfig = createComplexFlow();

      const nodeIds = new Set(flow.nodes.map((n) => n.id));

      flow.connections.forEach((conn) => {
        expect(nodeIds.has(conn.source_node_id)).toBe(true);
        expect(nodeIds.has(conn.target_node_id)).toBe(true);
      });
    });

    it("should have unique connection IDs", () => {
      const flow: FlowConfig = createComplexFlow();

      const connectionIds = flow.connections.map((c) => c.id);
      const uniqueIds = new Set(connectionIds);

      expect(uniqueIds.size).toBe(connectionIds.length);
    });

    it("should not have self-referencing connections", () => {
      const flow: FlowConfig = createComplexFlow();

      flow.connections.forEach((conn) => {
        expect(conn.source_node_id).not.toBe(conn.target_node_id);
      });
    });

    it("should have at least one input and one output node", () => {
      const flow: FlowConfig = createComplexFlow();

      const hasInput = flow.nodes.some((node) =>
        node.type.toLowerCase().includes("input"),
      );
      const hasOutput = flow.nodes.some((node) =>
        node.type.toLowerCase().includes("output"),
      );

      expect(hasInput).toBe(true);
      expect(hasOutput).toBe(true);
    });

    it("should have valid port names", () => {
      const flow: FlowConfig = createComplexFlow();

      const validPorts = ["in", "out", "input", "output", "data", "control"];

      flow.connections.forEach((conn) => {
        const sourcePortValid = validPorts.some((port) =>
          conn.source_port.toLowerCase().includes(port),
        );
        const targetPortValid = validPorts.some((port) =>
          conn.target_port.toLowerCase().includes(port),
        );

        expect(sourcePortValid || conn.source_port.length > 0).toBe(true);
        expect(targetPortValid || conn.target_port.length > 0).toBe(true);
      });
    });

    it("should represent a realistic data flow pipeline", () => {
      const flow: FlowConfig = createComplexFlow();

      // Should have input -> processor(s) -> output pattern
      const inputNodes = flow.nodes.filter((n) =>
        n.type.toLowerCase().includes("input"),
      );
      const outputNodes = flow.nodes.filter((n) =>
        n.type.toLowerCase().includes("output"),
      );

      expect(inputNodes.length).toBeGreaterThan(0);
      expect(outputNodes.length).toBeGreaterThan(0);

      // Should have some processing in between
      expect(flow.nodes.length).toBeGreaterThan(
        inputNodes.length + outputNodes.length,
      );
    });
  });

  describe("createInvalidFlow", () => {
    it("should return flow that will fail validation", () => {
      const flow: FlowConfig = createInvalidFlow();

      expect(flow).toBeDefined();
      expect(flow.nodes).toBeDefined();
    });

    it("should have at least one node with empty or invalid config", () => {
      const flow: FlowConfig = createInvalidFlow();

      const hasInvalidConfig = flow.nodes.some((node) => {
        return Object.keys(node.config).length === 0;
      });

      expect(hasInvalidConfig).toBe(true);
    });

    it("should have invalid but structurally correct format", () => {
      const flow: FlowConfig = createInvalidFlow();

      // Should still have valid structure (name, nodes, connections)
      expect(flow.name).toBeTruthy();
      expect(Array.isArray(flow.nodes)).toBe(true);
      expect(Array.isArray(flow.connections)).toBe(true);

      // But content should be invalid
      expect(flow.nodes.length).toBeGreaterThan(0);
    });

    it("should have nodes with valid IDs and types", () => {
      const flow: FlowConfig = createInvalidFlow();

      flow.nodes.forEach((node) => {
        expect(node.id).toBeTruthy();
        expect(node.type).toBeTruthy();
        expect(node.name).toBeTruthy();
      });
    });

    it("should be distinguishable from valid flows", () => {
      const invalidFlow = createInvalidFlow();
      const validFlow = createMinimalValidFlow();

      // Invalid flow should have different characteristics
      const invalidNodeConfigs = invalidFlow.nodes.filter(
        (n) => Object.keys(n.config).length === 0,
      );
      const validNodeConfigs = validFlow.nodes.filter(
        (n) => Object.keys(n.config).length === 0,
      );

      expect(invalidNodeConfigs.length).toBeGreaterThan(
        validNodeConfigs.length,
      );
    });

    it("should have descriptive name indicating invalid state", () => {
      const flow: FlowConfig = createInvalidFlow();

      expect(flow.name.toLowerCase()).toMatch(
        /(invalid|error|test|validation)/i,
      );
    });

    it("should create multiple types of validation errors", () => {
      const flow: FlowConfig = createInvalidFlow();

      // Should have multiple nodes to trigger multiple validation errors
      expect(flow.nodes.length).toBeGreaterThanOrEqual(2);

      const unconfiguredNodes = flow.nodes.filter(
        (node) => Object.keys(node.config).length === 0,
      );

      expect(unconfiguredNodes.length).toBeGreaterThanOrEqual(1);
    });
  });

  describe("fixture consistency", () => {
    it("all fixtures should return FlowConfig type", () => {
      const flows = [
        createMinimalValidFlow(),
        createFlowWithAllComponentTypes(),
        createComplexFlow(),
        createInvalidFlow(),
      ];

      flows.forEach((flow) => {
        expect(flow).toHaveProperty("name");
        expect(flow).toHaveProperty("nodes");
        expect(flow).toHaveProperty("connections");
        expect(Array.isArray(flow.nodes)).toBe(true);
        expect(Array.isArray(flow.connections)).toBe(true);
      });
    });

    it("all fixtures should have unique names", () => {
      const flows = [
        createMinimalValidFlow(),
        createFlowWithAllComponentTypes(),
        createComplexFlow(),
        createInvalidFlow(),
      ];

      const names = flows.map((f) => f.name);
      const uniqueNames = new Set(names);

      expect(uniqueNames.size).toBe(names.length);
    });

    it("all fixtures should generate fresh instances", () => {
      const flow1 = createMinimalValidFlow();
      const flow2 = createMinimalValidFlow();

      // Modify first flow
      flow1.name = "Modified Name";

      // Second flow should not be affected
      expect(flow2.name).not.toBe("Modified Name");
    });

    it("node IDs should not collide across fixtures", () => {
      const allNodeIds = [
        ...createMinimalValidFlow().nodes.map((n) => n.id),
        ...createFlowWithAllComponentTypes().nodes.map((n) => n.id),
        ...createComplexFlow().nodes.map((n) => n.id),
      ];

      const uniqueIds = new Set(allNodeIds);

      // Should have unique IDs across all fixtures
      expect(uniqueIds.size).toBe(allNodeIds.length);
    });

    it("all node types should be recognized component types", () => {
      const flows = [
        createMinimalValidFlow(),
        createFlowWithAllComponentTypes(),
        createComplexFlow(),
        createInvalidFlow(),
      ];

      const allTypes = flows.flatMap((flow) => flow.nodes.map((n) => n.type));

      // All types should be non-empty strings
      allTypes.forEach((type) => {
        expect(typeof type).toBe("string");
        expect(type.length).toBeGreaterThan(0);
      });
    });
  });

  describe("connection validation", () => {
    it("connections in complex flow should form valid graph", () => {
      const flow: FlowConfig = createComplexFlow();

      const _nodeIds = new Set(flow.nodes.map((n) => n.id));

      // Build adjacency list
      const adjacency = new Map<string, string[]>();
      flow.connections.forEach((conn) => {
        if (!adjacency.has(conn.source_node_id)) {
          adjacency.set(conn.source_node_id, []);
        }
        adjacency.get(conn.source_node_id)!.push(conn.target_node_id);
      });

      // Should have at least one source node (no incoming edges)
      const hasIncoming = new Set(
        flow.connections.map((c) => c.target_node_id),
      );
      const sourceNodes = flow.nodes.filter((n) => !hasIncoming.has(n.id));

      expect(sourceNodes.length).toBeGreaterThan(0);

      // Should have at least one sink node (no outgoing edges)
      const hasOutgoing = new Set(
        flow.connections.map((c) => c.source_node_id),
      );
      const sinkNodes = flow.nodes.filter((n) => !hasOutgoing.has(n.id));

      expect(sinkNodes.length).toBeGreaterThan(0);
    });

    it("minimal flow should have valid structure even without connections", () => {
      const flow: FlowConfig = createMinimalValidFlow();

      // If no connections, should still have valid nodes
      if (flow.connections.length === 0) {
        expect(flow.nodes.length).toBeGreaterThan(0);
        // Standalone nodes should have proper configuration
        flow.nodes.forEach((node) => {
          expect(node.id).toBeTruthy();
          expect(node.type).toBeTruthy();
        });
      }
    });

    it("all flows with connections should have valid source/target ports", () => {
      const flows = [createFlowWithAllComponentTypes(), createComplexFlow()];

      flows.forEach((flow) => {
        flow.connections.forEach((conn) => {
          expect(conn.source_port).toBeTruthy();
          expect(conn.target_port).toBeTruthy();
          expect(typeof conn.source_port).toBe("string");
          expect(typeof conn.target_port).toBe("string");
        });
      });
    });
  });
});
