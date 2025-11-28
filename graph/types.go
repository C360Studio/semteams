// Package graph provides types for entity state storage in the graph system.
package graph

import (
	"encoding/json"
	"time"

	"github.com/c360/semstreams/message"
)

// EntityStatus represents the operational status of an entity in the graph.
// This enum provides type-safe status values for entity monitoring and alerting.
type EntityStatus string

const (
	// StatusActive indicates the entity is operating normally.
	// This is the default healthy state for most entities.
	StatusActive EntityStatus = "active"

	// StatusWarning indicates the entity has detected issues but is still operational.
	// Example: low battery, performance degradation, minor failures
	StatusWarning EntityStatus = "warning"

	// StatusCritical indicates the entity has serious issues that require immediate attention.
	// Example: very low battery, major component failure, safety concerns
	StatusCritical EntityStatus = "critical"

	// StatusEmergency indicates the entity is in an emergency state requiring immediate intervention.
	// Example: critical battery level, system failure, safety hazard
	StatusEmergency EntityStatus = "emergency"

	// StatusInactive indicates the entity is not currently active or operational.
	// Example: powered down, offline, disabled
	StatusInactive EntityStatus = "inactive"

	// StatusUnknown indicates the entity status cannot be determined.
	// This is used when status information is unavailable or invalid.
	StatusUnknown EntityStatus = "unknown"
)

// String returns the string representation of the EntityStatus.
func (es EntityStatus) String() string {
	return string(es)
}

// MarshalJSON implements json.Marshaler to ensure EntityStatus serializes as a string.
func (es EntityStatus) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(es))
}

// UnmarshalJSON implements json.Unmarshaler to deserialize EntityStatus from string.
// This provides backward compatibility with existing string status values.
func (es *EntityStatus) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	*es = EntityStatus(s)
	return nil
}

// IsValid checks if the EntityStatus is one of the defined constants.
func (es EntityStatus) IsValid() bool {
	switch es {
	case StatusActive, StatusWarning, StatusCritical, StatusEmergency, StatusInactive, StatusUnknown:
		return true
	default:
		return false
	}
}

// IsHealthy returns true if the entity status indicates normal operation.
func (es EntityStatus) IsHealthy() bool {
	return es == StatusActive
}

// NeedsAttention returns true if the entity status indicates issues requiring attention.
func (es EntityStatus) NeedsAttention() bool {
	return es == StatusWarning || es == StatusCritical || es == StatusEmergency
}

// IsOperational returns true if the entity is currently operational (active or warning).
func (es EntityStatus) IsOperational() bool {
	return es == StatusActive || es == StatusWarning
}

// EntityState represents complete local graph state for an entity
type EntityState struct {
	Node        NodeProperties   `json:"node"`         // Entity properties
	Triples     []message.Triple `json:"triples"`      // Semantic triples (properties + relationships)
	ObjectRef   string           `json:"object_ref"`   // Reference to full message in ObjectStore
	MessageType string           `json:"message_type"` // Original message type (domain.category.version)
	Version     uint64           `json:"version"`      // Explicit versioning for conflict resolution
	UpdatedAt   time.Time        `json:"updated_at"`
}

// NodeProperties contains entity identification and query-essential properties
type NodeProperties struct {
	ID       string       `json:"id"`   // e.g., "drone_001"
	Type     string       `json:"type"` // e.g., "robotics.drone"
	Position *Position    `json:"position,omitempty"`
	Status   EntityStatus `json:"status"` // Entity operational status
}

// Position represents geographic location
type Position struct {
	Latitude  float64 `json:"lat"`
	Longitude float64 `json:"lon"`
	Altitude  float64 `json:"alt,omitempty"`
}

// GetTriple returns the first triple matching the given predicate.
// Returns nil if no matching triple is found.
// This helper method simplifies accessing triple-based properties.
func (es *EntityState) GetTriple(predicate string) *message.Triple {
	if es == nil {
		return nil
	}
	for i := range es.Triples {
		if es.Triples[i].Predicate == predicate {
			return &es.Triples[i]
		}
	}
	return nil
}

// GetPropertyValue returns the value for a property by predicate.
// It checks Triples for a matching predicate and returns the Object value.
// Returns (value, true) if found, (nil, false) if not found.
func (es *EntityState) GetPropertyValue(predicate string) (any, bool) {
	if es == nil {
		return nil, false
	}

	triple := es.GetTriple(predicate)
	if triple != nil {
		return triple.Object, true
	}

	return nil, false
}
