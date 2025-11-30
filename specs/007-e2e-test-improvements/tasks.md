# Implementation Tasks: E2E Test Suite Improvements

**Feature**: 007-e2e-test-improvements
**Branch**: `007-e2e-test-improvements`
**Date**: 2025-11-30
**Plan**: [plan.md](./plan.md)

## Task Overview

| Phase | Tasks | Status |
|-------|-------|--------|
| Phase 1: Setup | 2 | complete |
| Phase 2: US1 - Data Processing Validation | 3 | complete |
| Phase 3: US2 - Index Population Validation | 2 | complete |
| Phase 4: US3 - Metrics Validation | 2 | complete |
| Phase 5: US4 - Terminology Cleanup | 2 | complete |
| Phase 6: Polish | 1 | complete |
| **Total** | **12** | **complete** |

---

## Phase 1: Setup

### Task 1.1: Create NATSValidationClient

**Status**: `complete`
**Agent**: go-developer
**Depends on**: None
**Blocks**: 2.1, 2.2, 3.1

**Description**:
Create `NATSValidationClient`, a thin wrapper around `natsclient.Client` in `test/e2e/client/nats.go` that provides convenience methods for E2E test validation.

**Acceptance Criteria**:
- [x] `NATSValidationClient` struct wraps `*natsclient.Client`
- [x] `NewNATSValidationClient(natsURL string)` constructor uses `natsclient.NewClient()`
- [x] `Close(ctx)` delegates to underlying client
- [x] `CountEntities(ctx)` counts keys in ENTITY_STATES bucket
- [x] `GetEntity(ctx, id)` retrieves and unmarshals entity from ENTITY_STATES
- [x] `ValidateIndexPopulated(ctx, indexName)` checks if index bucket has entries
- [x] `BucketExists(ctx, bucketName)` checks bucket existence
- [x] Handle edge case: bucket doesn't exist (return 0, nil not error)
- [x] Handle edge case: NATS connection fails (return clear error message)
- [x] Handle edge case: partial results during iteration (return count so far)
- [x] Unit tests written FIRST (TDD)
- [x] All tests pass with `-race` flag

**Test Strategy**:
```go
// Tests use mock or test NATS server
func TestNATSValidationClient_CountEntities(t *testing.T)
func TestNATSValidationClient_CountEntities_BucketNotExists(t *testing.T)
func TestNATSValidationClient_GetEntity(t *testing.T)
func TestNATSValidationClient_GetEntity_NotFound(t *testing.T)
func TestNATSValidationClient_ValidateIndexPopulated(t *testing.T)
func TestNATSValidationClient_ValidateIndexPopulated_Empty(t *testing.T)
func TestNATSValidationClient_BucketExists(t *testing.T)
func TestNATSValidationClient_ConnectionFailure(t *testing.T)
```

**Files**:
- `test/e2e/client/nats.go` (new)
- `test/e2e/client/nats_test.go` (new)

---

### Task 1.2: Create Validation Config and Result Types

**Status**: `complete`
**Agent**: go-developer
**Depends on**: None
**Blocks**: 2.1, 3.1, 4.1

**Description**:
Create configuration and result types for E2E validation in `test/e2e/config/validation.go`.

**Acceptance Criteria**:
- [x] `ValidationConfig` struct with configurable thresholds
- [x] `ValidationResult` struct with quantitative metrics
- [x] Default values defined as constants
- [x] `DefaultValidationConfig()` returns sensible defaults
- [x] Types documented with godoc comments

**Types**:
```go
type ValidationConfig struct {
    MinStorageRate    float64       // Default: 0.80
    RequiredIndexes   []string      // Default: ["PREDICATE_INDEX", "SPATIAL_INDEX", "ALIAS_INDEX"]
    RequiredMetrics   []string      // Defined per scenario
    ValidationTimeout time.Duration // Default: 5s
}

type ValidationResult struct {
    EntitiesSent     int
    EntitiesStored   int
    StorageRate      float64
    IndexesChecked   []string
    IndexesPopulated int
    MetricsVerified  []string
    MetricsMissing   []string
}
```

**Test Strategy**:
```go
// Validation types are simple structs; test default factory and field access
func TestDefaultValidationConfig(t *testing.T)
func TestValidationResult_StorageRate(t *testing.T)
```

**Files**:
- `test/e2e/config/validation.go` (new)
- `test/e2e/config/validation_test.go` (new)

---

## Phase 2: US1 - Validate Data Processing

> **User Story**: As a developer, I want E2E tests to verify that entities sent through the pipeline are actually stored in the data layer, so I can trust that the system is processing data correctly.

### Task 2.1: Add NATS KV Validation to semantic_basic.go

**Status**: `complete`
**Agent**: go-developer
**Depends on**: 1.1, 1.2
**Blocks**: None

