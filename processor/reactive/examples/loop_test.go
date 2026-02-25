package examples

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/processor/reactive"
	"github.com/c360studio/semstreams/processor/reactive/testutil"
)

// ReviewLoopState represents a review loop workflow: review → fix → review (until approved).
type ReviewLoopState struct {
	reactive.ExecutionState
	Content       string   `json:"content"`
	ReviewNotes   []string `json:"review_notes,omitempty"`
	Verdict       string   `json:"verdict,omitempty"` // "approved", "needs_work", "rejected"
	MaxIterations int      `json:"max_iterations"`
}

// ReviewResult is the result of a review.
type ReviewResult struct {
	TaskID      string `json:"task_id"`
	Verdict     string `json:"verdict"`
	Notes       string `json:"notes,omitempty"`
	Suggestions string `json:"suggestions,omitempty"`
}

func (r *ReviewResult) Schema() message.Type {
	return message.Type{Domain: "example", Category: "review-result", Version: "v1"}
}

func (r *ReviewResult) Validate() error { return nil }

func (r *ReviewResult) MarshalJSON() ([]byte, error) {
	type Alias ReviewResult
	return json.Marshal((*Alias)(r))
}

func (r *ReviewResult) UnmarshalJSON(data []byte) error {
	type Alias ReviewResult
	return json.Unmarshal(data, (*Alias)(r))
}

// ReviewRequest is sent to request a review.
type ReviewRequest struct {
	TaskID    string `json:"task_id"`
	Content   string `json:"content"`
	Iteration int    `json:"iteration"`
}

func (r *ReviewRequest) Schema() message.Type {
	return message.Type{Domain: "example", Category: "review-request", Version: "v1"}
}

func (r *ReviewRequest) Validate() error { return nil }

func (r *ReviewRequest) MarshalJSON() ([]byte, error) {
	type Alias ReviewRequest
	return json.Marshal((*Alias)(r))
}

func (r *ReviewRequest) UnmarshalJSON(data []byte) error {
	type Alias ReviewRequest
	return json.Unmarshal(data, (*Alias)(r))
}

// buildReviewLoopWorkflow creates a review loop workflow:
// 1. "reviewing" → sends for review
// 2. "fixing" → receives feedback and fixes (loops back to reviewing)
// 3. "approved" → final success state
// 4. "rejected" → final failure state
// 5. "escalated" → max iterations exceeded
func buildReviewLoopWorkflow() *reactive.Definition {
	const maxIterations = 3

	return reactive.NewWorkflow("review-loop").
		WithDescription("Review loop: review → fix → review (until approved, rejected, or max iterations)").
		WithStateBucket("REVIEW_STATE").
		WithStateFactory(func() any {
			return &ReviewLoopState{MaxIterations: maxIterations}
		}).
		WithMaxIterations(maxIterations).
		WithTimeout(30 * time.Minute).
		// Rule 1: Start review when in reviewing phase
		AddRule(reactive.NewRule("request-review").
			WatchKV("REVIEW_STATE", "review-loop.*").
			When("phase is reviewing", reactive.PhaseIs("reviewing")).
			When("no pending task", reactive.NoPendingTask()).
			When("under max iterations", reactive.IterationLessThan(maxIterations)).
			PublishAsync(
				"reviewer.input",
				func(ctx *reactive.RuleContext) (message.Payload, error) {
					state := ctx.State.(*ReviewLoopState)
					return &ReviewRequest{
						TaskID:    state.ID,
						Content:   state.Content,
						Iteration: state.Iteration,
					}, nil
				},
				"example.review-result.v1",
				func(ctx *reactive.RuleContext, result any) error {
					state := ctx.State.(*ReviewLoopState)
					if res, ok := result.(*ReviewResult); ok {
						state.Verdict = res.Verdict
						if res.Notes != "" {
							state.ReviewNotes = append(state.ReviewNotes, res.Notes)
						}
						// Apply suggestions to content for next iteration
						if res.Suggestions != "" {
							state.Content = res.Suggestions
						}
					}
					state.Phase = "evaluated"
					return nil
				},
			).
			MustBuild()).
		// Rule 2: Handle approved verdict
		AddRule(reactive.NewRule("handle-approved").
			WatchKV("REVIEW_STATE", "review-loop.*").
			When("phase is evaluated", reactive.PhaseIs("evaluated")).
			When("verdict is approved", reactive.StateFieldEquals(
				func(s any) string { return s.(*ReviewLoopState).Verdict },
				"approved",
			)).
			CompleteWithMutation(func(ctx *reactive.RuleContext, _ any) error {
				state := ctx.State.(*ReviewLoopState)
				state.Phase = "approved"
				state.Status = reactive.StatusCompleted
				return nil
			}).
			MustBuild()).
		// Rule 3: Handle rejected verdict
		AddRule(reactive.NewRule("handle-rejected").
			WatchKV("REVIEW_STATE", "review-loop.*").
			When("phase is evaluated", reactive.PhaseIs("evaluated")).
			When("verdict is rejected", reactive.StateFieldEquals(
				func(s any) string { return s.(*ReviewLoopState).Verdict },
				"rejected",
			)).
			Mutate(func(ctx *reactive.RuleContext, _ any) error {
				state := ctx.State.(*ReviewLoopState)
				state.Phase = "rejected"
				state.Status = reactive.StatusFailed
				state.Error = "review rejected"
				return nil
			}).
			MustBuild()).
		// Rule 4: Handle needs_work verdict - loop back to reviewing
		AddRule(reactive.NewRule("handle-needs-work").
			WatchKV("REVIEW_STATE", "review-loop.*").
			When("phase is evaluated", reactive.PhaseIs("evaluated")).
			When("verdict is needs_work", reactive.StateFieldEquals(
				func(s any) string { return s.(*ReviewLoopState).Verdict },
				"needs_work",
			)).
			When("under max iterations", reactive.IterationLessThan(maxIterations)).
			Mutate(reactive.ChainMutators(
				reactive.IncrementIterationMutator(),
				reactive.PhaseTransition("reviewing"),
			)).
			MustBuild()).
		// Rule 5: Handle max iterations exceeded
		AddRule(reactive.NewRule("handle-max-iterations").
			WatchKV("REVIEW_STATE", "review-loop.*").
			When("phase is evaluated", reactive.PhaseIs("evaluated")).
			When("verdict is needs_work", reactive.StateFieldEquals(
				func(s any) string { return s.(*ReviewLoopState).Verdict },
				"needs_work",
			)).
			When("at or over max iterations", reactive.Not(reactive.IterationLessThan(maxIterations))).
			Mutate(func(ctx *reactive.RuleContext, _ any) error {
				state := ctx.State.(*ReviewLoopState)
				state.Phase = "escalated"
				state.Status = reactive.StatusEscalated
				state.Error = "max review iterations exceeded"
				return nil
			}).
			MustBuild()).
		MustBuild()
}

