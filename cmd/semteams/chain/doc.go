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
// What still lives here:
//
//   - Resolver (resolver.go): given a loop_id, walks `agent.loop.parent`
//     triples back to the chain root and returns the chain_id (= root loop's
//     ID) or the canonical 6-part chain entity ID. Used where no run anchor is
//     available on the execution context: chainpause (walks ancestry from an
//     agent.failed.* event, which carries no RunID) and the sandbox tools'
//     fallback path. NATSEntityReader / NATSParentReader back the walk over the
//     graph.query.entity RPC.
//
//   - ResolveChainEntityID + ChainEntityRoleKey (entity_resolver.go): the
//     canonical "what chain entity does this tool call belong to" lookup —
//     prefers related_loops["chain-entity-id"] (pinned by the spawn rule),
//     falling back to the Resolver ancestry walk. ValidateLoopID is the shared
//     entry-seam guard against malformed loop ids.
//
// MIGRATION NOTE (ADR-053 Phase 5): the long-term goal is to retire the
// ancestry-walking Resolver entirely by reading the loop's RunID directly off
// the execution context. That is BLOCKED upstream at beta.104 — agentic-loop
// does not inject the loop's RunID into ToolCall.Metadata (it injects only
// loop_id), and a failed-loop event carries no RunID. Once the framework
// exposes the run anchor on the tool/event context, the sandbox tools and
// chainpause can drop the Resolver. Tracked upstream in semstreams#250 and in
// the ADR-053 adoption plan §Phase 5.
package chain
