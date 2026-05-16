package chain

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/types"
	agvocab "github.com/c360studio/semstreams/vocabulary/agentic"
)

// recordingTerminalPublisher records AddTriple calls. Local to terminal_test
// to keep dependencies tight (sibling test files have their own equivalents).
type recordingTerminalPublisher struct {
	mu      sync.Mutex
	triples []message.Triple
	err     error
}

func (r *recordingTerminalPublisher) AddTriple(_ context.Context, t message.Triple) error {
	if r.err != nil {
		return r.err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.triples = append(r.triples, t)
	return nil
}

func (r *recordingTerminalPublisher) byPredicate(pred string) (message.Triple, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range r.triples {
		if t.Predicate == pred {
			return t, true
		}
	}
	return message.Triple{}, false
}

// stubTerminalEntityReader returns a fixed triple map for one entity ID and
// empty otherwise. Mirrors the chainmode/stamper_test.go shape so cross-
// package debugging stays predictable.
type stubTerminalEntityReader struct {
	entityID string
	triples  map[string]any
	err      error
}

func (s *stubTerminalEntityReader) ReadEntity(_ context.Context, entityID string) (map[string]any, error) {
	if s.err != nil {
		return nil, s.err
	}
	if entityID != s.entityID {
		return map[string]any{}, nil
	}
	return s.triples, nil
}

// stubTerminalParentReader is a minimal ParentReader returning configured
// parent entity IDs (or empty for chain root). Single-hop ancestry only;
// multi-hop tests use the full Resolver against a richer reader.
type stubTerminalParentReader struct {
	parents map[string]string
	err     error
}

func (s *stubTerminalParentReader) ReadParent(_ context.Context, loopID string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return s.parents[loopID], nil
}

func terminalTestPlatform() types.PlatformMeta {
	return types.PlatformMeta{Org: "c360", Platform: "test"}
}

// terminalLoopEntityID composes a 6-part loop entity id under the test
// platform — matches the chainmode test helper for consistency.
func terminalLoopEntityID(loopID string) string {
	return agentic.LoopExecutionEntityID("c360", "test", loopID)
}

func terminalChainEntityID(loopID string) string {
	return agentic.ChainExecutionEntityID("c360", "test", loopID)
}

// newTerminalStamperWith builds a TerminalStamper with a single
// stubbed reviewer entity. parents threads ancestry for the resolver.
func newTerminalStamperWith(reviewerLoopID, action string, parents map[string]string, pub *recordingTerminalPublisher) *TerminalStamper {
	resolver := NewResolver(&stubTerminalParentReader{parents: parents}, terminalTestPlatform())
	er := &stubTerminalEntityReader{
		entityID: terminalLoopEntityID(reviewerLoopID),
		triples: map[string]any{
			agvocab.CoordinatorNextAction: action,
		},
	}
	return NewTerminalStamper(pub, resolver, er, terminalTestPlatform(), nil)
}

// TestChainTerminal_ResearchApproved_StampsCluster — happy path for the
// research_only chain. reviewer-research(approved) lands all four triples
// on the chain entity.
func TestChainTerminal_ResearchApproved_StampsCluster(t *testing.T) {
	pub := &recordingTerminalPublisher{}
	s := newTerminalStamperWith("reviewer_xyz", approvedAction, map[string]string{}, pub)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "reviewer_xyz",
		Role:    reviewerResearchRole,
		Outcome: agentic.OutcomeSuccess,
	}
	if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantSubject := terminalChainEntityID("reviewer_xyz")
	cases := map[string]any{
		PredicateChainTerminalReached: "true",
		PredicateChainTerminalRole:    reviewerResearchRole,
		PredicateChainTerminalLoopID:  "reviewer_xyz",
		// observed_at is timestamp-driven; presence-only check below.
	}
	for pred, wantObj := range cases {
		got, ok := pub.byPredicate(pred)
		if !ok {
			t.Errorf("predicate %q missing", pred)
			continue
		}
		if got.Subject != wantSubject {
			t.Errorf("predicate %q subject: got %q, want %q", pred, got.Subject, wantSubject)
		}
		if got.Object != wantObj {
			t.Errorf("predicate %q object: got %v, want %v", pred, got.Object, wantObj)
		}
		if got.Source != chainTerminalSource {
			t.Errorf("predicate %q source: got %q, want %q", pred, got.Source, chainTerminalSource)
		}
	}
	if _, ok := pub.byPredicate(PredicateChainTerminalObservedAt); !ok {
		t.Error("predicate chain.terminal.observed_at missing (presence-only)")
	}
}

