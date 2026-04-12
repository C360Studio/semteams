<script lang="ts">
  import type { TaskInfo, ColumnDef } from "$lib/types/task";
  import TaskCard from "./TaskCard.svelte";

  interface Props {
    column: ColumnDef;
    tasks: TaskInfo[];
    selectedTaskId?: string | null;
    onTaskClick?: (id: string) => void;
  }

  let { column, tasks, selectedTaskId = null, onTaskClick }: Props = $props();
</script>

<section
  class="kanban-column"
  data-testid="kanban-column"
  data-column-id={column.id}
  aria-label="{column.label} column, {tasks.length} tasks"
>
  <header class="column-header">
    <span class="column-dot" style="background: {column.color}"></span>
    <h2 class="column-label">{column.label}</h2>
    <span class="column-count" data-testid="column-count">{tasks.length}</span>
  </header>

  <div class="column-cards" role="list">
    {#each tasks as task (task.id)}
      <div role="listitem">
        <TaskCard
          {task}
          selected={selectedTaskId === task.id}
          onclick={onTaskClick}
        />
      </div>
    {/each}

    {#if tasks.length === 0}
      <p class="column-empty">No tasks</p>
    {/if}
  </div>
</section>

<style>
  .kanban-column {
    display: flex;
    flex-direction: column;
    min-width: 220px;
    max-width: 320px;
    flex: 1;
    overflow: hidden;
  }

  .column-header {
    display: flex;
    align-items: center;
    gap: 0.375rem;
    padding: 0 0.5rem 0.5rem;
    flex-shrink: 0;
  }

  .column-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    flex-shrink: 0;
  }

  .column-label {
    margin: 0;
    font-size: 0.75rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--ui-text-secondary, #6b7280);
  }

  .column-count {
    font-size: 0.6875rem;
    font-weight: 600;
    font-variant-numeric: tabular-nums;
    color: var(--ui-text-secondary, #6b7280);
    background: var(--ui-surface-secondary, #f3f4f6);
    padding: 0 0.375rem;
    border-radius: 9999px;
    min-width: 1.25rem;
    text-align: center;
  }

  .column-cards {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    padding: 0 0.25rem;
    overflow-y: auto;
    flex: 1;
  }

  .column-empty {
    text-align: center;
    padding: 1.5rem 0.5rem;
    font-size: 0.75rem;
    color: var(--ui-text-secondary, #9ca3af);
    margin: 0;
  }
</style>
