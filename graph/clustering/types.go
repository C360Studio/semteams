package clustering

import (
	"context"

	gtypes "github.com/c360studio/semstreams/graph"
)

// Community represents a detected community/cluster in the graph
type Community struct {
	// ID is the unique identifier for this community
	ID string `json:"id"`

	// Level indicates the hierarchy level (0=bottom, 1=mid, 2=top)
	Level int `json:"level"`

	// Members contains the entity IDs belonging to this community
	Members []string `json:"members"`

	// ParentID references the parent community at the next level up (nil for top level)
	ParentID *string `json:"parent_id,omitempty"`

	// StatisticalSummary is the fast statistical baseline summary (always present)
	// Generated using TF-IDF keyword extraction and template-based summarization
	StatisticalSummary string `json:"statistical_summary,omitempty"`

	// LLMSummary is the enhanced LLM-generated summary (populated asynchronously)
	// Empty until LLM enhancement completes successfully
	LLMSummary string `json:"llm_summary,omitempty"`

	// Keywords are extracted key terms representing this community's themes
	// e.g., ["autonomous", "navigation", "sensor-fusion"]
	Keywords []string `json:"keywords,omitempty"`

	// RepEntities contains IDs of representative entities within this community
	// These entities best exemplify the community's characteristics
	RepEntities []string `json:"rep_entities,omitempty"`

	// SummaryStatus tracks the summarization state
	// Values: "statistical" (initial), "llm-enhanced" (enhanced), "llm-failed" (enhancement failed)
	SummaryStatus string `json:"summary_status,omitempty"`

	// Metadata stores additional community properties
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// CommunityDetector performs community detection on a graph
type CommunityDetector interface {
	// DetectCommunities runs community detection on the entire graph
	// Returns communities organized by hierarchical level
	DetectCommunities(ctx context.Context) (map[int][]*Community, error)

	// UpdateCommunities incrementally updates communities based on recent graph changes
	// entityIDs are entities that have been added/modified since last detection
	UpdateCommunities(ctx context.Context, entityIDs []string) error

	// GetCommunity retrieves a specific community by ID
	GetCommunity(ctx context.Context, id string) (*Community, error)

	// GetEntityCommunity returns the community containing the given entity
	// level specifies which hierarchical level to query (0=bottom, 1=mid, 2=top)
	GetEntityCommunity(ctx context.Context, entityID string, level int) (*Community, error)

	// GetCommunitiesByLevel returns all communities at a specific hierarchical level
	GetCommunitiesByLevel(ctx context.Context, level int) ([]*Community, error)

	// InferRelationshipsFromCommunities generates inferred triples from community co-membership.
	// For each community with >= minCommunitySize members, creates bidirectional
	// "inferred.clustered_with" triples between members.
	InferRelationshipsFromCommunities(ctx context.Context, level int, config InferenceConfig) ([]InferredTriple, error)
}

// Provider is an alias to the shared interface in graph package.
// Abstracts the graph data source for community detection.
type Provider = gtypes.Provider

// CommunityStorage abstracts persistence layer for communities
type CommunityStorage interface {
	// SaveCommunity persists a community
	SaveCommunity(ctx context.Context, community *Community) error

	// GetCommunity retrieves a community by ID
	GetCommunity(ctx context.Context, id string) (*Community, error)

	// GetCommunitiesByLevel retrieves all communities at a level
	GetCommunitiesByLevel(ctx context.Context, level int) ([]*Community, error)

	// GetEntityCommunity retrieves the community for an entity at a level
	GetEntityCommunity(ctx context.Context, entityID string, level int) (*Community, error)

	// DeleteCommunity removes a community
	DeleteCommunity(ctx context.Context, id string) error

	// Clear removes all communities (useful for full recomputation)
	Clear(ctx context.Context) error

	// GetAllCommunities returns all communities across all levels
	// Used for archiving enhanced communities before Clear()
	GetAllCommunities(ctx context.Context) ([]*Community, error)
}

// Note: Getter methods removed per ADR-PACKAGE-RESPONSIBILITIES-CONSOLIDATION.
// Use direct field access instead (e.g., community.ID instead of community.GetID())
