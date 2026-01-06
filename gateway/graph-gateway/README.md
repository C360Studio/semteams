# graph-gateway

HTTP gateway component for external access to the knowledge graph via GraphQL and MCP protocols.

## Overview

The graph-gateway component provides HTTP endpoints for querying and mutating the knowledge graph. It serves as the external access layer, translating HTTP requests into KV reads (via QueryManager) and NATS mutations.

## Architecture

```
                                   ┌─────────────────────┐
HTTP /graphql ──────────────────►  │                     │
                                   │   graph-gateway     │ ──► NATS graph.mutation.*
HTTP /mcp ──────────────────────►  │                     │
                                   │   (QueryManager)    │ ──► KV reads (direct)
HTTP / (playground) ────────────►  │                     │
                                   └─────────────────────┘
```

## Features

- **GraphQL API**: Full query and mutation support for graph operations
- **MCP Protocol**: Model Context Protocol for LLM tool integration
- **GraphQL Playground**: Interactive development IDE
- **Read-through KV**: Direct KV access for queries (no NATS overhead)
- **Mutation Forwarding**: Mutations sent via NATS to graph-ingest

## Configuration

```json
{
  "name": "graph-gateway",
  "type": "gateway",
  "config": {
    "graphql_path": "/graphql",
    "mcp_path": "/mcp",
    "bind_address": "localhost:8080",
    "enable_playground": true,
    "ports": {
      "inputs": [
        {"name": "http", "type": "http", "subject": "/graphql"}
      ],
      "outputs": [
        {"name": "mutations", "type": "nats-request", "subject": "graph.mutation.*"}
      ]
    }
  }
}
```

### Configuration Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `graphql_path` | string | `/graphql` | GraphQL endpoint path |
| `mcp_path` | string | `/mcp` | MCP endpoint path |
| `bind_address` | string | `localhost:8080` | HTTP server bind address |
| `enable_playground` | bool | `false` | Enable GraphQL playground UI |

## HTTP Endpoints

### GraphQL (`/graphql`)

Standard GraphQL endpoint supporting:

**Queries:**
- `entity(id: ID!)`: Fetch single entity
- `entities(filter: EntityFilter)`: Search entities
- `triples(subject: ID, predicate: String, object: ID)`: Query triples
- `traverse(start: ID!, depth: Int!)`: Graph traversal
- `communities(algorithm: String)`: Community detection results

**Mutations:**
- `createEntity(input: EntityInput!)`: Create new entity
- `updateEntity(id: ID!, input: EntityInput!)`: Update entity
- `deleteEntity(id: ID!)`: Delete entity
- `createTriple(input: TripleInput!)`: Create relationship
- `deleteTriple(id: ID!)`: Delete relationship

### MCP (`/mcp`)

Model Context Protocol endpoint for LLM integration:

**Tools:**
- `graph_query`: Execute graph queries
- `entity_lookup`: Find entities by alias or ID
- `relationship_find`: Discover relationships
- `context_build`: Build context from graph neighborhood

### Playground (`/`)

When enabled, serves an interactive GraphQL IDE for:
- Query composition and testing
- Schema exploration
- Response visualization

## KV Bucket Access

The gateway reads from multiple KV buckets via QueryManager:

| Bucket | Purpose |
|--------|---------|
| `ENTITY_STATES` | Entity data and properties |
| `OUTGOING_INDEX` | Forward relationship traversal |
| `INCOMING_INDEX` | Backward relationship traversal |
| `ALIAS_INDEX` | Entity lookup by alias |
| `PREDICATE_INDEX` | Predicate-based queries |
| `EMBEDDINGS_CACHE` | Vector similarity (semantic tier) |
| `COMMUNITY_INDEX` | Clustering results (semantic tier) |
| `SPATIAL_INDEX` | Geospatial queries (if enabled) |
| `TEMPORAL_INDEX` | Time-based queries (if enabled) |
| `STRUCTURAL_INDEX` | Graph analytics (if enabled) |

## Integration

### With graph-ingest

Mutations received by the gateway are forwarded via NATS:
- `graph.mutation.entity.create`
- `graph.mutation.entity.update`
- `graph.mutation.entity.delete`
- `graph.mutation.triple.add`
- `graph.mutation.triple.delete`

### With QueryManager

The gateway uses QueryManager for complex read operations:
- Multi-hop traversals
- Filtered entity searches
- Aggregation queries

## Security Considerations

Production deployments should:
1. Disable playground (`enable_playground: false`)
2. Deploy behind authentication proxy
3. Implement rate limiting
4. Use TLS termination at load balancer

## Deployment

### All Tiers

The gateway is required in all deployment tiers:

```json
{
  "components": [
    {"type": "processor", "name": "graph-ingest"},
    {"type": "processor", "name": "graph-index"},
    {"type": "gateway", "name": "graph-gateway"}
  ]
}
```

### High Availability

For production, deploy multiple gateway instances behind a load balancer. Each instance maintains its own QueryManager with KV connections.
