# Graph Processor Architecture

The graph processor transforms incoming entity messages into a rich semantic graph with community detection, LLM-enhanced summaries, and relationship inference.

## High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Graph Processor                              │
│                   processor/graph/processor.go                       │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  ┌─────────────┐   ┌──────────────┐   ┌──────────────────────────┐ │
│  │ NATS Input  │──▶│ MessageMgr   │──▶│ DataManager              │ │
│  │ (events)    │   │ (transform)  │   │ (EntityMgr + TripleMgr)  │ │
│  └─────────────┘   └──────────────┘   └───────────┬──────────────┘ │
│                                                    │                │
│                    ┌───────────────────────────────┴────────────┐   │
│                    ▼                                            │   │
│  ┌──────────────────────────┐   ┌────────────────────────────┐  │   │
│  │ IndexManager             │   │ QueryManager               │  │   │
│  │ - Secondary indexes      │   │ - Graph queries            │  │   │
│  │ - KV watchers            │   │ - Entity/edge traversal    │  │   │
│  │ - Index maintenance      │   │ - Caching                  │  │   │
│  └──────────────────────────┘   └────────────────────────────┘  │   │
│                    │                         │                   │   │
│                    ▼                         ▼                   │   │
│  ┌───────────────────────────────────────────────────────────┐  │   │
│  │                 Clustering Subsystem                       │  │   │
│  │                                                            │  │   │
│  │  ┌─────────────────┐   ┌─────────────────────────────────┐│  │   │
│  │  │ LPA Detector    │──▶│ Community Storage (KV)          ││  │   │
│  │  │ - PageRank      │   │ - COMMUNITY_INDEX bucket        ││  │   │
│  │  │ - Jaccard match │   │ - Entity→Community mapping      ││  │   │
│  │  └─────────────────┘   └─────────────────────────────────┘│  │   │
│  │           │                                                │  │   │
│  │           ▼                                                │  │   │
│  │  ┌─────────────────┐   ┌─────────────────────────────────┐│  │   │
│  │  │ Statistical     │──▶│ Enhancement Worker (async)      ││  │   │
│  │  │ Summarizer      │   │ - KV watcher for "statistical"  ││  │   │
│  │  │ - TF-IDF        │   │ - LLM summarization             ││  │   │
│  │  │ - PageRank rep  │   │ - Pause/Resume coordination     ││  │   │
│  │  └─────────────────┘   └─────────────────────────────────┘│  │   │
│  │                                                            │  │   │
│  └───────────────────────────────────────────────────────────┘  │   │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

## Component Reference

| Component | File | Responsibility |
|-----------|------|----------------|
| **Processor** | `processor/graph/processor.go` | Orchestration, lifecycle, configuration |
| **MessageManager** | `processor/graph/messagemanager/` | Message → Entity transformation |
| **DataManager** | `processor/graph/datamanager/` | Entity + Triple CRUD operations |
| **IndexManager** | `processor/graph/indexmanager/` | Secondary indexes, KV watchers |
| **QueryManager** | `processor/graph/querymanager/` | Graph queries with caching |
| **LPADetector** | `processor/graph/clustering/lpa.go` | Label Propagation Algorithm |
| **StatisticalSummarizer** | `processor/graph/clustering/summarizer.go` | TF-IDF keywords, PageRank centrality |
| **LLMSummarizer** | `processor/graph/clustering/summarizer.go` | LLM narrative generation |
| **EnhancementWorker** | `processor/graph/clustering/enhancement_worker.go` | Async LLM via KV watch |
| **CommunityStorage** | `processor/graph/clustering/storage.go` | COMMUNITY_INDEX KV operations |

## KV Buckets

| Bucket | Purpose | Key Pattern |
|--------|---------|-------------|
| `ENTITY_STATES` | Entity storage | `{entity_id}` |
| `PREDICATE_INDEX` | Triple lookup by predicate | `{predicate}` |
| `INCOMING_INDEX` | Inbound relationships | `{entity_id}` |
| `OUTGOING_INDEX` | Outbound relationships | `{entity_id}` |
| `COMMUNITY_INDEX` | Community records | `graph.community.{community_id}` |

