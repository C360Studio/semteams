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
// Coverage discipline: this file declares the predicates the smoke
// #8 run-5 D1 + D2 fix added (PredicateSlugStem,
// PredicatePlanReviewerLoop) plus the existing predicates downstream
// emit-tools READ (PredicateResearchArtifactLoop, PredicatePlanLoop,
// PredicateConsensusLoop). The remaining chain.* predicates
// (chain.dispatched_at, chain.research_artifact.{harness,actor_count,
// task_count,path}, chain.plan.path, chain.consensus.path,
// chain.spec_artifact.*, chain.evidence.*, chain.paused.*,
// chain.decision.*) are still defined as unexported per-stamper
// constants; promoting them here is mechanical follow-up scoped to
// a focused vocab-completion PR. The contract test
// test/contract/chain_entity_coverage_test.go pins the union both
// sides must agree on regardless.
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
	PredicateResearchArtifactLoop = "chain.research_artifact_loop"

	// PredicatePlanLoop names the chain-entity predicate that records
	// the planner loop_id whose plan the dev-via-spec-reviewer
	// approved. Read by emit_consensus + emit_dev_via_spec_artifact.
	PredicatePlanLoop = "chain.plan_loop"

	// PredicatePlanReviewerLoop names the chain-entity predicate that
	// records the dev-via-spec-reviewer loop_id that approved the
	// plan. Read by emit_consensus + emit_dev_via_spec_artifact to
	// populate the reviewer-loop slot without confusing it with the
	// planner's loop ID (smoke #8 run-5 D1 fix).
	PredicatePlanReviewerLoop = "chain.plan_reviewer_loop"

	// PredicateConsensusLoop names the chain-entity predicate that
	// records the dev-via-spec-challenger loop_id that accepted the
	// plan. Read by emit_dev_via_spec_artifact to populate
	// "provenance.challenger_loop".
	PredicateConsensusLoop = "chain.consensus_loop"

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
)

// init registers the chain vocabulary with the upstream vocabulary
// registry. Auto-runs at first import of this package; semteams' main
// imports cmd/semteams/chain via the wiring path so this fires once
// at process start.
//
// Naming: predicates use 2- or 3-level dotted notation. The vocabulary
// package documents 3-level as preferred (domain.category.property)
// but accepts 2-level — chain.research_artifact_loop and
// chain.plan_loop are loop_id references where the property is
// implicit, while chain.research_artifact.path / chain.plan.path /
// etc. follow the full 3-level shape. Both work; consistency comes
// from tracking each in this single file.
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
}
