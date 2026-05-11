// Package recoverycounter caps the number of source-curator recovery
// cycles a single chain can spend on research-reviewer rejections
// (ADR-040 §addendum 2026-05-11 "Chain-level recovery cap").
//
// The package implements one chain.CompletionHandler — the Counter —
// registered alongside the other ADR-038 milestone stampers in
// startChainMilestoneSubscribers. Each agent.complete event from a
// research-reviewer with coordinator.next_action="insufficient"
// triggers:
//
//  1. Walk ancestry from the reviewer loop to the canonical chain
//     entity via chain.Resolver.
//  2. Read the chain entity's current chain.recovery.count triple
//     (default 0 when absent — first rejection in the chain).
//  3. Increment.
//  4. Write the new count to the chain entity (audit always lands).
//  5. If within budget (new count ≤ threshold), write
//     chain.recovery.proceed="true" to the reviewer's loop entity.
//  6. If over budget (new count > threshold), write
//     chain.recovery.exhausted="true" to the chain entity. Do NOT
//     write proceed.
//
// # Why Counter-owned gating instead of rule-only
//
// The rule engine evaluates `$entity.triple.X` against the
// triggering entity's local triples — it does not walk ancestry to
// chain entities. Rule state is keyed `<rule_id>.<entity_id>` per-
// entity, so each new reviewer loop is a distinct entity with its
// own fresh state. A rule-only cap predicate would need to walk
// chain ancestry to read chain-level state; that primitive doesn't
// exist upstream.
//
// The Counter holds the chain context the rule engine doesn't.
// Splitting the work along this seam — Counter owns chain-aware
// decisions, rule action owns spawn shape (persona, tools, prompt
// template) — keeps each subsystem in its sweet spot. This is the
// same pattern chainpause uses (chainpause subscriber writes chain-
// level paused triples; operator HTTP handler consumes them) and
// ops-chain-observer follows.
//
// # The two-step rule fire
//
// Rule_02's third condition is `chain.recovery.proceed eq "true"`.
// When the reviewer's terminal triples land in the KV (graph-
// ingest writes coordinator.next_action=insufficient + other
// completion triples), the rule's entity_watcher fires and
// evaluates rule_02. The first eval fails the third condition
// (no proceed yet) and the rule does not fire. The Counter wakes
// on the same agent.complete event, does its chain-walk + cap
// check, and writes chain.recovery.proceed="true" to the reviewer
// entity (when within budget). The new KV write fires the entity
// watcher again; the rule re-evaluates; the third condition now
// passes; the rule fires.
//
// # Fail-safe semantics
//
// The cap "fails closed" — when the Counter cannot complete its
// chain walk (graph read failure, resolver error, NATS hiccup
// mid-write), no proceed sentinel lands and the rule never fires.
// The chain stalls at the most recent insufficient verdict. This
// matches the ADR-040 stance that curator routing is nice-to-have:
// a Counter hiccup costs at most one missed recovery cycle, never
// an unbounded retry storm.
//
// The chain entity audit (chain.recovery.count) is best-effort —
// when the chain write fails (the per-triple loop is fail-soft),
// the cycle still gates correctly via the proceed write or its
// absence. The audit is for operator visibility, not for the cap
// mechanism.
//
// # Predicate roles
//
//   - chain.recovery.count — on chain entity; per-cycle audit;
//     incremented monotonically. Observable via ops dashboards
//     and ops-agent diagnoses; the rule engine does NOT read it.
//   - chain.recovery.proceed — on reviewer loop entity; positive
//     gate sentinel; rule_02's third condition checks for it.
//     Stamped only when within budget. Per-cycle (each new
//     reviewer is a fresh entity).
//   - chain.recovery.exhausted — on chain entity; cap-hit marker
//     stamped when newCount > threshold. Operators + ops-agent
//     surface this to escalate stalled chains. Rule_02 does NOT
//     gate on it — the absence of proceed is what blocks the
//     fire.
//
// All three are 3-part names per the vocabulary system's
// domain.category.property convention; 2-part predicates would
// parseDomainCategory() with empty category (registry.go), failing
// downstream RDF export and hierarchy queries.
//
// # Distinct from neighbor predicate clusters
//
//   - chain.paused.* (ADR-037, FAILED loops with closed-enum
//     classifications) — failure semantics.
//   - chain.needs_review.* (ADR-039 Phase 1, builder
//     needs_clarification) — recoverable-pending semantics.
//   - chain.recovery.* (this package) — budget-bound retry
//     semantics.
//
// The three clusters share the chain entity as a subject but
// answer different operator questions.
package recoverycounter
