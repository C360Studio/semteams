import { describe, it, expect, beforeEach, vi } from "vitest";
import { messagesApi, MessagesApiError } from "./messagesApi";

describe("messagesApi", () => {
  // Mock fetch globally
  const mockFetch = vi.fn();
  globalThis.fetch = mockFetch;

  beforeEach(() => {
    mockFetch.mockClear();
  });

  describe("fetchMessages", () => {
    const flowId = "flow-123";
    const sampleResponse = {
      messages: [
        {
          message_id: "trace-001",
          timestamp: 1705329785123,
          subject: "test.subject",
          direction: "published" as const,
          component: "test-component",
        },
        {
          message_id: "trace-002",
          timestamp: 1705329785456,
          subject: "test.other",
          direction: "received" as const,
          component: "other-component",
        },
      ],
      total: 2,
    };

    it("makes GET request to correct endpoint", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => sampleResponse,
      });

      await messagesApi.fetchMessages(flowId);

      expect(mockFetch).toHaveBeenCalledWith(
        `/flows/${flowId}/runtime/messages`,
        {
          method: "GET",
        },
      );
    });

    it("returns messages on successful response", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => sampleResponse,
      });

      const result = await messagesApi.fetchMessages(flowId);

      expect(result).toEqual(sampleResponse);
      expect(result.messages).toHaveLength(2);
      expect(result.messages[0].message_id).toBe("trace-001");
    });

    it("handles successful response with empty messages array", async () => {
      const emptyResponse = {
        messages: [],
        total: 0,
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => emptyResponse,
      });

      const result = await messagesApi.fetchMessages(flowId);

      expect(result.messages).toEqual([]);
      expect(result.total).toBe(0);
    });

    it("supports optional limit parameter", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => sampleResponse,
      });

      await messagesApi.fetchMessages(flowId, { limit: 50 });

      expect(mockFetch).toHaveBeenCalledWith(
        `/flows/${flowId}/runtime/messages?limit=50`,
        {
          method: "GET",
        },
      );
    });

    it("supports optional offset parameter", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => sampleResponse,
      });

      await messagesApi.fetchMessages(flowId, { offset: 100 });

      expect(mockFetch).toHaveBeenCalledWith(
        `/flows/${flowId}/runtime/messages?offset=100`,
        {
          method: "GET",
        },
      );
    });

    it("supports both limit and offset parameters", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => sampleResponse,
      });

      await messagesApi.fetchMessages(flowId, { limit: 25, offset: 50 });

      expect(mockFetch).toHaveBeenCalledWith(
        `/flows/${flowId}/runtime/messages?limit=25&offset=50`,
        {
          method: "GET",
        },
      );
    });

    it("throws MessagesApiError on 404 not found", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 404,
        statusText: "Not Found",
        json: async () => ({ error: "Flow not found" }),
      });

      try {
        await messagesApi.fetchMessages("nonexistent-flow");
        expect.fail("Should have thrown MessagesApiError");
      } catch (error) {
        expect(error).toBeInstanceOf(MessagesApiError);
        expect((error as MessagesApiError).statusCode).toBe(404);
        expect((error as MessagesApiError).message).toContain(
          "Failed to fetch messages",
        );
        expect((error as MessagesApiError).flowId).toBe("nonexistent-flow");
        expect((error as MessagesApiError).details).toEqual({
          error: "Flow not found",
        });
      }
    });

    it("throws MessagesApiError on 500 internal server error", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        statusText: "Internal Server Error",
        json: async () => ({ error: "Database connection failed" }),
      });

      try {
        await messagesApi.fetchMessages(flowId);
        expect.fail("Should have thrown MessagesApiError");
      } catch (error) {
        expect(error).toBeInstanceOf(MessagesApiError);
        expect((error as MessagesApiError).statusCode).toBe(500);
        expect((error as MessagesApiError).message).toContain(
          "Failed to fetch messages",
        );
      }
    });

    it("throws MessagesApiError on 400 bad request", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 400,
        statusText: "Bad Request",
        json: async () => ({ error: "Invalid query parameters" }),
      });

      try {
        await messagesApi.fetchMessages(flowId);
        expect.fail("Should have thrown MessagesApiError");
      } catch (error) {
        expect(error).toBeInstanceOf(MessagesApiError);
        expect((error as MessagesApiError).statusCode).toBe(400);
        expect((error as MessagesApiError).details).toEqual({
          error: "Invalid query parameters",
        });
      }
    });

    it("handles malformed error responses gracefully", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        statusText: "Internal Server Error",
        json: async () => {
          throw new Error("Invalid JSON");
        },
      });

      try {
        await messagesApi.fetchMessages(flowId);
        expect.fail("Should have thrown MessagesApiError");
      } catch (error) {
        expect(error).toBeInstanceOf(MessagesApiError);
        expect((error as MessagesApiError).statusCode).toBe(500);
        expect((error as MessagesApiError).details).toEqual({});
      }
    });

    it("handles network errors", async () => {
      mockFetch.mockRejectedValueOnce(new Error("Network error"));

      try {
        await messagesApi.fetchMessages(flowId);
        expect.fail("Should have thrown error");
      } catch (error) {
        expect(error).toBeInstanceOf(Error);
        expect((error as Error).message).toContain("Network error");
      }
    });

    it("handles timeout errors", async () => {
      mockFetch.mockRejectedValueOnce(new Error("Request timeout"));

      try {
        await messagesApi.fetchMessages(flowId);
        expect.fail("Should have thrown error");
      } catch (error) {
        expect(error).toBeInstanceOf(Error);
        expect((error as Error).message).toContain("timeout");
      }
    });

    it("does not modify query parameters when none provided", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => sampleResponse,
      });

      await messagesApi.fetchMessages(flowId);

      const callUrl = mockFetch.mock.calls[0][0] as string;
      expect(callUrl).not.toContain("?");
      expect(callUrl).toBe(`/flows/${flowId}/runtime/messages`);
    });

    it("handles zero limit parameter", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({ messages: [], total: 0 }),
      });

      await messagesApi.fetchMessages(flowId, { limit: 0 });

      expect(mockFetch).toHaveBeenCalledWith(
        `/flows/${flowId}/runtime/messages?limit=0`,
        {
          method: "GET",
        },
      );
    });

    it("handles zero offset parameter", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => sampleResponse,
      });

      await messagesApi.fetchMessages(flowId, { offset: 0 });

      expect(mockFetch).toHaveBeenCalledWith(
        `/flows/${flowId}/runtime/messages?offset=0`,
        {
          method: "GET",
        },
      );
    });

    it("returns total count from response", async () => {
      const responseWithTotal = {
        messages: [
          {
            message_id: "trace-001",
            timestamp: 1705329785123,
            subject: "test.subject",
            direction: "published" as const,
            component: "test-component",
          },
        ],
        total: 150,
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => responseWithTotal,
      });

      const result = await messagesApi.fetchMessages(flowId);

      expect(result.total).toBe(150);
      expect(result.messages).toHaveLength(1);
    });

    it("handles response without total field", async () => {
      const responseNoTotal = {
        messages: [
          {
            message_id: "trace-001",
            timestamp: 1705329785123,
            subject: "test.subject",
            direction: "published" as const,
            component: "test-component",
          },
        ],
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => responseNoTotal,
      });

      const result = await messagesApi.fetchMessages(flowId);

      expect(result.messages).toHaveLength(1);
      expect(result.total).toBeUndefined();
    });

    it("preserves all message fields from response", async () => {
      const fullMessageResponse = {
        messages: [
          {
            message_id: "trace-complex",
            timestamp: 1705329785123,
            subject: "complex.message",
            direction: "processed" as const,
            component: "processor-comp",
            payload_size: 1024,
            headers: { "x-custom": "value" },
            metadata: { key: "value" },
          },
        ],
        total: 1,
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => fullMessageResponse,
      });

      const result = await messagesApi.fetchMessages(flowId);

      expect(result.messages[0]).toEqual(fullMessageResponse.messages[0]);
      expect(result.messages[0].payload_size).toBe(1024);
      expect(result.messages[0].headers).toEqual({ "x-custom": "value" });
      expect(result.messages[0].metadata).toEqual({ key: "value" });
    });
  });

  describe("MessagesApiError", () => {
    it("creates error with message, flowId, and status code", () => {
      const error = new MessagesApiError("Fetch failed", "flow-123", 500);

      expect(error.message).toBe("Fetch failed");
      expect(error.flowId).toBe("flow-123");
      expect(error.statusCode).toBe(500);
      expect(error.details).toBeUndefined();
      expect(error.name).toBe("MessagesApiError");
    });

    it("creates error with details", () => {
      const details = { reason: "Invalid flow state" };
      const error = new MessagesApiError(
        "Operation failed",
        "flow-123",
        400,
        details,
      );

      expect(error.message).toBe("Operation failed");
      expect(error.flowId).toBe("flow-123");
      expect(error.statusCode).toBe(400);
      expect(error.details).toEqual(details);
    });

    it("is instanceof Error", () => {
      const error = new MessagesApiError("Test", "flow-123", 500);

      expect(error).toBeInstanceOf(Error);
      expect(error).toBeInstanceOf(MessagesApiError);
    });

    it("preserves error stack trace", () => {
      const error = new MessagesApiError("Test", "flow-123", 500);

      expect(error.stack).toBeDefined();
      expect(error.stack).toContain("MessagesApiError");
    });
  });
});
