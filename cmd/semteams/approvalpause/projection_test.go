package approvalpause

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/c360studio/semstreams/pkg/lifecycle"
	"github.com/c360studio/semstreams/pkg/projection"
	"github.com/c360studio/semteams/cmd/semteams/vocab"
	"github.com/nats-io/nats.go"
)

func testProjection(t *testing.T, g *durableGraph) *NATSProjection {
	t.Helper()
	vocab.Register()
	p, err := newProjection(g, g, testOrg, testPlatform)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// Two independent readers rendezvous after taking the same authority revision.
// The second mutation must conflict and recompute, not overwrite the first gate.
func TestProjectionConcurrentWritersPreserveBothGates(t *testing.T) {
	g := &durableGraph{barrier: make(chan struct{}), readsReady: make(chan struct{}, 2)}
	p1, p2 := testProjection(t, g), testProjection(t, g)
	results := make(chan error, 2)
	go func() {
		_, err := p1.RecordApproval(context.Background(), replayRunID, replayLoopID, "a", false)
		results <- err
	}()
	go func() {
		_, err := p2.RecordApproval(context.Background(), replayRunID, replayLoopID, "b", true)
		results <- err
	}()
	<-g.readsReady
	<-g.readsReady
	close(g.barrier)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	pending, _ := g.ReadPredicateValues(context.Background(), replayRunID, MarkerApprovalPending)
	answered, _ := g.ReadPredicateValues(context.Background(), replayRunID, MarkerApprovalAnswered)
	if len(pending) != 2 || len(answered) != 1 {
		t.Fatalf("lost gate: pending=%v answered=%v", pending, answered)
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.conflicts != 1 {
		t.Fatalf("conflicts=%d", g.conflicts)
	}
	if got := outstanding(g.triples); got != 1 {
		t.Fatalf("outstanding=%v", got)
	}
}

func TestProjectionPreservesOtherFactsAndAnnotations(t *testing.T) {
	timestamp := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	original := message.Triple{Subject: replayRunID, Predicate: MarkerApprovalPending, Object: pair(replayLoopID, "a"), Source: "original", Timestamp: timestamp, Confidence: 0.7, Context: "audit"}
	other := message.Triple{Subject: replayRunID, Predicate: "agent.run.phase", Object: "awaiting_approval"}
	g := &durableGraph{triples: []message.Triple{original, other, {Subject: replayRunID, Predicate: MarkerApprovalOutstanding, Object: 1}}}
	p := testProjection(t, g)
	if _, err := p.RecordApproval(context.Background(), replayRunID, replayLoopID, "b", false); err != nil {
		t.Fatal(err)
	}
	if g.triples[0] != other {
		t.Fatalf("unrelated lifecycle fact changed: %+v", g.triples)
	}
	found := false
	for _, tr := range g.triples {
		if tr.Predicate == original.Predicate && tr.Object == original.Object {
			found = true
			if tr != original {
				t.Fatalf("annotations changed: %+v", tr)
			}
		}
	}
	if !found {
		t.Fatal("existing receipt lost")
	}
}

func TestProjectionFailClosed(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mutate  func(*durableGraph)
		unknown bool
	}{
		{"read error", func(g *durableGraph) { g.readErr = errors.New("read unavailable") }, false},
		{"malformed pair", func(g *durableGraph) {
			g.triples = []message.Triple{{Subject: replayRunID, Predicate: MarkerApprovalPending, Object: "old-loop-ref"}}
		}, false},
		{"answered outside pending", func(g *durableGraph) {
			g.triples = []message.Triple{{Subject: replayRunID, Predicate: MarkerApprovalAnswered, Object: pair(replayLoopID, "a")}}
		}, false},
		{"wrong authority type", func(g *durableGraph) { g.wrongType = true }, false},
		{"ambiguous mutation", func(g *durableGraph) { g.writeErr = context.DeadlineExceeded }, true},
		{"malformed reply", func(g *durableGraph) { g.reply = []byte(`{`) }, true},
		{"invalid reply", func(g *durableGraph) { g.reply = []byte(`{"outcome":"applied","kv_revision":0}`) }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := &durableGraph{}
			tc.mutate(g)
			p := testProjection(t, g)
			_, err := p.RecordApproval(context.Background(), replayRunID, replayLoopID, "b", false)
			if err == nil {
				t.Fatal("unsafe state accepted")
			}
			var me *projection.MutationError
			if tc.unknown && (!errors.As(err, &me) || me.Commit != projection.CommitUnknown) {
				t.Fatalf("lost unknown outcome: %v", err)
			}
			if g.attempts > 1 {
				t.Fatalf("retried ambiguous/invalid mutation %d times", g.attempts)
			}
		})
	}
}

