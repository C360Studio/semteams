package emitbootstrapverify

import (
	"context"
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

func TestExecutor_OKHappyPath(t *testing.T) {
	pub := &fakePub{}
	e := NewExecutor(pub, platform(), slog.Default())
	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "c1", Name: ToolName, LoopID: "verify-1",
		Arguments: map[string]any{
			"container_name":         "semteams-tenant-abc",
			"workspace_path":         "/workspace",
			"smoke_exit_code":        float64(0),
			"smoke_stdout_tail":      "PASS",
			"smoke_matches_expected": true,
			"verify_outcome":         "ok",
		},
	})
	if res.Error != "" {
		t.Fatalf("unexpected error: %s", res.Error)
	}
	if _, ok := pub.find("sandbox.verify.verify_outcome"); !ok {
		t.Errorf("verify_outcome triple missing")
	}
	if _, ok := pub.find("sandbox.verify.container_name"); !ok {
		t.Errorf("container_name triple missing")
	}
	if got, _ := pub.find("sandbox.verify.smoke_matches_expected"); got != true {
		t.Errorf("smoke_matches_expected = %v, want true", got)
	}
	// Subject should be the loop entity ID.
	wantSubject := "c360.ops.agent.agentic-loop.execution.verify-1"
	for _, tr := range pub.triples {
		if tr.Subject != wantSubject {
			t.Errorf("subject = %q, want %q", tr.Subject, wantSubject)
		}
	}
}

func TestExecutor_ContainerMissingAllowsBlankContainer(t *testing.T) {
	pub := &fakePub{}
	e := NewExecutor(pub, platform(), slog.Default())
	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "c2", Name: ToolName, LoopID: "verify-2",
		Arguments: map[string]any{
			"smoke_exit_code":        float64(-1),
			"smoke_matches_expected": false,
			"verify_outcome":         "container_missing",
		},
	})
	if res.Error != "" {
		t.Fatalf("unexpected error: %s", res.Error)
	}
	if got, _ := pub.find("sandbox.verify.verify_outcome"); got != "container_missing" {
		t.Errorf("verify_outcome = %v, want container_missing", got)
	}
}

func TestExecutor_OKRequiresContainerName(t *testing.T) {
	e := NewExecutor(&fakePub{}, platform(), slog.Default())
	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "c3", Name: ToolName, LoopID: "verify-3",
		Arguments: map[string]any{
			"smoke_exit_code":        float64(0),
			"smoke_matches_expected": true,
			"verify_outcome":         "ok",
			// container_name missing
		},
	})
	if res.Error == "" {
		t.Errorf("expected error when container_name missing on ok outcome")
	}
}

func TestExecutor_SmokeFailedRequiresContainerName(t *testing.T) {
	e := NewExecutor(&fakePub{}, platform(), slog.Default())
	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "c4", Name: ToolName, LoopID: "verify-4",
		Arguments: map[string]any{
			"smoke_exit_code":        float64(1),
			"smoke_matches_expected": false,
			"verify_outcome":         "smoke_failed",
			// container_name missing
		},
	})
	if res.Error == "" {
		t.Errorf("expected error when container_name missing on smoke_failed")
	}
}

func TestExecutor_InvalidVerifyOutcome(t *testing.T) {
	e := NewExecutor(&fakePub{}, platform(), slog.Default())
	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "c5", Name: ToolName, LoopID: "verify-5",
		Arguments: map[string]any{
			"verify_outcome": "garbage",
		},
	})
	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Errorf("error kind = %q, want %q", res.ErrorKind, agentic.ToolErrorInvalidArgs)
	}
}

func TestExecutor_MissingLoopIDFails(t *testing.T) {
	e := NewExecutor(&fakePub{}, platform(), slog.Default())
	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "c6", Name: ToolName,
		Arguments: map[string]any{"verify_outcome": "ok", "container_name": "x"},
	})
	if res.Error == "" {
		t.Errorf("expected error when loop_id missing")
	}
}
