package projectspecplan

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/types"
)

func platform() types.PlatformMeta { return types.PlatformMeta{Org: "c360", Platform: "ops"} }

const testRunEntity = "c360.ops.agent.chain.execution.run-123"

var fixedNow = time.Date(2026, 6, 24, 15, 0, 0, 0, time.UTC)

type fakeReader struct {
	id      string
	triples map[string]any
	err     error
}

func (f fakeReader) ReadEntity(_ context.Context, id string) (map[string]any, error) {
	if f.err != nil {
		return nil, f.err
	}
	if id != f.id {
		return map[string]any{}, nil
	}
	return f.triples, nil
}

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

func (f *fakePub) byPredicate() map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	m := make(map[string]any, len(f.triples))
	for _, tr := range f.triples {
		m[tr.Predicate] = tr.Object
	}
	return m
}

func callWith(args map[string]any) agentic.ToolCall {
	return agentic.ToolCall{
		ID:        "call-1",
		Name:      ToolName,
		Arguments: args,
		Metadata: map[string]any{
			agentic.MetadataKeyRunEntityID: testRunEntity,
		},
	}
}

func approvedChangeTriples() map[string]any {
	return map[string]any{
		"agent.run.outcome": "success",

		"change.add-mfa.status":             "draft",
		"change.add-mfa.acceptance_command": "go test ./...",
		"change.add-mfa.proposal.intent":    "Mitigate password-only compromise.",
		"change.add-mfa.proposal.scope_out": `["WebAuthn"]`,

		"change.add-mfa.task.0.text":             "Generate TOTP secrets",
		"change.add-mfa.task.0.goal":             "Persist per-user TOTP secrets.",
		"change.add-mfa.task.0.target_files":     `["auth/totp.go"]`,
		"change.add-mfa.task.0.test_command":     "go test ./auth -run TestTOTPSecret",
		"change.add-mfa.task.0.assumptions":      `["users already exist"]`,
		"change.add-mfa.task.0.non_goals":        `[]`,
		"change.add-mfa.task.0.expected_outcome": "TOTP secrets are stored and retrievable.",

		"change.add-mfa.task.1.text":         "Challenge on login",
		"change.add-mfa.task.1.goal":         "Require TOTP after password verification.",
		"change.add-mfa.task.1.target_files": `["auth/login.go","auth/login_test.go"]`,
		"change.add-mfa.task.1.test_command": "go test ./auth -run TestLoginRequiresTOTP",
		"change.add-mfa.task.1.assumptions":  `[]`,
		"change.add-mfa.task.1.non_goals":    `["WebAuthn"]`,
	}
}

func TestExecute_ProjectsApprovedChangeTasksToPlan(t *testing.T) {
	pub := &fakePub{}
	ex := NewExecutor(
		fakeReader{id: testRunEntity, triples: approvedChangeTriples()},
		pub,
		platform(),
		nil,
	)
	ex.now = func() time.Time { return fixedNow }

	res, err := ex.Execute(context.Background(), callWith(map[string]any{"slug": "add-mfa"}))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("unexpected tool error: %s", res.Error)
	}

	var body struct {
		Slug                   string   `json:"slug"`
		TaskCount              int      `json:"task_count"`
		TaskIDs                []string `json:"task_ids"`
		IntegrationTestCommand string   `json:"integration_test_command"`
		ChainStartGitTag       string   `json:"chain_start_git_tag"`
	}
	if err := json.Unmarshal([]byte(res.Content), &body); err != nil {
		t.Fatalf("unmarshal content: %v\n%s", err, res.Content)
	}
	if body.Slug != "add-mfa" || body.TaskCount != 2 || strings.Join(body.TaskIDs, ",") != "task-0,task-1" {
		t.Fatalf("content = %+v, want add-mfa with task-0/task-1", body)
	}
	if body.IntegrationTestCommand != "go test ./..." || body.ChainStartGitTag != "plan-start" {
		t.Fatalf("content integration fields = %+v", body)
	}

	for _, tr := range pub.triples {
		if tr.Subject != testRunEntity {
			t.Fatalf("triple %q stamped on %q, want %q", tr.Predicate, tr.Subject, testRunEntity)
		}
	}

	m := pub.byPredicate()
	wantStrings := map[string]string{
		"plan.goal":                           "Mitigate password-only compromise.",
		"plan.assumptions":                    `[]`,
		"plan.non_goals":                      `["WebAuthn"]`,
		"plan.integration_test_command":       "go test ./...",
		"plan.chain_start_git_tag":            "plan-start",
		"plan.source_change_slug":             "add-mfa",
		"plan.done_authority.policy":          "approved_openspec_change",
		"plan.done_authority.source_change":   "change.add-mfa",
		"plan.done_authority.final_gate":      "reviewer-dev-via-test",
		"dev_via_test.run.status":             "active",
		"plan.task.task-0.goal":               "Persist per-user TOTP secrets.",
		"plan.task.task-0.assumptions":        `["users already exist"]`,
		"plan.task.task-0.non_goals":          `[]`,
		"plan.task.task-0.target_files":       `["auth/totp.go"]`,
		"plan.task.task-0.depends_on":         `[]`,
		"plan.task.task-0.test_command":       "go test ./auth -run TestTOTPSecret",
		"plan.task.task-0.status":             "ready",
		"plan.task.task-0.expected_outcome":   "TOTP secrets are stored and retrievable.",
		"plan.task.task-0.source_change_task": "change.add-mfa.task.0",
		"plan.task.task-1.goal":               "Require TOTP after password verification.",
		"plan.task.task-1.non_goals":          `["WebAuthn"]`,
		"plan.task.task-1.target_files":       `["auth/login.go","auth/login_test.go"]`,
		"plan.task.task-1.test_command":       "go test ./auth -run TestLoginRequiresTOTP",
		"plan.task.task-1.source_change_task": "change.add-mfa.task.1",
	}
	for pred, want := range wantStrings {
		if got, _ := m[pred].(string); got != want {
			t.Errorf("%s = %q, want %q", pred, got, want)
		}
	}
	if got, _ := m["plan.task_count"].(int); got != 2 {
		t.Errorf("plan.task_count = %v, want 2", m["plan.task_count"])
	}
	if got, _ := m["plan.task.task-0.position"].(int); got != 0 {
		t.Errorf("task-0 position = %v, want 0", m["plan.task.task-0.position"])
	}
	if got, _ := m["plan.task.task-1.position"].(int); got != 1 {
		t.Errorf("task-1 position = %v, want 1", m["plan.task.task-1.position"])
	}
	if got, _ := m["plan.generated_at"].(string); !strings.HasPrefix(got, "2026-06-24T15:00:00") {
		t.Errorf("plan.generated_at = %q, want fixed timestamp", got)
	}
}

