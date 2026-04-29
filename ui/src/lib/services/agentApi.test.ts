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
  // submitApproval
  // =========================================================================

  describe("submitApproval", () => {
    it("POSTs to /teams-dispatch/loops/{id}/approval with X-User-Id and JSON body", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        statusText: "OK",
        json: async () => ({
          loop_id: "loop-1",
          decision: "approve",
          accepted: true,
          message: "Approval 'approve' submitted for loop loop-1",
          timestamp: "2026-04-29T10:30:00Z",
        }),
      });

      const result = await agentApi.submitApproval("loop-1", {
        decision: "approve",
      });

      expect(mockFetch).toHaveBeenCalledTimes(1);
      const [url, init] = mockFetch.mock.calls[0];
      expect(url).toBe("/teams-dispatch/loops/loop-1/approval");
      expect(init.method).toBe("POST");
      expect(init.headers["Content-Type"]).toBe("application/json");
      // The default identity is "ui-anonymous" — both header and body
      // carry it so the request resolves with or without the product
      // middleware deployed.
      expect(init.headers["X-User-Id"]).toBe("ui-anonymous");
      expect(JSON.parse(init.body)).toEqual({
        decision: "approve",
        user_id: "ui-anonymous",
      });
      expect(result.accepted).toBe(true);
      expect(result.decision).toBe("approve");
    });

    it("forwards modified_arguments and reason for the modify path", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        statusText: "OK",
        json: async () => ({
          loop_id: "loop-2",
          decision: "modify",
          accepted: true,
          timestamp: "2026-04-29T10:31:00Z",
        }),
      });

      await agentApi.submitApproval("loop-2", {
        decision: "modify",
        modified_arguments: { path: "/tmp/safe" },
        reason: "narrow scope",
      });

      const [, init] = mockFetch.mock.calls[0];
      expect(JSON.parse(init.body)).toEqual({
        decision: "modify",
        modified_arguments: { path: "/tmp/safe" },
        reason: "narrow scope",
        user_id: "ui-anonymous",
      });
    });

    it("body honours explicit user_id; header always reflects the store (middleware seam wins downstream)", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        statusText: "OK",
        json: async () => ({
          loop_id: "loop-3",
          decision: "reject",
          accepted: true,
          timestamp: "2026-04-29T10:32:00Z",
        }),
      });

      await agentApi.submitApproval("loop-3", {
        decision: "reject",
        user_id: "alice@example.com",
      });

      const [, init] = mockFetch.mock.calls[0];
      // Header always reflects the store — the middleware lifts it
      // into ctx, which dominates the body per upstream
      // IdentityFromRequest precedence. Body fallback carries the
      // caller's explicit value for deployments that skip the
      // middleware (then the body becomes the resolution source).
      expect(init.headers["X-User-Id"]).toBe("ui-anonymous");
      expect(JSON.parse(init.body).user_id).toBe("alice@example.com");
    });

    it("treats an empty user_id as absent and falls back to the store value", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        statusText: "OK",
        json: async () => ({
          loop_id: "loop-empty",
          decision: "approve",
          accepted: true,
          timestamp: "2026-04-29T10:33:00Z",
        }),
      });

      await agentApi.submitApproval("loop-empty", {
        decision: "approve",
        user_id: "   ",
      });

      const [, init] = mockFetch.mock.calls[0];
      // Empty / whitespace-only caller user_id collapses to the
      // store value so body and header agree.
      expect(JSON.parse(init.body).user_id).toBe("ui-anonymous");
    });

    it("throws AgentApiError on 404 unknown loop", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 404,
        statusText: "Not Found",
        json: async () => ({ error: "loop not tracked" }),
      });

      await expect(
        agentApi.submitApproval("loop-unknown", { decision: "approve" }),
      ).rejects.toMatchObject({
        name: "AgentApiError",
        statusCode: 404,
        message: expect.stringContaining("Failed to submit approval"),
      });
    });

    it("throws AgentApiError on 409 stale state", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 409,
        statusText: "Conflict",
        json: async () => ({ error: "loop is not awaiting approval" }),
      });

      await expect(
        agentApi.submitApproval("loop-stale", { decision: "approve" }),
      ).rejects.toMatchObject({
        name: "AgentApiError",
        statusCode: 409,
      });
    });

    it("throws AgentApiError on 400 bad body", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 400,
        statusText: "Bad Request",
        json: async () => ({ error: "unknown decision value" }),
      });

      await expect(
        agentApi.submitApproval("loop-1", {
          decision: "approve",
        }),
      ).rejects.toBeInstanceOf(AgentApiError);
    });

    it("tolerates a non-JSON error body without throwing in the catch", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        statusText: "Internal Server Error",
        json: async () => {
          throw new Error("not json");
        },
      });

      await expect(
        agentApi.submitApproval("loop-1", { decision: "approve" }),
      ).rejects.toMatchObject({
        statusCode: 500,
      });
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
