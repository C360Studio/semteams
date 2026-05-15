package chainmode

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/types"
	agvocab "github.com/c360studio/semstreams/vocabulary/agentic"

	"github.com/c360studio/semteams/cmd/semteams/chain"
)

// recordingPublisher records AddTriple calls. Mirrors the shape used
// across cmd/semteams/{chain,chainpause}/ test files — staying
// consistent makes cross-package debugging predictable.
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

// stubEntityReader returns a fixed triple map for one entity ID and
// errors for anything else. Tests sized for one read per call so the
// single-entity shape is enough.
type stubEntityReader struct {
	entityID string
	triples  map[string]any
	err      error
}

func (s *stubEntityReader) ReadEntity(_ context.Context, entityID string) (map[string]any, error) {
	if s.err != nil {
		return nil, s.err
	}
	if entityID != s.entityID {
		return map[string]any{}, nil
	}
	return s.triples, nil
}

// stubParentReader maps loop_id → parent entity ID. Used to drive the
// resolver without a live NATS round-trip. Empty value (or absent key)
// signals chain root.
type stubParentReader struct {
	parents map[string]string
	err     error
}

func (s *stubParentReader) ReadParent(_ context.Context, loopID string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return s.parents[loopID], nil
}

func testPlatform() types.PlatformMeta {
	return types.PlatformMeta{Org: "c360", Platform: "test"}
}

// loopEntityID composes a 6-part loop entity id under testPlatform.
func loopEntityID(loopID string) string {
	return agentic.LoopExecutionEntityID("c360", "test", loopID)
}

// chainEntityID composes a 6-part chain entity id under testPlatform.
func chainEntityID(loopID string) string {
	return agentic.ChainExecutionEntityID("c360", "test", loopID)
}

func newStamperWith(t *testing.T, action string, parents map[string]string, pub *recordingPublisher) *Stamper {
	t.Helper()
	resolver := chain.NewResolver(&stubParentReader{parents: parents}, testPlatform())
	er := &stubEntityReader{
		entityID: loopEntityID("coord_abc"),
		triples: map[string]any{
			agvocab.CoordinatorNextAction: action,
		},
	}
	return NewStamper(pub, resolver, er, testPlatform(), nil)
}

// TestStamper_DelegateResearch_StampsResearchOnly: the happy-path
// case for the research-only chain.
func TestStamper_DelegateResearch_StampsResearchOnly(t *testing.T) {
	pub := &recordingPublisher{}
	s := newStamperWith(t, actionDelegateResearch, map[string]string{}, pub)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "coord_abc",
		Role:    coordinatorRole,
		Outcome: agentic.OutcomeSuccess,
	}
	if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, ok := pub.byPredicate(chain.PredicateChainMode)
	if !ok {
		t.Fatal("chain.mode not written")
	}
	if got.Subject != chainEntityID("coord_abc") {
		t.Errorf("Subject: got %q, want %q", got.Subject, chainEntityID("coord_abc"))
	}
	if got.Object != ModeResearchOnly {
		t.Errorf("Object: got %v, want %q", got.Object, ModeResearchOnly)
	}
	if got.Source != triplesSource {
		t.Errorf("Source: got %q, want %q", got.Source, triplesSource)
	}
	if got.Confidence != 1.0 {
		t.Errorf("Confidence: got %v, want 1.0", got.Confidence)
	}
}

// TestStamper_DelegateDevChain_StampsDevViaSpec: the happy-path
// case for the dev-via-spec chain.
func TestStamper_DelegateDevChain_StampsDevViaSpec(t *testing.T) {
	pub := &recordingPublisher{}
	s := newStamperWith(t, actionDelegateDevChain, map[string]string{}, pub)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "coord_abc",
		Role:    coordinatorRole,
		Outcome: agentic.OutcomeSuccess,
	}
	if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, ok := pub.byPredicate(chain.PredicateChainMode)
	if !ok {
		t.Fatal("chain.mode not written")
	}
	if got.Object != ModeDevViaSpec {
		t.Errorf("Object: got %v, want %q", got.Object, ModeDevViaSpec)
	}
}

