# Vocabulary: Designing Your Predicates

Predicates are how you name facts in your graph. Consistent predicate naming determines whether your graph is queryable or a mess of unrelated data.

## Predicate Format

All predicates follow three-level dotted notation:

```text
domain.category.property
```

**Examples:**

- `sensor.measurement.celsius`
- `geo.location.latitude`
- `fleet.membership.current`
- `drone.telemetry.battery`

## Why Dotted Notation?

1. **NATS wildcard queries**: `sensor.measurement.*` finds all measurement predicates
2. **Self-documenting**: `drone.telemetry.battery` explains itself
3. **No collisions**: `sensor.temperature` vs `hvac.temperature` are distinct
4. **Consistent**: Matches entity ID patterns

## Naming Conventions

| Level | Rules | Examples |
|-------|-------|----------|
| domain | lowercase, business area | `sensor`, `geo`, `fleet`, `drone` |
| category | lowercase, groups related | `measurement`, `location`, `membership` |
| property | lowercase, specific name | `celsius`, `latitude`, `current` |

**Avoid:**

- Underscores: `sensor_temperature` (use `sensor.temperature`)
- camelCase: `sensorTemperature` (use `sensor.temperature`)
- Abbreviations: `sens.temp.c` (use `sensor.measurement.celsius`)

## Defining Your Vocabulary

Define predicates as package constants:

```go
package robotics

// Drone telemetry predicates
const (
    DroneBattery   = "drone.telemetry.battery"
    DroneAltitude  = "drone.telemetry.altitude"
    DroneStatus    = "drone.telemetry.status"
    DroneHeading   = "drone.telemetry.heading"
)

// Fleet relationship predicates
const (
    FleetMembership = "fleet.membership.current"
    FleetAssignment = "fleet.assignment.mission"
)

// Geospatial predicates
const (
    GeoLatitude  = "geo.location.latitude"
    GeoLongitude = "geo.location.longitude"
    GeoAltitude  = "geo.location.altitude"
    GeoZone      = "geo.location.zone"
)
```

## Registering Predicates (Optional)

For discoverability and standards compliance, register predicates with metadata:

```go
import "github.com/c360/semstreams/vocabulary"

func init() {
    vocabulary.Register(DroneBattery,
        vocabulary.WithDescription("Battery charge level percentage"),
        vocabulary.WithDataType("int"),
        vocabulary.WithUnits("percent"),
        vocabulary.WithRange("0-100"),
    )

    vocabulary.Register(GeoLatitude,
        vocabulary.WithDescription("Latitude in decimal degrees"),
        vocabulary.WithDataType("float64"),
        vocabulary.WithRange("-90 to 90"),
        vocabulary.WithIRI(vocabulary.GeoLatitude), // Optional: RDF mapping
    )
}
```

Registration is optional but enables:

- API introspection of available predicates
- Data type validation
- RDF/Turtle export with standard IRIs

## Unit-Specific Predicates

Include units in the predicate for clarity:

```go
// Good: Unit is explicit
"sensor.measurement.celsius"
"sensor.measurement.fahrenheit"
"sensor.measurement.percent"

// Bad: Unit is ambiguous
"sensor.temperature"  // Celsius? Fahrenheit? Kelvin?
"sensor.value"        // What value?
```

This allows mixed units in the same graph without confusion.

## Property vs Relationship Predicates

Property predicates have literal values:

```go
{Predicate: "drone.telemetry.battery", Object: 78}          // int
{Predicate: "sensor.measurement.celsius", Object: 23.5}     // float
{Predicate: "entity.status.name", Object: "active"}         // string
```

Relationship predicates have entity IDs as values:

```go
{Predicate: "fleet.membership.current", Object: "acme.ops.fleet.cargo.rescue"}
{Predicate: "geo.location.zone", Object: "acme.logistics.facility.zone.area.warehouse-7"}
```

The distinction matters because relationships create edges in the graph.

## Common Predicate Domains

| Domain | Use For | Examples |
|--------|---------|----------|
| `sensor` | Measurements, readings | `sensor.measurement.*`, `sensor.classification.*` |
| `geo` | Spatial data | `geo.location.*`, `geo.boundary.*` |
| `time` | Temporal data | `time.observation.*`, `time.lifecycle.*` |
| `entity` | Entity metadata | `entity.status.*`, `entity.label.*` |
| `fleet` | Fleet operations | `fleet.membership.*`, `fleet.assignment.*` |
| `drone` | Drone-specific | `drone.telemetry.*`, `drone.mission.*` |

## Alias Predicates

Some predicates provide alternate identifiers for entity resolution:

```go
vocabulary.Register("sensor.identifier.serial",
    vocabulary.WithDescription("Manufacturer serial number"),
    vocabulary.WithAlias(vocabulary.AliasTypeExternal, 1),
)
```

Alias types:

- `AliasTypeIdentity`: Entity equivalence (owl:sameAs)
- `AliasTypeAlternate`: Secondary unique ID
- `AliasTypeExternal`: External system ID
- `AliasTypeCommunication`: Call signs, hostnames
- `AliasTypeLabel`: Display names (NOT for resolution)

## Impact on Graph Queries

### PREDICATE_INDEX

Every predicate creates an index entry. Consistent naming enables queries:

```go
// All entities with temperature readings
QueryByPredicate(ctx, "sensor.measurement.celsius")

// All entities with fleet membership
QueryByPredicate(ctx, "fleet.membership.current")
```

Inconsistent naming fragments your index:

```go
// These are THREE separate predicates:
"temp"
"temperature"
"sensor.temperature"

// Queries only find entities using that exact predicate
```

### INCOMING_INDEX / OUTGOING_INDEX

Relationship predicates create edges:

```go
// Creates edge: drone → fleet
{Subject: droneID, Predicate: "fleet.membership.current", Object: fleetID}

// Query: Who belongs to this fleet?
GetIncoming(ctx, fleetID, "fleet.membership.current")
```

### Community Detection

LPA traverses edges created by relationship predicates. Entities that share relationships cluster together.

## Best Practices

### 1. Define a Domain Vocabulary File

```go
// pkg/vocabulary/robotics/predicates.go
package robotics

const (
    // Telemetry
    TelemetryBattery  = "drone.telemetry.battery"
    TelemetryAltitude = "drone.telemetry.altitude"

    // Relationships
    FleetMembership = "fleet.membership.current"
    MissionAssignment = "mission.assignment.current"
)
```

### 2. Use Constants, Not Strings

```go
// Good: Constants are refactorable, typo-proof
{Predicate: robotics.TelemetryBattery, Object: 78}

// Bad: String typos cause silent failures
{Predicate: "drone.telemtry.battery", Object: 78}  // typo!
```

### 3. Document Your Vocabulary

```go
// TelemetryBattery represents battery charge level as a percentage.
// Range: 0-100
// Source: GCS heartbeat messages
const TelemetryBattery = "drone.telemetry.battery"
```

### 4. Version Breaking Changes

If you must change a predicate name:

```go
// Deprecated: Use TelemetryBattery
const BatteryLevel = "battery.level"

// Current
const TelemetryBattery = "drone.telemetry.battery"
```

Migrate data before removing the old predicate.

## Next Steps

- [First Processor](05-first-processor.md) - Use your vocabulary in a processor
- [Tiers](06-tiers.md) - How vocabulary affects capabilities
- [Indexes](../graph/03-indexes.md) - How predicates become indexes
