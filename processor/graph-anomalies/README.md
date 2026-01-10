# graph-anomalies

> **⚠️ DEPRECATED**: This component is deprecated. Use `graph-clustering` with `enable_structural: true` and `enable_anomaly_detection: true` instead. The functionality has been merged into graph-clustering to eliminate race conditions and enable within-community anomaly scoping.

Graph cluster/community anomaly detection component using k-core decomposition and pivot-based distance indexing.

## Tier

**Tier: STATISTICAL (Tier 1)** - Required for Statistical and Semantic tiers.

This component is NOT used in Structural (Tier 0) deployments.

## Overview

The graph-anomalies component detects anomalies within clusters and communities by computing structural metrics. It watches the OUTGOING_INDEX and INCOMING_INDEX buckets and maintains the STRUCTURAL_INDEX bucket with computed metrics used for anomaly detection.

## Architecture

```
OUTGOING_INDEX ──┐
                 ├── kv-watch ──► graph-anomalies ──► STRUCTURAL_INDEX
INCOMING_INDEX ──┘
```

## Features

- **K-Core Decomposition**: Identifies nodes with unusual connectivity patterns
- **Pivot Distance Indexing**: Detects nodes with anomalous distance profiles
- **Community Outlier Detection**: Flags entities that don't fit their cluster
- **Periodic Recomputation**: Configurable interval for index updates

## Configuration

```json
{
  "name": "graph-anomalies",
  "type": "processor",
  "config": {
    "compute_interval": "1h",
    "pivot_count": 16,
    "max_hop_distance": 10,
    "compute_on_startup": true,
    "ports": {
      "inputs": [
        {"name": "outgoing_watch", "type": "kv-watch", "subject": "OUTGOING_INDEX"},
        {"name": "incoming_watch", "type": "kv-watch", "subject": "INCOMING_INDEX"}
      ],
      "outputs": [
        {"name": "structural_index", "type": "kv-write", "subject": "STRUCTURAL_INDEX"}
      ]
    }
  }
}
```

### Configuration Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `compute_interval` | duration | `1h` | Interval between recomputation cycles |
| `pivot_count` | int | `16` | Number of pivot nodes for distance indexing |
| `max_hop_distance` | int | `10` | Maximum BFS traversal depth |
| `compute_on_startup` | bool | `true` | Compute indices immediately on startup |

## KV Buckets

### Input Buckets

- **OUTGOING_INDEX**: Entity outgoing edge adjacency lists
- **INCOMING_INDEX**: Entity incoming edge adjacency lists

### Output Bucket

- **STRUCTURAL_INDEX**: Stores computed structural metrics for anomaly detection

## Anomaly Detection Metrics

### K-Core Number

The k-core of a graph is the maximal subgraph where every vertex has degree at least k. Nodes with k-core numbers significantly different from their community peers may be anomalies.

Uses:
- Identifying peripheral vs core community members
- Detecting nodes that don't fit their assigned cluster
- Finding bridge nodes between communities

### Pivot Distances

Pivot-based indexing precomputes distances from each node to a set of pivot nodes. Anomalous distance profiles indicate:
- Nodes incorrectly assigned to communities
- Outliers within a cluster
- Potential data quality issues

## Integration

### With graph-clustering

The graph-anomalies component works alongside graph-clustering:
- graph-clustering assigns nodes to communities
- graph-anomalies detects which nodes are anomalous within their community

### With graph-embedding

Uses embedding similarity to validate structural anomalies:
- Structural outliers with similar embeddings may be legitimate
- Structural outliers with dissimilar embeddings are likely true anomalies

## Deployment

### Statistical Tier (Tier 1)

```json
{
  "components": [
    {"type": "processor", "name": "graph-ingest"},
    {"type": "processor", "name": "graph-index"},
    {"type": "processor", "name": "graph-embedding"},
    {"type": "processor", "name": "graph-clustering"},
    {"type": "processor", "name": "graph-anomalies"},
    {"type": "gateway", "name": "graph-gateway"}
  ]
}
```

### Semantic Tier (Tier 2)

Same as Statistical, with HTTP embedder and LLM enhancement enabled.

### NOT in Structural Tier (Tier 0)

The graph-anomalies component requires clustering results and is not used in the Structural tier.

## Migration Guide

This component is deprecated. To migrate to the merged graph-clustering component:

### Before (separate components)

```json
{
  "graph-clustering": {
    "config": {
      "detection_interval": "30s",
      "min_community_size": 2
    }
  },
  "graph-anomalies": {
    "config": {
      "compute_interval": "1h",
      "pivot_count": 16,
      "max_hop_distance": 10,
      "enable_detection": true
    }
  }
}
```

### After (merged)

```json
{
  "graph-clustering": {
    "config": {
      "detection_interval": "30s",
      "min_community_size": 2,
      "enable_structural": true,
      "pivot_count": 16,
      "max_hop_distance": 10,
      "enable_anomaly_detection": true,
      "anomaly_config": {
        "enabled": true,
        "max_anomalies_per_run": 100,
        "core_anomaly": {
          "enabled": true,
          "min_core_level": 2
        },
        "semantic_gap": {
          "enabled": true,
          "similarity_threshold": 0.7
        }
      }
    }
  }
}
```

### Benefits of Migration

1. **Guaranteed ordering**: Structural computation happens after LPA completes
2. **Within-community scoping**: Core isolation detection is scoped to communities
3. **Simplified coordination**: Single component manages the full pipeline
4. **Semantic gap detection**: Uses embedding similarity when available
