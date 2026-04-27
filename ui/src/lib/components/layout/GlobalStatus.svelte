<script lang="ts">
  // Global status surface for the top-right of TopNav. Houses the
  // connection indicator and the "needs you" attention badge today;
  // user menu / notifications / breaker-state will land here later.
  import { agentStore } from "$lib/stores/agentStore.svelte";
  import { taskStore } from "$lib/stores/taskStore.svelte";
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

  <span
    class="connection-dot"
    data-testid="connection-status"
    data-connected={agentStore.connected}
    title={agentStore.connected ? "Connected" : "Connecting…"}
    aria-label={agentStore.connected ? "Connected" : "Connecting"}
  ></span>
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

  .connection-dot {
    display: inline-block;
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--status-error, #ef4444);
    flex-shrink: 0;
    transition: background 0.2s;
    cursor: help;
  }

  .connection-dot[data-connected="true"] {
    background: var(--status-success, #22c55e);
  }
</style>
