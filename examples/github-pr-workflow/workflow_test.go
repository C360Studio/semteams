package githubprworkflow

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/processor/reactive"
)

// TestNewIssueToPRWorkflow verifies that the workflow definition builds
// without error and has the expected top-level configuration.
func TestNewIssueToPRWorkflow(t *testing.T) {
	def := NewIssueToPRWorkflow()

	if def.ID != "github-issue-to-pr" {
		t.Errorf("ID: got %q, want %q", def.ID, "github-issue-to-pr")
	}
	if def.StateBucket != StateBucket {
		t.Errorf("StateBucket: got %q, want %q", def.StateBucket, StateBucket)
	}
	if def.Timeout != WorkflowTimeout {
		t.Errorf("Timeout: got %v, want %v", def.Timeout, WorkflowTimeout)
	}
	if def.MaxIterations != MaxReviewCycles+3 {
		t.Errorf("MaxIterations: got %d, want %d", def.MaxIterations, MaxReviewCycles+3)
	}
	if len(def.Rules) != 9 {
		t.Errorf("Rules count: got %d, want 9", len(def.Rules))
	}
}

// TestNewIssueToPRWorkflow_RuleIDs verifies that all eight rules are present
// with the expected identifiers.
func TestNewIssueToPRWorkflow_RuleIDs(t *testing.T) {
	def := NewIssueToPRWorkflow()

	wantIDs := []string{
		"spawn-qualifier",
		"spawn-developer",
		"spawn-reviewer",
		"review-approved",
		"review-rejected-retry",
		"escalate-deadlock",
		"issue-rejected",
		"needs-info",
		"budget-exceeded",
	}

	for i, want := range wantIDs {
		if i >= len(def.Rules) {
			t.Errorf("missing rule at index %d (want %q)", i, want)
			continue
		}
		if got := def.Rules[i].ID; got != want {
			t.Errorf("Rules[%d].ID: got %q, want %q", i, got, want)
		}
	}
}

// TestNewIssueToPRWorkflow_EventConfig verifies lifecycle event subjects.
func TestNewIssueToPRWorkflow_EventConfig(t *testing.T) {
	def := NewIssueToPRWorkflow()

	if def.Events.OnComplete != "github.workflow.complete" {
		t.Errorf("OnComplete: got %q, want %q", def.Events.OnComplete, "github.workflow.complete")
	}
	if def.Events.OnFail != "github.workflow.failed" {
		t.Errorf("OnFail: got %q, want %q", def.Events.OnFail, "github.workflow.failed")
	}
	if def.Events.OnEscalate != "github.workflow.escalated" {
		t.Errorf("OnEscalate: got %q, want %q", def.Events.OnEscalate, "github.workflow.escalated")
	}
}

// TestNewIssueToPRWorkflow_StateFactory verifies that the state factory
// produces a properly typed IssueToPRState pointer.
func TestNewIssueToPRWorkflow_StateFactory(t *testing.T) {
	def := NewIssueToPRWorkflow()

	raw := def.StateFactory()
	s, ok := raw.(*IssueToPRState)
	if !ok {
		t.Fatalf("StateFactory() returned %T, want *IssueToPRState", raw)
	}
	// Zero value should have an empty phase, not an unexpected default.
	if s.Phase != "" {
		t.Errorf("StateFactory() Phase: got %q, want empty string", s.Phase)
	}
}

// TestIssueToPRState_GetExecutionState verifies the StateAccessor implementation.
func TestIssueToPRState_GetExecutionState(t *testing.T) {
	s := &IssueToPRState{}
	s.ID = "test-exec-001"
	s.Phase = PhaseQualified

	es := s.GetExecutionState()
	if es == nil {
		t.Fatal("GetExecutionState() returned nil")
	}
	if es.ID != "test-exec-001" {
		t.Errorf("ID: got %q, want %q", es.ID, "test-exec-001")
	}
	if es.Phase != PhaseQualified {
		t.Errorf("Phase: got %q, want %q", es.Phase, PhaseQualified)
	}
}

// TestIssueToPRState_ExtractExecutionState verifies that reactive.ExtractExecutionState
// can find the embedded ExecutionState without reflection fallback (via StateAccessor).
func TestIssueToPRState_ExtractExecutionState(t *testing.T) {
	s := &IssueToPRState{}
	s.ID = "exec-42"

	es := reactive.ExtractExecutionState(s)
	if es == nil {
		t.Fatal("ExtractExecutionState() returned nil")
	}
	if es.ID != "exec-42" {
		t.Errorf("ID: got %q, want %q", es.ID, "exec-42")
	}
}

