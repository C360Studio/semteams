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

// TriplePublisher is the narrow write surface chain stampers need.
// agentictools.NATSTriplePublisher (the upstream production type)
// satisfies it structurally; tests inject an in-memory recorder.
type TriplePublisher interface {
	AddTriple(ctx context.Context, triple message.Triple) error
}

// DispatchedStamper writes the chain.dispatched_at triple on the chain
// entity when a chain-root loop (one with no parent) completes — i.e.,
// the dispatch loop closing the chain-start milestone of ADR-038 D2.
//
// The triple's Object is the dispatch loop's CompletedAt timestamp in
// RFC3339. CreatedAt would be more accurate semantically ("chain
// started at"), but LoopCompletedEvent doesn't carry it; CompletedAt
// is within seconds of chain start since dispatch loops are short, and
// the milestone semantics of "chain has dispatched" are met either way.
//
// Idempotent: re-receiving the same completion event re-writes the same
// triple (chain_id is stable per chain). If a second subscriber instance
// runs in parallel, both will write the same value — graph-ingest is
// last-writer-wins, no harm.
//
// Fail-soft: triple-write errors are returned to the caller for
// logging but do not abort. There's only one triple to write here, so
// "partial state" isn't a concern; the return is for caller
// observability only.
type DispatchedStamper struct {
	publisher TriplePublisher
	platform  types.PlatformMeta
	logger    *slog.Logger
}

// NewDispatchedStamper constructs a DispatchedStamper backed by the
// given publisher.
func NewDispatchedStamper(pub TriplePublisher, platform types.PlatformMeta, logger *slog.Logger) *DispatchedStamper {
	if logger == nil {
		logger = slog.Default()
	}
	return &DispatchedStamper{
		publisher: pub,
		platform:  platform,
		logger:    logger,
	}
}

// HandleLoopCompleted is the subscription entry point. Acts only when
// ev.ParentLoopID is empty — that's the chain-root signal. For
// non-root loops the function is a no-op (stamper is shared across all
// completion events on agent.complete.>).
//
// Returns nil when the event is not chain-root (no work). Returns the
// underlying triple-write error when one occurs, so the subscription
// can log with full context.
func (s *DispatchedStamper) HandleLoopCompleted(ctx context.Context, ev *agentic.LoopCompletedEvent) error {
	if ev == nil {
		return nil
	}
	if ev.ParentLoopID != "" {
		// Not the chain root; another stamper handles non-root milestones.
		return nil
	}
	if err := validateLoopID(ev.LoopID); err != nil {
		// Defensive: shouldn't happen for events that came off the wire,
		// but a malformed event id should surface as an error rather than
		// panic the subscriber goroutine via the upstream constructor.
		return fmt.Errorf("chain.DispatchedStamper.HandleLoopCompleted: %w", err)
	}

	chainEntityID := agentic.ChainExecutionEntityID(s.platform.Org, s.platform.Platform, ev.LoopID)
	now := time.Now().UTC()
	dispatchedAt := ev.CompletedAt
	if dispatchedAt.IsZero() {
		// Defensive fallback: framework should always set CompletedAt, but
		// a zero timestamp on the wire would render the triple useless. Use
		// the observation time so the milestone marker still exists.
		dispatchedAt = now
	}

	t := message.Triple{
		Subject:    chainEntityID,
		Predicate:  "chain.dispatched_at",
		Object:     dispatchedAt.Format(time.RFC3339),
		Source:     "chain.dispatched_stamper",
		Timestamp:  now,
		Confidence: 1.0,
	}
	if err := s.publisher.AddTriple(ctx, t); err != nil {
		return fmt.Errorf("write chain.dispatched_at on %q: %w", chainEntityID, err)
	}

	s.logger.Info("chain.dispatched_at stamped",
		slog.String("chain_id", ev.LoopID),
		slog.String("chain_entity", chainEntityID),
		slog.String("dispatched_at", dispatchedAt.Format(time.RFC3339)),
		slog.String("dispatch_role", ev.Role))

	return nil
}
