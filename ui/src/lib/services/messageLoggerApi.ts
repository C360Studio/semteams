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
 * Coarse classification for UI treatment. The Activity tab uses this
 * to drive structural grouping (lifecycle / LLM / tools / background)
 * rather than user-facing filter chips.
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

/**
 * Pull a request_id out of a message's payload. agent.request.* and
 * agent.response.* both carry it; pairing requests with responses by
 * this key produces a "round" — the user-facing unit "the LLM was
 * asked X and said Y."
 */
export function requestIdOf(entry: MessageLogEntry): string | null {
  const raw = entry.raw_data;
  if (!raw || typeof raw !== "object") return null;
  const r = raw as Record<string, unknown>;
  const payload = (r.payload as Record<string, unknown>) ?? r;
  const id = payload?.request_id;
  return typeof id === "string" ? id : null;
}

/**
 * Pull a tool call id from a tool.execute.* / tool.result.* payload.
 * Used to pair execute with result the same way request_id pairs LLM
 * sides. Returns null when not present.
 */
export function toolCallIdOf(entry: MessageLogEntry): string | null {
  const raw = entry.raw_data;
  if (!raw || typeof raw !== "object") return null;
  const r = raw as Record<string, unknown>;
  const payload = (r.payload as Record<string, unknown>) ?? r;
  const id = payload?.call_id ?? payload?.tool_call_id;
  return typeof id === "string" ? id : null;
}

export interface LLMRound {
  requestId: string;
  request: MessageLogEntry | null;
  response: MessageLogEntry | null;
}

export interface ToolRound {
  callId: string;
  execute: MessageLogEntry | null;
  result: MessageLogEntry | null;
}

/**
 * Pair LLM request and response messages into rounds keyed by
 * request_id. A round may have just a request (in-flight) or just a
 * response (request fell off the log window). Sorted by the earliest
 * timestamp seen for the round.
 */
export function pairLLMRounds(entries: MessageLogEntry[]): LLMRound[] {
  const byId = new Map<string, LLMRound>();
  for (const e of entries) {
    const kind = classifyEntry(e);
    if (kind !== "llm-request" && kind !== "llm-response") continue;
    const id = requestIdOf(e);
    if (!id) continue;
    let round = byId.get(id);
    if (!round) {
      round = { requestId: id, request: null, response: null };
      byId.set(id, round);
    }
    if (kind === "llm-request") round.request = e;
    else round.response = e;
  }
  const rounds = [...byId.values()];
  rounds.sort((a, b) => {
    const at = (a.request ?? a.response)?.timestamp ?? "";
    const bt = (b.request ?? b.response)?.timestamp ?? "";
    return at.localeCompare(bt);
  });
  return rounds;
}

/**
 * Pair tool execute / result messages into rounds keyed by call id.
 * Mirror of pairLLMRounds. Tool messages without an identifiable
 * call_id fall through to the background section.
 */
export function pairToolRounds(entries: MessageLogEntry[]): ToolRound[] {
  const byId = new Map<string, ToolRound>();
  for (const e of entries) {
    const kind = classifyEntry(e);
    if (kind !== "tool-execute" && kind !== "tool-result") continue;
    const id = toolCallIdOf(e);
    if (!id) continue;
    let round = byId.get(id);
    if (!round) {
      round = { callId: id, execute: null, result: null };
      byId.set(id, round);
    }
    if (kind === "tool-execute") round.execute = e;
    else round.result = e;
  }
  const rounds = [...byId.values()];
  rounds.sort((a, b) => {
    const at = (a.execute ?? a.result)?.timestamp ?? "";
    const bt = (b.execute ?? b.result)?.timestamp ?? "";
    return at.localeCompare(bt);
  });
  return rounds;
}
