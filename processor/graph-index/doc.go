// Package graphindex provides the graph-index component for maintaining graph relationship indexes.
//
// # Overview
//
// The graph-index component watches the ENTITY_STATES KV bucket and maintains
// relationship indexes that enable efficient graph traversal and querying.
//
// # Tier
//
// Tier: ALL TIERS (Tier 0, 1, 2) - Required for all deployments.
//
// # Architecture
//
// graph-index is a core component required for all deployment tiers (Structural,
// Statistical, Semantic). It watches entity state changes and updates four index
// buckets in parallel.
//
//	                    ┌─────────────────┐
//	                    │                 ├──► OUTGOING_INDEX (KV)
//	ENTITY_STATES ─────►│   graph-index   ├──► INCOMING_INDEX (KV)
//	   (KV watch)       │                 ├──► ALIAS_INDEX (KV)
//	                    │                 ├──► PREDICATE_INDEX (KV)
//	                    └─────────────────┘
//
// # Indexes
//
// The component maintains four relationship indexes:
//
//   - OUTGOING_INDEX: Maps entity ID → outgoing relationships (subject → predicate → object)
//   - INCOMING_INDEX: Maps entity ID → incoming relationships (object ← predicate ← subject)
//   - ALIAS_INDEX: Maps alias strings → entity IDs for fast lookup
//   - PREDICATE_INDEX: Maps predicate → entity IDs for predicate-based queries
//
// # Configuration
//
// The component is configured via JSON with the following structure:
//
//	{
//	  "ports": {
//	    "inputs": [
//	      {"name": "entity_watch", "subject": "ENTITY_STATES", "type": "kv-watch"}
//	    ],
//	    "outputs": [
//	      {"name": "outgoing_index", "subject": "OUTGOING_INDEX", "type": "kv"},
//	      {"name": "incoming_index", "subject": "INCOMING_INDEX", "type": "kv"},
//	      {"name": "alias_index", "subject": "ALIAS_INDEX", "type": "kv"},
//	      {"name": "predicate_index", "subject": "PREDICATE_INDEX", "type": "kv"}
//	    ]
//	  },
//	  "workers": 4,
//	  "batch_size": 50
//	}
//
// # Port Definitions
//
// Inputs:
//   - KV watch: ENTITY_STATES - watches for entity state changes
//
// Outputs:
//   - KV bucket: OUTGOING_INDEX - outgoing relationship index
//   - KV bucket: INCOMING_INDEX - incoming relationship index
//   - KV bucket: ALIAS_INDEX - entity alias lookup index
//   - KV bucket: PREDICATE_INDEX - predicate-based index
//
// # Usage
//
// Register the component with the component registry:
//
//	import graphindex "github.com/c360/semstreams/processor/graph-index"
//
//	func init() {
//	    graphindex.Register(registry)
//	}
//
// # Dependencies
//
// Upstream:
//   - graph-ingest: produces ENTITY_STATES that this component watches
//
// Downstream:
//   - graph-anomalies: watches OUTGOING_INDEX and INCOMING_INDEX for anomaly detection
//   - graph-gateway: reads indexes for query resolution
package graphindex
