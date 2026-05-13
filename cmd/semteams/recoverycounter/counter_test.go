package recoverycounter

import (
	"context"
	"errors"
	"testing"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/types"
	agvocab "github.com/c360studio/semstreams/vocabulary/agentic"

	"github.com/c360studio/semteams/cmd/semteams/chain"
)

// fakeParentReader implements chain.ParentReader. parents maps a child
// loop_id to its parent's 6-part execution entity ID. Missing entries
// resolve to "" — chain root.
type fakeParentReader struct {
	parents map[string]string
	err     error
}

func (f *fakeParentReader) ReadParent(_ context.Context, loopID string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.parents[loopID], nil
}

// fakeEntityReader implements chain.EntityTripleReader. entities maps
// entity_id to a flat predicate→object map. Missing entities return an
// empty map (not error) so the counter can distinguish "first cycle"
// (chain entity has no recovery_count predicate yet) from "graph
// error."
type fakeEntityReader struct {
	entities map[string]map[string]any
	err      error
	calls    int
}

func (f *fakeEntityReader) ReadEntity(_ context.Context, entityID string) (map[string]any, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	if m, ok := f.entities[entityID]; ok {
		return m, nil
	}
	return map[string]any{}, nil
}

// selectiveErrorReader wraps a fakeEntityReader and surfaces an error
// only for a configured entity_id; every other read returns the base
// reader's data. Lets a test exercise the post-filter, post-resolver
// chain-read branch in isolation (filter passes on the reviewer
// entity, resolver succeeds, then the chain-entity read fails).
type selectiveErrorReader struct {
	base     *fakeEntityReader
	errorOn  string
	errorMsg string
}

func (s *selectiveErrorReader) ReadEntity(ctx context.Context, entityID string) (map[string]any, error) {
	if entityID == s.errorOn {
		return nil, errors.New(s.errorMsg)
	}
	return s.base.ReadEntity(ctx, entityID)
}

// recordingPublisher captures every AddTriple invocation so tests can
// assert subject + predicate + object.
type recordingPublisher struct {
	triples []message.Triple
}

func (p *recordingPublisher) AddTriple(_ context.Context, t message.Triple) error {
	p.triples = append(p.triples, t)
	return nil
}

func (p *recordingPublisher) byPredicateOnSubject(predicate, subject string) (message.Triple, bool) {
	for _, t := range p.triples {
		if t.Predicate == predicate && t.Subject == subject {
			return t, true
		}
	}
	return message.Triple{}, false
}

// flakyPublisher fails AddTriple on a configured predicate; succeeds
// for every other write so the per-triple loop's "partial cluster is
// queryable" contract can be asserted.
type flakyPublisher struct {
	failOn  string
	written []message.Triple
}

func (f *flakyPublisher) AddTriple(_ context.Context, t message.Triple) error {
	if t.Predicate == f.failOn {
		return errors.New("simulated publisher failure")
	}
	f.written = append(f.written, t)
	return nil
}

func testPlatform() types.PlatformMeta {
	return types.PlatformMeta{Org: "c360", Platform: "test"}
}

func loopEntityID(t *testing.T, loopID string) string {
	t.Helper()
	return agentic.LoopExecutionEntityID("c360", "test", loopID)
}

func chainEntityID(t *testing.T, chainID string) string {
	t.Helper()
	return agentic.ChainExecutionEntityID("c360", "test", chainID)
}

// resolverWithChain builds a Resolver whose parent map walks the given
// loop_id → parent loop_id pairs (NOT entity_ids — the helper composes
// the entity_id form internally). Loop IDs not in the map resolve to
// chain root (i.e., that loop_id IS the chain_id).
func resolverWithChain(t *testing.T, parents map[string]string) *chain.Resolver {
	t.Helper()
	entityParents := make(map[string]string, len(parents))
	for child, parent := range parents {
		entityParents[child] = loopEntityID(t, parent)
	}
	return chain.NewResolver(&fakeParentReader{parents: entityParents}, testPlatform())
}

