package approvalpause

import (
	"context"
	"fmt"

	"github.com/c360studio/semstreams/agentic"
	agvocab "github.com/c360studio/semstreams/vocabulary/agentic"
)

// pauserSource tags every triple this package writes, mirroring chainpause's
// Source convention so an operator can attribute the run-phase approval marker.
const pauserSource = "approvalpause"

const (
	// MarkerApprovalPending records every observed gate as a JSON string pair
	// [loopID, executionID]. It is append-only, not a loop-entity edge.
	MarkerApprovalPending = "agent.run.approval-pending"

	// MarkerApprovalAnswered records answered pairs. A response appends to BOTH
	// sets atomically, preserving answered ⊆ pending even if it arrives first.
	MarkerApprovalAnswered = "agent.run.approval-answered"
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

// GateProjection atomically records a gate against the exact run authority.
type GateProjection interface {
	RecordApproval(ctx context.Context, runID, loopID, executionID string, answered bool) (bool, error)
}

// PauseResult reports what HandlePending did, for caller logging and test
// assertions. Stamped is true only when a run anchor resolved AND the marker
// write succeeded.
type PauseResult struct {
	LoopID          string
	RunEntityID     string
	ToolName        string
	Stamped         bool
	AlreadyAnswered bool
}

// ResumeResult reports whether a response's execution-specific receipt was
// persisted. Rule 13 resumes only when every observed gate has an answer and
// the current run pause was caused by rule 12.
type ResumeResult struct {
	LoopID          string
	RunEntityID     string
	Decision        string
	Stamped         bool
	AlreadyAnswered bool
}

// Pauser resolves each approval event's run anchor and appends execution-specific
// pending/answered facts through the framework's owned graph store. Rule 12 and
// rule 13 derive the phase from those sets; this subscriber never clears facts.
// Core-NATS delivery remains best effort, so a lost event can still need repair.
type Pauser struct {
	reader     EntityTripleReader
	projection GateProjection
	org        string
	platform   string
}

// NewPauser constructs a Pauser. org/platform are the product's platform identity
// (types.PlatformMeta) — used to reconstruct the 6-part loop-execution entity ID
// from the bare LoopID the ApprovalPendingEvent carries.
func NewPauser(reader EntityTripleReader, projection GateProjection, org, platform string) *Pauser {
	return &Pauser{reader: reader, projection: projection, org: org, platform: platform}
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
	if ctx == nil {
		return PauseResult{}, fmt.Errorf("approvalpause: context is required")
	}
	if ev == nil || ev.LoopID == "" {
		return PauseResult{}, nil
	}
	runEntityID, stamped, answered, err := p.stampRunMarker(ctx, ev.LoopID, ev.ExecutionID, MarkerApprovalPending)
	return PauseResult{LoopID: ev.LoopID, RunEntityID: runEntityID, ToolName: ev.ToolName, Stamped: stamped, AlreadyAnswered: answered}, err
}

// HandleResponse records a valid approve/reject/modify answer. Both the observed
// gate and its answer are appended in one run-entity mutation. This makes an
// answer delivered before its pending event safe without resuming another gate.
func (p *Pauser) HandleResponse(ctx context.Context, ev *agentic.ApprovalResponse) (ResumeResult, error) {
	if ctx == nil {
		return ResumeResult{}, fmt.Errorf("approvalpause: context is required")
	}
	if ev == nil || ev.LoopID == "" {
		return ResumeResult{}, nil
	}
	switch ev.Decision {
	case agentic.ApprovalDecisionApprove, agentic.ApprovalDecisionReject, agentic.ApprovalDecisionModify:
	default:
		return ResumeResult{}, fmt.Errorf("approvalpause: invalid approval decision %q", ev.Decision)
	}
	runEntityID, stamped, answered, err := p.stampRunMarker(ctx, ev.LoopID, ev.ExecutionID, MarkerApprovalAnswered)
	return ResumeResult{LoopID: ev.LoopID, RunEntityID: runEntityID, Decision: ev.Decision, Stamped: stamped, AlreadyAnswered: answered}, err
}

// stampRunMarker writes on the existing run entity through the framework's
// revision-fenced approval projection. Event order and concurrent process instances
// are resolved at the graph owner, without relying on a local callback mutex.
func (p *Pauser) stampRunMarker(ctx context.Context, loopID, executionID, predicate string) (runEntityID string, stamped, answered bool, err error) {
	if executionID == "" {
		return "", false, false, fmt.Errorf("approvalpause: execution_id is required")
	}
	loopEntityID, err := agentic.TryLoopExecutionEntityID(p.org, p.platform, loopID)
	if err != nil {
		return "", false, false, fmt.Errorf("approvalpause: build loop entity id for %q: %w", loopID, err)
	}

	if err := ctx.Err(); err != nil {
		return "", false, false, err
	}
	triples, err := p.reader.ReadEntity(ctx, loopEntityID)
	if err != nil {
		return "", false, false, fmt.Errorf("approvalpause: read loop entity %q: %w", loopEntityID, err)
	}
	runEntityID = resolveRunAnchor(triples)
	if runEntityID == "" {
		return "", false, false, nil
	}

	answered, err = p.projection.RecordApproval(ctx, runEntityID, loopID, executionID, predicate == MarkerApprovalAnswered)
	if err != nil {
		return runEntityID, false, false, fmt.Errorf("approvalpause: record gate on %q: %w", runEntityID, err)
	}
	return runEntityID, !answered, answered, nil
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
