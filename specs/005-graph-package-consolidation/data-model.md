# Data Model: Graph Package Consolidation

**Feature**: 005-graph-package-consolidation
**Date**: 2025-11-29

## Overview

This is a refactoring feature - no new data models are introduced. This document records the existing models being relocated and their relationships.

## Relocated Types

### Graphable Interface (message/ → graph/)

```go
// Current location: message/graphable.go
// New location: graph/graphable.go

// Graphable defines the contract for messages that can be represented
// as graph entities with semantic triples.
type Graphable interface {
    // EntityID returns the 6-part entity identifier
    EntityID() string

    // Triples returns semantic facts about this entity
    Triples() []Triple
}
```

**Rationale**: Graphable is a graph integration contract. Components implementing graph-compatible payloads naturally look to the `graph/` package for contracts.

### Community Struct (pkg/graphclustering/ → processor/graph/clustering/)

```go
// Current location: pkg/graphclustering/types.go
// New location: processor/graph/clustering/types.go

// Community represents a detected community in the graph
type Community struct {
    ID                 string
    Level              int
    Members            []string
    ParentID           *string
    Keywords           []string
    RepEntities        []string
    StatisticalSummary string
    LLMSummary         string
    SummaryStatus      string
    Metadata           map[string]interface{}
}

// NOTE: All getter methods (GetID(), GetLevel(), etc.) will be REMOVED
// Callers will use direct field access per Go idioms
```

**Rationale**:
- `pkg/` is for reusable library code; graphclustering has exactly 1 external consumer
- Moving inside `processor/graph/` eliminates the import cycle that necessitated `pkg/graphinterfaces`

### Embedder Interface (pkg/embedding/ → processor/graph/embedding/)

```go
// Current location: pkg/embedding/embedder.go
// New location: processor/graph/embedding/embedder.go

// Embedder defines the contract for text-to-vector embedding
type Embedder interface {
    // Embed converts text to a vector representation
    Embed(ctx context.Context, text string) ([]float32, error)

    // EmbedBatch converts multiple texts to vectors
    EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
}
```

**Rationale**:
- `pkg/` is for reusable library code; embedding has exactly 2 consumers in indexmanager
- Moving inside `processor/graph/` co-locates the code with its only consumers

## Deleted Types

### FederatedEntity (graph/federation.go)

```go
// DELETED - federation identity is in message.EntityID

// This struct duplicated information already in the 6-part EntityID format
type FederatedEntity struct {
    LocalID    string
    GlobalID   string
    PlatformID string
    Region     string
    MessageUID uuid.UUID
}
```

**Rationale**: EntityID already encodes federation (org.platform.domain.system.type.instance). Separate FederatedEntity created confusion about ID formats.

### Community Interface (pkg/graphinterfaces/community.go)

```go
// DELETED - Java-style anti-pattern

// This interface existed only to break import cycles
type Community interface {
    GetID() string
    GetLevel() int
    GetMembers() []string
    // ... 7 more getter methods
}
```

**Rationale**:
- Go interfaces should be small and behavior-focused
- Getters are not idiomatic Go
- Interface existed only to break import cycle (now resolved by moving graphclustering)

## Type Relationships

```text
graph/
├── EntityState          # Storage representation (unchanged)
├── Graphable           # Interface for graph-compatible payloads (MOVED HERE)
└── Triple              # Imported from message/ (unchanged)

message/
├── EntityID            # 6-part identity (unchanged)
├── Triple              # Semantic fact (unchanged)
└── FederationMeta      # Platform context (unchanged)

processor/graph/clustering/
└── Community           # Graph community (MOVED HERE, getters removed)

processor/graph/embedding/
└── Embedder            # Embedding interface (MOVED HERE)
```

## Migration Notes

1. **Graphable implementers** must update import: `message.Graphable` → `graph.Graphable`
2. **Community consumers** must update:
   - Import path: `pkg/graphclustering` → `processor/graph/clustering`
   - Method calls: `comm.GetID()` → `comm.ID`
3. **Embedding consumers** must update:
   - Import path: `pkg/embedding` → `processor/graph/embedding`
4. **No schema changes** - underlying data structures unchanged