func TestCounter_NonReviewerNoOp(t *testing.T) {
	pub := &recordingPublisher{}
	er := &fakeEntityReader{}
	c := NewCounter(pub, resolverWithChain(t, nil), er, testPlatform(), 0, nil)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "researcher_x",
		Role:    "researcher",
		Outcome: agentic.OutcomeSuccess,
	}
	if err := c.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if er.calls != 0 {
		t.Errorf("non-reviewer event should skip graph reads, got %d calls", er.calls)
	}
	if len(pub.triples) != 0 {
		t.Errorf("non-reviewer event should write no triples, got %d", len(pub.triples))
	}
}

func TestCounter_FailedReviewerNoOp(t *testing.T) {
	pub := &recordingPublisher{}
	er := &fakeEntityReader{}
	c := NewCounter(pub, resolverWithChain(t, nil), er, testPlatform(), 0, nil)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "reviewer_x",
		Role:    reviewerRoleResearch,
		Outcome: agentic.OutcomeFailed,
	}
	if err := c.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if er.calls != 0 || len(pub.triples) != 0 {
		t.Error("failed reviewer should skip both reads and writes (chainpause owns FAILED loops)")
	}
}

func TestCounter_ApprovedTerminalNoOp(t *testing.T) {
	pub := &recordingPublisher{}
	reviewerEntityID := loopEntityID(t, "reviewer_x")
	er := &fakeEntityReader{
		entities: map[string]map[string]any{
			reviewerEntityID: {
				agvocab.CoordinatorNextAction: "approved",
			},
		},
	}
	c := NewCounter(pub, resolverWithChain(t, nil), er, testPlatform(), 0, nil)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "reviewer_x",
		Role:    reviewerRoleResearch,
		Outcome: agentic.OutcomeSuccess,
	}
	if err := c.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pub.triples) != 0 {
		t.Errorf("approved terminal should write no triples, got %d", len(pub.triples))
	}
}

func TestCounter_FirstCycle_StampsCountAndProceed(t *testing.T) {
	// Chain ancestry: dispatch_root → researcher_a → reviewer_b
	resolver := resolverWithChain(t, map[string]string{
		"reviewer_b":   "researcher_a",
		"researcher_a": "dispatch_root",
	})
	pub := &recordingPublisher{}
	reviewerEntityID := loopEntityID(t, "reviewer_b")
	er := &fakeEntityReader{
		entities: map[string]map[string]any{
			reviewerEntityID: {
				agvocab.CoordinatorNextAction: insufficientAction,
			},
			// chain entity has no chain.recovery.count yet — first cycle.
		},
	}
	c := NewCounter(pub, resolver, er, testPlatform(), 3, nil)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "reviewer_b",
		Role:    reviewerRoleResearch,
		Outcome: agentic.OutcomeSuccess,
	}
	if err := c.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantChainID := chainEntityID(t, "dispatch_root")

	// chain.recovery.count = "1" on chain entity (audit).
	got, ok := pub.byPredicateOnSubject(chain.PredicateRecoveryCount, wantChainID)
	if !ok {
		t.Errorf("missing recovery.count on chain entity %q", wantChainID)
	} else if obj, _ := got.Object.(string); obj != "1" {
		t.Errorf("recovery.count = %v, want \"1\"", got.Object)
	}

	// chain.recovery.proceed = "true" on reviewer entity (gate).
	got, ok = pub.byPredicateOnSubject(chain.PredicateRecoveryProceed, reviewerEntityID)
	if !ok {
		t.Errorf("missing recovery.proceed on reviewer entity (within budget should open the gate)")
	} else if obj, _ := got.Object.(string); obj != "true" {
		t.Errorf("recovery.proceed = %v, want \"true\"", got.Object)
	}

	// recovery.count must NOT be on the reviewer entity (chain-only audit).
	if _, ok := pub.byPredicateOnSubject(chain.PredicateRecoveryCount, reviewerEntityID); ok {
		t.Error("recovery.count should not duplicate onto reviewer entity (chain entity is the audit surface)")
	}
	// recovery.exhausted must NOT be stamped within budget.
	for _, subject := range []string{wantChainID, reviewerEntityID} {
		if _, ok := pub.byPredicateOnSubject(chain.PredicateRecoveryExhausted, subject); ok {
			t.Errorf("recovery.exhausted should not stamp within budget on %q", subject)
		}
	}
}

