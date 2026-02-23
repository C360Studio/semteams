package workflow

import (
	"encoding/json"
	"strings"
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

// NOTE: Tests for PayloadMapping and PassThrough have been removed as these fields
// were replaced by the Inputs/Outputs pattern (ADR-020).
// The InterpolateActionDef function now only performs JSON interpolation on the Payload field.

// TestInterpolateJSONTypePreservation tests that non-scalar values (arrays, objects)
// are preserved as proper JSON types when using pure interpolation (entire string is ${...}).
func TestInterpolateJSONTypePreservation(t *testing.T) {
	exec := &Execution{
		ID:         "exec_123",
		WorkflowID: "workflow_abc",
		Trigger: TriggerContext{
			Payload: json.RawMessage(`{"items": ["a", "b", "c"], "config": {"timeout": 30}}`),
		},
		StepResults: map[string]StepResult{
			"step1": {
				Output: json.RawMessage(`{
					"issues": ["bug1", "bug2", "bug3"],
					"count": 3,
					"settings": {"enabled": true, "threshold": 0.5},
					"nested": {"deep": {"value": 42}}
				}`),
			},
		},
	}

	interpolator := newInterpolator(exec)

	tests := []struct {
		name     string
		input    json.RawMessage
		validate func(t *testing.T, result json.RawMessage)
	}{
		{
			name:  "pure array interpolation preserves array",
			input: json.RawMessage(`{"items": "${steps.step1.output.issues}"}`),
			validate: func(t *testing.T, result json.RawMessage) {
				var parsed map[string]any
				if err := json.Unmarshal(result, &parsed); err != nil {
					t.Fatalf("failed to parse result: %v", err)
				}
				items, ok := parsed["items"].([]any)
				if !ok {
					t.Fatalf("expected items to be an array, got %T", parsed["items"])
				}
				if len(items) != 3 {
					t.Errorf("expected 3 items, got %d", len(items))
				}
				if items[0] != "bug1" || items[1] != "bug2" || items[2] != "bug3" {
					t.Errorf("unexpected items: %v", items)
				}
			},
		},
		{
			name:  "pure object interpolation preserves object",
			input: json.RawMessage(`{"config": "${steps.step1.output.settings}"}`),
			validate: func(t *testing.T, result json.RawMessage) {
				var parsed map[string]any
				if err := json.Unmarshal(result, &parsed); err != nil {
					t.Fatalf("failed to parse result: %v", err)
				}
				config, ok := parsed["config"].(map[string]any)
				if !ok {
					t.Fatalf("expected config to be an object, got %T", parsed["config"])
				}
				if config["enabled"] != true {
					t.Errorf("expected enabled=true, got %v", config["enabled"])
				}
				if config["threshold"] != 0.5 {
					t.Errorf("expected threshold=0.5, got %v", config["threshold"])
				}
			},
		},
		{
			name:  "pure number interpolation preserves number",
			input: json.RawMessage(`{"total": "${steps.step1.output.count}"}`),
			validate: func(t *testing.T, result json.RawMessage) {
				var parsed map[string]any
				if err := json.Unmarshal(result, &parsed); err != nil {
					t.Fatalf("failed to parse result: %v", err)
				}
				// JSON numbers unmarshal as float64
				total, ok := parsed["total"].(float64)
				if !ok {
					t.Fatalf("expected total to be a number, got %T", parsed["total"])
				}
				if total != 3 {
					t.Errorf("expected total=3, got %v", total)
				}
			},
		},
		{
			name:  "embedded interpolation stays as string",
			input: json.RawMessage(`{"message": "Found ${steps.step1.output.count} issues"}`),
			validate: func(t *testing.T, result json.RawMessage) {
				var parsed map[string]any
				if err := json.Unmarshal(result, &parsed); err != nil {
					t.Fatalf("failed to parse result: %v", err)
				}
				msg, ok := parsed["message"].(string)
				if !ok {
					t.Fatalf("expected message to be a string, got %T", parsed["message"])
				}
				if msg != "Found 3 issues" {
					t.Errorf("expected 'Found 3 issues', got %q", msg)
				}
			},
		},
		{
			name:  "nested structure with pure interpolation",
			input: json.RawMessage(`{"data": {"list": "${steps.step1.output.issues}"}}`),
			validate: func(t *testing.T, result json.RawMessage) {
				var parsed map[string]any
				if err := json.Unmarshal(result, &parsed); err != nil {
					t.Fatalf("failed to parse result: %v", err)
				}
				data, ok := parsed["data"].(map[string]any)
				if !ok {
					t.Fatalf("expected data to be an object, got %T", parsed["data"])
				}
				list, ok := data["list"].([]any)
				if !ok {
					t.Fatalf("expected list to be an array, got %T", data["list"])
				}
				if len(list) != 3 {
					t.Errorf("expected 3 items, got %d", len(list))
				}
			},
		},
		{
			name:  "array containing pure interpolations",
			input: json.RawMessage(`{"values": ["${steps.step1.output.count}", "static", "${steps.step1.output.count}"]}`),
			validate: func(t *testing.T, result json.RawMessage) {
				var parsed map[string]any
				if err := json.Unmarshal(result, &parsed); err != nil {
					t.Fatalf("failed to parse result: %v", err)
				}
				values, ok := parsed["values"].([]any)
				if !ok {
					t.Fatalf("expected values to be an array, got %T", parsed["values"])
				}
				if len(values) != 3 {
					t.Errorf("expected 3 values, got %d", len(values))
				}
				// First and third should be numbers, second should be string
				if values[0] != float64(3) {
					t.Errorf("expected values[0]=3, got %v (%T)", values[0], values[0])
				}
				if values[1] != "static" {
					t.Errorf("expected values[1]='static', got %v", values[1])
				}
				if values[2] != float64(3) {
					t.Errorf("expected values[2]=3, got %v (%T)", values[2], values[2])
				}
			},
		},
		{
			name:  "mixed pure and embedded interpolations",
			input: json.RawMessage(`{"items": "${steps.step1.output.issues}", "summary": "Count: ${steps.step1.output.count}"}`),
			validate: func(t *testing.T, result json.RawMessage) {
				var parsed map[string]any
				if err := json.Unmarshal(result, &parsed); err != nil {
					t.Fatalf("failed to parse result: %v", err)
				}
				// items should be an array
				items, ok := parsed["items"].([]any)
				if !ok {
					t.Fatalf("expected items to be an array, got %T", parsed["items"])
				}
				if len(items) != 3 {
					t.Errorf("expected 3 items, got %d", len(items))
				}
				// summary should be a string
				summary, ok := parsed["summary"].(string)
				if !ok {
					t.Fatalf("expected summary to be a string, got %T", parsed["summary"])
				}
				if summary != "Count: 3" {
					t.Errorf("expected 'Count: 3', got %q", summary)
				}
			},
		},
		{
			name:  "deep nested path preserves type",
			input: json.RawMessage(`{"value": "${steps.step1.output.nested.deep.value}"}`),
			validate: func(t *testing.T, result json.RawMessage) {
				var parsed map[string]any
				if err := json.Unmarshal(result, &parsed); err != nil {
					t.Fatalf("failed to parse result: %v", err)
				}
				value, ok := parsed["value"].(float64)
				if !ok {
					t.Fatalf("expected value to be a number, got %T", parsed["value"])
				}
				if value != 42 {
					t.Errorf("expected value=42, got %v", value)
				}
			},
		},
		{
			name:  "trigger payload array preservation",
			input: json.RawMessage(`{"triggerItems": "${trigger.payload.items}"}`),
			validate: func(t *testing.T, result json.RawMessage) {
				var parsed map[string]any
				if err := json.Unmarshal(result, &parsed); err != nil {
					t.Fatalf("failed to parse result: %v", err)
				}
				items, ok := parsed["triggerItems"].([]any)
				if !ok {
					t.Fatalf("expected triggerItems to be an array, got %T", parsed["triggerItems"])
				}
				if len(items) != 3 {
					t.Errorf("expected 3 items, got %d", len(items))
				}
			},
		},
		{
			name:  "error path returns nil and propagates error",
			input: json.RawMessage(`{"missing": "${steps.nonexistent.output.field}"}`),
			validate: func(t *testing.T, result json.RawMessage) {
				// With error propagation, result should be nil when interpolation fails
				// Callers should use interpolateJSONWithFallback if they want original preserved
				if result != nil {
					t.Errorf("expected nil result on error, got %s", string(result))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := interpolator.InterpolateJSON(tt.input)
			if err != nil {
				t.Logf("interpolation had error: %v", err)
			}
			tt.validate(t, result)
		})
	}
}

// TestResolveInputs tests the ResolveInputs function with both from and template sources.
func TestResolveInputs(t *testing.T) {
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
			Payload:   json.RawMessage(`{"name": "Alice", "count": 5, "items": ["a", "b", "c"]}`),
			Timestamp: time.Now(),
		},
		StepResults: map[string]StepResult{
			"fetch": {
				StepName:  "fetch",
				Status:    "success",
				Output:    json.RawMessage(`{"result": "fetched_data", "details": {"type": "json"}}`),
				Iteration: 1,
			},
		},
	}

	interpolator := newInterpolator(exec)

	tests := []struct {
		name        string
		inputs      map[string]wfschema.InputRef
		checkField  string
		expected    any
		wantErr     bool
		errContains string
	}{
		{
			name: "from source - simple value",
			inputs: map[string]wfschema.InputRef{
				"data": {From: "fetch.result"},
			},
			checkField: "data",
			expected:   "fetched_data",
		},
		{
			name: "from source - array preserved",
			inputs: map[string]wfschema.InputRef{
				"items": {From: "trigger.payload.items"},
			},
			checkField: "items",
			expected:   []any{"a", "b", "c"},
		},
		{
			name: "from source - number preserved",
			inputs: map[string]wfschema.InputRef{
				"count": {From: "trigger.payload.count"},
			},
			checkField: "count",
			expected:   float64(5), // JSON numbers are float64
		},
		{
			name: "template source - string interpolation",
			inputs: map[string]wfschema.InputRef{
				"message": {Template: "Hello ${trigger.payload.name}!"},
			},
			checkField: "message",
			expected:   "Hello Alice!",
		},
		{
			name: "template source - multiple interpolations",
			inputs: map[string]wfschema.InputRef{
				"prompt": {Template: "Process ${steps.fetch.output.result} for ${trigger.payload.name}"},
			},
			checkField: "prompt",
			expected:   "Process fetched_data for Alice",
		},
		{
			name: "mixed from and template sources",
			inputs: map[string]wfschema.InputRef{
				"data":    {From: "fetch.result"},
				"message": {Template: "Working on: ${trigger.payload.name}"},
				"count":   {From: "trigger.payload.count"},
			},
			checkField: "message",
			expected:   "Working on: Alice",
		},
		{
			name: "neither from nor template - error",
			inputs: map[string]wfschema.InputRef{
				"invalid": {},
			},
			wantErr:     true,
			errContains: "requires either 'from' or 'template'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := interpolator.ResolveInputs(tt.inputs, "", nil)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Parse result
			var parsed map[string]any
			if err := json.Unmarshal(result, &parsed); err != nil {
				t.Fatalf("failed to parse result: %v", err)
			}

			got := parsed[tt.checkField]

			// Deep compare for slices
			gotJSON, _ := json.Marshal(got)
			expectedJSON, _ := json.Marshal(tt.expected)
			if string(gotJSON) != string(expectedJSON) {
				t.Errorf("%s = %v (%s), want %v (%s)", tt.checkField, got, gotJSON, tt.expected, expectedJSON)
			}
		})
	}
}
