# Structured baton, review, and decision overlays design

## Scope and evidence

[Issue #314](https://github.com/C360Studio/semteams/issues/314) owns this slice. This is the `architect` design on the
structured coordination overlay shapes and their read-API boundary. SemTeams starts from `main` at `f7c39e6a` with
SemStreams frozen at `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`.

Inspected:
- Shared work protocol: `.agents/protocol.md` (draft PR claim rules, `claude/` | `codex/` | `gemini/` branch prefixes,
  `Review request` / `Review record` commit-SHA handoff contract, `implemented-by:` / `reviewed-by:`,
  `status:needs-decision`, `status:blocked`, `class:flake` merge gate, and `scripts/openspec-queue.sh` caveat classes).
- Observable work baseline (#296 / PR #304): `openspec/specs/observable-work/spec.md`,
  `openspec/changes/archive/2026-10-08-work-board-read-only/design.md`, `ui/src/lib/types/work.ts`,
  `ui/src/lib/server/work/{service.ts,source.ts,fixtureSource.ts,overlays.ts}`,
  `ui/src/lib/utils/workView.ts`, `ui/src/routes/work/+page.svelte`, and
  `ui/src/lib/components/work/{WorkBoard,WorkTable,WorkItemCard,WorkItemOverview,OverlayBadge}.svelte`.

**Result of the architect read:** no Go runtime type or new HTTP endpoint is needed. Baton, review, and decision/hold
facts are repository-protocol facts owned by GitHub and in-repo OpenSpec files, not runtime graph triples. They belong
on `WorkItem` (served by `GET /api/work/items` and `GET /api/work/items/{owner}/{repo}/{number}`) so the board and
table can render and filter them immediately without N×M graph scans, while preserving the strict `Overlay<T>`
(`known | none | unknown`) contract established in D1/D2 of `work-board-read-only`.

## D1 — Read-API continuity: protocol overlays ride on `WorkItem`, run overlays stay on `LinkedRun`

The four `GET /api/work/*` endpoints from `work-board-read-only` remain unchanged in route structure and method safety
(`GET`-only, 405 on other methods, 404 on unconfigured repositories, 503 on unconfigured work source):

| Surface | Owner | Where it is served |
|---|---|---|
| Identity, column, PM status, priority, milestone, labels, assignees | GitHub | `WorkItem` on `GET /api/work/items` and `GET /api/work/items/{owner}/{repo}/{number}` |
| Protocol baton overlays (`claim`, `review`, `gate`) | GitHub PRs/issues/labels + OpenSpec `tasks.md` | `WorkItem` on `GET /api/work/items` and `GET /api/work/items/{owner}/{repo}/{number}` |
| Runtime execution overlays (`execution_stage`, `needs_you`, `verification`) | SemTeams runtime graph + `/teams-dispatch/loops/{id}` | `LinkedRun` on `GET /api/work/items/{owner}/{repo}/{number}/runs` (loaded on request) |

Why protocol overlays live on `WorkItem` rather than `LinkedRun`:
1. **Pre-run and cross-tool visibility:** A developer or CLI/Enjoy agent (`claude`, `codex`, `gemini`) claims an issue
   via a draft PR (`Closes #n`) and posts `Review request` / `Review record` comments whether or not a SemTeams
   backend `AgentRun` was dispatched.
2. **Zero graph-scan amplification:** Board-wide filtering (`?baton=review` or `?baton=needs-decision`) operates over
   the repository's `WorkItem[]` payload already returned by `GET /api/work/items?repository=owner/name`.
3. **Shared intake with #273 and #267:** When #273 implements `WORK_SOURCE=github`, its GraphQL query fetches issue
   fields, linked closing PRs (branch name, draft state, `headRefOid`, latest review comments), repository `class:flake`
   issues, and OpenSpec `tasks.md` caveats in the same repository-scoped read that populates `WorkItem`, and #267
   Program Pulse consumes the identical normalized fields.

## D2 — Deterministic derivation and `known | none | unknown` rules for `claim`, `review`, and `gate`

Every new overlay uses the existing discriminated union `Overlay<T>` (`{ state: "known", value, reason? } |
{ state: "none", reason? } | { state: "unknown", reason }`). `none` is legal only when the source inspected the
underlying records and confirmed absence.

### 1. Builder / Claim Baton (`claim: Overlay<WorkClaim>`)

```ts
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
```

Derivation rules (evaluated in priority order on open/draft PRs, then assignees):
1. Select active (state `"open"`, including draft) PRs in `delivery`.
2. If an active PR exists whose `branch` starts with `claude/`, `codex/`, or `gemini/` (or declares `implemented_by`
   matching a known agent persona):
   - Emit `{ state: "known", value: { kind: "agent-pr", holder, agent_vendor, pull_request, branch, draft, stop_point, implemented_by } }`.
3. Else if an active PR exists with a known `branch` or `implemented_by`:
   - Emit `{ state: "known", value: { kind: "human-pr", holder, pull_request, branch, draft, stop_point, implemented_by } }`.
4. Else if an active PR exists in `delivery` but the source did not supply `branch` or `implemented_by` metadata for it:
   - Emit `{ state: "unknown", reason: "active pull request has no branch or claim metadata" }`.
5. Else if `delivery.state === "unknown"`:
   - If `item.assignees.length > 0`, emit `{ state: "known", value: { kind: "assignee", holder: item.assignees.join(", ") }, reason: "assigned on issue; linked pull requests unknown" }`.
   - Otherwise emit `{ state: "unknown", reason: "source does not supply linked pull requests" }`.
6. Else (`delivery` is `none` or all linked PRs are `merged`/`closed`):
   - If `item.assignees.length > 0`, emit `{ state: "known", value: { kind: "assignee", holder: item.assignees.join(", ") } }`.
   - Otherwise emit `{ state: "none" }`.

### 2. Review Baton (`review: Overlay<WorkReviewBaton>`)

```ts
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
```

Derivation rules from the active claiming PR:
1. If `delivery.state === "unknown"`, emit `{ state: "unknown", reason: "source does not supply linked pull requests" }`.
2. If `delivery.state === "none"` or no linked PR is in state `"open"`, emit `{ state: "none" }`.
3. For the active open PR, if `review_inspection` is absent (`head_sha` not supplied by the source), emit
   `{ state: "unknown", reason: "source does not supply PR review records" }`.
4. When `head_sha` is supplied:
   - If a `review_record` is present:
     - If `review_record.sha !== head_sha`:
       - If a newer `review_request` exists with `review_request.sha === head_sha`, `status = "requested"`; else
         `status = "stale"` (commits landed after the last review record).
     - Else (`review_record.sha === head_sha`):
       - `status = review_record.verdict === "pass" ? "current" : "changes_requested"`.
   - Else if a `review_request` is present:
     - `status = "requested"`.
   - Else:
     - `status = "not_requested"`.

### 3. Decision & Hold Gate (`gate: Overlay<WorkGate[]>`)

```ts
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
```

Derivation rules:
1. **Issue labels & dependencies (always known on a valid `WorkItem`):**
   - Label `status:needs-decision` → `{ kind: "needs-decision", summary: "status:needs-decision (owner docket)" }`.
   - Label `status:blocked` → `{ kind: "blocked", summary: "status:blocked", ref }` (paired with any `blocked-by`
     dependency refs).
   - Each dependency with `source === "blocked-by"` → `{ kind: "blocked", summary: "blocked by <ref>", ref }`.
2. **Repository `class:flake` hold:**
   - When the item has an active PR and the repository declares open `flake_holds` (`owner/repo#n`), add
     `{ kind: "flake-hold", summary: "required-job flake hold (<ref>)", ref }`.
3. **OpenSpec `tasks.md` caveats:**
   - When the item links an `openspec_change` with inspectable caveats (`HALT`, `RED`, `BLOCKED`, `WONTDO`, `OPEN-Q`
     from unticked `- [ ]` or `- [~]` lines per `scripts/openspec-queue.sh`), add one `openspec-hold` entry per caveat.
4. **Monotonicity of positive gates vs. unknown secondary sources:**
   - If one or more `WorkGate` entries are found, emit `{ state: "known", value: gates, ...(missingReason && { reason: missingReason }) }`
     because a known gate holds regardless of uninspected secondary sources.
   - If zero `WorkGate` entries are found:
     - If an `openspec_change` is referenced with `inspected: false`, emit
       `{ state: "unknown", reason: "OpenSpec tasks.md caveats not inspected" }`.
     - Otherwise emit `{ state: "none" }`.

## D3 — Baton filter lens (`?baton=`) and strict column non-derivation

`/work` adds a URL-backed baton filter bar (`?baton=`, updated with `goto(..., { replaceState: true, keepFocus: true, noScroll: true })`)
beside the `Board | Table` view toggle:

| `?baton=` value | Label | Matches items where |
|---|---|---|
| `all` (default, omitted in URL) | All work | Every item in the configured portfolio |
| `needs-decision` | Needs decision | `gate` contains `kind === "needs-decision"`, OR loaded runtime `needsYou` is `{ state: "known", value: true }` |
| `claimed` | Active baton | `claim.state === "known"` |
| `review` | Review queue | `review.state === "known"` and `review.value.status` is `"requested"`, `"stale"`, or `"changes_requested"` |
| `blocked` | Held / blocked | `gate.state === "known"` and `gate.value` contains `"blocked"`, `"flake-hold"`, or `"openspec-hold"` |
| `ready-to-land` | Ready to land | `review.state === "known"` with `status === "current"`, and `gate.state === "none"` |

**Column non-derivation invariant:** Filtering by `?baton=` narrows the visible items within each repository's
existing columns (`Todo`, `In Progress`, `In Review`, `Done` or fallback `Todo`, `In Progress`, `Done`). Empty columns
continue to render so board structure never shifts. Neither `claim`, `review`, nor `gate` ever alters `item.column` or
`item.pm_status`.

## D4 — UI presentation on cards, table rows, and overview panel

- **`WorkItemCard.svelte` and `WorkTable.svelte`:**
  - Render compact `OverlayBadge` controls for `claim` (`label="Baton"`), `review` (`label="Review"`), and `gate`
    (`label="Gate"`) alongside the existing `PM status`, `Stage`, `Needs you`, and `Verification` badges.
- **`WorkItemOverview.svelte`:**
  - Add a structured **Coordination & Baton** section displaying:
    - **Builder baton:** holder, vendor/kind, linked PR & branch, `implemented-by`, and declared `stop-point` (or
      `none` / `unknown — <reason>`).
    - **Review baton:** review status (`not_requested`, `requested`, `current`, `stale`, `changes_requested`),
      `HEAD` SHA vs. `Review request` SHA and `Review record` SHA, reviewer lane, and `reviewed-by` (or `none` /
      `unknown — <reason>`).
    - **Decision & hold gates:** list of active `needs-decision`, `blocked`, `flake-hold`, and `openspec-hold` items
      with their references and caveat classes (or `none` / `unknown — <reason>`).

## D5 — Fixture set `board-mvp` coverage

Extend `ui/src/lib/server/work/fixtures/board-mvp/items.json` so the shipped fixture set exercises every protocol
overlay state without breaking existing `work-board.spec.ts` assertions:
- `#310`: `status:needs-decision` + `blocked-by #314` → `gate` known (`needs-decision` + `blocked`), `claim` none,
  `review` none.
- `#312`: claimed by `claude/gh312-mqtt-nats-research` draft PR with stop-point and an OpenSpec `BLOCKED` hold line →
  `claim` known (`agent-pr`, `claude`), `review` known (`not_requested`), `gate` known (`openspec-hold`).
- `#314`: claimed by `codex/gh314-stamp-issue-ref` open PR where `head_sha` (`a1b2c3d`) moved ahead of `review_record.sha`
  (`9988776`) → `claim` known (`agent-pr`, `codex`), `review` known (`stale`).
- `#316`: claimed by `claude/gh316-explained-controls` open PR with `review_record.sha === head_sha` (`verdict: "pass"`,
  `reviewed_by: "codex (svelte-reviewer)"`) and zero gates → `claim` known (`agent-pr`, `claude`), `review` known
  (`current`), `gate` none (matches `?baton=ready-to-land`).
- `c360studio/semsource#51`: `pull_requests` omitted → `delivery`, `claim` (with assignee fallback reason), and
  `review` exercise explicit `unknown` reasons.
