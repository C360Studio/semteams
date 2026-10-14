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
 * How a fixture item names its runs (design D5). Both forms are fixture-only;
 * live linkage is the `run.issue.ref` predicate, which no runtime writer emits.
 */
export type LinkedRunBinding =
  | { by: "run_entity_id"; value: string }
  | { by: "coordinator_prompt"; equals: string };

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
