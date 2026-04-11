/**
 * MCP Server Test Suite
 *
 * Tests for the MCP server implementation.
 * Tests tool registration, execution, caching, and flow generation workflow.
 *
 * Following TDD principles:
 * - Test server initialization
 * - Test tool registration
 * - Test tool execution
 * - Test component catalog caching
 * - Test end-to-end flow generation
 * - Test error handling and edge cases
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import type { ComponentType } from "$lib/types/component";
import type { ValidationResult } from "$lib/types/validation";

// Mock MCP Server types
interface MCPServer {
  registerTool: (tool: MCPTool) => void;
  executeTool: (
    name: string,
    input: Record<string, unknown>,
  ) => Promise<unknown>;
  getRegisteredTools: () => MCPTool[];
  clearCache: () => void;
}

interface MCPTool {
  name: string;
  description: string;
  inputSchema: {
    type: string;
    properties: Record<string, unknown>;
    required: string[];
  };
  execute: (input: Record<string, unknown>) => Promise<unknown>;
}

// Mock server implementation
class MockMCPServer implements MCPServer {
  private tools = new Map<string, MCPTool>();
  private cache = new Map<string, unknown>();

  registerTool(tool: MCPTool): void {
    this.tools.set(tool.name, tool);
  }

  async executeTool(
    name: string,
    input: Record<string, unknown>,
  ): Promise<unknown> {
    const tool = this.tools.get(name);
    if (!tool) {
      throw new Error(`Tool not found: ${name}`);
    }
    return await tool.execute(input);
  }

  getRegisteredTools(): MCPTool[] {
    return Array.from(this.tools.values());
  }

  clearCache(): void {
    this.cache.clear();
  }

  // Helper methods for testing
  getCachedValue(key: string): unknown {
    return this.cache.get(key);
  }

  setCachedValue(key: string, value: unknown): void {
    this.cache.set(key, value);
  }
}

describe("MCP Server - Initialization", () => {
  it("should create MCP server instance", () => {
    const server = new MockMCPServer();

    expect(server).toBeDefined();
    expect(server).toBeInstanceOf(MockMCPServer);
  });

  it("should start with no registered tools", () => {
    const server = new MockMCPServer();

    const tools = server.getRegisteredTools();

    expect(tools).toHaveLength(0);
  });

  it("should have empty cache on initialization", () => {
    const server = new MockMCPServer();

    expect(server.getCachedValue("component_catalog")).toBeUndefined();
  });
});

describe("MCP Server - Tool Registration", () => {
  let server: MockMCPServer;

  beforeEach(() => {
    server = new MockMCPServer();
  });

  describe("register get_component_catalog", () => {
    it("should register get_component_catalog tool", () => {
      const tool: MCPTool = {
        name: "get_component_catalog",
        description: "Fetches available component types",
        inputSchema: {
          type: "object",
          properties: {},
          required: [],
        },
        execute: async () => [],
      };

      server.registerTool(tool);

      const tools = server.getRegisteredTools();
      expect(tools).toHaveLength(1);
      expect(tools[0].name).toBe("get_component_catalog");
    });

    it("should have correct tool schema", () => {
      const tool: MCPTool = {
        name: "get_component_catalog",
        description: "Fetches available component types",
        inputSchema: {
          type: "object",
          properties: {},
          required: [],
        },
        execute: async () => [],
      };

      server.registerTool(tool);

      const registeredTool = server.getRegisteredTools()[0];
      expect(registeredTool.inputSchema.type).toBe("object");
      expect(registeredTool.inputSchema.required).toHaveLength(0);
    });
  });

  describe("register validate_flow", () => {
    it("should register validate_flow tool", () => {
      const tool: MCPTool = {
        name: "validate_flow",
        description: "Validates a flow configuration",
        inputSchema: {
          type: "object",
          properties: {
            flowId: { type: "string" },
            flow: { type: "object" },
          },
          required: ["flowId", "flow"],
        },
        execute: async () => ({}),
      };

      server.registerTool(tool);

      const tools = server.getRegisteredTools();
      expect(tools).toHaveLength(1);
      expect(tools[0].name).toBe("validate_flow");
    });

    it("should have correct tool schema", () => {
      const tool: MCPTool = {
        name: "validate_flow",
        description: "Validates a flow configuration",
        inputSchema: {
          type: "object",
          properties: {
            flowId: { type: "string" },
            flow: { type: "object" },
          },
          required: ["flowId", "flow"],
        },
        execute: async () => ({}),
      };

      server.registerTool(tool);

      const registeredTool = server.getRegisteredTools()[0];
      expect(registeredTool.inputSchema.required).toContain("flowId");
      expect(registeredTool.inputSchema.required).toContain("flow");
    });
  });

  describe("register multiple tools", () => {
    it("should register multiple tools", () => {
      const tool1: MCPTool = {
        name: "get_component_catalog",
        description: "Fetches components",
        inputSchema: { type: "object", properties: {}, required: [] },
        execute: async () => [],
      };

      const tool2: MCPTool = {
        name: "validate_flow",
        description: "Validates flow",
        inputSchema: {
          type: "object",
          properties: { flowId: { type: "string" }, flow: { type: "object" } },
          required: ["flowId", "flow"],
        },
        execute: async () => ({}),
      };

      server.registerTool(tool1);
      server.registerTool(tool2);

      const tools = server.getRegisteredTools();
      expect(tools).toHaveLength(2);
      expect(tools.map((t) => t.name)).toContain("get_component_catalog");
      expect(tools.map((t) => t.name)).toContain("validate_flow");
    });

    it("should replace tool with same name", () => {
      const tool1: MCPTool = {
        name: "test_tool",
        description: "First version",
        inputSchema: { type: "object", properties: {}, required: [] },
        execute: async () => "v1",
      };

      const tool2: MCPTool = {
        name: "test_tool",
        description: "Second version",
        inputSchema: { type: "object", properties: {}, required: [] },
        execute: async () => "v2",
      };

      server.registerTool(tool1);
      server.registerTool(tool2);

      const tools = server.getRegisteredTools();
      expect(tools).toHaveLength(1);
      expect(tools[0].description).toBe("Second version");
    });
  });
});

describe("MCP Server - Tool Execution", () => {
  let server: MockMCPServer;
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    server = new MockMCPServer();
    fetchMock = vi.fn();
    global.fetch = fetchMock;
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe("execute get_component_catalog", () => {
    it("should execute get_component_catalog tool", async () => {
      const mockComponents: ComponentType[] = [
        {
          id: "udp-listener",
          name: "UDP Listener",
          type: "input",
          protocol: "udp",
          category: "input",
          description: "Receives UDP packets",
          version: "1.0.0",
        },
      ];

      fetchMock.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => mockComponents,
      });

      const tool: MCPTool = {
        name: "get_component_catalog",
        description: "Fetches components",
        inputSchema: { type: "object", properties: {}, required: [] },
        execute: async () => {
          const response = await fetch(
            "http://localhost:8080/components/types",
          );
          return await response.json();
        },
      };

      server.registerTool(tool);
      const result = await server.executeTool("get_component_catalog", {});

      expect(result).toEqual(mockComponents);
    });

    it("should return component types array", async () => {
      const mockComponents: ComponentType[] = [
        {
          id: "component-1",
          name: "Component 1",
          type: "input",
          protocol: "nats",
          category: "input",
          description: "Test component",
          version: "1.0.0",
        },
        {
          id: "component-2",
          name: "Component 2",
          type: "output",
          protocol: "nats",
          category: "output",
          description: "Test output",
          version: "1.0.0",
        },
      ];

      fetchMock.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => mockComponents,
      });

      const tool: MCPTool = {
        name: "get_component_catalog",
        description: "Fetches components",
        inputSchema: { type: "object", properties: {}, required: [] },
        execute: async () => {
          const response = await fetch(
            "http://localhost:8080/components/types",
          );
          return await response.json();
        },
      };

      server.registerTool(tool);
      const result = (await server.executeTool(
        "get_component_catalog",
        {},
      )) as ComponentType[];

      expect(Array.isArray(result)).toBe(true);
      expect(result).toHaveLength(2);
    });

    it("should handle empty component catalog", async () => {
      fetchMock.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => [],
      });

      const tool: MCPTool = {
        name: "get_component_catalog",
        description: "Fetches components",
        inputSchema: { type: "object", properties: {}, required: [] },
        execute: async () => {
          const response = await fetch(
            "http://localhost:8080/components/types",
          );
          return await response.json();
        },
      };

      server.registerTool(tool);
      const result = await server.executeTool("get_component_catalog", {});

      expect(result).toEqual([]);
    });
  });

  describe("execute validate_flow", () => {
    it("should execute validate_flow tool", async () => {
      const mockValidation: ValidationResult = {
        validation_status: "valid",
        errors: [],
        warnings: [],
      };

      fetchMock.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => mockValidation,
      });

      const tool: MCPTool = {
        name: "validate_flow",
        description: "Validates flow",
        inputSchema: {
          type: "object",
          properties: { flowId: { type: "string" }, flow: { type: "object" } },
          required: ["flowId", "flow"],
        },
        execute: async (input) => {
          const { flowId, flow } = input;
          const response = await fetch(
            `http://localhost:8080/flowbuilder/flows/${flowId}/validate`,
            {
              method: "POST",
              headers: { "Content-Type": "application/json" },
              body: JSON.stringify(flow),
            },
          );
          return await response.json();
        },
      };

      server.registerTool(tool);
      const result = await server.executeTool("validate_flow", {
        flowId: "test-flow",
        flow: {},
      });

      expect(result).toEqual(mockValidation);
    });

    it("should pass flow data to backend", async () => {
      const flowData = {
        id: "test-flow",
        name: "Test Flow",
        version: 1,
        nodes: [],
        connections: [],
        runtime_state: "not_deployed" as const,
        created_at: "2025-01-01T00:00:00Z",
        updated_at: "2025-01-01T00:00:00Z",
        last_modified: "2025-01-01T00:00:00Z",
      };

      const mockValidation: ValidationResult = {
        validation_status: "valid",
        errors: [],
        warnings: [],
      };

      fetchMock.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => mockValidation,
      });

      const tool: MCPTool = {
        name: "validate_flow",
        description: "Validates flow",
        inputSchema: {
          type: "object",
          properties: { flowId: { type: "string" }, flow: { type: "object" } },
          required: ["flowId", "flow"],
        },
        execute: async (input) => {
          const { flowId, flow } = input;
          const response = await fetch(
            `http://localhost:8080/flowbuilder/flows/${flowId}/validate`,
            {
              method: "POST",
              headers: { "Content-Type": "application/json" },
              body: JSON.stringify(flow),
            },
          );
          return await response.json();
        },
      };

      server.registerTool(tool);
      await server.executeTool("validate_flow", {
        flowId: "test-flow",
        flow: flowData,
      });

      expect(fetchMock).toHaveBeenCalledWith(
        "http://localhost:8080/flowbuilder/flows/test-flow/validate",
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify(flowData),
        }),
      );
    });

    it("should return validation errors", async () => {
      const mockValidation: ValidationResult = {
        validation_status: "errors",
        errors: [
          {
            type: "orphaned_port",
            severity: "error",
            component_name: "UDP Input",
            message: "Port has no connections",
          },
        ],
        warnings: [],
      };

      fetchMock.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => mockValidation,
      });

      const tool: MCPTool = {
        name: "validate_flow",
        description: "Validates flow",
        inputSchema: {
          type: "object",
          properties: { flowId: { type: "string" }, flow: { type: "object" } },
          required: ["flowId", "flow"],
        },
        execute: async (input) => {
          const { flowId, flow } = input;
          const response = await fetch(
            `http://localhost:8080/flowbuilder/flows/${flowId}/validate`,
            {
              method: "POST",
              headers: { "Content-Type": "application/json" },
              body: JSON.stringify(flow),
            },
          );
          return await response.json();
        },
      };

      server.registerTool(tool);
      const result = (await server.executeTool("validate_flow", {
        flowId: "test-flow",
        flow: {},
      })) as ValidationResult;

      expect(result.validation_status).toBe("errors");
      expect(result.errors).toHaveLength(1);
    });
  });

  describe("error handling", () => {
    it("should throw error for unknown tool", async () => {
      await expect(server.executeTool("unknown_tool", {})).rejects.toThrow(
        "Tool not found: unknown_tool",
      );
    });

    it("should propagate tool execution errors", async () => {
      const tool: MCPTool = {
        name: "error_tool",
        description: "Tool that throws error",
        inputSchema: { type: "object", properties: {}, required: [] },
        execute: async () => {
          throw new Error("Tool execution failed");
        },
      };

      server.registerTool(tool);

      await expect(server.executeTool("error_tool", {})).rejects.toThrow(
        "Tool execution failed",
      );
    });

    it("should handle missing required parameters", async () => {
      const tool: MCPTool = {
        name: "validate_flow",
        description: "Validates flow",
        inputSchema: {
          type: "object",
          properties: { flowId: { type: "string" }, flow: { type: "object" } },
          required: ["flowId", "flow"],
        },
        execute: async (input) => {
          if (!input.flowId || !input.flow) {
            throw new Error("Missing required parameters");
          }
          return {};
        },
      };

      server.registerTool(tool);

      await expect(server.executeTool("validate_flow", {})).rejects.toThrow(
        "Missing required parameters",
      );
    });
  });
});

describe("MCP Server - Component Catalog Caching", () => {
  let server: MockMCPServer;
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    server = new MockMCPServer();
    fetchMock = vi.fn();
    global.fetch = fetchMock;
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe("cache management", () => {
    it("should cache component catalog on first fetch", async () => {
      const mockComponents: ComponentType[] = [
        {
          id: "component-1",
          name: "Component 1",
          type: "input",
          protocol: "nats",
          category: "input",
          description: "Test",
          version: "1.0.0",
        },
      ];

      fetchMock.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => mockComponents,
      });

      const tool: MCPTool = {
        name: "get_component_catalog",
        description: "Fetches components",
        inputSchema: { type: "object", properties: {}, required: [] },
        execute: async () => {
          const cached = server.getCachedValue("component_catalog");
          if (cached) {
            return cached;
          }

          const response = await fetch(
            "http://localhost:8080/components/types",
          );
          const data = await response.json();
          server.setCachedValue("component_catalog", data);
          return data;
        },
      };

      server.registerTool(tool);
      await server.executeTool("get_component_catalog", {});

      expect(server.getCachedValue("component_catalog")).toEqual(
        mockComponents,
      );
    });

    it("should return cached data on subsequent calls", async () => {
      const mockComponents: ComponentType[] = [
        {
          id: "component-1",
          name: "Component 1",
          type: "input",
          protocol: "nats",
          category: "input",
          description: "Test",
          version: "1.0.0",
        },
      ];

      fetchMock.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => mockComponents,
      });

      const tool: MCPTool = {
        name: "get_component_catalog",
        description: "Fetches components",
        inputSchema: { type: "object", properties: {}, required: [] },
        execute: async () => {
          const cached = server.getCachedValue("component_catalog");
          if (cached) {
            return cached;
          }

          const response = await fetch(
            "http://localhost:8080/components/types",
          );
          const data = await response.json();
          server.setCachedValue("component_catalog", data);
          return data;
        },
      };

      server.registerTool(tool);

      // First call - should fetch
      await server.executeTool("get_component_catalog", {});

      // Second call - should use cache
      await server.executeTool("get_component_catalog", {});

      // Fetch should only be called once
      expect(fetchMock).toHaveBeenCalledTimes(1);
    });

    it("should clear cache when requested", async () => {
      server.setCachedValue("component_catalog", []);

      server.clearCache();

      expect(server.getCachedValue("component_catalog")).toBeUndefined();
    });

    it("should be session-scoped cache", () => {
      const server1 = new MockMCPServer();
      const server2 = new MockMCPServer();

      server1.setCachedValue("component_catalog", ["data1"]);
      server2.setCachedValue("component_catalog", ["data2"]);

      expect(server1.getCachedValue("component_catalog")).toEqual(["data1"]);
      expect(server2.getCachedValue("component_catalog")).toEqual(["data2"]);
    });
  });
});

describe("MCP Server - Flow Generation Workflow", () => {
  let server: MockMCPServer;
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    server = new MockMCPServer();
    fetchMock = vi.fn();
    global.fetch = fetchMock;

    // Register tools
    const catalogTool: MCPTool = {
      name: "get_component_catalog",
      description: "Fetches components",
      inputSchema: { type: "object", properties: {}, required: [] },
      execute: async () => {
        const response = await fetch("http://localhost:8080/components/types");
        return await response.json();
      },
    };

    const validateTool: MCPTool = {
      name: "validate_flow",
      description: "Validates flow",
      inputSchema: {
        type: "object",
        properties: { flowId: { type: "string" }, flow: { type: "object" } },
        required: ["flowId", "flow"],
      },
      execute: async (input) => {
        const { flowId, flow } = input;
        const response = await fetch(
          `http://localhost:8080/flowbuilder/flows/${flowId}/validate`,
          {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(flow),
          },
        );
        return await response.json();
      },
    };

    server.registerTool(catalogTool);
    server.registerTool(validateTool);
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe("end-to-end flow generation", () => {
    it("should complete full flow generation workflow", async () => {
      // Step 1: Get component catalog
      const mockComponents: ComponentType[] = [
        {
          id: "udp-listener",
          name: "UDP Listener",
          type: "input",
          protocol: "udp",
          category: "input",
          description: "Receives UDP packets",
          version: "1.0.0",
        },
        {
          id: "nats-publisher",
          name: "NATS Publisher",
          type: "output",
          protocol: "nats",
          category: "output",
          description: "Publishes to NATS",
          version: "1.0.0",
        },
      ];

      fetchMock.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => mockComponents,
      });

      const components = await server.executeTool("get_component_catalog", {});

      expect(components).toEqual(mockComponents);

      // Step 2: Validate generated flow
      const generatedFlow = {
        id: "generated-flow-1",
        name: "UDP to NATS Flow",
        version: 1,
        nodes: [
          {
            id: "node-1",
            type: "udp-listener",
            name: "UDP Input",
            position: { x: 100, y: 100 },
            config: { port: 5000 },
          },
          {
            id: "node-2",
            type: "nats-publisher",
            name: "NATS Output",
            position: { x: 400, y: 100 },
            config: { subject: "udp.data" },
          },
        ],
        connections: [
          {
            id: "conn-1",
            source_node_id: "node-1",
            source_port: "output",
            target_node_id: "node-2",
            target_port: "input",
          },
        ],
        runtime_state: "not_deployed" as const,
        created_at: "2025-01-01T00:00:00Z",
        updated_at: "2025-01-01T00:00:00Z",
        last_modified: "2025-01-01T00:00:00Z",
      };

      const mockValidation: ValidationResult = {
        validation_status: "valid",
        errors: [],
        warnings: [],
      };

      fetchMock.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => mockValidation,
      });

      const validation = await server.executeTool("validate_flow", {
        flowId: "generated-flow-1",
        flow: generatedFlow,
      });

      expect(validation).toEqual(mockValidation);
    });

    it("should handle validation errors during flow generation", async () => {
      // Get components
      const mockComponents: ComponentType[] = [
        {
          id: "component-1",
          name: "Component 1",
          type: "input",
          protocol: "nats",
          category: "input",
          description: "Test",
          version: "1.0.0",
        },
      ];

      fetchMock.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => mockComponents,
      });

      await server.executeTool("get_component_catalog", {});

      // Validate invalid flow
      const invalidFlow = {
        id: "invalid-flow",
        name: "Invalid Flow",
        version: 1,
        nodes: [
          {
            id: "node-1",
            type: "unknown-component",
            name: "Unknown",
            position: { x: 100, y: 100 },
            config: {},
          },
        ],
        connections: [],
        runtime_state: "not_deployed" as const,
        created_at: "2025-01-01T00:00:00Z",
        updated_at: "2025-01-01T00:00:00Z",
        last_modified: "2025-01-01T00:00:00Z",
      };

      const mockValidation: ValidationResult = {
        validation_status: "errors",
        errors: [
          {
            type: "unknown_component",
            severity: "error",
            component_name: "Unknown",
            message: "Component type not found in registry",
          },
        ],
        warnings: [],
      };

      fetchMock.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => mockValidation,
      });

      const validation = (await server.executeTool("validate_flow", {
        flowId: "invalid-flow",
        flow: invalidFlow,
      })) as ValidationResult;

      expect(validation.validation_status).toBe("errors");
      expect(validation.errors).toHaveLength(1);
    });

    it("should support iterative flow refinement", async () => {
      // Get components
      fetchMock.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => [],
      });

      await server.executeTool("get_component_catalog", {});

      // First attempt - has warnings
      const flow1 = {
        id: "flow-v1",
        name: "Flow V1",
        version: 1,
        nodes: [
          {
            id: "node-1",
            type: "component-1",
            name: "Component 1",
            position: { x: 100, y: 100 },
            config: {},
          },
        ],
        connections: [],
        runtime_state: "not_deployed" as const,
        created_at: "2025-01-01T00:00:00Z",
        updated_at: "2025-01-01T00:00:00Z",
        last_modified: "2025-01-01T00:00:00Z",
      };

      fetchMock.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({
          validation_status: "warnings",
          errors: [],
          warnings: [
            {
              type: "disconnected_node",
              severity: "warning",
              component_name: "Component 1",
              message: "Component is not connected",
            },
          ],
        }),
      });

      const validation1 = (await server.executeTool("validate_flow", {
        flowId: "flow-v1",
        flow: flow1,
      })) as ValidationResult;

      expect(validation1.validation_status).toBe("warnings");

      // Second attempt - valid
      const flow2 = {
        ...flow1,
        nodes: [
          ...flow1.nodes,
          {
            id: "node-2",
            type: "component-2",
            name: "Component 2",
            position: { x: 400, y: 100 },
            config: {},
          },
        ],
        connections: [
          {
            id: "conn-1",
            source_node_id: "node-1",
            source_port: "output",
            target_node_id: "node-2",
            target_port: "input",
          },
        ],
      };

      fetchMock.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({
          validation_status: "valid",
          errors: [],
          warnings: [],
        }),
      });

      const validation2 = (await server.executeTool("validate_flow", {
        flowId: "flow-v1",
        flow: flow2,
      })) as ValidationResult;

      expect(validation2.validation_status).toBe("valid");
    });
  });
});
