# Graph Types README

**Last Updated**: 2024-08-29  
**Maintainer**: SemStreams Core Team

## Purpose & Scope

**What this component does**: Provides core type definitions for entity state storage in the graph system, including triples-based entity state and relationship edges.

**Key responsibilities**:
- Define EntityState structure for triple-based entity graph representation
- Define Edge structure for directed relationships between entities
- Support graph storage operations and relationship management
- Provide entity identity via 6-part EntityID format

**NOT responsible for**: Graph storage implementation, NATS operations, entity processing logic, semantic classification or status (domain responsibility)

## Design Decisions

### Architectural Choices
- **Outgoing edges only**: EntityState stores only outgoing relationships
  - **Rationale**: Avoids bi-directional storage complexity and inconsistency issues
  - **Trade-offs**: Requires reverse index for incoming edge queries vs simpler storage
  - **Alternatives considered**: Bi-directional storage (rejected due to consistency issues)

- **Explicit versioning**: EntityState includes version field for conflict resolution
  - **Rationale**: Enables optimistic locking and conflict detection in concurrent updates
  - **Trade-offs**: Additional storage overhead vs data consistency guarantees
  - **Alternatives considered**: Timestamp-based versioning (rejected for precision issues)

- **StorageRef pattern**: Optional storage reference with full metadata
  - **Rationale**: Supports "store once, reference anywhere" with complete storage metadata
  - **Trade-offs**: Additional lookup for full context vs optimized query performance
  - **Alternatives considered**: Embedding full message data (rejected for storage efficiency)

### API Contracts
- **Edge replacement**: AddEdge() replaces existing edges of same type to same entity
  - **Example**: Adding "NEAR" relationship twice updates distance, doesn't duplicate
  - **Enforcement**: AddEdge() method checks existing edges before adding
  - **Exceptions**: Different edge types to same entity are allowed (NEAR + POWERED_BY)

- **Expiration handling**: Edges with ExpiresAt are automatically filtered
  - **Example**: NEAR relationships expire after 5 minutes if not refreshed
  - **Enforcement**: RemoveExpiredEdges() method called during processing
  - **Exceptions**: Permanent relationships have nil ExpiresAt

### Anti-Patterns to Avoid
- **Storing incoming edges**: Only store outgoing edges, use reverse index for incoming
- **Manual version management**: Always increment version on EntityState updates
- **Hardcoding status**: Status is a domain concern, emit as domain-specific triples
- **Framework classification**: Semantic classification belongs in domain logic, not framework

## Architecture Context

### Integration Points
- **Consumes from**: EntityExtractor via message processing pipeline
- **Provides to**: GraphProcessor for NATS KV storage operations
- **External dependencies**: ObjectStore for complete message references

### Data Flow
```
Message → EntityExtractor → EntityState → GraphProcessor → NATS KV Storage
                                       ↘ Edge → Relationship indices
```

### Configuration
No direct configuration - types are used by GraphProcessor with NATS KV configuration.

## Critical Behaviors (Testing Focus)

### Happy Path - What Should Work
1. **EntityState edge management**: Add, update, and remove relationships
   - **Input**: EntityState with edges, call AddEdge() with new relationship
   - **Expected**: Edge added or existing edge of same type updated
   - **Verification**: Edges slice contains expected edge with correct properties

2. **Edge expiration cleanup**: Remove expired relationships automatically
   - **Input**: EntityState with expired edges (ExpiresAt < now)
   - **Expected**: RemoveExpiredEdges() removes expired, keeps current edges
   - **Verification**: Only non-expired edges remain in Edges slice

3. **Entity ID access**: Direct access to 6-part entity identifier
   - **Input**: EntityState with valid ID
   - **Expected**: state.ID returns complete identifier
   - **Verification**: ID format: org.platform.domain.system.type.instance

### Error Conditions - What Should Fail Gracefully
1. **Invalid EntityID format**: Malformed entity identifiers
   - **Trigger**: Entity ID without 6-part format (org.platform.domain.system.type.instance)
   - **Expected**: message.ParseEntityID() returns error
   - **Recovery**: Validate entity IDs before storage, reject malformed identifiers

2. **Nil StorageRef access**: Accessing storage reference when not set
   - **Trigger**: Accessing state.StorageRef fields when StorageRef is nil
   - **Expected**: Nil pointer dereference if not checked
   - **Recovery**: Always check `if state.StorageRef != nil` before access

