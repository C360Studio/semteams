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
// PredicateResearchArtifactLoop, PredicatePlanLoop,
// PredicatePlanReviewerLoop, PredicateConsensusLoop, PredicateRecovery*,
// PredicateNeedsReview*) Register against the constant. Predicates whose
// constants stay co-located with their writers (chain.dispatched.*,
// chain.paused.*, chain.decision.*, chain.evidence.*,
// chain.research_artifact.{harness,actor_count,task_count,path},
// chain.plan.path, chain.consensus.path, chain.spec_artifact.*) Register
// against string literals — promoting them to constants here is a
// deferred follow-up refactor. The contract test
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
// chain.consensus_loop → chain.consensus.loop;
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

	// PredicatePlanLoop names the chain-entity predicate that records
	// the planner loop_id whose plan the dev-via-spec-reviewer
	// approved. Read by emit_consensus + emit_dev_via_spec_artifact.
	PredicatePlanLoop = "chain.plan.loop"

	// PredicatePlanReviewerLoop names the chain-entity predicate that
	// records the dev-via-spec-reviewer loop_id that approved the
	// plan. Read by emit_consensus + emit_dev_via_spec_artifact to
	// populate the reviewer-loop slot without confusing it with the
	// planner's loop ID (smoke #8 run-5 D1 fix).
	PredicatePlanReviewerLoop = "chain.plan.reviewer_loop"

	// PredicateConsensusLoop names the chain-entity predicate that
	// records the dev-via-spec-challenger loop_id that accepted the
	// plan. Read by emit_dev_via_spec_artifact to populate
	// "provenance.challenger_loop".
	PredicateConsensusLoop = "chain.consensus.loop"

	// PredicateNeedsReviewClassification is the first predicate of the
	// chain.needs_review.* cluster (ADR-039 Phase 1 Tier 3). The full
	// cluster is stamped by NeedsReviewStamper when a
	// needs_clarification terminal lands with no Tier 1 rule match and
	// no Tier 2 coordinator configured — the cluster represents
	// "awaiting a recovery decision" and the consumer is
	// deployment-configured (coordinator agent, operator dashboard,
	// metric-emit job). Distinct from chain.paused.* (ADR-037, FAILED
	// loops with closed-enum classifications). Phase 1 only stamps for
	// dev-via-spec-builder; broader producer coverage is a follow-up.
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
	// loop (dev-via-spec-builder for Phase 1; broader when Slice C lands).
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

	// PredicateRecoveryCount is the per-chain count of research-reviewer
	// rejection cycles that have triggered a source-curator recovery
	// attempt (ADR-040 §addendum 2026-05-11 "Chain-level recovery cap").
	// Stamped on the canonical 6-part chain entity by RecoveryCounter
	// each time a research-reviewer completes with
	// coordinator.next_action="insufficient". Also mirrored onto the
	// triggering reviewer loop entity so rule_02 can gate via
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
		vocabulary.WithDescription("Loop_id of the researcher whose artifact the research-reviewer approved. The chain entity's canonical reference to the upstream research arc."),
		vocabulary.WithDataType("string"),
	)

	vocabulary.Register(PredicatePlanLoop,
		vocabulary.WithDescription("Loop_id of the planner whose plan the dev-via-spec-reviewer approved. The chain entity's canonical reference to the approved planner pass."),
		vocabulary.WithDataType("string"),
	)

	vocabulary.Register(PredicatePlanReviewerLoop,
		vocabulary.WithDescription("Loop_id of the dev-via-spec-reviewer that approved the plan. Distinct from the planner; downstream emit_consensus reads this to populate depends_on.reviewer_loop."),
		vocabulary.WithDataType("string"),
	)

	vocabulary.Register(PredicateConsensusLoop,
		vocabulary.WithDescription("Loop_id of the dev-via-spec-challenger that accepted the plan. The chain entity's canonical reference to the consensus terminal."),
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
		vocabulary.WithDescription("ADR-039 Phase 1 Tier 3 cluster: role name of the producer loop (dev-via-spec-builder for Phase 1; broader producer coverage in follow-up slices)."),
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
		vocabulary.WithDescription("ADR-040 §addendum 2026-05-11: per-chain count of research-reviewer rejection cycles that triggered a source-curator recovery attempt. Stored as string-formatted integer for rule-engine string comparisons. Stamped on chain entity (audit trail) and mirrored onto the triggering reviewer loop entity (rule-engine read surface)."),
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

	// Central registration for chain.* predicates written by other
	// packages. Constants stay co-located with their writers (chainpause,
	// evidence, emitspecartifact, etc.); registration is centralised here
	// so vocab queries (ListByDomain("chain"), RDF export, hierarchy
	// walks) see the complete catalog. Promoting these to constants in
	// this file is a deferred follow-up refactor; the strings are stable
	// (post-2026-05-11 3-part renames) so registration is safe.
	//
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
	// dev-via-spec-builder loop entities.
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

	// chain.plan.* — PlanMilestoneStamper (chain/plan.go). PR C Phase C2.
	vocabulary.Register("chain.plan.path",
		vocabulary.WithDescription("PR C Phase C2: path to the rendered planner artifact markdown."),
		vocabulary.WithDataType("string"),
	)

	// chain.consensus.* — ConsensusMilestoneStamper (chain/consensus.go).
	// PR C Phase C3.
	vocabulary.Register("chain.consensus.path",
		vocabulary.WithDescription("PR C Phase C3: path to the rendered consensus artifact markdown."),
		vocabulary.WithDataType("string"),
	)

	// chain.spec_artifact.* — emit_dev_via_spec_artifact tool
	// (cmd/semteams/tools/emitspecartifact). Phase 4 milestone.
	vocabulary.Register("chain.spec_artifact.loop",
		vocabulary.WithDescription("Phase 4: loop_id of the dev-via-spec-architect whose spec artifact the chain consumed. 3-part canonical shape (2026-05-11 rename from chain.spec_artifact_loop)."),
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
