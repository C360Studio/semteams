// Task store — derives tasks from agentStore loops and groups them into
// kanban columns. Reactive via Svelte 5 $derived — re-computes when the
// agentStore's SvelteMap changes.

import { agentStore } from "./agentStore.svelte";
import {
  type TaskInfo,
  type TaskColumn,
  COLUMNS,
  deriveTaskInfo,
} from "$lib/types/task";

function createTaskStore() {
  // Selected task for the context panel.
  let selectedId = $state<string | null>(null);

  // Derive tasks from agentStore. Top-level loops (no parent_loop_id)
  // become tasks; their children are grouped under them.
  let tasks = $derived.by(() => {
    const allLoops = agentStore.loopsList;

    // Separate top-level loops from children. Uses a plain object (not
    // Map) to avoid the svelte/prefer-svelte-reactivity lint rule — this
    // is a local grouping variable, not reactive state.
    // Top-level loops have an empty parent_loop_id (Go zero-value "").
    // Filter explicitly on empty string rather than falsy to match the
    // backend contract (parent_loop_id is always a string, never null).
    const topLevel = allLoops.filter((l) => l.parent_loop_id === "");
    const childrenByParent: Record<string, typeof allLoops> = {};

    for (const loop of allLoops) {
      if (loop.parent_loop_id !== "") {
        (childrenByParent[loop.parent_loop_id] ??= []).push(loop);
      }
    }

    return topLevel.map((loop) =>
      deriveTaskInfo(loop, childrenByParent[loop.loop_id] ?? []),
    );
  });

  // Derive column-grouped tasks.
  let byColumn = $derived.by(() => {
    const grouped: Record<TaskColumn, TaskInfo[]> = {
      thinking: [],
      executing: [],
      needs_you: [],
      done: [],
      failed: [],
    };
    for (const task of tasks) {
      grouped[task.column].push(task);
    }
    return grouped;
  });

  // Column counts for the toggle chips.
  let columnCounts = $derived.by(() => {
    const counts: Record<TaskColumn, number> = {
      thinking: 0,
      executing: 0,
      needs_you: 0,
      done: 0,
      failed: 0,
    };
    for (const task of tasks) {
      counts[task.column]++;
    }
    return counts;
  });

  return {
    /** All tasks (unfiltered). */
    get tasks() {
      return tasks;
    },

    /** Tasks grouped by kanban column. */
    get byColumn() {
      return byColumn;
    },

    /** Per-column task counts. */
    get columnCounts() {
      return columnCounts;
    },

    /** Column definitions (static metadata). */
    get columns() {
      return COLUMNS;
    },

    /** The currently selected task (for the context panel). */
    get selectedTask(): TaskInfo | undefined {
      if (!selectedId) return undefined;
      return tasks.find((t) => t.id === selectedId);
    },

    /** The selected task ID (or null). */
    get selectedId() {
      return selectedId;
    },

    /** Select a task (opens the context panel). */
    selectTask(id: string) {
      selectedId = id;
    },

    /** Deselect (closes the context panel). */
    deselectTask() {
      selectedId = null;
    },

    /** Toggle selection — click same card again to deselect. */
    toggleTask(id: string) {
      selectedId = selectedId === id ? null : id;
    },

    /** Get tasks for a specific column. */
    getColumn(column: TaskColumn): TaskInfo[] {
      return byColumn[column];
    },

    /** Total count of tasks needing user attention. */
    get needsAttentionCount(): number {
      return columnCounts.needs_you;
    },
  };
}

export const taskStore = createTaskStore();
