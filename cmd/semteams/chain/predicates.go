package chain

import "github.com/c360studio/semstreams/vocabulary"

// Exported chain-entity predicate names. These are the wire-format
// strings stampers WRITE and downstream consumers (emit-tools, future
// graph queries, ops agent) READ. Exported so consumers reference the
// constant rather than duplicating a string literal — a rename then
// becomes a compiler error at every read site.
//
// Registered with the upstream vocabulary package in init() per the
// SemStreams "domain.category.property" convention (see
// semstreams/vocabulary/README.md). Registration gives the chain
// vocabulary discoverability (vocabulary.ListRegisteredPredicates)
// and IRI mappings for RDF export.
//
// Coverage discipline: every chain.* predicate the codebase writes is
// registered here (2026-05-11 vocab-completion). Predicates whose
// canonical constants live in this file (PredicateSlugStem,
// PredicateResearchArtifactLoop, PredicateRecovery*,
// PredicateNeedsReview*) Register against the constant. Predicates whose
// constants stay co-located with their writers (chain.dispatched.*,
// chain.paused.*, chain.decision.*, chain.evidence.*,
// chain.research_artifact.{harness,actor_count,task_count,path},
// chain.spec_artifact.*) Register against string literals — promoting
// them to constants here is a deferred follow-up refactor. The
// PredicatePlanLoop + PredicatePlanReviewerLoop constants stay
// registered (read-side legacy fallback consumed by
// emit_dev_via_spec_artifact's overrideProvenanceFromChain), but no
// MVP writer stamps them — see their doc-comments below for the
// retention rationale. The contract test
// test/contract/chain_entity_coverage_test.go pins the predicate-name
// union both sides must agree on regardless.
//
// All registered predicates use 3-part (or deeper) names per the
// vocabulary system's domain.category.property convention. 2-part
// predicates parseDomainCategory() with empty category in
// registry.go — fixed in the same 2026-05-11 vocab-completion pass
// (renames: chain.research_artifact_loop → chain.research_artifact.loop;
// chain.plan_loop → chain.plan.loop;
// chain.plan_reviewer_loop → chain.plan.reviewer_loop;
// chain.consensus_loop → chain.consensus.loop (subsequently removed in ADR-041 Slice 2D-4 alongside the consensus stamper teardown);
// chain.spec_artifact_loop → chain.spec_artifact.loop;
// chain.dispatched_at → chain.dispatched.at;
// chain.resumed → chain.decision.resumed_task_id;
// chain.killed → chain.decision.killed_at;
// chain.deferred → chain.decision.deferred_at). Wire-format break,
// but SemTeams is a reference/demo product with no long-lived
// production graph data to migrate.
const (
	// PredicateSlugStem names the chain-stable slug stem extracted from
	// the researcher's rendered markdown path at first emit. Downstream
	// emit-tools compose "<stem>-plan" / "<stem>-consensus" /
	// "<stem>-implementation" so the slug stays consistent across the
	// chain's markdown artifacts (smoke #8 run-5 D2 fix).
	PredicateSlugStem = "chain.slug.stem"

	// PredicateResearchArtifactLoop names the chain-entity predicate
	// that records the loop_id of the researcher whose artifact the
	// research-reviewer approved. Read by emit_plan and
	// emit_dev_via_spec_artifact to populate
	// "depends_on.research_artifact_loop" /
	// "provenance.research_artifact_loop" without trusting the LLM's
	// local guess.
	PredicateResearchArtifactLoop = "chain.research_artifact.loop"

	// PredicatePlanLoop names the chain-entity predicate that recorded
	// the planner loop_id whose plan the reviewer approved. Stamper +
	// vocabulary registration retired in ADR-041 Phase 3; the constant
	// stays only because emit_dev_via_spec_artifact's
	// overrideProvenanceFromChain reads it as a fallback (returns empty
	// under MVP — nothing stamps it). Removable once
	// overrideProvenanceFromChain drops its PlannerLoop branch.
	PredicatePlanLoop = "chain.plan.loop"

	// PredicatePlanReviewerLoop names the chain-entity predicate that
	// recorded the reviewer loop_id that approved the plan. Same
	// retirement status as PredicatePlanLoop — constant kept for
	// emit_dev_via_spec_artifact's overrideProvenanceFromChain
	// ReviewerLoop branch.
	PredicatePlanReviewerLoop = "chain.plan.reviewer_loop"

	// PredicateNeedsReviewClassification is the first predicate of the
	// chain.needs_review.* cluster (ADR-039 Phase 1 Tier 3). The full
	// cluster is stamped by NeedsReviewStamper when a
	// needs_clarification terminal lands with no Tier 1 rule match and
	// no Tier 2 coordinator configured — the cluster represents
	// "awaiting a recovery decision" and the consumer is
	// deployment-configured (coordinator agent, operator dashboard,
	// metric-emit job). Distinct from chain.paused.* (ADR-037, FAILED
	// loops with closed-enum classifications). Phase 1 only stamps for
	// builder; broader producer coverage is a follow-up.
	//
	// This predicate itself is an open-valued tag describing why this
	// needs_clarification reached Tier 3. Phase 1 writes
	// "unrouted_needs_clarification" (catch-all). Future Tier 2
	// coordinator integration may write "coordinator_declined" or
	// finer reason-pattern tags (e.g. "catalog_gap",
	// "external_dependency").
	PredicateNeedsReviewClassification = "chain.needs_review.classification"

	// PredicateNeedsReviewProducerLoopID is the loop_id of the producer
	// that emitted the original needs_clarification terminal.
	PredicateNeedsReviewProducerLoopID = "chain.needs_review.producer_loop_id"

	// PredicateNeedsReviewProducerRole is the role name of the producer
	// loop (builder for Phase 1; broader when Slice C lands).
	PredicateNeedsReviewProducerRole = "chain.needs_review.producer_role"

	// PredicateNeedsReviewReason is the producer's
	// coordinator.decision_reason verbatim — short natural-language
	// justification the producer supplied. Length-bounded by the
	// upstream agentic.CoordinatorDecisionReason contract; not
	// re-truncated here.
	PredicateNeedsReviewReason = "chain.needs_review.reason"

	// PredicateNeedsReviewObservedAt is the RFC3339 timestamp of when
	// the stamper wrote the cluster. Distinct from the producer loop's
	// CompletedAt; observability surfaces use this to age the cluster.
	PredicateNeedsReviewObservedAt = "chain.needs_review.observed_at"

	// PredicateRecoveryCount is the per-chain count of reviewer
	// rejection cycles that have triggered a retry-research attempt
	// (ADR-040 §addendum 2026-05-11 "Chain-level recovery cap"; ADR-041
	// extended to cover MVP reviewer-research / reviewer-spec alongside
	// legacy research-reviewer). Stamped on the canonical 6-part chain
	// entity by RecoveryCounter each time a managed reviewer completes
	// with coordinator.next_action="insufficient". Also mirrored onto
	// the triggering reviewer loop entity so rule_02 can gate via
	// $entity.triple.chain.recovery_count without a chain-ancestry walk
	// (the rule engine reads triples on the triggering entity only).
	//
	// Stored as a string-formatted integer (Object: "1", "2", …) so the
	// rule engine's expression operators (eq/lte/gt) compare cleanly
	// against string-typed values from the rules JSON.
	PredicateRecoveryCount = "chain.recovery.count"

	// PredicateRecoveryExhausted is the chain-level marker stamped by
	// RecoveryCounter once chain.recovery_count exceeds the configured
	// threshold (default 3). Stamped on the chain entity (audit trail)
	// + the triggering reviewer loop entity (operator-visible). Object
	// value is the string "true". Distinct from PredicateRecoveryProceed:
	// exhausted is the *negative* signal ("this chain spent its budget,
	// no further curator spawns") that operator dashboards and the
	// ops-agent consume to surface stalled chains. The rule does NOT
	// gate on this predicate directly — the absence of
	// chain.recovery_proceed is what blocks the rule fire (see
	// PredicateRecoveryProceed). Both are stamped together when the cap
	// fires so consumers can distinguish "chain hit cap" (exhausted=true)
	// from "Counter is failing-soft" (no proceed AND no exhausted).
	//
	// Distinct from chain.paused.* (ADR-037 — failure semantics) and
	// chain.needs_review.* (ADR-039 Phase 1 — recoverable-pending
	// semantics). recovery_exhausted is "this chain has spent its
	// recovery budget"; downstream consumers (operator dashboard,
	// future coordinator agent) decide whether to escalate, abandon
	// the chain, or extend the cap by clearing the triple manually.
	PredicateRecoveryExhausted = "chain.recovery.exhausted"

	// PredicateRecoveryProceed is the per-cycle gate sentinel
	// RecoveryCounter writes onto the triggering reviewer's loop
	// entity when the chain has remaining recovery budget. Rule_02
	// gates on `chain.recovery_proceed eq "true"`, so curator spawns
	// only happen when the Counter has explicitly approved the cycle.
	//
	// The two-step pattern (rule's coordinator.next_action=insufficient
	// → rule's first eval fails because no proceed → Counter wakes on
	// the same agent.complete event, does the chain-walk + cap check,
	// writes proceed → KV update fires entity_watcher → rule re-evals
	// → fires) splits chain-aware logic (Counter) from spawn-shape
	// logic (rule action). Each subsystem owns the work it's best at.
	//
	// Object value is the string "true". Stamped only on the reviewer
	// loop entity (NOT the chain entity) — the gate is per-cycle and
	// the chain entity already carries chain.recovery_count for audit.
	// When the Counter fails (graph blip during the chain-walk), no
	// proceed lands; the rule never fires; the chain stalls fail-safe
	// at the most recent insufficient verdict. This matches the
	// ADR-040 stance that curator routing is nice-to-have: a Counter
	// hiccup costs at most one missed recovery cycle, never an
	// unbounded retry storm.
	PredicateRecoveryProceed = "chain.recovery.proceed"

	// PredicatePhaseCountPlan / Gather / Synthesize / Architect are the
	// per-phase researcher counters incremented when a phase-transition
	// validator approves a transition into that phase (ADR-041
	// §"Per-phase cap"). Stored as string-formatted integers ("0", "1",
	// …) on the chain entity so the rule engine's expression operators
	// (eq/lte/gt) compare cleanly. Each is a distinct 4-part predicate
	// rather than a structured object so rule-condition syntax can read
	// the value directly without JSON traversal.
	//
	// Cap semantics: plan=1, gather=3, synthesize=2, architect=2. When
	// the validator detects a target phase already at cap, no proceed
	// sentinel is written and the chain stalls fail-safe (same shape as
	// PredicateRecoveryProceed absence). The phase counter audit triple
	// always lands so operators see "this chain hit phase X cap" even
	// when no consuming rule fires.
	PredicatePhaseCountPlan       = "chain.researcher.phase_count.plan"
	PredicatePhaseCountGather     = "chain.researcher.phase_count.gather"
	PredicatePhaseCountSynthesize = "chain.researcher.phase_count.synthesize"
	PredicatePhaseCountArchitect  = "chain.researcher.phase_count.architect"

	// PredicatePhaseTransitionProceedGather / Synthesize / Architect are
	// per-cycle gate sentinels stamped on the completed researcher's
	// loop entity when the phase validator (cmd/semteams/phasevalidator)
	// approves a transition into the named target phase. The next-phase
	// researcher spawn rule (Slice 2D-3) gates on `chain.phase_transition
	// .proceed.<target> eq "true"`. Absence blocks the rule fire — same
	// fail-safe shape as PredicateRecoveryProceed.
	//
	// No `proceed.emit` sentinel exists because action="emit" terminates
	// the researcher arc and hands off to the research-reviewer via the
	// existing rule 01a, which has its own conditions. The phase
	// validator stamps `chain.phase_transition.target = "emit"` on the
	// chain entity for audit when a researcher emits but does not write
	// a proceed sentinel for it.
	//
	// Object value is the string "true". Stamped only on the completed
	// researcher's loop entity (not the chain entity) — the gate is
	// per-cycle. The chain entity carries phase_count + transition_target
	// for audit.
	PredicatePhaseTransitionProceedGather     = "chain.phase_transition.proceed.gather"
	PredicatePhaseTransitionProceedSynthesize = "chain.phase_transition.proceed.synthesize"
	PredicatePhaseTransitionProceedArchitect  = "chain.phase_transition.proceed.architect"

	// PredicatePhaseTransitionTarget is the last target-phase token the
	// phase validator evaluated for this chain. Stamped on the chain
	// entity for operator visibility regardless of whether the validator
	// approved or rejected (the rejected case has no proceed sentinel,
	// so target alone tells operators what the researcher tried to do).
	// Object value is the bare action token ("gather" / "synthesize" /
	// "architect" / "emit").
	PredicatePhaseTransitionTarget = "chain.phase_transition.target"

	// PredicatePhaseTransitionRejected marks the chain entity when the
	// phase validator structurally rejected a transition (invalid edge
	// OR target phase at cap). Object value "true". The chain stalls
	// because no proceed sentinel was written; this triple is the
	// operator-visible cause, parallel to PredicateRecoveryExhausted.
	// Distinct from chain.paused.* (ADR-037 — semstreams-level loop
	// failures) and chain.needs_review.* (ADR-039 Phase 1 —
	// recoverable-pending semantics).
	PredicatePhaseTransitionRejected = "chain.phase_transition.rejected"

	// PredicatePhaseTransitionRejectReason carries a short token
	// classifying the rejection: "invalid_edge" (target phase not
	// allowed from input phase) or "phase_cap" (target phase already at
	// cap). Ops-agent groups stalls by this token; persona content
	// stays narrative ("X happened because…"), this stays vocabulary.
	PredicatePhaseTransitionRejectReason = "chain.phase_transition.reject_reason"

	// PredicateSpecModeGateProceed is the per-cycle gate sentinel
	// SpecModeGate writes onto a completing reviewer-spec loop entity
	// when the chain has a research artifact upstream (ADR-041
	// §"Risks mitigation" spec-mode pre-check). Absence blocks the
	// reviewer-spec→builder spawn rule from firing.
	PredicateSpecModeGateProceed = "chain.spec_mode_gate.proceed"

	// PredicateSpecModeGateRejected marks the chain entity when the
	// spec-mode gate rejected an approval (no research-artifact
	// ancestor). Object value "true".
	PredicateSpecModeGateRejected = "chain.spec_mode_gate.rejected"

	// PredicateSpecModeGateRejectReason classifies a spec-mode gate
	// rejection. Currently the only token is "missing_research_artifact"
	// (no chain.research_artifact.loop on the chain entity).
	PredicateSpecModeGateRejectReason = "chain.spec_mode_gate.reject_reason"

	// PredicateQAModeGateProceed is the per-cycle gate sentinel
	// QAModeGate writes onto a completing builder loop entity when the
	// builder's claim of "tests_passing" is structurally consistent
	// (tests_run > 0 AND tests_failed = 0). Absence blocks the
	// builder→reviewer-qa spawn rule from firing.
	PredicateQAModeGateProceed = "chain.qa_mode_gate.proceed"

	// PredicateQAModeGateRejected marks the chain entity when the
	// qa-mode gate rejected a builder claim (tests_run=0 or
	// tests_failed>0). Object value "true".
	PredicateQAModeGateRejected = "chain.qa_mode_gate.rejected"

	// PredicateQAModeGateRejectReason classifies a qa-mode gate
	// rejection: "zero_tests_run" (action=tests_passing with
	// tests_run=0) or "tests_failed_nonzero" (action=tests_passing
	// with tests_failed>0). Ops-agent groups stalls by this token.
	PredicateQAModeGateRejectReason = "chain.qa_mode_gate.reject_reason"

	// PredicateChainMode is the closed-token classification a chain
	// carries from coordinator-dispatch onwards. Stamped by
	// cmd/semteams/chainmode.Stamper on the canonical 6-part chain
	// entity at coordinator-terminal time. Read by the phase validator
	// (cmd/semteams/phasevalidator) to gate the synthesize→architect
	// transition: chain.mode.classification == "research_only" refuses
	// the proceed sentinel; chain.mode.classification == "dev_via_spec"
	// allows it.
	//
	// Object values:
	//   - "research_only" — set on coordinator
	//     decide(action=delegate_research). Synthesize must terminate
	//     at reviewer-research; no architect / spec / builder phases.
	//   - "dev_via_spec"  — set on coordinator
	//     decide(action=delegate_dev_chain). Full plan → gather →
	//     synthesize → architect → reviewer-spec → builder →
	//     reviewer-qa arc.
	//
	// Slice 1b of the coordinator redesign — the structural authority
	// the validator gates against. Persona-only fork at synthesize
	// proved unreliable under real LLM (Slice 1 smoke evidence: 2 of 3
	// synthesize loops chose decide(action=architect) regardless of
	// the spawn prompt's framing).
	//
	// 3-part canonical shape `chain.mode.classification` parallels
	// `chain.needs_review.classification` (ADR-039 Phase 1 Tier 3) and
	// satisfies the vocabulary system's domain.category.property
	// convention. A 2-part `chain.mode` would parseDomainCategory()
	// with empty category (see this file's package docstring and the
	// 2026-05-11 vocab-completion pass that renamed every 2-part
	// chain.* predicate).
	PredicateChainMode = "chain.mode.classification"

	// PredicateChainTerminalReached marks the chain entity when the
	// chain has reached its expected end-of-arc. Stamped by
	// chain.TerminalStamper on reviewer-research(approved) or
	// reviewer-qa(accept). Object value "true". Distinct from
	// chain.paused.* (ADR-037 — FAILED loops), chain.needs_review.*
	// (ADR-039 Phase 1 — recoverable-pending), and chain.recovery.*
	// (ADR-040 — per-cycle retry budget). Coordinator-redesign Slice 1c
	// — the structural authority coordinator wake-up rules read from
	// the chain entity for audit; the rules themselves fire on the
	// reviewer's loop-entity triples (role + decision.next_action)
	// because the rule engine's firing entity is the loop, not the
	// chain.
	PredicateChainTerminalReached = "chain.terminal.reached"

	// PredicateChainTerminalRole records the role of the terminating
	// reviewer ("reviewer-research" or "reviewer-qa"). Operator-visible
	// classification for ops queries; rule wiring uses role directly.
	PredicateChainTerminalRole = "chain.terminal.role"

	// PredicateChainTerminalLoopID is the loop_id of the terminating
	// reviewer. Wake-up rules pass this via related_loops so the
	// spawned coordinator can read_loop_result on it. Audit predicate
	// on the chain entity.
	PredicateChainTerminalLoopID = "chain.terminal.loop_id"

	// PredicateChainTerminalObservedAt is the RFC3339 timestamp of when
	// the stamper wrote the cluster. Distinct from the reviewer loop's
	// CompletedAt; observability surfaces use this to age the cluster.
	PredicateChainTerminalObservedAt = "chain.terminal.observed_at"

	// chain.rejection.* — coordinator-redesign Slice 2. Per-chain cap
	// counter parallel to chain.recovery.* (ADR-040): chainstall
	// increments chain.rejection.recovery_count on every structural-gate
	// stall it observes and stamps chain.rejection.exhausted when the
	// count exceeds the configured threshold. Distinct from
	// chain.recovery.* because the recovery counter only tracks
	// research-reviewer / reviewer-spec INSUFFICIENT terminals, whereas
	// chain.rejection.* aggregates every reject-reason class
	// (mode_mismatch, phase_cap, missing_research_artifact, etc.) under
	// one chain-level cap. The shared "cap-hit → always-human" semantics
	// is enforced by chainstall regardless of per-reason default route.

	// PredicateChainRejectionRecoveryCount is the chain-level count of
	// stall-recovery cycles. Incremented monotonically by
	// cmd/semteams/chainstall on every rejection. String-formatted int so
	// rule-engine string operators compare cleanly without coercion.
	PredicateChainRejectionRecoveryCount = "chain.rejection.recovery_count"

	// PredicateChainRejectionExhausted is the chain-level cap-hit marker
	// stamped on the chain entity when recovery_count exceeds the
	// configured threshold. Object value "true". Operator-visible
	// escalation signal; chainstall always routes a cap-hit rejection to
	// the human-approval path regardless of the per-reason default.
	PredicateChainRejectionExhausted = "chain.rejection.exhausted"

	// chain.stall.* — coordinator-redesign Slice 2. Audit cluster
	// chainstall writes when it observes a structural-gate rejection on
	// a chain that has not reached chain.terminal.reached. Distinct from
	// chain.paused.* (ADR-037 — FAILED loops), chain.needs_review.*
	// (ADR-039 Phase 1 — recoverable-pending verdicts),
	// chain.terminal.* (Slice 1c — end-of-arc), chain.recovery.* (ADR-040
	// — research-reviewer retry budget), and chain.rejection.* (this
	// slice's cap counter). The stall cluster captures the per-cycle
	// "who rejected, why, what route" record so operators can audit the
	// chain's recovery decisions without re-deriving them from the gate's
	// reject-reason triples.

	// PredicateChainStallRoutedTo classifies the recovery route chainstall
	// chose for the rejection: "coordinator" (framing-fixable: persona
	// drift, weak spawn prompt) or "human" (budget exhaustion, structural
	// impossibility, or cap-hit escalation). The per-reason routing table
	// in cmd/semteams/chainstall lives as the single source of truth.
	PredicateChainStallRoutedTo = "chain.stall.routed_to"

	// PredicateChainStallReason copies the structural gate's reject_reason
	// token onto the chain.stall.* audit cluster so operators querying by
	// stall predicate don't have to join against
	// chain.phase_transition.reject_reason / chain.spec_mode_gate.reject_reason /
	// chain.qa_mode_gate.reject_reason / chain.needs_review.classification.
	PredicateChainStallReason = "chain.stall.reason"

	// PredicateChainStallProducerLoopID is the loop_id of the role whose
	// completion triggered the rejection (synthesize loop for
	// mode_mismatch; reviewer-spec loop for missing_research_artifact;
	// builder loop for zero_tests_run / tests_failed_nonzero /
	// unrouted_needs_clarification; reviewer-research / reviewer-spec
	// loop for chain.recovery.exhausted). Wake-up rules pass this via
	// related_loops so the spawned coordinator can read_loop_result on
	// the producer.
	PredicateChainStallProducerLoopID = "chain.stall.producer_loop_id"

	// PredicateChainStallProducerRole is the role name of the producer
	// loop. Audit predicate for ops queries; wake-up rule wiring uses the
	// related_loops handle directly.
	PredicateChainStallProducerRole = "chain.stall.producer_role"

	// PredicateChainStallObservedAt is the RFC3339 timestamp of when
	// chainstall wrote the cluster. Distinct from the producer loop's
	// CompletedAt; observability surfaces use this to age the cluster.
	PredicateChainStallObservedAt = "chain.stall.observed_at"

	// PredicateChainStallAwaitingHuman is the chain-level marker stamped
	// when chainstall routes a rejection to the human-approval path.
	// Object value "true". The HTTP endpoint at
	// /teams-loop/chain-stall/decide (deferred to a follow-up commit per
	// Slice 1c precedent) clears the marker via the operator's verdict.
	PredicateChainStallAwaitingHuman = "chain.stall.awaiting_human"

	// PredicateChainStallProceed is the per-cycle gate sentinel chainstall
	// writes onto the producer loop entity when it routes a rejection to
	// the coordinator wake-up path. Object value "true". The wake-up rule
	// (configs/rules/coordinator/05-chain-stall-to-coordinator.json) fires
	// only when this predicate is "true" on the producer loop — same
	// fail-safe semantics as PredicateRecoveryProceed
	// (ADR-040): chainstall failure to stamp leaves no proceed → rule
	// never fires → chain stalls until human inspection. Distinct from
	// PredicateChainStallRoutedTo (chain-entity audit value): proceed is
	// on the loop entity, routed_to is on the chain entity for ops
	// queries.
	PredicateChainStallProceed = "chain.stall.proceed"
)

