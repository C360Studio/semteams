import { describe, it, expect } from "vitest";
import type {
  MessageAttachment,
  HealthAttachment,
  FlowStatusAttachment,
  ErrorAttachment,
} from "$lib/types/chat";

// ---------------------------------------------------------------------------
// HealthAttachment — discriminated union shape
// ---------------------------------------------------------------------------

describe("HealthAttachment — type contract", () => {
  it("can be constructed with required fields only", () => {
    const att: HealthAttachment = {
      kind: "health",
      componentName: "http-input",
      status: "healthy",
    };

    expect(att.kind).toBe("health");
    expect(att.componentName).toBe("http-input");
    expect(att.status).toBe("healthy");
  });

  it("can be constructed with all optional fields", () => {
    const att: HealthAttachment = {
      kind: "health",
      componentName: "kafka-output",
      status: "degraded",
      message: "Reconnecting",
      metrics: { messages_per_second: 42 },
      lastCheck: "2026-03-09T14:00:00Z",
    };

    expect(att.message).toBe("Reconnecting");
    expect(att.metrics?.messages_per_second).toBe(42);
    expect(att.lastCheck).toBe("2026-03-09T14:00:00Z");
  });

  it("kind discriminant is the literal string 'health'", () => {
    const att: HealthAttachment = {
      kind: "health",
      componentName: "test",
      status: "healthy",
    };

    // TypeScript ensures this at compile time; runtime check confirms it
    expect(att.kind).toBe("health");
  });

  it("status 'healthy' is a valid value", () => {
    const att: HealthAttachment = {
      kind: "health",
      componentName: "test",
      status: "healthy",
    };
    expect(att.status).toBe("healthy");
  });

  it("status 'degraded' is a valid value", () => {
    const att: HealthAttachment = {
      kind: "health",
      componentName: "test",
      status: "degraded",
    };
    expect(att.status).toBe("degraded");
  });

  it("status 'unhealthy' is a valid value", () => {
    const att: HealthAttachment = {
      kind: "health",
      componentName: "test",
      status: "unhealthy",
    };
    expect(att.status).toBe("unhealthy");
  });

  it("status 'unknown' is a valid value", () => {
    const att: HealthAttachment = {
      kind: "health",
      componentName: "test",
      status: "unknown",
    };
    expect(att.status).toBe("unknown");
  });

  it("metrics is a Record<string, number>", () => {
    const att: HealthAttachment = {
      kind: "health",
      componentName: "test",
      status: "healthy",
      metrics: { throughput: 100, latency_ms: 5.5 },
    };

    expect(typeof att.metrics?.throughput).toBe("number");
    expect(typeof att.metrics?.latency_ms).toBe("number");
  });
});

// ---------------------------------------------------------------------------
// FlowStatusAttachment — discriminated union shape
// ---------------------------------------------------------------------------

describe("FlowStatusAttachment — type contract", () => {
  it("can be constructed with required fields only", () => {
    const att: FlowStatusAttachment = {
      kind: "flow-status",
      flowId: "flow-abc",
      flowName: "My Pipeline",
      state: "running",
      nodeCount: 3,
      connectionCount: 2,
    };

    expect(att.kind).toBe("flow-status");
    expect(att.flowId).toBe("flow-abc");
    expect(att.flowName).toBe("My Pipeline");
    expect(att.state).toBe("running");
    expect(att.nodeCount).toBe(3);
    expect(att.connectionCount).toBe(2);
  });

  it("can be constructed with warnings", () => {
    const att: FlowStatusAttachment = {
      kind: "flow-status",
      flowId: "flow-abc",
      flowName: "My Pipeline",
      state: "running",
      nodeCount: 2,
      connectionCount: 1,
      warnings: ["High error rate", "Low throughput"],
    };

    expect(att.warnings).toHaveLength(2);
    expect(att.warnings?.[0]).toBe("High error rate");
  });

  it("kind discriminant is the literal string 'flow-status'", () => {
    const att: FlowStatusAttachment = {
      kind: "flow-status",
      flowId: "flow-1",
      flowName: "Test",
      state: "stopped",
      nodeCount: 0,
      connectionCount: 0,
    };

    expect(att.kind).toBe("flow-status");
  });

  it("nodeCount and connectionCount are numbers", () => {
    const att: FlowStatusAttachment = {
      kind: "flow-status",
      flowId: "flow-1",
      flowName: "Test",
      state: "running",
      nodeCount: 5,
      connectionCount: 4,
    };

    expect(typeof att.nodeCount).toBe("number");
    expect(typeof att.connectionCount).toBe("number");
  });

  it("state is a string (accepts any runtime state value)", () => {
    const states = ["running", "stopped", "error", "deploying", "not_deployed"];

    for (const state of states) {
      const att: FlowStatusAttachment = {
        kind: "flow-status",
        flowId: "flow-1",
        flowName: "Test",
        state,
        nodeCount: 0,
        connectionCount: 0,
      };
      expect(att.state).toBe(state);
    }
  });
});

// ---------------------------------------------------------------------------
// MessageAttachment union — includes new kinds
// ---------------------------------------------------------------------------

