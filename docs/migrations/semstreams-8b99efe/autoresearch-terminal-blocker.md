# Autoresearch terminal route qualification blocker

The frozen mock journey completes the optimization chain and final coordinator decision, but publishes no typed
terminal result to the user's channel. This is a product lineage limitation exposed by the strengthened delivery
gate. It is not evidence that the closed upstream #1094 change regressed.

## Reproduced evidence

The submission root is `64158ebb-2f1c-434c-b88d-eb8f487b5529`, user `ui-anonymous`, channel type `http`, channel ID
`http-1790856197506307379`. Its `autoresearch` decision is correctly settled as a handoff, not shown as the answer.
The baseline child has this root as both typed parent and run anchor.

Rule `autoresearch_iteration_dispatch` fires on the run entity. Its synthesize task has no `parent_loop_id`,
`run_id`, or channel fields, although `agent.related_loops.autoresearch-run` retains the root as product metadata.
The final durable ancestry is:

- Coordinator `c5795807-6b92-4c64-bf90-134942ac49e2`, decision `respond_direct`.
- Reviewer `ec8bb9c9-e6b0-4cfd-9b39-85e9e4066ceb`.
- Synthesize `511ab881-bc70-4726-b8c4-4aabf355c9c3`; no typed parent or run anchor.

The captured message logger contains the root submission `status` response and no terminal result.
[Exact selected task/completion/response envelopes](autoresearch-route-evidence.json) retain correlation and
ancestry fields; absent fields were absent on the wire, not inferred from UI display.
Full local capture: `/tmp/semteams-migration-8b99efe/evidence/target-browser/autoresearch/`.

## Contract attribution

At frozen SHA `8b99efe9c66a`, `processor/rule/actions.go:1799-1886` sets the parent only for a loop-entity trigger
and inherits `RunID` only from the firing entity's `agent.loop.run` triple. The authored run-triggered rule carries
product metadata instead. There is no supported action field for an explicit parent/run override in that source.

`processor/agentic-dispatch/terminal_settlement.go:393-397` explicitly classifies an ancestry chain severed by a
non-loop trigger as `route_less_settled`. Its resolver at lines 427-452 follows durable typed `RunID` and
`ParentLoopID` records in `AGENT_LOOPS`; it does not interpret category-specific `related_loops` metadata.

The migration adds no response publisher shim, no invented loop predicate on the run entity, and no new runtime
primitive. The frozen SHA is unchanged. Restoring final autoresearch delivery requires qualification of a loop-triggered SemTeams pack reauthoring,
or an approved explicit run-trigger lineage contract. Neither is implemented or qualified in this draft.
The full terminal delivery gate remains red; the draft must not claim end-to-end autoresearch user delivery. This is separate from evidence-body rendering limitations.

## Consumer reauthoring feasibility

The architect's final read-only check confirms this is current SemTeams pack wiring, not an unavoidable upstream
blocker. Frozen `processor/rule/actions.go:731-753` already stamps the persistent root coordinator's typed run anchor;
a loop-triggered dispatch can therefore preserve ancestry using existing primitives. SemTeams can pursue that design
independently of SemEngine and SemSource.

Simply changing rule 05 to fire on each baseline/execute loop is not equivalent:

- `processor/rule/stateful_evaluator.go:110-115,183-186` keys rule state and its iteration counter by firing entity.
  Every newly spawned loop would restart the counter instead of preserving the run's cap.
- `emitautoresearchbaseline/executor.go:131-138,237-257` seeds cap, best, status and pending state on the run.
  Frozen `processor/rule/expression_factory.go:288-303` substitutes from the firing entity/lifecycle context;
  `related_loops` metadata does not make the run snapshot available to those conditions and prompts.
- Rules 04a/04b account for clean and failed executions separately; 04c independently promotes the kept best result.
  A replacement must preserve that accounting, the stop latch, and the ordering of state used for the next dispatch.

A persistent root-loop trigger is a design candidate. It needs an explicit cross-entity projection/marker handoff
and qualification of cap, failure accounting, best promotion, stale completion and restart behavior. This is a material
pack contract reauthoring, not a one-field compatibility patch. Issue #280 retains that unqualified work; an upstream
API change is not asserted to be mandatory, and the hold does not expand SemEngine's first-release requirements.