**Description**:
Enhance `semantic_basic.go` scenario to validate that entities sent via UDP are stored in NATS KV.

**Acceptance Criteria**:
- [x] Create `NATSValidationClient` after sending entities
- [x] Wait for processing (use ValidationTimeout)
- [x] Count entities in ENTITY_STATES bucket
- [x] Calculate storage rate (stored/sent)
- [x] Add storage_rate to result metrics
- [x] Fail if storage rate < MinStorageRate (80%)
- [x] Graceful error handling if NATS unavailable
- [x] Integration test passes

**Implementation Pattern**:
```go
// In Execute() after sending entities:
natsClient, err := client.NewNATSValidationClient(natsURL)
if err != nil {
    result.Warnings = append(result.Warnings, "NATS connection failed")
    return result, nil
}
defer natsClient.Close(ctx)

time.Sleep(config.ValidationTimeout)

stored, err := natsClient.CountEntities(ctx)
if err != nil {
    result.Warnings = append(result.Warnings, "Could not count entities")
} else {
    rate := float64(stored) / float64(sent)
    result.Metrics["storage_rate"] = rate
    if rate < config.MinStorageRate {
        result.Errors = append(result.Errors,
            fmt.Sprintf("Storage rate %.2f below threshold %.2f", rate, config.MinStorageRate))
    }
}
```

**Files**:
- `test/e2e/scenarios/semantic_basic.go` (update)

---

### Task 2.2: Add Entity Retrieval Validation

**Status**: `complete`
**Agent**: go-developer
**Depends on**: 2.1
**Blocks**: None

**Description**:
Enhance validation to retrieve specific entities and verify their structure.

**Acceptance Criteria**:
- [x] Retrieve at least one entity by known ID
- [x] Verify entity has expected fields (id, type, properties)
- [x] Verify entity type matches expected format
- [x] Add entity validation to result reporting
- [x] Integration test passes

**Files**:
- `test/e2e/scenarios/semantic_basic.go` (update)

---

### Task 2.3: Document Entity Validation in README

**Status**: `complete`
**Agent**: technical-writer
**Depends on**: 2.1, 2.2
**Blocks**: None

**Description**:
Update E2E README with entity validation documentation.

**Acceptance Criteria**:
- [x] Document how entity validation works
- [x] Document configuration options (MinStorageRate)
- [x] Document expected entity format
- [x] Provide troubleshooting guidance

**Files**:
- `test/e2e/README.md` (update)

---

## Phase 3: US2 - Index Population Validation

> **User Story**: As a developer, I want E2E tests to verify that indexes are populated correctly, so I can trust that queries will return expected results.

### Task 3.1: Add Index Validation to semantic_indexes.go

**Status**: `complete`
**Agent**: go-developer
**Depends on**: 1.1, 1.2
**Blocks**: None

**Description**:
Enhance `semantic_indexes.go` scenario to validate index population.

**Acceptance Criteria**:
- [x] Check PREDICATE_INDEX has entries
- [x] Check SPATIAL_INDEX has entries
- [x] Check ALIAS_INDEX has entries
- [x] Report count of populated indexes
- [x] Add indexes_populated to result metrics
- [x] Fail if required indexes are empty
- [x] Integration test passes

**Index Names** (from graph/constants.go):
- `PREDICATE_INDEX`
- `SPATIAL_INDEX`
- `ALIAS_INDEX`
- `INCOMING_INDEX`
- `OUTGOING_INDEX`
- `TEMPORAL_INDEX`

**Files**:
- `test/e2e/scenarios/semantic_indexes.go` (update)

---

### Task 3.2: Document Index Validation

**Status**: `complete`
**Agent**: technical-writer
**Depends on**: 3.1
**Blocks**: None

**Description**:
Update E2E README with index validation documentation.

**Acceptance Criteria**:
- [x] Document which indexes are validated
- [x] Document index bucket naming conventions
- [x] Document how to add new index validation

**Files**:
- `test/e2e/README.md` (update)

---

## Phase 4: US3 - Metrics Validation

> **User Story**: As a developer, I want E2E tests to verify that expected Prometheus metrics are present, so I can trust that observability is working correctly.

### Task 4.1: Update Metrics List in semantic_kitchen_sink.go

**Status**: `complete`
**Agent**: go-developer
**Depends on**: 1.2
**Blocks**: None

**Description**:
Update the required metrics list to match current codebase metrics.

**Acceptance Criteria**:
- [x] Update requiredMetrics list with current metric names
- [x] Verify metric names match those in codebase
- [x] Add metrics_verified and metrics_missing to result
- [x] Integration test passes

