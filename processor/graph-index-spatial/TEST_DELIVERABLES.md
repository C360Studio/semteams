# Test Suite: graph-index-spatial Component

## Mode: Greenfield

## Requirements Covered

| Spec Requirement | Test Function |
|------------------|---------------|
| Component type is "processor" | TestComponent_Meta_ReturnsCorrectMetadata |
| Implements Discoverable interface (6 methods) | TestComponent_Meta_*, TestComponent_InputPorts_*, etc. |
| Implements LifecycleComponent interface (3 methods) | TestComponent_Initialize_*, TestComponent_Start_*, TestComponent_Stop_* |
| Watch ENTITY_STATES KV bucket | TestComponent_InputPorts_ReturnsKVWatchPort |
| Write to SPATIAL_INDEX KV bucket | TestComponent_OutputPorts_ReturnsSpatialIndex |
| GeohashPrecision config (1-12) | TestConfig_Validate_InvalidGeohashPrecision |
| Workers config (min 1) | TestConfig_Validate_InvalidWorkers |
| BatchSize config (min 1) | TestConfig_Validate_InvalidBatchSize |
| Default config values | TestConfig_ApplyDefaults, TestDefaultConfig_ReturnsValidConfig |
| Factory function creates component | TestCreateGraphIndexSpatial_ValidConfig |
| Registry registration | TestRegister_AddsToRegistry |
| Context cancellation handling | TestComponent_RespectsContext_Cancellation |
| Thread-safe concurrent access | TestComponent_ConcurrentHealthChecks_ThreadSafe |

## Files Created (LOCKED)

- `/Users/coby/Code/c360/semstreams/processor/graph-index-spatial/component_test.go`
- `/Users/coby/Code/c360/semstreams/processor/graph-index-spatial/INTEGRATION_TEST_REQUIREMENTS.md`
- `/Users/coby/Code/c360/semstreams/processor/graph-index-spatial/TEST_DELIVERABLES.md`

## Unit Test Inventory

### Config Tests (11 tests)
```
TestConfig_Validate_ValidConfig/valid_minimal_config
TestConfig_Validate_ValidConfig/valid_config_with_custom_precision
TestConfig_Validate_ValidConfig/valid_config_with_precision_5
TestConfig_Validate_MissingPorts/missing_ports_config
TestConfig_Validate_MissingPorts/empty_inputs
TestConfig_Validate_MissingPorts/empty_outputs
TestConfig_Validate_MissingPorts/missing_SPATIAL_INDEX_output
TestConfig_Validate_InvalidGeohashPrecision/precision_too_low
TestConfig_Validate_InvalidGeohashPrecision/precision_negative
TestConfig_Validate_InvalidGeohashPrecision/precision_minimum_valid
TestConfig_Validate_InvalidGeohashPrecision/precision_normal
TestConfig_Validate_InvalidGeohashPrecision/precision_maximum_valid
TestConfig_Validate_InvalidGeohashPrecision/precision_too_high
TestConfig_Validate_InvalidWorkers/workers_zero
TestConfig_Validate_InvalidWorkers/workers_negative
TestConfig_Validate_InvalidWorkers/workers_valid_minimum
TestConfig_Validate_InvalidWorkers/workers_valid_normal
TestConfig_Validate_InvalidBatchSize/batch_size_zero
TestConfig_Validate_InvalidBatchSize/batch_size_negative
TestConfig_Validate_InvalidBatchSize/batch_size_valid_minimum
TestConfig_Validate_InvalidBatchSize/batch_size_valid_normal
TestConfig_ApplyDefaults
TestDefaultConfig_ReturnsValidConfig
```

### Discoverable Interface Tests (6 tests)
```
TestComponent_Meta_ReturnsCorrectMetadata
TestComponent_InputPorts_ReturnsKVWatchPort
TestComponent_OutputPorts_ReturnsSpatialIndex
TestComponent_ConfigSchema_ReturnsValidSchema
TestComponent_Health_NotStarted
TestComponent_Health_Running (skipped - requires NATS)
TestComponent_DataFlow_ReturnsMetrics
```

