// Package graphql provides a GraphQL HTTP gateway as an output port of the graph processor.
//
// This package exposes graph query capabilities via a standard GraphQL HTTP endpoint.
// Unlike standalone gateway components, this gateway has direct in-process access to
// the QueryManager, enabling efficient execution of PathRAG, GraphRAG, community queries,
// and entity operations without network overhead.
//
// # Architecture
//
// The GraphQL gateway is an output port of the graph processor, not an independent component.
// This design reflects the true architectural dependency: the gateway requires in-process
// access to QueryManager and cannot function without the graph processor running.
//
//	┌─────────────────────────────────────────┐
//	│           Graph Processor               │
//	│  ┌─────────────┐  ┌─────────────────┐   │
//	│  │ QueryManager│──│ GraphQL Gateway │───┼──► HTTP :8080/graphql
//	│  └─────────────┘  └─────────────────┘   │
//	│                   ┌─────────────────┐   │
//	│                   │   MCP Gateway   │───┼──► HTTP :8081/mcp
//	│                   └─────────────────┘   │
//	└─────────────────────────────────────────┘
//
// # Usage
//
// The gateway is configured through the graph processor's configuration:
//
//	{
//	  "components": {
//	    "graph": {
//	      "type": "processor",
//	      "name": "graph-processor",
//	      "config": {
//	        "gateway": {
//	          "graphql": {
//	            "enabled": true,
//	            "bind_address": ":8080",
//	            "path": "/graphql",
//	            "playground": true
//	          }
//	        }
//	      }
//	    }
//	  }
//	}
//
// # Query Types
//
// The gateway supports:
//   - Entity queries (by ID, type, property filters)
//   - Relationship queries (by type, direction)
//   - Path search (PathRAG) - finding paths between entities
//   - Local search (GraphRAG) - neighborhood exploration
//   - Global search (GraphRAG) - cross-graph queries
//   - Community queries - detecting and querying communities
//
// # Lifecycle
//
// The gateway lifecycle is managed by the graph processor:
//  1. Graph processor starts QueryManager
//  2. Graph processor creates Resolver with QueryManager reference
//  3. Graph processor creates and starts GraphQL Server
//  4. On shutdown, gateway stops before QueryManager
package graphql
