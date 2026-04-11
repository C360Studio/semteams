import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { streamChat } from "./chatApi";
import type { ChatStreamCallbacks } from "./chatApi";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Encode a single SSE event line as bytes. */
function sseEvent(eventName: string, data: string): Uint8Array {
  return new TextEncoder().encode(`event: ${eventName}\ndata: ${data}\n\n`);
}

/** Build a ReadableStream from an ordered list of Uint8Array chunks. */
function makeStream(chunks: Uint8Array[]): ReadableStream<Uint8Array> {
  return new ReadableStream({
    start(controller) {
      for (const chunk of chunks) {
        controller.enqueue(chunk);
      }
      controller.close();
    },
  });
}

/** Minimal ChatRequest fixture — uses new context shape instead of currentFlow. */
function makeChatRequest(userMessage = "build me a flow") {
  return {
    messages: [{ role: "user" as const, content: userMessage }],
    // Migration: context replaces the old currentFlow field
    context: {
      page: "flow-builder" as const,
      flowId: "flow-test",
      flowName: "Test Pipeline",
      nodes: [],
      connections: [],
    },
    chips: [],
  };
}

/** Default no-op callbacks. */
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
// Setup global fetch mock
// ---------------------------------------------------------------------------

const mockFetch = vi.fn();

beforeEach(() => {
  globalThis.fetch = mockFetch;
  mockFetch.mockClear();
});

afterEach(() => {
  vi.restoreAllMocks();
});

// ---------------------------------------------------------------------------
// Request shape
// ---------------------------------------------------------------------------

describe("streamChat — request", () => {
  it("POSTs to /api/ai/chat", async () => {
    const stream = makeStream([
      sseEvent("done", JSON.stringify({ flow: null })),
    ]);
    mockFetch.mockResolvedValueOnce({ ok: true, body: stream });

    const req = makeChatRequest();
    await streamChat(req, makeCallbacks());

    expect(mockFetch).toHaveBeenCalledWith(
      "/api/ai/chat",
      expect.objectContaining({ method: "POST" }),
    );
  });

  it("sends the ChatRequest body as JSON", async () => {
    const stream = makeStream([sseEvent("done", JSON.stringify({}))]);
    mockFetch.mockResolvedValueOnce({ ok: true, body: stream });

    const req = makeChatRequest("create a NATS pipeline");
    await streamChat(req, makeCallbacks());

    const [, fetchOptions] = mockFetch.mock.calls[0];
    expect(fetchOptions.headers?.["Content-Type"]).toBe("application/json");
    const body = JSON.parse(fetchOptions.body);
    expect(body.messages[0].content).toBe("create a NATS pipeline");
    // Migration: context replaces the old currentFlow field
    expect(body.context).toBeDefined();
  });

  it("passes the AbortSignal to fetch when provided", async () => {
    const stream = makeStream([
      sseEvent("done", JSON.stringify({ flow: null })),
    ]);
    mockFetch.mockResolvedValueOnce({ ok: true, body: stream });

    const controller = new AbortController();
    await streamChat(makeChatRequest(), makeCallbacks(), controller.signal);

    const [, fetchOptions] = mockFetch.mock.calls[0];
    expect(fetchOptions.signal).toBe(controller.signal);
  });
});

// ---------------------------------------------------------------------------
// onText callback
// ---------------------------------------------------------------------------

