package evidence_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/types"
	"github.com/c360studio/semteams/cmd/semteams/evidence"
	"github.com/c360studio/semteams/cmd/semteams/verification"
)

// fakePublisher records AddTriple calls for inspection.
type fakePublisher struct {
	mu      sync.Mutex
	triples []message.Triple
	errFn   func(triple message.Triple) error // nil → always succeed
}

func (f *fakePublisher) AddTriple(_ context.Context, t message.Triple) error {
	if f.errFn != nil {
		if err := f.errFn(t); err != nil {
			return err
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.triples = append(f.triples, t)
	return nil
}

func (f *fakePublisher) recorded() []message.Triple {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]message.Triple, len(f.triples))
	copy(out, f.triples)
	return out
}

func (f *fakePublisher) findByPredicate(predicate string) (message.Triple, bool) {
	for _, t := range f.recorded() {
		if t.Predicate == predicate {
			return t, true
		}
	}
	return message.Triple{}, false
}

// buildEvent returns a minimal LoopCompletedEvent for a dev-via-spec-builder
// loop with the given loopID. Callers may override fields before passing.
func buildEvent(loopID string) *agentic.LoopCompletedEvent {
	return &agentic.LoopCompletedEvent{
		LoopID:  loopID,
		TaskID:  "task-" + loopID,
		Role:    "dev-via-spec-builder",
		Outcome: agentic.OutcomeSuccess,
		Model:   "mock-llm",
	}
}

// writeChecksFile writes a []verification.Check slice to
// <wsRoot>/<loopID>/.evidence/checks.json, creating directories as
// needed.
func writeChecksFile(t *testing.T, wsRoot, loopID string, checks []verification.Check) {
	t.Helper()
	dir := filepath.Join(wsRoot, loopID, ".evidence")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create .evidence dir: %v", err)
	}
	data, err := json.Marshal(checks)
	if err != nil {
		t.Fatalf("marshal checks: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "checks.json"), data, 0o644); err != nil {
		t.Fatalf("write checks.json: %v", err)
	}
}

// newPreprocessor is a test helper that builds a Preprocessor with the
// given workspace root and a freshly-built Registry with builtins
// registered.
func newPreprocessor(t *testing.T, wsRoot string, pub *fakePublisher) *evidence.Preprocessor {
	t.Helper()
	reg, err := evidence.NewWithBuiltins()
	if err != nil {
		t.Fatalf("NewWithBuiltins: %v", err)
	}
	platform := types.PlatformMeta{Org: "c360", Platform: "test"}
	return evidence.New(reg, pub, wsRoot, platform, nil)
}

