<script lang="ts">
  import type { ContextChip } from "$lib/types/chat";

  interface Props {
    chip: ContextChip;
    onRemove: (chipId: string) => void;
  }

  let { chip, onRemove }: Props = $props();
</script>

<span data-testid="context-chip-{chip.id}" class="chip-pill">
  <span data-testid="chip-kind" class="chip-kind-dot" data-kind={chip.kind} title={chip.kind}>{chip.kind}</span>
  <span class="chip-label">{chip.label}</span>
  <button
    class="chip-remove"
    type="button"
    aria-label="Remove {chip.label}"
    onclick={() => onRemove(chip.id)}
  >
    ×
  </button>
</span>

<style>
  .chip-pill {
    display: inline-flex;
    align-items: center;
    gap: 0.25rem;
    padding: 0.1rem 0.1rem 0.1rem 0.4rem;
    border-radius: 999px;
    border: 1px solid var(--ui-border-subtle);
    background-color: var(--ui-surface-tertiary);
    font-size: 0.75rem;
    line-height: 1.4;
    color: var(--ui-text-secondary);
    max-width: 180px;
  }

  .chip-kind-dot {
    font-size: 0.6rem;
    padding: 0.05rem 0.3rem;
    border-radius: 999px;
    background-color: var(--ui-surface-elevated-2);
    color: var(--ui-text-tertiary);
    text-transform: uppercase;
    letter-spacing: 0.03em;
    white-space: nowrap;
    flex-shrink: 0;
  }

  /* Kind-specific accent colors */
  .chip-kind-dot[data-kind="entity"] {
    background-color: var(--domain-semantic-container);
    color: var(--domain-semantic);
  }

  .chip-kind-dot[data-kind="flow"] {
    background-color: var(--domain-network-container);
    color: var(--domain-network);
  }

  .chip-kind-dot[data-kind="component"] {
    background-color: var(--domain-robotics-container);
    color: var(--domain-robotics);
  }

  .chip-label {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--ui-text-primary);
  }

  .chip-remove {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 1.1rem;
    height: 1.1rem;
    border-radius: 50%;
    border: none;
    background: transparent;
    color: var(--ui-text-tertiary);
    cursor: pointer;
    font-size: 0.9rem;
    padding: 0;
    line-height: 1;
    flex-shrink: 0;
  }

  .chip-remove:hover {
    background-color: var(--ui-surface-elevated-3);
    color: var(--ui-text-primary);
  }
</style>
