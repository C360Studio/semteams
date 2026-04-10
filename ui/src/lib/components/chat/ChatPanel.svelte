<script lang="ts">
  import type { ChatMessage, ContextChip } from "$lib/types/chat";
  import type { SlashCommand } from "$lib/types/slashCommand";
  import ChatToolbar from "./ChatToolbar.svelte";
  import ChatMessageList from "./ChatMessageList.svelte";
  import ChatInput from "./ChatInput.svelte";

  interface Props {
    messages: ChatMessage[];
    isStreaming?: boolean;
    streamingContent?: string;
    error?: string | null;
    onSubmit: (content: string) => void;
    onCancel?: () => void;
    onApplyFlow?: (messageId: string) => void;
    onExportJson?: () => void;
    onNewChat: () => void;
    chips?: ContextChip[];
    onRemoveChip?: (chipId: string) => void;
    onClearChips?: () => void;
    commands?: SlashCommand[];
    onCommandSelect?: (command: SlashCommand) => void;
  }

  let {
    messages,
    isStreaming = false,
    streamingContent = "",
    error = null,
    onSubmit,
    onCancel,
    onApplyFlow,
    onExportJson,
    onNewChat,
    chips,
    onRemoveChip,
    onClearChips,
    commands,
    onCommandSelect,
  }: Props = $props();

  let errorDismissed = $state(false);

  $effect(() => {
    void error;
    errorDismissed = false;
  });
</script>

<div data-testid="chat-panel" class="chat-panel">
  <ChatToolbar {onExportJson} {onNewChat} />
  {#if error && !errorDismissed}
    <div role="alert" class="chat-error">
      {error}
      <button
        data-testid="chat-error-dismiss"
        type="button"
        aria-label="Dismiss error"
        onclick={() => (errorDismissed = true)}
      >
        X
      </button>
    </div>
  {/if}
  <div class="chat-messages">
    <ChatMessageList {messages} {isStreaming} {streamingContent} {onApplyFlow} />
  </div>
  <div class="chat-input-area">
    <ChatInput
      {onSubmit}
      {onCancel}
      {isStreaming}
      {chips}
      {onRemoveChip}
      {onClearChips}
      {commands}
      {onCommandSelect}
    />
  </div>
</div>

<style>
  .chat-panel {
    display: flex;
    flex-direction: column;
    height: 100%;
    overflow: hidden;
  }

  .chat-error {
    padding: 8px 12px;
    margin: 8px;
    background: var(--status-error-bg);
    border: 1px solid var(--status-error);
    border-radius: 4px;
    font-size: 13px;
    display: flex;
    align-items: center;
    justify-content: space-between;
    flex-shrink: 0;
  }

  .chat-messages {
    flex: 1;
    overflow-y: auto;
    min-height: 0;
  }

  .chat-input-area {
    flex-shrink: 0;
    border-top: 1px solid var(--ui-border-subtle);
    padding: 8px;
  }
</style>
