package approvalpause

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	agvocab "github.com/c360studio/semstreams/vocabulary/agentic"
)

const replayLoopID = "00000000-0000-4000-8000-000000000101"
const replayRunID = "c360.ops.chain.agent.execution.00000000-0000-4000-8000-000000000102"

// durableGraph survives replacement of the subscriber while keeping the same
// append-only values the production graph read exposes.
type durableGraph struct {
	mu                         sync.Mutex
	triples                    []message.Triple
	readErr                    error
	writeErr                   error
	revision                   uint64
	reads, attempts, conflicts int
	barrier                    chan struct{}
	readsReady                 chan struct{}
	alwaysConflict, wrongType  bool
	reply                      []byte
	requests                   []graph.ReconcilePredicatesRequest
	replyMutator               func(*graph.ReconcilePredicatesResponse)
}

func (g *durableGraph) ReadEntity(_ context.Context, _ string) (map[string]any, error) {
	return map[string]any{agvocab.LoopRunEntityID: replayRunID}, nil
}
func (g *durableGraph) ReadPredicateValues(_ context.Context, entityID, predicate string) ([]any, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.readErr != nil {
		return nil, g.readErr
	}
	var values []any
	for _, triple := range g.triples {
		if triple.Subject == entityID && triple.Predicate == predicate {
			values = append(values, triple.Object)
		}
	}
	return values, nil
}
func (g *durableGraph) Append(_ context.Context, triples []message.Triple) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.writeErr != nil {
		return g.writeErr
	}
	survivors, _ := message.DedupeAppendTriples(g.triples, triples)
	g.triples = append(g.triples, survivors...)
	return nil
}
func (g *durableGraph) markerCount(predicate string) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	count := 0
	for _, triple := range g.triples {
		if triple.Predicate == predicate {
			count++
		}
	}
	return count
}
func pendingExecution(executionID string) *agentic.ApprovalPendingEvent {
	return &agentic.ApprovalPendingEvent{LoopID: replayLoopID, CallID: "provider-call", ExecutionID: executionID, ToolName: "create_rule"}
}
func answeredExecution(executionID string) *agentic.ApprovalResponse {
	return &agentic.ApprovalResponse{LoopID: replayLoopID, CallID: "provider-call", ExecutionID: executionID, Decision: agentic.ApprovalDecisionApprove, ApprovedBy: "reviewer"}
}

func TestApprovalReplayAfterSubscriberReplacementDoesNotRePause(t *testing.T) {
	ctx := context.Background()
	graph := &durableGraph{}
	p := NewPauser(graph, graph, testOrg, testPlatform)
	if result, err := p.HandlePending(ctx, pendingExecution("execution-a")); err != nil || !result.Stamped {
		t.Fatalf("initial pending: %+v, %v", result, err)
	}
	if result, err := p.HandleResponse(ctx, answeredExecution("execution-a")); err != nil || !result.Stamped {
		t.Fatalf("answer: %+v, %v", result, err)
	}
	// The same receipt must survive a new subscriber, independent of the loop
	// still advertising the old gate during upstream's publish/write window.
	p = NewPauser(graph, graph, testOrg, testPlatform)
	if result, err := p.HandlePending(ctx, pendingExecution("execution-a")); err != nil || result.Stamped {
		t.Fatalf("answered gate re-paused after replacement: %+v, %v", result, err)
	}
	if got := graph.markerCount(MarkerApprovalPending); got != 1 {
		t.Fatalf("pending marker writes=%d, want 1", got)
	}
}

