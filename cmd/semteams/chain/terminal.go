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
	// chainTerminalSource tags the Source field on every triple this
	// handler writes — operators inspecting the graph can attribute the
	// write to this subscriber without parsing the predicate name.
	chainTerminalSource = "chain.terminal"

	// reviewerResearchRole is the terminating reviewer role for the
	// research-category chain shape. Pairs with approvedAction. Under the
	// ADR-042 MVP roster this is the only terminal reviewer; the legacy
	// reviewer-qa / accept pair retired in MVP-7 (PR #178) alongside the
	// dev-via-spec arc.
	reviewerResearchRole = "reviewer-research"

	// approvedAction is the coordinator.decision.next_action value
	// reviewer-research emits via decide(action="approved"). Sourced from
	// configs/personas/fragments/reviewer-research/ — action_allowlist
	// on configs/rules/research/04-synthesize-to-reviewer.json (MVP-2).
	approvedAction = "approved"
)

// roleToApprovalAction maps the terminating-reviewer role to the
// coordinator.decision.next_action token that signals "chain terminated
// cleanly." Single-entry map under the MVP roster — extending the
// chain taxonomy with a second terminal role (e.g. a future
// dev-via-spec category pack reintroducing reviewer-qa) requires
// adding an entry here AND the corresponding spawn-coordinator rule
// in the category's pack dir. The contract test
// test/contract/chain_entity_coverage_test.go pins the predicate
// shape both sides agree on.
var roleToApprovalAction = map[string]string{
	reviewerResearchRole: approvedAction,
}

// TerminalStamper writes the chain.terminal.* triple cluster on the
// chain entity when a terminating-reviewer loop completes with the
// role-specific approval action.
//
// "Terminal" here means "the chain has reached its expected end-of-arc"
// — distinct from chain.paused.* (FAILED loops, ADR-037), distinct from
// chain.needs_review.* (recoverable-pending verdicts, ADR-039 Phase 1
// Tier 3), distinct from chain.recovery.* (per-cycle retry accounting,
// ADR-040). The cluster represents "the user's request has been
// answered" so the coordinator can wake and deliver the result.
//
// Read pattern: the reviewer's own loop entity carries
// coordinator.decision.next_action (written by the upstream agentic.decide
// tool). The stamper reads the action, gates on the role-specific
// approval token, walks ancestry from reviewer loop_id → chain entity,
// and writes the cluster onto the chain entity.
//
// Write pattern: four triples land together. Per-triple write failures
// are logged and remaining triples proceed (partial cluster is
// queryable; a zero-triple record is invisible to ops queries).
// Resolver / entity-read failures skip the entire cluster fail-safe —
// the agent.complete event is on the wire regardless, so operators see
// the verdict even when chain.terminal.* doesn't land.
//
// Coordinator wake-up: configs/rules/coordinator/04a- and 04b- fire on
// the reviewer's terminal triples (role + decision.next_action) rather
// than chain.terminal.reached, because the rule engine's firing entity
// is the reviewer loop entity, not the chain entity. The chain.terminal
// cluster is for audit + ops queries; the rule-engine path uses the
// loop-entity triples directly.
type TerminalStamper struct {
	publisher TriplePublisher
	resolver  *Resolver
	entities  EntityTripleReader
	platform  types.PlatformMeta
	logger    *slog.Logger
}

// NewTerminalStamper constructs a stamper. logger may be nil; the
// constructor substitutes slog.Default() to match every sibling
// chain-handler.
func NewTerminalStamper(
	pub TriplePublisher,
	resolver *Resolver,
	entities EntityTripleReader,
	platform types.PlatformMeta,
	logger *slog.Logger,
) *TerminalStamper {
	if logger == nil {
		logger = slog.Default()
	}
	return &TerminalStamper{
		publisher: pub,
		resolver:  resolver,
		entities:  entities,
		platform:  platform,
		logger:    logger,
	}
}

// HandleLoopCompleted is the chain.CompletionHandler entry point.
//
// Filter contract:
//
//   - ev.Role ∈ {reviewer-research} (the roleToApprovalAction map's keys)
//   - ev.Outcome == OutcomeSuccess
//   - coordinator.decision.next_action == the role-specific approval token
//
// Every other event is a structural no-op (returns nil so sibling
// handlers see the event).
func (s *TerminalStamper) HandleLoopCompleted(ctx context.Context, ev *agentic.LoopCompletedEvent) error {
	if ev == nil {
		return nil
	}
	wantAction, ok := roleToApprovalAction[ev.Role]
	if !ok {
		return nil
	}
	if ev.Outcome != agentic.OutcomeSuccess {
		return nil
	}
	if err := ValidateLoopID(ev.LoopID); err != nil {
		return fmt.Errorf("chain.TerminalStamper: %w", err)
	}

	reviewerEntityID := agentic.LoopExecutionEntityID(s.platform.Org, s.platform.Platform, ev.LoopID)
	reviewerTriples, err := s.entities.ReadEntity(ctx, reviewerEntityID)
	if err != nil {
		s.logger.Warn("chain.terminal stamper: read reviewer entity failed; skipping",
			slog.String("reviewer_loop_id", ev.LoopID),
			slog.String("role", ev.Role),
			slog.String("error", err.Error()))
		return nil
	}
	action, _ := reviewerTriples[agvocab.CoordinatorNextAction].(string)
	if action != wantAction {
		// insufficient / reject / needs_clarification — not a terminal
		// approval. Other handlers (NeedsReviewStamper, RecoveryCounter)
		// pick these up.
		return nil
	}

	chainEntityID, err := s.resolver.ChainEntityID(ctx, ev.LoopID)
	if err != nil {
		s.logger.Warn("chain.terminal stamper: chain ancestry walk failed; skipping",
			slog.String("reviewer_loop_id", ev.LoopID),
			slog.String("role", ev.Role),
			slog.String("error", err.Error()))
		return nil
	}

	now := time.Now().UTC()
	base := func(pred string, obj any) message.Triple {
		return message.Triple{
			Subject:    chainEntityID,
			Predicate:  pred,
			Object:     obj,
			Source:     chainTerminalSource,
			Timestamp:  now,
			Confidence: 1.0,
		}
	}
	triples := []message.Triple{
		base(PredicateChainTerminalReached, "true"),
		base(PredicateChainTerminalRole, ev.Role),
		base(PredicateChainTerminalLoopID, ev.LoopID),
		base(PredicateChainTerminalObservedAt, now.Format(time.RFC3339)),
	}
	for _, t := range triples {
		if err := s.publisher.AddTriple(ctx, t); err != nil {
			s.logger.Warn("chain.terminal stamper: triple write failed",
				slog.String("predicate", t.Predicate),
				slog.String("chain_entity", chainEntityID),
				slog.String("error", err.Error()))
		}
	}

	s.logger.Info("chain.terminal stamped",
		slog.String("reviewer_loop_id", ev.LoopID),
		slog.String("chain_entity", chainEntityID),
		slog.String("role", ev.Role),
		slog.String("action", action))
	return nil
}
