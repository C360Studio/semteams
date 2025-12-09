# Configuration Reference

Complete configuration options for SemStreams components.

## Platform Configuration

Top-level platform identification:

```json
{
  "version": "1.1.0",
  "platform": {
    "org": "c360",
    "id": "semstreams-production",
    "type": "semantic-processing",
    "region": "us-east-1",
    "instance_id": "prod-001",
    "environment": "production"
  }
}
```

| Field | Required | Description |
|-------|----------|-------------|
| `version` | Yes | Configuration schema version |
| `platform.org` | Yes | Organization ID (first segment of entity IDs) |
| `platform.id` | Yes | Deployment identifier |
| `platform.type` | No | Deployment type description |
| `platform.region` | No | Deployment region |
| `platform.instance_id` | No | Instance identifier |
| `platform.environment` | No | Environment: `production`, `staging`, `test` |

## NATS Configuration

```json
{
  "nats": {
    "urls": ["nats://localhost:4222"],
    "credentials_file": "/etc/nats/creds",
    "tls": {
      "enabled": false,
      "cert_file": "/etc/nats/cert.pem",
      "key_file": "/etc/nats/key.pem"
    }
  }
}
```

| Field | Default | Description |
|-------|---------|-------------|
| `urls` | `["nats://localhost:4222"]` | NATS server URLs |
| `credentials_file` | - | Path to credentials file |
| `tls.enabled` | `false` | Enable TLS |
| `tls.cert_file` | - | TLS certificate path |
| `tls.key_file` | - | TLS private key path |

## Graph Processor Configuration

The graph processor transforms entity messages into a semantic graph with indexing, clustering, and optional LLM enhancement.

### Basic Options

```json
{
  "graph": {
    "type": "processor",
    "name": "graph-processor",
    "enabled": true,
    "config": {
      "workers": 8,
      "queue_size": 5000,
      "input_subject": "events.graph.entity.*",
      "stream_name": "GRAPH_EVENTS",
      "consumer_name": "graph-processor"
    }
  }
}
```

| Field | Default | Description |
|-------|---------|-------------|
| `workers` | 10 | Number of worker goroutines |
| `queue_size` | 10000 | Internal message queue capacity |
| `input_subject` | - | NATS subject pattern for entity messages |
| `stream_name` | - | JetStream stream name |
| `consumer_name` | - | JetStream consumer name |

### Message Handler

```json
{
  "message_handler": {
    "processing_timeout": "5s",
    "max_retries": 5
  }
}
```

| Field | Default | Description |
|-------|---------|-------------|
| `processing_timeout` | `5s` | Per-message processing timeout |
| `max_retries` | 3 | Retry count for failed messages |

### Data Manager

Controls entity storage and triple management:

```json
{
  "data_manager": {
    "kv_bucket": "ENTITY_STATES",
    "kv_options": {
      "max_retries": 15,
      "timeout": "10s"
    },
    "enable_edge_tracking": true,
    "edge_batch_size": 100
  }
}
```

| Field | Default | Description |
|-------|---------|-------------|
| `kv_bucket` | `ENTITY_STATES` | Primary entity storage bucket |
| `kv_options.max_retries` | 5 | KV operation retry count |
| `kv_options.timeout` | `5s` | KV operation timeout |
| `enable_edge_tracking` | `true` | Track edge updates for indexing |
| `edge_batch_size` | 50 | Batch size for edge operations |

### Index Manager

Controls secondary index generation:

```json
{
  "indexer": {
    "indexes": {
      "predicate": true,
      "incoming": true,
      "outgoing": true,
      "alias": true,
      "spatial": true,
      "temporal": true
    },
    "buckets": {
      "predicate": "PREDICATE_INDEX",
      "incoming": "INCOMING_INDEX",
      "outgoing": "OUTGOING_INDEX",
      "alias": "ALIAS_INDEX",
      "spatial": "SPATIAL_INDEX",
      "temporal": "TEMPORAL_INDEX"
    },
    "batch_processing": {
      "size": 50
    }
  }
}
```

