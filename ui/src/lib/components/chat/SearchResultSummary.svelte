<script lang="ts">
  import type { SearchResultAttachment } from "$lib/types/chat";

  interface Props {
    attachment: SearchResultAttachment;
    onViewEntity?: (entityId: string) => void;
  }

  let { attachment, onViewEntity }: Props = $props();

  let results = $derived(attachment.results ?? []);
  let totalCount = $derived(attachment.totalCount ?? attachment.count ?? 0);

  function handleClick(id: string) {
    onViewEntity?.(id);
  }
</script>

<div data-testid="search-result-summary">
  <div class="search-header">
    <span class="search-query">{attachment.query}</span>
    <span class="search-count">{totalCount} result{totalCount === 1 ? "" : "s"}</span>
  </div>

  {#if results.length === 0}
    <div class="no-results">No results for "{attachment.query}"</div>
  {:else}
    <ul class="result-list">
      {#each results as result (result.id)}
        <li>
          <button
            data-testid="search-result-item-{result.id}"
            onclick={() => handleClick(result.id)}
            class="result-item"
          >
            <span class="result-label">{result.label}</span>
            <span class="result-type">{result.type}</span>
            {#if result.domain}
              <span class="result-domain">{result.domain}</span>
            {/if}
          </button>
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .search-header {
    display: flex;
    gap: 0.5rem;
    align-items: baseline;
    margin-bottom: 0.5rem;
  }

  .search-query {
    font-weight: 600;
  }

  .search-count {
    font-size: 0.85em;
    color: var(--pico-muted-color, #6c757d);
  }

  .no-results {
    font-style: italic;
    color: var(--pico-muted-color, #6c757d);
  }

  .result-list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  .result-item {
    display: flex;
    gap: 0.5rem;
    align-items: center;
    background: none;
    border: 1px solid var(--pico-muted-border-color, #dee2e6);
    border-radius: 4px;
    padding: 0.4rem 0.6rem;
    cursor: pointer;
    text-align: left;
    width: 100%;
  }

  .result-item:hover {
    background: var(--pico-secondary-background, #f8f9fa);
  }

  .result-label {
    font-weight: 500;
  }

  .result-type {
    font-size: 0.8em;
    color: var(--pico-muted-color, #6c757d);
    text-transform: lowercase;
  }

  .result-domain {
    font-size: 0.8em;
    color: var(--pico-muted-color, #6c757d);
  }
</style>
