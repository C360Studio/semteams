// Package mcp provides a Model Context Protocol (MCP) gateway for SemStreams.
//
// The MCP gateway enables AI agents (Claude Desktop, Claude Code, etc.) to query
// the SemStreams semantic graph using GraphQL through a single MCP tool.
//
// # Architecture: In-Process GraphQL Execution
//
// Unlike traditional HTTP proxies, this gateway executes GraphQL queries in-process,
// eliminating network overhead and enabling direct access to the QueryManager's
// multi-tier caching system.
//
//	┌─────────────────────────────────────────────────────────────┐
//	│                    TRANSPORT LAYER                          │
//	│  MCP: Agent integration, auth, reconnection, SSE            │
//	│  Why: Claude/agents speak MCP natively                      │
//	└─────────────────────────────────────────────────────────────┘
//	                              ↓
//	┌─────────────────────────────────────────────────────────────┐
//	│                    QUERY LAYER                              │
//	│  GraphQL: Query language, validation, introspection         │
//	│  Why: Solves over/under-fetch, self-documenting             │
//	└─────────────────────────────────────────────────────────────┘
//	                              ↓
//	┌─────────────────────────────────────────────────────────────┐
//	│                    LOGIC LAYER                              │
//	│  BaseResolver: Entity queries, search, relationships        │
//	│  Why: Reusable across HTTP/MCP/CLI interfaces               │
//	└─────────────────────────────────────────────────────────────┘
//	                              ↓
//	┌─────────────────────────────────────────────────────────────┐
//	│                    DATA LAYER                               │
//	│  QueryManager: Multi-tier caching, NATS, indexes            │
//	│  Why: Performance, consistency, distributed access          │
//	└─────────────────────────────────────────────────────────────┘
//
// # Performance Comparison
//
//	| Approach             | Latency   | Network Hops | Serialization |
//	|----------------------|-----------|--------------|---------------|
//	| HTTP to GraphQL      | 10-50ms   | 1            | 2x            |
//	| Direct QueryManager  | 1-5μs     | 0            | 0x            |
//	| In-Process GraphQL   | 10-100μs  | 0            | 1x (NATS only)|
//
// # Design Philosophy
//
// Each layer does what it does best:
//
//   - MCP handles agent integration, authentication, and transport (SSE)
//   - GraphQL provides query language, validation, and introspection
//   - BaseResolver implements entity queries, search, and relationships
//   - QueryManager provides multi-tier caching and data access
//
// This separation means layers are independently replaceable. You could swap MCP
// for WebSocket, GraphQL for REST, or QueryManager for direct database access
// without affecting other layers.
//
// # Single Tool Design
//
// The gateway exposes a single "graphql" MCP tool rather than individual tools
// per operation (entity, search, relationships, etc.). This design:
//
//   - Leverages GraphQL's introspection for agent discovery
//   - Prevents over/under-fetching via GraphQL's query composition
//   - Reduces token waste from multiple tool definitions
//   - Provides type safety through GraphQL schema validation
//
// # Usage
//
// Configuration in component YAML:
//
//	components:
//	  mcp-gateway:
//	    name: mcp
//	    type: gateway
//	    enabled: true
//	    config:
//	      bind_address: ":8081"
//	      timeout: "30s"
//
// Claude Desktop configuration:
//
//	{
//	  "mcpServers": {
//	    "semstreams": {
//	      "url": "http://localhost:8081/mcp"
//	    }
//	  }
//	}
//
// # Example Agent Queries
//
// Schema discovery:
//
//	{ __schema { types { name } } }
//
// Entity query with relationships:
//
//	{
//	  entity(id: "robot-1") {
//	    id type
//	    relationships(direction: OUTGOING) {
//	      toEntityId edgeType
//	    }
//	  }
//	}
//
// Semantic search:
//
//	{
//	  semanticSearch(query: "navigation capable robots", limit: 5) {
//	    entity { id type }
//	    score
//	  }
//	}
//
// # Dependencies
//
// The MCP gateway requires:
//
//   - QueryManager: For cached data access (required)
//   - NATS Client: For fallback queries (optional)
//
// When QueryManager is available, queries benefit from:
//
//   - L1 hot cache: 10K most recently accessed entities
//   - L2 warm cache: Query result caching
//   - KV Watch: Automatic cache invalidation
//   - NATS fallback: When cache misses occur
package mcp
