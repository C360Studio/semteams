package agenticgovernance

import (
	"context"
	"time"
)

// Filter defines the interface all governance filters must implement
type Filter interface {
	// Name returns the unique filter identifier
	Name() string

	// Process examines a message and returns a filtering decision
	Process(ctx context.Context, msg *Message) (*FilterResult, error)
}

// FilterResult encapsulates the outcome of a filter's processing
type FilterResult struct {
	// Allowed indicates whether the message should proceed
	Allowed bool

	// Modified contains the potentially altered message (nil if unchanged)
	// Used for redaction filters that modify content
	Modified *Message

	// Violation contains details if a policy was violated
	Violation *Violation

	// Confidence indicates the filter's certainty (0.0-1.0)
	Confidence float64

	// Metadata provides additional context for downstream processing
	Metadata map[string]any
}

// Message represents an agentic message being processed
type Message struct {
	// ID is unique message identifier
	ID string `json:"id"`

	// Type is message type: task, request, or response
	Type MessageType `json:"type"`

	// UserID of the user who initiated the message
	UserID string `json:"user_id"`

	// SessionID of the session
	SessionID string `json:"session_id"`

	// ChannelID where message originated
	ChannelID string `json:"channel_id"`

	// Timestamp when message was created
	Timestamp time.Time `json:"timestamp"`

	// Content holds the message payload
	Content Content `json:"content"`
}

// MessageType categorizes the message flow direction
type MessageType string

const (
	// MessageTypeTask is a user task request
	MessageTypeTask MessageType = "task"

	// MessageTypeRequest is an outgoing model request
	MessageTypeRequest MessageType = "request"

	// MessageTypeResponse is an incoming model response
	MessageTypeResponse MessageType = "response"
)

// Content holds message content
type Content struct {
	// Text is the main message text
	Text string `json:"text"`

	// Metadata holds additional message context
	Metadata map[string]any `json:"metadata,omitempty"`
}

// Clone creates a deep copy of the message
func (m *Message) Clone() *Message {
	clone := &Message{
		ID:        m.ID,
		Type:      m.Type,
		UserID:    m.UserID,
		SessionID: m.SessionID,
		ChannelID: m.ChannelID,
		Timestamp: m.Timestamp,
		Content: Content{
			Text:     m.Content.Text,
			Metadata: make(map[string]any),
		},
	}

	// Deep copy metadata
	for k, v := range m.Content.Metadata {
		clone.Content.Metadata[k] = v
	}

	return clone
}

// SetMetadata sets a metadata value on the message content
func (m *Message) SetMetadata(key string, value any) {
	if m.Content.Metadata == nil {
		m.Content.Metadata = make(map[string]any)
	}
	m.Content.Metadata[key] = value
}

// GetMetadata gets a metadata value from the message content
func (m *Message) GetMetadata(key string) (any, bool) {
	if m.Content.Metadata == nil {
		return nil, false
	}
	v, ok := m.Content.Metadata[key]
	return v, ok
}

// NewFilterResult creates a new FilterResult with default values
func NewFilterResult(allowed bool) *FilterResult {
	return &FilterResult{
		Allowed:    allowed,
		Confidence: 1.0,
		Metadata:   make(map[string]any),
	}
}

// WithModified sets the modified message on the result
func (r *FilterResult) WithModified(msg *Message) *FilterResult {
	r.Modified = msg
	return r
}

// WithViolation sets the violation on the result
func (r *FilterResult) WithViolation(v *Violation) *FilterResult {
	r.Violation = v
	r.Allowed = false
	return r
}

// WithConfidence sets the confidence on the result
func (r *FilterResult) WithConfidence(c float64) *FilterResult {
	r.Confidence = c
	return r
}

// WithMetadata sets metadata on the result
func (r *FilterResult) WithMetadata(key string, value any) *FilterResult {
	if r.Metadata == nil {
		r.Metadata = make(map[string]any)
	}
	r.Metadata[key] = value
	return r
}
