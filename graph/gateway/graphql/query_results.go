// Package graphql provides GraphQL gateway types and query definitions.
package graphql

import (
	"time"

	gtypes "github.com/c360/semstreams/graph"
)

// QMRelationship represents a relationship between entities (internal query type).
type QMRelationship struct {
	FromEntityID string                 `json:"from_entity_id"`
	ToEntityID   string                 `json:"to_entity_id"`
	EdgeType     string                 `json:"edge_type"`
	Properties   map[string]interface{} `json:"properties"`
	Weight       float64                `json:"weight"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

// Truncation reason constants for QueryResult.
const (
	TruncationReasonTimeout   = "timeout"
	TruncationReasonCancelled = "cancelled"
	TruncationReasonMaxNodes  = "max_nodes"
)

// QueryResult represents the result of a path query.
type QueryResult struct {
	Entities         []*gtypes.EntityState `json:"entities"`
	Paths            []GraphPath           `json:"paths"`
	Count            int                   `json:"count"`
	Duration         time.Duration         `json:"duration"`
	Cached           bool                  `json:"cached"`
	CacheLayer       string                `json:"cache_layer,omitempty"`
	Error            error                 `json:"error,omitempty"`
	Truncated        bool                  `json:"truncated,omitempty"`
	TruncationReason string                `json:"truncation_reason,omitempty"`
	Scores           map[string]float64    `json:"scores,omitempty"`
}

// QMGraphSnapshot represents a snapshot of entities within bounds (internal query type).
type QMGraphSnapshot struct {
	Entities      []*gtypes.EntityState `json:"entities"`
	Relationships []QMRelationship      `json:"relationships"`
	Bounds        QueryBounds           `json:"bounds"`
	Timestamp     time.Time             `json:"timestamp"`
	Count         int                   `json:"count"`
	Truncated     bool                  `json:"truncated"`
}

// GraphPath represents a path through the entity graph.
type GraphPath struct {
	Entities []string    `json:"entities"`
	Edges    []GraphEdge `json:"edges"`
	Length   int         `json:"length"`
	Weight   float64     `json:"weight"`
}

// GraphEdge represents an edge in a graph path.
type GraphEdge struct {
	From       string                 `json:"from"`
	To         string                 `json:"to"`
	EdgeType   string                 `json:"edge_type"`
	Properties map[string]interface{} `json:"properties"`
	Weight     float64                `json:"weight"`
}

// QMLocalSearchResult represents the result of a local community search (internal query type).
type QMLocalSearchResult struct {
	Entities    []*gtypes.EntityState `json:"entities"`
	CommunityID string                `json:"community_id"`
	Count       int                   `json:"count"`
	Duration    time.Duration         `json:"duration"`
}

// QMGlobalSearchResult represents the result of a global cross-community search (internal query type).
type QMGlobalSearchResult struct {
	Entities           []*gtypes.EntityState `json:"entities"`
	CommunitySummaries []CommunitySummary    `json:"community_summaries"`
	Count              int                   `json:"count"`
	Duration           time.Duration         `json:"duration"`
	Answer             string                `json:"answer,omitempty"`
	AnswerModel        string                `json:"answer_model,omitempty"`
}

// QMSimilaritySearchResult represents the results of a similarity search query (internal query type).
type QMSimilaritySearchResult struct {
	Hits  []QMSimilarityHit `json:"hits"`
	Total int               `json:"total"`
}

// QMSimilarityHit represents a single similarity search result (internal query type).
type QMSimilarityHit struct {
	EntityID string  `json:"entity_id"`
	Score    float64 `json:"score"`
}
