import type { ChatMessage } from "$lib/types/chat";
import type { Flow } from "$lib/types/flow";

function createChatStore() {
  let messages = $state<ChatMessage[]>([]);
  let isStreaming = $state(false);
  let streamingContent = $state("");
  let error = $state<string | null>(null);

  function makeMessage(
    role: ChatMessage["role"],
    content: string,
    flow?: Partial<Flow>,
  ): ChatMessage {
    return {
      id: crypto.randomUUID(),
      role,
      content,
      timestamp: new Date(),
      flow,
    };
  }

  return {
    get messages() {
      return messages;
    },
    get isStreaming() {
      return isStreaming;
    },
    get streamingContent() {
      return streamingContent;
    },
    get error() {
      return error;
    },

    addUserMessage(content: string): ChatMessage {
      const msg = makeMessage("user", content);
      messages = [...messages, msg];
      return msg;
    },

    addAssistantMessage(content: string, flow?: Partial<Flow>): ChatMessage {
      const msg = makeMessage("assistant", content, flow);
      messages = [...messages, msg];
      return msg;
    },

    addSystemMessage(content: string): ChatMessage {
      const msg = makeMessage("system", content);
      messages = [...messages, msg];
      return msg;
    },

    setStreaming(streaming: boolean) {
      isStreaming = streaming;
      if (!streaming) {
        streamingContent = "";
      }
    },

    appendStreamContent(chunk: string) {
      streamingContent = streamingContent + chunk;
    },

    finalizeStream(fullContent: string, flow?: Partial<Flow>): ChatMessage {
      const msg = makeMessage("assistant", fullContent, flow);
      messages = [...messages, msg];
      isStreaming = false;
      streamingContent = "";
      return msg;
    },

    markFlowApplied(messageId: string) {
      const idx = messages.findIndex((m) => m.id === messageId);
      if (idx === -1) return;
      const updated = [...messages];
      updated[idx] = { ...updated[idx], applied: true };
      messages = updated;
    },

    clearConversation() {
      messages = [];
      error = null;
    },

    setError(err: string | null) {
      error = err;
    },
  };
}

export const chatStore = createChatStore();