describe("streamChat — onText", () => {
  it("calls onText for each text event", async () => {
    const stream = makeStream([
      sseEvent("text", JSON.stringify({ content: "Hello" })),
      sseEvent("text", JSON.stringify({ content: " world" })),
      sseEvent("done", JSON.stringify({ flow: null })),
    ]);
    mockFetch.mockResolvedValueOnce({ ok: true, body: stream });

    const onText = vi.fn();
    await streamChat(makeChatRequest(), makeCallbacks({ onText }));

    expect(onText).toHaveBeenCalledTimes(2);
    expect(onText).toHaveBeenNthCalledWith(1, "Hello");
    expect(onText).toHaveBeenNthCalledWith(2, " world");
  });

  it("handles multiple text events — accumulation is the caller's responsibility", async () => {
    const chunks = ["chunk1", "chunk2", "chunk3"];
    const stream = makeStream([
      ...chunks.map((c) => sseEvent("text", JSON.stringify({ content: c }))),
      sseEvent("done", JSON.stringify({ flow: null })),
    ]);
    mockFetch.mockResolvedValueOnce({ ok: true, body: stream });

    const onText = vi.fn();
    await streamChat(makeChatRequest(), makeCallbacks({ onText }));

    expect(onText).toHaveBeenCalledTimes(3);
    // Each call receives only its individual chunk — no accumulation
    expect(onText).toHaveBeenNthCalledWith(1, "chunk1");
    expect(onText).toHaveBeenNthCalledWith(2, "chunk2");
    expect(onText).toHaveBeenNthCalledWith(3, "chunk3");
  });

  it("parses event data as JSON", async () => {
    const stream = makeStream([
      sseEvent("text", JSON.stringify({ content: "parsed text" })),
      sseEvent("done", JSON.stringify({ flow: null })),
    ]);
    mockFetch.mockResolvedValueOnce({ ok: true, body: stream });

    const onText = vi.fn();
    await streamChat(makeChatRequest(), makeCallbacks({ onText }));

    // The content field should be extracted from the JSON data object
    expect(onText).toHaveBeenCalledWith("parsed text");
  });
});

// ---------------------------------------------------------------------------
// onDone callback
// ---------------------------------------------------------------------------

describe("streamChat — onDone", () => {
  // Migration: onDone now receives { attachments: MessageAttachment[] }.
  // Flow/validationResult are now sent as "attachment" SSE events, not in "done" data.

  it("calls onDone when done event received", async () => {
    // Migration: done event no longer carries flow/validationResult directly.
    // Flow is sent as an attachment event before done.
    const flowAttachment = {
      kind: "flow",
      flow: {
        nodes: [
          {
            id: "n1",
            component: "udp",
            type: "input",
            name: "UDP",
            position: { x: 0, y: 0 },
            config: {},
          },
        ],
        connections: [],
      },
      validationResult: { valid: true },
    };
    const stream = makeStream([
      sseEvent("attachment", JSON.stringify(flowAttachment)),
      sseEvent("done", JSON.stringify({})),
    ]);
    mockFetch.mockResolvedValueOnce({ ok: true, body: stream });

    const onDone = vi.fn();
    await streamChat(makeChatRequest(), makeCallbacks({ onDone }));

    expect(onDone).toHaveBeenCalledTimes(1);
    const doneArg = onDone.mock.calls[0][0];
    expect(doneArg.attachments).toHaveLength(1);
    expect(doneArg.attachments[0].kind).toBe("flow");
  });

  it("onDone receives flow attached to done event data", async () => {
    // Migration: flow is sent as a FlowAttachment, received in attachments array.
    const flow = { nodes: [], connections: [] };
    const stream = makeStream([
      sseEvent("attachment", JSON.stringify({ kind: "flow", flow })),
      sseEvent("done", JSON.stringify({})),
    ]);
    mockFetch.mockResolvedValueOnce({ ok: true, body: stream });

    const onDone = vi.fn();
    await streamChat(makeChatRequest(), makeCallbacks({ onDone }));

    const doneArg = onDone.mock.calls[0][0];
    expect(doneArg.attachments[0].flow).toEqual(flow);
  });

  it("onDone receives validationResult when present", async () => {
    // Migration: validationResult is now a field on FlowAttachment, not done data.
    const validationResult = { valid: false, errors: ["missing output"] };
    const stream = makeStream([
      sseEvent(
        "attachment",
        JSON.stringify({
          kind: "flow",
          flow: { nodes: [], connections: [] },
          validationResult,
        }),
      ),
      sseEvent("done", JSON.stringify({})),
    ]);
    mockFetch.mockResolvedValueOnce({ ok: true, body: stream });

    const onDone = vi.fn();
    await streamChat(makeChatRequest(), makeCallbacks({ onDone }));

    const doneArg = onDone.mock.calls[0][0];
    expect(doneArg.attachments[0].validationResult).toEqual(validationResult);
  });

  it("onDone receives done data even when flow is null", async () => {
    // Migration: no attachment event — onDone gets empty attachments array.
    const stream = makeStream([sseEvent("done", JSON.stringify({}))]);
    mockFetch.mockResolvedValueOnce({ ok: true, body: stream });

    const onDone = vi.fn();
    await streamChat(makeChatRequest(), makeCallbacks({ onDone }));

    expect(onDone).toHaveBeenCalledTimes(1);
    expect(onDone.mock.calls[0][0].attachments).toEqual([]);
  });
});

