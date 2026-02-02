// Package datamanager provides unified entity and triple data management for the knowledge graph.
//
// # Overview
//
// DataManager is the single writer to the ENTITY_STATES KV bucket, providing atomic
// entity persistence, triple management, and L1/L2 cache hierarchies. It consolidates
// what were previously separate EntityStore and EdgeManager components into a unified
// service following the Interface Segregation Principle.
//
// The package provides 6 focused interfaces for different use cases:
//   - [EntityReader]: Read-only entity access with caching
//   - [EntityWriter]: Basic entity mutation operations
//   - [EntityManager]: Complete entity lifecycle management
//   - [TripleManager]: Semantic triple operations for relationships
//   - [DataConsistency]: Graph integrity checking and maintenance
//   - [DataLifecycle]: Component lifecycle and observability
//
// # Architecture
//
// DataManager uses a write-behind buffer pattern for high throughput:
//
//	Incoming Writes → Buffer (10K capacity) → Batch Workers (5) → NATS KV
//	                     ↓
//	               L1 Cache (LRU, 1K)
//	                     ↓
//	               L2 Cache (TTL, 10K)
//	                     ↓
//	                 NATS KV
//
// # Usage
//
// Basic entity operations:
//
//	// Create manager with NATS client
//	cfg := datamanager.DefaultConfig()
//	mgr, err := datamanager.New(natsClient, cfg, registry, logger)
//
//	// Start the manager (blocks until context cancelled)
//	go mgr.Run(ctx, func() {
//	    log.Info("DataManager ready")
//	})
//
//	// Create entity with triples atomically
//	entity := &graph.EntityState{ID: "org.platform.domain.system.type.instance"}
//	triples := []message.Triple{
//	    {Subject: entity.ID, Predicate: "type", Object: "Drone"},
//	}
//	created, err := mgr.CreateEntityWithTriples(ctx, entity, triples)
//
//	// Read entity (uses L1/L2 cache)
//	entity, err := mgr.GetEntity(ctx, "org.platform.domain.system.type.instance")
//
// # Configuration
//
// Key configuration options:
//
//	Buffer:
//	  Capacity:        10000      # Write buffer size
//	  BatchingEnabled: true       # Enable batch writes
//	  FlushInterval:   50ms       # Batch flush frequency
//	  MaxBatchSize:    100        # Max writes per batch
//	  OverflowPolicy:  drop_oldest # Buffer overflow handling
//
//	Cache:
//	  L1Hot:
//	    Type: lru                 # LRU eviction for hot data
//	    Size: 1000                # Hot cache capacity
//	  L2Warm:
//	    Type: ttl                 # TTL eviction for warm data
//	    Size: 10000               # Warm cache capacity
//	    TTL:  5m                  # Time-to-live
//
//	Workers:      5               # Concurrent write workers
//	WriteTimeout: 5s              # KV write timeout
//	ReadTimeout:  2s              # KV read timeout
//	MaxRetries:   10              # CAS retry attempts
//
// # Entity ID Format
//
// Entity IDs must follow the 6-part hierarchical format:
//
//	org.platform.domain.system.type.instance
//
// Example: c360.platform1.robotics.gcs1.drone.1
//
// # Thread Safety
//
// All Manager methods are safe for concurrent use. The write buffer uses
// non-blocking submission with configurable overflow policies. Cache access
// is protected by the underlying cache implementations.
//
// # Metrics
//
// DataManager exports Prometheus metrics under the semstreams_datamanager namespace:
//   - writes_total: Total KV write operations by status and operation
//   - write_latency_seconds: KV write latency distribution
//   - queue_depth: Current write queue depth
//   - batch_size: Size of write batches
//   - dropped_writes: Writes dropped due to queue overflow
//   - cache_hits: Cache hits by level (l1, l2)
//   - cache_misses: Cache misses requiring KV fetch
//
// # See Also
//
// Related packages:
//   - [github.com/c360studio/semstreams/graph]: Core graph types and EntityState
//   - [github.com/c360studio/semstreams/message]: Triple and message types
//   - [github.com/c360studio/semstreams/graph/query]: Query execution using EntityReader
package datamanager