## Data Flow: Community Detection

```
1. Entity Changes (MessageManager → DataManager)
       │
       ▼
2. Entity Count Threshold Reached (entityChangeCount > threshold)
       │
       ▼
3. DetectCommunities() Called
       │
       ├── EnhancementWorker.Pause()
       │
       ▼
4. LPA Algorithm Runs
       │
       ├── For each level (0 → levels):
       │     ├── Initialize labels (each entity = own label)
       │     ├── Iterate until convergence/max_iterations
       │     │     └── Each entity adopts dominant neighbor label
       │     └── Group entities by label → Communities
       │
       ▼
5. Summary Preservation (Jaccard matching)
       │
       ├── Match new communities to existing by member overlap
       │     ├── Jaccard ≥ 0.8 → Copy summaries (preserve LLM work)
       │     └── Jaccard < 0.8 → New community (fresh summarization)
       │
       ▼
6. Statistical Summarization (immediate, ~1ms)
       │
       ├── extractKeywords() → TF-IDF weighted terms
       ├── findRepresentativeEntities() → PageRank centrality
       └── generateSummary() → "Community of N entities..."
       │
       ▼
7. Save to COMMUNITY_INDEX KV (status: "statistical")
       │
       ├── EnhancementWorker.Resume()
       │
       ▼
8. EnhancementWorker Picks Up (async, ~2s/community)
       │
       ├── KV watcher sees status="statistical"
       ├── Fetch entities via QueryManager
       ├── Call LLM for narrative summary
       └── Update status to "llm-enhanced" or "llm-failed"
```

## Configuration Reference

### Basic Configuration

```json
{
  "workers": 10,
  "queue_size": 10000,
  "input_subject": "events.graph.entity.*"
}
```

### Clustering Configuration

```json
{
  "clustering": {
    "enabled": true,
    "algorithm": {
      "max_iterations": 100,
      "levels": 3
    },
    "schedule": {
      "initial_delay": "10s",
      "detection_interval": "30s",
      "min_detection_interval": "5s",
      "entity_change_threshold": 100,
      "min_entities": 10,
      "min_embedding_coverage": 0.5,
      "enhancement_window": "120s",
      "enhancement_window_mode": "blocking"
    },
    "enhancement": {
      "enabled": true,
      "workers": 3,
      "domain": "default",
      "llm": {
        "provider": "openai",
        "base_url": "http://shimmy:8080/v1",
        "model": "mistral-7b-instruct",
        "timeout_seconds": 60,
        "max_retries": 3
      }
    },
    "semantic_edges": {
      "enabled": true,
      "similarity_threshold": 0.6,
      "max_virtual_neighbors": 5
    },
    "inference": {
      "enabled": false,
      "min_community_size": 2,
      "max_inferred_per_community": 50
    }
  }
}
```

### Configuration Options Explained

#### Algorithm Options

| Option | Default | Description |
|--------|---------|-------------|
| `max_iterations` | 100 | Maximum LPA iterations per level |
| `levels` | 3 | Hierarchical community levels (0 = finest) |

#### Schedule Options

| Option | Default | Description |
|--------|---------|-------------|
| `initial_delay` | 10s | Wait before first detection run |
| `detection_interval` | 30s | Maximum time between detection runs |
| `min_detection_interval` | 5s | Minimum time between runs (burst protection) |
| `entity_change_threshold` | 100 | Trigger detection after N new entities |
| `min_entities` | 10 | Minimum entities required for detection |
| `min_embedding_coverage` | 0.5 | Required embedding ratio for semantic clustering |
| `enhancement_window` | 0 | Pause detection duration for LLM (0 = disabled) |
| `enhancement_window_mode` | blocking | Mode: `blocking`, `soft`, or `none` |

#### Enhancement Window Modes

| Mode | Behavior |
|------|----------|
| `blocking` | Hard pause until window expires or all communities reach terminal status |
| `soft` | Allow detection if entity changes exceed threshold during window |
| `none` | No enhancement window (original behavior) |

