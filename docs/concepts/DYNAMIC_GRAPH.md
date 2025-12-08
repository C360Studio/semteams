# The Dynamic Knowledge Graph

SemStreams builds and maintains a living knowledge graph that updates in real-time as data flows through the system.

## Yes, It's Dynamic

This is NOT a batch ETL pipeline. SemStreams:

- Updates in real-time as new data arrives
- Tracks entity versions (optimistic concurrency)
- Recomputes relationships as topology changes
- Detects communities as the graph evolves
- Maintains consistent state across all indexes

## What Gets Stored

### Entities: ENTITY_STATES Bucket

Every entity with its current properties:

```json
{
  "id": "acme.robotics.aerial.drone.drone-007",
  "triples": [
    {"predicate": "entity.type", "object": "drone"},
    {"predicate": "drone.telemetry.battery", "object": "78"},
    {"predicate": "mission.assignment.current", "object": "acme.robotics.mission.delivery-42"}
  ],
  "version": 5,
  "last_updated": "2024-12-08T10:30:00Z"
}
```

- Versioned for conflict resolution
- Updated on each new message
- Version increments atomically

### Relationships: 7 Index Buckets

| Bucket | Question Answered |
|--------|-------------------|
| `INCOMING_INDEX` | "Who references this entity?" |
| `OUTGOING_INDEX` | "What does this entity reference?" |
| `PREDICATE_INDEX` | "All entities with this property" |
| `SPATIAL_INDEX` | "Entities near this location" |
| `TEMPORAL_INDEX` | "Entities in this time range" |
| `ALIAS_INDEX` | "Resolve friendly name to ID" |
| `EMBEDDING_INDEX` | "Semantically similar entities" |

### Communities: COMMUNITY_INDEX Bucket

```json
{
  "id": "community-level0-abc123",
  "level": 0,
  "members": ["drone-007", "drone-008", "sensor-alpha"],
  "statistical_summary": "Community of 3 entities...",
  "llm_summary": "A group of cargo drones...",
  "summary_status": "llm-enhanced",
  "created_at": "2024-12-08T10:00:00Z",
  "updated_at": "2024-12-08T10:35:00Z"
}
```

- Recomputed as entities change
- Preserved when membership is stable (Jaccard >= 0.8)
- Summaries updated progressively

## The Update Cycle

```
1. New telemetry arrives via NATS
        │
        ▼
2. MessageManager transforms to entity
        │
        ▼
3. DataManager updates ENTITY_STATES (version++)
        │
        ▼
4. IndexManager updates all 7 indexes
        │
        ├─── PREDICATE_INDEX: add/update predicate entries
        ├─── INCOMING_INDEX: update inbound refs
        ├─── OUTGOING_INDEX: update outbound refs
        ├─── SPATIAL_INDEX: update geohash entries
        ├─── TEMPORAL_INDEX: update time entries
        ├─── ALIAS_INDEX: update aliases
        └─── EMBEDDING_INDEX: queue for embedding
        │
        ▼
5. Rules engine evaluates conditions
        │
        ├─── on_enter: add_triple, publish
        └─── on_exit: remove_triple
        │
        ▼
6. Entity change count accumulates
        │
        ▼
7. Threshold reached → Community detection runs
        │
        ├─── LPA algorithm groups entities
        ├─── Jaccard matching preserves summaries
        ├─── Statistical summaries generated
        └─── EnhancementWorker queued for LLM
        │
        ▼
8. Graph is updated and queryable
```

## Consistency Model

### Entity Updates

- Optimistic concurrency via version numbers
- Compare-and-swap for safe updates
- Conflicts resolved by latest-writer-wins

### Index Updates

- Eventually consistent (KV watch pattern)
- Indexes update after entity saves
- Short window where entity exists but not indexed

### Community Updates

- Batch recomputation (not per-entity)
- Triggered by threshold, not time
- Pause/resume coordination prevents races

## When Things Change

### New Entity

1. Entity created in ENTITY_STATES (version=1)
2. All indexes updated
3. Rules evaluated
4. (Later) Community detection may run

### Updated Entity

1. Entity updated in ENTITY_STATES (version++)
2. Indexes updated with changes
3. Rules re-evaluated (may fire on_exit/on_enter)
4. Community membership may change

### Deleted Entity (via Rules)

1. Entity triples removed via `remove_triple`
2. Indexes updated (entity removed from indexes)
3. Community detection removes from communities
4. (Entity record may persist but with empty triples)

## Query Freshness

| Query Type | Freshness |
|------------|-----------|
| Entity by ID | Immediate |
| By predicate | Near-immediate (index lag) |
| By relationship | Near-immediate (index lag) |
| By embedding | Depends on embedding queue |
| Community membership | Batch (detection interval) |
| Community summary | Batch + async LLM |

## Configuration: Detection Timing

```json
{
  "clustering": {
    "schedule": {
      "initial_delay": "10s",
      "detection_interval": "30s",
      "min_detection_interval": "5s",
      "entity_change_threshold": 100
    }
  }
}
```

| Setting | Effect |
|---------|--------|
| `initial_delay` | Wait before first detection |
| `detection_interval` | Max time between runs |
| `min_detection_interval` | Burst protection |
| `entity_change_threshold` | Trigger after N changes |

## What Makes It "Live"

1. **No batch windows**: Updates flow continuously
2. **No schema migration**: New predicates just work
3. **No manual refresh**: Indexes update automatically
4. **No snapshot staleness**: Always current state

## Comparison to Static Approaches

| Aspect | Batch ETL | SemStreams |
|--------|-----------|------------|
| Update latency | Hours | Milliseconds |
| Schema changes | Migration required | Just use new predicate |
| Query freshness | As of last batch | Current |
| Community membership | Pre-computed | Emerges from data |

## Graceful Degradation

If components fail:

| Failure | Impact | Recovery |
|---------|--------|----------|
| LLM unavailable | Statistical summaries only | Retries on service restore |
| Embedding service down | No new embeddings | Queue for later |
| High entity volume | Detection delayed | Threshold eventually reached |
| Index lag | Temporary query gaps | Auto-catches up |

## Not a Database Replacement

SemStreams is not a general-purpose database:

**Good for:**
- Real-time entity tracking
- Relationship discovery
- Community-based queries
- AI-ready context

**Not designed for:**
- Arbitrary SQL queries
- Historical snapshots
- ACID transactions
- Point-in-time recovery

## Next Steps

- [What is a Community?](WHAT_IS_A_COMMUNITY.md) - How communities emerge
- [Index Usage](INDEX_USAGE.md) - What gets indexed and why
- [Rules and Graph](RULES_AND_GRAPH.md) - Dynamic relationships via rules
