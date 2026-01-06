# Integration Test Requirements for graph-index-spatial

Builder must create integration tests (`//go:build integration`) covering the scenarios below.

## Test File Location
`processor/graph-index-spatial/component_integration_test.go`

## Required Test Scenarios

### 1. Real NATS JetStream Integration

**Test:** `TestIntegration_ComponentLifecycle`

- Setup real NATS JetStream connection
- Create ENTITY_STATES and SPATIAL_INDEX KV buckets
- Initialize and start component
- Verify component health is healthy
- Stop component gracefully
- Verify cleanup (no goroutine leaks)

**Success Criteria:**
- Component starts without errors
- Health status shows healthy
- Stop completes within timeout
- No goroutine leaks detected

---

### 2. Entity Watch and Spatial Indexing

**Test:** `TestIntegration_EntityWatchAndIndex`

- Setup real NATS with KV buckets
- Start component watching ENTITY_STATES
- Add entity with geospatial data to ENTITY_STATES:
  ```json
  {
    "id": "sensor-001",
    "location": {
      "lat": 37.7749,
      "lon": -122.4194
    }
  }
  ```
- Verify SPATIAL_INDEX bucket contains geohash entry
- Verify geohash precision matches config (default 6)

**Success Criteria:**
- Entity appears in SPATIAL_INDEX within 5 seconds
- Geohash computed correctly for coordinates
- Index entry contains entity ID

---

### 3. Batch Processing

**Test:** `TestIntegration_BatchProcessing`

- Setup real NATS with KV buckets
- Configure component with BatchSize=10
- Start component
- Add 25 entities with geospatial data rapidly
- Verify all 25 entities indexed in SPATIAL_INDEX
- Verify batch processing metrics

**Success Criteria:**
- All entities indexed successfully
- Processing completes within reasonable time (< 10s)
- No entities lost during batching

---

### 4. Multiple Workers

**Test:** `TestIntegration_ConcurrentWorkers`

- Setup real NATS with KV buckets
- Configure component with Workers=4
- Start component
- Add 100 entities with geospatial data concurrently
- Verify all entities indexed in SPATIAL_INDEX
- Verify no duplicate entries

**Success Criteria:**
- All 100 entities indexed
- No duplicates in index
- Thread-safe operation (run with `-race`)

---

### 5. Context Cancellation

**Test:** `TestIntegration_ContextCancellation`

- Setup real NATS with KV buckets
- Start component with cancellable context
- Begin adding entities
- Cancel context mid-processing
- Verify component stops gracefully
- Verify partial results are persisted

**Success Criteria:**
- Component respects context cancellation
- Shutdown completes within 5 seconds
- No hanging goroutines
- No corrupted index entries

---

### 6. Geohash Precision Levels

**Test:** `TestIntegration_GeohashPrecisionLevels`

- Setup real NATS with KV buckets
- Test with precision levels: 5, 6, 8, 12
- Add same entity coordinates with each precision
- Verify geohash length matches precision
- Verify different precisions produce different geohashes

**Success Criteria:**
- Geohash length equals configured precision
- Precision 5 produces 5-character geohash
- Precision 12 produces 12-character geohash
- Higher precision is more specific (substring of lower)

---

### 7. Invalid Geospatial Data Handling

**Test:** `TestIntegration_InvalidGeospatialData`

- Setup real NATS with KV buckets
- Start component
- Add entities with invalid coordinates:
  - Missing location field
  - Invalid latitude (> 90)
  - Invalid longitude (> 180)
  - Non-numeric coordinates
  - Null coordinates
- Verify errors logged but component continues
- Verify error metrics incremented

**Success Criteria:**
- Component doesn't crash on invalid data
- Errors logged with structured logging
- Error count metrics incremented
- Valid entities still processed

---

### 8. Update and Delete Operations

**Test:** `TestIntegration_UpdateAndDelete`

- Setup real NATS with KV buckets
- Start component
- Add entity with coordinates
- Update entity with new coordinates
- Verify old geohash removed, new geohash added
- Delete entity from ENTITY_STATES
- Verify entity removed from SPATIAL_INDEX

**Success Criteria:**
- Updates reflected in SPATIAL_INDEX
- Old entries cleaned up
- Deletes propagate correctly
- No orphaned index entries

---

### 9. KV Bucket Recovery

**Test:** `TestIntegration_KVBucketRecovery`

- Setup real NATS with KV buckets
- Start component
- Simulate KV bucket error (NATS restart)
- Verify component detects error
- Verify component recovers when bucket available
- Verify processing resumes

**Success Criteria:**
- Component detects bucket unavailability
- Health status shows degraded during outage
- Automatic recovery when bucket returns
- No data loss after recovery

---

### 10. Metrics and Observability

**Test:** `TestIntegration_MetricsAndObservability`

- Setup real NATS with KV buckets
- Start component
- Process known number of entities
- Verify DataFlow metrics accurate:
  - MessagesPerSecond > 0
  - BytesPerSecond > 0
  - LastActivity recent
- Verify Health metrics accurate:
  - Uptime increasing
  - ErrorCount accurate

**Success Criteria:**
- All metrics reflect actual processing
- Metrics thread-safe (run with `-race`)
- Metrics reset on restart

---

## Test Setup Helper Requirements

Builder must provide:

```go
// setupTestNATS creates a real NATS connection for integration tests
func setupTestNATS(t *testing.T) *natsclient.Client

// setupTestKVBuckets creates ENTITY_STATES and SPATIAL_INDEX buckets
func setupTestKVBuckets(t *testing.T, js jetstream.JetStream) (entity, spatial jetstream.KeyValue)

// createTestEntity returns a test entity with valid geospatial data
func createTestEntity(id string, lat, lon float64) []byte

// waitForIndexEntry waits for an entry to appear in SPATIAL_INDEX (max 5s)
func waitForIndexEntry(t *testing.T, bucket jetstream.KeyValue, key string) []byte
```

---

## Running Integration Tests

```bash
# Run all integration tests
task test:int

# Run with race detector
go test -race -tags=integration ./processor/graph-index-spatial/...

# Run specific test
go test -tags=integration -run TestIntegration_EntityWatchAndIndex ./processor/graph-index-spatial/
```

---

## Success Criteria Summary

All integration tests must:
- [ ] Use real NATS JetStream (not mocks)
- [ ] Clean up resources in `t.Cleanup()`
- [ ] Pass with `-race` flag
- [ ] Complete within reasonable time (< 30s each)
- [ ] Skip if NATS unavailable with clear message
- [ ] Use structured assertions (require/assert)
- [ ] Test actual geospatial indexing behavior