### LifecycleComponent Interface Tests (9 tests)
```
TestComponent_Initialize_Success
TestComponent_Initialize_InvalidConfig
TestComponent_Initialize_Idempotent
TestComponent_Start_Success (skipped - requires NATS)
TestComponent_Start_BeforeInitialize
TestComponent_Start_AlreadyStarted (skipped - requires NATS)
TestComponent_Stop_Success (skipped - requires NATS)
TestComponent_Stop_BeforeStart
TestComponent_Stop_Timeout (skipped - requires NATS)
```

### Factory and Registration Tests (6 tests)
```
TestCreateGraphIndexSpatial_ValidConfig
TestCreateGraphIndexSpatial_EmptyConfig
TestCreateGraphIndexSpatial_InvalidConfig
TestCreateGraphIndexSpatial_MissingDependencies
TestCreateGraphIndexSpatial_CustomConfig
TestRegister_AddsToRegistry
```

### Context Handling Tests (2 tests)
```
TestComponent_RespectsContext_Cancellation (skipped - requires NATS)
TestComponent_RespectsContext_Timeout
```

### Concurrent Operations Tests (2 tests)
```
TestComponent_ConcurrentHealthChecks_ThreadSafe
TestComponent_ConcurrentMetaAccess_ThreadSafe
```

### Error Handling Tests (2 tests)
```
TestComponent_InitializeError_InvalidConfig
TestComponent_MultipleConfigValidations/valid_minimal
TestComponent_MultipleConfigValidations/valid_custom_precision
TestComponent_MultipleConfigValidations/invalid_-_precision_too_low
TestComponent_MultipleConfigValidations/invalid_-_workers_zero
```

**Total Unit Tests:** 39 test cases (including sub-tests)
**Skipped (integration):** 5 tests marked for integration testing

## Integration Test Requirements

Builder must create integration tests (`//go:build integration`) covering:

1. **Real NATS JetStream Integration**
   - [ ] Component lifecycle with real NATS
   - [ ] Health status verification
   - [ ] Graceful shutdown

2. **Entity Watch and Spatial Indexing**
   - [ ] Watch ENTITY_STATES bucket
   - [ ] Extract geospatial data from entities
   - [ ] Write geohash to SPATIAL_INDEX
   - [ ] Verify geohash precision

3. **Batch Processing**
   - [ ] Process multiple entities in batches
   - [ ] Verify BatchSize configuration works
   - [ ] No entities lost during batching

4. **Multiple Workers**
   - [ ] Concurrent processing with Workers > 1
   - [ ] No duplicate index entries
   - [ ] Thread-safe with race detector

5. **Context Cancellation**
   - [ ] Respects context cancellation
   - [ ] Graceful shutdown during processing
   - [ ] No hanging goroutines

6. **Geohash Precision Levels**
   - [ ] Test precision levels 5, 6, 8, 12
   - [ ] Verify geohash length matches precision
   - [ ] Higher precision more specific

7. **Invalid Geospatial Data Handling**
   - [ ] Missing location fields
   - [ ] Invalid coordinates (out of range)
   - [ ] Non-numeric values
   - [ ] Component continues processing

8. **Update and Delete Operations**
   - [ ] Update entity coordinates
   - [ ] Old geohash removed, new added
   - [ ] Delete entity from index
   - [ ] No orphaned entries

9. **KV Bucket Recovery**
   - [ ] Detect bucket unavailability
   - [ ] Automatic recovery
   - [ ] No data loss

10. **Metrics and Observability**
    - [ ] MessagesPerSecond accurate
    - [ ] BytesPerSecond accurate
    - [ ] ErrorCount accurate
    - [ ] Health status reflects reality

See `INTEGRATION_TEST_REQUIREMENTS.md` for detailed specifications.

## Test Coverage Expectations

### Critical Paths (Must Cover)
- ✅ Config validation (all fields, all edge cases)
- ✅ All 9 interface methods implemented
- ✅ Factory function creates valid component
- ✅ Registry registration works
- ✅ Initialize validates config
- ✅ Thread-safe concurrent access
- 🔄 Real NATS integration (integration tests)
- 🔄 Geospatial indexing behavior (integration tests)

### Edge Cases Covered
- ✅ Nil/missing config
- ✅ Invalid config values (zero, negative, out of range)
- ✅ Empty ports configuration
- ✅ Missing required outputs
- ✅ Geohash precision boundaries (0, 1, 12, 13)
- ✅ Workers boundaries (0, 1)
- ✅ BatchSize boundaries (0, 1)
- ✅ Initialize before start check
- ✅ Idempotent Initialize and Start
- ✅ Safe Stop before Start
- ✅ Context timeout handling
- ✅ Concurrent health checks (50 goroutines × 100 iterations)

