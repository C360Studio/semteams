package chain

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/types"
)

// fakeIDResolver lets ResolveChainEntityID tests script ChainID
// returns without booting NATS. Mirrors the chain.Resolver method
// signature.
type fakeIDResolver struct {
	chainID string
	err     error
	calls   int
}

func (f *fakeIDResolver) ChainID(_ context.Context, _ string) (string, error) {
	f.calls++
	return f.chainID, f.err
}

const entityResolverTestPlatformOrg = "c360"
const entityResolverTestPlatformPlatform = "platform1"

func entityResolverTestPlatform() types.PlatformMeta {
	return types.PlatformMeta{Org: entityResolverTestPlatformOrg, Platform: entityResolverTestPlatformPlatform}
}

func TestResolveChainEntityID_RelatedLoopsPriority(t *testing.T) {
	// When related_loops has the key, the resolver is never consulted.
	// This is the rule-pack happy path — wins over fallback.
	resolver := &fakeIDResolver{chainID: "should-not-be-used"}
	call := agentic.ToolCall{
		LoopID: "loop_xyz",
		Metadata: map[string]any{
			agentic.MetadataKeyRelatedLoops: map[string]any{
				ChainEntityRoleKey: "c360.platform1.agent.chain.execution.pinned-id",
			},
		},
	}
	got, err := ResolveChainEntityID(context.Background(), call, resolver, entityResolverTestPlatform(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != "c360.platform1.agent.chain.execution.pinned-id" {
		t.Fatalf("did not honor related_loops pin: %q", got)
	}
	if resolver.calls != 0 {
		t.Fatalf("resolver called %d times when related_loops should have short-circuited", resolver.calls)
	}
}

func TestResolveChainEntityID_FallbackToResolver(t *testing.T) {
	// related_loops missing → walk the resolver. Constructed entity ID
	// must match the {org}.{platform}.agent.chain.execution.{chainID}
	// shape downstream consumers (ReadEntity, AttestationRunner) expect.
	resolver := &fakeIDResolver{chainID: "chainABC"}
	call := agentic.ToolCall{LoopID: "loop_xyz"}
	got, err := ResolveChainEntityID(context.Background(), call, resolver, entityResolverTestPlatform(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := "c360.platform1.agent.chain.execution.chainABC"
	if got != want {
		t.Fatalf("constructed entity id wrong: got %q want %q", got, want)
	}
	if resolver.calls != 1 {
		t.Fatalf("resolver expected 1 call, got %d", resolver.calls)
	}
}

func TestResolveChainEntityID_NoResolverNoRelated_Errors(t *testing.T) {
	// Tool was wired without a resolver (config gap) AND the rule pack
	// didn't pin related_loops. Either is fixable independently; both
	// together is a hard stop.
	call := agentic.ToolCall{LoopID: "loop_xyz"}
	_, err := ResolveChainEntityID(context.Background(), call, nil, entityResolverTestPlatform(), nil)
	if err == nil {
		t.Fatalf("expected error when resolver and related_loops both unavailable")
	}
	if !strings.Contains(err.Error(), ChainEntityRoleKey) {
		t.Fatalf("error does not name the missing related_loops key: %v", err)
	}
}

func TestResolveChainEntityID_NoLoopIDNoRelated_Errors(t *testing.T) {
	// Resolver wired but tool call carries no loop_id. The fallback
	// path needs loop_id to walk; surface the gap rather than calling
	// resolver with "".
	resolver := &fakeIDResolver{chainID: "should-not-be-called"}
	call := agentic.ToolCall{}
	_, err := ResolveChainEntityID(context.Background(), call, resolver, entityResolverTestPlatform(), nil)
	if err == nil {
		t.Fatalf("expected error when loop_id unavailable")
	}
	if !strings.Contains(err.Error(), "no loop_id") {
		t.Fatalf("error does not name the missing loop_id: %v", err)
	}
	if resolver.calls != 0 {
		t.Fatalf("resolver called %d times despite missing loop_id", resolver.calls)
	}
}

func TestResolveChainEntityID_ResolverError_Wrapped(t *testing.T) {
	// resolver errors should propagate with %w so callers can errors.Is
	// them if needed. Common case: NATS request timed out, parent
	// triple not yet stamped.
	sentinel := errors.New("nats: connection lost")
	resolver := &fakeIDResolver{err: sentinel}
	call := agentic.ToolCall{LoopID: "loop_xyz"}
	_, err := ResolveChainEntityID(context.Background(), call, resolver, entityResolverTestPlatform(), nil)
	if err == nil {
		t.Fatalf("expected error to propagate")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("error did not wrap sentinel: %v", err)
	}
}

func TestResolveChainEntityID_EmptyChainID_Errors(t *testing.T) {
	// Resolver returned "" — defensive: don't construct
	// `...chain.execution.` (trailing dot, would fail IsValidEntityID
	// anyway, but the error there is less actionable).
	resolver := &fakeIDResolver{chainID: ""}
	call := agentic.ToolCall{LoopID: "loop_xyz"}
	_, err := ResolveChainEntityID(context.Background(), call, resolver, entityResolverTestPlatform(), nil)
	if err == nil {
		t.Fatalf("expected error on empty chain id")
	}
	if !strings.Contains(err.Error(), "empty chain id") {
		t.Fatalf("error does not name the empty chain id condition: %v", err)
	}
}

func TestResolveChainEntityID_InvalidEntityID_Errors(t *testing.T) {
	// Platform with empty fields produces an entity ID with leading
	// dots that fails IsValidEntityID. Surface explicitly so a
	// misconfigured platform crashes the call rather than stamping
	// triples on a malformed subject (silent data corruption).
	resolver := &fakeIDResolver{chainID: "chainABC"}
	call := agentic.ToolCall{LoopID: "loop_xyz"}
	_, err := ResolveChainEntityID(context.Background(), call, resolver, types.PlatformMeta{Org: "", Platform: ""}, nil)
	if err == nil {
		t.Fatalf("expected error on invalid entity id from empty platform")
	}
	if !strings.Contains(err.Error(), "IsValidEntityID") {
		t.Fatalf("error does not name the validity check: %v", err)
	}
}

func TestResolveChainEntityID_LoggerOptionalNoNilPanic(t *testing.T) {
	// Passing logger=nil is the contract for callers that don't care
	// about the fallback-warn (tests, ephemeral CLIs). Must not panic.
	resolver := &fakeIDResolver{chainID: "chainABC"}
	call := agentic.ToolCall{LoopID: "loop_xyz"}
	if _, err := ResolveChainEntityID(context.Background(), call, resolver, entityResolverTestPlatform(), nil); err != nil {
		t.Fatalf("unexpected error with nil logger: %v", err)
	}
}

func TestResolveChainEntityID_LoggerWarnFiresOnFallback(t *testing.T) {
	// When fallback path is taken AND a logger is provided, the warn
	// must fire — that's the only signal operators get that a
	// related_loops key is missing or typoed at the rule-pack layer.
	resolver := &fakeIDResolver{chainID: "chainABC"}
	call := agentic.ToolCall{LoopID: "loop_xyz"}

	var buf strings.Builder
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	if _, err := ResolveChainEntityID(context.Background(), call, resolver, entityResolverTestPlatform(), logger); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !strings.Contains(buf.String(), "related_loops chain-entity-id missing") {
		t.Fatalf("expected fallback-warn in log output; got: %q", buf.String())
	}
}
