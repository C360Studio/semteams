# SPEC: Enforce Semantic Contract via Graphable Interface

**Status:** Proposed
**Author:** Documentation Audit
**Date:** 2024-11-24

---

## Problem Statement

SemStreams has an identity crisis. It claims:

> "The 'Sem' in SemStreams is the semantic graph - it's not an optional add-on"

But the graph processor accepts non-Graphable data and creates low-quality entities from it via fallback paths.

### Current Behavior

The graph processor (`processor/graph/messagemanager/processor.go`) has multiple paths:

1. **Graphable path** (proper): Payloads implementing `Graphable` get full semantic processing
2. **Legacy path** (fallback): Payloads with just `EntityID()` method create degraded entities
3. **Map path** (fallback): Raw `map[string]any` creates minimal entities

The fallback paths create entities with:
- `confidence: 0.5`
- `source: "legacy_interface"` or `"map_message"`
- No triples, no relationships
- Default `entity_class: "Thing"`

This means PathRAG and GraphRAG work poorly on data that didn't come through Graphable.

### The Core Issue

SemStreams uses **interfaces as data contracts** - this is standard Go practice. The Graphable interface IS the semantic contract:

```go
type Graphable interface {
    EntityID() string      // Deterministic 6-part federated identifier
    Triples() []Triple     // Semantic facts about the entity
}
```

If a payload implements Graphable, it belongs in the graph. If it doesn't, it doesn't.

The fallback paths violate this principle by accepting non-Graphable data.

---

## Proposed Solution

**Remove fallback paths. Require Graphable interface.**

The graph processor should only accept payloads that implement the `Graphable` interface. Period.

---

## Detailed Changes

### 1. Graph Processor Changes

**File:** `processor/graph/messagemanager/processor.go`

#### Remove Fallback Paths

```go
// REMOVE: processNonGraphableMessage()
// REMOVE: processMapMessage()
```

#### Simplify ProcessMessage

```go
func (mp *Manager) ProcessMessage(ctx context.Context, msg any) ([]*gtypes.EntityState, error) {
    // Check for Storable (Graphable + storage reference)
    if storable, ok := msg.(message.Storable); ok {
        return mp.processSimpleGraphable(ctx, storable, storable.StorageRef().Key)
    }

    // Check for Graphable interface
    if graphable, ok := msg.(message.Graphable); ok {
        return mp.processSimpleGraphable(ctx, graphable, "")
    }

    // REJECT: No Graphable interface
    return nil, errors.WrapInvalid(errors.ErrInvalidData, "MessageManager",
        "ProcessMessage", "payload must implement Graphable interface (EntityID + Triples)")
}
```

### 2. Clear Error Messages

```go
// When payload doesn't implement Graphable:
"payload must implement Graphable interface (EntityID + Triples)"

// Additional context in logs:
mp.deps.Logger.Error("Rejected non-Graphable payload",
    "payload_type", fmt.Sprintf("%T", msg),
    "hint", "Implement EntityID() string and Triples() []Triple methods")
```

### 3. Domain-Specific Processors (Recommended Pattern)

Instead of generic processors, create domain-specific processors that understand your data semantics. See the IoT sensor example in `examples/processors/iot_sensor/` for the recommended pattern:

```
External JSON → Domain Processor → Domain Payload (Graphable) → Graph Processor
```

The domain processor:

1. Transforms JSON with semantic understanding
2. Applies organizational context (OrgID, Platform)
3. Produces payloads with proper 6-part entity IDs
4. Generates semantic triples using registered predicates

This approach produces high-quality graph data because it encodes domain knowledge into the transformation.

---

## Migration Path

### Phase 1: Add Warnings (Non-Breaking)

1. Log warnings when fallback paths are used
2. Add deprecation notice to fallback path code
3. Update documentation (DONE - see semdocs updates)

### Phase 2: Require Graphable (Breaking)

1. Remove fallback paths
2. Graph processor requires Graphable interface
3. Clear error messages guide users to implement interface

---

## Testing Changes

### New Tests

```go
func TestProcessMessage_RejectsNonGraphable(t *testing.T) {
    // Should return error for payloads without Graphable
    payload := &NonGraphablePayload{Data: "test"}
    _, err := manager.ProcessMessage(ctx, payload)
    require.Error(t, err)
    assert.Contains(t, err.Error(), "must implement Graphable")
}

func TestProcessMessage_AcceptsGraphable(t *testing.T) {
    // Should process payloads implementing Graphable
    payload := &GraphablePayload{ID: "test.entity.id"}
    states, err := manager.ProcessMessage(ctx, payload)
    require.NoError(t, err)
    assert.Len(t, states, 1)
}
```

### Remove Tests

```go
// Remove or update tests for removed fallback paths:
// - TestProcessNonGraphableMessage
// - TestProcessMapMessage
```

---

## Documentation Status

The semdocs documentation has been updated to reflect this spec:

- `05-message-system.md` - Graphable is THE required interface for graph storage
- `01-what-is-semstreams.md` - Uses proper 6-part entity IDs
- `04-first-flow.md` - Shows Graphable-based flow
- `06-vocabulary-registry.md` - Predicates for triples

---

## Acceptance Criteria

- [ ] Fallback paths (`processNonGraphableMessage`, `processMapMessage`) removed
- [ ] Graph processor returns clear error for non-Graphable payloads
- [ ] Error message includes hint to implement Graphable interface
- [ ] All tests pass
- [ ] Documentation matches implementation (DONE)

---

## Risks

1. **Breaking change** for users sending non-Graphable payloads
   - Mitigated by Phase 1 warnings
   - Domain-specific processors (see `examples/processors/iot_sensor/`) provide the correct pattern

2. **Backwards compatibility**
   - Users should create domain processors that implement Graphable
   - See migration guide in `examples/processors/iot_sensor/README.md`

---

## Timeline

- **Phase 1:** Next release (warnings only)
- **Phase 2:** Following release (enforce contract)

---

## Key Principle

**SemStreams uses interfaces as data contracts. The Graphable interface is the semantic contract. Implement it or don't use the graph.**
