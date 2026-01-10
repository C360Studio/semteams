# Query Discovery

How to discover and access query operations across all SemStreams graph components using the
QueryCapabilityProvider interface and NATS request/reply patterns.

## Overview

SemStreams components expose their query capabilities through a standardized discovery mechanism.
Each component that implements the `QueryCapabilityProvider` interface publishes schema information
about its query operations, enabling runtime discovery and dynamic routing.

This enables:

- **Dynamic query routing** - Coordinators like graph-query can discover and route to backend services
- **Schema validation** - Request and response schemas are documented and discoverable
- **API documentation** - Query operations self-document through JSON Schema
- **Graceful degradation** - Systems can adapt when optional components are unavailable

## Capabilities Discovery

Each component exposes its capabilities at a standardized NATS subject:

```text
graph.<component>.capabilities
```

### Discovery Example

Query capabilities from any component using NATS CLI:

```bash
# Discover graph-ingest capabilities
nats req graph.ingest.capabilities ''

# Discover graph-index capabilities
nats req graph.index.capabilities ''

# Discover graph-clustering capabilities
nats req graph.clustering.capabilities ''
```

### Capabilities Response Format

```json
{
  "component": "graph-ingest",
  "version": "1.0.0",
  "queries": [
    {
      "subject": "graph.ingest.query.entity",
      "operation": "getEntity",
      "description": "Get single entity by ID",
      "request_schema": {
        "type": "object",
        "properties": {
          "id": {"type": "string", "description": "Entity ID to retrieve"}
        },
        "required": ["id"]
      },
      "response_schema": {
        "$ref": "#/definitions/EntityState"
      }
    }
  ],
  "definitions": {
    "EntityState": {
      "type": "object",
      "properties": {
        "id": {"type": "string"},
        "triples": {"type": "array"},
        "version": {"type": "integer"},
        "updated_at": {"type": "string", "format": "date-time"}
      }
    }
  }
}
```

## Available Query Operations

### graph-ingest Queries

Entity storage and retrieval operations.

| Subject | Operation | Description |
|---------|-----------|-------------|
| `graph.ingest.query.entity` | getEntity | Get single entity by ID |
| `graph.ingest.query.batch` | getBatch | Get multiple entities by IDs |
| `graph.ingest.query.prefix` | listByPrefix | List entity IDs matching a prefix |

**Example: Get Entity**

```bash
nats req graph.ingest.query.entity '{"id":"acme.ops.robotics.gcs.drone.001"}'
```

Response:

```json
{
  "id": "acme.ops.robotics.gcs.drone.001",
  "triples": [
    {"subject": "...", "predicate": "rdf:type", "object": "Drone"}
  ],
  "version": 3,
  "updated_at": "2026-01-06T10:30:00Z"
}
```

### graph-index Queries

Relationship and alias resolution operations.

| Subject | Operation | Description |
|---------|-----------|-------------|
| `graph.index.query.outgoing` | getOutgoing | Get outgoing relationships for an entity |
| `graph.index.query.incoming` | getIncoming | Get incoming relationships for an entity |
| `graph.index.query.alias` | getAlias | Resolve alias to canonical entity ID |
| `graph.index.query.predicate` | getPredicate | Get entities with a specific predicate |

**Example: Get Outgoing Relationships**

```bash
nats req graph.index.query.outgoing '{"entity_id":"acme.ops.robotics.gcs.drone.001"}'
```

Response:

```json
[
  {"to_entity_id": "acme.ops.robotics.gcs.warehouse.main", "predicate": "locatedAt"},
  {"to_entity_id": "acme.ops.robotics.gcs.mission.42", "predicate": "assignedTo"}
]
```

### graph-clustering Queries

Community detection, structural analysis, and anomaly detection operations.

