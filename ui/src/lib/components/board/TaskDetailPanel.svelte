<script lang="ts">
  import type { TaskInfo } from "$lib/types/task";
  import type { ControlSignal } from "$lib/types/agent";
  import { isActiveState } from "$lib/types/agent";
  import { agentApi } from "$lib/services/agentApi";
  import TrajectoryViewer from "$lib/components/agents/TrajectoryViewer.svelte";
  import StateBadge from "./StateBadge.svelte";

  interface Props {
    task: TaskInfo;
    onClose?: () => void;
  }

  let { task, onClose }: Props = $props();

  let signalError = $state<string | null>(null);
  let closeBtnRef = $state<HTMLButtonElement | null>(null);

  // Focus the close button when the panel mounts so keyboard users know
  // the panel appeared. Without this, focus stays on the clicked card
  // and the panel is invisible to screen readers.
  $effect(() => {
    closeBtnRef?.focus();
  });

  async function handleSignal(signal: ControlSignal) {
    signalError = null;
    try {
      await agentApi.sendSignal(task.id, signal);
    } catch (err) {
      signalError = err instanceof Error ? err.message : "Signal failed";
    }
  }
</script>

<aside class="detail-panel" data-testid="task-detail-panel" aria-label="Task detail">
  <header class="panel-header">
    <div class="header-top">
      <StateBadge state={task.state} />
      <span class="role">{task.role}</span>
      <button
        class="close-btn"
        type="button"
        bind:this={closeBtnRef}
        onclick={onClose}
        aria-label="Close detail panel"
      >
        <span aria-hidden="true">×</span>
      </button>
    </div>
    <h2 class="task-title">{task.title}</h2>
    <div class="task-meta">
      <span class="meta-item">
        {task.iterations}/{task.maxIterations} iterations
      </span>
      <span class="meta-item">ID: {task.id.slice(0, 12)}</span>
    </div>

    {#if signalError}
      <div class="signal-error" role="alert">{signalError}</div>
    {/if}

    <div class="action-buttons">
      {#if isActiveState(task.state)}
        <button type="button" class="action-btn" onclick={() => handleSignal("pause")}>
          Pause
        </button>
        <button type="button" class="action-btn danger" onclick={() => handleSignal("cancel")}>
          Cancel
        </button>
      {:else if task.state === "paused"}
        <button type="button" class="action-btn" onclick={() => handleSignal("resume")}>
          Resume
        </button>
        <button type="button" class="action-btn danger" onclick={() => handleSignal("cancel")}>
          Cancel
        </button>
      {:else if task.state === "awaiting_approval"}
        <button type="button" class="action-btn approve" onclick={() => handleSignal("approve")}>
          Approve
        </button>
        <button type="button" class="action-btn danger" onclick={() => handleSignal("reject")}>
          Reject
        </button>
      {/if}
    </div>
  </header>

  <div class="panel-content">
    {#if task.childLoops.length > 0}
      <section class="panel-section">
        <h3 class="section-title">Child Loops ({task.childLoops.length})</h3>
        <ul class="child-list">
          {#each task.childLoops as child (child.loop_id)}
            <li class="child-item">
              <span class="child-state {child.state}">
                {child.state.replace(/_/g, " ")}
              </span>
              <span class="child-role">{child.role}</span>
              <span class="child-progress">
                {child.iterations}/{child.max_iterations}
              </span>
            </li>
          {/each}
        </ul>
      </section>
    {/if}

    <section class="panel-section trajectory-section">
      <h3 class="section-title">Trajectory</h3>
      <TrajectoryViewer loopId={task.id} />
    </section>
  </div>
</aside>

<style>
  .detail-panel {
    width: 320px;
    min-width: 280px;
    border-left: 1px solid var(--ui-border-subtle, #e5e7eb);
    background: var(--ui-surface-primary, #fff);
    display: flex;
    flex-direction: column;
    overflow: hidden;
    flex-shrink: 0;
  }

  .panel-header {
    padding: 0.75rem;
    border-bottom: 1px solid var(--ui-border-subtle, #e5e7eb);
    flex-shrink: 0;
  }

  .header-top {
    display: flex;
    align-items: center;
    gap: 0.375rem;
    margin-bottom: 0.375rem;
  }

  .role {
    font-size: 0.75rem;
    color: var(--ui-text-secondary, #6b7280);
  }

  .close-btn {
    all: unset;
    margin-left: auto;
    cursor: pointer;
    font-size: 1.125rem;
    color: var(--ui-text-secondary, #9ca3af);
    padding: 0.125rem 0.25rem;
    border-radius: 4px;
    line-height: 1;
  }

  .close-btn:hover {
    color: var(--ui-text-primary, #374151);
    background: var(--ui-surface-secondary, #f3f4f6);
  }

  .close-btn:focus-visible {
    outline: 2px solid var(--ui-interactive-primary, #3b82f6);
    outline-offset: 2px;
  }

  .task-title {
    margin: 0;
    font-size: 0.9375rem;
    font-weight: 600;
    color: var(--ui-text-primary, #111827);
    line-height: 1.3;
  }

  .task-meta {
    display: flex;
    gap: 0.75rem;
    margin-top: 0.25rem;
    font-size: 0.6875rem;
    color: var(--ui-text-secondary, #9ca3af);
  }

  .meta-item {
    font-variant-numeric: tabular-nums;
  }

  .signal-error {
    margin-top: 0.375rem;
    padding: 0.25rem 0.5rem;
    background: #fef2f2;
    border: 1px solid #fecaca;
    border-radius: 4px;
    font-size: 0.75rem;
    color: #991b1b;
  }

  .action-buttons {
    display: flex;
    gap: 0.375rem;
    margin-top: 0.5rem;
  }

  .action-btn {
    padding: 0.25rem 0.625rem;
    border: 1px solid var(--ui-border-subtle, #d1d5db);
    border-radius: 4px;
    font-size: 0.75rem;
    font-weight: 500;
    cursor: pointer;
    background: var(--ui-surface-primary, #fff);
    color: var(--ui-text-primary, #374151);
    transition: all 0.15s;
  }

  .action-btn:hover {
    background: var(--ui-surface-secondary, #f3f4f6);
  }

  .action-btn.approve {
    background: #d1fae5;
    border-color: #6ee7b7;
    color: #065f46;
  }

  .action-btn.approve:hover {
    background: #a7f3d0;
  }

  .action-btn.danger {
    color: #991b1b;
    border-color: #fca5a5;
  }

  .action-btn.danger:hover {
    background: #fee2e2;
  }

  .panel-content {
    flex: 1;
    overflow-y: auto;
    padding: 0.75rem;
  }

  .panel-section {
    margin-bottom: 1rem;
  }

  .section-title {
    margin: 0 0 0.5rem;
    font-size: 0.6875rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--ui-text-secondary, #6b7280);
  }

  .child-list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  .child-item {
    display: flex;
    align-items: center;
    gap: 0.375rem;
    padding: 0.25rem 0.5rem;
    border-radius: 4px;
    background: var(--ui-surface-secondary, #f9fafb);
    font-size: 0.75rem;
  }

  .child-state {
    padding: 0 0.25rem;
    border-radius: 3px;
    font-size: 0.625rem;
    font-weight: 600;
    text-transform: capitalize;
  }

  .child-state.awaiting_approval {
    background: #ffedd5;
    color: #9a3412;
  }

  .child-state.executing {
    background: #ccfbf1;
    color: #0f766e;
  }

  .child-state.complete {
    background: #d1fae5;
    color: #065f46;
  }

  .child-state.failed {
    background: #fee2e2;
    color: #991b1b;
  }

  .child-role {
    color: var(--ui-text-primary, #374151);
    font-weight: 500;
  }

  .child-progress {
    margin-left: auto;
    font-variant-numeric: tabular-nums;
    color: var(--ui-text-secondary, #9ca3af);
  }

  .trajectory-section {
    min-height: 200px;
  }
</style>
