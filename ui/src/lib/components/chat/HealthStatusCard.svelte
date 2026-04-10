<script lang="ts">
  import type { HealthAttachment } from "$lib/types/chat";

  interface Props {
    attachment: HealthAttachment;
  }

  let { attachment }: Props = $props();
</script>

<div data-testid="health-status-card" class="health-status-card">
  <div class="health-header">
    <span class="component-name">{attachment.componentName}</span>
    <span
      class="status-indicator {attachment.status}"
      data-status={attachment.status}
    >
      {attachment.status}
    </span>
  </div>

  {#if attachment.message}
    <p class="health-message">{attachment.message}</p>
  {/if}

  {#if attachment.metrics && Object.keys(attachment.metrics).length > 0}
    <dl class="health-metrics">
      {#each Object.entries(attachment.metrics) as [key, value]}
        <dt>{key}</dt>
        <dd>{value}</dd>
      {/each}
    </dl>
  {/if}

  {#if attachment.lastCheck}
    <p class="health-last-check">Last check: {attachment.lastCheck}</p>
  {/if}
</div>

<style>
  .health-status-card {
    padding: var(--pico-spacing, 0.75rem);
    border: 1px solid var(--pico-muted-border-color, #e0e0e0);
    border-radius: var(--pico-border-radius, 4px);
    background: var(--pico-card-background-color, #fff);
  }

  .health-header {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-bottom: 0.25rem;
  }

  .component-name {
    font-weight: 600;
  }

  .status-indicator {
    padding: 0.125rem 0.5rem;
    border-radius: 9999px;
    font-size: 0.75rem;
    font-weight: 500;
    text-transform: capitalize;
  }

  .status-indicator.healthy {
    background: #d1fae5;
    color: #065f46;
  }

  .status-indicator.degraded {
    background: #fef3c7;
    color: #92400e;
  }

  .status-indicator.unhealthy {
    background: #fee2e2;
    color: #991b1b;
  }

  .status-indicator.unknown {
    background: #f3f4f6;
    color: #6b7280;
  }

  .health-message {
    margin: 0.25rem 0 0;
    font-size: 0.875rem;
    color: var(--pico-muted-color, #6b7280);
  }

  .health-metrics {
    margin: 0.5rem 0 0;
    display: grid;
    grid-template-columns: auto 1fr;
    gap: 0.125rem 0.5rem;
    font-size: 0.8125rem;
  }

  .health-metrics dt {
    color: var(--pico-muted-color, #6b7280);
  }

  .health-metrics dd {
    margin: 0;
    font-variant-numeric: tabular-nums;
  }

  .health-last-check {
    margin: 0.25rem 0 0;
    font-size: 0.75rem;
    color: var(--pico-muted-color, #9ca3af);
  }
</style>
