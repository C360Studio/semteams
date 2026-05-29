package emitautoresearchmeasurement

import (
	"context"
	"errors"
	"log/slog"
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

func (f *fakePub) findOn(subject, pred string) (any, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, t := range f.triples {
		if t.Subject == subject && t.Predicate == pred {
			return t.Object, true
		}
	}
	return nil, false
}

type fakeReader struct {
	best   float64
	hasIt  bool
	err    error
	calls  int
	calls2 []string
}

func (r *fakeReader) ReadBestValue(_ context.Context, runEntityID string) (float64, bool, error) {
	r.calls++
	r.calls2 = append(r.calls2, runEntityID)
	if r.err != nil {
		return 0, false, r.err
	}
	if !r.hasIt {
		return 0, false, nil
	}
	return r.best, true, nil
}

func platform() types.PlatformMeta {
	return types.PlatformMeta{Org: "c360", Platform: "ops"}
}

func runMetadata() map[string]any {
	return map[string]any{
		agentic.MetadataKeyRelatedLoops: map[string]any{
			"run-loop-entity-id": "c360.ops.agent.agentic-loop.execution.coord-1",
		},
	}
}

// TestCompareOutcome pins the empirical-compare contract. This is
// the load-bearing structural test of the autoresearch substrate.
func TestCompareOutcome(t *testing.T) {
	cases := []struct {
		name        string
		value, best float64
		pass        bool
		wantOutcome string
		wantDelta   float64
	}{
		{"kept: lower than best", 100, 120, true, OutcomeKept, -20},
		{"reverted: equal to best", 120, 120, true, OutcomeReverted, 0},
		{"reverted: higher than best", 130, 120, true, OutcomeReverted, 10},
		{"crashed: pass false overrides numeric improvement", 50, 120, false, OutcomeCrashed, -70},
		{"crashed: pass false on worse value", 200, 120, false, OutcomeCrashed, 80},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotOutcome, gotDelta := compareOutcome(c.value, c.pass, c.best)
			if gotOutcome != c.wantOutcome {
				t.Errorf("outcome = %q, want %q", gotOutcome, c.wantOutcome)
			}
			if gotDelta != c.wantDelta {
				t.Errorf("delta = %v, want %v", gotDelta, c.wantDelta)
			}
		})
	}
}

func TestExecutor_KeptUpdatesBestOnRunEntity(t *testing.T) {
	pub := &fakePub{}
	reader := &fakeReader{best: 200, hasIt: true}
	e := NewExecutor(pub, reader, platform(), slog.Default())

	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "c1", Name: ToolName, LoopID: "exec-7",
		Arguments: map[string]any{
			"value": float64(150),
			"pass":  true,
		},
		Metadata: runMetadata(),
	})
	if res.Error != "" {
		t.Fatalf("unexpected error: %s", res.Error)
	}
	execEntity := "c360.ops.agent.agentic-loop.execution.exec-7"
	runEntity := "c360.ops.agent.agentic-loop.execution.coord-1"

	// outcome stamped kept on execute loop.
	out, _ := pub.findOn(execEntity, "autoresearch.measurement.outcome")
	if out != OutcomeKept {
		t.Errorf("execute outcome = %v, want %q", out, OutcomeKept)
	}

	// best.value updated on RUN entity to the new value.
	newBest, _ := pub.findOn(runEntity, "autoresearch.best.value")
	if newBest != float64(150) {
		t.Errorf("run best.value = %v, want 150", newBest)
	}
	// best.experiment_id = the execute loop_id.
	newExp, _ := pub.findOn(runEntity, "autoresearch.best.experiment_id")
	if newExp != "exec-7" {
		t.Errorf("run best.experiment_id = %v, want exec-7", newExp)
	}
}

func TestExecutor_RevertedLeavesRunBestAlone(t *testing.T) {
	pub := &fakePub{}
	reader := &fakeReader{best: 100, hasIt: true}
	e := NewExecutor(pub, reader, platform(), slog.Default())

	_, _ = e.Execute(context.Background(), agentic.ToolCall{
		ID: "c2", Name: ToolName, LoopID: "exec-8",
		Arguments: map[string]any{"value": float64(110), "pass": true},
		Metadata:  runMetadata(),
	})

	runEntity := "c360.ops.agent.agentic-loop.execution.coord-1"
	if _, found := pub.findOn(runEntity, "autoresearch.best.value"); found {
		t.Errorf("best.value should not be updated on reverted outcome")
	}
}

