package phasevalidator

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

// fakeParentReader implements chain.ParentReader.
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

// fakeEntityReader implements chain.EntityTripleReader.
type fakeEntityReader struct {
	entities map[string]map[string]any
	err      error
}

func (f *fakeEntityReader) ReadEntity(_ context.Context, entityID string) (map[string]any, error) {
	if f.err != nil {
		return nil, f.err
	}
	if m, ok := f.entities[entityID]; ok {
		return m, nil
	}
	return map[string]any{}, nil
}

// recordingPublisher captures every AddTriple invocation.
type recordingPublisher struct {
	triples []message.Triple
}

func (p *recordingPublisher) AddTriple(_ context.Context, t message.Triple) error {
	p.triples = append(p.triples, t)
	return nil
}

func (p *recordingPublisher) byPredicate(predicate string) (message.Triple, bool) {
	for _, t := range p.triples {
		if t.Predicate == predicate {
			return t, true
		}
	}
	return message.Triple{}, false
}

func (p *recordingPublisher) hasPredicate(predicate string) bool {
	_, ok := p.byPredicate(predicate)
	return ok
}

const (
	testOrg    = "c360"
	testPlat   = "test"
	testLoopID = "researcher_x"
	testRootID = "dispatch_root"
)

// testPlatformMeta returns the canonical test platform.
func testPlatformMeta() types.PlatformMeta {
	return types.PlatformMeta{Org: testOrg, Platform: testPlat}
}

// loopEntityID returns the 6-part execution entity ID for a loop_id.
func loopEntityID(loopID string) string {
	return agentic.LoopExecutionEntityID(testOrg, testPlat, loopID)
}

// chainEntityID returns the 6-part chain execution entity ID for a
// chain_id (which is the root loop_id).
func chainEntityID(chainID string) string {
	return agentic.ChainExecutionEntityID(testOrg, testPlat, chainID)
}

// resolverWithChain builds a Resolver whose parent map walks the given
// loop_id → parent loop_id pairs. Mirrors recoverycounter's helper.
// Loops not in the map resolve to chain root (i.e., that loop_id IS the
// chain_id). The validator's resolver call from testLoopID walks until
// it reaches a loop with no parent, then uses ChainExecutionEntityID.
func resolverWithChain(parents map[string]string) *chain.Resolver {
	entityParents := make(map[string]string, len(parents))
	for child, parent := range parents {
		entityParents[child] = loopEntityID(parent)
	}
	return chain.NewResolver(&fakeParentReader{parents: entityParents}, testPlatformMeta())
}

// buildValidator stamps up a PhaseValidator backed by the supplied
// fakes plus a one-hop chain (testLoopID → testRootID; root → chain
// entity). All tests use the same lineage so chain entity ID is
// deterministic.
func buildValidator(_ *testing.T, pub TriplePublisher, entities chain.EntityTripleReader) *PhaseValidator {
	resolver := resolverWithChain(map[string]string{
		testLoopID: testRootID,
	})
	return NewPhaseValidator(pub, resolver, entities, testPlatformMeta(), nil)
}

// stampEvent builds a LoopCompletedEvent with the supplied role.
func stampEvent(role string) *agentic.LoopCompletedEvent {
	return &agentic.LoopCompletedEvent{
		LoopID:  testLoopID,
		Role:    role,
		Outcome: agentic.OutcomeSuccess,
	}
}

// testChainEntityID is the resolver-derived chain entity for tests.
var testChainEntityID = chainEntityID(testRootID)

