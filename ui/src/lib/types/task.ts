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
    case "success":
    case "truncated":
      return "done";
    case "failed":
    case "error":
    case "cancelled":
      return "failed";
    default: {
      // Cross-repo type drift is a runtime fact (backend ships separately).
      // Log the unknown state so we notice, but don't crash the reactive
      // graph — that poisons Svelte's batched flush and breaks unrelated
      // bindings (chat input was the load-bearing victim).
      console.warn("[loopStateToColumn] unknown state, defaulting to thinking", state);
      return "thinking";
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
   * GitHub-style short ref ("#42"). Per-deployment monotonic counter
   * minted by taskRefs and persisted in localStorage. Null only if a
   * ref hasn't been assigned yet (e.g. layout effect hasn't fired).
   */
  shortRef: number | null;

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
 * Maximum length of an auto-generated task title in characters. The
 * card UI truncates with ellipsis; this just bounds the value we store.
 * Matches the rough length of a Claude Desktop chat title.
 */
const TITLE_MAX_LEN = 80;

/**
 * Derive a human-readable title from a loop's prompt. Strips leading
 * slash commands (e.g., "/research foo bar" → "foo bar") since the
 * command is captured in role/state. Collapses whitespace, hard-cuts
 * at TITLE_MAX_LEN with an ellipsis when the original is longer.
 */
function titleFromPrompt(prompt: string): string {
  let s = prompt.trim();
  // Drop a leading slash command token if present.
  if (s.startsWith("/")) {
    const rest = s.split(/\s+/).slice(1).join(" ");
    if (rest.length > 0) s = rest;
  }
  s = s.replace(/\s+/g, " ");
  if (s.length <= TITLE_MAX_LEN) return s;
  return s.slice(0, TITLE_MAX_LEN - 1).trimEnd() + "…";
}

/**
 * Derive a TaskInfo from a top-level loop and its children.
 * The task's column is the most-urgent state across itself and all
 * children (needs_you > failed > executing > thinking > done).
 *
 * `shortRef` is passed in rather than read from a store so this stays
 * a pure function — the caller (taskStore) reads taskRefs.get() and
 * passes the ref through.
 */
export function deriveTaskInfo(
  primaryLoop: AgentLoop,
  childLoops: AgentLoop[],
  shortRef: number | null = null,
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

  // Title preference: the user's prompt (most informative) → task_id
  // (UUID, ugly but stable) → short loop_id (always present). prompt
  // arrives via the dispatch's COMPLETE_<id> envelope merge in
  // agentStore, so freshly-created tasks may briefly show task_id
  // before the prompt lands.
  const title =
    (primaryLoop.prompt && titleFromPrompt(primaryLoop.prompt)) ||
    primaryLoop.task_id ||
    primaryLoop.loop_id.slice(0, 12);

  return {
    id: primaryLoop.loop_id,
    shortRef,
    title,
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

/**
 * Resolve an "@42" / "#42" / partial-title input to a TaskInfo.
 * Numeric tokens look up by ref via taskRefs (caller passes the
 * lookup function so this stays pure). Otherwise case-insensitive
 * substring match across titles. Aliases land in step 7 of the redesign.
 */
export function resolveTaskMention(
  input: string,
  tasks: TaskInfo[],
  loopByRef: (ref: number) => string | null,
): TaskInfo | null {
  const trimmed = input.replace(/^[@#]/, "").trim();
  if (!trimmed) return null;

  // Numeric → exact ref match
  if (/^\d+$/.test(trimmed)) {
    const ref = Number(trimmed);
    const loopId = loopByRef(ref);
    if (loopId) {
      const match = tasks.find((t) => t.id === loopId);
      if (match) return match;
    }
  }

  // Title fuzzy (case-insensitive substring). First match wins.
  const lower = trimmed.toLowerCase();
  return tasks.find((t) => t.title.toLowerCase().includes(lower)) ?? null;
}
