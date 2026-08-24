package implementspec

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	agenticdispatch "github.com/c360studio/semstreams/processor/agentic-dispatch"
	"github.com/c360studio/semstreams/types"
)

var fixedNow = time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)

func TestExecute_AnalyzesProofAndRequestsImplementationWhenReady(t *testing.T) {
	runEntityID := "c360.semteams.agent.chain.execution.loop_root"
	reader := &recordingReader{
		entities: map[string]map[string]any{
			runEntityID: readyHealthEndpointTriples(),
		},
	}
	publisher := &recordingPublisher{}
	cmd := testCommand(reader, publisher)

	resp, err := cmd.Execute(context.Background(), testCommandContext(), agentic.UserMessage{
		MessageID:   "msg-1",
		ChannelType: "web",
		ChannelID:   "ui",
		UserID:      "alice",
		Content:     "/implement-spec add-health-endpoint",
		RunID:       "loop_root",
	}, []string{"add-health-endpoint", ""}, "")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(resp.Content, "implementation handoff requested") {
		t.Fatalf("response content = %q, want implementation handoff confirmation", resp.Content)
	}
	if reader.readEntityID != runEntityID {
		t.Fatalf("read entity = %q, want %q", reader.readEntityID, runEntityID)
	}

	got := triplesByPredicate(publisher.triples)
	if got["formal_claims.status"] != "passed" {
		t.Fatalf("formal_claims.status = %q, want passed; triples=%#v", got["formal_claims.status"], got)
	}
	if got["formal_claims.route.implementation"] != "present" {
		t.Fatalf("formal_claims.route.implementation = %q, want present", got["formal_claims.route.implementation"])
	}
	if got["formal_claims.analyzer.version"] != "go-native-v1" {
		t.Fatalf("formal_claims.analyzer.version = %q, want go-native-v1", got["formal_claims.analyzer.version"])
	}
	if got["dev_from_task.requested"] != "implement-spec:msg-1" {
		t.Fatalf("dev_from_task.requested = %q, want implement-spec:msg-1", got["dev_from_task.requested"])
	}
	if got["dev_from_task.requested_change_slug"] != "add-health-endpoint" {
		t.Fatalf("requested_change_slug = %q, want add-health-endpoint", got["dev_from_task.requested_change_slug"])
	}
	if _, ok := got["proof_readiness.implementation_ready"]; ok {
		t.Fatalf("command stamped proof_readiness; proof-readiness rule must own that projection")
	}
}

func TestExecute_BlockedProofDoesNotRequestImplementation(t *testing.T) {
	runEntityID := "c360.semteams.agent.chain.execution.loop_root"
	reader := &recordingReader{
		entities: map[string]map[string]any{
			runEntityID: {
				"change.add-health-endpoint.proposal.intent": "Expose a health check.",
			},
		},
	}
	publisher := &recordingPublisher{}
	cmd := testCommand(reader, publisher)

	resp, err := cmd.Execute(context.Background(), testCommandContext(), agentic.UserMessage{
		MessageID:   "msg-2",
		ChannelType: "web",
		ChannelID:   "ui",
		UserID:      "alice",
		Content:     "/implement-spec add-health-endpoint",
		RunID:       "loop_root",
	}, []string{"add-health-endpoint", ""}, "")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(resp.Content, "not implementation-ready") {
		t.Fatalf("response content = %q, want blocked proof message", resp.Content)
	}

	got := triplesByPredicate(publisher.triples)
	if got["formal_claims.status"] != "ambiguous" {
		t.Fatalf("formal_claims.status = %q, want ambiguous", got["formal_claims.status"])
	}
	if got["formal_claims.route.coordinator"] != "present" {
		t.Fatalf("formal_claims.route.coordinator = %q, want present", got["formal_claims.route.coordinator"])
	}
	if _, ok := got["dev_from_task.requested"]; ok {
		t.Fatalf("blocked proof stamped dev_from_task.requested; implementation must stay gated")
	}
}

func TestExecute_RejectsSlugThatDoesNotExistOnRun(t *testing.T) {
	runEntityID := "c360.semteams.agent.chain.execution.loop_root"
	reader := &recordingReader{
		entities: map[string]map[string]any{
			runEntityID: readyHealthEndpointTriples(),
		},
	}
	publisher := &recordingPublisher{}
	cmd := testCommand(reader, publisher)

	_, err := cmd.Execute(context.Background(), testCommandContext(), agentic.UserMessage{
		MessageID:   "msg-3",
		ChannelType: "web",
		ChannelID:   "ui",
		UserID:      "alice",
		Content:     "/implement-spec wrong-change",
		RunID:       "loop_root",
	}, []string{"wrong-change", ""}, "")
	if err == nil || !strings.Contains(err.Error(), `no OpenSpec change "wrong-change"`) {
		t.Fatalf("Execute error = %v, want missing change guard", err)
	}
	if len(publisher.triples) != 0 {
		t.Fatalf("publisher wrote %d triples despite invalid slug", len(publisher.triples))
	}
}

