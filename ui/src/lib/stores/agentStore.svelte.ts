// Agent store — SSE-driven reactive state for agentic loops
// Connects to /teams-dispatch/activity for real-time loop updates
//
// Backend SSE protocol (from dispatch's KV watcher):
//   event: connected    — connection established
//   event: activity     — KV entry (loop data wrapped in ActivityEvent envelope)
//   event: sync_complete — all existing KV entries delivered (signal only, no loop data)
//   :heartbeat <ts>     — keep-alive comment (ignored by EventSource)

import { SvelteMap } from "svelte/reactivity";
import type { AgentLoop } from "$lib/types/agent";
import { isActiveState } from "$lib/types/agent";

const SSE_URL = "/teams-dispatch/activity";
const MAX_RECONNECT_ATTEMPTS = 5;
const BASE_RECONNECT_DELAY = 1000;

// ActivityEvent is the envelope the backend wraps around KV entries.
interface ActivityEvent {
  type: string; // loop_created, loop_updated, loop_deleted
  loop_id: string;
  timestamp: string;
  data?: AgentLoop;
}

function createAgentStore() {
  let connected = $state(false);
  let error = $state<string | null>(null);
  let loops = new SvelteMap<string, AgentLoop>();
  let eventSource: EventSource | null = null;
  let reconnectAttempts = 0;
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null;

  function handleActivity(event: MessageEvent) {
    try {
      const envelope = JSON.parse(event.data) as ActivityEvent;
      if (envelope.type === "loop_deleted") {
        loops.delete(envelope.loop_id);
        return;
      }
      if (!envelope.data) return;

      // Skip dispatch's "COMPLETE_<id>" ghost records — they carry
      // outcome/result for an already-tracked loop, no state field, and
      // they're not loops in their own right. They should ideally merge
      // onto the real loop entry; until the wire format settles, drop.
      if (envelope.loop_id?.startsWith("COMPLETE_")) return;

      // Normalize wire-format drift: the real loop record from the
      // KV watcher uses `id`, the dispatch LoopTracker shape uses
      // `loop_id`. Accept either, write under loop_id.
      const raw = envelope.data as AgentLoop & { id?: string };
      const id = raw.loop_id || raw.id;
      if (!id) return;
      loops.set(id, { ...raw, loop_id: id });
    } catch {
      // Ignore malformed events
    }
  }

  function handleConnected() {
    connected = true;
    error = null;
    reconnectAttempts = 0;
  }

  function handleSyncComplete() {
    // Signal-only: all existing KV entries already arrived as individual
    // 'activity' events during the initial watcher sync. Nothing to parse.
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
    eventSource.addEventListener("activity", handleActivity);
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
