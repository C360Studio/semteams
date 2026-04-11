// Runtime data store for WebSocket-driven updates
// Centralizes all runtime panel data: logs, metrics, health, flow status

import { SvelteMap } from "svelte/reactivity";

// ============================================================================
// Types
// ============================================================================

export type LogLevel = "DEBUG" | "INFO" | "WARN" | "ERROR";
export type FlowState =
  | "running"
  | "stopped"
  | "error"
  | "deploying"
  | "not_deployed";
export type ComponentStatus = "healthy" | "degraded" | "error";

export interface LogEntry {
  id: string;
  timestamp: number; // Unix ms
  level: LogLevel;
  source: string;
  message: string;
  fields?: Record<string, unknown>;
}

export interface ComponentHealth {
  name: string;
  component: string;
  type: string;
  status: ComponentStatus;
  healthy: boolean;
  message: string | null;
}

export interface HealthOverall {
  status: ComponentStatus;
  counts: {
    healthy: number;
    degraded: number;
    error: number;
  };
}

export interface FlowStatus {
  state: FlowState;
  prevState: FlowState | null;
  timestamp: number | null;
  error: string | null;
}

export interface MetricValue {
  name: string;
  type: string;
  value: number;
  labels: Record<string, string>;
}

// ============================================================================
// Store State (kept for backward compatibility with test mocks)
// ============================================================================

export interface RuntimeStoreState {
  // Connection state
  connected: boolean;
  error: string | null;
  flowId: string | null;

  // Flow status
  flowStatus: FlowStatus | null;

  // Health data
  healthOverall: HealthOverall | null;
  healthComponents: ComponentHealth[];

  // Logs (circular buffer, max 1000)
  logs: LogEntry[];

  // Metrics: raw counters + computed rates
  metricsRaw: Map<string, MetricValue>; // key: component:metricName
  metricsRates: Map<string, number>; // key: component:metricName -> rate/sec
  lastMetricsTimestamp: number | null;
}

const MAX_LOGS = 1000;
const _METRICS_INTERVAL_MS = 5000; // Expected interval for rate calculation (documentation)

// ============================================================================
// Store Implementation using Svelte 5 runes
// ============================================================================

