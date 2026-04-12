// Task types for the kanban board.
//
// Phase 1: a "Task" is a top-level agent loop (parent_loop_id is empty).
// Phase 2 (future): a Task becomes its own backend entity wrapping loops.
// The TaskInfo interface abstracts over both — board components consume
// TaskInfo, not AgentLoop directly.

import type { AgentLoop, AgentLoopState } from "./agent";

/** Kanban column identifiers. Each maps to one or more AgentLoopStates. */
export type TaskColumn =
  | "thinking"
  | "executing"
  | "needs_you"
  | "done"
  | "failed";

/** Column metadata for rendering. */
export interface ColumnDef {
  id: TaskColumn;
  label: string;
  color: string;
  /** Whether the column is visible by default. */
  defaultVisible: boolean;
}

/** All columns in display order. */
export const COLUMNS: ColumnDef[] = [
  { id: "thinking", label: "Thinking", color: "var(--col-thinking, #3b82f6)", defaultVisible: true },
  { id: "executing", label: "Executing", color: "var(--col-executing, #14b8a6)", defaultVisible: true },
  { id: "needs_you", label: "Needs You", color: "var(--col-needs-you, #f97316)", defaultVisible: true },
  { id: "done", label: "Done", color: "var(--col-done, #22c55e)", defaultVisible: true },
  { id: "failed", label: "Failed", color: "var(--col-failed, #ef4444)", defaultVisible: false },
];

/** Map an agent loop state to a kanban column. */
export function loopStateToColumn(state: AgentLoopState): TaskColumn {
  switch (state) {
    case "exploring":
    case "planning":
    case "architecting":
      return "thinking";
    case "executing":
    case "reviewing":
      return "executing";
    case "awaiting_approval":
    case "paused":
      return "needs_you";
    case "complete":
      return "done";
    case "failed":
    case "cancelled":
      return "failed";
    default: {
      // Exhaustiveness guard — if a new AgentLoopState is added and
      // not mapped here, TypeScript will error on `_exhaustive`.
      const _exhaustive: never = state;
      return _exhaustive;
    }
  }
}

/**
 * TaskInfo is the data model for a single kanban card.
 *
 * In Phase 1, this is derived from the top-level AgentLoop + any child
 * loops. In Phase 2, it will be backed by a proper Task backend entity.
 */
export interface TaskInfo {
  /** Unique identifier (loop_id of the top-level loop in Phase 1). */
  id: string;

  /**
   * Human-readable title for the card. Phase 1: falls back to
   * `task_id` since the loop entity doesn't carry the user's prompt.
   * Phase 2: will be the first line of the user's message.
   */
  title: string;

  /** The kanban column this task belongs to. */
  column: TaskColumn;

  /** The raw loop state of the primary loop. */
  state: AgentLoopState;

  /** Agent role (general, editor, researcher, etc.). */
  role: string;

  /** Iteration progress of the primary loop. */
  iterations: number;
  maxIterations: number;

  /** The primary (top-level) loop. */
  primaryLoop: AgentLoop;

  /** Child loops (parent_loop_id === this.id). */
  childLoops: AgentLoop[];

  /** True if any child loop is in a state that needs user attention. */
  childNeedsAttention: boolean;

  /** Count of child loops needing attention. */
  childAttentionCount: number;
}

/** Column priority for "most urgent wins" aggregation over child loops. */
const COLUMN_URGENCY: Record<TaskColumn, number> = {
  needs_you: 4,
  failed: 3,
  executing: 2,
  thinking: 1,
  done: 0,
};

/**
 * Derive a TaskInfo from a top-level loop and its children.
 * The task's column is the most-urgent state across itself and all
 * children (needs_you > failed > executing > thinking > done).
 */
export function deriveTaskInfo(
  primaryLoop: AgentLoop,
  childLoops: AgentLoop[],
): TaskInfo {
  const primaryColumn = loopStateToColumn(primaryLoop.state);

  // Find the most-urgent column across all loops in this task.
  let effectiveColumn = primaryColumn;
  for (const child of childLoops) {
    const childCol = loopStateToColumn(child.state);
    if (COLUMN_URGENCY[childCol] > COLUMN_URGENCY[effectiveColumn]) {
      effectiveColumn = childCol;
    }
  }

  const attentionChildren = childLoops.filter(
    (c) => loopStateToColumn(c.state) === "needs_you",
  );

  return {
    id: primaryLoop.loop_id,
    title: primaryLoop.task_id || primaryLoop.loop_id.slice(0, 12),
    column: effectiveColumn,
    state: primaryLoop.state,
    role: primaryLoop.role,
    iterations: primaryLoop.iterations,
    maxIterations: primaryLoop.max_iterations,
    primaryLoop,
    childLoops,
    childNeedsAttention: attentionChildren.length > 0,
    childAttentionCount: attentionChildren.length,
  };
}
