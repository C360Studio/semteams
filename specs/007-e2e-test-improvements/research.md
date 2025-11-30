# Research: E2E Test Suite Improvements

**Feature**: 007-e2e-test-improvements
**Date**: 2025-11-30

## Research Tasks

### R1: NATS KV Client for E2E Tests

**Question**: How should E2E tests connect to NATS KV to validate entity storage?

**Decision**: Reuse existing `natsclient.Client` which already provides full KV bucket access

**Rationale**:
- The `natsclient` package already provides `GetKeyValueBucket()` and `ListKeyValueBuckets()`
- No need to duplicate NATS connection logic - circuit breaker, reconnection, etc. already handled
- Consistent with how the rest of SemStreams accesses NATS

**Existing Methods in natsclient.Client**:
```go
// Get a KV bucket by name
bucket, err := client.GetKeyValueBucket(ctx, "ENTITY_STATES")

// List all KV buckets
buckets, err := client.ListKeyValueBuckets(ctx)

// The returned jetstream.KeyValue supports:
entry, err := bucket.Get(ctx, entityID)  // Get single entry
keys, err := bucket.Keys(ctx)            // List all keys
```

**E2E Wrapper**: Create thin wrapper in `test/e2e/client/nats.go` with convenience methods:
- `CountEntities(ctx)` - Count keys in ENTITY_STATES
- `GetEntity(ctx, id)` - Get and unmarshal entity
- `ValidateIndexPopulated(ctx, indexName)` - Check index has entries

**Alternatives Considered**:
1. Create new NATS client from scratch - Rejected: Duplicates existing natsclient functionality
2. Query via HTTP API - Rejected: No HTTP API for KV bucket contents
3. Use GraphQL queries - Rejected: Would test query layer, not raw storage

### R2: KV Bucket Names

**Question**: What are the correct bucket names to query?

**Decision**: Reuse constants from `graph/constants.go`

**Bucket Names**:
| Bucket | Constant | Purpose |
|--------|----------|---------|
| ENTITY_STATES | `BucketEntityStates` | Primary entity storage |
| PREDICATE_INDEX | (defined in indexmanager) | Property-based lookups |
| SPATIAL_INDEX | `BucketSpatialIndex` | Geo-location queries |
| TEMPORAL_INDEX | `BucketTemporalIndex` | Time-based queries |
| INCOMING_INDEX | `BucketIncomingIndex` | Reverse relationship lookups |
| ALIAS_INDEX | (defined in indexmanager) | Name-to-ID resolution |

**Rationale**: Using existing constants ensures consistency and catches rename issues

### R3: Current Prometheus Metrics

**Question**: What metrics should E2E tests validate?

**Decision**: Query actual metric names from codebase, not hardcode

**Current Metrics** (from `processor/graph/indexmanager/metrics.go`):
- `indexmanager_events_total` - Total events received
- `indexmanager_events_processed` - Successfully processed
- `indexmanager_events_failed` - Processing failures
- `indexmanager_index_updates_total{index="..."}` - Per-index updates
- `indexmanager_embeddings_generated_total` - Embedding count (optional)

**From `pkg/cache/metrics.go`**:
- `semstreams_cache_hits_total` - Cache hits
- `semstreams_cache_misses_total` - Cache misses

**From `processor/json_filter/metrics.go`**:
- `semstreams_json_filter_matched_total` - Filter matches
- `semstreams_json_filter_dropped_total` - Filter drops

**Rationale**: These metrics are registered at startup and should be present after any data processing

### R4: StreamKit to SemStreams Rename

**Question**: What files contain "StreamKit" references?

**Decision**: Search and replace all occurrences

**Files Identified**:
```bash
grep -r "StreamKit" test/e2e/ --include="*.go"
```
- `test/e2e/client/observability.go` - Package comment, struct comment
- `test/e2e/scenarios/core_health.go` - Package comment, description
- `test/e2e/scenarios/core_dataflow.go` - Package comment, description
- `test/e2e/scenarios/*.go` - Multiple files

**Rationale**: Consistent naming improves maintainability and avoids confusion

### R5: Validation Thresholds

**Question**: What validation thresholds are appropriate?

**Decision**: Use configurable thresholds with sensible defaults

| Metric | Default | Rationale |
|--------|---------|-----------|
| Entity storage rate | 80% | UDP packet loss acceptable |
| Index population | >= 1 entry | At least one entry proves mechanism works |
| Metrics present | All required | Missing metrics indicate registration issues |
| Test timeout | 30s per scenario | Allows processing time without CI timeout |

**Rationale**: 80% threshold accounts for UDP unreliability while still catching major failures

## Technical Decisions Summary

| Decision | Choice | Confidence |
|----------|--------|------------|
| NATS client approach | Direct jetstream.KeyValue | High |
| Bucket name source | Reuse graph/constants.go | High |
| Metrics validation | Query actual registered metrics | High |
| Validation threshold | 80% entity storage | Medium |
| Test structure | Extend existing scenarios | High |

## Dependencies

- `github.com/nats-io/nats.go` - Already in go.mod
- `github.com/nats-io/nats.go/jetstream` - Already in go.mod
- No new external dependencies required

## Risks

| Risk | Mitigation |
|------|------------|
| NATS connection timing | Add retry logic with backoff |
| Bucket doesn't exist | Report clear error, don't panic |
| Metrics change over time | Document expected metrics in README |
| UDP packet loss varies | Use 80% threshold, make configurable |
