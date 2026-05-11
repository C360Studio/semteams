package chain

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
)

// recordingPublisher records AddTriple calls for assertion. Mirrors
// chainpause/pauser_test.go's recordingPublisher to keep this package's
// test conventions consistent with sibling packages.
type recordingPublisher struct {
	mu      sync.Mutex
	triples []message.Triple
	err     error // if non-nil, returned on every call
}

func (r *recordingPublisher) AddTriple(_ context.Context, t message.Triple) error {
	if r.err != nil {
		return r.err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.triples = append(r.triples, t)
	return nil
}

func (r *recordingPublisher) byPredicate(pred string) (message.Triple, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range r.triples {
		if t.Predicate == pred {
			return t, true
		}
	}
	return message.Triple{}, false
}

// TestDispatchedStamper_RootStampsTriple: a chain-root completion event
// (ParentLoopID empty) writes chain.dispatched.at on the chain entity.
func TestDispatchedStamper_RootStampsTriple(t *testing.T) {
	pub := &recordingPublisher{}
	s := NewDispatchedStamper(pub, testPlatform(), nil)

	completedAt := time.Date(2026, 5, 7, 14, 30, 0, 0, time.UTC)
	ev := &agentic.LoopCompletedEvent{
		LoopID:       "dispatch_abc",
		Role:         "dispatch",
		ParentLoopID: "",
		CompletedAt:  completedAt,
	}

	if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, ok := pub.byPredicate("chain.dispatched.at")
	if !ok {
		t.Fatal("chain.dispatched.at not written")
	}
	wantSubject := "c360.test.agent.chain.execution.dispatch_abc"
	if got.Subject != wantSubject {
		t.Errorf("Subject: got %q, want %q", got.Subject, wantSubject)
	}
	objStr, ok := got.Object.(string)
	if !ok {
		t.Fatalf("Object should be string, got %T", got.Object)
	}
	if objStr != "2026-05-07T14:30:00Z" {
		t.Errorf("Object: got %q, want RFC3339 of CompletedAt", objStr)
	}
	if got.Source != "chain.dispatched" {
		t.Errorf("Source: got %q, want chain.dispatched", got.Source)
	}
	if got.Confidence != 1.0 {
		t.Errorf("Confidence: got %v, want 1.0", got.Confidence)
	}
}

// TestDispatchedStamper_NonRootIsNoop: when a non-root loop completes
// (ParentLoopID set), the stamper writes nothing — other milestone
// handlers will pick the event up.
func TestDispatchedStamper_NonRootIsNoop(t *testing.T) {
	pub := &recordingPublisher{}
	s := NewDispatchedStamper(pub, testPlatform(), nil)

	ev := &agentic.LoopCompletedEvent{
		LoopID:       "researcher_1",
		Role:         "researcher",
		ParentLoopID: "dispatch_abc",
		CompletedAt:  time.Now().UTC(),
	}

	if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pub.triples) != 0 {
		t.Errorf("expected no triples for non-root completion, got %d", len(pub.triples))
	}
}

// TestDispatchedStamper_NilEventIsNoop: defensive — a nil event from a
// malformed envelope must not panic the subscriber goroutine.
func TestDispatchedStamper_NilEventIsNoop(t *testing.T) {
	pub := &recordingPublisher{}
	s := NewDispatchedStamper(pub, testPlatform(), nil)

	if err := s.HandleLoopCompleted(context.Background(), nil); err != nil {
		t.Fatalf("unexpected error on nil event: %v", err)
	}
	if len(pub.triples) != 0 {
		t.Errorf("expected no triples for nil event, got %d", len(pub.triples))
	}
}

// TestDispatchedStamper_RejectsDottedLoopID: a malformed loop_id from
// the wire (containing dots) would panic agentic.ChainExecutionEntityID.
// validateLoopID rejects it and surfaces the error to the caller.
func TestDispatchedStamper_RejectsDottedLoopID(t *testing.T) {
	pub := &recordingPublisher{}
	s := NewDispatchedStamper(pub, testPlatform(), nil)

	ev := &agentic.LoopCompletedEvent{
		LoopID:       "loop.with.dots",
		Role:         "dispatch",
		ParentLoopID: "",
		CompletedAt:  time.Now().UTC(),
	}

	err := s.HandleLoopCompleted(context.Background(), ev)
	if err == nil {
		t.Fatal("expected error on dotted loopID, got nil")
	}
	if len(pub.triples) != 0 {
		t.Errorf("expected no triples written when loop_id is malformed, got %d", len(pub.triples))
	}
}

