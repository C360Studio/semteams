// Package messagemanager provides message processing and entity extraction for the
// knowledge graph.
//
// # Overview
//
// The messagemanager package converts incoming messages into EntityState objects
// that can be stored in the knowledge graph. It handles multiple message types:
// Graphable interfaces, Storable interfaces with ObjectStore references, and
// legacy map-based messages.
//
// Messages are processed through a unified pipeline that extracts entity IDs,
// triples (the single source of truth for properties and relationships), and
// optional storage references. Entity merging is performed atomically using
// upsert semantics to avoid race conditions.
//
// # Architecture
//
//	              Incoming Message ([]byte)
//	                       ↓
//	┌──────────────────────────────────────────────────────────────┐
//	│                   Manager.ProcessWork                        │
//	│  1. Parse BaseMessage envelope                               │
//	│  2. Extract payload type                                     │
//	│  3. Route to appropriate handler                             │
//	└──────────────────────────────────────────────────────────────┘
//	                       ↓
//	┌────────────┬─────────────────┬───────────────────────────────┐
//	│ Storable   │ Graphable       │ Map/Legacy                    │
//	│ (ObjectStore│ (EntityID +    │ (JSON map to                  │
//	│  reference) │  Triples)      │  entity state)                │
//	└────────────┴─────────────────┴───────────────────────────────┘
//	                       ↓
//	┌──────────────────────────────────────────────────────────────┐
//	│              EntityState (Triples = source of truth)         │
//	│                          ↓                                   │
//	│              EntityManager.UpsertEntity                      │
//	└──────────────────────────────────────────────────────────────┘
//
// # Usage
//
// Create and configure the message manager:
//
//	config := messagemanager.DefaultConfig()
//	deps := messagemanager.Dependencies{
//	    EntityManager:   entityManager,
//	    IndexManager:    indexManager,
//	    Logger:          logger,
//	    MetricsRegistry: registry,
//	}
//
//	manager := messagemanager.NewManager(config, deps, func(err string) {
//	    log.Error("Message processing error", "error", err)
//	})
//
// Process messages from a worker pool:
//
//	// ProcessWork handles raw message bytes from worker pool
//	err := manager.ProcessWork(ctx, messageData)
//
// Process messages directly:
//
//	// ProcessMessage handles typed messages
//	entityStates, err := manager.ProcessMessage(ctx, graphableMsg)
//
// # Message Types
//
// Storable messages:
//
// Messages implementing the Storable interface have content stored in ObjectStore.
// The manager extracts the StorageReference and passes it to the EntityState:
//
//	type Storable interface {
//	    Graphable
//	    StorageRef() *message.StorageReference
//	}
//
// Graphable messages:
//
// Messages implementing Graphable provide entity ID and triples directly:
//
//	type Graphable interface {
//	    EntityID() string
//	    Triples() []message.Triple
//	}
//
// Map messages:
//
// JSON objects (map[string]any) are converted to entities with auto-generated IDs
// and map keys becoming triple predicates.
//
// # Entity Merging
//
// When processing messages for existing entities, the manager:
//  1. Fetches existing entity state (if any)
//  2. Merges triples using gtypes.MergeTriples
//  3. Increments entity version
//  4. Uses UpsertEntity for atomic persistence
//
// This avoids TOCTOU race conditions in concurrent message processing.
//
// # Alias Resolution
//
// Entity IDs can be aliases resolved via IndexManager.ResolveAlias.
// If resolution fails, the original ID is used as-is.
//
// # Configuration
//
// Configuration options:
//
//	DefaultNamespace: "default"      # Namespace for auto-generated entity IDs
//	DefaultPlatform:  "semstreams"   # Platform for auto-generated entity IDs
//
// # Thread Safety
//
// The Manager is safe for concurrent use. Message processing uses atomic
// operations for statistics and upsert semantics for entity persistence.
//
// # Metrics
//
// The package exports Prometheus metrics:
//   - messages_processed_total: Total messages processed
//   - messages_failed_total: Messages that failed processing
//   - entities_extracted_total: Entities extracted from messages
//   - entities_update_attempts_total: Entity upsert attempts
//   - entities_update_success_total: Successful entity upserts
//   - entities_update_failed_total: Failed entity upserts
//
// # See Also
//
// Related packages:
//   - [github.com/c360/semstreams/graph]: EntityState and Graphable interface
//   - [github.com/c360/semstreams/graph/datamanager]: Entity persistence
//   - [github.com/c360/semstreams/message]: Message types and Triple
//   - [github.com/c360/semstreams/storage/objectstore]: Large content storage
package messagemanager
