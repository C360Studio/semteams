// systemStatus — single source of truth for "is this thing working?"
// state across all the moving parts users care about. Today: SSE
// connection + backend /health roll-up. Future: LLM endpoint breaker
// state (beta.15 exposes it), NATS connectivity detail, model registry
// staleness, ops-agent diagnoses summary.
//
// Why a store: the green dot in the top-right needs to roll up several
// signals into one summary, and a popover needs to enumerate them.
// Spreading those reads across components means the rollup logic
// duplicates and drifts. Centralized here, every consumer sees the same
// truth and a single $effect owns polling lifecycle.

import { agentStore } from "./agentStore.svelte";

const HEALTH_URL = "/health";
const POLL_INTERVAL_MS = 15_000;
const FETCH_TIMEOUT_MS = 5_000;

export type HealthSummary = "healthy" | "degraded" | "unhealthy" | "unknown";

export interface SubStatus {
  component: string;
  healthy: boolean;
  message: string;
}

interface BackendHealth {
  component: string;
  healthy: boolean;
  status: string;
  message: string;
  sub_statuses?: SubStatus[];
}

function createSystemStatus() {
  let backend = $state<BackendHealth | null>(null);
  let lastChecked = $state<Date | null>(null);
  let lastError = $state<string | null>(null);
  let pollTimer: ReturnType<typeof setTimeout> | null = null;
  let inFlight = false;

  // Connected state mirrors agentStore — the SSE pipe is the proxy
  // for "the UI is talking to the backend right now."
  const connected = $derived(agentStore.connected);

  // Roll up the signals we have into one user-facing summary.
  //
  //   healthy   — SSE up AND backend health ok
  //   degraded  — SSE up but backend reports a sub-component issue,
  //               OR we haven't yet had a backend response
  //   unhealthy — SSE down (most visible failure mode for the user)
  //   unknown   — initial cold-start before either signal arrives
  const summary: HealthSummary = $derived.by(() => {
    if (!connected && backend === null) return "unknown";
    if (!connected) return "unhealthy";
    if (backend === null) return "degraded";
    if (backend.healthy) return "healthy";
    return "degraded";
  });

  // One-word label that fits the dot. Avoids "Connected" because that
  // implies the SSE-only view; "Online" reads as "everything's fine."
  const label: string = $derived.by(() => {
    switch (summary) {
      case "healthy":
        return "Online";
      case "degraded":
        return "Issues";
      case "unhealthy":
        return "Offline";
      case "unknown":
        return "Connecting…";
    }
  });

  // Sub-status rows for the popover, with connection prepended so
  // it's consistently the first row regardless of what backend
  // returns. Empty array until a backend response arrives.
  const subStatuses: SubStatus[] = $derived.by(() => {
    const rows: SubStatus[] = [
      {
        component: "Live updates",
        healthy: connected,
        message: connected ? "Streaming activity" : "Reconnecting",
      },
    ];
    if (backend) {
      rows.push({
        component: "Backend",
        healthy: backend.healthy,
        message: backend.message,
      });
      for (const sub of backend.sub_statuses ?? []) {
        rows.push(sub);
      }
    }
    return rows;
  });

  async function refresh() {
    if (inFlight) return;
    inFlight = true;
    const ctrl = new AbortController();
    const timer = setTimeout(() => ctrl.abort(), FETCH_TIMEOUT_MS);
    try {
      const res = await fetch(HEALTH_URL, { signal: ctrl.signal });
      if (!res.ok) throw new Error(`/health ${res.status}`);
      backend = (await res.json()) as BackendHealth;
      lastError = null;
    } catch (err) {
      backend = null;
      lastError = err instanceof Error ? err.message : String(err);
    } finally {
      clearTimeout(timer);
      lastChecked = new Date();
      inFlight = false;
    }
  }

  function schedule() {
    if (pollTimer) return;
    pollTimer = setTimeout(async () => {
      pollTimer = null;
      await refresh();
      schedule();
    }, POLL_INTERVAL_MS);
  }

  return {
    /** Aggregate health across SSE + backend health. */
    get summary() {
      return summary;
    },
    /** One-word label for the dot ("Online", "Issues", etc.). */
    get label() {
      return label;
    },
    /** Detailed rows for the status popover. */
    get subStatuses() {
      return subStatuses;
    },
    /** SSE-side connected state, exposed for callers that only need it. */
    get connected() {
      return connected;
    },
    /** Whether a backend response has ever come back. */
    get backendKnown() {
      return backend !== null;
    },
    /** When the last /health response landed. */
    get lastChecked() {
      return lastChecked;
    },
    /** Last fetch error message, if any. */
    get lastError() {
      return lastError;
    },

    /** Start polling. Idempotent. */
    start() {
      if (pollTimer) return;
      void refresh();
      schedule();
    },

    /** Stop polling. Safe to call repeatedly. */
    stop() {
      if (pollTimer) {
        clearTimeout(pollTimer);
        pollTimer = null;
      }
    },

    /** Force an immediate /health fetch (e.g. user clicks refresh). */
    refresh,
  };
}

export const systemStatus = createSystemStatus();
