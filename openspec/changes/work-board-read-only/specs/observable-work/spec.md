## ADDED Requirements

### Requirement: Work lens over GitHub issue-backed work items

SemTeams SHALL present all work across the configured portfolio as GitHub issue-backed work items on a board and a
table grouped by project, before and independent of any run. GitHub SHALL own work-item identity, body, assignees,
labels and milestone. Columns SHALL follow the configured GitHub Projects status field; without a Project the
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
or park) SHALL carry an explicit unknown state. "No linked run" SHALL be shown only after a complete, successful,
repository- and run-scoped lookup; a failed, partial or unsupported lookup SHALL render unknown. Attention and
at-risk findings SHALL be absent until Program Pulse supplies them and SHALL NOT be simulated.

#### Scenario: Partial lookup

- **GIVEN** a run lookup that fails or returns a partial result for a work item
- **WHEN** the card and overview render
- **THEN** the linked-run overlay SHALL show unknown
- **AND** it SHALL NOT show "no linked run"

### Requirement: Read-only boundary

The work lens SHALL NOT change run state: no drag changes a column, and no control starts, approves or completes a
run or edits an issue, label or milestone. The browser SHALL read only through a product read API that enforces
repository and run scoping, and SHALL NOT connect to NATS or hold admin credentials. Fixture-backed enumeration and
live enumeration SHALL be served behind the same API.

#### Scenario: Card drag

- **GIVEN** a card on the work board
- **WHEN** the operator drags it to another column
- **THEN** no run, issue or Project field SHALL change
- **AND** the card SHALL return to the column its source status places it in

### Requirement: Rule-fired controls render as explained events

When the runs lens reached from a work item shows a control fired by a rule, it SHALL show the rule, the firing fact
and its value, the control identity and the control's outcome, so the story answers why the control happened without
the trajectory.

#### Scenario: Ops observer firing

- **GIVEN** a fixture run whose `agent.run.phase` reached a terminal value and fired the ops observer rule
- **WHEN** the operator drills into that run from its work item
- **THEN** the story SHALL show the rule name, `agent.run.phase` and its value, the fired control's identity and its
  outcome as one explained event
