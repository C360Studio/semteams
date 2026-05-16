package chain

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/types"
	agvocab "github.com/c360studio/semstreams/vocabulary/agentic"
)

const (
	chainNeedsReviewSource = "chain.needs_review"

	// builderRole is the role NeedsReviewStamper monitors in Phase 1.
	// Builder's needs_clarification carries coordinator.blocking_question
	// by shape — that signals "operator-actionable" per ADR-039 §"What
	// rules fire on" (run-10 catalog gap is the canonical case).
	builderRole = "builder"

	// needsClarificationAction is the coordinator.next_action value the
	// builder's builder_decide stamps when terminating with
	// a recoverable verdict the chain cannot route deterministically.
	needsClarificationAction = "needs_clarification"

	// needsReviewClassificationDefault is the Phase 1 single-bucket
	// classification. ADR-039 §"Tier 3" lists chain.needs_review.classification
	// as open-valued (e.g. catalog_gap, coordinator_declined,
	// unrouted_needs_clarification); Phase 1 starts with the catch-all
	// and refines once smoke evidence justifies finer pattern-matching
	// (which would live in this package, not the producer's persona).
	needsReviewClassificationDefault = "unrouted_needs_clarification"
)

// NeedsReviewStamper writes the chain.needs_review.* triple cluster on
// the chain entity when a builder loop completes with
// coordinator.next_action="needs_clarification" — the Tier 3 fallback
// per ADR-039 Phase 1 Slice B.
//
// Tier 3 is "the chain has a recoverable terminal that no Tier 1 rule
// could route, awaiting a recovery decision." Consumers of the cluster
// are deployment-configured: a coordinator agent that wakes on the
// write (Phase 2), an operator dashboard, or a metric-emit job for
// later triage. The stamper does not presume the consumer; the
// deployment policy does.
//
// The cluster is **deliberately distinct from chain.paused.*** (ADR-037).
// chain.paused is for FAILED loops with closed-enum classifications
// (api_overloaded, max_iterations_exhausted, etc.) — failure semantics.
// chain.needs_review is for SUCCESSFUL loops whose verdict is
// "I have a structured terminal but it requires information I don't
// have" — recoverable-pending semantics. Conflating the two predicate
// clusters would confuse downstream consumers (chainpause's existing
// handlers, ops-chain-observer's diagnosis logic).
//
// Read pattern: the builder's own loop entity carries both
// coordinator.next_action and coordinator.decision_reason (the
// upstream agentic.decide tool writes both as part of the
// builder_decide terminal). Resolver walks ancestry from the builder's
// loop_id to find chain_id → chain entity ID.
//
// Write pattern: the five chain.needs_review.* triples land together;
// per-triple write errors are logged and remaining triples proceed
// (partial cluster is queryable; a zero-triple record is invisible).
// Resolver failures (graph blip mid-write) skip the entire cluster and
// log Warn — the agent.complete event is on the wire regardless, so
// operators see the verdict even when chain.needs_review.* doesn't land.
type NeedsReviewStamper struct {
	publisher TriplePublisher
	resolver  *Resolver
	entities  EntityTripleReader
	platform  types.PlatformMeta
	logger    *slog.Logger
}

// NewNeedsReviewStamper constructs a stamper.
func NewNeedsReviewStamper(
	pub TriplePublisher,
	resolver *Resolver,
	entities EntityTripleReader,
	platform types.PlatformMeta,
	logger *slog.Logger,
) *NeedsReviewStamper {
	if logger == nil {
		logger = slog.Default()
	}
	return &NeedsReviewStamper{
		publisher: pub,
		resolver:  resolver,
		entities:  entities,
		platform:  platform,
		logger:    logger,
	}
}

// HandleLoopCompleted is the CompletionHandler entry point. Filters to
// builder success with coordinator.next_action=needs_clarification;
// for non-matching events returns nil so other handlers in the dispatch
// chain see the event.
func (s *NeedsReviewStamper) HandleLoopCompleted(ctx context.Context, ev *agentic.LoopCompletedEvent) error {
	if ev == nil {
		return nil
	}
	if ev.Role != builderRole || ev.Outcome != agentic.OutcomeSuccess {
		return nil
	}
	if err := ValidateLoopID(ev.LoopID); err != nil {
		return fmt.Errorf("chain.NeedsReviewStamper: %w", err)
	}

	builderEntityID := agentic.LoopExecutionEntityID(s.platform.Org, s.platform.Platform, ev.LoopID)
	builderTriples, err := s.entities.ReadEntity(ctx, builderEntityID)
	if err != nil {
		s.logger.Warn("needs-review milestone: read builder entity failed",
			slog.String("builder_loop_id", ev.LoopID),
			slog.String("error", err.Error()))
		return nil
	}
	action, _ := builderTriples[agvocab.CoordinatorNextAction].(string)
	if action != needsClarificationAction {
		// tests_passing / tests_failing / other terminal — handled by rule 07.
		return nil
	}
	reason, _ := builderTriples[agvocab.CoordinatorDecisionReason].(string)

	chainEntityID, err := s.resolver.ChainEntityID(ctx, ev.LoopID)
	if err != nil {
		s.logger.Warn("needs-review milestone: chain ancestry walk failed; skipping chain triples",
			slog.String("builder_loop_id", ev.LoopID),
			slog.String("error", err.Error()))
		return nil
	}

	now := time.Now().UTC()
	base := func(pred string, obj any) message.Triple {
		return message.Triple{
			Subject:    chainEntityID,
			Predicate:  pred,
			Object:     obj,
			Source:     chainNeedsReviewSource,
			Timestamp:  now,
			Confidence: 1.0,
		}
	}
	triples := []message.Triple{
		base(PredicateNeedsReviewClassification, needsReviewClassificationDefault),
		base(PredicateNeedsReviewProducerLoopID, ev.LoopID),
		base(PredicateNeedsReviewProducerRole, ev.Role),
		base(PredicateNeedsReviewReason, reason),
		base(PredicateNeedsReviewObservedAt, now.Format(time.RFC3339)),
	}

	for _, t := range triples {
		if err := s.publisher.AddTriple(ctx, t); err != nil {
			s.logger.Warn("needs-review milestone: chain triple write failed",
				slog.String("predicate", t.Predicate),
				slog.String("chain_entity", chainEntityID),
				slog.String("error", err.Error()))
		}
	}

	s.logger.Info("needs-review milestone: chain triples stamped",
		slog.String("builder_loop_id", ev.LoopID),
		slog.String("chain_entity", chainEntityID),
		slog.String("classification", needsReviewClassificationDefault),
		slog.Int("triples_attempted", len(triples)))
	return nil
}
