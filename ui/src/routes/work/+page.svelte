<script lang="ts">
  import { untrack } from "svelte";
  import { page } from "$app/state";
  import { replaceState } from "$app/navigation";
  import { createWorkStore } from "$lib/stores/workStore.svelte";
  import { summarizeItemOverlays } from "$lib/utils/workView";
  import WorkBoard from "$lib/components/work/WorkBoard.svelte";
  import WorkItemOverview from "$lib/components/work/WorkItemOverview.svelte";
  import WorkSourceState from "$lib/components/work/WorkSourceState.svelte";
  import WorkTable from "$lib/components/work/WorkTable.svelte";

  type View = "board" | "table";

  // Read-only work lens (design D5). The URL is the selection: `?view=board|table`
  // and `?item=owner/repo%23n`, written with shallow replaceState so a refresh or a
  // shared link restores the lens and the back button keeps meaning "leave the page".
  const store = createWorkStore();

  let view = $derived<View>(page.url.searchParams.get("view") === "table" ? "table" : "board");
  let itemRef = $derived(page.url.searchParams.get("item"));
  let sourceState = $derived(store.sourceState);
  let selection = $derived(store.selection);

  // load() and select() read the store's own state; keep that out of these
  // effects' dependencies so only the URL (itemRef) drives selection.
  $effect(() => {
    untrack(() => void store.load());
    return () => store.dispose();
  });

  $effect(() => {
    const ref = itemRef;
    untrack(() => store.select(ref));
  });

  function updateUrl(change: (params: URLSearchParams) => void): void {
    const url = new URL(page.url);
    change(url.searchParams);
    // Same page, different search: resolve() is typed against route literals and
    // cannot express that.
    // eslint-disable-next-line svelte/no-navigation-without-resolve
    replaceState(url, page.state);
  }

  function setView(next: View): void {
    updateUrl((params) => {
      if (next === "table") params.set("view", "table");
      else params.delete("view");
    });
  }

  function toggleItem(ref: string): void {
    updateUrl((params) => {
      if (params.get("item") === ref) params.delete("item");
      else params.set("item", ref);
    });
  }

  // Closing unmounts the control that has focus, so focus has to be put somewhere
  // on purpose: the card or row that opened the panel (found by its item ref, so a
  // deep link works too), else the page heading when the item has no card here.
  let workMain = $state<HTMLElement>();
  let pageHeading = $state<HTMLElement>();

  function focusOpener(ref: string): void {
    const opener = Array.from(workMain?.querySelectorAll<HTMLElement>("[data-item]") ?? []).find(
      (element) => element.dataset.item === ref,
    );
    const target = opener?.matches("button") ? opener : opener?.querySelector<HTMLElement>("button");
    (target ?? pageHeading)?.focus();
  }

  function closeItem(): void {
    const ref = itemRef;
    updateUrl((params) => params.delete("item"));
    if (ref) focusOpener(ref);
  }
</script>

<svelte:head>
  <title>Work - SemTeams</title>
</svelte:head>

