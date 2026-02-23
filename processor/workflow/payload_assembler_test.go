package workflow

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/component"
)

func TestParseTypeString(t *testing.T) {
	tests := []struct {
		name            string
		typeStr         string
		expectedDomain  string
		expectedCat     string
		expectedVersion string
		wantErr         bool
		errContains     string
	}{
		{
			name:            "valid type string",
			typeStr:         "agentic.task.v1",
			expectedDomain:  "agentic",
			expectedCat:     "task",
			expectedVersion: "v1",
			wantErr:         false,
		},
		{
			name:            "valid with whitespace",
			typeStr:         " robotics . heartbeat . v2 ",
			expectedDomain:  "robotics",
			expectedCat:     "heartbeat",
			expectedVersion: "v2",
			wantErr:         false,
		},
		{
			name:            "valid complex domain",
			typeStr:         "sensors.gps.v1",
			expectedDomain:  "sensors",
			expectedCat:     "gps",
			expectedVersion: "v1",
			wantErr:         false,
		},
		{
			name:        "empty string",
			typeStr:     "",
			wantErr:     true,
			errContains: "cannot be empty",
		},
		{
			name:        "missing version",
			typeStr:     "agentic.task",
			wantErr:     true,
			errContains: "domain.category.version",
		},
		{
			name:        "only domain",
			typeStr:     "agentic",
			wantErr:     true,
			errContains: "domain.category.version",
		},
		{
			name:        "too many parts",
			typeStr:     "a.b.c.d",
			wantErr:     true,
			errContains: "domain.category.version",
		},
		{
			name:        "empty domain",
			typeStr:     ".category.v1",
			wantErr:     true,
			errContains: "domain cannot be empty",
		},
		{
			name:        "empty category",
			typeStr:     "domain..v1",
			wantErr:     true,
			errContains: "category cannot be empty",
		},
		{
			name:        "empty version",
			typeStr:     "domain.category.",
			wantErr:     true,
			errContains: "version cannot be empty",
		},
		{
			name:        "whitespace only domain",
			typeStr:     "  .category.v1",
			wantErr:     true,
			errContains: "domain cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			domain, category, version, err := ParseTypeString(tt.typeStr)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseTypeString(%q) expected error, got nil", tt.typeStr)
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ParseTypeString(%q) error = %v, want error containing %q", tt.typeStr, err, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("ParseTypeString(%q) unexpected error: %v", tt.typeStr, err)
				return
			}

			if domain != tt.expectedDomain {
				t.Errorf("ParseTypeString(%q) domain = %q, want %q", tt.typeStr, domain, tt.expectedDomain)
			}
			if category != tt.expectedCat {
				t.Errorf("ParseTypeString(%q) category = %q, want %q", tt.typeStr, category, tt.expectedCat)
			}
			if version != tt.expectedVersion {
				t.Errorf("ParseTypeString(%q) version = %q, want %q", tt.typeStr, version, tt.expectedVersion)
			}
		})
	}
}

// testWorkflowPayload is a test payload type for workflow assembler tests
type testWorkflowPayload struct {
	TaskID   string `json:"task_id"`
	UserID   string `json:"user_id"`
	Priority int    `json:"priority"`
	Content  string `json:"content"`
}

// testWorkflowBuilder creates test payloads from field mappings
func testWorkflowBuilder(fields map[string]any) (any, error) {
	payload := &testWorkflowPayload{}

	if taskID, ok := fields["task_id"].(string); ok {
		payload.TaskID = taskID
	}
	if userID, ok := fields["user_id"].(string); ok {
		payload.UserID = userID
	}
	if priority, ok := fields["priority"].(int); ok {
		payload.Priority = priority
	} else if priority, ok := fields["priority"].(float64); ok {
		// JSON numbers decode as float64
		payload.Priority = int(priority)
	}
	if content, ok := fields["content"].(string); ok {
		payload.Content = content
	}

	return payload, nil
}

// testWorkflowFactory creates test payload instances
func testWorkflowFactory() any {
	return &testWorkflowPayload{}
}

