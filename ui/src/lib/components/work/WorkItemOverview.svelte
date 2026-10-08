<script lang="ts">
  import { resolve } from "$app/paths";
  import type { WorkItem } from "$lib/types/work";
  import { guardNone } from "$lib/utils/workView";
  import type { LinkedRunsState } from "$lib/utils/workView";
  import LookupNotice from "./LookupNotice.svelte";
  import OverlayBadge from "./OverlayBadge.svelte";

  interface Props {
    item: WorkItem;
    linkedRuns: LinkedRunsState;
    /** Reads this item's linked runs. Called only on the operator's request. */
    onloadruns: () => void;
    onclose: () => void;
  }

  let { item, linkedRuns, onloadruns, onclose }: Props = $props();

  const DEPENDENCY_SOURCE = {
    "blocked-by": "blocked by",
    "task-list": "task list",
  } as const;

  let runsLookup = $derived(
    linkedRuns.status === "loaded" ? linkedRuns.lookup : linkedRuns.status === "idle" ? "not-loaded" : linkedRuns.status,
  );

  function runLabel(entityId: string | null): string {
    return entityId ? (entityId.split(".").at(-1) ?? entityId) : "run entity not resolvable";
  }

  // The runs lens is the existing `/?task=<coordinator loop id>` selection.
  function drillInHref(loopId: string): string {
    return `${resolve("/")}?task=${encodeURIComponent(loopId)}`;
  }
</script>

