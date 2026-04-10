import { describe, it, expect, beforeEach } from "vitest";
import { runtimeStore } from "./runtimeStore.svelte";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function addLog(
  level: "DEBUG" | "INFO" | "WARN" | "ERROR",
  msg: string,
  id: string,
) {
  runtimeStore.addLog({ level, source: "test", message: msg }, id, Date.now());
}

// ---------------------------------------------------------------------------
// Reset between tests — store is a singleton
// ---------------------------------------------------------------------------

beforeEach(() => {
  runtimeStore.reset();
});

// ---------------------------------------------------------------------------
// Connection State
// ---------------------------------------------------------------------------

describe("runtimeStore — connection state", () => {
  it("setConnected(true) clears error", () => {
    runtimeStore.setError("oops");
    expect(runtimeStore.error).toBe("oops");
    runtimeStore.setConnected(true);
    expect(runtimeStore.error).toBeNull();
    expect(runtimeStore.connected).toBe(true);
  });

  it("setError with non-null disconnects", () => {
    runtimeStore.setConnected(true);
    runtimeStore.setError("network failure");
    expect(runtimeStore.connected).toBe(false);
    expect(runtimeStore.error).toBe("network failure");
  });

  it("setError(null) does not disconnect", () => {
    runtimeStore.setConnected(true);
    runtimeStore.setError(null);
    expect(runtimeStore.connected).toBe(true);
    expect(runtimeStore.error).toBeNull();
  });

  it("setConnected with flowId sets flowId", () => {
    runtimeStore.setConnected(true, "flow-abc");
    expect(runtimeStore.flowId).toBe("flow-abc");
  });

  it("setConnected without flowId preserves existing flowId", () => {
    runtimeStore.setConnected(true, "flow-xyz");
    runtimeStore.setConnected(false);
    expect(runtimeStore.flowId).toBe("flow-xyz");
  });
});

// ---------------------------------------------------------------------------
// Circular buffer
// ---------------------------------------------------------------------------

describe("runtimeStore — circular log buffer", () => {
  it("keeps at most 1000 logs (MAX_LOGS boundary)", { timeout: 15000 }, () => {
    for (let i = 0; i < 1100; i++) {
      addLog("INFO", `msg-${i}`, `id-${i}`);
    }
    expect(runtimeStore.logs.length).toBe(1000);
  });

  it(
    "retains the most recent entries after overflow",
    { timeout: 15000 },
    () => {
      for (let i = 0; i < 1100; i++) {
        addLog("INFO", `msg-${i}`, `id-${i}`);
      }
      // Oldest surviving log should be msg-100 (entries 0–99 were evicted)
      expect(runtimeStore.logs[0].message).toBe("msg-100");
      // Newest log should be msg-1099
      expect(runtimeStore.logs[999].message).toBe("msg-1099");
    },
  );

  it("clearLogs empties the buffer", () => {
    addLog("ERROR", "boom", "err-1");
    runtimeStore.clearLogs();
    expect(runtimeStore.logs).toHaveLength(0);
  });

  it("addLog with undefined fields does not throw", () => {
    expect(() => {
      runtimeStore.addLog(
        { level: "DEBUG", source: "x", message: "no fields" },
        "id-no-fields",
        Date.now(),
      );
    }).not.toThrow();
    expect(runtimeStore.logs[0].fields).toBeUndefined();
  });
});

// ---------------------------------------------------------------------------
// Health
// ---------------------------------------------------------------------------