func TestExecute_ProjectsFromRelatedRunEntityWhenRunAnchorMissing(t *testing.T) {
	pub := &fakePub{}
	ex := NewExecutor(
		fakeReader{id: testRunEntity, triples: approvedChangeTriples()},
		pub,
		platform(),
		nil,
	)
	ex.now = func() time.Time { return fixedNow }

	call := agentic.ToolCall{
		ID:        "call-1",
		Name:      ToolName,
		Arguments: map[string]any{"slug": "add-mfa"},
		Metadata: map[string]any{
			agentic.MetadataKeyRelatedLoops: map[string]any{
				"run-loop-entity-id": testRunEntity,
			},
		},
	}
	res, err := ex.Execute(context.Background(), call)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("unexpected tool error: %s", res.Error)
	}
	if got := res.Metadata["run_entity_id"]; got != testRunEntity {
		t.Fatalf("run_entity_id metadata = %v, want %s", got, testRunEntity)
	}
	for _, tr := range pub.triples {
		if tr.Subject != testRunEntity {
			t.Fatalf("triple %q stamped on %q, want %q", tr.Predicate, tr.Subject, testRunEntity)
		}
	}
}

func TestExecute_RequiresApprovedRun(t *testing.T) {
	tr := approvedChangeTriples()
	tr["agent.run.outcome"] = "executing"
	ex := NewExecutor(fakeReader{id: testRunEntity, triples: tr}, &fakePub{}, platform(), nil)

	res, err := ex.Execute(context.Background(), callWith(nil))
	if err != nil {
		t.Fatalf("Execute returned transport error: %v", err)
	}
	if res.ErrorKind != agentic.ToolErrorInvalidArgs || !strings.Contains(res.Error, "run is not approved") {
		t.Fatalf("error = (%s) %q, want invalid args run approval error", res.ErrorKind, res.Error)
	}
}

func TestExecute_RejectsThinOpenSpecTasks(t *testing.T) {
	tr := approvedChangeTriples()
	delete(tr, "change.add-mfa.task.0.target_files")
	ex := NewExecutor(fakeReader{id: testRunEntity, triples: tr}, &fakePub{}, platform(), nil)

	res, err := ex.Execute(context.Background(), callWith(nil))
	if err != nil {
		t.Fatalf("Execute returned transport error: %v", err)
	}
	if res.ErrorKind != agentic.ToolErrorInvalidArgs || !strings.Contains(res.Error, "target_files") {
		t.Fatalf("error = (%s) %q, want target_files validation error", res.ErrorKind, res.Error)
	}
}

// CreateEntityWithTriples satisfies beta.159's widened TriplePublisher;
// the fake delegates to AddTriplesBatch so recording semantics are identical.
func (f *fakePub) CreateEntityWithTriples(ctx context.Context, _ string, _ message.Type, triples []message.Triple) error {
	return f.AddTriplesBatch(ctx, triples)
}
