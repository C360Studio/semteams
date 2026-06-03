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

func TestExecutor_KeptStampsOutcomeButLeavesRunBestToRule04c(t *testing.T) {
	// 2026-06-03: the executor used to AddTriple best.value /
	// best.experiment_id on the run entity directly. That hit the
	// first-write-wins trap (GetFieldValue returns the FIRST matching
	// triple; baseline.value stamped first wins forever). The
	// TriplePublisher interface upstream has no upsert primitive,
	// so the update_triple responsibility now lives in
	// configs/rules/autoresearch/04c-execute-promote-best-on-kept.json
	// which performs the proper scalar upsert via the rule engine.
	//
	// This test enforces the new contract: the executor stamps
	// measurement.* (including outcome=kept) on the execute entity
	// ONLY; it does NOT touch the run entity's best.*.
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

	// outcome stamped kept on execute loop — this is what rule 04c
	// fires on.
	out, _ := pub.findOn(execEntity, "autoresearch.measurement.outcome")
	if out != OutcomeKept {
		t.Errorf("execute outcome = %v, want %q", out, OutcomeKept)
	}

	// value stamped on execute loop — rule 04c's update_triple
	// substitutes against this.
	val, _ := pub.findOn(execEntity, "autoresearch.measurement.value")
	if val != float64(150) {
		t.Errorf("execute measurement.value = %v, want 150 (rule 04c's update_triple substitution source)", val)
	}

	// CRITICAL: run entity must NOT receive best.value / best.experiment_id
	// from the executor anymore. Any such stamp would (a) be invisible to
	// scalar reads due to the first-write-wins trap, AND (b) waste a NATS
	// round-trip. Rule 04c handles this.
	if v, found := pub.findOn(runEntity, "autoresearch.best.value"); found {
		t.Errorf("run entity best.value stamped by executor (%v); must be delegated to rule 04c update_triple — see executor.go fix-comment 2026-06-03", v)
	}
	if v, found := pub.findOn(runEntity, "autoresearch.best.experiment_id"); found {
		t.Errorf("run entity best.experiment_id stamped by executor (%v); must be delegated to rule 04c update_triple", v)
	}
}

func TestExecutor_RevertedDoesNotStampRunBest(t *testing.T) {
	// Reverted outcome must also NOT stamp best.* on run entity (rule
	// 04c only fires on outcome=kept). Sanity-check that no path
	// leaks through.
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
		t.Errorf("best.value stamped on run entity for reverted outcome — must be 0 stamps (rule 04c gates on outcome=kept and lives in the rule layer regardless)")
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
