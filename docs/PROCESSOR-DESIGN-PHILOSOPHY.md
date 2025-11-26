# Processor Design Philosophy

**Status**: Reference
**Date**: 2025-11-26

---

## Core Principle

> **Take control of your semantic processing as close to source as you can. Don't let people who don't understand your use case determine your graph.**

SemStreams exists to help developers transform incoming data into meaningful semantic graphs. The transformation step is where domain knowledge lives - it cannot be automated away.

---

## The Graphable Contract

The `Graphable` interface is the semantic contract:

```go
type Graphable interface {
    EntityID() string      // Deterministic 6-part federated identifier
    Triples() []Triple     // Semantic facts about the entity
}
```

Any payload implementing this interface belongs in the graph. The interface forces you to answer two questions:

1. **What is this entity?** (EntityID)
2. **What facts do we know about it?** (Triples)

These questions require domain understanding. Generic processors cannot answer them well.

---

## Why Generic Processors Fail

Consider incoming sensor data:

```json
{
  "device_id": "sensor-042",
  "type": "temperature",
  "reading": 23.5,
  "unit": "celsius",
  "location": "warehouse-7",
  "timestamp": "2025-11-26T10:30:00Z"
}
```

A **generic processor** might produce:

```go
EntityID: "sensor-042"
Triples:
  - {Predicate: "temperature.reading", Object: 23.5}
  - {Predicate: "temperature.unit", Object: "celsius"}
  - {Predicate: "temperature.location", Object: "warehouse-7"}
```

Problems:

- EntityID has no federated structure (org, platform, domain, system, type)
- Predicates derived from JSON structure, not semantic meaning
- No relationship to the warehouse entity
- No classification of reading type (ambient? equipment? product?)
- `location` is a string, not a relationship to a Place entity

A **domain processor** with understanding produces:

```go
EntityID: "acme.logistics.warehouse.environmental.temperature.sensor-042"
Triples:
  - {Predicate: "sensor.measurement.celsius", Object: 23.5}
  - {Predicate: "sensor.classification.ambient", Object: true}
  - {Predicate: "geo.location.zone", Object: "acme.logistics.warehouse.facility.zone.warehouse-7"}
  - {Predicate: "time.observation.recorded", Object: "2025-11-26T10:30:00Z"}
```

The domain processor knows:

- How to construct federated entity IDs for this organization
- Which predicates from the vocabulary capture the semantic meaning
- That location should reference another entity, not be a string
- How to classify sensor readings in this domain

---

## Processor Categories

### 1. Generic Utilities (No Semantic Decisions)

These processors handle data plumbing without making semantic choices:

| Processor | Purpose |
|-----------|---------|
| `parser` | Parse formats (JSON, CSV, protobuf) into Go structures |
| `filter` | Route/filter messages based on conditions |
| `router` | Direct messages to different outputs |
| `throttle` | Rate limiting and backpressure |

These are domain-agnostic and belong in the core framework.

### 2. Domain Processors (Encode Semantic Understanding)

These processors transform data into meaningful graph structures:

| Processor | Domain | Semantic Decisions |
|-----------|--------|-------------------|
| `iot_sensor` | IoT/Environmental | Sensor classification, measurement predicates, zone relationships |
| `logistics_tracking` | Supply Chain | Shipment entities, location transitions, custody chains |
| `user_activity` | SaaS/Analytics | User entities, action predicates, session relationships |

**These belong in domain-specific repositories**, not in the core SemStreams framework.

### 3. Boundary Adapters (Translate Between Models)

For integration with external systems that have their own semantic models:

| Adapter | Purpose |
|---------|---------|
| `ontology_mapper` | Translate to/from formal ontologies (OWL, RDF) |
| `schema_translator` | Map external schemas to internal predicates |

---

## SemStreams Repository Structure

The core framework provides:

```text
semstreams/
├── processor/
│   ├── parser/          # Generic: format parsing
│   ├── filter/          # Generic: message filtering
│   ├── router/          # Generic: message routing
│   ├── graph/           # Core: graph storage (requires Graphable)
│   └── rule/            # Generic: rule evaluation
├── message/
│   └── graphable.go     # The Graphable interface
├── vocabulary/
│   ├── predicates.go    # Predicate validation
│   └── examples/        # Example predicate definitions
└── examples/
    └── processors/
        └── iot_sensor/  # EXAMPLE domain processor (for reference)
```

Domain-specific processors live in their own repositories:

```text
customer-robotics/
├── processors/
│   ├── drone_telemetry/
│   ├── arm_status/
│   └── mission_tracker/
├── vocabulary/
│   └── robotics_predicates.go
└── payloads/
    ├── drone_payload.go
    └── mission_payload.go
```

---

## Building a Domain Processor