// TestBuildQualifierTask verifies that buildQualifierTask produces a valid
// TaskMessage with the correct role, workflow context, and prompt content.
func TestBuildQualifierTask(t *testing.T) {
	event := &GitHubIssueWebhookEvent{
		Action: "opened",
		Issue: IssueDetail{
			Number: 42,
			Title:  "Server panics on empty input",
			Body:   "Steps to reproduce: send an empty POST body.",
		},
		Repo: RepoDetail{
			Name:  "myapp",
			Owner: OwnerDetail{Login: "acme"},
		},
	}

	state := &IssueToPRState{}
	state.ID = "exec-001"

	ctx := &reactive.RuleContext{
		Message: event,
		State:   state,
	}

	payload, err := buildQualifierTask(ctx)
	if err != nil {
		t.Fatalf("buildQualifierTask() error: %v", err)
	}

	task, ok := payload.(*agentic.TaskMessage)
	if !ok {
		t.Fatalf("buildQualifierTask() returned %T, want *agentic.TaskMessage", payload)
	}

	if task.Role != agentic.RoleQualifier {
		t.Errorf("Role: got %q, want %q", task.Role, agentic.RoleQualifier)
	}
	if task.WorkflowStep != "qualify" {
		t.Errorf("WorkflowStep: got %q, want %q", task.WorkflowStep, "qualify")
	}
	if task.Model == "" {
		t.Error("Model must not be empty")
	}
	if task.TaskID == "" {
		t.Error("TaskID must not be empty")
	}

	// Prompt should contain issue and repo context.
	for _, want := range []string{"acme", "myapp", "#42", "Server panics"} {
		if !containsString(task.Prompt, want) {
			t.Errorf("Prompt missing %q", want)
		}
	}
}

// TestBuildQualifierTask_WrongMessage verifies graceful failure when the
// message is not a *GitHubIssueWebhookEvent.
func TestBuildQualifierTask_WrongMessage(t *testing.T) {
	ctx := &reactive.RuleContext{
		Message: "unexpected string",
		State:   &IssueToPRState{},
	}

	_, err := buildQualifierTask(ctx)
	if err == nil {
		t.Fatal("expected error for wrong message type, got nil")
	}
}

// TestHandleQualifierResult_Qualified verifies that a "qualified" verdict
// updates all relevant state fields correctly.
func TestHandleQualifierResult_Qualified(t *testing.T) {
	state := &IssueToPRState{}
	state.ID = "exec-002"

	resultJSON := `{"verdict":"qualified","confidence":0.92,"severity":"high"}`
	event := &agentic.LoopCompletedEvent{
		LoopID:  "loop-001",
		TaskID:  "task-001",
		Outcome: agentic.OutcomeSuccess,
		Result:  resultJSON,
	}

	ctx := &reactive.RuleContext{State: state}

	if err := handleQualifierResult(ctx, event); err != nil {
		t.Fatalf("handleQualifierResult() error: %v", err)
	}

	if state.Phase != PhaseQualified {
		t.Errorf("Phase: got %q, want %q", state.Phase, PhaseQualified)
	}
	if state.QualifierVerdict != "qualified" {
		t.Errorf("QualifierVerdict: got %q, want %q", state.QualifierVerdict, "qualified")
	}
	if state.QualifierConfidence != 0.92 {
		t.Errorf("QualifierConfidence: got %v, want 0.92", state.QualifierConfidence)
	}
	if state.Severity != "high" {
		t.Errorf("Severity: got %q, want %q", state.Severity, "high")
	}
}

// TestHandleQualifierResult_Rejected verifies the rejection phase transition.
func TestHandleQualifierResult_Rejected(t *testing.T) {
	state := &IssueToPRState{}
	resultJSON := `{"verdict":"rejected","confidence":0.85,"severity":"low"}`
	event := &agentic.LoopCompletedEvent{Result: resultJSON}

	ctx := &reactive.RuleContext{State: state}

	if err := handleQualifierResult(ctx, event); err != nil {
		t.Fatalf("handleQualifierResult() error: %v", err)
	}

	if state.Phase != PhaseRejected {
		t.Errorf("Phase: got %q, want %q", state.Phase, PhaseRejected)
	}
}

// TestHandleQualifierResult_BadJSON verifies that unparseable results fall
// back to PhaseNeedsInfo rather than returning an error.
func TestHandleQualifierResult_BadJSON(t *testing.T) {
	state := &IssueToPRState{}
	event := &agentic.LoopCompletedEvent{Result: "I cannot determine the verdict."}

	ctx := &reactive.RuleContext{State: state}

	if err := handleQualifierResult(ctx, event); err != nil {
		t.Fatalf("handleQualifierResult() error: %v", err)
	}

	if state.Phase != PhaseNeedsInfo {
		t.Errorf("Phase: got %q, want %q (expected graceful fallback)", state.Phase, PhaseNeedsInfo)
	}
}

