package approvalpause

import (
	"context"
	"errors"
	"testing"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
)

const (
	testOrg      = "c360"
	testPlatform = "ops"
)

// fakeReader returns a fixed triple map (or error) for any entity, capturing the
// entity ID it was asked to read so tests can assert the loop-entity reconstruction.
type fakeReader struct {
	triples    map[string]any
	err        error
	lastReadID string
}

func (f *fakeReader) ReadEntity(_ context.Context, entityID string) (map[string]any, error) {
	f.lastReadID = entityID
	if f.err != nil {
		return nil, f.err
	}
	return f.triples, nil
}

// fakePublisher records every triple written and can be primed to fail.
type fakePublisher struct {
	written []message.Triple
	err     error
}

func (f *fakePublisher) AddTriple(_ context.Context, t message.Triple) error {
	if f.err != nil {
		return f.err
	}
	f.written = append(f.written, t)
	return nil
}

func newEvent(loopID string) *agentic.ApprovalPendingEvent {
	return &agentic.ApprovalPendingEvent{
		LoopID:   loopID,
		CallID:   "call-1",
		ToolName: "create_rule",
	}
}

// TestHandlePending_InheritAnchor: a loop carrying agent.run.entity_id (the inherit
// anchor) gets agent.run.approval_pending stamped on that run entity, with the loop
// entity as the navigable object.
func TestHandlePending_InheritAnchor(t *testing.T) {
	const runEntity = "c360.ops.agent.chain.execution.run-123"
	reader := &fakeReader{triples: map[string]any{"agent.run.entity_id": runEntity}}
	pub := &fakePublisher{}
	p := NewPauser(reader, pub, testOrg, testPlatform)

	res, err := p.HandlePending(context.Background(), newEvent("loop-abc"))
	if err != nil {
		t.Fatalf("HandlePending error: %v", err)
	}
	if !res.Stamped {
		t.Fatal("expected Stamped=true for a run-anchored loop")
	}
	if res.RunEntityID != runEntity {
		t.Errorf("RunEntityID = %q, want %q", res.RunEntityID, runEntity)
	}
	// The reader must be asked for the reconstructed 6-part loop entity.
	wantLoopEntity := agentic.LoopExecutionEntityID(testOrg, testPlatform, "loop-abc")
	if reader.lastReadID != wantLoopEntity {
		t.Errorf("read entity = %q, want %q", reader.lastReadID, wantLoopEntity)
	}
	if len(pub.written) != 1 {
		t.Fatalf("want exactly 1 triple written, got %d", len(pub.written))
	}
	got := pub.written[0]
	if got.Subject != runEntity {
		t.Errorf("triple subject = %q, want run entity %q", got.Subject, runEntity)
	}
	if got.Predicate != MarkerApprovalPending {
		t.Errorf("triple predicate = %q, want %q", got.Predicate, MarkerApprovalPending)
	}
	if got.Object != wantLoopEntity {
		t.Errorf("triple object = %q, want gated loop entity %q", got.Object, wantLoopEntity)
	}
	if got.Source != pauserSource {
		t.Errorf("triple source = %q, want %q", got.Source, pauserSource)
	}
}

// TestHandlePending_LineageFallback: a threaded loop carrying ONLY
// lineage.run-loop-entity-id (no bare agent.run.entity_id) resolves via the Go-side
// fallback — the precedence that rules 07/08 split across two files.
func TestHandlePending_LineageFallback(t *testing.T) {
	const runEntity = "c360.ops.agent.chain.execution.run-xyz"
	reader := &fakeReader{triples: map[string]any{"lineage.run-loop-entity-id": runEntity}}
	pub := &fakePublisher{}
	p := NewPauser(reader, pub, testOrg, testPlatform)

	res, err := p.HandlePending(context.Background(), newEvent("loop-def"))
	if err != nil {
		t.Fatalf("HandlePending error: %v", err)
	}
	if !res.Stamped || res.RunEntityID != runEntity {
		t.Fatalf("lineage fallback failed: Stamped=%v RunEntityID=%q", res.Stamped, res.RunEntityID)
	}
}

