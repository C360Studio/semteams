import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { agentApi, AgentApiError } from "./agentApi";
import type { AgentLoop, TrajectoryEntry } from "$lib/types/agent";

describe("agentApi", () => {
  const mockFetch = vi.fn();
  const originalFetch = globalThis.fetch;

  beforeEach(() => {
    mockFetch.mockClear();
    globalThis.fetch = mockFetch;
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  // Helper to create a mock AgentLoop
  const createMockLoop = (overrides?: Partial<AgentLoop>): AgentLoop => ({
    loop_id: "loop-1",
    task_id: "task-1",
    state: "exploring",
    role: "architect",
    iterations: 1,
    max_iterations: 10,
    user_id: "user-1",
    channel_type: "chat",
    parent_loop_id: "",
    outcome: "",
    error: "",
    ...overrides,
  });

  // Helper to create a mock TrajectoryEntry
  const createMockTrajectory = (
    overrides?: Partial<TrajectoryEntry>,
  ): TrajectoryEntry => ({
    loop_id: "loop-1",
    role: "architect",
    iterations: 5,
    outcome: "success",
    duration_ms: 12000,
    ...overrides,
  });

  // =========================================================================
  // sendMessage
  // =========================================================================

  describe("sendMessage", () => {
    it("should POST to /teams-dispatch/message with JSON body", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({ content: "Hello from agent" }),
      });

      const result = await agentApi.sendMessage("Hello");

      expect(mockFetch).toHaveBeenCalledWith("/teams-dispatch/message", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ content: "Hello" }),
      });
      expect(result).toEqual({ content: "Hello from agent" });
    });

    it("should throw AgentApiError on failure", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        statusText: "Internal Server Error",
        json: async () => ({ error: "Backend down" }),
      });

      try {
        await agentApi.sendMessage("Hello");
        expect.fail("Should have thrown AgentApiError");
      } catch (error) {
        expect(error).toBeInstanceOf(AgentApiError);
        expect((error as AgentApiError).statusCode).toBe(500);
        expect((error as AgentApiError).details).toEqual({
          error: "Backend down",
        });
      }
    });

    it("should handle malformed error JSON gracefully", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 502,
        statusText: "Bad Gateway",
        json: async () => {
          throw new Error("Invalid JSON");
        },
      });

      await expect(agentApi.sendMessage("Hello")).rejects.toThrow(
        AgentApiError,
      );
    });
  });

  // =========================================================================
  // listLoops
  // =========================================================================

  describe("listLoops", () => {
    it("should GET /teams-dispatch/loops", async () => {
      const loops = [createMockLoop(), createMockLoop({ loop_id: "loop-2" })];
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => loops,
      });

      const result = await agentApi.listLoops();

      expect(mockFetch).toHaveBeenCalledWith("/teams-dispatch/loops");
      expect(result).toEqual(loops);
    });

    it("should throw AgentApiError on failure", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 503,
        statusText: "Service Unavailable",
      });

      try {
        await agentApi.listLoops();
        expect.fail("Should have thrown AgentApiError");
      } catch (error) {
        expect(error).toBeInstanceOf(AgentApiError);
        expect((error as AgentApiError).statusCode).toBe(503);
        expect((error as AgentApiError).message).toContain(
          "Failed to list loops",
        );
      }
    });

    it("should return empty array for no loops", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => [],
      });

      const result = await agentApi.listLoops();
      expect(result).toEqual([]);
    });
  });

  // =========================================================================
  // getLoop
  // =========================================================================

  describe("getLoop", () => {
    it("should GET /teams-dispatch/loops/:id", async () => {
      const loop = createMockLoop();
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => loop,
      });

      const result = await agentApi.getLoop("loop-1");

      expect(mockFetch).toHaveBeenCalledWith("/teams-dispatch/loops/loop-1");
      expect(result).toEqual(loop);
    });

    it("should throw AgentApiError on 404", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 404,
        statusText: "Not Found",
      });

      try {
        await agentApi.getLoop("nonexistent");
        expect.fail("Should have thrown AgentApiError");
      } catch (error) {
        expect(error).toBeInstanceOf(AgentApiError);
        expect((error as AgentApiError).statusCode).toBe(404);
      }
    });
  });

  // =========================================================================
  // sendSignal
  // =========================================================================

  describe("sendSignal", () => {
    it("should POST signal without reason", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          loop_id: "loop-1",
          signal: "pause",
          status: "accepted",
        }),
      });

      const result = await agentApi.sendSignal("loop-1", "pause");

      expect(mockFetch).toHaveBeenCalledWith(
        "/teams-dispatch/loops/loop-1/signal",
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ type: "pause" }),
        },
      );
      expect(result).toEqual({
        loop_id: "loop-1",
        signal: "pause",
        status: "accepted",
      });
    });

    it("should POST signal with reason", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          loop_id: "loop-1",
          signal: "reject",
          status: "accepted",
        }),
      });

      const result = await agentApi.sendSignal(
        "loop-1",
        "reject",
        "Does not meet requirements",
      );

      expect(mockFetch).toHaveBeenCalledWith(
        "/teams-dispatch/loops/loop-1/signal",
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            type: "reject",
            reason: "Does not meet requirements",
          }),
        },
      );
      expect(result).toEqual({
        loop_id: "loop-1",
        signal: "reject",
        status: "accepted",
      });
    });

    it("should throw AgentApiError on failure", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 400,
        statusText: "Bad Request",
        json: async () => ({ error: "Invalid signal for current state" }),
      });

      try {
        await agentApi.sendSignal("loop-1", "approve");
        expect.fail("Should have thrown AgentApiError");
      } catch (error) {
        expect(error).toBeInstanceOf(AgentApiError);
        expect((error as AgentApiError).statusCode).toBe(400);
        expect((error as AgentApiError).details).toEqual({
          error: "Invalid signal for current state",
        });
      }
    });

    it("should handle malformed error JSON gracefully", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        statusText: "Internal Server Error",
        json: async () => {
          throw new Error("Invalid JSON");
        },
      });

      await expect(agentApi.sendSignal("loop-1", "cancel")).rejects.toThrow(
        AgentApiError,
      );
    });
  });

  // =========================================================================
  // getTrajectories
  // =========================================================================

  describe("getTrajectories", () => {
    it("should GET /teams-loop/trajectories", async () => {
      const trajectories = [
        createMockTrajectory(),
        createMockTrajectory({ loop_id: "loop-2" }),
      ];
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => trajectories,
      });

      const result = await agentApi.getTrajectories();

      expect(mockFetch).toHaveBeenCalledWith("/teams-loop/trajectories");
      expect(result).toEqual(trajectories);
    });

    it("should throw AgentApiError on failure", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        statusText: "Internal Server Error",
      });

      try {
        await agentApi.getTrajectories();
        expect.fail("Should have thrown AgentApiError");
      } catch (error) {
        expect(error).toBeInstanceOf(AgentApiError);
        expect((error as AgentApiError).statusCode).toBe(500);
        expect((error as AgentApiError).message).toContain(
          "Failed to get trajectories",
        );
      }
    });
  });

  // =========================================================================
  // getTrajectory
  // =========================================================================

  describe("getTrajectory", () => {
    it("should GET /teams-loop/trajectories/:loopId", async () => {
      const trajectory = createMockTrajectory();
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => trajectory,
      });

      const result = await agentApi.getTrajectory("loop-1");

      expect(mockFetch).toHaveBeenCalledWith(
        "/teams-loop/trajectories/loop-1",
      );
      expect(result).toEqual(trajectory);
    });

    it("should throw AgentApiError on 404", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 404,
        statusText: "Not Found",
      });

      try {
        await agentApi.getTrajectory("nonexistent");
        expect.fail("Should have thrown AgentApiError");
      } catch (error) {
        expect(error).toBeInstanceOf(AgentApiError);
        expect((error as AgentApiError).statusCode).toBe(404);
      }
    });
  });

  // =========================================================================
  // AgentApiError class
  // =========================================================================

  describe("AgentApiError", () => {
    it("should create error with message and status code", () => {
      const error = new AgentApiError("Test error", 500);

      expect(error.message).toBe("Test error");
      expect(error.statusCode).toBe(500);
      expect(error.details).toBeUndefined();
      expect(error.name).toBe("AgentApiError");
    });

    it("should create error with details", () => {
      const details = { field: "type", reason: "invalid" };
      const error = new AgentApiError("Validation failed", 400, details);

      expect(error.message).toBe("Validation failed");
      expect(error.statusCode).toBe(400);
      expect(error.details).toEqual(details);
    });

    it("should be instanceof Error", () => {
      const error = new AgentApiError("Test", 500);

      expect(error).toBeInstanceOf(Error);
      expect(error).toBeInstanceOf(AgentApiError);
    });
  });
});