describe("MessageAttachment union — includes health and flow-status", () => {
  it("HealthAttachment is assignable to MessageAttachment", () => {
    const health: HealthAttachment = {
      kind: "health",
      componentName: "test",
      status: "healthy",
    };

    // Type assertion: if this compiles, the union includes HealthAttachment
    const att: MessageAttachment = health;
    expect(att.kind).toBe("health");
  });

  it("FlowStatusAttachment is assignable to MessageAttachment", () => {
    const flowStatus: FlowStatusAttachment = {
      kind: "flow-status",
      flowId: "flow-1",
      flowName: "Test",
      state: "running",
      nodeCount: 2,
      connectionCount: 1,
    };

    const att: MessageAttachment = flowStatus;
    expect(att.kind).toBe("flow-status");
  });

  it("ErrorAttachment is still assignable to MessageAttachment", () => {
    const error: ErrorAttachment = {
      kind: "error",
      code: "TEST_ERROR",
      message: "Something went wrong",
    };

    const att: MessageAttachment = error;
    expect(att.kind).toBe("error");
  });

  it("MessageAttachment array can hold mixed kinds", () => {
    const attachments: MessageAttachment[] = [
      { kind: "health", componentName: "test", status: "healthy" },
      {
        kind: "flow-status",
        flowId: "f1",
        flowName: "Test",
        state: "running",
        nodeCount: 1,
        connectionCount: 0,
      },
      { kind: "error", code: "ERR", message: "oops" },
    ];

    expect(attachments).toHaveLength(3);
    expect(attachments[0].kind).toBe("health");
    expect(attachments[1].kind).toBe("flow-status");
    expect(attachments[2].kind).toBe("error");
  });
});

// ---------------------------------------------------------------------------
// Discriminated union narrowing
// ---------------------------------------------------------------------------

describe("MessageAttachment — narrowing by kind works for all six kinds", () => {
  it("narrowing to 'health' gives access to componentName and status", () => {
    const att: MessageAttachment = {
      kind: "health",
      componentName: "my-component",
      status: "degraded",
    };

    if (att.kind === "health") {
      expect(att.componentName).toBe("my-component");
      expect(att.status).toBe("degraded");
    } else {
      throw new Error("Expected kind to be 'health'");
    }
  });

  it("narrowing to 'flow-status' gives access to flowId and state", () => {
    const att: MessageAttachment = {
      kind: "flow-status",
      flowId: "flow-narrow",
      flowName: "Narrow Test",
      state: "stopped",
      nodeCount: 2,
      connectionCount: 1,
    };

    if (att.kind === "flow-status") {
      expect(att.flowId).toBe("flow-narrow");
      expect(att.state).toBe("stopped");
    } else {
      throw new Error("Expected kind to be 'flow-status'");
    }
  });

  it("switch statement exhaustively covers all known kinds", () => {
    const allKinds: MessageAttachment[] = [
      { kind: "flow", flow: {} },
      { kind: "search-result", query: "test" },
      { kind: "entity-detail" },
      { kind: "error", code: "E", message: "m" },
      { kind: "health", componentName: "c", status: "healthy" },
      {
        kind: "flow-status",
        flowId: "f",
        flowName: "F",
        state: "running",
        nodeCount: 0,
        connectionCount: 0,
      },
    ];

    const kinds: string[] = [];
    for (const att of allKinds) {
      switch (att.kind) {
        case "flow":
          kinds.push("flow");
          break;
        case "search-result":
          kinds.push("search-result");
          break;
        case "entity-detail":
          kinds.push("entity-detail");
          break;
        case "error":
          kinds.push("error");
          break;
        case "health":
          kinds.push("health");
          break;
        case "flow-status":
          kinds.push("flow-status");
          break;
        default:
          // TypeScript exhaustiveness check: this should never execute
          kinds.push("unknown");
      }
    }

    expect(kinds).toEqual([
      "flow",
      "search-result",
      "entity-detail",
      "error",
      "health",
      "flow-status",
    ]);
  });
});

// ---------------------------------------------------------------------------
// HealthAttachment — edge cases
// ---------------------------------------------------------------------------

describe("HealthAttachment — edge cases", () => {
  it("metrics can be an empty object", () => {
    const att: HealthAttachment = {
      kind: "health",
      componentName: "test",
      status: "healthy",
      metrics: {},
    };

    expect(att.metrics).toBeDefined();
    expect(Object.keys(att.metrics!)).toHaveLength(0);
  });

  it("metrics values are numbers (not strings or booleans)", () => {
    const att: HealthAttachment = {
      kind: "health",
      componentName: "test",
      status: "healthy",
      metrics: { count: 100, rate: 1.5 },
    };

    for (const value of Object.values(att.metrics!)) {
      expect(typeof value).toBe("number");
    }
  });
});

// ---------------------------------------------------------------------------
// FlowStatusAttachment — edge cases
// ---------------------------------------------------------------------------

describe("FlowStatusAttachment — edge cases", () => {
  it("warnings can be an empty array", () => {
    const att: FlowStatusAttachment = {
      kind: "flow-status",
      flowId: "f",
      flowName: "F",
      state: "running",
      nodeCount: 1,
      connectionCount: 0,
      warnings: [],
    };

    expect(att.warnings).toBeDefined();
    expect(att.warnings).toHaveLength(0);
  });

  it("nodeCount of 0 is valid", () => {
    const att: FlowStatusAttachment = {
      kind: "flow-status",
      flowId: "f",
      flowName: "Empty",
      state: "not_deployed",
      nodeCount: 0,
      connectionCount: 0,
    };

    expect(att.nodeCount).toBe(0);
  });
});
