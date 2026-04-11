// WebSocket service for runtime status streaming
// Connects to /flowbuilder/status/stream and routes messages to runtimeStore

import { runtimeStore, type LogLevel } from "$lib/stores/runtimeStore.svelte";
import type { components } from "$lib/types/api.generated";

// ============================================================================
// Types from OpenAPI spec
// ============================================================================

export type MessageType =
  | "flow_status"
  | "component_health"
  | "component_metrics"
  | "log_entry";

// Use generated types (note: some runtime formats differ from OpenAPI spec)
type StatusStreamEnvelope = components["schemas"]["StatusStreamEnvelope"];
type LogEntryPayload = components["schemas"]["LogEntryPayload"];
type FlowStatusPayload = components["schemas"]["FlowStatusPayload"];
type RuntimeHealthResponse = components["schemas"]["RuntimeHealthResponse"];

interface SubscribeCommand {
  command: "subscribe";
  message_types?: MessageType[];
  log_level?: LogLevel;
  sources?: string[];
}

export interface SubscribeOptions {
  messageTypes?: MessageType[];
  logLevel?: LogLevel;
  sources?: string[];
}

// ============================================================================
// WebSocket Service
// ============================================================================

class RuntimeWebSocketService {
  private ws: WebSocket | null = null;
  private flowId: string = "";
  private reconnectAttempts = 0;
  private maxReconnectAttempts = 5;
  private reconnectDelay = 1000; // Start with 1s, exponential backoff
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private intentionalClose = false;
  private pendingSubscription: SubscribeOptions | null = null;

  /**
   * Connect to the WebSocket endpoint for a specific flow
   */
  connect(flowId: string): void {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      // Already connected - check if same flow
      if (this.flowId === flowId) {
        console.log("[RuntimeWS] Already connected to flow:", flowId);
        return;
      }
      // Different flow - disconnect first
      this.disconnect();
    }

    this.flowId = flowId;
    this.intentionalClose = false;
    this.reconnectAttempts = 0;