// TestHandlePending_AnchorPrecedence: when BOTH anchors are present the inherit
// anchor (agent.run.entity_id) wins — same precedence as runanchor.ChainEntityID.
func TestHandlePending_AnchorPrecedence(t *testing.T) {
	const inherit = "c360.ops.agent.chain.execution.inherit-run"
	const lineage = "c360.ops.agent.chain.execution.lineage-run"
	reader := &fakeReader{triples: map[string]any{
		"agent.run.entity_id":        inherit,
		"lineage.run-loop-entity-id": lineage,
	}}
	pub := &fakePublisher{}
	p := NewPauser(reader, pub, testOrg, testPlatform)

	res, err := p.HandlePending(context.Background(), newEvent("loop-ghi"))
	if err != nil {
		t.Fatalf("HandlePending error: %v", err)
	}
	if res.RunEntityID != inherit {
		t.Errorf("precedence: RunEntityID = %q, want inherit anchor %q", res.RunEntityID, inherit)
	}
}

// TestHandlePending_RunlessLoop: a loop with neither anchor (front-door coordinator
// answering plain chat) is a benign no-op — no stamp, no error.
func TestHandlePending_RunlessLoop(t *testing.T) {
	reader := &fakeReader{triples: map[string]any{"agent.loop.role": "coordinator"}}
	pub := &fakePublisher{}
	p := NewPauser(reader, pub, testOrg, testPlatform)

	res, err := p.HandlePending(context.Background(), newEvent("loop-frontdoor"))
	if err != nil {
		t.Fatalf("run-less loop must not error, got %v", err)
	}
	if res.Stamped {
		t.Error("run-less loop must not stamp a marker")
	}
	if len(pub.written) != 0 {
		t.Errorf("run-less loop wrote %d triples, want 0", len(pub.written))
	}
}

// TestHandlePending_EmptyLoopID: a nil/empty event is a no-op (defensive).
func TestHandlePending_EmptyLoopID(t *testing.T) {
	p := NewPauser(&fakeReader{}, &fakePublisher{}, testOrg, testPlatform)
	if _, err := p.HandlePending(context.Background(), newEvent("")); err != nil {
		t.Errorf("empty loop id must be a no-op, got %v", err)
	}
	if _, err := p.HandlePending(context.Background(), nil); err != nil {
		t.Errorf("nil event must be a no-op, got %v", err)
	}
}

// TestHandlePending_MalformedLoopID: a loop id that can't form a valid entity must
// fail soft (error returned, no panic) — a runtime subscriber must never crash the
// dispatch goroutine.
func TestHandlePending_MalformedLoopID(t *testing.T) {
	reader := &fakeReader{}
	pub := &fakePublisher{}
	p := NewPauser(reader, pub, testOrg, testPlatform)

	// A dot in the loop id breaks the 6-part entity construction.
	_, err := p.HandlePending(context.Background(), newEvent("bad.loop.id"))
	if err == nil {
		t.Fatal("expected an error for a malformed loop id")
	}
	if reader.lastReadID != "" {
		t.Error("must not attempt a graph read for a malformed loop id")
	}
	if len(pub.written) != 0 {
		t.Error("must not write any triple for a malformed loop id")
	}
}

// TestHandlePending_ReaderError: a graph-read failure surfaces as a wrapped error
// and writes nothing.
func TestHandlePending_ReaderError(t *testing.T) {
	reader := &fakeReader{err: errors.New("graph unavailable")}
	pub := &fakePublisher{}
	p := NewPauser(reader, pub, testOrg, testPlatform)

	_, err := p.HandlePending(context.Background(), newEvent("loop-err"))
	if err == nil {
		t.Fatal("expected an error when the reader fails")
	}
	if len(pub.written) != 0 {
		t.Error("must not write any triple when the reader fails")
	}
}

// TestHandlePending_PublisherError: a triple-write failure surfaces as a wrapped
// error with Stamped=false.
func TestHandlePending_PublisherError(t *testing.T) {
	reader := &fakeReader{triples: map[string]any{"agent.run.entity_id": "c360.ops.agent.chain.execution.run-1"}}
	pub := &fakePublisher{err: errors.New("nats down")}
	p := NewPauser(reader, pub, testOrg, testPlatform)

	res, err := p.HandlePending(context.Background(), newEvent("loop-pub"))
	if err == nil {
		t.Fatal("expected an error when the publisher fails")
	}
	if res.Stamped {
		t.Error("Stamped must be false when the write fails")
	}
}
