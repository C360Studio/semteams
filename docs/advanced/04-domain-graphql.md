# Domain-Specific GraphQL APIs

SemStreams provides two GraphQL access patterns. This guide covers the domain-specific approach using `domain-graphql-generator` code generation for type-safe APIs.

## Overview

### Two GraphQL Patterns

| Pattern | Tool | Schema | Type Safety | Setup |
|---------|------|--------|-------------|-------|
| **Generic** | Built-in executor | Hardcoded | Runtime | Zero config |
| **Domain** | `domain-graphql-generator` | Custom `.graphql` | Compile-time | Code generation |

### When to Use Domain-Specific APIs

Use the generic executor when:
- Building AI agents or MCP tools
- Exploring or debugging the graph
- Prototyping quickly

Use domain-specific APIs (this guide) when:
- Building production applications with known types
- Exposing APIs to third-party consumers
- Generating frontend TypeScript types
- Requiring compile-time type safety

## Architecture

```text
┌─────────────────────────────────────────────────────────────┐
│                    GraphQL Access Patterns                   │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  Domain-Specific (domain-graphql-generator)  Generic (executor) │
│  ┌─────────────────────────┐      ┌─────────────────────┐  │
│  │ type Robot {            │      │ type Entity {       │  │
│  │   id: ID!               │      │   id: ID!           │  │
│  │   name: String!         │      │   type: String!     │  │
│  │   battery: Int!         │      │   triples: [Triple] │  │
│  │   fleet: Fleet          │      │ }                   │  │
│  │ }                       │      │                     │  │
│  └─────────────────────────┘      └─────────────────────┘  │
│           ↓                              ↓                  │
│  Production Apps              AI Agents, MCP, Exploration   │
│  Type-safe clients            Universal graph access        │
│                                                             │
└─────────────────────────────────────────────────────────────┘
                              ↓
                    gateway/graphql.BaseResolver
                    (shared query implementation)
```

Both patterns use the same underlying `BaseResolver` for data access. The difference is whether the API exposes generic `Entity` types or domain-specific types like `Robot`.

## Prerequisites

Before using domain-specific GraphQL:

1. **Working SemStreams deployment** with entities in NATS KV
2. **Defined domain vocabulary** (entity types, predicates)
3. **Understanding of your entity structure** (what properties exist)

## Step-by-Step Guide

### Step 1: Design Your GraphQL Schema

Start by defining the types your API will expose:

```graphql
# schema.graphql

"""A robot in the fleet management system."""
type Robot {
  id: ID!
  name: String!
  model: String!
  batteryLevel: Int!
  status: RobotStatus!
  location: Location
  currentTask: Task
  fleet: Fleet!
  completedTasks: [Task!]!
}

"""Task assigned to a robot."""
type Task {
  id: ID!
  description: String!
  priority: Int!
  status: TaskStatus!
  assignedRobot: Robot
  createdAt: String!
}

"""Fleet of robots."""
type Fleet {
  id: ID!
  name: String!
  region: String!
  robots: [Robot!]!
  activeTasks: [Task!]!
}

"""Geographic location."""
type Location {
  latitude: Float!
  longitude: Float!
  zone: String
}

enum RobotStatus {
  IDLE
  WORKING
  CHARGING
  OFFLINE
}

enum TaskStatus {
  PENDING
  IN_PROGRESS
  COMPLETED
  FAILED
}

type Query {
  """Get a robot by ID."""
  robot(id: ID!): Robot

  """List robots in a fleet."""
  robotsByFleet(fleetId: ID!): [Robot!]!

  """Get a task by ID."""
  task(id: ID!): Task

  """Get a fleet by ID."""
  fleet(id: ID!): Fleet

  """Search robots by status."""
  robotsByStatus(status: RobotStatus!): [Robot!]!

  """Semantic search across all entities."""
  search(query: String!, limit: Int!): [Robot!]!
}
```