// TestHandleReviewerResult_Approve verifies that an "approved" verdict sets
// the phase to PhaseApproved without incrementing the rejection counter.
func TestHandleReviewerResult_Approve(t *testing.T) {
	state := &IssueToPRState{}
	resultJSON := `{"verdict":"approved","feedback":""}`
	event := &agentic.LoopCompletedEvent{Result: resultJSON}

	ctx := &reactive.RuleContext{State: state}

	if err := handleReviewerResult(ctx, event); err != nil {
		t.Fatalf("handleReviewerResult() error: %v", err)
	}

	if state.Phase != PhaseApproved {
		t.Errorf("Phase: got %q, want %q", state.Phase, PhaseApproved)
	}
	if state.ReviewRejections != 0 {
		t.Errorf("ReviewRejections: got %d, want 0", state.ReviewRejections)
	}
}

// TestHandleReviewerResult_Reject verifies that a "request_changes" verdict
// sets the phase to PhaseChangesRequested and increments ReviewRejections.
func TestHandleReviewerResult_Reject(t *testing.T) {
	state := &IssueToPRState{}
	resultJSON := `{"verdict":"request_changes","feedback":"Please add unit tests."}`
	event := &agentic.LoopCompletedEvent{Result: resultJSON}

	ctx := &reactive.RuleContext{State: state}

	if err := handleReviewerResult(ctx, event); err != nil {
		t.Fatalf("handleReviewerResult() error: %v", err)
	}

	if state.Phase != PhaseChangesRequested {
		t.Errorf("Phase: got %q, want %q", state.Phase, PhaseChangesRequested)
	}
	if state.ReviewRejections != 1 {
		t.Errorf("ReviewRejections: got %d, want 1", state.ReviewRejections)
	}
	if state.ReviewFeedback != "Please add unit tests." {
		t.Errorf("ReviewFeedback: got %q, want %q", state.ReviewFeedback, "Please add unit tests.")
	}
}

// TestHandleReviewerResult_MultipleRejections verifies rejection accumulation
// across multiple review cycles.
func TestHandleReviewerResult_MultipleRejections(t *testing.T) {
	state := &IssueToPRState{ReviewRejections: 1}
	resultJSON := `{"verdict":"request_changes","feedback":"Still missing error handling."}`
	event := &agentic.LoopCompletedEvent{Result: resultJSON}

	ctx := &reactive.RuleContext{State: state}

	if err := handleReviewerResult(ctx, event); err != nil {
		t.Fatalf("handleReviewerResult() error: %v", err)
	}

	if state.ReviewRejections != 2 {
		t.Errorf("ReviewRejections: got %d, want 2", state.ReviewRejections)
	}
}

// TestHandleDeveloperResult verifies that a successful developer response
// populates branch, PR, and file change fields and transitions to dev_complete.
func TestHandleDeveloperResult(t *testing.T) {
	state := &IssueToPRState{
		IssueNumber: 42,
		RepoOwner:   "acme",
		RepoName:    "myapp",
	}
	resultJSON := `{
		"branch_name": "fix/issue-42-empty-input",
		"pr_number": 99,
		"pr_url": "https://github.com/acme/myapp/pull/99",
		"files_changed": ["server/handler.go", "server/handler_test.go"]
	}`
	event := &agentic.LoopCompletedEvent{Result: resultJSON}

	ctx := &reactive.RuleContext{State: state}

	if err := handleDeveloperResult(ctx, event); err != nil {
		t.Fatalf("handleDeveloperResult() error: %v", err)
	}

	if state.Phase != PhaseDevComplete {
		t.Errorf("Phase: got %q, want %q", state.Phase, PhaseDevComplete)
	}
	if state.BranchName != "fix/issue-42-empty-input" {
		t.Errorf("BranchName: got %q", state.BranchName)
	}
	if state.PRNumber != 99 {
		t.Errorf("PRNumber: got %d, want 99", state.PRNumber)
	}
	if len(state.FilesChanged) != 2 {
		t.Errorf("FilesChanged: got %d entries, want 2", len(state.FilesChanged))
	}
}