func TestReviewLoopWorkflow_Definition(t *testing.T) {
	def := buildReviewLoopWorkflow()

	if def.ID != "review-loop" {
		t.Errorf("Expected ID 'review-loop', got %q", def.ID)
	}
	if len(def.Rules) != 5 {
		t.Errorf("Expected 5 rules, got %d", len(def.Rules))
	}
	if def.MaxIterations != 3 {
		t.Errorf("Expected MaxIterations 3, got %d", def.MaxIterations)
	}

	// Verify rule IDs
	expectedRules := []string{
		"request-review",
		"handle-approved",
		"handle-rejected",
		"handle-needs-work",
		"handle-max-iterations",
	}
	for i, expected := range expectedRules {
		if def.Rules[i].ID != expected {
			t.Errorf("Expected rule %d ID %q, got %q", i, expected, def.Rules[i].ID)
		}
	}
}

func TestReviewLoopWorkflow_ApprovedPath(t *testing.T) {
	def := buildReviewLoopWorkflow()

	// Initial state
	state := &ReviewLoopState{
		ExecutionState: reactive.ExecutionState{
			ID:        "exec-001",
			Phase:     "evaluated",
			Status:    reactive.StatusRunning,
			Iteration: 0,
		},
		Content: "good content",
		Verdict: "approved",
	}
	ctx := &reactive.RuleContext{State: state}

	// Find the handle-approved rule
	var approvedRule *reactive.RuleDef
	for i := range def.Rules {
		if def.Rules[i].ID == "handle-approved" {
			approvedRule = &def.Rules[i]
			break
		}
	}
	if approvedRule == nil {
		t.Fatal("handle-approved rule not found")
	}

	// Check conditions match
	for _, cond := range approvedRule.Conditions {
		if !cond.Evaluate(ctx) {
			t.Errorf("Condition %q should pass", cond.Description)
		}
	}

	// Apply mutation
	err := approvedRule.Action.MutateState(ctx, nil)
	if err != nil {
		t.Fatalf("MutateState failed: %v", err)
	}

	if state.Phase != "approved" {
		t.Errorf("Expected phase 'approved', got %q", state.Phase)
	}
	if state.Status != reactive.StatusCompleted {
		t.Errorf("Expected status StatusCompleted, got %v", state.Status)
	}
}