// TestChainTerminal_QAAccept_StampsCluster — happy path for the
// dev_via_spec chain. reviewer-qa(accept) lands all four triples.
func TestChainTerminal_QAAccept_StampsCluster(t *testing.T) {
	pub := &recordingTerminalPublisher{}
	s := newTerminalStamperWith("reviewer_qa_xyz", acceptAction, map[string]string{}, pub)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "reviewer_qa_xyz",
		Role:    reviewerQARole,
		Outcome: agentic.OutcomeSuccess,
	}
	if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, ok := pub.byPredicate(PredicateChainTerminalRole)
	if !ok {
		t.Fatal("chain.terminal.role not written")
	}
	if got.Object != reviewerQARole {
		t.Errorf("Object: got %v, want %q", got.Object, reviewerQARole)
	}
}

// TestChainTerminal_NonTerminalActions_NoOp — every other reviewer action
// (insufficient, reject, needs_clarification, empty) MUST be a no-op for
// each terminal role. Adding a new approval token requires extending
// roleToApprovalAction; the table here pins the closed taxonomy.
func TestChainTerminal_NonTerminalActions_NoOp(t *testing.T) {
	cases := []struct {
		role   string
		action string
	}{
		// reviewer-research's non-approval terminals
		{reviewerResearchRole, "insufficient"},
		{reviewerResearchRole, "needs_clarification"},
		{reviewerResearchRole, ""},
		// reviewer-research must NOT fire on QA's approval token
		{reviewerResearchRole, acceptAction},
		// reviewer-qa's non-approval terminals
		{reviewerQARole, "reject"},
		{reviewerQARole, "needs_clarification"},
		{reviewerQARole, ""},
		// reviewer-qa must NOT fire on research's approval token
		{reviewerQARole, approvedAction},
	}
	for _, c := range cases {
		t.Run(c.role+"/"+c.action, func(t *testing.T) {
			pub := &recordingTerminalPublisher{}
			s := newTerminalStamperWith("reviewer_x", c.action, map[string]string{}, pub)
			ev := &agentic.LoopCompletedEvent{
				LoopID:  "reviewer_x",
				Role:    c.role,
				Outcome: agentic.OutcomeSuccess,
			}
			if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(pub.triples) != 0 {
				t.Errorf("expected no triples for role=%q action=%q, got %d", c.role, c.action, len(pub.triples))
			}
		})
	}
}

// TestChainTerminal_NonTerminalRoles_NoOp — shared agent.complete.>
// subscription fans every role through this handler. Roles outside the
// closed terminal taxonomy must not produce writes.
func TestChainTerminal_NonTerminalRoles_NoOp(t *testing.T) {
	pub := &recordingTerminalPublisher{}
	s := newTerminalStamperWith("loop_id", approvedAction, map[string]string{}, pub)
	for _, role := range []string{"coordinator", "researcher-plan", "researcher-synthesize", "builder", "dispatch", ""} {
		ev := &agentic.LoopCompletedEvent{
			LoopID:  "loop_id",
			Role:    role,
			Outcome: agentic.OutcomeSuccess,
		}
		if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
			t.Fatalf("unexpected error for role=%q: %v", role, err)
		}
	}
	if len(pub.triples) != 0 {
		t.Errorf("expected no triples for non-terminal roles, got %d", len(pub.triples))
	}
}

// TestChainTerminal_FailedOutcome_NoOp — a reviewer that failed (api
// overload, timeout) leaves no terminal triple to read. Skip clean.
func TestChainTerminal_FailedOutcome_NoOp(t *testing.T) {
	pub := &recordingTerminalPublisher{}
	s := newTerminalStamperWith("reviewer_xyz", approvedAction, map[string]string{}, pub)
	ev := &agentic.LoopCompletedEvent{
		LoopID:  "reviewer_xyz",
		Role:    reviewerResearchRole,
		Outcome: agentic.OutcomeFailed,
	}
	if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pub.triples) != 0 {
		t.Errorf("expected no triples for failed reviewer, got %d", len(pub.triples))
	}
}

// TestChainTerminal_NilEvent_NoOp — defensive against malformed envelope
// decode; must not panic the subscriber goroutine.
func TestChainTerminal_NilEvent_NoOp(t *testing.T) {
	pub := &recordingTerminalPublisher{}
	s := newTerminalStamperWith("reviewer_xyz", approvedAction, map[string]string{}, pub)
	if err := s.HandleLoopCompleted(context.Background(), nil); err != nil {
		t.Fatalf("unexpected error on nil event: %v", err)
	}
	if len(pub.triples) != 0 {
		t.Errorf("expected no triples for nil event, got %d", len(pub.triples))
	}
}