// TestDispatchedStamper_PropagatesPublishError: a publisher failure
// surfaces with chain entity context for log-driven debugging.
func TestDispatchedStamper_PropagatesPublishError(t *testing.T) {
	pub := &recordingPublisher{err: errors.New("graph mutation refused")}
	s := NewDispatchedStamper(pub, testPlatform(), nil)

	ev := &agentic.LoopCompletedEvent{
		LoopID:       "dispatch_xyz",
		Role:         "dispatch",
		ParentLoopID: "",
		CompletedAt:  time.Now().UTC(),
	}

	err := s.HandleLoopCompleted(context.Background(), ev)
	if err == nil {
		t.Fatal("expected publish error to surface")
	}
}

// TestDispatchedStamper_FallsBackOnZeroCompletedAt: a zero CompletedAt
// would render the triple useless. Stamper falls back to observation
// time so the milestone marker still exists, AND stamps a companion
// chain.dispatched.observed_fallback="true" triple so a graph-only
// downstream consumer (ops agent, future analytics) can distinguish
// "real wire timestamp" from "subscriber-observed fallback" without
// parsing logs. Smoke-grade defense; we don't expect framework to send
// zero timestamps but a malformed event should not silently produce a
// bad triple AND no breadcrumb.
func TestDispatchedStamper_FallsBackOnZeroCompletedAt(t *testing.T) {
	pub := &recordingPublisher{}
	s := NewDispatchedStamper(pub, testPlatform(), nil)

	ev := &agentic.LoopCompletedEvent{
		LoopID:       "dispatch_zero",
		Role:         "dispatch",
		ParentLoopID: "",
		// CompletedAt left zero
	}

	if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, ok := pub.byPredicate("chain.dispatched.at")
	if !ok {
		t.Fatal("chain.dispatched.at not written")
	}
	objStr, ok := got.Object.(string)
	if !ok {
		t.Fatalf("chain.dispatched.at Object should be string, got %T", got.Object)
	}
	if objStr == "0001-01-01T00:00:00Z" {
		t.Errorf("Object should not be zero-time RFC3339; got %q", objStr)
	}
	parsed, err := time.Parse(time.RFC3339, objStr)
	if err != nil {
		t.Fatalf("Object should be valid RFC3339, got %q: %v", objStr, err)
	}
	if time.Since(parsed) > time.Minute {
		t.Errorf("fallback timestamp should be recent, got %v", parsed)
	}

	// The companion fallback marker must be written so downstream graph
	// consumers can detect the framework bug without parsing logs.
	marker, ok := pub.byPredicate("chain.dispatched.observed_fallback")
	if !ok {
		t.Fatal("chain.dispatched.observed_fallback companion triple missing")
	}
	if marker.Object != "true" {
		t.Errorf("observed_fallback Object: got %v, want \"true\"", marker.Object)
	}
	if marker.Subject != got.Subject {
		t.Errorf("observed_fallback Subject must match dispatched_at Subject; got %q vs %q", marker.Subject, got.Subject)
	}
}

// TestDispatchedStamper_NoFallbackMarkerOnHappyPath confirms the
// observed_fallback companion triple is NOT written when CompletedAt
// is non-zero (the normal case). Avoids polluting graph queries with
// fallback markers on healthy chains.
func TestDispatchedStamper_NoFallbackMarkerOnHappyPath(t *testing.T) {
	pub := &recordingPublisher{}
	s := NewDispatchedStamper(pub, testPlatform(), nil)

	ev := &agentic.LoopCompletedEvent{
		LoopID:       "dispatch_ok",
		Role:         "dispatch",
		ParentLoopID: "",
		CompletedAt:  time.Now().UTC(),
	}

	if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := pub.byPredicate("chain.dispatched.observed_fallback"); ok {
		t.Error("observed_fallback marker must not be written on happy path")
	}
}
