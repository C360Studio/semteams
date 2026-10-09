// Work board wire types (openspec change work-board-read-only, design D1/D2/D4).
//
// These are the shapes served by GET /api/work/*. GitHub owns work-item
// fields; the runtime owns run facts. The two meet only in LinkedRun, and a
// linked run never moves a work item's column.
//
// There is deliberately no attention / at-risk field: it is absent until
// Program Pulse (#267) and is never simulated.

/** How completely a lookup answered. `none` overlays are legal only on `complete`. */
export type LookupStatus = "complete" | "partial" | "failed" | "unsupported";

export type WorkSourceKind = "fixture" | "github" | "unconfigured";

/**
 * A value that may not be knowable. `known` is the only variant that carries a
 * value; `unknown` always carries a reason; `none` means "looked, and there is
 * nothing" and is only emitted when the enclosing lookup is `complete`.
 */
export type Overlay<T> =
  | { state: "known"; value: T; reason?: string }
  | { state: "none"; reason?: string }
  | { state: "unknown"; reason: string };

// ---------------------------------------------------------------------------
// Portfolio (D4)
// ---------------------------------------------------------------------------

export interface ProjectBoard {
  owner: string;
  number: number;
  status_field: string;
  priority_field?: string;
}

export interface Repository {
  owner: string;
  name: string;
  project_board?: ProjectBoard;
}

export interface Project {
  id: string;
  name: string;
  repositories: Repository[];
}

export interface Program {
  id: string;
  name: string;
  projects: Project[];
}

export interface Portfolio {
  programs: Program[];
}

// ---------------------------------------------------------------------------
// Work items (D2)
// ---------------------------------------------------------------------------

export type WorkItemState = "open" | "closed";

/** Informational only: dependencies never gate or move a card. */
export interface WorkDependency {
  /** `owner/repo#number` */
  ref: string;
  source: "blocked-by" | "task-list";
}

export interface PullRequestRef {
  /** `owner/repo#number` */
  ref: string;
  title?: string;
  state?: "open" | "merged" | "closed";
}

export interface WorkItem {
  /** `owner/repo#number` */
  ref: string;
  /** `owner/repo` as configured in the portfolio */
  repository: string;
  number: number;
  title: string;
  body_excerpt: string;
  state: WorkItemState;
  labels: string[];
  assignees: string[];
  milestone?: string;
  /** Project status field value. Unset when the repository has no project board. */
  pm_status?: string;
  /** Project priority field value. Unset when no priority source is configured. */
  priority?: string;
  /** Board column: Project status when configured, else the GitHub-field fallback. */
  column: string;
  dependencies: WorkDependency[];
  /** Linked pull requests. `unknown` unless the source supplies them. */
  delivery: Overlay<PullRequestRef[]>;
}

// ---------------------------------------------------------------------------
// Linked runs (D2 overlays)
// ---------------------------------------------------------------------------

export type RunPhase =
  | "dispatched"
  | "executing"
  | "awaiting_approval"
  | "completed"
  | "failed"
  | "cancelled";

export interface LinkedRun {
  /** Full run entity id, or null while the run entity is not resolvable. */
  run_entity_id: string | null;
  /** Bare coordinator loop id (navigate to `/?task=<id>`), or null when unknown. */
  coordinator_loop_id: string | null;
  /** Completeness of this run's facts. */
  lookup: LookupStatus;
  reason?: string;
  execution_stage: Overlay<RunPhase>;
  needs_you: Overlay<boolean>;
  verification: Overlay<string>;
}

// ---------------------------------------------------------------------------
// Responses (D1)
// ---------------------------------------------------------------------------

export interface PortfolioResponse {
  source: WorkSourceKind;
  programs: Program[];
}

export interface ItemsResponse {
  lookup: LookupStatus;
  reason?: string;
  /** `owner/repo` as configured in the portfolio */
  repository: string;
  /** Ordered board columns for this repository. */
  columns: string[];
  items: WorkItem[];
  next_cursor?: string;
}

export interface ItemResponse {
  lookup: LookupStatus;
  reason?: string;
  item?: WorkItem;
}

export interface RunsResponse {
  lookup: LookupStatus;
  reason?: string;
  runs: LinkedRun[];
}

export interface PortfolioIssue {
  path: string;
  message: string;
}

/** Non-2xx body for every /api/work/* endpoint. */
export interface WorkApiError {
  code: string;
  error: string;
  issues?: PortfolioIssue[];
}
