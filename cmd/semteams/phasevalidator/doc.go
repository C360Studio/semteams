// Package phasevalidator implements structural pre-filters that gate
// rule-engine role-transition fires when the upstream loop's terminal
// triples do not satisfy a contract enforced in Go before any LLM is
// invoked (ADR-041 §"Allowed transitions" + §"Per-phase cap" +
// §"Risks mitigation").
//
// Three handlers share infrastructure (chain.CompletionHandler interface,
// TriplePublisher, EntityTripleReader, chain.Resolver) and the
// recoverycounter idiom of "write a positive proceed sentinel onto the
// triggering loop entity when the contract holds; rely on absence-of-
// proceed to block the consuming rule when it does not":
//
//  1. PhaseValidator — researcher phase transitions. Reads the completed
//     researcher loop's role (carries the phase suffix —
//     researcher-{plan,gather,synthesize,architect}) and its
//     coordinator.next_action triple (the target phase token per ADR-041
//     §"Wire shape"). Validates the (input, target) edge against the
//     allowed-set in §"Allowed transitions" and the per-phase cap in
//     §"Per-phase cap" (plan=1, gather=3, synthesize=2, architect=2).
//     Approved: writes chain.phase_transition.proceed.<target>="true"
//     onto the researcher loop entity AND increments the per-phase
//     counter on the chain entity. Rejected (invalid edge OR cap hit):
//     writes chain.phase_transition.rejected="true" +
//     chain.phase_transition.reject_reason on the chain entity. action=
//     "emit" terminates the researcher arc and is approved structurally
//     (the existing rule 01a handles researcher→reviewer); the validator
//     stamps target="emit" for audit but no proceed sentinel.
//
//  2. SpecModeGate — reviewer-spec→builder transition. Reads the
//     completing reviewer-spec loop's coordinator.next_action ("approved"
//     to gate). Walks loop metadata for coordinator.evidence_loop_ids;
//     validates non-empty AND at least one referenced loop's role
//     matches a researcher (plain "researcher" or "researcher-architect"
//     under ADR-041's phase suffixes). Approved: writes
//     chain.spec_mode_gate.proceed="true" onto the reviewer-spec loop
//     entity. Rejected: writes chain.spec_mode_gate.rejected="true" +
//     chain.spec_mode_gate.reject_reason on the chain entity.
//
//  3. QAModeGate — builder→reviewer-qa transition. Reads the completing
//     builder loop's coordinator.next_action (must be "tests_passing")
//     and the builder_decide triples
//     (dev_via_spec.builder.tests_run > 0 AND
//     dev_via_spec.builder.tests_failed = 0). Approved: writes
//     chain.qa_mode_gate.proceed="true" onto the builder loop entity.
//     Rejected: writes chain.qa_mode_gate.rejected="true" +
//     chain.qa_mode_gate.reject_reason on the chain entity.
//
// # Why three handlers in one package
//
// All three implement the same chain.CompletionHandler contract on
// agent.complete events and write the same shape (proceed sentinel +
// audit/rejection triples). Spinning three packages would duplicate
// the shared infra and obscure the "structural pre-filter" pattern —
// keeping them co-located makes the architecture readable and the
// shared test scaffolding reusable. See [[project_adr041_mvp_role_compression]]
// for the slice context.
//
// # Fail-safe semantics (mirrors recoverycounter)
//
// Every handler "fails closed" — when the chain walk, entity read, or
// any precondition cannot be satisfied (graph blip, malformed triples,
// missing fields), no proceed sentinel lands and the rule never fires.
// The chain stalls at the most recent terminal. This matches the
// ADR-041 stance that structural gates are *additional* discipline:
// when the framework gate cannot run, the LLM-driven path stays gated
// (no implicit fallback to "allow"). Per-triple AddTriple errors are
// logged and the remaining writes proceed (partial cluster is
// queryable).
//
// # Predicate roles (subjects vary by handler)
//
//   - chain.researcher.phase_count.{plan,gather,synthesize,architect} —
//     chain entity. Per-phase audit; incremented monotonically. Operator
//     observable; rules MAY read.
//   - chain.phase_transition.proceed.{gather,synthesize,architect} —
//     researcher loop entity. Positive gate sentinel. Stamped only
//     when within budget AND edge is allowed.
//   - chain.phase_transition.target — chain entity. Last target token
//     evaluated (regardless of approve/reject outcome).
//   - chain.phase_transition.rejected — chain entity. Negative marker
//     when the validator structurally rejected.
//   - chain.phase_transition.reject_reason — chain entity. Short
//     classification token ("invalid_edge" | "phase_cap").
//
// SpecModeGate and QAModeGate use sibling vocabularies
// (chain.spec_mode_gate.{proceed,rejected,reject_reason} and
// chain.qa_mode_gate.{...}) registered in chain/predicates.go so the
// vocab catalog stays complete.
//
// # Wiring
//
// Each handler registers as a chain.CompletionHandler in
// cmd/semteams/main.go alongside recoverycounter.Counter,
// NeedsReviewStamper, and the other ADR-038 milestone stampers. The
// CompletionSubscriber dispatches each agent.complete event to every
// handler; each handler's filter (role match) short-circuits for
// non-target events.
package phasevalidator
