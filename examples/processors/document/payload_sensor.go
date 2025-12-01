package document

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/c360/semstreams/message"
)

// SensorDocument represents rich-text documentation for a sensor.
type SensorDocument struct {
	// Input fields
	ID          string   `json:"id"`          // e.g., "sensor-doc-001"
	Title       string   `json:"title"`       // e.g., "Temperature Sensor T-42"
	Description string   `json:"description"` // Sensor description
	Body        string   `json:"body"`        // Detailed documentation
	Location    string   `json:"location"`    // Physical location description
	Reading     float64  `json:"reading"`     // Current/reference reading
	Unit        string   `json:"unit"`        // Unit of measurement
	Category    string   `json:"category"`    // temperature, pressure, humidity
	Tags        []string `json:"tags"`

	// Context fields
	OrgID    string `json:"-"`
	Platform string `json:"-"`
}

// EntityID returns a federated entity ID for the sensor document.
// Example: "acme.logistics.sensor.document.temperature.sensor-doc-001"
func (s *SensorDocument) EntityID() string {
	category := s.Category
	if category == "" {
		category = "general"
	}
	return fmt.Sprintf("%s.%s.sensor.document.%s.%s",
		s.OrgID,
		s.Platform,
		category,
		s.ID,
	)
}

// Triples returns semantic facts about this sensor document.
func (s *SensorDocument) Triples() []message.Triple {
	entityID := s.EntityID()
	now := time.Now()

	triples := []message.Triple{
		{
			Subject:    entityID,
			Predicate:  PredicateContentTitle,
			Object:     s.Title,
			Source:     tripleSourceName,
			Timestamp:  now,
			Confidence: defaultConfidence,
		},
		{
			Subject:    entityID,
			Predicate:  PredicateContentDescription,
			Object:     s.Description,
			Source:     tripleSourceName,
			Timestamp:  now,
			Confidence: defaultConfidence,
		},
		{
			Subject:    entityID,
			Predicate:  PredicateContentType,
			Object:     "sensor_doc",
			Source:     tripleSourceName,
			Timestamp:  now,
			Confidence: defaultConfidence,
		},
	}

	if s.Location != "" {
		triples = append(triples, message.Triple{
			Subject:    entityID,
			Predicate:  PredicateSensorLocation,
			Object:     s.Location,
			Source:     tripleSourceName,
			Timestamp:  now,
			Confidence: defaultConfidence,
		})
	}

	if s.Reading != 0 {
		triples = append(triples, message.Triple{
			Subject:    entityID,
			Predicate:  PredicateSensorReading,
			Object:     s.Reading,
			Source:     tripleSourceName,
			Timestamp:  now,
			Confidence: defaultConfidence,
		})
	}

	if s.Unit != "" {
		triples = append(triples, message.Triple{
			Subject:    entityID,
			Predicate:  PredicateSensorUnit,
			Object:     s.Unit,
			Source:     tripleSourceName,
			Timestamp:  now,
			Confidence: defaultConfidence,
		})
	}

	if s.Body != "" {
		triples = append(triples, message.Triple{
			Subject:    entityID,
			Predicate:  PredicateContentBody,
			Object:     s.Body,
			Source:     tripleSourceName,
			Timestamp:  now,
			Confidence: defaultConfidence,
		})
	}

	if s.Category != "" {
		triples = append(triples, message.Triple{
			Subject:    entityID,
			Predicate:  PredicateContentCategory,
			Object:     s.Category,
			Source:     tripleSourceName,
			Timestamp:  now,
			Confidence: defaultConfidence,
		})
	}

	for _, tag := range s.Tags {
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

// Schema returns the message type for sensor documents.
func (s *SensorDocument) Schema() message.Type {
	return message.Type{
		Domain:   "content",
		Category: "sensor_doc",
		Version:  "v1",
	}
}

// Validate checks required fields.
func (s *SensorDocument) Validate() error {
	if s.ID == "" {
		return fmt.Errorf("id is required")
	}
	if s.Title == "" {
		return fmt.Errorf("title is required")
	}
	if s.OrgID == "" {
		return fmt.Errorf("org_id is required (set by processor)")
	}
	if s.Platform == "" {
		return fmt.Errorf("platform is required (set by processor)")
	}
	return nil
}

// MarshalJSON implements json.Marshaler.
func (s *SensorDocument) MarshalJSON() ([]byte, error) {
	type Alias SensorDocument
	return json.Marshal((*Alias)(s))
}

// UnmarshalJSON implements json.Unmarshaler.
func (s *SensorDocument) UnmarshalJSON(data []byte) error {
	type Alias SensorDocument
	return json.Unmarshal(data, (*Alias)(s))
}
