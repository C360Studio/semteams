# Feature Specification: E2E Test Suite Improvements

**Feature Branch**: `007-e2e-test-improvements`
**Created**: 2025-11-30
**Status**: Draft
**Input**: User description: "Improve e2e tests suite based on findings above"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Validate Actual Data Processing (Priority: P1)

As a developer running E2E tests, I want tests to verify that data actually flows through the pipeline and is stored correctly, not just that components are "healthy", so I can trust that the system is working as intended.

**Why this priority**: Current tests only validate component health status, which can pass even when data processing is broken. This is the core value proposition of E2E tests - proving the system actually works end-to-end.

**Independent Test**: Can be validated by sending a known message through the pipeline and verifying it appears in NATS KV storage with expected entity structure.

**Acceptance Scenarios**:

1. **Given** a running SemStreams instance with graph processor, **When** a test entity message is sent via UDP, **Then** the test can query NATS KV and verify the entity was stored with correct properties
2. **Given** a semantic-basic test run, **When** test entities are sent, **Then** the test verifies at least 80% of sent entities exist in ENTITY_STATES bucket
3. **Given** a test completes with "success", **When** reviewing test output, **Then** the output shows actual entity count stored vs sent, not just "component healthy"

---

### User Story 2 - Validate Index Population (Priority: P2)

As a developer working on semantic indexing, I want E2E tests to verify that indexes are actually populated when entities are stored, so I can catch indexing bugs before they reach production.

**Why this priority**: Indexes are critical for semantic search functionality. Currently tests assume indexes work if the graph processor is healthy, but index population could be silently failing.

**Independent Test**: Can be validated by sending entities with known properties and querying the predicate index to verify entries exist.

**Acceptance Scenarios**:

1. **Given** entities sent with `temperature` property, **When** semantic-indexes test completes, **Then** the test verifies predicate index contains entries for `temperature`
2. **Given** entities sent with geo-location data, **When** test validates indexes, **Then** the test verifies spatial index was populated
3. **Given** a semantic-indexes test fails, **When** reviewing the error, **Then** the error message specifies which index failed and what was expected vs actual

---

### User Story 3 - Consistent Metrics Validation (Priority: P3)

As a platform operator, I want E2E tests to validate that the correct Prometheus metrics are exposed and have reasonable values, so I can trust monitoring dashboards in production.

**Why this priority**: The semantic-kitchen-sink test validates metrics, but the required metrics list may be stale and doesn't reflect current implementation. Accurate metrics validation ensures observability works.

**Independent Test**: Can be validated by querying /metrics endpoint and verifying expected metric names exist with non-zero counters after test data is processed.

**Acceptance Scenarios**:

1. **Given** test data has been processed through the pipeline, **When** metrics are validated, **Then** the test checks for current actual metrics emitted by SemStreams (not a stale hardcoded list)
2. **Given** a metrics validation fails, **When** reviewing the error, **Then** the error shows which specific metrics are missing or have unexpected values
3. **Given** new metrics are added to the codebase, **When** E2E tests run, **Then** the metrics list is maintained as a curated static list in test configuration, updated when new metrics are added to production code

---

### User Story 4 - Clean Up Stale References (Priority: P4)

As a developer maintaining the codebase, I want E2E test code to use consistent naming and remove outdated references, so the codebase is easier to understand and maintain.

**Why this priority**: Lower priority but improves maintainability. Stale "StreamKit" references and inconsistent naming create confusion for new developers.

**Independent Test**: Can be validated by grep search for outdated terminology and verifying zero matches.

**Acceptance Scenarios**:

1. **Given** the E2E test codebase, **When** searching for "StreamKit", **Then** zero occurrences are found (all replaced with "SemStreams")
2. **Given** E2E test scenarios, **When** reviewing component names, **Then** names consistently match current component registry names

---

### Edge Cases

- What happens when NATS KV bucket doesn't exist (first run)?
- How does system handle partial entity storage (some succeed, some fail)?
- What happens when metrics endpoint is temporarily unavailable during validation?
- How do tests behave when running against an incomplete Docker environment?

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: E2E tests MUST include a NATS client capability to query KV buckets directly for validation
- **FR-002**: Semantic tests MUST verify entity storage by querying ENTITY_STATES KV bucket after sending test data
- **FR-003**: Semantic-indexes tests MUST verify index population by querying at least one index type (predicate, spatial, or alias)
- **FR-004**: Test results MUST report quantitative metrics (entities sent vs stored, indexes populated) not just pass/fail
- **FR-005**: Tests MUST fail if expected entity count in storage is below 80% of sent count (configurable threshold)
- **FR-006**: Metrics validation MUST use a curated static list of expected metrics maintained in `test/e2e/config/`, updated when production metrics change
- **FR-007**: All E2E test code MUST use "SemStreams" terminology consistently (no "StreamKit" references)
- **FR-008**: Test scenarios MUST use component names that match the component registry (e.g., "graph-processor" not just "graph" if that's the registered name)
- **FR-009**: NATS KV validation MUST handle the case where buckets don't exist and report a clear error

### Key Entities

- **Test Result**: Represents the outcome of an E2E scenario including quantitative validation metrics (entities_sent, entities_stored, indexes_populated, metrics_validated)
- **NATS KV Client**: A test utility for connecting to NATS and querying KV buckets (ENTITY_STATES, predicate indexes, etc.)
- **Validation Threshold**: Configurable success criteria (e.g., 80% entity storage rate, required metrics list)

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: All semantic E2E tests report actual entity storage counts, with at least 80% of sent entities verifiable in NATS KV
- **SC-002**: Semantic-indexes test validates at least 3 index types (predicate, spatial, alias) with non-empty entries after test run
- **SC-003**: Zero occurrences of "StreamKit" in E2E test codebase after cleanup
- **SC-004**: Test failure messages include specific details (expected vs actual counts, missing metrics names) enabling developers to diagnose issues within 5 minutes
- **SC-005**: Metrics validation uses a current list of 5+ core metrics that are verified present and non-zero after data processing

## Assumptions

- NATS JetStream KV buckets (ENTITY_STATES, indexes) follow the current naming conventions in the codebase
- The E2E test environment has network access to NATS on standard ports
- Prometheus metrics endpoint is available at /metrics on port 9090 during test runs
- The 80% entity storage threshold is appropriate for UDP-based input (some packet loss acceptable)
- Test scenarios should continue to use Docker Compose for environment setup
