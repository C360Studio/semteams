<script lang="ts">
  /**
   * NlqSearchBar — Natural language query search input for the knowledge graph.
   *
   * Provides a text input for submitting NLQ queries against the graph,
   * with loading, error, and search-mode states.
   */

  import type { SearchMode } from "$lib/types/graph";

  interface Props {
    onSearch: (query: string) => void;
    onClear: () => void;
    onCancel?: () => void;
    loading?: boolean;
    inSearchMode?: boolean;
    error?: string | null;
    searchMode?: SearchMode;
    onSearchModeChange?: (mode: SearchMode) => void;
  }

  let {
    onSearch,
    onClear,
    onCancel = undefined,
    loading = false,
    inSearchMode = false,
    error = null,
    searchMode = undefined,
    onSearchModeChange = undefined,
  }: Props = $props();

  // Local state: the current input value and a local error flag that clears on typing
  let query = $state("");
  let localErrorDismissed = $state(false);

  // Elapsed time counter for long-running queries
  let elapsedSeconds = $state(0);

  $effect(() => {
    if (loading) {
      elapsedSeconds = 0;
      const intervalId = setInterval(() => {
        elapsedSeconds += 1;
      }, 1000);
      return () => {
        clearInterval(intervalId);
      };
    } else {
      elapsedSeconds = 0;
    }
  });

  // Show error only when prop is set and user hasn't started typing since
  const visibleError = $derived(error && !localErrorDismissed ? error : null);

  function handleInput() {
    if (error) {
      localErrorDismissed = true;
    }
  }

  function submitSearch() {
    const trimmed = query.trim();
    if (!trimmed || loading) return;
    // Reset dismissed state for next error
    localErrorDismissed = false;
    onSearch(trimmed);
  }

  function handleKeydown(event: KeyboardEvent) {
    if (event.key === "Enter") {
      submitSearch();
    }
  }

  function handleClear() {
    query = "";
    localErrorDismissed = false;
    onClear();
  }

  // When a new error prop arrives, reset the dismissed flag so it shows
  $effect(() => {
    if (error) {
      localErrorDismissed = false;
    }
  });

  function handleToggleMode() {
    if (!onSearchModeChange || !searchMode) return;
    const nextMode: SearchMode = searchMode === "replace" ? "merge" : "replace";
    onSearchModeChange(nextMode);
  }
</script>

<div class="nlq-search-bar">
  <label for="nlq-input" class="nlq-label">Search knowledge graph</label>
  <div class="nlq-input-row">
    <input
      id="nlq-input"
      type="text"
      class="nlq-input"
      placeholder="Ask the knowledge graph a question..."
      bind:value={query}
      disabled={loading}
      oninput={handleInput}
      onkeydown={handleKeydown}
      aria-label="Search knowledge graph"
    />
    <button
      type="button"
      class="nlq-search-button"
      onclick={submitSearch}
      disabled={loading}
      aria-label="Search"
    >
      Search
    </button>
    {#if inSearchMode}
      <button
        type="button"
        class="nlq-clear-button"
        onclick={handleClear}
        aria-label="Back to browse"
      >
        Back to browse
      </button>
    {/if}
    {#if searchMode !== undefined}
      <button
        type="button"
        class="nlq-mode-toggle"
        data-testid="search-mode-toggle"
        onclick={handleToggleMode}
        disabled={loading}
        aria-label="Toggle mode: {searchMode}"
      >
        {searchMode === "replace" ? "Replace" : "Merge"}
      </button>
    {/if}
  </div>

  {#if loading}
    <div class="nlq-loading-row">
      <div class="nlq-loading" data-testid="nlq-loading-indicator" aria-live="polite">
        Searching…
      </div>
      <span class="nlq-elapsed" data-testid="nlq-elapsed-time">{elapsedSeconds}s</span>
      <button
        type="button"
        class="nlq-cancel-button"
        data-testid="nlq-cancel-button"
        onclick={() => onCancel?.()}
        aria-label="Cancel"
      >
        Cancel
      </button>
    </div>
  {/if}

  {#if visibleError}
    <div class="nlq-error" role="alert">
      {visibleError}
    </div>
  {/if}
</div>

<style>
  .nlq-search-bar {
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding: 8px 12px;
  }

  .nlq-label {
    font-size: 12px;
    color: var(--ui-text-secondary, #888);
    font-weight: 500;
  }

  .nlq-input-row {
    display: flex;
    gap: 6px;
    align-items: center;
  }

  .nlq-input {
    flex: 1;
    padding: 6px 10px;
    border: 1px solid var(--ui-border-subtle, #ccc);
    border-radius: 4px;
    font-size: 13px;
    background: var(--ui-surface-primary, #fff);
    color: var(--ui-text-primary, #222);
  }

  .nlq-input:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }

  .nlq-search-button,
  .nlq-clear-button {
    padding: 6px 12px;
    border-radius: 4px;
    border: 1px solid var(--ui-border-subtle, #ccc);
    cursor: pointer;
    font-size: 13px;
    white-space: nowrap;
  }

  .nlq-search-button {
    background: var(--ui-interactive-primary, #4a90e2);
    color: white;
    border-color: transparent;
  }

  .nlq-search-button:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }

  .nlq-clear-button {
    background: transparent;
    color: var(--ui-text-secondary, #888);
  }

  .nlq-mode-toggle {
    padding: 6px 12px;
    border-radius: 4px;
    border: 1px solid var(--ui-border-subtle, #ccc);
    cursor: pointer;
    font-size: 13px;
    white-space: nowrap;
    background: var(--ui-surface-secondary, #f5f5f5);
    color: var(--ui-text-primary, #222);
  }

  .nlq-mode-toggle:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }

  .nlq-loading-row {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .nlq-loading {
    font-size: 12px;
    color: var(--ui-text-secondary, #888);
    font-style: italic;
  }

  .nlq-elapsed {
    font-size: 12px;
    color: var(--ui-text-secondary, #888);
    font-variant-numeric: tabular-nums;
  }

  .nlq-cancel-button {
    padding: 2px 8px;
    border-radius: 4px;
    border: 1px solid var(--ui-border-subtle, #ccc);
    cursor: pointer;
    font-size: 12px;
    background: transparent;
    color: var(--ui-text-secondary, #888);
  }

  .nlq-cancel-button:hover {
    background: var(--ui-surface-secondary, #f5f5f5);
  }

  .nlq-error {
    font-size: 12px;
    color: var(--status-error, #d32f2f);
    background: var(--status-error-bg, #fdecea);
    border: 1px solid var(--status-error, #d32f2f);
    border-radius: 4px;
    padding: 4px 8px;
  }
</style>
