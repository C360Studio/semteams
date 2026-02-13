// Package trustgraph provides a TrustGraph output component that watches SemStreams
// entity state changes and exports them as RDF triples to TrustGraph knowledge cores.
//
// This enables operational data (sensor readings, telemetry, events) to be available
// in TrustGraph's GraphRAG queries, combining document intelligence with real-time
// operational context.
//
// # Architecture
//
//	┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
//	│   entity.>      │────▶│  trustgraph      │────▶│  TrustGraph     │
//	│   (NATS)        │     │  -output         │     │  REST API       │
//	└─────────────────┘     └──────────────────┘     └─────────────────┘
//	                               │
//	                               ▼
//	                        ┌──────────────────┐
//	                        │  Batch Buffer    │
//	                        │  (in-memory)     │
//	                        └──────────────────┘
//
// # Features
//
//   - Subscribes to NATS entity subjects
//   - Filters by entity prefix and source (loop prevention)
//   - Translates SemStreams entities to RDF triples
//   - Batches triples for efficient API calls
//   - Flushes on batch size threshold or interval
//   - Retry with backoff on transient errors
//   - Full Prometheus metrics for observability
//
// # Configuration
//
//	{
//	    "endpoint": "http://trustgraph:8088",
//	    "kg_core_id": "semstreams-ops",
//	    "user": "semstreams",
//	    "collection": "operational",
//	    "batch_size": 100,
//	    "flush_interval": "5s",
//	    "entity_prefixes": ["acme.ops."],
//	    "exclude_sources": ["trustgraph"],
//	    "vocab": {
//	        "org_mappings": {
//	            "acme": "https://data.acme-corp.com/"
//	        },
//	        "predicate_mappings": {
//	            "sensor.measurement.celsius": "http://www.w3.org/ns/sosa/hasSimpleResult",
//	            "geo.location.zone": "http://www.w3.org/ns/sosa/isHostedBy"
//	        },
//	        "default_uri_base": "http://semstreams.local/e/"
//	    },
//	    "ports": {
//	        "inputs": [
//	            {"name": "entity", "type": "nats", "subject": "entity.>"}
//	        ]
//	    }
//	}
//
// # Loop Prevention
//
// When both trustgraph-input and trustgraph-output are deployed, loop prevention
// is critical. The output component filters entities based on the Source field:
//
//   - trustgraph-input stamps imported triples with Source: "trustgraph"
//   - trustgraph-output defaults exclude_sources to ["trustgraph"]
//   - Entities where ALL triples have an excluded source are filtered out
//   - Mixed-source entities (some local, some imported) are still exported
//
// This prevents circular data flow while allowing enrichment of imported entities.
//
// # Vocabulary Translation
//
// The component uses the vocabulary/trustgraph package for translation from
// SemStreams entity IDs to RDF URIs.
//
// Entity ID to URI example:
//
//	acme.ops.environmental.sensor.temperature.zone-7
//	→ https://data.acme-corp.com/ops/environmental/sensor/temperature/zone-7
//
// Predicate translation uses configured mappings or structural fallback:
//
//	sensor.measurement.celsius
//	→ http://www.w3.org/ns/sosa/hasSimpleResult (if mapped)
//	→ https://data.acme-corp.com/predicate/sensor/measurement/celsius (fallback)
//
// # Batching
//
// Triples are batched for efficient API calls to TrustGraph:
//
//   - Entities translated and added to in-memory batch buffer
//   - Flush triggered when batch_size reached OR flush_interval elapsed
//   - Failed batches preserved and prepended to next batch
//   - Buffer capped at 2x batch_size to prevent unbounded growth
//   - Final flush on component shutdown
//
// # Thread Safety
//
// The component is fully thread-safe:
//   - Configuration is immutable after construction
//   - Atomic counters for metrics
//   - Mutex protection for batch buffer
//   - Message handler called from NATS goroutine pool
//   - Single goroutine owns the flush timer loop
//
// # Error Handling
//
// Errors follow the SemStreams error pattern:
//   - Transient errors (network, TrustGraph unavailable): batch preserved for retry
//   - Invalid config: returned from Initialize/Start
//   - Context cancellation: graceful shutdown with final flush
//
// Export errors increment the exportErrors counter and are logged. Failed batches
// are preserved (up to 2x batch_size) for retry on the next flush cycle.
//
// # Metrics
//
// Prometheus metrics exposed (namespace: semstreams, subsystem: trustgraph_output):
//
//   - entities_received_total: Total entities received from NATS
//   - entities_exported_total: Total entities successfully exported
//   - entities_filtered_total: Total entities filtered (prefix or source)
//   - triples_exported_total: Total RDF triples exported
//   - batches_sent_total: Total batches sent to TrustGraph
//   - export_errors_total: Total export failures
//   - export_duration_seconds: Histogram of export durations
//
// # Usage
//
// The component subscribes to entity changes and exports to TrustGraph:
//
//	graph-ingest → ENTITY_STATES → entity.> → trustgraph-output → TrustGraph
//
// For bidirectional sync, pair with trustgraph-input using matching vocab
// configuration and ensure loop prevention via exclude_sources.
//
// # Testing
//
// Unit tests use mock HTTP servers to simulate TrustGraph API responses.
// Integration tests require a NATS server (via testcontainers).
//
//	go test -v ./output/trustgraph/...
//	go test -tags=integration -v ./output/trustgraph/...
//
// # See Also
//
//   - input/trustgraph: Import entities from TrustGraph
//   - vocabulary/trustgraph: URI/EntityID translation
//   - trustgraph: TrustGraph REST API client
//   - docs/integration/trustgraph-integration.md: Integration guide
package trustgraph
