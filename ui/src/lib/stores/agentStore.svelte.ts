// Agent store — SSE-driven reactive state for agentic loops
// Connects to /teams-dispatch/activity for real-time loop updates

import { SvelteMap } from "svelte/reactivity";
import type { AgentLoop } from "$lib/types/agent";
import { isActiveState } from "$lib/types/agent";

const SSE_URL = "/teams-dispatch/activity";
const MAX_RECONNECT_ATTEMPTS = 5;
const BASE_RECONNECT_DELAY = 1000;

function createAgentStore() {
  let connected = $state(false);
  let error = $state<string | null>(null);
  let loops = new SvelteMap<string, AgentLoop>();
  let eventSource: EventSource | null = null;
  let reconnectAttempts = 0;
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null;

  function handleLoopUpdate(event: MessageEvent) {
    try {
      const data = JSON.parse(event.data) as AgentLoop;
      loops.set(data.loop_id, data);
    } catch {
      // Ignore malformed events
    }
  }

  function handleConnected() {
    connected = true;
    error = null;
    reconnectAttempts = 0;
  }

  function handleSyncComplete(event: MessageEvent) {
    try {
      const data = JSON.parse(event.data);
      if (Array.isArray(data)) {
        for (const loop of data) {
          loops.set(loop.loop_id, loop);
        }
      }
    } catch {
      // Ignore malformed events
    }
  }

  function scheduleReconnect() {
    if (reconnectAttempts >= MAX_RECONNECT_ATTEMPTS) {
      error = `Failed to connect after ${MAX_RECONNECT_ATTEMPTS} attempts`;
      return;
    }
    const delay = BASE_RECONNECT_DELAY * Math.pow(2, reconnectAttempts);
    reconnectAttempts++;
    reconnectTimer = setTimeout(() => {
      if (eventSource) {
        eventSource.close();
        eventSource = null;
      }
      connectInternal();
    }, delay);
  }

  function connectInternal() {
    eventSource = new EventSource(SSE_URL);

    eventSource.addEventListener("connected", handleConnected);
    eventSource.addEventListener("sync_complete", handleSyncComplete);
    eventSource.addEventListener("loop_update", handleLoopUpdate);
    // heartbeat is just a keep-alive, no action needed

    eventSource.onerror = () => {
      connected = false;
      scheduleReconnect();
    };
  }

  return {
    get connected() {
      return connected;
    },
    get error() {
      return error;
    },
    get loops() {
      return loops;
    },

    get loopsList(): AgentLoop[] {
      return [...loops.values()];
    },

    get activeLoops(): AgentLoop[] {
      return [...loops.values()].filter((l) => isActiveState(l.state));
    },

    get awaitingApproval(): AgentLoop[] {
      return [...loops.values()].filter((l) => l.state === "awaiting_approval");
    },

    getLoop(id: string): AgentLoop | undefined {
      return loops.get(id);
    },

    connect() {
      if (eventSource) return; // Already connected
      connectInternal();
    },

    disconnect() {
      if (reconnectTimer) {
        clearTimeout(reconnectTimer);
        reconnectTimer = null;
      }
      if (eventSource) {
        eventSource.close();
        eventSource = null;
      }
      connected = false;
      reconnectAttempts = 0;
    },

    updateLoop(loop: AgentLoop) {
      loops.set(loop.loop_id, loop);
    },

    removeLoop(id: string) {
      loops.delete(id);
    },

    reset() {
      this.disconnect();
      loops.clear();
      error = null;
    },
  };
}

export const agentStore = createAgentStore();
