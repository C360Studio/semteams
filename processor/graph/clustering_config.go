package graph

import (
	"github.com/c360/semstreams/processor/graph/inference"
)

// AnalysisConfig configures graph analysis features including community detection,
// structural indexing, and anomaly detection. Each feature can be enabled independently.
type AnalysisConfig struct {
	// HierarchyInference configures automatic edge creation based on 6-part EntityID structure.
	// When an entity is created, sibling edges are created to other entities with the same
	// 5-part type prefix (org.platform.domain.system.type).
	// This creates REAL edges at ingestion time, eliminating redundant work:
	// - LPA uses real edges directly (no virtual computation)
	// - Anomaly detector won't flag siblings as semantic gaps
	HierarchyInference *inference.HierarchyConfig `json:"hierarchy_inference,omitempty"`

	// CommunityDetection configures LPA-based community detection
	CommunityDetection *CommunityDetectionConfig `json:"community_detection,omitempty"`

	// StructuralIndex configures structural graph indices (k-core, pivot distance)
	// These indices enable query-time filtering and pruning for improved performance
	// Can be enabled independently of community detection
	StructuralIndex StructuralIndexConfig `json:"structural_index,omitempty"`

	// AnomalyDetection configures structural anomaly detection (Phase 3 inference)
	// Detects semantic-structural gaps, core isolation, and transitivity gaps
	AnomalyDetection *inference.Config `json:"anomaly_detection,omitempty"`

	// EntityIDEdges configures virtual edges based on 6-part EntityID hierarchy
	// Entities with the same TypePrefix (org.platform.domain.system.type) are siblings
	// This provides graph structure without ML/embeddings - works on any tier
	// Note: Can also be configured under community_detection for backwards compatibility
	// DEPRECATED: Use HierarchyInference for real edges instead of virtual edges
	EntityIDEdges *EntityIDEdgesConfig `json:"entityid_edges,omitempty"`
}

// CommunityDetectionConfig configures LPA-based community detection behavior
type CommunityDetectionConfig struct {
	// Enabled controls whether community detection is active
	Enabled bool `json:"enabled" schema:"type:bool,description:Enable community detection,default:false"`

	// Algorithm configures the detection algorithm
	Algorithm AlgorithmConfig `json:"algorithm,omitempty"`

	// Schedule configures detection timing
	Schedule ScheduleConfig `json:"schedule,omitempty"`

	// Enhancement configures LLM summarization
	Enhancement EnhancementConfig `json:"enhancement,omitempty"`

	// SemanticEdges configures virtual edges based on embedding similarity
	// This enables community detection even when explicit relationship triples don't exist
	SemanticEdges SemanticEdgesConfig `json:"semantic_edges,omitempty"`

	// EntityIDEdges configures virtual edges based on EntityID hierarchy
	// Entities with the same TypePrefix are treated as siblings for clustering
	// This is enabled by default since it has zero storage overhead
	EntityIDEdges *EntityIDEdgesConfig `json:"entityid_edges,omitempty"`

	// Inference configures relationship inference from community detection
	Inference InferenceConfig `json:"inference,omitempty"`
}

// StructuralIndexConfig configures structural graph indexing for query optimization
type StructuralIndexConfig struct {
	// Enabled controls whether structural indices are computed
	Enabled bool `json:"enabled" schema:"type:bool,description:Enable structural indexing,default:false"`

	// KCore configures k-core decomposition filtering
	KCore KCoreConfig `json:"kcore,omitempty"`

	// Pivot configures pivot-based distance indexing
	Pivot PivotConfig `json:"pivot,omitempty"`

	// ComputeInterval is how often to recompute structural indices
	// Default: 1h (indices change slowly as graph structure evolves)
	ComputeInterval string `json:"compute_interval" schema:"type:string,description:Interval between index recomputation,default:1h"`

	// ComputeOnStartup triggers index computation when processor starts
	// Default: true
	ComputeOnStartup bool `json:"compute_on_startup" schema:"type:bool,description:Compute indices on startup,default:true"`
}

// KCoreConfig configures k-core decomposition for query filtering
type KCoreConfig struct {
	// Enabled activates k-core filtering in semantic search
	// When true, entities with low core numbers (peripheral nodes) can be filtered out
	Enabled bool `json:"enabled" schema:"type:bool,description:Enable k-core filtering,default:false"`

	// MinCoreFilter is the minimum core number for entities in search results
	// Entities with core < MinCoreFilter are excluded from semantic search results
	// Set to 0 to disable filtering (include all entities)
	// Default: 0 (no filtering)
	MinCoreFilter int `json:"min_core_filter" schema:"type:int,description:Minimum core number for search results,default:0"`
}

// PivotConfig configures pivot-based distance indexing for path queries
type PivotConfig struct {
	// Enabled activates pivot-based distance pruning in path traversal
	// When true, unreachable candidates are pruned early using distance bounds
	Enabled bool `json:"enabled" schema:"type:bool,description:Enable pivot distance pruning,default:false"`

	// PivotCount is the number of pivot nodes to select for distance computation
	// More pivots = tighter bounds but higher computation cost
	// Default: 16
	PivotCount int `json:"pivot_count" schema:"type:int,description:Number of pivot nodes,default:16"`

	// MaxHopDistance is the maximum hop distance for path queries
	// Candidates beyond this distance are pruned
	// Default: 10
	MaxHopDistance int `json:"max_hop_distance" schema:"type:int,description:Maximum hop distance for pruning,default:10"`
}
