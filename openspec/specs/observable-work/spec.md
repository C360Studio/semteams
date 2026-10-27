# Observable Work Specification

## Purpose

Define the read-only work lens over GitHub issue-backed work items. GitHub owns work-item identity and fields; the
portfolio document scopes repositories and Project status columns; every overlay read from the runtime carries an
explicit known, none or unknown state; and rule-fired controls on the runs lens render as explained events. The lens
asserts no write path and derives no status from another.
## Requirements
### Requirement: Work lens over GitHub issue-backed work items

SemTeams SHALL present the work items of the configured portfolio as GitHub issue-backed work items on a board and
a table grouped by project, before and independent of any run. GitHub SHALL own work-item identity, body, assignees,
labels and milestone. Both fixture-backed (`WORK_SOURCE=fixture`) and live GitHub (`WORK_SOURCE=github` with
`SEMTEAMS_PORTFOLIO_PATH`) work sources SHALL be served behind `/api/work/*`, and a failed or truncated GitHub read
SHALL answer `failed` or `partial` with a reason and SHALL NOT render an empty board as "no work". Columns SHALL
follow the configured GitHub Projects status field; without a Project the columns SHALL be Todo, In Progress and Done
with unknown metadata left unset. PM status, execution stage and verification SHALL be displayed as separate values and
SHALL NOT be derived from one another.

#### Scenario: Repository without a configured Project

- **GIVEN** a configured repository that has no GitHub Project
- **WHEN** its issues are rendered on the board
- **THEN** cards SHALL be placed in Todo, In Progress or Done from the documented fallback mapping
- **AND** priority and PM status fields with no source SHALL be left unset, not defaulted

#### Scenario: Work item without a run

- **GIVEN** a work item that has never had a linked run
- **WHEN** its overview is opened
- **THEN** purpose, owner, priority, milestone, dependencies and delivery context SHALL render from the issue
- **AND** the execution stage SHALL render as no run or unknown, never as a stage

#### Scenario: Live GitHub enumeration derives baton, review, and gate overlays

- **GIVEN** `WORK_SOURCE=github` and a valid `SEMTEAMS_PORTFOLIO_PATH`
- **WHEN** `/api/work/items?repository=owner/name` or `/api/work/items/owner/name/number` is requested
- **THEN** the server SHALL read issues, linked pull requests, and PR review comments from GitHub and derive `claim`,
  `review`, and `gate` overlays without altering GitHub-owned columns
- **AND** any failed or truncated GitHub read SHALL report `lookup: "failed"` or `"partial"` with its reason rather than
  an empty `complete` result

#### Scenario: Live GitHub Projects v2 status and priority hydration with honest fallback

- **GIVEN** `WORK_SOURCE=github` and a repository in `SEMTEAMS_PORTFOLIO_PATH` declaring `project_board` (`owner`, `number`, `status_field`, and optional `priority_field`)
- **WHEN** `/api/work/items?repository=owner/name` or `/api/work/items/owner/name/number` is requested
- **THEN** the server SHALL read the configured GitHub ProjectV2 single-select `status_field` options and item field values via a bounded read-only GraphQL query and populate ordered board columns, `pm_status`, and `priority`
- **AND** if the Projects v2 GraphQL read fails or the configured status field cannot be resolved, the response SHALL report `lookup: "partial"` with the failure reason, leave `pm_status` and `priority` unset, and fall back to the issue-state columns (`Todo`, `In Progress`, `Done`)

### Requirement: Explicit unknown overlays over scoped reads

Each graph-derived overlay (linked runs, execution stage from `agent.run.phase`, needs-you from `ask_user`, approval
or park) and each protocol-derived coordination overlay (builder claim baton, cross-agent review baton, and
decision/hold gate) SHALL carry an explicit `known`, `none` or `unknown` state. For live work items, linked runs SHALL
be resolved through `run.issue.ref` graph facts and work-scoped coordinator prompt headers (`[Work item: owner/repo#number ...]`)
on `agent.loop.description`. "No linked run", "unclaimed", "no review", or "no hold gate" SHALL be shown only after a
complete, successful, repository-scoped lookup that inspected the underlying records; a failed, partial, or uninspected
source SHALL render `unknown` with its reason. Attention and at-risk findings SHALL be absent until Program Pulse
supplies them and SHALL NOT be simulated.

#### Scenario: Partial lookup

- **GIVEN** a run lookup that fails or returns a partial result for a work item
- **WHEN** the card and overview render
- **THEN** the linked-run overlay SHALL show unknown
- **AND** it SHALL NOT show "no linked run"

#### Scenario: Uninspected pull request review records

- **GIVEN** a work item whose source does not supply linked pull requests or pull request review records
- **WHEN** the card, table row, and overview render
- **THEN** the review baton overlay SHALL show `unknown` with its reason
- **AND** it SHALL NOT show "no review" or "not requested"

