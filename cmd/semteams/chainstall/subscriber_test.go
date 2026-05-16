package chainstall

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

// --- Test fakes (mirrors recoverycounter/counter_test.go style) -------

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

type selectiveErrorReader struct {
	base    *fakeEntityReader
	errorOn string
}

func (s *selectiveErrorReader) ReadEntity(ctx context.Context, entityID string) (map[string]any, error) {
	if entityID == s.errorOn {
		return nil, errors.New("simulated read failure")
	}
	return s.base.ReadEntity(ctx, entityID)
}

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

// --- Helpers -----------------------------------------------------------

func testPlatform() types.PlatformMeta {
	return types.PlatformMeta{Org: "c360", Platform: "test"}
}

func loopEntityID(t *testing.T, loopID string) string {
	t.Helper()
	return agentic.LoopExecutionEntityID("c360", "test", loopID)
}

// resolverWithChain builds a chain.Resolver whose parent map walks the
// given loop_id → parent loop_id pairs. Loop IDs not in the map resolve
// to chain root.
func resolverWithChain(t *testing.T, parents map[string]string) *chain.Resolver {
	t.Helper()
	entityParents := make(map[string]string, len(parents))
	for child, parent := range parents {
		entityParents[child] = loopEntityID(t, parent)
	}
	return chain.NewResolver(&fakeParentReader{parents: entityParents}, testPlatform())
}

// synthesizeFixture builds the canonical fixture for the mode_mismatch
// happy path: synthesize loop emitted decide(architect) under a chain
// with chain.mode.classification=research_only. Returns the subscriber,
// the recording publisher, the event, and the chain entity ID (so tests
// can assert against the audit cluster's subject).
func synthesizeFixture(t *testing.T, threshold int, authority string, chainTripleOverrides map[string]any) (*Subscriber, *recordingPublisher, *agentic.LoopCompletedEvent, string) {
	t.Helper()
	resolver := resolverWithChain(t, map[string]string{
		"synth_loop": "coord_root",
	})
	chainEntityID, err := resolver.ChainEntityID(context.Background(), "synth_loop")
	if err != nil {
		t.Fatalf("resolve chain entity: %v", err)
	}
	synthEntityID := loopEntityID(t, "synth_loop")
	chainTriples := map[string]any{
		chain.PredicateChainMode: "research_only",
	}
	for k, v := range chainTripleOverrides {
		chainTriples[k] = v
	}
	er := &fakeEntityReader{
		entities: map[string]map[string]any{
			synthEntityID: {
				agvocab.CoordinatorNextAction: "architect",
			},
			chainEntityID: chainTriples,
		},
	}
	pub := &recordingPublisher{}
	sub := NewSubscriber(pub, resolver, er, testPlatform(), threshold, authority, nil)
	ev := &agentic.LoopCompletedEvent{
		LoopID:  "synth_loop",
		Role:    "researcher-synthesize",
		Outcome: agentic.OutcomeSuccess,
	}
	return sub, pub, ev, chainEntityID
}

// --- Filter contract --------------------------------------------------

func TestSubscriber_NilEventNoOp(t *testing.T) {
	pub := &recordingPublisher{}
	sub := NewSubscriber(pub, resolverWithChain(t, nil), &fakeEntityReader{}, testPlatform(), 0, "", nil)
	if err := sub.HandleLoopCompleted(context.Background(), nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pub.triples) != 0 {
		t.Errorf("nil event must write no triples, got %d", len(pub.triples))
	}
}

func TestSubscriber_FailedOutcomeNoOp(t *testing.T) {
	pub := &recordingPublisher{}
	er := &fakeEntityReader{}
	sub := NewSubscriber(pub, resolverWithChain(t, nil), er, testPlatform(), 0, "", nil)
	ev := &agentic.LoopCompletedEvent{
		LoopID:  "synth_loop",
		Role:    "researcher-synthesize",
		Outcome: agentic.OutcomeFailed,
	}
	if err := sub.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if er.calls != 0 || len(pub.triples) != 0 {
		t.Error("failed outcome must skip both reads and writes (chainpause owns FAILED loops)")
	}
}

func TestSubscriber_NonProducerRoleNoOp(t *testing.T) {
	pub := &recordingPublisher{}
	er := &fakeEntityReader{}
	sub := NewSubscriber(pub, resolverWithChain(t, nil), er, testPlatform(), 0, "", nil)
	ev := &agentic.LoopCompletedEvent{
		LoopID:  "coord_loop",
		Role:    "coordinator",
		Outcome: agentic.OutcomeSuccess,
	}
	if err := sub.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if er.calls != 0 {
		t.Errorf("non-producer role must skip graph reads, got %d calls", er.calls)
	}
}