describe("runtimeStore — updateComponentHealth", () => {
  it("adds a new component when not present", () => {
    runtimeStore.updateComponentHealth({
      name: "parser",
      status: "healthy",
      message: null,
    });
    expect(runtimeStore.healthComponents).toHaveLength(1);
    expect(runtimeStore.healthComponents[0].name).toBe("parser");
  });

  it("updates existing component without duplicating", () => {
    runtimeStore.updateComponentHealth({
      name: "parser",
      status: "healthy",
      message: null,
    });
    runtimeStore.updateComponentHealth({
      name: "parser",
      status: "error",
      message: "parse failed",
    });
    expect(runtimeStore.healthComponents).toHaveLength(1);
    expect(runtimeStore.healthComponents[0].status).toBe("error");
  });

  it("recalculates overall health correctly with mixed statuses", () => {
    runtimeStore.updateComponentHealth({
      name: "a",
      status: "healthy",
      message: null,
    });
    runtimeStore.updateComponentHealth({
      name: "b",
      status: "degraded",
      message: "slow",
    });
    runtimeStore.updateComponentHealth({
      name: "c",
      status: "error",
      message: "dead",
    });

    expect(runtimeStore.healthOverall?.status).toBe("error");
    expect(runtimeStore.healthOverall?.counts.healthy).toBe(1);
    expect(runtimeStore.healthOverall?.counts.degraded).toBe(1);
    expect(runtimeStore.healthOverall?.counts.error).toBe(1);
  });

  it("overall becomes healthy when all components are healthy", () => {
    runtimeStore.updateComponentHealth({
      name: "a",
      status: "error",
      message: "bad",
    });
    runtimeStore.updateComponentHealth({
      name: "a",
      status: "healthy",
      message: null,
    });
    expect(runtimeStore.healthOverall?.status).toBe("healthy");
  });

  it("overall is degraded when no errors but some degraded", () => {
    runtimeStore.updateComponentHealth({
      name: "a",
      status: "healthy",
      message: null,
    });
    runtimeStore.updateComponentHealth({
      name: "b",
      status: "degraded",
      message: "warn",
    });
    expect(runtimeStore.healthOverall?.status).toBe("degraded");
  });
});

// ---------------------------------------------------------------------------
// Metrics — rate calculation
// ---------------------------------------------------------------------------

describe("runtimeStore — metrics rate calculation", () => {
  it("rate is null before second data point", () => {
    runtimeStore.updateMetrics(
      {
        component: "parser",
        name: "bytes_in",
        type: "counter",
        value: 100,
        labels: {},
      },
      1000,
    );
    expect(runtimeStore.getMetricRate("parser", "bytes_in")).toBeNull();
  });

  it("calculates rate correctly across two data points", () => {
    // NOTE: prevTimestamp=0 is falsy so the rate guard `if (prevMetric && prevTimestamp ...)`
    // rejects it. Use timestamps > 0 to exercise the rate path.
    runtimeStore.updateMetrics(
      {
        component: "parser",
        name: "bytes_in",
        type: "counter",
        value: 0,
        labels: {},
      },
      1000, // t=1s (non-zero so it passes the truthiness check on second call)
    );
    runtimeStore.updateMetrics(
      {
        component: "parser",
        name: "bytes_in",
        type: "counter",
        value: 1000,
        labels: {},
      },
      2000, // t=2s, delta = 1s
    );
    // 1000 bytes / 1 second = 1000 bytes/sec
    expect(runtimeStore.getMetricRate("parser", "bytes_in")).toBe(1000);
  });

  it("clamps rate to zero when counter decreases (reset)", () => {
    runtimeStore.updateMetrics(
      {
        component: "parser",
        name: "bytes_in",
        type: "counter",
        value: 500,
        labels: {},
      },
      1000,
    );
    runtimeStore.updateMetrics(
      {
        component: "parser",
        name: "bytes_in",
        type: "counter",
        value: 0,
        labels: {},
      },
      2000,
    );
    expect(runtimeStore.getMetricRate("parser", "bytes_in")).toBe(0);
  });

  it("getMetricsArray returns all raw metrics with rate info", () => {
    runtimeStore.updateMetrics(
      { component: "a", name: "x", type: "counter", value: 10, labels: {} },
      0,
    );
    runtimeStore.updateMetrics(
      { component: "b", name: "y", type: "gauge", value: 5, labels: {} },
      0,
    );
    const arr = runtimeStore.getMetricsArray();
    expect(arr).toHaveLength(2);
    // Sorted by component name
    expect(arr[0].component).toBe("a");
    expect(arr[1].component).toBe("b");
  });
});

// ---------------------------------------------------------------------------
// getFilteredLogs
// ---------------------------------------------------------------------------

