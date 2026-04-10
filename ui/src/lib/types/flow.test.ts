import { describe, it, expect } from "vitest";
import type {
  Flow,
  FlowMetadata,
  RuntimeState,
  SaveStatus,
  ComponentInstance,
  Position,
  ComponentHealth,
  HealthStatus,
  Connection,
  InteractionPattern,
} from "./flow";
import type { ValidationState } from "./port";

describe("Flow Type Definitions", () => {
  describe("Flow interface", () => {
    it("should match backend schema structure", () => {
      const flow: Flow = {
        version: 1,
        id: "550e8400-e29b-41d4-a716-446655440000",
        name: "Test Pipeline",
        description: "Test description",
        nodes: [],
        connections: [],
        runtime_state: "not_deployed",
        created_at: "2025-10-10T12:00:00Z",
        updated_at: "2025-10-10T14:30:00Z",
        created_by: "user-123",
        last_modified: "2025-10-10T14:30:00Z",
      };

      expect(flow.version).toBe(1);
      expect(flow.id).toBe("550e8400-e29b-41d4-a716-446655440000");
      expect(flow.name).toBe("Test Pipeline");
      expect(flow.nodes).toEqual([]);
      expect(flow.connections).toEqual([]);
      expect(flow.runtime_state).toBe("not_deployed");
    });

    it("should allow optional deployed_at field", () => {
      const flow: Flow = {
        version: 1,
        id: "550e8400-e29b-41d4-a716-446655440000",
        name: "Test Pipeline",
        nodes: [],
        connections: [],
        runtime_state: "deployed_stopped",
        deployed_at: "2025-10-10T15:00:00Z",
        created_at: "2025-10-10T12:00:00Z",
        updated_at: "2025-10-10T14:30:00Z",
        last_modified: "2025-10-10T14:30:00Z",
      };

      expect(flow.deployed_at).toBe("2025-10-10T15:00:00Z");
    });
  });

  describe("RuntimeState enum", () => {
    it("should have correct enum values", () => {
      const states: RuntimeState[] = [
        "not_deployed",
        "deployed_stopped",
        "running",
        "error",
      ];

      states.forEach((state) => {
        const flow: Flow = {
          version: 1,
          id: "test-flow",
          name: "Test Flow",
          nodes: [],
          connections: [],
          runtime_state: state,
          created_at: "2025-10-10T12:00:00Z",
          updated_at: "2025-10-10T12:00:00Z",
          last_modified: "2025-10-10T12:00:00Z",
        };

        expect(flow.runtime_state).toBe(state);
      });
    });
  });

  describe("SaveStatus enum", () => {
    it("should have correct enum values", () => {
      const statuses: SaveStatus[] = ["clean", "dirty", "saving", "error"];

      statuses.forEach((status) => {
        const metadata: FlowMetadata = {
          saveStatus: status,
        };

        expect(metadata.saveStatus).toBe(status);
      });
    });
  });

  describe("ComponentInstance interface", () => {
    it("should match backend schema", () => {
      const component: ComponentInstance = {
        id: "comp-1",
        component: "udp-input",
        type: "input",
        name: "UDP Input 1",
        position: { x: 100, y: 200 },
        config: { port: 14550, host: "0.0.0.0" },
        health: {
          status: "not_running",
          lastUpdated: "2025-10-10T12:00:00Z",
        },
      };

      expect(component.id).toBe("comp-1");
      expect(component.component).toBe("udp-input");
      expect(component.name).toBe("UDP Input 1");
      expect(component.position.x).toBe(100);
      expect(component.position.y).toBe(200);
      expect(component.config.port).toBe(14550);
    });
  });

  describe("Position interface", () => {
    it("should store canvas coordinates", () => {
      const position: Position = { x: 150.5, y: 220.8 };

      expect(position.x).toBe(150.5);
      expect(position.y).toBe(220.8);
    });
  });

  describe("ComponentHealth interface", () => {
    it("should support all health statuses", () => {
      const statuses: HealthStatus[] = [
        "healthy",
        "degraded",
        "unhealthy",
        "not_running",
      ];

      statuses.forEach((status) => {
        const health: ComponentHealth = {
          status,
          lastUpdated: "2025-10-10T12:00:00Z",
        };

        expect(health.status).toBe(status);
      });
    });

    it("should allow optional error message", () => {
      const health: ComponentHealth = {
        status: "unhealthy",
        errorMessage: "Connection timeout",
        lastUpdated: "2025-10-10T12:00:00Z",
      };

      expect(health.errorMessage).toBe("Connection timeout");
    });
  });

  describe("Connection interface", () => {
    it("should match backend schema", () => {
      const connection: Connection = {
        id: "conn-1",
        source_node_id: "comp-1",
        source_port: "output",
        target_node_id: "comp-2",
        target_port: "input",
        pattern: "stream",
        validationState: "valid",
      };

      expect(connection.id).toBe("conn-1");
      expect(connection.source_node_id).toBe("comp-1");
      expect(connection.target_node_id).toBe("comp-2");
      expect(connection.pattern).toBe("stream");
      expect(connection.validationState).toBe("valid");
    });

    it("should allow optional validation error", () => {
      const connection: Connection = {
        id: "conn-1",
        source_node_id: "comp-1",
        source_port: "output",
        target_node_id: "comp-2",
        target_port: "input",
        pattern: "stream",
        validationState: "error",
        validationError: "Incompatible port types",
      };

      expect(connection.validationError).toBe("Incompatible port types");
    });

    it("should allow optional connection metrics", () => {
      const connection: Connection = {
        id: "conn-1",
        source_node_id: "comp-1",
        source_port: "output",
        target_node_id: "comp-2",
        target_port: "input",
        pattern: "stream",
        validationState: "valid",
        metrics: {
          messageCount: 1500,
          throughputRate: 150.5,
          lastActivity: "2025-10-10T12:05:00Z",
        },
      };

      expect(connection.metrics?.messageCount).toBe(1500);
      expect(connection.metrics?.throughputRate).toBe(150.5);
    });
  });

  describe("InteractionPattern enum", () => {
    it("should have correct enum values", () => {
      const patterns: InteractionPattern[] = [
        "stream",
        "request",
        "watch",
        "network",
      ];

      patterns.forEach((pattern) => {
        const connection: Connection = {
          id: "conn-1",
          source_node_id: "comp-1",
          source_port: "output",
          target_node_id: "comp-2",
          target_port: "input",
          pattern,
          validationState: "valid",
        };

        expect(connection.pattern).toBe(pattern);
      });
    });
  });

  describe("ValidationState enum", () => {
    it("should have correct enum values", () => {
      const states: ValidationState[] = [
        "valid",
        "error",
        "warning",
        "unknown",
      ];

      states.forEach((state) => {
        const connection: Connection = {
          id: "conn-1",
          source_node_id: "comp-1",
          source_port: "output",
          target_node_id: "comp-2",
          target_port: "input",
          pattern: "stream",
          validationState: state,
        };

        expect(connection.validationState).toBe(state);
      });
    });
  });
});