func TestCounter_AtThreshold_StampsProceedNotExhausted(t *testing.T) {
	// threshold=3, prior count=2 → newCount=3, 3 <= 3 → still within budget.
	// Cap allows N cycles (3, in default), exhausted fires at N+1.
	resolver := resolverWithChain(t, map[string]string{
		"reviewer_b": "dispatch_root",
	})
	pub := &recordingPublisher{}
	reviewerEntityID := loopEntityID(t, "reviewer_b")
	chainID := chainEntityID(t, "dispatch_root")
	er := &fakeEntityReader{
		entities: map[string]map[string]any{
			reviewerEntityID: {
				agvocab.CoordinatorNextAction: insufficientAction,
			},
			chainID: {
				chain.PredicateRecoveryCount: "2",
			},
		},
	}
	c := NewCounter(pub, resolver, er, testPlatform(), 3, nil)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "reviewer_b",
		Role:    reviewerRoleResearch,
		Outcome: agentic.OutcomeSuccess,
	}
	if err := c.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// count=3 on chain.
	got, _ := pub.byPredicateOnSubject(chain.PredicateRecoveryCount, chainID)
	if obj, _ := got.Object.(string); obj != "3" {
		t.Errorf("recovery.count = %v, want \"3\"", got.Object)
	}
	// proceed=true on reviewer (3 <= 3 still passes).
	if _, ok := pub.byPredicateOnSubject(chain.PredicateRecoveryProceed, reviewerEntityID); !ok {
		t.Error("at-threshold (3<=3) should still proceed; cycle 4 is the first to exhaust")
	}
	// exhausted NOT stamped (still within budget).
	if _, ok := pub.byPredicateOnSubject(chain.PredicateRecoveryExhausted, chainID); ok {
		t.Error("exhausted should not stamp at threshold; only when newCount > threshold")
	}
}

func TestCounter_OverThreshold_StampsExhaustedNotProceed(t *testing.T) {
	// threshold=3, prior count=3 → newCount=4, 4 > 3 → exhausted.
	resolver := resolverWithChain(t, map[string]string{
		"reviewer_b": "dispatch_root",
	})
	pub := &recordingPublisher{}
	reviewerEntityID := loopEntityID(t, "reviewer_b")
	chainID := chainEntityID(t, "dispatch_root")
	er := &fakeEntityReader{
		entities: map[string]map[string]any{
			reviewerEntityID: {
				agvocab.CoordinatorNextAction: insufficientAction,
			},
			chainID: {
				chain.PredicateRecoveryCount: "3",
			},
		},
	}
	c := NewCounter(pub, resolver, er, testPlatform(), 3, nil)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "reviewer_b",
		Role:    reviewerRoleResearch,
		Outcome: agentic.OutcomeSuccess,
	}
	if err := c.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// count=4 on chain (audit always lands).
	got, _ := pub.byPredicateOnSubject(chain.PredicateRecoveryCount, chainID)
	if obj, _ := got.Object.(string); obj != "4" {
		t.Errorf("recovery.count = %v, want \"4\"", got.Object)
	}
	// exhausted=true on chain.
	got, ok := pub.byPredicateOnSubject(chain.PredicateRecoveryExhausted, chainID)
	if !ok {
		t.Error("missing recovery.exhausted on chain entity (over budget should mark the chain)")
	} else if obj, _ := got.Object.(string); obj != "true" {
		t.Errorf("recovery.exhausted = %v, want \"true\"", got.Object)
	}
	// proceed must NOT be stamped (this is the gate that closes).
	if _, ok := pub.byPredicateOnSubject(chain.PredicateRecoveryProceed, reviewerEntityID); ok {
		t.Error("recovery.proceed must not stamp when over budget; absence is what blocks the rule fire")
	}
}

