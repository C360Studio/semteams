# ADR-002: Query Capability Discovery Pattern

## Status

Accepted

## Context

The SemStreams graph system was decomposed from a monolithic processor into 9 specialized components:

- graph-ingest, graph-index, graph-anomalies, graph-clustering
- graph-embedding, graph-index-spatial, graph-index-temporal
- graph-gateway, graph-query

Each component owns specific KV buckets and provides query capabilities over its data. Key challenges:

1. **Discovery**: How do clients discover available queries across components?
2. **Routing**: How does the gateway route queries to appropriate handlers?
3. **Contracts**: How do we ensure type-safe request/response handling?
4. **Ownership**: Which component should handle which queries?

## Decision

Adopt the `QueryCapabilityProvider` interface pattern for runtime query discovery.

### Core Interface

```go
type QueryCapabilityProvider interface {
    QueryCapabilities() QueryCapabilities
}

type QueryCapabilities struct {
    Component   string            `json:"component"`
    Version     string            `json:"version"`
    Queries     []QueryCapability `json:"queries"`
    Definitions map[string]any    `json:"definitions,omitempty"`
}

type QueryCapability struct {
    Subject        string `json:"subject"`
    Operation      string `json:"operation"`
    Description    string `json:"description,omitempty"`
    RequestSchema  any    `json:"request_schema"`
    ResponseSchema any    `json:"response_schema"`
}
```

### Key Principles

1. **Optional Interface**: Components implement `QueryCapabilityProvider` only if they expose queries
2. **JSON Schema Validation**: Request/response contracts defined via JSON Schema
3. **Subject Convention**: `graph.<component>.query.<operation>`
4. **Single-Owner**: Components query the data they write (no cross-bucket queries)
5. **Capability Discovery**: Each component exposes `graph.<component>.capabilities`

### Implementation Pattern

Components declare query endpoints via `NATSRequestPort` in their port configuration:

```go
{Name: "query_entity", Type: "nats-request", Subject: "graph.ingest.query.entity"}
```

And implement the handler:

```go
func (c *Component) handleQueryEntity(ctx context.Context, data []byte) ([]byte, error) {
    // Parse request, query KV bucket, return JSON response
}
```

## Consequences

### Positive

- **Runtime Discovery**: Coordinators discover capabilities without code generation
- **Self-Documenting**: JSON Schema provides API documentation automatically
- **Type Safety**: Schema validation catches contract violations early
- **Clear Ownership**: Single-owner pattern prevents query conflicts
- **Loose Coupling**: Components don't know about each other

### Negative

- **Schema Maintenance**: JSON Schema definitions must be kept in sync with code
- **Runtime Overhead**: Schema validation adds latency (mitigated by caching)
- **No Query Composition**: Complex queries requiring multiple components need coordinator

### Neutral

- **JSON Schema Choice**: Alternative would be protobuf/gRPC, but JSON aligns with NATS patterns
- **Coordinator Required**: graph-query aggregates capabilities and routes queries

## Current Implementations

| Component | Queries | Capability Subject |
|-----------|---------|-------------------|
| graph-ingest | getEntity, getBatch, prefix | graph.ingest.capabilities |
| graph-index | getOutgoing, getIncoming, getAlias, getPredicate | graph.index.capabilities |
| graph-anomalies | getKCore, getPivotDistances, findOutliers | graph.anomalies.capabilities |
| graph-clustering | getCommunity, getMembers, getEntityCommunity, getCommunitiesByLevel | graph.clustering.capabilities |
| graph-embedding | findSimilar, search | graph.embedding.capabilities |

## Key Files

| File | Purpose |
|------|---------|
| `component/query.go` | QueryCapabilityProvider interface |
| `processor/graph-ingest/query.go` | Entity query implementation |
| `processor/graph-index/query.go` | Relationship query implementation |
| `processor/graph-query/query.go` | Query coordinator routing |

## References

- [Query Discovery Concept](../contributing/06-query-capabilities.md)
- [Graph Components Reference](../advanced/07-graph-components.md)
- [ADR-001: Pragmatic Semantic Web](./adr-001-pragmatic-semantic-web.md)
