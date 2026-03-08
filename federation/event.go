package federation

import (
	"errors"
	"fmt"
	"time"
)

// EventType is the type of a graph event emitted during federation.
type EventType string

const (
	// EventTypeSEED is emitted on startup and consumer reconnect with a full graph snapshot.
	EventTypeSEED EventType = "SEED"

	// EventTypeDELTA is emitted for incremental upserts from watch events.
	EventTypeDELTA EventType = "DELTA"

	// EventTypeRETRACT is emitted when entities are explicitly removed.
	EventTypeRETRACT EventType = "RETRACT"

	// EventTypeHEARTBEAT is emitted during quiet periods as a liveness signal.
	EventTypeHEARTBEAT EventType = "HEARTBEAT"
)

// Event represents a single graph mutation event carrying one entity.
// Events flow between services over WebSocket for graph-ingest processing.
type Event struct {
	// Type is the event type enum.
	Type EventType `json:"type"`

	// SourceID identifies the source that produced this event.
	SourceID string `json:"source_id"`

	// Namespace is the org namespace for this event (e.g., "acme", "public").
	Namespace string `json:"namespace"`

	// Timestamp is when the event was generated.
	Timestamp time.Time `json:"timestamp"`

	// Entity is the graph entity for SEED and DELTA events.
	Entity Entity `json:"entity"`

	// Retractions contains entity IDs to remove for RETRACT events.
	Retractions []string `json:"retractions,omitempty"`
}

// ValidEventTypes is the set of known event types for validation.
var ValidEventTypes = map[EventType]bool{
	EventTypeSEED:      true,
	EventTypeDELTA:     true,
	EventTypeRETRACT:   true,
	EventTypeHEARTBEAT: true,
}

// Validate checks that the Event contains all required fields and a known event type.
func (e *Event) Validate() error {
	if e.Type == "" {
		return errors.New("event type is required")
	}
	if !ValidEventTypes[e.Type] {
		return fmt.Errorf("unknown event type %q", e.Type)
	}
	if e.SourceID == "" {
		return errors.New("source ID is required")
	}
	if e.Namespace == "" {
		return errors.New("namespace is required")
	}
	if e.Timestamp.IsZero() {
		return errors.New("timestamp is required")
	}
	return nil
}