// TestStamper_NonDelegateActions_NoOp covers every other coordinator
// action — respond_direct, ask_user, needs_clarification, and the
// empty-string case — to lock the "no chain spawned, no mode stamp"
// contract. Adding a third routing class (Slice 3) is intended to fail
// this table until the new action is mapped.
func TestStamper_NonDelegateActions_NoOp(t *testing.T) {
	cases := []string{"respond_direct", "ask_user", "needs_clarification", ""}
	for _, action := range cases {
		t.Run("action="+action, func(t *testing.T) {
			pub := &recordingPublisher{}
			s := newStamperWith(t, action, map[string]string{}, pub)

			ev := &agentic.LoopCompletedEvent{
				LoopID:  "coord_abc",
				Role:    coordinatorRole,
				Outcome: agentic.OutcomeSuccess,
			}
			if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(pub.triples) != 0 {
				t.Errorf("expected no triples for action=%q, got %d", action, len(pub.triples))
			}
		})
	}
}

// TestStamper_NonCoordinatorRole_NoOp: shared agent.complete.>
// subscription will fan events from every role to this handler — a
// researcher / builder / reviewer event must not produce a chain.mode
// write.
func TestStamper_NonCoordinatorRole_NoOp(t *testing.T) {
	pub := &recordingPublisher{}
	s := newStamperWith(t, actionDelegateResearch, map[string]string{}, pub)

	for _, role := range []string{"researcher-plan", "builder", "reviewer-research", "dispatch", ""} {
		ev := &agentic.LoopCompletedEvent{
			LoopID:  "coord_abc",
			Role:    role,
			Outcome: agentic.OutcomeSuccess,
		}
		if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
			t.Fatalf("unexpected error for role=%q: %v", role, err)
		}
	}
	if len(pub.triples) != 0 {
		t.Errorf("expected no triples for non-coordinator roles, got %d", len(pub.triples))
	}
}

// TestStamper_FailedOutcome_NoOp: a coordinator that failed (e.g. API
// overload) leaves no terminal triple to read — skip without trying.
func TestStamper_FailedOutcome_NoOp(t *testing.T) {
	pub := &recordingPublisher{}
	s := newStamperWith(t, actionDelegateResearch, map[string]string{}, pub)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "coord_abc",
		Role:    coordinatorRole,
		Outcome: agentic.OutcomeFailed,
	}
	if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pub.triples) != 0 {
		t.Errorf("expected no triples for failed coordinator, got %d", len(pub.triples))
	}
}

// TestStamper_NilEvent_NoOp: defensive — malformed envelope decode
// must not panic the subscriber goroutine.
func TestStamper_NilEvent_NoOp(t *testing.T) {
	pub := &recordingPublisher{}
	s := newStamperWith(t, actionDelegateResearch, map[string]string{}, pub)
	if err := s.HandleLoopCompleted(context.Background(), nil); err != nil {
		t.Fatalf("unexpected error on nil event: %v", err)
	}
	if len(pub.triples) != 0 {
		t.Errorf("expected no triples for nil event, got %d", len(pub.triples))
	}
}

// TestStamper_InvalidLoopID_Errors: a wire envelope that didn't go
// through Validate (subscriber path) might carry an empty OR
// dotted LoopID, either of which would panic LoopExecutionEntityID
// downstream. chain.ValidateLoopID is the shared entry-seam check;
// the stamper surfaces violations as an error before reaching the
// constructor.
func TestStamper_InvalidLoopID_Errors(t *testing.T) {
	cases := []string{"", "loop.with.dots", "loop.id"}
	for _, loopID := range cases {
		t.Run("loop_id="+loopID, func(t *testing.T) {
			pub := &recordingPublisher{}
			s := newStamperWith(t, actionDelegateResearch, map[string]string{}, pub)

			ev := &agentic.LoopCompletedEvent{
				LoopID:  loopID,
				Role:    coordinatorRole,
				Outcome: agentic.OutcomeSuccess,
			}
			if err := s.HandleLoopCompleted(context.Background(), ev); err == nil {
				t.Fatalf("expected error on invalid loopID %q, got nil", loopID)
			}
			if len(pub.triples) != 0 {
				t.Errorf("expected no triples on invalid loopID %q, got %d", loopID, len(pub.triples))
			}
		})
	}
}

