# Implementation Plan: E2E Test Suite Improvements

**Branch**: `007-e2e-test-improvements` | **Date**: 2025-11-30 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/007-e2e-test-improvements/spec.md`

## Summary

Enhance the E2E test suite to validate actual data processing and storage rather than just component health status. The current tests pass when components are "healthy" but don't verify that data actually flows through the pipeline and is stored in NATS KV buckets. This plan adds direct NATS KV validation, index population verification, and metrics consistency checks.

## Technical Context

**Language/Version**: Go 1.25+
**Primary Dependencies**: nats.go/jetstream (NATS client), stretchr/testify (assertions)
**Storage**: NATS JetStream KV buckets (ENTITY_STATES, PREDICATE_INDEX, SPATIAL_INDEX, ALIAS_INDEX, INCOMING_INDEX, OUTGOING_INDEX, TEMPORAL_INDEX)
**Testing**: Go test with `-race` flag, integration tests
**Target Platform**: Linux containers (Docker Compose)
**Project Type**: Go backend infrastructure
**Performance Goals**: E2E tests complete within 30 seconds per scenario
**Constraints**: Tests run in Docker environment, UDP packet loss acceptable (80% threshold)
**Scale/Scope**: 9 existing E2E scenarios, 3 Docker Compose configurations

## Constitution Check

### Pre-Phase 0 Gate (PASSED)

| Principle | Status | Notes |
|-----------|--------|-------|
| Spec-First Development | PASS | Spec complete at spec.md |
| TDD (NON-NEGOTIABLE) | PASS | Test utility code will follow TDD; this IS test code |
| Quality Gate Compliance | PASS | Standard 6-gate process applies |
| Code Review Standards | PASS | Changes require go-reviewer approval |
| Documentation & Traceability | PASS | E2E README will be updated |

**Gate Result**: PASS - Proceeded to Phase 0

### Post-Phase 1 Gate (PASSED)

| Principle | Status | Notes |
|-----------|--------|-------|
| Spec-First Development | PASS | research.md and data-model.md complete |
| TDD (NON-NEGOTIABLE) | PASS | NATSClient will have unit tests first |
| Quality Gate Compliance | PASS | No design changes affecting gates |
| Code Review Standards | PASS | Design reviewed, ready for implementation |
| Documentation & Traceability | PASS | quickstart.md provides developer guidance |

**Gate Result**: PASS - Ready for `/speckit.tasks`

## Project Structure

### Documentation (this feature)

```text
specs/007-e2e-test-improvements/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output (N/A - internal test tooling)
└── tasks.md             # Phase 2 output
```

### Source Code (repository root)

```text
test/e2e/
├── client/
│   ├── observability.go     # Existing HTTP client
│   └── nats.go              # NEW: Thin wrapper using natsclient pkg
├── config/
│   └── constants.go         # Existing test constants
├── scenarios/
│   ├── scenario.go          # Interface with enhanced Result struct
│   ├── core_health.go       # Updated terminology
│   ├── core_dataflow.go     # Updated terminology
│   ├── core_federation.go   # Updated terminology
│   ├── semantic_basic.go    # Add NATS KV validation
│   ├── semantic_indexes.go  # Add index population checks
│   ├── semantic_kitchen_sink.go  # Update metrics list
│   ├── iot_sensor_pipeline.go    # Updated terminology
│   ├── rules_graph.go       # Updated terminology
│   └── rules_performance.go # Updated terminology
└── README.md                # Updated documentation

natsclient/
└── client.go                # REUSE: Existing client with GetKeyValueBucket()

graph/
└── constants.go             # KV bucket name constants (reuse)
```

**Structure Decision**: Reuse existing `natsclient` package which already provides `GetKeyValueBucket()` and `ListKeyValueBuckets()`. The E2E client wrapper will be minimal - just convenience methods for test validation.

## Complexity Tracking

> No constitution violations required. All changes follow standard patterns.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| N/A | N/A | N/A |
