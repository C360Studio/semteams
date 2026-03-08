<script lang="ts">
  import type { ClassificationMeta } from "$lib/types/graph";

  interface Props {
    classification?: ClassificationMeta | null;
  }

  let { classification = null }: Props = $props();

  let expanded = $state(false);

  function toggle() {
    expanded = !expanded;
  }

  const confidencePct = $derived(
    classification ? Math.round(classification.confidence * 100) : 0,
  );

  const tierLabel = $derived(
    classification ? `T${classification.tier}` : "",
  );
</script>

{#if classification}
  <button
    data-testid="nlq-debug-badge"
    class="nlq-debug-badge"
    aria-label="NLQ classification debug info"
    aria-expanded={expanded}
    onclick={toggle}
  >
    <span class="tier-label">{tierLabel}</span>
    <span class="confidence">{confidencePct}%</span>
  </button>

  {#if expanded}
    <div data-testid="nlq-debug-expanded" class="nlq-debug-expanded">
      <dl>
        <dt>Tier</dt>
        <dd>{classification.tier}</dd>
        <dt>Confidence</dt>
        <dd>{confidencePct}%</dd>
        <dt>Intent</dt>
        <dd>{classification.intent}</dd>
      </dl>
    </div>
  {/if}
{/if}

<style>
  .nlq-debug-badge {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 3px 8px;
    background: var(--ui-surface-secondary, #f0f4ff);
    border: 1px solid var(--ui-border-subtle, #c0c8e0);
    border-radius: 12px;
    font-size: 11px;
    cursor: pointer;
    color: var(--ui-text-primary, #333);
  }

  .nlq-debug-badge:hover {
    background: var(--ui-surface-hover, #e0e8ff);
  }

  .tier-label {
    font-weight: 700;
    color: var(--ui-interactive-primary, #3366cc);
  }

  .confidence {
    color: var(--ui-text-secondary, #555);
  }

  .nlq-debug-expanded {
    padding: 8px 12px;
    background: var(--ui-surface-secondary, #f5f7ff);
    border: 1px solid var(--ui-border-subtle, #dde);
    border-radius: 6px;
    font-size: 12px;
  }

  .nlq-debug-expanded dl {
    margin: 0;
    display: grid;
    grid-template-columns: auto 1fr;
    gap: 4px 12px;
  }

  .nlq-debug-expanded dt {
    font-weight: 600;
    color: var(--ui-text-secondary, #666);
  }

  .nlq-debug-expanded dd {
    margin: 0;
    color: var(--ui-text-primary, #222);
  }
</style>
