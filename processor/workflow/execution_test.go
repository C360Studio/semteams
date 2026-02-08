package workflow

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNewExecution(t *testing.T) {
	trigger := TriggerContext{
		Subject:   "workflow.trigger.test",
		Payload:   json.RawMessage(`{"key": "value"}`),
		Timestamp: time.Now(),
	}

	exec := NewExecution("workflow-123", "Test Workflow", trigger, 10*time.Minute)

	if exec.ID == "" {
		t.Error("execution ID should not be empty")
	}
	if exec.WorkflowID != "workflow-123" {
		t.Errorf("WorkflowID = %q, want %q", exec.WorkflowID, "workflow-123")
	}
	if exec.WorkflowName != "Test Workflow" {
		t.Errorf("WorkflowName = %q, want %q", exec.WorkflowName, "Test Workflow")
	}
	if exec.State != ExecutionStatePending {
		t.Errorf("State = %v, want %v", exec.State, ExecutionStatePending)
	}
	if exec.CurrentStep != 0 {
		t.Errorf("CurrentStep = %d, want 0", exec.CurrentStep)
	}
	if exec.Iteration != 1 {
		t.Errorf("Iteration = %d, want 1", exec.Iteration)
	}
	if exec.StepResults == nil {
		t.Error("StepResults should be initialized")
	}
	if exec.StartedAt.IsZero() {
		t.Error("StartedAt should be set")
	}
	if exec.Deadline.IsZero() {
		t.Error("Deadline should be set")
	}
}

func TestExecutionStateTransitions(t *testing.T) {
	trigger := TriggerContext{Subject: "test"}
	exec := NewExecution("wf-1", "Test", trigger, time.Hour)

	// Initially pending
	if exec.State != ExecutionStatePending {
		t.Errorf("initial state = %v, want pending", exec.State)
	}

	// Mark running
	exec.MarkRunning()
	if exec.State != ExecutionStateRunning {
		t.Errorf("after MarkRunning, state = %v, want running", exec.State)
	}

	// Mark completed
	exec.MarkCompleted()
	if exec.State != ExecutionStateCompleted {
		t.Errorf("after MarkCompleted, state = %v, want completed", exec.State)
	}
	if exec.CompletedAt == nil {
		t.Error("CompletedAt should be set after completion")
	}
}

func TestExecutionMarkFailed(t *testing.T) {
	trigger := TriggerContext{Subject: "test"}
	exec := NewExecution("wf-1", "Test", trigger, time.Hour)
	exec.MarkRunning()

	errMsg := "step failed: timeout"
	exec.MarkFailed(errMsg)

	if exec.State != ExecutionStateFailed {
		t.Errorf("state = %v, want failed", exec.State)
	}
	if exec.Error != errMsg {
		t.Errorf("Error = %q, want %q", exec.Error, errMsg)
	}
	if exec.CompletedAt == nil {
		t.Error("CompletedAt should be set after failure")
	}
}

func TestExecutionMarkTimedOut(t *testing.T) {
	trigger := TriggerContext{Subject: "test"}
	exec := NewExecution("wf-1", "Test", trigger, time.Hour)
	exec.MarkRunning()

	exec.MarkTimedOut()

	if exec.State != ExecutionStateTimedOut {
		t.Errorf("state = %v, want timed_out", exec.State)
	}
	if exec.Error != "workflow timeout exceeded" {
		t.Errorf("Error = %q, want 'workflow timeout exceeded'", exec.Error)
	}
}

func TestExecutionIsTimedOut(t *testing.T) {
	trigger := TriggerContext{Subject: "test"}

	// Test with short timeout (already expired)
	exec := NewExecution("wf-1", "Test", trigger, -time.Second)
	if !exec.IsTimedOut() {
		t.Error("execution with past deadline should be timed out")
	}

	// Test with long timeout
	exec2 := NewExecution("wf-2", "Test", trigger, time.Hour)
	if exec2.IsTimedOut() {
		t.Error("execution with future deadline should not be timed out")
	}
}

