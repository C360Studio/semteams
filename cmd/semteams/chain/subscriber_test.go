package chain

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/c360studio/semstreams/agentic"
)

// recordingHandler captures HandleLoopCompleted calls for assertion.
// errOn is keyed by loop_id; if a key matches, the configured error is
// returned for that call. panicOn forces a panic for the matching key
// (covers the contract-violation defensive path in invokeHandler).
type recordingHandler struct {
	calls   atomic.Int64
	seen    []*agentic.LoopCompletedEvent
	errOn   map[string]error
	panicOn map[string]bool
}

func (r *recordingHandler) HandleLoopCompleted(_ context.Context, ev *agentic.LoopCompletedEvent) error {
	r.calls.Add(1)
	r.seen = append(r.seen, ev)
	if r.panicOn != nil && r.panicOn[ev.LoopID] {
		panic("contract violation: handler panicked on " + ev.LoopID)
	}
	if err, ok := r.errOn[ev.LoopID]; ok {
		return err
	}
	return nil
}

// envelope shape mirrors the wire format the subscriber decodes —
// matches semstreams BaseMessage envelope without importing the
// framework's helpers (which would require a richer NATS test fixture).
type testEnvelope struct {
	Type struct {
		Category string `json:"category"`
	} `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

func encodeCompletion(t *testing.T, ev *agentic.LoopCompletedEvent) []byte {
	t.Helper()
	payload, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	env := testEnvelope{Payload: payload}
	env.Type.Category = agentic.CategoryLoopCompleted
	out, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	return out
}

func TestCompletionSubscriber_DispatchesToAllHandlers(t *testing.T) {
	h1 := &recordingHandler{}
	h2 := &recordingHandler{}
	s := NewCompletionSubscriber([]CompletionHandler{h1, h2}, nil)

	ev := &agentic.LoopCompletedEvent{LoopID: "loop_a", Role: "dispatch"}
	s.handleMsg(context.Background(), encodeCompletion(t, ev))

	if got := h1.calls.Load(); got != 1 {
		t.Errorf("h1: got %d calls, want 1", got)
	}
	if got := h2.calls.Load(); got != 1 {
		t.Errorf("h2: got %d calls, want 1", got)
	}
}

func TestCompletionSubscriber_HandlerErrorDoesNotAbortSiblings(t *testing.T) {
	h1 := &recordingHandler{errOn: map[string]error{"loop_a": errors.New("write failed")}}
	h2 := &recordingHandler{}
	s := NewCompletionSubscriber([]CompletionHandler{h1, h2}, nil)

	ev := &agentic.LoopCompletedEvent{LoopID: "loop_a", Role: "dispatch"}
	s.handleMsg(context.Background(), encodeCompletion(t, ev))

	// Both handlers must run despite h1 returning an error.
	if got := h1.calls.Load(); got != 1 {
		t.Errorf("h1: got %d calls, want 1", got)
	}
	if got := h2.calls.Load(); got != 1 {
		t.Errorf("h2 should have run despite h1 failure; got %d calls, want 1", got)
	}
}

func TestCompletionSubscriber_SkipsNonCompletedCategory(t *testing.T) {
	h := &recordingHandler{}
	s := NewCompletionSubscriber([]CompletionHandler{h}, nil)

	// Synthesize a tool-result-shaped envelope on the same subject.
	env := testEnvelope{Payload: []byte("{}")}
	env.Type.Category = "tool_result"
	data, _ := json.Marshal(env)
	s.handleMsg(context.Background(), data)

	if got := h.calls.Load(); got != 0 {
		t.Errorf("expected 0 handler calls for non-completed category, got %d", got)
	}
}

func TestCompletionSubscriber_SkipsMalformedEnvelope(t *testing.T) {
	h := &recordingHandler{}
	s := NewCompletionSubscriber([]CompletionHandler{h}, nil)

	// Garbage bytes — must not panic, must not call handler.
	s.handleMsg(context.Background(), []byte("not json"))
	if got := h.calls.Load(); got != 0 {
		t.Errorf("expected 0 handler calls for malformed envelope, got %d", got)
	}
}

func TestCompletionSubscriber_NilHandlersFiltered(t *testing.T) {
	h := &recordingHandler{}
	// Pass nil mixed with a real handler — nil should be filtered, real
	// one should still receive events.
	s := NewCompletionSubscriber([]CompletionHandler{nil, h, nil}, nil)

	ev := &agentic.LoopCompletedEvent{LoopID: "loop_a"}
	s.handleMsg(context.Background(), encodeCompletion(t, ev))

	if got := h.calls.Load(); got != 1 {
		t.Errorf("real handler: got %d calls, want 1", got)
	}
	// No way to assert "did not panic on nil" except the test reaching here.
}

// TestCompletionSubscriber_PanickingHandlerDoesNotKillSubscription
// pins the contract that a panic in one handler is recovered, logged,
// and does not abort siblings. Without this, a single buggy handler
// would silently halt every chain milestone write across the
// deployment by killing the NATS callback goroutine.
func TestCompletionSubscriber_PanickingHandlerDoesNotKillSubscription(t *testing.T) {
	h1 := &recordingHandler{panicOn: map[string]bool{"loop_a": true}}
	h2 := &recordingHandler{}
	s := NewCompletionSubscriber([]CompletionHandler{h1, h2}, nil)

	ev := &agentic.LoopCompletedEvent{LoopID: "loop_a", Role: "dispatch"}
	// handleMsg must not propagate the panic; if it does, the test fails
	// here with "test panicked".
	s.handleMsg(context.Background(), encodeCompletion(t, ev))

	if got := h1.calls.Load(); got != 1 {
		t.Errorf("h1: got %d calls, want 1 (panic still increments before defer fires)", got)
	}
	if got := h2.calls.Load(); got != 1 {
		t.Errorf("h2 should run after h1 panic; got %d calls, want 1", got)
	}
}
