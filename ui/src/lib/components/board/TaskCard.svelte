<script lang="ts">
  import type { TaskInfo } from "$lib/types/task";
  import StateBadge from "./StateBadge.svelte";

  interface Props {
    task: TaskInfo;
    selected?: boolean;
    onclick?: (id: string) => void;
  }

  let { task, selected = false, onclick }: Props = $props();

  function formatElapsed(iterations: number, max: number): string {
    if (max <= 0) return `${iterations} iters`;
    return `${iterations}/${max}`;
  }

  function progressPercent(iterations: number, max: number): number {
    if (max <= 0) return 0;
    return Math.min(100, (iterations / max) * 100);
  }
</script>

<button
  class="task-card"
  class:selected
  data-testid="task-card"
  data-task-id={task.id}
  data-column={task.column}
  onclick={() => onclick?.(task.id)}
  type="button"
>
  <div class="card-header">
    <StateBadge state={task.state} />
    <span class="role-badge">{task.role}</span>
    {#if task.shortRef !== null}
      <span class="card-ref" data-testid="card-ref">#{task.shortRef}</span>
    {/if}
  </div>

  <p class="card-title">{task.title}</p>

  {#if task.maxIterations > 0}
    <div class="card-progress">
      <div class="progress-bar">
        <div
          class="progress-fill"
          style="width: {progressPercent(task.iterations, task.maxIterations)}%"
        ></div>
      </div>
      <span class="progress-text">{formatElapsed(task.iterations, task.maxIterations)}</span>
    </div>
  {/if}

  {#if task.childNeedsAttention}
    <div class="child-attention" data-testid="child-attention">
      {task.childAttentionCount} child{task.childAttentionCount === 1 ? '' : 'ren'} awaiting approval
    </div>
  {/if}
</button>

<style>
  .task-card {
    all: unset;
    display: flex;
    flex-direction: column;
    gap: 0.375rem;
    padding: 0.625rem 0.75rem;
    border: 1px solid var(--ui-border-subtle, #e5e7eb);
    border-radius: 6px;
    background: var(--ui-surface-primary, #fff);
    cursor: pointer;
    transition: border-color 0.15s, box-shadow 0.15s;
    width: 100%;
    box-sizing: border-box;
    text-align: left;
  }

  .task-card:hover {
    border-color: var(--ui-border-emphasis, #d1d5db);
    box-shadow: 0 1px 3px rgba(0, 0, 0, 0.06);
  }

  .task-card.selected {
    border-color: var(--ui-interactive-primary, #3b82f6);
    box-shadow: 0 0 0 1px var(--ui-interactive-primary, #3b82f6);
  }

  .task-card:focus-visible {
    outline: 2px solid var(--ui-interactive-primary, #3b82f6);
    outline-offset: 2px;
  }

  .card-header {
    display: flex;
    align-items: center;
    gap: 0.375rem;
  }

  .role-badge {
    font-size: 0.6875rem;
    color: var(--ui-text-secondary, #6b7280);
    font-weight: 500;
  }

  .card-ref {
    margin-left: auto;
    font-size: 0.6875rem;
    font-weight: 600;
    font-variant-numeric: tabular-nums;
    color: var(--ui-text-tertiary, #9ca3af);
    letter-spacing: 0.01em;
  }

  .card-title {
    margin: 0;
    font-size: 0.8125rem;
    font-weight: 500;
    color: var(--ui-text-primary, #111827);
    line-height: 1.4;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .card-progress {
    display: flex;
    align-items: center;
    gap: 0.375rem;
  }

  .progress-bar {
    flex: 1;
    height: 4px;
    background: var(--ui-border-subtle, #e5e7eb);
    border-radius: 2px;
    overflow: hidden;
  }

  .progress-fill {
    height: 100%;
    background: var(--ui-interactive-primary, #3b82f6);
    border-radius: 2px;
    transition: width 0.3s ease;
  }

  .progress-text {
    font-size: 0.6875rem;
    font-variant-numeric: tabular-nums;
    color: var(--ui-text-secondary, #6b7280);
    white-space: nowrap;
  }

  .child-attention {
    font-size: 0.6875rem;
    font-weight: 600;
    color: var(--col-needs-you, #f97316);
    display: flex;
    align-items: center;
    gap: 0.25rem;
  }

  .child-attention::before {
    content: '⚠';
    font-size: 0.75rem;
  }
</style>
