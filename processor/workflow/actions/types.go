package actions

import (
	"encoding/json"
	"fmt"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/message"
)

func init() {
	// Register AsyncTaskPayload factory for publish_async action
	err := component.RegisterPayload(&component.PayloadRegistration{
		Domain:      "workflow",
		Category:    "async_task",
		Version:     "v1",
		Description: "Async task payload with callback correlation",
		Factory:     func() any { return &AsyncTaskPayload{} },
	})
	if err != nil {
		panic("failed to register AsyncTaskPayload: " + err.Error())
	}
}

// AsyncTaskPayload represents a payload sent by publish_async action.
// It wraps the original payload with correlation fields for async callback handling.
type AsyncTaskPayload struct {
	// TaskID is the unique identifier for this async task.
	// Used to correlate the response with the original request.
	TaskID string `json:"task_id"`

	// CallbackSubject is the NATS subject where the response should be published.
	// Optional - may be empty if no callback is configured.
	CallbackSubject string `json:"callback_subject,omitempty"`

	// Data contains the original payload data.
	// Preserved as json.RawMessage for flexibility.
	Data json.RawMessage `json:"data,omitempty"`
}

// Schema implements message.Payload for proper serialization with BaseMessage.
func (a *AsyncTaskPayload) Schema() message.Type {
	return message.Type{Domain: "workflow", Category: "async_task", Version: "v1"}
}

// Validate checks if the AsyncTaskPayload is valid.
func (a *AsyncTaskPayload) Validate() error {
	if a.TaskID == "" {
		return fmt.Errorf("task_id is required")
	}
	return nil
}

// MarshalJSON implements json.Marshaler.
func (a *AsyncTaskPayload) MarshalJSON() ([]byte, error) {
	type Alias AsyncTaskPayload
	return json.Marshal((*Alias)(a))
}

// UnmarshalJSON implements json.Unmarshaler.
func (a *AsyncTaskPayload) UnmarshalJSON(data []byte) error {
	type Alias AsyncTaskPayload
	return json.Unmarshal(data, (*Alias)(a))
}
