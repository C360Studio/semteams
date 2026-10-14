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
labels and milestone. The shipped work source is fixture-backed (`WORK_SOURCE=fixture`); a `github` work source
SHALL answer `unsupported` with a reason and SHALL NOT render an empty board as "no work" until live enumeration
lands (#273). Columns SHALL follow the configured GitHub Projects status field; without a Project the
columns SHALL be Todo, In Progress and Done with unknown metadata left unset. PM status, execution stage and
verification SHALL be displayed as separate values and SHALL NOT be derived from one another.

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

### Requirement: Explicit unknown overlays over scoped reads

Each graph-derived overlay (linked runs, execution stage from `agent.run.phase`, needs-you from `ask_user`, approval
or park) and each protocol-derived coordination overlay (builder claim baton, cross-agent review baton, and
decision/hold gate) SHALL carry an explicit `known`, `none` or `unknown` state. "No linked run", "unclaimed", "no
review", or "no hold gate" SHALL be shown only after a complete, successful, repository-scoped lookup that inspected the
underlying records; a failed, partial, or uninspected source SHALL render `unknown` with its reason. Attention and
at-risk findings SHALL be absent until Program Pulse supplies them and SHALL NOT be simulated.

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

### Requirement: Read-only boundary

The work lens SHALL NOT change run state: no drag changes a column, and no control starts, approves or completes a
run or edits an issue, label or milestone. The browser SHALL read only through a product read API that enforces
repository and run scoping, and SHALL NOT connect to NATS or hold admin credentials. Fixture-backed enumeration is
served behind that API, and live enumeration (#273) SHALL be served behind the same API when it lands.

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
  `Review request` and `Review record` comments and report `not_requested`, `requested`, `current` (when the latest
  `Review record` passes at `HEAD`), `stale` (when commits landed after the latest `Review record` without a newer
  request at `HEAD`), or `changes_requested` (when the latest `Review record` at `HEAD` reports blocking findings),
  along with `reviewed-by` and reviewer-lane attribution.
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

