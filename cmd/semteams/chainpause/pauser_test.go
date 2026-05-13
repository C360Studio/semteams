package chainpause

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/types"
)

// recordingPublisher records AddTriple calls for assertion.
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

func testPlatform() types.PlatformMeta {
	return types.PlatformMeta{Org: "c360", Platform: "test"}
}

// fakeChainResolver is a deterministic ChainEntityResolver for tests.
// It returns a chain entity ID derived from a configurable chain root,
// or an error when err is set. Both the Pauser tests and the
// DecisionHandler tests share this fake.
type fakeChainResolver struct {
	chainRoot string // loop_id used as chain_id in the returned 6-part ID
	err       error
	calls     int
	lastArg   string
}

func (f *fakeChainResolver) ChainEntityID(_ context.Context, loopID string) (string, error) {
	f.calls++
	f.lastArg = loopID
	if f.err != nil {
		return "", f.err
	}
	root := f.chainRoot
	if root == "" {
		// Default: treat the failed loop as its own chain root so tests
		// that don't care about ancestry get a deterministic subject.
		root = loopID
	}
	return agentic.ChainExecutionEntityID(testPlatform().Org, testPlatform().Platform, root), nil
}

// testChainEntityID returns the canonical 6-part chain entity ID a test
// fixture expects under testPlatform(). Saves the boilerplate at every
// assertion site.
func testChainEntityID(chainRoot string) string {
	return agentic.ChainExecutionEntityID(testPlatform().Org, testPlatform().Platform, chainRoot)
}

func TestPauser_HandleFailed_ManagedRoleWritesTriplesD5(t *testing.T) {
	pub := &recordingPublisher{}
	p := NewPauser(pub, &fakeChainResolver{})

	ev := &agentic.LoopFailedEvent{
		LoopID:  "abc123",
		TaskID:  "task-1",
		Outcome: agentic.OutcomeFailed,
		Role:    "reviewer-spec",
		Error:   "Anthropic API: overloaded",
	}

	result, err := p.HandleFailed(context.Background(), ev)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.FailedLoopID != "abc123" {
		t.Errorf("FailedLoopID: got %q, want %q", result.FailedLoopID, "abc123")
	}
	if result.Cause != "api_overloaded" {
		t.Errorf("Cause: got %q, want api_overloaded", result.Cause)
	}
	if result.Classification != "anthropic_overloaded_529" {
		t.Errorf("Classification: got %q, want anthropic_overloaded_529", result.Classification)
	}

	// Verify each §D5 predicate is written.
	predicates := []string{
		"chain.paused.cause",
		"chain.paused.classification",
		"chain.paused.role",
		"chain.paused.original_model",
		"chain.paused.error_shape",
		"chain.paused.prior_attempts",
		"chain.paused.failed_loop_id",
		"chain.paused.spawn_loop_id",
		"chain.paused.observed_at",
	}
	for _, pred := range predicates {
		if _, ok := pub.byPredicate(pred); !ok {
			t.Errorf("expected triple with predicate %q to be written", pred)
		}
	}

	// Source must be chainpause (identifies the writer for audit trail).
	if t2, ok := pub.byPredicate("chain.paused.cause"); ok {
		if t2.Source != "chainpause" {
			t.Errorf("Source: got %q, want chainpause", t2.Source)
		}
	}
}

func TestPauser_HandleFailed_UnmanagedRoleSkipped(t *testing.T) {
	pub := &recordingPublisher{}
	p := NewPauser(pub, &fakeChainResolver{})

	ev := &agentic.LoopFailedEvent{
		LoopID:  "xyz",
		TaskID:  "task-2",
		Outcome: agentic.OutcomeFailed,
		Role:    "general", // not a managed arc role
		Error:   "some error",
	}

	result, err := p.HandleFailed(context.Background(), ev)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.FailedLoopID != "" {
		t.Errorf("expected empty result for unmanaged role, got %+v", result)
	}
	if len(pub.triples) != 0 {
		t.Errorf("expected no triples for unmanaged role, got %d", len(pub.triples))
	}
}