func TestApprovalReplayKeepsEveryAnsweredExecution(t *testing.T) {
	ctx := context.Background()
	graph := &durableGraph{}
	p := NewPauser(graph, graph, testOrg, testPlatform)
	for _, executionID := range []string{"execution-a", "execution-b"} {
		if _, err := p.HandlePending(ctx, pendingExecution(executionID)); err != nil {
			t.Fatal(err)
		}
		if _, err := p.HandleResponse(ctx, answeredExecution(executionID)); err != nil {
			t.Fatal(err)
		}
	}
	p = NewPauser(graph, graph, testOrg, testPlatform)
	for _, executionID := range []string{"execution-a", "execution-b"} {
		if result, err := p.HandlePending(ctx, pendingExecution(executionID)); err != nil || result.Stamped {
			t.Fatalf("replayed %s: %+v, %v", executionID, result, err)
		}
		if result, err := p.HandleResponse(ctx, answeredExecution(executionID)); err != nil || result.Stamped {
			t.Fatalf("duplicate answer %s: %+v, %v", executionID, result, err)
		}
	}
	if result, err := p.HandlePending(ctx, pendingExecution("execution-c")); err != nil || !result.Stamped {
		t.Fatalf("fresh gate must pause: %+v, %v", result, err)
	}
	if got := graph.markerCount(MarkerApprovalAnswered); got != 2 {
		t.Fatalf("resume writes=%d, want 2", got)
	}
	values, err := graph.ReadPredicateValues(ctx, replayRunID, "agent.run.approval-answered")
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 {
		t.Fatalf("durable receipts=%d, want 2", len(values))
	}
	for _, value := range values {
		var key []string
		if err := json.Unmarshal([]byte(value.(string)), &key); err != nil || len(key) != 2 || key[0] != replayLoopID {
			t.Fatalf("receipt lost loop/execution identity: %v, %v", value, err)
		}
	}
}

func TestApprovalReceiptReadFailureDoesNotRePauseOrResume(t *testing.T) {
	graph := &durableGraph{readErr: errors.New("graph unavailable")}
	p := NewPauser(graph, graph, testOrg, testPlatform)
	if result, err := p.HandlePending(context.Background(), pendingExecution("execution-a")); err == nil || result.Stamped {
		t.Fatalf("pending must fail closed: %+v, %v", result, err)
	}
	if result, err := p.HandleResponse(context.Background(), answeredExecution("execution-a")); err == nil || result.Stamped {
		t.Fatalf("answer must fail closed: %+v, %v", result, err)
	}
	if len(graph.triples) != 0 {
		t.Fatal("receipt read failure wrote graph state")
	}
}

func TestApprovalReceiptWriteFailureDoesNotClaimResume(t *testing.T) {
	graph := &durableGraph{writeErr: errors.New("graph unavailable")}
	p := NewPauser(graph, graph, testOrg, testPlatform)
	if result, err := p.HandleResponse(context.Background(), answeredExecution("execution-a")); err == nil || result.Stamped {
		t.Fatalf("answer must not claim a durable receipt: %+v, %v", result, err)
	}
	if len(graph.triples) != 0 {
		t.Fatal("failed atomic write persisted state")
	}
}

func TestApprovalWithoutExecutionIdentityIsRefused(t *testing.T) {
	graph := &durableGraph{}
	p := NewPauser(graph, graph, testOrg, testPlatform)
	if result, err := p.HandlePending(context.Background(), pendingExecution("")); err == nil || result.Stamped {
		t.Fatalf("uncorrelated pending: %+v, %v", result, err)
	}
	if result, err := p.HandleResponse(context.Background(), answeredExecution("")); err == nil || result.Stamped {
		t.Fatalf("uncorrelated answer: %+v, %v", result, err)
	}
	if len(graph.triples) != 0 {
		t.Fatal("uncorrelated gate wrote graph state")
	}
}

func TestApprovalSeparatePausersPreserveAnswerReceipt(t *testing.T) {
	ctx := context.Background()
	graph := &durableGraph{}
	p := NewPauser(graph, graph, testOrg, testPlatform)
	other := NewPauser(graph, graph, testOrg, testPlatform)
	start := make(chan struct{})
	results := make(chan error, 2)
	go func() { <-start; _, err := p.HandlePending(ctx, pendingExecution("execution-a")); results <- err }()
	go func() { <-start; _, err := other.HandleResponse(ctx, answeredExecution("execution-a")); results <- err }()
	close(start)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	p = NewPauser(graph, graph, testOrg, testPlatform)
	if result, err := p.HandlePending(ctx, pendingExecution("execution-a")); err != nil || result.Stamped {
		t.Fatalf("concurrent answer lost its durable receipt: %+v, %v", result, err)
	}
}

func (g *durableGraph) RecordApproval(ctx context.Context, run, loop, execution string, answer bool) (bool, error) {
	p := &NATSProjection{reader: g, requester: g, org: testOrg, platform: testPlatform}
	return p.RecordApproval(ctx, run, loop, execution, answer)
}