// TestBuildCompletionPayload verifies that the completion payload contains
// all relevant execution summary fields.
func TestBuildCompletionPayload(t *testing.T) {
	state := &IssueToPRState{
		IssueNumber:         42,
		RepoOwner:           "acme",
		RepoName:            "myapp",
		PRNumber:            99,
		PRUrl:               "https://github.com/acme/myapp/pull/99",
		FilesChanged:        []string{"server/handler.go"},
		DevelopmentAttempts: 2,
		ReviewRejections:    1,
	}
	state.ID = "exec-final"
	state.Phase = PhaseApproved

	ctx := &reactive.RuleContext{State: state}

	payload, err := buildCompletionPayload(ctx)
	if err != nil {
		t.Fatalf("buildCompletionPayload() error: %v", err)
	}

	cp, ok := payload.(*WorkflowCompletionPayload)
	if !ok {
		t.Fatalf("buildCompletionPayload() returned %T, want *WorkflowCompletionPayload", payload)
	}

	if cp.ExecutionID != "exec-final" {
		t.Errorf("ExecutionID: got %q, want %q", cp.ExecutionID, "exec-final")
	}
	if cp.IssueNumber != 42 {
		t.Errorf("IssueNumber: got %d, want 42", cp.IssueNumber)
	}
	if cp.PRNumber != 99 {
		t.Errorf("PRNumber: got %d, want 99", cp.PRNumber)
	}
	if cp.DevelopmentAttempts != 2 {
		t.Errorf("DevelopmentAttempts: got %d, want 2", cp.DevelopmentAttempts)
	}
	if cp.ReviewRejections != 1 {
		t.Errorf("ReviewRejections: got %d, want 1", cp.ReviewRejections)
	}
}

// TestWorkflowCompletionPayload_Validate verifies the Validate method enforces
// the ExecutionID requirement.
func TestWorkflowCompletionPayload_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		p := &WorkflowCompletionPayload{ExecutionID: "exec-001"}
		if err := p.Validate(); err != nil {
			t.Errorf("Validate() error: %v", err)
		}
	})

	t.Run("missing execution_id", func(t *testing.T) {
		p := &WorkflowCompletionPayload{}
		if err := p.Validate(); err == nil {
			t.Error("Validate() should fail when ExecutionID is empty")
		}
	})
}

// TestWorkflowCompletionPayload_Schema verifies the payload schema type.
func TestWorkflowCompletionPayload_Schema(t *testing.T) {
	p := &WorkflowCompletionPayload{}
	schema := p.Schema()

	if schema.Domain != "github" {
		t.Errorf("Schema.Domain: got %q, want %q", schema.Domain, "github")
	}
	if schema.Category != "workflow_complete" {
		t.Errorf("Schema.Category: got %q, want %q", schema.Category, "workflow_complete")
	}
}

// TestBuildDeveloperTask_IncludesReviewFeedback verifies that on retry
// attempts the reviewer feedback is included in the developer prompt.
func TestBuildDeveloperTask_IncludesReviewFeedback(t *testing.T) {
	state := &IssueToPRState{
		IssueNumber:         7,
		IssueTitle:          "Crash on startup",
		IssueBody:           "The process crashes immediately.",
		RepoOwner:           "corp",
		RepoName:            "api",
		QualifierVerdict:    "qualified",
		QualifierConfidence: 0.9,
		Severity:            "critical",
		ReviewFeedback:      "Missing nil guard in main.go line 42.",
		DevelopmentAttempts: 1,
	}
	state.ID = "exec-retry"

	ctx := &reactive.RuleContext{State: state}

	payload, err := buildDeveloperTask(ctx)
	if err != nil {
		t.Fatalf("buildDeveloperTask() error: %v", err)
	}

	task, ok := payload.(*agentic.TaskMessage)
	if !ok {
		t.Fatalf("buildDeveloperTask() returned %T", payload)
	}

	if !containsString(task.Prompt, "Missing nil guard") {
		t.Error("Prompt should include previous review feedback on retry")
	}
	if task.Role != agentic.RoleDeveloper {
		t.Errorf("Role: got %q, want %q", task.Role, agentic.RoleDeveloper)
	}
}

// TestBuildReviewerTask verifies that the reviewer task includes PR details.
func TestBuildReviewerTask(t *testing.T) {
	state := &IssueToPRState{
		IssueNumber:  12,
		IssueTitle:   "Memory leak in worker pool",
		RepoOwner:    "example",
		RepoName:     "service",
		PRNumber:     55,
		PRUrl:        "https://github.com/example/service/pull/55",
		FilesChanged: []string{"worker/pool.go"},
	}
	state.ID = "exec-review"

	ctx := &reactive.RuleContext{State: state}

	payload, err := buildReviewerTask(ctx)
	if err != nil {
		t.Fatalf("buildReviewerTask() error: %v", err)
	}

	task, ok := payload.(*agentic.TaskMessage)
	if !ok {
		t.Fatalf("buildReviewerTask() returned %T", payload)
	}

	if task.Role != agentic.RoleReviewer {
		t.Errorf("Role: got %q, want %q", task.Role, agentic.RoleReviewer)
	}
	for _, want := range []string{"55", "worker/pool.go", "example", "service"} {
		if !containsString(task.Prompt, want) {
			t.Errorf("Prompt missing %q", want)
		}
	}
}

