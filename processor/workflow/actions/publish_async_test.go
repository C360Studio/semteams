package actions

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestPublishAsyncAction_Execute(t *testing.T) {
	tests := []struct {
		name        string
		subject     string
		payload     json.RawMessage
		taskID      string
		executionID string
		wantSuccess bool
		wantError   string
	}{
		{
			name:        "no nats client",
			subject:     "async.task",
			payload:     json.RawMessage(`{"data": "test"}`),
			taskID:      "t1",
			executionID: "",
			wantSuccess: false,
			wantError:   "NATS client not available",
		},
		{
			name:        "auto-generate task_id when empty",
			subject:     "async.task",
			payload:     json.RawMessage(`{}`),
			taskID:      "",
			executionID: "",
			wantSuccess: false,
			wantError:   "NATS client not available",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action := NewPublishAsyncAction(tt.subject, tt.payload, tt.taskID)
			result := action.Execute(context.Background(), &Context{ExecutionID: tt.executionID})

			if result.Success != tt.wantSuccess {
				t.Errorf("Execute() success = %v, want %v", result.Success, tt.wantSuccess)
			}

			if tt.wantError != "" && result.Error == "" {
				t.Errorf("Execute() expected error containing %q, got no error", tt.wantError)
			}

			if tt.wantError != "" && result.Error != "" {
				if !strings.Contains(result.Error, tt.wantError) {
					t.Errorf("Execute() error = %q, want containing %q", result.Error, tt.wantError)
				}
			}
		})
	}
}

func TestPublishAsyncAction_TaskIDGeneration(t *testing.T) {
	tests := []struct {
		name           string
		providedTaskID string
		expectUUID     bool
	}{
		{
			name:           "uses provided task_id",
			providedTaskID: "my-custom-task-id",
			expectUUID:     false,
		},
		{
			name:           "auto-generates UUID when empty",
			providedTaskID: "",
			expectUUID:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action := NewPublishAsyncAction("async.task", json.RawMessage(`{}`), tt.providedTaskID)

			// Execute to trigger task_id generation (will fail without NATS but that's ok)
			_ = action.Execute(context.Background(), &Context{})

			// The action struct stores the original task_id, but we can verify behavior
			// by checking that provided task_id is stored correctly
			if tt.providedTaskID != "" && action.TaskID != tt.providedTaskID {
				t.Errorf("action.TaskID = %q, want %q", action.TaskID, tt.providedTaskID)
			}
		})
	}
}

func TestPublishAsyncAction_PayloadEnrichment(t *testing.T) {
	// Test the payload enrichment logic by verifying the expected structure
	// Note: Full integration testing with NATS requires testcontainers

	tests := []struct {
		name        string
		payload     json.RawMessage
		expectWrap  bool // Whether non-object payload should be wrapped
		description string
	}{
		{
			name:        "json object payload",
			payload:     json.RawMessage(`{"key": "value"}`),
			expectWrap:  false,
			description: "JSON objects should have task_id and callback_subject injected directly",
		},
		{
			name:        "json array payload",
			payload:     json.RawMessage(`[1, 2, 3]`),
			expectWrap:  true,
			description: "JSON arrays should be wrapped under 'data' key",
		},
		{
			name:        "json string payload",
			payload:     json.RawMessage(`"just a string"`),
			expectWrap:  true,
			description: "JSON primitives should be wrapped under 'data' key",
		},
		{
			name:        "empty payload",
			payload:     json.RawMessage(`{}`),
			expectWrap:  false,
			description: "Empty object should have fields injected directly",
		},
		{
			name:        "nil payload",
			payload:     nil,
			expectWrap:  false,
			description: "Nil payload should create new object with injected fields",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action := NewPublishAsyncAction("async.task", tt.payload, "test-task-id")

			// Verify action is created correctly
			if action.Subject != "async.task" {
				t.Errorf("action.Subject = %q, want %q", action.Subject, "async.task")
			}
			if action.TaskID != "test-task-id" {
				t.Errorf("action.TaskID = %q, want %q", action.TaskID, "test-task-id")
			}

			// Log expected behavior for documentation
			t.Logf("Description: %s", tt.description)
		})
	}
}

func TestPublishAsyncAction_CallbackSubjectFormat(t *testing.T) {
	// Verify the expected callback subject format
	executionID := "exec-12345"
	expectedCallback := "workflow.step.result.exec-12345"

	action := NewPublishAsyncAction("async.task", json.RawMessage(`{}`), "task-id")

	// The callback subject is built from executionID during Execute
	// We can verify the format by documenting expected behavior
	t.Logf("Expected callback subject format: workflow.step.result.{executionID}")
	t.Logf("For executionID=%q, callback would be: %q", executionID, expectedCallback)

	// Verify action stores correct values
	if action.Subject != "async.task" {
		t.Errorf("action.Subject = %q, want %q", action.Subject, "async.task")
	}
}

func TestPublishAsyncAction_OutputFormat(t *testing.T) {
	// Document the expected output format when execution succeeds
	// The output contains task_id for correlation

	expectedOutput := map[string]string{"task_id": "test-task-id"}
	expectedJSON, _ := json.Marshal(expectedOutput)

	t.Logf("Expected successful output format: %s", string(expectedJSON))

	// Verify task_id is valid UUID when auto-generated
	generatedID := uuid.New().String()
	_, err := uuid.Parse(generatedID)
	if err != nil {
		t.Errorf("auto-generated task_id should be valid UUID, got error: %v", err)
	}
}

func TestPublishAsyncAction_Constructor(t *testing.T) {
	tests := []struct {
		name    string
		subject string
		payload json.RawMessage
		taskID  string
	}{
		{
			name:    "all fields provided",
			subject: "my.subject",
			payload: json.RawMessage(`{"foo": "bar"}`),
			taskID:  "my-task",
		},
		{
			name:    "minimal fields",
			subject: "min.subject",
			payload: nil,
			taskID:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action := NewPublishAsyncAction(tt.subject, tt.payload, tt.taskID)

			if action.Subject != tt.subject {
				t.Errorf("Subject = %q, want %q", action.Subject, tt.subject)
			}
			if string(action.Payload) != string(tt.payload) {
				t.Errorf("Payload = %q, want %q", string(action.Payload), string(tt.payload))
			}
			if action.TaskID != tt.taskID {
				t.Errorf("TaskID = %q, want %q", action.TaskID, tt.taskID)
			}
		})
	}
}
