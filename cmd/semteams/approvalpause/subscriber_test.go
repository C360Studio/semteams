package approvalpause

import (
	"context"
	"encoding/json"
	agvocab "github.com/c360studio/semstreams/vocabulary/agentic"
	"testing"
	"time"

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

func newSubscriber(reader EntityTripleReader, pub TriplePublisher) *Subscriber {
	return NewSubscriber(NewPauser(reader, pub, testOrg, testPlatform), "", "", nil)
}

// TestHandlePendingMsg_ApprovalPending decodes a real ApprovalPendingEvent envelope
// and drives the Pauser — the wire-decode half of the PAUSE path, exercising the
// category gate + payload unmarshal against the upstream marshaller.
func TestHandlePendingMsg_ApprovalPending(t *testing.T) {
	const runEntity = "c360.ops.agent.chain.execution.run-77"
	reader := &fakeReader{triples: map[string]any{agvocab.LoopRunEntityID: runEntity}}
	pub := &fakePublisher{}
	sub := newSubscriber(reader, pub)

	sub.handlePendingMsg(context.Background(), envelopeFor(t, &agentic.ApprovalPendingEvent{
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

// TestHandleResponseMsg_ApprovalResponse decodes a real ApprovalResponse envelope and
// drives the resume half — stamps agent.run.approval-resumed (4c PR-2).
func TestHandleResponseMsg_ApprovalResponse(t *testing.T) {
	const runEntity = "c360.ops.agent.chain.execution.run-88"
	reader := &fakeReader{triples: map[string]any{agvocab.LoopRunEntityID: runEntity}}
	pub := &fakePublisher{}
	sub := newSubscriber(reader, pub)

	sub.handleResponseMsg(context.Background(), envelopeFor(t, &agentic.ApprovalResponse{
		LoopID:     "loop-88",
		CallID:     "call-88",
		Decision:   agentic.ApprovalDecisionApprove,
		ApprovedBy: "ui-anonymous",
		DecidedAt:  time.Unix(0, 0).UTC(),
	}))

	if len(pub.written) != 1 {
		t.Fatalf("want 1 triple written from a decoded approval_response event, got %d", len(pub.written))
	}
	if pub.written[0].Subject != runEntity || pub.written[0].Predicate != MarkerApprovalResumed {
		t.Errorf("decoded response stamped wrong triple: %+v", pub.written[0])
	}
}

// TestHandlePendingMsg_SkipsOtherCategory: a non-approval_pending envelope on the
// pending wildcard is ignored (no read, no write). Cross-channel guard — the
// response category must NOT trigger the pending handler.
func TestHandlePendingMsg_SkipsOtherCategory(t *testing.T) {
	reader := &fakeReader{triples: map[string]any{agvocab.LoopRunEntityID: "x"}}
	pub := &fakePublisher{}
	sub := newSubscriber(reader, pub)

	// An approval_response envelope must be ignored by the PENDING handler.
	sub.handlePendingMsg(context.Background(), []byte(`{"type":{"category":"approval_response"},"payload":{}}`))

	if reader.lastReadID != "" {
		t.Error("non-approval_pending category must not trigger a graph read on the pending handler")
	}
	if len(pub.written) != 0 {
		t.Errorf("non-approval_pending category wrote %d triples, want 0", len(pub.written))
	}
}

// TestHandleResponseMsg_SkipsOtherCategory: the response handler ignores a
// pending-category envelope (the reverse cross-channel guard).
func TestHandleResponseMsg_SkipsOtherCategory(t *testing.T) {
	reader := &fakeReader{triples: map[string]any{agvocab.LoopRunEntityID: "x"}}
	pub := &fakePublisher{}
	sub := newSubscriber(reader, pub)

	sub.handleResponseMsg(context.Background(), []byte(`{"type":{"category":"approval_pending"},"payload":{}}`))

	if reader.lastReadID != "" {
		t.Error("non-approval_response category must not trigger a graph read on the response handler")
	}
	if len(pub.written) != 0 {
		t.Errorf("non-approval_response category wrote %d triples, want 0", len(pub.written))
	}
}

// TestHandleMsg_SkipsMalformed: garbage bytes are dropped without panicking on both
// handlers.
func TestHandleMsg_SkipsMalformed(t *testing.T) {
	pub := &fakePublisher{}
	sub := newSubscriber(&fakeReader{}, pub)
	sub.handlePendingMsg(context.Background(), []byte("{not json"))
	sub.handleResponseMsg(context.Background(), []byte("{not json"))
	if len(pub.written) != 0 {
		t.Error("malformed envelope must write nothing")
	}
}

// TestNewSubscriber_DefaultSubjects: empty subjects fall back to the wildcards.
func TestNewSubscriber_DefaultSubjects(t *testing.T) {
	sub := NewSubscriber(nil, "", "", nil)
	if sub.pendingSubject != DefaultApprovalPendingSubject {
		t.Errorf("default pending subject = %q, want %q", sub.pendingSubject, DefaultApprovalPendingSubject)
	}
	if sub.responseSubject != DefaultApprovalResponseSubject {
		t.Errorf("default response subject = %q, want %q", sub.responseSubject, DefaultApprovalResponseSubject)
	}
}