// TestPreprocessor_HappyPath_TwoChecks verifies that a builder loop
// with a checks.json containing one passing and one failing check
// produces evidence.summary + evidence.summary_ready stamps.
func TestPreprocessor_HappyPath_TwoChecks(t *testing.T) {
	wsRoot := t.TempDir()
	loopID := "loop-happy"
	pub := &fakePublisher{}
	p := newPreprocessor(t, wsRoot, pub)

	// Plant a real file at the workspace path so FileExistsChecker can pass.
	testFilePath := filepath.Join(wsRoot, loopID, "src", "Foo.java")
	if err := os.MkdirAll(filepath.Dir(testFilePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(testFilePath, []byte("// stub"), 0o644); err != nil {
		t.Fatal(err)
	}

	checks := []verification.Check{
		{
			Target:  "file exists check",
			Runtime: verification.RuntimeInProcessUnit,
			Ref:     verification.Ref{Type: verification.RefFilepath, Path: "src/Foo.java"},
			Evidence: []verification.EvidenceRule{
				{Kind: "test_file_exists", Args: map[string]any{"path": "src/Foo.java"}},
			},
		},
		{
			Target:  "file missing check",
			Runtime: verification.RuntimeInProcessUnit,
			Ref:     verification.Ref{Type: verification.RefFilepath, Path: "src/Missing.java"},
			Evidence: []verification.EvidenceRule{
				{Kind: "test_file_exists", Args: map[string]any{"path": "src/Missing.java"}},
			},
		},
	}
	writeChecksFile(t, wsRoot, loopID, checks)

	ev := buildEvent(loopID)
	if err := p.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("HandleLoopCompleted: %v", err)
	}

	// Verify summary triple was stamped.
	sumTriple, ok := pub.findByPredicate("evidence.summary")
	if !ok {
		t.Fatal("expected evidence.summary triple to be stamped")
	}
	sumStr, _ := sumTriple.Object.(string)
	if sumStr == "" {
		t.Error("evidence.summary Object is empty")
	}
	// Should contain both check headings.
	for _, want := range []string{"## Check 1", "## Check 2", "pass", "fail"} {
		if len(sumStr) == 0 || !strings.Contains(sumStr, want) {
			t.Errorf("evidence.summary missing %q; got:\n%s", want, sumStr)
		}
	}

	// Verify ready triple.
	readyTriple, ok := pub.findByPredicate("evidence.summary_ready")
	if !ok {
		t.Fatal("expected evidence.summary_ready triple to be stamped")
	}
	if readyTriple.Object != "true" {
		t.Errorf("evidence.summary_ready Object = %v, want %q", readyTriple.Object, "true")
	}
}

// TestPreprocessor_EmptyChecksSlice verifies that an empty checks slice
// stamps "(no checks)\n" as the summary — chain plumbing is healthy but
// architect emitted no checks. evidence.summary_ready must still be stamped.
func TestPreprocessor_EmptyChecksSlice(t *testing.T) {
	wsRoot := t.TempDir()
	loopID := "loop-empty-checks"
	pub := &fakePublisher{}
	p := newPreprocessor(t, wsRoot, pub)

	writeChecksFile(t, wsRoot, loopID, []verification.Check{})

	ev := buildEvent(loopID)
	if err := p.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("HandleLoopCompleted: %v", err)
	}

	sumTriple, ok := pub.findByPredicate("evidence.summary")
	if !ok {
		t.Fatal("expected evidence.summary triple")
	}
	sumStr, _ := sumTriple.Object.(string)
	// Summarize(nil) returns "(no checks)\n".
	if !strings.Contains(sumStr, "(no checks)") {
		t.Errorf("empty checks: expected '(no checks)' in summary, got %q", sumStr)
	}

	_, ok = pub.findByPredicate("evidence.summary_ready")
	if !ok {
		t.Fatal("expected evidence.summary_ready triple even on empty checks")
	}
}

// TestPreprocessor_MissingChecksFile verifies fail-soft: when
// checks.json does not exist, a plumbing-failure summary is stamped AND
// evidence.summary_ready is still written so rule_07 can fire.
func TestPreprocessor_MissingChecksFile(t *testing.T) {
	wsRoot := t.TempDir()
	loopID := "loop-no-file"
	pub := &fakePublisher{}
	p := newPreprocessor(t, wsRoot, pub)

	// Deliberately do NOT write checks.json.
	ev := buildEvent(loopID)
	if err := p.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("HandleLoopCompleted must not return error: %v", err)
	}

	sumTriple, ok := pub.findByPredicate("evidence.summary")
	if !ok {
		t.Fatal("expected evidence.summary triple even on missing file")
	}
	sumStr, _ := sumTriple.Object.(string)
	if !strings.Contains(sumStr, "no checks file") || !strings.Contains(sumStr, "chain plumbing failure") {
		t.Errorf("missing file: expected plumbing-failure text in summary, got %q", sumStr)
	}

	_, ok = pub.findByPredicate("evidence.summary_ready")
	if !ok {
		t.Fatal("expected evidence.summary_ready triple even on missing file")
	}
}

// TestPreprocessor_MalformedChecksJSON verifies fail-soft: a
// syntactically broken checks.json stamps a parse-error summary AND
// evidence.summary_ready so rule_07 still unblocks.
func TestPreprocessor_MalformedChecksJSON(t *testing.T) {
	wsRoot := t.TempDir()
	loopID := "loop-malformed"
	pub := &fakePublisher{}
	p := newPreprocessor(t, wsRoot, pub)

	dir := filepath.Join(wsRoot, loopID, ".evidence")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "checks.json"), []byte("{bad json{{"), 0o644); err != nil {
		t.Fatal(err)
	}

	ev := buildEvent(loopID)
	if err := p.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("HandleLoopCompleted must not return error: %v", err)
	}

	sumTriple, ok := pub.findByPredicate("evidence.summary")
	if !ok {
		t.Fatal("expected evidence.summary triple on parse error")
	}
	sumStr, _ := sumTriple.Object.(string)
	if !strings.Contains(sumStr, "chain plumbing failure") {
		t.Errorf("malformed JSON: expected plumbing-failure text, got %q", sumStr)
	}

	_, ok = pub.findByPredicate("evidence.summary_ready")
	if !ok {
		t.Fatal("expected evidence.summary_ready triple on parse error")
	}
}

