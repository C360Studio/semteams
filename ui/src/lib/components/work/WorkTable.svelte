<script lang="ts">
  import type { Program, WorkItem } from "$lib/types/work";
  import type { RepositoryBoard } from "$lib/stores/workStore.svelte";
  import type { ItemOverlays } from "$lib/utils/workView";
  import OverlayBadge from "./OverlayBadge.svelte";
  import PortfolioSections from "./PortfolioSections.svelte";

  interface Props {
    programs: Program[];
    boards: Record<string, RepositoryBoard>;
    overlaysFor: (ref: string) => ItemOverlays;
    selectedRef?: string | null;
    onselect?: (ref: string) => void;
  }

  let { programs, boards, overlaysFor, selectedRef = null, onselect }: Props = $props();

  type SortKey = "title" | "pm_status" | "milestone";
  type SortDirection = "ascending" | "descending";

  let sortKey = $state<SortKey | null>(null);
  let sortDirection = $state<SortDirection>("ascending");

  function pmStatusOf(item: WorkItem): string {
    return item.pm_status ?? item.column;
  }

  function sortValue(item: WorkItem, key: SortKey): string {
    if (key === "title") return item.title;
    if (key === "pm_status") return pmStatusOf(item);
    return item.milestone ?? "";
  }

  // An item with no value sorts last in either direction; it is not "smallest".
  function sorted(items: WorkItem[]): WorkItem[] {
    if (sortKey === null) return items;
    const key = sortKey;
    const sign = sortDirection === "ascending" ? 1 : -1;
    return [...items].sort((a, b) => {
      const left = sortValue(a, key);
      const right = sortValue(b, key);
      if (left === right) return 0;
      if (left === "") return 1;
      if (right === "") return -1;
      return sign * left.localeCompare(right);
    });
  }

  function toggleSort(key: SortKey): void {
    if (sortKey === key) {
      sortDirection = sortDirection === "ascending" ? "descending" : "ascending";
    } else {
      sortKey = key;
      sortDirection = "ascending";
    }
  }

  function ariaSort(key: SortKey): SortDirection | "none" {
    return sortKey === key ? sortDirection : "none";
  }

  function runCount(count: string | number | boolean): string {
    return count === 1 ? "1 run" : `${count} runs`;
  }
</script>

<PortfolioSections {programs} {boards} testid="work-table">
  {#snippet repository(board: RepositoryBoard)}
    {#if board.items.length === 0}
      <p class="empty">No items could be read for this repository.</p>
    {:else}
      <!-- The scroll area holds focusable buttons, so it is keyboard-scrollable without a tab stop of its own. -->
      <div class="table-scroll">
        <table>
          <caption class="sr-only">Work items in {board.repository}</caption>
          <thead>
            <tr>
              <th scope="col">Item</th>
              {@render sortableHeader("title", "Title")}
              {@render sortableHeader("pm_status", "PM status")}
              <th scope="col">Priority</th>
              {@render sortableHeader("milestone", "Milestone")}
              <th scope="col">Assignees</th>
              <th scope="col">Execution stage</th>
              <th scope="col">Needs you</th>
              <th scope="col">Verification</th>
              <th scope="col">Linked runs</th>
            </tr>
          </thead>
          <tbody>
            {#each sorted(board.items) as item (item.ref)}
              {@const overlays = overlaysFor(item.ref)}
              <tr data-testid="work-item-row" data-item={item.ref} data-column={item.column}>
                <td>
                  <button
                    type="button"
                    class="item-button"
                    aria-pressed={selectedRef === item.ref}
                    onclick={() => onselect?.(item.ref)}
                  >
                    {item.ref}
                  </button>
                </td>
                <td>{item.title}</td>
                <td>{pmStatusOf(item)}</td>
                <td>{item.priority ?? "not set"}</td>
                <td>{item.milestone ?? "none"}</td>
                <td>{item.assignees.length > 0 ? item.assignees.join(", ") : "unassigned"}</td>
                <td>
                  <OverlayBadge
                    name="execution-stage"
                    overlay={overlays.executionStage}
                    noneLabel="no linked run"
                  />
                </td>
                <td>
                  <OverlayBadge
                    name="needs-you"
                    overlay={overlays.needsYou}
                    noneLabel="no linked run"
                    format={(value) => (value ? "yes" : "no")}
                  />
                </td>
                <td>
                  <OverlayBadge
                    name="verification"
                    overlay={overlays.verification}
                    noneLabel="no linked run"
                  />
                </td>
                <td>
                  <OverlayBadge
                    name="linked-runs"
                    overlay={overlays.linkedRuns}
                    noneLabel="no linked run"
                    format={runCount}
                  />
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  {/snippet}
</PortfolioSections>

{#snippet sortableHeader(key: SortKey, label: string)}
  <th scope="col" aria-sort={ariaSort(key)}>
    <button type="button" class="sort-button" data-testid="sort-{key}" onclick={() => toggleSort(key)}>
      {label}
      <span aria-hidden="true">
        {sortKey === key ? (sortDirection === "ascending" ? "▲" : "▼") : "⇅"}
      </span>
    </button>
  </th>
{/snippet}

<style>
  .table-scroll {
    overflow-x: auto;
    border: 1px solid var(--ui-border-subtle, #e5e7eb);
    border-radius: 8px;
  }

  table {
    width: 100%;
    border-collapse: collapse;
    font-size: 0.8125rem;
  }

  th,
  td {
    padding: 0.4375rem 0.625rem;
    border-bottom: 1px solid var(--ui-border-subtle, #e5e7eb);
    text-align: left;
    vertical-align: top;
  }

  th {
    background: var(--ui-surface-secondary, #f3f4f6);
    font-size: 0.75rem;
    white-space: nowrap;
  }

  .sort-button,
  .item-button {
    padding: 0;
    border: none;
    background: none;
    color: inherit;
    font: inherit;
    cursor: pointer;
  }

  .sort-button {
    font-weight: 700;
  }

  .item-button {
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    color: var(--ui-interactive-primary, #3b82f6);
    text-decoration: underline;
    white-space: nowrap;
  }

  .item-button[aria-pressed="true"] {
    font-weight: 700;
    outline: 2px solid var(--ui-interactive-primary, #3b82f6);
    outline-offset: 2px;
  }

  .sort-button:focus-visible,
  .item-button:focus-visible {
    outline: 2px solid var(--ui-focus-ring, #0f62fe);
    outline-offset: 2px;
  }

  .empty {
    margin: 0;
    font-size: 0.75rem;
    font-style: italic;
    color: var(--ui-text-secondary, #6b7280);
  }

  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    margin: -1px;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
    white-space: nowrap;
    border-width: 0;
  }
</style>
