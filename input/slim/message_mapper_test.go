package slim

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/c360studio/semstreams/agentic"
)

func TestNewMessageMapper(t *testing.T) {
	mapper := NewMessageMapper()

	if mapper == nil {
		t.Fatal("expected message mapper, got nil")
	}

	if mapper.defaultChannelType != "slim" {
		t.Errorf("expected defaultChannelType 'slim', got %q", mapper.defaultChannelType)
	}
}

func TestMessageMapperToUserMessage(t *testing.T) {
	mapper := NewMessageMapper()

	tests := []struct {
		name     string
		slimMsg  *Message
		wantErr  bool
		validate func(*testing.T, *agentic.UserMessage)
	}{
		{
			name: "plain text message",
			slimMsg: &Message{
				GroupID:   "test-group",
				SenderDID: "did:key:sender123",
				Content:   []byte("Hello world"),
				Timestamp: time.Now(),
				MessageID: "msg-001",
			},
			wantErr: false,
			validate: func(t *testing.T, msg *agentic.UserMessage) {
				if msg.Content != "Hello world" {
					t.Errorf("expected content 'Hello world', got %q", msg.Content)
				}
				if msg.ChannelType != "slim" {
					t.Errorf("expected channel type 'slim', got %q", msg.ChannelType)
				}
				if msg.UserID != "did:key:sender123" {
					t.Errorf("expected user ID 'did:key:sender123', got %q", msg.UserID)
				}
			},
		},
		{
			name: "json user message",
			slimMsg: &Message{
				GroupID:   "test-group",
				SenderDID: "did:key:sender456",
				Content: []byte(`{
					"type": "text",
					"content": "Structured message",
					"thread_id": "thread-001",
					"reply_to": "msg-000"
				}`),
				Timestamp: time.Now(),
				MessageID: "msg-002",
			},
			wantErr: false,
			validate: func(t *testing.T, msg *agentic.UserMessage) {
				if msg.Content != "Structured message" {
					t.Errorf("expected content 'Structured message', got %q", msg.Content)
				}
				if msg.Metadata["slim_thread_id"] != "thread-001" {
					t.Errorf("expected thread_id in metadata")
				}
				if msg.Metadata["slim_reply_to"] != "msg-000" {
					t.Errorf("expected reply_to in metadata")
				}
			},
		},
		{
			name: "message with attachments",
			slimMsg: &Message{
				GroupID:   "test-group",
				SenderDID: "did:key:sender789",
				Content: []byte(`{
					"type": "text",
					"content": "Check this file",
					"attachments": [
						{
							"name": "doc.pdf",
							"mime_type": "application/pdf",
							"size": 12345
						}
					]
				}`),
				Timestamp: time.Now(),
				MessageID: "msg-003",
			},
			wantErr: false,
			validate: func(t *testing.T, msg *agentic.UserMessage) {
				if len(msg.Attachments) != 1 {
					t.Fatalf("expected 1 attachment, got %d", len(msg.Attachments))
				}
				if msg.Attachments[0].Name != "doc.pdf" {
					t.Errorf("expected attachment name 'doc.pdf', got %q", msg.Attachments[0].Name)
				}
				if msg.Attachments[0].MimeType != "application/pdf" {
					t.Errorf("expected mime type 'application/pdf', got %q", msg.Attachments[0].MimeType)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := mapper.ToUserMessage(tt.slimMsg)
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

			// Common validations
			if msg.MessageID != tt.slimMsg.MessageID {
				t.Errorf("expected message ID %q, got %q", tt.slimMsg.MessageID, msg.MessageID)
			}

			if msg.ChannelID != tt.slimMsg.GroupID {
				t.Errorf("expected channel ID %q, got %q", tt.slimMsg.GroupID, msg.ChannelID)
			}

			if msg.Metadata["slim_group_id"] != tt.slimMsg.GroupID {
				t.Errorf("expected slim_group_id in metadata")
			}

			if msg.Metadata["slim_sender_did"] != tt.slimMsg.SenderDID {
				t.Errorf("expected slim_sender_did in metadata")
			}

			if tt.validate != nil {
				tt.validate(t, msg)
			}
		})
	}
}

func TestMessageMapperToTaskMessage(t *testing.T) {
	mapper := NewMessageMapper()

	deadline := time.Now().Add(time.Hour)
	taskContent := TaskDelegation{
		Type:               "task",
		TaskID:             "task-001",
		Prompt:             "Analyze this data",
		Role:               "analyst",
		Model:              "gpt-4",
		RequestingAgentDID: "did:key:agent123",
		TargetCapabilities: []string{"data-analysis", "reporting"},
		Priority:           "high",
		Deadline:           &deadline,
		Context: map[string]any{
			"source": "external",
		},
	}

	content, err := json.Marshal(taskContent)
	if err != nil {
		t.Fatalf("failed to marshal task content: %v", err)
	}

	slimMsg := &Message{
		GroupID:   "task-group",
		SenderDID: "did:key:sender",
		Content:   content,
		Timestamp: time.Now(),
		MessageID: "msg-task-001",
	}

	msg, err := mapper.ToTaskMessage(slimMsg)
	if err != nil {
		t.Fatalf("ToTaskMessage failed: %v", err)
	}

	if msg.TaskID != "task-001" {
		t.Errorf("expected task ID 'task-001', got %q", msg.TaskID)
	}

	if msg.Prompt != "Analyze this data" {
		t.Errorf("expected prompt 'Analyze this data', got %q", msg.Prompt)
	}

	if msg.ChannelType != "slim" {
		t.Errorf("expected channel type 'slim', got %q", msg.ChannelType)
	}

	if msg.UserID != "did:key:agent123" {
		t.Errorf("expected user ID from requesting agent")
	}

	if msg.Role != "analyst" {
		t.Errorf("expected role 'analyst', got %q", msg.Role)
	}

	if msg.Model != "gpt-4" {
		t.Errorf("expected model 'gpt-4', got %q", msg.Model)
	}
}

func TestMessageMapperToTaskMessageDefaults(t *testing.T) {
	mapper := NewMessageMapper()

	// Task without role and model
	taskContent := TaskDelegation{
		Type:               "task",
		TaskID:             "task-002",
		Prompt:             "Simple task",
		RequestingAgentDID: "did:key:agent456",
	}

	content, _ := json.Marshal(taskContent)

	slimMsg := &Message{
		GroupID:   "task-group",
		SenderDID: "did:key:sender",
		Content:   content,
		Timestamp: time.Now(),
		MessageID: "msg-task-002",
	}

	msg, err := mapper.ToTaskMessage(slimMsg)
	if err != nil {
		t.Fatalf("ToTaskMessage failed: %v", err)
	}

	if msg.Role != "general" {
		t.Errorf("expected default role 'general', got %q", msg.Role)
	}

	if msg.Model != "default" {
		t.Errorf("expected default model 'default', got %q", msg.Model)
	}
}

func TestMessageMapperToTaskMessageInvalid(t *testing.T) {
	mapper := NewMessageMapper()

	slimMsg := &Message{
		GroupID:   "task-group",
		SenderDID: "did:key:sender",
		Content:   []byte("not valid json for task"),
		Timestamp: time.Now(),
		MessageID: "msg-invalid",
	}

	_, err := mapper.ToTaskMessage(slimMsg)
	if err == nil {
		t.Error("expected error for invalid task message")
	}
}

func TestMessageMapperFromUserResponse(t *testing.T) {
	mapper := NewMessageMapper()

	tests := []struct {
		name     string
		response *agentic.UserResponse
		validate func(*testing.T, []byte)
	}{
		{
			name: "success response",
			response: &agentic.UserResponse{
				InReplyTo: "msg-001",
				Type:      "text",
				Content:   "Here is your answer",
			},
			validate: func(t *testing.T, data []byte) {
				var resp ResponseMessage
				if err := json.Unmarshal(data, &resp); err != nil {
					t.Fatalf("failed to unmarshal response: %v", err)
				}
				if resp.Status != "success" {
					t.Errorf("expected status 'success', got %q", resp.Status)
				}
				if resp.Content != "Here is your answer" {
					t.Errorf("expected content 'Here is your answer', got %q", resp.Content)
				}
			},
		},
		{
			name: "error response",
			response: &agentic.UserResponse{
				InReplyTo: "msg-002",
				Type:      "error",
				Content:   "Failed to process",
			},
			validate: func(t *testing.T, data []byte) {
				var resp ResponseMessage
				if err := json.Unmarshal(data, &resp); err != nil {
					t.Fatalf("failed to unmarshal response: %v", err)
				}
				if resp.Status != "error" {
					t.Errorf("expected status 'error', got %q", resp.Status)
				}
				if resp.Error != "Failed to process" {
					t.Errorf("expected error 'Failed to process', got %q", resp.Error)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := mapper.FromUserResponse(tt.response)
			if err != nil {
				t.Fatalf("FromUserResponse failed: %v", err)
			}

			tt.validate(t, data)
		})
	}
}

func TestMessageMapperFromTaskResult(t *testing.T) {
	mapper := NewMessageMapper()

	tests := []struct {
		name     string
		result   *TaskResult
		validate func(*testing.T, []byte)
	}{
		{
			name: "success result",
			result: &TaskResult{
				TaskID:      "task-001",
				Result:      "Task completed successfully",
				CompletedAt: time.Now(),
			},
			validate: func(t *testing.T, data []byte) {
				var resp ResponseMessage
				if err := json.Unmarshal(data, &resp); err != nil {
					t.Fatalf("failed to unmarshal result: %v", err)
				}
				if resp.Type != "task_result" {
					t.Errorf("expected type 'task_result', got %q", resp.Type)
				}
				if resp.Status != "success" {
					t.Errorf("expected status 'success', got %q", resp.Status)
				}
				if resp.InReplyTo != "task-001" {
					t.Errorf("expected in_reply_to 'task-001', got %q", resp.InReplyTo)
				}
			},
		},
		{
			name: "error result",
			result: &TaskResult{
				TaskID:      "task-002",
				Error:       "Task failed",
				CompletedAt: time.Now(),
			},
			validate: func(t *testing.T, data []byte) {
				var resp ResponseMessage
				if err := json.Unmarshal(data, &resp); err != nil {
					t.Fatalf("failed to unmarshal result: %v", err)
				}
				if resp.Status != "error" {
					t.Errorf("expected status 'error', got %q", resp.Status)
				}
				if resp.Error != "Task failed" {
					t.Errorf("expected error 'Task failed', got %q", resp.Error)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := mapper.FromTaskResult(tt.result)
			if err != nil {
				t.Fatalf("FromTaskResult failed: %v", err)
			}

			tt.validate(t, data)
		})
	}
}

func TestMessageMapperCreateTaskMessage(t *testing.T) {
	mapper := NewMessageMapper()

	data, err := mapper.CreateTaskMessage(
		"Analyze the data",
		"did:key:requester",
		[]string{"analysis", "reporting"},
	)
	if err != nil {
		t.Fatalf("CreateTaskMessage failed: %v", err)
	}

	var task TaskDelegation
	if err := json.Unmarshal(data, &task); err != nil {
		t.Fatalf("failed to unmarshal task: %v", err)
	}

	if task.Type != "task" {
		t.Errorf("expected type 'task', got %q", task.Type)
	}

	if task.TaskID == "" {
		t.Error("expected task ID to be generated")
	}

	if task.Prompt != "Analyze the data" {
		t.Errorf("expected prompt 'Analyze the data', got %q", task.Prompt)
	}

	if task.RequestingAgentDID != "did:key:requester" {
		t.Errorf("expected requesting agent DID")
	}

	if len(task.TargetCapabilities) != 2 {
		t.Errorf("expected 2 capabilities, got %d", len(task.TargetCapabilities))
	}
}

func TestMessageMapperParseMessageType(t *testing.T) {
	mapper := NewMessageMapper()

	tests := []struct {
		name    string
		content []byte
		want    string
	}{
		{
			name:    "plain text defaults to user",
			content: []byte("Hello world"),
			want:    "user",
		},
		{
			name:    "user type message",
			content: []byte(`{"type": "text", "content": "hello"}`),
			want:    "user",
		},
		{
			name:    "task message",
			content: []byte(`{"type": "task", "task_id": "123"}`),
			want:    "task",
		},
		{
			name:    "response message",
			content: []byte(`{"type": "response", "content": "result"}`),
			want:    "response",
		},
		{
			name:    "task_result message",
			content: []byte(`{"type": "task_result", "task_id": "123"}`),
			want:    "response",
		},
		{
			name:    "unknown type defaults to user",
			content: []byte(`{"type": "custom", "data": "foo"}`),
			want:    "user",
		},
		{
			name:    "invalid json defaults to user",
			content: []byte(`{not json`),
			want:    "user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mapper.ParseMessageType(tt.content)
			if err != nil {
				t.Fatalf("ParseMessageType failed: %v", err)
			}
			if got != tt.want {
				t.Errorf("ParseMessageType() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUserMessageSerialization(t *testing.T) {
	msg := UserMessage{
		Type:     "text",
		Content:  "Hello",
		ThreadID: "thread-001",
		ReplyTo:  "msg-000",
		Attachments: []Attachment{
			{
				Name:     "file.txt",
				MimeType: "text/plain",
				Size:     100,
			},
		},
		Metadata: map[string]any{
			"key": "value",
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded UserMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Type != msg.Type {
		t.Errorf("type mismatch")
	}
	if decoded.Content != msg.Content {
		t.Errorf("content mismatch")
	}
	if decoded.ThreadID != msg.ThreadID {
		t.Errorf("thread_id mismatch")
	}
}
