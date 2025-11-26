# Research: Remove Legacy RDF Predicates

**Feature**: 001-predicate-notation
**Date**: 2025-11-26
**Status**: Complete

## Research Questions

### 1. Why are rdf:type and rdf:class problematic?

**Question**: What's wrong with the current `rdf:type` and `rdf:class` predicates?

**Findings**:

1. **Colon notation breaks vocabulary registry**: The registry uses `parseDomainCategory()` which expects dotted notation. Colons break this pattern.

2. **Redundant data**: EntityPayload already has `Type` and `Class` fields:
   ```go
   type EntityPayload struct {
       Type  string      `json:"entity_type"`
       Class EntityClass `json:"class,omitempty"`
       // ...
   }
   ```
   Creating separate triples for these duplicates information already available on the struct.

3. **Inconsistent with vocabulary pattern**: All other predicates use `domain.category.property` (e.g., `sensor.temperature.celsius`). The RDF-style predicates don't fit.

**Decision**: Remove `rdf:type` and `rdf:class` triple generation entirely.

**Rationale**: They add no value and create inconsistency. Entity type/class are already accessible via struct fields.

### 2. Current Usage Locations

**Question**: Where exactly are these predicates used?

**Findings**:

| File | Line | Code |
|------|------|------|
| `message/entity_payload.go` | 124-132 | Generates `rdf:type` triple |
| `message/entity_payload.go` | 134-144 | Generates `rdf:class` triple |
| `message/graphable.go` | 45 | Documentation example |
| `processor/json_to_entity/json_to_entity_test.go` | 135-144 | Tests expect `rdf:type` |
| `processor/json_to_entity/json_to_entity_integration_test.go` | 180-189 | Tests expect `rdf:type` |

**Decision**: Remove code in entity_payload.go, update tests to not expect these triples, update documentation examples.

### 3. Impact Analysis

**Question**: What breaks if we remove these predicates?

**Findings**:

- **Tests**: Will fail initially, need to update expectations
- **External systems**: None depend on these predicates (per assumptions)
- **Stored data**: None - predicates are generated at runtime
- **Property triples**: Unaffected - these use `{entityType}.{propertyKey}` format

**Decision**: Safe to remove. Only test updates needed.

### 4. Documentation Examples

**Question**: What should documentation examples use instead?

**Findings**: The vocabulary README already has good examples:
- `sensor.temperature.celsius`
- `geo.location.latitude`
- `time.lifecycle.created`
- `robotics.battery.level`

**Decision**: Update graphable.go examples to use `geo.location.latitude` and similar real predicates.

## Summary

| Question | Decision |
|----------|----------|
| What to do with rdf:type/rdf:class | Remove entirely |
| Why remove | Redundant, breaks vocab pattern |
| Test impact | Update to not expect type/class triples |
| Doc examples | Use real predicates like `sensor.temperature.celsius` |

All research complete. Ready for implementation.