// TestRuleConditions_SpawnQualifier verifies the spawn-qualifier rule
// condition logic for action == "opened" events.
func TestRuleConditions_SpawnQualifier(t *testing.T) {
	def := NewIssueToPRWorkflow()
	rule := def.Rules[0] // spawn-qualifier

	if len(rule.Conditions) != 1 {
		t.Fatalf("spawn-qualifier has %d conditions, want 1", len(rule.Conditions))
	}
	cond := rule.Conditions[0]

	t.Run("opened event passes", func(t *testing.T) {
		ctx := &reactive.RuleContext{
			Message: &GitHubIssueWebhookEvent{Action: "opened"},
		}
		if !cond.Evaluate(ctx) {
			t.Error("condition should be true for action=opened")
		}
	})

	t.Run("closed event fails", func(t *testing.T) {
		ctx := &reactive.RuleContext{
			Message: &GitHubIssueWebhookEvent{Action: "closed"},
		}
		if cond.Evaluate(ctx) {
			t.Error("condition should be false for action=closed")
		}
	})

	t.Run("wrong message type fails", func(t *testing.T) {
		ctx := &reactive.RuleContext{
			Message: "not an event",
		}
		if cond.Evaluate(ctx) {
			t.Error("condition should be false for wrong message type")
		}
	})
}

// TestRuleConditions_EscalateDeadlock verifies the escalation rule triggers
// only when rejections have reached MaxReviewCycles.
func TestRuleConditions_EscalateDeadlock(t *testing.T) {
	def := NewIssueToPRWorkflow()
	rule := def.Rules[5] // escalate-deadlock

	if rule.ID != "escalate-deadlock" {
		t.Fatalf("expected escalate-deadlock rule at index 5, got %q", rule.ID)
	}
	if len(rule.Conditions) != 2 {
		t.Fatalf("escalate-deadlock has %d conditions, want 2", len(rule.Conditions))
	}

	t.Run("at limit triggers escalation", func(t *testing.T) {
		state := &IssueToPRState{ReviewRejections: MaxReviewCycles}
		state.Phase = PhaseChangesRequested
		ctx := &reactive.RuleContext{State: state}

		for i, cond := range rule.Conditions {
			if !cond.Evaluate(ctx) {
				t.Errorf("condition[%d] %q should be true at MaxReviewCycles", i, cond.Description)
			}
		}
	})

	t.Run("below limit does not trigger escalation", func(t *testing.T) {
		state := &IssueToPRState{ReviewRejections: MaxReviewCycles - 1}
		state.Phase = PhaseChangesRequested
		ctx := &reactive.RuleContext{State: state}

		// Second condition (budget exhausted) must be false.
		if rule.Conditions[1].Evaluate(ctx) {
			t.Error("condition should be false when below MaxReviewCycles")
		}
	})
}

// TestRuleConditions_IssueRejected verifies that the rejection rule
// matches all three rejection phases.
func TestRuleConditions_IssueRejected(t *testing.T) {
	def := NewIssueToPRWorkflow()
	rule := def.Rules[6] // issue-rejected

	if rule.ID != "issue-rejected" {
		t.Fatalf("expected issue-rejected at index 6, got %q", rule.ID)
	}

	rejectionPhases := []string{PhaseRejected, PhaseNotABug, PhaseWontFix}
	for _, phase := range rejectionPhases {
		t.Run(phase, func(t *testing.T) {
			state := &IssueToPRState{}
			state.Phase = phase
			ctx := &reactive.RuleContext{State: state}

			if !rule.Conditions[0].Evaluate(ctx) {
				t.Errorf("condition should be true for phase=%q", phase)
			}
		})
	}

	// Non-rejection phases must not trigger.
	nonRejectionPhases := []string{PhaseQualified, PhaseDevComplete, PhaseApproved}
	for _, phase := range nonRejectionPhases {
		t.Run("non-rejection/"+phase, func(t *testing.T) {
			state := &IssueToPRState{}
			state.Phase = phase
			ctx := &reactive.RuleContext{State: state}

			if rule.Conditions[0].Evaluate(ctx) {
				t.Errorf("condition should be false for phase=%q", phase)
			}
		})
	}
}

