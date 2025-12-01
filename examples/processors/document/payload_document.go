package document

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/c360/semstreams/message"
)

// Document represents a generic document entity. It implements the Graphable
// interface with federated entity IDs and semantic predicates.
type Document struct {
	// Input fields (from incoming JSON)
	ID          string   `json:"id"`          // e.g., "doc-001"
	Title       string   `json:"title"`       // e.g., "Safety Manual"
	Description string   `json:"description"` // Primary text for semantic search
	Body        string   `json:"body"`        // Full text content
	Summary     string   `json:"summary"`     // Brief summary
	Category    string   `json:"category"`    // e.g., "safety", "operations"
	Tags        []string `json:"tags"`        // Classification tags
	CreatedAt   string   `json:"created_at"`  // ISO timestamp
	UpdatedAt   string   `json:"updated_at"`  // ISO timestamp

	// Context fields (set by processor from config)
	OrgID    string `json:"-"` // e.g., "acme"
	Platform string `json:"-"` // e.g., "logistics"
}

// EntityID returns a deterministic 6-part federated entity ID following the pattern:
// {org}.{platform}.{domain}.{system}.{type}.{instance}
//
// Example: "acme.logistics.content.document.safety.doc-001"
func (d *Document) EntityID() string {
	category := d.Category
	if category == "" {
		category = "general"
	}
	return fmt.Sprintf("%s.%s.content.document.%s.%s",
		d.OrgID,
		d.Platform,
		category,
		d.ID,
	)
}

// Triples returns semantic facts about this document using domain-appropriate predicates.
func (d *Document) Triples() []message.Triple {
	entityID := d.EntityID()
	now := time.Now()

	triples := []message.Triple{
		// Title - important for search
		{
			Subject:    entityID,
			Predicate:  PredicateContentTitle,
			Object:     d.Title,
			Source:     tripleSourceName,
			Timestamp:  now,
			Confidence: defaultConfidence,
		},
		// Description - primary semantic search field
		{
			Subject:    entityID,
			Predicate:  PredicateContentDescription,
			Object:     d.Description,
			Source:     tripleSourceName,
			Timestamp:  now,
			Confidence: defaultConfidence,
		},
		// Content type classification
		{
			Subject:    entityID,
			Predicate:  PredicateContentType,
			Object:     "document",
			Source:     tripleSourceName,
			Timestamp:  now,
			Confidence: defaultConfidence,
		},
	}

	// Optional fields
	if d.Body != "" {
		triples = append(triples, message.Triple{
			Subject:    entityID,
			Predicate:  PredicateContentBody,
			Object:     d.Body,
			Source:     tripleSourceName,
			Timestamp:  now,
			Confidence: defaultConfidence,
		})
	}

	if d.Summary != "" {
		triples = append(triples, message.Triple{
			Subject:    entityID,
			Predicate:  PredicateContentSummary,
			Object:     d.Summary,
			Source:     tripleSourceName,
			Timestamp:  now,
			Confidence: defaultConfidence,
		})
	}

	if d.Category != "" {
		triples = append(triples, message.Triple{
			Subject:    entityID,
			Predicate:  PredicateContentCategory,
			Object:     d.Category,
			Source:     tripleSourceName,
			Timestamp:  now,
			Confidence: defaultConfidence,
		})
	}

	// Add tags as separate triples
	for _, tag := range d.Tags {
		triples = append(triples, message.Triple{
			Subject:    entityID,
			Predicate:  PredicateContentTag,
			Object:     tag,
			Source:     tripleSourceName,
			Timestamp:  now,
			Confidence: defaultConfidence,
		})
	}

	// Time predicates - log warnings for invalid timestamps instead of silently skipping
	if d.CreatedAt != "" {
		ts, err := time.Parse(time.RFC3339, d.CreatedAt)
		if err != nil {
			slog.Warn("invalid created_at timestamp",
				"entity_id", entityID,
				"value", d.CreatedAt,
				"error", err)
		} else {
			triples = append(triples, message.Triple{
				Subject:    entityID,
				Predicate:  PredicateTimeCreated,
				Object:     ts,
				Source:     tripleSourceName,
				Timestamp:  now,
				Confidence: defaultConfidence,
			})
		}
	}

	if d.UpdatedAt != "" {
		ts, err := time.Parse(time.RFC3339, d.UpdatedAt)
		if err != nil {
			slog.Warn("invalid updated_at timestamp",
				"entity_id", entityID,
				"value", d.UpdatedAt,
				"error", err)
		} else {
			triples = append(triples, message.Triple{
				Subject:    entityID,
				Predicate:  PredicateTimeUpdated,
				Object:     ts,
				Source:     tripleSourceName,
				Timestamp:  now,
				Confidence: defaultConfidence,
			})
		}
	}

	return triples
}

// Schema returns the message type for documents.
func (d *Document) Schema() message.Type {
	return message.Type{
		Domain:   "content",
		Category: "document",
		Version:  "v1",
	}
}

// Validate checks that the document has all required fields.
func (d *Document) Validate() error {
	if d.ID == "" {
		return fmt.Errorf("id is required")
	}
	if d.Title == "" {
		return fmt.Errorf("title is required")
	}
	if d.OrgID == "" {
		return fmt.Errorf("org_id is required (set by processor)")
	}
	if d.Platform == "" {
		return fmt.Errorf("platform is required (set by processor)")
	}
	return nil
}

// MarshalJSON implements json.Marshaler for Document.
func (d *Document) MarshalJSON() ([]byte, error) {
	type Alias Document
	return json.Marshal((*Alias)(d))
}

// UnmarshalJSON implements json.Unmarshaler for Document.
func (d *Document) UnmarshalJSON(data []byte) error {
	type Alias Document
	return json.Unmarshal(data, (*Alias)(d))
}
