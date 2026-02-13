package a2a

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/google/uuid"
)

// TaskMapper translates between A2A tasks and SemStreams messages.
type TaskMapper struct {
	// defaultChannelType is the channel type for A2A messages.
	defaultChannelType string
}

// NewTaskMapper creates a new task mapper.
func NewTaskMapper() *TaskMapper {
	return &TaskMapper{
		defaultChannelType: "a2a",
	}
}

// Task represents an A2A task request.
// Based on the A2A protocol specification.
type Task struct {
	// ID is the unique task identifier.
	ID string `json:"id"`

	// SessionID groups related tasks in a conversation.
	SessionID string `json:"sessionId,omitempty"`

	// Status is the current task status.
	Status TaskStatus `json:"status"`

	// Message contains the task input.
	Message TaskMessage `json:"message"`

	// Artifacts are the task outputs.
	Artifacts []Artifact `json:"artifacts,omitempty"`

	// History contains previous messages in the session.
	History []TaskMessage `json:"history,omitempty"`

	// Metadata contains additional task metadata.
	Metadata map[string]any `json:"metadata,omitempty"`
}

// TaskStatus represents the status of an A2A task.
type TaskStatus struct {
	// State is the current state of the task.
	State string `json:"state"` // submitted, working, completed, failed, canceled

	// Message provides additional status information.
	Message string `json:"message,omitempty"`

	// Timestamp is when the status was last updated.
	Timestamp time.Time `json:"timestamp,omitempty"`
}

// TaskMessage represents a message in an A2A task.
type TaskMessage struct {
	// Role identifies the message sender role.
	Role string `json:"role"` // user, agent

	// Parts contains the message content parts.
	Parts []MessagePart `json:"parts"`

	// Metadata contains additional message metadata.
	Metadata map[string]any `json:"metadata,omitempty"`
}

// MessagePart represents a part of a task message.
type MessagePart struct {
	// Type identifies the part type.
	Type string `json:"type"` // text, file, data

	// Text contains text content (for type="text").
	Text string `json:"text,omitempty"`

	// File contains file information (for type="file").
	File *FilePart `json:"file,omitempty"`

	// Data contains structured data (for type="data").
	Data json.RawMessage `json:"data,omitempty"`
}

// FilePart represents a file in a message.
type FilePart struct {
	// Name is the filename.
	Name string `json:"name"`

	// MimeType is the content type.
	MimeType string `json:"mimeType"`

	// URI is the file location.
	URI string `json:"uri,omitempty"`

	// Bytes is base64-encoded file content.
	Bytes string `json:"bytes,omitempty"`
}

// Artifact represents an output artifact from a task.
type Artifact struct {
	// Name is the artifact name.
	Name string `json:"name"`

	// Description describes the artifact.
	Description string `json:"description,omitempty"`

	// Parts contains the artifact content.
	Parts []MessagePart `json:"parts"`

	// Index is the artifact position in the output sequence.
	Index int `json:"index,omitempty"`

	// Metadata contains additional artifact metadata.
	Metadata map[string]any `json:"metadata,omitempty"`
}

// TaskResult represents the result of a completed A2A task.
type TaskResult struct {
	// TaskID is the task identifier.
	TaskID string `json:"task_id"`

	// Status is the final task status.
	Status TaskStatus `json:"status"`

	// Artifacts are the task outputs.
	Artifacts []Artifact `json:"artifacts,omitempty"`

	// Error contains error information if the task failed.
	Error string `json:"error,omitempty"`
}

// ToTaskMessage converts an A2A task to a SemStreams TaskMessage.
func (m *TaskMapper) ToTaskMessage(task *Task, requesterDID string) (*agentic.TaskMessage, error) {
	if task == nil {
		return nil, fmt.Errorf("task cannot be nil")
	}

	// Extract text content from the task message
	prompt := m.extractTextContent(task.Message)
	if prompt == "" {
		return nil, fmt.Errorf("task message has no text content")
	}

	// Determine role based on metadata or default
	role := m.mapRole(task.Metadata)
	model := m.mapModel(task.Metadata)

	taskID := task.ID
	if taskID == "" {
		taskID = uuid.New().String()
	}

	msg := &agentic.TaskMessage{
		TaskID:      taskID,
		Prompt:      prompt,
		Role:        role,
		Model:       model,
		ChannelType: m.defaultChannelType,
		ChannelID:   task.SessionID,
		UserID:      requesterDID,
	}

	return msg, nil
}

// FromTaskResult converts a SemStreams result to an A2A task result.
func (m *TaskMapper) FromTaskResult(taskID string, result string, err error) *TaskResult {
	status := TaskStatus{
		State:     "completed",
		Timestamp: time.Now().UTC(),
	}

	var errStr string
	if err != nil {
		status.State = "failed"
		status.Message = err.Error()
		errStr = err.Error()
	}

	artifacts := []Artifact{}
	if result != "" {
		artifacts = append(artifacts, Artifact{
			Name:        "result",
			Description: "Task execution result",
			Parts: []MessagePart{
				{
					Type: "text",
					Text: result,
				},
			},
			Index: 0,
		})
	}

	return &TaskResult{
		TaskID:    taskID,
		Status:    status,
		Artifacts: artifacts,
		Error:     errStr,
	}
}

// CreateTaskStatusUpdate creates a status update for a task.
func (m *TaskMapper) CreateTaskStatusUpdate(taskID string, state string, message string) *Task {
	return &Task{
		ID: taskID,
		Status: TaskStatus{
			State:     state,
			Message:   message,
			Timestamp: time.Now().UTC(),
		},
	}
}

// extractTextContent extracts text from a task message.
func (m *TaskMapper) extractTextContent(msg TaskMessage) string {
	var texts []string
	for _, part := range msg.Parts {
		if part.Type == "text" && part.Text != "" {
			texts = append(texts, part.Text)
		}
	}

	if len(texts) == 0 {
		return ""
	}

	// Join multiple text parts with newlines
	result := texts[0]
	for i := 1; i < len(texts); i++ {
		result += "\n" + texts[i]
	}
	return result
}

// mapRole maps A2A metadata to an agentic role.
func (m *TaskMapper) mapRole(metadata map[string]any) string {
	if metadata == nil {
		return "general"
	}

	// Check for explicit role in metadata
	if role, ok := metadata["role"].(string); ok && role != "" {
		return role
	}

	// Check for capability hints
	if caps, ok := metadata["capabilities"].([]any); ok {
		for _, cap := range caps {
			if capStr, ok := cap.(string); ok {
				switch capStr {
				case "architecture", "design", "planning":
					return "architect"
				case "code", "implementation", "development":
					return "editor"
				}
			}
		}
	}

	return "general"
}

// mapModel maps A2A metadata to a model identifier.
func (m *TaskMapper) mapModel(metadata map[string]any) string {
	if metadata == nil {
		return "default"
	}

	if model, ok := metadata["model"].(string); ok && model != "" {
		return model
	}

	return "default"
}

// ParseTask parses a JSON task request.
func (m *TaskMapper) ParseTask(data []byte) (*Task, error) {
	var task Task
	if err := json.Unmarshal(data, &task); err != nil {
		return nil, fmt.Errorf("parse A2A task: %w", err)
	}
	return &task, nil
}

// SerializeTaskResult serializes a task result to JSON.
func (m *TaskMapper) SerializeTaskResult(result *TaskResult) ([]byte, error) {
	return json.Marshal(result)
}
