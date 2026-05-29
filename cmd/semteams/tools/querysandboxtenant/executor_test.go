package querysandboxtenant

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/c360studio/semstreams/agentic"

	"github.com/c360studio/semteams/cmd/semteams/sandboxfleet"
)

type fakeRegistry struct {
	rec *sandboxfleet.Record
	err error
}

func (f *fakeRegistry) Lookup(_ context.Context, sig sandboxfleet.TargetSignature) (*sandboxfleet.Record, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.rec == nil {
		return nil, nil
	}
	out := *f.rec
	out.Signature = sig.Hash()
	return &out, nil
}

func callArgs() map[string]any {
	return map[string]any{
		"command":    "task test:integration",
		"repo_url":   "https://github.com/c360studio/semteams",
		"repo_ref":   "main",
		"toolchain":  map[string]any{"go": "1.26.0"},
		"base_image": "ubuntu:24.04",
	}
}

func TestExecutor_Miss(t *testing.T) {
	e := NewExecutor(&fakeRegistry{}, slog.Default())
	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "c1", Name: ToolName, Arguments: callArgs(),
	})
	if res.Error != "" {
		t.Fatalf("unexpected error: %s", res.Error)
	}
	var body resultBody
	if err := json.Unmarshal([]byte(res.Content), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Hit {
		t.Errorf("expected hit=false on miss")
	}
	if body.Signature == "" {
		t.Errorf("signature should be set even on miss")
	}
}

func TestExecutor_Hit(t *testing.T) {
	now := time.Now().UTC()
	e := NewExecutor(&fakeRegistry{rec: &sandboxfleet.Record{
		State:         sandboxfleet.StateReadyRunning,
		ContainerName: "semteams-tenant-abc123",
		Image:         "ubuntu:24.04",
		Workspace:     "/workspace",
		ReadyAt:       now,
		LastUsed:      now,
		PlanHash:      "ph",
	}}, slog.Default())
	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "c2", Name: ToolName, Arguments: callArgs(),
	})
	if res.Error != "" {
		t.Fatalf("unexpected error: %s", res.Error)
	}
	var body resultBody
	if err := json.Unmarshal([]byte(res.Content), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.Hit {
		t.Errorf("expected hit=true")
	}
	if body.State != string(sandboxfleet.StateReadyRunning) {
		t.Errorf("state = %q", body.State)
	}
	if body.PlanHash != "ph" {
		t.Errorf("plan_hash = %q", body.PlanHash)
	}
	if body.ReadyAt == "" {
		t.Errorf("ready_at should be set on a populated record")
	}
}

func TestExecutor_CanonicalizationConvergence(t *testing.T) {
	// Two variants of the same target should produce the same signature
	// in the tool result. Pins the rule pack's cache-key invariant at
	// the tool layer.
	e := NewExecutor(&fakeRegistry{}, slog.Default())

	args1 := callArgs()
	res1, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "v1", Name: ToolName, Arguments: args1,
	})

	args2 := map[string]any{
		"command":    "  Task Test:Integration  ",
		"repo_url":   "git@github.com:c360studio/semteams.git",
		"repo_ref":   "main",
		"toolchain":  map[string]any{"Go": "v1.26"},
		"base_image": "ubuntu:24.04",
	}
	res2, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "v2", Name: ToolName, Arguments: args2,
	})

	var b1, b2 resultBody
	if err := json.Unmarshal([]byte(res1.Content), &b1); err != nil {
		t.Fatalf("decode v1: %v", err)
	}
	if err := json.Unmarshal([]byte(res2.Content), &b2); err != nil {
		t.Fatalf("decode v2: %v", err)
	}
	if b1.Signature != b2.Signature {
		t.Errorf("variants should share signature; v1=%s v2=%s", b1.Signature, b2.Signature)
	}
}

func TestExecutor_CanonicalizeError(t *testing.T) {
	e := NewExecutor(&fakeRegistry{}, slog.Default())
	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "bad", Name: ToolName, Arguments: map[string]any{
			"command":    "",
			"base_image": "ubuntu:24.04",
		},
	})
	if res.Error == "" {
		t.Errorf("expected error on empty command")
	}
	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Errorf("error kind = %q, want %q", res.ErrorKind, agentic.ToolErrorInvalidArgs)
	}
}

func TestExecutor_RegistryErrorIsNetwork(t *testing.T) {
	e := NewExecutor(&fakeRegistry{err: errors.New("nats timeout")}, slog.Default())
	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "n", Name: ToolName, Arguments: callArgs(),
	})
	if res.Error == "" {
		t.Errorf("expected propagated error")
	}
	if res.ErrorKind != agentic.ToolErrorNetwork {
		t.Errorf("error kind = %q, want %q", res.ErrorKind, agentic.ToolErrorNetwork)
	}
}

func TestExecutor_UnknownTool(t *testing.T) {
	e := NewExecutor(&fakeRegistry{}, slog.Default())
	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "u", Name: "not_my_tool", Arguments: callArgs(),
	})
	if res.ErrorKind != agentic.ToolErrorNotFound {
		t.Errorf("error kind = %q, want %q", res.ErrorKind, agentic.ToolErrorNotFound)
	}
}

func TestExecutor_ListToolsSchema(t *testing.T) {
	e := NewExecutor(&fakeRegistry{}, slog.Default())
	defs := e.ListTools()
	if len(defs) != 1 {
		t.Fatalf("expected 1 tool def, got %d", len(defs))
	}
	if defs[0].Name != ToolName {
		t.Errorf("def name = %q, want %q", defs[0].Name, ToolName)
	}
	params := defs[0].Parameters
	if params == nil {
		t.Fatalf("parameters nil")
	}
	req, _ := params["required"].([]string)
	if len(req) == 0 {
		t.Errorf("required fields not declared")
	}
}
