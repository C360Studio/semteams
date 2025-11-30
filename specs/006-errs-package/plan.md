# Implementation Plan: Move and Rename Errors Package

**Branch**: `006-errs-package` | **Date**: 2025-11-29 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/006-errs-package/spec.md`

## Summary

Move the `errors/` package from repo root to `pkg/errs/` and rename from `errors` to `errs` to avoid stdlib naming collision. This is a pure refactoring task with no functional changes - all existing types, functions, constants, and sentinel errors are preserved with only import path and package name updates required.

## Technical Context

**Language/Version**: Go 1.25+
**Primary Dependencies**: `github.com/c360/semstreams/pkg/retry` (internal dependency, unchanged)
**Storage**: N/A (utility package, no persistence)
**Testing**: `go test` with `-race` flag
**Target Platform**: Linux/macOS (same as existing)
**Project Type**: Single Go module (package move within existing structure)
**Performance Goals**: N/A (no performance changes - refactoring only)
**Constraints**: Zero functional changes, all tests must pass after migration
**Scale/Scope**: 99 files require import path updates

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Spec-First Development | PASS | Specification complete with success criteria and requirements |
| II. TDD (NON-NEGOTIABLE) | PASS | Existing tests preserved; TDD applies to any new test code |
| III. Quality Gate Compliance | PASS | Six gates will be applied during implementation |
| IV. Code Review Standards | PASS | Review required before integration |
| V. Documentation & Traceability | PASS | doc.go will be updated; ADR not required (routine refactor) |

**Gate Evaluation**: All constitution gates pass. This is a straightforward refactoring task:
- No new architecture decisions requiring ADR
- No new APIs or data models
- Existing test coverage preserved
- Standard Go tooling (`go fmt`, `revive`) validates changes

## Project Structure

### Documentation (this feature)

```text
specs/006-errs-package/
├── plan.md              # This file
├── research.md          # Phase 0: migration strategy validation
├── data-model.md        # Phase 1: package public API inventory
├── quickstart.md        # Phase 1: migration steps for developers
└── tasks.md             # Phase 2 output (/speckit.tasks command)
```

### Source Code (repository root)

```text
# Current structure (before)
errors/
├── doc.go
├── errors.go
└── errors_test.go

pkg/
├── acme/
├── buffer/
├── cache/
├── retry/            # errs depends on this
├── security/
├── timestamp/
├── tlsutil/
└── worker/

# Target structure (after)
pkg/
├── acme/
├── buffer/
├── cache/
├── errs/             # NEW: moved and renamed
│   ├── doc.go        # Updated package declaration
│   ├── errs.go       # Renamed, updated package declaration
│   └── errs_test.go  # Renamed, updated package declaration
├── retry/
├── security/
├── timestamp/
├── tlsutil/
└── worker/

# Root errors/ directory: DELETED
```

**Structure Decision**: Package moved to `pkg/errs/` to align with existing utility packages (buffer, cache, retry, etc.). Files renamed to match new package name following Go conventions.

## Complexity Tracking

> No constitution violations. This is a routine package refactoring with:
> - No new abstractions introduced
> - No architectural changes
> - No external dependencies added
> - Simple find-and-replace migration pattern

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| None | N/A | N/A |