// TestStamper_MultiHopChainAncestry: the stamper's docstring asserts
// "the coordinator is the chain root so the walk is one-hop; the
// abstraction holds if a future config spawns coordinator off a
// dispatch parent." Pin the multi-hop case so a config evolution
// (coordinator becomes a child loop of a higher dispatch) doesn't
// silently break chain-entity targeting. The resolver walks
// coord_abc → dispatch_root → (no parent → root). The triple lands
// on the chain entity built from dispatch_root, not from coord_abc.
func TestStamper_MultiHopChainAncestry(t *testing.T) {
	pub := &recordingPublisher{}
	parents := map[string]string{
		"coord_abc": loopEntityID("dispatch_root"),
	}
	resolver := chain.NewResolver(&stubParentReader{parents: parents}, testPlatform())
	er := &stubEntityReader{
		entityID: loopEntityID("coord_abc"),
		triples: map[string]any{
			agvocab.CoordinatorNextAction: actionDelegateDevChain,
		},
	}
	s := NewStamper(pub, resolver, er, testPlatform(), nil)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "coord_abc",
		Role:    coordinatorRole,
		Outcome: agentic.OutcomeSuccess,
	}
	if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, ok := pub.byPredicate(chain.PredicateChainMode)
	if !ok {
		t.Fatal("chain.mode.classification not written")
	}
	wantSubject := chainEntityID("dispatch_root")
	if got.Subject != wantSubject {
		t.Errorf("Subject: got %q, want %q (chain entity must be built from chain root, not coordinator loop_id)", got.Subject, wantSubject)
	}
	if got.Object != ModeDevViaSpec {
		t.Errorf("Object: got %v, want %q", got.Object, ModeDevViaSpec)
	}
}

// TestStamper_EntityReadFailure_Skips: graph read blip while looking
// up the coordinator's terminal action must not propagate as a panic
// or error to the subscriber. Skip fail-safe.
func TestStamper_EntityReadFailure_Skips(t *testing.T) {
	pub := &recordingPublisher{}
	resolver := chain.NewResolver(&stubParentReader{parents: map[string]string{}}, testPlatform())
	er := &stubEntityReader{err: errors.New("graph KV unavailable")}
	s := NewStamper(pub, resolver, er, testPlatform(), nil)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "coord_abc",
		Role:    coordinatorRole,
		Outcome: agentic.OutcomeSuccess,
	}
	if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pub.triples) != 0 {
		t.Errorf("expected no triples on entity-read failure, got %d", len(pub.triples))
	}
}

// TestStamper_ResolverFailure_Skips: ancestry walk blip must skip
// fail-safe so a graph hiccup doesn't drop the chain.
func TestStamper_ResolverFailure_Skips(t *testing.T) {
	pub := &recordingPublisher{}
	resolver := chain.NewResolver(&stubParentReader{err: errors.New("parent reader down")}, testPlatform())
	er := &stubEntityReader{
		entityID: loopEntityID("coord_abc"),
		triples: map[string]any{
			agvocab.CoordinatorNextAction: actionDelegateResearch,
		},
	}
	s := NewStamper(pub, resolver, er, testPlatform(), nil)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "coord_abc",
		Role:    coordinatorRole,
		Outcome: agentic.OutcomeSuccess,
	}
	if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pub.triples) != 0 {
		t.Errorf("expected no triples on resolver failure, got %d", len(pub.triples))
	}
}

// TestStamper_PublishFailure_Skips: a publisher failure mid-handler
// must not propagate (returned-error short-circuits the
// agent.complete subscription's per-handler logging only; we want
// the warn log but no error propagation to the dispatcher).
func TestStamper_PublishFailure_Skips(t *testing.T) {
	pub := &recordingPublisher{err: errors.New("graph mutation refused")}
	s := newStamperWith(t, actionDelegateResearch, map[string]string{}, pub)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "coord_abc",
		Role:    coordinatorRole,
		Outcome: agentic.OutcomeSuccess,
	}
	if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestActionToMode_TablePin: locks the action→mode map so any new
// coordinator action requires touching this table — keeping the
// stamper and the phase validator's mode gate (cmd/semteams/
// phasevalidator) in sync as the taxonomy grows.
func TestActionToMode_TablePin(t *testing.T) {
	want := map[string]string{
		"delegate_research":  ModeResearchOnly,
		"delegate_dev_chain": ModeDevViaSpec,
	}
	if len(actionToMode) != len(want) {
		t.Fatalf("actionToMode size: got %d, want %d (any new entry must extend phasevalidator's mode gate)", len(actionToMode), len(want))
	}
	for k, v := range want {
		got, ok := actionToMode[k]
		if !ok {
			t.Errorf("actionToMode missing key %q", k)
			continue
		}
		if got != v {
			t.Errorf("actionToMode[%q]: got %q, want %q", k, got, v)
		}
	}
}
