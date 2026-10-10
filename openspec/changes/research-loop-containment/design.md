# Frozen per-spawn loop containment

## Boundary and plan review

Base: SemTeams `d19c3014394935997cbbe59448ab451f2ef32dc4`; frozen SemStreams
`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`. ADR-060 requires spawn-level ceilings; ADR-061 assigns
pack policy here and generic durability to SemEngine. Independent architecture review approved this bounded slice.

The existing `rule.Action.LoopMaxIterations` is a string resolving to a positive integer. The action executor writes
`TaskMessage.MaxIterations`; loop intake takes the smaller positive task limit and component ceiling. This is distinct
from the rule evaluator's per-entity `max_iterations`. No new public contract or storage is introduced.

## Qualification

First record a failing regression for currently omitted budgets. Decode live rules through frozen public types,
execute their spawn actions and inspect the canonical task. Qualify public loop intake and component-ceiling narrowing.
Exercise iteration exhaustion using a supported public boundary or real component fixture; do not copy upstream-only
carrier test hooks. Any direct guard fixture must be labeled separately from a full multi-turn journey.

The research entry rule is hashed in `test/fixtures/rule-family-consumer/provenance.json`. Preserve the hash check,
refresh provenance and explicitly classify the new field in that fixture's projection.

## Delivery limits

Conservative role budgets need room for reads, artifact emission and a terminal action. Fan-out is not yet schema-bounded,
so per-loop budgets cannot promise all widths complete. Rule-driven fresh loops do not share a proved total-run counter.
Clarification remains coordinator recovery. Existing routing, tools, actions and run inheritance remain unchanged.
Evidence/typed handoffs (#271), fan-out/run-wide/scheduling work (#272) and deep research (#274) remain separate.
