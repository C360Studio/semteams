<script lang="ts">
  import { agentApi } from "$lib/services/agentApi";
  import { taskStore } from "$lib/stores/taskStore.svelte";
  import type { ControlSignal } from "$lib/types/agent";

  let input = $state("");
  let sending = $state(false);
  let error = $state<string | null>(null);

  /** Slash commands that operate on the selected task. */
  const TASK_COMMANDS: Record<string, ControlSignal> = {
    "/approve": "approve",
    "/reject": "reject",
    "/pause": "pause",
    "/resume": "resume",
    "/cancel": "cancel",
  };

  let placeholder = $derived(
    taskStore.selectedTask
      ? `Message task ${taskStore.selectedTask.title}... or /approve, /reject`
      : "What should I work on?",
  );

  async function handleSubmit() {
    const text = input.trim();
    if (!text || sending) return;

    error = null;

    // Check for slash commands before entering the async sending state.
    // The guard must run before `sending = true` to avoid getting stuck
    // in the disabled state when the guard early-returns.
    const firstWord = text.split(/\s+/)[0].toLowerCase();
    if (firstWord in TASK_COMMANDS) {
      if (!taskStore.selectedTask) {
        error = "Select a task first to use slash commands.";
        return;
      }
    }

    sending = true;

    try {
      if (firstWord in TASK_COMMANDS) {
        // Slash command — route to signal API on the selected task.
        const signal = TASK_COMMANDS[firstWord];
        const reason = text.slice(firstWord.length).trim() || undefined;
        await agentApi.sendSignal(taskStore.selectedTask!.id, signal, reason);
      } else {
        // Regular message — dispatch to create a new agent loop.
        await agentApi.sendMessage(text);
      }
      input = "";
    } catch (err) {
      error = err instanceof Error ? err.message : "Failed to send message";
    } finally {
      sending = false;
    }
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      handleSubmit();
    }
  }
</script>

<div class="chat-bar" data-testid="chat-bar">
  {#if error}
    <div class="chat-error" role="alert" data-testid="chat-error">
      {error}
      <button
        class="error-dismiss"
        type="button"
        onclick={() => (error = null)}
        aria-label="Dismiss error"
      >×</button>
    </div>
  {/if}

  {#if taskStore.selectedTask}
    <div class="chat-context" data-testid="chat-context">
      <span class="context-label">Task:</span>
      <span class="context-title">{taskStore.selectedTask.title}</span>
      <span class="context-state {taskStore.selectedTask.state}">
        {taskStore.selectedTask.state.replace(/_/g, " ")}
      </span>
      <button
        class="context-clear"
        type="button"
        onclick={() => taskStore.deselectTask()}
        aria-label="Clear task selection"
      >×</button>
    </div>
  {/if}

  <div class="input-row">
    <input
      class="chat-input"
      type="text"
      bind:value={input}
      {placeholder}
      onkeydown={handleKeydown}
      disabled={sending}
      data-testid="chat-input"
      aria-label="Chat input"
    />
    <button
      class="send-button"
      type="button"
      onclick={handleSubmit}
      disabled={sending || !input.trim()}
      data-testid="send-button"
      aria-label="Send message"
    >
      {sending ? "..." : "Send"}
    </button>
  </div>

  <div class="slash-hints" aria-hidden="true">
    {#if taskStore.selectedTask}
      {#each Object.keys(TASK_COMMANDS) as cmd (cmd)}
        <span class="hint-chip">{cmd}</span>
      {/each}
    {:else}
      <span class="hint-chip">Type a task to get started</span>
    {/if}
  </div>
</div>

<style>
  .chat-bar {
    border-top: 1px solid var(--ui-border-subtle, #e5e7eb);
    background: var(--ui-surface-primary, #fff);
    padding: 0.5rem 1rem;
    flex-shrink: 0;
  }

  .chat-error {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.375rem 0.5rem;
    margin-bottom: 0.375rem;
    background: #fef2f2;
    border: 1px solid #fecaca;
    border-radius: 4px;
    font-size: 0.8125rem;
    color: #991b1b;
  }

  .error-dismiss {
    all: unset;
    cursor: pointer;
    padding: 0 0.25rem;
    font-size: 1rem;
    line-height: 1;
    color: #991b1b;
    opacity: 0.6;
  }

  .error-dismiss:hover {
    opacity: 1;
  }

  .chat-context {
    display: flex;
    align-items: center;
    gap: 0.375rem;
    padding: 0.25rem 0;
    margin-bottom: 0.25rem;
    font-size: 0.75rem;
  }

  .context-label {
    color: var(--ui-text-secondary, #6b7280);
    font-weight: 500;
  }

  .context-title {
    color: var(--ui-text-primary, #111827);
    font-weight: 600;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 200px;
  }

  .context-state {
    padding: 0 0.25rem;
    border-radius: 3px;
    font-size: 0.6875rem;
    font-weight: 600;
    text-transform: capitalize;
  }

  .context-state.awaiting_approval {
    background: #ffedd5;
    color: #9a3412;
  }

  .context-state.executing,
  .context-state.reviewing {
    background: #ccfbf1;
    color: #0f766e;
  }

  .context-state.complete {
    background: #d1fae5;
    color: #065f46;
  }

  .context-state.failed {
    background: #fee2e2;
    color: #991b1b;
  }

  .context-clear {
    all: unset;
    cursor: pointer;
    color: var(--ui-text-secondary, #9ca3af);
    font-size: 0.875rem;
    line-height: 1;
    margin-left: auto;
  }

  .context-clear:hover {
    color: var(--ui-text-primary, #374151);
  }

  .input-row {
    display: flex;
    gap: 0.5rem;
  }

  .chat-input {
    flex: 1;
    padding: 0.5rem 0.75rem;
    border: 1px solid var(--ui-border-subtle, #d1d5db);
    border-radius: 6px;
    font-size: 0.875rem;
    color: var(--ui-text-primary, #111827);
    background: var(--ui-surface-primary, #fff);
    outline: none;
    transition: border-color 0.15s;
  }

  .chat-input:focus {
    border-color: var(--ui-interactive-primary, #3b82f6);
    box-shadow: 0 0 0 1px var(--ui-interactive-primary, #3b82f6);
  }

  .chat-input::placeholder {
    color: var(--ui-text-secondary, #9ca3af);
  }

  .chat-input:disabled {
    opacity: 0.6;
  }

  .send-button {
    padding: 0.5rem 1rem;
    border: none;
    border-radius: 6px;
    background: var(--ui-interactive-primary, #3b82f6);
    color: white;
    font-size: 0.8125rem;
    font-weight: 600;
    cursor: pointer;
    transition: background 0.15s;
    white-space: nowrap;
  }

  .send-button:hover:not(:disabled) {
    background: var(--ui-interactive-primary-hover, #2563eb);
  }

  .send-button:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .slash-hints {
    display: flex;
    gap: 0.25rem;
    padding-top: 0.375rem;
    flex-wrap: wrap;
  }

  .hint-chip {
    font-size: 0.6875rem;
    color: var(--ui-text-secondary, #9ca3af);
    padding: 0.0625rem 0.375rem;
    background: var(--ui-surface-secondary, #f9fafb);
    border-radius: 3px;
  }
</style>
