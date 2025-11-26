# Implementation Plan: Graphable Examples

**Branch**: `002-graphable-examples` | **Date**: 2025-11-26 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/002-graphable-examples/spec.md`

## Summary

Remove the `json_to_entity` processor (anti-pattern that makes semantic decisions without domain knowledge) and replace it with a proper IoT sensor example that demonstrates the correct pattern: domain-specific payloads implementing the Graphable interface with federated entity IDs and semantic predicates. Update all documentation to reflect the processor design philosophy.

## Technical Context

**Language/Version**: Go 1.25+
**Primary Dependencies**: message package (Graphable interface), vocabulary package (predicate registration)
**Storage**: N/A (example code, no persistence)
**Testing**: `go test -race`, table-driven tests
**Target Platform**: Linux server (NATS JetStream infrastructure)
**Project Type**: Single Go project with package structure
**Performance Goals**: N/A (example code for reference, not production)
**Constraints**: Must demonstrate all aspects of Graphable pattern (6-part IDs, semantic predicates, vocabulary registration)
**Scale/Scope**: ~5 files to remove (json_to_entity), ~8 files to create (example), ~10 docs to update

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Spec-First Development | PASS | Specification complete with user stories and requirements |
| II. TDD (Non-Negotiable) | PASS | Example will include tests demonstrating Graphable contract |
| III. Quality Gate Compliance | PASS | Following 6-gate process |
| IV. Code Review Standards | PASS | Will submit for go-reviewer approval |
| V. Documentation & Traceability | PASS | Core goal is documentation consistency |
| Go Standards | PASS | Will run `go fmt`, `revive`, `go test -race` |

No constitution violations. Proceeding with implementation.

## Project Structure

### Documentation (this feature)

```text
specs/002-graphable-examples/
├── plan.md              # This file
├── spec.md              # Feature specification
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
└── checklists/
    └── requirements.md  # Specification quality checklist
```

### Source Code (repository root)

```text
# Files to REMOVE
processor/json_to_entity/
├── json_to_entity.go
├── json_to_entity_test.go
├── json_to_entity_integration_test.go
└── config.go

# Files to CREATE
examples/
└── processors/
    └── iot_sensor/
        ├── README.md           # How to use and adapt the example
        ├── payload.go          # SensorReading implementing Graphable
        ├── payload_test.go     # Tests for Graphable contract
        ├── processor.go        # Domain processor logic
        ├── processor_test.go   # Processor tests
        └── vocabulary.go       # Predicate registration for IoT domain

# Documentation to UPDATE
docs/
├── SPEC-SEMANTIC-CONTRACT.md   # Update to reference new example
├── PROCESSOR-DESIGN-PHILOSOPHY.md  # Already references IoT example
└── [other docs referencing json_to_entity]
```

**Structure Decision**: This is a refactoring/example task. Removing anti-pattern processor from `processor/` and adding reference implementation in `examples/processors/`.

## Complexity Tracking

No constitution violations requiring justification.