### Step 2: Map Schema to Entity Structure

Create a configuration file that maps GraphQL types to your entity structure:

```json
{
  "package": "robotapi",
  "schema_path": "schema.graphql",

  "queries": {
    "robot": {
      "resolver": "QueryEntityByID",
      "subject": "graph.robot.get"
    },
    "robotsByFleet": {
      "resolver": "QueryRelationships",
      "subject": "graph.fleet.robots"
    },
    "task": {
      "resolver": "QueryEntityByID",
      "subject": "graph.task.get"
    },
    "fleet": {
      "resolver": "QueryEntityByID",
      "subject": "graph.fleet.get"
    },
    "robotsByStatus": {
      "resolver": "QueryEntitiesByType",
      "subject": "graph.robot.bystatus"
    },
    "search": {
      "resolver": "SemanticSearch",
      "subject": "graph.search"
    }
  },

  "fields": {
    "Robot.id": { "property": "id", "type": "string" },
    "Robot.name": { "property": "properties.name", "type": "string" },
    "Robot.model": { "property": "properties.model", "type": "string" },
    "Robot.batteryLevel": { "property": "properties.battery", "type": "int" },
    "Robot.status": { "property": "properties.status", "type": "string" },
    "Robot.currentTask": {
      "resolver": "QueryRelationships",
      "subject": "graph.robot.task"
    },
    "Robot.fleet": {
      "resolver": "QueryRelationships",
      "subject": "graph.robot.fleet"
    },
    "Robot.completedTasks": {
      "resolver": "QueryRelationships",
      "subject": "graph.robot.completed"
    },

    "Task.id": { "property": "id", "type": "string" },
    "Task.description": { "property": "properties.description", "type": "string" },
    "Task.priority": { "property": "properties.priority", "type": "int" },
    "Task.status": { "property": "properties.status", "type": "string" },
    "Task.assignedRobot": {
      "resolver": "QueryRelationships",
      "subject": "graph.task.robot"
    },
    "Task.createdAt": { "property": "properties.created_at", "type": "string" },

    "Fleet.id": { "property": "id", "type": "string" },
    "Fleet.name": { "property": "properties.name", "type": "string" },
    "Fleet.region": { "property": "properties.region", "type": "string" },
    "Fleet.robots": {
      "resolver": "QueryRelationships",
      "subject": "graph.fleet.robots"
    },
    "Fleet.activeTasks": {
      "resolver": "QueryRelationships",
      "subject": "graph.fleet.tasks"
    },

    "Location.latitude": { "property": "properties.lat", "type": "float64" },
    "Location.longitude": { "property": "properties.lng", "type": "float64" },
    "Location.zone": { "property": "properties.zone", "type": "string", "nullable": true }
  },

  "types": {
    "Robot": { "entity_type": "robot" },
    "Task": { "entity_type": "task" },
    "Fleet": { "entity_type": "fleet" },
    "Location": { "entity_type": "location" }
  }
}
```

### Step 3: Generate Resolver Code

Run the code generator:

```bash
domain-graphql-generator -config=robotapi-config.json -output=internal/api/robotapi
```

This generates:
- `resolver.go` - Query resolvers
- `models.go` - Type converters
- `converters.go` - Property accessors

### Step 4: Integrate with Your Server

```go
package main

import (
    "net/http"

    "github.com/99designs/gqlgen/graphql/handler"
    "github.com/c360/semstreams/gateway/graphql"
    "myapp/internal/api/robotapi"
)

func main() {
    // Create NATS client
    nc, _ := nats.Connect("nats://localhost:4222")

    // Create base resolver (shared infrastructure)
    base := graphql.NewBaseResolver(nc)

    // Create domain-specific resolver
    resolver := robotapi.NewResolver(base)

    // Create GraphQL handler
    srv := handler.NewDefaultServer(robotapi.NewExecutableSchema(
        robotapi.Config{Resolvers: resolver},
    ))

    http.Handle("/graphql", srv)
    http.ListenAndServe(":8080", nil)
}
```