### Edge Cases - Boundary Conditions
- **Empty edges slice**: Edge operations on EntityState with no existing edges
- **Duplicate edge types**: Multiple edges of same type to same entity
- **Simultaneous expiration**: Multiple edges expiring at exact same time

## Common Patterns

### Standard Implementation Patterns
- **Entity state creation**: Initialize with version 1 and current timestamp
  - **When to use**: Creating new EntityState from message extraction
  - **Implementation**: `EntityState{ID: entityID, Version: 1, UpdatedAt: time.Now(), ...}`
  - **Pitfalls**: Forgetting to set Version, UpdatedAt, or using invalid EntityID format

- **Edge lifecycle management**: Use expiration for temporary relationships
  - **When to use**: Proximity relationships, temporary operational states
  - **Implementation**: `Edge{ExpiresAt: &time.Time{}, ...}` for temporary edges
  - **Pitfalls**: Not calling RemoveExpiredEdges() regularly

- **Domain status as triples**: Emit status as domain-specific semantic facts
  - **When to use**: Any time domain logic determines entity status
  - **Implementation**: `Triple{Predicate: "domain.type.status", Object: "value"}`
  - **Pitfalls**: Trying to use non-existent EntityStatus enum (removed)

### Optional Feature Patterns
- **Spatial data**: Use geo.location.* triples for position/spatial relationships
- **Storage references**: Set StorageRef when original message is in ObjectStore
- **Triple optimization**: Focus triples on queryable semantic facts

### Integration Patterns
- **Graph storage**: GraphProcessor uses these types for NATS KV operations
- **Relationship indexing**: Edge data used to build reverse relationship indices
- **Context retrieval**: ObjectRef used to fetch complete message context

## Usage Patterns

### Typical Usage (How Other Code Uses This)
```go
// Create entity state with new structure (as of 004-semantic-refactor)
state := &gtypes.EntityState{
    ID: "c360.semstreams.robotics.drone.mavlink.drone_001",
    Triples: []message.Triple{
        {
            Subject:   "c360.semstreams.robotics.drone.mavlink.drone_001",
            Predicate: "robotics.drone.battery_level",
            Object:    85.0,
            Source:    "battery_monitor",
            Timestamp: time.Now(),
        },
        {
            Subject:   "c360.semstreams.robotics.drone.mavlink.drone_001",
            Predicate: "robotics.drone.status",
            Object:    "armed",
            Source:    "flight_controller",
            Timestamp: time.Now(),
        },
    },
    StorageRef: &message.StorageReference{
        StorageInstance: "semstreams-objects",
        Key:             "robotics.battery.v1:20240315-103045:drone_001",
        ContentType:     "application/json",
        Size:            1024,
    },
    MessageType: message.Type{
        Domain:   "robotics",
        Category: "mavlink",
        Version:  "v1",
    },
    Version:   1,
    UpdatedAt: time.Now(),
}

// Add relationship
powerEdge := gtypes.Edge{
    ToEntityID: "c360.semstreams.robotics.battery.lithium.battery_001",
    EdgeType:   "POWERED_BY",
    Weight:     1.0,
    Confidence: 0.95,
    CreatedAt:  time.Now(),
}
state.AddEdge(powerEdge)

// Clean up expired edges
state.RemoveExpiredEdges()

// Parse entity type from ID when needed
eid, _ := message.ParseEntityID(state.ID)
entityType := eid.Type  // "mavlink"
```

### Common Integration Patterns
- **Storage operations**: GraphProcessor serializes EntityState to NATS KV
- **Relationship queries**: Edge data used to build relationship graphs
- **Status monitoring**: EntityStatus used for health checks and alerting

## Testing Strategy

### Test Categories
1. **Unit Tests**: Individual type methods (AddEdge, RemoveExpiredEdges, etc.)
2. **State Management Tests**: EntityState lifecycle operations
3. **Serialization Tests**: JSON marshaling/unmarshaling of all types

### Test Quality Standards
- ✅ Tests MUST verify actual state changes (not just method calls)
- ✅ Tests MUST cover edge expiration and cleanup behavior
- ✅ Tests MUST validate EntityStatus enumeration values
- ❌ NO tests that don't verify state mutations
- ❌ NO tests that ignore time-based behavior (expiration)

### Mock vs Real Dependencies
- **Use real types for**: All graph type testing (no external dependencies)
- **Use mocks for**: Not applicable (pure value types)
- **Testcontainers for**: Not applicable (no external services)

## Implementation Notes