#### Scenario: Live work item run linkage via work-scoped prompt or `run.issue.ref`

- **GIVEN** a work item `owner/repo#number` whose run bindings declare `{ by: "work_item_ref", ref: "owner/repo#number" }`
- **WHEN** `/api/work/items/owner/repo/number/runs` resolves against `/graph/triples`
- **THEN** any coordinator loop carrying a `[Work item: owner/repo#number ...]` prefix on `agent.loop.description` or any
  run/coordinator entity carrying `run.issue.ref = "owner/repo#number"` SHALL resolve to a `LinkedRun` with its execution
  stage and needs-you state
- **AND** when neither query matches a run, the lookup SHALL report `partial` with its reason rather than fabricating
  "no linked run"

### Requirement: Read-only boundary

The work lens SHALL NOT mutate GitHub or portfolio work-source state: no drag changes a column, and `/api/work/*` SHALL NOT expose any write verb to create or edit an issue, pull request, label, milestone or Project field. The browser SHALL read work-source records only through the product read API that enforces repository and run scoping, and SHALL NOT connect to NATS or hold admin credentials. Fixture-backed enumeration is served behind that API, and live enumeration (#273) SHALL be served behind the same API when it lands. Inline agentic runtime interactions (inspecting a linked run's trajectory or evidence, resolving a runtime approval/clarification, or sending a work-scoped prompt to the coordinator) SHALL route through the runtime dispatch/loop APIs without altering any work item's GitHub-owned column or Project status.

#### Scenario: Card drag

- **GIVEN** a card on the work board
- **WHEN** the operator drags it to another column
- **THEN** no run, issue or Project field SHALL change
- **AND** the card SHALL return to the column its source status places it in

### Requirement: Rule-fired controls render as explained events

When the runs lens reached from a work item shows a control fired by a rule, it SHALL show the control identity, the
firing entity, the firing fact and its value at spawn, the control's outcome, and the rule identity when the runtime
records it, otherwise an explicit unknown with its reason, so the story answers why the control happened without the
trajectory.

#### Scenario: Ops observer firing

- **GIVEN** a fixture run whose `agent.run.phase` reached a terminal value and fired the ops observer rule
- **WHEN** the operator drills into that run from its work item
- **THEN** the story SHALL show `agent.run.phase` and its value at spawn, the fired control's identity and its outcome
  as one explained event
- **AND** the rule identity SHALL render as unknown with its reason while the runtime does not record it

### Requirement: Structured builder claim, review parity, and decision/hold gate overlays

SemTeams SHALL derive structured coordination overlays on each work item from repository-protocol records without
introducing a separate agent message bus or mutating GitHub state:
- **Builder claim baton (`claim`):** SHALL identify an active claiming pull request (`agent-pr` when the branch prefix
  is `claude/`, `codex/`, or `gemini/` or `implemented-by` names an agent persona; `human-pr` otherwise) or issue
  assignee (`assignee`), including the pull request reference, draft status, `implemented-by` attribution, and declared
  stop-point when present.
- **Review baton (`review`):** SHALL compare the active pull request's `HEAD` commit SHA against its latest structured
  `Review request` and its effective `Review record` — accepting both ATeam's canonical `reviewed-by:` (or an
  `Independent Cross-Agent Review` markdown heading naming a single agent vendor `claude`, `codex`, or `gemini`) +
  `(Re-)Reviewed at <sha>` + `Verdict:` PR comment records (outside CommonMark `0..3`-space fenced code blocks and `>`
  blockquotes) and historical `<!-- semteams-review-request -->` / `<!-- semteams-review-record -->` markers — and
  report `not_requested`, `requested`, `current` (when the effective `Review record` passes at `HEAD`), `stale` (when
  commits landed after the latest `Review record` without a newer request at `HEAD`), or `changes_requested` (when the
  effective `Review record` at `HEAD` reports blocking findings or `REQUEST CHANGES`), along with `reviewed-by` and
  reviewer-lane attribution. Matching ATeam's merge gate (`C360Studio/ateam` `internal/review/gate.go` `gateVerdict()`,
  `36b999b`), an ATeam record SHALL map to `pass` only when it carries exactly one `Verdict:` line outside code fences
  and that line, trimmed, is `Verdict: APPROVE` (case-insensitive); a value led by `REQUEST CHANGES`,
  `CHANGES REQUESTED`, `BLOCKING`, or `FAIL` SHALL map to `blocking`; any other value — including conditional or
  decorated approvals such as `APPROVE pending H1`, `APPROVE, but REQUEST CHANGES on H1`, or `APPROVE with nits` — and
  more than one `Verdict:` line SHALL leave the comment unparsed. The effective `Review record` SHALL aggregate valid
  records at `HEAD` across reviewer lanes (`<name>-reviewer` tokens of the lane line; a record without a lane line
  covers all lanes): a blocking lane SHALL remain effective until a newer record for that lane or a newer all-lanes
  record supersedes it. When no valid record is at `HEAD`, the latest valid record is effective. Open pull request
  comments SHALL be read page by page up to `MAX_PR_COMMENT_PAGES`; when the last page read is full, or when a comment
  carrying `<!-- semteams-review-record -->` or unquoted ATeam review-record signals fails to parse unambiguously and is
  not followed by a valid `Review record` covering its reviewer lane (matching ATeam `record()`, which skips non-records
  and keeps the latest valid record), the source SHALL mark `review_inspected: false` (`lookup: "partial"`) and render
  the `review` overlay as `unknown` rather than retaining an earlier `current` or `known` state.