func TestCounter_MalformedPriorCount_TreatedAsZero(t *testing.T) {
	resolver := resolverWithChain(t, map[string]string{
		"reviewer_b": "dispatch_root",
	})
	pub := &recordingPublisher{}
	reviewerEntityID := loopEntityID(t, "reviewer_b")
	chainID := chainEntityID(t, "dispatch_root")
	er := &fakeEntityReader{
		entities: map[string]map[string]any{
			reviewerEntityID: {
				agvocab.CoordinatorNextAction: insufficientAction,
			},
			chainID: {
				// Schema drift / corruption: object is not a number-shaped
				// string. Counter must treat as zero rather than crash, so
				// the chain still gets a fresh count rather than wedging
				// on the next reviewer rejection.
				chain.PredicateRecoveryCount: "not-a-number",
			},
		},
	}
	c := NewCounter(pub, resolver, er, testPlatform(), 3, nil)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "reviewer_b",
		Role:    reviewerRoleResearch,
		Outcome: agentic.OutcomeSuccess,
	}
	if err := c.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, _ := pub.byPredicateOnSubject(chain.PredicateRecoveryCount, chainID)
	if obj, _ := got.Object.(string); obj != "1" {
		t.Errorf("malformed prior count should reset to 1 (treat as 0 + 1), got %v", got.Object)
	}
}

func TestCounter_DefaultThresholdAppliedWhenZero(t *testing.T) {
	// threshold=0 → DefaultThreshold (3). prior=3 → new=4 → 4 > 3 → exhausted.
	resolver := resolverWithChain(t, map[string]string{"reviewer_b": "dispatch_root"})
	pub := &recordingPublisher{}
	reviewerEntityID := loopEntityID(t, "reviewer_b")
	chainID := chainEntityID(t, "dispatch_root")
	er := &fakeEntityReader{
		entities: map[string]map[string]any{
			reviewerEntityID: {
				agvocab.CoordinatorNextAction: insufficientAction,
			},
			chainID: {
				chain.PredicateRecoveryCount: "3",
			},
		},
	}
	c := NewCounter(pub, resolver, er, testPlatform(), 0, nil)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "reviewer_b",
		Role:    reviewerRoleResearch,
		Outcome: agentic.OutcomeSuccess,
	}
	if err := c.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := pub.byPredicateOnSubject(chain.PredicateRecoveryExhausted, chainID); !ok {
		t.Error("threshold=0 should fall back to DefaultThreshold (3); count 3→4 should stamp exhausted")
	}
}

func TestCounter_ResolverError_Skipped(t *testing.T) {
	resolver := chain.NewResolver(&fakeParentReader{err: errors.New("graph KV unavailable")}, testPlatform())
	pub := &recordingPublisher{}
	reviewerEntityID := loopEntityID(t, "reviewer_x")
	er := &fakeEntityReader{
		entities: map[string]map[string]any{
			reviewerEntityID: {
				agvocab.CoordinatorNextAction: insufficientAction,
			},
		},
	}
	c := NewCounter(pub, resolver, er, testPlatform(), 0, nil)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "reviewer_x",
		Role:    reviewerRoleResearch,
		Outcome: agentic.OutcomeSuccess,
	}
	if err := c.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("expected fail-soft, got error: %v", err)
	}
	// Critical: no proceed stamp on graph failure → chain stalls fail-safe.
	if len(pub.triples) != 0 {
		t.Errorf("resolver error should drop the cycle entirely (no proceed → fail-safe), got %d triples", len(pub.triples))
	}
}

