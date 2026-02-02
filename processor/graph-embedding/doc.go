// Package graphembedding provides the graph-embedding component for generating entity embeddings.
//
// # Overview
//
// The graph-embedding component watches the ENTITY_STATES KV bucket and generates
// vector embeddings for entities, storing them in the EMBEDDINGS_CACHE KV bucket.
// These embeddings enable semantic similarity search and clustering.
//
// # Tier
//
// Tier: STATISTICAL (Tier 1) with BM25, SEMANTIC (Tier 2) with HTTP embeddings.
// Not used in Structural (Tier 0) deployments.
//
// # Architecture
//
// graph-embedding is a Tier 1+ component. It is not used in Structural-only
// deployments but required for semantic search and community detection features.
//
//	                    ┌──────────────────┐
//	ENTITY_STATES ─────►│                  │
//	   (KV watch)       │  graph-embedding ├──► EMBEDDINGS_CACHE (KV)
//	                    │                  │
//	                    └────────┬─────────┘
//	                             │
//	                             ▼
//	                    ┌──────────────────┐
//	                    │  Embedding API   │
//	                    │  (HTTP/BM25)     │
//	                    └──────────────────┘
//
// # Features
//
//   - Entity text extraction from configurable fields
//   - HTTP embedding API integration (OpenAI-compatible)
//   - BM25 fallback for offline/lightweight deployments
//   - Batch processing for efficiency
//   - Caching with configurable TTL
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
//	      {"name": "embeddings", "subject": "EMBEDDINGS_CACHE", "type": "kv"}
//	    ]
//	  },
//	  "embedder_type": "http",
//	  "embedder_url": "http://semembed:8081/v1",
//	  "batch_size": 50,
//	  "cache_ttl": "1h"
//	}
//
// # Port Definitions
//
// Inputs:
//   - KV watch: ENTITY_STATES - watches for entity state changes
//
// Outputs:
//   - KV bucket: EMBEDDINGS_CACHE - stores vector embeddings keyed by entity ID
//
// # Embedder Types
//
//   - http: Uses HTTP API (OpenAI-compatible) for embedding generation
//   - bm25: Uses BM25 sparse vectors for lightweight deployments
//
// # Usage
//
// Register the component with the component registry:
//
//	import graphembedding "github.com/c360studio/semstreams/processor/graph-embedding"
//
//	func init() {
//	    graphembedding.Register(registry)
//	}
//
// # Dependencies
//
// Upstream:
//   - graph-ingest: produces ENTITY_STATES that this component watches
//
// Downstream:
//   - graph-clustering: reads EMBEDDINGS_CACHE for semantic similarity in community detection
//   - graph-gateway: reads EMBEDDINGS_CACHE for semantic search queries
package graphembedding