func TestExecutor_CrashedStampsOutcomeNoRunBestUpdate(t *testing.T) {
	pub := &fakePub{}
	reader := &fakeReader{best: 100, hasIt: true}
	e := NewExecutor(pub, reader, platform(), slog.Default())

	_, _ = e.Execute(context.Background(), agentic.ToolCall{
		ID: "c3", Name: ToolName, LoopID: "exec-9",
		Arguments: map[string]any{"value": float64(50), "pass": false},
		Metadata:  runMetadata(),
	})

	execEntity := "c360.ops.agent.agentic-loop.execution.exec-9"
	runEntity := "c360.ops.agent.agentic-loop.execution.coord-1"
	out, _ := pub.findOn(execEntity, "autoresearch.measurement.outcome")
	if out != OutcomeCrashed {
		t.Errorf("outcome = %v, want crashed", out)
	}
	if _, found := pub.findOn(runEntity, "autoresearch.best.value"); found {
		t.Errorf("best.value should not be updated on crashed outcome")
	}
}

func TestExecutor_MissingBestErrorsExplicit(t *testing.T) {
	pub := &fakePub{}
	reader := &fakeReader{hasIt: false}
	e := NewExecutor(pub, reader, platform(), slog.Default())

	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "c4", Name: ToolName, LoopID: "exec-10",
		Arguments: map[string]any{"value": float64(100), "pass": true},
		Metadata:  runMetadata(),
	})
	if res.Error == "" {
		t.Errorf("expected error when best.value absent")
	}
	if res.ErrorKind != agentic.ToolErrorInternal {
		t.Errorf("error kind = %q, want %q", res.ErrorKind, agentic.ToolErrorInternal)
	}
}

func TestExecutor_ReadBestErrorIsNetwork(t *testing.T) {
	pub := &fakePub{}
	reader := &fakeReader{err: errors.New("graph timeout")}
	e := NewExecutor(pub, reader, platform(), slog.Default())

	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "c5", Name: ToolName, LoopID: "exec-11",
		Arguments: map[string]any{"value": float64(100), "pass": true},
		Metadata:  runMetadata(),
	})
	if res.ErrorKind != agentic.ToolErrorNetwork {
		t.Errorf("error kind = %q, want %q", res.ErrorKind, agentic.ToolErrorNetwork)
	}
}

func TestExecutor_RequiredFields(t *testing.T) {
	pub := &fakePub{}
	reader := &fakeReader{best: 100, hasIt: true}
	e := NewExecutor(pub, reader, platform(), slog.Default())

	cases := []map[string]any{
		{},                     // no value, no pass
		{"value": float64(50)}, // no pass
		{"pass": true},         // no value
	}
	for i, args := range cases {
		t.Run("case", func(t *testing.T) {
			res, _ := e.Execute(context.Background(), agentic.ToolCall{
				ID: "c", Name: ToolName, LoopID: "ex",
				Arguments: args, Metadata: runMetadata(),
			})
			if res.ErrorKind != agentic.ToolErrorInvalidArgs {
				t.Errorf("case %d: error kind = %q, want %q", i, res.ErrorKind, agentic.ToolErrorInvalidArgs)
			}
		})
	}
}

func TestExecutor_MissingRunEntityErrors(t *testing.T) {
	pub := &fakePub{}
	reader := &fakeReader{best: 100, hasIt: true}
	e := NewExecutor(pub, reader, platform(), slog.Default())

	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "c", Name: ToolName, LoopID: "ex",
		Arguments: map[string]any{"value": float64(100), "pass": true},
		// no Metadata
	})
	if res.Error == "" {
		t.Errorf("expected error without run-loop-entity-id")
	}
}

func TestExecutor_MissingLoopIDErrors(t *testing.T) {
	pub := &fakePub{}
	reader := &fakeReader{best: 100, hasIt: true}
	e := NewExecutor(pub, reader, platform(), slog.Default())

	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "c", Name: ToolName,
		Arguments: map[string]any{"value": float64(100), "pass": true},
		Metadata:  runMetadata(),
	})
	if res.Error == "" {
		t.Errorf("expected error without loop_id")
	}
}