| Subject | Operation | Description |
|---------|-----------|-------------|
| `graph.clustering.query.community` | getCommunity | Get community by ID |
| `graph.clustering.query.members` | getMembers | Get members of a community |
| `graph.clustering.query.entity` | getEntityCommunity | Get community for an entity at a level |
| `graph.clustering.query.level` | getCommunitiesByLevel | Get all communities at a hierarchy level |
| `graph.clustering.query.kcore` | getKCore | Get k-core decomposition info for entities |
| `graph.clustering.query.anomalies` | getAnomalies | Get detected anomalies |

**Example: Get Entity Community**

```bash
nats req graph.clustering.query.entity '{"entity_id":"acme.ops.drone.001","level":0}'
```

Response:

```json
{
  "entity_id": "acme.ops.drone.001",
  "level": 0,
  "community": {
    "id": "comm-42-L0",
    "level": 0,
    "members": ["acme.ops.drone.001", "acme.ops.drone.002"],
    "summary": "Drone operations cluster",
    "keywords": ["drone", "robotics", "operations"]
  }
}
```

### graph-embedding Queries

Semantic similarity and vector search operations.

| Subject | Operation | Description |
|---------|-----------|-------------|
| `graph.embedding.query.similar` | findSimilar | Find entities similar to a given entity |
| `graph.embedding.query.search` | search | Text-to-entity semantic search |

**Example: Find Similar Entities**

```bash
nats req graph.embedding.query.similar '{"entity_id":"acme.ops.drone.001","limit":5}'
```

Response:

```json
{
  "entity_id": "acme.ops.drone.001",
  "similar": [
    {"entity_id": "acme.ops.drone.002", "similarity": 0.95},
    {"entity_id": "acme.ops.robot.003", "similarity": 0.87}
  ],
  "duration": "45ms"
}
```

**Example: Semantic Text Search**

```bash
nats req graph.embedding.query.search '{"query":"autonomous flying robots","limit":5}'
```

Response:

```json
{
  "query": "autonomous flying robots",
  "results": [
    {"entity_id": "acme.ops.drone.001", "similarity": 0.91},
    {"entity_id": "acme.ops.uav.005", "similarity": 0.88}
  ],
  "duration": "120ms"
}
```

### graph-index-spatial Queries

Geographic and spatial query operations.

| Subject | Operation | Description |
|---------|-----------|-------------|
| `graph.spatial.query.bounds` | getBounds | Get entities within geographic bounding box |

**Example: Spatial Bounding Box Query**

```bash
nats req graph.spatial.query.bounds \
  '{"north":37.8,"south":37.7,"east":-122.3,"west":-122.5,"limit":100}'
```

Response:

```json
[
  {"id": "acme.ops.sensor.042", "type": "entity"},
  {"id": "acme.ops.drone.001", "type": "entity"}
]
```

### graph-index-temporal Queries

Time-based query operations.

| Subject | Operation | Description |
|---------|-----------|-------------|
| `graph.temporal.query.range` | getRange | Get entities within time range |

**Example: Temporal Range Query**

```bash
nats req graph.temporal.query.range \
  '{"startTime":"2026-01-01T00:00:00Z","endTime":"2026-01-07T23:59:59Z","limit":100}'
```

Response:

```json
[
  {"id": "acme.ops.event.001", "type": "entity"},
  {"id": "acme.ops.event.002", "type": "entity"}
]
```

## Query Coordinator (graph-query)

The `graph-query` component acts as a unified routing layer that orchestrates queries across
backend components. It provides a single entry point for complex operations that require
coordination across multiple components.

### Coordinator Operations

| Subject | Operation | Routes To |
|---------|-----------|-----------|
| `graph.query.entity` | entity | graph-ingest |
| `graph.query.relationships` | relationships | graph-index |
| `graph.query.pathSearch` | pathSearch | Internal PathSearcher |
| `graph.query.hierarchyStats` | hierarchyStats | graph-ingest (orchestrated) |
| `graph.query.prefix` | prefix | graph-ingest |
| `graph.query.spatial` | spatial | graph-index-spatial |
| `graph.query.temporal` | temporal | graph-index-temporal |
| `graph.query.semantic` | semantic | graph-embedding |
| `graph.query.similar` | similar | graph-embedding |

