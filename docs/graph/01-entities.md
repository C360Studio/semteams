# Entities

Entities are the nodes in your knowledge graph. Each entity represents something that exists and has properties expressed as semantic triples.

## Entity IDs

Every entity has a 6-part federated identifier:

```text
org.platform.domain.system.type.instance
```

| Part | Description | Example |
|------|-------------|---------|
| org | Organization namespace | `acme` |
| platform | Platform/deployment | `logistics` |
| domain | Data domain | `environmental` |
| system | System identifier | `sensor` |
| type | Entity type | `temperature` |
| instance | Unique instance | `sensor-042` |

**Examples:**

```text
acme.logistics.environmental.sensor.temperature.sensor-042
acme.ops.robotics.gcs.drone.drone-007
acme.platform.facility.zone.area.warehouse-7
```

**Why 6-part IDs?**

- Globally unique across federated deployments
- Hierarchical for pattern matching (`acme.ops.*`)
- Self-documenting - IDs explain provenance
- Collision-free without coordination

## How Entities Are Created

Entities come from payloads that implement the Graphable interface:

```go
type Graphable interface {
    EntityID() string      // 6-part federated identifier
    Triples() []Triple     // Semantic facts about this entity
}
```

The graph processor only accepts payloads implementing Graphable.

```go
type SensorReading struct {
    DeviceID   string
    SensorType string
    Value      float64
    Unit       string
    OrgID      string
    Platform   string
}

func (s *SensorReading) EntityID() string {
    return fmt.Sprintf("%s.%s.environmental.sensor.%s.%s",
        s.OrgID,
        s.Platform,
        s.SensorType,
        s.DeviceID,
    )
}

func (s *SensorReading) Triples() []message.Triple {
    entityID := s.EntityID()
    return []message.Triple{
        {Subject: entityID, Predicate: fmt.Sprintf("sensor.measurement.%s", s.Unit), Object: s.Value},
        {Subject: entityID, Predicate: "sensor.classification.type", Object: s.SensorType},
    }
}
```

## Entity State

When stored, entities become `EntityState` in the `ENTITY_STATES` bucket:

```json
{
  "id": "acme.logistics.environmental.sensor.temperature.sensor-042",
  "triples": [
    {"subject": "acme.logistics...", "predicate": "sensor.measurement.celsius", "object": 23.5},
    {"subject": "acme.logistics...", "predicate": "sensor.classification.type", "object": "temperature"}
  ],
  "version": 42,
  "updated_at": "2024-12-08T10:30:00Z"
}
```

Key fields:

| Field | Purpose |
|-------|---------|
| `id` | 6-part federated entity ID |
| `triples` | All semantic facts about this entity |
| `version` | Incremented on each update (optimistic concurrency) |
| `updated_at` | Last update timestamp |

## Version Tracking

Entities use optimistic concurrency via version numbers:

1. Read entity (version=5)
2. Prepare update
3. Write with expected version (version=5)
4. If version changed, compare-and-swap fails
5. Retry with fresh read

This prevents lost updates in concurrent scenarios.

## Entity Updates

Entities are upserted - created if new, updated if exists:

- **Node.ID match** → Update existing entity
- **Triples** replaced with new set from `Triples()`
- **Version** incremented
- **Indexes** updated asynchronously

## Pattern Matching

Entity IDs support glob patterns for rules and queries:

```text
*.*.environmental.sensor.*.*     # All sensors from any org
acme.*.*.*.*.*                   # All entities in acme org
acme.logistics.*.*.*.*           # All logistics entities
```

Used in:
- Rules engine entity patterns
- NATS KV bucket watches
- Query filters

## EntityID Requirements

Your `EntityID()` implementation must be:

1. **Deterministic**: Same input always produces same ID
2. **6 parts**: Exactly 6 dot-separated segments
3. **Hierarchical**: Higher levels should group related entities
4. **Unique**: No collisions within your system

```go
func (d *Drone) EntityID() string {
    // Deterministic: same drone always gets same ID
    return fmt.Sprintf("%s.%s.robotics.gcs.drone.%s",
        d.OrgID,      // org
        d.Platform,   // platform
                      // domain: robotics
                      // system: gcs
                      // type: drone
        d.DroneID,    // instance
    )
}
```

## Common Patterns

### Context from Configuration

```go
type Processor struct {
    config Config // Contains OrgID, Platform
}

func (p *Processor) Process(input map[string]any) (*SensorReading, error) {
    return &SensorReading{
        DeviceID: input["device_id"].(string),
        OrgID:    p.config.OrgID,     // From processor config
        Platform: p.config.Platform,   // From processor config
    }, nil
}
```

### Entity ID Helper Functions

```go
// Single source of truth for zone IDs
func ZoneEntityID(orgID, platform, zoneType, zoneID string) string {
    return fmt.Sprintf("%s.%s.facility.zone.%s.%s",
        orgID, platform, zoneType, zoneID)
}
```

## Next Steps

- [Triples](02-triples.md) - How to express facts about entities
- [Indexes](03-indexes.md) - How entities are indexed
- [Graphable Interface](../basics/03-graphable-interface.md) - Complete interface reference
