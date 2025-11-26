# TODO: Predicate Notation Consistency

**Status:** Open
**Priority:** Low
**Related:** ADR-TRIPLES-AS-SOURCE-OF-TRUTH.md, TODO-GRAPH-INDEXING-ARCHITECTURE.md

---

## Problem

The codebase has inconsistent predicate notation:

| Pattern | Example | Location |
|---------|---------|----------|
| **Dotted (correct)** | `robotics.battery.level` | Vocabulary registry, most code |
| **Colon (legacy)** | `rdf:type`, `rdf:class` | `message/entity_payload.go` |

The vocabulary registry and documentation standardize on **three-level dotted notation**:

```text
domain.category.property
```

But legacy code still uses RDF-style colon notation for type predicates.

---

## Affected Code

### `message/entity_payload.go`

```go
// Line 127
Predicate: "rdf:type",

// Line 138
Predicate: "rdf:class",
```

### `message/graphable.go`

```go
// Line 45 (documentation example)
{Subject: entityID, Predicate: "rdf:type", Object: "robotics:Drone"},
```

### Tests

*Note: The `processor/json_to_entity/` tests were removed as part of the json_to_entity processor removal (see `examples/processors/iot_sensor/README.md` for migration guidance).*

---

## Proposed Fix

Replace colon notation with dotted notation:

| Current | Proposed |
|---------|----------|
| `rdf:type` | `entity.type` |
| `rdf:class` | `entity.class` |
| `robotics:Drone` (object) | `robotics.Drone` or keep as type URI |

---

## Migration Steps

1. **Add new predicates to vocabulary registry:**
   ```go
   vocabulary.Register("entity.type",
       vocabulary.WithDescription("Entity type classification"),
       vocabulary.WithDataType("string"))

   vocabulary.Register("entity.class",
       vocabulary.WithDescription("Entity class for inheritance"),
       vocabulary.WithDataType("string"))
   ```

2. **Update entity_payload.go** to use dotted notation

3. **Update tests** to expect dotted notation

4. **Update documentation examples** in graphable.go

5. **Consider backward compatibility:**
   - Add index migration if PREDICATE_INDEX has `rdf:type` entries
   - Or accept both during transition period

---

## Why This Matters

1. **Consistency** - All predicates should follow same convention
2. **NATS wildcards** - Dotted notation works with NATS subject wildcards
3. **Vocabulary registry** - Can't register `rdf:type` (colon breaks pattern)
4. **Documentation** - Confusing to explain two conventions

---

## Decision

Low priority because:
- Current code works
- Limited to type/class predicates
- No user-facing impact

Address when:
- Refactoring entity_payload.go
- Or as part of larger vocabulary cleanup

---

## References

- Vocabulary Registry: `vocabulary/registry.go`
- Predicate Documentation: `docs/basics/06-vocabulary-registry.md`
- Entity Payload: `message/entity_payload.go`
