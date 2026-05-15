package chainmode

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/types"
	agvocab "github.com/c360studio/semstreams/vocabulary/agentic"

	"github.com/c360studio/semteams/cmd/semteams/chain"
)

const (
	// coordinatorRole is the role this stamper monitors. Only loops
	// whose role.name is exactly "coordinator" emit a delegate_*
	// terminal that classifies a chain.
	coordinatorRole = "coordinator"

	// actionDelegateResearch / actionDelegateDevChain are the two
	// coordinator decide-action values that route into a downstream
	// chain. Every other action (respond_direct, ask_user,
	// needs_clarification) terminates without spawning a chain; this
	// stamper is a no-op for them.
	actionDelegateResearch = "delegate_research"
	actionDelegateDevChain = "delegate_dev_chain"

	// ModeResearchOnly classifies a chain spawned via
	// decide(action=delegate_research). Synthesize must terminate at
	// reviewer-research; the phase validator refuses synthesize →
	// architect under this mode.
	ModeResearchOnly = "research_only"

	// ModeDevViaSpec classifies a chain spawned via
	// decide(action=delegate_dev_chain). Synthesize → architect is
	// allowed; the chain continues through reviewer-spec / builder /
	// reviewer-qa.
	ModeDevViaSpec = "dev_via_spec"

	// triplesSource tags the Source field on every triple this handler
	// writes — operators inspecting the graph can attribute the write
	// to this subscriber without parsing the predicate name.
	triplesSource = "chain.mode"
)

// actionToMode maps a coordinator decide-action terminal to the
// chain.mode token. Single source of truth — the unit test pins this
// table so adding a new action without updating the validator gate is
// a compile-then-test failure rather than silent drift.
var actionToMode = map[string]string{
	actionDelegateResearch: ModeResearchOnly,
	actionDelegateDevChain: ModeDevViaSpec,
}

// TriplePublisher is the narrow write surface this package needs.
// agentictools.NATSTriplePublisher satisfies it structurally. Mirrors
// the per-package interface convention used by chain, chainpause,
// evidence, and phasevalidator.
type TriplePublisher interface {
	AddTriple(ctx context.Context, triple message.Triple) error
}

// Stamper writes the chain.mode triple onto the chain entity when a
// coordinator loop completes with a delegate_* action.
//
// Read pattern: the coordinator's own loop entity carries
// coordinator.next_action (written by the upstream agentic.decide
// tool). The Stamper reads the action, maps to a mode token, walks
// the resolver from coordinator loop_id → chain entity (the
// coordinator is the chain root so the walk is one-hop; the
// abstraction holds if a future config spawns coordinator off a
// dispatch parent), and writes chain.mode on the chain entity.
//
// Write pattern: one triple. Per-write failure is logged with full
// context; no partial-cluster recovery needed.
type Stamper struct {
	publisher TriplePublisher
	resolver  *chain.Resolver
	entities  chain.EntityTripleReader
	platform  types.PlatformMeta
	logger    *slog.Logger
}

// NewStamper constructs a Stamper. logger may be nil; the constructor
// substitutes slog.Default() to match every sibling chain-handler.
func NewStamper(
	pub TriplePublisher,
	resolver *chain.Resolver,
	entities chain.EntityTripleReader,
	platform types.PlatformMeta,
	logger *slog.Logger,
) *Stamper {
	if logger == nil {
		logger = slog.Default()
	}
	return &Stamper{
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
//   - ev.Role == "coordinator"
//   - ev.Outcome == OutcomeSuccess
//   - coordinator.next_action ∈ {delegate_research, delegate_dev_chain}
//
// Every other event is a structural no-op (returns nil so sibling
// handlers see the event).
func (s *Stamper) HandleLoopCompleted(ctx context.Context, ev *agentic.LoopCompletedEvent) error {
	if ev == nil {
		return nil
	}
	if ev.Role != coordinatorRole || ev.Outcome != agentic.OutcomeSuccess {
		return nil
	}
	if err := chain.ValidateLoopID(ev.LoopID); err != nil {
		// Defensive: LoopCompletedEvent.Validate enforces non-empty
		// LoopID at publish time, but the subscriber path reaches us
		// via JSON unmarshal which doesn't re-run Validate. Empty or
		// dotted LoopID would propagate into LoopExecutionEntityID's
		// panic. ValidateLoopID is the shared entry-seam check every
		// chain CompletionHandler uses.
		return fmt.Errorf("chainmode.Stamper: %w", err)
	}

	coordEntityID := agentic.LoopExecutionEntityID(s.platform.Org, s.platform.Platform, ev.LoopID)
	coordTriples, err := s.entities.ReadEntity(ctx, coordEntityID)
	if err != nil {
		s.logger.Warn("chain.mode stamper: read coordinator entity failed; skipping",
			slog.String("loop_id", ev.LoopID),
			slog.String("error", err.Error()))
		return nil
	}
	action, _ := coordTriples[agvocab.CoordinatorNextAction].(string)
	mode, ok := actionToMode[action]
	if !ok {
		// respond_direct / ask_user / needs_clarification / anything else
		// — coordinator terminated without spawning a chain, so no
		// chain.mode to stamp.
		return nil
	}

	chainEntityID, err := s.resolver.ChainEntityID(ctx, ev.LoopID)
	if err != nil {
		s.logger.Warn("chain.mode stamper: chain ancestry walk failed; skipping",
			slog.String("loop_id", ev.LoopID),
			slog.String("action", action),
			slog.String("error", err.Error()))
		return nil
	}

	now := time.Now().UTC()
	triple := message.Triple{
		Subject:    chainEntityID,
		Predicate:  chain.PredicateChainMode,
		Object:     mode,
		Source:     triplesSource,
		Timestamp:  now,
		Confidence: 1.0,
	}
	if err := s.publisher.AddTriple(ctx, triple); err != nil {
		s.logger.Warn("chain.mode stamper: triple write failed",
			slog.String("loop_id", ev.LoopID),
			slog.String("chain_entity", chainEntityID),
			slog.String("mode", mode),
			slog.String("error", err.Error()))
		return nil
	}

	s.logger.Info("chain.mode stamped",
		slog.String("loop_id", ev.LoopID),
		slog.String("chain_entity", chainEntityID),
		slog.String("action", action),
		slog.String("mode", mode))
	return nil
}
