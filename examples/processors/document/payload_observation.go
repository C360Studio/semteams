package document

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/c360/semstreams/message"
)

// Observation represents an observation or inspection record.
type Observation struct {
	// Input fields
	ID          string   `json:"id"`          // e.g., "obs-001"
	Title       string   `json:"title"`       // e.g., "Safety Hazard Report"
	Description string   `json:"description"` // What was observed
	Body        string   `json:"body"`        // Detailed notes
	Observer    string   `json:"observer"`    // Who made the observation
	Severity    string   `json:"severity"`    // low, medium, high, critical
	ObservedAt  string   `json:"observed_at"` // ISO timestamp
	Category    string   `json:"category"`    // safety, quality, environment
	Tags        []string `json:"tags"`

	// Context fields
	OrgID    string `json:"-"`
	Platform string `json:"-"`
}

// EntityID returns a federated entity ID for the observation.
// Example: "acme.logistics.observation.record.high.obs-001"
func (o *Observation) EntityID() string {
	severity := o.Severity
	if severity == "" {
		severity = "medium"
	}
	return fmt.Sprintf("%s.%s.observation.record.%s.%s",
		o.OrgID,
		o.Platform,
		severity,
		o.ID,
	)
}

// Triples returns semantic facts about this observation.
func (o *Observation) Triples() []message.Triple {
	entityID := o.EntityID()
	now := time.Now()

	triples := []message.Triple{
		{
			Subject:    entityID,
			Predicate:  PredicateContentTitle,
			Object:     o.Title,
			Source:     tripleSourceName,
			Timestamp:  now,
			Confidence: defaultConfidence,
		},
		{
			Subject:    entityID,
			Predicate:  PredicateContentDescription,
			Object:     o.Description,
			Source:     tripleSourceName,
			Timestamp:  now,
			Confidence: defaultConfidence,
		},
		{
			Subject:    entityID,
			Predicate:  PredicateContentType,
			Object:     "observation",
			Source:     tripleSourceName,
			Timestamp:  now,
			Confidence: defaultConfidence,
		},
		{
			Subject:    entityID,
			Predicate:  PredicateObservationSeverity,
			Object:     o.Severity,
			Source:     tripleSourceName,
			Timestamp:  now,
			Confidence: defaultConfidence,
		},
	}

	if o.Observer != "" {
		triples = append(triples, message.Triple{
			Subject:    entityID,
			Predicate:  PredicateObservationObserver,
			Object:     o.Observer,
			Source:     tripleSourceName,
			Timestamp:  now,
			Confidence: defaultConfidence,
		})
	}

	if o.ObservedAt != "" {
		ts, err := time.Parse(time.RFC3339, o.ObservedAt)
		if err != nil {
			slog.Warn("invalid observed_at timestamp",
				"entity_id", entityID,
				"value", o.ObservedAt,
				"error", err)
		} else {
			triples = append(triples, message.Triple{
				Subject:    entityID,
				Predicate:  PredicateObservationObservedAt,
				Object:     ts,
				Source:     tripleSourceName,
				Timestamp:  now,
				Confidence: defaultConfidence,
			})
		}
	}

	if o.Body != "" {
		triples = append(triples, message.Triple{
			Subject:    entityID,
			Predicate:  PredicateContentBody,
			Object:     o.Body,
			Source:     tripleSourceName,
			Timestamp:  now,
			Confidence: defaultConfidence,
		})
	}

	if o.Category != "" {
		triples = append(triples, message.Triple{
			Subject:    entityID,
			Predicate:  PredicateContentCategory,
			Object:     o.Category,
			Source:     tripleSourceName,
			Timestamp:  now,
			Confidence: defaultConfidence,
		})
	}

	for _, tag := range o.Tags {
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

// Schema returns the message type for observations.
func (o *Observation) Schema() message.Type {
	return message.Type{
		Domain:   "content",
		Category: "observation",
		Version:  "v1",
	}
}

// Validate checks required fields.
func (o *Observation) Validate() error {
	if o.ID == "" {
		return fmt.Errorf("id is required")
	}
	if o.Title == "" {
		return fmt.Errorf("title is required")
	}
	if o.OrgID == "" {
		return fmt.Errorf("org_id is required (set by processor)")
	}
	if o.Platform == "" {
		return fmt.Errorf("platform is required (set by processor)")
	}
	return nil
}

// MarshalJSON implements json.Marshaler.
func (o *Observation) MarshalJSON() ([]byte, error) {
	type Alias Observation
	return json.Marshal((*Alias)(o))
}

// UnmarshalJSON implements json.Unmarshaler.
func (o *Observation) UnmarshalJSON(data []byte) error {
	type Alias Observation
	return json.Unmarshal(data, (*Alias)(o))
}