func TestAllowedEdgeTable(t *testing.T) {
	// Exhaustive: every cell of the 5x5 (input × target including emit)
	// is asserted against the documented allowed set. Catches any drift
	// in allowedEdges without trusting the closed-over private map.
	cases := []struct {
		from, to string
		want     bool
	}{
		// plan
		{PhasePlan, PhaseGather, true},
		{PhasePlan, "emit", true},
		{PhasePlan, PhaseSynthesize, false},
		{PhasePlan, PhaseArchitect, false},
		{PhasePlan, PhasePlan, false},
		// gather
		{PhaseGather, PhaseSynthesize, true},
		{PhaseGather, PhasePlan, false},
		{PhaseGather, "emit", false},
		{PhaseGather, PhaseArchitect, false},
		{PhaseGather, PhaseGather, false},
		// synthesize
		{PhaseSynthesize, PhaseGather, true},
		{PhaseSynthesize, PhaseArchitect, true},
		{PhaseSynthesize, PhasePlan, false},
		{PhaseSynthesize, "emit", false},
		{PhaseSynthesize, PhaseSynthesize, false},
		// architect
		{PhaseArchitect, PhaseGather, true},
		{PhaseArchitect, "emit", true},
		{PhaseArchitect, PhasePlan, false},
		{PhaseArchitect, PhaseSynthesize, false},
		{PhaseArchitect, PhaseArchitect, false},
	}
	for _, c := range cases {
		got := AllowedEdge(c.from, c.to)
		if got != c.want {
			t.Errorf("AllowedEdge(%q, %q) = %v, want %v", c.from, c.to, got, c.want)
		}
	}
}

func TestPhaseCapsMatchADR(t *testing.T) {
	cases := []struct {
		phase string
		want  int
	}{
		{PhasePlan, 1},
		{PhaseGather, 3},
		{PhaseSynthesize, 2},
		{PhaseArchitect, 2},
	}
	for _, c := range cases {
		if got := PhaseCap(c.phase); got != c.want {
			t.Errorf("PhaseCap(%q) = %d, want %d", c.phase, got, c.want)
		}
	}
	if got := PhaseCap("unknown"); got != 0 {
		t.Errorf("PhaseCap(\"unknown\") = %d, want 0 (defensive zero for unrecognised phase)", got)
	}
}

func TestPhaseValidator_NonResearcherIgnored(t *testing.T) {
	pub := &recordingPublisher{}
	entities := &fakeEntityReader{entities: map[string]map[string]any{}}
	v := buildValidator(t, pub, entities)

	cases := []string{
		"builder",
		"reviewer-spec",
		"reviewer-qa",
		"researcher",  // plain "researcher" — no phase suffix
		"researcher-", // empty suffix
		"coordinator",
		"",
	}
	for _, role := range cases {
		err := v.HandleLoopCompleted(context.Background(), stampEvent(role))
		if err != nil {
			t.Errorf("role %q: HandleLoopCompleted returned error %v, want nil", role, err)
		}
	}
	if len(pub.triples) != 0 {
		t.Errorf("non-researcher events triggered triple writes; got %d", len(pub.triples))
	}
}

func TestPhaseValidator_NilEvent(t *testing.T) {
	pub := &recordingPublisher{}
	v := buildValidator(t, pub, &fakeEntityReader{})
	if err := v.HandleLoopCompleted(context.Background(), nil); err != nil {
		t.Errorf("nil event: HandleLoopCompleted returned %v, want nil", err)
	}
	if len(pub.triples) != 0 {
		t.Errorf("nil event triggered writes")
	}
}

func TestPhaseValidator_NonSuccessIgnored(t *testing.T) {
	pub := &recordingPublisher{}
	v := buildValidator(t, pub, &fakeEntityReader{})
	ev := stampEvent("researcher-plan")
	ev.Outcome = agentic.OutcomeFailed
	if err := v.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Errorf("failure outcome: HandleLoopCompleted returned %v, want nil", err)
	}
	if len(pub.triples) != 0 {
		t.Errorf("failure outcome triggered writes")
	}
}

func TestPhaseValidator_EmptyLoopIDError(t *testing.T) {
	pub := &recordingPublisher{}
	v := buildValidator(t, pub, &fakeEntityReader{})
	ev := stampEvent("researcher-plan")
	ev.LoopID = ""
	err := v.HandleLoopCompleted(context.Background(), ev)
	if err == nil {
		t.Fatal("expected error on empty LoopID")
	}
}

