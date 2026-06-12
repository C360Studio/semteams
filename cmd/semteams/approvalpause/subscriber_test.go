package approvalpause

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
)

// envelopeFor wraps a payload in the BaseMessage shape the subscriber decodes.
func envelopeFor(t *testing.T, payload message.Payload) []byte {
	t.Helper()
	env := message.NewBaseMessage(payload.Schema(), payload, "agentic-loop")
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	return data
}

// TestHandleMsg_ApprovalPending decodes a real ApprovalPendingEvent envelope and
// drives the Pauser — the wire-decode half of the subscriber, exercising the
// category gate + payload unmarshal against the upstream marshaller.
func TestHandleMsg_ApprovalPending(t *testing.T) {
	const runEntity = "c360.ops.agent.chain.execution.run-77"
	reader := &fakeReader{triples: map[string]any{"agent.run.entity_id": runEntity}}
	pub := &fakePublisher{}
	sub := NewSubscriber(NewPauser(reader, pub, testOrg, testPlatform), "", nil)

	sub.handleMsg(context.Background(), envelopeFor(t, &agentic.ApprovalPendingEvent{
		LoopID:   "loop-77",
		CallID:   "call-77",
		ToolName: "create_rule",
	}))

	if len(pub.written) != 1 {
		t.Fatalf("want 1 triple written from a decoded approval_pending event, got %d", len(pub.written))
	}
	if pub.written[0].Subject != runEntity || pub.written[0].Predicate != MarkerApprovalPending {
		t.Errorf("decoded event stamped wrong triple: %+v", pub.written[0])
	}
}

// TestHandleMsg_SkipsOtherCategory: a non-approval_pending envelope on the wildcard
// is ignored (no read, no write).
func TestHandleMsg_SkipsOtherCategory(t *testing.T) {
	reader := &fakeReader{triples: map[string]any{"agent.run.entity_id": "x"}}
	pub := &fakePublisher{}
	sub := NewSubscriber(NewPauser(reader, pub, testOrg, testPlatform), "", nil)

	// A different category on the same wildcard — must be skipped. Hand-built so
	// the test does not depend on another payload's validation rules.
	sub.handleMsg(context.Background(), []byte(`{"type":{"category":"loop_failed"},"payload":{}}`))

	if reader.lastReadID != "" {
		t.Error("non-approval_pending category must not trigger a graph read")
	}
	if len(pub.written) != 0 {
		t.Errorf("non-approval_pending category wrote %d triples, want 0", len(pub.written))
	}
}

// TestHandleMsg_SkipsMalformed: garbage bytes are dropped without panicking.
func TestHandleMsg_SkipsMalformed(t *testing.T) {
	pub := &fakePublisher{}
	sub := NewSubscriber(NewPauser(&fakeReader{}, pub, testOrg, testPlatform), "", nil)
	sub.handleMsg(context.Background(), []byte("{not json"))
	if len(pub.written) != 0 {
		t.Error("malformed envelope must write nothing")
	}
}

// TestNewSubscriber_DefaultSubject: empty subject falls back to the wildcard.
func TestNewSubscriber_DefaultSubject(t *testing.T) {
	sub := NewSubscriber(nil, "", nil)
	if sub.subject != DefaultApprovalPendingSubject {
		t.Errorf("default subject = %q, want %q", sub.subject, DefaultApprovalPendingSubject)
	}
}