### Error Handling
- ✅ Config validation errors
- ✅ Missing dependencies (NATSClient)
- ✅ Invalid JSON unmarshaling
- ✅ Initialize without valid config
- ✅ Start before Initialize

## Handoff to Builder

### Builder Must:

1. **Create Component Implementation**
   - File: `processor/graph-index-spatial/component.go`
   - Implement all 9 interface methods
   - Follow exact patterns from `processor/graph-clustering/component.go`
   - Component type MUST be "processor"
   - Component name MUST be "graph-index-spatial"

2. **Make All Unit Tests Pass**
   - Cannot modify `component_test.go` (LOCKED)
   - Run: `go test ./processor/graph-index-spatial/`
   - Run with race detector: `go test -race ./processor/graph-index-spatial/`
   - All tests must pass (skipped tests excluded)

3. **Write Integration Tests**
   - File: `processor/graph-index-spatial/component_integration_test.go`
   - Build tag: `//go:build integration`
   - Follow requirements in `INTEGRATION_TEST_REQUIREMENTS.md`
   - Minimum 10 integration test scenarios

4. **Implement Geospatial Indexing Logic**
   - Extract lat/lon from entity data
   - Compute geohash at configured precision
   - Store in SPATIAL_INDEX KV bucket
   - Handle invalid/missing coordinates gracefully

5. **Run All Verification Tasks**
   - `task test` - unit tests must pass
   - `task test:int` - integration tests must pass
   - `task lint` - no linting errors
   - All with `-race` flag

### Implementation Notes

**Config Structure:**
```go
type Config struct {
    Ports            *component.PortConfig `json:"ports"`
    GeohashPrecision int                   `json:"geohash_precision"`
    Workers          int                   `json:"workers"`
    BatchSize        int                   `json:"batch_size"`
}
```

**Validation Rules:**
- Ports: required, non-empty inputs/outputs
- SPATIAL_INDEX output: required
- GeohashPrecision: 1-12 inclusive
- Workers: >= 1
- BatchSize: >= 1

**Defaults:**
- GeohashPrecision: 6
- Workers: 4
- BatchSize: 100

**Port Definitions:**
- Input: KV watch on ENTITY_STATES
- Output: KV write to SPATIAL_INDEX

**Component Metadata:**
- Type: "processor"
- Name: "graph-index-spatial"
- Description: "Geospatial indexing processor for graph entities"
- Version: "1.0.0"

### Files Builder Will Create

- `processor/graph-index-spatial/component.go` (main implementation)
- `processor/graph-index-spatial/component_integration_test.go` (integration tests)

### Files Builder Cannot Modify

- `processor/graph-index-spatial/component_test.go` (LOCKED - Tester owned)
- `processor/graph-index-spatial/INTEGRATION_TEST_REQUIREMENTS.md` (Tester owned)
- `processor/graph-index-spatial/TEST_DELIVERABLES.md` (Tester owned)

## Verification Checklist

Before declaring task complete, Builder must verify:

- [ ] All unit tests pass: `go test ./processor/graph-index-spatial/`
- [ ] All integration tests pass: `go test -tags=integration ./processor/graph-index-spatial/`
- [ ] Race detector clean: `go test -race ./processor/graph-index-spatial/`
- [ ] Race detector clean (integration): `go test -race -tags=integration ./processor/graph-index-spatial/`
- [ ] Linting passes: `task lint`
- [ ] Component registered with registry
- [ ] All 9 interface methods implemented
- [ ] Geospatial indexing works with real NATS
- [ ] No test files modified (component_test.go unchanged)

## Success Metrics

**Unit Tests:**
- 39+ test cases covering all requirements
- 100% of config validation logic covered
- All interface methods verified
- Thread safety verified (concurrent tests)

**Integration Tests:**
- 10+ real NATS scenarios
- Actual geospatial indexing tested
- Error handling verified
- Metrics accuracy verified

**Code Quality:**
- Follows Go patterns from `docs/agents/go-patterns.md`
- Structured logging with slog
- Error wrapping with errs package
- Atomic counters for metrics
- Proper context propagation
- Clean goroutine management
