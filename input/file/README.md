# File Input Component

File input component for reading JSONL/JSON files and publishing to NATS subjects.

## Purpose

The file input component reads JSON Lines or JSON files from the filesystem and publishes each line or record as a
message to NATS. It supports glob patterns for batch processing, rate-limited publishing, and continuous replay loops
for test data generation and development scenarios.

## Configuration

```yaml
name: event-ingest
type: input
protocol: file
config:
  ports:
    outputs:
      - name: output
        type: jetstream
        subject: events.raw
        required: true
  path: /data/events/*.jsonl
  format: jsonl
  interval: 10ms
  loop: false
```

### Configuration Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `path` | string | Yes | - | File path or glob pattern (e.g., `/data/*.jsonl`) |
| `format` | string | No | `jsonl` | File format: `jsonl` or `json` |
| `interval` | string | No | `10ms` | Delay between publishing lines (rate control) |
| `loop` | boolean | No | `false` | Continuously re-read files after completion |
| `ports.outputs` | array | Yes | - | Output port configuration for NATS publishing |

## Input/Output Ports

### Input Ports

| Port | Type | Description |
|------|------|-------------|
| `file_source` | file | File source reading from specified path pattern |

### Output Ports

| Port | Type | Description |
|------|------|-------------|
| `nats_output` | nats/jetstream | NATS subject for publishing file lines |

## Supported File Formats

### JSONL (JSON Lines)

One JSON object per line, no outer array. Each line is validated before publishing.

```jsonl
{"id": "1", "type": "event", "timestamp": "2024-01-15T10:00:00Z"}
{"id": "2", "type": "event", "timestamp": "2024-01-15T10:00:01Z"}
{"id": "3", "type": "event", "timestamp": "2024-01-15T10:00:02Z"}
```

Invalid lines are logged and skipped without stopping file processing.

### JSON

Standard JSON format. For arrays, each element is published as a separate message.

```json
[
  {"id": "1", "type": "event"},
  {"id": "2", "type": "event"}
]
```

## Example Use Cases

### Data Replay

Replay archived event data for testing or recovery scenarios.

```yaml
config:
  path: /archive/events-2024-01-15.jsonl
  format: jsonl
  interval: 10ms
  loop: false
```

### Continuous Test Data

Generate continuous test data for development and load testing.

```yaml
config:
  path: /testdata/sample-events.jsonl
  format: jsonl
  interval: 100ms
  loop: true
```

### Batch Import

Maximum throughput import of large datasets.

```yaml
config:
  path: /import/batch-*.jsonl
  format: jsonl
  interval: 0
  loop: false
```

### Multi-File Processing

Process multiple files matching a glob pattern.

```yaml
config:
  path: /data/2024-01/*/*.jsonl
  format: jsonl
  interval: 5ms
  loop: false
```

## Rate Control

The `interval` setting controls publishing rate to prevent overwhelming downstream consumers.

| Interval | Max Throughput | Use Case |
|----------|----------------|----------|
| `0` | Unlimited | Batch imports, maximum throughput |
| `1ms` | ~1000 msg/s | High-volume ingestion |
| `10ms` | ~100 msg/s | Moderate load testing |
| `100ms` | ~10 msg/s | Gentle replay, development |

## Observability

### Prometheus Metrics

- `semstreams_file_input_lines_read_total` - Total lines read from files
- `semstreams_file_input_lines_published_total` - Lines successfully published to NATS
- `semstreams_file_input_bytes_read_total` - Total bytes read
- `semstreams_file_input_parse_errors_total` - JSON parse failures
- `semstreams_file_input_files_processed_total` - Files completely processed

### Health Status

```go
health := input.Health()
// Healthy: true if component running
// ErrorCount: Parse/publish errors
// Uptime: Time since Start()
```

### Data Flow Metrics

```go
dataFlow := input.DataFlow()
// MessagesPerSecond: Publishing rate
// BytesPerSecond: Byte throughput
// ErrorRate: Error percentage
```

## Lifecycle Management

```go
// Initialize (validate path, check files exist)
if err := input.Initialize(); err != nil {
    log.Fatal(err)
}

// Start reading and publishing
if err := input.Start(ctx); err != nil {
    log.Fatal(err)
}

// Graceful shutdown with timeout
if err := input.Stop(5 * time.Second); err != nil {
    log.Warn(err)
}
```

## Performance Characteristics

- **Throughput**: 10,000+ lines/second (without interval delay)
- **Memory**: O(1) per file (buffered line-by-line reading)
- **Buffer**: 1MB initial, 10MB max per line
- **Context checks**: Every 100 lines (responsive shutdown)

## Error Handling

- **Invalid configuration**: Returns error during initialization
- **No matching files**: Returns error during initialization
- **Parse errors**: Logged and skipped, processing continues
- **Publish errors**: Logged, processing continues

Individual line errors do not stop file processing. File-level errors are logged but do not stop glob pattern
processing.

## Limitations

- No compression support (gzip, zstd)
- No offset tracking (always starts from beginning)
- No file watching (new files require restart)
- No parallel file processing
- Maximum line length: 10MB

## Related Components

- [UDP Input](../udp/) - UDP datagram input
- [WebSocket Input](../websocket/) - WebSocket stream input
- [NATS Client](../../natsclient/) - NATS connection and publishing
