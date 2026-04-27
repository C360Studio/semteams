<script lang="ts">
  import { SvelteSet } from "svelte/reactivity";
  import { taskStore } from "$lib/stores/taskStore.svelte";
  import { COLUMNS, type TaskColumn } from "$lib/types/task";
  import KanbanColumn from "./KanbanColumn.svelte";

  /** Column visibility, persisted to localStorage. */
  let visibleColumns = $state<SvelteSet<TaskColumn>>(loadVisibility());

  function loadVisibility(): SvelteSet<TaskColumn> {
    try {
      const saved = localStorage.getItem("semteams:board:columns");
      if (saved) return new SvelteSet(JSON.parse(saved) as TaskColumn[]);
    } catch {
      // Ignore — use defaults
    }
    return new SvelteSet(COLUMNS.filter((c) => c.defaultVisible).map((c) => c.id));
  }

  function saveVisibility() {
    try {
      localStorage.setItem(
        "semteams:board:columns",
        JSON.stringify([...visibleColumns]),
      );
    } catch {
      // Ignore storage errors
    }
  }

  function toggleColumn(id: TaskColumn) {
    if (visibleColumns.has(id)) {
      visibleColumns.delete(id);
    } else {
      visibleColumns.add(id);
    }
    saveVisibility();
  }

  function handleTaskClick(id: string) {
    taskStore.toggleTask(id);
  }
</script>

<div class="kanban-board" data-testid="kanban-board">
  <div class="column-toggles" data-testid="column-toggles">
    {#each COLUMNS as col (col.id)}
      <button
        class="toggle-chip"
        class:active={visibleColumns.has(col.id)}
        data-testid="toggle-{col.id}"
        onclick={() => toggleColumn(col.id)}
        type="button"
        aria-pressed={visibleColumns.has(col.id)}
      >
        <span class="chip-dot" style="background: {col.color}"></span>
        {col.label}
        <span class="chip-count">{taskStore.columnCounts[col.id]}</span>
      </button>
    {/each}
  </div>

  <div class="columns-container">
    {#each COLUMNS.filter((c) => visibleColumns.has(c.id)) as col (col.id)}
      <KanbanColumn
        column={col}
        tasks={taskStore.getColumn(col.id)}
        selectedTaskId={taskStore.selectedId}
        onTaskClick={handleTaskClick}
      />
    {/each}
  </div>
</div>

<style>
  .kanban-board {
    display: flex;
    flex-direction: column;
    height: 100%;
    overflow: hidden;
  }

  .column-toggles {
    display: flex;
    gap: 0.25rem;
    padding: 0 0.5rem 0.75rem;
    flex-shrink: 0;
    flex-wrap: wrap;
  }

  .toggle-chip {
    all: unset;
    display: inline-flex;
    align-items: center;
    gap: 0.25rem;
    padding: 0.25rem 0.5rem;
    border-radius: 9999px;
    font-size: 0.75rem;
    font-weight: 500;
    color: var(--ui-text-secondary, #9ca3af);
    cursor: pointer;
    transition: all 0.15s;
    border: 1px solid transparent;
  }

  .toggle-chip:hover {
    color: var(--ui-text-primary, #374151);
    background: var(--ui-surface-secondary, #f9fafb);
  }

  .toggle-chip.active {
    color: var(--ui-text-primary, #374151);
    background: var(--ui-surface-secondary, #f3f4f6);
    border-color: var(--ui-border-subtle, #e5e7eb);
  }

  .toggle-chip:focus-visible {
    outline: 2px solid var(--ui-interactive-primary, #3b82f6);
    outline-offset: 2px;
  }

  .chip-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    flex-shrink: 0;
  }

  .chip-count {
    font-variant-numeric: tabular-nums;
    opacity: 0.7;
  }

  .columns-container {
    display: flex;
    gap: 1rem;
    padding: 0.25rem 1rem 1rem;
    flex: 1;
    overflow-x: auto;
    overflow-y: hidden;
    align-items: stretch;
  }
</style>
