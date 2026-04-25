// Task store — derives tasks from agentStore loops and groups them into
// kanban columns. Reactive via Svelte 5 $derived — re-computes when the
// agentStore's SvelteMap changes.
//
// Selection lives in the URL (?task=<loop_id>) rather than as local state,
// so refresh, share, and the back button preserve which task the chat is
// targeting. URL writes use SvelteKit's shallow `replaceState` to avoid
// re-running +page.ts load functions and to keep the back button focused
// on cross-page navigation rather than cycling through selections.

import { page } from "$app/state";
import { replaceState } from "$app/navigation";
import { agentStore } from "./agentStore.svelte";
import {
  type TaskInfo,
  type TaskColumn,
  COLUMNS,
  deriveTaskInfo,
} from "$lib/types/task";

const SELECTION_PARAM = "task";

function createTaskStore() {
  // Read selection from the URL — reactive via $app/state's `page` rune.
  const selectedId = $derived<string | null>(
    page.url?.searchParams.get(SELECTION_PARAM) ?? null,
  );

  function setSelection(id: string | null) {
    // page.url is a getter on $app/state; clone before mutating.
    if (!page.url) return;
    const url = new URL(page.url);
    if (id === null) url.searchParams.delete(SELECTION_PARAM);
    else url.searchParams.set(SELECTION_PARAM, id);
    // We're mutating search params on the current page, not navigating
    // to a different route. resolve() is typed against route literals
    // and can't express "same path, different search".
    // eslint-disable-next-line svelte/no-navigation-without-resolve
    replaceState(url, page.state);
  }

  // Derive tasks from agentStore. Top-level loops (no parent_loop_id)
  // become tasks; their children are grouped under them.
  let tasks = $derived.by(() => {
    const allLoops = agentStore.loopsList;

    // Separate top-level loops from children. Uses a plain object (not
    // Map) to avoid the svelte/prefer-svelte-reactivity lint rule — this
    // is a local grouping variable, not reactive state.
    // Top-level loops have an empty or absent parent_loop_id. The Go
    // struct uses `omitempty`, so the field is omitted from JSON when
    // empty — treat both "" and undefined/missing as top-level.
    const topLevel = allLoops.filter((l) => !l.parent_loop_id);
    const childrenByParent: Record<string, typeof allLoops> = {};

    for (const loop of allLoops) {
      if (loop.parent_loop_id) {
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
      // Defensive: a future unmapped column would otherwise throw and
      // poison the entire reactive batch (breaks bind:value updates on
      // ChatBar etc.). Keep the column-aware Map happy by silently
      // bucketing strangers into "thinking".
      const bucket = grouped[task.column] ?? grouped.thinking;
      bucket.push(task);
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
      setSelection(id);
    },

    /** Deselect (closes the context panel). */
    deselectTask() {
      setSelection(null);
    },

    /** Toggle selection — click same card again to deselect. */
    toggleTask(id: string) {
      setSelection(selectedId === id ? null : id);
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
