<script lang="ts">
  import { tick } from "svelte";
  import { resolve } from "$app/paths";
  import PendingApprovalSection from "$lib/components/board/PendingApprovalSection.svelte";
  import RunEvidencePanel from "$lib/components/board/RunEvidencePanel.svelte";
  import RunHealthPanel from "$lib/components/board/RunHealthPanel.svelte";
  import RunWaitingSection from "$lib/components/board/RunWaitingSection.svelte";
  import TaskStory from "$lib/components/board/TaskStory.svelte";
  import { taskStore } from "$lib/stores/taskStore.svelte";
  import { controlOutcome } from "$lib/types/control";
  import type { LinkedRun, WorkItem } from "$lib/types/work";
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

  // Opening (or switching to another item) lands focus on the panel's heading, so
  // keyboard and screen reader users arrive where the content is. It keys on the
  // ref, not the item object: a refresh re-reads the same item and must not pull
  // focus back from wherever the operator is.
  let heading = $state<HTMLHeadingElement>();
  let itemRef = $derived(item.ref);
  $effect(() => {
    void itemRef;
    selectedRunLoopId = null;
    activeTab = "activity";
    manualFocusId = null;
    heading?.focus();
  });

  const DEPENDENCY_SOURCE = {
    "blocked-by": "blocked by",
    "task-list": "task list",
  } as const;

  let runsLookup = $derived(
    linkedRuns.status === "loaded" ? linkedRuns.lookup : linkedRuns.status === "idle" ? "not-loaded" : linkedRuns.status,
  );

  let loading = $derived(linkedRuns.status === "loading");
  let loadLabel = $derived(
    {
      idle: "Load linked runs",
      loading: "Loading linked runs\u2026",
      error: "Try again",
      loaded: "Reload linked runs",
    }[linkedRuns.status],
  );

  // The button stays mounted and focusable while the read is in flight. It says
  // so with aria-disabled rather than `disabled`: a disabled control is dropped
  // from the focus order and focus falls back to the page.
  function requestRuns(): void {
    if (!loading) onloadruns();
  }

  function runLabel(entityId: string | null): string {
    return entityId ? (entityId.split(".").at(-1) ?? entityId) : "run entity not resolvable";
  }

  // The runs lens is the existing `/?task=<coordinator loop id>` selection.
  function drillInHref(loopId: string): string {
    return `${resolve("/")}?task=${encodeURIComponent(loopId)}`;
  }

  type ResolvedLinkedRun = LinkedRun & { coordinator_loop_id: string };
  type InlineRunTab = "activity" | "evidence";
  const INLINE_RUN_TABS: { id: InlineRunTab; label: string }[] = [
    { id: "activity", label: "Activity" },
    { id: "evidence", label: "Evidence" },
  ];

  let selectedRunLoopId = $state<string | null>(null);
  let activeTab = $state<InlineRunTab>("activity");
  let manualFocusId = $state<string | null>(null);
  let backBtnRef = $state<HTMLButtonElement | null>(null);
  let workbenchContentEl = $state<HTMLElement>();

  let resolvedRuns = $derived(
    linkedRuns.status === "loaded"
      ? linkedRuns.runs.filter(
          (run): run is ResolvedLinkedRun => Boolean(run.coordinator_loop_id),
        )
      : [],
  );

  let activeRun = $derived.by(() => {
    if (resolvedRuns.length === 0) return null;
    if (selectedRunLoopId) {
      const match = resolvedRuns.find((r) => r.coordinator_loop_id === selectedRunLoopId);
      if (match) return match;
    }
    return resolvedRuns[0];
  });

  let liveTask = $derived(
    activeRun
      ? (taskStore.tasks.find((t) => t.id === activeRun.coordinator_loop_id) ?? null)
      : null,
  );

  let focusableLoops = $derived(
    liveTask
      ? [...liveTask.childLoops, ...liveTask.controls.map((c) => c.loop)]
      : [],
  );

  let focusedLoopId = $derived.by(() => {
    if (!activeRun) return "";
    const primaryId = activeRun.coordinator_loop_id;
    if (!manualFocusId || manualFocusId === primaryId) return primaryId;
    return focusableLoops.some((c) => c.loop_id === manualFocusId)
      ? manualFocusId
      : primaryId;
  });

  let focusedChild = $derived(
    !activeRun || focusedLoopId === activeRun.coordinator_loop_id
      ? null
      : (focusableLoops.find((c) => c.loop_id === focusedLoopId) ?? null),
  );

  function selectInlineRun(loopId: string): void {
    selectedRunLoopId = loopId;
    manualFocusId = null;
  }

  function focusChild(loopId: string): void {
    manualFocusId = loopId;
  }

  async function focusPrimary(): Promise<void> {
    const returnTo = focusedLoopId;
    manualFocusId = null;
    await tick();
    const row = Array.from(
      workbenchContentEl?.querySelectorAll<HTMLElement>(
        '[data-testid="work-inline-child-item"], [data-testid="work-inline-control-row"]',
      ) ?? [],
    ).find((r) => r.dataset.loopId === returnTo);
    row?.focus();
  }

  async function focusFromStory(loopId: string): Promise<void> {
    focusChild(loopId);
    await tick();
    backBtnRef?.focus();
  }
