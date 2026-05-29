package emitautoresearchartifact

import (
	"context"
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
		"iterations_completed": float64(8),
		"iterations_kept":      float64(2),
		"iterations_reverted":  float64(5),
		"iterations_crashed":   float64(1),
		"best_diff_summary":    "Batched setup in test/fixtures/setup.go",
		"journey":              []any{"1: hypothesis A → 115.0 (reverted)", "2: hypothesis B → 98.5 (kept)"},
		"open_opportunities":   []any{"explore concurrent fixture setup"},
		"recommended_action":   "Commit the kept diff; re-run with surface widened to internal/.",
	}
}

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
	// path is workspace-relative when outputDir is relative; we
	// pass an absolute tmp, so the renderer returns an absolute path.
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
	for _, k := range []string{"title", "command", "journey"} {
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
