import { describe, expect, test, vi, beforeEach, afterEach } from "vitest";
import { streamChat } from "./chatApi";
import type { ChatStreamCallbacks } from "./chatApi";
import type { ChatRequest } from "$lib/types/chat";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Encode a sequence of SSE events into a ReadableStream body. */
function makeSSEStream(events: string[]): ReadableStream<Uint8Array> {
  const encoder = new TextEncoder();
  return new ReadableStream<Uint8Array>({
    start(controller) {
      for (const event of events) {
        controller.enqueue(encoder.encode(event));
      }
      controller.close();
    },
  });
}

/** Build a complete SSE event block from parts. */
function sseEvent(name: string, data: unknown): string {
  return `event: ${name}\ndata: ${JSON.stringify(data)}\n\n`;
}

function makeResponse(
  body: ReadableStream<Uint8Array>,
  status = 200,
): Response {
  return new Response(body, {
    status,
    statusText: status === 200 ? "OK" : "Error",
    headers: { "Content-Type": "text/event-stream" },
  });
}

const MINIMAL_REQUEST: ChatRequest = {
  messages: [{ role: "user", content: "Build a pipeline" }],
  // Migration: context replaces the old currentFlow field
  context: {
    page: "flow-builder",
    flowId: "flow-test",
    flowName: "Test Pipeline",
    nodes: [],
    connections: [],
  },
  chips: [],
};

