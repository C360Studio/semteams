package graphql

import "time"

// Entity represents a generic entity from the graph.
type Entity struct {
	ID         string                 `json:"id"`
	Type       string                 `json:"type"`
	Properties map[string]interface{} `json:"properties"`
	CreatedAt  time.Time              `json:"created_at,omitempty"`
	UpdatedAt  time.Time              `json:"updated_at,omitempty"`
	Score      float64                `json:"score,omitempty"` // Similarity score for search results
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

// SimilaritySearchResult represents a similarity search result.
type SimilaritySearchResult struct {
	Entity *Entity `json:"entity"`
	Score  float64 `json:"score"`
}

// LocalSearchResult represents the result of a local community search.
type LocalSearchResult struct {
	Entities    []*Entity `json:"entities"`
	CommunityID string    `json:"community_id"`
	Count       int       `json:"count"`
}

// GlobalSearchResult represents the result of a global cross-community search.
type GlobalSearchResult struct {
	Entities           []*Entity          `json:"entities"`
	CommunitySummaries []CommunitySummary `json:"community_summaries"`
	Count              int                `json:"count"`
}

// PathSearchResult represents the result of a path traversal query (PathRAG).
type PathSearchResult struct {
	Entities  []*PathEntity `json:"entities"`
	Paths     [][]PathStep  `json:"paths"`
	Truncated bool          `json:"truncated"`
}

// PathEntity represents an entity discovered during path traversal.
type PathEntity struct {
	ID         string                 `json:"id"`
	Type       string                 `json:"type"`
	Score      float64                `json:"score"`
	Properties map[string]interface{} `json:"properties,omitempty"`
}

// PathStep represents a single edge in a traversal path.
type PathStep struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Predicate string `json:"predicate"`
}

// GraphSnapshot represents a bounded spatial/temporal subgraph.
type GraphSnapshot struct {
	Entities      []*Entity              `json:"entities"`
	Relationships []SnapshotRelationship `json:"relationships"`
	Count         int                    `json:"count"`
	Truncated     bool                   `json:"truncated"`
	Timestamp     time.Time              `json:"timestamp"`
}

// PrefixQueryResult represents the result of an EntityID prefix query.
type PrefixQueryResult struct {
	EntityIDs  []string `json:"entity_ids"`
	TotalCount int      `json:"total_count"`
	Truncated  bool     `json:"truncated"`
	Prefix     string   `json:"prefix"`
}

// HierarchyStats represents EntityID hierarchy statistics.
type HierarchyStats struct {
	Prefix        string           `json:"prefix"`
	TotalEntities int              `json:"total_entities"`
	Children      []HierarchyLevel `json:"children"`
}

// HierarchyLevel represents a single level in the EntityID hierarchy.
type HierarchyLevel struct {
	Prefix string `json:"prefix"`
	Name   string `json:"name"`
	Count  int    `json:"count"`
}