| Index | Purpose | Default Bucket |
|-------|---------|----------------|
| `predicate` | Find entities by predicate | `PREDICATE_INDEX` |
| `incoming` | Find entities that reference a target | `INCOMING_INDEX` |
| `outgoing` | Find entities a source references | `OUTGOING_INDEX` |
| `alias` | Resolve friendly names to entity IDs | `ALIAS_INDEX` |
| `spatial` | Geospatial lookup by geohash | `SPATIAL_INDEX` |
| `temporal` | Time-based entity lookup | `TEMPORAL_INDEX` |

### Embedding Configuration

Controls which triples generate text for embeddings:

```json
{
  "indexer": {
    "embedding": {
      "enabled": true,
      "text_fields": [
        "dc.title",
        "dc.subject",
        "dc.description",
        "title",
        "description",
        "summary",
        "name",
        "observation.text"
      ]
    }
  }
}
```

| Field | Default | Description |
|-------|---------|-------------|
| `enabled` | `false` | Enable embedding extraction |
| `text_fields` | `[]` | Predicates containing text for embeddings |

### Query Manager

Controls graph query caching:

```json
{
  "querier": {
    "cache_enabled": true,
    "cache_ttl": "15m",
    "max_cache_size": 5000,
    "query_timeout": "10s",
    "result_limit": 1000
  }
}
```

| Field | Default | Description |
|-------|---------|-------------|
| `cache_enabled` | `true` | Enable query result caching |
| `cache_ttl` | `5m` | Cache entry lifetime |
| `max_cache_size` | 1000 | Maximum cache entries |
| `query_timeout` | `5s` | Query execution timeout |
| `result_limit` | 100 | Maximum results per query |

## Clustering Configuration

Controls community detection and LLM enhancement.

### Algorithm Options

```json
{
  "clustering": {
    "enabled": true,
    "algorithm": {
      "max_iterations": 100,
      "levels": 3
    }
  }
}
```

| Field | Default | Description |
|-------|---------|-------------|
| `enabled` | `false` | Enable community detection |
| `max_iterations` | 100 | Maximum LPA iterations per level |
| `levels` | 3 | Hierarchical community levels (0 = finest) |

### Schedule Options

Control when detection runs:

```json
{
  "schedule": {
    "initial_delay": "10s",
    "detection_interval": "30s",
    "min_detection_interval": "5s",
    "entity_change_threshold": 100,
    "min_entities": 10,
    "min_embedding_coverage": 0.5,
    "enhancement_window": "120s",
    "enhancement_window_mode": "blocking"
  }
}
```

| Field | Default | Description |
|-------|---------|-------------|
| `initial_delay` | `10s` | Wait before first detection run |
| `detection_interval` | `30s` | Maximum time between detection runs |
| `min_detection_interval` | `5s` | Minimum time between runs (burst protection) |
| `entity_change_threshold` | 100 | Trigger detection after N entity changes |
| `min_entities` | 10 | Minimum entities required for detection |
| `min_embedding_coverage` | 0.5 | Required embedding ratio for semantic clustering |
| `enhancement_window` | `0` | Pause detection duration for LLM (0 = disabled) |
| `enhancement_window_mode` | `blocking` | Window mode: `blocking`, `soft`, `none` |

### Enhancement Window Modes

| Mode | Behavior |
|------|----------|
| `blocking` | Hard pause until window expires or all communities reach terminal status |
| `soft` | Allow detection if entity changes exceed threshold during window |
| `none` | No enhancement window (original behavior) |

### LLM Enhancement

```json
{
  "enhancement": {
    "enabled": true,
    "workers": 3,
    "domain": "default",
    "summarizer_endpoint": "http://seminstruct:8083",
    "llm": {
      "provider": "openai",
      "base_url": "http://shimmy:8080/v1",
      "model": "mistral-7b-instruct",
      "timeout_seconds": 60,
      "max_retries": 3
    }
  }
}
```

| Field | Default | Description |
|-------|---------|-------------|
| `enabled` | `false` | Enable LLM enhancement |
| `workers` | 3 | Concurrent enhancement goroutines |
| `domain` | `default` | Prompt domain: `default`, `iot`, custom |
| `summarizer_endpoint` | - | Endpoint for seminstruct service |
| `llm.provider` | `none` | Backend: `openai` (any compatible), `none` |
| `llm.base_url` | - | LLM service URL |
| `llm.model` | - | Model identifier |
| `llm.timeout_seconds` | 60 | Per-request timeout |
| `llm.max_retries` | 3 | Retry count for transient failures |

