# JSON Map Processor

Protocol-layer processor for transforming GenericJSON message fields through mapping, adding, and removing operations.

## Purpose

The JSON Map processor enables flexible field-level transformations of GenericJSON payloads without making semantic
decisions. It subscribes to NATS subjects carrying GenericJSON messages (`core .json.v1` interface), applies mapping
rules to transform the data, and publishes the transformed messages to output subjects. This processor handles
protocol-layer normalization such as field renaming, debug data removal, and metadata injection before semantic
processing occurs.

## Configuration

```yaml
type: processor
component: json_map
config:
  ports:
    inputs:
      - name: input
        type: nats
        subject: sensor.raw
        interface: core .json.v1
        required: true
    outputs:
      - name: output
        type: nats
        subject: sensor.normalized
        interface: core .json.v1
        required: true
  mappings:
    - source_field: temp
      target_field: temperature
      transform: copy
    - source_field: stat
      target_field: status
      transform: lowercase
  add_fields:
    unit: celsius
    version: 2
    source: sensor-network
  remove_fields:
    - debug_timestamp
    - internal_id
```

### Configuration Fields

**Ports** (required)

- `inputs`: Array of input port definitions. Each port must specify `interface: core .json.v1`.
- `outputs`: Array of output port definitions. Transformed messages use the same `core .json.v1` interface.

**Mappings** (optional)

Array of field mapping rules. Each mapping contains:

- `source_field`: Original field name to map from
- `target_field`: New field name to map to
- `transform`: Transformation type (`copy`, `uppercase`, `lowercase`, `trim`)

Source fields are removed after mapping unless `source_field` equals `target_field`.

**Add Fields** (optional)

Map of constant fields to add to every message. Useful for injecting metadata, version tags, or configuration values.

**Remove Fields** (optional)

Array of field names to remove from payloads. Applied after mappings but before string transformations.

## Input/Output Ports

### Input Ports

- **Type**: NATS or JetStream
- **Interface**: `core .json.v1` (GenericJSON)
- **Wildcard Support**: Yes (e.g., `sensor.*.raw`, `data.>`)
- **Required**: Yes (at least one input port)

### Output Ports

- **Type**: NATS or JetStream
- **Interface**: `core .json.v1` (GenericJSON)
- **Required**: No (processor can be used for side effects only)

## Transformation Operations

The processor applies operations in this order:

1. **Field Mappings**: Rename fields and remove source
2. **Add Fields**: Inject constant values
3. **Remove Fields**: Delete unwanted fields
4. **String Transformations**: Apply string operations to mapped field names

### Field Mapping Example

```json
// Input
{"data": {"temp": 23.5, "location": "lab-1"}}

// Mapping: {"source_field": "temp", "target_field": "temperature"}

// Output
{"data": {"temperature": 23.5, "location": "lab-1"}}
```

### Transformation Types

- `copy`: No transformation (default)
- `uppercase`: Convert string to uppercase (`"active"` → `"ACTIVE"`)
- `lowercase`: Convert string to lowercase (`"ACTIVE"` → `"active"`)
- `trim`: Remove leading and trailing whitespace (`"  error  "` → `"error"`)

Non-string values are passed through unchanged when string transforms are specified.

## Example Use Cases

### Schema Migration

Migrate sensor data from version 1 to version 2 format.

```yaml
mappings:
  - source_field: temp
    target_field: temperature
    transform: copy
  - source_field: loc
    target_field: location
    transform: copy
add_fields:
  schema_version: 2
remove_fields:
  - deprecated_field
```

### Data Sanitization

Remove PII and debug information before publishing to external systems.

```yaml
remove_fields:
  - email
  - phone
  - ssn
  - internal_id
  - debug_timestamp
add_fields:
  sanitized: true
  sanitized_at: 2026-02-11T00:00:00Z
```

### Field Standardization

Normalize inconsistent field names and values from multiple data sources.

```yaml
mappings:
  - source_field: temp
    target_field: temperature
    transform: copy
  - source_field: stat
    target_field: status
    transform: uppercase
  - source_field: msg
    target_field: message
    transform: trim
add_fields:
  standardized: true
  source: data-ingestion-pipeline
```

### Enrichment

Add contextual metadata to raw sensor readings.

```yaml
add_fields:
  facility: warehouse-a
  region: north-america
  timezone: America/New_York
  processed_by: json-map-v2
```

## Pipeline Position

The JSON Map processor is a protocol-layer utility positioned before semantic processors:

```text
GenericJSON → [json_map] → Transformed GenericJSON → [Domain Processor] → Graph
               ^^^^^^^^                               ^^^^^^^^^^^^^^^^
               Protocol layer                         Semantic layer
```

This processor does NOT:

- Generate EntityIDs (no identity determination)
- Create semantic triples (no Graphable implementation)
- Interpret domain meaning (transformations are mechanical)

Use domain processors for semantic transformations like "classify this sensor reading as critical."

## Performance Characteristics

- **Complexity**: O(n + m + k) where n = fields to add, m = fields to remove, k = transformations
- **Throughput**: 10,000+ messages/second per instance
- **Memory**: Constant per message (creates new map for each transformation)
- **Concurrency**: Thread-safe with goroutine-per-message processing

## Observability

### Health Metrics

```go
health := processor.Health()
// Healthy: true/false
// ErrorCount: Parse + transformation errors
// Uptime: Time since start
```

### Data Flow Metrics

```go
flow := processor.DataFlow()
// ErrorRate: errors / messages processed
// LastActivity: Timestamp of last message
```

### Prometheus Metrics

The processor exports Prometheus metrics when a registry is provided:

- `semstreams_json_map_messages_processed_total`: Total messages received
- `semstreams_json_map_transformations_total`: Successful transformations
- `semstreams_json_map_errors_total`: Error count by type
- `semstreams_json_map_transformation_duration_seconds`: Transformation latency

## Limitations

Current version does not support:

- Nested field mapping (e.g., `position.lat` → `latitude`)
- Conditional transformations (transform based on field values)
- Computed fields (combine multiple fields)
- Custom transformation functions
- Type conversions (string to number, etc.)

These features may be added in future versions based on requirements.

## Testing

```bash
# Unit tests
go test ./processor/json_map -v

# Integration tests (requires Docker)
go test -tags=integration ./processor/json_map -v

# Race detection
go test -race ./processor/json_map -v
```

## Related Documentation

- [Processor Design Philosophy](/Users/coby/Code/c360/semstreams/docs/PROCESSOR-DESIGN-PHILOSOPHY.md)
- [GenericJSON Interface](/Users/coby/Code/c360/semstreams/message/generic_json.go)
- [Component Architecture](/Users/coby/Code/c360/semstreams/docs/concepts/09-components.md)
