// Package datamanager consolidates entity and triple operations into a unified data management service.
// This package is the result of Phase 1 consolidation, providing atomic entity+triple operations
// and simplified transaction management.
//
// The DataManager is the single writer to ENTITY_STATES KV bucket and handles all
// entity persistence, triple management, and maintains L1/L2 cache hierarchies.
//
// The package follows Interface Segregation Principle with 5 focused interfaces:
// - EntityReader: Read-only entity access with caching
// - EntityWriter: Basic entity mutation operations
// - EntityManager: Complete entity lifecycle management
// - TripleManager: Semantic triple operations (replaces EdgeManager)
// - DataConsistency: Graph integrity checking and maintenance
// - DataLifecycle: Component lifecycle and observability
package datamanager

import (
	"context"

	gtypes "github.com/c360/semstreams/graph"
	"github.com/c360/semstreams/message"
)

// EntityReader provides read-only entity access with caching.
// Used by components that only need to query entities (e.g., QueryManager).
type EntityReader interface {
	GetEntity(ctx context.Context, id string) (*gtypes.EntityState, error)
	ExistsEntity(ctx context.Context, id string) (bool, error)
	BatchGet(ctx context.Context, ids []string) ([]*gtypes.EntityState, error)
	// ListWithPrefix returns entity IDs that have the given prefix.
	// Used for hierarchical entity queries like finding siblings in PathRAG.
	ListWithPrefix(ctx context.Context, prefix string) ([]string, error)
}

// EntityWriter provides basic entity mutation operations.
// Used by components that need simple CRUD operations without edge management.
type EntityWriter interface {
	CreateEntity(ctx context.Context, entity *gtypes.EntityState) (*gtypes.EntityState, error)
	UpdateEntity(ctx context.Context, entity *gtypes.EntityState) (*gtypes.EntityState, error)
	// UpsertEntity atomically creates or updates an entity using Put semantics.
	// This is the preferred method for streaming data where idempotency is required.
	// It avoids TOCTOU races that occur with separate GetEntity → Create/Update patterns.
	UpsertEntity(ctx context.Context, entity *gtypes.EntityState) (*gtypes.EntityState, error)
	DeleteEntity(ctx context.Context, id string) error
}

// EntityManager provides complete entity lifecycle management.
// Combines reader, writer, and advanced operations like atomic entity+triple writes.
type EntityManager interface {
	EntityReader
	EntityWriter

	CreateEntityWithTriples(ctx context.Context, entity *gtypes.EntityState, triples []message.Triple) (*gtypes.EntityState, error)
	UpdateEntityWithTriples(ctx context.Context, entity *gtypes.EntityState, addTriples []message.Triple, removePredicates []string) (*gtypes.EntityState, error)
	BatchWrite(ctx context.Context, writes []EntityWrite) error
	List(ctx context.Context, pattern string) ([]string, error)
	// ListWithPrefix returns entity IDs that have the given prefix.
	// Used for hierarchical entity queries like finding siblings in PathRAG.
	ListWithPrefix(ctx context.Context, prefix string) ([]string, error)
}

// TripleManager provides semantic triple operations.
// Used by components that manage entity relationships and properties as triples.
type TripleManager interface {
	AddTriple(ctx context.Context, triple message.Triple) error
	RemoveTriple(ctx context.Context, subject, predicate string) error
	CreateRelationship(ctx context.Context, fromEntityID, toEntityID string, predicate string, metadata map[string]any) error
	DeleteRelationship(ctx context.Context, fromEntityID, toEntityID string, predicate string) error
}

// DataConsistency provides graph integrity checking and maintenance.
// Used by components that need to verify or repair graph consistency.
type DataConsistency interface {
	CleanupIncomingReferences(ctx context.Context, deletedEntityID string, outgoingTriples []message.Triple) error
	CheckOutgoingTriplesConsistency(ctx context.Context, entityID string, entity *gtypes.EntityState, status *EntityIndexStatus)
	HasRelationshipToEntity(entity *gtypes.EntityState, targetEntityID string, predicate string) bool
}

// DataLifecycle manages component lifecycle and observability.
// Used by the main processor to start/stop the manager and monitor health.
type DataLifecycle interface {
	// Run starts the DataManager and blocks until context is cancelled.
	// If onReady is provided, it is called once initialization completes successfully.
	Run(ctx context.Context, onReady func()) error
	FlushPendingWrites(ctx context.Context) error
	GetPendingWriteCount() int
	GetCacheStats() CacheStats
}

// Compile-time verification that Manager implements all interfaces
var (
	_ EntityReader    = (*Manager)(nil)
	_ EntityWriter    = (*Manager)(nil)
	_ EntityManager   = (*Manager)(nil)
	_ TripleManager   = (*Manager)(nil)
	_ DataConsistency = (*Manager)(nil)
	_ DataLifecycle   = (*Manager)(nil)
)
