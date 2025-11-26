# Implementation Plan: Remove Legacy RDF Predicates

**Branch**: `001-predicate-notation` | **Date**: 2025-11-26 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/001-predicate-notation/spec.md`

## Summary

Remove the legacy `rdf:type` and `rdf:class` triple generation from EntityPayload.Triples(). These predicates are redundant (entity type/class already exist as struct fields) and use colon notation which breaks the vocabulary registry pattern. Update documentation examples to use real predicates like `sensor.temperature.celsius`.

## Technical Context

**Language/Version**: Go 1.25+
**Primary Dependencies**: message package
**Storage**: N/A (code removal only)
**Testing**: `go test -race`, table-driven tests
**Target Platform**: Linux server (NATS JetStream infrastructure)
**Project Type**: Single Go project with package structure
**Performance Goals**: Slight improvement (fewer triples generated)
**Constraints**: Tests expecting rdf:type/rdf:class triples must be updated
**Scale/Scope**: 4 files to modify, ~20 lines of code removed

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Spec-First Development | PASS | Specification complete |
| II. TDD (Non-Negotiable) | PASS | Tests will be updated |
| III. Quality Gate Compliance | PASS | Following 6-gate process |
| IV. Code Review Standards | PASS | Will submit for go-reviewer approval |
| V. Documentation & Traceability | PASS | Documentation examples will be updated |
| Go Standards | PASS | Will run `go fmt`, `revive`, `go test -race` |

No constitution violations. Proceeding with implementation.

## Project Structure

### Documentation (this feature)

```text
specs/001-predicate-notation/
├── plan.md              # This file
├── spec.md              # Feature specification
├── research.md          # Phase 0 output
├── tasks.md             # Task breakdown
└── checklists/
    └── requirements.md  # Specification quality checklist
```

### Source Code (affected files)

```text
message/
├── entity_payload.go      # Remove rdf:type and rdf:class triple generation (lines 124-144)
└── graphable.go           # Update doc examples to use geo.location.latitude

processor/json_to_entity/
├── json_to_entity_test.go           # Remove rdf:type expectations
└── json_to_entity_integration_test.go  # Remove rdf:type expectations
```

**Structure Decision**: This is a removal/cleanup task. No new code or directories needed.

## Complexity Tracking

No constitution violations requiring justification.