// TestChainTerminal_InvalidLoopID_Errors — wire envelope without Validate
// would carry an empty / dotted LoopID; reject before the constructor panics.
func TestChainTerminal_InvalidLoopID_Errors(t *testing.T) {
	cases := []string{"", "loop.with.dots", "loop.id"}
	for _, loopID := range cases {
		t.Run("loop_id="+loopID, func(t *testing.T) {
			pub := &recordingTerminalPublisher{}
			s := newTerminalStamperWith("reviewer_xyz", approvedAction, map[string]string{}, pub)
			ev := &agentic.LoopCompletedEvent{
				LoopID:  loopID,
				Role:    reviewerResearchRole,
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

// TestChainTerminal_MultiHopAncestry — chain entity must be built from
// chain root, not the reviewer's loop_id. Pin the multi-hop walk so a
// dispatch-parent or coordinator-parent topology doesn't silently target
// the wrong entity.
func TestChainTerminal_MultiHopAncestry(t *testing.T) {
	pub := &recordingTerminalPublisher{}
	// dispatch_root → coord_abc → researcher_a → reviewer_b
	parents := map[string]string{
		"reviewer_b":    terminalLoopEntityID("researcher_a"),
		"researcher_a":  terminalLoopEntityID("coord_abc"),
		"coord_abc":     terminalLoopEntityID("dispatch_root"),
		"dispatch_root": "",
	}
	resolver := NewResolver(&stubTerminalParentReader{parents: parents}, terminalTestPlatform())
	er := &stubTerminalEntityReader{
		entityID: terminalLoopEntityID("reviewer_b"),
		triples: map[string]any{
			agvocab.CoordinatorNextAction: approvedAction,
		},
	}
	s := NewTerminalStamper(pub, resolver, er, terminalTestPlatform(), nil)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "reviewer_b",
		Role:    reviewerResearchRole,
		Outcome: agentic.OutcomeSuccess,
	}
	if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, ok := pub.byPredicate(PredicateChainTerminalReached)
	if !ok {
		t.Fatal("chain.terminal.reached not written")
	}
	wantSubject := terminalChainEntityID("dispatch_root")
	if got.Subject != wantSubject {
		t.Errorf("Subject: got %q, want %q (chain entity must be built from chain root, not reviewer loop_id)", got.Subject, wantSubject)
	}
}

// TestChainTerminal_EntityReadFailure_Skips — graph blip looking up the
// reviewer's terminal action must not propagate to the subscriber.
func TestChainTerminal_EntityReadFailure_Skips(t *testing.T) {
	pub := &recordingTerminalPublisher{}
	resolver := NewResolver(&stubTerminalParentReader{parents: map[string]string{}}, terminalTestPlatform())
	er := &stubTerminalEntityReader{err: errors.New("graph KV unavailable")}
	s := NewTerminalStamper(pub, resolver, er, terminalTestPlatform(), nil)
	ev := &agentic.LoopCompletedEvent{
		LoopID:  "reviewer_xyz",
		Role:    reviewerResearchRole,
		Outcome: agentic.OutcomeSuccess,
	}
	if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pub.triples) != 0 {
		t.Errorf("expected no triples on entity-read failure, got %d", len(pub.triples))
	}
}

// TestChainTerminal_ResolverFailure_Skips — ancestry walk blip skips
// fail-safe so a graph hiccup doesn't lose the chain terminal.
func TestChainTerminal_ResolverFailure_Skips(t *testing.T) {
	pub := &recordingTerminalPublisher{}
	resolver := NewResolver(&stubTerminalParentReader{err: errors.New("parent reader down")}, terminalTestPlatform())
	er := &stubTerminalEntityReader{
		entityID: terminalLoopEntityID("reviewer_xyz"),
		triples: map[string]any{
			agvocab.CoordinatorNextAction: approvedAction,
		},
	}
	s := NewTerminalStamper(pub, resolver, er, terminalTestPlatform(), nil)
	ev := &agentic.LoopCompletedEvent{
		LoopID:  "reviewer_xyz",
		Role:    reviewerResearchRole,
		Outcome: agentic.OutcomeSuccess,
	}
	if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pub.triples) != 0 {
		t.Errorf("expected no triples on resolver failure, got %d", len(pub.triples))
	}
}

// TestChainTerminal_PublishFailure_DoesNotPropagate — every-write fails
// permanently. The stamper must log Warn for each but return nil so the
// dispatcher doesn't see an error.
func TestChainTerminal_PublishFailure_DoesNotPropagate(t *testing.T) {
	pub := &recordingTerminalPublisher{err: errors.New("graph mutation refused")}
	s := newTerminalStamperWith("reviewer_xyz", approvedAction, map[string]string{}, pub)
	ev := &agentic.LoopCompletedEvent{
		LoopID:  "reviewer_xyz",
		Role:    reviewerResearchRole,
		Outcome: agentic.OutcomeSuccess,
	}
	if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error on publish failure: %v", err)
	}
}

// partialFailurePublisher returns an error on the Nth AddTriple call
// only; all other calls succeed and record. Used to drive the
// partial-cluster contract — when one triple in a multi-triple cluster
// fails, the remaining triples MUST still land so a graph query sees
// a degraded-but-queryable record rather than zero state.
type partialFailurePublisher struct {
	mu        sync.Mutex
	triples   []message.Triple
	failOn    int // 0-indexed call number that fails
	callsSeen int
}

func (p *partialFailurePublisher) AddTriple(_ context.Context, t message.Triple) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	idx := p.callsSeen
	p.callsSeen++
	if idx == p.failOn {
		return errors.New("transient graph mutation refused")
	}
	p.triples = append(p.triples, t)
	return nil
}