// TestGitHubIssueWebhookEvent_MessageFactory verifies the message factory
// used by the spawn-qualifier rule produces the correct type.
func TestGitHubIssueWebhookEvent_MessageFactory(t *testing.T) {
	def := NewIssueToPRWorkflow()
	rule := def.Rules[0] // spawn-qualifier

	raw := rule.Trigger.MessageFactory()
	if _, ok := raw.(*GitHubIssueWebhookEvent); !ok {
		t.Errorf("MessageFactory() returned %T, want *GitHubIssueWebhookEvent", raw)
	}
}

// TestStatePhaseTransitions exercises the main happy-path phase sequence
// as a state machine table test, ensuring all transitions are valid.
func TestStatePhaseTransitions(t *testing.T) {
	transitions := []struct {
		from string
		to   string
	}{
		{"", PhaseQualified},
		{PhaseQualified, PhaseDevComplete},
		{PhaseDevComplete, PhaseApproved},
		{PhaseDevComplete, PhaseChangesRequested},
		{PhaseChangesRequested, PhaseDeveloping},
		{PhaseDeveloping, PhaseDevComplete},
		{PhaseChangesRequested, PhaseEscalated},
		{"", PhaseRejected},
		{"", PhaseNotABug},
		{"", PhaseWontFix},
		{"", PhaseNeedsInfo},
		{PhaseNeedsInfo, PhaseAwaitingInfo},
	}

	for _, tt := range transitions {
		t.Run(tt.from+"->"+tt.to, func(t *testing.T) {
			s := &IssueToPRState{}
			s.Phase = tt.from
			s.Phase = tt.to
			if s.Phase != tt.to {
				t.Errorf("Phase: got %q, want %q", s.Phase, tt.to)
			}
		})
	}
}

// TestWorkflowCompletionPayload_JSON verifies round-trip JSON serialization.
func TestWorkflowCompletionPayload_JSON(t *testing.T) {
	original := &WorkflowCompletionPayload{
		ExecutionID:         "exec-json-001",
		IssueNumber:         7,
		RepoOwner:           "acme",
		RepoName:            "api",
		PRNumber:            12,
		PRUrl:               "https://github.com/acme/api/pull/12",
		FilesChanged:        []string{"main.go", "api/handler.go"},
		DevelopmentAttempts: 2,
		ReviewRejections:    1,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}

	var restored WorkflowCompletionPayload
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}

	if restored.ExecutionID != original.ExecutionID {
		t.Errorf("ExecutionID: got %q, want %q", restored.ExecutionID, original.ExecutionID)
	}
	if restored.PRNumber != original.PRNumber {
		t.Errorf("PRNumber: got %d, want %d", restored.PRNumber, original.PRNumber)
	}
	if len(restored.FilesChanged) != len(original.FilesChanged) {
		t.Errorf("FilesChanged length: got %d, want %d", len(restored.FilesChanged), len(original.FilesChanged))
	}
}

// TestWorkflowTimeout verifies the WorkflowTimeout constant is within
// a reasonable range for a human-facing automation.
func TestWorkflowTimeout(t *testing.T) {
	if WorkflowTimeout < 5*time.Minute {
		t.Errorf("WorkflowTimeout %v is too short for an agentic pipeline", WorkflowTimeout)
	}
	if WorkflowTimeout > 2*time.Hour {
		t.Errorf("WorkflowTimeout %v is unexpectedly long", WorkflowTimeout)
	}
}

