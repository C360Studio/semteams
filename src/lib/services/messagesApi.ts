// Messages API client
// Handles fetching historical runtime messages

export interface RuntimeMessage {
  message_id: string;
  timestamp: number;
  subject: string;
  direction: "published" | "received" | "processed";
  component: string;
  [key: string]: unknown; // Additional metadata fields
}

export interface RuntimeMessagesResponse {
  messages: RuntimeMessage[];
  total?: number;
}

export class MessagesApiError extends Error {
  constructor(
    message: string,
    public flowId: string,
    public statusCode: number,
    public details?: unknown,
  ) {
    super(message);
    this.name = "MessagesApiError";
  }
}

export const messagesApi = {
  async fetchMessages(
    flowId: string,
    options?: { limit?: number; offset?: number },
  ): Promise<RuntimeMessagesResponse> {
    // Build URL with query params
    let url = `/flows/${flowId}/runtime/messages`;
    const params = new URLSearchParams();
    if (options?.limit !== undefined)
      params.append("limit", String(options.limit));
    if (options?.offset !== undefined)
      params.append("offset", String(options.offset));
    if (params.toString()) url += `?${params.toString()}`;

    const response = await fetch(url, { method: "GET" });

    if (!response.ok) {
      const error = await response.json().catch(() => ({}));
      throw new MessagesApiError(
        `Failed to fetch messages: ${response.statusText}`,
        flowId,
        response.status,
        error,
      );
    }

    return response.json();
  },
};