### PathRAG Traversal

The graph-query component provides PathRAG traversal orchestration that coordinates entity
retrieval and relationship traversal across graph-ingest and graph-index:

```bash
nats req graph.query.pathSearch \
  '{"startEntity":"acme.ops.drone.001","maxDepth":3,"direction":"outgoing"}'
```

Response includes traversed paths, visited entities, and relationship chains.

### Aggregated Capabilities

The coordinator can aggregate capabilities from all backend components:

```bash
nats req graph.query.capabilities ''
```

Response:

```json
{
  "components": [
    {
      "component": "graph-ingest",
      "version": "1.0.0",
      "queries": [...]
    },
    {
      "component": "graph-index",
      "version": "1.0.0",
      "queries": [...]
    }
  ]
}
```

## QueryCapabilityProvider Interface

Components implement this interface to expose their query capabilities:

```go
type QueryCapabilityProvider interface {
    QueryCapabilities() QueryCapabilities
}

type QueryCapabilities struct {
    Component   string              `json:"component"`
    Version     string              `json:"version"`
    Queries     []QueryCapability   `json:"queries"`
    Definitions map[string]any      `json:"definitions,omitempty"`
}

type QueryCapability struct {
    Subject         string `json:"subject"`
    Operation       string `json:"operation"`
    Description     string `json:"description,omitempty"`
    RequestSchema   any    `json:"request_schema"`
    ResponseSchema  any    `json:"response_schema"`
}
```

### Implementation Pattern

Components follow this pattern:

1. **Implement QueryCapabilityProvider** - Define QueryCapabilities() method
2. **Register capabilities handler** - Subscribe to `graph.<component>.capabilities`
3. **Document schemas** - Use JSON Schema for request/response validation
4. **Share type definitions** - Use `$ref` syntax to reference common types

Example from graph-ingest:

```go
func (c *Component) QueryCapabilities() component.QueryCapabilities {
    return component.QueryCapabilities{
        Component: "graph-ingest",
        Version:   "1.0.0",
        Queries: []component.QueryCapability{
            {
                Subject:     "graph.ingest.query.entity",
                Operation:   "getEntity",
                Description: "Get single entity by ID",
                RequestSchema: map[string]any{
                    "type": "object",
                    "properties": map[string]any{
                        "id": map[string]any{
                            "type": "string",
                            "description": "Entity ID to retrieve",
                        },
                    },
                    "required": []string{"id"},
                },
                ResponseSchema: map[string]any{
                    "$ref": "#/definitions/EntityState",
                },
            },
        },
        Definitions: map[string]any{
            "EntityState": map[string]any{
                "type": "object",
                "properties": map[string]any{
                    "id":         map[string]any{"type": "string"},
                    "triples":    map[string]any{"type": "array"},
                    "version":    map[string]any{"type": "integer"},
                    "updated_at": map[string]any{"type": "string", "format": "date-time"},
                },
            },
        },
    }
}
```

## Subject Naming Convention

All query subjects follow this pattern:

```text
graph.<component>.query.<operation>
```

Examples:

- `graph.ingest.query.entity` - Get entity from graph-ingest
- `graph.index.query.outgoing` - Get outgoing relationships from graph-index
- `graph.clustering.query.kcore` - Get k-core data from graph-clustering

Capabilities discovery uses:

```text
graph.<component>.capabilities
```

## Error Handling

Components return structured error responses:

```json
{
  "error": "not found"
}
```

Standard error messages:

| Error | Meaning |
|-------|---------|
| `"invalid request"` | Malformed JSON or missing required fields |
| `"not found"` | Requested entity or resource not found |
| `"internal error"` | Component-side failure |
| `"timeout"` | Operation exceeded time limit |
| `"storage not initialized"` | Component storage not ready |

