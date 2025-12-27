package graphql

import "time"

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
