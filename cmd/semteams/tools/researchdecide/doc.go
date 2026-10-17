// Package researchdecide wraps upstream's agentictools.DecideExecutor to
// enforce the research planner fan-out containment ceiling (ADR-060 §7 / #272)
// under the frozen SemStreams dependency.
//
// Why this wrapper exists:
//
//  1. In the research category, `emit_plan` is additive audit; the active
//     fan-out trigger that rule 02 (`02-plan-to-gather.json`) iterates via
//     `for_each` is `coordinator.decision.subtopics`, stamped by `decide`.
//  2. Upstream's frozen `agentictools.DecideExecutor` accepts any `[]string`
//     for `subtopics`, stamps `coordinator.decision.next-action` and
//     `coordinator.decision.subtopics` on the planner loop entity, and
//     returns `StopLoop: true` — terminating the planner loop immediately.
//  3. By omitting upstream's builtin `decide` via `SkipBuiltins: ["bash", "decide"]`
//     (matching the `chainbash` pattern from ADR-041 Phase 4) and registering
//     this wrapper under the canonical `"decide"` name, SemTeams:
//     - advertises `minItems: 1, maxItems: 4` on `decide.subtopics` in
//     `ListTools()`; and
//     - rejects invalid `subtopics` counts (`0` or `> 4`, or missing
//     `subtopics` on `action="gather"`) with `ToolErrorInvalidArgs` and
//     `StopLoop: false` BEFORE `inner.Execute` writes any graph triples,
//     keeping the planner loop alive to self-correct within its
//     `loop_max_iterations` budget.
package researchdecide
