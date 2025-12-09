# Performance Tuning

Guidelines for optimizing SemStreams performance across different deployment scenarios.

## Latency Expectations

### LLM Enhancement

| Environment | Latency per Community | Total (54 communities) |
|-------------|----------------------|------------------------|
| Local Ollama (CPU) | 2-5s | 108-270s |
| Local Ollama (GPU) | 0.3-1s | 16-54s |
| Cloud API (GPT-4) | 1-3s | 54-162s |
| Cloud API (GPT-4o-mini) | 0.5-1s | 27-54s |

### Statistical Summarization

| Operation | Expected Latency |
|-----------|-----------------|
| TF-IDF keyword extraction | < 10ms |
| PageRank representative selection | < 5ms |
| Summary generation | < 1ms |
| Total per community | < 20ms |

### Entity Processing

| Operation | Expected Latency |
|-----------|-----------------|
| Message deserialization | < 1ms |
| Triple extraction | < 1ms |
| Entity update (KV put) | 1-5ms |
| Index updates (all indexes) | 5-20ms |
| Total per entity | 10-30ms |

### Community Detection (LPA)

| Graph Size | Expected Latency |
|------------|-----------------|
| 100 entities | < 100ms |
| 1,000 entities | 100-500ms |
| 10,000 entities | 1-5s |
| 100,000 entities | 10-60s |

## Tier-Specific Tuning

### Tier 0 (Rules Only)

Minimal resource usage. Focus on rule evaluation throughput.

```json
{
  "graph": {
    "config": {
      "workers": 4,
      "queue_size": 5000,
      "clustering": {"enabled": false},
      "indexer": {
        "embedding": {"enabled": false}
      }
    }
  }
}
```

**Recommendations:**
- Fewer workers (rules are CPU-bound)
- Smaller queue (no clustering backlog)
- Disable embedding extraction

### Tier 1 (Statistical Clustering)

Moderate resources. Balance entity throughput and clustering frequency.

```json
{
  "graph": {
    "config": {
      "workers": 8,
      "queue_size": 10000,
      "clustering": {
        "enabled": true,
        "enhancement": {"enabled": false},
        "schedule": {
          "detection_interval": "60s",
          "entity_change_threshold": 500
        }
      }
    }
  }
}
```

**Recommendations:**
- Increase detection interval for stability
- Higher change threshold for batch processing
- More workers for entity throughput

### Tier 2 (Full Semantic)

Full resource utilization. Tune for LLM latency and embedding throughput.

```json
{
  "graph": {
    "config": {
      "workers": 16,
      "queue_size": 20000,
      "clustering": {
        "enabled": true,
        "enhancement": {
          "enabled": true,
          "workers": 5
        },
        "schedule": {
          "enhancement_window": "120s",
          "enhancement_window_mode": "soft"
        }
      }
    }
  }
}
```

**Recommendations:**
- More entity workers for throughput
- Tune enhancement workers to LLM capacity
- Use soft enhancement window for freshness

## Worker Tuning

### Entity Processing Workers

Controls concurrent entity message processing:

```json
{
  "workers": 8
}
```

| Scenario | Recommended Workers |
|----------|---------------------|
| Low throughput (< 100 msg/s) | 2-4 |
| Medium throughput (100-1000 msg/s) | 8-16 |
| High throughput (> 1000 msg/s) | 16-32 |

**Considerations:**
- Each worker holds messages in memory
- Too many workers = memory pressure
- Too few workers = backlog growth

### Enhancement Workers

Controls concurrent LLM API calls:

```json
{
  "enhancement": {
    "workers": 3
  }
}
```

| LLM Backend | Recommended Workers |
|-------------|---------------------|
| Local CPU | 1-2 |
| Local GPU | 3-5 |
| Cloud API (rate limited) | 3-5 |
| Cloud API (high quota) | 5-10 |

**Considerations:**
- More workers = more concurrent API calls
- Cloud APIs may rate limit
- Local models may not parallelize well

## Queue Sizing

```json
{
  "queue_size": 10000
}
```

### Sizing Guidelines

| Message Rate | Queue Size |
|--------------|------------|
| < 100 msg/s | 1,000-5,000 |
| 100-1000 msg/s | 10,000-50,000 |
| > 1000 msg/s | 50,000-100,000 |

### Queue Overflow

When queue is full:
1. New messages are dropped
2. NATS redelivers (if JetStream)
3. Monitor `queue_dropped_total` metric

## Detection Scheduling

### Detection Interval

Time between scheduled detection runs:

```json
{
  "schedule": {
    "detection_interval": "30s"
  }
}
```

| Use Case | Recommended Interval |
|----------|---------------------|
| Real-time dashboards | 10-30s |
| Batch processing | 60-300s |
| Development | 5-10s |

### Change Threshold

Trigger detection after N entity changes:

```json
{
  "schedule": {
    "entity_change_threshold": 100
  }
}
```

| Graph Size | Recommended Threshold |
|------------|----------------------|
| < 1,000 entities | 50-100 |
| 1,000-10,000 entities | 100-500 |
| > 10,000 entities | 500-1000 |

### Minimum Interval

Burst protection:

```json
{
  "schedule": {
    "min_detection_interval": "5s"
  }
}
```

Prevents detection from running too frequently during high update rates.

## Enhancement Window

### Mode Selection

| Mode | Trade-off |
|------|-----------|
| `blocking` | Best summary quality, lowest freshness |
| `soft` | Balanced quality and freshness |
| `none` | Best freshness, may invalidate summaries |

### Window Duration

```json
{
  "schedule": {
    "enhancement_window": "120s"
  }
}
```