## Discovery Workflow

### 1. Discover Available Components

Query each component's capabilities endpoint to discover what operations are available:

```bash
for component in ingest index clustering embedding; do
    echo "=== graph-$component ==="
    nats req graph.$component.capabilities '' 2>/dev/null || echo "Not available"
done
```

### 2. Inspect Operation Schemas

Parse the capabilities response to understand request/response formats:

- **Request Schema** - What fields are required/optional
- **Response Schema** - What data structure is returned
- **Definitions** - Shared type definitions for complex objects

### 3. Execute Queries

Use the discovered subject and schema information to construct valid queries:

```bash
# Discovered: graph.ingest.query.entity requires {"id": "..."}
nats req graph.ingest.query.entity '{"id":"my.entity.id"}'
```

### 4. Handle Graceful Degradation

If a component's capabilities query fails, the component is not available. Coordinators like
graph-query handle this by:

- Skipping unavailable components in aggregated responses
- Returning partial results when optional components are down
- Logging warnings rather than failing requests

## Architecture Pattern

The query discovery system follows a federated pattern:

```text
                    ┌─────────────────┐
                    │  Query Client   │
                    └────────┬────────┘
                             │
                    ┌────────▼────────┐
                    │   graph-query   │
                    │  (Coordinator)  │
                    └────────┬────────┘
                             │
        ┌────────┬───────────┼───────────┬────────┐
        ▼        ▼           ▼           ▼        ▼
   ┌────────┐ ┌────────┐ ┌────────┐ ┌────────┐ ┌────────┐
   │ ingest │ │ index  │ │cluster │ │spatial │ │embed   │
   └────────┘ └────────┘ └────────┘ └────────┘ └────────┘
```

> See [Full Diagram](../diagrams/query-discovery.mmd) for complete flow.

Key design principles:

- **Single-owner pattern** - Each component queries only the data it writes
- **Capability-based discovery** - Runtime discovery of available operations
- **Schema-based validation** - JSON Schema documents request/response contracts
- **Graceful degradation** - Optional components can be unavailable
- **Coordinator orchestration** - Complex queries coordinate across multiple components

## Use Cases

### Dynamic API Documentation

Generate API documentation from discovered capabilities:

```bash
# List all available query operations
for comp in ingest index clustering embedding; do
    nats req graph.$comp.capabilities '' | jq -r '.queries[].subject'
done
```

### Schema Validation

Validate requests before sending using the published JSON schemas:

```python
import json
import jsonschema

# Get capabilities
caps = nats_request("graph.ingest.capabilities", "")
entity_query = next(q for q in caps["queries"] if q["operation"] == "getEntity")

# Validate request
request = {"id": "my.entity.id"}
jsonschema.validate(request, entity_query["request_schema"])

# Send validated request
response = nats_request("graph.ingest.query.entity", json.dumps(request))
```

### Capability-Based Routing

Route queries to the appropriate component based on discovered capabilities:

```go
// Discover all components with "search" operations
for _, component := range []string{"ingest", "index", "clustering", "embedding"} {
    capsSubject := fmt.Sprintf("graph.%s.capabilities", component)
    caps := queryCapabilities(ctx, capsSubject)

    for _, query := range caps.Queries {
        if strings.Contains(query.Operation, "search") {
            // Route semantic searches to this component
            registerRoute("semantic-search", query.Subject)
        }
    }
}
```

## Related

- [Query Access](09-query-access.md) - GraphQL and MCP gateway access methods
- [PathRAG Pattern](08-pathrag-pattern.md) - Structural graph traversal
- [GraphRAG Pattern](07-graphrag-pattern.md) - Community-based search
- [Knowledge Graphs](02-knowledge-graphs.md) - The data model exposed by queries