- **Decision and hold gate (`gate`):** SHALL surface `status:needs-decision` owner-docket labels, `status:blocked`
  labels and `blocked-by` dependencies, repository-wide open `class:flake` merge holds on active pull requests, and
  unticked OpenSpec `tasks.md` caveats (`HALT`, `RED`, `BLOCKED`, `WONTDO`, `OPEN-Q`).

None of `claim`, `review`, or `gate` SHALL alter the work item's board column (`column`) or Project status (`pm_status`).

#### Scenario: Agent draft pull request claim with declared stop-point

- **GIVEN** a work item linked to an open draft pull request on branch `claude/gh312-mqtt-nats-research` declaring a
  stop-point
- **WHEN** the work item renders on the board, table, and overview panel
- **THEN** the builder claim baton SHALL render as `known` with vendor `claude`, the pull request reference, draft
  status, and the declared stop-point
- **AND** the work item's column SHALL remain determined solely by its GitHub Project status or GitHub field fallback

#### Scenario: Stale cross-agent review record after follow-up commit

- **GIVEN** a work item whose active pull request has `head_sha` `a1b2c3d` and whose latest `Review record` comment was
  recorded at commit `9988776`
- **WHEN** the work item renders on the board, table, and overview panel
- **THEN** the review baton SHALL render as `known` with status `stale`
- **AND** the overview panel SHALL display both the `HEAD` SHA and the stale `Review record` SHA

#### Scenario: ATeam upstream review record and historical SemTeams marker compatibility

- **GIVEN** a pull request carrying an ATeam review record comment with vendor-identifying `reviewed-by: codex (gpt-6-astra, high)`
  (or an `Independent Cross-Agent Review` heading naming `<vendor>`), `(Re-)Reviewed at <sha>`, and `Verdict: APPROVE` or
  `Verdict: REQUEST CHANGES` outside `0..3`-space code fences and `>` blockquotes
- **WHEN** `parseReviewComments` inspects the pull request's comments alongside historical `<!-- semteams-review-* -->`
  comments
- **THEN** ATeam's `APPROVE` and `REQUEST CHANGES` verdicts SHALL map to `pass` and `blocking` respectively at the
  single designated `<sha>` (ignoring fenced examples and quoted replies, and rejecting vendor-less headings,
  non-`Reviewed at` SHA lines, or ambiguous multi-commit comments)
- **AND** valid historical `<!-- semteams-review-request -->` and `<!-- semteams-review-record -->` comments outside
  code and blockquotes SHALL continue to parse to the same result

#### Scenario: Unparseable review record fails closed to unknown

- **GIVEN** an open pull request whose comments include a valid `APPROVE` record at `HEAD` followed by a comment
  carrying `<!-- semteams-review-record -->` or unquoted ATeam review-record signals whose reviewer identity,
  `Reviewed at <sha>`, or `Verdict:` cannot be parsed unambiguously
- **WHEN** the GitHub work source enriches the pull request and derives baton overlays
- **THEN** `parseReviewComments` SHALL set `review_unparsed: true`
- **AND** the work item lookup SHALL report `partial` with `review_inspected: false` and render the `review` overlay as
  `unknown` rather than `current`

#### Scenario: Later valid review record clears an earlier unparseable comment

- **GIVEN** an open pull request whose comments include an unparseable record-shaped comment (for example an
  implementer reply quoting a verdict and commit SHA) followed by a valid `Review record` at `HEAD`
- **WHEN** `parseReviewComments` inspects the comments
- **THEN** it SHALL NOT set `review_unparsed` when the earlier comment names no reviewer lane, or names only lanes the
  later record covers
- **AND** it SHALL keep `review_unparsed: true` when the earlier comment names a reviewer lane (for example
  `svelte-reviewer`) that no later record covers

#### Scenario: Conditional APPROVE verdict does not pass

