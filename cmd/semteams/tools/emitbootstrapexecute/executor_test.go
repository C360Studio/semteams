package emitbootstrapexecute

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

const executeLoopID = "execute-loop-A"

func validArgs() map[string]any {
	return map[string]any{
		"signature":                "0fa875ab980df345ea62bbb0fed8581c246d91b5da5825a2c56c2561a3673cb5",
		"container_name":           "semteams-tenant-0fa875ab980d",
		"image":                    "golang:1.25-bookworm",
		"workspace":                "/workspace",
		"plan_hash":                "3fb5a8364d841b397b364def584099e83a9a25f7ac9156dece0712cf11a7d281",
		"plan_action":              "provision",
		"verify_command":           "go version",
		"expected_smoke_signature": "exit_code=0; stdout_contains=go version go1.25",
	}
}

func TestExecutor_HappyPath_StampsTenantTriplesOnExecuteLoop(t *testing.T) {
	pub := &fakePub{}
	e := NewExecutor(pub, platform(), slog.Default())
	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "c1", Name: ToolName, LoopID: executeLoopID,
		Arguments: validArgs(),
	})
	if res.Error != "" {
		t.Fatalf("unexpected error: %s", res.Error)
	}

	// Subject must be the calling EXECUTE loop entity — that's the
	// load-bearing claim of this tool. Any stamp landing elsewhere
	// defeats the rule 03 substitution fix.
	wantSubject := "c360.ops.agent.agentic-loop.execution.execute-loop-A"
	for _, tr := range pub.triples {
		if tr.Subject != wantSubject {
			t.Errorf("triple %s subject = %q, want %q", tr.Predicate, tr.Subject, wantSubject)
		}
	}

	// Predicates rule 03 substitutes into verify's spawn prompt MUST
	// be present. Missing any of these reverts the smoke #9 wedge.
	mustPresent := []string{
		"sandbox.tenant.signature",
		"sandbox.tenant.container_name",
		"sandbox.tenant.image",
		"sandbox.tenant.workspace",
		"sandbox.tenant.plan_hash",
		"sandbox.tenant.verify_command",
		"sandbox.tenant.expected_smoke_signature",
		"sandbox.tenant.execute_forwarded_at",
		"sandbox.tenant.execute_forwarded_source",
	}
	for _, p := range mustPresent {
		if _, ok := pub.find(p); !ok {
			t.Errorf("predicate %q missing from stamped triples", p)
		}
	}

	// plan_action is optional in args but should pass through when
	// supplied (audit trail for "is this the skip or provision path
	// retrospectively?").
	if got, _ := pub.find("sandbox.tenant.plan_action"); got != "provision" {
		t.Errorf("plan_action = %v, want provision", got)
	}

	// Verify the smoke-contract values flow through verbatim — these
	// are the ones rule 03 substitutes into verify's prompt.
	if got, _ := pub.find("sandbox.tenant.verify_command"); got != "go version" {
		t.Errorf("verify_command = %v, want 'go version'", got)
	}
	if got, _ := pub.find("sandbox.tenant.container_name"); got != "semteams-tenant-0fa875ab980d" {
		t.Errorf("container_name = %v, want semteams-tenant-0fa875ab980d", got)
	}
}

func TestExecutor_MissingLoopIDFails(t *testing.T) {
	e := NewExecutor(&fakePub{}, platform(), slog.Default())
	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "c2", Name: ToolName,
		Arguments: validArgs(),
	})
	if res.Error == "" {
		t.Errorf("expected error when loop_id missing")
	}
	if res.ErrorKind != agentic.ToolErrorInternal {
		t.Errorf("error kind = %q, want %q", res.ErrorKind, agentic.ToolErrorInternal)
	}
}

func TestExecutor_MissingRequiredArgsFail(t *testing.T) {
	required := []string{
		"signature",
		"container_name",
		"image",
		"workspace",
		"plan_hash",
		"verify_command",
		"expected_smoke_signature",
	}
	for _, key := range required {
		t.Run("missing_"+key, func(t *testing.T) {
			args := validArgs()
			delete(args, key)
			e := NewExecutor(&fakePub{}, platform(), slog.Default())
			res, _ := e.Execute(context.Background(), agentic.ToolCall{
				ID: "c", Name: ToolName, LoopID: executeLoopID,
				Arguments: args,
			})
			if res.Error == "" {
				t.Errorf("expected error when %s missing", key)
			}
			if res.ErrorKind != agentic.ToolErrorInvalidArgs {
				t.Errorf("error kind = %q, want %q", res.ErrorKind, agentic.ToolErrorInvalidArgs)
			}
		})
	}
}

func TestExecutor_PlanActionOptional(t *testing.T) {
	pub := &fakePub{}
	e := NewExecutor(pub, platform(), slog.Default())
	args := validArgs()
	delete(args, "plan_action")
	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "c3", Name: ToolName, LoopID: executeLoopID,
		Arguments: args,
	})
	if res.Error != "" {
		t.Fatalf("unexpected error: %s", res.Error)
	}
	// plan_action should be absent — it's optional and omit-when-empty.
	if _, ok := pub.find("sandbox.tenant.plan_action"); ok {
		t.Errorf("plan_action present in triples but was not supplied")
	}
}

func TestExecutor_UnknownToolName(t *testing.T) {
	e := NewExecutor(&fakePub{}, platform(), slog.Default())
	res, _ := e.Execute(context.Background(), agentic.ToolCall{
		ID: "c4", Name: "garbage", LoopID: executeLoopID,
		Arguments: validArgs(),
	})
	if res.ErrorKind != agentic.ToolErrorNotFound {
		t.Errorf("error kind = %q, want %q", res.ErrorKind, agentic.ToolErrorNotFound)
	}
}

func TestNewExecutor_PanicsWithoutPublisher(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic when publisher nil")
		}
	}()
	NewExecutor(nil, platform(), slog.Default())
}

func TestNewExecutor_PanicsWithoutPlatform(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic when platform empty")
		}
	}()
	NewExecutor(&fakePub{}, types.PlatformMeta{}, slog.Default())
}
