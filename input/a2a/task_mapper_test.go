package a2a

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestNewTaskMapper(t *testing.T) {
	mapper := NewTaskMapper()

	if mapper == nil {
		t.Fatal("expected task mapper, got nil")
	}

	if mapper.defaultChannelType != "a2a" {
		t.Errorf("expected defaultChannelType 'a2a', got %q", mapper.defaultChannelType)
	}
}

func TestTaskMapperToTaskMessage(t *testing.T) {
	mapper := NewTaskMapper()

	tests := []struct {
		name         string
		task         *Task
		requesterDID string
		wantErr      bool
		validate     func(*testing.T, *Task, interface{})
	}{
		{
			name: "valid task with text",
			task: &Task{
				ID:        "task-001",
				SessionID: "session-001",
				Message: TaskMessage{
					Role: "user",
					Parts: []MessagePart{
						{Type: "text", Text: "Analyze this data"},
					},
				},
			},
			requesterDID: "did:key:requester123",
			wantErr:      false,
		},
		{
			name: "task with metadata role",
			task: &Task{
				ID: "task-002",
				Message: TaskMessage{
					Parts: []MessagePart{
						{Type: "text", Text: "Design the system"},
					},
				},
				Metadata: map[string]any{
					"role": "architect",
				},
			},
			requesterDID: "did:key:requester456",
			wantErr:      false,
		},
		{
			name: "task with capability hints",
			task: &Task{
				ID: "task-003",
				Message: TaskMessage{
					Parts: []MessagePart{
						{Type: "text", Text: "Write code"},
					},
				},
				Metadata: map[string]any{
					"capabilities": []any{"code", "implementation"},
				},
			},
			requesterDID: "did:key:requester789",
			wantErr:      false,
		},
		{
			name:         "nil task",
			task:         nil,
			requesterDID: "did:key:test",
			wantErr:      true,
		},
		{
			name: "task with no text content",
			task: &Task{
				ID: "task-004",
				Message: TaskMessage{
					Parts: []MessagePart{
						{Type: "file", File: &FilePart{Name: "data.txt"}},
					},
				},
			},
			requesterDID: "did:key:test",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := mapper.ToTaskMessage(tt.task, tt.requesterDID)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if msg == nil {
				t.Fatal("expected message, got nil")
			}

			if msg.TaskID != tt.task.ID {
				t.Errorf("expected task ID %q, got %q", tt.task.ID, msg.TaskID)
			}

			if msg.ChannelType != "a2a" {
				t.Errorf("expected channel type 'a2a', got %q", msg.ChannelType)
			}

			if msg.UserID != tt.requesterDID {
				t.Errorf("expected user ID %q, got %q", tt.requesterDID, msg.UserID)
			}
		})
	}
}

