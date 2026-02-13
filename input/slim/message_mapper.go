package slim

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/google/uuid"
)

// MessageMapper translates between SLIM messages and SemStreams messages.
type MessageMapper struct {
	// defaultChannelType is the channel type for SLIM messages.
	defaultChannelType string
}

// NewMessageMapper creates a new message mapper.
func NewMessageMapper() *MessageMapper {
	return &MessageMapper{
		defaultChannelType: "slim",
	}
}

// UserMessage represents a user message in SLIM format.
type UserMessage struct {
	// Type identifies the message type.
	Type string `json:"type"`

	// Content is the message content.
	Content string `json:"content"`

	// Attachments are optional file attachments.
	Attachments []Attachment `json:"attachments,omitempty"`

	// Metadata contains additional message metadata.
	Metadata map[string]any `json:"metadata,omitempty"`

	// ReplyTo is the message ID this is replying to.
	ReplyTo string `json:"reply_to,omitempty"`

	// ThreadID groups related messages.
	ThreadID string `json:"thread_id,omitempty"`
}

// Attachment represents an attachment in a SLIM message.
type Attachment struct {
	// Name is the filename.
	Name string `json:"name"`

	// MimeType is the content type.
	MimeType string `json:"mime_type"`

	// Data is the base64-encoded content.
	Data string `json:"data,omitempty"`

	// URL is an alternative to inline data.
	URL string `json:"url,omitempty"`

	// Size is the file size in bytes.
	Size int64 `json:"size,omitempty"`
}

// TaskDelegation represents a task delegation in SLIM format.
type TaskDelegation struct {
	// Type identifies this as a task message.
	Type string `json:"type"`

	// TaskID is the unique task identifier.
	TaskID string `json:"task_id"`

	// Prompt is the task description.
	Prompt string `json:"prompt"`

	// Role specifies the agent role for execution.
	Role string `json:"role,omitempty"`

	// Model specifies the LLM model to use.
	Model string `json:"model,omitempty"`

	// RequestingAgentDID is the DID of the requesting agent.
	RequestingAgentDID string `json:"requesting_agent_did"`

	// TargetCapabilities are the required capabilities.
	TargetCapabilities []string `json:"target_capabilities,omitempty"`

	// Priority is the task priority.
	Priority string `json:"priority,omitempty"`

	// Deadline is the task deadline.
	Deadline *time.Time `json:"deadline,omitempty"`

	// Context contains additional task context.
	Context map[string]any `json:"context,omitempty"`
}

// ResponseMessage represents a response in SLIM format.
type ResponseMessage struct {
	// Type identifies this as a response message.
	Type string `json:"type"`

	// InReplyTo is the original message/task ID.
	InReplyTo string `json:"in_reply_to"`

	// Status indicates success/failure.
	Status string `json:"status"`

	// Content is the response content.
	Content string `json:"content"`

	// Error contains error details if status is error.
	Error string `json:"error,omitempty"`

	// Metadata contains additional response metadata.
	Metadata map[string]any `json:"metadata,omitempty"`
}

// TaskResult represents the result of a completed task.
// This is used for outbound SLIM messages when tasks complete.
type TaskResult struct {
	// TaskID is the task identifier.
	TaskID string `json:"task_id"`

	// Result is the task output content.
	Result string `json:"result"`

	// Error contains error message if task failed.
	Error string `json:"error,omitempty"`

	// CompletedAt is when the task finished.
	CompletedAt time.Time `json:"completed_at"`
}

