<script lang="ts">
  import type { WorkItem } from "$lib/types/work";
  import type { ItemOverlays } from "$lib/utils/workView";
  import OverlayBadge from "./OverlayBadge.svelte";

  interface Props {
    item: WorkItem;
    overlays: ItemOverlays;
    selected?: boolean;
    onselect?: (ref: string) => void;
  }

  let { item, overlays, selected = false, onselect }: Props = $props();

  // PM status is a GitHub field, never run state. Without a configured Project
  // status the column is the documented GitHub-field fallback, and says so.
  let pmStatus = $derived(
    item.pm_status
      ? ({ state: "known", value: item.pm_status } as const)
      : ({
          state: "known",
          value: item.column,
          reason: "derived from GitHub fields; no Project status is configured",
        } as const),
  );
</script>

<!--
  Selection is the only interaction. There is deliberately no draggable
  attribute and no drag handler: read-only is structural, not a disabled
  control. A button holds phrasing content only, hence spans throughout.
-->
<button
  type="button"
  class="work-item-card"
  class:selected
  data-testid="work-item-card"
  data-item={item.ref}
  data-column={item.column}
  aria-pressed={selected}
  onclick={() => onselect?.(item.ref)}
>
  <span class="card-ref">{item.ref}</span>
  <span class="card-title">{item.title}</span>

  {#if item.labels.length > 0}
    <span class="card-labels" data-testid="card-labels">
      {#each item.labels as label (label)}
        <span class="chip">{label}</span>
      {/each}
    </span>
  {/if}

  <span class="card-meta">
    <span data-testid="card-assignees">
      {item.assignees.length > 0 ? `Assigned: ${item.assignees.join(", ")}` : "Unassigned"}
    </span>
    {#if item.milestone}
      <span data-testid="card-milestone">Milestone: {item.milestone}</span>
    {/if}
  </span>

  <span class="card-badges">
    <OverlayBadge name="pm-status" label="PM status" overlay={pmStatus} noneLabel="not set" />
    <OverlayBadge
      name="execution-stage"
      label="Stage"
      overlay={overlays.executionStage}
      noneLabel="no linked run"
    />
    <OverlayBadge
      name="needs-you"
      label="Needs you"
      overlay={overlays.needsYou}
      noneLabel="no linked run"
      format={(value) => (value ? "yes" : "no")}
    />
    <OverlayBadge
      name="verification"
      label="Verification"
      overlay={overlays.verification}
      noneLabel="no linked run"
    />
  </span>
</button>

<style>
  .work-item-card {
    display: flex;
    flex-direction: column;
    gap: 0.375rem;
    width: 100%;
    padding: 0.625rem 0.75rem;
    border: 1px solid var(--ui-border-subtle, #e5e7eb);
    border-radius: 8px;
    background: var(--ui-surface-primary, #fff);
    color: var(--ui-text-primary, #111827);
    font: inherit;
    text-align: left;
    cursor: pointer;
  }

  .work-item-card:hover {
    border-color: var(--ui-border-interactive, #3b82f6);
  }

  .work-item-card.selected {
    border-color: var(--ui-interactive-primary, #3b82f6);
    box-shadow: 0 0 0 2px var(--ui-interactive-primary, #3b82f6);
  }

  .work-item-card:focus-visible {
    outline: 2px solid var(--ui-focus-ring, #0f62fe);
    outline-offset: 2px;
  }

  .card-ref {
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.6875rem;
    color: var(--ui-text-secondary, #6b7280);
  }

  .card-title {
    font-size: 0.875rem;
    font-weight: 600;
    line-height: 1.3;
  }

  .card-labels,
  .card-badges {
    display: flex;
    flex-wrap: wrap;
    gap: 0.25rem;
  }

  .chip {
    padding: 0 0.375rem;
    border: 1px solid var(--ui-border-subtle, #e5e7eb);
    border-radius: 4px;
    font-size: 0.6875rem;
    color: var(--ui-text-secondary, #6b7280);
  }

  .card-meta {
    display: flex;
    flex-direction: column;
    gap: 0.125rem;
    font-size: 0.75rem;
    color: var(--ui-text-secondary, #6b7280);
  }
</style>
