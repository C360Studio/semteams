// Package output provides a TrustGraph output component that watches SemStreams
// entity state changes and exports them as RDF triples to TrustGraph knowledge cores.
//
// This enables operational data (sensor readings, telemetry, events) to be available
// in TrustGraph's GraphRAG queries.
//
// # Features
//
//   - Subscribes to NATS entity subjects
//   - Filters by entity prefix and source (loop prevention)
//   - Translates SemStreams entities to RDF triples
//   - Batches triples for efficient API calls
//   - Flushes on batch size or interval
//
// # Configuration
//
//	{
//	    "endpoint": "http://trustgraph:8088",
//	    "kg_core_id": "semstreams-ops",
//	    "collection": "operational",
//	    "batch_size": 100,
//	    "flush_interval": "5s",
//	    "entity_prefixes": ["acme.ops."],
//	    "exclude_sources": ["trustgraph"],
//	    "vocab": {
//	        "org_mappings": {
//	            "acme": "https://data.acme-corp.com/"
//	        }
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
// is critical. The output component excludes entities where all triples have
// Source == "trustgraph" (or any other configured exclude source).
package output
