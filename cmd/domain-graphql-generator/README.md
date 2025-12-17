# domain-graphql-generator

Optional code generator for domain-specific GraphQL APIs.

> **Note:** This tool generates code from **GraphQL schemas** (`.graphql` files defining API types like `Robot`, `Fleet`). This is unrelated to **Component Schemas** (struct tags for config validation exported to `schemas/*.json`). See [Schema Concepts](#schema-concepts) below.

## When to Use This

SemStreams provides two GraphQL access patterns:

| Use Case | Recommended Approach |
|----------|---------------------|
| AI agents, MCP tools | Generic executor (built-in) |
| Exploratory tools, debugging | Generic executor |
| Production domain APIs | **This tool** (domain codegen) |
| Third-party API consumers | **This tool** (schema-as-contract) |
| Frontend type generation | **This tool** (enables client codegen) |

### Generic Executor (Default)

The built-in GraphQL executor (`gateway/graphql/executor.go`) works immediately with any SemStreams graph:

```graphql
# Generic API - returns Entity with triples
query {
  entity(id: "acme.logistics.fleet.robot.unit.bot-001") {
    id
    type
    triples { predicate, object }
  }
  semanticSearch(query: "low battery robots", limit: 10) {
    id
    triples { predicate, object }
  }
}
```

**Pros:** Zero configuration, works with any domain, ideal for exploration
**Cons:** No compile-time type safety, clients must understand triple structure

### Domain-Specific APIs (This Tool)

This code generator creates type-safe APIs for your domain:

```graphql
# Domain API - returns Robot with typed fields
query {
  robot(id: "bot-001") {
    id
    name
    batteryLevel
    currentTask { id, status }
    fleet { id, name }
  }
}
```

**Pros:** Compile-time type safety, self-documenting API, enables client codegen
**Cons:** Requires schema design and code generation step

## Quick Start

### 1. Define Your GraphQL Schema

```graphql
# schema.graphql
type Robot {
  id: ID!
  name: String!
  batteryLevel: Int!
  status: String!
  currentTask: Task
  fleet: Fleet!
}

type Task {
  id: ID!
  description: String!
  status: String!
  assignedRobot: Robot
}

type Fleet {
  id: ID!
  name: String!
  robots: [Robot!]!
}

type Query {
  robot(id: ID!): Robot
  robots(fleetId: ID!): [Robot!]!
  task(id: ID!): Task
  fleet(id: ID!): Fleet
}
```

### 2. Create Configuration

```json
{
  "package": "robotapi",
  "schema_path": "schema.graphql",
  "queries": {
    "robot": {
      "resolver": "QueryEntityByID",
      "subject": "graph.robot.get"
    },
    "robots": {
      "resolver": "QueryEntitiesByIDs",
      "subject": "graph.robot.list"
    }
  },
  "fields": {
    "Robot.id": { "property": "id", "type": "string" },
    "Robot.name": { "property": "properties.name", "type": "string" },
    "Robot.batteryLevel": { "property": "properties.battery", "type": "int" },
    "Robot.fleet": {
      "resolver": "QueryRelationships",
      "subject": "graph.robot.fleet"
    }
  },
  "types": {
    "Robot": { "entity_type": "robot" },
    "Task": { "entity_type": "task" },
    "Fleet": { "entity_type": "fleet" }
  }
}
```

### 3. Generate Code

```bash
domain-graphql-generator -config=graphql-config.json -output=generated
```

### 4. Integrate with Your Server

The generated code integrates with `gateway/graphql.BaseResolver`:

```go
import (
    "github.com/c360/semstreams/gateway/graphql"
    "myapp/generated/robotapi"
)

base := graphql.NewBaseResolver(natsClient)
resolver := robotapi.NewResolver(base)
// Use resolver with your GraphQL server
```

## Installation

```bash
go install github.com/c360/semstreams/cmd/domain-graphql-generator@latest
```

Or build from source:

```bash
cd cmd/domain-graphql-generator
go build .
```

## CLI Usage

```bash
domain-graphql-generator -config=graphql-config.json -output=generated
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-config` | `graphql-config.json` | Path to configuration file |
| `-output` | `generated` | Output directory for generated code |
| `-schema` | (from config) | Path to GraphQL schema (overrides config) |

## Configuration Reference

### Top-Level Fields

| Field | Required | Description |
|-------|----------|-------------|
| `package` | Yes | Go package name for generated code |
| `schema_path` | Yes | Path to GraphQL schema file |
| `queries` | Yes | Query resolver mappings |
| `fields` | Yes | Field resolver mappings |
| `types` | Yes | Entity type mappings |

### Query Configuration

```json
{
  "queries": {
    "robot": {
      "resolver": "QueryEntityByID",
      "subject": "graph.robot.get"
    }
  }
}
```

| Field | Description |
|-------|-------------|
| `resolver` | BaseResolver method name |
| `subject` | NATS subject for the query |

### Field Configuration

```json
{
  "fields": {
    "Robot.name": {
      "property": "properties.name",
      "type": "string"
    },
    "Robot.tasks": {
      "resolver": "QueryRelationships",
      "subject": "graph.robot.tasks"
    }
  }
}
```

| Field | Description |
|-------|-------------|
| `property` | Property path in Entity (e.g., `"properties.name"`) |
| `type` | Go type: `string`, `int`, `float64`, `bool`, `[]string`, `map[string]interface{}` |
| `resolver` | Optional: BaseResolver method for relationship fields |
| `subject` | Optional: NATS subject for relationship queries |
| `nullable` | Optional: Allow null values |

### Type Configuration

```json
{
  "types": {
    "Robot": { "entity_type": "robot" }
  }
}
```

| Field | Description |
|-------|-------------|
| `entity_type` | Entity type name in the graph |

## Supported BaseResolver Methods

### Entity Queries

| Method | Description |
|--------|-------------|
| `QueryEntityByID(ctx, id)` | Single entity by ID |
| `QueryEntityByAlias(ctx, aliasOrID)` | Entity by alias or ID |
| `QueryEntitiesByIDs(ctx, ids)` | Batch entity query |
| `QueryEntitiesByType(ctx, entityType, limit)` | Query by type |

### Relationship Queries

| Method | Description |
|--------|-------------|
| `QueryRelationships(ctx, filters)` | Relationship traversal |

### Search Queries

| Method | Description |
|--------|-------------|
| `SemanticSearch(ctx, query, limit)` | Semantic similarity search |

### Community Queries (GraphRAG)

| Method | Description |
|--------|-------------|
| `LocalSearch(ctx, entityID, query, level)` | Search within entity's community |
| `GlobalSearch(ctx, query, level, maxCommunities)` | Search across community summaries |
| `GetCommunity(ctx, communityID)` | Get community by ID |
| `GetEntityCommunity(ctx, entityID, level)` | Get entity's community at level |

## Generated Code Structure

The generator produces three files:

### resolver.go

Query resolvers delegating to `BaseResolver`:

```go
func (r *queryResolver) Robot(ctx context.Context, id string) (*Robot, error) {
    entity, err := r.base.QueryEntityByID(ctx, id)
    if err != nil {
        return nil, err
    }
    return entityToRobot(entity)
}
```

### models.go

Entity-to-GraphQL type converters:

```go
func entityToRobot(e *graphql.Entity) (*Robot, error) {
    if e == nil {
        return nil, nil
    }
    return &Robot{
        ID:           getID(e),
        Name:         getName(e),
        BatteryLevel: getBatteryLevel(e),
    }, nil
}
```

### converters.go

Property extraction helpers:

```go
func getName(e *graphql.Entity) string {
    return graphql.GetStringProp(e, "properties.name")
}
```

## Schema Concepts

SemStreams uses "schema" in two different contexts:

| Concept | Tool | Files | Purpose |
|---------|------|-------|---------|
| **Component Schema** | `cmd/component-schema-exporter` | `schemas/*.json` | Describes component config fields for UI editors and validation |
| **GraphQL Schema** | `cmd/domain-graphql-generator` | `.graphql` files | Describes GraphQL API types for type-safe queries |

### Component Schema (struct tags)

Used for self-describing component configuration:

```go
type Config struct {
    Directory string `json:"directory" schema:"type:string,description:Output directory"`
    MaxSize   int    `json:"max_size" schema:"type:int,min:1,max:1000"`
}
```

Exported to `schemas/*.json` via `cmd/component-schema-exporter` for UI form generation.

### GraphQL Schema (SDL files)

Used by this tool to generate type-safe API code:

```graphql
type Robot {
  id: ID!
  name: String!
  batteryLevel: Int!
}
```

**These are completely unrelated.** Component schemas describe configuration. GraphQL schemas describe API types.

## Example

See `testdata/` for a complete example:

- `testdata/robot-schema.graphql` - GraphQL schema
- `testdata/robot-config.json` - Configuration
- `testdata/generated/` - Generated code

### Running the Example

```bash
cd cmd/domain-graphql-generator
go run . -config=testdata/robot-config.json -output=testdata/generated
```

## GraphRAG Support

The generator supports GraphRAG (Graph Retrieval-Augmented Generation) with community detection:

```graphql
type Community {
  id: ID!
  level: Int!
  members: [String!]!
  summary: String
  keywords: [String!]
}

type Query {
  localSearch(entityID: ID!, query: String!, level: Int!): [Entity!]!
  globalSearch(query: String!, level: Int!, maxCommunities: Int!): [Entity!]!
  community(id: ID!): Community
}
```

```json
{
  "queries": {
    "localSearch": { "resolver": "LocalSearch" },
    "globalSearch": { "resolver": "GlobalSearch" },
    "community": { "resolver": "GetCommunity" }
  }
}
```

## Testing

```bash
go test ./cmd/domain-graphql-generator/...
```

## Related Documentation

- [Domain-Specific GraphQL](../../docs/advanced/04-domain-graphql.md) - Comprehensive guide
- [Architecture](../../docs/basics/02-architecture.md) - System overview
- [Configuration](../../docs/reference/configuration.md) - Full config reference