func TestPhaseValidator_MissingActionTripleSkips(t *testing.T) {
	pub := &recordingPublisher{}
	entities := &fakeEntityReader{
		entities: map[string]map[string]any{
			loopEntityID(testLoopID): {}, // no coordinator.next_action
		},
	}
	v := buildValidator(t, pub, entities)
	if err := v.HandleLoopCompleted(context.Background(), stampEvent("researcher-plan")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pub.triples) != 0 {
		t.Errorf("missing action triple should be silent skip; got %d writes", len(pub.triples))
	}
}

func TestPhaseValidator_ApprovedForwardEdge(t *testing.T) {
	pub := &recordingPublisher{}
	entities := &fakeEntityReader{
		entities: map[string]map[string]any{
			loopEntityID(testLoopID): {
				agvocab.CoordinatorNextAction: "gather",
			},
			testChainEntityID: {}, // first gather → count 0 → newCount 1
		},
	}
	v := buildValidator(t, pub, entities)
	if err := v.HandleLoopCompleted(context.Background(), stampEvent("researcher-plan")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := map[string]string{
		chain.PredicatePhaseTransitionTarget:        "gather",
		chain.PredicatePhaseCountGather:             "1",
		chain.PredicatePhaseTransitionProceedGather: "true",
	}
	for pred, wantObj := range want {
		got, ok := pub.byPredicate(pred)
		if !ok {
			t.Errorf("missing predicate %s", pred)
			continue
		}
		if s, _ := got.Object.(string); s != wantObj {
			t.Errorf("predicate %s object = %v, want %q", pred, got.Object, wantObj)
		}
	}
	if pub.hasPredicate(chain.PredicatePhaseTransitionRejected) {
		t.Error("approved transition should not stamp rejected marker")
	}

	// Subject sanity: proceed sentinel must land on loop entity, not
	// chain entity.
	proceed, _ := pub.byPredicate(chain.PredicatePhaseTransitionProceedGather)
	if proceed.Subject != loopEntityID(testLoopID) {
		t.Errorf("proceed sentinel subject = %q, want loop entity %q", proceed.Subject, loopEntityID(testLoopID))
	}
	counter, _ := pub.byPredicate(chain.PredicatePhaseCountGather)
	if counter.Subject != testChainEntityID {
		t.Errorf("phase count subject = %q, want chain entity %q", counter.Subject, testChainEntityID)
	}
}

func TestPhaseValidator_BackEdgeAllowed(t *testing.T) {
	// synthesize → gather is a legitimate back-edge (re-gather after
	// synthesis surfaces a corpus gap). Same gather counter is the cap;
	// fires identically.
	pub := &recordingPublisher{}
	entities := &fakeEntityReader{
		entities: map[string]map[string]any{
			loopEntityID(testLoopID): {
				agvocab.CoordinatorNextAction: "gather",
			},
			testChainEntityID: {
				chain.PredicatePhaseCountGather: "1", // initial gather already counted
			},
		},
	}
	v := buildValidator(t, pub, entities)
	if err := v.HandleLoopCompleted(context.Background(), stampEvent("researcher-synthesize")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	count, ok := pub.byPredicate(chain.PredicatePhaseCountGather)
	if !ok {
		t.Fatal("expected gather counter write")
	}
	if s, _ := count.Object.(string); s != "2" {
		t.Errorf("gather counter = %q, want \"2\"", s)
	}
	if !pub.hasPredicate(chain.PredicatePhaseTransitionProceedGather) {
		t.Error("expected proceed.gather sentinel")
	}
}

func TestPhaseValidator_RejectsInvalidEdge(t *testing.T) {
	// synthesize → plan is not in the allowed set.
	pub := &recordingPublisher{}
	entities := &fakeEntityReader{
		entities: map[string]map[string]any{
			loopEntityID(testLoopID): {
				agvocab.CoordinatorNextAction: "plan",
			},
		},
	}
	v := buildValidator(t, pub, entities)
	if err := v.HandleLoopCompleted(context.Background(), stampEvent("researcher-synthesize")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rejected, ok := pub.byPredicate(chain.PredicatePhaseTransitionRejected)
	if !ok {
		t.Fatal("expected chain.phase_transition.rejected stamp")
	}
	if s, _ := rejected.Object.(string); s != "true" {
		t.Errorf("rejected object = %v, want \"true\"", rejected.Object)
	}
	reason, _ := pub.byPredicate(chain.PredicatePhaseTransitionRejectReason)
	if s, _ := reason.Object.(string); s != rejectReasonInvalidEdge {
		t.Errorf("reject reason = %v, want %q", reason.Object, rejectReasonInvalidEdge)
	}
	target, _ := pub.byPredicate(chain.PredicatePhaseTransitionTarget)
	if s, _ := target.Object.(string); s != "plan" {
		t.Errorf("target audit = %v, want \"plan\"", target.Object)
	}
	// No proceed sentinel, no counter increment.
	if pub.hasPredicate(chain.PredicatePhaseTransitionProceedGather) {
		t.Error("invalid edge should not stamp any proceed sentinel")
	}
	if pub.hasPredicate(chain.PredicatePhaseCountPlan) {
		t.Error("invalid edge should not stamp counter")
	}
}

func TestPhaseValidator_RejectsCapHit(t *testing.T) {
	// architect → gather, with chain.researcher.phase_count.gather
	// already at 3 (the cap). New count would be 4; reject.
	pub := &recordingPublisher{}
	entities := &fakeEntityReader{
		entities: map[string]map[string]any{
			loopEntityID(testLoopID): {
				agvocab.CoordinatorNextAction: "gather",
			},
			testChainEntityID: {
				chain.PredicatePhaseCountGather: "3",
			},
		},
	}
	v := buildValidator(t, pub, entities)
	if err := v.HandleLoopCompleted(context.Background(), stampEvent("researcher-architect")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reason, ok := pub.byPredicate(chain.PredicatePhaseTransitionRejectReason)
	if !ok {
		t.Fatal("expected reject reason")
	}
	if s, _ := reason.Object.(string); s != rejectReasonPhaseCap {
		t.Errorf("reject reason = %v, want %q", reason.Object, rejectReasonPhaseCap)
	}
	if pub.hasPredicate(chain.PredicatePhaseTransitionProceedGather) {
		t.Error("cap hit should not stamp proceed sentinel")
	}
}

func TestPhaseValidator_CapBoundary(t *testing.T) {
	// Cap boundary: prior count = cap-1 → newCount = cap → ALLOWED.
	// Gather cap is 3; prior 2 → new 3 → allowed.
	pub := &recordingPublisher{}
	entities := &fakeEntityReader{
		entities: map[string]map[string]any{
			loopEntityID(testLoopID): {
				agvocab.CoordinatorNextAction: "gather",
			},
			testChainEntityID: {
				chain.PredicatePhaseCountGather: "2",
			},
		},
	}
	v := buildValidator(t, pub, entities)
	if err := v.HandleLoopCompleted(context.Background(), stampEvent("researcher-synthesize")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !pub.hasPredicate(chain.PredicatePhaseTransitionProceedGather) {
		t.Error("at-boundary fire should be allowed; expected proceed sentinel")
	}
	count, _ := pub.byPredicate(chain.PredicatePhaseCountGather)
	if s, _ := count.Object.(string); s != "3" {
		t.Errorf("gather counter at boundary = %q, want \"3\"", s)
	}
}

func TestPhaseValidator_EmitApprovedWithoutProceed(t *testing.T) {
	// architect → emit terminates the researcher arc. Target stamped
	// for audit; no proceed sentinel; no counter (emit not in caps).
	pub := &recordingPublisher{}
	entities := &fakeEntityReader{
		entities: map[string]map[string]any{
			loopEntityID(testLoopID): {
				agvocab.CoordinatorNextAction: "emit",
			},
		},
	}
	v := buildValidator(t, pub, entities)
	if err := v.HandleLoopCompleted(context.Background(), stampEvent("researcher-architect")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	target, ok := pub.byPredicate(chain.PredicatePhaseTransitionTarget)
	if !ok {
		t.Fatal("expected target audit")
	}
	if s, _ := target.Object.(string); s != "emit" {
		t.Errorf("target = %v, want \"emit\"", target.Object)
	}
	if pub.hasPredicate(chain.PredicatePhaseTransitionRejected) {
		t.Error("emit should not be rejected")
	}
	// Every proceed predicate variant must be absent.
	for _, p := range []string{
		chain.PredicatePhaseTransitionProceedGather,
		chain.PredicatePhaseTransitionProceedSynthesize,
		chain.PredicatePhaseTransitionProceedArchitect,
	} {
		if pub.hasPredicate(p) {
			t.Errorf("emit should not stamp %s", p)
		}
	}
}

func TestPhaseValidator_PrematureEmitFromPlanAllowed(t *testing.T) {
	// plan → emit is structurally legal per ADR-041 §"Premature emit
	// from plan is allowed by design" — reviewer will reject and the
	// chain.recovery cap handles the loop.
	pub := &recordingPublisher{}
	entities := &fakeEntityReader{
		entities: map[string]map[string]any{
			loopEntityID(testLoopID): {
				agvocab.CoordinatorNextAction: "emit",
			},
		},
	}
	v := buildValidator(t, pub, entities)
	if err := v.HandleLoopCompleted(context.Background(), stampEvent("researcher-plan")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pub.hasPredicate(chain.PredicatePhaseTransitionRejected) {
		t.Error("plan → emit should be structurally approved (reviewer's job to reject content)")
	}
}

func TestPhaseValidator_EntityReadErrorFailsClosed(t *testing.T) {
	pub := &recordingPublisher{}
	entities := &fakeEntityReader{err: errors.New("graph blip")}
	v := buildValidator(t, pub, entities)
	if err := v.HandleLoopCompleted(context.Background(), stampEvent("researcher-plan")); err != nil {
		t.Fatalf("HandleLoopCompleted: %v", err)
	}
	if len(pub.triples) != 0 {
		t.Errorf("entity-read failure should write no triples (fail-closed); got %d", len(pub.triples))
	}
}

// selectiveErrorReader returns base entities normally but errors when
// reading the configured entity_id. Lets a test exercise the chain-
// entity-read branch in isolation (loop-entity read succeeds, resolver
// succeeds, chain-entity read fails) — the fail-closed semantics must
// hold there too, not just on the loop-entity read.
type selectiveErrorReader struct {
	base    *fakeEntityReader
	errorOn string
}

func (s *selectiveErrorReader) ReadEntity(ctx context.Context, entityID string) (map[string]any, error) {
	if entityID == s.errorOn {
		return nil, errors.New("simulated chain-entity read failure")
	}
	return s.base.ReadEntity(ctx, entityID)
}

func TestPhaseValidator_ChainEntityReadErrorFailsClosed(t *testing.T) {
	// Loop-entity read succeeds (action stamped); resolver succeeds
	// (chain entity resolved); chain-entity read fails. Validator must
	// fail-closed with no writes.
	base := &fakeEntityReader{
		entities: map[string]map[string]any{
			loopEntityID(testLoopID): {
				agvocab.CoordinatorNextAction: "gather",
			},
		},
	}
	pub := &recordingPublisher{}
	v := buildValidator(t, pub, &selectiveErrorReader{base: base, errorOn: testChainEntityID})
	if err := v.HandleLoopCompleted(context.Background(), stampEvent("researcher-plan")); err != nil {
		t.Fatalf("HandleLoopCompleted: %v", err)
	}
	if len(pub.triples) != 0 {
		t.Errorf("chain-entity read failure should write no triples; got %d", len(pub.triples))
	}
}
