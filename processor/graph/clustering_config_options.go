package graph

import "github.com/c360/semstreams/processor/graph/llm"

// SemanticEdgesConfig configures virtual edge creation from embedding similarity
type SemanticEdgesConfig struct {
	// Enabled activates semantic-based virtual edges for clustering
	// When true, entities with similar embeddings are treated as neighbors
	Enabled bool `json:"enabled" schema:"type:bool,description:Enable semantic virtual edges,default:false"`

	// SimilarityThreshold is the minimum cosine similarity for virtual edges (0.0-1.0)
	// Higher values = fewer but stronger virtual connections
	// Default: 0.6 (stricter than search threshold of 0.3)
	SimilarityThreshold float64 `json:"similarity_threshold" schema:"type:float,description:Min similarity for virtual edge,default:0.6"`

	// MaxVirtualNeighbors limits virtual neighbors per entity
	// Controls computation cost during LPA iterations
	// Default: 5
	MaxVirtualNeighbors int `json:"max_virtual_neighbors" schema:"type:int,description:Max virtual neighbors per entity,default:5"`
}

// EntityIDEdgesConfig configures EntityID-based virtual edges for sibling detection.
// Entities with the same TypePrefix (org.platform.domain.system.type) are treated as siblings.
// This enables community detection without explicit relationship triples.
type EntityIDEdgesConfig struct {
	// Enabled controls whether EntityID edges are generated
	// Default: true (zero storage overhead, works with any data)
	Enabled bool `json:"enabled" schema:"type:bool,description:Enable EntityID virtual edges,default:true"`

	// SiblingWeight is the edge weight for sibling relationships (0.0-1.0)
	// Lower than explicit relationships (1.0) to ensure explicit edges take precedence
	// Default: 0.7
	SiblingWeight float64 `json:"sibling_weight" schema:"type:float,description:Weight for sibling edges,default:0.7"`

	// MaxSiblings limits siblings per entity to prevent large clusters
	// Default: 10
	MaxSiblings int `json:"max_siblings" schema:"type:int,description:Max siblings per entity,default:10"`

	// IncludeSiblings enables sibling detection via shared TypePrefix
	// Default: true
	IncludeSiblings bool `json:"include_siblings" schema:"type:bool,description:Include sibling relationships,default:true"`
}

// InferenceConfig configures relationship inference from clustering results
type InferenceConfig struct {
	// Enabled activates triple inference from community detection
	// When true, co-membership in communities creates inferred.clustered_with triples
	Enabled bool `json:"enabled" schema:"type:bool,description:Enable relationship inference,default:false"`

	// MinCommunitySize is the minimum community size for inference
	// Singleton communities (size=1) never produce inferences
	// Default: 2
	MinCommunitySize int `json:"min_community_size" schema:"type:int,description:Min community size for inference,default:2"`

	// MaxInferredPerCommunity limits inferred relationships per community
	// Prevents O(n²) explosion in large communities
	// Default: 50
	MaxInferredPerCommunity int `json:"max_inferred_per_community" schema:"type:int,description:Max inferred relationships per community,default:50"`
}

// AlgorithmConfig configures the LPA detector
type AlgorithmConfig struct {
	// MaxIterations limits LPA iterations
	MaxIterations int `json:"max_iterations" schema:"type:int,description:Max LPA iterations,default:100"`

	// Levels is hierarchical community levels
	Levels int `json:"levels" schema:"type:int,description:Hierarchical levels,default:3"`
}

// EnhancementWindowMode controls detection behavior during LLM enhancement
type EnhancementWindowMode string

const (
	// WindowModeBlocking pauses detection until window expires or all communities reach terminal status
	WindowModeBlocking EnhancementWindowMode = "blocking"
	// WindowModeSoft allows detection if significant entity changes occur during window
	WindowModeSoft EnhancementWindowMode = "soft"
	// WindowModeNone disables enhancement window (original behavior)
	WindowModeNone EnhancementWindowMode = "none"
)

// ScheduleConfig configures detection timing
type ScheduleConfig struct {
	// InitialDelay before first detection
	InitialDelay string `json:"initial_delay" schema:"type:string,description:Delay before first detection,default:10s"`

	// DetectionInterval is the maximum time between detection runs (triggers even if no new entities)
	DetectionInterval string `json:"detection_interval" schema:"type:string,description:Max interval between detection runs,default:30s"`

	// MinDetectionInterval is the minimum time between detection runs (burst protection)
	MinDetectionInterval string `json:"min_detection_interval" schema:"type:string,description:Min interval between detection runs,default:5s"`

	// EntityChangeThreshold triggers detection after N new entities arrive (0 disables)
	EntityChangeThreshold int `json:"entity_change_threshold" schema:"type:int,description:Trigger detection after N new entities,default:100"`

	// MinEntities threshold for triggering detection
	MinEntities int `json:"min_entities" schema:"type:int,description:Min entities for detection,default:10"`

	// MinEmbeddingCoverage is the minimum ratio of embeddings to entities required for semantic clustering.
	// When semantic_edges is enabled, clustering will be skipped until this coverage threshold is met.
	// This prevents clustering from running before embeddings are generated.
	// Range: 0.0-1.0, Default: 0.5 (50%)
	MinEmbeddingCoverage float64 `json:"min_embedding_coverage" schema:"type:float,description:Min embedding coverage for semantic clustering (0.0-1.0),default:0.5"`

	// EnhancementWindow is the duration to pause detection after clustering to allow LLM enhancement.
	// During this window, re-detection is paused to prevent overwriting LLM-enhanced communities.
	// Set to "0" or empty to disable (original behavior).
	// Default: 0 (disabled)
	EnhancementWindow string `json:"enhancement_window" schema:"type:string,description:Pause detection duration for LLM enhancement,default:0"`

	// EnhancementWindowMode controls how the enhancement window behaves:
	// - "blocking": Hard pause until window expires or all communities reach terminal status (llm-enhanced/llm-failed)
	// - "soft": Allow detection if entity changes exceed threshold during window
	// - "none": Disable enhancement window (original behavior)
	// Default: "blocking"
	EnhancementWindowMode EnhancementWindowMode `json:"enhancement_window_mode" schema:"type:string,description:Enhancement window mode (blocking|soft|none),default:blocking"`
}

// EnhancementConfig configures LLM summary enhancement
type EnhancementConfig struct {
	// Enabled activates the enhancement worker
	Enabled bool `json:"enabled" schema:"type:bool,description:Enable LLM enhancement,default:false"`

	// LLM configures the LLM client for summarization
	LLM llm.Config `json:"llm,omitempty" schema:"type:object,description:LLM configuration"`

	// Workers is concurrent enhancement workers
	Workers int `json:"workers" schema:"type:int,description:Concurrent workers,default:3"`

	// Domain for prompt selection (e.g., "iot", "default")
	Domain string `json:"domain,omitempty" schema:"type:string,description:Prompt domain,default:default"`
}
