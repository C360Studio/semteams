package emitautoresearchartifact

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/types"
)

type fakePub struct {
	mu      sync.Mutex
	triples []message.Triple
}

func (f *fakePub) AddTriple(_ context.Context, t message.Triple) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.triples = append(f.triples, t)
	return nil
}

func (f *fakePub) AddTriplesBatch(_ context.Context, ts []message.Triple) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.triples = append(f.triples, ts...)
	return nil
}

func (f *fakePub) find(pred string) (any, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, t := range f.triples {
		if t.Predicate == pred {
			return t.Object, true
		}
	}
	return nil, false
}

func platform() types.PlatformMeta {
	return types.PlatformMeta{Org: "c360", Platform: "ops"}
}

func baseArgs() map[string]any {
	return map[string]any{
		"title":                "Autoresearch on task test:integration",
		"command":              "task test:integration",
		"baseline_value":       float64(120.0),
		"best_value":           float64(98.5),
		"cap":                  float64(10),
		"iterations_completed": float64(8),
		"iterations_kept":      float64(2),
		"iterations_reverted":  float64(5),
		"iterations_crashed":   float64(1),
		"best_experiment_id":   "iter-2-loop-id",
		"best_diff_summary":    "Batched setup in test/fixtures/setup.go",
		"journey":              []any{"1: hypothesis A → 115.0 (reverted)", "2: hypothesis B → 98.5 (kept)"},
		"open_opportunities":   []any{"explore concurrent fixture setup"},
		"recommended_action":   "Commit the kept diff; re-run with surface widened to internal/.",
	}
}

// TestExecutor_HappyPath exercises the LOCAL-FS fallback branch
// (SetAttestationWriter NOT called). Use TestExecutor_AttestationPresent
// for the attestation-aware branch.
func TestExecutor_HappyPath(t *testing.T) {
	tmp := t.TempDir()
	pub := &fakePub{}
	e := NewExecutor(pub, platform(), slog.Default(), tmp)

	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "c1", Name: ToolName, LoopID: "synth-1", Arguments: baseArgs(),
	})
	if res.Error != "" {
		t.Fatalf("unexpected error: %s", res.Error)
	}

	wantSubject := "c360.ops.agent.agentic-loop.execution.synth-1"
	for _, tr := range pub.triples {
		if tr.Subject != wantSubject {
			t.Errorf("subject = %q, want %q", tr.Subject, wantSubject)
		}
	}

	// path triple set + file actually rendered.
	pathAny, ok := pub.find("autoresearch.artifact.path")
	if !ok {
		t.Fatalf("artifact path triple missing")
	}
	path, _ := pathAny.(string)
	if path == "" {
		t.Errorf("artifact path is empty")
	}
	// Local-fs branch returns filepath.Join(outputDir, filename). With
	// an absolute tmp passed in, the stamped path is absolute under
	// tmp. (Attestation branch stamps a workspace-relative path; see
	// TestExecutor_AttestationPresent.)
	if !strings.HasPrefix(path, tmp) {
		t.Errorf("path %q should be under tmp %q", path, tmp)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read rendered artifact: %v", err)
	}
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "## Summary") {
		t.Errorf("rendered artifact missing Summary heading")
	}
	if !strings.Contains(bodyStr, "## Journey") {
		t.Errorf("rendered artifact missing Journey heading")
	}
	if !strings.Contains(bodyStr, "hypothesis B → 98.5 (kept)") {
		t.Errorf("rendered artifact missing journey entry")
	}
	if !strings.Contains(bodyStr, "Recommended action") {
		t.Errorf("rendered artifact missing recommended action heading")
	}
	if !strings.Contains(bodyStr, "Improvement | 17.92%") {
		// 120 → 98.5 = 17.92%
		t.Errorf("expected improvement percent 17.92, got body:\n%s", bodyStr)
	}
}

func TestExecutor_RequiredFields(t *testing.T) {
	e := NewExecutor(&fakePub{}, platform(), slog.Default(), t.TempDir())
	for _, k := range []string{"title", "command", "journey", "cap", "best_experiment_id"} {
		t.Run("missing "+k, func(t *testing.T) {
			args := baseArgs()
			delete(args, k)
			res, _ := e.Execute(context.Background(), agentic.ToolCall{
				ID: "c", Name: ToolName, LoopID: "s", Arguments: args,
			})
			if res.Error == "" {
				t.Errorf("expected error when %s missing", k)
			}
		})
	}
}