</script>

<aside class="overview" data-testid="work-item-overview" data-item={item.ref} aria-labelledby="overview-heading">
  <header class="overview-header">
    <div>
      <h2 id="overview-heading" class="overview-ref" tabindex="-1" bind:this={heading}>{item.ref}</h2>
      <p class="overview-title">{item.title}</p>
    </div>
    <button type="button" class="close" data-testid="work-overview-close" onclick={onclose}>
      Close <span class="sr-only">overview of {item.ref}</span>
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

  <section class="coordination" aria-labelledby="coordination-heading" data-testid="overview-coordination">
    <h3 id="coordination-heading">Coordination &amp; baton</h3>
    <dl class="facts">
      <div class="fact">
        <dt>Builder baton</dt>
        <dd data-testid="overview-claim" data-state={item.claim.state}>
          {#if item.claim.state === "known"}
            {@const claim = item.claim.value}
            <span>
              {claim.holder}
              {#if claim.agent_vendor}
                <span class="source">({claim.agent_vendor} {claim.draft ? "draft PR" : "PR"})</span>
              {:else if claim.kind === "human-pr"}
                <span class="source">({claim.draft ? "draft PR" : "PR"})</span>
              {:else}
                <span class="source">(assigned)</span>
              {/if}
            </span>
            {#if claim.pull_request}
              <div class="source">
                PR: {claim.pull_request}{#if claim.branch} on <code>{claim.branch}</code>{/if}
              </div>
            {/if}
            {#if claim.stop_point}
              <div class="source" data-testid="overview-claim-stop-point">
                Stop-point: {claim.stop_point}
              </div>
            {/if}
            {#if item.claim.reason}
              <div class="source">&mdash; {item.claim.reason}</div>
            {/if}
          {:else if item.claim.state === "none"}
            unclaimed
          {:else}
            unknown
            {#if item.claim.reason}
              <span class="source">&mdash; {item.claim.reason}</span>
            {/if}
          {/if}
        </dd>
      </div>

      <div class="fact">
        <dt>Review baton</dt>
        <dd data-testid="overview-review" data-state={item.review.state}>
          {#if item.review.state === "known"}
            {@const rev = item.review.value}
            <span>
              {rev.status.replace(/_/g, " ")}
              <span class="source">({rev.pull_request})</span>
            </span>
            <div class="source" data-testid="overview-review-shas">
              {#if rev.head_sha}HEAD <code>{rev.head_sha}</code>{/if}
              {#if rev.request_sha}&middot; requested <code>{rev.request_sha}</code>{/if}
              {#if rev.record_sha}&middot; recorded <code>{rev.record_sha}</code>{/if}
            </div>
            {#if rev.reviewer_lane || rev.reviewed_by}
              <div class="source">
                {#if rev.reviewer_lane}Lane: {rev.reviewer_lane}{/if}
                {#if rev.reviewed_by}
                  {#if rev.reviewer_lane} &middot; {/if}Reviewed by: {rev.reviewed_by}
                {/if}
              </div>
            {/if}
          {:else if item.review.state === "none"}
            no active pull request review
          {:else}
            unknown
            {#if item.review.reason}
              <span class="source">&mdash; {item.review.reason}</span>
            {/if}
          {/if}
        </dd>
      </div>

      <div class="fact">
        <dt>Decision &amp; hold gates</dt>
        <dd data-testid="overview-gate" data-state={item.gate.state}>
          {#if item.gate.state === "known"}
            <ul>
              {#each item.gate.value as gate (`${gate.kind}-${gate.ref ?? gate.summary}`)}
                <li>
                  {gate.summary}
                  {#if gate.caveat_class}
                    <span class="source">[{gate.caveat_class}]</span>
                  {/if}
                </li>
              {/each}
            </ul>
            {#if item.gate.reason}
              <div class="source">&mdash; {item.gate.reason}</div>
            {/if}
          {:else if item.gate.state === "none"}
            no hold gate
          {:else}
            unknown
            {#if item.gate.reason}
              <span class="source">&mdash; {item.gate.reason}</span>
            {/if}
          {/if}
        </dd>
      </div>
    </dl>
  </section>

  <section class="runs" aria-labelledby="linked-runs-heading" data-testid="work-linked-runs" data-lookup={runsLookup}>
    <h3 id="linked-runs-heading">Linked runs</h3>

    {#if linkedRuns.status === "idle"}
      <p>Linked runs are read on request, not for the whole board.</p>
    {/if}

    <button
      type="button"
      class="load-runs-btn"
      data-testid="load-linked-runs"
      aria-busy={loading}
      aria-disabled={loading ? "true" : undefined}
      onclick={requestRuns}
    >
      {loadLabel}
    </button>

    <!-- Mounted from the start so each result is announced when it arrives. -->
    <div role="status" class="runs-status" data-testid="linked-runs-status">
      {#if linkedRuns.status === "loading"}
        <p>Loading linked runs&hellip;</p>
      {:else if linkedRuns.status === "loaded"}
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
          <p data-testid="linked-runs-summary">
            {linkedRuns.runs.length} linked {linkedRuns.runs.length === 1 ? "run" : "runs"} read.
          </p>
        {/if}
      {/if}
    </div>

    {#if linkedRuns.status === "error"}
      <p role="alert" data-testid="linked-runs-error">
        Linked runs could not be read (<code>{linkedRuns.code}</code>): {linkedRuns.message}
      </p>
    {:else if linkedRuns.status === "loaded" && linkedRuns.runs.length > 0}
      <ul class="run-list">
        {#each linkedRuns.runs as run, index (run.run_entity_id ?? `${run.coordinator_loop_id}-${index}`)}
          <li class="run-row" data-testid="linked-run-row" data-run={run.run_entity_id ?? undefined}>
            <span class="run-id" title={run.run_entity_id ?? undefined}>{runLabel(run.run_entity_id)}</span>
            <span class="run-facts">
              <span class="run-fact">
                <OverlayBadge
                  name="execution-stage"
                  label="Stage"
                  overlay={guardNone(run.execution_stage, run.lookup)}
                  noneLabel="not recorded"
                  showReason
                />
              </span>
              <span class="run-fact">
                <OverlayBadge
                  name="needs-you"
                  label="Needs you"
                  overlay={guardNone(run.needs_you, run.lookup)}
                  noneLabel="not recorded"
                  format={(value) => (value ? "yes" : "no")}
                  showReason
                />
              </span>
              <span class="run-fact">
                <OverlayBadge
                  name="verification"
                  label="Verification"
                  overlay={guardNone(run.verification, run.lookup)}
                  noneLabel="not recorded"
                  showReason
                />
              </span>
            </span>
            <LookupNotice lookup={run.lookup} reason={run.reason} subject="Run facts" />
            {#if run.coordinator_loop_id}
              <!-- The href starts from resolve("/"); the rule cannot see through the query string. -->
              <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
              <a class="drill-in" href={drillInHref(run.coordinator_loop_id)} data-testid="work-drill-in">
                Open this run in the runs lens <span class="sr-only">({runLabel(run.run_entity_id)})</span>
              </a>
            {:else}
              <span class="drill-in-unavailable" data-testid="work-drill-in-unavailable">
                Drill-in unavailable: the coordinator loop for this run could not be resolved.
              </span>
            {/if}
          </li>
        {/each}
      </ul>

      {#if activeRun}
        <div
          class="inline-workbench"
          data-testid="work-inline-run-workbench"
          data-loop-id={activeRun.coordinator_loop_id}
        >
          {#if resolvedRuns.length > 1}
            <div
              class="run-selector"
              role="group"
              aria-label="Inspect linked run"
              data-testid="work-inline-run-selector"
            >
              {#each resolvedRuns as run (run.coordinator_loop_id)}
                <button
                  type="button"
                  class="run-selector-btn"
                  class:active={activeRun.coordinator_loop_id === run.coordinator_loop_id}
                  data-testid="work-inline-run-option"
                  data-loop-id={run.coordinator_loop_id}
                  aria-pressed={activeRun.coordinator_loop_id === run.coordinator_loop_id}
                  onclick={() => selectInlineRun(run.coordinator_loop_id)}
                >
                  {runLabel(run.run_entity_id)}
                </button>
              {/each}
            </div>
          {/if}

          {#if liveTask?.state === "awaiting_approval" && liveTask.primaryLoop.pending_approval}
            <PendingApprovalSection
              loopId={liveTask.id}
              pendingApproval={liveTask.primaryLoop.pending_approval}
            />
          {/if}

          {#if liveTask?.runPause}
            <RunWaitingSection runId={liveTask.id} pause={liveTask.runPause} />
          {/if}

          {#if liveTask?.runHealth}
            <RunHealthPanel health={liveTask.runHealth} />
          {/if}

          <div
            class="workbench-tabs"
            role="tablist"
            aria-label="Run views"
            data-testid="work-inline-run-tabs"
          >
            {#each INLINE_RUN_TABS as tab (tab.id)}
              <button
                type="button"
                class="workbench-tab"
                class:active={activeTab === tab.id}
                role="tab"
                aria-selected={activeTab === tab.id}
                aria-controls="work-inline-panel-{tab.id}"
                data-testid="work-run-tab-{tab.id}"
                onclick={() => (activeTab = tab.id)}
              >
                {tab.label}
              </button>
            {/each}
          </div>

          <div class="workbench-content" bind:this={workbenchContentEl}>
            {#if activeTab === "activity"}
              <div
                id="work-inline-panel-activity"
                role="tabpanel"
                data-testid="work-inline-panel-activity"
              >
                {#if liveTask && liveTask.childLoops.length > 0}
                  <section class="workbench-section" data-testid="work-inline-children-section">
                    <h4 class="workbench-section-title">
                      Sub-tasks ({liveTask.childLoops.length})
                    </h4>
                    <ul class="child-list">
                      {#each liveTask.childLoops as child (child.loop_id)}
                        <li>
                          <button
                            type="button"
                            class="child-item"
                            class:focused={focusedLoopId === child.loop_id}
                            onclick={() => focusChild(child.loop_id)}
                            data-testid="work-inline-child-item"
                            data-loop-id={child.loop_id}
                            aria-pressed={focusedLoopId === child.loop_id}
                          >
                            <span class="child-state {child.state}">
                              {child.state.replace(/_/g, " ")}
                            </span>
                            <span class="child-role">{child.role}</span>
                            <span class="child-progress">
                              {child.iterations}/{child.max_iterations}
                            </span>
                          </button>
                        </li>
                      {/each}
                    </ul>
                  </section>
                {/if}

                {#if liveTask && liveTask.controls.length > 0}
                  <section class="workbench-section" data-testid="work-inline-controls-section">
                    <h4 class="workbench-section-title">
                      Controls ({liveTask.controls.length})
                    </h4>
                    {#if liveTask.controlsTruncated}
                      <p class="controls-note" data-testid="work-inline-controls-truncated">
                        Some rule-fired loops on this run could not be classified, so this list may be incomplete.
                      </p>
                    {/if}
                    <ul class="child-list">
                      {#each liveTask.controls as control (control.loopId)}
                        <li>
                          <button
                            type="button"
                            class="child-item"
                            class:focused={focusedLoopId === control.loopId}
                            onclick={() => focusChild(control.loopId)}
                            data-testid="work-inline-control-row"
                            data-loop-id={control.loopId}
                            aria-pressed={focusedLoopId === control.loopId}
                          >
                            <span class="child-state {control.loop.state}">
                              {control.loop.state.replace(/_/g, " ")}
                            </span>
                            <span class="child-role">{control.loop.role || "control"}</span>
                            <span class="child-progress">
                              {controlOutcome(control.loop.state)}
                            </span>
                          </button>
                        </li>
                      {/each}
                    </ul>
                  </section>
                {/if}

                {#if focusedChild}
                  <div class="focus-breadcrumb" data-testid="work-inline-focus-breadcrumb">
                    <button
                      type="button"
                      class="focus-back"
                      bind:this={backBtnRef}
                      onclick={focusPrimary}
                      data-testid="work-inline-focus-back"
                    >
                      &larr; Back to {liveTask?.title ?? runLabel(activeRun.run_entity_id)}
                    </button>
                    <span class="focus-current">
                      Viewing <span class="focus-current-role">{focusedChild.role}</span>
                    </span>
                  </div>
                {/if}

                <section class="workbench-section">
                  <TaskStory
                    loopId={focusedLoopId}
                    prompt={focusedChild ? undefined : liveTask?.primaryLoop.prompt}
                    controls={focusedChild ? [] : (liveTask?.controls ?? [])}
                    controlsTruncated={liveTask?.controlsTruncated ?? false}
                    onFocusLoop={focusFromStory}
                  />
                </section>
              </div>
            {:else if activeTab === "evidence"}
              <div
                id="work-inline-panel-evidence"
                role="tabpanel"
                data-testid="work-inline-panel-evidence"
              >
                <RunEvidencePanel
                  loopId={focusedLoopId}
                  runEntityId={activeRun.run_entity_id ?? liveTask?.runHealth?.runEntityId ?? null}
                />
              </div>
            {/if}
          </div>
        </div>
      {/if}
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

  .coordination,
  .runs {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    padding-top: 0.75rem;
    border-top: 1px solid var(--ui-border-subtle, #e5e7eb);
  }

  .coordination h3,
  .runs h3 {
    margin: 0;
    font-size: 0.9375rem;
  }

  .runs > p,
  .runs-status p {
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

  .run-facts {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  .run-fact {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 0.25rem 0.5rem;
  }

  .runs-status {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .runs-status:empty {
    display: none;
  }

  .drill-in-unavailable {
    font-size: 0.75rem;
    color: var(--ui-text-secondary, #6b7280);
  }

  .close,
  .load-runs-btn {
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

  .load-runs-btn[aria-disabled="true"] {
    cursor: progress;
    opacity: 0.7;
  }

  .inline-workbench {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    margin-top: 0.375rem;
    padding-top: 0.625rem;
    border-top: 1px solid var(--ui-border-subtle, #e5e7eb);
  }

  .run-selector {
    display: flex;
    flex-wrap: wrap;
    gap: 0.375rem;
  }

  .run-selector-btn {
    padding: 0.25rem 0.5rem;
    border: 1px solid var(--ui-border-subtle, #d1d5db);
    border-radius: 6px;
    background: var(--ui-surface-primary, #fff);
    font: inherit;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.75rem;
    color: var(--ui-text-secondary, #4b5563);
    cursor: pointer;
  }

  .run-selector-btn.active {
    border-color: var(--ui-interactive-primary, #3b82f6);
    background: color-mix(
      in srgb,
      var(--ui-interactive-primary, #3b82f6) 10%,
      var(--ui-surface-primary, #fff)
    );
    color: var(--ui-text-primary, #111827);
    font-weight: 600;
  }

  .workbench-tabs {
    display: flex;
    border-bottom: 1px solid var(--ui-border-subtle, #e5e7eb);
  }

  .workbench-tab {
    flex: 1;
    padding: 0.375rem 0.625rem;
    border: none;
    border-bottom: 2px solid transparent;
    background: transparent;
    font: inherit;
    font-size: 0.75rem;
    font-weight: 600;
    color: var(--ui-text-secondary, #6b7280);
    cursor: pointer;
    text-align: center;
  }

  .workbench-tab.active {
    color: var(--ui-text-primary, #111827);
    border-bottom-color: var(--ui-interactive-primary, #3b82f6);
  }

  .workbench-content {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .workbench-section {
    display: flex;
    flex-direction: column;
    gap: 0.375rem;
    padding: 0.375rem 0;
  }

  .workbench-section-title {
    margin: 0;
    font-size: 0.6875rem;
    font-weight: 700;
    letter-spacing: 0.05em;
    text-transform: uppercase;
    color: var(--ui-text-secondary, #6b7280);
  }

  .controls-note {
    margin: 0;
    font-size: 0.75rem;
    color: var(--ui-text-secondary, #6b7280);
  }

  .child-list {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .child-item {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    width: 100%;
    padding: 0.375rem 0.5rem;
    border: 1px solid var(--ui-border-subtle, #e5e7eb);
    border-radius: 6px;
    background: var(--ui-surface-primary, #fff);
    font: inherit;
    font-size: 0.75rem;
    text-align: left;
    cursor: pointer;
  }

  .child-item.focused {
    border-color: var(--ui-interactive-primary, #3b82f6);
    background: color-mix(
      in srgb,
      var(--ui-interactive-primary, #3b82f6) 8%,
      var(--ui-surface-primary, #fff)
    );
  }

  .child-state {
    font-size: 0.6875rem;
    font-weight: 600;
    text-transform: uppercase;
    color: var(--ui-text-secondary, #6b7280);
  }

  .child-role {
    font-weight: 600;
    color: var(--ui-text-primary, #111827);
  }

  .child-progress {
    margin-left: auto;
    font-size: 0.6875rem;
    color: var(--ui-text-secondary, #6b7280);
  }

  .focus-breadcrumb {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
    padding: 0.375rem 0.5rem;
    border: 1px solid var(--ui-border-subtle, #e5e7eb);
    border-radius: 6px;
    background: var(--ui-surface-primary, #fff);
    font-size: 0.75rem;
  }

  .focus-back {
    padding: 0.125rem 0.375rem;
    border: none;
    background: transparent;
    font: inherit;
    font-size: 0.75rem;
    font-weight: 600;
    color: var(--ui-interactive-primary, #2563eb);
    cursor: pointer;
  }

  .focus-current {
    color: var(--ui-text-secondary, #6b7280);
  }

  .focus-current-role {
    font-weight: 600;
    color: var(--ui-text-primary, #111827);
  }

  .overview-ref:focus-visible,
  .close:focus-visible,
  .load-runs-btn:focus-visible,
  .drill-in:focus-visible,
  .run-selector-btn:focus-visible,
  .workbench-tab:focus-visible,
  .child-item:focus-visible,
  .focus-back:focus-visible {
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
