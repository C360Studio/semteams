# Building Your First Processor

This tutorial walks through building a domain processor from scratch. We'll use IoT sensor readings as the example domain.

## What You're Building

A processor that:

1. Receives sensor readings (temperature, humidity)
2. Transforms them into graph entities with semantic triples
3. Creates relationships to zone entities

## Step 1: Define Your Payload

Start with the struct that holds your incoming data:

```go
package iotsensor

import (
    "fmt"
    "time"
    "github.com/c360/semstreams/message"
)

// SensorReading represents an IoT sensor measurement.
type SensorReading struct {
    DeviceID   string    // e.g., "sensor-042"
    SensorType string    // e.g., "temperature", "humidity"
    Value      float64   // e.g., 23.5
    Unit       string    // e.g., "celsius", "percent"
    ObservedAt time.Time

    // Reference to another entity
    ZoneEntityID string // e.g., "acme.logistics.facility.zone.area.warehouse-7"

    // Context (set by processor)
    OrgID    string
    Platform string
}
```

## Step 2: Implement EntityID()

Return a deterministic 6-part federated identifier:

```go
func (s *SensorReading) EntityID() string {
    return fmt.Sprintf("%s.%s.environmental.sensor.%s.%s",
        s.OrgID,      // "acme"
        s.Platform,   // "logistics"
        s.SensorType, // "temperature"
        s.DeviceID,   // "sensor-042"
    )
}
// Result: "acme.logistics.environmental.sensor.temperature.sensor-042"
```

**Requirements:**

- 6 parts separated by dots
- Deterministic: same input always produces same ID
- Hierarchical: enables prefix queries (`acme.logistics.*`)

## Step 3: Implement Triples()

Return all facts about this entity:

```go
func (s *SensorReading) Triples() []message.Triple {
    entityID := s.EntityID()

    return []message.Triple{
        // Property: measurement value
        {
            Subject:   entityID,
            Predicate: fmt.Sprintf("sensor.measurement.%s", s.Unit),
            Object:    s.Value,
            Source:    "iot_sensor",
            Timestamp: s.ObservedAt,
        },
        // Property: sensor type
        {
            Subject:   entityID,
            Predicate: "sensor.classification.type",
            Object:    s.SensorType,
            Source:    "iot_sensor",
            Timestamp: s.ObservedAt,
        },
        // Relationship: zone reference (creates graph edge)
        {
            Subject:   entityID,
            Predicate: "geo.location.zone",
            Object:    s.ZoneEntityID,
            Source:    "iot_sensor",
            Timestamp: s.ObservedAt,
        },
        // Property: timestamp
        {
            Subject:   entityID,
            Predicate: "time.observation.recorded",
            Object:    s.ObservedAt,
            Source:    "iot_sensor",
            Timestamp: s.ObservedAt,
        },
    }
}
```

**Key distinction:**

- `Object: s.Value` (a number) = property
- `Object: s.ZoneEntityID` (entity ID string) = relationship (edge)

## Step 4: Define Your Predicates

Create constants for your vocabulary:

```go
package iotsensor

// Measurement predicates (unit-specific)
const (
    PredicateMeasurementCelsius    = "sensor.measurement.celsius"
    PredicateMeasurementFahrenheit = "sensor.measurement.fahrenheit"
    PredicateMeasurementPercent    = "sensor.measurement.percent"
)

// Classification predicates
const (
    PredicateClassificationType = "sensor.classification.type"
)

// Geospatial predicates
const (
    PredicateLocationZone      = "geo.location.zone"
    PredicateLocationLatitude  = "geo.location.latitude"
    PredicateLocationLongitude = "geo.location.longitude"
)

// Temporal predicates
const (
    PredicateObservationRecorded = "time.observation.recorded"
)
```

Use these constants in your `Triples()` method instead of string literals.

## Step 5: Create the Processor

The processor applies organizational context and transforms raw input:

```go
type Config struct {
    OrgID    string
    Platform string
}

type Processor struct {
    config Config
}

func NewProcessor(config Config) *Processor {
    return &Processor{config: config}
}

func (p *Processor) Process(input map[string]any) (*SensorReading, error) {
    deviceID, err := getString(input, "device_id")
    if err != nil {
        return nil, fmt.Errorf("missing device_id: %w", err)
    }

    sensorType, err := getString(input, "type")
    if err != nil {
        return nil, fmt.Errorf("missing type: %w", err)
    }

    value, err := getFloat64(input, "reading")
    if err != nil {
        return nil, fmt.Errorf("missing reading: %w", err)
    }

    unit, err := getString(input, "unit")
    if err != nil {
        return nil, fmt.Errorf("missing unit: %w", err)
    }

    locationID, err := getString(input, "location")
    if err != nil {
        return nil, fmt.Errorf("missing location: %w", err)
    }

    return &SensorReading{
        DeviceID:     deviceID,
        SensorType:   sensorType,
        Value:        value,
        Unit:         unit,
        ObservedAt:   time.Now(),
        ZoneEntityID: ZoneEntityID(p.config.OrgID, p.config.Platform, "area", locationID),
        OrgID:        p.config.OrgID,
        Platform:     p.config.Platform,
    }, nil
}
```

