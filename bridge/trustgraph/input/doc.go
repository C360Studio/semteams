// Package input provides a TrustGraph input component that polls TrustGraph's
// triples-query API and emits SemStreams entity messages.
//
// This is the highest-value bridge component, bringing document-extracted knowledge
// from TrustGraph into the SemStreams operational graph.
//
// # Features
//
//   - Polls TrustGraph REST API on configurable interval
//   - Groups triples by subject to assemble entities
//   - Translates RDF URIs to SemStreams entity IDs
//   - Deduplicates via hash-based change detection
//   - Publishes to NATS entity subjects
//
// # Configuration
//
//	{
//	    "endpoint": "http://trustgraph:8088",
//	    "poll_interval": "60s",
//	    "collections": ["intelligence"],
//	    "source": "trustgraph",
//	    "vocab": {
//	        "uri_mappings": {
//	            "trustgraph.ai": {
//	                "org": "client",
//	                "platform": "intel"
//	            }
//	        }
//	    },
//	    "ports": {
//	        "outputs": [
//	            {"name": "entity", "type": "nats", "subject": "entity.>"}
//	        ]
//	    }
//	}
//
// # Usage
//
// The component is typically deployed as part of a SemStreams flow that includes
// graph-ingest to process the imported entities:
//
//	trustgraph-input → entity.> → graph-ingest → ENTITY_STATES
package input