## Property Mapping

### Simple Properties

Map scalar values from entity properties:

```json
{
  "Robot.name": {
    "property": "properties.name",
    "type": "string"
  }
}
```

Supported types: `string`, `int`, `float64`, `bool`, `[]string`, `map[string]interface{}`

### Nested Properties

Use dot notation for nested paths:

```json
{
  "Robot.location": {
    "property": "properties.location.coordinates",
    "type": "[]float64"
  }
}
```

### Relationship Fields

For fields that traverse relationships:

```json
{
  "Robot.fleet": {
    "resolver": "QueryRelationships",
    "subject": "graph.robot.fleet"
  }
}
```

The resolver fetches related entities via the outgoing index.

### Nullable Fields

Mark optional fields:

```json
{
  "Robot.currentTask": {
    "resolver": "QueryRelationships",
    "subject": "graph.robot.task",
    "nullable": true
  }
}
```

## GraphRAG Integration

Domain APIs can expose community-based search:

```graphql
type Community {
  id: ID!
  level: Int!
  memberCount: Int!
  summary: String
  keywords: [String!]!
}

type Query {
  # Local search within entity's community
  localSearch(entityID: ID!, query: String!, level: Int!): [Robot!]!

  # Global search across community summaries
  globalSearch(query: String!, level: Int!, maxCommunities: Int!): [Robot!]!

  # Get community details
  community(id: ID!): Community
}
```

Configuration:

```json
{
  "queries": {
    "localSearch": {
      "resolver": "LocalSearch",
      "subject": "graph.search.local"
    },
    "globalSearch": {
      "resolver": "GlobalSearch",
      "subject": "graph.search.global"
    },
    "community": {
      "resolver": "GetCommunity",
      "subject": "graph.community.get"
    }
  }
}
```

## Best Practices

### Schema Design

1. **Start with use cases**: Design types based on what clients need, not internal structure
2. **Use meaningful names**: `batteryLevel` not `bat_lvl`
3. **Document everything**: GraphQL descriptions become API docs
4. **Keep it simple**: Don't expose internal complexity

### Configuration

1. **Match entity structure**: Property paths must match actual entity data
2. **Test incrementally**: Generate and test one type at a time
3. **Version carefully**: Schema changes affect all clients

### Performance

1. **Batch where possible**: Use `QueryEntitiesByIDs` for lists
2. **Limit depth**: Deep relationship chains can be slow
3. **Use indexes**: Ensure predicates used in queries are indexed

## Troubleshooting

### Generated Code Won't Compile

1. Check property paths match actual entity structure
2. Verify type mappings are correct
3. Ensure all referenced types are configured

### Queries Return Null

1. Verify entity exists in `ENTITY_STATES`
2. Check property paths in configuration
3. Confirm indexes are populated

### Relationship Fields Empty

1. Check outgoing relationships exist
2. Verify NATS subject is correct
3. Ensure relationship predicates use entity IDs as objects

## Comparison with Generic Executor

| Aspect | Domain-Specific | Generic |
|--------|-----------------|---------|
| Type safety | Compile-time | Runtime |
| Schema | Custom per domain | Fixed |
| Setup effort | Medium (schema + config) | Zero |
| Client codegen | Yes (graphql-codegen) | Manual |
| API documentation | Self-documenting | Generic |
| Flexibility | Fixed types | Any entity |

Choose domain-specific when you need type safety and stable contracts. Choose generic when you need flexibility and zero configuration.

## Related Documentation

- [domain-graphql-generator README](../../cmd/domain-graphql-generator/README.md) - CLI reference
- [Architecture](../basics/02-architecture.md) - System overview
- [LLM Enhancement](02-llm-enhancement.md) - Community summaries
- [Clustering](01-clustering.md) - Community detection