<aside class="overview" data-testid="work-item-overview" data-item={item.ref} aria-labelledby="overview-heading">
  <header class="overview-header">
    <div>
      <h2 id="overview-heading" class="overview-ref">{item.ref}</h2>
      <p class="overview-title">{item.title}</p>
    </div>
    <button type="button" class="close" data-testid="work-overview-close" onclick={onclose}>
      Close<span class="sr-only"> overview of {item.ref}</span>
    </button>
  </header>

  <dl class="facts">
    <div class="fact">
      <dt>Purpose</dt>
      <dd class="purpose" data-testid="overview-purpose">{item.body_excerpt || "No description."}</dd>
    </div>
    <div class="fact">
      <dt>Owner</dt>
      <dd data-testid="overview-owner">
        {item.assignees.length > 0 ? item.assignees.join(", ") : "unassigned"}
      </dd>
    </div>
    <div class="fact">
      <dt>Priority</dt>
      <dd data-testid="overview-priority">{item.priority ?? "not set"}</dd>
    </div>
    <div class="fact">
      <dt>Milestone</dt>
      <dd data-testid="overview-milestone">{item.milestone ?? "none"}</dd>
    </div>
    <div class="fact">
      <dt>State</dt>
      <dd>{item.state}</dd>
    </div>
    <div class="fact">
      <dt>Labels</dt>
      <dd data-testid="overview-labels">{item.labels.length > 0 ? item.labels.join(", ") : "none"}</dd>
    </div>
    <div class="fact">
      <dt>Dependencies</dt>
      <dd data-testid="overview-dependencies">
        {#if item.dependencies.length > 0}
          <ul>
            {#each item.dependencies as dependency (dependency.ref)}
              <li>{dependency.ref} <span class="source">({DEPENDENCY_SOURCE[dependency.source]})</span></li>
            {/each}
          </ul>
        {:else}
          none declared
        {/if}
      </dd>
    </div>
    <div class="fact">
      <dt>Delivery context</dt>
      <dd data-testid="overview-delivery" data-state={item.delivery.state}>
        {#if item.delivery.state === "known"}
          <ul>
            {#each item.delivery.value as pullRequest (pullRequest.ref)}
              <li>
                {pullRequest.ref}
                {#if pullRequest.title}&mdash; {pullRequest.title}{/if}
                {#if pullRequest.state}<span class="source">({pullRequest.state})</span>{/if}
              </li>
            {/each}
          </ul>
        {:else if item.delivery.state === "none"}
          no linked pull requests
        {:else}
          unknown
          {#if item.delivery.reason}
            <span class="source">&mdash; {item.delivery.reason}</span>
          {/if}
        {/if}
      </dd>
    </div>
  </dl>

  <section class="runs" aria-labelledby="linked-runs-heading" data-testid="work-linked-runs" data-lookup={runsLookup}>
    <h3 id="linked-runs-heading">Linked runs</h3>

    {#if linkedRuns.status === "idle"}
      <p>Linked runs are read on request, not for the whole board.</p>
      <button type="button" data-testid="load-linked-runs" onclick={onloadruns}>Load linked runs</button>
    {:else if linkedRuns.status === "loading"}
      <p role="status" aria-busy="true">Loading linked runs&hellip;</p>
    {:else if linkedRuns.status === "error"}
      <p role="alert" data-testid="linked-runs-error">
        Linked runs could not be read (<code>{linkedRuns.code}</code>): {linkedRuns.message}
      </p>
      <button type="button" data-testid="load-linked-runs" onclick={onloadruns}>Try again</button>
    {:else}
      <LookupNotice
        lookup={linkedRuns.lookup}
        reason={linkedRuns.reason}
        subject="Linked runs"
        testid="work-linked-runs-lookup"
      />

      {#if linkedRuns.runs.length === 0}
        {#if linkedRuns.lookup === "complete"}
          <p data-testid="no-linked-run">no linked run</p>
        {:else}
          <p data-testid="linked-runs-unknown">
            unknown &mdash; the lookup did not complete, so this item is not reported as having no linked run.
          </p>
        {/if}
      {:else}
        <ul class="run-list">
          {#each linkedRuns.runs as run, index (run.run_entity_id ?? `${run.coordinator_loop_id}-${index}`)}
            <li class="run-row" data-testid="linked-run-row" data-run={run.run_entity_id ?? undefined}>
              <span class="run-id" title={run.run_entity_id ?? undefined}>{runLabel(run.run_entity_id)}</span>
              <span class="run-badges">
                <OverlayBadge
                  name="execution-stage"
                  label="Stage"
                  overlay={guardNone(run.execution_stage, run.lookup)}
                  noneLabel="no linked run"
                />
                <OverlayBadge
                  name="needs-you"
                  label="Needs you"
                  overlay={guardNone(run.needs_you, run.lookup)}
                  noneLabel="no linked run"
                  format={(value) => (value ? "yes" : "no")}
                />
                <OverlayBadge
                  name="verification"
                  label="Verification"
                  overlay={guardNone(run.verification, run.lookup)}
                  noneLabel="no linked run"
                />
              </span>
              <LookupNotice lookup={run.lookup} reason={run.reason} subject="Run facts" />
              {#if run.coordinator_loop_id}
                <!-- The href starts from resolve("/"); the rule cannot see through the query string. -->
                <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
                <a class="drill-in" href={drillInHref(run.coordinator_loop_id)} data-testid="work-drill-in">
                  Open this run in the runs lens
                </a>
              {:else}
                <span class="drill-in-unavailable" data-testid="work-drill-in-unavailable">
                  Drill-in unavailable: the coordinator loop for this run could not be resolved.
                </span>
              {/if}
            </li>
          {/each}
        </ul>
      {/if}

      <button type="button" data-testid="load-linked-runs" onclick={onloadruns}>Reload linked runs</button>
    {/if}
  </section>
</aside>

<style>
  .overview {
    display: flex;
    flex-direction: column;
    gap: 1rem;
    padding: 1rem;
    color: var(--ui-text-primary, #111827);
  }

  .overview-header {
    display: flex;
    justify-content: space-between;
    gap: 0.75rem;
  }

  .overview-ref {
    margin: 0;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.8125rem;
    color: var(--ui-text-secondary, #6b7280);
  }

  .overview-title {
    margin: 0.125rem 0 0;
    font-size: 1rem;
    font-weight: 700;
  }

  .facts {
    display: flex;
    flex-direction: column;
    gap: 0.625rem;
    margin: 0;
  }

  dt {
    font-size: 0.6875rem;
    font-weight: 700;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--ui-text-secondary, #6b7280);
  }

  dd {
    margin: 0.125rem 0 0;
    font-size: 0.8125rem;
  }

  .purpose {
    white-space: pre-wrap;
  }

  ul {
    margin: 0;
    padding-left: 1.125rem;
  }

  .source {
    font-size: 0.75rem;
    color: var(--ui-text-secondary, #6b7280);
  }

  .runs {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    padding-top: 0.75rem;
    border-top: 1px solid var(--ui-border-subtle, #e5e7eb);
  }

  .runs h3 {
    margin: 0;
    font-size: 0.9375rem;
  }

  .runs p {
    margin: 0;
    font-size: 0.8125rem;
  }

  .run-list {
    display: flex;
    flex-direction: column;
    gap: 0.625rem;
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .run-row {
    display: flex;
    flex-direction: column;
    gap: 0.375rem;
    padding: 0.5rem 0.625rem;
    border: 1px solid var(--ui-border-subtle, #e5e7eb);
    border-radius: 8px;
  }

  .run-id {
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.75rem;
    color: var(--ui-text-secondary, #6b7280);
  }

  .run-badges {
    display: flex;
    flex-wrap: wrap;
    gap: 0.25rem;
  }

  .drill-in-unavailable {
    font-size: 0.75rem;
    color: var(--ui-text-secondary, #6b7280);
  }

  .close,
  .runs button {
    align-self: flex-start;
    padding: 0.25rem 0.625rem;
    border: 1px solid var(--ui-border-strong, #9ca3af);
    border-radius: var(--radius-md, 6px);
    background: var(--ui-surface-primary, #fff);
    color: inherit;
    font: inherit;
    font-size: 0.8125rem;
    cursor: pointer;
  }

  .close:focus-visible,
  .runs button:focus-visible,
  .drill-in:focus-visible {
    outline: 2px solid var(--ui-focus-ring, #0f62fe);
    outline-offset: 2px;
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