// TestExecutor_BestExperimentIDConsistency catches the M2 structural
// failure mode: best_experiment_id and iterations_kept must agree on
// whether a kept iteration exists. The reviewer's consistency check
// otherwise routes through reviewer-rejection → resynthesize, which
// is expensive.
func TestExecutor_BestExperimentIDConsistency(t *testing.T) {
	e := NewExecutor(&fakePub{}, platform(), slog.Default(), t.TempDir())

	t.Run("kept=0 but best_experiment_id is non-baseline", func(t *testing.T) {
		args := baseArgs()
		args["iterations_kept"] = float64(0)
		args["best_experiment_id"] = "some-id"
		res, _ := e.Execute(context.Background(), agentic.ToolCall{
			ID: "c", Name: ToolName, LoopID: "s", Arguments: args,
		})
		if res.Error == "" || !strings.Contains(res.Error, "use literal 'baseline'") {
			t.Errorf("expected baseline-guidance error; got %q", res.Error)
		}
	})

	t.Run("kept>0 but best_experiment_id is baseline", func(t *testing.T) {
		args := baseArgs()
		args["iterations_kept"] = float64(2)
		args["best_experiment_id"] = "baseline"
		res, _ := e.Execute(context.Background(), agentic.ToolCall{
			ID: "c", Name: ToolName, LoopID: "s", Arguments: args,
		})
		if res.Error == "" || !strings.Contains(res.Error, "pass the kept iteration") {
			t.Errorf("expected kept-id-required error; got %q", res.Error)
		}
	})

	t.Run("kept=0 with baseline accepted", func(t *testing.T) {
		args := baseArgs()
		args["iterations_kept"] = float64(0)
		args["best_experiment_id"] = "baseline"
		res, _ := e.Execute(context.Background(), agentic.ToolCall{
			ID: "c", Name: ToolName, LoopID: "s", Arguments: args,
		})
		if res.Error != "" {
			t.Errorf("baseline+kept=0 should pass; got %q", res.Error)
		}
	})
}

// TestExecutor_StampsCapAndBestExperimentID guards the new required
// fields surface as triples + render in the markdown template. These
// are reviewer-autoresearch evaluation-contract inputs (cap + the
// kept-iteration consistency check on best_experiment_id), and an
// emit that silently dropped them would re-introduce the
// post-#194 reviewer-rejection class.
func TestExecutor_StampsCapAndBestExperimentID(t *testing.T) {
	tmp := t.TempDir()
	pub := &fakePub{}
	e := NewExecutor(pub, platform(), slog.Default(), tmp)

	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "c", Name: ToolName, LoopID: "synth-cap", Arguments: baseArgs(),
	})
	if res.Error != "" {
		t.Fatalf("unexpected error: %s", res.Error)
	}

	capAny, ok := pub.find("autoresearch.artifact.cap")
	if !ok {
		t.Fatalf("cap triple missing")
	}
	if v, _ := capAny.(int); v != 10 {
		t.Errorf("cap triple = %v, want 10", capAny)
	}

	bestAny, ok := pub.find("autoresearch.artifact.best_experiment_id")
	if !ok {
		t.Fatalf("best_experiment_id triple missing")
	}
	if v, _ := bestAny.(string); v != "iter-2-loop-id" {
		t.Errorf("best_experiment_id triple = %q, want %q", v, "iter-2-loop-id")
	}

	// Rendered markdown carries both values for human review.
	pathAny, _ := pub.find("autoresearch.artifact.path")
	body, err := os.ReadFile(pathAny.(string))
	if err != nil {
		t.Fatalf("read rendered artifact: %v", err)
	}
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "| Cap | 10 |") {
		t.Errorf("rendered artifact missing cap row; got:\n%s", bodyStr)
	}
	if !strings.Contains(bodyStr, "iter-2-loop-id") {
		t.Errorf("rendered artifact missing best_experiment_id; got:\n%s", bodyStr)
	}
}

