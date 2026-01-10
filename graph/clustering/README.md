# Clustering Package

Community detection and hierarchical clustering algorithms for the SemStreams entity graph.

## Purpose

This package provides graph clustering capabilities using Label Propagation Algorithm (LPA) and PageRank
to detect communities of related entities. Detected communities are enriched with statistical summaries
and optionally enhanced with LLM-generated descriptions.

## Key Types

### Community

Represents a detected cluster of related entities in the graph:

```go
type Community struct {
    ID                 string              // Unique community identifier
    Level              int                 // Hierarchy level (0=bottom, 1=mid, 2=top)
    Members            []string            // Entity IDs in this community
    ParentID           *string             // Parent community at next level (nil for top)
    StatisticalSummary string              // Fast baseline summary (always present)
    LLMSummary         string              // Enhanced description (populated async)
    Keywords           []string            // Key terms representing themes
    RepEntities        []string            // Representative entity IDs
    SummaryStatus      string              // "statistical", "llm-enhanced", or "llm-failed"
    Metadata           map[string]interface{} // Additional properties
}
```

**Note on field access**: This package uses **direct field access**, not getter methods. This is idiomatic
Go - simply reference `community.ID`, `community.Members`, etc.

### CommunityDetector

Interface for running community detection on the entity graph:

```go
type CommunityDetector interface {
    DetectCommunities(ctx context.Context) (map[int][]*Community, error)
    UpdateCommunities(ctx context.Context, entityIDs []string) error
    GetCommunity(ctx context.Context, id string) (*Community, error)
    GetEntityCommunity(ctx context.Context, entityID string, level int) (*Community, error)
    GetCommunitiesByLevel(ctx context.Context, level int) ([]*Community, error)
}
```

## Configuration

Configure clustering behavior using builder pattern methods:

```go
detector := clustering.NewLPADetector(provider, storage).
    WithMaxIterations(100).     // Maximum LPA iterations
    WithLevels(3).               // Hierarchy levels (0=bottom, 1=mid, 2=top)
    WithProgressiveSummarization(summarizer, entityProvider)
```

Available configuration methods:

- `WithMaxIterations(int)`: Set max iteration count (default: 100, max: 10000)
- `WithLevels(int)`: Set hierarchy depth (default: 3, max: 10)
- `WithProgressiveSummarization(CommunitySummarizer, EntityProvider)`: Enable summarization

## Usage Example

```go
// Create graph provider for specific entity types
provider := clustering.NewPredicateGraphProvider(queryManager, "robotics.drone")

// Create storage backend
storage := clustering.NewNATSStorage(js, "COMMUNITIES")

// Initialize detector
detector := clustering.NewLPADetector(provider, storage)

// Detect communities
communities, err := detector.DetectCommunities(ctx)
if err != nil {
    return fmt.Errorf("community detection failed: %w", err)
}

// Process results by hierarchy level
for level, comms := range communities {
    log.Printf("Level %d: %d communities", level, len(comms))
    for _, comm := range comms {
        // Direct field access - no getters needed
        log.Printf("  Community %s: %d members", comm.ID, len(comm.Members))
        log.Printf("    Keywords: %v", comm.Keywords)
        log.Printf("    Summary: %s", comm.StatisticalSummary)
    }
}
```

## Integration with QueryManager

The package defines local interfaces to avoid import cycles with `querymanager`:

```go
type RelationshipQuerier interface {
    EntityQuerier
    QueryRelationships(ctx context.Context, entityID string, direction Direction) ([]*Relationship, error)
    QueryByPredicate(ctx context.Context, predicate string) ([]string, error)
}
```

Two graph provider implementations are available:

### QueryManagerGraphProvider

Direct integration with QueryManager for neighborhood queries:

```go
provider := clustering.NewQueryManagerGraphProvider(queryManager)
```

**Limitation**: Does not support full graph scans (`GetAllEntityIDs` returns error).

### PredicateGraphProvider

Recommended for real-world use - clusters entities matching a specific predicate:

```go
// Cluster only drone entities
provider := clustering.NewPredicateGraphProvider(queryManager, "robotics.drone.type")
```

Caches the entity set at construction for efficient repeated queries.

## Algorithms

### Label Propagation (LPA)

Detects communities by iteratively propagating labels through the graph until convergence:

1. Each entity starts with unique label
2. Iteratively adopt most common neighbor label
3. Repeat until stable or max iterations reached

Produces bottom-level communities, which are then hierarchically aggregated.

### PageRank

Identifies representative entities within each community based on graph centrality. Representatives
best exemplify the community's characteristics.

### Summarization

Two-tier approach for community descriptions:

1. **Statistical Summary** (instant): TF-IDF keyword extraction + template-based generation
2. **LLM Enhancement** (async): Optional enrichment via background workers

Check `community.SummaryStatus` to determine which summary type is available.

## Storage

Communities are persisted in NATS KV buckets with configurable retention:

```go
storage := clustering.NewNATSStorage(js, "COMMUNITIES")
```

Supports incremental updates - only recompute affected communities on graph changes.

## Package Location

Previously located at `pkg/graphclustering/`, this package was moved to `processor/graph/clustering/`
per ADR-PACKAGE-RESPONSIBILITIES-CONSOLIDATION. The move eliminated import cycles and clarified that
clustering is graph processing logic, not a standalone reusable library.

All graph processing capabilities now live under `processor/graph/`:

- `processor/graph/` - Main processor and mutations
- `processor/graph/querymanager/` - Query execution
- `processor/graph/indexmanager/` - Indexing operations
- `processor/graph/clustering/` - Community detection (this package)
- `processor/graph/embedding/` - Vector embeddings
