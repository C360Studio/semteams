import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { aiApi, AiApiError } from "./aiApi";
import type { Flow } from "$lib/types/flow";

describe("aiApi", () => {
  // Mock fetch globally
  const mockFetch = vi.fn();
  globalThis.fetch = mockFetch;

  beforeEach(() => {
    mockFetch.mockClear();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  // Helper to create mock flow
  const createMockFlow = (): Flow => ({
    version: 1,
    id: "flow-123",
    name: "Generated Flow",
    description: "AI-generated flow",
    nodes: [
      {
        id: "node-1",
        component: "udp-input",
        type: "input",
        name: "UDP Input",
        position: { x: 100, y: 100 },
        config: { port: 5000 },
      },
      {
        id: "node-2",
        component: "json-transform",
        type: "processor",
        name: "JSON Transform",
        position: { x: 300, y: 100 },
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
    runtime_state: "not_deployed",
    created_at: "2026-01-06T12:00:00Z",
    updated_at: "2026-01-06T12:00:00Z",
    created_by: "user-123",
    last_modified: "2026-01-06T12:00:00Z",
  });

  const createMockValidationResult = () => ({
    validation_status: "valid" as const,
    errors: [] as Array<{
      type: string;
      severity: string;
      component_name: string;
      message: string;
    }>,
    warnings: [] as Array<{
      type: string;
      severity: string;
      component_name: string;
      message: string;
    }>,
  });

  // =========================================================================
  // generateFlow Tests
  // =========================================================================

  describe("generateFlow", () => {
    it("should generate flow from prompt", async () => {
      const mockResponse = {
        flow: createMockFlow(),
        validationResult: createMockValidationResult(),
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => mockResponse,
      });

      const prompt = "Create a flow that reads UDP on port 5000";
      const result = await aiApi.generateFlow(prompt);

      expect(mockFetch).toHaveBeenCalledWith("/ai/generate-flow", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({ prompt }),
      });

      expect(result).toEqual(mockResponse);
      expect(result.flow).toBeDefined();
      expect(result.validationResult).toBeDefined();
    });

    it("should generate flow with existing flow context", async () => {
      const existingFlow = createMockFlow();
      const mockResponse = {
        flow: createMockFlow(),
        validationResult: createMockValidationResult(),
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => mockResponse,
      });

      const prompt = "Add a NATS publisher";
      const result = await aiApi.generateFlow(prompt, existingFlow);

      expect(mockFetch).toHaveBeenCalledWith("/ai/generate-flow", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          prompt,
          existingFlow,
        }),
      });

      expect(result).toEqual(mockResponse);
    });

    it("should handle API errors", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 400,
        statusText: "Bad Request",
        json: async () => ({
          error: "Invalid prompt: prompt too short",
        }),
      });

      try {
        await aiApi.generateFlow("");
        expect.fail("Should have thrown AiApiError");
      } catch (error) {
        expect(error).toBeInstanceOf(AiApiError);
        expect((error as AiApiError).statusCode).toBe(400);
        expect((error as AiApiError).message).toContain(
          "Failed to generate flow",
        );
        expect((error as AiApiError).details).toEqual({
          error: "Invalid prompt: prompt too short",
        });
      }
    });

    it("should handle network errors", async () => {
      mockFetch.mockRejectedValueOnce(new Error("Network error"));

      try {
        await aiApi.generateFlow("Test prompt");
        expect.fail("Should have thrown error");
      } catch (error) {
        expect(error).toBeInstanceOf(Error);
        expect((error as Error).message).toContain("Network error");
      }
    });

    it("should handle timeout errors", async () => {
      mockFetch.mockImplementationOnce(
        () =>
          new Promise((_, reject) =>
            setTimeout(() => reject(new Error("Timeout")), 100),
          ),
      );

      try {
        await aiApi.generateFlow("Test prompt");
        expect.fail("Should have thrown error");
      } catch (error) {
        expect(error).toBeInstanceOf(Error);
      }
    });

    it("should handle malformed JSON response", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => {
          throw new Error("Invalid JSON");
        },
      });

      try {
        await aiApi.generateFlow("Test prompt");
        expect.fail("Should have thrown error");
      } catch (error) {
        expect(error).toBeInstanceOf(Error);
        expect((error as Error).message).toContain("Invalid JSON");
      }
    });

    it("should handle validation errors in response", async () => {
      const mockResponse = {
        flow: createMockFlow(),
        validationResult: {
          validation_status: "errors" as const,
          errors: [
            {
              type: "missing_config",
              severity: "error",
              component_name: "node-1",
              message: "Missing required field",
            },
          ],
          warnings: [],
        },
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => mockResponse,
      });

      const result = await aiApi.generateFlow("Test prompt");

      expect(result.validationResult.validation_status).toBe("errors");
      expect(result.validationResult.errors).toHaveLength(1);
    });

    it("should handle server errors (500)", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        statusText: "Internal Server Error",
        json: async () => ({
          error: "AI service unavailable",
        }),
      });

      try {
        await aiApi.generateFlow("Test prompt");
        expect.fail("Should have thrown AiApiError");
      } catch (error) {
        expect(error).toBeInstanceOf(AiApiError);
        expect((error as AiApiError).statusCode).toBe(500);
      }
    });

    it("should handle rate limiting (429)", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 429,
        statusText: "Too Many Requests",
        json: async () => ({
          error: "Rate limit exceeded",
          retryAfter: 60,
        }),
      });

      try {
        await aiApi.generateFlow("Test prompt");
        expect.fail("Should have thrown AiApiError");
      } catch (error) {
        expect(error).toBeInstanceOf(AiApiError);
        expect((error as AiApiError).statusCode).toBe(429);
        expect((error as AiApiError).details).toHaveProperty("retryAfter");
      }
    });

    it("should handle unauthorized errors (401)", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 401,
        statusText: "Unauthorized",
        json: async () => ({
          error: "API key required",
        }),
      });

      try {
        await aiApi.generateFlow("Test prompt");
        expect.fail("Should have thrown AiApiError");
      } catch (error) {
        expect(error).toBeInstanceOf(AiApiError);
        expect((error as AiApiError).statusCode).toBe(401);
      }
    });

    it("should trim prompt before sending", async () => {
      const mockResponse = {
        flow: createMockFlow(),
        validationResult: createMockValidationResult(),
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => mockResponse,
      });

      await aiApi.generateFlow("  Test prompt  ");

      expect(mockFetch).toHaveBeenCalledWith(
        "/ai/generate-flow",
        expect.objectContaining({
          body: JSON.stringify({ prompt: "Test prompt" }),
        }),
      );
    });

    it("should handle empty prompt", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 400,
        statusText: "Bad Request",
        json: async () => ({
          error: "Prompt cannot be empty",
        }),
      });

      try {
        await aiApi.generateFlow("");
        expect.fail("Should have thrown AiApiError");
      } catch (error) {
        expect(error).toBeInstanceOf(AiApiError);
      }
    });

    it("should handle very long prompts", async () => {
      const longPrompt = "a".repeat(10000);
      const mockResponse = {
        flow: createMockFlow(),
        validationResult: createMockValidationResult(),
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => mockResponse,
      });

      const result = await aiApi.generateFlow(longPrompt);

      expect(result).toBeDefined();
    });

    it("should handle special characters in prompt", async () => {
      const mockResponse = {
        flow: createMockFlow(),
        validationResult: createMockValidationResult(),
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => mockResponse,
      });

      const specialPrompt =
        "Create flow: UDP->NATS (port: 5000) & transform {json}";
      const result = await aiApi.generateFlow(specialPrompt);

      expect(result).toBeDefined();
      expect(mockFetch).toHaveBeenCalledWith(
        expect.anything(),
        expect.objectContaining({
          body: expect.stringContaining("UDP->NATS"),
        }),
      );
    });
  });

  // =========================================================================
  // streamGenerateFlow Tests
  // =========================================================================

  describe("streamGenerateFlow", () => {
    it("should stream flow generation with progress updates", async () => {
      const mockResponse = {
        flow: createMockFlow(),
        validationResult: createMockValidationResult(),
      };

      const chunks = [
        "Analyzing prompt...",
        "Identifying components...",
        "Creating connections...",
        "Validating flow...",
      ];

      const onProgress = vi.fn();

      // Mock ReadableStream
      const mockStream = new ReadableStream({
        start(controller) {
          chunks.forEach((chunk) => {
            controller.enqueue(new TextEncoder().encode(`data: ${chunk}\n\n`));
          });
          controller.enqueue(
            new TextEncoder().encode(
              `data: ${JSON.stringify(mockResponse)}\n\n`,
            ),
          );
          controller.close();
        },
      });

      mockFetch.mockResolvedValueOnce({
        ok: true,
        body: mockStream,
      });

      const result = await aiApi.streamGenerateFlow(
        "Test prompt",
        undefined,
        onProgress,
      );

      expect(onProgress).toHaveBeenCalledTimes(chunks.length);
      expect(onProgress).toHaveBeenCalledWith("Analyzing prompt...");
      expect(onProgress).toHaveBeenCalledWith("Identifying components...");
      expect(result).toEqual(mockResponse);
    });

    it("should stream with existing flow context", async () => {
      const existingFlow = createMockFlow();
      const mockResponse = {
        flow: createMockFlow(),
        validationResult: createMockValidationResult(),
      };

      const mockStream = new ReadableStream({
        start(controller) {
          controller.enqueue(
            new TextEncoder().encode(
              `data: ${JSON.stringify(mockResponse)}\n\n`,
            ),
          );
          controller.close();
        },
      });

      mockFetch.mockResolvedValueOnce({
        ok: true,
        body: mockStream,
      });

      const result = await aiApi.streamGenerateFlow(
        "Add component",
        existingFlow,
      );

      expect(mockFetch).toHaveBeenCalledWith(
        "/ai/stream-generate-flow",
        expect.objectContaining({
          body: JSON.stringify({
            prompt: "Add component",
            existingFlow,
          }),
        }),
      );

      expect(result).toEqual(mockResponse);
    });

    it("should handle streaming errors", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        statusText: "Internal Server Error",
        json: async () => ({
          error: "Streaming failed",
        }),
      });

      try {
        await aiApi.streamGenerateFlow("Test prompt");
        expect.fail("Should have thrown AiApiError");
      } catch (error) {
        expect(error).toBeInstanceOf(AiApiError);
        expect((error as AiApiError).statusCode).toBe(500);
      }
    });

    it("should handle stream interruption", async () => {
      const mockStream = new ReadableStream({
        start(controller) {
          controller.enqueue(new TextEncoder().encode("data: Starting...\n\n"));
          controller.error(new Error("Stream interrupted"));
        },
      });

      mockFetch.mockResolvedValueOnce({
        ok: true,
        body: mockStream,
      });

      try {
        await aiApi.streamGenerateFlow("Test prompt");
        expect.fail("Should have thrown error");
      } catch (error) {
        expect(error).toBeInstanceOf(Error);
        expect((error as Error).message).toContain("Stream interrupted");
      }
    });

    it("should work without onProgress callback", async () => {
      const mockResponse = {
        flow: createMockFlow(),
        validationResult: createMockValidationResult(),
      };

      const mockStream = new ReadableStream({
        start(controller) {
          controller.enqueue(
            new TextEncoder().encode("data: Progress update\n\n"),
          );
          controller.enqueue(
            new TextEncoder().encode(
              `data: ${JSON.stringify(mockResponse)}\n\n`,
            ),
          );
          controller.close();
        },
      });

      mockFetch.mockResolvedValueOnce({
        ok: true,
        body: mockStream,
      });

      const result = await aiApi.streamGenerateFlow("Test prompt");

      expect(result).toEqual(mockResponse);
    });

    it("should handle malformed SSE data", async () => {
      const mockStream = new ReadableStream({
        start(controller) {
          controller.enqueue(
            new TextEncoder().encode("invalid data format\n\n"),
          );
          controller.close();
        },
      });

      mockFetch.mockResolvedValueOnce({
        ok: true,
        body: mockStream,
      });

      try {
        await aiApi.streamGenerateFlow("Test prompt");
        expect.fail("Should have thrown error");
      } catch (error) {
        expect(error).toBeInstanceOf(Error);
      }
    });

    it("should handle empty stream", async () => {
      const mockStream = new ReadableStream({
        start(controller) {
          controller.close();
        },
      });

      mockFetch.mockResolvedValueOnce({
        ok: true,
        body: mockStream,
      });

      try {
        await aiApi.streamGenerateFlow("Test prompt");
        expect.fail("Should have thrown error");
      } catch (error) {
        expect(error).toBeInstanceOf(Error);
      }
    });

    it("should handle partial JSON in stream", async () => {
      const mockResponse = {
        flow: createMockFlow(),
        validationResult: createMockValidationResult(),
      };

      const responseJson = JSON.stringify(mockResponse);
      const midpoint = Math.floor(responseJson.length / 2);
      const part1 = responseJson.slice(0, midpoint);
      const part2 = responseJson.slice(midpoint);

      const mockStream = new ReadableStream({
        start(controller) {
          controller.enqueue(new TextEncoder().encode(`data: ${part1}`));
          controller.enqueue(new TextEncoder().encode(`${part2}\n\n`));
          controller.close();
        },
      });

      mockFetch.mockResolvedValueOnce({
        ok: true,
        body: mockStream,
      });

      const result = await aiApi.streamGenerateFlow("Test prompt");

      expect(result).toEqual(mockResponse);
    });

    it("should call onProgress for each chunk", async () => {
      const onProgress = vi.fn();
      const mockResponse = {
        flow: createMockFlow(),
        validationResult: createMockValidationResult(),
      };

      const mockStream = new ReadableStream({
        start(controller) {
          controller.enqueue(new TextEncoder().encode("data: Step 1\n\n"));
          controller.enqueue(new TextEncoder().encode("data: Step 2\n\n"));
          controller.enqueue(new TextEncoder().encode("data: Step 3\n\n"));
          controller.enqueue(
            new TextEncoder().encode(
              `data: ${JSON.stringify(mockResponse)}\n\n`,
            ),
          );
          controller.close();
        },
      });

      mockFetch.mockResolvedValueOnce({
        ok: true,
        body: mockStream,
      });

      await aiApi.streamGenerateFlow("Test prompt", undefined, onProgress);

      expect(onProgress).toHaveBeenCalledTimes(3);
      expect(onProgress).toHaveBeenNthCalledWith(1, "Step 1");
      expect(onProgress).toHaveBeenNthCalledWith(2, "Step 2");
      expect(onProgress).toHaveBeenNthCalledWith(3, "Step 3");
    });

    it("should handle network errors during streaming", async () => {
      mockFetch.mockRejectedValueOnce(new Error("Network error"));

      try {
        await aiApi.streamGenerateFlow("Test prompt");
        expect.fail("Should have thrown error");
      } catch (error) {
        expect(error).toBeInstanceOf(Error);
        expect((error as Error).message).toContain("Network error");
      }
    });
  });

  // =========================================================================
  // AiApiError Tests
  // =========================================================================

  describe("AiApiError", () => {
    it("should create error with message and status code", () => {
      const error = new AiApiError("Test error", 500);

      expect(error.message).toBe("Test error");
      expect(error.statusCode).toBe(500);
      expect(error.details).toBeUndefined();
      expect(error.name).toBe("AiApiError");
    });

    it("should create error with details", () => {
      const details = { field: "prompt", reason: "too short" };
      const error = new AiApiError("Validation failed", 400, details);

      expect(error.message).toBe("Validation failed");
      expect(error.statusCode).toBe(400);
      expect(error.details).toEqual(details);
    });

    it("should be instanceof Error", () => {
      const error = new AiApiError("Test", 500);

      expect(error).toBeInstanceOf(Error);
      expect(error).toBeInstanceOf(AiApiError);
    });

    it("should have proper stack trace", () => {
      const error = new AiApiError("Test", 500);

      expect(error.stack).toBeDefined();
      expect(error.stack).toContain("AiApiError");
    });
  });

  // =========================================================================
  // Request Cancellation Tests
  // =========================================================================

  describe("Request Cancellation", () => {
    it("should cancel generateFlow request when signal is aborted", async () => {
      const abortController = new AbortController();

      // Mock fetch that waits and checks for abort
      mockFetch.mockImplementationOnce(
        (_url: string, options: { signal?: AbortSignal }) => {
          return new Promise((_, reject) => {
            if (options?.signal) {
              options.signal.addEventListener("abort", () => {
                reject(
                  new DOMException("The operation was aborted.", "AbortError"),
                );
              });
            }
          });
        },
      );

      const promise = aiApi.generateFlow("Test prompt", undefined, {
        signal: abortController.signal,
      });

      // Abort after a short delay
      setTimeout(() => abortController.abort(), 10);

      try {
        await promise;
        expect.fail("Should have thrown AbortError");
      } catch (error) {
        expect(error).toBeInstanceOf(DOMException);
        expect((error as DOMException).name).toBe("AbortError");
      }
    });

    it("should cancel generateFlow request immediately if already aborted", async () => {
      const abortController = new AbortController();
      abortController.abort(); // Abort before starting

      mockFetch.mockImplementationOnce(
        (_url: string, options: { signal?: AbortSignal }) => {
          if (options?.signal?.aborted) {
            return Promise.reject(
              new DOMException("The operation was aborted.", "AbortError"),
            );
          }
          return Promise.resolve({
            ok: true,
            json: async () => ({}),
          });
        },
      );

      try {
        await aiApi.generateFlow("Test prompt", undefined, {
          signal: abortController.signal,
        });
        expect.fail("Should have thrown AbortError");
      } catch (error) {
        expect(error).toBeInstanceOf(DOMException);
        expect((error as DOMException).name).toBe("AbortError");
      }
    });

    it("should pass signal to fetch in generateFlow", async () => {
      const abortController = new AbortController();
      const mockResponse = {
        flow: createMockFlow(),
        validationResult: createMockValidationResult(),
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => mockResponse,
      });

      await aiApi.generateFlow("Test prompt", undefined, {
        signal: abortController.signal,
      });

      expect(mockFetch).toHaveBeenCalledWith(
        "/ai/generate-flow",
        expect.objectContaining({
          signal: abortController.signal,
        }),
      );
    });

    it("should cancel streamGenerateFlow request when signal is aborted before fetch", async () => {
      const abortController = new AbortController();

      // Mock fetch that responds to abort signal
      mockFetch.mockImplementationOnce(
        (_url: string, options: { signal?: AbortSignal }) => {
          return new Promise((_, reject) => {
            if (options?.signal) {
              if (options.signal.aborted) {
                reject(
                  new DOMException("The operation was aborted.", "AbortError"),
                );
                return;
              }
              options.signal.addEventListener("abort", () => {
                reject(
                  new DOMException("The operation was aborted.", "AbortError"),
                );
              });
            }
          });
        },
      );

      const promise = aiApi.streamGenerateFlow(
        "Test prompt",
        undefined,
        undefined,
        {
          signal: abortController.signal,
        },
      );

      // Abort after a short delay
      setTimeout(() => abortController.abort(), 10);

      try {
        await promise;
        expect.fail("Should have thrown AbortError");
      } catch (error) {
        expect(error).toBeInstanceOf(DOMException);
        expect((error as DOMException).name).toBe("AbortError");
      }
    });

    it("should pass signal to fetch in streamGenerateFlow", async () => {
      const abortController = new AbortController();
      const mockResponse = {
        flow: createMockFlow(),
        validationResult: createMockValidationResult(),
      };

      const mockStream = new ReadableStream({
        start(controller) {
          controller.enqueue(
            new TextEncoder().encode(
              `data: ${JSON.stringify(mockResponse)}\n\n`,
            ),
          );
          controller.close();
        },
      });

      mockFetch.mockResolvedValueOnce({
        ok: true,
        body: mockStream,
      });

      await aiApi.streamGenerateFlow("Test prompt", undefined, undefined, {
        signal: abortController.signal,
      });

      expect(mockFetch).toHaveBeenCalledWith(
        "/ai/stream-generate-flow",
        expect.objectContaining({
          signal: abortController.signal,
        }),
      );
    });

    it("should work without abort signal", async () => {
      const mockResponse = {
        flow: createMockFlow(),
        validationResult: createMockValidationResult(),
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => mockResponse,
      });

      const result = await aiApi.generateFlow("Test prompt");

      expect(result).toEqual(mockResponse);
      expect(mockFetch).toHaveBeenCalledWith(
        "/ai/generate-flow",
        expect.objectContaining({
          signal: undefined,
        }),
      );
    });
  });

  // =========================================================================
  // Edge Cases Tests
  // =========================================================================

  describe("Edge Cases", () => {
    it("should handle concurrent requests", async () => {
      const mockResponse = {
        flow: createMockFlow(),
        validationResult: createMockValidationResult(),
      };

      mockFetch.mockResolvedValue({
        ok: true,
        json: async () => mockResponse,
      });

      const promises = [
        aiApi.generateFlow("Prompt 1"),
        aiApi.generateFlow("Prompt 2"),
        aiApi.generateFlow("Prompt 3"),
      ];

      const results = await Promise.all(promises);

      expect(results).toHaveLength(3);
      expect(mockFetch).toHaveBeenCalledTimes(3);
    });

    it("should handle response with missing flow", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          validationResult: createMockValidationResult(),
          // Missing flow field
        }),
      });

      const result = await aiApi.generateFlow("Test prompt");

      expect(result.flow).toBeUndefined();
    });

    it("should handle response with missing validationResult", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          flow: createMockFlow(),
          // Missing validationResult
        }),
      });

      const result = await aiApi.generateFlow("Test prompt");

      expect(result.validationResult).toBeUndefined();
    });

    it("should handle error response without details", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        statusText: "Internal Server Error",
        json: async () => ({}),
      });

      try {
        await aiApi.generateFlow("Test prompt");
        expect.fail("Should have thrown AiApiError");
      } catch (error) {
        expect(error).toBeInstanceOf(AiApiError);
        expect((error as AiApiError).details).toEqual({});
      }
    });

    it("should handle non-JSON error response", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        statusText: "Internal Server Error",
        json: async () => {
          throw new Error("Not JSON");
        },
      });

      try {
        await aiApi.generateFlow("Test prompt");
        expect.fail("Should have thrown error");
      } catch (error) {
        expect(error).toBeInstanceOf(AiApiError);
      }
    });

    it("should handle Unicode characters in prompt", async () => {
      const mockResponse = {
        flow: createMockFlow(),
        validationResult: createMockValidationResult(),
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => mockResponse,
      });

      const unicodePrompt = "Create flow with 中文字符 and émojis 🚀";
      const result = await aiApi.generateFlow(unicodePrompt);

      expect(result).toBeDefined();
    });

    it("should handle very large flow responses", async () => {
      const largeFlow: Flow = {
        ...createMockFlow(),
        nodes: Array.from({ length: 1000 }, (_, i) => ({
          id: `node-${i}`,
          component: "test-component",
          type: "processor",
          name: `Component ${i}`,
          position: { x: i * 100, y: i * 100 },
          config: {},
        })),
        connections: Array.from({ length: 999 }, (_, i) => ({
          id: `conn-${i}`,
          source_node_id: `node-${i}`,
          source_port: "output",
          target_node_id: `node-${i + 1}`,
          target_port: "input",
        })),
      };

      const mockResponse = {
        flow: largeFlow,
        validationResult: createMockValidationResult(),
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => mockResponse,
      });

      const result = await aiApi.generateFlow("Test prompt");

      expect(result.flow.nodes).toHaveLength(1000);
      expect(result.flow.connections).toHaveLength(999);
    });
  });
});
