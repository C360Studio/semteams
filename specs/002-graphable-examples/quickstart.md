# Quickstart: Graphable Examples

**Branch**: `002-graphable-examples` | **Date**: 2025-11-26 | **Spec**: [spec.md](./spec.md)

## Overview

This feature removes the `json_to_entity` anti-pattern and provides a proper IoT sensor example demonstrating domain-specific Graphable payloads.

## Prerequisites

- Go 1.25+
- Understanding of the Graphable interface
- Read [PROCESSOR-DESIGN-PHILOSOPHY.md](/docs/PROCESSOR-DESIGN-PHILOSOPHY.md)

## Quick Reference

### What's Being Removed

```text
processor/json_to_entity/  ← DELETE (anti-pattern)
```

### What's Being Added

```text
examples/processors/iot_sensor/
├── README.md           # How to adapt the example
├── payload.go          # SensorReading implementing Graphable
├── payload_test.go     # Graphable contract tests
├── processor.go        # Domain processor logic
├── processor_test.go   # Processor unit tests
└── vocabulary.go       # IoT predicate registration
```

## The Graphable Contract

Every domain payload must implement:

```go
type Graphable interface {
    EntityID() string      // 6-part federated identifier
    Triples() []Triple     // Semantic facts
}
```

## Implementation Steps

### Step 1: Define Your Payload

```go
type SensorReading struct {
    DeviceID   string
    SensorType string
    Value      float64
    Unit       string
    LocationID string
    ObservedAt time.Time
    OrgID      string
    Platform   string
}
```

### Step 2: Implement EntityID

```go
func (s *SensorReading) EntityID() string {
    return fmt.Sprintf("%s.%s.environmental.sensor.%s.%s",
        s.OrgID,
        s.Platform,
        s.SensorType,
        s.DeviceID,
    )
}
```

### Step 3: Implement Triples

```go
func (s *SensorReading) Triples() []Triple {
    return []Triple{
        {
            Subject:   s.EntityID(),
            Predicate: fmt.Sprintf("sensor.measurement.%s", s.Unit),
            Object:    s.Value,
            Timestamp: s.ObservedAt,
        },
        {
            Subject:   s.EntityID(),
            Predicate: "sensor.classification.type",
            Object:    s.SensorType,
            Timestamp: s.ObservedAt,
        },
        {
            Subject:   s.EntityID(),
            Predicate: "geo.location.zone",
            Object:    s.zoneEntityID(),
            Timestamp: s.ObservedAt,
        },
    }
}
```

### Step 4: Register Predicates

```go
func init() {
    vocabulary.Register("sensor.measurement.celsius", PredicateMetadata{
        Domain:   "sensor",
        Category: "measurement",
        Property: "celsius",
        DataType: "float64",
    })
    // ... more predicates
}
```

## Testing Your Graphable

```go
func TestSensorReading_Graphable(t *testing.T) {
    reading := &SensorReading{
        DeviceID:   "sensor-042",
        SensorType: "temperature",
        Value:      23.5,
        Unit:       "celsius",
        LocationID: "warehouse-7",
        OrgID:      "acme",
        Platform:   "logistics",
    }

    // Verify EntityID has 6 parts
    parts := strings.Split(reading.EntityID(), ".")
    assert.Equal(t, 6, len(parts))

    // Verify triples are meaningful
    triples := reading.Triples()
    assert.GreaterOrEqual(t, len(triples), 3)

    // Verify no colon notation
    for _, triple := range triples {
        assert.NotContains(t, triple.Predicate, ":")
    }
}
```

## Adapting for Your Domain

1. Copy `examples/processors/iot_sensor/` to your domain repo
2. Replace `SensorReading` with your domain entity
3. Define your federated ID structure
4. Register your domain predicates
5. Implement domain-specific transformation logic

## Common Patterns

### Entity References (Not Strings)

```go
// WRONG: Location as string
{Predicate: "location", Object: "warehouse-7"}

// RIGHT: Location as entity reference
{Predicate: "geo.location.zone", Object: "acme.logistics.facility.zone.area.warehouse-7"}
```

### Unit-Specific Predicates

```go
// WRONG: Generic with unit as separate triple
{Predicate: "measurement.value", Object: 23.5}
{Predicate: "measurement.unit", Object: "celsius"}

// RIGHT: Unit encoded in predicate
{Predicate: "sensor.measurement.celsius", Object: 23.5}
```

### Classification Triples

```go
// Add domain knowledge about the entity
{Predicate: "sensor.classification.type", Object: "temperature"}
{Predicate: "sensor.classification.ambient", Object: true}
```

## Verification

After implementation, run:

```bash
# Unit tests
go test ./examples/processors/iot_sensor/...

# Verify no json_to_entity references
grep -r "json_to_entity" processor/

# Lint check
go fmt ./...
revive ./...
```

## References

- [PROCESSOR-DESIGN-PHILOSOPHY.md](/docs/PROCESSOR-DESIGN-PHILOSOPHY.md) - Why domain processors matter
- [data-model.md](./data-model.md) - Full entity and predicate definitions
- [research.md](./research.md) - Decision rationale
