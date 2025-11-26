# Data Model: Graphable Examples

**Branch**: `002-graphable-examples` | **Date**: 2025-11-26 | **Spec**: [spec.md](./spec.md)

## Overview

This document defines the data structures for the IoT sensor example that demonstrates the correct Graphable implementation pattern.

## Entity Definitions

### SensorReading

**Purpose**: Domain payload representing an IoT sensor measurement. Implements Graphable with federated entity IDs and semantic predicates.

```go
type SensorReading struct {
    // Input fields (from incoming JSON)
    DeviceID    string    // e.g., "sensor-042"
    SensorType  string    // e.g., "temperature", "humidity", "pressure"
    Value       float64   // e.g., 23.5
    Unit        string    // e.g., "celsius", "percent", "hpa"
    LocationID  string    // e.g., "warehouse-7"
    ObservedAt  time.Time // When measurement was taken

    // Context fields (set by processor from config)
    OrgID    string // e.g., "acme"
    Platform string // e.g., "logistics"
}
```

**Graphable Implementation**:

| Method | Return | Description |
|--------|--------|-------------|
| `EntityID()` | `string` | 6-part federated ID: `{org}.{platform}.environmental.sensor.{type}.{device_id}` |
| `Triples()` | `[]Triple` | 4+ semantic triples about the reading |

**Example EntityID**: `acme.logistics.environmental.sensor.temperature.sensor-042`

**Example Triples**:

| Subject | Predicate | Object | Notes |
|---------|-----------|--------|-------|
| (self) | `sensor.measurement.celsius` | `23.5` | Value with unit-specific predicate |
| (self) | `sensor.classification.type` | `"temperature"` | Sensor type classification |
| (self) | `geo.location.zone` | `acme.logistics.facility.zone.area.warehouse-7` | Entity reference |
| (self) | `time.observation.recorded` | `2025-11-26T10:30:00Z` | Timestamp |

---

### Zone (Referenced Entity)

**Purpose**: Demonstrates entity relationships - SensorReading references Zone by entity ID, not string.

```go
type Zone struct {
    ZoneID   string // e.g., "warehouse-7"
    ZoneType string // e.g., "warehouse", "office", "outdoor"
    Name     string // e.g., "Main Warehouse"

    // Context fields
    OrgID    string
    Platform string
}
```

**Graphable Implementation**:

| Method | Return | Description |
|--------|--------|-------------|
| `EntityID()` | `string` | 6-part federated ID: `{org}.{platform}.facility.zone.{type}.{zone_id}` |
| `Triples()` | `[]Triple` | Properties of the zone |

**Example EntityID**: `acme.logistics.facility.zone.area.warehouse-7`

---

## Federated Entity ID Structure

All entity IDs follow the 6-part federated pattern:

```text
{org}.{platform}.{domain}.{system}.{type}.{instance}
```

| Part | Description | Example |
|------|-------------|---------|
| `org` | Organization identifier | `acme` |
| `platform` | Platform/product | `logistics` |
| `domain` | Business domain | `environmental` |
| `system` | System/subsystem | `sensor` |
| `type` | Entity type | `temperature` |
| `instance` | Unique instance | `sensor-042` |

---

## Predicate Registry

### IoT Domain Predicates

| Predicate | Domain | Category | Property | DataType | Description |
|-----------|--------|----------|----------|----------|-------------|
| `sensor.measurement.celsius` | sensor | measurement | celsius | float64 | Temperature in Celsius |
| `sensor.measurement.fahrenheit` | sensor | measurement | fahrenheit | float64 | Temperature in Fahrenheit |
| `sensor.measurement.percent` | sensor | measurement | percent | float64 | Percentage (humidity) |
| `sensor.measurement.hpa` | sensor | measurement | hpa | float64 | Pressure in hectopascals |
| `sensor.classification.type` | sensor | classification | type | string | Sensor type |
| `sensor.classification.ambient` | sensor | classification | ambient | bool | Is ambient measurement |

### Geo Domain Predicates

| Predicate | Domain | Category | Property | DataType | Description |
|-----------|--------|----------|----------|----------|-------------|
| `geo.location.zone` | geo | location | zone | entity_ref | Reference to zone entity |
| `geo.location.latitude` | geo | location | latitude | float64 | GPS latitude |
| `geo.location.longitude` | geo | location | longitude | float64 | GPS longitude |

### Time Domain Predicates

| Predicate | Domain | Category | Property | DataType | Description |
|-----------|--------|----------|----------|----------|-------------|
| `time.observation.recorded` | time | observation | recorded | timestamp | When observation occurred |
| `time.observation.received` | time | observation | received | timestamp | When observation was received |

---

## Input/Output Mapping

### Input: Raw Sensor JSON

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

### Output: Graphable Payload

```go
&SensorReading{
    DeviceID:   "sensor-042",
    SensorType: "temperature",
    Value:      23.5,
    Unit:       "celsius",
    LocationID: "warehouse-7",
    ObservedAt: time.Parse(...),
    OrgID:      "acme",      // from config
    Platform:   "logistics", // from config
}
```

### Output: Graph Triples

```text
Subject: acme.logistics.environmental.sensor.temperature.sensor-042
Triples:
  - sensor.measurement.celsius = 23.5
  - sensor.classification.type = "temperature"
  - geo.location.zone = "acme.logistics.facility.zone.area.warehouse-7"
  - time.observation.recorded = 2025-11-26T10:30:00Z
```

---

## Validation Rules

### EntityID Validation

- Must have exactly 6 dot-separated parts
- No part may be empty
- Parts must be lowercase alphanumeric with hyphens allowed

### Predicate Validation

- Must follow `domain.category.property` format
- Must be registered in vocabulary before use
- No colon notation (`:`) allowed

### Triple Validation

- Subject must be valid EntityID
- Predicate must be registered
- Object type must match predicate's declared DataType

---

## Relationships

```text
SensorReading ──geo.location.zone──▶ Zone
```

The `geo.location.zone` predicate creates a typed relationship between entities. The object is the Zone's EntityID, not a string.

This enables graph traversal: "Find all sensors in warehouse-7" becomes a simple predicate query.
