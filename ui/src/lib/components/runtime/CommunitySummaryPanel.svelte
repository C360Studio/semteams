<script lang="ts">
  import type { CommunitySummary } from "$lib/types/graph";

  interface Props {
    summaries?: CommunitySummary[];
    onFilterCommunity?: (communityId: string) => void;
  }

  let { summaries = [], onFilterCommunity }: Props = $props();

  // Track dismissed card IDs locally
  let dismissedIds = $state<Set<string>>(new Set());

  const visibleSummaries = $derived(
    summaries.filter((s) => !dismissedIds.has(s.communityId)),
  );

  function dismiss(communityId: string) {
    dismissedIds = new Set([...dismissedIds, communityId]);
  }

  function handleCardClick(communityId: string) {
    onFilterCommunity?.(communityId);
  }
</script>

{#if visibleSummaries.length > 0}
  <section
    data-testid="community-summary-panel"
    aria-label="Community Summaries"
    class="community-summary-panel"
  >
    <h3 class="panel-heading">Community Summaries</h3>
    {#each visibleSummaries as summary (summary.communityId)}
      <div
        data-testid="community-card"
        class="community-card"
        role="button"
        tabindex="0"
        onclick={() => handleCardClick(summary.communityId)}
        onkeydown={(e) => e.key === "Enter" && handleCardClick(summary.communityId)}
      >
        <div class="card-body">
          <p class="card-text">{summary.text}</p>
          {#if summary.keywords.length > 0}
            <div class="keyword-chips">
              {#each summary.keywords as keyword (keyword)}
                <span data-testid="community-keyword" class="keyword-chip"
                  >{keyword}</span
                >
              {/each}
            </div>
          {/if}
        </div>
        <button
          class="dismiss-button"
          aria-label="Dismiss"
          onclick={(e) => {
            e.stopPropagation();
            dismiss(summary.communityId);
          }}
        >
          &times;
        </button>
      </div>
    {/each}
  </section>
{/if}

<style>
  .community-summary-panel {
    padding: 8px;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .panel-heading {
    margin: 0 0 4px 0;
    font-size: 13px;
    font-weight: 600;
    color: var(--ui-text-secondary, #666);
  }

  .community-card {
    position: relative;
    padding: 10px 36px 10px 12px;
    background: var(--ui-surface-secondary, #f5f5f5);
    border: 1px solid var(--ui-border-subtle, #ddd);
    border-radius: 6px;
    cursor: pointer;
    text-align: left;
  }

  .community-card:hover {
    background: var(--ui-surface-hover, #ebebeb);
  }

  .card-body {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .card-text {
    margin: 0;
    font-size: 13px;
    line-height: 1.4;
    color: var(--ui-text-primary, #222);
  }

  .keyword-chips {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
  }

  .keyword-chip {
    padding: 2px 8px;
    background: var(--ui-interactive-muted, #e0e8ff);
    border-radius: 12px;
    font-size: 11px;
    color: var(--ui-interactive-primary, #3366cc);
  }

  .dismiss-button {
    position: absolute;
    top: 6px;
    right: 8px;
    background: none;
    border: none;
    cursor: pointer;
    font-size: 16px;
    color: var(--ui-text-secondary, #888);
    line-height: 1;
    padding: 2px 4px;
  }

  .dismiss-button:hover {
    color: var(--ui-text-primary, #222);
  }
</style>
