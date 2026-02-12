# JSON Generic Processor

A protocol-layer processor that wraps plain JSON into GenericJSON (core .json.v1) format for integration with
SemStreams pipelines.

## Purpose

The JSON generic processor acts as an ingestion adapter, converting external JSON data into the SemStreams
GenericJSON message format. It subscribes to NATS subjects carrying plain JSON, wraps the data in
GenericJSONPayload structure, and publishes to output subjects for downstream processing. This processor is a
protocol-layer utility that handles data plumbing without making semantic decisions about entity identity,
graph triples, or domain meaning.

## Configuration

The processor requires port configuration specifying input subjects (plain JSON) and output subjects
(GenericJSON format).

### YAML Example

```yaml
components:
  - name: json-wrapper
    type: json_generic
    ports:
      inputs:
        - name: nats_input
          type: nats
          subject: external.sensors.>
          required: true
          description: NATS subjects with plain JSON data
      outputs:
        - name: nats_output
          type: nats
          subject: internal.sensors
          interface: core .json.v1
          required: true
          description: NATS subject for GenericJSON wrapped messages
```

### JetStream Support

The processor supports both core NATS and JetStream for inputs and outputs:

```yaml
components:
  - name: json-wrapper-jetstream
    type: json_generic
    ports:
      inputs:
        - name: jetstream_input
          type: jetstream
          subject: raw.weather.*
          stream_name: RAW
          required: true
      outputs:
        - name: jetstream_output
          type: jetstream
          subject: generic.weather
          interface: core .json.v1
          required: true
```

### Default Configuration

If no configuration is provided, the processor uses these defaults:

- **Input**: `raw.>` (all subjects under raw namespace)
- **Output**: `generic.messages` (wrapped GenericJSON messages)

## Input/Output Ports

### Input Ports

- **Type**: `nats` or `jetstream`
- **Format**: Plain JSON objects
- **Required**: At least one input port must be configured
- **Wildcards**: Supports NATS wildcard subscriptions (`>`, `*`)

**Example Input (Plain JSON):**

```json
{
  "sensor_id": "temp-001",
  "value": 23.5,
  "unit": "celsius",
  "timestamp": 1234567890
}
```

### Output Ports

- **Type**: `nats` or `jetstream`
- **Interface**: `core .json.v1` (GenericJSON format)
- **Format**: BaseMessage wrapper containing GenericJSONPayload
- **Required**: Typically one output port

**Example Output (GenericJSON):**

```json
{
  "type": {
    "domain": "core",
    "category": "json",
    "version": "v1"
  },
  "payload": {
    "data": {
      "sensor_id": "temp-001",
      "value": 23.5,
      "unit": "celsius",
      "timestamp": 1234567890
    }
  },
  "source": "json-generic-processor"
}
```

## Message Flow

```mermaid
flowchart LR
    A[External System] -->|Plain JSON| B[NATS: raw.>]
    B --> C[JSON Generic Processor]
    C -->|Wrap in GenericJSON| D[NATS: generic.messages]
    D --> E[Downstream Processors]

    style C fill:#e1f5ff
    style D fill:#fff5e1
```

## Example Use Cases

### External API Integration

Ingest third-party JSON APIs into SemStreams pipelines:

```yaml
# Weather API publishes plain JSON to raw.weather
# JSON Generic wraps it for processing pipeline
components:
  - name: weather-wrapper
    type: json_generic
    ports:
      inputs:
        - name: api_input
          type: nats
          subject: raw.weather
      outputs:
        - name: wrapped_output
          type: nats
          subject: internal.weather
          interface: core .json.v1
```

### Legacy System Migration

Wrap legacy JSON formats for modern pipeline compatibility:

```yaml
# Legacy system emits plain JSON
# Wrap for processing by json_filter and json_map
components:
  - name: legacy-wrapper
    type: json_generic
    ports:
      inputs:
        - name: legacy_input
          type: nats
          subject: legacy.data
      outputs:
        - name: modern_output
          type: nats
          subject: modern.pipeline
          interface: core .json.v1
```

### Data Normalization

Standardize multiple JSON sources into unified GenericJSON format:

```yaml
# Multiple sources feed into single wrapper
# Output goes to unified processing pipeline
components:
  - name: multi-source-wrapper
    type: json_generic
    ports:
      inputs:
        - name: source_a
          type: nats
          subject: source.a.data
        - name: source_b
          type: nats
          subject: source.b.data
        - name: source_c
          type: nats
          subject: source.c.data
      outputs:
        - name: unified_output
          type: nats
          subject: unified.data
          interface: core .json.v1
```

### Pipeline Entry Point

Convert raw JSON to pipeline-compatible format for filtering and transformation:

```yaml
# Raw input → Wrap → Filter → Map → Domain processor
components:
  - name: entry-wrapper
    type: json_generic
    ports:
      inputs:
        - name: raw_input
          type: nats
          subject: raw.input
      outputs:
        - name: validated_output
          type: nats
          subject: validated.input
          interface: core .json.v1

  - name: filter-step
    type: json_filter
    ports:
      inputs:
        - name: filter_input
          type: nats
          subject: validated.input
          interface: core .json.v1
      outputs:
        - name: filtered_output
          type: nats
          subject: filtered.data
          interface: core .json.v1
```

