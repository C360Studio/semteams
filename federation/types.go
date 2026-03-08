package federation

import (
	"time"

	"github.com/c360studio/semstreams/message"
)

// Entity represents a normalized knowledge graph entity for cross-service exchange.
// It carries an ID and triples — the same data model as graph.Graphable.
// Relationships are encoded as triples, not as a separate edge structure.
//
// The ID field is the 6-part entity identifier: org.platform.domain.system.type.instance
// which must be a valid NATS KV key.
type Entity struct {
	// ID is the deterministic 6-part entity identifier.
	ID string `json:"id"`

	// Triples are the single source of truth for all semantic properties and relationships.
	Triples []message.Triple `json:"triples"`

	// Provenance records the origin of this entity.
	Provenance Provenance `json:"provenance"`
}

// Provenance records the origin of an entity or event.
type Provenance struct {
	// SourceType identifies the class of source (e.g., "git", "ast", "url", "doc", "config").
	SourceType string `json:"source_type"`

	// SourceID is the unique identifier for the specific source instance.
	SourceID string `json:"source_id"`

	// Timestamp records when this provenance record was created.
	Timestamp time.Time `json:"timestamp"`

	// Handler is the name of the handler that produced this entity.
	Handler string `json:"handler"`
}