func TestProjectionRetryIsBoundedAndContextAware(t *testing.T) {
	g := &durableGraph{alwaysConflict: true}
	p := testProjection(t, g)
	_, err := p.RecordApproval(context.Background(), replayRunID, replayLoopID, "a", false)
	if !errors.Is(err, errs.ErrRevisionMismatch) || g.attempts != maxProjectionAttempts {
		t.Fatalf("unbounded/wrong retry: %d %v", g.attempts, err)
	}
	g = &durableGraph{}
	p = testProjection(t, g)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.RecordApproval(ctx, replayRunID, replayLoopID, "a", false); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation=%v", err)
	}
	if g.attempts != 0 {
		t.Fatal("canceled request reached mutation")
	}
}

func TestProjectionRejectsForeignRunBeforeIO(t *testing.T) {
	g := &durableGraph{}
	p := testProjection(t, g)
	for _, run := range []string{"other.ops.chain.agent.execution.abc", "c360.ops.agentic-loop.agent.execution.abc", "invalid"} {
		if _, err := p.RecordApproval(context.Background(), run, replayLoopID, "a", false); err == nil {
			t.Fatalf("accepted %s", run)
		}
	}
	if g.reads != 0 || g.attempts != 0 {
		t.Fatal("invalid authority reached IO")
	}
}

func pair(loop, execution string) string {
	encoded, _ := json.Marshal([2]string{loop, execution})
	return string(encoded)
}
func outstanding(triples []message.Triple) int {
	for _, tr := range triples {
		if tr.Predicate == MarkerApprovalOutstanding {
			switch n := tr.Object.(type) {
			case int:
				return n
			case float64:
				return int(n)
			}
		}
	}
	return -1
}

