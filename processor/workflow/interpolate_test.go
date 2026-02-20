package workflow

import (
	"encoding/json"
	"testing"
	"time"

	wfschema "github.com/c360studio/semstreams/processor/workflow/schema"
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

	interpolator := newInterpolator(exec)

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

	interpolator := newInterpolator(exec)

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
				Output: json.RawMessage(`{"issues_count": 0, "severity": "low", "score": 85, "rejection_type": "misscoped"}`),
			},
			"analysis": {
				Status: "success",
				Output: json.RawMessage(`{"has_errors": true}`),
			},
		},
	}

	interpolator := newInterpolator(exec)

	tests := []struct {
		name      string
		condition wfschema.ConditionDef
		expected  bool
		wantErr   bool
	}{
		{
			name:      "eq true - number zero",
			condition: wfschema.ConditionDef{Field: "steps.review.output.issues_count", Operator: "eq", Value: 0},
			expected:  true,
		},
		{
			name:      "eq false - number mismatch",
			condition: wfschema.ConditionDef{Field: "steps.review.output.issues_count", Operator: "eq", Value: 5},
			expected:  false,
		},
		{
			name:      "eq true - string",
			condition: wfschema.ConditionDef{Field: "steps.review.output.severity", Operator: "eq", Value: "low"},
			expected:  true,
		},
		{
			name:      "ne true",
			condition: wfschema.ConditionDef{Field: "steps.review.output.issues_count", Operator: "ne", Value: 5},
			expected:  true,
		},
		{
			name:      "gt true",
			condition: wfschema.ConditionDef{Field: "steps.review.output.score", Operator: "gt", Value: 80},
			expected:  true,
		},
		{
			name:      "gt false",
			condition: wfschema.ConditionDef{Field: "steps.review.output.score", Operator: "gt", Value: 90},
			expected:  false,
		},
		{
			name:      "lt true",
			condition: wfschema.ConditionDef{Field: "steps.review.output.score", Operator: "lt", Value: 90},
			expected:  true,
		},
		{
			name:      "gte true - equal",
			condition: wfschema.ConditionDef{Field: "steps.review.output.score", Operator: "gte", Value: 85},
			expected:  true,
		},
		{
			name:      "gte true - greater",
			condition: wfschema.ConditionDef{Field: "steps.review.output.score", Operator: "gte", Value: 80},
			expected:  true,
		},
		{
			name:      "lte true",
			condition: wfschema.ConditionDef{Field: "steps.review.output.score", Operator: "lte", Value: 85},
			expected:  true,
		},
		{
			name:      "exists true",
			condition: wfschema.ConditionDef{Field: "steps.review.output.score", Operator: "exists"},
			expected:  true,
		},
		{
			name:      "exists false - missing field",
			condition: wfschema.ConditionDef{Field: "steps.review.output.missing", Operator: "exists"},
			expected:  false,
		},
		{
			name:      "not_exists true - missing field",
			condition: wfschema.ConditionDef{Field: "steps.review.output.missing", Operator: "not_exists"},
			expected:  true,
		},
		{
			name:      "not_exists false - field exists",
			condition: wfschema.ConditionDef{Field: "steps.review.output.score", Operator: "not_exists"},
			expected:  false,
		},
		{
			name:      "eq true - boolean",
			condition: wfschema.ConditionDef{Field: "steps.analysis.output.has_errors", Operator: "eq", Value: true},
			expected:  true,
		},
		{
			name:      "execution iteration",
			condition: wfschema.ConditionDef{Field: "execution.iteration", Operator: "eq", Value: 2},
			expected:  true,
		},
		{
			name:      "missing step",
			condition: wfschema.ConditionDef{Field: "steps.nonexistent.status", Operator: "eq", Value: "success"},
			expected:  false,
			wantErr:   true,
		},
		// in operator tests
		{
			name:      "in true - value in array",
			condition: wfschema.ConditionDef{Field: "steps.review.output.rejection_type", Operator: "in", Value: []any{"misscoped", "architectural"}},
			expected:  true,
		},
		{
			name:      "in false - value not in array",
			condition: wfschema.ConditionDef{Field: "steps.review.output.rejection_type", Operator: "in", Value: []any{"invalid", "incomplete"}},
			expected:  false,
		},
		{
			name:      "in true - string in array",
			condition: wfschema.ConditionDef{Field: "steps.review.output.severity", Operator: "in", Value: []any{"low", "medium", "high"}},
			expected:  true,
		},
		{
			name:      "in false - empty array",
			condition: wfschema.ConditionDef{Field: "steps.review.output.severity", Operator: "in", Value: []any{}},
			expected:  false,
		},
		{
			name:      "in error - non-array value",
			condition: wfschema.ConditionDef{Field: "steps.review.output.severity", Operator: "in", Value: "not-an-array"},
			expected:  false,
			wantErr:   true,
		},
		// not_in operator tests
		{
			name:      "not_in true - value not in array",
			condition: wfschema.ConditionDef{Field: "steps.review.output.rejection_type", Operator: "not_in", Value: []any{"invalid", "incomplete"}},
			expected:  true,
		},
		{
			name:      "not_in false - value in array",
			condition: wfschema.ConditionDef{Field: "steps.review.output.rejection_type", Operator: "not_in", Value: []any{"misscoped", "architectural"}},
			expected:  false,
		},
		{
			name:      "not_in true - empty array",
			condition: wfschema.ConditionDef{Field: "steps.review.output.severity", Operator: "not_in", Value: []any{}},
			expected:  true,
		},
		{
			name:      "not_in error - non-array value",
			condition: wfschema.ConditionDef{Field: "steps.review.output.severity", Operator: "not_in", Value: "not-an-array"},
			expected:  false,
			wantErr:   true,
		},
		// numeric in/not_in tests
		{
			name:      "in true - numeric value in array",
			condition: wfschema.ConditionDef{Field: "steps.review.output.score", Operator: "in", Value: []any{80, 85, 90}},
			expected:  true,
		},
		{
			name:      "in false - numeric value not in array",
			condition: wfschema.ConditionDef{Field: "steps.review.output.score", Operator: "in", Value: []any{80, 90, 95}},
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := interpolator.EvaluateCondition(&tt.condition)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but evaluation succeeded")
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
	interpolator := newInterpolator(exec)

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

	interpolator := newInterpolator(exec)

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
	interpolator := newInterpolator(exec)

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

// TestInterpolateTriggerPayloadStructFields tests that struct fields in TriggerPayload
// are accessible via ${trigger.payload.*} interpolation and take precedence over Data fields.
func TestInterpolateTriggerPayloadStructFields(t *testing.T) {
	// Build a merged payload with struct fields taking precedence over Data
	trigger := &TriggerPayload{
		WorkflowID:  "wf-123",
		Role:        "developer",
		Model:       "claude-3-opus",
		Prompt:      "Write tests",
		UserID:      "user-456",
		ChannelType: "cli",
		ChannelID:   "session-789",
		RequestID:   "req-abc",
		Data:        json.RawMessage(`{"custom_field": "custom_value", "role": "should_be_overwritten"}`),
	}

	mergedPayload, err := buildMergedPayload(trigger)
	if err != nil {
		t.Fatalf("failed to build merged payload: %v", err)
	}

	exec := &Execution{
		ID:         "exec_123",
		WorkflowID: "wf-123",
		Trigger: TriggerContext{
			Subject:   "workflow.trigger.wf-123",
			Payload:   mergedPayload,
			Timestamp: time.Now(),
		},
		StepResults: make(map[string]StepResult),
	}

	interpolator := newInterpolator(exec)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "struct field - role takes precedence",
			input:    "${trigger.payload.role}",
			expected: "developer", // struct field takes precedence over Data
		},
		{
			name:     "struct field - model",
			input:    "${trigger.payload.model}",
			expected: "claude-3-opus",
		},
		{
			name:     "struct field - prompt",
			input:    "${trigger.payload.prompt}",
			expected: "Write tests",
		},
		{
			name:     "struct field - user_id",
			input:    "${trigger.payload.user_id}",
			expected: "user-456",
		},
		{
			name:     "struct field - channel_type",
			input:    "${trigger.payload.channel_type}",
			expected: "cli",
		},
		{
			name:     "struct field - channel_id",
			input:    "${trigger.payload.channel_id}",
			expected: "session-789",
		},
		{
			name:     "struct field - request_id",
			input:    "${trigger.payload.request_id}",
			expected: "req-abc",
		},
		{
			name:     "data field - custom",
			input:    "${trigger.payload.custom_field}",
			expected: "custom_value",
		},
		{
			name:     "combined struct and data fields",
			input:    "Role: ${trigger.payload.role}, Custom: ${trigger.payload.custom_field}",
			expected: "Role: developer, Custom: custom_value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := interpolator.InterpolateString(tt.input)
			if err != nil {
				t.Logf("interpolation had error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("got %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestBuildMergedPayload tests the buildMergedPayload helper function.
func TestBuildMergedPayload(t *testing.T) {
	tests := []struct {
		name     string
		trigger  *TriggerPayload
		checkKey string
		expected any
	}{
		{
			name: "struct fields only",
			trigger: &TriggerPayload{
				WorkflowID: "wf-1",
				Role:       "architect",
				Model:      "gpt-4",
			},
			checkKey: "role",
			expected: "architect",
		},
		{
			name: "struct takes precedence over data",
			trigger: &TriggerPayload{
				WorkflowID: "wf-1",
				Role:       "architect",
				Data:       json.RawMessage(`{"role": "developer"}`),
			},
			checkKey: "role",
			expected: "architect",
		},
		{
			name: "data field preserved",
			trigger: &TriggerPayload{
				WorkflowID: "wf-1",
				Data:       json.RawMessage(`{"custom": "value"}`),
			},
			checkKey: "custom",
			expected: "value",
		},
		{
			name: "empty trigger",
			trigger: &TriggerPayload{
				WorkflowID: "wf-1",
			},
			checkKey: "workflow_id",
			expected: "wf-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merged, err := buildMergedPayload(tt.trigger)
			if err != nil {
				t.Fatalf("buildMergedPayload failed: %v", err)
			}

			var result map[string]any
			if err := json.Unmarshal(merged, &result); err != nil {
				t.Fatalf("failed to unmarshal result: %v", err)
			}

			if result[tt.checkKey] != tt.expected {
				t.Errorf("got %v, want %v", result[tt.checkKey], tt.expected)
			}
		})
	}
}
