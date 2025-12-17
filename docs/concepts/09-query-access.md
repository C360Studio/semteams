# Query Access

How to query SemStreams knowledge graphs via GraphQL.

## Access Methods

SemStreams provides two ways to query knowledge graphs, both exposing the same GraphQL schema:

| Method | Port | Purpose | Audience |
|--------|------|---------|----------|
| GraphQL HTTP | 8080 | Standard GraphQL endpoint | Applications, users |
| MCP Gateway | 8081 | AI agent integration | Claude, LLM agents |

Both methods share the same GraphQL executor and BaseResolver, ensuring consistent behavior.

## Why GraphQL

GraphQL provides several advantages over REST for knowledge graph access:

### Request Only What You Need

GraphQL lets clients request exactly the fields they need:

```graphql
query {
  entity(id: "sensor-042") {
    id
    type
    properties
  }
}
```

A REST API would return all fields. With large graphs, this bloats responses unnecessarily.

### Structured Queries

Clients express intent through schema-validated queries. The schema documents available operations:

```graphql
type Query {
  entity(id: ID!): Entity
  relationships(entityId: ID!, direction: RelationshipDirection): [Relationship!]!
  semanticSearch(query: String!, limit: Int): [SemanticSearchResult!]!
  localSearch(entityId: ID!, query: String!): LocalSearchResult
  globalSearch(query: String!): GlobalSearchResult
  pathSearch(startEntity: ID!, maxDepth: Int): PathSearchResult
}
```

### Security and Auditability

Every query is:

- **Schema-validated**: Only defined operations are allowed
- **Logged**: Full query text captured for audit
- **Bounded**: Resource limits prevent runaway queries
- **Read-only**: No mutations, no data modification

## GraphQL HTTP Gateway

Standard GraphQL endpoint for applications and users.

### Endpoint

- **Query**: `POST /graphql`
- **Playground**: `GET /` (optional, for exploration)
- **Health**: `GET /health`

### Example Request

```bash
curl -X POST http://localhost:8080/graphql \
  -H "Content-Type: application/json" \
  -d '{
    "query": "{ entity(id: \"sensor-042\") { id type properties } }"
  }'
```

### Response Format

```json
{
  "data": {
    "entity": {
      "id": "sensor-042",
      "type": "Sensor",
      "properties": {"location": "warehouse-a", "model": "TH-200"}
    }
  }
}
```

### GraphQL Playground

When enabled, visiting the root URL opens an interactive GraphQL IDE for exploring the schema and testing queries.

## MCP Gateway

For AI agents using Model Context Protocol.

### What is MCP?

Model Context Protocol is Anthropic's open standard for connecting AI assistants to external data sources. Instead of giving an AI agent arbitrary code execution, MCP provides structured, auditable operations.

### Why MCP for AI Agents?

| Aspect | MCP Gateway | Direct Tools (shell, Python) |
|--------|-------------|------------------------------|
| Capabilities | Defined by schema | Unlimited |
| Auditability | Full query logging | Limited |
| Security | Schema-validated | Trust-based |
| Token efficiency | Request only needed fields | Full responses |
| Error handling | Structured errors | Arbitrary failures |

MCP trades flexibility for safety—ideal for production AI agent deployments.

### Connection

AI agents connect via HTTP with Server-Sent Events (SSE) transport:

- **Endpoint**: `http://localhost:8081/mcp`
- **Protocol**: MCP over SSE

### Tool Interface

The MCP gateway exposes a single `graphql` tool that accepts any valid GraphQL query:

```json
{
  "tool": "graphql",
  "arguments": {
    "query": "{ entity(id: \"sensor-042\") { id type } }",
    "variables": {}
  }
}
```

## Available Operations

Both gateways expose the same operations:

### Entity Operations

| Query | Description |
|-------|-------------|
| `entity(id)` | Get single entity by ID |
| `entityByAlias(aliasOrID)` | Get entity by alias or ID |
| `entities(ids)` | Batch get multiple entities |
| `entitiesByType(type, limit)` | Get all entities of a type |

### Relationship Queries

| Query | Description |
|-------|-------------|
| `relationships(entityId, direction, edgeTypes)` | Query relationships for an entity |
| `pathSearch(startEntity, maxDepth, direction, edgeTypes)` | Bounded graph traversal (PathRAG) |

### Search Operations

| Query | Description |
|-------|-------------|
| `semanticSearch(query, limit)` | Vector similarity search |
| `localSearch(entityId, query)` | Search within entity's community |
| `globalSearch(query, maxCommunities)` | Search across all communities |
| `spatialSearch(north, south, east, west)` | Entities within geographic bounds |
| `temporalSearch(startTime, endTime)` | Entities within time range |

### Community Operations

| Query | Description |
|-------|-------------|
| `community(id)` | Get community by ID |
| `entityCommunity(entityId, level)` | Get community containing an entity |
| `communitiesByLevel(level)` | List all communities at a hierarchy level |

### Snapshot Operations

| Query | Description |
|-------|-------------|
| `graphSnapshot(bounds, timeRange, types)` | Extract bounded subgraph |

## Architecture

Both gateways share the same execution path:

```text
┌──────────────────┐     ┌──────────────────┐
│  GraphQL HTTP    │     │   MCP Gateway    │
│  (port 8080)     │     │   (port 8081)    │
└────────┬─────────┘     └────────┬─────────┘
         │                        │
         └──────────┬─────────────┘
                    │
         ┌──────────▼──────────┐
         │   GraphQL Executor  │
         └──────────┬──────────┘
                    │
         ┌──────────▼──────────┐
         │    BaseResolver     │
         └──────────┬──────────┘
                    │
         ┌──────────▼──────────┐
         │    QueryManager     │◄── NATS KV
         └─────────────────────┘
```

Key design decisions:

- **Shared executor**: Both gateways use the same GraphQL execution engine
- **In-process execution**: No network hop between gateway and graph data
- **Direct QueryManager access**: Bypasses serialization overhead

## When to Use Which

| Use Case | Recommended |
|----------|-------------|
| Application integration | GraphQL HTTP |
| AI agent (Claude, etc.) | MCP Gateway |
| Interactive exploration | GraphQL Playground |
| Automated scripts | GraphQL HTTP |
| Browser-based apps | GraphQL HTTP (with CORS) |

## Configuration

Both gateways are configured via the component configuration. See [Configuration Guide](../basics/06-configuration.md) for details.

### Resource Limits

Both gateways enforce the same limits:

| Limit | Default | Description |
|-------|---------|-------------|
| Query timeout | 30s | Maximum query execution time |
| Max results | 1000 | Maximum entities per query |
| Max depth | 10 | Maximum PathRAG traversal depth |

## Related

- [GraphRAG Pattern](07-graphrag-pattern.md) - Community-based search
- [PathRAG Pattern](08-pathrag-pattern.md) - Structural traversal
- [Knowledge Graphs](02-knowledge-graphs.md) - The data model exposed by GraphQL
- [Event-Driven Basics](01-event-driven-basics.md) - HTTP Request/Reply gateway for NATS services