## Error Handling

The processor handles errors gracefully to maintain pipeline resilience:

### Invalid JSON

Messages that fail JSON parsing are logged at Debug level and dropped:

```text
Input: {this is not valid json}
Log: "Failed to parse message as JSON"
Action: Message dropped, error counter incremented
Impact: No output published, processing continues
```

### NATS Publish Failures

Network issues during publish are logged as errors with full context:

```text
Error: Failed to publish wrapped message
Logged Fields: component, output_subject, error
Action: Error counter incremented
Impact: Message lost (no retry), processing continues
```

### Validation Failures

If wrapped payload fails validation (rare, indicates internal issue):

```text
Error: Wrapped payload validation failed
Action: Error counter incremented
Impact: Message dropped, processing continues
```

## Performance

Typical throughput: 15,000+ messages/second per processor instance

**Complexity:**

- Wrapping: O(1) - Single map allocation per message
- Validation: O(n) - Validates payload structure (minimal overhead)
- Marshaling: O(n) - JSON serialization of wrapped payload

**Resource Usage:**

- Memory: ~1KB per message in flight
- CPU: Minimal, dominated by JSON marshaling
- Network: Adds ~150 bytes overhead per message (BaseMessage wrapper)

## Observability

The processor implements the `Discoverable` interface for comprehensive monitoring:

### Metadata

```go
meta := processor.Meta()
// Name: json-generic-processor
// Type: processor
// Description: Wraps plain JSON into GenericJSON (core .json.v1) format
// Version: 0.1.0
```

### Metrics

```go
dataFlow := processor.DataFlow()
// ErrorRate: JSON parse errors / Messages processed
// LastActivity: Timestamp of last message processed
```

### Health Status

```go
health := processor.Health()
// Healthy: true if processor is running
// ErrorCount: Total errors (parse + publish)
// Uptime: Time since processor started
```

### Key Metrics

- **MessagesProcessed**: Total messages received (valid + invalid JSON)
- **MessagesWrapped**: Successfully wrapped and published messages
- **Errors**: JSON parse errors + NATS publish errors

**Quality Indicator:**

```text
Error Rate = Errors / MessagesProcessed

< 0.01 (1%): Good input quality
0.01-0.05: Investigate data sources
> 0.05: Significant data quality issues
```

## Design Philosophy

The JSON generic processor follows SemStreams protocol-layer design principles:

### What It Does NOT Do

- **No EntityID Generation**: Does not determine entity identities
- **No Triple Creation**: Does not create semantic graph relationships
- **No Domain Interpretation**: Does not classify or interpret field meanings

These responsibilities belong to domain processors that understand your data semantics.

### Pipeline Position

```text
External JSON → [json_generic] → GenericJSON → [json_filter/map] → [Domain Processor] → Graph
                 ^^^^^^^^^^^^                                       ^^^^^^^^^^^^^^^^
                 Protocol layer                                     Semantic layer
                 (this package)                                     (your code)
```

### When to Use

Use json_generic when:

- Ingesting data from external systems that emit plain JSON
- Converting raw JSON to GenericJSON for use with json_filter or json_map
- Normalizing heterogeneous JSON sources into standard format
- Adding GenericJSON interface compatibility to legacy data sources

Do NOT use when:

- Input is already in GenericJSON format (use json_filter or json_map directly)
- You need custom wrapping structure (extend or create custom processor)
- Schema validation is required (add json_filter downstream)

## Comparison with Other Processors

### vs JSONFilterProcessor

- **json_generic**: Wraps plain JSON → GenericJSON (no filtering)
- **json_filter**: Filters GenericJSON → GenericJSON (no wrapping)

### vs JSONMapProcessor

- **json_generic**: Wraps plain JSON → GenericJSON (no transformation)
- **json_map**: Transforms GenericJSON → GenericJSON (no wrapping)

### Typical Pipeline

```text
raw.json → [json_generic] → wrapped.json → [json_filter] → filtered.json → [json_map] → mapped.json
```

## Limitations

Current version limitations:

- No schema validation of input JSON (accepts any valid JSON object)
- No custom wrapping structure (always uses "data" field in GenericJSONPayload)
- No metadata injection (timestamps, source tags, etc.)
- Invalid JSON messages are dropped (no dead letter queue or retry)
- Single output subject (no routing based on content)

These may be addressed in future versions based on user requirements.

## Testing

Run the test suite:

```bash
# Unit tests
task test -- ./processor/json_generic -v

# With race detection
task test:race -- ./processor/json_generic -v

# Integration tests (when available)
task test:integration -- ./processor/json_generic -v
```

## See Also

- [Processor Design Philosophy](../../docs/PROCESSOR-DESIGN-PHILOSOPHY.md)
- [GenericJSON Message Format](../../message/generic_json.go)
- [JSON Filter Processor](../json_filter/README.md)
- [JSON Map Processor](../json_map/README.md)
