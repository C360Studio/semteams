// Package approvalpause reflects the loop-level human-approval tool-gate at the
// RUN-phase level (ADR-053 §4c PR-1).
//
// When a loop calls a tool listed in agentic-tools.approval_required, the
// upstream ApprovalFilter rejects the call with the "approval_required: " prefix
// and the agentic-loop transitions the LOOP to LoopStateAwaitingApproval, emitting
// agent.approval_pending.<loopID> (agentic.ApprovalPendingEvent) "so a product-layer
// UI can surface the request" (handlers.go gateForApproval). The loop pauses today;
// the RUN it belongs to stays misleadingly `executing` — the same honesty gap 4b-2
// closed for clarification (decide(ask_user)).
//
// This package closes that gap for the tool-gate:
//
//  1. Subscriber binds a core-NATS subscription to agent.approval_pending.> and
//     decodes the ApprovalPendingEvent (which carries only LoopID — NO run anchor,
//     unlike the #250 LoopFailedEvent).
//  2. Pauser resolves the loop's run anchor with a single graph read of the loop
//     entity (agent.run.entity_id, falling back to lineage.run-loop-entity-id — the
//     4b-1a inherit/threaded anchor split, absorbed in Go so the pause RULE needs no
//     07/08-style split) and stamps agent.run.approval_pending on the run entity.
//  3. configs/rules/agent-run/12-executing-to-awaiting-on-approval.json consumes the
//     marker (mirror of rule 09) and transitions the run executing→awaiting_approval.
//
// A run-less loop (a front-door coordinator answering plain chat, or any standalone
// loop) has no run anchor and is a no-op — exactly mirroring the front-door
// decide(ask_user) no-op (rules 07/08 run-anchor guard).
//
// # 4b-2 vs 4c disambiguation
//
// Both pauses land the run in `awaiting_approval`; they are distinguished by
// CAUSE-MARKER, not phase. 4b-2 (clarification) stamps agent.run.clarification_pending
// (the asking loop carries coordinator.user_question, no pending_approval); 4c
// (this package) stamps agent.run.approval_pending (the loop is in
// LoopStateAwaitingApproval with pending_approval set). The two resume halves key on
// DISTINCT markers (clarification_resumed vs approval_resumed) and MUST NOT
// cross-resume — pinned by the agent_run_pack contract tests.
//
// # Framework-alignment review (ADR-053 §4c, MANDATORY for a new product-shell subscriber)
//
// Upstream models approval at the LOOP level (in-memory LoopEntity.State + the two
// wire events for a UI); there is no upstream/roadmapped run-phase consumer of these
// events. Run-phase modelling is SemTeams' layer (ADR-053 adoption). The in-product
// precedent is cmd/semteams/chainpause — a product-shell Subscriber+Pauser that fires
// on a loop wire event, resolves the run anchor via a graph read, and stamps run-level
// triples. This package is the parallel subscriber pair, same shape, same
// anchor-resolution mechanism (chain.NATSEntityReader, the single surviving chain/
// reader after Phase 5). Migration posture: if upstream ever stamps an
// agent.run.approval_pending itself, retire this subscriber for the upstream triple —
// same posture as the run-anchor wire (#250/#256).
package approvalpause