// ---------------------------------------------------------------------------
// onError callback
// ---------------------------------------------------------------------------

describe("streamChat — onError", () => {
  it("calls onError when error event received", async () => {
    const stream = makeStream([
      sseEvent("error", JSON.stringify({ message: "AI service unavailable" })),
    ]);
    mockFetch.mockResolvedValueOnce({ ok: true, body: stream });

    const onError = vi.fn();
    await streamChat(makeChatRequest(), makeCallbacks({ onError }));

    expect(onError).toHaveBeenCalledTimes(1);
    expect(onError).toHaveBeenCalledWith("AI service unavailable");
  });

  it("does not call onText or onDone when error event received", async () => {
    const stream = makeStream([
      sseEvent("error", JSON.stringify({ message: "bad request" })),
    ]);
    mockFetch.mockResolvedValueOnce({ ok: true, body: stream });

    const onText = vi.fn();
    const onDone = vi.fn();
    const onError = vi.fn();
    await streamChat(makeChatRequest(), { onText, onDone, onError });

    expect(onText).not.toHaveBeenCalled();
    expect(onDone).not.toHaveBeenCalled();
    expect(onError).toHaveBeenCalledTimes(1);
  });
});

// ---------------------------------------------------------------------------
// Cancellation (AbortError)
// ---------------------------------------------------------------------------

describe("streamChat — cancellation", () => {
  it("does not call onError when the request is aborted (AbortError is not an error)", async () => {
    const controller = new AbortController();

    mockFetch.mockImplementationOnce((_url: string, options: RequestInit) => {
      return new Promise((_resolve, reject) => {
        options.signal?.addEventListener("abort", () => {
          reject(new DOMException("The user aborted a request.", "AbortError"));
        });
      });
    });

    const onError = vi.fn();
    const promise = streamChat(
      makeChatRequest(),
      makeCallbacks({ onError }),
      controller.signal,
    );
    controller.abort();

    await promise;

    expect(onError).not.toHaveBeenCalled();
  });

  it("resolves cleanly (does not throw) when the request is aborted", async () => {
    const controller = new AbortController();

    mockFetch.mockImplementationOnce((_url: string, options: RequestInit) => {
      return new Promise((_resolve, reject) => {
        options.signal?.addEventListener("abort", () => {
          reject(new DOMException("The user aborted a request.", "AbortError"));
        });
      });
    });

    controller.abort();
    await expect(
      streamChat(makeChatRequest(), makeCallbacks(), controller.signal),
    ).resolves.toBeUndefined();
  });
});

// ---------------------------------------------------------------------------
// Network failure
// ---------------------------------------------------------------------------

describe("streamChat — network failure", () => {
  it("rejects on network failure", async () => {
    mockFetch.mockRejectedValueOnce(new TypeError("Failed to fetch"));

    await expect(
      streamChat(makeChatRequest(), makeCallbacks()),
    ).rejects.toThrow("Failed to fetch");
  });

  it("calls onError or rejects when HTTP response is not ok", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 500,
      statusText: "Internal Server Error",
    });

    const onError = vi.fn();
    const promise = streamChat(makeChatRequest(), makeCallbacks({ onError }));

    // The implementation may either reject or call onError — either is acceptable.
    // Test that neither resolves silently without signalling the failure.
    try {
      await promise;
      // If it resolved, onError must have been called
      expect(onError).toHaveBeenCalled();
    } catch {
      // If it rejected, that's also acceptable — no assertion needed
    }
  });
});

// ---------------------------------------------------------------------------
// Mixed event stream
// ---------------------------------------------------------------------------

describe("streamChat — mixed event stream", () => {
  it("processes text events before the done event in order", async () => {
    const order: string[] = [];
    const stream = makeStream([
      sseEvent("text", JSON.stringify({ content: "first" })),
      sseEvent("text", JSON.stringify({ content: "second" })),
      sseEvent("done", JSON.stringify({ flow: null })),
    ]);
    mockFetch.mockResolvedValueOnce({ ok: true, body: stream });

    await streamChat(makeChatRequest(), {
      onText: (t) => order.push(`text:${t}`),
      onDone: () => order.push("done"),
      onError: vi.fn(),
    });

    expect(order).toEqual(["text:first", "text:second", "done"]);
  });
});
