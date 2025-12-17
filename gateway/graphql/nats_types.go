package graphql

import "time"

// Entity represents a generic entity from the graph.
// This is a generic structure - Phase 2 will generate domain-specific types.
type Entity struct {
	ID         string                 `json:"id"`
	Type       string                 `json:"type"`
	Properties map[string]interface{} `json:"properties"`
	CreatedAt  time.Time              `json:"created_at,omitempty"`
	UpdatedAt  time.Time              `json:"updated_at,omitempty"`
}

// Relationship represents a generic relationship between entities.
type Relationship struct {
	FromEntityID string                 `json:"from_entity_id"`
	ToEntityID   string                 `json:"to_entity_id"`
	EdgeType     string                 `json:"edge_type"`
	Properties   map[string]interface{} `json:"properties,omitempty"`
	CreatedAt    time.Time              `json:"created_at,omitempty"`
}

// RelationshipFilters defines filters for relationship queries.
type RelationshipFilters struct {
	EntityID  string   `json:"entity_id"`
	Direction string   `json:"direction"` // "outgoing", "incoming", "both"
	EdgeTypes []string `json:"edge_types,omitempty"`
	Limit     int      `json:"limit,omitempty"`
}

// Community represents a community/cluster in the graph.
type Community struct {
	ID            string   `json:"id"`
	Level         int      `json:"level"`
	Members       []string `json:"members"`
	Summary       string   `json:"summary,omitempty"`
	Keywords      []string `json:"keywords,omitempty"`
	RepEntities   []string `json:"rep_entities,omitempty"`
	Summarizer    string   `json:"summarizer,omitempty"`
	SummaryStatus string   `json:"summary_status,omitempty"`
}

// CommunitySummary represents a community's summary with relevance score.
type CommunitySummary struct {
	CommunityID string   `json:"community_id"`
	Summary     string   `json:"summary"`
	Keywords    []string `json:"keywords"`
	Level       int      `json:"level"`
	Relevance   float64  `json:"relevance"`
}

// SnapshotRelationship represents a relationship within a graph snapshot.
type SnapshotRelationship struct {
	FromEntityID string `json:"from_entity_id"`
	ToEntityID   string `json:"to_entity_id"`
	EdgeType     string `json:"edge_type"`
}
