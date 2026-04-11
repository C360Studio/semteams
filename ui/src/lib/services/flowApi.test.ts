import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { flowApi, FlowApiError } from "./flowApi";
import type { Flow } from "$lib/types/flow";

describe("flowApi", () => {
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
    name: "Test Flow",
    description: "Test description",
    nodes: [],
    connections: [],
    runtime_state: "not_deployed",
    created_at: "2025-10-10T12:00:00Z",
    updated_at: "2025-10-10T12:00:00Z",
    created_by: "user-123",
    last_modified: "2025-10-10T12:00:00Z",
  });

  describe("createFlow", () => {
    it("should create a new flow", async () => {
      const mockFlow = createMockFlow();
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => mockFlow,
      });

      const result = await flowApi.createFlow({
        name: "Test Flow",
        description: "Test description",
      });

      expect(mockFetch).toHaveBeenCalledWith("/flowbuilder/flows", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          name: "Test Flow",
          description: "Test description",
        }),
      });
      expect(result).toEqual(mockFlow);
    });

    it("should handle creation errors", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 400,
        statusText: "Bad Request",
        json: async () => ({ error: "Invalid name" }),
      });

      try {
        await flowApi.createFlow({ name: "" });
        expect.fail("Should have thrown FlowApiError");
      } catch (error) {
        expect(error).toBeInstanceOf(FlowApiError);
        expect((error as FlowApiError).statusCode).toBe(400);
        expect((error as FlowApiError).details).toEqual({
          error: "Invalid name",
        });
      }
    });

    it("should handle malformed error responses", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        statusText: "Internal Server Error",
        json: async () => {
          throw new Error("Invalid JSON");
        },
      });

      await expect(flowApi.createFlow({ name: "Test" })).rejects.toThrow(
        FlowApiError,
      );
    });
  });

  describe("listFlows", () => {
    it("should list all flows", async () => {
      const mockFlows = [
        createMockFlow(),
        { ...createMockFlow(), id: "flow-456" },
      ];
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({ flows: mockFlows }),
      });

      const result = await flowApi.listFlows();

      expect(mockFetch).toHaveBeenCalledWith("/flowbuilder/flows");
      expect(result).toEqual(mockFlows);
    });

    it("should handle list errors", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        statusText: "Internal Server Error",
        json: async () => ({}),
      });

      try {
        await flowApi.listFlows();
        expect.fail("Should have thrown FlowApiError");
      } catch (error) {
        expect(error).toBeInstanceOf(FlowApiError);
        expect((error as FlowApiError).statusCode).toBe(500);
        expect((error as FlowApiError).message).toContain(
          "Failed to list flows",
        );
      }
    });

    it("should handle empty flow list", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({ flows: [] }),
      });

      const result = await flowApi.listFlows();

      expect(result).toEqual([]);
    });
  });

  describe("getFlow", () => {
    it("should get flow by id", async () => {
      const mockFlow = createMockFlow();
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => mockFlow,
      });

      const result = await flowApi.getFlow("flow-123");

      expect(mockFetch).toHaveBeenCalledWith("/flowbuilder/flows/flow-123");
      expect(result).toEqual(mockFlow);
    });

    it("should handle not found errors", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 404,
        statusText: "Not Found",
        json: async () => ({}),
      });

      try {
        await flowApi.getFlow("nonexistent");
        expect.fail("Should have thrown FlowApiError");
      } catch (error) {
        expect(error).toBeInstanceOf(FlowApiError);
        expect((error as FlowApiError).statusCode).toBe(404);
      }
    });
  });

  describe("updateFlow", () => {
    it("should update an existing flow", async () => {
      const mockFlow = createMockFlow();
      const updatedFlow = { ...mockFlow, name: "Updated Flow" };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => updatedFlow,
      });

      const result = await flowApi.updateFlow("flow-123", mockFlow);

      expect(mockFetch).toHaveBeenCalledWith("/flowbuilder/flows/flow-123", {
        method: "PUT",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify(mockFlow),
      });
      expect(result).toEqual(updatedFlow);
    });

    it("should handle version conflicts (409)", async () => {
      const mockFlow = createMockFlow();

      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 409,
        statusText: "Conflict",
        json: async () => ({
          error: "Version conflict",
          currentVersion: "2.0",
        }),
      });

      try {
        await flowApi.updateFlow("flow-123", mockFlow);
        expect.fail("Should have thrown FlowApiError");
      } catch (error) {
        expect(error).toBeInstanceOf(FlowApiError);
        expect((error as FlowApiError).statusCode).toBe(409);
        expect((error as FlowApiError).details).toEqual({
          error: "Version conflict",
          currentVersion: "2.0",
        });
        expect((error as FlowApiError).message).toContain(
          "Failed to update flow",
        );
      }
    });

    it("should handle update errors", async () => {
      const mockFlow = createMockFlow();

      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 400,
        statusText: "Bad Request",
        json: async () => ({ error: "Invalid flow data" }),
      });

      await expect(flowApi.updateFlow("flow-123", mockFlow)).rejects.toThrow(
        FlowApiError,
      );
    });

    it("should handle malformed error responses on update", async () => {
      const mockFlow = createMockFlow();

      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        statusText: "Internal Server Error",
        json: async () => {
          throw new Error("Invalid JSON");
        },
      });

      await expect(flowApi.updateFlow("flow-123", mockFlow)).rejects.toThrow(
        FlowApiError,
      );
    });
  });

  describe("deleteFlow", () => {
    it("should delete a flow", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
      });

      await flowApi.deleteFlow("flow-123");

      expect(mockFetch).toHaveBeenCalledWith("/flowbuilder/flows/flow-123", {
        method: "DELETE",
      });
    });

    it("should handle delete errors", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 404,
        statusText: "Not Found",
        json: async () => ({}),
      });

      try {
        await flowApi.deleteFlow("flow-123");
        expect.fail("Should have thrown FlowApiError");
      } catch (error) {
        expect(error).toBeInstanceOf(FlowApiError);
        expect((error as FlowApiError).statusCode).toBe(404);
        expect((error as FlowApiError).message).toContain(
          "Failed to delete flow",
        );
      }
    });

    it("should handle forbidden delete (flow running)", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 403,
        statusText: "Forbidden",
        json: async () => ({}),
      });

      try {
        await flowApi.deleteFlow("flow-123");
        expect.fail("Should have thrown FlowApiError");
      } catch (error) {
        expect(error).toBeInstanceOf(FlowApiError);
        expect((error as FlowApiError).statusCode).toBe(403);
      }
    });
  });

  describe("FlowApiError", () => {
    it("should create error with message and status code", () => {
      const error = new FlowApiError("Test error", 500);

      expect(error.message).toBe("Test error");
      expect(error.statusCode).toBe(500);
      expect(error.details).toBeUndefined();
      expect(error.name).toBe("FlowApiError");
    });

    it("should create error with details", () => {
      const details = { field: "name", reason: "required" };
      const error = new FlowApiError("Validation failed", 400, details);

      expect(error.message).toBe("Validation failed");
      expect(error.statusCode).toBe(400);
      expect(error.details).toEqual(details);
    });

    it("should be instanceof Error", () => {
      const error = new FlowApiError("Test", 500);

      expect(error).toBeInstanceOf(Error);
      expect(error).toBeInstanceOf(FlowApiError);
    });
  });
});