#### Enhancement Options

| Option | Default | Description |
|--------|---------|-------------|
| `workers` | 3 | Concurrent LLM enhancement goroutines |
| `domain` | default | Prompt domain (e.g., "iot", "default") |

#### LLM Options

| Option | Default | Description |
|--------|---------|-------------|
| `provider` | none | Backend: `openai` (any compatible), `none` |
| `base_url` | localhost:8080/v1 | LLM service URL |
| `model` | mistral-7b-instruct | Model identifier |
| `timeout_seconds` | 60 | Per-request timeout |
| `max_retries` | 3 | Retry count for transient failures |

#### Semantic Edges Options

| Option | Default | Description |
|--------|---------|-------------|
| `enabled` | false | Enable virtual edges from embeddings |
| `similarity_threshold` | 0.6 | Minimum cosine similarity for virtual edge |
| `max_virtual_neighbors` | 5 | Limit virtual neighbors per entity |

#### Inference Options

| Option | Default | Description |
|--------|---------|-------------|
| `enabled` | false | Create inferred triples from communities |
| `min_community_size` | 2 | Skip singleton communities |
| `max_inferred_per_community` | 50 | Prevent O(n^2) explosion |

## Performance Tuning

### LLM Latency Expectations

| Environment | Latency per Community | Total (54 communities) |
|-------------|----------------------|------------------------|
| Local Ollama (CPU) | 2-5s | 108-270s |
| Local Ollama (GPU) | 0.3-1s | 16-54s |
| Cloud API (GPT-4) | 1-3s | 54-162s |
| Cloud API (GPT-4o-mini) | 0.5-1s | 27-54s |

### Tuning Recommendations

1. **Faster LLM backend** → Increase `enhancement.workers`
2. **Reduce LLM calls** → Lower `schedule.min_entities` threshold
3. **Prioritize freshness** → Set `enhancement_window_mode: none`
4. **Prioritize quality** → Set `enhancement_window: 120s` + `blocking`

## Community Summary States

| Status | Description |
|--------|-------------|
| `statistical` | Statistical summary only (immediate) |
| `llm-enhanced` | LLM narrative added (async) |
| `llm-failed` | LLM enhancement failed |
| `statistical-fallback` | LLM unavailable, using statistical |

## Metrics

### Prometheus Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `graph_processor_entities_processed_total` | Counter | Total entities processed |
| `graph_processor_communities_detected_total` | Counter | Communities detected |
| `graph_processor_enhancement_latency_seconds` | Histogram | LLM enhancement latency |
| `graph_processor_enhancement_queue_depth` | Gauge | Pending enhancements |

### E2E Test Metrics (captured in results JSON)

| Metric | Description |
|--------|-------------|
| `communities_total` | Total communities at level 0 |
| `communities_non_singleton` | Communities with >1 member |
| `communities_llm_enhanced` | Communities with LLM summaries |
| `llm_wait_duration_ms` | Time waiting for LLM completion |
| `largest_community_size` | Maximum community member count |
| `avg_non_singleton_size` | Average members in real clusters |

## Operational Notes

### Pause/Resume During Detection

The EnhancementWorker implements pause/resume to prevent race conditions during detection:

```go
// In processor.go DetectCommunities():
if p.enhancementWorker != nil {
    p.enhancementWorker.Pause()
    defer p.enhancementWorker.Resume()
}
```

This ensures:
1. No concurrent writes to COMMUNITY_INDEX during detection
2. In-flight LLM work completes before pause
3. Worker resumes automatically after detection

### Summary Preservation

When communities evolve, summaries are preserved via Jaccard similarity matching:

```go
// Jaccard index = |intersection| / |union|
// If Jaccard >= 0.8, copy existing summary to new community
```

This prevents re-running expensive LLM calls when community membership changes slightly.

### Graceful Degradation

If LLM service is unavailable:
1. Statistical summary is always generated (immediate)
2. Enhancement worker marks status as `llm-failed`
3. System continues operating with statistical summaries
4. Retries can be triggered by re-saving community with `statistical` status
