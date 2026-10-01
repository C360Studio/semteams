<script lang="ts">
  import type { PageData } from "./$types";

  let { data }: { data: PageData } = $props();

</script>

<svelte:head>
  <title>Flows - SemTeams</title>
</svelte:head>

<div class="flows-page" data-testid="flows-page">
  <header class="page-header">
    <h1 class="page-title">Flows</h1>
    <p class="page-subtitle">
      Read-only inventory. Configured components and their admitted connections come from the configured bootstrap.
      Definitions live in <code>configs/flow-bootstrap.json</code>.
    </p>
  </header>

  {#if data.error}
    <div class="error-banner" role="alert" data-testid="error-banner">
      <strong>Error:</strong>
      {data.error}
    </div>
  {/if}

  {#if data.validation}
    <section class="page-header" aria-label="Composition validation" data-testid="composition-validation">
      <p>Validation: {data.validation.status}</p>
      {#each [...data.validation.errors, ...data.validation.warnings] as finding, index (index)}
        <p>{finding.severity}: {finding.component} — {finding.message}</p>
      {/each}
    </section>
  {/if}

  {#if data.components.length === 0 && !data.error}
    <p class="empty-state">No components in the admitted composition.</p>
  {:else}
    <div class="flow-list" data-testid="flow-list">
      {#each data.components as component (component.instance)}
        <div class="flow-row" data-testid="flow-row">
          <div class="flow-row-main">
            <span class="flow-name">{component.instance}</span>
            {#if component.factory}
              <span class="flow-description">{component.factory}</span>
            {/if}
          </div>
          <div class="flow-row-meta">
            <span class="meta-pill" title="Component count">
              {component.type}
            </span>
            <span class="meta-id" title={component.instance}>
              {component.inputs.length} inputs · {component.outputs.length} outputs
            </span>
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>

<style>
  .flows-page {
    width: 100%;
    height: 100%;
    overflow-y: auto;
    padding: 2rem;
    box-sizing: border-box;
  }

  .page-header {
    max-width: 56rem;
    margin: 0 auto 1.75rem;
  }

  .page-title {
    margin: 0 0 0.5rem;
    font-size: 1.5rem;
    font-weight: 700;
    color: var(--ui-text-primary, #111827);
  }

  .page-subtitle {
    margin: 0;
    font-size: 0.875rem;
    color: var(--ui-text-secondary, #6b7280);
    max-width: 56ch;
    line-height: 1.5;
  }

  .page-subtitle code {
    background: var(--ui-surface-tertiary, #e5e7eb);
    padding: 0.0625rem 0.375rem;
    border-radius: 4px;
    font-size: 0.8125rem;
  }

  .error-banner {
    max-width: 56rem;
    margin: 0 auto 1rem;
    padding: 0.625rem 0.875rem;
    background: #fef2f2;
    border: 1px solid #fecaca;
    border-radius: 6px;
    color: #991b1b;
    font-size: 0.8125rem;
  }

  .empty-state {
    max-width: 56rem;
    margin: 0 auto;
    padding: 2rem;
    text-align: center;
    color: var(--ui-text-tertiary, #9ca3af);
    font-style: italic;
  }

  .flow-list {
    max-width: 56rem;
    margin: 0 auto;
    background: var(--ui-surface-secondary, #f7f7f7);
    border: 1px solid var(--ui-border-subtle, #e5e7eb);
    border-radius: 10px;
    overflow: hidden;
  }

  .flow-row {
    display: flex;
    align-items: center;
    gap: 1rem;
    padding: 0.75rem 1rem;
    border-bottom: 1px solid var(--ui-border-subtle, #e5e7eb);
  }

  .flow-row:last-child {
    border-bottom: none;
  }

  .flow-row-main {
    display: flex;
    flex-direction: column;
    gap: 0.125rem;
    flex: 1;
    min-width: 0;
  }

  .flow-name {
    font-size: 0.9375rem;
    font-weight: 600;
    color: var(--ui-text-primary, #111827);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .flow-description {
    font-size: 0.75rem;
    color: var(--ui-text-secondary, #6b7280);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .flow-row-meta {
    display: flex;
    align-items: center;
    gap: 0.625rem;
    flex-shrink: 0;
  }

  .meta-pill {
    font-size: 0.6875rem;
    font-weight: 600;
    color: var(--ui-text-secondary, #6b7280);
    background: var(--ui-surface-primary, #fff);
    border: 1px solid var(--ui-border-subtle, #e5e7eb);
    padding: 0.125rem 0.5rem;
    border-radius: 9999px;
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }

  .meta-id {
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.6875rem;
    color: var(--ui-text-tertiary, #9ca3af);
    cursor: help;
  }
</style>
