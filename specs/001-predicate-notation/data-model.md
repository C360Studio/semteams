# Data Model: Remove Legacy RDF Predicates

**Feature**: 001-predicate-notation
**Date**: 2025-11-26

## Overview

This feature removes code that generates `rdf:type` and `rdf:class` triples. No new data structures or predicates are introduced.

## Affected Code

### EntityPayload.Triples() (message/entity_payload.go)

**Before**: Generated 2 + N triples (type triple, class triple, N property triples)

**After**: Generates only N property triples

```go
// REMOVED - lines 124-132
// Triple{
//     Subject:   e.ID,
//     Predicate: "rdf:type",
//     Object:    e.Type,
//     ...
// }

// REMOVED - lines 134-144
// Triple{
//     Subject:   e.ID,
//     Predicate: "rdf:class",
//     Object:    string(e.Class),
//     ...
// }
```

**Rationale**: Entity type and class are already available as `EntityPayload.Type` and `EntityPayload.Class` struct fields. Separate triples are redundant.

## Predicate Pattern

The vocabulary registry expects three-level dotted notation:

```
domain.category.property
```

**Valid Examples** (from vocabulary README):
- `sensor.temperature.celsius`
- `geo.location.latitude`
- `time.lifecycle.created`
- `robotics.battery.level`

**Invalid** (removed by this feature):
- `rdf:type` (uses colon)
- `rdf:class` (uses colon)

## No New Predicates

This is a removal-only change. The `entity.meta.type` and `entity.meta.class` predicates proposed earlier are NOT being added.