### Semantic Edges

Virtual edges from embedding similarity:

```json
{
  "semantic_edges": {
    "enabled": true,
    "similarity_threshold": 0.6,
    "max_virtual_neighbors": 5
  }
}
```

| Field | Default | Description |
|-------|---------|-------------|
| `enabled` | `false` | Enable virtual edges from embeddings |
| `similarity_threshold` | 0.6 | Minimum cosine similarity for edge |
| `max_virtual_neighbors` | 5 | Limit virtual neighbors per entity |

### Inference

Automatic triple creation from community relationships:

```json
{
  "inference": {
    "enabled": false,
    "min_community_size": 2,
    "max_inferred_per_community": 50
  }
}
```

| Field | Default | Description |
|-------|---------|-------------|
| `enabled` | `false` | Create inferred triples from communities |
| `min_community_size` | 2 | Skip singleton communities |
| `max_inferred_per_community` | 50 | Prevent O(n^2) explosion |

## Rule Processor Configuration

Controls the rules engine for state-based triggers.

### Basic Options

```json
{
  "rule": {
    "type": "processor",
    "name": "rule-processor",
    "enabled": true,
    "config": {
      "entity_watch_patterns": ["c360.>"],
      "enable_graph_integration": true,
      "graph_event_subject_prefix": "events.graph.entity"
    }
  }
}
```

| Field | Default | Description |
|-------|---------|-------------|
| `entity_watch_patterns` | `[">"]` | NATS patterns for entity watching |
| `enable_graph_integration` | `false` | Enable add_triple/remove_triple actions |
| `graph_event_subject_prefix` | - | Prefix for graph entity events |
| `buffer_window_size` | `10m` | Message buffer window |
| `alert_cooldown_period` | `2m` | Default cooldown between triggers |

### Inline Rules

```json
{
  "inline_rules": [
    {
      "id": "low-battery-alert",
      "type": "expression",
      "name": "Low Battery Alert",
      "description": "Triggers when battery level drops below 20%",
      "enabled": true,
      "conditions": [
        {
          "field": "battery.level",
          "operator": "lte",
          "value": 20.0,
          "required": true
        }
      ],
      "logic": "and",
      "cooldown": "30s",
      "metadata": {
        "severity": "warning",
        "category": "power"
      },
      "on_enter": [
        {"type": "add_triple", "predicate": "alert.status", "object": "low_battery"},
        {"type": "publish", "subject": "alerts.battery.low"}
      ],
      "on_exit": [
        {"type": "remove_triple", "predicate": "alert.status"}
      ]
    }
  ]
}
```

See [Rules Documentation](../rules/02-rule-syntax.md) for complete rule syntax.

### External Rules Files

```json
{
  "rules_files": [
    "/etc/semstreams/rules/alerts.json",
    "/etc/semstreams/rules/relationships.json"
  ]
}
```

## Services Configuration

### Metrics Service

```json
{
  "metrics": {
    "name": "metrics",
    "enabled": true,
    "config": {
      "port": 9090,
      "path": "/metrics",
      "include_go_metrics": true
    }
  }
}
```

| Field | Default | Description |
|-------|---------|-------------|
| `port` | 9090 | Prometheus metrics port |
| `path` | `/metrics` | Metrics endpoint path |
| `include_go_metrics` | `true` | Include Go runtime metrics |

### Service Manager

```json
{
  "service-manager": {
    "name": "service-manager",
    "enabled": true,
    "config": {
      "http_port": 8080,
      "swagger_ui": true,
      "server_info": {
        "title": "SemStreams API",
        "description": "Semantic processing API",
        "version": "0.7.0"
      }
    }
  }
}
```

### Message Logger

```json
{
  "message-logger": {
    "name": "message-logger",
    "enabled": true,
    "config": {
      "buffer_size": 10000,
      "monitor_subjects": ["*"],
      "enable_kv_query": true,
      "log_level": "INFO"
    }
  }
}
```

## Input Components

### File Input

```json
{
  "file_input": {
    "type": "input",
    "name": "file_input",
    "enabled": true,
    "config": {
      "ports": {
        "outputs": [
          {"name": "out", "subject": "raw.data", "type": "nats"}
        ]
      },
      "path": "/data/input.jsonl",
      "format": "jsonl",
      "interval": "100ms",
      "loop": false
    }
  }
}
```

