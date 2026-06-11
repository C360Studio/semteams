// Package chain ships the product-shell consumers of ADR-038's chain
// entity primitive. The 6-part chain entity ID constructor and the
// reliable parent linkage on rule-fanned spawns both live upstream
// (semstreams: agentic.ChainExecutionEntityID +
// rule.executePublishAgent's task.ParentLoopID propagation, see
// agentic/entity_ids.go and processor/rule/actions.go).
//
// Post-ADR-053 the chain entity IS the run entity (agent.chain.execution.<id>,
// rooted at the coordinator loop). The hand-rolled run/milestone layer that
// once lived here (DispatchedStamper / ResearchMilestoneStamper /
// CompletionSubscriber and the chain.* milestone projections) was RETIRED in
// ADR-053 Phase 4a — the agent-run substrate (lifecycle Participant + the
// agent-run rule pack) owns run-phase authority now. Phase 5 removed the
// orphaned vocabulary (chain/predicates.go) and the dead chain/lineage_reader.go.
//
// ADR-053 Phase 5 (semstreams#250 / beta.105) then retired the ancestry-walking
// Resolver itself. Run identity now travels on the wire — ToolCall.Metadata
// carries agent.run_id + agent.run_entity_id, and LoopFailedEvent /
// LoopCompletedEvent carry RunEntityID — so product-shell consumers reconstruct
// run identity from the execution context (see cmd/semteams/runanchor) instead
// of walking agent.loop.parent triples. The Resolver, ParentReader,
// NATSParentReader, ResolveChainEntityID, IDResolver, ChainEntityRoleKey, and
// ValidateLoopID all retired with it (the role key moved to runanchor).
//
// What still lives here:
//
//   - NATSEntityReader / EntityTripleReader (entity_reader.go): a single-entity
//     read over the graph.query.entity RPC, returning a flat predicate→object
//     map. The one consumer that cannot recover its run anchor from the wire is
//     the operator-decision path (chainpause.DecisionHandler): it has only an
//     HTTP-body failed_loop_id, so it reads the failed loop entity's
//     agent.run.entity_id via this reader. DefaultGraphQueryEntitySubject is the
//     fallback RPC subject (operators override via the graph-query port config).
package chain
