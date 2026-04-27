<script lang="ts">
  // Anchored under the GlobalStatus trigger. Reads from systemStatus.
  // Closes on outside-click or Escape.
  import { systemStatus } from "$lib/stores/systemStatus.svelte";

  interface Props {
    onClose: () => void;
  }

  let { onClose }: Props = $props();

  let popover: HTMLElement | undefined = $state();

  function handleDocClick(e: MouseEvent) {
    if (!popover) return;
    if (e.target instanceof Node && !popover.contains(e.target)) {
      onClose();
    }
  }

  function handleKey(e: KeyboardEvent) {
    if (e.key === "Escape") onClose();
  }

  $effect(() => {
    // Defer the listener registration to the next tick so the click
    // that opened the popover doesn't immediately close it.
    const id = window.setTimeout(() => {
      document.addEventListener("click", handleDocClick);
    }, 0);
    document.addEventListener("keydown", handleKey);
    return () => {
      window.clearTimeout(id);
      document.removeEventListener("click", handleDocClick);
      document.removeEventListener("keydown", handleKey);
    };
  });

  function formatChecked(d: Date | null): string {
    if (!d) return "—";
    const ms = Date.now() - d.getTime();
    if (ms < 5_000) return "just now";
    if (ms < 60_000) return `${Math.round(ms / 1000)}s ago`;
    const m = Math.round(ms / 60_000);
    return `${m}m ago`;
  }
</script>

<div
  class="status-popover"
  data-testid="status-popover"
  role="dialog"
  aria-label="System status"
  bind:this={popover}
>
  <div class="popover-header">
    <span class="popover-title">{systemStatus.label}</span>
    <button
      class="popover-refresh"
      type="button"
      data-testid="status-refresh"
      onclick={() => systemStatus.refresh()}
      aria-label="Refresh status"
      title="Refresh"
    >↻</button>
  </div>

  <ul class="status-list">
    {#each systemStatus.subStatuses as sub (sub.component)}
      <li class="status-row" data-healthy={sub.healthy}>
        <span
          class="row-dot"
          aria-label={sub.healthy ? "OK" : "Issue"}
          data-healthy={sub.healthy}
        ></span>
        <span class="row-name">{sub.component}</span>
        <span class="row-message">{sub.message}</span>
      </li>
    {/each}
  </ul>

  {#if systemStatus.lastError}
    <p class="status-error" data-testid="status-error">
      Couldn't reach /health: {systemStatus.lastError}
    </p>
  {/if}

  <p class="popover-footer">
    Last checked {formatChecked(systemStatus.lastChecked)}
  </p>
</div>

<style>
  .status-popover {
    position: absolute;
    top: calc(100% + 0.375rem);
    right: 0;
    min-width: 18rem;
    max-width: 24rem;
    background: var(--ui-surface-secondary, #f7f7f7);
    border: 1px solid var(--ui-border-subtle, #e5e7eb);
    border-radius: 8px;
    box-shadow: 0 4px 16px rgba(0, 0, 0, 0.25);
    padding: 0.625rem 0;
    z-index: 100;
    font-size: 0.8125rem;
  }

  .popover-header {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0 0.875rem 0.5rem;
    border-bottom: 1px solid var(--ui-border-subtle, #e5e7eb);
    margin-bottom: 0.375rem;
  }

  .popover-title {
    font-weight: 600;
    color: var(--ui-text-primary, #111827);
    flex: 1;
  }

  .popover-refresh {
    all: unset;
    cursor: pointer;
    color: var(--ui-text-secondary, #6b7280);
    font-size: 1rem;
    line-height: 1;
    width: 1.375rem;
    height: 1.375rem;
    text-align: center;
    border-radius: 4px;
  }

  .popover-refresh:hover {
    color: var(--ui-text-primary, #111827);
    background: var(--ui-surface-tertiary, #e5e7eb);
  }

  .status-list {
    list-style: none;
    padding: 0;
    margin: 0;
  }

  .status-row {
    display: grid;
    grid-template-columns: 0.625rem max-content 1fr;
    align-items: center;
    gap: 0.5rem;
    padding: 0.3125rem 0.875rem;
    color: var(--ui-text-secondary, #6b7280);
  }

  .row-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--status-error, #ef4444);
    display: inline-block;
  }

  .row-dot[data-healthy="true"] {
    background: var(--status-success, #22c55e);
  }

  .row-name {
    color: var(--ui-text-primary, #111827);
    font-weight: 500;
  }

  .row-message {
    color: var(--ui-text-tertiary, #9ca3af);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 0.75rem;
  }

  .status-error {
    margin: 0.5rem 0.875rem 0;
    padding: 0.375rem 0.5rem;
    background: #fef2f2;
    border: 1px solid #fecaca;
    border-radius: 4px;
    font-size: 0.75rem;
    color: #991b1b;
  }

  .popover-footer {
    margin: 0.375rem 0.875rem 0;
    font-size: 0.6875rem;
    color: var(--ui-text-tertiary, #9ca3af);
  }
</style>