func TestReviewLoopWorkflow_NeedsWorkLoop(t *testing.T) {
	def := buildReviewLoopWorkflow()

	// State that needs work, iteration 0
	state := &ReviewLoopState{
		ExecutionState: reactive.ExecutionState{
			ID:        "exec-001",
			Phase:     "evaluated",
			Status:    reactive.StatusRunning,
			Iteration: 0,
		},
		Content:       "needs improvement",
		Verdict:       "needs_work",
		MaxIterations: 3,
	}
	ctx := &reactive.RuleContext{State: state}

	// Find the handle-needs-work rule
	var needsWorkRule *reactive.RuleDef
	for i := range def.Rules {
		if def.Rules[i].ID == "handle-needs-work" {
			needsWorkRule = &def.Rules[i]
			break
		}
	}
	if needsWorkRule == nil {
		t.Fatal("handle-needs-work rule not found")
	}

	// Check conditions match
	for _, cond := range needsWorkRule.Conditions {
		if !cond.Evaluate(ctx) {
			t.Errorf("Condition %q should pass", cond.Description)
		}
	}

	// Apply mutation
	err := needsWorkRule.Action.MutateState(ctx, nil)
	if err != nil {
		t.Fatalf("MutateState failed: %v", err)
	}

	if state.Phase != "reviewing" {
		t.Errorf("Expected phase 'reviewing', got %q", state.Phase)
	}
	if state.Iteration != 1 {
		t.Errorf("Expected iteration 1, got %d", state.Iteration)
	}
}

func TestReviewLoopWorkflow_MaxIterationsEscalation(t *testing.T) {
	def := buildReviewLoopWorkflow()

	// State at max iterations
	state := &ReviewLoopState{
		ExecutionState: reactive.ExecutionState{
			ID:        "exec-001",
			Phase:     "evaluated",
			Status:    reactive.StatusRunning,
			Iteration: 3, // At max
		},
		Content:       "still needs work",
		Verdict:       "needs_work",
		MaxIterations: 3,
	}
	ctx := &reactive.RuleContext{State: state}

	// handle-needs-work should NOT match (iteration check fails)
	var needsWorkRule *reactive.RuleDef
	for i := range def.Rules {
		if def.Rules[i].ID == "handle-needs-work" {
			needsWorkRule = &def.Rules[i]
			break
		}
	}

	allMatch := true
	for _, cond := range needsWorkRule.Conditions {
		if !cond.Evaluate(ctx) {
			allMatch = false
			break
		}
	}
	if allMatch {
		t.Error("handle-needs-work should NOT match at max iterations")
	}

	// handle-max-iterations SHOULD match
	var maxIterRule *reactive.RuleDef
	for i := range def.Rules {
		if def.Rules[i].ID == "handle-max-iterations" {
			maxIterRule = &def.Rules[i]
			break
		}
	}

	for _, cond := range maxIterRule.Conditions {
		if !cond.Evaluate(ctx) {
			t.Errorf("Condition %q should pass for max iterations", cond.Description)
		}
	}

	// Apply mutation
	err := maxIterRule.Action.MutateState(ctx, nil)
	if err != nil {
		t.Fatalf("MutateState failed: %v", err)
	}

	if state.Phase != "escalated" {
		t.Errorf("Expected phase 'escalated', got %q", state.Phase)
	}
	if state.Status != reactive.StatusEscalated {
		t.Errorf("Expected status StatusEscalated, got %v", state.Status)
	}
}

func TestReviewLoopWorkflow_WithTestEngine(t *testing.T) {
	engine := testutil.NewTestEngine(t)
	def := buildReviewLoopWorkflow()

	if err := engine.RegisterWorkflow(def); err != nil {
		t.Fatalf("RegisterWorkflow failed: %v", err)
	}

	// Create initial state
	state := &ReviewLoopState{
		ExecutionState: reactive.ExecutionState{
			ID:         "exec-001",
			WorkflowID: "review-loop",
			Phase:      "reviewing",
			Status:     reactive.StatusRunning,
			Iteration:  0,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		},
		Content:       "content to review",
		MaxIterations: 3,
	}

	key := "review-loop.exec-001"
	err := engine.TriggerKV(context.Background(), key, state)
	if err != nil {
		t.Fatalf("TriggerKV failed: %v", err)
	}

	engine.AssertPhase(key, "reviewing")
	engine.AssertIteration(key, 0)
}

func TestReviewLoopWorkflow_IterationTracking(t *testing.T) {
	// Test that iterations are properly tracked through the loop
	state := &ReviewLoopState{
		ExecutionState: reactive.ExecutionState{
			Iteration: 0,
		},
	}

	// Simulate 3 iterations
	for i := 0; i < 3; i++ {
		reactive.IncrementIteration(state)
	}

	if state.Iteration != 3 {
		t.Errorf("Expected 3 iterations, got %d", state.Iteration)
	}
}
