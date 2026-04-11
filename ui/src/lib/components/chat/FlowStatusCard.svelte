<script lang="ts">
  import type { FlowStatusAttachment } from "$lib/types/chat";

  interface Props {
    attachment: FlowStatusAttachment;
    onViewFlow?: (flowId: string) => void;
  }

  let { attachment, onViewFlow }: Props = $props();

  function handleFlowNameClick() {
    onViewFlow?.(attachment.flowId);
  }
</script>

<div data-testid="flow-status-card" class="flow-status-card">
  <div class="flow-header">
    <button
      data-testid="flow-status-flow-name"
      class="flow-name-button"
      onclick={handleFlowNameClick}
      type="button"
    >
      {attachment.flowName}
    </button>
    <span class="flow-state">{attachment.state}</span>
  </div>

  <div class="flow-counts">
    <span class="flow-count">{attachment.nodeCount} node{attachment.nodeCount === 1 ? "" : "s"}</span>
    <span class="flow-count-separator">·</span>
    <span class="flow-count">{attachment.connectionCount} connection{attachment.connectionCount === 1 ? "" : "s"}</span>
  </div>

  {#if attachment.warnings && attachment.warnings.length > 0}
    <ul class="flow-warnings">
      {#each attachment.warnings as warning, i (i)}
        <li>{warning}</li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .flow-status-card {
    padding: var(--pico-spacing, 0.75rem);
    border: 1px solid var(--pico-muted-border-color, #e0e0e0);
    border-radius: var(--pico-border-radius, 4px);
    background: var(--pico-card-background-color, #fff);
  }

  .flow-header {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-bottom: 0.25rem;
  }

  .flow-name-button {
    background: none;
    border: none;
    padding: 0;
    margin: 0;
    font-weight: 600;
    cursor: pointer;
    color: var(--pico-primary, #1d4ed8);
    text-decoration: underline;
    font-size: inherit;
  }

  .flow-name-button:hover {
    opacity: 0.8;
  }

  .flow-state {
    font-size: 0.75rem;
    color: var(--pico-muted-color, #6b7280);
    text-transform: capitalize;
  }

  .flow-counts {
    font-size: 0.875rem;
    color: var(--pico-muted-color, #6b7280);
    display: flex;
    gap: 0.25rem;
  }

  .flow-count-separator {
    opacity: 0.5;
  }

  .flow-warnings {
    margin: 0.5rem 0 0;
    padding-left: 1.25rem;
    font-size: 0.875rem;
    color: var(--pico-del-color, #b45309);
  }

  .flow-warnings li {
    margin-bottom: 0.125rem;
  }
</style>
