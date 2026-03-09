<script lang="ts">
  import type { ChatMessage, FlowAttachment, MessageAttachment } from "$lib/types/chat";
  import FlowDiffSummary from "./FlowDiffSummary.svelte";

  interface Props {
    message: ChatMessage;
    onApplyFlow?: (messageId: string) => void;
    onViewEntity?: (entityId: string) => void;
  }

  let { message, onApplyFlow }: Props = $props();

  // Normalize attachments: handle both array and single-object forms defensively
  let attachmentList = $derived(
    Array.isArray(message.attachments)
      ? message.attachments
      : message.attachments != null
        ? [message.attachments as MessageAttachment]
        : [],
  );
</script>

<div data-testid="chat-message" role="article">
  <div data-testid="chat-message-{message.role}">
    <p>{message.content}</p>
    <time datetime={message.timestamp.toISOString()}>{message.timestamp.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}</time>

    {#each attachmentList as attachment, i (i)}
      {#if attachment.kind === "flow" && message.role === "assistant"}
        {#if attachment.diff}
          <FlowDiffSummary diff={attachment.diff} />
        {/if}
        <button
          data-testid="apply-flow-button"
          type="button"
          disabled={(attachment as FlowAttachment).applied === true}
          onclick={() => onApplyFlow?.(message.id)}
        >
          {(attachment as FlowAttachment).applied ? "Applied" : "Apply to Canvas"}
        </button>
      {:else if attachment.kind === "search-result"}
        <div data-testid="search-result-attachment">
          <span>{attachment.count} results for "{attachment.query}" ({attachment.durationMs}ms)</span>
        </div>
      {:else if attachment.kind === "entity-detail"}
        <div data-testid="entity-detail-attachment">
          <span>{attachment.summary}</span>
        </div>
      {:else if attachment.kind === "error"}
        <div data-testid="error-attachment">
          <span>{attachment.message}</span>
        </div>
      {/if}
    {/each}
  </div>
</div>
