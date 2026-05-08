package chain

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/types"
)

// Predicate names for the consensus-milestone cluster on the chain
// entity. Schema-stable per ADR-038 D2:
//
//	chain.consensus_loop                 <challenger_loop_id_at_accept>
//	chain.consensus.path                 <docs/consensus/<slug>.md>
//
// Path absence is the legacy-consensus signal; chain triple omitted,
// not fabricated. Same shape as plan and research milestones.
//
// Path shape: relative to process working directory under the default
// outputDir ("docs/consensus"); absolute when operators set
// SEMTEAMS_CONSENSUS_DIR to an absolute path.
const (
	chainPredicateConsensusLoop = "chain.consensus_loop"
	chainPredicateConsensusPath = "chain.consensus.path"

	chainConsensusSource = "chain.consensus"
)

// Predicate names on the challenger's own loop entity (read side).
// Unlike the plan milestone (which walks one hop reviewer→planner),
// the challenger IS the consensus loop — no walk needed. The
// challenger's accept terminal IS the consensus event.
const (
	challengerArtifactPathPredicate = "dev_via_spec.consensus.path"

	// challengerRole is the role we milestone on. The challenger's
	// accept terminal is captured directly: ev.LoopID is the
	// consensus_loop, and ev.LoopID's own loop entity carries the
	// dev_via_spec.consensus.path predicate (when emit_consensus was
	// called).
	challengerRole = "dev-via-spec-challenger"

	// consensusAcceptAction is the coordinator.next_action value the
	// challenger persona stamps via decide(action="accept"). Sourced
	// from rule_05's spawn action_allowlist.
	consensusAcceptAction = "accept"
)

// ConsensusMilestoneStamper writes the chain.consensus.* triple
// cluster on the chain entity when a dev-via-spec-challenger loop
// completes with coordinator.next_action="accept".
//
// Read pattern: the challenger's own loop entity carries both
// coordinator.next_action and dev_via_spec.consensus.path. No
// parent walk needed; the challenger IS the consensus loop. Resolver
// walks ancestry from the challenger's loop_id to find chain_id →
// chain entity ID.
//
// Write pattern: chain.consensus_loop always (when challenger
// accepts); chain.consensus.path only when emit_consensus was called
// (legacy challengers without rendering signal absence — chain
// triple omitted, not fabricated).
//
// Fail-soft per the sibling milestone stampers: log Warn and skip on
// every read / resolver error.
type ConsensusMilestoneStamper struct {
	publisher TriplePublisher
	resolver  *Resolver
	entities  EntityTripleReader
	platform  types.PlatformMeta
	logger    *slog.Logger
}

// NewConsensusMilestoneStamper constructs a stamper.
func NewConsensusMilestoneStamper(
	pub TriplePublisher,
	resolver *Resolver,
	entities EntityTripleReader,
	platform types.PlatformMeta,
	logger *slog.Logger,
) *ConsensusMilestoneStamper {
	if logger == nil {
		logger = slog.Default()
	}
	return &ConsensusMilestoneStamper{
		publisher: pub,
		resolver:  resolver,
		entities:  entities,
		platform:  platform,
		logger:    logger,
	}
}

// HandleLoopCompleted is the subscription entry point. Filters to
// dev-via-spec-challenger success and the accept coordinator action;
// for non-matching events returns nil (other handlers may fire).
func (s *ConsensusMilestoneStamper) HandleLoopCompleted(ctx context.Context, ev *agentic.LoopCompletedEvent) error {
	if ev == nil {
		return nil
	}
	if ev.Role != challengerRole || ev.Outcome != agentic.OutcomeSuccess {
		return nil
	}
	if err := validateLoopID(ev.LoopID); err != nil {
		return fmt.Errorf("chain.ConsensusMilestoneStamper: %w", err)
	}

	challengerEntityID := agentic.LoopExecutionEntityID(s.platform.Org, s.platform.Platform, ev.LoopID)
	challengerTriples, err := s.entities.ReadEntity(ctx, challengerEntityID)
	if err != nil {
		s.logger.Warn("consensus milestone: read challenger entity failed",
			slog.String("challenger_loop_id", ev.LoopID),
			slog.String("error", err.Error()))
		return nil
	}
	if action, _ := challengerTriples[reviewerNextActionPredicate].(string); action != consensusAcceptAction {
		// concerns_raised / other terminal — not the consensus milestone.
		return nil
	}

	chainEntityID, err := s.resolver.ChainEntityID(ctx, ev.LoopID)
	if err != nil {
		s.logger.Warn("consensus milestone: chain ancestry walk failed; skipping chain triples",
			slog.String("challenger_loop_id", ev.LoopID),
			slog.String("error", err.Error()))
		return nil
	}

	now := time.Now().UTC()
	base := func(pred string, obj any) message.Triple {
		return message.Triple{
			Subject:    chainEntityID,
			Predicate:  pred,
			Object:     obj,
			Source:     chainConsensusSource,
			Timestamp:  now,
			Confidence: 1.0,
		}
	}
	triples := []message.Triple{
		base(chainPredicateConsensusLoop, ev.LoopID),
	}
	if path, _ := challengerTriples[challengerArtifactPathPredicate].(string); path != "" {
		triples = append(triples, base(chainPredicateConsensusPath, path))
	}

	for _, t := range triples {
		if err := s.publisher.AddTriple(ctx, t); err != nil {
			s.logger.Warn("consensus milestone: chain triple write failed",
				slog.String("predicate", t.Predicate),
				slog.String("chain_entity", chainEntityID),
				slog.String("error", err.Error()))
		}
	}

	s.logger.Debug("consensus milestone: chain triples stamped",
		slog.String("challenger_loop_id", ev.LoopID),
		slog.String("chain_entity", chainEntityID),
		slog.Int("triples_attempted", len(triples)))
	return nil
}
