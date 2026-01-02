// Package indexmanager provides the Indexer interface and IndexManager implementation.
// This service is responsible for maintaining secondary indexes by watching ENTITY_STATES KV
// for changes and updating indexes asynchronously with event buffering and deduplication.
//
// The IndexManager watches KV changes instead of consuming events directly,
// enabling eventual consistency and decoupled processing.
package indexmanager

import (
	"context"
	"time"

	gtypes "github.com/c360/semstreams/graph"
)

// Indexer watches KV for entity changes and maintains indexes.
// It provides both update and query operations for all secondary indexes.
type Indexer interface {
	// Run starts the IndexManager and blocks until error or context done.
	// If onReady is provided, it is called once initialization completes successfully.
	Run(ctx context.Context, onReady func()) error

	// Index update operations (write operations - kept for special operations like bulk imports)
	UpdatePredicateIndex(ctx context.Context, entityID string, entityState interface{}) error
	UpdateSpatialIndex(ctx context.Context, entityID string, position interface{}) error
	UpdateTemporalIndex(ctx context.Context, entityID string, entityState *gtypes.EntityState) error
	UpdateIncomingIndex(ctx context.Context, targetEntityID, sourceEntityID, predicate string) error
	RemoveFromIncomingIndex(ctx context.Context, targetEntityID, sourceEntityID string) error
	UpdateAliasIndex(ctx context.Context, alias, entityID string) error

	// Index deletion operations
	DeleteFromIndexes(ctx context.Context, entityID string) error
	DeleteFromPredicateIndex(ctx context.Context, entityID string) error
	DeleteFromSpatialIndex(ctx context.Context, entityID string) error
	DeleteFromTemporalIndex(ctx context.Context, entityID string) error
	DeleteFromIncomingIndex(ctx context.Context, entityID string) error
	DeleteFromAliasIndex(ctx context.Context, alias string) error

	// Simple index Gets (single KV lookup operations)
	GetPredicateIndex(ctx context.Context, predicate string) ([]string, error)
	GetIncomingRelationships(ctx context.Context, targetEntityID string) ([]Relationship, error)
	// GetOutgoingRelationships returns outgoing relationships from an entity using OUTGOING_INDEX.
	// This provides O(1) lookup for forward edge traversal without loading full entity state.
	// Used by PathRAG traversal, relationship listing APIs, and dependency analysis.
	// Returns empty slice if entity has no outgoing relationships or is not in the index.
	// Returns error only on actual failures (not-found is not an error).
	GetOutgoingRelationships(ctx context.Context, entityID string) ([]OutgoingRelationship, error)
	ResolveAlias(ctx context.Context, alias string) (string, error)

	// Complex queries (multiple KV lookups)
	QuerySpatial(ctx context.Context, bounds Bounds) ([]string, error)
	QueryTemporal(ctx context.Context, start, end time.Time) ([]string, error)

	// Batch operations for efficiency
	GetPredicateIndexes(ctx context.Context, predicates []string) (map[string][]string, error)
	ResolveAliases(ctx context.Context, aliases []string) (map[string]string, error)

	// Semantic search operations (requires embedding configuration)
	SearchSemantic(ctx context.Context, query string, opts *SemanticSearchOptions) (*SearchResults, error)
	SearchHybrid(ctx context.Context, query *HybridQuery) (*SearchResults, error)

	// Similarity operations for inference (requires embedding configuration)
	// FindSimilarEntities returns entities similar to the given entity based on embedding similarity.
	// Used by SemanticGraphProvider to create virtual edges for clustering.
	FindSimilarEntities(ctx context.Context, entityID string, threshold float64, limit int) ([]SimilarityHit, error)

	// Metrics
	GetBacklog() int
	GetDeduplicationStats() DeduplicationStats

	// Embedding metrics (for clustering coordination)
	// GetEmbeddingCount returns the number of entities with embeddings in the vector cache.
	// Used by the clustering system to check embedding coverage before running LPA with semantic edges.
	GetEmbeddingCount() int

	// Structural index operations (requires structural index configuration)
	// GetKCoreIndex returns the current k-core index for filtering queries.
	// Returns nil if structural indexing is disabled or not yet computed.
	GetKCoreIndex() KCoreIndex

	// GetPivotIndex returns the current pivot index for distance estimation.
	// Returns nil if structural indexing is disabled or not yet computed.
	GetPivotIndex() PivotIndex

	// FilterByKCore filters entity IDs to only include those with core number >= minCore.
	// Returns the input slice unchanged if k-core index is not available.
	FilterByKCore(entityIDs []string, minCore int) []string

	// PruneByPivotDistance filters candidates to those potentially reachable from source within maxHops.
	// Uses triangle inequality bounds - may include some unreachable entities (false positives ok).
	// Returns the input slice unchanged if pivot index is not available.
	PruneByPivotDistance(source string, candidates []string, maxHops int) []string

	// Note: SetOnEntityCreatedCallback removed - HierarchyInference now uses its own
	// KV watcher on ENTITY_STATES for better decoupling and async processing.

	// GetAllEntityIDs returns all entity IDs from the ENTITY_STATES bucket.
	// Used by HierarchyInference to discover siblings for new entities.
	GetAllEntityIDs(ctx context.Context) ([]string, error)
}