func TestCounter_ChainEntityReadError_Skipped(t *testing.T) {
	resolver := resolverWithChain(t, map[string]string{"reviewer_x": "dispatch_root"})
	pub := &recordingPublisher{}
	reviewerEntityID := loopEntityID(t, "reviewer_x")
	chainID := chainEntityID(t, "dispatch_root")
	er := &selectiveErrorReader{
		base: &fakeEntityReader{
			entities: map[string]map[string]any{
				reviewerEntityID: {
					agvocab.CoordinatorNextAction: insufficientAction,
				},
			},
		},
		errorOn:  chainID,
		errorMsg: "graph timeout",
	}
	c := NewCounter(pub, resolver, er, testPlatform(), 0, nil)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "reviewer_x",
		Role:    reviewerRoleResearch,
		Outcome: agentic.OutcomeSuccess,
	}
	if err := c.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("expected fail-soft, got error: %v", err)
	}
	if len(pub.triples) != 0 {
		t.Errorf("chain-entity read error should drop the cycle, got %d triples", len(pub.triples))
	}
}

func TestCounter_ProceedWriteFails_AuditStillLands(t *testing.T) {
	// Within budget, but the proceed write to the reviewer entity fails.
	// The chain.recovery.count audit write to the chain entity must still
	// land — partial-cluster fail-soft, "audit beats nothing."
	resolver := resolverWithChain(t, map[string]string{"reviewer_b": "dispatch_root"})
	pub := &flakyPublisher{failOn: chain.PredicateRecoveryProceed}
	reviewerEntityID := loopEntityID(t, "reviewer_b")
	chainID := chainEntityID(t, "dispatch_root")
	er := &fakeEntityReader{
		entities: map[string]map[string]any{
			reviewerEntityID: {
				agvocab.CoordinatorNextAction: insufficientAction,
			},
			chainID: {
				chain.PredicateRecoveryCount: "1",
			},
		},
	}
	c := NewCounter(pub, resolver, er, testPlatform(), 3, nil)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "reviewer_b",
		Role:    reviewerRoleResearch,
		Outcome: agentic.OutcomeSuccess,
	}
	if err := c.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("partial-cluster failure should be fail-soft, got error: %v", err)
	}
	// Only the count write should have landed (proceed failed).
	if len(pub.written) != 1 {
		t.Fatalf("expected 1 write (count) after proceed failed, got %d (%v)", len(pub.written), pub.written)
	}
	if pub.written[0].Predicate != chain.PredicateRecoveryCount {
		t.Errorf("expected count to land, got %q", pub.written[0].Predicate)
	}
	if pub.written[0].Subject != chainID {
		t.Errorf("count must land on chain entity %q, got %q", chainID, pub.written[0].Subject)
	}
}

func TestCounter_NilEvent_NoOp(t *testing.T) {
	pub := &recordingPublisher{}
	er := &fakeEntityReader{}
	c := NewCounter(pub, resolverWithChain(t, nil), er, testPlatform(), 0, nil)

	if err := c.HandleLoopCompleted(context.Background(), nil); err != nil {
		t.Fatalf("nil event should fail-soft, got error: %v", err)
	}
	if len(pub.triples) != 0 || er.calls != 0 {
		t.Error("nil event should write nothing and read nothing")
	}
}

func TestCounter_EmptyLoopID_Errors(t *testing.T) {
	pub := &recordingPublisher{}
	er := &fakeEntityReader{}
	c := NewCounter(pub, resolverWithChain(t, nil), er, testPlatform(), 0, nil)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "",
		Role:    reviewerRoleResearch,
		Outcome: agentic.OutcomeSuccess,
	}
	if err := c.HandleLoopCompleted(context.Background(), ev); err == nil {
		t.Error("empty LoopID should surface an error rather than panic in entity-id construction")
	}
}

func TestReadCount_Variants(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want int
	}{
		{"nil → 0", nil, 0},
		{"missing-string → 0", "", 0},
		{"valid string", "5", 5},
		{"float64 (json.Unmarshal default)", float64(7), 7},
		{"int", int(3), 3},
		{"int64", int64(11), 11},
		{"non-numeric string", "abc", 0},
		{"unknown type", []int{1, 2, 3}, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := readCount(tc.in); got != tc.want {
				t.Errorf("readCount(%v) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}