describe("runtimeStore — getFilteredLogs", () => {
  beforeEach(() => {
    runtimeStore.addLog(
      { level: "DEBUG", source: "svc-a", message: "debug msg" },
      "1",
      1000,
    );
    runtimeStore.addLog(
      { level: "INFO", source: "svc-a", message: "info msg" },
      "2",
      2000,
    );
    runtimeStore.addLog(
      { level: "WARN", source: "svc-b", message: "warn msg" },
      "3",
      3000,
    );
    runtimeStore.addLog(
      { level: "ERROR", source: "svc-b", message: "error msg" },
      "4",
      4000,
    );
  });

  it("returns all logs when no filter options given", () => {
    expect(runtimeStore.getFilteredLogs({})).toHaveLength(4);
  });

  it("filters by minimum log level", () => {
    const result = runtimeStore.getFilteredLogs({ minLevel: "WARN" });
    expect(result).toHaveLength(2);
    expect(result.map((l) => l.level)).toEqual(["WARN", "ERROR"]);
  });

  it("filters by source", () => {
    const result = runtimeStore.getFilteredLogs({ sources: ["svc-a"] });
    expect(result).toHaveLength(2);
    expect(result.every((l) => l.source === "svc-a")).toBe(true);
  });

  it("combines level and source filters", () => {
    const result = runtimeStore.getFilteredLogs({
      minLevel: "INFO",
      sources: ["svc-a"],
    });
    expect(result).toHaveLength(1);
    expect(result[0].level).toBe("INFO");
  });

  it("returns empty array when no logs match", () => {
    const result = runtimeStore.getFilteredLogs({ sources: ["nonexistent"] });
    expect(result).toHaveLength(0);
  });
});

// ---------------------------------------------------------------------------
// Reset isolation
// ---------------------------------------------------------------------------

describe("runtimeStore — reset", () => {
  it("clears all state after reset", () => {
    runtimeStore.setConnected(true, "flow-1");
    runtimeStore.setError("some error");
    addLog("ERROR", "bad", "err-1");
    runtimeStore.updateHealth({
      overall: {
        status: "error",
        counts: { healthy: 0, degraded: 0, error: 1 },
      },
      components: [
        {
          name: "x",
          component: "x",
          type: "t",
          status: "error",
          healthy: false,
          message: "x",
        },
      ],
    });
    runtimeStore.updateMetrics(
      { component: "c", name: "m", type: "counter", value: 1, labels: {} },
      1000,
    );

    runtimeStore.reset();

    expect(runtimeStore.connected).toBe(false);
    expect(runtimeStore.error).toBeNull();
    expect(runtimeStore.flowId).toBeNull();
    expect(runtimeStore.flowStatus).toBeNull();
    expect(runtimeStore.healthOverall).toBeNull();
    expect(runtimeStore.healthComponents).toHaveLength(0);
    expect(runtimeStore.logs).toHaveLength(0);
    expect(runtimeStore.metricsRaw.size).toBe(0);
    expect(runtimeStore.metricsRates.size).toBe(0);
    expect(runtimeStore.lastMetricsTimestamp).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// Rapid mutations (no crash, no state corruption)
// ---------------------------------------------------------------------------

describe("runtimeStore — rapid mutations", () => {
  it("handles 500 rapid log additions without corruption", () => {
    for (let i = 0; i < 500; i++) {
      addLog("INFO", `rapid-${i}`, `r-${i}`);
    }
    expect(runtimeStore.logs.length).toBe(500);
    expect(runtimeStore.logs[499].message).toBe("rapid-499");
  });

  it("handles rapid health component updates without duplication", () => {
    for (let i = 0; i < 50; i++) {
      runtimeStore.updateComponentHealth({
        name: "singleton",
        status: i % 2 === 0 ? "healthy" : "error",
        message: null,
      });
    }
    expect(runtimeStore.healthComponents).toHaveLength(1);
    // Last update was i=49 (odd) → error
    expect(runtimeStore.healthComponents[0].status).toBe("error");
  });
});
