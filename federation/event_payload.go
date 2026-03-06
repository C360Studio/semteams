package federation

import (
	"encoding/json"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/message"
)

// EventPayload implements message.Payload for federation graph events.
// It wraps Event for transport through the semstreams message bus.
type EventPayload struct {
	Event Event `json:"event"`
}

// Schema returns the message type for the Payload interface.
// The domain is set to "federation" as a default; consuming services register
// their own domain via RegisterPayload.
func (p *EventPayload) Schema() message.Type {
	return message.Type{Domain: "federation", Category: "graph_event", Version: "v1"}
}

// Validate validates the payload for the Payload interface.
func (p *EventPayload) Validate() error {
	return p.Event.Validate()
}

// MarshalJSON implements json.Marshaler.
func (p *EventPayload) MarshalJSON() ([]byte, error) {
	type Alias EventPayload
	return json.Marshal((*Alias)(p))
}

// UnmarshalJSON implements json.Unmarshaler.
func (p *EventPayload) UnmarshalJSON(data []byte) error {
	type Alias EventPayload
	return json.Unmarshal(data, (*Alias)(p))
}

// RegisterPayload registers the federation event payload for a specific domain.
// Each sem* service calls this with its own domain (e.g., "semsource", "semquery").
// This enables domain-specific message routing while sharing the same payload structure.
func RegisterPayload(domain string) error {
	return component.RegisterPayload(&component.PayloadRegistration{
		Domain:      domain,
		Category:    "graph_event",
		Version:     "v1",
		Description: "Federation graph event payload for " + domain,
		Factory:     func() any { return &EventPayload{} },
	})
}