function makeCallbacks(
  overrides: Partial<ChatStreamCallbacks> = {},
): ChatStreamCallbacks {
  return {
    onText: vi.fn(),
    onDone: vi.fn(),
    onError: vi.fn(),
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// Setup: mock global fetch
// ---------------------------------------------------------------------------

beforeEach(() => {
  vi.stubGlobal("fetch", vi.fn());
});

afterEach(() => {
  vi.unstubAllGlobals();
});

// ---------------------------------------------------------------------------
// 1. AbortController — abort before fetch
// ---------------------------------------------------------------------------

describe("streamChat — AbortController", () => {
  test("resolves silently without calling any callback when aborted before start", async () => {
    const controller = new AbortController();
    controller.abort();

    const callbacks = makeCallbacks();
    await streamChat(MINIMAL_REQUEST, callbacks, controller.signal);

    expect(callbacks.onText).not.toHaveBeenCalled();
    expect(callbacks.onDone).not.toHaveBeenCalled();
    expect(callbacks.onError).not.toHaveBeenCalled();
    expect(fetch).not.toHaveBeenCalled();
  });

  test("AbortError thrown by fetch is handled silently — onError is NOT called", async () => {
    const controller = new AbortController();

    const abortError = new DOMException(
      "The operation was aborted.",
      "AbortError",
    );
    vi.mocked(fetch).mockRejectedValueOnce(abortError);

    const callbacks = makeCallbacks();
    await expect(
      streamChat(MINIMAL_REQUEST, callbacks, controller.signal),
    ).resolves.toBeUndefined();

    expect(callbacks.onError).not.toHaveBeenCalled();
    expect(callbacks.onText).not.toHaveBeenCalled();
  });

  test("non-AbortError fetch errors propagate as thrown exceptions", async () => {
    const networkError = new TypeError("Failed to fetch");
    vi.mocked(fetch).mockRejectedValueOnce(networkError);

    const callbacks = makeCallbacks();

    await expect(streamChat(MINIMAL_REQUEST, callbacks)).rejects.toThrow(
      "Failed to fetch",
    );
    expect(callbacks.onError).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// 2. HTTP error handling
// ---------------------------------------------------------------------------

describe("streamChat — HTTP errors", () => {
  test("calls onError with HTTP status string when response is not ok", async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      new Response(null, { status: 500, statusText: "Internal Server Error" }),
    );

    const callbacks = makeCallbacks();
    await streamChat(MINIMAL_REQUEST, callbacks);

    expect(callbacks.onError).toHaveBeenCalledOnce();
    expect(callbacks.onError).toHaveBeenCalledWith(
      "HTTP 500: Internal Server Error",
    );
    expect(callbacks.onText).not.toHaveBeenCalled();
    expect(callbacks.onDone).not.toHaveBeenCalled();
  });

  test.each([
    { status: 401, statusText: "Unauthorized" },
    { status: 403, statusText: "Forbidden" },
    { status: 404, statusText: "Not Found" },
    { status: 503, statusText: "Service Unavailable" },
  ])("calls onError for HTTP $status", async ({ status, statusText }) => {
    vi.mocked(fetch).mockResolvedValueOnce(
      new Response(null, { status, statusText }),
    );

    const callbacks = makeCallbacks();
    await streamChat(MINIMAL_REQUEST, callbacks);

    expect(callbacks.onError).toHaveBeenCalledWith(
      `HTTP ${status}: ${statusText}`,
    );
  });
});

// ---------------------------------------------------------------------------
// 3. SSE parsing — event: text
// ---------------------------------------------------------------------------

describe("streamChat — SSE parsing: text events", () => {
  test("parses 'event: text' with content chunk and calls onText", async () => {
    const stream = makeSSEStream([sseEvent("text", { content: "Hello, " })]);
    vi.mocked(fetch).mockResolvedValueOnce(makeResponse(stream));

    const callbacks = makeCallbacks();
    await streamChat(MINIMAL_REQUEST, callbacks);

    expect(callbacks.onText).toHaveBeenCalledOnce();
    expect(callbacks.onText).toHaveBeenCalledWith("Hello, ");
  });

  test("calls onText for each text chunk in sequence", async () => {
    const stream = makeSSEStream([
      sseEvent("text", { content: "Hello, " }),
      sseEvent("text", { content: "world" }),
      sseEvent("text", { content: "!" }),
    ]);
    vi.mocked(fetch).mockResolvedValueOnce(makeResponse(stream));

    const callbacks = makeCallbacks();
    await streamChat(MINIMAL_REQUEST, callbacks);

    expect(callbacks.onText).toHaveBeenCalledTimes(3);
    expect(callbacks.onText).toHaveBeenNthCalledWith(1, "Hello, ");
    expect(callbacks.onText).toHaveBeenNthCalledWith(2, "world");
    expect(callbacks.onText).toHaveBeenNthCalledWith(3, "!");
  });

  test("handles empty content string in text event", async () => {
    const stream = makeSSEStream([sseEvent("text", { content: "" })]);
    vi.mocked(fetch).mockResolvedValueOnce(makeResponse(stream));

    const callbacks = makeCallbacks();
    await streamChat(MINIMAL_REQUEST, callbacks);

    expect(callbacks.onText).toHaveBeenCalledWith("");
  });
});

// ---------------------------------------------------------------------------
// 4. SSE parsing — event: done
// ---------------------------------------------------------------------------

describe("streamChat — SSE parsing: done events", () => {
  // Migration: flow is now sent as an "attachment" event, not in the "done" payload.
  // onDone now receives { attachments: MessageAttachment[] }.

  test("parses 'event: done' with flow payload and calls onDone", async () => {
    const flow = {
      nodes: [
        {
          id: "node-1",
          component: "http_source",
          type: "input",
          name: "HTTP Source",
          position: { x: 0, y: 0 },
          config: {},
        },
      ],
      connections: [],
    };

    // Migration: flow is sent as an attachment event before done
    const stream = makeSSEStream([
      sseEvent("attachment", { kind: "flow", flow }),
      sseEvent("done", {}),
    ]);
    vi.mocked(fetch).mockResolvedValueOnce(makeResponse(stream));

    const callbacks = makeCallbacks();
    await streamChat(MINIMAL_REQUEST, callbacks);

    expect(callbacks.onDone).toHaveBeenCalledOnce();
    const doneArg = vi.mocked(callbacks.onDone).mock.calls[0][0];
    expect(doneArg.attachments).toHaveLength(1);
    expect(doneArg.attachments[0]).toMatchObject({
      kind: "flow",
      flow: expect.objectContaining({
        nodes: expect.arrayContaining([
          expect.objectContaining({ id: "node-1" }),
        ]),
      }),
    });
  });

  test("parses 'event: done' with no flow (text-only response)", async () => {
    const stream = makeSSEStream([sseEvent("done", {})]);
    vi.mocked(fetch).mockResolvedValueOnce(makeResponse(stream));

    const callbacks = makeCallbacks();
    await streamChat(MINIMAL_REQUEST, callbacks);

    expect(callbacks.onDone).toHaveBeenCalledOnce();
    // No attachment events — attachments is empty
    const doneArg = vi.mocked(callbacks.onDone).mock.calls[0][0];
    expect(doneArg.attachments).toEqual([]);
  });

  test("parses full stream: multiple text chunks then done", async () => {
    const flow = { nodes: [], connections: [] };
    const stream = makeSSEStream([
      sseEvent("text", { content: "Building " }),
      sseEvent("text", { content: "pipeline..." }),
      sseEvent("attachment", { kind: "flow", flow }),
      sseEvent("done", {}),
    ]);
    vi.mocked(fetch).mockResolvedValueOnce(makeResponse(stream));

    const callbacks = makeCallbacks();
    await streamChat(MINIMAL_REQUEST, callbacks);

    expect(callbacks.onText).toHaveBeenCalledTimes(2);
    expect(callbacks.onDone).toHaveBeenCalledOnce();
    expect(callbacks.onError).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// 5. SSE parsing — event: error
// ---------------------------------------------------------------------------

describe("streamChat — SSE parsing: error events", () => {
  test("parses 'event: error' and calls onError with message", async () => {
    const stream = makeSSEStream([
      sseEvent("error", { message: "AI service unavailable" }),
    ]);
    vi.mocked(fetch).mockResolvedValueOnce(makeResponse(stream));

    const callbacks = makeCallbacks();
    await streamChat(MINIMAL_REQUEST, callbacks);

    expect(callbacks.onError).toHaveBeenCalledOnce();
    expect(callbacks.onError).toHaveBeenCalledWith("AI service unavailable");
  });

  test.each([
    { message: "Context window exceeded" },
    { message: "Rate limit reached" },
    { message: "Invalid flow schema" },
  ])("calls onError with message: '$message'", async ({ message }) => {
    const stream = makeSSEStream([sseEvent("error", { message })]);
    vi.mocked(fetch).mockResolvedValueOnce(makeResponse(stream));

    const callbacks = makeCallbacks();
    await streamChat(MINIMAL_REQUEST, callbacks);

    expect(callbacks.onError).toHaveBeenCalledWith(message);
  });
});

// ---------------------------------------------------------------------------
// 6. SSE parsing — malformed/unknown events
// ---------------------------------------------------------------------------

describe("streamChat — SSE parsing: edge cases", () => {
  test("ignores events with missing event name", async () => {
    // Only data line, no event line
    const encoder = new TextEncoder();
    const stream = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(encoder.encode(`data: {"content": "orphan"}\n\n`));
        controller.close();
      },
    });
    vi.mocked(fetch).mockResolvedValueOnce(makeResponse(stream));

    const callbacks = makeCallbacks();
    await streamChat(MINIMAL_REQUEST, callbacks);

    expect(callbacks.onText).not.toHaveBeenCalled();
    expect(callbacks.onDone).not.toHaveBeenCalled();
    expect(callbacks.onError).not.toHaveBeenCalled();
  });

  test("ignores events with invalid JSON data without throwing", async () => {
    const encoder = new TextEncoder();
    const stream = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(
          encoder.encode(`event: text\ndata: not-valid-json\n\n`),
        );
        controller.close();
      },
    });
    vi.mocked(fetch).mockResolvedValueOnce(makeResponse(stream));

    const callbacks = makeCallbacks();
    await expect(
      streamChat(MINIMAL_REQUEST, callbacks),
    ).resolves.toBeUndefined();

    expect(callbacks.onText).not.toHaveBeenCalled();
  });

  test("ignores unknown event types without calling any callback", async () => {
    const stream = makeSSEStream([sseEvent("ping", { ts: Date.now() })]);
    vi.mocked(fetch).mockResolvedValueOnce(makeResponse(stream));

    const callbacks = makeCallbacks();
    await streamChat(MINIMAL_REQUEST, callbacks);

    expect(callbacks.onText).not.toHaveBeenCalled();
    expect(callbacks.onDone).not.toHaveBeenCalled();
    expect(callbacks.onError).not.toHaveBeenCalled();
  });

  test("handles empty/whitespace-only SSE parts gracefully", async () => {
    const encoder = new TextEncoder();
    const stream = new ReadableStream<Uint8Array>({
      start(controller) {
        // Multiple empty lines (legal SSE keep-alives)
        controller.enqueue(encoder.encode("\n\n\n\n"));
        controller.enqueue(encoder.encode(sseEvent("text", { content: "ok" })));
        controller.close();
      },
    });
    vi.mocked(fetch).mockResolvedValueOnce(makeResponse(stream));

    const callbacks = makeCallbacks();
    await streamChat(MINIMAL_REQUEST, callbacks);

    expect(callbacks.onText).toHaveBeenCalledWith("ok");
  });

  test("handles events split across multiple stream chunks (buffer reassembly)", async () => {
    const encoder = new TextEncoder();
    const fullEvent = sseEvent("text", { content: "streamed" });

    // Split the event raw bytes into two chunks
    const half = Math.floor(fullEvent.length / 2);
    const chunk1 = fullEvent.slice(0, half);
    const chunk2 = fullEvent.slice(half);

    const stream = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(encoder.encode(chunk1));
        controller.enqueue(encoder.encode(chunk2));
        controller.close();
      },
    });
    vi.mocked(fetch).mockResolvedValueOnce(makeResponse(stream));

    const callbacks = makeCallbacks();
    await streamChat(MINIMAL_REQUEST, callbacks);

    expect(callbacks.onText).toHaveBeenCalledWith("streamed");
  });
});

// ---------------------------------------------------------------------------
// 7. Request construction
// ---------------------------------------------------------------------------

describe("streamChat — request construction", () => {
  test("sends POST to /api/ai/chat with JSON body", async () => {
    const stream = makeSSEStream([]);
    vi.mocked(fetch).mockResolvedValueOnce(makeResponse(stream));

    await streamChat(MINIMAL_REQUEST, makeCallbacks());

    expect(fetch).toHaveBeenCalledWith(
      "/api/ai/chat",
      expect.objectContaining({
        method: "POST",
        headers: expect.objectContaining({
          "Content-Type": "application/json",
        }),
        body: JSON.stringify(MINIMAL_REQUEST),
      }),
    );
  });

  test("passes AbortSignal to fetch when provided", async () => {
    const controller = new AbortController();
    const stream = makeSSEStream([]);
    vi.mocked(fetch).mockResolvedValueOnce(makeResponse(stream));

    await streamChat(MINIMAL_REQUEST, makeCallbacks(), controller.signal);

    expect(fetch).toHaveBeenCalledWith(
      "/api/ai/chat",
      expect.objectContaining({ signal: controller.signal }),
    );
  });
});