## Step 6: Handle Related Entities

When you reference another entity, you need that entity to exist. Create a Zone type:

```go
type Zone struct {
    ZoneID   string
    ZoneType string
    Name     string
    OrgID    string
    Platform string
}

func (z *Zone) EntityID() string {
    return fmt.Sprintf("%s.%s.facility.zone.%s.%s",
        z.OrgID,
        z.Platform,
        z.ZoneType,
        z.ZoneID,
    )
}

func (z *Zone) Triples() []message.Triple {
    entityID := z.EntityID()
    return []message.Triple{
        {
            Subject:   entityID,
            Predicate: "facility.zone.name",
            Object:    z.Name,
            Timestamp: time.Now(),
        },
        {
            Subject:   entityID,
            Predicate: "facility.zone.type",
            Object:    z.ZoneType,
            Timestamp: time.Now(),
        },
    }
}
```

Use a helper function for consistent zone entity IDs:

```go
func ZoneEntityID(orgID, platform, zoneType, zoneID string) string {
    return fmt.Sprintf("%s.%s.facility.zone.%s.%s",
        orgID, platform, zoneType, zoneID)
}
```

## Step 7: Test Your Implementation

```go
func TestSensorReading_EntityID(t *testing.T) {
    reading := &SensorReading{
        DeviceID:   "sensor-042",
        SensorType: "temperature",
        OrgID:      "acme",
        Platform:   "logistics",
    }

    expected := "acme.logistics.environmental.sensor.temperature.sensor-042"
    assert.Equal(t, expected, reading.EntityID())
}

func TestSensorReading_Triples(t *testing.T) {
    reading := &SensorReading{
        DeviceID:     "sensor-042",
        SensorType:   "temperature",
        Value:        23.5,
        Unit:         "celsius",
        ZoneEntityID: "acme.logistics.facility.zone.area.warehouse-7",
        OrgID:        "acme",
        Platform:     "logistics",
        ObservedAt:   time.Now(),
    }

    triples := reading.Triples()

    // Verify measurement triple exists
    var found bool
    for _, t := range triples {
        if t.Predicate == "sensor.measurement.celsius" {
            assert.Equal(t, 23.5, t.Object)
            found = true
        }
    }
    assert.True(t, found, "measurement triple not found")
}
```

## What Happens Next

When your processor runs:

1. **Entity Storage**: The entity is stored in `ENTITY_STATES` KV bucket
2. **Predicate Index**: Each predicate creates an entry in `PREDICATE_INDEX`
3. **Relationship Index**: The zone reference creates entries in `INCOMING_INDEX` and `OUTGOING_INDEX`
4. **Community Detection**: If enabled, entities with relationships cluster together

## Common Patterns

### Conditional Triples

Add triples only when data is present:

```go
func (s *Sensor) Triples() []message.Triple {
    triples := []message.Triple{...}

    if s.SerialNumber != "" {
        triples = append(triples, message.Triple{
            Subject:   s.EntityID(),
            Predicate: "sensor.identifier.serial",
            Object:    s.SerialNumber,
        })
    }

    return triples
}
```

### Geospatial Data

Add lat/lon for spatial indexing:

```go
if s.Latitude != nil && s.Longitude != nil {
    triples = append(triples, message.Triple{
        Subject:   s.EntityID(),
        Predicate: "geo.location.latitude",
        Object:    *s.Latitude,
    })
    triples = append(triples, message.Triple{
        Subject:   s.EntityID(),
        Predicate: "geo.location.longitude",
        Object:    *s.Longitude,
    })
}
```

### Dynamic Predicates

Include unit in the predicate for clarity:

```go
// Results in: sensor.measurement.celsius, sensor.measurement.fahrenheit, etc.
Predicate: fmt.Sprintf("sensor.measurement.%s", s.Unit)
```

## Checklist

Before deploying your processor:

- [ ] EntityID returns exactly 6 parts
- [ ] EntityID is deterministic (same input = same output)
- [ ] Predicates use dotted notation (domain.category.property)
- [ ] Entity references use full entity IDs, not partial strings
- [ ] Required fields are validated
- [ ] Constants defined for all predicates
- [ ] Tests cover EntityID and Triples methods

## Next Steps

- [Tiers](06-tiers.md) - Choose your capability level
- [Vocabulary](04-vocabulary.md) - Design your predicates
- [Indexes](../graph/03-indexes.md) - How triples become queryable
