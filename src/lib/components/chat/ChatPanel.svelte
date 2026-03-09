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
    onLoadJson: (data: unknown) => void;
    onExportJson: () => void;
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
    onLoadJson,
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
    // Read `error` to subscribe to its changes, then reset the dismissed flag
    void error;
    errorDismissed = false;
  });
</script>

<div data-testid="chat-panel">
  <ChatToolbar {onLoadJson} {onExportJson} {onNewChat} />
  {#if error && !errorDismissed}
    <div role="alert">
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
  <ChatMessageList {messages} {isStreaming} {streamingContent} {onApplyFlow} />
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