func TestAssemblePayload_Success(t *testing.T) {
	// Setup registry with test payload type
	registry := component.NewPayloadRegistry()
	err := registry.RegisterPayload(&component.PayloadRegistration{
		Factory:     testWorkflowFactory,
		Builder:     testWorkflowBuilder,
		Domain:      "test",
		Category:    "workflow",
		Version:     "v1",
		Description: "Test workflow payload",
	})
	if err != nil {
		t.Fatalf("Failed to register test payload: %v", err)
	}

	// Setup test execution context
	executionContext := map[string]any{
		"trigger": map[string]any{
			"payload": map[string]any{
				"task_id":  "task-123",
				"priority": float64(5), // JSON numbers are float64
				"extra":    "ignored",
			},
		},
		"steps": map[string]any{
			"analyze": map[string]any{
				"output": map[string]any{
					"user_id": "user-456",
					"result":  "approved",
				},
			},
		},
	}

	// Path resolver that navigates the context map
	resolvePath := func(path string) (any, error) {
		parts := strings.Split(path, ".")
		current := executionContext

		for i, part := range parts {
			val, ok := current[part]
			if !ok {
				return nil, fmt.Errorf("path not found: %s (at %s)", path, part)
			}

			if i == len(parts)-1 {
				return val, nil
			}

			current, ok = val.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("cannot navigate through %s in path %s", part, path)
			}
		}

		return nil, fmt.Errorf("unexpected end of path: %s", path)
	}

	// Define mapping and pass-through
	mapping := map[string]string{
		"user_id": "steps.analyze.output.user_id",
		"content": "steps.analyze.output.result",
	}
	passThrough := []string{"task_id", "priority"}

	// Assemble the payload
	result, err := AssemblePayload(
		registry,
		"test.workflow.v1",
		mapping,
		passThrough,
		resolvePath,
	)

	if err != nil {
		t.Fatalf("AssemblePayload() unexpected error: %v", err)
	}

	// Verify the result
	payload, ok := result.(*testWorkflowPayload)
	if !ok {
		t.Fatalf("AssemblePayload() returned type %T, want *testWorkflowPayload", result)
	}

	if payload.TaskID != "task-123" {
		t.Errorf("payload.TaskID = %q, want %q", payload.TaskID, "task-123")
	}
	if payload.UserID != "user-456" {
		t.Errorf("payload.UserID = %q, want %q", payload.UserID, "user-456")
	}
	if payload.Priority != 5 {
		t.Errorf("payload.Priority = %d, want %d", payload.Priority, 5)
	}
	if payload.Content != "approved" {
		t.Errorf("payload.Content = %q, want %q", payload.Content, "approved")
	}
}

func TestAssemblePayload_UnknownType(t *testing.T) {
	registry := component.NewPayloadRegistry()

	resolvePath := func(path string) (any, error) {
		return "value", nil
	}

	_, err := AssemblePayload(
		registry,
		"unknown.type.v1",
		map[string]string{"field": "trigger.payload.value"},
		nil,
		resolvePath,
	)

	if err == nil {
		t.Fatal("AssemblePayload() expected error for unknown type, got nil")
	}

	if !strings.Contains(err.Error(), "not registered") {
		t.Errorf("AssemblePayload() error = %v, want error containing 'not registered'", err)
	}
}

func TestAssemblePayload_InvalidTypeString(t *testing.T) {
	registry := component.NewPayloadRegistry()

	resolvePath := func(path string) (any, error) {
		return "value", nil
	}

	tests := []struct {
		name        string
		targetType  string
		errContains string
	}{
		{
			name:        "missing version",
			targetType:  "domain.category",
			errContains: "invalid target type",
		},
		{
			name:        "empty string",
			targetType:  "",
			errContains: "invalid target type",
		},
		{
			name:        "too many parts",
			targetType:  "a.b.c.d",
			errContains: "invalid target type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := AssemblePayload(
				registry,
				tt.targetType,
				map[string]string{"field": "trigger.payload.value"},
				nil,
				resolvePath,
			)

			if err == nil {
				t.Fatalf("AssemblePayload(%q) expected error, got nil", tt.targetType)
			}

			if !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("AssemblePayload(%q) error = %v, want error containing %q", tt.targetType, err, tt.errContains)
			}
		})
	}
}

func TestAssemblePayload_ResolutionFailure(t *testing.T) {
	// Setup registry
	registry := component.NewPayloadRegistry()
	err := registry.RegisterPayload(&component.PayloadRegistration{
		Factory:     testWorkflowFactory,
		Builder:     testWorkflowBuilder,
		Domain:      "test",
		Category:    "workflow",
		Version:     "v1",
		Description: "Test workflow payload",
	})
	if err != nil {
		t.Fatalf("Failed to register test payload: %v", err)
	}

	tests := []struct {
		name        string
		mapping     map[string]string
		passThrough []string
		errContains string
	}{
		{
			name: "mapping path not found",
			mapping: map[string]string{
				"user_id": "steps.unknown.output.user_id",
			},
			passThrough: nil,
			errContains: "failed to resolve mapping",
		},
		{
			name:    "pass-through field not found",
			mapping: nil,
			passThrough: []string{
				"nonexistent_field",
			},
			errContains: "failed to resolve pass-through field",
		},
		{
			name: "multiple failures - first one reported",
			mapping: map[string]string{
				"user_id": "steps.unknown.output.user_id",
				"task_id": "trigger.payload.task_id", // This one would work
			},
			passThrough: nil,
			errContains: "failed to resolve mapping",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Path resolver that fails on unknown paths
			resolvePath := func(path string) (any, error) {
				if strings.Contains(path, "unknown") || strings.Contains(path, "nonexistent") {
					return nil, fmt.Errorf("path not found: %s", path)
				}
				return "valid-value", nil
			}

			_, err := AssemblePayload(
				registry,
				"test.workflow.v1",
				tt.mapping,
				tt.passThrough,
				resolvePath,
			)

			if err == nil {
				t.Fatal("AssemblePayload() expected error for path resolution failure, got nil")
			}

			if !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("AssemblePayload() error = %v, want error containing %q", err, tt.errContains)
			}
		})
	}
}

