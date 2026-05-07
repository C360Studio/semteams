// Package chain ships the product-shell consumers of ADR-038's chain
// entity primitive. The 6-part chain entity ID constructor and the
// reliable parent linkage on rule-fanned spawns both live upstream
// (semstreams beta.54: agentic.ChainExecutionEntityID +
// rule.executePublishAgent's task.ParentLoopID propagation, see
// agentic/entity_ids.go and processor/rule/actions.go in beta.54).
//
// What lives here:
//
//   - Resolver: given a loop_id, walks `agent.loop.parent` triples back
//     to the chain root and returns the chain_id (= root loop's ID) or
//     the canonical 6-part chain entity ID. Used by every chain-triple
//     consumer (chainpause, evidence preprocessor, emit_dev_via_spec
//     tool, milestone subscribers).
//
//   - Stamper: subscribes to agent.complete.dispatch and mints
//     chain.dispatched_at on the chain entity at chain start. Idempotent
//     re-writes are safe (chain_id is stable per chain).
//
//   - Per-arc milestone subscribers (research, …) — TBD per ADR-038
//     phasing; each milestone picks up its predicate cluster and writes
//     them onto the chain entity via the Resolver.
//
// All chain-triple writes go through this package's helpers rather than
// rule-action add_triple steps. Rationale: chain milestones are
// per-arc semantic concerns (which transitions count, which predicates
// land), best expressed in Go subscribers/preprocessors that already
// have the graph reader and platform identity in hand. Rule files stay
// focused on flow transitions; chain-triple emission is observable
// drift-detection territory (test/contract/chain_entity_coverage_test.go).
package chain
