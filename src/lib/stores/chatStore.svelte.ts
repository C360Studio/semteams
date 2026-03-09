import type {
  ChatMessage,
  ContextChip,
  MessageAttachment,
} from "$lib/types/chat";

function createChatStore() {
  let messages = $state<ChatMessage[]>([]);
  let isStreaming = $state(false);
  let streamingContent = $state("");
  let error = $state<string | null>(null);
  let chips = $state<ContextChip[]>([]);

  function makeMessage(
    role: ChatMessage["role"],
    content: string,
    attachments?: MessageAttachment[],
    snapshotChips?: ContextChip[],
  ): ChatMessage {
    const msg: ChatMessage = {
      id: crypto.randomUUID(),
      role,
      content,
      timestamp: new Date(),
    };
    if (attachments !== undefined) {
      msg.attachments = attachments;
    }
    if (snapshotChips !== undefined && snapshotChips.length > 0) {
      msg.chips = [...snapshotChips];
    }
    return msg;
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
    get chips() {
      return chips;
    },

    addUserMessage(content: string): ChatMessage {
      // Snapshot current chips into the message
      const snapshotChips = chips.length > 0 ? [...chips] : undefined;
      const msg = makeMessage("user", content, undefined, snapshotChips);
      if (snapshotChips) {
        msg.chips = snapshotChips;
      }
      messages = [...messages, msg];
      return msg;
    },

    addAssistantMessage(
      content: string,
      attachments?: MessageAttachment[],
    ): ChatMessage {
      const msg = makeMessage("assistant", content, attachments);
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

    finalizeStream(
      fullContent: string,
      attachments?: MessageAttachment[],
    ): ChatMessage {
      const msg = makeMessage("assistant", fullContent, attachments);
      messages = [...messages, msg];
      isStreaming = false;
      streamingContent = "";
      return msg;
    },

    /**
     * Update an attachment on a specific message by kind.
     * Replaces markFlowApplied() — more general.
     */
    updateAttachment(
      messageId: string,
      attachmentKind: MessageAttachment["kind"],
      update: Record<string, unknown>,
    ) {
      const idx = messages.findIndex((m) => m.id === messageId);
      if (idx === -1) return;

      const msg = messages[idx];
      if (!msg.attachments) return;

      const attachmentIdx = msg.attachments.findIndex(
        (a) => a.kind === attachmentKind,
      );
      if (attachmentIdx === -1) return;

      const updated = [...messages];
      const updatedAttachments = [...msg.attachments];
      updatedAttachments[attachmentIdx] = {
        ...updatedAttachments[attachmentIdx],
        ...update,
      } as MessageAttachment;
      updated[idx] = { ...msg, attachments: updatedAttachments };
      messages = updated;
    },

    /**
     * Kept for backward compatibility with existing page integration.
     * @deprecated Use updateAttachment(id, "flow", { applied: true }) instead.
     */
    markFlowApplied(messageId: string) {
      this.updateAttachment(messageId, "flow", { applied: true });
    },

    // ---------------------------------------------------------------------------
    // Chip management
    // ---------------------------------------------------------------------------

    addChip(chip: ContextChip) {
      // Deduplicate by kind + value
      const isDuplicate = chips.some(
        (c) => c.kind === chip.kind && c.value === chip.value,
      );
      if (isDuplicate) return;

      // Cap at 10
      if (chips.length >= 10) return;

      chips = [...chips, chip];
    },

    removeChip(chipId: string) {
      chips = chips.filter((c) => c.id !== chipId);
    },

    clearChips() {
      chips = [];
    },

    setChips(newChips: ContextChip[]) {
      chips = [...newChips];
    },

    clearConversation() {
      messages = [];
      error = null;
      // chips are intentionally preserved across conversation resets
    },

    setError(err: string | null) {
      error = err;
    },
  };
}

export const chatStore = createChatStore();