function createRuntimeStore() {
  // Reactive state using $state rune
  let connected = $state(false);
  let error = $state<string | null>(null);
  let flowId = $state<string | null>(null);
  let flowStatus = $state<FlowStatus | null>(null);
  let healthOverall = $state<HealthOverall | null>(null);
  let healthComponents = $state<ComponentHealth[]>([]);
  let logs = $state<LogEntry[]>([]);
  let metricsRaw = new SvelteMap<string, MetricValue>();
  let metricsRates = new SvelteMap<string, number>();
  let lastMetricsTimestamp = $state<number | null>(null);

  return {
    // ========================================================================
    // Reactive getters — consumers read these directly (no subscribe needed)
    // ========================================================================

    get connected() {
      return connected;
    },
    get error() {
      return error;
    },
    get flowId() {
      return flowId;
    },
    get flowStatus() {
      return flowStatus;
    },
    get healthOverall() {
      return healthOverall;
    },
    get healthComponents() {
      return healthComponents;
    },
    get logs() {
      return logs;
    },
    get metricsRaw() {
      return metricsRaw;
    },
    get metricsRates() {
      return metricsRates;
    },
    get lastMetricsTimestamp() {
      return lastMetricsTimestamp;
    },

    // ========================================================================
    // Connection State
    // ========================================================================

    setConnected(isConnected: boolean, newFlowId?: string) {
      connected = isConnected;
      if (newFlowId !== undefined) {
        flowId = newFlowId;
      }
      if (isConnected) {
        error = null;
      }
    },

    setError(newError: string | null) {
      error = newError;
      if (newError) {
        connected = false;
      }
    },

    // ========================================================================
    // Flow Status
    // ========================================================================

    updateFlowStatus(payload: {
      state: FlowState;
      prev_state?: FlowState;
      timestamp?: number;
      error?: string;
    }) {
      flowStatus = {
        state: payload.state,
        prevState: payload.prev_state ?? null,
        timestamp: payload.timestamp ?? null,
        error: payload.error ?? null,
      };
    },

    // ========================================================================
    // Health
    // ========================================================================

    updateHealth(payload: {
      overall: {
        status: ComponentStatus;
        counts: { healthy: number; degraded: number; error: number };
      };
      components: Array<{
        name: string;
        component: string;
        type: string;
        status: ComponentStatus;
        healthy: boolean;
        message: string | null;
      }>;
    }) {
      healthOverall = payload.overall;
      healthComponents = payload.components;
    },

    /**
     * Update a single component's health (streaming per-component updates)
     */
    updateComponentHealth(payload: {
      name: string;
      status: ComponentStatus;
      message: string | null;
    }) {
      const existingIndex = healthComponents.findIndex(
        (c) => c.name === payload.name,
      );

      if (existingIndex >= 0) {
        // Update existing — replace array to trigger reactivity
        const updated = [...healthComponents];
        updated[existingIndex] = {
          ...updated[existingIndex],
          status: payload.status,
          healthy: payload.status === "healthy",
          message: payload.message,
        };
        healthComponents = updated;
      } else {
        // Add new component
        healthComponents = [
          ...healthComponents,
          {
            name: payload.name,
            component: payload.name, // Use name as component ID
            type: "unknown",
            status: payload.status,
            healthy: payload.status === "healthy",
            message: payload.message,
          },
        ];
      }

      // Recalculate overall health
      const counts = {
        healthy: healthComponents.filter((c) => c.status === "healthy").length,
        degraded: healthComponents.filter((c) => c.status === "degraded")
          .length,
        error: healthComponents.filter((c) => c.status === "error").length,
      };

      const overallStatus: ComponentStatus =
        counts.error > 0
          ? "error"
          : counts.degraded > 0
            ? "degraded"
            : "healthy";

      healthOverall = { status: overallStatus, counts };
    },

    // ========================================================================
    // Logs
    // ========================================================================

    addLog(
      payload: {
        level: LogLevel;
        source: string;
        message: string;
        fields?: Record<string, unknown>;
      },
      id: string,
      timestamp: number,
    ) {
      const newLog: LogEntry = {
        id,
        timestamp,
        level: payload.level,
        source: payload.source,
        message: payload.message,
        fields: payload.fields,
      };

      // Circular buffer: keep last MAX_LOGS
      logs = [...logs, newLog].slice(-MAX_LOGS);
    },

    clearLogs() {
      logs = [];
    },

    // ========================================================================
    // Metrics (with rate calculation)
    // ========================================================================

    updateMetrics(
      payload: {
        component: string;
        name: string;
        type: string;
        value: number;
        labels: Record<string, string>;
      },
      timestamp: number,
    ) {
      const key = `${payload.component}:${payload.name}`;

      // Get previous value for rate calculation
      const prevMetric = metricsRaw.get(key);
      const prevTimestamp = lastMetricsTimestamp;

      // Store new raw value
      metricsRaw.set(key, {
        name: payload.name,
        type: payload.type,
        value: payload.value,
        labels: payload.labels,
      });

      // Calculate rate if we have previous data
      if (prevMetric && prevTimestamp && payload.type === "counter") {
        const timeDeltaSec = (timestamp - prevTimestamp) / 1000;
        if (timeDeltaSec > 0) {
          const valueDelta = payload.value - prevMetric.value;
          const rate = valueDelta / timeDeltaSec;
          metricsRates.set(key, Math.max(0, rate)); // Clamp to non-negative
        }
      }

      lastMetricsTimestamp = timestamp;
    },

    // ========================================================================
    // Helpers
    // ========================================================================

    /**
     * Get logs filtered by level and/or source
     */
    getFilteredLogs(options: {
      minLevel?: LogLevel;
      sources?: string[];
    }): LogEntry[] {
      const levelOrder: Record<LogLevel, number> = {
        DEBUG: 0,
        INFO: 1,
        WARN: 2,
        ERROR: 3,
      };

      return logs.filter((log) => {
        // Filter by level
        if (
          options.minLevel &&
          levelOrder[log.level] < levelOrder[options.minLevel]
        ) {
          return false;
        }

        // Filter by source
        if (options.sources && options.sources.length > 0) {
          if (!options.sources.includes(log.source)) {
            return false;
          }
        }

        return true;
      });
    },

    /**
     * Get metrics rate for a specific component and metric name
     */
    getMetricRate(component: string, metricName: string): number | null {
      const key = `${component}:${metricName}`;
      return metricsRates.get(key) ?? null;
    },

    /**
     * Get all metrics as an array for display
     * Shows metrics immediately, rate is null until second data point
     */
    getMetricsArray(): Array<{
      component: string;
      metricName: string;
      rate: number | null;
      raw: MetricValue;
    }> {
      const result: Array<{
        component: string;
        metricName: string;
        rate: number | null;
        raw: MetricValue;
      }> = [];

      // Iterate over raw metrics so they show immediately
      for (const [key, raw] of metricsRaw) {
        const [component, metricName] = key.split(":");
        result.push({
          component,
          metricName,
          rate: metricsRates.get(key) ?? null,
          raw,
        });
      }

      return result.sort((a, b) => a.component.localeCompare(b.component));
    },

    // ========================================================================
    // Reset
    // ========================================================================

    reset() {
      connected = false;
      error = null;
      flowId = null;
      flowStatus = null;
      healthOverall = null;
      healthComponents = [];
      logs = [];
      metricsRaw.clear();
      metricsRates.clear();
      lastMetricsTimestamp = null;
    },
  };
}

export const runtimeStore = createRuntimeStore();