// ToUserMessage converts a SLIM message to a SemStreams UserMessage.
func (m *MessageMapper) ToUserMessage(slimMsg *Message) (*agentic.UserMessage, error) {
	// Parse the SLIM message content
	var userMsg UserMessage
	if err := json.Unmarshal(slimMsg.Content, &userMsg); err != nil {
		// If not JSON, treat as plain text
		userMsg = UserMessage{
			Type:    "text",
			Content: string(slimMsg.Content),
		}
	}

	// Create UserMessage
	msg := &agentic.UserMessage{
		MessageID:   slimMsg.MessageID,
		Content:     userMsg.Content,
		ChannelType: m.defaultChannelType,
		ChannelID:   slimMsg.GroupID,
		UserID:      slimMsg.SenderDID,
		Timestamp:   slimMsg.Timestamp,
	}

	// Map attachments
	if len(userMsg.Attachments) > 0 {
		msg.Attachments = make([]agentic.Attachment, len(userMsg.Attachments))
		for i, att := range userMsg.Attachments {
			msg.Attachments[i] = agentic.Attachment{
				Name:     att.Name,
				MimeType: att.MimeType,
				Content:  att.Data, // SLIM Data maps to agentic Content
				URL:      att.URL,
				Size:     att.Size,
			}
		}
	}

	// Store SLIM-specific metadata
	msg.Metadata = make(map[string]string)
	msg.Metadata["slim_group_id"] = slimMsg.GroupID
	msg.Metadata["slim_sender_did"] = slimMsg.SenderDID
	if userMsg.ReplyTo != "" {
		msg.Metadata["slim_reply_to"] = userMsg.ReplyTo
	}
	if userMsg.ThreadID != "" {
		msg.Metadata["slim_thread_id"] = userMsg.ThreadID
	}

	return msg, nil
}

// ToTaskMessage converts a SLIM task message to a SemStreams TaskMessage.
func (m *MessageMapper) ToTaskMessage(slimMsg *Message) (*agentic.TaskMessage, error) {
	var taskMsg TaskDelegation
	if err := json.Unmarshal(slimMsg.Content, &taskMsg); err != nil {
		return nil, fmt.Errorf("parse SLIM task message: %w", err)
	}

	// Determine role and model
	role := taskMsg.Role
	if role == "" {
		role = "general" // Default role for external tasks
	}

	model := taskMsg.Model
	if model == "" {
		model = "default" // Will be resolved by agentic-model component
	}

	// Map to TaskMessage
	msg := &agentic.TaskMessage{
		TaskID:      taskMsg.TaskID,
		Prompt:      taskMsg.Prompt,
		Role:        role,
		Model:       model,
		ChannelType: m.defaultChannelType,
		ChannelID:   slimMsg.GroupID,
		UserID:      taskMsg.RequestingAgentDID,
	}

	return msg, nil
}

// FromUserResponse converts a SemStreams response to a SLIM response message.
func (m *MessageMapper) FromUserResponse(response *agentic.UserResponse) ([]byte, error) {
	status := "success"
	errorMsg := ""

	// Check if response type indicates an error
	if response.Type == "error" {
		status = "error"
		errorMsg = response.Content // Error content is in the Content field
	}

	slimResp := ResponseMessage{
		Type:      "response",
		InReplyTo: response.InReplyTo,
		Status:    status,
		Content:   response.Content,
		Error:     errorMsg,
	}

	return json.Marshal(slimResp)
}

// FromTaskResult converts a task result to a SLIM response message.
func (m *MessageMapper) FromTaskResult(result *TaskResult) ([]byte, error) {
	status := "success"
	if result.Error != "" {
		status = "error"
	}

	slimResp := ResponseMessage{
		Type:      "task_result",
		InReplyTo: result.TaskID,
		Status:    status,
		Content:   result.Result,
		Error:     result.Error,
		Metadata: map[string]any{
			"completed_at": result.CompletedAt.Format(time.RFC3339),
		},
	}

	return json.Marshal(slimResp)
}

// CreateTaskMessage creates a new SLIM task message for delegation.
func (m *MessageMapper) CreateTaskMessage(prompt string, requesterDID string, capabilities []string) ([]byte, error) {
	taskMsg := TaskDelegation{
		Type:               "task",
		TaskID:             uuid.New().String(),
		Prompt:             prompt,
		RequestingAgentDID: requesterDID,
		TargetCapabilities: capabilities,
	}

	return json.Marshal(taskMsg)
}

// ParseMessageType determines the type of a SLIM message.
func (m *MessageMapper) ParseMessageType(content []byte) (string, error) {
	var msg struct {
		Type string `json:"type"`
	}

	if err := json.Unmarshal(content, &msg); err != nil {
		// Default to user message for plain text
		return "user", nil
	}

	switch msg.Type {
	case "task":
		return "task", nil
	case "response", "task_result":
		return "response", nil
	default:
		return "user", nil
	}
}
