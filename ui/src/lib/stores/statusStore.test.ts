import { describe, it, expect, beforeEach } from "vitest";
import { statusStore } from "./statusStore.svelte";

describe("statusStore", () => {
  beforeEach(() => {
    statusStore.reset();
  });

  describe("initial state", () => {
    it("should have not_deployed runtime state", () => {
      expect(statusStore.runtimeState).toBe("not_deployed");
    });
  });

  describe("setRuntimeState", () => {
    it("should update runtime state", () => {
      statusStore.setRuntimeState("running");

      expect(statusStore.runtimeState).toBe("running");
    });

    it("should support all runtime states", () => {
      const states = [
        "not_deployed",
        "deployed_stopped",
        "running",
        "error",
      ] as const;

      states.forEach((state) => {
        statusStore.setRuntimeState(state);
        expect(statusStore.runtimeState).toBe(state);
      });
    });
  });

  describe("updateFromWebSocket", () => {
    it("should update state from flow_status message", () => {
      const message = {
        type: "flow_status" as const,
        flowId: "test-flow-123",
        timestamp: "2025-10-10T12:00:00Z",
        payload: {
          state: "running" as const,
        },
      };

      statusStore.updateFromWebSocket(message);

      expect(statusStore.runtimeState).toBe("running");
    });

    it("should ignore non-flow_status messages", () => {
      statusStore.setRuntimeState("deployed_stopped");

      const message = {
        type: "component_health" as const,
        flowId: "test-flow-123",
        timestamp: "2025-10-10T12:00:00Z",
        payload: {
          componentId: "comp-1",
          health: "healthy",
        },
      };

      statusStore.updateFromWebSocket(message);

      // State should not change
      expect(statusStore.runtimeState).toBe("deployed_stopped");
    });

    it("should handle multiple state transitions", () => {
      // not_deployed → deployed_stopped
      statusStore.updateFromWebSocket({
        type: "flow_status",
        flowId: "test-flow",
        timestamp: "2025-10-10T12:00:00Z",
        payload: { state: "deployed_stopped" },
      });
      expect(statusStore.runtimeState).toBe("deployed_stopped");

      // deployed_stopped → running
      statusStore.updateFromWebSocket({
        type: "flow_status",
        flowId: "test-flow",
        timestamp: "2025-10-10T12:01:00Z",
        payload: { state: "running" },
      });
      expect(statusStore.runtimeState).toBe("running");

      // running → error
      statusStore.updateFromWebSocket({
        type: "flow_status",
        flowId: "test-flow",
        timestamp: "2025-10-10T12:02:00Z",
        payload: { state: "error" },
      });
      expect(statusStore.runtimeState).toBe("error");
    });
  });

  describe("reset", () => {
    it("should reset to not_deployed state", () => {
      statusStore.setRuntimeState("running");
      statusStore.reset();

      expect(statusStore.runtimeState).toBe("not_deployed");
    });
  });

  describe("state reactivity", () => {
    it("should trigger updates when runtime state changes", () => {
      expect(statusStore.runtimeState).toBe("not_deployed");

      statusStore.setRuntimeState("running");

      expect(statusStore.runtimeState).toBe("running");
    });
  });
});
