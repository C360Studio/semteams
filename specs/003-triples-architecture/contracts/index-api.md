# Index API Contract: OUTGOING_INDEX

**Feature**: 003-triples-architecture
**Component**: processor/graph/indexmanager
**Date**: 2025-11-27

## Overview

OUTGOING_INDEX provides forward relationship traversal from a source entity to its target entities, complementing the existing INCOMING_INDEX for reverse traversal.

## Interface

### OutgoingIndex

```go
// OutgoingIndex handles outgoing relationship indexing
type OutgoingIndex struct {
    bucket      jetstream.KeyValue
    kvStore     *natsclient.KVStore
    metrics     *InternalMetrics
    promMetrics *PrometheusMetrics
    logger      *slog.Logger
}

// Index interface implementation
func (idx *OutgoingIndex) HandleCreate(ctx context.Context, entityID string, entityState interface{}) error
func (idx *OutgoingIndex) HandleUpdate(ctx context.Context, entityID string, entityState interface{}) error
func (idx *OutgoingIndex) HandleDelete(ctx context.Context, entityID string) error

// Query methods
func (idx *OutgoingIndex) GetOutgoing(ctx context.Context, entityID string) ([]OutgoingEntry, error)
func (idx *OutgoingIndex) GetOutgoingByPredicate(ctx context.Context, entityID, predicate string) ([]OutgoingEntry, error)
```

### OutgoingEntry

```go
type OutgoingEntry struct {
    Predicate  string `json:"predicate"`
    ToEntityID string `json:"to_entity_id"`
}
```

## Operations

### GetOutgoing

**Purpose**: Retrieve all outgoing relationships for an entity

**Input**:

- `ctx`: Context for cancellation/timeout
- `entityID`: Source entity ID (6-part format)

**Output**:

- `[]OutgoingEntry`: List of outgoing relationships
- `error`: nil on success, wrapped error on failure

**Errors**:

- `ErrNotFound`: Entity has no outgoing relationships
- `ErrInvalidKey`: Invalid entity ID format
- Context errors: Cancellation or timeout

**Example**:

```go
entries, err := idx.GetOutgoing(ctx, "acme.telemetry.robotics.gcs1.drone.001")
if err != nil {
    return fmt.Errorf("get outgoing: %w", err)
}
for _, e := range entries {
    fmt.Printf("Relationship: %s → %s\n", e.Predicate, e.ToEntityID)
}
```

### GetOutgoingByPredicate

**Purpose**: Retrieve outgoing relationships filtered by predicate

**Input**:

- `ctx`: Context for cancellation/timeout
- `entityID`: Source entity ID
- `predicate`: Predicate to filter by (e.g., "ops.fleet.member_of")

**Output**:

- `[]OutgoingEntry`: Filtered list of outgoing relationships
- `error`: nil on success, wrapped error on failure

**Example**:

```go
entries, err := idx.GetOutgoingByPredicate(ctx, entityID, "ops.fleet.member_of")
```

## KV Storage

### Bucket Configuration

```go
bucketConfig := jetstream.KeyValueConfig{
    Bucket:      "OUTGOING_INDEX",
    Description: "Forward relationship traversal index",
    History:     1,  // Keep only latest value
    Storage:     jetstream.FileStorage,
}
```

### Key Format

```text
Key: {sanitizedEntityID}
Example: "acme.telemetry.robotics.gcs1.drone.001"
```

### Value Format

```json
[
  {"predicate": "ops.fleet.member_of", "to_entity_id": "acme.ops.logistics.hq.fleet.rescue"},
  {"predicate": "robotics.operator.controlled_by", "to_entity_id": "acme.platform.auth.main.user.alice"}
]
```

## Synchronization with INCOMING_INDEX

OUTGOING_INDEX and INCOMING_INDEX must be updated atomically to maintain consistency:

```go
func (m *Manager) updateRelationshipIndexes(ctx context.Context, entityID string, change EntityChange) error {
    // Extract relationship triples
    oldRels := extractRelationships(change.OldTriples)
    newRels := extractRelationships(change.NewTriples)

    // Compute diff
    added, removed := diffRelationships(oldRels, newRels)

    // Update both indexes
    if err := m.outgoingIndex.UpdateRelationships(ctx, entityID, added, removed); err != nil {
        return fmt.Errorf("update outgoing: %w", err)
    }

    if err := m.incomingIndex.UpdateRelationships(ctx, entityID, added, removed); err != nil {
        // TODO: Rollback outgoing? Or accept eventual consistency?
        return fmt.Errorf("update incoming: %w", err)
    }

    return nil
}
```

## Metrics

### Prometheus Metrics

```go
// Operations
outgoing_index_operations_total{operation="get",status="success|error"}
outgoing_index_operations_total{operation="update",status="success|error"}

// Latency
outgoing_index_operation_duration_seconds{operation="get|update"}

// Size
outgoing_index_entries_total  // Total entries across all keys
outgoing_index_keys_total     // Number of entity keys
```

### Internal Metrics

```go
type OutgoingIndexMetrics struct {
    GetCount      int64
    UpdateCount   int64
    GetLatencyMs  int64
    UpdateLatencyMs int64
    ErrorCount    int64
}
```

## Error Handling

All errors should be wrapped with context:

```go
if err != nil {
    return nil, fmt.Errorf("outgoing index get %s: %w", entityID, err)
}
```

Standard error types:

```go
var (
    ErrNotFound     = errors.New("entity not found in outgoing index")
    ErrInvalidKey   = errors.New("invalid entity ID format")
    ErrStorageError = errors.New("KV storage error")
)
```