func TestExecutor_ComputesImprovementPctWhenMissing(t *testing.T) {
	tmp := t.TempDir()
	pub := &fakePub{}
	e := NewExecutor(pub, platform(), slog.Default(), tmp)

	args := baseArgs()
	delete(args, "improvement_pct")
	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "c", Name: ToolName, LoopID: "synth-2", Arguments: args,
	})
	if res.Error != "" {
		t.Fatalf("unexpected error: %s", res.Error)
	}
	pct, ok := pub.find("autoresearch.artifact.improvement_pct")
	if !ok {
		t.Fatalf("improvement_pct triple missing")
	}
	got, _ := pct.(float64)
	// (120 - 98.5) / 120 * 100 = 17.9166...
	if got < 17.9 || got > 18.0 {
		t.Errorf("improvement_pct = %v, want ~17.92", got)
	}
}

func TestExecutor_MissingLoopIDErrors(t *testing.T) {
	e := NewExecutor(&fakePub{}, platform(), slog.Default(), t.TempDir())
	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "c", Name: ToolName, Arguments: baseArgs(),
	})
	if res.Error == "" {
		t.Errorf("expected error without loop_id")
	}
}

func TestExecutor_OutputDirFromEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SEMTEAMS_AUTORESEARCH_ARTIFACT_DIR", dir)
	pub := &fakePub{}
	e := NewExecutor(pub, platform(), slog.Default(), "")

	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "c", Name: ToolName, LoopID: "synth-3", Arguments: baseArgs(),
	})
	if res.Error != "" {
		t.Fatalf("unexpected error: %s", res.Error)
	}
	// at least one .md file landed in dir
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	found := false
	for _, ent := range entries {
		if filepath.Ext(ent.Name()) == ".md" {
			found = true
		}
	}
	if !found {
		t.Errorf("no .md rendered into %s", dir)
	}
}

// fakeChainReader stubs ChainAttestationReader for the attestation
// path tests. Either wsf OR err is set per call shape; nil means
// "no chain reader wired" (passthrough handled in executor).
type fakeChainReader struct {
	wsf string
	err error
}

func (f *fakeChainReader) HostWorkspaceFolder(_ context.Context, _ agentic.ToolCall) (string, error) {
	return f.wsf, f.err
}

// fakeSandboxWriter captures the (wsf, relPath, content) tuple from
// each WriteFile call so the test can assert routing + content.
type fakeSandboxWriter struct {
	mu       sync.Mutex
	called   bool
	wsf      string
	relPath  string
	content  []byte
	writeErr error
}

func (f *fakeSandboxWriter) WriteFile(_ context.Context, hostWsf, relPath string, content []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.called = true
	f.wsf = hostWsf
	f.relPath = relPath
	f.content = append([]byte(nil), content...)
	return f.writeErr
}

// TestExecutor_AttestationPresent verifies the executor routes the
// artifact write through the sandbox writer (NOT the local fs) when
// the chain reader returns a non-empty host_workspace_folder, AND
// stamps a workspace-relative path on the artifact.path triple.
func TestExecutor_AttestationPresent(t *testing.T) {
	pub := &fakePub{}
	// Use a tmp local dir so a leaked os.WriteFile would be detectable.
	localTmp := t.TempDir()
	e := NewExecutor(pub, platform(), slog.Default(), localTmp)
	reader := &fakeChainReader{wsf: "/var/lib/semteams-tenants/sigxyz/workspace"}
	writer := &fakeSandboxWriter{}
	e.SetAttestationWriter(reader, writer)

	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "c-att", Name: ToolName, LoopID: "synth-att", Arguments: baseArgs(),
	})
	if res.Error != "" {
		t.Fatalf("unexpected error: %s", res.Error)
	}

	if !writer.called {
		t.Fatalf("expected sandbox writer to be called")
	}
	if writer.wsf != "/var/lib/semteams-tenants/sigxyz/workspace" {
		t.Errorf("wsf passed to writer = %q, want host wsf", writer.wsf)
	}
	if !strings.HasPrefix(writer.relPath, ".artifacts/autoresearch/") || !strings.HasSuffix(writer.relPath, ".md") {
		t.Errorf("relPath %q should look like .artifacts/autoresearch/<slug>.md", writer.relPath)
	}
	if !strings.Contains(string(writer.content), "## Summary") {
		t.Errorf("rendered content missing Summary heading; got:\n%s", string(writer.content))
	}

	// Stamped path matches the relPath the writer received (so the
	// reviewer's bash-cat resolves the same file).
	stampedAny, ok := pub.find("autoresearch.artifact.path")
	if !ok {
		t.Fatalf("artifact path triple missing")
	}
	stamped, _ := stampedAny.(string)
	if stamped != writer.relPath {
		t.Errorf("stamped path %q != writer relPath %q", stamped, writer.relPath)
	}

	// And NO local-fs leak — the tmp output dir should be empty.
	entries, _ := os.ReadDir(localTmp)
	for _, ent := range entries {
		if filepath.Ext(ent.Name()) == ".md" {
			t.Errorf("attestation path should not leak to local fs, found %s", ent.Name())
		}
	}
}

