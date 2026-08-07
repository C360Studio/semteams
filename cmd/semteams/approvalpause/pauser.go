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
	// DISTINCT from the 4b-2 clarification marker (agent.run.clarification-pending):
	// both pauses land in awaiting_approval, but the two are kept on separate
	// predicates so their resume halves never cross-resume (ADR-053 §4c).
	MarkerApprovalPending = "agent.run.approval-pending"

	// MarkerApprovalResumed is the resume marker the PR-2 half will stamp on the
	// run (the 4c twin of agent.run.clarification-resumed). It is declared here in
	// PR-1 so the pause rule's bounce-proof guard and the disambiguation contract
	// tests reference a single source of truth; PR-1 never stamps it.
	MarkerApprovalResumed = "agent.run.approval-resumed"
)

// lineageRunAnchor is the related_loops-threaded run anchor a run-entity-descended
// loop carries when it lacks a bare agent.run.entity-id (the 4b-1a inherit/threaded
// anchor split — see rules 05/06 and 07/08). The Pauser tries the inherit anchor
// (agent.run.entity-id) first, then falls back to this — absorbing IN GO the
// precedence that the clarification pause had to express as two mutually-exclusive
// rule files (07 run-anchor / 08 lineage-anchor). Because the subscriber reads the
// loop entity directly and can branch in code, the 4c pause RULE needs no such split.
const lineageRunAnchor = "agent.lineage.run-loop-entity-id"

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

// ResumeResult reports what HandleResponse did (the 4c PR-2 resume half).
// Stamped is true only when a run anchor resolved AND the agent.run.approval-resumed
// write succeeded. Decision is the human's approve/reject/modify decision — carried
// for logging; the run resumes regardless of decision (any response means the loop
// is no longer parked in awaiting_approval).
type ResumeResult struct {
	LoopID      string
	RunEntityID string
	Decision    string
	Stamped     bool
}

// Pauser resolves the run anchor of an approval-gated loop and stamps the run-phase
// approval markers on the run entity — agent.run.approval-pending when the loop
// PAUSES (HandlePending), agent.run.approval-resumed when the human responds
// (HandleResponse, PR-2). Both halves share one anchor-resolution path. Fail-soft:
// a malformed loop id, a graph-read error, or a run-less loop never panics — a
// runtime subscriber must not crash the dispatch goroutine, and the wire events are
// delivered regardless (the UI surfaces the loop-level request independently).
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
// to a run) stamps agent.run.approval-pending on the run entity so rule 12 can
// transition the run executing→awaiting_approval.
//
// Returns the PauseResult for caller logging. A run-less loop returns a
// zero-Stamped result with a nil error (benign no-op). Errors (malformed loop id,
// graph-read failure, triple-write failure) are returned wrapped so the subscriber
// can log them without aborting the subscription.
func (p *Pauser) HandlePending(ctx context.Context, ev *agentic.ApprovalPendingEvent) (PauseResult, error) {
	if ev == nil {
		return PauseResult{}, nil
	}
	runEntityID, stamped, err := p.stampRunMarker(ctx, ev.LoopID, MarkerApprovalPending)
	return PauseResult{LoopID: ev.LoopID, RunEntityID: runEntityID, ToolName: ev.ToolName, Stamped: stamped}, err
}

// HandleResponse is the subscription entry point for agent.approval_response.*
// events (4c PR-2). The human's approve/reject/modify decision moves the gated loop
// out of LoopStateAwaitingApproval (the framework re-injects the result and the loop
// continues), so the RUN should resume executing. This stamps
// agent.run.approval-resumed on the run entity; rule agent-run/13 consumes it and
// transitions the run awaiting_approval→executing, clearing both approval markers.
//
// The marker is stamped regardless of decision — approve, reject, AND modify all
// un-park the loop, so all three resume the run. A run-less loop is a benign no-op.
func (p *Pauser) HandleResponse(ctx context.Context, ev *agentic.ApprovalResponse) (ResumeResult, error) {
	if ev == nil {
		return ResumeResult{}, nil
	}
	runEntityID, stamped, err := p.stampRunMarker(ctx, ev.LoopID, MarkerApprovalResumed)
	return ResumeResult{LoopID: ev.LoopID, RunEntityID: runEntityID, Decision: ev.Decision, Stamped: stamped}, err
}

// stampRunMarker is the shared anchor-resolve-and-stamp path for both the pause
// (HandlePending) and resume (HandleResponse) halves. It reconstructs the loop
// entity, reads its run anchor, and (when the loop belongs to a run) stamps
// `predicate` = the loop entity on the run entity. Returns the resolved run entity
// ("" for a run-less loop, a benign no-op) and whether the marker was written.
//
// Uses the error-returning TryLoopExecutionEntityID — a malformed loop id must fail
// soft, not panic the subscription goroutine (ADR-036 Stage 3.8). The marker object
// is the navigable 6-part loop-entity ref (mirrors agent.loop.parent / reply_to);
// it is audit-only (the rules key on predicate presence, not object).
func (p *Pauser) stampRunMarker(ctx context.Context, loopID, predicate string) (runEntityID string, stamped bool, err error) {
	if loopID == "" {
		return "", false, nil
	}
	loopEntityID, err := agentic.TryLoopExecutionEntityID(p.org, p.platform, loopID)
	if err != nil {
		return "", false, fmt.Errorf("approvalpause: build loop entity id for %q: %w", loopID, err)
	}

	triples, err := p.reader.ReadEntity(ctx, loopEntityID)
	if err != nil {
		return "", false, fmt.Errorf("approvalpause: read loop entity %q: %w", loopEntityID, err)
	}

	runEntityID = resolveRunAnchor(triples)
	if runEntityID == "" {
		// Run-less loop (front-door single coordinator, or a standalone loop): no
		// run to pause/resume. No-op — mirrors the front-door decide(ask_user) no-op.
		return "", false, nil
	}

	now := time.Now().UTC()
	triple := message.Triple{
		Subject:    runEntityID,
		Predicate:  predicate,
		Object:     loopEntityID,
		Source:     pauserSource,
		Timestamp:  now,
		Confidence: 1.0,
	}
	if err := p.publisher.AddTriple(ctx, triple); err != nil {
		return runEntityID, false, fmt.Errorf("approvalpause: stamp %s on %q: %w", predicate, runEntityID, err)
	}
	return runEntityID, true, nil
}

// resolveRunAnchor returns the run entity a gated loop belongs to, in precedence
// order: (1) agent.run.entity-id (the inherit anchor stamped at spawn by
// buildSpawnIdentityTriples when the loop carries a RunID); (2)
// agent.lineage.run-loop-entity-id (the threaded anchor a run-entity-descended loop
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