func TestAssemblePayload_EmptyMappingAndPassThrough(t *testing.T) {
	// Setup registry
	registry := component.NewPayloadRegistry()
	err := registry.RegisterPayload(&component.PayloadRegistration{
		Factory:     testWorkflowFactory,
		Builder:     testWorkflowBuilder,
		Domain:      "test",
		Category:    "workflow",
		Version:     "v1",
		Description: "Test workflow payload",
	})
	if err != nil {
		t.Fatalf("Failed to register test payload: %v", err)
	}

	resolvePath := func(path string) (any, error) {
		return nil, fmt.Errorf("should not be called")
	}

	// Should succeed with empty fields
	result, err := AssemblePayload(
		registry,
		"test.workflow.v1",
		nil,   // no mapping
		nil,   // no pass-through
		resolvePath,
	)

	if err != nil {
		t.Fatalf("AssemblePayload() unexpected error: %v", err)
	}

	payload, ok := result.(*testWorkflowPayload)
	if !ok {
		t.Fatalf("AssemblePayload() returned type %T, want *testWorkflowPayload", result)
	}

	// All fields should be zero values
	if payload.TaskID != "" || payload.UserID != "" || payload.Priority != 0 || payload.Content != "" {
		t.Errorf("payload = %+v, want all zero values", payload)
	}
}

func TestAssemblePayload_Integration(t *testing.T) {
	// This test simulates a realistic workflow scenario with JSON payload
	registry := component.NewPayloadRegistry()
	err := registry.RegisterPayload(&component.PayloadRegistration{
		Factory:     testWorkflowFactory,
		Builder:     testWorkflowBuilder,
		Domain:      "agentic",
		Category:    "task",
		Version:     "v1",
		Description: "Agent task payload",
	})
	if err != nil {
		t.Fatalf("Failed to register test payload: %v", err)
	}

	// Simulate workflow execution context with JSON-decoded data
	triggerPayloadJSON := `{
		"task_id": "task-999",
		"priority": 10,
		"original_message": "Process this request"
	}`

	stepOutputJSON := `{
		"user_id": "user-alice",
		"analysis_result": "Task approved for execution",
		"confidence": 0.95
	}`

	var triggerPayload map[string]any
	var stepOutput map[string]any

	if err := json.Unmarshal([]byte(triggerPayloadJSON), &triggerPayload); err != nil {
		t.Fatalf("Failed to unmarshal trigger payload: %v", err)
	}
	if err := json.Unmarshal([]byte(stepOutputJSON), &stepOutput); err != nil {
		t.Fatalf("Failed to unmarshal step output: %v", err)
	}

	context := map[string]any{
		"trigger": map[string]any{
			"payload": triggerPayload,
		},
		"steps": map[string]any{
			"analyze_request": map[string]any{
				"output": stepOutput,
			},
		},
	}

	resolvePath := func(path string) (any, error) {
		parts := strings.Split(path, ".")
		current := context

		for i, part := range parts {
			val, ok := current[part]
			if !ok {
				return nil, fmt.Errorf("path not found: %s", path)
			}

			if i == len(parts)-1 {
				return val, nil
			}

			current, ok = val.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("cannot navigate path at %s", part)
			}
		}

		return nil, fmt.Errorf("unexpected path end")
	}

	result, err := AssemblePayload(
		registry,
		"agentic.task.v1",
		map[string]string{
			"user_id": "steps.analyze_request.output.user_id",
			"content": "steps.analyze_request.output.analysis_result",
		},
		[]string{"task_id", "priority"},
		resolvePath,
	)

	if err != nil {
		t.Fatalf("AssemblePayload() unexpected error: %v", err)
	}

	payload := result.(*testWorkflowPayload)

	if payload.TaskID != "task-999" {
		t.Errorf("payload.TaskID = %q, want %q", payload.TaskID, "task-999")
	}
	if payload.UserID != "user-alice" {
		t.Errorf("payload.UserID = %q, want %q", payload.UserID, "user-alice")
	}
	if payload.Priority != 10 {
		t.Errorf("payload.Priority = %d, want %d", payload.Priority, 10)
	}
	if payload.Content != "Task approved for execution" {
		t.Errorf("payload.Content = %q, want %q", payload.Content, "Task approved for execution")
	}
}
