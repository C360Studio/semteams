# KV Buckets Reference

SemStreams uses NATS JetStream Key-Value buckets for all persistent storage. This document covers all buckets, their key patterns, and query examples.

## Bucket Overview

| Bucket | Purpose | Key Pattern |
|--------|---------|-------------|
| `ENTITY_STATES` | Primary entity storage | `{entity_id}` |
| `PREDICATE_INDEX` | Triple lookup by predicate | `{predicate}` |
| `INCOMING_INDEX` | Inbound relationships | `{entity_id}` |
| `OUTGOING_INDEX` | Outbound relationships | `{entity_id}` |
| `COMMUNITY_INDEX` | Community records | `graph.community.{level}.{id}` |
| `ALIAS_INDEX` | Name resolution | `{alias}` |
| `SPATIAL_INDEX` | Geospatial lookup | `{geohash}` |
| `TEMPORAL_INDEX` | Time-based lookup | `{time_bucket}` |
| `EMBEDDING_INDEX` | Vector embeddings | `{entity_id}` |
| `RULE_STATE` | Rule match state | `{rule_id}.{entity_key}` |

## ENTITY_STATES

Primary storage for entity records.

**Key Pattern:** `{entity_id}`

**Value Structure:**
```json
{
  "id": "acme.robotics.aerial.fleet.drone.drone-007",
  "triples": [
    {"predicate": "entity.type", "object": "drone"},
    {"predicate": "drone.telemetry.battery", "object": 78},
    {"predicate": "fleet.membership", "object": "acme.robotics.aerial.fleet.rescue"}
  ],
  "version": 5
}
```

**Commands:**
```bash
# Get entity
nats kv get ENTITY_STATES "acme.robotics.aerial.fleet.drone.drone-007"

# List all entities
nats kv ls ENTITY_STATES

# Watch for changes
nats kv watch ENTITY_STATES "acme.*.robotics.>"

# Put entity (with version for CAS)
nats kv put ENTITY_STATES "drone-007" '{"id":"drone-007","triples":[],"version":1}'
```

## PREDICATE_INDEX

Maps predicates to entity IDs. "Find all entities with this property."

**Key Pattern:** `{predicate}` (the predicate name itself)

**Value Structure:**
```json
{
  "entities": [
    "acme.robotics.aerial.fleet.drone.drone-007",
    "acme.robotics.aerial.fleet.drone.drone-008"
  ],
  "count": 2,
  "last_update": 1640995200
}
```

**Commands:**
```bash
# Find entities with battery data
nats kv get PREDICATE_INDEX "drone.telemetry.battery"

# Find all sensors with temperature readings
nats kv get PREDICATE_INDEX "sensor.measurement.celsius"

# List all predicates
nats kv ls PREDICATE_INDEX
```

**Key Insight:** The entity IDs are in the **value**, not the key. Query the predicate name to get a list of entities that have that predicate.

## INCOMING_INDEX

Maps entities to entities that reference them. "Who points to me?"

**Key Pattern:** `{target_entity_id}`

**Value Structure:**
```json
{
  "incoming": [
    "acme.robotics.aerial.fleet.drone.drone-007",
    "acme.robotics.aerial.fleet.sensor.sensor-001"
  ]
}
```

**Commands:**
```bash
# Find what references the rescue fleet
nats kv get INCOMING_INDEX "acme.robotics.aerial.fleet.rescue"

# Find entities that reference a specific drone
nats kv get INCOMING_INDEX "acme.robotics.aerial.fleet.drone.drone-007"
```

**Use Case:** Community detection uses this to traverse the graph in both directions.

## OUTGOING_INDEX

Maps entities to entities they reference. "What do I point to?"

**Key Pattern:** `{source_entity_id}`

**Value Structure:**
```json
{
  "outgoing": [
    "acme.robotics.aerial.fleet.rescue",
    "acme.robotics.aerial.base.hangar-1"
  ]
}
```

**Commands:**
```bash
# Find what drone-007 references
nats kv get OUTGOING_INDEX "acme.robotics.aerial.fleet.drone.drone-007"
```

## COMMUNITY_INDEX

Stores detected communities and entity-to-community mappings.

**Key Patterns:**
- Community records: `graph.community.{level}.{community_id}`
- Entity mappings: `graph.community.entity.{level}.{entity_id}`

**Community Value:**
```json
{
  "id": "comm-0-A1",
  "level": 0,
  "members": ["drone-007", "drone-008", "sensor-001"],
  "parent_id": "comm-1-B2",
  "statistical_summary": "Community of 3 aerial entities...",
  "llm_summary": "This community represents the rescue fleet...",
  "summary_status": "llm-enhanced",
  "keywords": ["rescue", "aerial", "emergency"],
  "rep_entities": ["drone-007"]
}
```

**Entity Mapping Value:**
```
"comm-0-A1"
```

**Commands:**
```bash
# Get community
nats kv get COMMUNITY_INDEX "graph.community.0.comm-0-A1"

# Find entity's community
nats kv get COMMUNITY_INDEX "graph.community.entity.0.drone-007"

# List all level-0 communities
nats kv ls COMMUNITY_INDEX | grep "graph.community.0."
```

## ALIAS_INDEX

Maps human-readable aliases to canonical entity IDs.

**Key Pattern:** `{alias}`

**Value:** `{canonical_entity_id}`

**Example:**
```
Key:   "rescue-lead"
Value: "acme.robotics.aerial.fleet.drone.drone-007"
```