<div class="work-page" data-testid="work-page">
  <header class="work-header">
    <h1 tabindex="-1" bind:this={pageHeading}>Work</h1>
    <div class="view-toggle" role="group" aria-label="View" data-testid="work-view-toggle">
      <button
        type="button"
        data-testid="view-board"
        aria-pressed={view === "board"}
        onclick={() => setView("board")}
      >
        Board
      </button>
      <button
        type="button"
        data-testid="view-table"
        aria-pressed={view === "table"}
        onclick={() => setView("table")}
      >
        Table
      </button>
    </div>
    <button
      type="button"
      class="refresh"
      data-testid="work-refresh"
      disabled={store.refreshing}
      onclick={() => void store.refresh()}
    >
      {store.refreshing ? "Refreshing…" : "Refresh"}
    </button>
  </header>

  <div class="work-body">
    <div class="work-main" bind:this={workMain}>
      {#if sourceState.kind !== "ready"}
        <WorkSourceState state={sourceState} onretry={() => void store.load()} />
      {:else if view === "board"}
        <WorkBoard
          programs={store.programs}
          boards={store.boards}
          overlaysFor={(ref) => summarizeItemOverlays(store.linkedRunsFor(ref))}
          selectedRef={itemRef}
          onselect={toggleItem}
        />
      {:else}
        <WorkTable
          programs={store.programs}
          boards={store.boards}
          overlaysFor={(ref) => summarizeItemOverlays(store.linkedRunsFor(ref))}
          selectedRef={itemRef}
          onselect={toggleItem}
        />
      {/if}
    </div>

    {#if sourceState.kind === "ready" && selection.kind !== "none"}
      <div class="work-side" data-testid="work-side">
        {#if selection.kind === "item"}
          <WorkItemOverview
            item={selection.item}
            linkedRuns={store.linkedRunsFor(selection.ref)}
            onloadruns={() => void store.loadLinkedRuns()}
            onclose={closeItem}
          />
        {:else}
          <section class="side-state" data-testid="work-item-state" aria-label="Work item">
            {#if selection.kind === "loading"}
              <p role="status" aria-busy="true">Loading {selection.ref}&hellip;</p>
            {:else}
              <p role="alert">{selection.ref} cannot be shown: {selection.message}</p>
            {/if}
            <button type="button" onclick={closeItem}>Close</button>
          </section>
        {/if}
      </div>
    {/if}
  </div>
</div>

<style>
  .work-page {
    display: flex;
    flex-direction: column;
    width: 100%;
    height: 100%;
    overflow: hidden;
  }

  .work-header {
    display: flex;
    align-items: center;
    gap: 1rem;
    padding: 0.75rem 1rem;
    border-bottom: 1px solid var(--ui-border-subtle, #e5e7eb);
  }

  .work-header h1 {
    margin: 0;
    font-size: 1.125rem;
  }

  .work-header h1:focus-visible {
    outline: 2px solid var(--ui-focus-ring, #0f62fe);
    outline-offset: 2px;
  }

  .view-toggle {
    display: inline-flex;
  }

  .view-toggle button,
  .refresh,
  .side-state button {
    padding: 0.25rem 0.75rem;
    border: 1px solid var(--ui-border-strong, #9ca3af);
    background: var(--ui-surface-primary, #fff);
    color: var(--ui-text-primary, #111827);
    font: inherit;
    font-size: 0.8125rem;
    cursor: pointer;
  }

  .view-toggle button:first-child {
    border-radius: var(--radius-md, 6px) 0 0 var(--radius-md, 6px);
  }

  .view-toggle button:last-child {
    border-left: none;
    border-radius: 0 var(--radius-md, 6px) var(--radius-md, 6px) 0;
  }

  .refresh,
  .side-state button {
    border-radius: var(--radius-md, 6px);
  }

  .refresh {
    margin-left: auto;
  }

  .view-toggle button[aria-pressed="true"] {
    background: var(--ui-interactive-primary, #3b82f6);
    border-color: var(--ui-interactive-primary, #3b82f6);
    color: var(--ui-text-on-primary, #fff);
    font-weight: 700;
  }

  .refresh:disabled {
    cursor: progress;
    opacity: 0.7;
  }

  .view-toggle button:focus-visible,
  .refresh:focus-visible,
  .side-state button:focus-visible {
    outline: 2px solid var(--ui-focus-ring, #0f62fe);
    outline-offset: 2px;
  }

  .work-body {
    display: flex;
    flex: 1;
    min-height: 0;
  }

  .work-main {
    flex: 1;
    min-width: 0;
    padding: 1rem;
    overflow: auto;
  }

  .work-side {
    flex: 0 0 24rem;
    max-width: 40%;
    overflow-y: auto;
    border-left: 1px solid var(--ui-border-subtle, #e5e7eb);
    background: var(--ui-surface-primary, #fff);
  }

  .side-state {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
    padding: 1rem;
    font-size: 0.875rem;
  }

  .side-state p {
    margin: 0;
  }
</style>