func TestTaskMapperFromTaskResult(t *testing.T) {
	mapper := NewTaskMapper()

	tests := []struct {
		name     string
		taskID   string
		result   string
		err      error
		validate func(*testing.T, *TaskResult)
	}{
		{
			name:   "success result",
			taskID: "task-001",
			result: "Task completed successfully",
			err:    nil,
			validate: func(t *testing.T, tr *TaskResult) {
				if tr.Status.State != "completed" {
					t.Errorf("expected state 'completed', got %q", tr.Status.State)
				}
				if len(tr.Artifacts) != 1 {
					t.Fatalf("expected 1 artifact, got %d", len(tr.Artifacts))
				}
				if tr.Artifacts[0].Parts[0].Text != "Task completed successfully" {
					t.Errorf("unexpected result content")
				}
			},
		},
		{
			name:   "error result",
			taskID: "task-002",
			result: "",
			err:    errors.New("task execution failed"),
			validate: func(t *testing.T, tr *TaskResult) {
				if tr.Status.State != "failed" {
					t.Errorf("expected state 'failed', got %q", tr.Status.State)
				}
				if tr.Error != "task execution failed" {
					t.Errorf("expected error message, got %q", tr.Error)
				}
			},
		},
		{
			name:   "empty result",
			taskID: "task-003",
			result: "",
			err:    nil,
			validate: func(t *testing.T, tr *TaskResult) {
				if tr.Status.State != "completed" {
					t.Errorf("expected state 'completed', got %q", tr.Status.State)
				}
				if len(tr.Artifacts) != 0 {
					t.Errorf("expected 0 artifacts for empty result, got %d", len(tr.Artifacts))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapper.FromTaskResult(tt.taskID, tt.result, tt.err)

			if result.TaskID != tt.taskID {
				t.Errorf("expected task ID %q, got %q", tt.taskID, result.TaskID)
			}

			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

func TestTaskMapperCreateTaskStatusUpdate(t *testing.T) {
	mapper := NewTaskMapper()

	task := mapper.CreateTaskStatusUpdate("task-123", "working", "Processing request")

	if task.ID != "task-123" {
		t.Errorf("expected task ID 'task-123', got %q", task.ID)
	}

	if task.Status.State != "working" {
		t.Errorf("expected state 'working', got %q", task.Status.State)
	}

	if task.Status.Message != "Processing request" {
		t.Errorf("expected message 'Processing request', got %q", task.Status.Message)
	}

	if task.Status.Timestamp.IsZero() {
		t.Error("expected timestamp to be set")
	}
}

func TestTaskMapperParseTask(t *testing.T) {
	mapper := NewTaskMapper()

	taskJSON := `{
		"id": "task-001",
		"sessionId": "session-001",
		"status": {
			"state": "submitted"
		},
		"message": {
			"role": "user",
			"parts": [
				{"type": "text", "text": "Hello, agent!"}
			]
		}
	}`

	task, err := mapper.ParseTask([]byte(taskJSON))
	if err != nil {
		t.Fatalf("ParseTask failed: %v", err)
	}

	if task.ID != "task-001" {
		t.Errorf("expected ID 'task-001', got %q", task.ID)
	}

	if task.SessionID != "session-001" {
		t.Errorf("expected sessionId 'session-001', got %q", task.SessionID)
	}

	if len(task.Message.Parts) != 1 {
		t.Fatalf("expected 1 message part, got %d", len(task.Message.Parts))
	}

	if task.Message.Parts[0].Text != "Hello, agent!" {
		t.Errorf("expected text 'Hello, agent!', got %q", task.Message.Parts[0].Text)
	}
}

func TestTaskMapperParseTaskInvalid(t *testing.T) {
	mapper := NewTaskMapper()

	_, err := mapper.ParseTask([]byte("not valid json"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestTaskMapperSerializeTaskResult(t *testing.T) {
	mapper := NewTaskMapper()

	result := &TaskResult{
		TaskID: "task-001",
		Status: TaskStatus{
			State:     "completed",
			Timestamp: time.Now(),
		},
		Artifacts: []Artifact{
			{
				Name: "result",
				Parts: []MessagePart{
					{Type: "text", Text: "Done!"},
				},
			},
		},
	}

	data, err := mapper.SerializeTaskResult(result)
	if err != nil {
		t.Fatalf("SerializeTaskResult failed: %v", err)
	}

	// Verify it's valid JSON
	var decoded TaskResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to decode serialized result: %v", err)
	}

	if decoded.TaskID != result.TaskID {
		t.Errorf("task ID mismatch")
	}

	if decoded.Status.State != result.Status.State {
		t.Errorf("status state mismatch")
	}
}

func TestTaskMapperRoleMapping(t *testing.T) {
	mapper := NewTaskMapper()

	tests := []struct {
		name     string
		metadata map[string]any
		wantRole string
	}{
		{
			name:     "nil metadata",
			metadata: nil,
			wantRole: "general",
		},
		{
			name:     "empty metadata",
			metadata: map[string]any{},
			wantRole: "general",
		},
		{
			name: "explicit role",
			metadata: map[string]any{
				"role": "architect",
			},
			wantRole: "architect",
		},
		{
			name: "architecture capability",
			metadata: map[string]any{
				"capabilities": []any{"architecture"},
			},
			wantRole: "architect",
		},
		{
			name: "code capability",
			metadata: map[string]any{
				"capabilities": []any{"code"},
			},
			wantRole: "editor",
		},
		{
			name: "unknown capability",
			metadata: map[string]any{
				"capabilities": []any{"unknown"},
			},
			wantRole: "general",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapper.mapRole(tt.metadata)
			if got != tt.wantRole {
				t.Errorf("mapRole() = %q, want %q", got, tt.wantRole)
			}
		})
	}
}

func TestTaskTypeSerialization(t *testing.T) {
	task := Task{
		ID:        "test-task",
		SessionID: "test-session",
		Status: TaskStatus{
			State:     "submitted",
			Message:   "Task submitted",
			Timestamp: time.Now(),
		},
		Message: TaskMessage{
			Role: "user",
			Parts: []MessagePart{
				{Type: "text", Text: "Hello"},
				{Type: "file", File: &FilePart{Name: "data.txt", MimeType: "text/plain"}},
			},
		},
		Artifacts: []Artifact{
			{
				Name: "output",
				Parts: []MessagePart{
					{Type: "text", Text: "Result"},
				},
			},
		},
		Metadata: map[string]any{
			"key": "value",
		},
	}

	data, err := json.Marshal(task)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded Task
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.ID != task.ID {
		t.Errorf("ID mismatch")
	}
	if decoded.SessionID != task.SessionID {
		t.Errorf("SessionID mismatch")
	}
	if decoded.Status.State != task.Status.State {
		t.Errorf("Status.State mismatch")
	}
	if len(decoded.Message.Parts) != 2 {
		t.Errorf("Message.Parts count mismatch")
	}
	if len(decoded.Artifacts) != 1 {
		t.Errorf("Artifacts count mismatch")
	}
}
