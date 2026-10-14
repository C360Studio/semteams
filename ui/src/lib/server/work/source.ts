// The work source boundary (design D1). The browser contract is source
// independent: fixtures serve it today, the live GitHub adapter (#273) serves
// it later, and neither changes the endpoints.

import type {
  LookupStatus,
  Repository,
  WorkItem,
} from "$lib/types/work";

export interface ItemRef {
  repository: Repository;
  number: number;
}

/**
 * How a work item names its runs:
 * - `run_entity_id` / `coordinator_prompt`: explicit fixture bindings (design D5)
 * - `work_item_ref`: live/fixture work-item reference (`owner/repo#number`) resolved
 *   through `run.issue.ref` triples and `[Work item: owner/repo#number ...]` prompt scope (#320)
 */
export type LinkedRunBinding =
  | { by: "run_entity_id"; value: string }
  | { by: "coordinator_prompt"; equals: string }
  | { by: "work_item_ref"; ref: string };

export interface ItemsResult {
  lookup: LookupStatus;
  reason?: string;
  columns: string[];
  items: WorkItem[];
  next_cursor?: string;
}

export interface ItemResult {
  lookup: LookupStatus;
  reason?: string;
  /** Absent with `complete` means the item does not exist (404). */
  item?: WorkItem;
}

export interface BindingsResult {
  lookup: LookupStatus;
  reason?: string;
  bindings: LinkedRunBinding[];
}

export interface WorkSource {
  readonly kind: "fixture" | "github";
  listItems(repository: Repository): Promise<ItemsResult>;
  getItem(ref: ItemRef): Promise<ItemResult>;
  linkedRunBindings(ref: ItemRef): Promise<BindingsResult>;
}

export const GITHUB_ITEMS_UNSUPPORTED = "github source arrives with #273";
export const GITHUB_RUNS_UNSUPPORTED =
  "no run.issue.ref writer is wired on this runtime";

export { createGithubSource } from "./githubSource";
export type { GithubSourceOptions } from "./githubSource";
