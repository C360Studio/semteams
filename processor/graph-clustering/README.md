# graph-clustering

Community detection component for the graph subsystem.

## Overview

The `graph-clustering` component performs community detection on the entity graph using Label Propagation Algorithm (LPA). It identifies clusters of related entities and optionally enhances community descriptions using LLM.

## Architecture

```
                    ┌───────────────────┐
ENTITY_STATES ─────►│                   │
   (KV watch)       │  graph-clustering ├──► COMMUNITY_INDEX (KV)
                    │                   │
                    └─────────┬─────────┘
                              │ (reads)
              ┌───────────────┼───────────────┐
              ▼               ▼               ▼
       OUTGOING_INDEX  INCOMING_INDEX  EMBEDDINGS_CACHE
```

## Features

- **Label Propagation Algorithm (LPA)**: Efficient community detection
- **Configurable Scheduling**: Timer-based or event-count triggered
- **LLM Enhancement**: Optional community summarization using LLM
- **Semantic Edges**: Uses embedding similarity for virtual edges
- **Inferred Relationships**: Creates edges between community members

## Configuration

```json
{
  "type": "processor",
  "name": "graph-clustering",
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
          "name": "communities",
          "subject": "COMMUNITY_INDEX",
          "type": "kv"
        }
      ]
    },
    "detection_interval": "30s",
    "batch_size": 100,
    "enable_llm": true,
    "llm_endpoint": "http://seminstruct:8083/v1",
    "min_community_size": 2,
    "max_iterations": 100
  }
}
```

### Configuration Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `ports` | object | required | Port configuration |
| `detection_interval` | duration | "30s" | Time between detection runs |
| `batch_size` | int | 100 | Entity change count to trigger detection |
| `enable_llm` | bool | false | Enable LLM community summarization |
| `llm_endpoint` | string | "" | LLM API endpoint (required if enable_llm) |
| `min_community_size` | int | 2 | Minimum entities to form community |
| `max_iterations` | int | 100 | Max LPA iterations |

## Ports

### Inputs

| Name | Type | Subject | Description |
|------|------|---------|-------------|
| entity_watch | kv-watch | ENTITY_STATES | Watch for entity changes |

### Outputs

| Name | Type | Subject | Description |
|------|------|---------|-------------|
| communities | kv | COMMUNITY_INDEX | Community detection results |

## Scheduling

Community detection triggers when either condition is met:

1. **Timer**: `detection_interval` elapsed since last run
2. **Batch**: `batch_size` entity changes accumulated

This ensures:
- Regular detection even with low activity
- Responsive detection during high activity

## Community Index Structure

```json
{
  "community_id": "comm-abc123",
  "members": [
    "c360.logistics.warehouse.sensor.temperature.temp-001",
    "c360.logistics.warehouse.sensor.temperature.temp-002",
    "c360.logistics.warehouse.sensor.humidity.hum-001"
  ],
  "centroid": "c360.logistics.warehouse.sensor.temperature.temp-001",
  "size": 3,
  "density": 0.85,
  "summary": "Cold storage environmental sensors",
  "created_at": "2024-01-15T10:30:00Z",
  "updated_at": "2024-01-15T11:00:00Z"
}
```

## Label Propagation Algorithm

LPA works by:

1. Initialize each entity with unique label
2. Iteratively update labels to match most common neighbor label
3. Stop when labels stabilize or max_iterations reached
4. Entities with same label form a community

The algorithm considers:
- Structural edges (from OUTGOING/INCOMING indexes)
- Semantic edges (from embedding similarity in EMBEDDINGS_CACHE)

## Dependencies

### Upstream (reads during detection)
- `graph-ingest` - watches ENTITY_STATES for triggers
- `graph-index` - reads indexes for graph structure
- `graph-embedding` - reads embeddings for semantic similarity

### Downstream
- `graph-gateway` - queries community data

### External
- LLM API service (if LLM enhancement enabled)

## Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `graph_clustering_runs_total` | counter | Total detection runs |
| `graph_clustering_communities_detected` | gauge | Current community count |
| `graph_clustering_duration_seconds` | histogram | Detection run duration |
| `graph_clustering_llm_enhancements_total` | counter | LLM enhancement calls |

## Health

The component reports healthy when:
- KV watch subscription is active
- Detection runs complete within timeout
- LLM API reachable (if enabled)
