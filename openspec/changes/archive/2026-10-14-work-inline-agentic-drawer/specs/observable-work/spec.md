## MODIFIED Requirements

### Requirement: Read-only boundary

The work lens SHALL NOT mutate GitHub or portfolio work-source state: no drag changes a column, and `/api/work/*` SHALL NOT expose any write verb to create or edit an issue, pull request, label, milestone or Project field. The browser SHALL read work-source records only through the product read API that enforces repository and run scoping, and SHALL NOT connect to NATS or hold admin credentials. Fixture-backed enumeration is served behind that API, and live enumeration (#273) SHALL be served behind the same API when it lands. Inline agentic runtime interactions (inspecting a linked run's trajectory or evidence, resolving a runtime approval/clarification, or sending a work-scoped prompt to the coordinator) SHALL route through the runtime dispatch/loop APIs without altering any work item's GitHub-owned column or Project status.

#### Scenario: Card drag

- **GIVEN** a card on the work board
- **WHEN** the operator drags it to another column
- **THEN** no run, issue or Project field SHALL change
- **AND** the card SHALL return to the column its source status places it in

## ADDED Requirements

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