**Commands:**
```bash
# Resolve alias
nats kv get ALIAS_INDEX "rescue-lead"

# Set alias
nats kv put ALIAS_INDEX "rescue-lead" "acme.robotics.aerial.fleet.drone.drone-007"
```

## SPATIAL_INDEX

Geospatial indexing using geohash prefixes.

**Key Pattern:** `{geohash_prefix}` (e.g., "dr5r" for NYC area)

**Value Structure:**
```json
{
  "entities": {
    "drone-007": {
      "lat": 40.7589,
      "lon": -73.9851,
      "alt": 100.0,
      "updated": 1640995200
    }
  },
  "last_update": 1640995200
}
```

**Commands:**
```bash
# Get entities in geohash area
nats kv get SPATIAL_INDEX "dr5r"

# List all spatial cells
nats kv ls SPATIAL_INDEX
```

## TEMPORAL_INDEX

Time-based indexing with hourly buckets.

**Key Pattern:** `{YYYY}.{MM}.{DD}.{HH}`

**Value Structure:**
```json
{
  "events": [
    {
      "entity": "drone-007",
      "type": "update",
      "timestamp": "2024-01-15T14:30:00Z"
    }
  ],
  "entity_count": 1
}
```

**Commands:**
```bash
# Get entities active in specific hour
nats kv get TEMPORAL_INDEX "2024.01.15.14"

# List all time buckets
nats kv ls TEMPORAL_INDEX
```

## EMBEDDING_INDEX

Vector embeddings for semantic similarity search.

**Key Pattern:** `{entity_id}`

**Value Structure:**
```json
{
  "entity_id": "drone-007",
  "vector": [0.123, -0.456, 0.789, ...],
  "content_hash": "a1b2c3d4...",
  "source_text": "Rescue drone with low battery...",
  "model": "all-MiniLM-L6-v2",
  "dimensions": 384,
  "generated_at": "2024-01-15T14:30:00Z",
  "status": "generated"
}
```

**Status Values:**
- `pending`: Awaiting embedding generation
- `generated`: Embedding available
- `failed`: Generation failed

**Commands:**
```bash
# Get entity embedding
nats kv get EMBEDDING_INDEX "acme.robotics.aerial.fleet.drone.drone-007"

# Check embedding status
nats kv get EMBEDDING_INDEX "drone-007" | jq '.status'
```

**Related Bucket:** `EMBEDDING_DEDUP` stores content-hash to vector mappings for deduplication.

## RULE_STATE

Tracks rule match state for stateful rule evaluation.

**Key Pattern:** `{rule_id}.{entity_key}`

**Value Structure:**
```json
{
  "rule_id": "battery-low-alert",
  "entity_key": "acme.robotics.aerial.fleet.drone.drone-007",
  "is_matching": true,
  "last_transition": "entered",
  "transition_at": "2024-01-15T10:30:00Z",
  "source_revision": 5,
  "last_checked": "2024-01-15T10:35:00Z"
}
```

**Commands:**
```bash
# Get rule state for entity
nats kv get RULE_STATE "battery-low-alert.drone-007"

# List all states for a rule
nats kv ls RULE_STATE | grep "battery-low"

# List all states for an entity
nats kv ls RULE_STATE | grep "drone-007"
```

## KV Watch Patterns

NATS KV supports watch patterns for real-time updates:

```bash
# Watch all entities
nats kv watch ENTITY_STATES ">"

# Watch specific organization
nats kv watch ENTITY_STATES "acme.>"

# Watch specific entity type
nats kv watch ENTITY_STATES "*.*.robotics.*.drone.*"

# Watch predicate changes
nats kv watch PREDICATE_INDEX ">"
```

Pattern wildcards:
- `*` - matches single segment
- `>` - matches one or more segments (end only)

## Bucket Configuration

Default bucket settings:

```go
jetstream.KeyValueConfig{
    Bucket:      "ENTITY_STATES",
    Description: "Entity state storage",
    History:     10,              // Keep 10 versions
    TTL:         7 * 24 * time.Hour, // 7 day retention
    MaxBytes:    -1,              // Unlimited size
}
```

## Index Manager Integration

The IndexManager watches `ENTITY_STATES` and maintains all secondary indexes automatically:

```
ENTITY_STATES (source of truth)
       │
       ▼
   KV Watcher
       │
       ▼
   Event Buffer + Deduplication
       │
       ▼
   ┌───┴───┬───────┬───────┬───────┬───────┐
   ▼       ▼       ▼       ▼       ▼       ▼
PREDICATE INCOMING OUTGOING ALIAS SPATIAL TEMPORAL
```

Indexes are eventually consistent with ENTITY_STATES (milliseconds latency).

## Common Queries

### Find entities by type
```bash
nats kv get PREDICATE_INDEX "entity.type" | jq '.entities'
```

### Check entity relationships
```bash
# Outgoing
nats kv get OUTGOING_INDEX "drone-007"

# Incoming
nats kv get INCOMING_INDEX "drone-007"
```

### Get community membership
```bash
nats kv get COMMUNITY_INDEX "graph.community.entity.0.drone-007"
```

### Debug rule state
```bash
nats kv get RULE_STATE "battery-low.drone-007"
```

## Troubleshooting

### Bucket not found
```bash
# List all buckets
nats kv ls

# Create missing bucket
nats kv add ENTITY_STATES --history 10 --ttl 168h
```

### Check bucket status
```bash
nats kv status ENTITY_STATES
```

### View recent changes
```bash
nats kv history ENTITY_STATES "drone-007"
```
