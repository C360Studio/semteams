package emitdevviatestmeasurement

import (
	"context"
	"log/slog"
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

func (f *fakePub) snapshot() []message.Triple {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]message.Triple, len(f.triples))
	copy(out, f.triples)
	return out
}

func testPlatform() types.PlatformMeta {
	return types.PlatformMeta{Org: "c360", Platform: "ops"}
}

func baseCall(args map[string]any) agentic.ToolCall {
	return agentic.ToolCall{
		ID:        "call-1",
		Name:      ToolName,
		LoopID:    "ralph-loop-001",
		Arguments: args,
	}
}

func TestExecutor_PassTrueDefaultsValueTo1(t *testing.T) {
	pub := &fakePub{}
	e := NewExecutor(pub, testPlatform(), slog.Default())
	res, _ := e.Execute(context.Background(), baseCall(map[string]any{
		"pass": true,
	}))
	if res.Error != "" {
		t.Fatalf("unexpected error: %s", res.Error)
	}
	got, _ := pub.find(predicateMeasurementValue)
	if got != 1.0 {
		t.Errorf("default value when pass=true: got %v, want 1.0", got)
	}
	gotPass, _ := pub.find(predicateMeasurementPass)
	if gotPass != true {
		t.Errorf("pass = %v, want true", gotPass)
	}
}

func TestExecutor_PassFalseDefaultsValueTo0(t *testing.T) {
	pub := &fakePub{}
	e := NewExecutor(pub, testPlatform(), slog.Default())
	res, _ := e.Execute(context.Background(), baseCall(map[string]any{
		"pass": false,
	}))
	if res.Error != "" {
		t.Fatalf("unexpected error: %s", res.Error)
	}
	got, _ := pub.find(predicateMeasurementValue)
	if got != 0.0 {
		t.Errorf("default value when pass=false: got %v, want 0.0", got)
	}
}

func TestExecutor_PassMissingRejected(t *testing.T) {
	pub := &fakePub{}
	e := NewExecutor(pub, testPlatform(), slog.Default())
	res, _ := e.Execute(context.Background(), baseCall(map[string]any{}))
	if res.Error == "" {
		t.Errorf("expected error when pass missing, got none")
	}
	if !strings.Contains(res.Error, "pass is required") {
		t.Errorf("error = %q, want substring %q", res.Error, "pass is required")
	}
	if len(pub.snapshot()) > 0 {
		t.Errorf("expected no triples on validation failure, got %d", len(pub.snapshot()))
	}
}

func TestExecutor_PassWrongTypeRejected(t *testing.T) {
	pub := &fakePub{}
	e := NewExecutor(pub, testPlatform(), slog.Default())
	res, _ := e.Execute(context.Background(), baseCall(map[string]any{
		"pass": "true", // string, not bool
	}))
	if res.Error == "" {
		t.Errorf("expected error when pass is wrong type")
	}
}

func TestExecutor_ValueIsNotLLMArg(t *testing.T) {
	// Per Slice 2 reviewer R3 + N6: value is NOT an LLM-visible arg
	// in v1 (derived from pass only). Pinning the rejection here
	// guards against re-introducing the field accidentally — v2
	// fractional support is the ONLY thing that lifts this gate.
	pub := &fakePub{}
	e := NewExecutor(pub, testPlatform(), slog.Default())
	res, _ := e.Execute(context.Background(), baseCall(map[string]any{
		"pass":  true,
		"value": float64(0.7),
	}))
	if res.Error == "" {
		t.Fatalf("expected unknown-field error for 'value'; got none")
	}
	if !strings.Contains(res.Error, "unknown field") {
		t.Errorf("error = %q, want unknown-field rejection", res.Error)
	}
}

func TestExecutor_MissingLoopIDFails(t *testing.T) {
	pub := &fakePub{}
	e := NewExecutor(pub, testPlatform(), slog.Default())
	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID:        "c",
		Name:      ToolName,
		Arguments: map[string]any{"pass": true},
		// LoopID intentionally empty.
	})
	if res.Error == "" {
		t.Errorf("expected error when loop_id missing")
	}
}

func TestExecutor_UnknownFieldRejected(t *testing.T) {
	pub := &fakePub{}
	e := NewExecutor(pub, testPlatform(), slog.Default())
	res, _ := e.Execute(context.Background(), baseCall(map[string]any{
		"pass":             true,
		"unexpected_field": "should fail",
	}))
	if res.Error == "" {
		t.Errorf("expected error on unknown field")
	}
	if !strings.Contains(res.Error, "unknown field") {
		t.Errorf("error = %q, want substring %q", res.Error, "unknown field")
	}
}

func TestExecutor_StdoutStderrTailsStamped(t *testing.T) {
	pub := &fakePub{}
	e := NewExecutor(pub, testPlatform(), slog.Default())
	res, _ := e.Execute(context.Background(), baseCall(map[string]any{
		"pass":        false,
		"stdout_tail": "FAIL TestParseISO8601",
		"stderr_tail": "panic: invalid input",
	}))
	if res.Error != "" {
		t.Fatalf("unexpected error: %s", res.Error)
	}
	stdout, _ := pub.find(predicateMeasurementStdoutTail)
	if stdout != "FAIL TestParseISO8601" {
		t.Errorf("stdout_tail = %v", stdout)
	}
	stderr, _ := pub.find(predicateMeasurementStderrTail)
	if stderr != "panic: invalid input" {
		t.Errorf("stderr_tail = %v", stderr)
	}
}

func TestExecutor_EmptyTailsNotStamped(t *testing.T) {
	pub := &fakePub{}
	e := NewExecutor(pub, testPlatform(), slog.Default())
	res, _ := e.Execute(context.Background(), baseCall(map[string]any{
		"pass": true,
	}))
	if res.Error != "" {
		t.Fatalf("unexpected error: %s", res.Error)
	}
	if _, ok := pub.find(predicateMeasurementStdoutTail); ok {
		t.Error("stdout_tail stamped when not supplied; should be omitted")
	}
	if _, ok := pub.find(predicateMeasurementStderrTail); ok {
		t.Error("stderr_tail stamped when not supplied; should be omitted")
	}
}

func TestExecutor_SubjectIsRalphLoopEntity(t *testing.T) {
	pub := &fakePub{}
	e := NewExecutor(pub, testPlatform(), slog.Default())
	res, _ := e.Execute(context.Background(), baseCall(map[string]any{
		"pass": true,
	}))
	if res.Error != "" {
		t.Fatalf("unexpected error: %s", res.Error)
	}
	// Every triple must target Ralph's loop entity, not the run entity.
	// Subject derived from platform.Org + platform.Platform + LoopID
	// via agentic.LoopExecutionEntityID — exact format expressed via
	// the framework helper to avoid coupling tests to the format.
	wantSubject, err := agentic.TryLoopExecutionEntityID("c360", "ops", "ralph-loop-001")
	if err != nil {
		t.Fatalf("derive expected subject: %v", err)
	}
	for _, tr := range pub.snapshot() {
		if tr.Subject != wantSubject {
			t.Errorf("subject = %q, want %q", tr.Subject, wantSubject)
		}
		if tr.Source != toolSource {
			t.Errorf("source = %q, want %q", tr.Source, toolSource)
		}
	}
}
