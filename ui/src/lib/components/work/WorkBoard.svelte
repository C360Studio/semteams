<script lang="ts">
  import type { Program } from "$lib/types/work";
  import type { RepositoryBoard } from "$lib/stores/workStore.svelte";
  import { slug } from "$lib/utils/workView";
  import type { ItemOverlays } from "$lib/utils/workView";
  import PortfolioSections from "./PortfolioSections.svelte";
  import WorkItemCard from "./WorkItemCard.svelte";

  interface Props {
    programs: Program[];
    boards: Record<string, RepositoryBoard>;
    overlaysFor: (ref: string) => ItemOverlays;
    selectedRef?: string | null;
    onselect?: (ref: string) => void;
  }

  let { programs, boards, overlaysFor, selectedRef = null, onselect }: Props = $props();

  // The source orders the columns and names empty ones, so an empty column still
  // renders. An item whose column the source did not name gets one appended
  // rather than vanishing.
  function columnsOf(board: RepositoryBoard): string[] {
    const extra = board.items
      .map((item) => item.column)
      .filter((column, index, all) => !board.columns.includes(column) && all.indexOf(column) === index);
    return [...board.columns, ...extra];
  }
</script>

<PortfolioSections {programs} {boards} testid="work-board">
  {#snippet repository(board: RepositoryBoard)}
    {@const columns = columnsOf(board)}
    {#if columns.length === 0}
      <p class="empty">No items could be read for this repository.</p>
    {:else}
      <div class="columns">
        {#each columns as column (column)}
          {@const items = board.items.filter((item) => item.column === column)}
          <section
            class="column"
            data-testid="work-column-{slug(column)}"
            aria-label="{column}, {items.length} {items.length === 1 ? 'item' : 'items'}"
          >
            <header class="column-header">
              <h5 class="column-label">{column}</h5>
              <span class="column-count" data-testid="column-count">{items.length}</span>
            </header>
            {#if items.length > 0}
              <ul class="column-cards">
                {#each items as item (item.ref)}
                  <li>
                    <WorkItemCard
                      {item}
                      overlays={overlaysFor(item.ref)}
                      selected={selectedRef === item.ref}
                      {onselect}
                    />
                  </li>
                {/each}
              </ul>
            {:else}
              <p class="column-empty">No items</p>
            {/if}
          </section>
        {/each}
      </div>
    {/if}
  {/snippet}
</PortfolioSections>

<style>
  .columns {
    display: flex;
    gap: 0.75rem;
    align-items: flex-start;
    overflow-x: auto;
    padding-bottom: 0.5rem;
  }

  .column {
    display: flex;
    flex-direction: column;
    flex: 1 0 16rem;
    max-width: 22rem;
    border: 1px solid var(--ui-border-subtle, #e5e7eb);
    border-radius: 10px;
    background: var(--ui-surface-secondary, #f3f4f6);
  }

  .column-header {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.5rem 0.75rem;
    border-bottom: 1px solid var(--ui-border-subtle, #e5e7eb);
  }

  .column-label {
    flex: 1;
    margin: 0;
    font-size: 0.75rem;
    font-weight: 700;
    letter-spacing: 0.06em;
    text-transform: uppercase;
  }

  .column-count {
    min-width: 1.5rem;
    padding: 0.0625rem 0.5rem;
    border: 1px solid var(--ui-border-subtle, #e5e7eb);
    border-radius: 9999px;
    background: var(--ui-surface-primary, #fff);
    font-size: 0.6875rem;
    font-weight: 700;
    text-align: center;
  }

  .column-cards {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    margin: 0;
    padding: 0.625rem;
    list-style: none;
  }

  .column-empty,
  .empty {
    margin: 0;
    padding: 1rem 0.5rem;
    font-size: 0.75rem;
    font-style: italic;
    text-align: center;
    color: var(--ui-text-secondary, #6b7280);
  }
</style>