// TestPreprocessor_NonBuilderRole verifies that events for roles other
// than dev-via-spec-builder are silently skipped with no triple stamps.
func TestPreprocessor_NonBuilderRole(t *testing.T) {
	wsRoot := t.TempDir()
	pub := &fakePublisher{}
	p := newPreprocessor(t, wsRoot, pub)

	for _, role := range []string{
		"dev-via-spec-challenger",
		"dev-via-spec-reviewer",
		"researcher",
		"coordinator",
	} {
		t.Run(role, func(t *testing.T) {
			pub.mu.Lock()
			pub.triples = nil
			pub.mu.Unlock()

			ev := buildEvent("loop-other")
			ev.Role = role
			if err := p.HandleLoopCompleted(context.Background(), ev); err != nil {
				t.Fatalf("HandleLoopCompleted: %v", err)
			}
			if got := pub.recorded(); len(got) != 0 {
				t.Errorf("role %q: expected 0 triples, got %d", role, len(got))
			}
		})
	}
}

// TestPreprocessor_NonSuccessOutcome verifies that a failed (not
// completed) builder loop is skipped unless its role was explicitly
// builder and Outcome was success. This guards against double-stamps on
// loop failures that also hit the loopCompletedSubject.
func TestPreprocessor_NonSuccessOutcome(t *testing.T) {
	wsRoot := t.TempDir()
	pub := &fakePublisher{}
	p := newPreprocessor(t, wsRoot, pub)

	ev := buildEvent("loop-failed")
	ev.Outcome = agentic.OutcomeFailed
	// Even with the right role, outcome != success → skip.
	if err := p.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("HandleLoopCompleted: %v", err)
	}
	if got := pub.recorded(); len(got) != 0 {
		t.Errorf("failed outcome: expected 0 triples, got %d", len(got))
	}
}

// TestPreprocessor_WorkspaceRootEmpty verifies that a Preprocessor
// constructed with workspaceRoot="" skips all events without stamping
// anything.
func TestPreprocessor_WorkspaceRootEmpty(t *testing.T) {
	pub := &fakePublisher{}
	reg, err := evidence.NewWithBuiltins()
	if err != nil {
		t.Fatalf("NewWithBuiltins: %v", err)
	}
	p := evidence.New(reg, pub, "", types.PlatformMeta{Org: "c360", Platform: "test"}, nil)

	ev := buildEvent("loop-disabled")
	if err := p.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("HandleLoopCompleted: %v", err)
	}
	if got := pub.recorded(); len(got) != 0 {
		t.Errorf("disabled preprocessor: expected 0 triples, got %d", len(got))
	}
}

