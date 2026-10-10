# research-containment Specification

## Purpose
Bound individual agent loops in the live research and autoresearch packs through the frozen spawn-budget contract,
and keep containment claims within the behavior qualified at public execution boundaries.

## Requirements
### Requirement: Explicit live-pack loop ceilings

Every `publish_agent` action in the live research and autoresearch packs SHALL declare a positive literal
`loop_max_iterations` and document its role budget. The existing frozen executor SHALL carry the value in the canonical
task; the loop SHALL enforce the smaller of this value and the component ceiling. Budget exhaustion SHALL NOT be
reported as successful research.

#### Scenario: Authored role budget reaches loop intake

- **GIVEN** a live-pack spawn action with an explicit positive ceiling
- **WHEN** the frozen action executor publishes its task and public loop intake admits it
- **THEN** the task carries the authored ceiling and the effective loop budget reflects it
- **AND** a lower component ceiling narrows the budget

#### Scenario: A loop exhausts its allowed iterations

- **GIVEN** a loop whose allowed iterations are exhausted without a successful terminal action
- **WHEN** the runtime evaluates the next iteration boundary
- **THEN** it rejects the response at the exhausted boundary and records the existing failed/`max_iterations` outcome
- **AND** a terminal action within the budget retains its existing successful semantics
- **AND** qualification records that the frozen runtime may publish a further request before that response-side guard; the configured ceiling is not an exact provider-call limit

#### Scenario: Qualification finds an omitted or invalid budget

- **GIVEN** a live-pack agent spawn whose budget is absent, zero, negative or malformed
- **WHEN** the live-pack qualification runs
- **THEN** it rejects that configuration instead of accepting an implicit component default

### Requirement: Containment claims match enforcement scope

Research guidance SHALL identify clarification as coordinator recovery and SHALL NOT describe per-entity rule counters
as a whole-run ceiling. Per-spawn qualification SHALL NOT claim bounded fan-out, cross-loop recovery, unattended
scheduling, paid-model quality or future SemEngine compatibility.

#### Scenario: Rejection creates a fresh loop identity

- **GIVEN** reviewer rejection spawns another planner
- **WHEN** the pack documents its budget guarantees
- **THEN** it identifies only the per-loop ceiling as established by this change
- **AND** total-run recovery enforcement remains a separate unresolved contract