// KCoreIndex provides k-core decomposition data for query filtering.
// This interface abstracts the structural index to avoid import cycles.
type KCoreIndex interface {
	// GetCore returns the core number for an entity (0 if not found).
	GetCore(entityID string) int

	// FilterByMinCore returns only entities with core >= minCore.
	FilterByMinCore(entityIDs []string, minCore int) []string

	// GetEntitiesInCore returns all entities with exactly the given core number.
	GetEntitiesInCore(core int) []string

	// GetEntitiesAboveCore returns all entities with core >= minCore.
	GetEntitiesAboveCore(minCore int) []string
}

// PivotIndex provides pivot-based distance estimation for path pruning.
// This interface abstracts the structural index to avoid import cycles.
type PivotIndex interface {
	// EstimateDistance returns lower and upper bounds for shortest path distance.
	// Returns (MaxHopDistance, MaxHopDistance) if entities are disconnected or unknown.
	EstimateDistance(entityA, entityB string) (lower, upper int)

	// IsWithinHops returns true if entities might be within maxHops of each other.
	// Uses lower bound - if lower > maxHops, definitely not within range.
	IsWithinHops(entityA, entityB string, maxHops int) bool

	// GetReachableCandidates returns entities potentially within maxHops of source.
	// May include false positives but no false negatives.
	GetReachableCandidates(source string, maxHops int) []string
}

// Bounds represents spatial query bounds
type Bounds struct {
	North float64 `json:"north"`
	South float64 `json:"south"`
	East  float64 `json:"east"`
	West  float64 `json:"west"`
}

// Relationship represents an incoming relationship
type Relationship struct {
	FromEntityID string                 `json:"from_entity_id"`
	EdgeType     string                 `json:"edge_type"`
	Weight       float64                `json:"weight"`
	Properties   map[string]interface{} `json:"properties"`
	CreatedAt    time.Time              `json:"created_at"`
}

// OutgoingRelationship represents an outgoing relationship from an entity.
// Used by GetOutgoingRelationships to return forward edges in the graph.
type OutgoingRelationship struct {
	ToEntityID string `json:"to_entity_id"`
	EdgeType   string `json:"edge_type"` // Predicate/relationship type
}

// DeduplicationStats provides metrics on event deduplication
type DeduplicationStats struct {
	TotalEvents       int64   `json:"total_events"`
	DuplicateEvents   int64   `json:"duplicate_events"`
	ProcessedEvents   int64   `json:"processed_events"`
	DeduplicationRate float64 `json:"deduplication_rate"`
	CacheSize         int     `json:"cache_size"`
	CacheHitRate      float64 `json:"cache_hit_rate"`
}

// SimilarityHit represents an entity similarity match for inference operations.
// Used by FindSimilarEntities to return entities similar to a query entity.
type SimilarityHit struct {
	EntityID   string  `json:"entity_id"`   // ID of the similar entity
	Similarity float64 `json:"similarity"`  // Cosine similarity score (0.0-1.0)
	EntityType string  `json:"entity_type"` // Type of the entity (for type-batched filtering)
}

// EntityChange represents a KV change event to be processed
type EntityChange struct {
	Key       string    `json:"key"`       // Entity ID (KV key)
	Operation Operation `json:"operation"` // create|update|delete
	Value     []byte    `json:"value"`     // Entity state JSON (nil for delete)
	Revision  uint64    `json:"revision"`  // KV revision
	Timestamp time.Time `json:"timestamp"` // When change was detected
}

// Operation represents the type of KV operation detected
type Operation string

const (
	// OperationCreate represents a KV Put with revision=1.
	OperationCreate Operation = "create"
	// OperationUpdate represents a KV Put with revision>1.
	OperationUpdate Operation = "update"
	// OperationDelete represents a KV Delete operation.
	OperationDelete Operation = "delete"
)

// String returns the string representation of the operation
func (o Operation) String() string {
	return string(o)
}

// IsValid checks if the operation is valid
func (o Operation) IsValid() bool {
	switch o {
	case OperationCreate, OperationUpdate, OperationDelete:
		return true
	default:
		return false
	}
}
