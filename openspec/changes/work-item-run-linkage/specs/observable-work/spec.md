## MODIFIED Requirements

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