var classifyTests = []struct {
	name         string
	errStr       string
	wantCause    string
	wantClassify string
}{
	{
		name:         "anthropic overloaded",
		errStr:       "Anthropic API returned 529 Overloaded",
		wantCause:    "api_overloaded",
		wantClassify: "anthropic_overloaded_529",
	},
	{
		name:         "timeout",
		errStr:       "context deadline exceeded",
		wantCause:    "api_timeout",
		wantClassify: "llm_call_timeout",
	},
	{
		name:         "timeout variant",
		errStr:       "request timeout after 180s",
		wantCause:    "api_timeout",
		wantClassify: "llm_call_timeout",
	},
	{
		name:         "persona load failure",
		errStr:       "persona load failed: bucket not found",
		wantCause:    "config_load_failure",
		wantClassify: "persona_load_error",
	},
	{
		name:         "config load failure",
		errStr:       "invalid config: missing required field",
		wantCause:    "config_load_failure",
		wantClassify: "config_load_error",
	},
	{
		name:         "executor panic",
		errStr:       "executor panic: nil pointer dereference",
		wantCause:    "tool_executor_panic",
		wantClassify: "tool_executor_panic",
	},
	{
		name:         "max iterations",
		errStr:       "maximum iterations reached",
		wantCause:    "max_iterations",
		wantClassify: "max_iterations_exhausted",
	},
	{
		name:         "unknown error",
		errStr:       "something unexpected happened",
		wantCause:    "unknown",
		wantClassify: "unknown",
	},
	{
		name:         "empty error",
		errStr:       "",
		wantCause:    "unknown",
		wantClassify: "unknown",
	},
}

func TestClassifyError(t *testing.T) {
	for _, tc := range classifyTests {
		t.Run(tc.name, func(t *testing.T) {
			cause, classification := classifyError(tc.errStr)
			if cause != tc.wantCause {
				t.Errorf("cause: got %q, want %q", cause, tc.wantCause)
			}
			if classification != tc.wantClassify {
				t.Errorf("classification: got %q, want %q", classification, tc.wantClassify)
			}
		})
	}
}

func TestSanitiseErrorShape_LengthBound(t *testing.T) {
	long := make([]byte, 512)
	for i := range long {
		long[i] = 'a'
	}
	result := sanitiseErrorShape(string(long))
	// 256 chars + ellipsis
	if len(result) > 260 {
		t.Errorf("sanitised error shape too long: %d bytes", len(result))
	}
}

func TestSanitiseErrorShape_ControlCharsStripped(t *testing.T) {
	input := "error\x00with\x01control\x7fchars"
	result := sanitiseErrorShape(input)
	// No NUL or SOH bytes — sanitiseErrorShape must strip classic control chars.
	for i := 0; i < len(result); i++ {
		if result[i] == 0x00 || result[i] == 0x01 {
			t.Errorf("control char found at offset %d in sanitised string: %q", i, result)
		}
	}
}

func TestPauser_HandleFailed_PartialWriteReturnsFirstErr(t *testing.T) {
	pub := &recordingPublisher{err: errors.New("NATS publish failed")}
	p := NewPauser(pub, &fakeChainResolver{})

	ev := &agentic.LoopFailedEvent{
		LoopID:  "loop-err",
		TaskID:  "task-3",
		Outcome: agentic.OutcomeFailed,
		Role:    "researcher-plan",
		Error:   "some error",
	}

	result, err := p.HandleFailed(context.Background(), ev)
	if err == nil {
		t.Fatal("expected error from publisher, got nil")
	}
	// Result should still be populated — we attempted the write.
	if result.FailedLoopID != "loop-err" {
		t.Errorf("FailedLoopID: got %q, want loop-err", result.FailedLoopID)
	}
}

func TestPauser_HandleFailed_ObservedAtIsRFC3339(t *testing.T) {
	pub := &recordingPublisher{}
	p := NewPauser(pub, &fakeChainResolver{})

	ev := &agentic.LoopFailedEvent{
		LoopID:  "loop-ts",
		TaskID:  "task-4",
		Outcome: agentic.OutcomeFailed,
		Role:    "researcher-plan",
		Error:   "timeout",
	}

	_, err := p.HandleFailed(context.Background(), ev)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t2, ok := pub.byPredicate("chain.paused.observed_at")
	if !ok {
		t.Fatal("chain.paused.observed_at triple missing")
	}
	ts, ok := t2.Object.(string)
	if !ok {
		t.Fatalf("observed_at Object not string: %T", t2.Object)
	}
	if _, parseErr := time.Parse(time.RFC3339, ts); parseErr != nil {
		t.Errorf("observed_at is not valid RFC3339: %q (%v)", ts, parseErr)
	}
}

