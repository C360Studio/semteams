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
    min-width: 260px;
    max-width: 340px;
    flex: 1 1 280px;
    /* Container surface so columns read as columns. Subtle elevation
       above the page background; cards sit on top with their own
       slightly-brighter surface. */
    background: var(--ui-surface-secondary, #f3f4f6);
    border: 1px solid var(--ui-border-subtle, #e5e7eb);
    border-radius: 10px;
    overflow: hidden;
  }

  .column-header {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.625rem 0.875rem;
    border-bottom: 1px solid var(--ui-border-subtle, #e5e7eb);
    background: var(--ui-surface-tertiary, transparent);
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
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--ui-text-primary, #111827);
    flex: 1;
  }

  .column-count {
    font-size: 0.6875rem;
    font-weight: 700;
    font-variant-numeric: tabular-nums;
    color: var(--ui-text-secondary, #6b7280);
    background: var(--ui-surface-primary, #fff);
    border: 1px solid var(--ui-border-subtle, #e5e7eb);
    padding: 0.0625rem 0.5rem;
    border-radius: 9999px;
    min-width: 1.5rem;
    text-align: center;
  }

  .column-cards {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    padding: 0.625rem;
    overflow-y: auto;
    flex: 1;
  }

  .column-empty {
    text-align: center;
    padding: 2rem 0.5rem;
    font-size: 0.75rem;
    color: var(--ui-text-secondary, #9ca3af);
    margin: 0;
    font-style: italic;
  }
</style>
