# Communities

A community is a group of entities that are more connected to each other than to everything else. You don't define communities - they emerge automatically from the relationships in your data.

## Why Communities Matter

### 1. Discovery Without Questions

Traditional query: "Show me sensors in warehouse 7"
- Requires knowing that warehouse 7 has sensors
- Requires knowing the right field to query

With communities: "Show me related equipment"
- Discovers sensors, actuators, maintenance records you didn't know existed
- Finds connections across entity types

### 2. Context for AI

When an LLM needs to answer "what's happening in the factory?", communities provide pre-organized context:

**Without communities**: Dump 10,000 unrelated entities, hope the LLM figures it out
**With communities**: "Here are the 5 related equipment clusters and their summaries"

### 3. Anomaly Detection

If sensor-42 suddenly appears in a different community, something changed:
- Network topology shifted
- Equipment was moved
- A relationship was broken or created

You didn't have to write alert rules for this - the change in community membership IS the signal.

## Example: Robotics Fleet

**Your data:**
- 50 drones
- 200 sensors (battery, GPS, altitude)
- 100 maintenance records

**What communities detect:**

```
Community: "Warehouse-7 Fleet"
Members: [drone-007, drone-008, drone-012, sensor-alpha, sensor-beta, maintenance-rec-42]
Statistical Summary: "6 entities, keywords: cargo, delivery, cold-storage"
LLM Summary: "Fleet of cargo drones operating delivery missions in cold storage warehouse 7.
              Recent maintenance focused on battery systems."
```

**Now you can ask:**
- "What's the status of Warehouse-7 Fleet?" (instead of querying 6 entities individually)
- "Which fleet has the most maintenance issues?"
- "Show me fleets similar to Warehouse-7 Fleet"

## How Communities Are Built

### Label Propagation Algorithm (LPA)

1. Each entity starts as its own community
2. Iteratively, each entity adopts the most common label among its neighbors
3. Process repeats until stable
4. Groups with the same label form communities

### What Creates Edges?

Edges come from two sources:

**Explicit edges** (from your triples):
```go
// This triple creates an edge drone → mission
Triple{Predicate: "mission.assignment", Object: "mission-delivery-42"}
```

**Virtual edges** (from similarity, Tier 2 only):
```
If embedding_similarity(sensor-A, sensor-B) > 0.6
    → Create virtual edge between them
```

### Hierarchical Levels

Communities are detected at multiple levels:

| Level | Granularity | Example |
|-------|-------------|---------|
| 0 | Finest | Individual equipment clusters |
| 1 | Medium | Department-level groupings |
| 2 | Coarsest | Facility-wide groupings |

## The Three Summary Levels

### 1. Statistical (Immediate, ~1ms)

Generated instantly after detection:
- TF-IDF keywords extracted from entity content
- PageRank identifies representative entities
- Template-based summary

```
"Community of 8 entities. Keywords: drone, battery, warehouse-7.
 Representative: drone-007"
```

### 2. LLM-Enhanced (Async, 1-5s)

Background worker picks up statistical summaries and generates narrative:

```
"A fleet of cargo drones operating in the cold storage section of warehouse 7.
 The fleet consists of 5 drones and associated telemetry sensors, with recent
 activity focused on delivery missions. Battery levels are a primary concern
 based on maintenance records."
```

### 3. Queryable (GraphRAG)

Communities + summaries become context for complex questions:

```
Q: "What equipment might be affected if warehouse 7 loses power?"
A: [Uses community membership to identify all related equipment]
```

## Community States

| Status | Meaning |
|--------|---------|
| `statistical` | Initial summary generated, awaiting LLM |
| `llm-enhanced` | LLM narrative complete |
| `llm-failed` | LLM unavailable, using statistical fallback |

## Summary Preservation

When entities change, communities are recomputed. But summaries are preserved when possible:

- Jaccard similarity measures membership overlap
- If overlap >= 80%, existing summaries are copied to new community
- Prevents re-running expensive LLM calls for minor membership changes

## Configuration

```json
{
  "clustering": {
    "enabled": true,
    "schedule": {
      "initial_delay": "10s",
      "detection_interval": "30s",
      "entity_change_threshold": 100
    },
    "lpa": {
      "max_iterations": 10,
      "levels": 3
    },
    "llm": {
      "base_url": "http://seminstruct:8083/v1",
      "model": "default"
    }
  }
}
```

| Setting | Effect |
|---------|--------|
| `initial_delay` | Wait before first detection |
| `detection_interval` | Max time between runs |
| `entity_change_threshold` | Trigger after N entity changes |
| `max_iterations` | LPA convergence limit |
| `levels` | Hierarchical depth (0-2) |

## What You Control

1. **Triple design**: How you structure triples determines what relationships exist
2. **Entity references**: Triples with entity IDs as objects create edges
3. **Text content**: `TextContent()` method provides material for embeddings and keywords
4. **Tier selection**: Tier 0 (no communities), Tier 1 (statistical), Tier 2 (LLM)

## What You Don't Control

- Which entities cluster together (emerges from data)
- Community boundaries (determined by LPA algorithm)
- When detection runs (triggered by entity change threshold)

## Common Questions

**Q: How many communities will I get?**
A: Depends on your data's structure. Dense, well-connected data produces fewer, larger communities. Sparse, isolated entities produce many singleton communities.

**Q: Can I force entities into specific communities?**
A: Not directly. But you can create explicit relationships via rules (`add_triple`) that will influence clustering.

**Q: What if communities are wrong?**
A: Review your triple design. If entities should cluster together, they need edges between them - either explicit (triples) or virtual (embeddings).

**Q: Do I need LLM summaries?**
A: No. Statistical summaries work well for programmatic use. LLM summaries are for human readability and complex AI queries.

## Next Steps

- [Indexes](03-indexes.md) - How indexes feed community detection
- [Triples](02-triples.md) - Design relationships that cluster
- [Tiers](../basics/06-tiers.md) - Choose your capability level