func TestExecutionRecordStepResult(t *testing.T) {
	trigger := TriggerContext{Subject: "test"}
	exec := NewExecution("wf-1", "Test", trigger, time.Hour)

	result := StepResult{
		StepName:    "step1",
		Status:      "success",
		Output:      json.RawMessage(`{"data": "output"}`),
		StartedAt:   time.Now().Add(-time.Second),
		CompletedAt: time.Now(),
		Duration:    time.Second,
		Iteration:   1,
	}

	exec.RecordStepResult("step1", result)

	recorded, ok := exec.StepResults["step1"]
	if !ok {
		t.Fatal("step result not found")
	}
	if recorded.Status != "success" {
		t.Errorf("Status = %q, want 'success'", recorded.Status)
	}
	if recorded.Iteration != 1 {
		t.Errorf("Iteration = %d, want 1", recorded.Iteration)
	}
}

func TestExecutionIncrementIteration(t *testing.T) {
	trigger := TriggerContext{Subject: "test"}
	exec := NewExecution("wf-1", "Test", trigger, time.Hour)

	if exec.Iteration != 1 {
		t.Errorf("initial Iteration = %d, want 1", exec.Iteration)
	}

	exec.IncrementIteration()
	if exec.Iteration != 2 {
		t.Errorf("after increment, Iteration = %d, want 2", exec.Iteration)
	}

	exec.IncrementIteration()
	if exec.Iteration != 3 {
		t.Errorf("after second increment, Iteration = %d, want 3", exec.Iteration)
	}
}

func TestExecutionStateIsTerminal(t *testing.T) {
	tests := []struct {
		state    ExecutionState
		terminal bool
	}{
		{ExecutionStatePending, false},
		{ExecutionStateRunning, false},
		{ExecutionStateCompleted, true},
		{ExecutionStateFailed, true},
		{ExecutionStateTimedOut, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			if tt.state.IsTerminal() != tt.terminal {
				t.Errorf("%s.IsTerminal() = %v, want %v", tt.state, tt.state.IsTerminal(), tt.terminal)
			}
		})
	}
}

func TestExecutionJSONRoundTrip(t *testing.T) {
	trigger := TriggerContext{
		Subject:   "workflow.trigger.test",
		Payload:   json.RawMessage(`{"input": "data"}`),
		Timestamp: time.Now().Truncate(time.Second),
		Headers:   map[string]string{"trace_id": "abc123"},
	}

	original := NewExecution("wf-123", "Test Workflow", trigger, 10*time.Minute)
	original.MarkRunning()
	original.RecordStepResult("step1", StepResult{
		StepName:    "step1",
		Status:      "success",
		Output:      json.RawMessage(`{"result": 42}`),
		StartedAt:   time.Now().Add(-time.Second).Truncate(time.Second),
		CompletedAt: time.Now().Truncate(time.Second),
		Duration:    time.Second,
		Iteration:   1,
	})

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal execution: %v", err)
	}

	var decoded Execution
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal execution: %v", err)
	}

	if decoded.ID != original.ID {
		t.Errorf("ID = %q, want %q", decoded.ID, original.ID)
	}
	if decoded.WorkflowID != original.WorkflowID {
		t.Errorf("WorkflowID = %q, want %q", decoded.WorkflowID, original.WorkflowID)
	}
	if decoded.State != original.State {
		t.Errorf("State = %v, want %v", decoded.State, original.State)
	}
	if decoded.Iteration != original.Iteration {
		t.Errorf("Iteration = %d, want %d", decoded.Iteration, original.Iteration)
	}
	if len(decoded.StepResults) != len(original.StepResults) {
		t.Errorf("StepResults count = %d, want %d", len(decoded.StepResults), len(original.StepResults))
	}
}