func TestExecute_RejectsRunIDArgument(t *testing.T) {
	runEntityID := "c360.semteams.agent.chain.execution.loop_root"
	reader := &recordingReader{
		entities: map[string]map[string]any{
			runEntityID: readyHealthEndpointTriples(),
		},
	}
	publisher := &recordingPublisher{}
	cmd := testCommand(reader, publisher)

	_, err := cmd.Execute(context.Background(), testCommandContext(), agentic.UserMessage{
		MessageID:   "msg-4",
		ChannelType: "web",
		ChannelID:   "ui",
		UserID:      "alice",
		Content:     "/implement-spec c360.semteams.agent.chain.execution.other",
		RunID:       "loop_root",
	}, []string{"c360.semteams.agent.chain.execution.other", ""}, "")
	if err == nil || !strings.Contains(err.Error(), "not a run id") {
		t.Fatalf("Execute error = %v, want run-id argument rejection", err)
	}
	if reader.readEntityID != "" {
		t.Fatalf("reader read %q despite invalid run-id argument", reader.readEntityID)
	}
	if len(publisher.triples) != 0 {
		t.Fatalf("publisher wrote %d triples despite invalid run-id argument", len(publisher.triples))
	}
}

func TestExecute_RejectsUntrackedSelectedRun(t *testing.T) {
	reader := &recordingReader{}
	publisher := &recordingPublisher{}
	cmd := testCommand(reader, publisher)

	_, err := cmd.Execute(context.Background(), testCommandContext(), agentic.UserMessage{
		MessageID:   "msg-5",
		ChannelType: "web",
		ChannelID:   "ui",
		UserID:      "alice",
		Content:     "/implement-spec add-health-endpoint",
		RunID:       "other-run",
	}, []string{"add-health-endpoint", ""}, "")
	if err == nil || !strings.Contains(err.Error(), `selected run "other-run" is not tracked`) {
		t.Fatalf("Execute error = %v, want untracked selected-run rejection", err)
	}
	if len(publisher.triples) != 0 {
		t.Fatalf("publisher wrote %d triples despite untracked selected run", len(publisher.triples))
	}
}

func TestExecute_RejectsSelectedRunOwnedByDifferentUser(t *testing.T) {
	reader := &recordingReader{}
	publisher := &recordingPublisher{}
	cmd := testCommand(reader, publisher)
	cmdCtx := testCommandContext()
	cmdCtx.LoopTracker.Track(&agenticdispatch.LoopInfo{
		LoopID:      "bob-run",
		UserID:      "bob",
		ChannelType: "web",
		ChannelID:   "ui",
		State:       "complete",
	})

	_, err := cmd.Execute(context.Background(), cmdCtx, agentic.UserMessage{
		MessageID:   "msg-6",
		ChannelType: "web",
		ChannelID:   "ui",
		UserID:      "alice",
		Content:     "/implement-spec add-health-endpoint",
		RunID:       "bob-run",
	}, []string{"add-health-endpoint", ""}, "")
	if err == nil || !strings.Contains(err.Error(), `permission denied for selected run "bob-run"`) {
		t.Fatalf("Execute error = %v, want ownership rejection", err)
	}
	if len(publisher.triples) != 0 {
		t.Fatalf("publisher wrote %d triples despite foreign selected run", len(publisher.triples))
	}
}

func TestExecute_RejectsMissingApprovePermission(t *testing.T) {
	reader := &recordingReader{}
	publisher := &recordingPublisher{}
	cmd := testCommand(reader, publisher)
	cmdCtx := testCommandContext()
	cmdCtx.HasPermission = func(_, _ string) bool { return false }

	_, err := cmd.Execute(context.Background(), cmdCtx, agentic.UserMessage{
		MessageID:   "msg-7",
		ChannelType: "web",
		ChannelID:   "ui",
		UserID:      "alice",
		Content:     "/implement-spec add-health-endpoint",
		RunID:       "loop_root",
	}, []string{"add-health-endpoint", ""}, "")
	if err == nil || !strings.Contains(err.Error(), `requires "approve"`) {
		t.Fatalf("Execute error = %v, want approve permission rejection", err)
	}
	if reader.readEntityID != "" {
		t.Fatalf("reader read %q despite permission denial", reader.readEntityID)
	}
	if len(publisher.triples) != 0 {
		t.Fatalf("publisher wrote %d triples despite permission denial", len(publisher.triples))
	}
}

