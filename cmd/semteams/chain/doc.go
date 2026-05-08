// Package chain ships the product-shell consumers of ADR-038's chain
// entity primitive. The 6-part chain entity ID constructor and the
// reliable parent linkage on rule-fanned spawns both live upstream
// (semstreams beta.54: agentic.ChainExecutionEntityID +
// rule.executePublishAgent's task.ParentLoopID propagation, see
// agentic/entity_ids.go and processor/rule/actions.go in beta.54).
//
// What lives here (PR B):
//
//   - Resolver: given a loop_id, walks `agent.loop.parent` triples back
//     to the chain root and returns the chain_id (= root loop's ID) or
//     the canonical 6-part chain entity ID. Used by every chain-triple
//     consumer (evidence preprocessor, emit_dev_via_spec tool, milestone
//     subscribers).
//
//   - DispatchedStamper: a CompletionHandler on agent.complete.> that
//     mints chain.dispatched_at on the chain entity when the chain root
//     completes (LoopCompletedEvent.ParentLoopID == ""). Falls back to
//     observation time on zero CompletedAt and stamps a companion
//     observed_fallback marker so the framework bug is visible to
//     downstream graph consumers.
//
//   - ResearchMilestoneStamper: a CompletionHandler that fires when
//     research-reviewer completes with coordinator.next_action="approved"
//     and writes the chain.research_artifact.* cluster (loop, harness,
//     actor_count, task_count) on the chain entity. The .path predicate
//     is deferred to PR C alongside emit_research_artifact markdown
//     rendering.
//
//   - CompletionSubscriber: single agent.complete.> subscription that
//     demuxes one decoded LoopCompletedEvent to N registered
//     CompletionHandlers. Each handler is invoked under a defer/recover
//     guard so a buggy handler cannot kill the subscription goroutine.
//     New milestones plug in by appending to the slice in main.go's
//     startChainMilestoneSubscribers.
//
// What's deferred (per ADR-038 §D5 migration table):
//
//   - chainpause re-point onto the chain entity (Phase 3): blocked on
//     upstream failed-loop ancestry stamping. LoopFailedEvent has no
//     ParentLoopID and buildLoopFailureTriples does not stamp
//     agent.loop.parent, so the resolver's walk would short-circuit
//     incorrectly. Tracked upstream; until then chain.paused.* keeps
//     landing on the failed loop entity per ADR-037 §D5.
//
//   - emit_research_artifact markdown rendering + chain.research_artifact.path
//     (PR C): out of PR B scope.
//
//   - Persona / rule prompt cleanup, lineage threading retirement, and
//     real-LLM smoke (PR D): out of PR B scope.
//
// All chain-triple writes go through this package's helpers rather than
// rule-action add_triple steps. Rationale: chain milestones are
// per-arc semantic concerns (which transitions count, which predicates
// land), best expressed in Go subscribers/preprocessors that already
// have the graph reader and platform identity in hand. Rule files stay
// focused on flow transitions; chain-triple emission is observable
// drift-detection territory (test/contract/chain_entity_coverage_test.go).
package chain