### Thread Safety
- **Concurrency model**: Value types, no inherent thread safety required
- **Shared state**: EntityState mutations not thread-safe, callers must synchronize
- **Critical sections**: Edge slice operations in concurrent environments

### Performance Considerations
- **Expected throughput**: High-frequency entity updates in graph processing
- **Memory usage**: Properties map and Edges slice can grow with entity complexity
- **Bottlenecks**: Edge removal with linear search through Edges slice

### Error Handling Philosophy
- **Error propagation**: Methods use boolean returns for success/failure
- **Retry strategy**: Not applicable (pure data types)
- **Circuit breaking**: Not applicable

## Troubleshooting

### Investigation Workflow
1. **First steps** when debugging entity state issues:
   - **Check version consistency**: Verify version increments on updates
   - **Validate edge state**: Use RemoveExpiredEdges() to clean stale relationships
   - **Verify property updates**: Check Properties map for expected key-value pairs
   - **Inspect ObjectRef**: Ensure reference points to valid ObjectStore entry

2. **Common debugging commands**:
   ```bash
   # Find EntityState usage patterns
   rg "EntityState" --type go -A 5
   
   # Check edge management
   rg "AddEdge|RemoveEdge" --type go
   
   # Verify status usage
   rg "Status\w+" pkg/types/graph/ --type go
   ```

### Decision Trees for Common Issues

#### When entity updates fail:
```
Entity state updates not persisting
├── Version conflict? → Check version increment on updates
├── Edge not adding? → Verify AddEdge() logic for duplicates
├── Properties not updating? → Check UpdateProperties() merge logic
└── Status not changing? → Verify EntityStatus.IsValid()
```

#### When relationships missing:
```
Expected relationships not found
├── Edge expired? → Call RemoveExpiredEdges(), check ExpiresAt
├── Wrong direction? → Remember only outgoing edges stored
├── Duplicate type? → AddEdge() replaces existing edges of same type
└── Index out of sync? → Rebuild relationship indices
```

### Common Issues
1. **Edge duplication**: Multiple edges of same type to same entity
   - **Cause**: Not using AddEdge() method, manually appending to Edges slice
   - **Investigation**: Check Edges slice for duplicate entries
   - **Solution**: Always use AddEdge() which handles replacement logic
   - **Prevention**: Use AddEdge() method exclusively for edge management

2. **Stale relationships**: Expired edges not cleaned up
   - **Cause**: RemoveExpiredEdges() not called during processing
   - **Investigation**: Check ExpiresAt timestamps on edges
   - **Solution**: Call RemoveExpiredEdges() regularly in processing pipeline
   - **Prevention**: Include expiration cleanup in standard processing flow

### Debug Information
- **Logs to check**: GraphProcessor logs for entity state operations
- **Metrics to monitor**: Entity state storage and retrieval rates
- **Health checks**: EntityStatus distribution across entities
- **Config validation**: Properties map size and content patterns
- **Integration verification**: ObjectRef validity and accessibility

## 004-semantic-refactor: EntityState Simplification (COMPLETED)

### **Breaking Changes Applied**
- **NodeProperties deleted**: ID promoted to top-level field on EntityState
- **EntityStatus removed**: Status is now domain responsibility via triples
- **Position removed**: Spatial data now stored as geo.location.* triples
- **ObjectRef replaced**: Now StorageRef with full *message.StorageReference metadata
- **MessageType typed**: Changed from string to message.Type struct

### New EntityState Structure
```go
type EntityState struct {
    ID          string                   `json:"id"`
    Triples     []message.Triple         `json:"triples"`
    StorageRef  *message.StorageReference `json:"storage_ref,omitempty"`
    MessageType message.Type             `json:"message_type"`
    Version     uint64                   `json:"version"`
    UpdatedAt   time.Time                `json:"updated_at"`
}
```

### Migration Patterns
- `state.Node.ID` → `state.ID`
- `state.Node.Type` → `message.ParseEntityID(state.ID).Type`
- `state.ObjectRef` → `state.StorageRef.Key` (check nil first)
- Status → Domain-specific triples with domain predicates
- Position → geo.location.latitude/longitude/altitude triples

See [004-semantic-refactor quickstart](../../../specs/004-semantic-refactor/quickstart.md) for complete migration guide.

## Related Documentation
- [SEMSTREAMS_NATS_ARCHITECTURE.md](../../../docs/architecture/SEMSTREAMS_NATS_ARCHITECTURE.md) - Graph storage patterns
- [Message types](../../message/README.md) - Entity type definitions
- [GraphProcessor](../../processor/graph/) - Graph processing implementation