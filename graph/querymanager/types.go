// Package querymanager provides types for query operations.
package querymanager

import "time"

// PathPattern represents a graph traversal pattern
type PathPattern struct {
	MaxDepth    int                    `json:"max_depth"`
	EdgeTypes   []string               `json:"edge_types"`
	NodeTypes   []string               `json:"node_types"`
	Filters     map[string]interface{} `json:"filters"`
	Direction   Direction              `json:"direction"`
	IncludeSelf bool                   `json:"include_self"`

	// Resource limits (for context processor and other resource-conscious queries)
	MaxNodes    int           `json:"max_nodes,omitempty"`    // Max nodes to visit (0 = unlimited)
	MaxTime     time.Duration `json:"max_time,omitempty"`     // Query timeout (0 = use default)
	DecayFactor float64       `json:"decay_factor,omitempty"` // Relevance decay per hop (0 = no decay)

	// Hierarchical relationship inference based on 6-part EntityID structure
	// When enabled, PathRAG will infer relationships from EntityID prefixes
	IncludeSiblings bool `json:"include_siblings,omitempty"` // Include entities with same type prefix (siblings)
}

// QueryBounds represents spatial/temporal bounds for graph snapshots
type QueryBounds struct {
	Spatial     *SpatialBounds  `json:"spatial,omitempty"`
	Temporal    *TemporalBounds `json:"temporal,omitempty"`
	EntityTypes []string        `json:"entity_types,omitempty"`
	MaxEntities int             `json:"max_entities,omitempty"`
}

// SpatialBounds represents spatial query bounds
type SpatialBounds struct {
	North float64 `json:"north"`
	South float64 `json:"south"`
	East  float64 `json:"east"`
	West  float64 `json:"west"`
}

// TemporalBounds represents temporal query bounds
type TemporalBounds struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// Relationship represents a relationship between entities
type Relationship struct {
	FromEntityID string                 `json:"from_entity_id"`
	ToEntityID   string                 `json:"to_entity_id"`
	EdgeType     string                 `json:"edge_type"`
	Properties   map[string]interface{} `json:"properties"`
	Weight       float64                `json:"weight"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

// HierarchyStats represents entity counts grouped by EntityID hierarchy level.
// Used for navigating the implicit graph structure created by 6-part EntityIDs.
type HierarchyStats struct {
	// Prefix is the queried prefix (empty string = root)
	Prefix string `json:"prefix"`

	// TotalEntities is the count of all entities under this prefix
	TotalEntities int `json:"total_entities"`

	// Children contains breakdown by next hierarchy level
	Children []HierarchyLevel `json:"children"`
}

// HierarchyLevel represents a single level in the EntityID hierarchy
type HierarchyLevel struct {
	// Prefix is the full prefix for this level
	Prefix string `json:"prefix"`

	// Name is the human-readable name (last segment of prefix)
	Name string `json:"name"`

	// Count is the number of entities at or under this level
	Count int `json:"count"`
}
