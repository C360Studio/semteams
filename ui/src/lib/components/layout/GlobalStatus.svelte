<script lang="ts">
  // Top-right of TopNav. Houses the system-status indicator (dot +
  // label, click for details popover), the "needs you" attention
  // pill, and the admin entrypoint. User menu / notifications bell
  // will land here next.
  import { resolve } from "$app/paths";
  import { systemStatus } from "$lib/stores/systemStatus.svelte";
  import { taskStore } from "$lib/stores/taskStore.svelte";
  import StatusPopover from "./StatusPopover.svelte";

  let popoverOpen = $state(false);
</script>

<div class="global-status" data-testid="global-status">
  {#if taskStore.needsAttentionCount > 0}
    <span
      class="attention-badge"
      data-testid="attention-badge"
      title="{taskStore.needsAttentionCount} task{taskStore.needsAttentionCount === 1 ? '' : 's'} need your attention"
    >
      {taskStore.needsAttentionCount} needs you
    </span>
  {/if}

  <a
    class="admin-link"
    href={resolve("/admin")}
    data-testid="admin-link"
    title="Admin"
    aria-label="Admin"
  >
    <span aria-hidden="true">⚙</span>
  </a>

  <div class="status-anchor">
    <button
      class="status-trigger"
      type="button"
      data-testid="status-trigger"
      data-summary={systemStatus.summary}
      onclick={() => (popoverOpen = !popoverOpen)}
      aria-haspopup="dialog"
      aria-expanded={popoverOpen}
      aria-label="System status: {systemStatus.label}"
      title="System status — click for details"
    >
      <span
        class="connection-dot"
        data-testid="connection-status"
        data-summary={systemStatus.summary}
      ></span>
      <span class="status-label">{systemStatus.label}</span>
    </button>

    {#if popoverOpen}
      <StatusPopover onClose={() => (popoverOpen = false)} />
    {/if}
  </div>
</div>

<style>
  .global-status {
    display: flex;
    align-items: center;
    gap: 0.625rem;
    margin-left: auto;
    font-size: 0.75rem;
    color: var(--ui-text-secondary, #6b7280);
  }

  .attention-badge {
    background: var(--col-needs-you, #f97316);
    color: white;
    padding: 0.125rem 0.5rem;
    border-radius: 9999px;
    font-size: 0.6875rem;
    font-weight: 600;
    letter-spacing: 0.01em;
  }

  .admin-link {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 1.5rem;
    height: 1.5rem;
    border-radius: 4px;
    color: var(--ui-text-secondary, #6b7280);
    text-decoration: none;
    font-size: 0.875rem;
    transition: background 0.15s, color 0.15s;
  }

  .admin-link:hover {
    background: var(--ui-surface-tertiary, #e5e7eb);
    color: var(--ui-text-primary, #111827);
  }

  .admin-link:focus-visible {
    outline: 2px solid var(--ui-interactive-primary, #3b82f6);
    outline-offset: 2px;
  }

  /* Anchor lets the absolutely-positioned popover lock to this slot. */
  .status-anchor {
    position: relative;
  }

  .status-trigger {
    all: unset;
    cursor: pointer;
    display: inline-flex;
    align-items: center;
    gap: 0.4375rem;
    padding: 0.1875rem 0.5rem 0.1875rem 0.4375rem;
    border-radius: 9999px;
    color: var(--ui-text-secondary, #6b7280);
    font-size: 0.6875rem;
    font-weight: 500;
    transition: background 0.15s, color 0.15s;
  }

  .status-trigger:hover,
  .status-trigger[aria-expanded="true"] {
    background: var(--ui-surface-tertiary, #e5e7eb);
    color: var(--ui-text-primary, #111827);
  }

  .status-trigger:focus-visible {
    outline: 2px solid var(--ui-interactive-primary, #3b82f6);
    outline-offset: 2px;
  }

  .connection-dot {
    display: inline-block;
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--ui-text-tertiary, #9ca3af);
    flex-shrink: 0;
    transition: background 0.2s;
  }

  .connection-dot[data-summary="healthy"] {
    background: var(--status-success, #22c55e);
  }

  .connection-dot[data-summary="degraded"] {
    background: var(--status-warning, #eab308);
  }

  .connection-dot[data-summary="unhealthy"] {
    background: var(--status-error, #ef4444);
  }

  .connection-dot[data-summary="unknown"] {
    background: var(--ui-text-tertiary, #9ca3af);
  }

  .status-label {
    font-variant-numeric: tabular-nums;
  }
</style>
