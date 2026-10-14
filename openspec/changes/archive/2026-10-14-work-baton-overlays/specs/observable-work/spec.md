# Observable Work Specification

## MODIFIED Requirements

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

## ADDED Requirements

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
