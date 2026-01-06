# graph-index-temporal

Temporal indexing component for the graph subsystem.

## Overview

The `graph-index-temporal` component watches the `ENTITY_STATES` KV bucket and maintains a time-based index for entities. This enables efficient time-range queries.

## Architecture

```
                    ┌──────────────────────┐
ENTITY_STATES ─────►│                      │
   (KV watch)       │  graph-index-temporal├──► TEMPORAL_INDEX (KV)
                    │                      │
                    └──────────────────────┘
```

## Features

- **Configurable Resolution**: minute, hour, or day granularity
- **Automatic Timestamp Extraction**: Extracts timestamps from entity data
- **Time Bucket Keys**: Efficient range queries using bucket keys
- **Batch Processing**: Efficient bulk index updates

## Configuration

```json
{
  "type": "processor",
  "name": "graph-index-temporal",
  "enabled": true,
  "config": {
    "ports": {
      "inputs": [
        {
          "name": "entity_watch",
          "subject": "ENTITY_STATES",
          "type": "kv-watch"
        }
      ],
      "outputs": [
        {
          "name": "temporal_index",
          "subject": "TEMPORAL_INDEX",
          "type": "kv"
        }
      ]
    },
    "time_resolution": "hour",
    "workers": 4,
    "batch_size": 100
  }
}
```

### Configuration Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `ports` | object | required | Port configuration |
| `time_resolution` | string | "hour" | Resolution: "minute", "hour", or "day" |
| `workers` | int | 4 | Number of worker goroutines |
| `batch_size` | int | 100 | Batch size for index updates |

## Ports

### Inputs

| Name | Type | Subject | Description |
|------|------|---------|-------------|
| entity_watch | kv-watch | ENTITY_STATES | Watch entity state changes |

### Outputs

| Name | Type | Subject | Description |
|------|------|---------|-------------|
| temporal_index | kv | TEMPORAL_INDEX | Temporal index |

## Time Resolution Guide

| Resolution | Key Format | Use Case |
|------------|------------|----------|
| minute | 2024-01-15T10:30 | Real-time monitoring |
| hour | 2024-01-15T10 | Operational dashboards |
| day | 2024-01-15 | Historical analysis |

## Index Structure

The TEMPORAL_INDEX uses time bucket as key:

```json
{
  "time_bucket": "2024-01-15T10",
  "entities": [
    {
      "entity_id": "c360.logistics.warehouse.sensor.temperature.temp-001",
      "timestamp": "2024-01-15T10:30:00Z",
      "event_type": "updated"
    }
  ]
}
```

## Timestamp Extraction

The component extracts timestamps from:

- Entity `updated_at` field
- Entity `created_at` field
- `timestamp` predicate in triples
- `observation.timestamp` predicate

## Temporal Queries

The gateway component uses this index for:

- **Time range**: Find entities modified between two timestamps
- **Time bucket**: Find entities in specific hour/day
- **Recent**: Find entities modified in last N hours

## Dependencies

### Upstream
- `graph-ingest` - produces ENTITY_STATES

### Downstream
- `graph-gateway` - queries temporal index

## Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `graph_temporal_indexed_total` | counter | Total entities indexed |
| `graph_temporal_buckets_active` | gauge | Active time buckets |
| `graph_temporal_errors_total` | counter | Indexing errors |

## Health

The component reports healthy when:
- KV watch subscription is active
- TEMPORAL_INDEX bucket is accessible
- Index updates completing successfully
