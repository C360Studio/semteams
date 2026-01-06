// Package graphclustering provides the graph-clustering component for community detection.
//
// # Overview
//
// The graph-clustering component performs community detection on the entity graph
// using Label Propagation Algorithm (LPA). It identifies clusters of related entities
// and optionally enhances community descriptions using LLM.
//
// # Tier
//
// Tier: STATISTICAL (Tier 1) without LLM, SEMANTIC (Tier 2) with LLM enhancement.
// Not used in Structural (Tier 0) deployments.
//
// # Architecture
//
// graph-clustering is a Tier 1+ component. It watches ENTITY_STATES for change
// events and triggers community detection based on configurable thresholds.
//
//	                    ┌───────────────────┐
//	ENTITY_STATES ─────►│                   │
//	   (KV watch)       │  graph-clustering ├──► COMMUNITY_INDEX (KV)
//	                    │                   │
//	                    └─────────┬─────────┘
//	                              │ (reads)
//	              ┌───────────────┼───────────────┐
//	              ▼               ▼               ▼
//	       OUTGOING_INDEX  INCOMING_INDEX  EMBEDDINGS_CACHE
//
// # Features
//
//   - Label Propagation Algorithm (LPA) for community detection
//   - Configurable detection interval and batch thresholds
//   - Optional LLM-based community summarization
//   - Semantic edge generation based on embedding similarity
//   - Inferred relationship creation within communities
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
//	      {"name": "communities", "subject": "COMMUNITY_INDEX", "type": "kv"}
//	    ]
//	  },
//	  "detection_interval": "30s",
//	  "batch_size": 100,
//	  "enable_llm": true,
//	  "llm_endpoint": "http://seminstruct:8083/v1",
//	  "min_community_size": 2,
//	  "max_iterations": 100
//	}
//
// # Scheduling
//
// Community detection is triggered by:
//   - Timer: Runs every detection_interval
//   - Batch threshold: Runs when batch_size entity changes accumulate
//
// Whichever comes first triggers detection.
//
// # Port Definitions
//
// Inputs:
//   - KV watch: ENTITY_STATES - watches for entity changes to count events
//
// Outputs:
//   - KV bucket: COMMUNITY_INDEX - stores detected communities
//
// # Usage
//
// Register the component with the component registry:
//
//	import graphclustering "github.com/c360/semstreams/processor/graph-clustering"
//
//	func init() {
//	    graphclustering.Register(registry)
//	}
//
// # Dependencies
//
// Upstream (reads during detection):
//   - graph-ingest: watches ENTITY_STATES for change events
//   - graph-index: reads OUTGOING_INDEX and INCOMING_INDEX for graph structure
//   - graph-embedding: reads EMBEDDINGS_CACHE for semantic similarity
//
// Downstream:
//   - graph-gateway: reads COMMUNITY_INDEX for community queries
package graphclustering
