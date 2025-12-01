package document

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/c360/semstreams/message"
)

// Maintenance represents a maintenance record entity.
type Maintenance struct {
	// Input fields
	ID             string   `json:"id"`              // e.g., "maint-001"
	Title          string   `json:"title"`           // e.g., "Pump Repair"
	Description    string   `json:"description"`     // Work description
	Body           string   `json:"body"`            // Detailed work log
	Technician     string   `json:"technician"`      // Who performed the work
	Status         string   `json:"status"`          // completed, pending, in_progress
	CompletionDate string   `json:"completion_date"` // ISO timestamp
	Category       string   `json:"category"`        // equipment, facility, etc.
	Tags           []string `json:"tags"`

	// Context fields
	OrgID    string `json:"-"`
	Platform string `json:"-"`
}

// EntityID returns a federated entity ID for the maintenance record.
// Example: "acme.logistics.maintenance.work.completed.maint-001"
func (m *Maintenance) EntityID() string {
	status := m.Status
	if status == "" {
		status = "pending"
	}
	return fmt.Sprintf("%s.%s.maintenance.work.%s.%s",
		m.OrgID,
		m.Platform,
		status,
		m.ID,
	)
}

// Triples returns semantic facts about this maintenance record.
func (m *Maintenance) Triples() []message.Triple {
	entityID := m.EntityID()
	now := time.Now()

	triples := []message.Triple{
		{
			Subject:    entityID,
			Predicate:  PredicateContentTitle,
			Object:     m.Title,
			Source:     tripleSourceName,
			Timestamp:  now,
			Confidence: defaultConfidence,
		},
		{
			Subject:    entityID,
			Predicate:  PredicateContentDescription,
			Object:     m.Description,
			Source:     tripleSourceName,
			Timestamp:  now,
			Confidence: defaultConfidence,
		},
		{
			Subject:    entityID,
			Predicate:  PredicateContentType,
			Object:     "maintenance",
			Source:     tripleSourceName,
			Timestamp:  now,
			Confidence: defaultConfidence,
		},
		{
			Subject:    entityID,
			Predicate:  PredicateMaintenanceStatus,
			Object:     m.Status,
			Source:     tripleSourceName,
			Timestamp:  now,
			Confidence: defaultConfidence,
		},
	}

	if m.Technician != "" {
		triples = append(triples, message.Triple{
			Subject:    entityID,
			Predicate:  PredicateMaintenanceTechnician,
			Object:     m.Technician,
			Source:     tripleSourceName,
			Timestamp:  now,
			Confidence: defaultConfidence,
		})
	}

	if m.CompletionDate != "" {
		ts, err := time.Parse(time.RFC3339, m.CompletionDate)
		if err != nil {
			slog.Warn("invalid completion_date timestamp",
				"entity_id", entityID,
				"value", m.CompletionDate,
				"error", err)
		} else {
			triples = append(triples, message.Triple{
				Subject:    entityID,
				Predicate:  PredicateMaintenanceDate,
				Object:     ts,
				Source:     tripleSourceName,
				Timestamp:  now,
				Confidence: defaultConfidence,
			})
		}
	}

	if m.Body != "" {
		triples = append(triples, message.Triple{
			Subject:    entityID,
			Predicate:  PredicateContentBody,
			Object:     m.Body,
			Source:     tripleSourceName,
			Timestamp:  now,
			Confidence: defaultConfidence,
		})
	}

	if m.Category != "" {
		triples = append(triples, message.Triple{
			Subject:    entityID,
			Predicate:  PredicateContentCategory,
			Object:     m.Category,
			Source:     tripleSourceName,
			Timestamp:  now,
			Confidence: defaultConfidence,
		})
	}

	for _, tag := range m.Tags {
		triples = append(triples, message.Triple{
			Subject:    entityID,
			Predicate:  PredicateContentTag,
			Object:     tag,
			Source:     tripleSourceName,
			Timestamp:  now,
			Confidence: defaultConfidence,
		})
	}

	return triples
}

// Schema returns the message type for maintenance records.
func (m *Maintenance) Schema() message.Type {
	return message.Type{
		Domain:   "content",
		Category: "maintenance",
		Version:  "v1",
	}
}

// Validate checks required fields.
func (m *Maintenance) Validate() error {
	if m.ID == "" {
		return fmt.Errorf("id is required")
	}
	if m.Title == "" {
		return fmt.Errorf("title is required")
	}
	if m.OrgID == "" {
		return fmt.Errorf("org_id is required (set by processor)")
	}
	if m.Platform == "" {
		return fmt.Errorf("platform is required (set by processor)")
	}
	return nil
}

// MarshalJSON implements json.Marshaler.
func (m *Maintenance) MarshalJSON() ([]byte, error) {
	type Alias Maintenance
	return json.Marshal((*Alias)(m))
}

// UnmarshalJSON implements json.Unmarshaler.
func (m *Maintenance) UnmarshalJSON(data []byte) error {
	type Alias Maintenance
	return json.Unmarshal(data, (*Alias)(m))
}
