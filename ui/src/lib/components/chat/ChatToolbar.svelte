<script lang="ts">
  interface Props {
    onExportJson?: () => void;
    onNewChat: () => void;
  }

  let { onExportJson, onNewChat }: Props = $props();
</script>

<div data-testid="chat-toolbar" class="chat-toolbar">
  <span class="toolbar-title">Chat</span>
  <div class="toolbar-actions">
    {#if onExportJson}
      <button
        data-testid="chat-export-json"
        type="button"
        aria-label="Export JSON"
        onclick={onExportJson}
        class="toolbar-btn"
      >
        Export
      </button>
    {/if}
    <button
      data-testid="chat-new-chat"
      type="button"
      aria-label="New Chat"
      onclick={onNewChat}
      class="toolbar-btn"
    >
      New
    </button>
  </div>
</div>

<style>
  .chat-toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 8px 12px;
    border-bottom: 1px solid var(--ui-border-subtle);
    flex-shrink: 0;
  }

  .toolbar-title {
    font-size: 12px;
    font-weight: 600;
    color: var(--ui-text-secondary);
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .toolbar-actions {
    display: flex;
    gap: 4px;
  }

  .toolbar-btn {
    padding: 3px 8px;
    border: 1px solid var(--ui-border-subtle);
    border-radius: 4px;
    background: transparent;
    color: var(--ui-text-secondary);
    font-size: 11px;
    font-weight: 500;
    cursor: pointer;
    transition: all 0.15s;
  }

  .toolbar-btn:hover {
    color: var(--ui-text-primary);
    background: var(--ui-surface-secondary);
    border-color: var(--ui-border-default);
  }
</style>