// init registers the chain vocabulary with the upstream vocabulary
// registry. Auto-runs at first import of this package; semteams' main
// imports cmd/semteams/chain via the wiring path so this fires once
// at process start.
//
// Naming: predicates use 3-level (or deeper) dotted notation per the
// vocabulary system convention (domain.category.property). 2-level
// names parseDomainCategory() with empty category in registry.go,
// breaking RDF export and hierarchy queries — the 2026-05-11 vocab-
// completion pass renamed every 2-part chain.* predicate to 3-part
// (see the migration log in this file's package docstring).
//
// DataType values match the on-the-wire shape, not the Go field type
// (e.g. counts arrive as float64 from JSON unmarshal, but their
// vocabulary type is "int" — what consumers should treat them as).
func init() {
	vocabulary.Register(PredicateSlugStem,
		vocabulary.WithDescription("Chain-stable slug stem derived from the researcher's rendered markdown path. Downstream emit-tools compose <stem>-{plan,consensus,implementation} for their own filenames so the slug stays consistent across the chain."),
		vocabulary.WithDataType("string"),
	)

	vocabulary.Register(PredicateResearchArtifactLoop,
		vocabulary.WithDescription("Loop_id of the researcher whose artifact the reviewer (research mode, or legacy research-reviewer) approved. The chain entity's canonical reference to the upstream research arc."),
		vocabulary.WithDataType("string"),
	)

	// chain.plan.* — stamper retired in ADR-041 Phase 3 (no MVP rule
	// spawns the dev-via-spec-reviewer loop the stamper keyed on). The
	// vocabulary entries stay registered because the constants are still
	// imported by emit_dev_via_spec_artifact's overrideProvenanceFromChain
	// (read-side fallback); the registry exists for vocab-completeness
	// audits even though no live producer writes these triples under MVP.
	vocabulary.Register(PredicatePlanLoop,
		vocabulary.WithDescription("Loop_id of the planner whose plan the reviewer approved. ADR-041 Phase 3: stamper retired — under MVP roster the planner role is gone (researcher-plan emits structured loop output, not a markdown artifact). Constant retained for emit_dev_via_spec_artifact's read-side fallback only."),
		vocabulary.WithDataType("string"),
	)

	vocabulary.Register(PredicatePlanReviewerLoop,
		vocabulary.WithDescription("Loop_id of the reviewer that approved the plan. ADR-041 Phase 3: stamper retired — same status as PredicatePlanLoop."),
		vocabulary.WithDataType("string"),
	)

	vocabulary.Register(PredicateNeedsReviewClassification,
		vocabulary.WithDescription("ADR-039 Phase 1 Tier 3 cluster: open-valued tag describing why a needs_clarification terminal reached Tier 3 (no Tier 1 rule match, no Tier 2 coordinator configured). Phase 1 default: \"unrouted_needs_clarification\"."),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register(PredicateNeedsReviewProducerLoopID,
		vocabulary.WithDescription("ADR-039 Phase 1 Tier 3 cluster: loop_id of the producer that emitted the original needs_clarification terminal."),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register(PredicateNeedsReviewProducerRole,
		vocabulary.WithDescription("ADR-039 Phase 1 Tier 3 cluster: role name of the producer loop (builder for Phase 1; broader producer coverage in follow-up slices)."),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register(PredicateNeedsReviewReason,
		vocabulary.WithDescription("ADR-039 Phase 1 Tier 3 cluster: producer's coordinator.decision_reason verbatim — short natural-language justification the producer supplied alongside its decide(action=needs_clarification) call."),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register(PredicateNeedsReviewObservedAt,
		vocabulary.WithDescription("ADR-039 Phase 1 Tier 3 cluster: RFC3339 timestamp of when NeedsReviewStamper wrote the cluster. Distinct from the producer loop's CompletedAt — observability surfaces use this to age the cluster."),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register(PredicateRecoveryCount,
		vocabulary.WithDescription("ADR-040 §addendum 2026-05-11: per-chain count of reviewer rejection cycles (legacy `research-reviewer` and ADR-041 MVP `reviewer-research` / `reviewer-spec`) that triggered a retry-research recovery attempt. Stored as string-formatted integer for rule-engine string comparisons. Stamped on chain entity (audit trail) and mirrored onto the triggering reviewer loop entity (rule-engine read surface)."),
		vocabulary.WithDataType("int"),
	)
	vocabulary.Register(PredicateRecoveryExhausted,
		vocabulary.WithDescription("ADR-040 §addendum 2026-05-11: chain-level cap-hit marker (\"true\") stamped on the chain entity once chain.recovery_count exceeds the configured threshold. Operator dashboards + ops-agent consume this to surface stalled chains. The rule's gate is chain.recovery_proceed (positive sentinel), not this predicate (negative marker)."),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register(PredicateRecoveryProceed,
		vocabulary.WithDescription("ADR-040 §addendum 2026-05-11: per-cycle gate sentinel stamped on the triggering reviewer's loop entity when the chain has remaining recovery budget. Rule_02 fires only when this predicate is \"true\". Counter failure (graph blip mid chain-walk) → no proceed → chain stalls fail-safe."),
		vocabulary.WithDataType("string"),
	)

	// ADR-041 predicates (phase validator + spec/qa gates) live in a
	// helper to keep init() under revive's function-length limit.
	registerADR041Predicates()
}

// registerADR041Predicates registers the chain.phase_transition.*,
// chain.researcher.phase_count.*, chain.spec_mode_gate.*, and
// chain.qa_mode_gate.* predicates introduced by ADR-041 (MVP role
// compression + structural gates). Split out of init() to keep that
// function under revive's function-length threshold; semantically a
// continuation of init's central-registration block.
func registerADR041Predicates() {
	// Per-phase counters on chain entity. Stored as string-formatted
	// integers so rule-engine string operators compare cleanly; the
	// validator (cmd/semteams/phasevalidator) is the sole writer.
	vocabulary.Register(PredicatePhaseCountPlan,
		vocabulary.WithDescription("ADR-041 §Per-phase cap: per-chain count of researcher 'plan' phase fires. Cap = 1. Stored as string-formatted integer for rule-engine string comparisons."),
		vocabulary.WithDataType("int"),
	)
	vocabulary.Register(PredicatePhaseCountGather,
		vocabulary.WithDescription("ADR-041 §Per-phase cap: per-chain count of researcher 'gather' phase fires. Cap = 3 (initial + 2 back-edge re-gathers from synthesize/architect)."),
		vocabulary.WithDataType("int"),
	)
	vocabulary.Register(PredicatePhaseCountSynthesize,
		vocabulary.WithDescription("ADR-041 §Per-phase cap: per-chain count of researcher 'synthesize' phase fires. Cap = 2 (initial + one revision after back-edge re-gather)."),
		vocabulary.WithDataType("int"),
	)
	vocabulary.Register(PredicatePhaseCountArchitect,
		vocabulary.WithDescription("ADR-041 §Per-phase cap: per-chain count of researcher 'architect' phase fires. Cap = 2 (initial + one revision after back-edge re-gather)."),
		vocabulary.WithDataType("int"),
	)
	vocabulary.Register(PredicatePhaseTransitionProceedGather,
		vocabulary.WithDescription("ADR-041 §Wire shape: per-cycle gate sentinel stamped on the completed researcher's loop entity when the phase validator approves transition into 'gather'. Next-phase spawn rule fires only when this is \"true\"."),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register(PredicatePhaseTransitionProceedSynthesize,
		vocabulary.WithDescription("ADR-041 §Wire shape: per-cycle gate sentinel for transition into 'synthesize'. See PredicatePhaseTransitionProceedGather."),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register(PredicatePhaseTransitionProceedArchitect,
		vocabulary.WithDescription("ADR-041 §Wire shape: per-cycle gate sentinel for transition into 'architect'. See PredicatePhaseTransitionProceedGather."),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register(PredicatePhaseTransitionTarget,
		vocabulary.WithDescription("ADR-041: bare action token the phase validator last evaluated for this chain ('gather'/'synthesize'/'architect'/'emit'). Operator-visible regardless of approve-or-reject outcome."),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register(PredicatePhaseTransitionRejected,
		vocabulary.WithDescription("ADR-041: chain-entity marker (\"true\") stamped when the phase validator structurally rejected a transition. Parallel to PredicateRecoveryExhausted for cap-shape stalls."),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register(PredicatePhaseTransitionRejectReason,
		vocabulary.WithDescription("ADR-041: short classification token for a phase-transition rejection ('invalid_edge' | 'phase_cap'). Ops-agent groups stalls by this token."),
		vocabulary.WithDataType("string"),
	)

	// ADR-041 §"Risks mitigation" structural pre-checks at role
	// boundaries. SpecModeGate gates reviewer-spec→builder; QAModeGate
	// gates builder→reviewer-qa. Both follow the proceed-sentinel +
	// chain-entity-marker pattern of recoverycounter + phase_transition.
	vocabulary.Register(PredicateSpecModeGateProceed,
		vocabulary.WithDescription("ADR-041 §Risks mitigation: per-cycle gate sentinel stamped on the reviewer-spec loop entity when the chain carries a chain.research_artifact.loop (i.e., the spec is grounded in upstream research). Reviewer-spec→builder spawn rule fires only when this is \"true\"."),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register(PredicateSpecModeGateRejected,
		vocabulary.WithDescription("ADR-041 §Risks mitigation: chain-entity marker (\"true\") stamped when SpecModeGate rejects a reviewer-spec approval (no research-artifact ancestor)."),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register(PredicateSpecModeGateRejectReason,
		vocabulary.WithDescription("ADR-041 §Risks mitigation: classification token for spec-mode-gate rejections ('missing_research_artifact')."),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register(PredicateQAModeGateProceed,
		vocabulary.WithDescription("ADR-041 §Risks mitigation: per-cycle gate sentinel stamped on the builder loop entity when the builder's tests_passing claim is structurally consistent (tests_run>0 AND tests_failed=0). Builder→reviewer-qa spawn rule fires only when this is \"true\"."),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register(PredicateQAModeGateRejected,
		vocabulary.WithDescription("ADR-041 §Risks mitigation: chain-entity marker (\"true\") stamped when QAModeGate rejects a builder claim (tests_run=0 or tests_failed>0 with action=tests_passing)."),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register(PredicateQAModeGateRejectReason,
		vocabulary.WithDescription("ADR-041 §Risks mitigation: classification token for qa-mode-gate rejections ('zero_tests_run' | 'tests_failed_nonzero')."),
		vocabulary.WithDataType("string"),
	)

	// chain.mode — coordinator-redesign Slice 1b. Closed-token
	// classification stamped by cmd/semteams/chainmode.Stamper at
	// coordinator-terminal time; read by phase validator to gate
	// synthesize→architect.
	vocabulary.Register(PredicateChainMode,
		vocabulary.WithDescription("Coordinator-redesign Slice 1b: closed-token chain classification (chain.mode.classification) stamped on the canonical 6-part chain entity at coordinator-terminal time. Object values: 'research_only' (coordinator decide(action=delegate_research) — synthesize must terminate at reviewer-research) | 'dev_via_spec' (coordinator decide(action=delegate_dev_chain) — synthesize→architect allowed, full dev arc continues)."),
		vocabulary.WithDataType("string"),
	)

	// chain.terminal.* — coordinator-redesign Slice 1c. TerminalStamper
	// audit cluster stamped on the chain entity when a terminating-reviewer
	// loop completes with its role-specific approval action. Distinct from
	// chain.paused.* (ADR-037 failures), chain.needs_review.* (ADR-039
	// Phase 1 recoverable-pending), chain.recovery.* (ADR-040 retry budget).
	vocabulary.Register(PredicateChainTerminalReached,
		vocabulary.WithDescription("Coordinator-redesign Slice 1c: chain-entity marker (\"true\") stamped when a terminating-reviewer loop (reviewer-research/approved or reviewer-qa/accept) completes the chain. Distinct from chain.paused.* (FAILED loops), chain.needs_review.* (recoverable-pending), chain.recovery.* (retry budget) — this is the end-of-arc 'user request answered' signal that coordinator wake-up rules read."),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register(PredicateChainTerminalRole,
		vocabulary.WithDescription("Coordinator-redesign Slice 1c: role of the terminating reviewer ('reviewer-research' for research-only chains, 'reviewer-qa' for dev-via-spec chains). Audit predicate for ops queries; rule wiring uses role directly off the loop entity."),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register(PredicateChainTerminalLoopID,
		vocabulary.WithDescription("Coordinator-redesign Slice 1c: loop_id of the terminating reviewer. Wake-up rules pass this via related_loops so the spawned coordinator can read_loop_result on the terminal."),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register(PredicateChainTerminalObservedAt,
		vocabulary.WithDescription("Coordinator-redesign Slice 1c: RFC3339 timestamp when TerminalStamper wrote the cluster. Distinct from the reviewer loop's CompletedAt — observability surfaces use this to age the cluster."),
		vocabulary.WithDataType("string"),
	)

	registerCoordinatorSlice2Predicates()
	registerOtherPackagePredicates()
}

// registerCoordinatorSlice2Predicates registers the chain.rejection.* +
// chain.stall.* predicates introduced by coordinator-redesign Slice 2
// (cmd/semteams/chainstall). Split out of registerADR041Predicates to
// keep functions under revive's length threshold; semantically a
// continuation of init's central-registration block.
func registerCoordinatorSlice2Predicates() {
	vocabulary.Register(PredicateChainRejectionRecoveryCount,
		vocabulary.WithDescription("Coordinator-redesign Slice 2: per-chain count of structural-gate rejection cycles aggregated across all reject-reason classes (mode_mismatch, phase_cap, missing_research_artifact, zero_tests_run, tests_failed_nonzero, unrouted_needs_clarification, chain.recovery.exhausted, invalid_edge). Stored as string-formatted int for rule-engine string comparisons. Parallel to chain.recovery.count (ADR-040) but with broader aggregation scope."),
		vocabulary.WithDataType("int"),
	)
	vocabulary.Register(PredicateChainRejectionExhausted,
		vocabulary.WithDescription("Coordinator-redesign Slice 2: chain-level cap-hit marker (\"true\") stamped when chain.rejection.recovery_count exceeds the configured threshold. Always routes the rejection to the human-approval path regardless of per-reason default. Parallel to chain.recovery.exhausted (ADR-040)."),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register(PredicateChainStallRoutedTo,
		vocabulary.WithDescription("Coordinator-redesign Slice 2: routing decision chainstall made for the rejection. Closed-token values: \"coordinator\" (framing-fixable: persona drift, weak spawn prompt — LLM retry has a real chance) | \"human\" (budget exhaustion, structural impossibility, or cap-hit — more LLM calls won't help)."),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register(PredicateChainStallReason,
		vocabulary.WithDescription("Coordinator-redesign Slice 2: copy of the structural gate's reject_reason token (mode_mismatch | phase_cap | invalid_edge | missing_research_artifact | zero_tests_run | tests_failed_nonzero | unrouted_needs_clarification | chain.recovery.exhausted) so operators querying by stall predicate don't have to join against four different reject_reason predicates."),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register(PredicateChainStallProducerLoopID,
		vocabulary.WithDescription("Coordinator-redesign Slice 2: loop_id of the role whose completion triggered the rejection. Wake-up rules pass this via related_loops so the spawned coordinator can read_loop_result on the producer."),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register(PredicateChainStallProducerRole,
		vocabulary.WithDescription("Coordinator-redesign Slice 2: role name of the producer loop (e.g. researcher-synthesize for mode_mismatch, reviewer-spec for missing_research_artifact, builder for qa-mode rejections / unrouted_needs_clarification)."),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register(PredicateChainStallObservedAt,
		vocabulary.WithDescription("Coordinator-redesign Slice 2: RFC3339 timestamp when chainstall wrote the cluster. Distinct from the producer loop's CompletedAt — observability surfaces use this to age the cluster."),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register(PredicateChainStallAwaitingHuman,
		vocabulary.WithDescription("Coordinator-redesign Slice 2: chain-level marker (\"true\") stamped when chainstall routes a rejection to the human-approval path. The HTTP endpoint at /teams-loop/chain-stall/decide (deferred follow-up) clears the marker via the operator's verdict."),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register(PredicateChainStallProceed,
		vocabulary.WithDescription("Coordinator-redesign Slice 2: per-cycle gate sentinel stamped on the producer loop entity when chainstall routes a rejection to the coordinator wake-up path. Wake-up rule 05 fires only when this is \"true\". Same fail-safe semantics as PredicateRecoveryProceed (ADR-040)."),
		vocabulary.WithDataType("string"),
	)
}

// registerOtherPackagePredicates registers chain.* predicates whose
// constants stay co-located with their writers (chainpause, evidence,
// emitspecartifact, dispatched, paused, decision). Centralising the
// registrations here keeps vocab queries (ListByDomain("chain"), RDF
// export, hierarchy walks) seeing the complete catalog without forcing
// constants to migrate. Promoting these to constants in this file is a
// deferred follow-up refactor; the strings are stable (post-2026-05-11
// 3-part renames) so registration is safe.
//
// Split out of init() / registerADR041Predicates() to keep both under
// revive's function-length threshold. Semantically a continuation of
// init's central-registration block.
func registerOtherPackagePredicates() {
	// chain.dispatched.* — DispatchedStamper (chain/dispatched.go).
	vocabulary.Register("chain.dispatched.at",
		vocabulary.WithDescription("ADR-038 §D2: RFC3339 timestamp at which the chain dispatched (chain root's LoopCreated event time). Phase 1b."),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register("chain.dispatched.observed_fallback",
		vocabulary.WithDescription("ADR-038 §D2: \"true\" companion triple stamped when DispatchedStamper had to fall back to observed wall-clock time because the LoopCreatedEvent.CreatedAt was zero. Operator-visible health signal."),
		vocabulary.WithDataType("string"),
	)

	// chain.paused.* — chainpause Pauser (chainpause/pauser.go). ADR-037
	// audit cluster for FAILED loops in managed-arc roles.
	vocabulary.Register("chain.paused.cause",
		vocabulary.WithDescription("ADR-037 §D5: closed-token cause for chain pause (api_overloaded | api_timeout | config_load_failure | tool_executor_panic | max_iterations | unknown)."),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register("chain.paused.classification",
		vocabulary.WithDescription("ADR-037 §D5: finer-grained classification of the pause cause for ops-agent pattern aggregation."),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register("chain.paused.role",
		vocabulary.WithDescription("ADR-037 §D5: role of the failed loop that triggered the pause."),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register("chain.paused.original_model",
		vocabulary.WithDescription("ADR-037 §D7: model name of the failed loop so a resume re-publishes with the same model without needing a live graph query."),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register("chain.paused.error_shape",
		vocabulary.WithDescription("ADR-037 §D5: length-bounded, control-char-stripped representative of the error string for forensic triage. PII-sanitised per ADR-030."),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register("chain.paused.prior_attempts",
		vocabulary.WithDescription("ADR-037 §D5: count of prior pauses on this chain. Hardcoded \"1\" in v1 per OQ2; per-(chain,role) counting earns its slot when a real cycle surfaces."),
		vocabulary.WithDataType("int"),
	)
	vocabulary.Register("chain.paused.failed_loop_id",
		vocabulary.WithDescription("ADR-037 §D5: loop_id of the failed loop that triggered the pause."),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register("chain.paused.spawn_loop_id",
		vocabulary.WithDescription("ADR-037 §D5 (v1: empty string; derived from prior_loop_id at query time per OQ1)."),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register("chain.paused.observed_at",
		vocabulary.WithDescription("ADR-037 §D5: RFC3339 timestamp when chainpause wrote the §D5 cluster."),
		vocabulary.WithDataType("string"),
	)

	// chain.decision.* — chainpause DecisionHandler. Operator decision
	// audit on a paused chain (resume / kill / defer).
	vocabulary.Register("chain.decision.verb",
		vocabulary.WithDescription("ADR-037 decision audit: closed-token verb the operator chose (resume | kill | defer)."),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register("chain.decision.authority",
		vocabulary.WithDescription("ADR-037 decision audit: authority that made the decision. v1: always \"operator\"."),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register("chain.decision.actor",
		vocabulary.WithDescription("ADR-037 decision audit: identifier of the actor (operator user-id, future coordinator agent id)."),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register("chain.decision.reason",
		vocabulary.WithDescription("ADR-037 decision audit: free-form reason the operator supplied."),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register("chain.decision.decided_at",
		vocabulary.WithDescription("ADR-037 decision audit: RFC3339 timestamp when the decision landed."),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register("chain.decision.resumed_task_id",
		vocabulary.WithDescription("ADR-037 decision audit: task_id of the new task spawned when the operator resumed the chain. Only written on verb=resume. Carries verb-specific data not duplicated elsewhere."),
		vocabulary.WithDataType("string"),
	)
	// killed_at + deferred_at are redundant with verb + decided_at (the
	// verb-specific timestamp matches decided_at when written), but kept
	// for verb-typed query convenience — operators querying for kill/
	// defer events get a single predicate per verb without joining
	// against decision.verb. Drop or fold if a future query layer makes
	// the join cheap.
	vocabulary.Register("chain.decision.killed_at",
		vocabulary.WithDescription("ADR-037 decision audit: RFC3339 timestamp when the operator killed the chain. Only written on verb=kill. Redundant with verb+decided_at — kept for verb-typed query convenience."),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register("chain.decision.deferred_at",
		vocabulary.WithDescription("ADR-037 decision audit: RFC3339 timestamp when the operator deferred the decision. Only written on verb=defer. Redundant with verb+decided_at — kept for verb-typed query convenience."),
		vocabulary.WithDataType("string"),
	)

	// chain.evidence.* — evidence.Preprocessor (evidence/). ADR-036
	// §Phase 2 R3.7.2.k′-bis evidence-summary milestone on
	// builder loop entities.
	vocabulary.Register("chain.evidence.summary_ready",
		vocabulary.WithDescription("ADR-036 §Phase 2 evidence summary: \"true\" marker indicating the preprocessor stamped a summary on the builder's loop entity."),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register("chain.evidence.summary.path",
		vocabulary.WithDescription("ADR-036 §Phase 2 + PR C Phase C4: path to the rendered evidence-summary markdown artifact. Composed from chain.slug.stem + suffix."),
		vocabulary.WithDataType("string"),
	)

	// chain.research_artifact.* — ResearchMilestoneStamper (chain/research.go).
	// PR C Phase C1 extension of the Phase 2 research milestone.
	vocabulary.Register("chain.research_artifact.harness",
		vocabulary.WithDescription("Test harness identifier extracted from the approved researcher's artifact. Read by emit_dev_via_spec_artifact + ops-agent harness-selection diagnoses."),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register("chain.research_artifact.actor_count",
		vocabulary.WithDescription("Actor count from the approved researcher's artifact (research.artifact.actors_count)."),
		vocabulary.WithDataType("int"),
	)
	vocabulary.Register("chain.research_artifact.task_count",
		vocabulary.WithDescription("Task count from the approved researcher's artifact (research.artifact.tasks_count)."),
		vocabulary.WithDataType("int"),
	)
	vocabulary.Register("chain.research_artifact.path",
		vocabulary.WithDescription("PR C Phase C1: path to the rendered research artifact markdown."),
		vocabulary.WithDataType("string"),
	)

	// chain.spec_artifact.* — emit_dev_via_spec_artifact tool
	// (cmd/semteams/tools/emitspecartifact). Phase 4 milestone.
	vocabulary.Register("chain.spec_artifact.loop",
		vocabulary.WithDescription("Phase 4: loop_id of the architect-flavour role (researcher-architect under ADR-041 MVP, dev-via-spec-architect under legacy) whose spec artifact the chain consumed. 3-part canonical shape (2026-05-11 rename from chain.spec_artifact_loop)."),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register("chain.spec_artifact.path",
		vocabulary.WithDescription("Phase 4: path to the rendered dev-via-spec artifact markdown."),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register("chain.spec_artifact.check_count",
		vocabulary.WithDescription("Phase 4: structural check count for the dev-via-spec artifact (evidence rules + assertions)."),
		vocabulary.WithDataType("int"),
	)
}
