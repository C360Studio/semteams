# Graph Processor Migration Guide

This guide documents the migration from the monolithic `graph-processor` component to the modular graph component architecture.

## Overview

The original `processor/graph/processor.go` (3,695 LOC) has been decomposed into 8 focused components:

| Old Component | New Components |
|---------------|----------------|
| `graph-processor` | `graph-ingest`, `graph-index`, `graph-embedding`, `graph-clustering`, `graph-index-spatial`, `graph-index-temporal`, `graph-structural`, `graph-gateway` |

## Breaking Changes

### Configuration

The single `graph` component configuration is replaced by individual component configurations.

**Before:**
```json
{
  "components": {
    "graph": {
      "type": "processor",
      "name": "graph",
      "enabled": true,
      "config": {
        "enable_community_detection": true,
        "enable_embeddings": true,
        "graphql_port": 8084
      }
    }
  }
}
```

**After:**
```json
{
  "components": {
    "graph-ingest": {
      "type": "processor",
      "name": "graph-ingest",
      "enabled": true,
      "config": {
        "ports": {
          "inputs": [
            {"name": "entity_in", "subject": "entity.>", "type": "jetstream"}
          ],
          "outputs": [
            {"name": "entity_states", "subject": "ENTITY_STATES", "type": "kv"}
          ]
        },
        "enable_hierarchy": true
      }
    },
    "graph-index": {
      "type": "processor",
      "name": "graph-index",
      "enabled": true,
      "config": {
        "ports": {
          "inputs": [
            {"name": "entity_watch", "subject": "ENTITY_STATES", "type": "kv-watch"}
          ],
          "outputs": [
            {"name": "outgoing_index", "subject": "OUTGOING_INDEX", "type": "kv"},
            {"name": "incoming_index", "subject": "INCOMING_INDEX", "type": "kv"},
            {"name": "alias_index", "subject": "ALIAS_INDEX", "type": "kv"},
            {"name": "predicate_index", "subject": "PREDICATE_INDEX", "type": "kv"}
          ]
        },
        "workers": 4,
        "batch_size": 50
      }
    },
    "graph-gateway": {
      "type": "gateway",
      "name": "graph-gateway",
      "enabled": true,
      "config": {
        "graphql_path": "/graphql",
        "bind_address": ":8084"
      }
    }
  }
}
```

### Schema Changes

The JSON schema has been updated:
- Removed: `schemas/graph-processor.v1.json`
- Added: Individual schemas for each component

Run `task schema:generate` after migration to regenerate schemas.

### Component Registration

The component registry now registers 8 separate factories instead of one:

```go
// Old
graphprocessor.Register(registry)

// New
graphingest.Register(registry)
graphindex.Register(registry)
graphembedding.Register(registry)
graphclustering.Register(registry)
graphindexspatial.Register(registry)
graphindextemporal.Register(registry)
graphstructural.Register(registry)
graphgateway.Register(registry)
```

## Migration Steps

### Step 1: Update Configuration Files

Replace the single `graph` component with the new modular components. Use the tier-appropriate configuration:

**Structural Tier (no ML):**
- `graph-ingest`
- `graph-index`
- `graph-structural` (optional)
- `graph-gateway`

**Semantic Tier (with ML):**
- All structural components
- `graph-embedding`
- `graph-clustering`
- `graph-index-spatial` (optional)
- `graph-index-temporal` (optional)

### Step 2: Update Port Configuration

Each component now has explicit port definitions. Ensure input/output subjects match your data flow:

```json
{
  "ports": {
    "inputs": [
      {"name": "entity_watch", "subject": "ENTITY_STATES", "type": "kv-watch"}
    ],
    "outputs": [
      {"name": "index_out", "subject": "OUTGOING_INDEX", "type": "kv"}
    ]
  }
}
```

### Step 3: Verify KV Bucket Names

The modular architecture uses consistent bucket names:

| Bucket | Component |
|--------|-----------|
| `ENTITY_STATES` | graph-ingest writes, others watch |
| `OUTGOING_INDEX` | graph-index writes |
| `INCOMING_INDEX` | graph-index writes |
| `ALIAS_INDEX` | graph-index writes |
| `PREDICATE_INDEX` | graph-index writes |
| `EMBEDDINGS_CACHE` | graph-embedding writes |
| `COMMUNITY_INDEX` | graph-clustering writes |
| `SPATIAL_INDEX` | graph-index-spatial writes |
| `TEMPORAL_INDEX` | graph-index-temporal writes |
| `STRUCTURAL_INDEX` | graph-structural writes |

### Step 4: Regenerate Schemas

```bash
task schema:generate
```

### Step 5: Run Tests

```bash
task test
task test:integration
```

### Step 6: Run E2E Tests

```bash
task e2e:structural
task e2e:semantic
```

## Component Dependencies

The components have the following dependency chain:

```
graph-ingest (no dependencies)
     │
     ▼
graph-index (depends on: ENTITY_STATES)
     │
     ├──► graph-embedding (depends on: ENTITY_STATES)
     │         │
     │         ▼
     │    graph-clustering (depends on: ENTITY_STATES, indexes, EMBEDDINGS_CACHE)
     │
     ├──► graph-index-spatial (depends on: ENTITY_STATES)
     │
     ├──► graph-index-temporal (depends on: ENTITY_STATES)
     │
     └──► graph-structural (depends on: OUTGOING_INDEX, INCOMING_INDEX)

graph-gateway (reads all KV buckets, no write dependencies)
```

## Troubleshooting

### Component Not Starting

Check that all required KV buckets exist. The NATS JetStream will auto-create buckets, but ensure your NATS configuration allows this.

### Missing Data in Index

Verify the kv-watch port configuration matches the bucket name exactly:
```json
{"name": "entity_watch", "subject": "ENTITY_STATES", "type": "kv-watch"}
```

### GraphQL Not Responding

Ensure graph-gateway is configured with the correct bind address and ports:
```json
{
  "graphql_path": "/graphql",
  "bind_address": ":8084"
}
```

### Embeddings Not Generated

For semantic tier, verify:
1. `graph-embedding` is enabled
2. `embedder_url` points to a running embedding service
3. ENTITY_STATES bucket is being populated by graph-ingest

## Rollback

To rollback to the monolithic processor:

1. Revert configuration to use single `graph` component
2. Re-register `graphprocessor.Register(registry)` in component registry
3. Restore `processor/graph/processor.go` from git history

Note: The modular architecture is the recommended path forward. Rollback should only be used for emergency situations.

## Support

For issues with the migration:
1. Check component logs for specific error messages
2. Verify NATS connectivity and JetStream configuration
3. Review the component README files in `processor/graph-*/README.md`