- **GIVEN** an ATeam review record at `HEAD` whose only `Verdict:` line is `Verdict: APPROVE pending H1 fix`,
  `Verdict: APPROVE, but REQUEST CHANGES on H1`, or another value that is not exactly `APPROVE`
- **WHEN** the GitHub work source derives the `review` overlay
- **THEN** the record SHALL NOT map to `pass` and the `review` overlay SHALL render as `unknown`, not `current`

#### Scenario: Blocking reviewer lane survives another lane's approval

- **GIVEN** an open pull request with a `svelte-reviewer` `REQUEST CHANGES` record followed by a `go-reviewer`
  `APPROVE` record, both at `HEAD`
- **WHEN** the GitHub work source derives the `review` overlay
- **THEN** the `review` overlay SHALL render `changes_requested` with reviewer lane `svelte-reviewer`
- **AND** a later `svelte-reviewer` `APPROVE` or all-lanes `APPROVE` record at `HEAD` SHALL make it `current`

#### Scenario: Review comments span more than one page

- **GIVEN** an open pull request whose first comment page holds `GITHUB_PAGE_LIMIT` comments including an `APPROVE` at
  `HEAD` and whose second page holds a `REQUEST CHANGES` at `HEAD` for the same lane
- **WHEN** the GitHub work source enriches the pull request
- **THEN** it SHALL read the second page and render the `review` overlay as `changes_requested`
- **AND** when every page up to `MAX_PR_COMMENT_PAGES` is full it SHALL mark `review_inspected: false` and render the
  `review` overlay as `unknown`

#### Scenario: Decision docket and OpenSpec hold gates

- **GIVEN** a work item carrying `status:needs-decision` or linked to an OpenSpec change with an unticked `Hold:` task
  line
- **WHEN** the work item renders on the board, table, and overview panel
- **THEN** the gate overlay SHALL render as `known` with the corresponding `needs-decision` or `openspec-hold` entry and
  reference

### Requirement: Baton lens filtering on the work view

The `/work` route SHALL provide a URL-backed baton filter (`?baton=all|needs-decision|claimed|review|blocked|ready-to-land`)
that slices visible work items across both board and table views by their coordination state (`Needs decision`, `Active
baton`, `Review queue`, `Held / blocked`, `Ready to land`) while preserving each repository's ordered columns and
keeping empty columns visible.

#### Scenario: Filtering the work board by review queue or ready-to-land

- **GIVEN** a portfolio on `/work` with items across multiple baton states
- **WHEN** the operator selects the `Review queue` (`?baton=review`) or `Ready to land` (`?baton=ready-to-land`) filter
- **THEN** only work items matching that baton state SHALL render inside their GitHub-owned columns
- **AND** switching between `Board` and `Table` views SHALL preserve the active `?baton=` filter in the URL

### Requirement: Inline agentic run workbench in the work item drawer

When a work item's linked runs have been loaded on `/work` and at least one linked run resolves a `coordinator_loop_id`, `WorkItemOverview` SHALL render an inline run workbench directly inside the `/work` drawer without requiring navigation away from `/work`. The inline workbench SHALL provide an `Activity | Evidence` tab bar rendering `TaskStory` (including rule-fired control explanations and sub-loop focus navigation) and `RunEvidencePanel` (raw trajectory, message log, and run entity graph triples), and SHALL surface `RunHealthPanel`, `PendingApprovalSection`, and `RunWaitingSection` inline when live loop state is present in `taskStore`, while retaining the link to open the run in the standalone Runs lens (`/?task=<loopId>`).

#### Scenario: Inspecting a linked run's story and evidence inline on `/work`

- **GIVEN** a work item on `/work` whose loaded linked run resolves `coordinator_loop_id`
- **WHEN** the operator loads the item's linked runs in the `/work` drawer
- **THEN** the inline run workbench SHALL render `TaskStory` under the `Activity` tab inside the `/work` drawer
- **AND** selecting the `Evidence` tab SHALL render `RunEvidencePanel` for that run inline without leaving `/work`

### Requirement: Work-scoped prompt and context chip in ChatBar

When a work item is selected on `/work`, `ChatBar` SHALL display a work-item context chip identifying the selected item's portfolio scope (`Program / Project`) and reference (`owner/repo#number` and title), and submitting a prompt from `ChatBar` while that scope is active SHALL include the work item reference and title in the outbound coordinator message.

#### Scenario: Sending a work-scoped prompt from `/work`

- **GIVEN** an operator viewing `/work?item=c360studio%2Fsemteams%23312`
- **WHEN** the operator enters a prompt in `ChatBar` and submits it
- **THEN** `ChatBar` SHALL display the `c360studio/semteams#312` context chip before submission
- **AND** the outbound coordinator message SHALL carry the `[Work item: c360studio/semteams#312 — ...]` context header