**Required Metrics** (from research.md):
```go
requiredMetrics := []string{
    "indexmanager_events_processed",
    "indexmanager_index_updates_total",
    "semstreams_cache_hits_total",
    "semstreams_cache_misses_total",
    "semstreams_json_filter_matched_total",
}
```

**Files**:
- `test/e2e/scenarios/semantic_kitchen_sink.go` (update)

---

### Task 4.2: Document Metrics Validation

**Status**: `complete`
**Agent**: technical-writer
**Depends on**: 4.1
**Blocks**: None

**Description**:
Update E2E README with metrics validation documentation.

**Acceptance Criteria**:
- [x] Document which metrics are validated
- [x] Document how to add new metric requirements
- [x] Reference metric source files

**Files**:
- `test/e2e/README.md` (update)

---

## Phase 5: US4 - Terminology Cleanup

> **User Story**: As a developer, I want all E2E test code to use consistent "SemStreams" terminology, so the codebase is clear and maintainable.

### Task 5.1: Replace StreamKit References

**Status**: `complete`
**Agent**: go-developer
**Depends on**: None
**Blocks**: None

**Description**:
Find and replace all "StreamKit" references with "SemStreams".

**Acceptance Criteria**:
- [x] Search all files in test/e2e/ for "StreamKit"
- [x] Replace with "SemStreams" in comments and strings
- [x] Preserve case (StreamKit → SemStreams, streamkit → semstreams)
- [x] All tests still pass after changes

**Files** (from research.md):
- `test/e2e/client/observability.go`
- `test/e2e/scenarios/core_health.go`
- `test/e2e/scenarios/core_dataflow.go`
- `test/e2e/scenarios/*.go` (all scenario files)

---

### Task 5.2: Verify Terminology Consistency

**Status**: `complete`
**Agent**: go-reviewer
**Depends on**: 5.1
**Blocks**: None

**Description**:
Review all E2E code for terminology consistency.

**Acceptance Criteria**:
- [x] No "StreamKit" references remain
- [x] Consistent use of "SemStreams" throughout
- [x] Code review approved

**Files**:
- `test/e2e/**/*.go` (all Go files)

---

## Phase 6: Polish

### Task 6.1: Final README Update and Quickstart Verification

**Status**: `complete`
**Agent**: technical-writer
**Depends on**: 2.3, 3.2, 4.2, 5.2
**Blocks**: None

**Description**:
Final review and update of E2E documentation.

**Acceptance Criteria**:
- [x] README reflects all new validation capabilities
- [x] Running instructions are accurate
- [x] Troubleshooting section is complete
- [x] Quickstart matches implementation
- [x] All documentation reviewed and approved

**Files**:
- `test/e2e/README.md` (final review)
- `specs/007-e2e-test-improvements/quickstart.md` (verify accuracy)

---

## Dependency Graph

```text
Phase 1 (Setup)
├── 1.1 NATSValidationClient ────┬──▶ Phase 2 (2.1, 2.2)
│                                └──▶ Phase 3 (3.1)
└── 1.2 Validation Types ────────┬──▶ Phase 2 (2.1)
                                 ├──▶ Phase 3 (3.1)
                                 └──▶ Phase 4 (4.1)

Phase 2 (US1: Data Processing)
├── 2.1 Add NATS KV Validation ──▶ 2.2
├── 2.2 Entity Retrieval ────────▶ 2.3
└── 2.3 Documentation ───────────▶ Phase 6

Phase 3 (US2: Index Validation)
├── 3.1 Index Validation ────────▶ 3.2
└── 3.2 Documentation ───────────▶ Phase 6

Phase 4 (US3: Metrics)
├── 4.1 Update Metrics List ─────▶ 4.2
└── 4.2 Documentation ───────────▶ Phase 6

Phase 5 (US4: Terminology)
├── 5.1 Replace StreamKit ───────▶ 5.2
└── 5.2 Review ──────────────────▶ Phase 6

Phase 6 (Polish)
└── 6.1 Final Documentation (depends on 2.3, 3.2, 4.2, 5.2)
```

---

## Quality Gates Reference

Each task must pass through all 6 quality gates:

1. **Specification** (architect → developer) - Task requirements clear
2. **Readiness** (developer prep) - Dependencies met, test strategy defined
3. **Code Complete** (developer → reviewer) - Implementation done, tests pass
4. **Review** (reviewer approval) - Code review approved (BLOCKING)
5. **Validation** (automated) - All tests pass, no regressions
6. **Integration** (docs, deployment) - Documentation updated

---

## Execution Order

1. **Parallel Start**: Tasks 1.1, 1.2, and 5.1 can start in parallel
2. **Phase 2-4**: After Phase 1 completes, Phases 2-4 can run in parallel
3. **Phase 5**: Independent, can run anytime
4. **Phase 6**: After all other phases complete
