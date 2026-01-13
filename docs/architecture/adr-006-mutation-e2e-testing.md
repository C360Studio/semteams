# ADR-006: Mutation E2E Testing Strategy

## Status

Proposed

## Context

SemStreams E2E tests validate the tiered processing pipeline (core → structural → statistical → semantic). Current test coverage focuses on:

- Entity ingestion and retrieval
- Relationship indexing
- Community detection
- Search functionality (PathRAG, GraphRAG, similarity)

However, **mutation operations** (edit/delete) are only tested **indirectly** through the rules engine:

| Operation | API Available | Unit Tests | E2E Direct | E2E Indirect |
|-----------|--------------|------------|------------|--------------|
| AddTriple | Yes | Yes | No | Yes (via rules) |
| RemoveTriple | Yes | Yes | No | Yes (via rules) |
| CreateRelationship | Yes | Yes | No | No |
| DeleteRelationship | Yes | Yes | No | No |

### Current Indirect Testing

The structural tier configuration includes rules that fire `add_triple` and `remove_triple` actions:

```json
{
  "on_enter": [{"type": "add_triple", "predicate": "alert.active", "value": "true"}],
  "on_exit": [{"type": "remove_triple", "predicate": "alert.active"}]
}
```

E2E tests validate that `OnEnterFired` and `OnExitFired` counts are correct, but don't verify:
- The actual triple was added/removed
- Index consistency after mutation
- Error handling for invalid mutations

### Gap Analysis

1. **No direct API mutation tests**: NATS request to `graph.mutation.triple.add` never tested
2. **No relationship mutation tests**: Create/Delete relationship APIs untested in E2E
3. **No index verification**: OUTGOING_INDEX, INCOMING_INDEX updates not validated
4. **No error case coverage**: Invalid entity IDs, timeouts, etc.

## Decision

Extend E2E test infrastructure for explicit mutation coverage:

### Test Client Extensions

Add mutation methods to `test/e2e/client/nats.go`:

```go
// Mutation operations
func (c *NATSClient) AddTriple(ctx context.Context, subject, predicate, object string) error
func (c *NATSClient) RemoveTriple(ctx context.Context, subject, predicate string) error
func (c *NATSClient) CreateRelationship(ctx context.Context, from, to, predicate string) error
func (c *NATSClient) DeleteRelationship(ctx context.Context, from, to, predicate string) error
```

### Validation Methods

Add index query methods to verify mutation effects:

```go
// Index verification
func (c *NATSValidationClient) GetOutgoingIndex(ctx context.Context, entityID string) ([]OutgoingEntry, error)
func (c *NATSValidationClient) GetIncomingIndex(ctx context.Context, entityID string) ([]IncomingEntry, error)
func (c *NATSValidationClient) GetPredicateIndex(ctx context.Context, predicate string) ([]PredicateIndexEntry, error)
```

### Test Scenarios

#### Scenario 1: Direct Mutation Tests

```go
// 1. Create entity via normal ingestion
// 2. Add triple via mutation API
// 3. Verify triple exists in entity
// 4. Remove triple via mutation API
// 5. Verify triple removed from entity
```

#### Scenario 2: Index Consistency

```go
// 1. Create two entities A and B
// 2. Create relationship A -> B with predicate "related_to"
// 3. Verify A's OUTGOING_INDEX contains {predicate: "related_to", to: B}
// 4. Verify B's INCOMING_INDEX contains A
// 5. Delete relationship
// 6. Verify indexes updated
```

#### Scenario 3: Error Handling

```go
// 1. Attempt to create relationship with non-existent target
// 2. Verify appropriate error returned
// 3. Verify no orphan index entries created
```

### Integration with Existing Tiers

Add mutation tests to the structural tier (where rules already exist):

| Test | Validates |
|------|-----------|
| Rule-triggered mutation | OnEnter adds triple, verify via GetEntity |
| Rule-triggered removal | OnExit removes triple, verify via GetEntity |
| Direct API mutation | Bypass rules, verify direct API path |
| Index consistency | After mutation, verify all indexes updated |

### Test Execution

Mutation tests should run in the structural tier since:
- Rules infrastructure already present
- Index processor available
- Minimal additional setup required

```bash
task e2e:structural  # Now includes mutation validation
```

## Consequences

### Positive

- **Explicit Coverage**: Mutation APIs directly tested, not just inferred from side effects
- **Index Verification**: Confirms indexes stay consistent after mutations
- **Error Handling**: Validates graceful handling of invalid operations
- **Regression Prevention**: Catches mutation bugs that indirect tests might miss

### Negative

- **Test Complexity**: More setup required for mutation scenarios
- **Execution Time**: Additional test scenarios add to E2E duration
- **State Management**: Mutation tests need careful isolation to avoid interference

### Neutral

- **Client Extension**: NATSClient grows but maintains consistent patterns
- **Documentation**: Test scenarios document expected mutation behavior

## Implementation Plan

### Phase 1: Client Extensions
- Add mutation methods to `test/e2e/client/nats.go`
- Add validation methods for index queries

### Phase 2: Basic Mutation Tests
- Test AddTriple/RemoveTriple via API
- Verify entity state changes

### Phase 3: Index Consistency Tests
- Test relationship creation
- Verify incoming/outgoing indexes

### Phase 4: Error Handling Tests
- Invalid entity targets
- Timeout handling

## Key Files

| File | Change |
|------|--------|
| `test/e2e/client/nats.go` | Add mutation and validation methods |
| `test/e2e/scenarios/stages/mutations.go` | New: Mutation test stage |
| `test/e2e/scenarios/tiered_structural.go` | Add mutation validation |
| `processor/graph-ingest/mutations.go` | Reference: mutation handlers |

## References

- [E2E Testing Guide](../contributing/02-e2e-tests.md)
- [ADR-002: Query Capability Discovery](./adr-002-query-capability-discovery.md)
