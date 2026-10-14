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

export interface PullRequestReviewRequest {
  sha: string;
  reviewer_lane?: string;
}

export interface PullRequestReviewRecord {
  sha: string;
  verdict: "pass" | "blocking";
  reviewer_lane?: string;
  reviewed_by?: string;
}

export interface PullRequestRef {
  /** `owner/repo#number` */
  ref: string;
  title?: string;
  state?: "open" | "merged" | "closed";
  draft?: boolean;
  branch?: string;
  head_sha?: string;
  stop_point?: string;
  implemented_by?: string;
  reviewed_by?: string;
  review_request?: PullRequestReviewRequest;
  review_record?: PullRequestReviewRecord;
}

export type AgentVendor = "claude" | "codex" | "gemini";

export interface WorkClaim {
  kind: "agent-pr" | "human-pr" | "assignee";
  /** Agent vendor/persona, PR author/branch, or GitHub assignee login(s). */
  holder: string;
  agent_vendor?: AgentVendor;
  /** `owner/repo#number` when claimed by an open or draft PR. */
  pull_request?: string;
  branch?: string;
  draft?: boolean;
  /** Declared stop-point from the PR description when present. */
  stop_point?: string;
  implemented_by?: string;
}

export type WorkReviewStatus =
  | "not_requested"
  | "requested"
  | "current"
  | "stale"
  | "changes_requested";

export interface WorkReviewBaton {
  pull_request: string;
  status: WorkReviewStatus;
  head_sha?: string;
  request_sha?: string;
  record_sha?: string;
  reviewer_lane?: string;
  implemented_by?: string;
  reviewed_by?: string;
}

export type WorkGateKind =
  | "needs-decision"
  | "blocked"
  | "flake-hold"
  | "openspec-hold";

export type OpenSpecCaveatClass = "HALT" | "RED" | "BLOCKED" | "WONTDO" | "OPEN-Q";

export interface WorkGate {
  kind: WorkGateKind;
  summary: string;
  ref?: string;
  caveat_class?: OpenSpecCaveatClass;
}

export type BatonFilter =
  | "all"
  | "needs-decision"
  | "claimed"
  | "review"
  | "blocked"
  | "ready-to-land";

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
  /** Builder / claim baton derived from active PRs and issue assignees. */
  claim: Overlay<WorkClaim>;
  /** Cross-agent / PR review baton derived from active PR review records and HEAD SHA. */
  review: Overlay<WorkReviewBaton>;
  /** Decision docket, dependency blockers, flake holds, and OpenSpec caveats. */
  gate: Overlay<WorkGate[]>;
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
