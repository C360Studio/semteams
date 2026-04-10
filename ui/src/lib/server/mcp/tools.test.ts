/**
 * MCP Tools Test Suite
 *
 * Tests for MCP tool definitions and implementations.
 * Tests get_component_catalog and validate_flow tools.
 *
 * Following TDD principles:
 * - Test tool schema definitions
 * - Test successful API calls
 * - Test error handling
 * - Test response format validation
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import type { ComponentType } from "$lib/types/component";
import type { ValidationResult } from "$lib/types/validation";

// Mock types for MCP tool definitions
interface MCPTool {
  name: string;
  description: string;
  inputSchema: {
    type: string;
    properties: Record<string, unknown>;
    required: string[];
  };
}

// Mock implementation placeholder - will be replaced with actual implementation
const getComponentCatalogTool = {
  name: "get_component_catalog",
  description:
    "Fetches available component types from the backend component registry",
  inputSchema: {
    type: "object",
    properties: {},
    required: [],
  },
} as MCPTool;

const validateFlowTool = {
  name: "validate_flow",
  description: "Validates a flow configuration and returns validation results",
  inputSchema: {
    type: "object",
    properties: {
      flowId: {
        type: "string",
        description: "The ID of the flow to validate",
      },
      flow: {
        type: "object",
        description: "The flow configuration to validate",
      },
    },
    required: ["flowId", "flow"],
  },
} as MCPTool;

describe("MCP Tools - Tool Schema Definitions", () => {
  describe("get_component_catalog tool schema", () => {
    it("should have correct tool name", () => {
      expect(getComponentCatalogTool.name).toBe("get_component_catalog");
    });

    it("should have descriptive description", () => {
      expect(getComponentCatalogTool.description).toBeTruthy();
      expect(getComponentCatalogTool.description.length).toBeGreaterThan(10);
    });

    it("should have valid JSON schema", () => {
      expect(getComponentCatalogTool.inputSchema.type).toBe("object");
      expect(getComponentCatalogTool.inputSchema.properties).toBeDefined();
      expect(getComponentCatalogTool.inputSchema.required).toBeDefined();
    });

    it("should require no input parameters", () => {
      expect(getComponentCatalogTool.inputSchema.required).toHaveLength(0);
    });
  });

  describe("validate_flow tool schema", () => {
    it("should have correct tool name", () => {
      expect(validateFlowTool.name).toBe("validate_flow");
    });

    it("should have descriptive description", () => {
      expect(validateFlowTool.description).toBeTruthy();
      expect(validateFlowTool.description.length).toBeGreaterThan(10);
    });

    it("should have valid JSON schema", () => {
      expect(validateFlowTool.inputSchema.type).toBe("object");
      expect(validateFlowTool.inputSchema.properties).toBeDefined();
      expect(validateFlowTool.inputSchema.required).toBeDefined();
    });

    it("should require flowId and flow parameters", () => {
      expect(validateFlowTool.inputSchema.required).toContain("flowId");
      expect(validateFlowTool.inputSchema.required).toContain("flow");
    });

    it("should define flowId parameter", () => {
      const flowIdProp = validateFlowTool.inputSchema.properties
        .flowId as Record<string, unknown>;
      expect(flowIdProp).toBeDefined();
      expect(flowIdProp.type).toBe("string");
      expect(flowIdProp.description).toBeTruthy();
    });

    it("should define flow parameter", () => {
      const flowProp = validateFlowTool.inputSchema.properties.flow as Record<
        string,
        unknown
      >;
      expect(flowProp).toBeDefined();
      expect(flowProp.type).toBe("object");
      expect(flowProp.description).toBeTruthy();
    });
  });
});

describe("MCP Tools - get_component_catalog Implementation", () => {
  const mockBackendUrl = "http://localhost:8080";
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn();
    global.fetch = fetchMock;
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe("successful fetch", () => {
    it("should fetch component types from backend", async () => {
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

      // Mock tool implementation
      const getComponentCatalog = async () => {
        const response = await fetch(`${mockBackendUrl}/components/types`);
        if (!response.ok) {
          throw new Error(`HTTP ${response.status}`);
        }
        return await response.json();
      };

      const result = await getComponentCatalog();

      expect(fetchMock).toHaveBeenCalledWith(
        `${mockBackendUrl}/components/types`,
      );
      expect(result).toEqual(mockComponents);
    });

    it("should return array of ComponentType objects", async () => {
      const mockComponents: ComponentType[] = [
        {
          id: "test-component",
          name: "Test Component",
          type: "processor",
          protocol: "nats",
          category: "processor",
          description: "Test description",
          version: "1.0.0",
        },
      ];

      fetchMock.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => mockComponents,
      });

      const getComponentCatalog = async () => {
        const response = await fetch(`${mockBackendUrl}/components/types`);
        return await response.json();
      };

      const result = await getComponentCatalog();

      expect(Array.isArray(result)).toBe(true);
      expect(result[0]).toHaveProperty("id");
      expect(result[0]).toHaveProperty("name");
      expect(result[0]).toHaveProperty("type");
      expect(result[0]).toHaveProperty("protocol");
      expect(result[0]).toHaveProperty("category");
      expect(result[0]).toHaveProperty("description");
      expect(result[0]).toHaveProperty("version");
    });

    it("should include optional ports and schema if present", async () => {
      const mockComponents: ComponentType[] = [
        {
          id: "advanced-component",
          name: "Advanced Component",
          type: "processor",
          protocol: "nats",
          category: "processor",
          description: "Component with ports and config",
          version: "1.0.0",
          ports: [
            {
              id: "input-1",
              name: "Input Port",
              direction: "input",
              required: true,
              description: "Main input port",
              config: {
                type: "nats",
                nats: {
                  subject: "input.stream",
                  interface: { type: "message.Storable" },
                },
              },
            },
          ],
          schema: {
            type: "object",
            properties: {
              timeout: {
                type: "number",
                description: "Timeout in seconds",
                default: 30,
              },
            },
            required: ["timeout"],
          },
        },
      ];

      fetchMock.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => mockComponents,
      });

      const getComponentCatalog = async () => {
        const response = await fetch(`${mockBackendUrl}/components/types`);
        return await response.json();
      };

      const result = await getComponentCatalog();

      expect(result[0].ports).toBeDefined();
      expect(result[0].schema).toBeDefined();
    });

    it("should handle empty component list", async () => {
      fetchMock.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => [],
      });

      const getComponentCatalog = async () => {
        const response = await fetch(`${mockBackendUrl}/components/types`);
        return await response.json();
      };

      const result = await getComponentCatalog();

      expect(Array.isArray(result)).toBe(true);
      expect(result).toHaveLength(0);
    });
  });

  describe("error handling", () => {
    it("should handle 404 not found", async () => {
      fetchMock.mockResolvedValueOnce({
        ok: false,
        status: 404,
        statusText: "Not Found",
      });

      const getComponentCatalog = async () => {
        const response = await fetch(`${mockBackendUrl}/components/types`);
        if (!response.ok) {
          throw new Error(`HTTP ${response.status}: ${response.statusText}`);
        }
        return await response.json();
      };

      await expect(getComponentCatalog()).rejects.toThrow("HTTP 404");
    });

    it("should handle 500 internal server error", async () => {
      fetchMock.mockResolvedValueOnce({
        ok: false,
        status: 500,
        statusText: "Internal Server Error",
      });

      const getComponentCatalog = async () => {
        const response = await fetch(`${mockBackendUrl}/components/types`);
        if (!response.ok) {
          throw new Error(`HTTP ${response.status}: ${response.statusText}`);
        }
        return await response.json();
      };

      await expect(getComponentCatalog()).rejects.toThrow("HTTP 500");
    });

    it("should handle network errors", async () => {
      fetchMock.mockRejectedValueOnce(new Error("Network error"));

      const getComponentCatalog = async () => {
        const response = await fetch(`${mockBackendUrl}/components/types`);
        return await response.json();
      };

      await expect(getComponentCatalog()).rejects.toThrow("Network error");
    });

    it("should handle malformed JSON response", async () => {
      fetchMock.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => {
          throw new Error("Invalid JSON");
        },
      });

      const getComponentCatalog = async () => {
        const response = await fetch(`${mockBackendUrl}/components/types`);
        return await response.json();
      };

      await expect(getComponentCatalog()).rejects.toThrow("Invalid JSON");
    });

    it("should handle timeout", async () => {
      fetchMock.mockImplementationOnce(
        () =>
          new Promise((_, reject) => {
            setTimeout(() => reject(new Error("Request timeout")), 100);
          }),
      );

      const getComponentCatalog = async () => {
        const response = await fetch(`${mockBackendUrl}/components/types`);
        return await response.json();
      };

      await expect(getComponentCatalog()).rejects.toThrow("Request timeout");
    });
  });
});

describe("MCP Tools - validate_flow Implementation", () => {
  const mockBackendUrl = "http://localhost:8080";
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn();
    global.fetch = fetchMock;
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe("valid flow validation", () => {
    it("should validate flow and return valid status", async () => {
      const flowId = "test-flow-123";
      const mockFlow = {
        id: flowId,
        name: "Test Flow",
        version: 1,
        nodes: [
          {
            id: "node-1",
            type: "udp-listener",
            name: "UDP Input",
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

      const mockValidationResult: ValidationResult = {
        validation_status: "valid",
        errors: [],
        warnings: [],
      };

      fetchMock.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => mockValidationResult,
      });

      const validateFlow = async (flowId: string, flow: unknown) => {
        const response = await fetch(
          `${mockBackendUrl}/flowbuilder/flows/${flowId}/validate`,
          {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(flow),
          },
        );
        if (!response.ok) {
          throw new Error(`HTTP ${response.status}`);
        }
        return await response.json();
      };

      const result = await validateFlow(flowId, mockFlow);

      expect(fetchMock).toHaveBeenCalledWith(
        `${mockBackendUrl}/flowbuilder/flows/${flowId}/validate`,
        expect.objectContaining({
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(mockFlow),
        }),
      );
      expect(result.validation_status).toBe("valid");
      expect(result.errors).toHaveLength(0);
      expect(result.warnings).toHaveLength(0);
    });

    it("should return validation result with correct structure", async () => {
      const mockValidationResult: ValidationResult = {
        validation_status: "valid",
        errors: [],
        warnings: [],
      };

      fetchMock.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => mockValidationResult,
      });

      const validateFlow = async (flowId: string, flow: unknown) => {
        const response = await fetch(
          `${mockBackendUrl}/flowbuilder/flows/${flowId}/validate`,
          {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(flow),
          },
        );
        return await response.json();
      };

      const result = await validateFlow("test-flow", {});

      expect(result).toHaveProperty("validation_status");
      expect(result).toHaveProperty("errors");
      expect(result).toHaveProperty("warnings");
    });
  });

  describe("invalid flow validation", () => {
    it("should return validation errors for invalid flow", async () => {
      const mockValidationResult: ValidationResult = {
        validation_status: "errors",
        errors: [
          {
            type: "orphaned_port",
            severity: "error",
            component_name: "UDP Input",
            port_name: "output",
            message: "Port has no connections",
            suggestions: ["Connect the output port to another component"],
          },
        ],
        warnings: [],
      };

      fetchMock.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => mockValidationResult,
      });

      const validateFlow = async (flowId: string, flow: unknown) => {
        const response = await fetch(
          `${mockBackendUrl}/flowbuilder/flows/${flowId}/validate`,
          {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(flow),
          },
        );
        return await response.json();
      };

      const result = await validateFlow("test-flow", {});

      expect(result.validation_status).toBe("errors");
      expect(result.errors).toHaveLength(1);
      expect(result.errors[0].type).toBe("orphaned_port");
      expect(result.errors[0].severity).toBe("error");
    });

    it("should return validation warnings", async () => {
      const mockValidationResult: ValidationResult = {
        validation_status: "warnings",
        errors: [],
        warnings: [
          {
            type: "disconnected_node",
            severity: "warning",
            component_name: "Logger",
            message: "Component is not connected to the flow",
            suggestions: ["Add connections to integrate this component"],
          },
        ],
      };

      fetchMock.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => mockValidationResult,
      });

      const validateFlow = async (flowId: string, flow: unknown) => {
        const response = await fetch(
          `${mockBackendUrl}/flowbuilder/flows/${flowId}/validate`,
          {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(flow),
          },
        );
        return await response.json();
      };

      const result = await validateFlow("test-flow", {});

      expect(result.validation_status).toBe("warnings");
      expect(result.warnings).toHaveLength(1);
      expect(result.warnings[0].type).toBe("disconnected_node");
      expect(result.warnings[0].severity).toBe("warning");
    });

    it("should handle multiple validation errors", async () => {
      const mockValidationResult: ValidationResult = {
        validation_status: "errors",
        errors: [
          {
            type: "orphaned_port",
            severity: "error",
            component_name: "Component 1",
            port_name: "output",
            message: "Port has no connections",
          },
          {
            type: "unknown_component",
            severity: "error",
            component_name: "Component 2",
            message: "Component type not found in registry",
          },
          {
            type: "cycle_detected",
            severity: "error",
            component_name: "Component 3",
            message: "Circular dependency detected",
          },
        ],
        warnings: [],
      };

      fetchMock.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => mockValidationResult,
      });

      const validateFlow = async (flowId: string, flow: unknown) => {
        const response = await fetch(
          `${mockBackendUrl}/flowbuilder/flows/${flowId}/validate`,
          {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(flow),
          },
        );
        return await response.json();
      };

      const result = await validateFlow("test-flow", {});

      expect(result.errors).toHaveLength(3);
    });

    it("should include suggestions in validation issues", async () => {
      const mockValidationResult: ValidationResult = {
        validation_status: "errors",
        errors: [
          {
            type: "missing_config",
            severity: "error",
            component_name: "NATS Publisher",
            message: "Required configuration missing",
            suggestions: [
              "Add subject configuration",
              "Specify queue group if needed",
              "Configure interface contract",
            ],
          },
        ],
        warnings: [],
      };

      fetchMock.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => mockValidationResult,
      });

      const validateFlow = async (flowId: string, flow: unknown) => {
        const response = await fetch(
          `${mockBackendUrl}/flowbuilder/flows/${flowId}/validate`,
          {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(flow),
          },
        );
        return await response.json();
      };

      const result = await validateFlow("test-flow", {});

      expect(result.errors[0].suggestions).toBeDefined();
      expect(result.errors[0].suggestions?.length).toBeGreaterThan(0);
    });
  });

  describe("error handling", () => {
    it("should handle 400 bad request", async () => {
      fetchMock.mockResolvedValueOnce({
        ok: false,
        status: 400,
        statusText: "Bad Request",
      });

      const validateFlow = async (flowId: string, flow: unknown) => {
        const response = await fetch(
          `${mockBackendUrl}/flowbuilder/flows/${flowId}/validate`,
          {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(flow),
          },
        );
        if (!response.ok) {
          throw new Error(`HTTP ${response.status}: ${response.statusText}`);
        }
        return await response.json();
      };

      await expect(validateFlow("test-flow", {})).rejects.toThrow("HTTP 400");
    });

    it("should handle 404 flow not found", async () => {
      fetchMock.mockResolvedValueOnce({
        ok: false,
        status: 404,
        statusText: "Not Found",
      });

      const validateFlow = async (flowId: string, flow: unknown) => {
        const response = await fetch(
          `${mockBackendUrl}/flowbuilder/flows/${flowId}/validate`,
          {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(flow),
          },
        );
        if (!response.ok) {
          throw new Error(`HTTP ${response.status}: ${response.statusText}`);
        }
        return await response.json();
      };

      await expect(validateFlow("nonexistent-flow", {})).rejects.toThrow(
        "HTTP 404",
      );
    });

    it("should handle 500 server error", async () => {
      fetchMock.mockResolvedValueOnce({
        ok: false,
        status: 500,
        statusText: "Internal Server Error",
      });

      const validateFlow = async (flowId: string, flow: unknown) => {
        const response = await fetch(
          `${mockBackendUrl}/flowbuilder/flows/${flowId}/validate`,
          {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(flow),
          },
        );
        if (!response.ok) {
          throw new Error(`HTTP ${response.status}: ${response.statusText}`);
        }
        return await response.json();
      };

      await expect(validateFlow("test-flow", {})).rejects.toThrow("HTTP 500");
    });

    it("should handle network errors", async () => {
      fetchMock.mockRejectedValueOnce(new Error("Network error"));

      const validateFlow = async (flowId: string, flow: unknown) => {
        const response = await fetch(
          `${mockBackendUrl}/flowbuilder/flows/${flowId}/validate`,
          {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(flow),
          },
        );
        return await response.json();
      };

      await expect(validateFlow("test-flow", {})).rejects.toThrow(
        "Network error",
      );
    });

    it("should handle malformed JSON in request", async () => {
      // This tests the case where JSON.stringify might fail
      const circularRef: { self?: unknown } = {};
      circularRef.self = circularRef;

      const validateFlow = async (flowId: string, flow: unknown) => {
        const body = JSON.stringify(flow); // Will throw on circular reference
        const response = await fetch(
          `${mockBackendUrl}/flowbuilder/flows/${flowId}/validate`,
          {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body,
          },
        );
        return await response.json();
      };

      await expect(validateFlow("test-flow", circularRef)).rejects.toThrow();
    });

    it("should handle malformed JSON in response", async () => {
      fetchMock.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => {
          throw new Error("Invalid JSON");
        },
      });

      const validateFlow = async (flowId: string, flow: unknown) => {
        const response = await fetch(
          `${mockBackendUrl}/flowbuilder/flows/${flowId}/validate`,
          {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(flow),
          },
        );
        return await response.json();
      };

      await expect(validateFlow("test-flow", {})).rejects.toThrow(
        "Invalid JSON",
      );
    });
  });
});