func TestSubscriber_EmptyLoopIDReturnsError(t *testing.T) {
	pub := &recordingPublisher{}
	er := &fakeEntityReader{}
	sub := NewSubscriber(pub, resolverWithChain(t, nil), er, testPlatform(), 0, "", nil)
	ev := &agentic.LoopCompletedEvent{
		LoopID:  "",
		Role:    "researcher-synthesize",
		Outcome: agentic.OutcomeSuccess,
	}
	if err := sub.HandleLoopCompleted(context.Background(), ev); err == nil {
		t.Error("empty LoopID must return error (ValidateLoopID gate)")
	}
}

// --- Read-failure fail-safe -------------------------------------------

func TestSubscriber_ProducerReadFailureNoOp(t *testing.T) {
	resolver := resolverWithChain(t, map[string]string{"synth_loop": "coord_root"})
	pub := &recordingPublisher{}
	synthEntityID := loopEntityID(t, "synth_loop")
	er := &fakeEntityReader{}
	wrapped := &selectiveErrorReader{base: er, errorOn: synthEntityID}
	sub := NewSubscriber(pub, resolver, wrapped, testPlatform(), 0, "", nil)
	ev := &agentic.LoopCompletedEvent{
		LoopID:  "synth_loop",
		Role:    "researcher-synthesize",
		Outcome: agentic.OutcomeSuccess,
	}
	if err := sub.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("read-failure path returned error: %v", err)
	}
	if len(pub.triples) != 0 {
		t.Errorf("producer-read failure must write no triples (fail-safe), got %d", len(pub.triples))
	}
}

func TestSubscriber_ChainReadFailureNoOp(t *testing.T) {
	resolver := resolverWithChain(t, map[string]string{"synth_loop": "coord_root"})
	chainEntityID, _ := resolver.ChainEntityID(context.Background(), "synth_loop")
	synthEntityID := loopEntityID(t, "synth_loop")
	pub := &recordingPublisher{}
	base := &fakeEntityReader{
		entities: map[string]map[string]any{
			synthEntityID: {agvocab.CoordinatorNextAction: "architect"},
		},
	}
	wrapped := &selectiveErrorReader{base: base, errorOn: chainEntityID}
	sub := NewSubscriber(pub, resolver, wrapped, testPlatform(), 0, "", nil)
	ev := &agentic.LoopCompletedEvent{
		LoopID:  "synth_loop",
		Role:    "researcher-synthesize",
		Outcome: agentic.OutcomeSuccess,
	}
	if err := sub.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("chain-read failure returned error: %v", err)
	}
	if len(pub.triples) != 0 {
		t.Errorf("chain-read failure must write no triples (fail-safe), got %d", len(pub.triples))
	}
}

// --- chain.terminal.reached short-circuit -----------------------------

func TestSubscriber_TerminalReachedSkips(t *testing.T) {
	sub, pub, ev, _ := synthesizeFixture(t, 0, "", map[string]any{
		chain.PredicateChainTerminalReached: "true",
	})
	if err := sub.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pub.triples) != 0 {
		t.Errorf("chain.terminal.reached=true must short-circuit (Slice 1c handled it), got %d triples", len(pub.triples))
	}
}

// --- No rejection detected --------------------------------------------

func TestSubscriber_SynthesizeWithEmitActionNoOp(t *testing.T) {
	// Synthesize emitting decide(emit) under research_only is the
	// expected happy path; no rejection.
	resolver := resolverWithChain(t, map[string]string{"synth_loop": "coord_root"})
	chainEntityID, _ := resolver.ChainEntityID(context.Background(), "synth_loop")
	synthEntityID := loopEntityID(t, "synth_loop")
	pub := &recordingPublisher{}
	er := &fakeEntityReader{
		entities: map[string]map[string]any{
			synthEntityID: {agvocab.CoordinatorNextAction: "emit"},
			chainEntityID: {chain.PredicateChainMode: "research_only"},
		},
	}
	sub := NewSubscriber(pub, resolver, er, testPlatform(), 0, "", nil)
	ev := &agentic.LoopCompletedEvent{
		LoopID:  "synth_loop",
		Role:    "researcher-synthesize",
		Outcome: agentic.OutcomeSuccess,
	}
	if err := sub.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pub.triples) != 0 {
		t.Errorf("synthesize emit under research_only must NOT trigger stall, got %d triples", len(pub.triples))
	}
}

func TestSubscriber_SynthesizeArchitectUnderDevViaSpecNoOp(t *testing.T) {
	// Synthesize decide(architect) under dev_via_spec is the canonical
	// success path — phase validator approves; no stall.
	sub, pub, ev, _ := synthesizeFixture(t, 0, "", map[string]any{
		chain.PredicateChainMode: "dev_via_spec",
	})
	if err := sub.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pub.triples) != 0 {
		t.Errorf("synthesize architect under dev_via_spec must NOT trigger stall, got %d triples", len(pub.triples))
	}
}