// TestPreprocessor_SymlinkInWorkspace verifies that the existing
// evidence.Context symlink refusal is exercised. The test plants a
// symlink at the expected checks.json path and expects a plumbing-
// failure summary (not a panic or silent skip).
//
// Note: the symlink guard lives in Context.Resolve which the checkers
// use, not in the checks.json read path itself. A symlink AT the
// checks.json path is caught by os.ReadFile following the symlink (it
// reads the target file). The workspace path traversal test for actual
// checker args (e.g. a symlinked test file) is exercised via
// Context.Resolve; this test verifies the preprocessor reaches a
// readable workspace without panicking.
func TestPreprocessor_SymlinkInWorkspace(t *testing.T) {
	wsRoot := t.TempDir()
	loopID := "loop-symlink"
	pub := &fakePublisher{}
	p := newPreprocessor(t, wsRoot, pub)

	// Write a real file and create a symlink for a check argument.
	realFile := filepath.Join(wsRoot, "real.java")
	if err := os.WriteFile(realFile, []byte("// real"), 0o644); err != nil {
		t.Fatal(err)
	}
	loopDir := filepath.Join(wsRoot, loopID)
	if err := os.MkdirAll(loopDir, 0o755); err != nil {
		t.Fatal(err)
	}
	symlinkTarget := filepath.Join(loopDir, "linked.java")
	if err := os.Symlink(realFile, symlinkTarget); err != nil {
		t.Skip("symlink creation not supported:", err)
	}

	// Check that references a symlinked path — Context.Resolve will
	// refuse the symlink and the checker returns StatusError.
	checks := []verification.Check{
		{
			Target:  "symlink path check",
			Runtime: verification.RuntimeInProcessUnit,
			Ref:     verification.Ref{Type: verification.RefFilepath, Path: "linked.java"},
			Evidence: []verification.EvidenceRule{
				{Kind: "test_file_exists", Args: map[string]any{"path": "linked.java"}},
			},
		},
	}
	writeChecksFile(t, wsRoot, loopID, checks)

	ev := buildEvent(loopID)
	if err := p.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("HandleLoopCompleted must not return error: %v", err)
	}

	// Summary is stamped (not empty) and the loop didn't panic.
	sumTriple, ok := pub.findByPredicate("evidence.summary")
	if !ok {
		t.Fatal("expected evidence.summary triple even with symlinked path")
	}
	if sumStr, _ := sumTriple.Object.(string); sumStr == "" {
		t.Error("evidence.summary Object is empty")
	}
	// evidence.summary_ready must always be stamped.
	_, ok = pub.findByPredicate("evidence.summary_ready")
	if !ok {
		t.Fatal("expected evidence.summary_ready triple")
	}
}

// TestPreprocessor_TripleSummaryShape verifies the evidence.summary
// triple's Subject uses the LoopExecutionEntityID pattern and its
// Timestamp is recent.
func TestPreprocessor_TripleSummaryShape(t *testing.T) {
	wsRoot := t.TempDir()
	loopID := "loop-shape"
	pub := &fakePublisher{}

	reg, err := evidence.NewWithBuiltins()
	if err != nil {
		t.Fatalf("NewWithBuiltins: %v", err)
	}
	platform := types.PlatformMeta{Org: "myorg", Platform: "mypf"}
	p := evidence.New(reg, pub, wsRoot, platform, nil)

	writeChecksFile(t, wsRoot, loopID, []verification.Check{})

	before := time.Now()
	ev := buildEvent(loopID)
	if err := p.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("HandleLoopCompleted: %v", err)
	}
	after := time.Now()

	sumTriple, ok := pub.findByPredicate("evidence.summary")
	if !ok {
		t.Fatal("expected evidence.summary triple")
	}

	// Subject must embed org, platform, loopID as parts.
	want := agentic.LoopExecutionEntityID("myorg", "mypf", loopID)
	if sumTriple.Subject != want {
		t.Errorf("evidence.summary Subject = %q, want %q", sumTriple.Subject, want)
	}
	if sumTriple.Timestamp.Before(before) || sumTriple.Timestamp.After(after) {
		t.Errorf("evidence.summary Timestamp %v not in [%v, %v]", sumTriple.Timestamp, before, after)
	}
	if sumTriple.Source != "evidence-preprocessor" {
		t.Errorf("evidence.summary Source = %q, want %q", sumTriple.Source, "evidence-preprocessor")
	}
}

// TestPreprocessor_PublisherError verifies that even when AddTriple
// returns an error, HandleLoopCompleted never propagates that error
// to the caller (fail-soft contract).
func TestPreprocessor_PublisherError(t *testing.T) {
	wsRoot := t.TempDir()
	loopID := "loop-pub-err"
	pub := &fakePublisher{
		errFn: func(_ message.Triple) error {
			return errors.New("NATS down")
		},
	}
	p := newPreprocessor(t, wsRoot, pub)
	writeChecksFile(t, wsRoot, loopID, []verification.Check{})

	ev := buildEvent(loopID)
	if err := p.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Errorf("HandleLoopCompleted must be fail-soft even on publisher error; got: %v", err)
	}
}
