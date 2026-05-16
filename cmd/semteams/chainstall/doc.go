// Package chainstall implements coordinator-redesign Slice 2: chain-stall
// recovery via per-reject-reason routing.
//
// # The gap
//
// Slice 1b's phase validator + ADR-041's spec/qa gates + ADR-039 Phase 1's
// Tier 3 catch-all + ADR-040's recovery cap all REJECT chains under various
// structural-violation conditions. Each writes a rejection cluster onto the
// chain entity (chain.phase_transition.rejected, chain.spec_mode_gate.rejected,
// chain.qa_mode_gate.rejected, chain.needs_review.classification,
// chain.recovery.exhausted). The rejection is observable in the graph but
// not actionable by any rule — the rule engine fires on loop-entity triples,
// not chain-entity triples. So a rejected chain wedges fail-safe, queryable
// but stalled.
//
// # Predicate-cluster naming
//
// Two distinct predicate clusters: chain.rejection.* (counter cluster —
// recovery_count + exhausted) and chain.stall.* (audit cluster — routed_to
// + reason + producer_loop_id + producer_role + observed_at +
// awaiting_human + proceed). The split keeps the cap-counter shape
// parallel to ADR-040's chain.recovery.* while keeping the audit cluster
// independent from the cap (operators can query "stalls routed today"
// without joining against the counter). Merging the two would couple
// the audit-cluster's vocabulary to cap-counter semantics it doesn't
// share.
//
// # The fix
//
// chainstall is a chain.CompletionHandler that runs AFTER the structural-gate
// handlers in startChainMilestoneSubscribers. On every loop completion it:
//
//  1. Skips if the chain already reached chain.terminal.reached (Slice 1c
//     handled the happy path).
//  2. Reads the chain entity for any of eight reject-reason predicates.
//     Absence → no-op (chain still progressing).
//  3. Increments chain.rejection.recovery_count (parallel to ADR-040's
//     chain.recovery.count but with broader aggregation scope — every
//     reject-reason class counts against the same cap).
//  4. Looks up the reason in reasonToRoute to choose coordinator-wake vs
//     human-approval. Cap-hit (count > threshold) ALWAYS routes human
//     regardless of per-reason default. The chain_stall_authority config
//     flag ("per_reason" | "all_human") is a global override.
//  5. Stamps the chain.stall.* audit cluster on the chain entity.
//  6. For coordinator-route: writes chain.stall.proceed="true" + the
//     chain.stall.reason token onto the PRODUCER loop entity. The wake-up
//     rule (configs/rules/coordinator/05-chain-stall-to-coordinator.json)
//     fires on those per-loop triples and publishes a coordinator task
//     with stall context. Same fail-safe semantics as ADR-040's
//     PredicateRecoveryProceed.
//  7. For human-route: stamps chain.stall.awaiting_human="true" on the
//     chain entity. The HTTP endpoint at /teams-loop/chain-stall/decide
//     (deferred to a follow-up commit per Slice 1c precedent) accepts the
//     operator's verdict (retry | abandon | defer).
//
// # Per-reason routing
//
// See routing.go's reasonToRoute table for the full assignment + rationale.
// Summary: framing-fixable rejections (mode_mismatch,
// missing_research_artifact, unrouted_needs_clarification) route to
// coordinator — an LLM can re-frame and the retry may converge. Budget /
// structural-impossibility rejections (phase_cap, invalid_edge,
// zero_tests_run, tests_failed_nonzero, chain.recovery.exhausted) route to
// human — more LLM calls won't fix these, they just cost more.
//
// # Within-event read-after-sibling-write coupling
//
// chainstall reads chain-entity triples that sibling CompletionHandlers
// (phasevalidator, specModeGate, qaModeGate, NeedsReviewStamper,
// recoverycounter) just wrote on the SAME agent.complete event. This is
// the first handler in the codebase with a within-event read-after-sibling-write
// dependency. It is safe because:
//
//  1. CompletionSubscriber dispatches handlers sequentially per event
//     (chain.CompletionSubscriber.handleMsg → invokeHandler loop).
//  2. Each sibling's AddTriple call is a synchronous NATS request/reply
//     against graph.mutation.triple.add — returns only after graph-ingest
//     has applied the triple.
//  3. chainstall's ReadEntity call is a synchronous NATS request/reply
//     against graph.query.entity — graph-ingest serves from its current
//     post-write state.
//
// startChainMilestoneSubscribers MUST register chainstall LAST in the
// handler slice. A contract test (TestChainstallRegistersLast) pins the
// ordering so a future handler addition can't silently break the
// dependency.
//
// # Migration target
//
// Product-local under ADR-029 product-shell-tool discipline. The
// framework-alignment review (memo §"Product-Shell-Tool Discipline") found
// no upstream equivalent — every other product (semspec, semdragons,
// semstreams cmd/semstreams) treats chain stalls as operator-only.
// SemTeams's coordinator-first stance (Slice 1c return path + this slice's
// LLM-driven recovery) is what makes the per-reason routing concept
// useful. If a sibling product adopts the same pattern, propose
// upstreaming the chainstall primitive then.
package chainstall