// TestExecutor_AttestationReaderError verifies the executor logs +
// degrades to the local-fs fallback when the chain reader errors
// (e.g. transient graph failure). A reader error MUST NOT fail the
// tool — the artifact still emits, just to the local fs.
func TestExecutor_AttestationReaderError(t *testing.T) {
	tmp := t.TempDir()
	pub := &fakePub{}
	e := NewExecutor(pub, platform(), slog.Default(), tmp)
	e.SetAttestationWriter(&fakeChainReader{err: errors.New("graph timeout")}, &fakeSandboxWriter{})

	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "c", Name: ToolName, LoopID: "synth-err", Arguments: baseArgs(),
	})
	if res.Error != "" {
		t.Fatalf("reader error should degrade gracefully, got tool error: %s", res.Error)
	}
	// One .md file landed in tmp (local fallback).
	entries, _ := os.ReadDir(tmp)
	found := false
	for _, ent := range entries {
		if filepath.Ext(ent.Name()) == ".md" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected local-fs fallback to write .md into %s", tmp)
	}
}

// TestExecutor_AttestationEmptyWorkspace verifies an empty wsf
// (attestation not ready, or host_workspace_folder triple absent)
// routes to the local-fs fallback — same shape as no-reader-wired.
func TestExecutor_AttestationEmptyWorkspace(t *testing.T) {
	tmp := t.TempDir()
	pub := &fakePub{}
	e := NewExecutor(pub, platform(), slog.Default(), tmp)
	writer := &fakeSandboxWriter{}
	e.SetAttestationWriter(&fakeChainReader{wsf: ""}, writer)

	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "c", Name: ToolName, LoopID: "synth-empty", Arguments: baseArgs(),
	})
	if res.Error != "" {
		t.Fatalf("unexpected error: %s", res.Error)
	}
	if writer.called {
		t.Errorf("sandbox writer should NOT be called when wsf is empty")
	}
	entries, _ := os.ReadDir(tmp)
	found := false
	for _, ent := range entries {
		if filepath.Ext(ent.Name()) == ".md" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected local-fs fallback to write .md into %s", tmp)
	}
}

// TestExecutor_AttestationWriterError surfaces sandbox-writer failure
// as a tool error. Unlike the reader-error case (transient, fall
// back), a writer-error means we KNOW where the artifact should go
// but couldn't get it there — silently writing to the wrong filesystem
// would re-introduce the #194 bug shape.
func TestExecutor_AttestationWriterError(t *testing.T) {
	tmp := t.TempDir()
	pub := &fakePub{}
	e := NewExecutor(pub, platform(), slog.Default(), tmp)
	reader := &fakeChainReader{wsf: "/var/lib/semteams-tenants/sig/workspace"}
	writer := &fakeSandboxWriter{writeErr: errors.New("sandbox /exec timed out")}
	e.SetAttestationWriter(reader, writer)

	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "c", Name: ToolName, LoopID: "synth-werr", Arguments: baseArgs(),
	})
	if res.Error == "" {
		t.Fatalf("expected tool error when sandbox write fails")
	}
	if !strings.Contains(res.Error, "tenant workspace") {
		t.Errorf("error message should reference tenant workspace; got %q", res.Error)
	}
}
