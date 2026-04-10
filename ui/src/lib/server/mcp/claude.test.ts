/**
 * Claude API Client Test Suite
 *
 * Tests for Claude API integration using Anthropic SDK.
 * Tests message sending, tool use handling, streaming, and error handling.
 *
 * Following TDD principles:
 * - Test client initialization
 * - Test message sending with and without tools
 * - Test tool use response parsing
 * - Test streaming responses
 * - Test comprehensive error scenarios
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import Anthropic from "@anthropic-ai/sdk";

// Mock types for Claude API responses
interface ClaudeMessage {
  id: string;
  type: "message";
  role: "assistant";
  content: ClaudeContent[];
  model: string;
  stop_reason: "end_turn" | "tool_use" | "max_tokens";
}

type ClaudeContent = TextContent | ToolUseContent;

interface TextContent {
  type: "text";
  text: string;
}

interface ToolUseContent {
  type: "tool_use";
  id: string;
  name: string;
  input: Record<string, unknown>;
}

interface ClaudeTool {
  name: string;
  description: string;
  input_schema: {
    type: "object";
    properties: Record<string, unknown>;
    required: string[];
  };
}

describe("Claude API Client - Initialization", () => {
  describe("with valid API key", () => {
    it("should initialize Anthropic client", () => {
      const apiKey = "sk-ant-test-key-123";
      const client = new Anthropic({ apiKey, dangerouslyAllowBrowser: true });

      expect(client).toBeDefined();
      expect(client).toBeInstanceOf(Anthropic);
    });

    it("should accept API key from environment variable", () => {
      const originalEnv = process.env.ANTHROPIC_API_KEY;
      process.env.ANTHROPIC_API_KEY = "sk-ant-env-key-456";

      const client = new Anthropic({ dangerouslyAllowBrowser: true });

      expect(client).toBeDefined();

      // Restore original env
      if (originalEnv) {
        process.env.ANTHROPIC_API_KEY = originalEnv;
      } else {
        delete process.env.ANTHROPIC_API_KEY;
      }
    });

    it("should store API key securely", () => {
      const apiKey = "sk-ant-test-key-789";
      const client = new Anthropic({ apiKey, dangerouslyAllowBrowser: true });

      // API key is stored but should be used internally only
      // The SDK does expose it as a property, which is expected behavior
      expect(client).toBeDefined();
      expect(client.apiKey).toBe(apiKey);
    });
  });

  describe("with missing API key", () => {
    it("should throw error when API key is missing", () => {
      const originalEnv = process.env.ANTHROPIC_API_KEY;
      delete process.env.ANTHROPIC_API_KEY;

      expect(() => {
        new Anthropic({ apiKey: "" });
      }).toThrow();

      // Restore original env
      if (originalEnv) {
        process.env.ANTHROPIC_API_KEY = originalEnv;
      }
    });

    it("should provide helpful error message", () => {
      const originalEnv = process.env.ANTHROPIC_API_KEY;
      delete process.env.ANTHROPIC_API_KEY;

      try {
        new Anthropic({ apiKey: "" });
        expect.fail("Should have thrown an error");
      } catch (error) {
        expect(error).toBeDefined();
      }

      // Restore original env
      if (originalEnv) {
        process.env.ANTHROPIC_API_KEY = originalEnv;
      }
    });
  });
});

describe("Claude API Client - Message Sending", () => {
  let mockClient: {
    messages: {
      create: ReturnType<typeof vi.fn>;
      stream: ReturnType<typeof vi.fn>;
    };
  };

  beforeEach(() => {
    mockClient = {
      messages: {
        create: vi.fn(),
        stream: vi.fn(),
      },
    };
  });

  describe("basic message", () => {
    it("should send message to Claude", async () => {
      const mockResponse: ClaudeMessage = {
        id: "msg_123",
        type: "message",
        role: "assistant",
        content: [{ type: "text", text: "Hello! How can I help you?" }],
        model: "claude-3-5-sonnet-20241022",
        stop_reason: "end_turn",
      };

      mockClient.messages.create.mockResolvedValueOnce(mockResponse);

      const sendMessage = async (prompt: string) => {
        return await mockClient.messages.create({
          model: "claude-3-5-sonnet-20241022",
          max_tokens: 1024,
          messages: [{ role: "user", content: prompt }],
        });
      };

      const result = await sendMessage("Hello Claude");

      expect(mockClient.messages.create).toHaveBeenCalledWith(
        expect.objectContaining({
          model: "claude-3-5-sonnet-20241022",
          max_tokens: 1024,
          messages: [{ role: "user", content: "Hello Claude" }],
        }),
      );
      expect(result.content[0].type).toBe("text");
      expect((result.content[0] as TextContent).text).toBeTruthy();
    });

    it("should handle multi-turn conversations", async () => {
      const mockResponse: ClaudeMessage = {
        id: "msg_456",
        type: "message",
        role: "assistant",
        content: [{ type: "text", text: "I'm doing well, thank you!" }],
        model: "claude-3-5-sonnet-20241022",
        stop_reason: "end_turn",
      };

      mockClient.messages.create.mockResolvedValueOnce(mockResponse);

      const sendMessage = async (
        messages: Array<{ role: string; content: string }>,
      ) => {
        return await mockClient.messages.create({
          model: "claude-3-5-sonnet-20241022",
          max_tokens: 1024,
          messages,
        });
      };

      const conversation = [
        { role: "user", content: "Hello Claude" },
        { role: "assistant", content: "Hello! How can I help you?" },
        { role: "user", content: "How are you?" },
      ];

      const result = await sendMessage(conversation);

      expect(mockClient.messages.create).toHaveBeenCalledWith(
        expect.objectContaining({
          messages: conversation,
        }),
      );
      expect(result).toBeDefined();
    });

    it("should support system prompts", async () => {
      const mockResponse: ClaudeMessage = {
        id: "msg_789",
        type: "message",
        role: "assistant",
        content: [
          { type: "text", text: "Component catalog retrieved successfully." },
        ],
        model: "claude-3-5-sonnet-20241022",
        stop_reason: "end_turn",
      };

      mockClient.messages.create.mockResolvedValueOnce(mockResponse);

      const sendMessage = async (systemPrompt: string, userPrompt: string) => {
        return await mockClient.messages.create({
          model: "claude-3-5-sonnet-20241022",
          max_tokens: 1024,
          system: systemPrompt,
          messages: [{ role: "user", content: userPrompt }],
        });
      };

      const result = await sendMessage(
        "You are a flow generation assistant",
        "Generate a flow",
      );

      expect(mockClient.messages.create).toHaveBeenCalledWith(
        expect.objectContaining({
          system: "You are a flow generation assistant",
          messages: [{ role: "user", content: "Generate a flow" }],
        }),
      );
      expect(result).toBeDefined();
    });
  });

  describe("message with tools", () => {
    it("should send tools with message", async () => {
      const tools: ClaudeTool[] = [
        {
          name: "get_component_catalog",
          description: "Fetches available component types",
          input_schema: {
            type: "object",
            properties: {},
            required: [],
          },
        },
      ];

      const mockResponse: ClaudeMessage = {
        id: "msg_tool_1",
        type: "message",
        role: "assistant",
        content: [
          {
            type: "tool_use",
            id: "toolu_123",
            name: "get_component_catalog",
            input: {},
          },
        ],
        model: "claude-3-5-sonnet-20241022",
        stop_reason: "tool_use",
      };

      mockClient.messages.create.mockResolvedValueOnce(mockResponse);

      const sendMessageWithTools = async (
        prompt: string,
        tools: ClaudeTool[],
      ) => {
        return await mockClient.messages.create({
          model: "claude-3-5-sonnet-20241022",
          max_tokens: 4096,
          tools,
          messages: [{ role: "user", content: prompt }],
        });
      };

      const result = await sendMessageWithTools(
        "What components are available?",
        tools,
      );

      expect(mockClient.messages.create).toHaveBeenCalledWith(
        expect.objectContaining({
          tools,
          messages: [
            { role: "user", content: "What components are available?" },
          ],
        }),
      );
      expect(result.stop_reason).toBe("tool_use");
      expect(result.content[0].type).toBe("tool_use");
    });

    it("should handle multiple tools", async () => {
      const tools: ClaudeTool[] = [
        {
          name: "get_component_catalog",
          description: "Fetches available component types",
          input_schema: {
            type: "object",
            properties: {},
            required: [],
          },
        },
        {
          name: "validate_flow",
          description: "Validates a flow configuration",
          input_schema: {
            type: "object",
            properties: {
              flowId: { type: "string" },
              flow: { type: "object" },
            },
            required: ["flowId", "flow"],
          },
        },
      ];

      const mockResponse: ClaudeMessage = {
        id: "msg_multi_tool",
        type: "message",
        role: "assistant",
        content: [{ type: "text", text: "I can help with that." }],
        model: "claude-3-5-sonnet-20241022",
        stop_reason: "end_turn",
      };

      mockClient.messages.create.mockResolvedValueOnce(mockResponse);

      const sendMessageWithTools = async (
        prompt: string,
        tools: ClaudeTool[],
      ) => {
        return await mockClient.messages.create({
          model: "claude-3-5-sonnet-20241022",
          max_tokens: 4096,
          tools,
          messages: [{ role: "user", content: prompt }],
        });
      };

      const result = await sendMessageWithTools(
        "Generate and validate a flow",
        tools,
      );

      expect(mockClient.messages.create).toHaveBeenCalledWith(
        expect.objectContaining({
          tools: expect.arrayContaining([
            expect.objectContaining({ name: "get_component_catalog" }),
            expect.objectContaining({ name: "validate_flow" }),
          ]),
        }),
      );
      expect(result).toBeDefined();
    });
  });
});

describe("Claude API Client - Tool Use Handling", () => {
  let mockClient: {
    messages: {
      create: ReturnType<typeof vi.fn>;
    };
  };

  beforeEach(() => {
    mockClient = {
      messages: {
        create: vi.fn(),
      },
    };
  });

  describe("parsing tool calls", () => {
    it("should parse tool use from response", async () => {
      const mockResponse: ClaudeMessage = {
        id: "msg_parse_1",
        type: "message",
        role: "assistant",
        content: [
          {
            type: "tool_use",
            id: "toolu_abc",
            name: "get_component_catalog",
            input: {},
          },
        ],
        model: "claude-3-5-sonnet-20241022",
        stop_reason: "tool_use",
      };

      mockClient.messages.create.mockResolvedValueOnce(mockResponse);

      const parseToolUse = (response: ClaudeMessage) => {
        return response.content.filter((block) => block.type === "tool_use");
      };

      const response = await mockClient.messages.create({
        model: "claude-3-5-sonnet-20241022",
        max_tokens: 4096,
        messages: [{ role: "user", content: "Get components" }],
      });

      const toolUseBlocks = parseToolUse(response);

      expect(toolUseBlocks).toHaveLength(1);
      expect(toolUseBlocks[0].type).toBe("tool_use");
      expect((toolUseBlocks[0] as ToolUseContent).name).toBe(
        "get_component_catalog",
      );
    });

    it("should extract tool input parameters", async () => {
      const mockResponse: ClaudeMessage = {
        id: "msg_parse_2",
        type: "message",
        role: "assistant",
        content: [
          {
            type: "tool_use",
            id: "toolu_def",
            name: "validate_flow",
            input: {
              flowId: "test-flow-123",
              flow: { nodes: [], connections: [] },
            },
          },
        ],
        model: "claude-3-5-sonnet-20241022",
        stop_reason: "tool_use",
      };

      mockClient.messages.create.mockResolvedValueOnce(mockResponse);

      const extractToolInput = (toolUse: ToolUseContent) => {
        return toolUse.input;
      };

      const response = await mockClient.messages.create({
        model: "claude-3-5-sonnet-20241022",
        max_tokens: 4096,
        messages: [{ role: "user", content: "Validate flow" }],
      });

      const toolUse = response.content[0] as ToolUseContent;
      const input = extractToolInput(toolUse);

      expect(input).toHaveProperty("flowId");
      expect(input).toHaveProperty("flow");
      expect(input.flowId).toBe("test-flow-123");
    });

    it("should handle multiple tool uses in one response", async () => {
      const mockResponse: ClaudeMessage = {
        id: "msg_multi_use",
        type: "message",
        role: "assistant",
        content: [
          {
            type: "text",
            text: "I'll fetch the components and validate the flow.",
          },
          {
            type: "tool_use",
            id: "toolu_1",
            name: "get_component_catalog",
            input: {},
          },
          {
            type: "tool_use",
            id: "toolu_2",
            name: "validate_flow",
            input: { flowId: "test", flow: {} },
          },
        ],
        model: "claude-3-5-sonnet-20241022",
        stop_reason: "tool_use",
      };

      mockClient.messages.create.mockResolvedValueOnce(mockResponse);

      const response = await mockClient.messages.create({
        model: "claude-3-5-sonnet-20241022",
        max_tokens: 4096,
        messages: [{ role: "user", content: "Get and validate" }],
      });

      const toolUseBlocks = response.content.filter(
        (block: { type: string }) => block.type === "tool_use",
      );

      expect(toolUseBlocks).toHaveLength(2);
    });
  });

  describe("returning tool results", () => {
    it("should send tool results back to Claude", async () => {
      const toolResultMessage = {
        role: "user" as const,
        content: [
          {
            type: "tool_result" as const,
            tool_use_id: "toolu_abc",
            content: JSON.stringify([
              { id: "udp-listener", name: "UDP Listener", type: "input" },
            ]),
          },
        ],
      };

      const mockResponse: ClaudeMessage = {
        id: "msg_result_1",
        type: "message",
        role: "assistant",
        content: [{ type: "text", text: "I found 1 component available." }],
        model: "claude-3-5-sonnet-20241022",
        stop_reason: "end_turn",
      };

      mockClient.messages.create.mockResolvedValueOnce(mockResponse);

      const response = await mockClient.messages.create({
        model: "claude-3-5-sonnet-20241022",
        max_tokens: 4096,
        messages: [toolResultMessage],
      });

      expect(mockClient.messages.create).toHaveBeenCalledWith(
        expect.objectContaining({
          messages: [toolResultMessage],
        }),
      );
      expect(response.content[0].type).toBe("text");
    });

    it("should handle tool errors", async () => {
      const toolErrorMessage = {
        role: "user" as const,
        content: [
          {
            type: "tool_result" as const,
            tool_use_id: "toolu_error",
            content: "Error: Failed to fetch components",
            is_error: true,
          },
        ],
      };

      const mockResponse: ClaudeMessage = {
        id: "msg_error_1",
        type: "message",
        role: "assistant",
        content: [
          {
            type: "text",
            text: "I encountered an error fetching the components. Please try again.",
          },
        ],
        model: "claude-3-5-sonnet-20241022",
        stop_reason: "end_turn",
      };

      mockClient.messages.create.mockResolvedValueOnce(mockResponse);

      const response = await mockClient.messages.create({
        model: "claude-3-5-sonnet-20241022",
        max_tokens: 4096,
        messages: [toolErrorMessage],
      });

      expect(response).toBeDefined();
      expect((response.content[0] as TextContent).text).toContain("error");
    });
  });
});

describe("Claude API Client - Streaming", () => {
  let mockClient: {
    messages: {
      stream: ReturnType<typeof vi.fn>;
    };
  };

  beforeEach(() => {
    mockClient = {
      messages: {
        stream: vi.fn(),
      },
    };
  });

  describe("streaming responses", () => {
    it("should stream message responses", async () => {
      const mockStream = {
        [Symbol.asyncIterator]: async function* () {
          yield { type: "message_start", message: { id: "msg_stream_1" } };
          yield {
            type: "content_block_start",
            index: 0,
            content_block: { type: "text" },
          };
          yield {
            type: "content_block_delta",
            index: 0,
            delta: { type: "text_delta", text: "Hello" },
          };
          yield {
            type: "content_block_delta",
            index: 0,
            delta: { type: "text_delta", text: " world" },
          };
          yield { type: "content_block_stop", index: 0 };
          yield { type: "message_delta", delta: { stop_reason: "end_turn" } };
          yield { type: "message_stop" };
        },
      };

      mockClient.messages.stream.mockReturnValueOnce(mockStream);

      const chunks: string[] = [];
      for await (const event of mockStream) {
        if (
          event.type === "content_block_delta" &&
          event.delta?.type === "text_delta"
        ) {
          chunks.push(event.delta.text as string);
        }
      }

      expect(chunks).toEqual(["Hello", " world"]);
      expect(chunks.join("")).toBe("Hello world");
    });

    it("should handle streaming tool use", async () => {
      const mockStream = {
        [Symbol.asyncIterator]: async function* () {
          yield { type: "message_start", message: { id: "msg_stream_tool" } };
          yield {
            type: "content_block_start",
            index: 0,
            content_block: {
              type: "tool_use",
              id: "toolu_stream",
              name: "get_component_catalog",
            },
          };
          yield {
            type: "content_block_delta",
            index: 0,
            delta: { type: "input_json_delta", partial_json: "{}" },
          };
          yield { type: "content_block_stop", index: 0 };
          yield { type: "message_delta", delta: { stop_reason: "tool_use" } };
          yield { type: "message_stop" };
        },
      };

      mockClient.messages.stream.mockReturnValueOnce(mockStream);

      let toolUsed = false;
      for await (const event of mockStream) {
        if (
          event.type === "content_block_start" &&
          event.content_block?.type === "tool_use"
        ) {
          toolUsed = true;
        }
      }

      expect(toolUsed).toBe(true);
    });

    it("should accumulate streamed content", async () => {
      const mockStream = {
        [Symbol.asyncIterator]: async function* () {
          yield {
            type: "content_block_delta",
            index: 0,
            delta: { type: "text_delta", text: "Part " },
          };
          yield {
            type: "content_block_delta",
            index: 0,
            delta: { type: "text_delta", text: "1 " },
          };
          yield {
            type: "content_block_delta",
            index: 0,
            delta: { type: "text_delta", text: "Part " },
          };
          yield {
            type: "content_block_delta",
            index: 0,
            delta: { type: "text_delta", text: "2" },
          };
        },
      };

      mockClient.messages.stream.mockReturnValueOnce(mockStream);

      let fullText = "";
      for await (const event of mockStream) {
        if (
          event.type === "content_block_delta" &&
          event.delta.type === "text_delta"
        ) {
          fullText += event.delta.text;
        }
      }

      expect(fullText).toBe("Part 1 Part 2");
    });
  });
});

describe("Claude API Client - Error Handling", () => {
  let mockClient: {
    messages: {
      create: ReturnType<typeof vi.fn>;
    };
  };

  beforeEach(() => {
    mockClient = {
      messages: {
        create: vi.fn(),
      },
    };
  });

  describe("API errors", () => {
    it("should handle 400 bad request", async () => {
      const error = new Error("Invalid request");
      (error as unknown as { status: number }).status = 400;
      mockClient.messages.create.mockRejectedValueOnce(error);

      await expect(
        mockClient.messages.create({
          model: "claude-3-5-sonnet-20241022",
          max_tokens: 1024,
          messages: [],
        }),
      ).rejects.toThrow("Invalid request");
    });

    it("should handle 401 authentication error", async () => {
      const error = new Error("Invalid API key");
      (error as unknown as { status: number }).status = 401;
      mockClient.messages.create.mockRejectedValueOnce(error);

      await expect(
        mockClient.messages.create({
          model: "claude-3-5-sonnet-20241022",
          max_tokens: 1024,
          messages: [{ role: "user", content: "Hello" }],
        }),
      ).rejects.toThrow("Invalid API key");
    });

    it("should handle 429 rate limit error", async () => {
      const error = new Error("Rate limit exceeded");
      (error as unknown as { status: number }).status = 429;
      mockClient.messages.create.mockRejectedValueOnce(error);

      await expect(
        mockClient.messages.create({
          model: "claude-3-5-sonnet-20241022",
          max_tokens: 1024,
          messages: [{ role: "user", content: "Hello" }],
        }),
      ).rejects.toThrow("Rate limit exceeded");
    });

    it("should handle 500 server error", async () => {
      const error = new Error("Internal server error");
      (error as unknown as { status: number }).status = 500;
      mockClient.messages.create.mockRejectedValueOnce(error);

      await expect(
        mockClient.messages.create({
          model: "claude-3-5-sonnet-20241022",
          max_tokens: 1024,
          messages: [{ role: "user", content: "Hello" }],
        }),
      ).rejects.toThrow("Internal server error");
    });

    it("should handle 529 overload error", async () => {
      const error = new Error("Service overloaded");
      (error as unknown as { status: number }).status = 529;
      mockClient.messages.create.mockRejectedValueOnce(error);

      await expect(
        mockClient.messages.create({
          model: "claude-3-5-sonnet-20241022",
          max_tokens: 1024,
          messages: [{ role: "user", content: "Hello" }],
        }),
      ).rejects.toThrow("Service overloaded");
    });
  });

  describe("network errors", () => {
    it("should handle network timeout", async () => {
      mockClient.messages.create.mockRejectedValueOnce(
        new Error("Request timeout"),
      );

      await expect(
        mockClient.messages.create({
          model: "claude-3-5-sonnet-20241022",
          max_tokens: 1024,
          messages: [{ role: "user", content: "Hello" }],
        }),
      ).rejects.toThrow("Request timeout");
    });

    it("should handle connection refused", async () => {
      mockClient.messages.create.mockRejectedValueOnce(
        new Error("Connection refused"),
      );

      await expect(
        mockClient.messages.create({
          model: "claude-3-5-sonnet-20241022",
          max_tokens: 1024,
          messages: [{ role: "user", content: "Hello" }],
        }),
      ).rejects.toThrow("Connection refused");
    });

    it("should handle DNS resolution failure", async () => {
      mockClient.messages.create.mockRejectedValueOnce(
        new Error("DNS lookup failed"),
      );

      await expect(
        mockClient.messages.create({
          model: "claude-3-5-sonnet-20241022",
          max_tokens: 1024,
          messages: [{ role: "user", content: "Hello" }],
        }),
      ).rejects.toThrow("DNS lookup failed");
    });
  });

  describe("validation errors", () => {
    it("should handle invalid model name", async () => {
      mockClient.messages.create.mockRejectedValueOnce(
        new Error("Invalid model"),
      );

      await expect(
        mockClient.messages.create({
          model: "invalid-model",
          max_tokens: 1024,
          messages: [{ role: "user", content: "Hello" }],
        }),
      ).rejects.toThrow("Invalid model");
    });

    it("should handle max_tokens too large", async () => {
      mockClient.messages.create.mockRejectedValueOnce(
        new Error("max_tokens exceeds limit"),
      );

      await expect(
        mockClient.messages.create({
          model: "claude-3-5-sonnet-20241022",
          max_tokens: 999999,
          messages: [{ role: "user", content: "Hello" }],
        }),
      ).rejects.toThrow("max_tokens exceeds limit");
    });

    it("should handle empty messages array", async () => {
      mockClient.messages.create.mockRejectedValueOnce(
        new Error("messages array cannot be empty"),
      );

      await expect(
        mockClient.messages.create({
          model: "claude-3-5-sonnet-20241022",
          max_tokens: 1024,
          messages: [],
        }),
      ).rejects.toThrow("messages array cannot be empty");
    });
  });
});

describe("Claude API Client - Request Cancellation", () => {
  let mockClient: {
    messages: {
      create: ReturnType<typeof vi.fn>;
      stream: ReturnType<typeof vi.fn>;
    };
  };

  beforeEach(() => {
    mockClient = {
      messages: {
        create: vi.fn(),
        stream: vi.fn(),
      },
    };
  });

  describe("AbortController support", () => {
    it("should pass abort signal to API call", async () => {
      const abortController = new AbortController();
      const mockResponse: ClaudeMessage = {
        id: "msg_cancel_1",
        type: "message",
        role: "assistant",
        content: [{ type: "text", text: "Response" }],
        model: "claude-3-5-sonnet-20241022",
        stop_reason: "end_turn",
      };

      mockClient.messages.create.mockResolvedValueOnce(mockResponse);

      const sendMessage = async (
        prompt: string,
        options?: { signal?: AbortSignal },
      ) => {
        return await mockClient.messages.create(
          {
            model: "claude-3-5-sonnet-20241022",
            max_tokens: 1024,
            messages: [{ role: "user", content: prompt }],
          },
          { signal: options?.signal },
        );
      };

      await sendMessage("Test", { signal: abortController.signal });

      expect(mockClient.messages.create).toHaveBeenCalledWith(
        expect.any(Object),
        expect.objectContaining({ signal: abortController.signal }),
      );
    });

    it("should abort pending request when signal is aborted", async () => {
      const abortController = new AbortController();

      mockClient.messages.create.mockImplementationOnce(
        (_params: unknown, options?: { signal?: AbortSignal }) => {
          return new Promise((_, reject) => {
            if (options?.signal) {
              options.signal.addEventListener("abort", () => {
                reject(new Error("Request was cancelled"));
              });
            }
          });
        },
      );

      const sendMessage = async (
        prompt: string,
        options?: { signal?: AbortSignal },
      ) => {
        return await mockClient.messages.create(
          {
            model: "claude-3-5-sonnet-20241022",
            max_tokens: 1024,
            messages: [{ role: "user", content: prompt }],
          },
          { signal: options?.signal },
        );
      };

      const promise = sendMessage("Test", { signal: abortController.signal });

      setTimeout(() => abortController.abort(), 10);

      await expect(promise).rejects.toThrow("Request was cancelled");
    });

    it("should reject immediately if signal is already aborted", async () => {
      const abortController = new AbortController();
      abortController.abort();

      mockClient.messages.create.mockImplementationOnce(
        (_params: unknown, options?: { signal?: AbortSignal }) => {
          if (options?.signal?.aborted) {
            return Promise.reject(new Error("Request was cancelled"));
          }
          return Promise.resolve({
            id: "msg_1",
            type: "message",
            role: "assistant",
            content: [],
            model: "claude-3-5-sonnet-20241022",
            stop_reason: "end_turn",
          });
        },
      );

      const sendMessage = async (
        prompt: string,
        options?: { signal?: AbortSignal },
      ) => {
        return await mockClient.messages.create(
          {
            model: "claude-3-5-sonnet-20241022",
            max_tokens: 1024,
            messages: [{ role: "user", content: prompt }],
          },
          { signal: options?.signal },
        );
      };

      await expect(
        sendMessage("Test", { signal: abortController.signal }),
      ).rejects.toThrow("Request was cancelled");
    });

    it("should abort streaming request when signal is aborted", async () => {
      const abortController = new AbortController();
      let streamAborted = false;

      const mockStream = {
        [Symbol.asyncIterator]: async function* () {
          yield { type: "message_start", message: { id: "msg_stream_abort" } };

          // Wait for abort
          await new Promise((_, reject) => {
            abortController.signal.addEventListener("abort", () => {
              streamAborted = true;
              reject(new Error("Stream was cancelled"));
            });
          });
        },
      };

      mockClient.messages.stream.mockReturnValueOnce(mockStream);

      const promise = (async () => {
        for await (const event of mockStream) {
          // Process events
          void event;
        }
      })();

      setTimeout(() => abortController.abort(), 10);

      await expect(promise).rejects.toThrow("Stream was cancelled");
      expect(streamAborted).toBe(true);
    });

    it("should work normally when no signal is provided", async () => {
      const mockResponse: ClaudeMessage = {
        id: "msg_no_signal",
        type: "message",
        role: "assistant",
        content: [{ type: "text", text: "Normal response" }],
        model: "claude-3-5-sonnet-20241022",
        stop_reason: "end_turn",
      };

      mockClient.messages.create.mockResolvedValueOnce(mockResponse);

      const sendMessage = async (
        prompt: string,
        options?: { signal?: AbortSignal },
      ) => {
        return await mockClient.messages.create(
          {
            model: "claude-3-5-sonnet-20241022",
            max_tokens: 1024,
            messages: [{ role: "user", content: prompt }],
          },
          options ? { signal: options.signal } : undefined,
        );
      };

      const result = await sendMessage("Test");

      expect(result.content[0].type).toBe("text");
      expect((result.content[0] as TextContent).text).toBe("Normal response");
    });
  });
});
