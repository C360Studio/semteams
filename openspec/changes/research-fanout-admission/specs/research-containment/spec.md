## ADDED Requirements

### Requirement: Authoritative fan-out design admission

A planner fan-out enforcement design SHALL identify the terminal decision's authoritative list and require validation
before its routing facts are committed. It SHALL derive the model-visible constraint and executor enforcement from
one pack-owned policy, preserve unrelated decisions, and distinguish per-plan width from concurrency and total-run
budgets. A design or audit-artifact schema limit SHALL NOT be described as shipped dispatch containment.

#### Scenario: Audit output can be bypassed

- **GIVEN** the research planner can terminate through `decide` without invoking `emit_plan`
- **WHEN** a design proposes a fan-out ceiling
- **THEN** it identifies and closes the terminal-list admission path rather than relying on audit emission
- **AND** it specifies an over-limit rejection with no committed decision facts or investigator dispatch

#### Scenario: The runtime integration seam is unresolved

- **GIVEN** the selected integration requires behavior absent from the frozen dependency
- **WHEN** implementation admission is assessed
- **THEN** the design identifies the exact ownership change and proving checks
- **AND** no speculative engine API, generic journal or silent shared-policy copy is admitted
