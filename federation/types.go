package federation

import (
	"time"

	"github.com/c360studio/semstreams/message"
)

// Entity represents a normalized knowledge graph entity for cross-service exchange.
// It carries triples, explicit edges, and provenance for federation merge operations.
//
// The ID field is the 6-part entity identifier: org.platform.domain.system.type.instance
// which must be a valid NATS KV key.
type Entity struct {
	// ID is the deterministic 6-part entity identifier.
	ID string `json:"id"`

	// Triples are the single source of truth for all semantic properties.
	Triples []message.Triple `json:"triples"`

	// Edges represent explicit relationships to other entities.
	Edges []Edge `json:"edges,omitempty"`

	// Provenance records the primary (most recent) origin of this entity.
	Provenance Provenance `json:"provenance"`

	// AdditionalProvenance accumulates provenance records from prior merges.
	// The federation processor appends previous Provenance here on each merge.
	// This field is always appended, never replaced.
	AdditionalProvenance []Provenance `json:"additional_provenance,omitempty"`
}

// Edge represents a directed relationship between two graph entities.
type Edge struct {
	// FromID is the source entity's 6-part ID.
	FromID string `json:"from_id"`

	// ToID is the target entity's 6-part ID.
	ToID string `json:"to_id"`

	// EdgeType describes the semantic relationship (e.g., "authored_by", "imports", "calls").
	EdgeType string `json:"edge_type"`

	// Weight is an optional edge weight (0.0 = unweighted, positive = weighted).
	Weight float64 `json:"weight,omitempty"`

	// Properties holds any additional edge metadata.
	Properties map[string]any `json:"properties,omitempty"`
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
