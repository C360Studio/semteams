package emitbootstrapcommitted

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

type fakeRegistry struct {
	calls []registryCall
	err   error
}

type registryCall struct {
	sigHash, containerName, image, workspace, planHash string
}

func (f *fakeRegistry) CommitByHash(_ context.Context, sigHash, containerName, image, workspace, planHash string) error {
	if f.err != nil {
		return f.err
	}
	f.calls = append(f.calls, registryCall{
		sigHash: sigHash, containerName: containerName, image: image, workspace: workspace, planHash: planHash,
	})
	return nil
}

func platform() types.PlatformMeta {
	return types.PlatformMeta{Org: "c360", Platform: "ops"}
}

func baseArgs() map[string]any {
	return map[string]any{
		"signature":      "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		"container_name": "semteams-tenant-e3b0c4429",
		"image":          "ubuntu:24.04",
		"workspace":      "/workspace",
		"plan_hash":      "abcd1234",
	}
}

func TestExecutor_Happy(t *testing.T) {
	pub := &fakePub{}
	reg := &fakeRegistry{}
	e := NewExecutor(pub, reg, platform(), slog.Default())
	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "c1", Name: ToolName, LoopID: "rev-1", Arguments: baseArgs(),
	})
	if res.Error != "" {
		t.Fatalf("unexpected error: %s", res.Error)
	}

	// Triples stamped on reviewer loop entity.
	wantSubject := "c360.ops.agent.agentic-loop.execution.rev-1"
	for _, tr := range pub.triples {
		if tr.Subject != wantSubject {
			t.Errorf("subject = %q, want %q", tr.Subject, wantSubject)
		}
	}
	mustHave := []string{
		"sandbox.tenant.signature",
		"sandbox.tenant.container_name",
		"sandbox.tenant.image",
		"sandbox.tenant.workspace",
		"sandbox.tenant.plan_hash",
		"sandbox.tenant.committed_at",
	}
	for _, p := range mustHave {
		if _, ok := pub.find(p); !ok {
			t.Errorf("missing predicate %q", p)
		}
	}

	// Registry committed.
	if len(reg.calls) != 1 {
		t.Fatalf("expected 1 CommitByHash, got %d", len(reg.calls))
	}
	if reg.calls[0].sigHash != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Errorf("registry sig = %q", reg.calls[0].sigHash)
	}
	if reg.calls[0].containerName != "semteams-tenant-e3b0c4429" {
		t.Errorf("registry container_name = %q", reg.calls[0].containerName)
	}
}

func TestExecutor_RequiredFieldsMissing(t *testing.T) {
	e := NewExecutor(&fakePub{}, &fakeRegistry{}, platform(), slog.Default())
	cases := []string{"signature", "container_name", "image", "workspace", "plan_hash"}
	for _, missing := range cases {
		t.Run(missing, func(t *testing.T) {
			args := baseArgs()
			delete(args, missing)
			res, _ := e.Execute(context.Background(), agentic.ToolCall{
				ID: "c", Name: ToolName, LoopID: "rev-x", Arguments: args,
			})
			if res.Error == "" {
				t.Errorf("expected error when %q missing", missing)
			}
		})
	}
}

func TestExecutor_RegistryErrorIsNetwork(t *testing.T) {
	pub := &fakePub{}
	reg := &fakeRegistry{err: errors.New("nats")}
	e := NewExecutor(pub, reg, platform(), slog.Default())
	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "c2", Name: ToolName, LoopID: "rev-2", Arguments: baseArgs(),
	})
	if res.ErrorKind != agentic.ToolErrorNetwork {
		t.Errorf("error kind = %q, want %q", res.ErrorKind, agentic.ToolErrorNetwork)
	}
}

func TestExecutor_MissingLoopIDFails(t *testing.T) {
	e := NewExecutor(&fakePub{}, &fakeRegistry{}, platform(), slog.Default())
	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "c3", Name: ToolName, Arguments: baseArgs(),
	})
	if res.Error == "" {
		t.Errorf("expected error when loop_id missing")
	}
}
