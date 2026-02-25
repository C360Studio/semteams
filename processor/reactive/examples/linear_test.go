// Package examples provides example workflows demonstrating reactive patterns.
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

// LinearWorkflowState represents a simple linear workflow: start → process → complete.
type LinearWorkflowState struct {
	reactive.ExecutionState
	Input  string `json:"input"`
	Output string `json:"output,omitempty"`
}

// ProcessResult is the result of the processing step.
type ProcessResult struct {
	TaskID   string `json:"task_id"`
	Status   string `json:"status"`
	Computed string `json:"computed"`
}

func (r *ProcessResult) Schema() message.Type {
	return message.Type{Domain: "example", Category: "process-result", Version: "v1"}
}

func (r *ProcessResult) Validate() error { return nil }

func (r *ProcessResult) MarshalJSON() ([]byte, error) {
	type Alias ProcessResult
	return json.Marshal((*Alias)(r))
}

func (r *ProcessResult) UnmarshalJSON(data []byte) error {
	type Alias ProcessResult
	return json.Unmarshal(data, (*Alias)(r))
}

// ProcessRequest is sent to trigger processing.
type ProcessRequest struct {
	TaskID string `json:"task_id"`
	Input  string `json:"input"`
}

func (r *ProcessRequest) Schema() message.Type {
	return message.Type{Domain: "example", Category: "process-request", Version: "v1"}
}

func (r *ProcessRequest) Validate() error { return nil }

func (r *ProcessRequest) MarshalJSON() ([]byte, error) {
	type Alias ProcessRequest
	return json.Marshal((*Alias)(r))
}

func (r *ProcessRequest) UnmarshalJSON(data []byte) error {
	type Alias ProcessRequest
	return json.Unmarshal(data, (*Alias)(r))
}

// buildLinearWorkflow creates a simple linear workflow with three phases:
// 1. "pending" → triggers processing
// 2. "processing" → waits for async result
// 3. "completed" → final state
func buildLinearWorkflow() *reactive.Definition {
	return reactive.NewWorkflow("linear-example").
		WithDescription("Simple linear workflow: start → process → complete").
		WithStateBucket("EXAMPLE_STATE").
		WithStateFactory(func() any { return &LinearWorkflowState{} }).
		WithTimeout(5 * time.Minute).
		// Rule 1: Start processing when in pending phase
		AddRule(reactive.NewRule("start-processing").
			WatchKV("EXAMPLE_STATE", "linear-example.*").
			When("phase is pending", reactive.PhaseIs("pending")).
			When("no pending task", reactive.NoPendingTask()).
			PublishAsync(
				"processor.input",
				func(ctx *reactive.RuleContext) (message.Payload, error) {
					state := ctx.State.(*LinearWorkflowState)
					return &ProcessRequest{
						TaskID: state.ID,
						Input:  state.Input,
					}, nil
				},
				"example.process-result.v1",
				func(ctx *reactive.RuleContext, result any) error {
					state := ctx.State.(*LinearWorkflowState)
					if res, ok := result.(*ProcessResult); ok {
						state.Output = res.Computed
					}
					state.Phase = "completed"
					state.Status = reactive.StatusCompleted
					return nil
				},
			).
			MustBuild()).
		// Rule 2: Mark as completed when processing is done
		AddRule(reactive.NewRule("complete").
			WatchKV("EXAMPLE_STATE", "linear-example.*").
			When("phase is completed", reactive.PhaseIs("completed")).
			When("status is completed", reactive.StatusIs(reactive.StatusCompleted)).
			Complete().
			MustBuild()).
		MustBuild()
}

func TestLinearWorkflow_Definition(t *testing.T) {
	def := buildLinearWorkflow()

	if def.ID != "linear-example" {
		t.Errorf("Expected ID 'linear-example', got %q", def.ID)
	}
	if len(def.Rules) != 2 {
		t.Errorf("Expected 2 rules, got %d", len(def.Rules))
	}

	// Check first rule
	rule1 := def.Rules[0]
	if rule1.ID != "start-processing" {
		t.Errorf("Expected first rule ID 'start-processing', got %q", rule1.ID)
	}
	if rule1.Action.Type != reactive.ActionPublishAsync {
		t.Errorf("Expected first rule action type ActionPublishAsync, got %v", rule1.Action.Type)
	}

	// Check second rule
	rule2 := def.Rules[1]
	if rule2.ID != "complete" {
		t.Errorf("Expected second rule ID 'complete', got %q", rule2.ID)
	}
	if rule2.Action.Type != reactive.ActionComplete {
		t.Errorf("Expected second rule action type ActionComplete, got %v", rule2.Action.Type)
	}
}

