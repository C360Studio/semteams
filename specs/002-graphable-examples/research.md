# Research: Graphable Examples

**Branch**: `002-graphable-examples` | **Date**: 2025-11-26 | **Spec**: [spec.md](./spec.md)

## Research Summary

This document captures the architectural decisions and research findings for replacing the `json_to_entity` anti-pattern with proper domain-specific Graphable examples.

## Key Decisions

### Decision 1: Remove json_to_entity Processor

**Context**: The `json_to_entity` processor exists in `processor/json_to_entity/` and provides a generic way to convert arbitrary JSON into graph entities.

**Problem**: This contradicts the core SemStreams philosophy documented in `docs/PROCESSOR-DESIGN-PHILOSOPHY.md`:

> "Take control of your semantic processing as close to source as you can. Don't let people who don't understand your use case determine your graph."

**Issues with json_to_entity**:

1. Makes semantic decisions without domain knowledge
2. Produces low-quality, auto-generated triples
3. Creates entity IDs without federated structure
4. Derives predicates from JSON keys, not semantic meaning
5. Treats relationships as strings, not entity references

**Decision**: Remove `json_to_entity` entirely. It is a fallback we should not support.

**Rationale**: SemStreams exists to help developers create domain-specific processors. A generic processor that skips semantic modeling defeats the purpose.

---

### Decision 2: Use IoT Sensors as Example Domain

**Context**: Examples need a domain to demonstrate patterns. Options considered:

1. Robotics operations (first customer domain)
2. IoT sensors (neutral, universally understood)
3. Generic "widget" examples

**Decision**: Use IoT sensors as the example domain.

**Rationale**:

- Simple enough to understand quickly
- Complex enough to show real patterns (measurements, locations, classifications)
- Universally understood across industries
- Won't conflict with production robotics implementation (lives in separate repo)
- Demonstrates entity relationships (sensor → zone)

---

### Decision 3: Example Location

**Context**: Where should example processors live?

**Decision**: Create `examples/processors/iot_sensor/` in the SemStreams repo.

**Rationale**:

- Clearly marked as example, not production code
- Consistent with `vocabulary/examples/` pattern
- Domain processors belong in domain repos; examples belong in framework

---

## Existing Code Analysis

### Files to Remove

```text
processor/json_to_entity/
├── json_to_entity.go           # Main processor
├── json_to_entity_test.go      # Unit tests
├── json_to_entity_integration_test.go  # Integration tests
└── config.go                   # Configuration
```

### Graphable Interface (message/graphable.go)

```go
type Graphable interface {
    EntityID() string      // Deterministic 6-part federated identifier
    Triples() []Triple     // Semantic facts about the entity
}
```

### Triple Structure (message/triple.go)

```go
type Triple struct {
    Subject   string
    Predicate string
    Object    any
    Timestamp time.Time
}
```

### Vocabulary Registration Pattern (vocabulary/predicates.go)

```go
func Register(predicate string, meta PredicateMetadata) error
```

---

## IoT Sensor Example Design

### Entity: SensorReading

**Purpose**: Represents a single sensor measurement with full semantic context.

**EntityID Pattern**: `{org}.{platform}.environmental.sensor.{type}.{device_id}`

Example: `acme.logistics.environmental.sensor.temperature.sensor-042`

**Triples Generated**:

1. `sensor.measurement.{unit}` → measurement value
2. `sensor.classification.type` → sensor type
3. `geo.location.zone` → entity reference to zone
4. `time.observation.recorded` → timestamp

### Entity: Zone

**Purpose**: Related entity showing entity references in triples.

**EntityID Pattern**: `{org}.{platform}.facility.zone.area.{zone_id}`

Example: `acme.logistics.facility.zone.area.warehouse-7`

---

## Documentation Updates Required

### Primary Documents

1. **SPEC-SEMANTIC-CONTRACT.md** - Remove json_to_entity references, point to IoT example
2. **PROCESSOR-DESIGN-PHILOSOPHY.md** - Already references IoT pattern (no changes needed)

### Search for References

Documents potentially referencing `json_to_entity`:

- Any processor documentation
- Integration guides
- Getting started guides

---

## Test Migration Strategy

### Current json_to_entity Tests

Tests in `processor/json_to_entity/` will be removed with the processor.

### Tests Using json_to_entity

Search for imports: `github.com/c360/semstreams/processor/json_to_entity`

Migration options:

1. Update to use direct `EntityPayload` construction
2. Update to use new IoT sensor example
3. Remove if testing generic JSON conversion (no longer supported)

---

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Breaking existing flows | Low | Medium | json_to_entity marked deprecated, migration guidance provided |
| Missing test coverage | Low | Low | IoT example includes comprehensive tests |
| Documentation inconsistency | Medium | Low | Systematic search and update |

---

## Open Questions

**All resolved** - No remaining clarifications needed.

---

## References

- [PROCESSOR-DESIGN-PHILOSOPHY.md](/docs/PROCESSOR-DESIGN-PHILOSOPHY.md) - Core philosophy
- [SPEC-SEMANTIC-CONTRACT.md](/docs/SPEC-SEMANTIC-CONTRACT.md) - Semantic contract proposal
- [spec.md](./spec.md) - Feature specification
