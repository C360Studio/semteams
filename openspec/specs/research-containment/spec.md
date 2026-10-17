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
as a whole-run ceiling. Containment qualification SHALL verify per-spawn `loop_max_iterations`, schema and pre-mutation
planner fan-out bounds (`1..4` subtopics), and scheduled cron inflight/cooldown/kill-switch behavior on the frozen
runtime, while explicitly recording that cross-loop run-wide retry and cost ceilings across fresh loop identities remain
tracked upstream (`semstreams#383`, `semstreams#1005`, `semengine#24`).

#### Scenario: Rejection creates a fresh loop identity

- **GIVEN** reviewer rejection spawns another planner and subsequent review passes run under fresh reviewer loop entity IDs
- **WHEN** rule `05-reviewer-rejected-retry` evaluates `max_iterations: 3` across distinct reviewer loop entities
- **THEN** its per-entity counter applies per reviewer entity rather than enforcing a run-wide retry cap across fresh loop identities
- **AND** total-run recovery enforcement remains explicitly documented as an upstream runtime/SemEngine contract

### Requirement: Bounded research planner fan-out

The research pack SHALL bound planner subtopic decomposition to `1..4` items (`minItems: 1`, `maxItems: 4`) in both the `emit_plan` (`epics`) and `decide` (`subtopics`) tool schemas, and SHALL enforce that bound at execution time before any decision triples are written to the graph.

#### Scenario: Planner tool schemas advertise the 1..4 fan-out ceiling

- **GIVEN** the registered `emit_plan` and `decide` tool definitions resolved for `researcher-research-plan`
- **WHEN** a caller inspects `emit_plan`'s `epics` schema and `decide`'s `subtopics` schema
- **THEN** both array schemas declare `minItems: 1` and `maxItems: 4`
- **AND** `subtopics` remains optional on `decide` for roles and actions that do not fan out

#### Scenario: Valid planner fan-out dispatches N gatherers and joins

- **GIVEN** a `researcher-research-plan` loop calling `decide(action="gather", subtopics=[...])` with `N=1` or `N=4` strings
- **WHEN** the `decide` executor and research rule `02-plan-to-gather` process the decision
- **THEN** `decide` returns `StopLoop=true`, stamps `coordinator.decision.subtopics`, and rule `02-plan-to-gather` dispatches exactly `N` `researcher-research-gather` tasks

#### Scenario: Over-limit or empty planner fan-out is rejected pre-mutation and allows same-loop correction

- **GIVEN** a `researcher-research-plan` loop calling `decide(action="gather")` with missing, empty (`N=0`), or over-limit (`N=5`) `subtopics` (even if `emit_plan` was skipped)
- **WHEN** the `decide` executor evaluates the call
- **THEN** it returns `ToolErrorInvalidArgs` with `StopLoop=false` before writing any graph triples
- **AND** rule `02-plan-to-gather` dispatches zero gatherers for the rejected call
- **AND** a subsequent `decide(action="gather")` call with `1..4` subtopics in the same loop succeeds

### Requirement: Scheduled cron inflight, cooldown, and Pattern-B kill-switch containment

Scheduled cron rules SHALL prevent stacked dispatches via the scheduler's inflight and cooldown gates, and operators SHALL be able to disable a live cron rule via the Pattern-B `rule.ConfigManager` without restarting the process.

#### Scenario: Inflight and cooldown gates prevent stacked cron dispatches

- **GIVEN** an enabled cron rule whose action execution exceeds a tick interval or falls within its configured `cooldown`
- **WHEN** subsequent scheduler ticks arrive while the prior fire is in flight or within cooldown
- **THEN** the scheduler skips the overlapping ticks without dispatching duplicate actions

#### Scenario: Pattern-B rule disable stops future cron firings without restart

- **GIVEN** a running `rule.Processor` and `rule.ConfigManager` with an enabled cron rule that has fired
- **WHEN** the rule is updated to `enabled: false` through `rule.ConfigManager.SaveRule` without restarting the process
- **THEN** hot-reload reconciliation deregisters the scheduled rule and no further ticks fire across a bounded post-disable observation window
- **AND** disabling the schedule does not retroactively cancel work already dispatched by the prior fire

