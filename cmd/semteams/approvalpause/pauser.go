package approvalpause

import (
	"context"
	"fmt"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	agvocab "github.com/c360studio/semstreams/vocabulary/agentic"
)

// pauserSource tags every triple this package writes, mirroring chainpause's
// Source convention so an operator can attribute the run-phase approval marker.
const pauserSource = "approvalpause"

const (
	// MarkerApprovalPending is stamped on the RUN entity to record that one of
	// its loops is parked in LoopStateAwaitingApproval on a gated tool call.
	// Rule 12 (agent-run/12-executing-to-awaiting-on-approval.json) consumes it
	// and transitions the run executing→awaiting_approval.
	//
	// DISTINCT from the 4b-2 clarification marker (agent.run.clarification_pending):
	// both pauses land in awaiting_approval, but the two are kept on separate
	// predicates so their resume halves never cross-resume (ADR-053 §4c).
	MarkerApprovalPending = "agent.run.approval_pending"

	// MarkerApprovalResumed is the resume marker the PR-2 half will stamp on the
	// run (the 4c twin of agent.run.clarification_resumed). It is declared here in
	// PR-1 so the pause rule's bounce-proof guard and the disambiguation contract
	// tests reference a single source of truth; PR-1 never stamps it.
	MarkerApprovalResumed = "agent.run.approval_resumed"
)

// lineageRunAnchor is the related_loops-threaded run anchor a run-entity-descended
// loop carries when it lacks a bare agent.run.entity_id (the 4b-1a inherit/threaded
// anchor split — see rules 05/06 and 07/08). The Pauser tries the inherit anchor
// (agent.run.entity_id) first, then falls back to this — absorbing IN GO the
// precedence that the clarification pause had to express as two mutually-exclusive
// rule files (07 run-anchor / 08 lineage-anchor). Because the subscriber reads the
// loop entity directly and can branch in code, the 4c pause RULE needs no such split.
const lineageRunAnchor = "lineage.run-loop-entity-id"

// EntityTripleReader reads a flat predicate→object map for a single graph entity.
// chain.NATSEntityReader satisfies it structurally; a fake satisfies it in tests.
type EntityTripleReader interface {
	ReadEntity(ctx context.Context, entityID string) (map[string]any, error)
}

// TriplePublisher is the narrow write surface the Pauser needs.
// agentictools.NATSTriplePublisher satisfies it structurally.
type TriplePublisher interface {
	AddTriple(ctx context.Context, triple message.Triple) error
}

// PauseResult reports what HandlePending did, for caller logging and test
// assertions. Stamped is true only when a run anchor resolved AND the marker
// write succeeded.
type PauseResult struct {
	LoopID      string
	RunEntityID string
	ToolName    string
	Stamped     bool
}

// Pauser resolves the run anchor of an approval-gated loop and stamps
// agent.run.approval_pending on the run entity. Fail-soft: a malformed loop id,
// a graph-read error, or a run-less loop never panics — a runtime subscriber must
// not crash the dispatch goroutine, and the agent.approval_pending event is on the
// wire regardless (the UI surfaces the loop-level request independently).
type Pauser struct {
	reader    EntityTripleReader
	publisher TriplePublisher
	org       string
	platform  string
}

// NewPauser constructs a Pauser. org/platform are the product's platform identity
// (types.PlatformMeta) — used to reconstruct the 6-part loop-execution entity ID
// from the bare LoopID the ApprovalPendingEvent carries.
func NewPauser(reader EntityTripleReader, pub TriplePublisher, org, platform string) *Pauser {
	return &Pauser{reader: reader, publisher: pub, org: org, platform: platform}
}

// HandlePending is the subscription entry point for agent.approval_pending.* events.
// It reconstructs the loop entity, reads its run anchor, and (when the loop belongs
// to a run) stamps agent.run.approval_pending on the run entity so rule 12 can
// transition the run executing→awaiting_approval.
//
// Returns the PauseResult for caller logging. A run-less loop returns a
// zero-Stamped result with a nil error (benign no-op). Errors (malformed loop id,
// graph-read failure, triple-write failure) are returned wrapped so the subscriber
// can log them without aborting the subscription.
func (p *Pauser) HandlePending(ctx context.Context, ev *agentic.ApprovalPendingEvent) (PauseResult, error) {
	if ev == nil || ev.LoopID == "" {
		return PauseResult{}, nil
	}
	res := PauseResult{LoopID: ev.LoopID, ToolName: ev.ToolName}

	// Reconstruct the loop-execution entity (org.platform.agent.agentic-loop.
	// execution.<loopID>). Use the error-returning variant — a malformed loop id
	// must fail soft, not panic the subscription goroutine (ADR-036 Stage 3.8).
	loopEntityID, err := agentic.TryLoopExecutionEntityID(p.org, p.platform, ev.LoopID)
	if err != nil {
		return res, fmt.Errorf("approvalpause: build loop entity id for %q: %w", ev.LoopID, err)
	}

	triples, err := p.reader.ReadEntity(ctx, loopEntityID)
	if err != nil {
		return res, fmt.Errorf("approvalpause: read loop entity %q: %w", loopEntityID, err)
	}

	runEntityID := resolveRunAnchor(triples)
	if runEntityID == "" {
		// Run-less loop (front-door single coordinator answering plain chat, or a
		// standalone loop): no run to pause. No-op — mirrors the front-door
		// decide(ask_user) no-op (rules 07/08 run-anchor guard).
		return res, nil
	}
	res.RunEntityID = runEntityID

	now := time.Now().UTC()
	triple := message.Triple{
		Subject:   runEntityID,
		Predicate: MarkerApprovalPending,
		// Record WHICH loop is gated as a navigable 6-part loop-entity ref (mirrors
		// agent.loop.parent / reply_to). The PR-2 resume reads the same loop's run
		// anchor off agent.approval_response.<loopID>, so the object is audit-only.
		Object:     loopEntityID,
		Source:     pauserSource,
		Timestamp:  now,
		Confidence: 1.0,
	}
	if err := p.publisher.AddTriple(ctx, triple); err != nil {
		return res, fmt.Errorf("approvalpause: stamp %s on %q: %w", MarkerApprovalPending, runEntityID, err)
	}
	res.Stamped = true
	return res, nil
}

// resolveRunAnchor returns the run entity a gated loop belongs to, in precedence
// order: (1) agent.run.entity_id (the inherit anchor stamped at spawn by
// buildSpawnIdentityTriples when the loop carries a RunID); (2)
// lineage.run-loop-entity-id (the threaded anchor a run-entity-descended loop
// carries via related_loops). Returns "" when neither is present (a run-less loop).
func resolveRunAnchor(triples map[string]any) string {
	if v, ok := triples[agvocab.LoopRunEntityID].(string); ok && v != "" {
		return v
	}
	if v, ok := triples[lineageRunAnchor].(string); ok && v != "" {
		return v
	}
	return ""
}