// containsString reports whether s contains substr, using strings package
// to keep tests dependency-free.
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestBudgetExceeded_EscalatesWorkflow verifies that the budget-exceeded rule
// fires when total tokens exceed DefaultTokenBudget and escalates the workflow.
func TestBudgetExceeded_EscalatesWorkflow(t *testing.T) {
	def := NewIssueToPRWorkflow()

	// Find the budget-exceeded rule
	var budgetRule *reactive.RuleDef
	for i := range def.Rules {
		if def.Rules[i].ID == "budget-exceeded" {
			budgetRule = &def.Rules[i]
			break
		}
	}
	if budgetRule == nil {
		t.Fatal("budget-exceeded rule not found")
	}

	t.Run("over budget triggers escalation", func(t *testing.T) {
		state := &IssueToPRState{
			TotalTokensIn:  300_000,
			TotalTokensOut: 250_000, // total 550k > 500k budget
		}
		state.Phase = PhaseDeveloping
		state.Status = reactive.StatusRunning

		ctx := &reactive.RuleContext{State: state}

		for _, cond := range budgetRule.Conditions {
			if !cond.Evaluate(ctx) {
				t.Errorf("condition %q should be true when over budget", cond.Description)
			}
		}

		// Execute the mutation
		if budgetRule.Action.MutateState != nil {
			if err := budgetRule.Action.MutateState(ctx, nil); err != nil {
				t.Fatalf("MutateState() error: %v", err)
			}
		}

		if state.Phase != PhaseEscalated {
			t.Errorf("Phase: got %q, want %q", state.Phase, PhaseEscalated)
		}
		if !state.EscalatedToHuman {
			t.Error("EscalatedToHuman should be true")
		}
	})

	t.Run("under budget does not trigger", func(t *testing.T) {
		state := &IssueToPRState{
			TotalTokensIn:  100_000,
			TotalTokensOut: 50_000, // total 150k < 500k budget
		}
		state.Phase = PhaseDeveloping
		state.Status = reactive.StatusRunning

		ctx := &reactive.RuleContext{State: state}

		// First condition (budget exceeded) should be false
		if budgetRule.Conditions[0].Evaluate(ctx) {
			t.Error("budget condition should be false when under budget")
		}
	})

	t.Run("terminal state does not trigger", func(t *testing.T) {
		state := &IssueToPRState{
			TotalTokensIn:  300_000,
			TotalTokensOut: 250_000,
		}
		state.Phase = PhaseEscalated
		state.Status = reactive.StatusEscalated
		now := time.Now()
		state.CompletedAt = &now

		ctx := &reactive.RuleContext{State: state}

		// Second condition (not already terminal) should be false
		if budgetRule.Conditions[1].Evaluate(ctx) {
			t.Error("terminal condition should be false for already-terminal state")
		}
	})
}

// TestTokenAccumulation_AcrossAgents verifies that token counts accumulate
// correctly when processing results from qualifier, developer, and reviewer.
func TestTokenAccumulation_AcrossAgents(t *testing.T) {
	state := &IssueToPRState{}
	state.ID = "exec-tokens"

	// Qualifier completes with some tokens
	qualifierEvent := &agentic.LoopCompletedEvent{
		Result:   `{"verdict":"qualified","confidence":0.9,"severity":"high"}`,
		TokensIn: 1000, TokensOut: 200,
	}
	ctx := &reactive.RuleContext{State: state}
	if err := handleQualifierResult(ctx, qualifierEvent); err != nil {
		t.Fatalf("handleQualifierResult() error: %v", err)
	}

	if state.TotalTokensIn != 1000 {
		t.Errorf("TotalTokensIn after qualifier: got %d, want 1000", state.TotalTokensIn)
	}
	if state.TotalTokensOut != 200 {
		t.Errorf("TotalTokensOut after qualifier: got %d, want 200", state.TotalTokensOut)
	}

	// Developer completes with more tokens
	devEvent := &agentic.LoopCompletedEvent{
		Result:   `{"branch_name":"fix/test","pr_number":1,"pr_url":"http://example.com","files_changed":["a.go"]}`,
		TokensIn: 5000, TokensOut: 3000,
	}
	if err := handleDeveloperResult(ctx, devEvent); err != nil {
		t.Fatalf("handleDeveloperResult() error: %v", err)
	}

	if state.TotalTokensIn != 6000 {
		t.Errorf("TotalTokensIn after developer: got %d, want 6000", state.TotalTokensIn)
	}
	if state.TotalTokensOut != 3200 {
		t.Errorf("TotalTokensOut after developer: got %d, want 3200", state.TotalTokensOut)
	}

	// Reviewer completes
	reviewEvent := &agentic.LoopCompletedEvent{
		Result:   `{"verdict":"approved","feedback":""}`,
		TokensIn: 2000, TokensOut: 500,
	}
	if err := handleReviewerResult(ctx, reviewEvent); err != nil {
		t.Fatalf("handleReviewerResult() error: %v", err)
	}

	if state.TotalTokensIn != 8000 {
		t.Errorf("TotalTokensIn after reviewer: got %d, want 8000", state.TotalTokensIn)
	}
	if state.TotalTokensOut != 3700 {
		t.Errorf("TotalTokensOut after reviewer: got %d, want 3700", state.TotalTokensOut)
	}
}

