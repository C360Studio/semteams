import type { ChatRequest } from "$lib/types/chat";
import type { Flow } from "$lib/types/flow";

export interface ChatStreamCallbacks {
  onText: (content: string) => void;
  onDone: (data: { flow?: Partial<Flow>; validationResult?: unknown }) => void;
  onError: (error: string) => void;
}

export async function streamChat(
  request: ChatRequest,
  callbacks: ChatStreamCallbacks,
  signal?: AbortSignal,
): Promise<void> {
  // If signal is already aborted before we start, resolve silently.
  if (signal?.aborted) {
    return;
  }

  let response: Response;
  try {
    response = await fetch("/api/ai/chat", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(request),
      signal,
    });
  } catch (err) {
    if (err instanceof DOMException && err.name === "AbortError") {
      return;
    }
    throw err;
  }

  if (!response.ok) {
    callbacks.onError(`HTTP ${response.status}: ${response.statusText}`);
    return;
  }

  const body = response.body;
  if (!body) return;

  const reader = body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;

    buffer += decoder.decode(value, { stream: true });

    // Split on double newline (SSE event boundary)
    const parts = buffer.split("\n\n");
    // Last part may be incomplete — keep it in the buffer
    buffer = parts.pop() ?? "";

    for (const part of parts) {
      if (!part.trim()) continue;

      let eventName = "";
      let dataLine = "";

      for (const line of part.split("\n")) {
        if (line.startsWith("event: ")) {
          eventName = line.slice("event: ".length).trim();
        } else if (line.startsWith("data: ")) {
          dataLine = line.slice("data: ".length).trim();
        }
      }

      if (!eventName || !dataLine) continue;

      let parsed: Record<string, unknown>;
      try {
        parsed = JSON.parse(dataLine);
      } catch {
        continue;
      }

      if (eventName === "text") {
        callbacks.onText(parsed["content"] as string);
      } else if (eventName === "done") {
        callbacks.onDone(parsed as { flow?: Partial<Flow>; validationResult?: unknown });
      } else if (eventName === "error") {
        callbacks.onError(parsed["message"] as string);
      }
    }
  }
}
