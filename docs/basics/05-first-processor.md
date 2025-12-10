# Building Your First Processor

This tutorial walks through building a domain processor from scratch. We'll use IoT sensor readings as the example domain.

> **Note**: Processors are the most common Component type to develop—they transform domain data into semantic entities. For other Component types (Input, Output, Gateway), see [Component Development](../development/components.md).

## Designing Your Domain Model

Building a processor means creating a lightweight domain ontology:

- **Vocabulary**: The predicates that describe facts about your entities
- **Entity structure**: How you identify and relate entities
- **Business logic**: Rules encoded in your Triples() method

SemStreams lets you define this without OWL/SPARQL complexity. You can:

- Create custom vocabularies from scratch
- Map to standard vocabularies (Dublin Core, Schema.org) via IRI mappings
- Encode as much or as little ontological structure as you need

Start with your vocabulary—it shapes everything else.

## What You're Building

A processor that:

1. Receives sensor readings (temperature, humidity)
2. Transforms them into graph entities with semantic triples
3. Creates relationships to zone entities

## Step 1: Design Your Vocabulary

Start by defining the predicates that describe facts about your entities. This shapes your entire domain model.

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

**Vocabulary design principles:**

- Use three-part dotted notation: `domain.category.property`
- Include units in measurement predicates for clarity
- Distinguish property predicates (literal values) from relationship predicates (entity references)
- Use constants—not string literals—to prevent typos

See [Vocabulary](04-vocabulary.md) for complete design guidelines.

## Step 2: Define Your Payload

With your vocabulary defined, create the struct that holds incoming data:

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

## Step 3: Implement EntityID()

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
- Hierarchical: enables pattern queries via NATS wildcards (`acme.logistics.*`, `*.sensor.*`)

## Step 4: Implement Triples()

Return all facts about this entity using your vocabulary constants:

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
            Predicate: PredicateClassificationType,
            Object:    s.SensorType,
            Source:    "iot_sensor",
            Timestamp: s.ObservedAt,
        },
        // Relationship: zone reference (creates graph edge)
        {
            Subject:   entityID,
            Predicate: PredicateLocationZone,
            Object:    s.ZoneEntityID,
            Source:    "iot_sensor",
            Timestamp: s.ObservedAt,
        },
        // Property: timestamp
        {
            Subject:   entityID,
            Predicate: PredicateObservationRecorded,
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

- [Configuration](06-configuration.md) - Choose your capability level
- [Indexes](../graph/03-indexes.md) - How triples become queryable
- [Testing Guide](../development/testing.md#testing-graphable-implementations) - Test your Graphable implementations
