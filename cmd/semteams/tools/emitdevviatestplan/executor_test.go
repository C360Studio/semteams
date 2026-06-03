package emitdevviatestplan

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
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

func baseTask() map[string]any {
	return map[string]any{
		"id":               "t1",
		"goal":             "Implement ParseISO8601",
		"assumptions":      []any{"input is UTF-8"},
		"non_goals":        []any{},
		"target_files":     []any{"parse.go", "parse_test.go"},
		"depends_on":       []any{},
		"test_command":     "go test -run TestParseISO8601 ./...",
		"expected_outcome": "all six ISO-8601 edge cases pass",
	}
}

func baseArgs() map[string]any {
	return map[string]any{
		"goal":                     "Implement and test the ISO-8601 parser",
		"assumptions":              []any{"Go 1.25 toolchain available"},
		"non_goals":                []any{"do not refactor time package"},
		"integration_test_command": "go test ./... && go vet ./...",
		"revision":                 float64(1),
		"tasks":                    []any{baseTask()},
	}
}

func runMetadata() map[string]any {
	return map[string]any{
		agentic.MetadataKeyRelatedLoops: map[string]any{
			"run-loop-entity-id": "c360.ops.agent.agentic-loop.execution.coord-1",
			"dev-via-test-run":   "coord-1",
		},
	}
}

func TestExecutor_Happy(t *testing.T) {
	pub := &fakePub{}
	e := NewExecutor(pub, slog.Default())
	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "c1", Name: ToolName, Arguments: baseArgs(), Metadata: runMetadata(),
	})
	if res.Error != "" {
		t.Fatalf("unexpected error: %s", res.Error)
	}

	wantSubject := "c360.ops.agent.agentic-loop.execution.coord-1"
	for _, tr := range pub.snapshot() {
		if tr.Subject != wantSubject {
			t.Errorf("subject = %q, want %q", tr.Subject, wantSubject)
		}
		if tr.Source != toolSource {
			t.Errorf("source = %q, want %q", tr.Source, toolSource)
		}
	}

	taskCount, _ := pub.find(predicatePlanTaskCount)
	if taskCount != 1 {
		t.Errorf("task_count = %v, want 1", taskCount)
	}
	tag, _ := pub.find(predicatePlanChainStartGitTag)
	if tag != defaultChainStartGitTag {
		t.Errorf("chain_start_git_tag = %v, want %q", tag, defaultChainStartGitTag)
	}
	cmd, _ := pub.find(predicatePlanIntegrationTestCmd)
	if cmd != "go test ./... && go vet ./..." {
		t.Errorf("integration_test_command = %v", cmd)
	}

	// Per-task stamps.
	gotGoal, ok := pub.find("plan.task.t1." + predicateTaskGoal)
	if !ok || gotGoal != "Implement ParseISO8601" {
		t.Errorf("plan.task.t1.goal = %v", gotGoal)
	}
	gotStatus, ok := pub.find("plan.task.t1." + predicateTaskStatus)
	if !ok || gotStatus != taskStatusReady {
		t.Errorf("plan.task.t1.status = %v, want %q", gotStatus, taskStatusReady)
	}
	gotPos, ok := pub.find("plan.task.t1." + predicateTaskPosition)
	if !ok || gotPos != 0 {
		t.Errorf("plan.task.t1.position = %v, want 0", gotPos)
	}
	// Arrays are JSON-encoded strings (so $entity.triple substitutions
	// interpolate a predictable shape into downstream prompts).
	gotTargetFilesAny, ok := pub.find("plan.task.t1." + predicateTaskTargetFiles)
	if !ok {
		t.Fatal("plan.task.t1.target_files missing")
	}
	gotTargetFiles, ok := gotTargetFilesAny.(string)
	if !ok {
		t.Fatalf("plan.task.t1.target_files type = %T, want string (JSON-encoded array)", gotTargetFilesAny)
	}
	var roundtripped []string
	if err := json.Unmarshal([]byte(gotTargetFiles), &roundtripped); err != nil {
		t.Fatalf("plan.task.t1.target_files not valid JSON: %v", err)
	}
	if len(roundtripped) != 2 || roundtripped[0] != "parse.go" || roundtripped[1] != "parse_test.go" {
		t.Errorf("plan.task.t1.target_files = %v", roundtripped)
	}
}

func TestExecutor_MissingRunEntityFails(t *testing.T) {
	e := NewExecutor(&fakePub{}, slog.Default())
	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "c2", Name: ToolName, Arguments: baseArgs(),
	})
	if res.Error == "" {
		t.Errorf("expected error when run-loop-entity-id missing")
	}
}

