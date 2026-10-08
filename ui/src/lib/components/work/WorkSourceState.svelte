<script lang="ts">
  import type { SourceState } from "$lib/stores/workStore.svelte";

  interface Props {
    state: Exclude<SourceState, { kind: "ready" }>;
    onretry?: () => void;
  }

  let { state, onretry }: Props = $props();
</script>

<!--
  Never an empty board: a source that is not configured, still loading or
  failing says so, so "no work" is only ever shown for a source that answered.
-->
<section
  class="source-state"
  data-testid="work-source-state"
  data-state={state.kind}
  data-code={state.kind === "error" ? state.code : undefined}
  aria-labelledby="work-source-state-heading"
>
  {#if state.kind === "loading"}
    <h2 id="work-source-state-heading">Loading work</h2>
    <p role="status" aria-busy="true">Reading the configured portfolio&hellip;</p>
  {:else if state.kind === "unconfigured"}
    <h2 id="work-source-state-heading">Work source not configured</h2>
    <p>
      No work source is configured, so there is nothing to show yet &mdash; this is not an empty board.
      Set <code>WORK_SOURCE</code> to <code>fixture</code> or <code>github</code> and point
      <code>SEMTEAMS_PORTFOLIO_PATH</code> at a portfolio document, then reload.
    </p>
  {:else}
    <h2 id="work-source-state-heading">Work source unavailable</h2>
    <p role="alert">
      <strong data-testid="work-source-code">{state.code}</strong>: {state.message}
    </p>
    {#if state.issues && state.issues.length > 0}
      <ul class="issues" data-testid="work-source-issues">
        {#each state.issues as issue (issue.path + issue.message)}
          <li><code>{issue.path}</code> {issue.message}</li>
        {/each}
      </ul>
    {/if}
    {#if onretry}
      <button type="button" onclick={onretry}>Try again</button>
    {/if}
  {/if}
</section>

<style>
  .source-state {
    max-width: 40rem;
    padding: 1rem 1.25rem;
    border: 1px dashed var(--ui-border-strong, #9ca3af);
    border-radius: 8px;
    background: var(--ui-surface-secondary, #f3f4f6);
    color: var(--ui-text-primary, #111827);
  }

  h2 {
    margin: 0 0 0.5rem;
    font-size: 1rem;
  }

  p {
    margin: 0 0 0.5rem;
    font-size: 0.875rem;
  }

  .issues {
    margin: 0 0 0.75rem;
    padding-left: 1.25rem;
    font-size: 0.8125rem;
  }

  button:focus-visible {
    outline: 2px solid var(--ui-focus-ring, #0f62fe);
    outline-offset: 2px;
  }
</style>
