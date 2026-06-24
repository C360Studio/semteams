<script lang="ts">
  import { agentApi } from "$lib/services/agentApi";
  import { getTriples, type RawTriple } from "$lib/services/runStatusApi";
  import type { LoopTrajectory } from "$lib/types/agent";
  import TaskTrace from "./TaskTrace.svelte";

  interface Props {
    loopId: string;
    runEntityId?: string | null;
  }

  let { loopId, runEntityId = null }: Props = $props();

  let trajectory = $state<LoopTrajectory | null>(null);
  let triples = $state<RawTriple[]>([]);
  let lastError = $state<string | null>(null);
  let requestSeq = 0;
  let displayedLoopId: string | null = null;
  let displayedRunEntityId: string | null = null;

  async function refresh() {
    const requestedLoopId = loopId;
    const requestedRunEntityId = runEntityId;
    const requestId = ++requestSeq;
    if (displayedLoopId !== requestedLoopId || displayedRunEntityId !== requestedRunEntityId) {
      trajectory = null;
      triples = [];
      lastError = null;
      displayedLoopId = requestedLoopId;
      displayedRunEntityId = requestedRunEntityId;
    }
    try {
      const [nextTrajectory, nextTriples] = await Promise.all([
        agentApi.getLoopTrajectory(requestedLoopId),
        requestedRunEntityId
          ? getTriples({ subject: requestedRunEntityId, limit: 250 })
          : Promise.resolve([]),
      ]);
      if (
        requestId !== requestSeq ||
        requestedLoopId !== loopId ||
        requestedRunEntityId !== runEntityId
      ) {
        return;
      }
      trajectory = nextTrajectory;
      triples = Array.isArray(nextTriples) ? nextTriples : [];
      lastError = null;
    } catch (err) {
      if (
        requestId !== requestSeq ||
        requestedLoopId !== loopId ||
        requestedRunEntityId !== runEntityId
      ) {
        return;
      }
      lastError = err instanceof Error ? err.message : String(err);
    }
  }

  $effect(() => {
    const id = loopId;
    const runId = runEntityId;
    if (!id) return;
    void runId;
    void refresh();
  });

  const trajectoryStepCount = $derived(trajectory?.steps.length ?? 0);
</script>