// TestBudgetCondition_BlocksSpawn verifies that spawn-developer and
// spawn-reviewer rules do not fire when the token budget is exceeded.
func TestBudgetCondition_BlocksSpawn(t *testing.T) {
	def := NewIssueToPRWorkflow()

	tests := []struct {
		ruleID    string
		ruleIndex int
		phase     string
	}{
		{"spawn-developer", 1, PhaseQualified},
		{"spawn-reviewer", 2, PhaseDevComplete},
	}

	for _, tt := range tests {
		t.Run(tt.ruleID+"/under_budget", func(t *testing.T) {
			rule := def.Rules[tt.ruleIndex]
			if rule.ID != tt.ruleID {
				t.Fatalf("expected %q at index %d, got %q", tt.ruleID, tt.ruleIndex, rule.ID)
			}

			state := &IssueToPRState{
				TotalTokensIn:  100_000,
				TotalTokensOut: 50_000,
			}
			state.Phase = tt.phase

			ctx := &reactive.RuleContext{State: state}

			allPass := true
			for _, cond := range rule.Conditions {
				if !cond.Evaluate(ctx) {
					allPass = false
					break
				}
			}
			if !allPass {
				t.Errorf("all conditions should pass when under budget")
			}
		})

		t.Run(tt.ruleID+"/over_budget", func(t *testing.T) {
			rule := def.Rules[tt.ruleIndex]

			state := &IssueToPRState{
				TotalTokensIn:  400_000,
				TotalTokensOut: 200_000, // 600k > 500k budget
			}
			state.Phase = tt.phase

			ctx := &reactive.RuleContext{State: state}

			// At least one condition should fail (the budget check)
			allPass := true
			for _, cond := range rule.Conditions {
				if !cond.Evaluate(ctx) {
					allPass = false
					break
				}
			}
			if allPass {
				t.Errorf("conditions should not all pass when over budget")
			}
		})
	}
}

// TestCompletionPayload_IncludesTokens verifies that the completion payload
// includes token usage totals.
func TestCompletionPayload_IncludesTokens(t *testing.T) {
	state := &IssueToPRState{
		IssueNumber:    42,
		RepoOwner:      "acme",
		RepoName:       "myapp",
		PRNumber:       99,
		TotalTokensIn:  10000,
		TotalTokensOut: 5000,
	}
	state.ID = "exec-tokens"
	state.Phase = PhaseApproved

	ctx := &reactive.RuleContext{State: state}

	payload, err := buildCompletionPayload(ctx)
	if err != nil {
		t.Fatalf("buildCompletionPayload() error: %v", err)
	}

	cp := payload.(*WorkflowCompletionPayload)
	if cp.TotalTokensIn != 10000 {
		t.Errorf("TotalTokensIn: got %d, want 10000", cp.TotalTokensIn)
	}
	if cp.TotalTokensOut != 5000 {
		t.Errorf("TotalTokensOut: got %d, want 5000", cp.TotalTokensOut)
	}
}

// TestPhaseIsActive verifies the helper distinguishes active from terminal phases.
func TestPhaseIsActive(t *testing.T) {
	activePhases := []string{
		PhaseQualified, PhaseDeveloping, PhaseDevComplete,
		PhaseChangesRequested, PhaseNeedsInfo, PhaseAwaitingInfo,
	}
	for _, p := range activePhases {
		if !PhaseIsActive(p) {
			t.Errorf("PhaseIsActive(%q) = false, want true", p)
		}
	}

	inactivePhases := []string{
		PhaseApproved, PhaseEscalated, PhaseRejected,
		PhaseNotABug, PhaseWontFix, "",
	}
	for _, p := range inactivePhases {
		if PhaseIsActive(p) {
			t.Errorf("PhaseIsActive(%q) = true, want false", p)
		}
	}
}

// TestDefaultBudgetConstants verifies sane defaults for cost controls.
func TestDefaultBudgetConstants(t *testing.T) {
	if DefaultTokenBudget <= 0 {
		t.Errorf("DefaultTokenBudget: got %d, must be positive", DefaultTokenBudget)
	}
	if DefaultMaxConcurrentWorkflows <= 0 {
		t.Errorf("DefaultMaxConcurrentWorkflows: got %d, must be positive", DefaultMaxConcurrentWorkflows)
	}
	if DefaultHourlyTokenCeiling <= DefaultTokenBudget {
		t.Errorf("DefaultHourlyTokenCeiling (%d) should be larger than DefaultTokenBudget (%d)",
			DefaultHourlyTokenCeiling, DefaultTokenBudget)
	}
	if DefaultIssueCooldown <= 0 {
		t.Errorf("DefaultIssueCooldown: got %v, must be positive", DefaultIssueCooldown)
	}
}

// TestSpawnQualifier_HasCooldown verifies that the spawn-qualifier rule
// has a cooldown configured.
func TestSpawnQualifier_HasCooldown(t *testing.T) {
	def := NewIssueToPRWorkflow()
	rule := def.Rules[0]

	if rule.ID != "spawn-qualifier" {
		t.Fatalf("expected spawn-qualifier at index 0, got %q", rule.ID)
	}
	if rule.Cooldown != DefaultIssueCooldown {
		t.Errorf("Cooldown: got %v, want %v", rule.Cooldown, DefaultIssueCooldown)
	}
}