// The fixture applies the documented typed operation's revision fence. Production
// owns all tuple validation, union, count computation, and retry logic above it.
func (g *durableGraph) ReadExactEntity(ctx context.Context, id string) (*graph.ExactEntity, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	g.mu.Lock()
	g.reads++
	n := g.reads
	entity := &graph.EntityState{ID: id, MessageType: lifecycle.HarnessMessageType(), Triples: append([]message.Triple(nil), g.triples...)}
	if g.wrongType {
		entity.MessageType = message.Type{Domain: "agentic", Category: "loop_execution", Version: "v1"}
	}
	exact := &graph.ExactEntity{Entity: entity, KVRevision: g.revision + 1}
	readErr := g.readErr
	barrier := g.barrier
	ready := g.readsReady
	g.mu.Unlock()
	if barrier != nil && n <= 2 {
		ready <- struct{}{}
		select {
		case <-barrier:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return exact, readErr
}
func (g *durableGraph) RequestClassified(_ context.Context, subject string, data []byte, _ time.Duration) ([]byte, error) {
	if subject != approvalReconcileSubject {
		return nil, fmt.Errorf("unexpected subject %s", subject)
	}
	var req graph.ReconcilePredicatesRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.attempts++
	g.requests = append(g.requests, req)
	if g.writeErr != nil {
		return nil, g.writeErr
	}
	if g.alwaysConflict || req.ExpectedRevision != g.revision+1 {
		g.conflicts++
		return nil, errs.ErrRevisionMismatch
	}
	if g.reply != nil {
		return g.reply, nil
	}
	kept := []message.Triple{}
	selectedFacts := []message.Triple{}
	for _, tr := range g.triples {
		selected := false
		for _, pred := range req.Predicates {
			selected = selected || tr.Predicate == pred
		}
		if !selected {
			kept = append(kept, tr)
		} else {
			selectedFacts = append(selectedFacts, tr)
		}
	}
	outcome := graph.MutationUnchanged
	if !reflect.DeepEqual(selectedFacts, req.Desired) {
		g.triples = append(kept, req.Desired...)
		g.revision++
		outcome = graph.MutationApplied
	}
	response := graph.ReconcilePredicatesResponse{Outcome: outcome, Entity: &graph.EntityState{ID: req.EntityID, MessageType: lifecycle.HarnessMessageType(), Triples: append([]message.Triple(nil), g.triples...)}, KVRevision: g.revision + 1, RequestID: req.RequestID}
	if g.replyMutator != nil {
		g.replyMutator(&response)
	}
	return json.Marshal(response)
}

func TestProjectionRejectsInconsistentDuplicateHistory(t *testing.T) {
	g := &durableGraph{}
	p := testProjection(t, g)
	if _, err := p.RecordApproval(context.Background(), replayRunID, replayLoopID, "a", true); err != nil {
		t.Fatal(err)
	}
	for i := range g.triples {
		if g.triples[i].Predicate == MarkerApprovalOutstanding {
			g.triples[i].Object = 5
		}
	}
	before := g.attempts
	if _, err := p.RecordApproval(context.Background(), replayRunID, replayLoopID, "a", true); err == nil {
		t.Fatal("duplicate trusted inconsistent count")
	}
	if g.attempts != before {
		t.Fatal("invalid history reached mutation")
	}
}
func TestProjectionBadMutationReceiptsAreUnknownAndNeverRetried(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*graph.ReconcilePredicatesResponse)
	}{
		{"wrong identity", func(r *graph.ReconcilePredicatesResponse) { r.Entity.ID = "other.ops.chain.agent.execution.abc" }},
		{"wrong type", func(r *graph.ReconcilePredicatesResponse) {
			r.Entity.MessageType = message.Type{Domain: "agentic", Category: "loop_execution", Version: "v1"}
		}},
		{"zero revision", func(r *graph.ReconcilePredicatesResponse) { r.KVRevision = 0 }},
		{"nonadvancing applied", func(r *graph.ReconcilePredicatesResponse) { r.KVRevision = 1 }},
		{"advancing unchanged", func(r *graph.ReconcilePredicatesResponse) { r.Outcome = graph.MutationUnchanged }},
		{"wrong request", func(r *graph.ReconcilePredicatesResponse) { r.RequestID = "other" }},
		{"wrong outcome", func(r *graph.ReconcilePredicatesResponse) { r.Outcome = graph.MutationEntityNotFound }},
		{"wrong facts", func(r *graph.ReconcilePredicatesResponse) { r.Entity.Triples = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := &durableGraph{replyMutator: tc.mutate}
			p := testProjection(t, g)
			_, err := p.RecordApproval(context.Background(), replayRunID, replayLoopID, "a", false)
			var me *projection.MutationError
			if !errors.As(err, &me) || me.Commit != projection.CommitUnknown || g.attempts != 1 {
				t.Fatalf("unsafe receipt handling attempts=%d err=%v", g.attempts, err)
			}
		})
	}
}
func TestProjectionConflictRetryPreservesLogicalProvenance(t *testing.T) {
	g := &durableGraph{alwaysConflict: true}
	p := testProjection(t, g)
	_, _ = p.RecordApproval(context.Background(), replayRunID, replayLoopID, `execution:with|delimiters"`, false)
	if len(g.requests) != maxProjectionAttempts {
		t.Fatal("missing conflict attempts")
	}
	first := g.requests[0]
	for _, req := range g.requests[1:] {
		if req.RequestID != first.RequestID || !req.Desired[0].Timestamp.Equal(first.Desired[0].Timestamp) {
			t.Fatal("logical event provenance changed across CAS retries")
		}
	}
}

func TestProjectionDefiniteRefusalsAreNotUnknown(t *testing.T) {
	for _, failure := range []error{nats.ErrNoResponders, errs.ClassifiedCode(errs.ErrorTransient, "not_ready", errors.New("not ready")), errs.ClassifiedCode(errs.ErrorFatal, "internal", errors.New("refused")), errs.ClassifiedCode(errs.ErrorInvalid, "invalid_request", errors.New("invalid"))} {
		g := &durableGraph{writeErr: failure}
		p := testProjection(t, g)
		_, err := p.RecordApproval(context.Background(), replayRunID, replayLoopID, "a", false)
		var me *projection.MutationError
		if !errors.As(err, &me) || me.Commit != projection.CommitNotCommitted || g.attempts != 1 {
			t.Fatalf("definite refusal reported incorrectly: %v", err)
		}
		if !errors.Is(err, failure) {
			t.Fatalf("lost failure classification: %v", err)
		}
	}
}

func TestProjectionDuplicateAcceptsUnchangedReceipt(t *testing.T) {
	g := &durableGraph{}
	p := testProjection(t, g)
	_, err := p.RecordApproval(context.Background(), replayRunID, replayLoopID, "a", true)
	if err != nil {
		t.Fatal(err)
	}
	revision := g.revision
	already, err := p.RecordApproval(context.Background(), replayRunID, replayLoopID, "a", false)
	if err != nil || !already || g.revision != revision {
		t.Fatalf("unchanged answered replay rejected: already=%v revision=%d err=%v", already, g.revision, err)
	}
}
