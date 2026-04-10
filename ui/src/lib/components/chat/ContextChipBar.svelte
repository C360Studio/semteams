<script lang="ts">
  import type { ContextChip } from "$lib/types/chat";
  import ContextChipPill from "./ContextChipPill.svelte";

  interface Props {
    chips: ContextChip[];
    onRemoveChip: (chipId: string) => void;
    onClearAll: () => void;
  }

  let { chips, onRemoveChip, onClearAll }: Props = $props();

  // Deduplicate chips by id to prevent Svelte's each_key_duplicate error
  let uniqueChips = $derived(
    chips.filter((chip, index, arr) => arr.findIndex((c) => c.id === chip.id) === index),
  );
</script>

{#if chips.length > 0}
  <div data-testid="context-chip-bar">
    <span class="chip-count" aria-label="{chips.length} context chips">{chips.length}</span>
    {#each uniqueChips as chip (chip.id)}
      <ContextChipPill {chip} onRemove={onRemoveChip} />
    {/each}
    {#if chips.length > 1}
      <button
        data-testid="chip-bar-clear-all"
        type="button"
        onclick={onClearAll}
      >
        Clear all
      </button>
    {/if}
  </div>
{/if}

<style>
  div[data-testid="context-chip-bar"] {
    display: flex;
    flex-direction: row;
    align-items: center;
    flex-wrap: wrap;
    gap: 0.25rem;
    padding: 0.375rem 0.5rem;
    border-top: 1px solid var(--ui-border-subtle);
    background-color: var(--ui-surface-secondary);
  }

  .chip-count {
    font-size: 0.7rem;
    color: var(--ui-text-tertiary);
    padding: 0.1rem 0.35rem;
    border-radius: var(--radius-sm);
    border: 1px solid var(--ui-border-subtle);
    background-color: var(--ui-surface-tertiary);
    line-height: 1.4;
  }

  button[data-testid="chip-bar-clear-all"] {
    font-size: 0.7rem;
    padding: 0.1rem 0.4rem;
    border-radius: var(--radius-sm);
    border: 1px solid var(--ui-border-subtle);
    background: transparent;
    color: var(--ui-text-tertiary);
    cursor: pointer;
    line-height: 1.4;
    margin-left: auto;
  }

  button[data-testid="chip-bar-clear-all"]:hover {
    color: var(--ui-text-secondary);
    border-color: var(--ui-border-strong);
  }
</style>
