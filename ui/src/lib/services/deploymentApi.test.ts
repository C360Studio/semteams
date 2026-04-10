import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { deploymentApi, DeploymentApiError } from "./deploymentApi";

describe("deploymentApi", () => {
  // Mock fetch globally
  const mockFetch = vi.fn();
  globalThis.fetch = mockFetch;

  beforeEach(() => {
    mockFetch.mockClear();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe("deployFlow", () => {
    it("should deploy a flow", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
      });

      await deploymentApi.deployFlow("flow-123");

      expect(mockFetch).toHaveBeenCalledWith(
        "/flowbuilder/deployment/flow-123/deploy",
        {
          method: "POST",
        },
      );
    });

    it("should handle deploy errors", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 400,
        statusText: "Bad Request",
        json: async () => ({ error: "Invalid flow state" }),
      });

      try {
        await deploymentApi.deployFlow("flow-123");
        expect.fail("Should have thrown DeploymentApiError");
      } catch (error) {
        expect(error).toBeInstanceOf(DeploymentApiError);
        expect((error as DeploymentApiError).statusCode).toBe(400);
        expect((error as DeploymentApiError).operation).toBe("deploy");
        expect((error as DeploymentApiError).details).toEqual({
          error: "Invalid flow state",
        });
      }
    });

    it("should handle flow not found", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 404,
        statusText: "Not Found",
        json: async () => ({}),
      });

      try {
        await deploymentApi.deployFlow("nonexistent");
        expect.fail("Should have thrown DeploymentApiError");
      } catch (error) {
        expect(error).toBeInstanceOf(DeploymentApiError);
        expect((error as DeploymentApiError).statusCode).toBe(404);
      }
    });
  });

  describe("startFlow", () => {
    it("should start a deployed flow", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
      });

      await deploymentApi.startFlow("flow-123");

      expect(mockFetch).toHaveBeenCalledWith(
        "/flowbuilder/deployment/flow-123/start",
        {
          method: "POST",
        },
      );
    });

    it("should handle start errors (flow not deployed)", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 400,
        statusText: "Bad Request",
        json: async () => ({ error: "Flow must be deployed before starting" }),
      });

      try {
        await deploymentApi.startFlow("flow-123");
        expect.fail("Should have thrown DeploymentApiError");
      } catch (error) {
        expect(error).toBeInstanceOf(DeploymentApiError);
        expect((error as DeploymentApiError).statusCode).toBe(400);
        expect((error as DeploymentApiError).operation).toBe("start");
        expect((error as DeploymentApiError).message).toContain(
          "Failed to start flow",
        );
      }
    });

    it("should handle already running flow", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 409,
        statusText: "Conflict",
        json: async () => ({ error: "Flow is already running" }),
      });

      try {
        await deploymentApi.startFlow("flow-123");
        expect.fail("Should have thrown DeploymentApiError");
      } catch (error) {
        expect(error).toBeInstanceOf(DeploymentApiError);
        expect((error as DeploymentApiError).statusCode).toBe(409);
      }
    });
  });

  describe("stopFlow", () => {
    it("should stop a running flow", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
      });

      await deploymentApi.stopFlow("flow-123");

      expect(mockFetch).toHaveBeenCalledWith(
        "/flowbuilder/deployment/flow-123/stop",
        {
          method: "POST",
        },
      );
    });

    it("should handle stop errors (flow not running)", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 400,
        statusText: "Bad Request",
        json: async () => ({ error: "Flow is not running" }),
      });

      try {
        await deploymentApi.stopFlow("flow-123");
        expect.fail("Should have thrown DeploymentApiError");
      } catch (error) {
        expect(error).toBeInstanceOf(DeploymentApiError);
        expect((error as DeploymentApiError).statusCode).toBe(400);
        expect((error as DeploymentApiError).operation).toBe("stop");
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

      try {
        await deploymentApi.stopFlow("flow-123");
        expect.fail("Should have thrown DeploymentApiError");
      } catch (error) {
        expect(error).toBeInstanceOf(DeploymentApiError);
        expect((error as DeploymentApiError).statusCode).toBe(500);
      }
    });
  });

  describe("undeployFlow", () => {
    it("should undeploy a stopped flow", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
      });

      await deploymentApi.undeployFlow("flow-123");

      expect(mockFetch).toHaveBeenCalledWith(
        "/flowbuilder/deployment/flow-123/undeploy",
        {
          method: "POST",
        },
      );
    });

    it("should handle undeploy errors (flow still running)", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 400,
        statusText: "Bad Request",
        json: async () => ({ error: "Cannot undeploy running flow" }),
      });

      try {
        await deploymentApi.undeployFlow("flow-123");
        expect.fail("Should have thrown DeploymentApiError");
      } catch (error) {
        expect(error).toBeInstanceOf(DeploymentApiError);
        expect((error as DeploymentApiError).statusCode).toBe(400);
        expect((error as DeploymentApiError).operation).toBe("undeploy");
        expect((error as DeploymentApiError).details).toEqual({
          error: "Cannot undeploy running flow",
        });
      }
    });

    it("should handle flow not found", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 404,
        statusText: "Not Found",
        json: async () => ({}),
      });

      try {
        await deploymentApi.undeployFlow("nonexistent");
        expect.fail("Should have thrown DeploymentApiError");
      } catch (error) {
        expect(error).toBeInstanceOf(DeploymentApiError);
        expect((error as DeploymentApiError).statusCode).toBe(404);
      }
    });
  });

  describe("DeploymentApiError", () => {
    it("should create error with message, operation, and status code", () => {
      const error = new DeploymentApiError("Deploy failed", "deploy", 500);

      expect(error.message).toBe("Deploy failed");
      expect(error.operation).toBe("deploy");
      expect(error.statusCode).toBe(500);
      expect(error.details).toBeUndefined();
      expect(error.name).toBe("DeploymentApiError");
    });

    it("should create error with details", () => {
      const details = { reason: "Invalid state transition" };
      const error = new DeploymentApiError(
        "Operation failed",
        "start",
        400,
        details,
      );

      expect(error.message).toBe("Operation failed");
      expect(error.operation).toBe("start");
      expect(error.statusCode).toBe(400);
      expect(error.details).toEqual(details);
    });

    it("should be instanceof Error", () => {
      const error = new DeploymentApiError("Test", "deploy", 500);

      expect(error).toBeInstanceOf(Error);
      expect(error).toBeInstanceOf(DeploymentApiError);
    });
  });
});
