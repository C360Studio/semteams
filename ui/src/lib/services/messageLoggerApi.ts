// Wrapper for the backend message-logger HTTP surface.
// Endpoint reference: /go/pkg/mod/.../service/message_logger_http.go.
// Returns the raw NATS message log entries the agentic loop produces
// (agent.request.*, agent.response.*, tool.execute.*, tool.result.*,
// graph.mutation.*, etc.). The Trace tab in TaskDetailPanel consumes
// these and filters them client-side to a single task's loop chain.

export interface MessageLogEntry {
  sequence: number;
  timestamp: string;
  subject: string;
  message_type: string;
  trace_id?: string;
  span_id?: string;
  summary?: string;
  raw_data?: unknown;
}

export class MessageLoggerError extends Error {
  constructor(
    message: string,
    public statusCode: number,
  ) {
    super(message);
    this.name = "MessageLoggerError";
  }
}

export interface FetchEntriesOptions {
  limit?: number;
  /** NATS-pattern subject filter (e.g. "agent.>" or "tool.*.foo"). */
  subject?: string;
}

export const messageLoggerApi = {
  async fetchEntries(
    options?: FetchEntriesOptions,
  ): Promise<MessageLogEntry[]> {
    const params = new URLSearchParams();
    params.append("limit", String(options?.limit ?? 200));
    if (options?.subject) params.append("subject", options.subject);
    const url = `/message-logger/entries?${params.toString()}`;
    const res = await fetch(url, { method: "GET" });
    if (!res.ok) {
      throw new MessageLoggerError(
        `message-logger /entries returned ${res.status}`,
        res.status,
      );
    }
    return res.json();
  },
};

/**
 * Does this entry reference the given loop id? Checks:
 *   - the NATS subject (where loop-keyed messages put it)
 *   - the raw_data payload (where graph mutations carry it)
 *
 * Match is substring, case-sensitive. loop_id values are themselves
 * already case-stable (UUIDs / "loop_<hex>"), so substring is safe.
 */
export function entryMentionsLoop(
  entry: MessageLogEntry,
  loopId: string,
): boolean {
  if (!loopId) return false;
  if (entry.subject?.includes(loopId)) return true;
  if (entry.raw_data !== undefined) {
    try {
      const serialised = JSON.stringify(entry.raw_data);
      if (serialised.includes(loopId)) return true;
    } catch {
      // Circular or otherwise unserialisable — skip the payload check.
    }
  }
  return false;
}

/**
 * Coarse classification for UI treatment. The Trace tab uses this to
 * pick an icon / color and to decide which entries to surface
 * prominently (LLM requests/responses + tool calls) vs. group as
 * background noise (graph mutations, lifecycle).
 */
export type EntryKind =
  | "llm-request"
  | "llm-response"
  | "tool-execute"
  | "tool-result"
  | "lifecycle"
  | "graph"
  | "other";

export function classifyEntry(entry: MessageLogEntry): EntryKind {
  const s = entry.subject ?? "";
  if (s.startsWith("agent.request.")) return "llm-request";
  if (s.startsWith("agent.response.")) return "llm-response";
  if (s.startsWith("tool.execute.")) return "tool-execute";
  if (s.startsWith("tool.result.")) return "tool-result";
  if (s.startsWith("agent.task.") || s.startsWith("agent.complete.")) {
    return "lifecycle";
  }
  if (s.startsWith("graph.mutation.")) return "graph";
  return "other";
}