func TestConfig_RequiresApprovePermission(t *testing.T) {
	cfg := NewCommand(types.PlatformMeta{Org: "c360", Platform: "semteams"}, nil).Config()
	if cfg.Permission != "approve" {
		t.Fatalf("permission = %q, want approve", cfg.Permission)
	}
	if !strings.Contains(cfg.Pattern, "implement-spec") {
		t.Fatalf("pattern = %q, want implement-spec command", cfg.Pattern)
	}
}

func testCommand(reader *recordingReader, publisher *recordingPublisher) *Command {
	return NewCommand(
		types.PlatformMeta{Org: "c360", Platform: "semteams"},
		nil,
		WithNow(func() time.Time { return fixedNow }),
		WithDependenciesFactory(func(_ *natsclient.Client) (Dependencies, error) {
			return Dependencies{Reader: reader, Publisher: publisher}, nil
		}),
	)
}

func testCommandContext() *agenticdispatch.CommandContext {
	tracker := agenticdispatch.NewLoopTracker()
	tracker.Track(&agenticdispatch.LoopInfo{
		LoopID:      "loop_root",
		UserID:      "alice",
		ChannelType: "web",
		ChannelID:   "ui",
		State:       "complete",
	})
	return &agenticdispatch.CommandContext{
		LoopTracker:   tracker,
		HasPermission: func(_, _ string) bool { return true },
	}
}

func readyHealthEndpointTriples() map[string]any {
	return map[string]any{
		"change.add-health-endpoint.proposal.intent": "Expose a health check.",

		"proof.claim.health.endpoint.status":    "accepted",
		"proof.claim.health.endpoint.statement": "GET /health returns 200 with status ok.",
		"proof.claim.health.endpoint.requires":  []any{"go_health_endpoint_test"},
		"proof.claim.health.endpoint.task_refs": []any{"change.add-health-endpoint.task.0"},

		"proof.dependency.go_health_endpoint_test.kind":         "test",
		"proof.dependency.go_health_endpoint_test.description":  "Focused Go test proves the health endpoint contract.",
		"proof.dependency.go_health_endpoint_test.required_for": []any{"health.endpoint"},
		"proof.dependency.go_health_endpoint_test.status":       "ready",
		"proof.dependency.go_health_endpoint_test.profile_ref":  "go.unit-test@v1",

		"proof.harness_profile.go.unit-test@v1.status":           "ready",
		"proof.harness_profile.go.unit-test@v1.version":          "v1",
		"proof.harness_profile.go.unit-test@v1.team":             "local-go",
		"proof.harness_profile.go.unit-test@v1.claims_supported": []any{"health.endpoint"},
		"proof.harness_profile.go.unit-test@v1.dependencies":     []any{"go_health_endpoint_test"},
		"proof.harness_profile.go.unit-test@v1.readiness_probes": []any{"health_smoke"},
		"proof.harness_profile.go.unit-test@v1.smoke_command":    "go test ./internal/health",
		"proof.harness_profile.go.unit-test@v1.artifacts":        []any{"go-test.log"},
		"proof.harness_profile.go.unit-test@v1.renderer":         "local",
		"proof.harness_profile.go.unit-test@v1.ttl_seconds":      "3600",

		"proof.readiness.health_smoke.profile_ref":  "go.unit-test@v1",
		"proof.readiness.health_smoke.status":       "passed",
		"proof.readiness.health_smoke.smoke_status": "passed",
		"proof.readiness.health_smoke.expires_at":   fixedNow.Add(time.Hour).Format(time.RFC3339Nano),
		"proof.readiness.health_smoke.evidence":     []any{"health_endpoint_test_log"},

		"proof.evidence.health_endpoint_test_log.kind":     "test",
		"proof.evidence.health_endpoint_test_log.producer": "e2e",
		"proof.evidence.health_endpoint_test_log.covers":   []any{"health.endpoint"},
	}
}

type recordingReader struct {
	readEntityID string
	entities     map[string]map[string]any
}

func (r *recordingReader) ReadEntity(_ context.Context, entityID string) (map[string]any, error) {
	r.readEntityID = entityID
	return r.entities[entityID], nil
}

type recordingPublisher struct {
	triples []message.Triple
}

func (p *recordingPublisher) Append(_ context.Context, triples []message.Triple) error {
	p.triples = append(p.triples, triples...)
	return nil
}

func triplesByPredicate(triples []message.Triple) map[string]string {
	out := map[string]string{}
	for _, tr := range triples {
		out[tr.Predicate] = fmt.Sprint(tr.Object)
	}
	return out
}
