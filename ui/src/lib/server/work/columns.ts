// Board column mapping (design D2). Columns come from GitHub fields only and
// are never derived from execution state: a linked run never moves a card.

import type { Repository, WorkItemState } from "$lib/types/work";

export const FALLBACK_COLUMNS = ["Todo", "In Progress", "Done"] as const;

/**
 * Column for an item whose repository has a project board but whose status
 * field is empty. GitHub renders the same bucket for an empty Status.
 */
export const NO_STATUS_COLUMN = "No Status";

interface ColumnInput {
  state: WorkItemState;
  assignees: readonly string[];
  pm_status?: string;
}

export function columnFor(
  item: ColumnInput,
  repository: Repository,
): string {
  if (repository.project_board) {
    return item.pm_status?.trim() || NO_STATUS_COLUMN;
  }
  if (item.state === "closed") return "Done";
  return item.assignees.length > 0 ? "In Progress" : "Todo";
}

/**
 * Ordered columns for a repository. With a project board the order is the
 * board's status options; any column an item landed in that the options do not
 * name (e.g. "No Status") is appended so no item is orphaned.
 */
export function columnsFor(
  repository: Repository,
  statusOptions: readonly string[],
  itemColumns: readonly string[],
): string[] {
  if (!repository.project_board) return [...FALLBACK_COLUMNS];
  const columns = [...statusOptions];
  for (const column of itemColumns) {
    if (!columns.includes(column)) columns.push(column);
  }
  return columns;
}