<section class="evidence-panel" data-testid="run-evidence-panel">
  <header class="evidence-header">
    <div>
      <h3>Evidence</h3>
      <p>Raw trajectory, message log, and run graph facts behind this health state.</p>
      <p class="scope-note">
        Trajectory and message log follow the focused loop; graph triples follow the run entity.
      </p>
    </div>
    <button
      type="button"
      class="refresh-btn"
      onclick={refresh}
      aria-label="Refresh evidence"
      title="Refresh evidence"
    >↻</button>
  </header>

  {#if lastError}
    <p class="evidence-error" role="alert" data-testid="run-evidence-error">
      Couldn't load evidence: {lastError}
    </p>
  {/if}

  <dl class="evidence-grid" data-testid="run-evidence-counts">
    <div>
      <dt>Trajectory</dt>
      <dd>{trajectoryStepCount} step{trajectoryStepCount === 1 ? "" : "s"}</dd>
    </div>
    <div>
      <dt>Graph triples</dt>
      <dd>{triples.length}</dd>
    </div>
    <div>
      <dt>Run entity</dt>
      <dd title={runEntityId ?? undefined}>{runEntityId ? runEntityId.split(".").slice(-1)[0] : "unknown"}</dd>
    </div>
  </dl>

  <details class="evidence-section" open>
    <summary>Raw trajectory</summary>
    {#if trajectory}
      <pre data-testid="run-evidence-trajectory">{JSON.stringify(trajectory, null, 2)}</pre>
    {:else}
      <p class="empty">Trajectory has not loaded yet.</p>
    {/if}
  </details>

  <details class="evidence-section">
    <summary>Message log</summary>
    <TaskTrace {loopId} />
  </details>

  <details class="evidence-section" open>
    <summary>Run graph triples</summary>
    {#if !runEntityId}
      <p class="empty">Run entity id is not available yet.</p>
    {:else if triples.length === 0}
      <p class="empty">No triples returned for this run entity.</p>
    {:else}
      <div class="triple-list" data-testid="run-evidence-triples">
        {#each triples as triple, idx (`${triple.predicate}-${idx}`)}
          <div class="triple-row">
            <span class="triple-predicate">{triple.predicate}</span>
            <span class="triple-object">{triple.object}</span>
          </div>
        {/each}
      </div>
    {/if}
  </details>
</section>

<style>
  .evidence-panel {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }

  .evidence-header {
    display: flex;
    align-items: flex-start;
    gap: 0.75rem;
  }

  h3 {
    margin: 0;
    font-size: 0.875rem;
    color: var(--ui-text-primary, #111827);
  }

  .evidence-header p {
    margin: 0.125rem 0 0;
    font-size: 0.75rem;
    line-height: 1.35;
    color: var(--ui-text-secondary, #4b5563);
  }

  .evidence-header .scope-note {
    color: var(--ui-text-tertiary, #6b7280);
  }

  .refresh-btn {
    all: unset;
    margin-left: auto;
    cursor: pointer;
    width: 1.625rem;
    height: 1.625rem;
    border: 1px solid var(--ui-border-subtle, #d1d5db);
    border-radius: 4px;
    text-align: center;
    color: var(--ui-text-secondary, #4b5563);
    background: var(--ui-surface-primary, #fff);
  }

  .refresh-btn:hover {
    background: var(--ui-surface-tertiary, #e5e7eb);
    color: var(--ui-text-primary, #111827);
  }

  .refresh-btn:focus-visible {
    outline: 2px solid var(--ui-interactive-primary, #3b82f6);
    outline-offset: 2px;
  }

  .evidence-error {
    margin: 0;
    padding: 0.5rem;
    border: 1px solid #fecaca;
    border-radius: 4px;
    color: #991b1b;
    background: #fef2f2;
    font-size: 0.75rem;
  }

  .evidence-grid {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 0.5rem;
  }

  .evidence-grid div {
    min-width: 0;
    padding: 0.5rem;
    border: 1px solid var(--ui-border-subtle, #e5e7eb);
    border-radius: 6px;
    background: var(--ui-surface-primary, #fff);
  }

  dt {
    font-size: 0.625rem;
    font-weight: 700;
    color: var(--ui-text-tertiary, #6b7280);
    text-transform: uppercase;
  }

  dd {
    margin: 0.125rem 0 0;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 0.75rem;
    font-weight: 600;
    color: var(--ui-text-primary, #111827);
  }

  .evidence-section {
    border: 1px solid var(--ui-border-subtle, #e5e7eb);
    border-radius: 6px;
    background: var(--ui-surface-primary, #fff);
    overflow: hidden;
  }

  summary {
    cursor: pointer;
    padding: 0.5rem 0.625rem;
    font-size: 0.75rem;
    font-weight: 700;
    color: var(--ui-text-primary, #111827);
    background: var(--ui-surface-secondary, #f9fafb);
  }

  summary:focus-visible {
    outline: 2px solid var(--ui-interactive-primary, #3b82f6);
    outline-offset: -2px;
  }

  pre {
    margin: 0;
    max-height: 18rem;
    overflow: auto;
    padding: 0.625rem;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.6875rem;
    line-height: 1.45;
    white-space: pre-wrap;
    word-break: break-word;
    color: var(--ui-text-primary, #111827);
  }

  .empty {
    margin: 0;
    padding: 0.625rem;
    font-size: 0.75rem;
    color: var(--ui-text-tertiary, #6b7280);
  }

  .triple-list {
    max-height: 18rem;
    overflow: auto;
  }

  .triple-row {
    display: grid;
    grid-template-columns: minmax(9rem, 45%) 1fr;
    gap: 0.5rem;
    padding: 0.375rem 0.625rem;
    border-top: 1px solid var(--ui-border-subtle, #f3f4f6);
    font-size: 0.6875rem;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  }

  .triple-predicate {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    color: var(--ui-text-secondary, #4b5563);
  }

  .triple-object {
    min-width: 0;
    overflow-wrap: anywhere;
    color: var(--ui-text-primary, #111827);
  }
</style>