func TestExecutor_RequiredPlanFields(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(map[string]any)
		want   string
	}{
		{
			name:   "missing goal",
			mutate: func(m map[string]any) { delete(m, "goal") },
			want:   "plan.goal is required",
		},
		{
			name:   "missing assumptions (Karpathy Rule 1)",
			mutate: func(m map[string]any) { delete(m, "assumptions") },
			want:   "plan.assumptions is required",
		},
		{
			name:   "missing non_goals (Karpathy Rule 2)",
			mutate: func(m map[string]any) { delete(m, "non_goals") },
			want:   "plan.non_goals is required",
		},
		{
			name:   "missing integration_test_command (Karpathy Rule 4 plan-level)",
			mutate: func(m map[string]any) { delete(m, "integration_test_command") },
			want:   "plan.integration_test_command is required",
		},
		{
			name:   "tasks empty",
			mutate: func(m map[string]any) { m["tasks"] = []any{} },
			want:   "plan.tasks must contain at least one task",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := baseArgs()
			tc.mutate(args)
			pub := &fakePub{}
			e := NewExecutor(pub, slog.Default())
			res, _ := e.Execute(context.Background(), agentic.ToolCall{
				ID: "c", Name: ToolName, Arguments: args, Metadata: runMetadata(),
			})
			if res.Error == "" {
				t.Fatalf("expected error %q, got none", tc.want)
			}
			if !strings.Contains(res.Error, tc.want) {
				t.Errorf("error = %q, want substring %q", res.Error, tc.want)
			}
			if len(pub.snapshot()) > 0 {
				t.Errorf("expected no triples on validation failure, got %d", len(pub.snapshot()))
			}
		})
	}
}

func TestExecutor_RequiredTaskFields(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(map[string]any)
		want   string
	}{
		{
			name:   "task missing id",
			mutate: func(t map[string]any) { delete(t, "id") },
			want:   "tasks[0].id",
		},
		{
			name:   "task id violates pattern",
			mutate: func(t map[string]any) { t["id"] = "Task With Spaces" },
			want:   "does not match required pattern",
		},
		{
			name:   "task missing goal",
			mutate: func(t map[string]any) { delete(t, "goal") },
			want:   ".goal is required",
		},
		{
			name:   "task missing assumptions",
			mutate: func(t map[string]any) { delete(t, "assumptions") },
			want:   ".assumptions is required",
		},
		{
			name:   "task missing non_goals",
			mutate: func(t map[string]any) { delete(t, "non_goals") },
			want:   ".non_goals is required",
		},
		{
			name:   "task missing target_files (Karpathy Rule 3)",
			mutate: func(t map[string]any) { delete(t, "target_files") },
			want:   "target_files must contain at least one path",
		},
		{
			name:   "task target_files empty",
			mutate: func(t map[string]any) { t["target_files"] = []any{} },
			want:   "target_files must contain at least one path",
		},
		{
			name:   "task missing test_command (Karpathy Rule 4)",
			mutate: func(t map[string]any) { delete(t, "test_command") },
			want:   "test_command is required",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := baseArgs()
			task := baseTask()
			tc.mutate(task)
			args["tasks"] = []any{task}
			pub := &fakePub{}
			e := NewExecutor(pub, slog.Default())
			res, _ := e.Execute(context.Background(), agentic.ToolCall{
				ID: "c", Name: ToolName, Arguments: args, Metadata: runMetadata(),
			})
			if res.Error == "" {
				t.Fatalf("expected error %q, got none", tc.want)
			}
			if !strings.Contains(res.Error, tc.want) {
				t.Errorf("error = %q, want substring %q", res.Error, tc.want)
			}
		})
	}
}

func TestExecutor_DuplicateTaskIDsRejected(t *testing.T) {
	args := baseArgs()
	t1 := baseTask()
	t2 := baseTask()
	t2["goal"] = "Different goal but same id"
	args["tasks"] = []any{t1, t2}
	pub := &fakePub{}
	e := NewExecutor(pub, slog.Default())
	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "c", Name: ToolName, Arguments: args, Metadata: runMetadata(),
	})
	if res.Error == "" || !strings.Contains(res.Error, "duplicated") {
		t.Errorf("expected duplicate-id error, got %q", res.Error)
	}
}

func TestExecutor_MultiTaskOrdering(t *testing.T) {
	args := baseArgs()
	t1 := baseTask()
	t2 := baseTask()
	t2["id"] = "t2"
	t2["goal"] = "Second task"
	t2["depends_on"] = []any{"t1"}
	args["tasks"] = []any{t1, t2}
	pub := &fakePub{}
	e := NewExecutor(pub, slog.Default())
	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "c", Name: ToolName, Arguments: args, Metadata: runMetadata(),
	})
	if res.Error != "" {
		t.Fatalf("unexpected error: %s", res.Error)
	}
	gotCount, _ := pub.find(predicatePlanTaskCount)
	if gotCount != 2 {
		t.Errorf("task_count = %v, want 2", gotCount)
	}
	// Positions assigned by emit order.
	p1, _ := pub.find("plan.task.t1." + predicateTaskPosition)
	p2, _ := pub.find("plan.task.t2." + predicateTaskPosition)
	if p1 != 0 || p2 != 1 {
		t.Errorf("positions = (t1=%v, t2=%v), want (0, 1)", p1, p2)
	}
	// Per-task status seeded to "ready".
	s1, _ := pub.find("plan.task.t1." + predicateTaskStatus)
	s2, _ := pub.find("plan.task.t2." + predicateTaskStatus)
	if s1 != taskStatusReady || s2 != taskStatusReady {
		t.Errorf("status = (t1=%v, t2=%v), want both %q", s1, s2, taskStatusReady)
	}
}