| Field | Default | Description |
|-------|---------|-------------|
| `path` | - | Path to input file |
| `format` | `jsonl` | File format: `jsonl`, `json` |
| `interval` | `0` | Delay between messages (0 = no delay) |
| `loop` | `false` | Repeat file when finished |

### UDP Input

```json
{
  "udp": {
    "type": "input",
    "name": "udp",
    "enabled": true,
    "config": {
      "bind": "0.0.0.0",
      "port": 14550,
      "buffer_size": 65536
    }
  }
}
```

## Output Components

### File Output

```json
{
  "file": {
    "type": "output",
    "name": "file",
    "enabled": true,
    "config": {
      "directory": "/var/log/semstreams",
      "file_prefix": "entities",
      "format": "jsonl"
    }
  }
}
```

### HTTP POST Output

```json
{
  "httppost": {
    "type": "output",
    "name": "httppost",
    "enabled": true,
    "config": {
      "url": "http://webhook.example.com/events",
      "retry_max": 3
    }
  }
}
```

### WebSocket Output

```json
{
  "websocket": {
    "type": "output",
    "name": "websocket",
    "enabled": true,
    "config": {
      "http_port": 8082,
      "path": "/ws"
    }
  }
}
```

## Storage Components

### Object Store

```json
{
  "objectstore": {
    "type": "storage",
    "name": "objectstore",
    "enabled": true,
    "config": {
      "bucket_name": "semstreams_store",
      "data_cache": {
        "enabled": true,
        "strategy": "hybrid",
        "max_size": 5000,
        "ttl": "1h",
        "cleanup_interval": "5m"
      }
    }
  }
}
```

| Field | Default | Description |
|-------|---------|-------------|
| `bucket_name` | - | NATS Object Store bucket name |
| `data_cache.enabled` | `true` | Enable in-memory cache |
| `data_cache.strategy` | `lru` | Cache strategy: `lru`, `hybrid` |
| `data_cache.max_size` | 1000 | Maximum cache entries |
| `data_cache.ttl` | `30m` | Cache entry lifetime |
| `data_cache.cleanup_interval` | `5m` | Cache cleanup frequency |

## Tier-Specific Configurations

### Tier 0 (Rules Only)

Minimal configuration for rule-based processing without clustering:

```json
{
  "graph": {
    "config": {
      "clustering": {"enabled": false},
      "indexer": {
        "embedding": {"enabled": false}
      }
    }
  },
  "rule": {
    "config": {
      "enable_graph_integration": true
    }
  }
}
```

### Tier 1 (Rules + Statistical Clustering)

Adds clustering without LLM:

```json
{
  "graph": {
    "config": {
      "clustering": {
        "enabled": true,
        "enhancement": {"enabled": false}
      }
    }
  }
}
```

### Tier 2 (Full Semantic)

Complete configuration with LLM enhancement:

```json
{
  "graph": {
    "config": {
      "clustering": {
        "enabled": true,
        "enhancement": {
          "enabled": true,
          "summarizer_endpoint": "http://seminstruct:8083"
        },
        "semantic_edges": {"enabled": true}
      },
      "indexer": {
        "embedding": {"enabled": true}
      }
    }
  }
}
```

## Environment Variables

Configuration values can reference environment variables:

```json
{
  "nats": {
    "urls": ["${NATS_URL:-nats://localhost:4222}"]
  },
  "clustering": {
    "enhancement": {
      "llm": {
        "base_url": "${LLM_BASE_URL}"
      }
    }
  }
}
```

## Configuration Validation

SemStreams validates configuration at startup. Common errors:

| Error | Cause | Fix |
|-------|-------|-----|
| `missing input_subject` | No input subject specified | Add `input_subject` to processor config |
| `invalid entity_watch_patterns` | Invalid NATS pattern | Check pattern syntax |
| `kv bucket not found` | Referenced bucket doesn't exist | Ensure NATS JetStream is running |
| `enhancement enabled but no endpoint` | LLM enabled without endpoint | Add `summarizer_endpoint` or `llm.base_url` |

## Complete Example

See `configs/semantic-kitchen-sink.json` for a comprehensive example with all components configured.
