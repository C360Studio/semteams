# Observable Work Specification

## MODIFIED Requirements

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
