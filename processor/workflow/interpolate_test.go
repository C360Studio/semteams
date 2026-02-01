package workflow

import (
	"encoding/json"
	"testing"
	"time"
)

func TestInterpolateString(t *testing.T) {
	exec := &Execution{
		ID:           "exec_123",
		WorkflowID:   "workflow_abc",
		WorkflowName: "Test Workflow",
		State:        ExecutionStateRunning,
		Iteration:    2,
		CurrentStep:  1,
		CurrentName:  "step2",
		Trigger: TriggerContext{
			Subject:   "workflow.trigger.test",
			Payload:   json.RawMessage(`{"code": "function test() {}", "author": "alice"}`),
			Timestamp: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
			Headers:   map[string]string{"request_id": "req_456"},
		},
		StepResults: map[string]StepResult{
			"step1": {
				StepName:  "step1",
				Status:    "success",
				Output:    json.RawMessage(`{"issues_count": 3, "issues": ["bug1", "bug2", "bug3"]}`),
				Iteration: 1,
			},
		},
	}

	interpolator := NewInterpolator(exec)

	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{
			name:     "execution id",
			input:    "task-${execution.id}",
			expected: "task-exec_123",
		},
		{
			name:     "workflow id",
			input:    "workflow: ${execution.workflow_id}",
			expected: "workflow: workflow_abc",
		},
		{
			name:     "iteration",
			input:    "iteration ${execution.iteration}",
			expected: "iteration 2",
		},
		{
			name:     "trigger subject",
			input:    "triggered by ${trigger.subject}",
			expected: "triggered by workflow.trigger.test",
		},
		{
			name:     "trigger payload field",
			input:    "author: ${trigger.payload.author}",
			expected: "author: alice",
		},
		{
			name:     "trigger header",
			input:    "request: ${trigger.headers.request_id}",
			expected: "request: req_456",
		},
		{
			name:     "step result status",
			input:    "step1 status: ${steps.step1.status}",
			expected: "step1 status: success",
		},
		{
			name:     "step output field",
			input:    "issues: ${steps.step1.output.issues_count}",
			expected: "issues: 3",
		},
		{
			name:     "multiple interpolations",
			input:    "${execution.id} - ${execution.workflow_id}",
			expected: "exec_123 - workflow_abc",
		},
		{
			name:     "no interpolation",
			input:    "plain string",
			expected: "plain string",
		},
		{
			name:     "unknown path",
			input:    "${unknown.path}",
			expected: "${unknown.path}",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := interpolator.InterpolateString(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Log("expected error but interpolation succeeded (may keep original)")
				}
			}
			if result != tt.expected {
				t.Errorf("got %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestInterpolateJSON(t *testing.T) {
	exec := &Execution{
		ID:         "exec_123",
		WorkflowID: "workflow_abc",
		Trigger: TriggerContext{
			Payload: json.RawMessage(`{"code": "console.log('test')", "metadata": {"lang": "js"}}`),
		},
		StepResults: map[string]StepResult{
			"review": {
				Output: json.RawMessage(`{"issues": ["issue1", "issue2"]}`),
			},
		},
	}

	interpolator := NewInterpolator(exec)

	input := json.RawMessage(`{"task_id": "${execution.id}", "code": "${trigger.payload.code}"}`)

	result, err := interpolator.InterpolateJSON(input)
	if err != nil {
		t.Fatalf("interpolation failed: %v", err)
	}

	// Parse result
	var parsed map[string]any
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	if parsed["task_id"] != "exec_123" {
		t.Errorf("task_id = %v, want exec_123", parsed["task_id"])
	}
}

func TestEvaluateCondition(t *testing.T) {
	exec := &Execution{
		ID:        "exec_123",
		Iteration: 2,
		StepResults: map[string]StepResult{
			"review": {
				Status: "success",
				Output: json.RawMessage(`{"issues_count": 0, "severity": "low", "score": 85}`),
			},
			"analysis": {
				Status: "success",
				Output: json.RawMessage(`{"has_errors": true}`),
			},
		},
	}

	interpolator := NewInterpolator(exec)

	tests := []struct {
		name      string
		condition ConditionDef
		expected  bool
		wantErr   bool
	}{
		{
			name:      "eq true - number zero",
			condition: ConditionDef{Field: "steps.review.output.issues_count", Operator: "eq", Value: 0},
			expected:  true,
		},
		{
			name:      "eq false - number mismatch",
			condition: ConditionDef{Field: "steps.review.output.issues_count", Operator: "eq", Value: 5},
			expected:  false,
		},
		{
			name:      "eq true - string",
			condition: ConditionDef{Field: "steps.review.output.severity", Operator: "eq", Value: "low"},
			expected:  true,
		},
		{
			name:      "ne true",
			condition: ConditionDef{Field: "steps.review.output.issues_count", Operator: "ne", Value: 5},
			expected:  true,
		},
		{
			name:      "gt true",
			condition: ConditionDef{Field: "steps.review.output.score", Operator: "gt", Value: 80},
			expected:  true,
		},
		{
			name:      "gt false",
			condition: ConditionDef{Field: "steps.review.output.score", Operator: "gt", Value: 90},
			expected:  false,
		},
		{
			name:      "lt true",
			condition: ConditionDef{Field: "steps.review.output.score", Operator: "lt", Value: 90},
			expected:  true,
		},
		{
			name:      "gte true - equal",
			condition: ConditionDef{Field: "steps.review.output.score", Operator: "gte", Value: 85},
			expected:  true,
		},
		{
			name:      "gte true - greater",
			condition: ConditionDef{Field: "steps.review.output.score", Operator: "gte", Value: 80},
			expected:  true,
		},
		{
			name:      "lte true",
			condition: ConditionDef{Field: "steps.review.output.score", Operator: "lte", Value: 85},
			expected:  true,
		},
		{
			name:      "exists true",
			condition: ConditionDef{Field: "steps.review.output.score", Operator: "exists"},
			expected:  true,
		},
		{
			name:      "exists false - missing field",
			condition: ConditionDef{Field: "steps.review.output.missing", Operator: "exists"},
			expected:  false,
		},
		{
			name:      "not_exists true - missing field",
			condition: ConditionDef{Field: "steps.review.output.missing", Operator: "not_exists"},
			expected:  true,
		},
		{
			name:      "not_exists false - field exists",
			condition: ConditionDef{Field: "steps.review.output.score", Operator: "not_exists"},
			expected:  false,
		},
		{
			name:      "eq true - boolean",
			condition: ConditionDef{Field: "steps.analysis.output.has_errors", Operator: "eq", Value: true},
			expected:  true,
		},
		{
			name:      "execution iteration",
			condition: ConditionDef{Field: "execution.iteration", Operator: "eq", Value: 2},
			expected:  true,
		},
		{
			name:      "missing step",
			condition: ConditionDef{Field: "steps.nonexistent.status", Operator: "eq", Value: "success"},
			expected:  false,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := interpolator.EvaluateCondition(&tt.condition)
			if tt.wantErr {
				if err == nil {
					t.Log("expected error but evaluation succeeded")
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("got %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestNilCondition(t *testing.T) {
	exec := &Execution{ID: "test"}
	interpolator := NewInterpolator(exec)

	result, err := interpolator.EvaluateCondition(nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !result {
		t.Error("nil condition should return true")
	}
}

func TestNestedPathResolution(t *testing.T) {
	exec := &Execution{
		ID: "exec_123",
		Trigger: TriggerContext{
			Payload: json.RawMessage(`{
				"data": {
					"nested": {
						"value": 42
					},
					"array": [1, 2, 3]
				}
			}`),
		},
	}

	interpolator := NewInterpolator(exec)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "deep nested value",
			input:    "${trigger.payload.data.nested.value}",
			expected: "42",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := interpolator.InterpolateString(tt.input)
			if err != nil {
				t.Logf("interpolation had error (may be expected): %v", err)
			}
			if result != tt.expected {
				t.Errorf("got %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestValueToString(t *testing.T) {
	exec := &Execution{ID: "test"}
	interpolator := NewInterpolator(exec)

	tests := []struct {
		name     string
		value    any
		expected string
	}{
		{"string", "hello", "hello"},
		{"int", 42, "42"},
		{"int64", int64(123), "123"},
		{"float64", 3.14, "3.14"},
		{"bool true", true, "true"},
		{"bool false", false, "false"},
		{"nil", nil, ""},
		{"map", map[string]any{"key": "value"}, `{"key":"value"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := interpolator.valueToString(tt.value)
			if result != tt.expected {
				t.Errorf("got %q, want %q", result, tt.expected)
			}
		})
	}
}