func TestEvent(t *testing.T) {
	event := Event{
		Type:        "completed",
		ExecutionID: "exec_123",
		WorkflowID:  "workflow_abc",
		StepName:    "final_step",
		Iteration:   3,
		State:       ExecutionStateCompleted,
		Timestamp:   time.Now(),
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal event: %v", err)
	}

	var decoded Event
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal event: %v", err)
	}

	if decoded.Type != event.Type {
		t.Errorf("Type = %q, want %q", decoded.Type, event.Type)
	}
	if decoded.ExecutionID != event.ExecutionID {
		t.Errorf("ExecutionID = %q, want %q", decoded.ExecutionID, event.ExecutionID)
	}
	if decoded.Iteration != event.Iteration {
		t.Errorf("Iteration = %d, want %d", decoded.Iteration, event.Iteration)
	}
}

func TestStepCompleteMessage(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	msg := StepCompleteMessage{
		ExecutionID: "exec_123",
		StepName:    "review",
		Status:      "success",
		StartedAt:   now.Add(-time.Second),
		CompletedAt: now,
		Duration:    "1s",
		Iteration:   1,
		Output:      json.RawMessage(`{"issues": []}`),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal message: %v", err)
	}

	var decoded StepCompleteMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal message: %v", err)
	}

	if decoded.ExecutionID != msg.ExecutionID {
		t.Errorf("ExecutionID = %q, want %q", decoded.ExecutionID, msg.ExecutionID)
	}
	if decoded.StepName != msg.StepName {
		t.Errorf("StepName = %q, want %q", decoded.StepName, msg.StepName)
	}
	if decoded.Status != msg.Status {
		t.Errorf("Status = %q, want %q", decoded.Status, msg.Status)
	}
	if decoded.Duration != msg.Duration {
		t.Errorf("Duration = %q, want %q", decoded.Duration, msg.Duration)
	}
	if decoded.Iteration != msg.Iteration {
		t.Errorf("Iteration = %d, want %d", decoded.Iteration, msg.Iteration)
	}
}

func TestTriggerPayloadValidation(t *testing.T) {
	tests := []struct {
		name    string
		payload TriggerPayload
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid minimal",
			payload: TriggerPayload{WorkflowID: "test-workflow"},
			wantErr: false,
		},
		{
			name: "valid with all fields",
			payload: TriggerPayload{
				WorkflowID:  "test-workflow",
				Role:        "architect",
				Model:       "claude-3-opus",
				Prompt:      "Design a feature",
				UserID:      "user-123",
				ChannelType: "slack",
				ChannelID:   "C12345",
				RequestID:   "req-abc",
				Data:        json.RawMessage(`{"custom": "data"}`),
			},
			wantErr: false,
		},
		{
			name:    "missing workflow_id",
			payload: TriggerPayload{Role: "test"},
			wantErr: true,
			errMsg:  "workflow_id required",
		},
		{
			name:    "invalid data json",
			payload: TriggerPayload{WorkflowID: "test", Data: json.RawMessage(`{invalid}`)},
			wantErr: true,
			errMsg:  "data must be valid JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.payload.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && err != nil && tt.errMsg != "" {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %q, want to contain %q", err.Error(), tt.errMsg)
				}
			}
		})
	}
}

