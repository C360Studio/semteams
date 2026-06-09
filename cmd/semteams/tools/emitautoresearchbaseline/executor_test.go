package emitautoresearchbaseline

import (
	"context"
	"log/slog"
	"sync"
	"testing"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/types"
)

func platform() types.PlatformMeta {
	return types.PlatformMeta{Org: "c360", Platform: "ops"}
}

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

func baseArgs() map[string]any {
	return map[string]any{
		"command":              "task test:integration",
		"surface":              "test/, Taskfile.yml",
		"cap":                  float64(10),
		"metric_parser":        "last line matching ^Total time: ([0-9.]+)s$",
		"baseline_value":       float64(123.4),
		"baseline_pass":        true,
		"baseline_stdout_tail": "Total time: 123.4s",
	}
}

func runMetadata() map[string]any {
	return map[string]any{
		agentic.MetadataKeyRelatedLoops: map[string]any{
			"run-loop-entity-id": "c360.ops.agent.agentic-loop.execution.coord-1",
			"autoresearch-run":   "coord-1",
		},
	}
}

func TestExecutor_Happy(t *testing.T) {
	pub := &fakePub{}
	e := NewExecutor(pub, platform(), slog.Default())
	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "c1", Name: ToolName, Arguments: baseArgs(), Metadata: runMetadata(),
	})
	if res.Error != "" {
		t.Fatalf("unexpected error: %s", res.Error)
	}

	// ADR-053 Phase 3a: the run entity id is derived from the run loop id
	// (autoresearch-run="coord-1") via TryChainExecutionEntityID, NOT taken
	// verbatim from run-loop-entity-id. So the subject is the chain.execution
	// entity, not the coordinator's agentic-loop entity.
	wantSubject := "c360.ops.agent.chain.execution.coord-1"
	for _, tr := range pub.triples {
		if tr.Subject != wantSubject {
			t.Errorf("subject = %q, want %q", tr.Subject, wantSubject)
		}
	}

	// Phase 3a: the run-lifecycle markers are now seeded here (were rule 01).
	if st, ok := pub.find("autoresearch.run.status"); !ok || st != "active" {
		t.Errorf("run.status = %v (ok=%v), want active", st, ok)
	}
	if ip, ok := pub.find("autoresearch.iteration.pending"); !ok || ip != "initial" {
		t.Errorf("iteration.pending = %v (ok=%v), want initial", ip, ok)
	}

	// best.value seeded from baseline.value
	bv, _ := pub.find("autoresearch.best.value")
	if bv != float64(123.4) {
		t.Errorf("best.value = %v, want 123.4", bv)
	}
	exp, _ := pub.find("autoresearch.best.experiment_id")
	if exp != "baseline" {
		t.Errorf("best.experiment_id = %v, want baseline", exp)
	}
	gotCap, _ := pub.find("autoresearch.cap")
	if gotCap != 10 {
		t.Errorf("cap = %v, want 10", gotCap)
	}
}

func TestExecutor_MissingRunEntityFails(t *testing.T) {
	e := NewExecutor(&fakePub{}, platform(), slog.Default())
	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "c2", Name: ToolName, Arguments: baseArgs(),
	})
	if res.Error == "" {
		t.Errorf("expected error when autoresearch-run (run loop id) missing")
	}
}

func TestExecutor_RequiredFields(t *testing.T) {
	e := NewExecutor(&fakePub{}, platform(), slog.Default())
	for _, k := range []string{"command", "surface", "metric_parser"} {
		t.Run("missing "+k, func(t *testing.T) {
			args := baseArgs()
			delete(args, k)
			res, _ := e.Execute(context.Background(), agentic.ToolCall{
				ID: "c", Name: ToolName, Arguments: args, Metadata: runMetadata(),
			})
			if res.Error == "" {
				t.Errorf("expected error when %s missing", k)
			}
		})
	}
}

func TestExecutor_CapMinimum(t *testing.T) {
	e := NewExecutor(&fakePub{}, platform(), slog.Default())
	args := baseArgs()
	args["cap"] = float64(0)
	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "c3", Name: ToolName, Arguments: args, Metadata: runMetadata(),
	})
	if res.Error == "" {
		t.Errorf("expected error when cap < 1")
	}
}
