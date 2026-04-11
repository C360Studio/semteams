/**
 * API Route Tests: AI Flow Generation
 *
 * Tests for the /api/ai/generate-flow endpoint that orchestrates
 * Claude-based flow generation using MCP tools.
 *
 * Test Coverage:
 * - POST request handling with prompt and existing flow
 * - MCP server integration and tool execution
 * - Claude API integration for flow generation
 * - Error handling for invalid requests and API failures
 * - Response format validation
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { POST } from "./+server";
import type { RequestEvent } from "./$types";
import type { MCPServer } from "$lib/server/mcp/server";
import type { ClaudeClient } from "$lib/server/mcp/claude";

// Define mock class using vi.hoisted so it's available for vi.mock
const { MockClaudeApiError } = vi.hoisted(() => {
  class MockClaudeApiError extends Error {
    code: string;
    statusCode?: number;
    retryable: boolean;

    constructor(
      message: string,
      code: string,
      statusCode?: number,
      retryable = false,
    ) {
      super(message);
      this.name = "ClaudeApiError";
      this.code = code;
      this.statusCode = statusCode;
      this.retryable = retryable;
    }
  }
  return { MockClaudeApiError };
});

// Mock MCP server and Claude client
vi.mock("$lib/server/mcp/server", () => ({
  createMCPServer: vi.fn(),
}));

vi.mock("$lib/server/mcp/claude", () => ({
  createClaudeClient: vi.fn(),
  ClaudeApiError: MockClaudeApiError,
}));

// Mock environment variables (dynamic)
vi.mock("$env/dynamic/private", () => ({
  env: {
    ANTHROPIC_API_KEY: "test-api-key",
    ANTHROPIC_MODEL: "claude-sonnet-4-20250514",
    BACKEND_URL: "http://localhost:8080",
  },
}));

// Mock rate limiter to always allow requests in tests
vi.mock("$lib/server/middleware/rateLimit", () => ({
  checkRateLimit: vi.fn().mockReturnValue({
    allowed: true,
    remaining: 10,
    limit: 10,
  }),
  AI_RATE_LIMIT_CONFIG: {
    maxRequests: 10,
    windowMs: 60000,
  },
}));

describe("AI Flow Generation API Route", () => {
  let mockMCPServer: Record<string, ReturnType<typeof vi.fn>>;
  let mockClaudeClient: Record<string, ReturnType<typeof vi.fn>>;
  let mockFetch: ReturnType<typeof vi.fn>;

  beforeEach(async () => {
    // Reset mocks
    vi.clearAllMocks();

    // Mock MCP server
    mockMCPServer = {
      executeTool: vi.fn(),
      getComponentCatalog: vi.fn(),
      validateFlow: vi.fn(),
    };

    // Mock Claude client
    mockClaudeClient = {
      sendMessageWithTools: vi.fn(),
      parseToolUse: vi.fn(),
      sendToolResult: vi.fn(),
    };

    // Mock fetch
    mockFetch = vi.fn();

    // Setup mock factories
    const { createMCPServer } = await import("$lib/server/mcp/server");
    const { createClaudeClient } = await import("$lib/server/mcp/claude");
    const { checkRateLimit } = await import("$lib/server/middleware/rateLimit");

    vi.mocked(createMCPServer).mockReturnValue(
      mockMCPServer as unknown as MCPServer,
    );
    vi.mocked(createClaudeClient).mockReturnValue(
      mockClaudeClient as unknown as ClaudeClient,
    );
    // Re-apply rate limit mock after global vi.resetAllMocks() in setup.ts
    vi.mocked(checkRateLimit).mockReturnValue({
      allowed: true,
      remaining: 10,
      limit: 10,
    });
  });

  describe("Request Validation", () => {
    it("should reject requests without prompt", async () => {
      const request = new Request("http://localhost/api/ai/generate-flow", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({}),
      });

      const event = { request, fetch: mockFetch } as unknown as RequestEvent;

      const response = await POST(event);
      const data = await response.json();

      expect(response.status).toBe(400);
      expect(data.error).toContain("Prompt");
    });

    it("should accept requests with prompt only", async () => {
      // Mock component catalog
      mockMCPServer.getComponentCatalog.mockResolvedValue([
        {
          id: "http-source",
          name: "HTTP Source",
          category: "input",
          description: "HTTP input",
        },
      ]);

      // Mock Claude response with tool use
      mockClaudeClient.sendMessageWithTools.mockResolvedValue({
        content: [
          {
            type: "tool_use",
            id: "tool_1",
            name: "create_flow",
            input: { nodes: [], connections: [] },
          },
        ],
        stop_reason: "tool_use",
      });

      // Mock parseToolUse to return the tool
      mockClaudeClient.parseToolUse.mockReturnValue([
        {
          name: "create_flow",
          id: "tool_1",
          input: { nodes: [], connections: [] },
        },
      ]);

      // Mock flow validation
      mockMCPServer.validateFlow.mockResolvedValue({
        validation_status: "valid",
        errors: [],
        warnings: [],
      });

      const request = new Request("http://localhost/api/ai/generate-flow", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ prompt: "Create a simple flow" }),
      });

      const event = { request, fetch: mockFetch } as unknown as RequestEvent;

      const response = await POST(event);

      expect(response.status).toBe(200);
    });

    it("should accept requests with prompt and existing flow", async () => {
      // Mock component catalog
      mockMCPServer.getComponentCatalog.mockResolvedValue([
        {
          id: "http-source",
          name: "HTTP Source",
          category: "input",
          description: "HTTP input",
        },
      ]);

      // Mock Claude response with tool use
      mockClaudeClient.sendMessageWithTools.mockResolvedValue({
        content: [
          {
            type: "tool_use",
            id: "tool_1",
            name: "create_flow",
            input: { nodes: [], connections: [] },
          },
        ],
        stop_reason: "tool_use",
      });

      // Mock parseToolUse to return the tool
      mockClaudeClient.parseToolUse.mockReturnValue([
        {
          name: "create_flow",
          id: "tool_1",
          input: { nodes: [], connections: [] },
        },
      ]);

      // Mock flow validation
      mockMCPServer.validateFlow.mockResolvedValue({
        validation_status: "valid",
        errors: [],
        warnings: [],
      });

      const existingFlow = {
        id: "flow-1",
        name: "Existing Flow",
        nodes: [],
        connections: [],
      };

      const request = new Request("http://localhost/api/ai/generate-flow", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          prompt: "Add a new component",
          existingFlow,
        }),
      });

      const event = { request, fetch: mockFetch } as unknown as RequestEvent;

      const response = await POST(event);

      expect(response.status).toBe(200);
    });
  });

  describe("MCP Integration", () => {
    it("should fetch component catalog via MCP", async () => {
      const mockCatalog = [
        { id: "http-source", name: "HTTP Source", category: "sources" },
      ];

      mockMCPServer.getComponentCatalog.mockResolvedValue(mockCatalog);
      mockClaudeClient.sendMessageWithTools.mockResolvedValue({
        content: [{ type: "text", text: "Flow generated" }],
        stop_reason: "end_turn",
      });

      const request = new Request("http://localhost/api/ai/generate-flow", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ prompt: "Create a flow" }),
      });

      const event = { request, fetch: mockFetch } as unknown as RequestEvent;

      await POST(event);

      expect(mockMCPServer.getComponentCatalog).toHaveBeenCalled();
    });

    it("should validate generated flow via MCP", async () => {
      const generatedFlow = {
        nodes: [
          {
            id: "n1",
            type: "http-source",
            name: "Source",
            position: { x: 0, y: 0 },
            config: {},
          },
        ],
        connections: [],
      };

      const validationResult = {
        validation_status: "valid",
        errors: [],
        warnings: [],
      };

      // Mock component catalog
      mockMCPServer.getComponentCatalog.mockResolvedValue([
        {
          id: "http-source",
          name: "HTTP Source",
          category: "input",
          description: "HTTP input",
        },
      ]);

      mockClaudeClient.sendMessageWithTools.mockResolvedValue({
        content: [
          {
            type: "tool_use",
            id: "tool-1",
            name: "create_flow",
            input: generatedFlow,
          },
        ],
        stop_reason: "tool_use",
      });

      mockClaudeClient.parseToolUse.mockReturnValue([
        {
          name: "create_flow",
          id: "tool-1",
          input: generatedFlow,
        },
      ]);

      mockMCPServer.validateFlow.mockResolvedValue(validationResult);

      const request = new Request("http://localhost/api/ai/generate-flow", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ prompt: "Create a flow" }),
      });

      const event = { request, fetch: mockFetch } as unknown as RequestEvent;

      const response = await POST(event);
      const data = await response.json();

      expect(mockMCPServer.validateFlow).toHaveBeenCalled();
      expect(data.validationResult).toEqual(validationResult);
    });
  });

  describe("Claude Integration", () => {
    it("should send system prompt with component catalog", async () => {
      const mockCatalog = [
        { id: "http-source", name: "HTTP Source", category: "sources" },
      ];

      mockMCPServer.getComponentCatalog.mockResolvedValue(mockCatalog);
      mockClaudeClient.sendMessageWithTools.mockResolvedValue({
        content: [{ type: "text", text: "Flow generated" }],
        stop_reason: "end_turn",
      });

      const request = new Request("http://localhost/api/ai/generate-flow", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ prompt: "Create a flow" }),
      });

      const event = { request, fetch: mockFetch } as unknown as RequestEvent;

      await POST(event);

      expect(mockClaudeClient.sendMessageWithTools).toHaveBeenCalledWith(
        expect.any(String),
        expect.any(Array),
        expect.objectContaining({
          systemPrompt: expect.stringContaining("component catalog"),
        }),
      );
    });

    it("should handle tool use responses from Claude", async () => {
      const generatedFlow = {
        nodes: [
          {
            id: "n1",
            type: "http-source",
            name: "Source",
            position: { x: 0, y: 0 },
            config: {},
          },
        ],
        connections: [],
      };

      // Mock component catalog
      mockMCPServer.getComponentCatalog.mockResolvedValue([
        {
          id: "http-source",
          name: "HTTP Source",
          category: "input",
          description: "HTTP input",
        },
      ]);

      mockClaudeClient.sendMessageWithTools.mockResolvedValue({
        content: [
          {
            type: "tool_use",
            id: "tool-1",
            name: "create_flow",
            input: generatedFlow,
          },
        ],
        stop_reason: "tool_use",
      });

      mockClaudeClient.parseToolUse.mockReturnValue([
        {
          name: "create_flow",
          id: "tool-1",
          input: generatedFlow,
        },
      ]);

      mockMCPServer.validateFlow.mockResolvedValue({
        validation_status: "valid",
        errors: [],
        warnings: [],
      });

      const request = new Request("http://localhost/api/ai/generate-flow", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ prompt: "Create a flow" }),
      });

      const event = { request, fetch: mockFetch } as unknown as RequestEvent;

      const response = await POST(event);
      const data = await response.json();

      expect(data.flow).toEqual(generatedFlow);
    });

    it("should use configured API key and model", async () => {
      mockClaudeClient.sendMessageWithTools.mockResolvedValue({
        content: [{ type: "text", text: "Flow generated" }],
        stop_reason: "end_turn",
      });

      const request = new Request("http://localhost/api/ai/generate-flow", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ prompt: "Create a flow" }),
      });

      const event = { request, fetch: mockFetch } as unknown as RequestEvent;

      await POST(event);

      const { createClaudeClient } = await import("$lib/server/mcp/claude");
      expect(createClaudeClient).toHaveBeenCalledWith(
        expect.objectContaining({
          apiKey: "test-api-key",
        }),
      );
    });
  });

  describe("Response Format", () => {
    it("should return flow and validation result on success", async () => {
      const generatedFlow = {
        nodes: [
          {
            id: "n1",
            type: "http-source",
            name: "Source",
            position: { x: 0, y: 0 },
            config: {},
          },
        ],
        connections: [],
      };

      const validationResult = {
        validation_status: "valid",
        errors: [],
        warnings: [],
      };

      // Mock component catalog
      mockMCPServer.getComponentCatalog.mockResolvedValue([
        {
          id: "http-source",
          name: "HTTP Source",
          category: "input",
          description: "HTTP input",
        },
      ]);

      mockClaudeClient.sendMessageWithTools.mockResolvedValue({
        content: [
          {
            type: "tool_use",
            id: "tool-1",
            name: "create_flow",
            input: generatedFlow,
          },
        ],
        stop_reason: "tool_use",
      });

      mockClaudeClient.parseToolUse.mockReturnValue([
        {
          name: "create_flow",
          id: "tool-1",
          input: generatedFlow,
        },
      ]);

      mockMCPServer.validateFlow.mockResolvedValue(validationResult);

      const request = new Request("http://localhost/api/ai/generate-flow", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ prompt: "Create a flow" }),
      });

      const event = { request, fetch: mockFetch } as unknown as RequestEvent;

      const response = await POST(event);
      const data = await response.json();

      expect(response.status).toBe(200);
      expect(data).toHaveProperty("flow");
      expect(data).toHaveProperty("validationResult");
      expect(data.flow).toEqual(generatedFlow);
      expect(data.validationResult).toEqual(validationResult);
    });
  });

  describe("Error Handling", () => {
    it("should handle MCP server errors", async () => {
      mockMCPServer.getComponentCatalog.mockRejectedValue(
        new Error("Failed to fetch component catalog"),
      );

      const request = new Request("http://localhost/api/ai/generate-flow", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ prompt: "Create a flow" }),
      });

      const event = { request, fetch: mockFetch } as unknown as RequestEvent;

      const response = await POST(event);
      const data = await response.json();

      expect(response.status).toBe(503);
      expect(data.error).toContain("available components");
    });

    it("should handle Claude API errors", async () => {
      mockMCPServer.getComponentCatalog.mockResolvedValue([]);
      mockClaudeClient.sendMessageWithTools.mockRejectedValue(
        new Error("Claude API rate limit exceeded"),
      );

      const request = new Request("http://localhost/api/ai/generate-flow", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ prompt: "Create a flow" }),
      });

      const event = { request, fetch: mockFetch } as unknown as RequestEvent;

      const response = await POST(event);
      const data = await response.json();

      // Claude API errors return 503 (service unavailable)
      expect(response.status).toBe(503);
      expect(data.code).toBe("CLAUDE_API_FAILED");
    });

    it("should handle validation errors", async () => {
      const generatedFlow = {
        nodes: [
          {
            id: "n1",
            type: "invalid-type",
            name: "Invalid",
            position: { x: 0, y: 0 },
            config: {},
          },
        ],
        connections: [],
      };

      // Mock component catalog
      mockMCPServer.getComponentCatalog.mockResolvedValue([
        {
          id: "http-source",
          name: "HTTP Source",
          category: "input",
          description: "HTTP input",
        },
      ]);

      mockClaudeClient.sendMessageWithTools.mockResolvedValue({
        content: [
          {
            type: "tool_use",
            id: "tool-1",
            name: "create_flow",
            input: generatedFlow,
          },
        ],
        stop_reason: "tool_use",
      });

      mockClaudeClient.parseToolUse.mockReturnValue([
        {
          name: "create_flow",
          id: "tool-1",
          input: generatedFlow,
        },
      ]);

      mockMCPServer.validateFlow.mockResolvedValue({
        validation_status: "errors",
        errors: [
          {
            type: "unknown_component",
            severity: "error",
            component_name: "n1",
            message: "Unknown component type: invalid-type",
          },
        ],
        warnings: [],
      });

      const request = new Request("http://localhost/api/ai/generate-flow", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ prompt: "Create a flow" }),
      });

      const event = { request, fetch: mockFetch } as unknown as RequestEvent;

      const response = await POST(event);
      const data = await response.json();

      // Should still return 200 but with validation errors in the result
      expect(response.status).toBe(200);
      expect(data.validationResult.validation_status).toBe("errors");
      expect(data.validationResult.errors.length).toBeGreaterThan(0);
    });
  });
});