// TestPauser_HandleFailed_CapturesOriginalModel verifies that chain.paused.original_model
// is written from LoopFailedEvent.Model so the retry path can re-spawn with the
// correct model without a live graph query (ADR-037 §D7).
func TestPauser_HandleFailed_CapturesOriginalModel(t *testing.T) {
	pub := &recordingPublisher{}
	p := NewPauser(pub, &fakeChainResolver{})

	ev := &agentic.LoopFailedEvent{
		LoopID:  "loop-model",
		TaskID:  "task-model-1",
		Outcome: agentic.OutcomeFailed,
		Role:    "researcher-plan",
		Model:   "claude-opus-4-5",
		Error:   "overloaded",
	}

	if _, err := p.HandleFailed(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t2, ok := pub.byPredicate("chain.paused.original_model")
	if !ok {
		t.Fatal("chain.paused.original_model triple missing")
	}
	obj, ok := t2.Object.(string)
	if !ok {
		t.Fatalf("chain.paused.original_model Object not string: %T", t2.Object)
	}
	if obj != "claude-opus-4-5" {
		t.Errorf("chain.paused.original_model = %q, want claude-opus-4-5", obj)
	}
}

// TestPauser_HandleFailed_SubjectIsChainEntity pins the ADR-038 PR B
// Phase 3 re-point: every §D5 triple targets the canonical chain
// entity (resolved from the failed loop's ancestry), not the failed
// loop's own entity. A regression here would silently fragment
// chain-paused state across loop entities and break the operator
// retry flow when DecisionHandler reads from the chain entity.
func TestPauser_HandleFailed_SubjectIsChainEntity(t *testing.T) {
	pub := &recordingPublisher{}
	resolver := &fakeChainResolver{chainRoot: "dispatch_root"}
	p := NewPauser(pub, resolver)

	ev := &agentic.LoopFailedEvent{
		LoopID:       "researcher_gather_8",
		TaskID:       "task-gather-8",
		Outcome:      agentic.OutcomeFailed,
		Role:         "researcher-gather",
		ParentLoopID: "researcher_plan_7",
		Error:        "max iterations reached",
	}

	if _, err := p.HandleFailed(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resolver.calls != 1 {
		t.Errorf("ChainEntityID calls = %d, want 1", resolver.calls)
	}
	if resolver.lastArg != ev.LoopID {
		t.Errorf("resolver called with %q, want %q (the failed loop_id)", resolver.lastArg, ev.LoopID)
	}

	wantSubject := testChainEntityID("dispatch_root")
	for _, tr := range pub.triples {
		if tr.Subject != wantSubject {
			t.Errorf("§D5 triple %q wrote to subject %q, want %q (chain entity)", tr.Predicate, tr.Subject, wantSubject)
		}
	}
}

// TestPauser_HandleFailed_ResolverErrorSurfaces verifies that a graph
// blip mid-pause returns a wrapped error to the caller so the
// subscriber logs the failure cleanly. Without a chain entity ID
// the §D5 cluster has no Subject, so writing partial triples to the
// failed loop entity would silently fragment audit data — surfacing
// the error and skipping is the correct fail-soft policy.
func TestPauser_HandleFailed_ResolverErrorSurfaces(t *testing.T) {
	pub := &recordingPublisher{}
	resolver := &fakeChainResolver{err: errors.New("graph KV unavailable")}
	p := NewPauser(pub, resolver)

	ev := &agentic.LoopFailedEvent{
		LoopID:  "loop-resolver-err",
		TaskID:  "task-x",
		Outcome: agentic.OutcomeFailed,
		Role:    "builder",
		Error:   "executor panic",
	}

	_, err := p.HandleFailed(context.Background(), ev)
	if err == nil {
		t.Fatal("expected resolver error to surface; got nil")
	}
	if len(pub.triples) != 0 {
		t.Errorf("no §D5 triples should be written on resolver failure; got %d", len(pub.triples))
	}
}

func TestIsManagedRole(t *testing.T) {
	managed := []string{
		"researcher-plan",
		"researcher-gather",
		"researcher-synthesize",
		"researcher-architect",
		"reviewer-research",
		"reviewer-spec",
		"reviewer-qa",
		"builder",
		"dispatch",
		// Legacy roles retained for research-iterative configs.
		"researcher",
		"research-reviewer",
	}
	for _, role := range managed {
		if !isManagedRole(role) {
			t.Errorf("expected %q to be managed, but isManagedRole returned false", role)
		}
	}

	unmanaged := []string{"general", "ops-analyst", "coordinator", "dev-via-spec-builder", "dev-via-spec-planner", ""}
	for _, role := range unmanaged {
		if isManagedRole(role) {
			t.Errorf("expected %q to be unmanaged, but isManagedRole returned true", role)
		}
	}
}

// Ensure each managed role in isManagedRole matches the rule's "in" condition list.
// A mismatch between the two would cause the rule to fire but the subscriber to skip.
func TestManagedRoleListMirrorRule(t *testing.T) {
	ruleRoles := []string{
		"researcher-plan",
		"researcher-gather",
		"researcher-synthesize",
		"researcher-architect",
		"reviewer-research",
		"reviewer-spec",
		"reviewer-qa",
		"builder",
		"dispatch",
		"researcher",
		"research-reviewer",
	}
	for _, role := range ruleRoles {
		if !isManagedRole(role) {
			t.Errorf("rule role %q not in isManagedRole — rule fires but subscriber skips; update isManagedRole", role)
		}
	}
}
