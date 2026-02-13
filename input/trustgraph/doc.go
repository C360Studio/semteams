// Package trustgraph provides a TrustGraph input component that polls TrustGraph's
// triples-query API and emits SemStreams entity messages.
//
// This is the highest-value bridge component, bringing document-extracted knowledge
// from TrustGraph into the SemStreams operational graph. TrustGraph excels at using
// LLMs to extract entities and relationships from documents; this component makes
// that knowledge available for real-time operational reasoning.
//
// # Architecture
//
//	┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
//	│   TrustGraph    │────▶│  trustgraph      │────▶│  entity.>       │
//	│   REST API      │     │  -input          │     │  (NATS)         │
//	└─────────────────┘     └──────────────────┘     └─────────────────┘
//	                               │
//	                               ▼
//	                        ┌──────────────────┐
//	                        │  TRUSTGRAPH_SYNC │
//	                        │  (KV bucket)     │
//	                        └──────────────────┘
//
// # Features
//
//   - Polls TrustGraph REST API on configurable interval
//   - Groups triples by subject to assemble complete entities
//   - Translates RDF URIs to SemStreams 6-part entity IDs
//   - Translates RDF predicates to SemStreams predicate format
//   - Deduplicates via SHA256 hash-based change detection
//   - Publishes entities to NATS subjects based on entity type
//   - Full Prometheus metrics for observability
//
// # Configuration
//
//	{
//	    "endpoint": "http://trustgraph:8088",
//	    "poll_interval": "60s",
//	    "timeout": "30s",
//	    "limit": 1000,
//	    "collections": ["intelligence"],
//	    "subject_filter": "http://trustgraph.ai/e/",
//	    "source": "trustgraph",
//	    "vocab": {
//	        "uri_mappings": {
//	            "trustgraph.ai": {
//	                "org": "client",
//	                "platform": "intel",
//	                "domain": "knowledge",
//	                "system": "trustgraph",
//	                "type": "entity"
//	            }
//	        },
//	        "predicate_mappings": {
//	            "entity.classification.type": "http://www.w3.org/1999/02/22-rdf-syntax-ns#type",
//	            "entity.metadata.label": "http://www.w3.org/2000/01/rdf-schema#label"
//	        },
//	        "default_org": "external"
//	    },
//	    "ports": {
//	        "outputs": [
//	            {"name": "entity", "type": "nats", "subject": "entity.>"}
//	        ]
//	    }
//	}
//
// # Vocabulary Translation
//
// The component uses the vocabulary/trustgraph package for bidirectional translation
// between RDF URIs and SemStreams entity IDs.
//
// URI to Entity ID example:
//
//	http://trustgraph.ai/e/supply-chain-risk
//	→ client.intel.knowledge.trustgraph.entity.supply-chain-risk
//
// Predicate translation uses a two-tier approach:
//  1. Exact match from predicate_mappings configuration
//  2. Structural fallback for unmapped predicates
//
// # Change Detection
//
// The component maintains a TRUSTGRAPH_SYNC KV bucket for deduplication:
//
//	Key: input:hash:{entity_id}
//	Value: SHA256 hash of sorted triple set
//
// Entities are only published when their hash changes, preventing redundant
// processing of unchanged data.
//
// # Thread Safety
//
// The component is fully thread-safe:
//   - Configuration is immutable after construction
//   - Atomic counters for metrics
//   - Mutex protection for mutable state (lastPollTime)
//   - Single goroutine owns the poll loop
//
// # Error Handling
//
// Errors follow the SemStreams error pattern:
//   - Transient errors (network, TrustGraph unavailable): logged, poll continues
//   - Invalid config: returned from Initialize/Start
//   - Context cancellation: graceful shutdown
//
// Poll errors increment the pollErrors counter and are logged but don't stop
// the component. The Health() method considers the component unhealthy if
// no successful poll occurs within 2x the poll interval.
//
// # Metrics
//
// Prometheus metrics exposed (namespace: semstreams, subsystem: trustgraph_input):
//
//   - triples_imported_total: Total RDF triples imported
//   - entities_published_total: Total entities published to NATS
//   - polls_total: Total poll operations
//   - poll_errors_total: Total poll failures
//   - poll_duration_seconds: Histogram of poll durations
//
// # Usage
//
// The component is typically deployed as part of a SemStreams flow that includes
// graph-ingest to process the imported entities:
//
//	trustgraph-input → entity.> → graph-ingest → ENTITY_STATES
//
// For bidirectional sync, pair with trustgraph-output using matching vocab
// configuration and ensure loop prevention via source-based filtering.
//
// # Testing
//
// Unit tests use mock HTTP servers to simulate TrustGraph API responses.
// Integration tests require a NATS server (via testcontainers).
//
//	go test -v ./input/trustgraph/...
//	go test -tags=integration -v ./input/trustgraph/...
//
// # See Also
//
//   - output/trustgraph: Export entities to TrustGraph
//   - vocabulary/trustgraph: URI/EntityID translation
//   - trustgraph: TrustGraph REST API client
//   - docs/integration/trustgraph-integration.md: Integration guide
package trustgraph