func TestExecutor_RevisionAbsentRejected(t *testing.T) {
	// Per reviewer R1 (Slice 1 review): revision is required at the
	// schema level. Absent (Go zero-int) and explicit zero both
	// surface as the same validator error — no silent coercion.
	args := baseArgs()
	delete(args, "revision")
	pub := &fakePub{}
	e := NewExecutor(pub, slog.Default())
	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "c", Name: ToolName, Arguments: args, Metadata: runMetadata(),
	})
	if res.Error == "" {
		t.Fatalf("expected error when revision absent, got none")
	}
	if !strings.Contains(res.Error, "plan.revision is required") {
		t.Errorf("error = %q, want substring %q", res.Error, "plan.revision is required")
	}
}

func TestExecutor_RevisionZeroRejected(t *testing.T) {
	args := baseArgs()
	args["revision"] = float64(0)
	pub := &fakePub{}
	e := NewExecutor(pub, slog.Default())
	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "c", Name: ToolName, Arguments: args, Metadata: runMetadata(),
	})
	if res.Error == "" {
		t.Fatalf("expected error when revision=0, got none")
	}
	if !strings.Contains(res.Error, "plan.revision is required") {
		t.Errorf("error = %q, want substring %q", res.Error, "plan.revision is required")
	}
}

func TestExecutor_RevisionExplicit(t *testing.T) {
	args := baseArgs()
	args["revision"] = float64(3)
	pub := &fakePub{}
	e := NewExecutor(pub, slog.Default())
	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "c", Name: ToolName, Arguments: args, Metadata: runMetadata(),
	})
	if res.Error != "" {
		t.Fatalf("unexpected error: %s", res.Error)
	}
	rev, _ := pub.find(predicatePlanRevision)
	if rev != 3 {
		t.Errorf("revision = %v, want 3", rev)
	}
}

func TestExecutor_UnknownFieldsRejected(t *testing.T) {
	args := baseArgs()
	args["unexpected_top_level"] = "should fail"
	pub := &fakePub{}
	e := NewExecutor(pub, slog.Default())
	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "c", Name: ToolName, Arguments: args, Metadata: runMetadata(),
	})
	if res.Error == "" {
		t.Errorf("expected error on unknown top-level field")
	}
}

func TestExecutor_UnknownTaskFieldRejected(t *testing.T) {
	// Per reviewer N2 (Slice 1 review): per-task unknown-field
	// rejection is the same DisallowUnknownFields call walking into
	// []task — pin it explicitly so a future schema split doesn't
	// silently relax this side.
	args := baseArgs()
	task := baseTask()
	task["unexpected_task_field"] = "should fail"
	args["tasks"] = []any{task}
	pub := &fakePub{}
	e := NewExecutor(pub, slog.Default())
	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "c", Name: ToolName, Arguments: args, Metadata: runMetadata(),
	})
	if res.Error == "" {
		t.Errorf("expected error on per-task unknown field")
	}
	if !strings.Contains(res.Error, "unknown field") {
		t.Errorf("error = %q, want substring %q", res.Error, "unknown field")
	}
}

func TestExecutor_ExplicitNullRequiredArrayRejected(t *testing.T) {
	// Per reviewer N7 (Slice 1 review): the validator rejects nil
	// for required arrays. Persona spec says "emit [] explicitly" —
	// explicit null is not [] and must surface as a clear error
	// (not silently treated as the empty array).
	for _, field := range []string{"assumptions", "non_goals"} {
		t.Run("plan."+field+" null", func(t *testing.T) {
			args := baseArgs()
			args[field] = nil
			pub := &fakePub{}
			e := NewExecutor(pub, slog.Default())
			res, _ := e.Execute(context.Background(), agentic.ToolCall{
				ID: "c", Name: ToolName, Arguments: args, Metadata: runMetadata(),
			})
			if res.Error == "" {
				t.Fatalf("expected error when plan.%s = null", field)
			}
			if !strings.Contains(res.Error, "plan."+field+" is required") {
				t.Errorf("error = %q, want substring %q", res.Error, "plan."+field+" is required")
			}
		})
	}
}