// TestChainTerminal_PartialClusterFailure_RemainingTriplesLand pins the
// per-write fail-soft contract: a transient graph blip on one triple
// must not abort the cluster. terminal.go:190-197 iterates and logs
// per write; the test asserts the other 3 triples are recorded.
func TestChainTerminal_PartialClusterFailure_RemainingTriplesLand(t *testing.T) {
	// Fail the second AddTriple call (chain.terminal.role); the other
	// three (reached, loop_id, observed_at) must land.
	pub := &partialFailurePublisher{failOn: 1}
	resolver := NewResolver(&stubTerminalParentReader{parents: map[string]string{}}, terminalTestPlatform())
	er := &stubTerminalEntityReader{
		entityID: terminalLoopEntityID("reviewer_xyz"),
		triples: map[string]any{
			agvocab.CoordinatorNextAction: approvedAction,
		},
	}
	s := NewTerminalStamper(pub, resolver, er, terminalTestPlatform(), nil)
	ev := &agentic.LoopCompletedEvent{
		LoopID:  "reviewer_xyz",
		Role:    reviewerResearchRole,
		Outcome: agentic.OutcomeSuccess,
	}
	if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pub.mu.Lock()
	defer pub.mu.Unlock()
	if pub.callsSeen != 4 {
		t.Fatalf("expected 4 AddTriple calls, got %d (stamper aborted early)", pub.callsSeen)
	}
	if len(pub.triples) != 3 {
		t.Fatalf("expected 3 triples landed (4 attempted, 1 failed), got %d", len(pub.triples))
	}
	// Verify chain.terminal.role is the one MISSING — the others must
	// be present.
	gotPredicates := map[string]bool{}
	for _, tr := range pub.triples {
		gotPredicates[tr.Predicate] = true
	}
	if gotPredicates[PredicateChainTerminalRole] {
		t.Error("chain.terminal.role should not have landed (failOn=1 targeted this predicate)")
	}
	for _, pred := range []string{PredicateChainTerminalReached, PredicateChainTerminalLoopID, PredicateChainTerminalObservedAt} {
		if !gotPredicates[pred] {
			t.Errorf("predicate %q missing (must land despite partial failure)", pred)
		}
	}
}

// TestRoleToApprovalAction_TablePin — locks the role→approval-action map
// so any new terminating reviewer requires touching this table AND adding
// the corresponding configs/rules/coordinator/ wake-up rule.
func TestRoleToApprovalAction_TablePin(t *testing.T) {
	want := map[string]string{
		"reviewer-research": "approved",
		"reviewer-qa":       "accept",
	}
	if len(roleToApprovalAction) != len(want) {
		t.Fatalf("roleToApprovalAction size: got %d, want %d (any new entry must add a corresponding configs/rules/coordinator/ wake-up rule)", len(roleToApprovalAction), len(want))
	}
	for k, v := range want {
		got, ok := roleToApprovalAction[k]
		if !ok {
			t.Errorf("roleToApprovalAction missing key %q", k)
			continue
		}
		if got != v {
			t.Errorf("roleToApprovalAction[%q]: got %q, want %q", k, got, v)
		}
	}
}