// --- Mode-mismatch happy path -----------------------------------------

func TestSubscriber_ModeMismatchFirstCycleRoutesCoordinator(t *testing.T) {
	sub, pub, ev, chainEntityID := synthesizeFixture(t, 0, "", nil)
	synthEntityID := loopEntityID(t, "synth_loop")

	if err := sub.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// chain entity must carry the full audit cluster.
	wantOnChain := map[string]string{
		chain.PredicateChainRejectionRecoveryCount: "1",
		chain.PredicateChainStallRoutedTo:          string(RouteCoordinator),
		chain.PredicateChainStallReason:            ReasonModeMismatch,
		chain.PredicateChainStallProducerLoopID:    "synth_loop",
		chain.PredicateChainStallProducerRole:      "researcher-synthesize",
	}
	for predicate, want := range wantOnChain {
		got, ok := pub.byPredicateOnSubject(predicate, chainEntityID)
		if !ok {
			t.Errorf("missing %s on chain entity", predicate)
			continue
		}
		if obj, ok := got.Object.(string); !ok || obj != want {
			t.Errorf("%s on chain entity = %v, want %q", predicate, got.Object, want)
		}
	}
	// observed_at must be present (timestamp content not pinned).
	if _, ok := pub.byPredicateOnSubject(chain.PredicateChainStallObservedAt, chainEntityID); !ok {
		t.Errorf("missing %s on chain entity", chain.PredicateChainStallObservedAt)
	}
	// awaiting_human + rejection.exhausted MUST be absent on coordinator-route
	// first cycle.
	if _, ok := pub.byPredicateOnSubject(chain.PredicateChainStallAwaitingHuman, chainEntityID); ok {
		t.Error("awaiting_human must NOT be present on coordinator-route first cycle")
	}
	if _, ok := pub.byPredicateOnSubject(chain.PredicateChainRejectionExhausted, chainEntityID); ok {
		t.Error("rejection.exhausted must NOT be present on coordinator-route first cycle")
	}

	// producer loop entity must carry the per-loop sentinels.
	wantOnLoop := map[string]string{
		chain.PredicateChainStallProceed: "true",
		chain.PredicateChainStallReason:  ReasonModeMismatch,
	}
	for predicate, want := range wantOnLoop {
		got, ok := pub.byPredicateOnSubject(predicate, synthEntityID)
		if !ok {
			t.Errorf("missing %s on producer loop entity", predicate)
			continue
		}
		if obj, ok := got.Object.(string); !ok || obj != want {
			t.Errorf("%s on producer loop = %v, want %q", predicate, got.Object, want)
		}
	}
}

// --- Cap-hit escalation -----------------------------------------------

func TestSubscriber_ModeMismatchCapHitForcesHuman(t *testing.T) {
	// threshold=2; prior count=2 → newCount=3, which exceeds threshold → human.
	sub, pub, ev, chainEntityID := synthesizeFixture(t, 2, "", map[string]any{
		chain.PredicateChainRejectionRecoveryCount: "2",
	})
	synthEntityID := loopEntityID(t, "synth_loop")

	if err := sub.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got, _ := pub.byPredicateOnSubject(chain.PredicateChainStallRoutedTo, chainEntityID); got.Object != string(RouteHuman) {
		t.Errorf("cap-hit must route human, got %v", got.Object)
	}
	if _, ok := pub.byPredicateOnSubject(chain.PredicateChainRejectionExhausted, chainEntityID); !ok {
		t.Error("cap-hit must stamp chain.rejection.exhausted=true")
	}
	if _, ok := pub.byPredicateOnSubject(chain.PredicateChainStallAwaitingHuman, chainEntityID); !ok {
		t.Error("human route must stamp chain.stall.awaiting_human=true")
	}
	// Cap-hit must NOT stamp the per-loop coordinator-route sentinel —
	// rule 05 must not fire on a cap-hit chain.
	if _, ok := pub.byPredicateOnSubject(chain.PredicateChainStallProceed, synthEntityID); ok {
		t.Error("cap-hit must NOT stamp chain.stall.proceed (rule 05 must not fire)")
	}
}

// --- Authority all_human override --------------------------------------

func TestSubscriber_AllHumanAuthorityOverridesCoordinator(t *testing.T) {
	sub, pub, ev, chainEntityID := synthesizeFixture(t, 0, AuthorityAllHuman, nil)
	synthEntityID := loopEntityID(t, "synth_loop")

	if err := sub.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, _ := pub.byPredicateOnSubject(chain.PredicateChainStallRoutedTo, chainEntityID); got.Object != string(RouteHuman) {
		t.Errorf("all_human authority must force route=human even for framing-fixable reasons, got %v", got.Object)
	}
	if _, ok := pub.byPredicateOnSubject(chain.PredicateChainStallAwaitingHuman, chainEntityID); !ok {
		t.Error("all_human authority must stamp awaiting_human=true")
	}
	if _, ok := pub.byPredicateOnSubject(chain.PredicateChainStallProceed, synthEntityID); ok {
		t.Error("all_human authority must NOT stamp chain.stall.proceed")
	}
}

