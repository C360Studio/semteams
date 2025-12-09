# Triples

Triples are Subject-Predicate-Object facts about entities. They're the fundamental unit of information in the knowledge graph.

## Triple Structure

```go
type Triple struct {
    Subject    string    // EntityID (who this fact is about)
    Predicate  string    // Property name (domain.category.property)
    Object     any       // Value or entity reference
    Source     string    // Provenance ("iot_sensor", "operator", "rule")
    Timestamp  time.Time // When fact was observed
    Confidence float64   // 0.0-1.0 reliability score
}
```

## Properties vs Relationships

The Object field determines whether a triple is a property or a relationship:

### Property Triple (value)

```go
{
    Subject:   "acme.logistics.environmental.sensor.temperature.sensor-042",
    Predicate: "sensor.measurement.celsius",
    Object:    23.5,  // A value = property
}
```

Properties are stored but don't create graph edges.

### Relationship Triple (entity reference)

```go
{
    Subject:   "acme.logistics.environmental.sensor.temperature.sensor-042",
    Predicate: "geo.location.zone",
    Object:    "acme.logistics.facility.zone.area.warehouse-7",  // Entity ID = relationship
}
```

Relationships create edges in the graph:
- Indexed in `INCOMING_INDEX` and `OUTGOING_INDEX`
- Traversed by community detection
- Queryable via relationship queries

## Predicate Format

Predicates use dotted notation: `domain.category.property`

```text
sensor.measurement.celsius
geo.location.zone
fleet.membership.current
time.observation.recorded
```

**Why dotted notation?**

- NATS wildcard queries: `sensor.measurement.*`
- Self-documenting: The predicate explains itself
- Collision-free: Different domains don't overlap
- Consistent with entity ID patterns

## Triple Types

### Measurement Triples

```go
{
    Subject:   entityID,
    Predicate: "sensor.measurement.celsius",
    Object:    23.5,
    Source:    "iot_sensor",
    Timestamp: observedAt,
}
```

### Classification Triples

```go
{
    Subject:   entityID,
    Predicate: "sensor.classification.type",
    Object:    "temperature",
    Source:    "iot_sensor",
    Timestamp: observedAt,
}
```

### Relationship Triples

```go
{
    Subject:   entityID,
    Predicate: "geo.location.zone",
    Object:    zoneEntityID,  // Another entity's ID
    Source:    "iot_sensor",
    Timestamp: observedAt,
}
```

### Alias Triples

```go
{
    Subject:   entityID,
    Predicate: "iot.sensor.serial",  // Registered as alias in vocabulary
    Object:    "SN-2025-001234",
    Source:    "iot_sensor",
    Timestamp: observedAt,
}
```

Alias predicates must be registered in the vocabulary with `WithAlias()` to be indexed in `ALIAS_INDEX`.

## Confidence Levels

| Level | Meaning | Example |
|-------|---------|---------|
| 1.0 | Direct measurement or explicit data | Sensor readings, operator input |
| 0.9 | High-confidence readings | GPS with good signal |
| 0.7 | Calculated or derived values | Estimated values |
| 0.5 | Inferred relationships | AI-detected patterns |
| 0.0 | Uncertain or placeholder | Default values |

## Relationship Detection

SemStreams determines if a triple is a relationship by checking if the Object is a valid 6-part entity ID:

```go
// Internally uses IsRelationship()
if triple.IsRelationship() {
    // Index in INCOMING_INDEX and OUTGOING_INDEX
}
```

## Creating Triples

Triples come from your `Triples()` method:

```go
func (s *SensorReading) Triples() []message.Triple {
    entityID := s.EntityID()

    triples := []message.Triple{
        // Property: measurement
        {
            Subject:    entityID,
            Predicate:  fmt.Sprintf("sensor.measurement.%s", s.Unit),
            Object:     s.Value,
            Source:     "iot_sensor",
            Timestamp:  s.ObservedAt,
            Confidence: 1.0,
        },
        // Property: classification
        {
            Subject:    entityID,
            Predicate:  "sensor.classification.type",
            Object:     s.SensorType,
            Source:     "iot_sensor",
            Timestamp:  s.ObservedAt,
            Confidence: 1.0,
        },
        // Relationship: zone
        {
            Subject:    entityID,
            Predicate:  "geo.location.zone",
            Object:     s.ZoneEntityID,
            Source:     "iot_sensor",
            Timestamp:  s.ObservedAt,
            Confidence: 1.0,
        },
    }

    return triples
}
```

## Rules and Triples

Rules can add or remove triples dynamically:

```json
{
  "id": "battery-low-alert",
  "expression": "drone.telemetry.battery < 20",
  "on_enter": [
    {"action": "add_triple", "predicate": "alert.status", "object": "battery_low"}
  ],
  "on_exit": [
    {"action": "remove_triple", "predicate": "alert.status"}
  ]
}
```

Rule-generated triples:
- Appear in entity state like any other triple
- Create relationships if object is entity ID
- Are removed when rule condition exits

## Predicate Best Practices

### Use Consistent Naming

```go
// Good: Hierarchical, consistent
"sensor.measurement.celsius"
"sensor.measurement.humidity"
"sensor.status.battery"

// Bad: Fragments the index
"temp"
"Humidity"
"battery_status"
```

### Unit-Specific Predicates

```go
// Good: Unit is explicit
"sensor.measurement.celsius"
"sensor.measurement.fahrenheit"

// Bad: Ambiguous
"sensor.temperature"  // Celsius? Fahrenheit?
```

### Use Constants

```go
const (
    PredicateMeasurementCelsius = "sensor.measurement.celsius"
    PredicateLocationZone       = "geo.location.zone"
)

// In Triples():
{Predicate: PredicateMeasurementCelsius, Object: s.Value}
```

### Full Entity IDs in Relationships

```go
// Good: Full 6-part ID
{Predicate: "geo.location.zone", Object: "acme.logistics.facility.zone.area.warehouse-7"}

// Bad: Partial or simple ID
{Predicate: "geo.location.zone", Object: "warehouse-7"}
```

## How Triples Become Indexed

| Triple Type | Index Updated |
|-------------|---------------|
| Any triple | `PREDICATE_INDEX` |
| Relationship triple | `INCOMING_INDEX`, `OUTGOING_INDEX` |
| Alias triple | `ALIAS_INDEX` |
| Geo triple | `SPATIAL_INDEX` |
| Timestamp triple | `TEMPORAL_INDEX` |

## Next Steps

- [Indexes](03-indexes.md) - How triples feed indexes
- [Communities](04-communities.md) - How relationships enable clustering
- [Vocabulary](../basics/04-vocabulary.md) - Predicate design patterns