### Step 1: Define Your Payload

```go
// payloads/sensor_reading.go
package payloads

import "github.com/c360/semstreams/message"

type SensorReading struct {
    DeviceID    string
    SensorType  string
    Value       float64
    Unit        string
    ZoneID      string
    ObservedAt  time.Time

    // Internal: set by processor
    orgID       string
    platform    string
}

func (s *SensorReading) EntityID() string {
    // Federated 6-part ID encoding organizational structure
    return fmt.Sprintf("%s.%s.environmental.sensor.%s.%s",
        s.orgID,
        s.platform,
        s.SensorType,
        s.DeviceID,
    )
}

func (s *SensorReading) Triples() []message.Triple {
    ts := s.ObservedAt

    return []message.Triple{
        {
            Subject:   s.EntityID(),
            Predicate: fmt.Sprintf("sensor.measurement.%s", s.Unit),
            Object:    s.Value,
            Timestamp: ts,
        },
        {
            Subject:   s.EntityID(),
            Predicate: "sensor.classification.type",
            Object:    s.SensorType,
            Timestamp: ts,
        },
        {
            Subject:   s.EntityID(),
            Predicate: "geo.location.zone",
            Object:    s.zoneEntityID(), // Reference to zone entity
            Timestamp: ts,
        },
    }
}

func (s *SensorReading) zoneEntityID() string {
    return fmt.Sprintf("%s.%s.facility.zone.area.%s",
        s.orgID,
        s.platform,
        s.ZoneID,
    )
}
```

### Step 2: Build the Processor

```go
// processors/iot_sensor/processor.go
package iotsensor

type Processor struct {
    config Config
    // ...
}

type Config struct {
    OrgID       string   // Organization identifier
    Platform    string   // Platform identifier
    SensorTypes []string // Allowed sensor types
}

func (p *Processor) Process(ctx context.Context, msg *message.BaseMessage) (message.Graphable, error) {
    // Parse incoming JSON
    data := msg.Payload().(*message.GenericJSON).Data()

    // Transform to domain payload with semantic understanding
    reading := &payloads.SensorReading{
        DeviceID:   data["device_id"].(string),
        SensorType: p.classifySensor(data),           // Domain logic
        Value:      p.extractValue(data),              // Domain logic
        Unit:       p.normalizeUnit(data),             // Domain logic
        ZoneID:     p.resolveZone(data["location"]),   // Domain logic
        ObservedAt: p.parseTimestamp(data),
        orgID:      p.config.OrgID,
        platform:   p.config.Platform,
    }

    // Return Graphable payload
    return reading, nil
}

// Domain-specific logic lives here
func (p *Processor) classifySensor(data map[string]any) string {
    // Your domain knowledge about sensor classification
}

func (p *Processor) resolveZone(location any) string {
    // Your domain knowledge about location -> zone mapping
}
```

### Step 3: Register Vocabulary

```go
// vocabulary/iot_predicates.go
package vocabulary

func init() {
    // Register domain predicates
    Register("sensor.measurement.celsius", PredicateMetadata{
        Domain:      "sensor",
        Category:    "measurement",
        Property:    "celsius",
        DataType:    "float64",
        Description: "Temperature reading in Celsius",
    })

    Register("sensor.classification.type", PredicateMetadata{
        Domain:      "sensor",
        Category:    "classification",
        Property:    "type",
        DataType:    "string",
        Description: "Sensor type classification",
    })

    Register("geo.location.zone", PredicateMetadata{
        Domain:      "geo",
        Category:    "location",
        Property:    "zone",
        DataType:    "entity_ref",
        Description: "Reference to zone entity",
    })
}
```

---

## json_to_entity Has Been Removed

The `json_to_entity` processor has been **removed** from the codebase.

It was an anti-pattern that:

- Made semantic decisions without domain knowledge
- Produced low-quality, auto-generated triples
- Let developers skip the work of understanding their data

**Migration**: See `examples/processors/iot_sensor/README.md` for migration guidance. Copy the IoT sensor example and adapt it to your domain. The time spent understanding your data semantics will pay off in graph quality.

---

## Key Takeaways

1. **Graphable is the contract** - implement it with domain understanding
2. **Generic processors don't make semantic decisions** - they handle plumbing
3. **Domain processors encode your knowledge** - they live in your repo
4. **SemStreams provides the framework** - you provide the semantics
5. **Examples use neutral domains** - your production code uses your domain

---

## Example Domains in SemStreams

The `examples/` directory uses IoT sensors as a neutral, universally-understood domain. This is intentional:

- Simple enough to understand quickly
- Complex enough to show real patterns
- Not tied to any specific customer domain
- Won't conflict with production implementations

Your production processors should live in your own repository, using your domain vocabulary and your semantic understanding.