| LLM Speed | Recommended Window |
|-----------|-------------------|
| Fast (< 1s/community) | 30-60s |
| Medium (1-3s/community) | 60-120s |
| Slow (> 3s/community) | 120-300s |

## Cache Tuning

### Query Cache

```json
{
  "querier": {
    "cache_enabled": true,
    "cache_ttl": "15m",
    "max_cache_size": 5000
  }
}
```

| Access Pattern | Cache TTL | Cache Size |
|----------------|-----------|------------|
| Real-time queries | 1-5m | Small (1000) |
| Dashboard views | 5-15m | Medium (5000) |
| Batch analysis | 30-60m | Large (10000) |

### Object Store Cache

```json
{
  "objectstore": {
    "data_cache": {
      "enabled": true,
      "strategy": "hybrid",
      "max_size": 5000,
      "ttl": "1h"
    }
  }
}
```

## Index Performance

### Batch Processing

```json
{
  "indexer": {
    "batch_processing": {
      "size": 50
    }
  }
}
```

| Update Rate | Batch Size |
|-------------|------------|
| Low | 10-25 |
| Medium | 50-100 |
| High | 100-200 |

Larger batches = fewer KV operations, higher latency.

### Index Selection

Disable unused indexes for better performance:

```json
{
  "indexer": {
    "indexes": {
      "predicate": true,
      "incoming": true,
      "outgoing": true,
      "alias": false,
      "spatial": false,
      "temporal": false
    }
  }
}
```

Each enabled index adds write overhead.

## KV Bucket Tuning

### Retry Configuration

```json
{
  "data_manager": {
    "kv_options": {
      "max_retries": 15,
      "timeout": "10s"
    }
  }
}
```

| Network Quality | Max Retries | Timeout |
|-----------------|-------------|---------|
| Local/fast | 3-5 | 1-5s |
| Remote/slow | 10-15 | 5-10s |
| Unreliable | 15-20 | 10-30s |

### History Depth

```go
jetstream.KeyValueConfig{
    History: 10,  // Versions to keep
}
```

More history = more storage, better debugging.

## Memory Management

### Message Buffer

Each worker buffers messages:

```
Memory per worker ≈ queue_size_per_worker * avg_message_size
```

For 8 workers with 1KB messages and 1000 buffer:
```
8 * 1000 * 1KB = 8MB message buffer
```

### Entity Cache

Query manager caches entities:

```
Cache memory ≈ max_cache_size * avg_entity_size
```

For 5000 entities at 2KB average:
```
5000 * 2KB = 10MB entity cache
```

## Metrics to Monitor

### Throughput

| Metric | Description | Target |
|--------|-------------|--------|
| `messages_processed_total` | Entity messages processed | Matches input rate |
| `communities_detected_total` | Detection runs | Regular intervals |
| `enhancement_success_total` | LLM enhancements | Matches detection rate |

### Latency

| Metric | Description | Target |
|--------|-------------|--------|
| `message_processing_seconds` | Entity processing time | < 100ms P99 |
| `detection_duration_seconds` | LPA detection time | < 5s P99 |
| `enhancement_latency_seconds` | LLM call time | < 5s P99 |

### Resource Usage

| Metric | Description | Target |
|--------|-------------|--------|
| `queue_depth` | Message queue size | < 50% of queue_size |
| `enhancement_queue_depth` | Pending LLM work | < 10 |
| `cache_hit_ratio` | Query cache effectiveness | > 80% |

## Bottleneck Identification

### High Queue Depth

**Symptom**: `queue_depth` consistently high

**Causes**:
- Too few workers
- Slow KV operations
- High message rate

**Solutions**:
1. Increase `workers`
2. Check NATS latency
3. Increase `queue_size` (temporary)

### High Detection Latency

**Symptom**: `detection_duration_seconds` > 10s

**Causes**:
- Large graph
- Too many iterations
- Slow graph queries

**Solutions**:
1. Reduce `max_iterations`
2. Increase `min_detection_interval`
3. Use `PredicateGraphProvider` for subset

### LLM Backlog

**Symptom**: `enhancement_queue_depth` growing

**Causes**:
- Slow LLM backend
- Too few enhancement workers
- Frequent detection runs

**Solutions**:
1. Use faster LLM model
2. Increase `enhancement.workers`
3. Increase detection interval

## Production Recommendations

### Startup

1. Start with statistical-only (no LLM)
2. Validate entity throughput
3. Enable clustering
4. Tune detection schedule
5. Enable LLM enhancement last

### Monitoring

1. Set up Prometheus scraping
2. Create dashboard for key metrics
3. Alert on queue overflow
4. Alert on high latency percentiles

### Capacity Planning

| Component | Scaling Factor |
|-----------|---------------|
| Workers | Linear with message rate |
| Queue size | Linear with burst size |
| Enhancement workers | Linear with LLM capacity |
| Cache size | Based on working set |

## Benchmarking

### Message Throughput

```bash
# Generate load
for i in {1..10000}; do
  nats pub events.graph.entity.sensor '{"id":"sensor-'$i'","triples":[...]}'
done

# Monitor throughput
curl -s localhost:9090/metrics | grep messages_processed
```

### Detection Performance

```bash
# Trigger detection
nats pub system.clustering.trigger '{}'

# Monitor duration
curl -s localhost:9090/metrics | grep detection_duration
```

### LLM Latency

```bash
# Monitor enhancement latency distribution
curl -s localhost:9090/metrics | grep enhancement_latency_seconds_bucket
```

## Related Documentation

- [Clustering](01-clustering.md) - Detection algorithm details
- [LLM Enhancement](02-llm-enhancement.md) - LLM configuration
- [Configuration](../reference/configuration.md) - All configuration options