func TestStepCompleteMessageValidation(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name    string
		msg     StepCompleteMessage
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid with all fields",
			msg: StepCompleteMessage{
				ExecutionID: "exec-1",
				StepName:    "step1",
				Status:      "success",
				StartedAt:   now.Add(-time.Second),
				CompletedAt: now,
				Duration:    "1s",
				Iteration:   1,
				Output:      json.RawMessage(`{"result": "ok"}`),
			},
			wantErr: false,
		},
		{
			name: "valid failed status",
			msg: StepCompleteMessage{
				ExecutionID: "exec-1",
				StepName:    "step1",
				Status:      "failed",
				StartedAt:   now.Add(-time.Second),
				CompletedAt: now,
				Duration:    "1s",
				Iteration:   1,
				Error:       "something went wrong",
			},
			wantErr: false,
		},
		{
			name: "missing execution_id",
			msg: StepCompleteMessage{
				StepName:    "step1",
				Status:      "success",
				StartedAt:   now.Add(-time.Second),
				CompletedAt: now,
				Duration:    "1s",
				Iteration:   1,
			},
			wantErr: true,
			errMsg:  "execution_id required",
		},
		{
			name: "missing step_name",
			msg: StepCompleteMessage{
				ExecutionID: "exec-1",
				Status:      "success",
				StartedAt:   now.Add(-time.Second),
				CompletedAt: now,
				Duration:    "1s",
				Iteration:   1,
			},
			wantErr: true,
			errMsg:  "step_name required",
		},
		{
			name: "invalid status",
			msg: StepCompleteMessage{
				ExecutionID: "exec-1",
				StepName:    "step1",
				Status:      "pending",
				StartedAt:   now.Add(-time.Second),
				CompletedAt: now,
				Duration:    "1s",
				Iteration:   1,
			},
			wantErr: true,
			errMsg:  "status must be one of",
		},
		{
			name: "missing started_at",
			msg: StepCompleteMessage{
				ExecutionID: "exec-1",
				StepName:    "step1",
				Status:      "success",
				CompletedAt: now,
				Duration:    "1s",
				Iteration:   1,
			},
			wantErr: true,
			errMsg:  "started_at required",
		},
		{
			name: "missing completed_at",
			msg: StepCompleteMessage{
				ExecutionID: "exec-1",
				StepName:    "step1",
				Status:      "success",
				StartedAt:   now.Add(-time.Second),
				Duration:    "1s",
				Iteration:   1,
			},
			wantErr: true,
			errMsg:  "completed_at required",
		},
		{
			name: "completed_at before started_at",
			msg: StepCompleteMessage{
				ExecutionID: "exec-1",
				StepName:    "step1",
				Status:      "success",
				StartedAt:   now,
				CompletedAt: now.Add(-time.Second),
				Duration:    "1s",
				Iteration:   1,
			},
			wantErr: true,
			errMsg:  "completed_at cannot be before started_at",
		},
		{
			name: "missing duration",
			msg: StepCompleteMessage{
				ExecutionID: "exec-1",
				StepName:    "step1",
				Status:      "success",
				StartedAt:   now.Add(-time.Second),
				CompletedAt: now,
				Iteration:   1,
			},
			wantErr: true,
			errMsg:  "duration required",
		},
		{
			name: "invalid duration format",
			msg: StepCompleteMessage{
				ExecutionID: "exec-1",
				StepName:    "step1",
				Status:      "success",
				StartedAt:   now.Add(-time.Second),
				CompletedAt: now,
				Duration:    "invalid",
				Iteration:   1,
			},
			wantErr: true,
			errMsg:  "duration must be valid",
		},
		{
			name: "zero iteration",
			msg: StepCompleteMessage{
				ExecutionID: "exec-1",
				StepName:    "step1",
				Status:      "success",
				StartedAt:   now.Add(-time.Second),
				CompletedAt: now,
				Duration:    "1s",
				Iteration:   0,
			},
			wantErr: true,
			errMsg:  "iteration must be >= 1",
		},
		{
			name: "negative iteration",
			msg: StepCompleteMessage{
				ExecutionID: "exec-1",
				StepName:    "step1",
				Status:      "success",
				StartedAt:   now.Add(-time.Second),
				CompletedAt: now,
				Duration:    "1s",
				Iteration:   -1,
			},
			wantErr: true,
			errMsg:  "iteration must be >= 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.msg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && err != nil && tt.errMsg != "" {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %q, want to contain %q", err.Error(), tt.errMsg)
				}
			}
		})
	}
}