func TestSubscriber_AuthorityFallsBackToPerReason(t *testing.T) {
	// Unrecognised authority value falls back to per_reason → coordinator
	// route for mode_mismatch.
	sub, pub, ev, chainEntityID := synthesizeFixture(t, 0, "bogus_value", nil)

	if err := sub.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, _ := pub.byPredicateOnSubject(chain.PredicateChainStallRoutedTo, chainEntityID); got.Object != string(RouteCoordinator) {
		t.Errorf("unrecognised authority must fall back to per_reason, got route=%v", got.Object)
	}
}

// --- Threshold boundary -----------------------------------------------

func TestSubscriber_ThresholdBoundaryAtCapValue(t *testing.T) {
	// threshold=3; prior count=2 → newCount=3, which equals threshold →
	// still coordinator route (cap-hit is strict >, not >=).
	sub, pub, ev, chainEntityID := synthesizeFixture(t, 3, "", map[string]any{
		chain.PredicateChainRejectionRecoveryCount: "2",
	})
	if err := sub.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, _ := pub.byPredicateOnSubject(chain.PredicateChainStallRoutedTo, chainEntityID); got.Object != string(RouteCoordinator) {
		t.Errorf("count == threshold must still route coordinator (strict-> cap), got %v", got.Object)
	}
	if _, ok := pub.byPredicateOnSubject(chain.PredicateChainRejectionExhausted, chainEntityID); ok {
		t.Error("count == threshold must NOT stamp exhausted (cap-hit is count > threshold)")
	}
}

// --- Redelivery idempotency (NATS at-least-once) ----------------------

func TestSubscriber_RedeliveryDoesNotDoubleIncrement(t *testing.T) {
	// Simulates NATS at-least-once redelivery: chain entity already
	// carries chain.stall.producer_loop_id matching ev.LoopID from a
	// prior successful fire. Subscriber must skip without writing any
	// new triples.
	sub, pub, ev, _ := synthesizeFixture(t, 0, "", map[string]any{
		chain.PredicateChainStallProducerLoopID:    "synth_loop",
		chain.PredicateChainRejectionRecoveryCount: "1",
	})
	if err := sub.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pub.triples) != 0 {
		t.Errorf("redelivery for same producer_loop_id must write no triples; got %d (double-fire would double-increment recovery_count)", len(pub.triples))
	}
}

func TestSubscriber_DifferentProducerNotSkipped(t *testing.T) {
	// Sanity inverse: a DIFFERENT prior producer_loop_id on the chain
	// entity must NOT short-circuit the current event's processing.
	// This is the legitimate-retry case — coordinator re-dispatched, a
	// new producer loop completed.
	sub, pub, ev, _ := synthesizeFixture(t, 0, "", map[string]any{
		chain.PredicateChainStallProducerLoopID:    "different_prior_synth_loop",
		chain.PredicateChainRejectionRecoveryCount: "1",
	})
	if err := sub.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pub.triples) == 0 {
		t.Error("different-producer rejection must still be processed (legitimate retry case)")
	}
}

// --- Partial-cluster recovery -----------------------------------------

func TestSubscriber_PartialAuditClusterFailureContinues(t *testing.T) {
	// Fail on the routed_to triple; the rest of the cluster MUST still land.
	resolver := resolverWithChain(t, map[string]string{"synth_loop": "coord_root"})
	chainEntityID, _ := resolver.ChainEntityID(context.Background(), "synth_loop")
	synthEntityID := loopEntityID(t, "synth_loop")
	er := &fakeEntityReader{
		entities: map[string]map[string]any{
			synthEntityID: {agvocab.CoordinatorNextAction: "architect"},
			chainEntityID: {chain.PredicateChainMode: "research_only"},
		},
	}
	pub := &flakyPublisher{failOn: chain.PredicateChainStallRoutedTo}
	sub := NewSubscriber(pub, resolver, er, testPlatform(), 0, "", nil)
	ev := &agentic.LoopCompletedEvent{
		LoopID:  "synth_loop",
		Role:    "researcher-synthesize",
		Outcome: agentic.OutcomeSuccess,
	}
	if err := sub.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("partial-failure path returned error: %v", err)
	}
	// The other audit triples must still land — partial cluster is queryable.
	const wantAtLeast = 5 // recovery_count, reason, producer_loop_id, producer_role, observed_at (+ proceed/reason on loop)
	if len(pub.written) < wantAtLeast {
		t.Errorf("partial failure should still land %d+ triples, got %d", wantAtLeast, len(pub.written))
	}
}
