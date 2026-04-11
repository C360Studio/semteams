<script lang="ts">
  import type { ChatMessage } from "$lib/types/chat";
  import ChatMessageComponent from "./ChatMessage.svelte";

  interface Props {
    messages: ChatMessage[];
    isStreaming?: boolean;
    streamingContent?: string;
    onApplyFlow?: (messageId: string) => void;
  }

  let {
    messages,
    isStreaming = false,
    streamingContent = "",
    onApplyFlow,
  }: Props = $props();

  let showStreaming = $derived(isStreaming && streamingContent.length > 0);
</script>

<div data-testid="chat-message-list">
  {#if messages.length === 0 && !showStreaming}
    <div data-testid="chat-empty-state">Ask a question, explore the graph, or type / for commands.</div>
  {:else}
    {#each messages as message (message.id)}
      <ChatMessageComponent {message} {onApplyFlow} />
    {/each}
    {#if showStreaming}
      <div data-testid="chat-message-streaming">{streamingContent}</div>
    {/if}
  {/if}
</div>
