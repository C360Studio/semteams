package subjects

import (
	"encoding/json"
	"testing"
	"time"
)

func TestWorkflowStartedEvent_Serialization(t *testing.T) {
	event := WorkflowStartedEvent{
		ExecutionID:  "exec-123",
		WorkflowID:   "wf-456",
		WorkflowName: "test-workflow",
		StartedAt:    time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
	}

	// Test serialization using subject's codec
	data, err := WorkflowStarted.Codec.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Verify JSON structure
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("JSON parsing failed: %v", err)
	}

	if parsed["execution_id"] != "exec-123" {
		t.Errorf("Expected execution_id 'exec-123', got %v", parsed["execution_id"])
	}
	if parsed["workflow_id"] != "wf-456" {
		t.Errorf("Expected workflow_id 'wf-456', got %v", parsed["workflow_id"])
	}
	if parsed["workflow_name"] != "test-workflow" {
		t.Errorf("Expected workflow_name 'test-workflow', got %v", parsed["workflow_name"])
	}
}

func TestWorkflowCompletedEvent_Serialization(t *testing.T) {
	event := WorkflowCompletedEvent{
		ExecutionID:  "exec-123",
		WorkflowID:   "wf-456",
		WorkflowName: "test-workflow",
		Iterations:   3,
		CompletedAt:  time.Date(2024, 1, 15, 10, 35, 0, 0, time.UTC),
	}

	data, err := WorkflowCompleted.Codec.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded WorkflowCompletedEvent
	if err := WorkflowCompleted.Codec.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.ExecutionID != "exec-123" {
		t.Errorf("Expected execution_id 'exec-123', got %s", decoded.ExecutionID)
	}
	if decoded.Iterations != 3 {
		t.Errorf("Expected iterations 3, got %d", decoded.Iterations)
	}
}

func TestWorkflowFailedEvent_Serialization(t *testing.T) {
	event := WorkflowFailedEvent{
		ExecutionID: "exec-789",
		WorkflowID:  "wf-456",
		Error:       "step timeout exceeded",
		Iterations:  2,
		FailedAt:    time.Now().UTC(),
	}

	data, err := WorkflowFailed.Codec.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded WorkflowFailedEvent
	if err := WorkflowFailed.Codec.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Error != "step timeout exceeded" {
		t.Errorf("Expected error 'step timeout exceeded', got %s", decoded.Error)
	}
}

func TestStepCompletedEvent_Serialization(t *testing.T) {
	event := StepCompletedEvent{
		ExecutionID: "exec-123",
		WorkflowID:  "wf-456",
		StepName:    "process_data",
		Status:      "success",
		Iteration:   1,
		CompletedAt: time.Now().UTC(),
	}

	data, err := StepCompleted.Codec.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded StepCompletedEvent
	if err := StepCompleted.Codec.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.StepName != "process_data" {
		t.Errorf("Expected step_name 'process_data', got %s", decoded.StepName)
	}
	if decoded.Status != "success" {
		t.Errorf("Expected status 'success', got %s", decoded.Status)
	}
}

func TestSubjectPatterns(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
	}{
		{"WorkflowStarted", WorkflowStarted.Pattern},
		{"WorkflowCompleted", WorkflowCompleted.Pattern},
		{"WorkflowFailed", WorkflowFailed.Pattern},
		{"WorkflowTimedOut", WorkflowTimedOut.Pattern},
		{"StepStarted", StepStarted.Pattern},
		{"StepCompleted", StepCompleted.Pattern},
		{"WorkflowEvents", WorkflowEvents.Pattern},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.pattern == "" {
				t.Errorf("%s has empty pattern", tt.name)
			}
		})
	}

	// Verify specific patterns
	if WorkflowStarted.Pattern != "workflow.events.started" {
		t.Errorf("WorkflowStarted pattern: expected 'workflow.events.started', got %s", WorkflowStarted.Pattern)
	}
	if WorkflowEvents.Pattern != "workflow.events.>" {
		t.Errorf("WorkflowEvents pattern: expected 'workflow.events.>', got %s", WorkflowEvents.Pattern)
	}
}

func TestEventRoundTrip(t *testing.T) {
	// Test that all event types survive a marshal/unmarshal cycle

	t.Run("WorkflowStartedEvent", func(t *testing.T) {
		original := WorkflowStartedEvent{
			ExecutionID:  "exec-1",
			WorkflowID:   "wf-1",
			WorkflowName: "test",
			StartedAt:    time.Now().UTC().Truncate(time.Second),
		}

		data, _ := WorkflowStarted.Codec.Marshal(original)
		var decoded WorkflowStartedEvent
		_ = WorkflowStarted.Codec.Unmarshal(data, &decoded)

		if decoded.ExecutionID != original.ExecutionID {
			t.Errorf("ExecutionID mismatch")
		}
		if !decoded.StartedAt.Equal(original.StartedAt) {
			t.Errorf("StartedAt mismatch")
		}
	})

	t.Run("WorkflowCompletedEvent", func(t *testing.T) {
		original := WorkflowCompletedEvent{
			ExecutionID: "exec-2",
			WorkflowID:  "wf-2",
			Iterations:  5,
			CompletedAt: time.Now().UTC().Truncate(time.Second),
		}

		data, _ := WorkflowCompleted.Codec.Marshal(original)
		var decoded WorkflowCompletedEvent
		_ = WorkflowCompleted.Codec.Unmarshal(data, &decoded)

		if decoded.Iterations != original.Iterations {
			t.Errorf("Iterations mismatch: expected %d, got %d", original.Iterations, decoded.Iterations)
		}
	})

	t.Run("StepCompletedEvent", func(t *testing.T) {
		original := StepCompletedEvent{
			ExecutionID: "exec-3",
			WorkflowID:  "wf-3",
			StepName:    "validate",
			Status:      "skipped",
			Iteration:   2,
			CompletedAt: time.Now().UTC().Truncate(time.Second),
		}

		data, _ := StepCompleted.Codec.Marshal(original)
		var decoded StepCompletedEvent
		_ = StepCompleted.Codec.Unmarshal(data, &decoded)

		if decoded.Status != "skipped" {
			t.Errorf("Status mismatch: expected 'skipped', got %s", decoded.Status)
		}
	})
}