    this.doConnect();
  }

  private doConnect(): void {
    // Determine WebSocket URL
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const host = window.location.host;
    const url = `${protocol}//${host}/flowbuilder/status/stream?flowId=${this.flowId}`;

    console.log("[RuntimeWS] Connecting to:", url);
    runtimeStore.setConnected(false, this.flowId);

    try {
      this.ws = new WebSocket(url);

      this.ws.onopen = () => {
        console.log("[RuntimeWS] Connected");
        this.reconnectAttempts = 0;
        this.reconnectDelay = 1000;
        runtimeStore.setConnected(true, this.flowId);

        // Send any pending subscription
        if (this.pendingSubscription) {
          this.sendSubscribe(this.pendingSubscription);
          this.pendingSubscription = null;
        }
      };

      this.ws.onmessage = (event) => {
        this.handleMessage(event.data);
      };

      this.ws.onerror = (error) => {
        console.error("[RuntimeWS] Error:", error);
        runtimeStore.setError("WebSocket connection error");
      };

      this.ws.onclose = (event) => {
        console.log("[RuntimeWS] Closed:", event.code, event.reason);
        runtimeStore.setConnected(false);

        // Attempt reconnection if not intentionally closed
        if (
          !this.intentionalClose &&
          this.reconnectAttempts < this.maxReconnectAttempts
        ) {
          this.scheduleReconnect();
        } else if (this.reconnectAttempts >= this.maxReconnectAttempts) {
          runtimeStore.setError(
            "Connection lost. Max reconnect attempts reached.",
          );
        }
      };
    } catch (error) {
      console.error("[RuntimeWS] Failed to create WebSocket:", error);
      runtimeStore.setError("Failed to establish WebSocket connection");
    }
  }

  private scheduleReconnect(): void {
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
    }

    this.reconnectAttempts++;
    const delay = Math.min(
      this.reconnectDelay * Math.pow(2, this.reconnectAttempts - 1),
      30000,
    );

    console.log(
      `[RuntimeWS] Reconnecting in ${delay}ms (attempt ${this.reconnectAttempts}/${this.maxReconnectAttempts})`,
    );

    this.reconnectTimer = setTimeout(() => {
      this.doConnect();
    }, delay);
  }

  /**
   * Disconnect from the WebSocket
   */
  disconnect(): void {
    console.log("[RuntimeWS] Disconnecting");
    this.intentionalClose = true;

    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }

    if (this.ws) {
      this.ws.close(1000, "Client disconnect");
      this.ws = null;
    }

    this.flowId = "";
    runtimeStore.setConnected(false);
  }

  /**
   * Send subscribe command to filter messages
   * If not connected yet, queues the subscription to send on connect
   */
  subscribe(options: SubscribeOptions): void {
    // Queue subscription if not connected yet
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      console.log("[RuntimeWS] Queueing subscription until connected");
      this.pendingSubscription = options;
      return;
    }
    this.sendSubscribe(options);
  }

  private sendSubscribe(options: SubscribeOptions): void {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      console.warn("[RuntimeWS] Cannot send subscribe - not connected");
      return;
    }

    const command: SubscribeCommand = {
      command: "subscribe",
      message_types: options.messageTypes,
      log_level: options.logLevel,
      sources: options.sources,
    };

    console.log("[RuntimeWS] Sending subscribe:", command);
    this.ws.send(JSON.stringify(command));
  }

  /**
   * Check if currently connected
   */
  isConnected(): boolean {
    return this.ws !== null && this.ws.readyState === WebSocket.OPEN;
  }

  /**
   * Get the current flow ID
   */
  getFlowId(): string {
    return this.flowId;
  }

  // ========================================================================
  // Message Handling
  // ========================================================================

  private handleMessage(data: string): void {
    try {
      const envelope = JSON.parse(data) as StatusStreamEnvelope;

      // Validate envelope structure
      if (!envelope.type || !envelope.id || !envelope.timestamp) {
        console.warn("[RuntimeWS] Invalid envelope format:", envelope);
        return;
      }

      // Parse payload - backend may send as JSON string or object
      let payload: unknown;
      if (typeof envelope.payload === "string") {
        try {
          // Try to decode base64 first (OpenAPI spec says "byte" format)
          const decoded = atob(envelope.payload);
          payload = JSON.parse(decoded);
        } catch {
          // Not base64, try direct JSON parse
          try {
            payload = JSON.parse(envelope.payload);
          } catch {
            // Use as-is if not JSON
            payload = envelope.payload;
          }
        }
      } else {
        payload = envelope.payload;
      }

      // Route to appropriate handler
      switch (envelope.type) {
        case "flow_status":
          this.handleFlowStatus(envelope, payload as FlowStatusPayload);
          break;

        case "component_health":
          this.handleComponentHealth(envelope, payload);
          break;

        case "component_metrics":
          this.handleComponentMetrics(envelope, payload);
          break;

        case "log_entry":
          this.handleLogEntry(envelope, payload);
          break;

        case "subscribe_ack":
          // Control message acknowledging subscription - log for debugging
          console.log("[RuntimeWS] Subscription acknowledged:", payload);
          break;

        default:
          console.warn("[RuntimeWS] Unknown message type:", envelope.type);
      }
    } catch (error) {
      console.error("[RuntimeWS] Failed to parse message:", error, data);
    }
  }

  private handleFlowStatus(
    envelope: StatusStreamEnvelope,
    payload: FlowStatusPayload,
  ): void {
    runtimeStore.updateFlowStatus({
      state: payload.state as
        | "running"
        | "stopped"
        | "error"
        | "deploying"
        | "not_deployed",
      prev_state: payload.prev_state as
        | "running"
        | "stopped"
        | "error"
        | "deploying"
        | "not_deployed"
        | undefined,
      timestamp: payload.timestamp,
      error: payload.error,
    });
  }

  private handleComponentHealth(
    envelope: StatusStreamEnvelope,
    payload: unknown,
  ): void {
    // Backend sends per-component health updates: { name, health: { status, message }, timestamp }
    const update = payload as {
      name: string;
      health?: {
        status?: string;
        message?: string;
      };
      timestamp?: number;
    };

    // Handle per-component update format
    if (update?.name && update?.health) {
      runtimeStore.updateComponentHealth({
        name: update.name,
        status:
          (update.health.status as "healthy" | "degraded" | "error") ||
          "healthy",
        message: update.health.message || null,
      });
      return;
    }

    // Fallback: Handle aggregated format (overall + components array)
    const health = payload as RuntimeHealthResponse;
    if (health?.overall && health?.components) {
      runtimeStore.updateHealth({
        overall: {
          status: health.overall.status as "healthy" | "degraded" | "error",
          counts: {
            healthy: health.overall.running_count,
            degraded: health.overall.degraded_count,
            error: health.overall.error_count,
          },
        },
        components: health.components.map((c) => ({
          name: c.name,
          component: c.component,
          type: c.type,
          status: c.status as "healthy" | "degraded" | "error",
          healthy: c.healthy,
          message: c.message,
        })),
      });
      return;
    }

    console.warn("[RuntimeWS] Unexpected health payload format:", payload);
  }

  private handleComponentMetrics(
    envelope: StatusStreamEnvelope,
    payload: unknown,
  ): void {
    // Actual backend sends single metric per message: {component, name, type, value, labels}
    // (OpenAPI spec shows array format, but runtime uses single metric)
    const metric = payload as {
      component: string;
      name: string;
      type: string;
      value: number;
      labels: Record<string, string>;
    };

    runtimeStore.updateMetrics(
      {
        component: metric.component,
        name: metric.name,
        type: metric.type,
        value: metric.value,
        labels: metric.labels,
      },
      envelope.timestamp,
    );
  }

  private handleLogEntry(
    envelope: StatusStreamEnvelope,
    payload: unknown,
  ): void {
    const log = payload as LogEntryPayload;

    // Validate expected fields
    if (!log?.level || !log?.source || !log?.message) {
      console.warn("[RuntimeWS] Unexpected log entry format:", payload);
      return;
    }

    runtimeStore.addLog(
      {
        level: log.level as LogLevel,
        source: log.source,
        message: log.message,
        fields: log.fields,
      },
      envelope.id,
      envelope.timestamp,
    );
  }
}

// Export singleton instance
export const runtimeWS = new RuntimeWebSocketService();