func TestLinearWorkflow_StateFactory(t *testing.T) {
	def := buildLinearWorkflow()

	// Test state factory creates correct type
	state := def.StateFactory()
	if state == nil {
		t.Fatal("StateFactory returned nil")
	}

	_, ok := state.(*LinearWorkflowState)
	if !ok {
		t.Errorf("StateFactory returned wrong type: %T", state)
	}
}

func TestLinearWorkflow_ConditionEvaluation(t *testing.T) {
	def := buildLinearWorkflow()
	rule1 := &def.Rules[0]

	// Test conditions evaluate correctly
	t.Run("pending phase matches", func(t *testing.T) {
		state := &LinearWorkflowState{
			ExecutionState: reactive.ExecutionState{
				Phase:  "pending",
				Status: reactive.StatusRunning,
			},
			Input: "test input",
		}
		ctx := &reactive.RuleContext{State: state}

		// Check all conditions pass
		for _, cond := range rule1.Conditions {
			if !cond.Evaluate(ctx) {
				t.Errorf("Condition %q should pass for pending state", cond.Description)
			}
		}
	})

	t.Run("processing phase does not match", func(t *testing.T) {
		state := &LinearWorkflowState{
			ExecutionState: reactive.ExecutionState{
				Phase:  "processing",
				Status: reactive.StatusRunning,
			},
		}
		ctx := &reactive.RuleContext{State: state}

		// First condition (phase check) should fail
		if rule1.Conditions[0].Evaluate(ctx) {
			t.Error("PhaseIs('pending') should not match 'processing' phase")
		}
	})
}

func TestLinearWorkflow_PayloadBuilder(t *testing.T) {
	def := buildLinearWorkflow()
	rule1 := &def.Rules[0]

	state := &LinearWorkflowState{
		ExecutionState: reactive.ExecutionState{
			ID:     "exec-123",
			Phase:  "pending",
			Status: reactive.StatusRunning,
		},
		Input: "test input data",
	}
	ctx := &reactive.RuleContext{State: state}

	// Test payload builder produces correct output
	payload, err := rule1.Action.BuildPayload(ctx)
	if err != nil {
		t.Fatalf("BuildPayload failed: %v", err)
	}

	req, ok := payload.(*ProcessRequest)
	if !ok {
		t.Fatalf("Expected *ProcessRequest, got %T", payload)
	}

	if req.TaskID != "exec-123" {
		t.Errorf("Expected TaskID 'exec-123', got %q", req.TaskID)
	}
	if req.Input != "test input data" {
		t.Errorf("Expected Input 'test input data', got %q", req.Input)
	}
}

func TestLinearWorkflow_StateMutation(t *testing.T) {
	def := buildLinearWorkflow()
	rule1 := &def.Rules[0]

	state := &LinearWorkflowState{
		ExecutionState: reactive.ExecutionState{
			ID:     "exec-123",
			Phase:  "pending",
			Status: reactive.StatusRunning,
		},
		Input: "test input",
	}
	ctx := &reactive.RuleContext{State: state}

	// Simulate callback result
	result := &ProcessResult{
		TaskID:   "exec-123",
		Status:   "success",
		Computed: "processed output",
	}

	// Apply mutation
	err := rule1.Action.MutateState(ctx, result)
	if err != nil {
		t.Fatalf("MutateState failed: %v", err)
	}

	if state.Output != "processed output" {
		t.Errorf("Expected Output 'processed output', got %q", state.Output)
	}
	if state.Phase != "completed" {
		t.Errorf("Expected Phase 'completed', got %q", state.Phase)
	}
	if state.Status != reactive.StatusCompleted {
		t.Errorf("Expected Status StatusCompleted, got %v", state.Status)
	}
}

func TestLinearWorkflow_WithTestEngine(t *testing.T) {
	engine := testutil.NewTestEngine(t)
	def := buildLinearWorkflow()

	// Register workflow
	if err := engine.RegisterWorkflow(def); err != nil {
		t.Fatalf("RegisterWorkflow failed: %v", err)
	}

	// Create initial execution state
	state := &LinearWorkflowState{
		ExecutionState: reactive.ExecutionState{
			ID:         "exec-001",
			WorkflowID: "linear-example",
			Phase:      "pending",
			Status:     reactive.StatusRunning,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		},
		Input: "hello world",
	}

	// Trigger by storing state in KV
	key := "linear-example.exec-001"
	err := engine.TriggerKV(context.Background(), key, state)
	if err != nil {
		t.Fatalf("TriggerKV failed: %v", err)
	}

	// Verify state was stored
	engine.AssertPhase(key, "pending")
	engine.AssertStatus(key, reactive.StatusRunning)
}
