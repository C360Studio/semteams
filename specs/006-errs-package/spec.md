# Feature Specification: Move and Rename Errors Package

**Feature Branch**: `006-errs-package`
**Created**: 2025-11-29
**Status**: Draft
**Input**: User description: "Move and rename errors package to pkg/errs for Go stdlib compatibility"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Developer Uses errs Package Without Naming Conflicts (Priority: P1)

As a Go developer working on SemStreams, I want to import the project's error utilities without naming conflicts with the Go standard library `errors` package, so I can use both packages cleanly in the same file without aliases.

**Why this priority**: This is the primary motivation for the rename - eliminating the constant need for import aliases and potential confusion when both packages are used together.

**Independent Test**: Can be verified by importing both `errors` (stdlib) and `github.com/c360/semstreams/pkg/errs` in the same file without compilation errors or alias requirements.

**Acceptance Scenarios**:

1. **Given** a Go source file importing both stdlib `errors` and the project's error utilities, **When** the developer compiles the code, **Then** it compiles successfully without import alias conflicts.
2. **Given** an existing codebase using `github.com/c360/semstreams/errors`, **When** imports are updated to `github.com/c360/semstreams/pkg/errs`, **Then** all existing functionality remains intact.

---

### User Story 2 - Consistent Package Organization (Priority: P2)

As a project maintainer, I want all utility packages to reside in the `pkg/` directory, so the codebase follows Go project layout conventions and developers know where to find shared utilities.

**Why this priority**: Organizational consistency improves discoverability and aligns with the existing `pkg/` structure (retry, cache, buffer, etc.).

**Independent Test**: Can be verified by confirming the package exists at `pkg/errs/` and follows the same patterns as sibling packages.

**Acceptance Scenarios**:

1. **Given** the `pkg/` directory structure, **When** a developer looks for utility packages, **Then** the `errs` package is found alongside other utilities (retry, cache, buffer).
2. **Given** the old `errors/` directory at the repo root, **When** the migration is complete, **Then** the root-level `errors/` directory no longer exists.

---

### User Story 3 - Zero Breaking Changes for Consumers (Priority: P1)

As a developer with code depending on the errors package, I want the migration to preserve all existing types, functions, and behaviors, so my code works after updating import paths.

**Why this priority**: Tied with P1 - functionality preservation is critical for a safe migration.

**Independent Test**: Can be verified by running all existing tests after the migration with no failures related to errs package usage.

**Acceptance Scenarios**:

1. **Given** code using `ErrorClass`, `ClassifiedError`, `Wrap`, `WrapTransient`, `WrapFatal`, `WrapInvalid`, **When** imports are updated, **Then** all functions and types work identically.
2. **Given** code using error sentinel values (ErrAlreadyStarted, ErrNotStarted, etc.), **When** imports are updated, **Then** all sentinel values remain available with identical values.
3. **Given** code using `IsTransient`, `IsFatal`, `IsInvalid`, `Classify` functions, **When** imports are updated, **Then** error classification behavior is unchanged.

---

### Edge Cases

- What happens if code imports the old path `github.com/c360/semstreams/errors`? Build fails with clear "package not found" error directing to new location.
- What happens if developers mix old and new import paths in the same project? Build fails; all imports must be migrated together.
- How does the system handle the `errs` package importing `pkg/retry`? No change - this internal dependency remains valid.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Package MUST be moved from `errors/` (repo root) to `pkg/errs/`
- **FR-002**: Package name MUST change from `errors` to `errs` to avoid stdlib naming collision
- **FR-003**: All public types MUST be preserved: `ErrorClass`, `ClassifiedError`, `RetryConfig`
- **FR-004**: All public functions MUST be preserved: `Wrap`, `WrapTransient`, `WrapFatal`, `WrapInvalid`, `IsTransient`, `IsFatal`, `IsInvalid`, `Classify`, `DefaultRetryConfig`
- **FR-005**: All public constants MUST be preserved: `ErrorTransient`, `ErrorInvalid`, `ErrorFatal`
- **FR-006**: All sentinel error variables MUST be preserved with identical values (ErrAlreadyStarted, ErrNotStarted, etc.)
- **FR-007**: All 99 importing files MUST be updated from `"github.com/c360/semstreams/errors"` to `"github.com/c360/semstreams/pkg/errs"`
- **FR-008**: The old `errors/` directory MUST be removed after migration
- **FR-009**: All existing tests MUST pass after migration
- **FR-010**: Internal package documentation (doc.go) MUST be updated to reflect new package name and location

### Key Entities

- **errs package**: The renamed error utilities package providing standardized error handling patterns including error classification (transient/invalid/fatal), wrapped errors with context, and retry configuration integration.
- **ErrorClass**: Enumeration type for error classification (ErrorTransient, ErrorInvalid, ErrorFatal).
- **ClassifiedError**: Wrapper type that attaches classification metadata to underlying errors.
- **Sentinel Errors**: Predefined error variables for common conditions (connection, storage, config, lifecycle, resource errors).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: All 99 files with `errors` imports successfully compile after import path updates
- **SC-002**: All existing unit tests in the errors package pass after rename to errs
- **SC-003**: All project-wide tests pass after the migration (go test ./...)
- **SC-004**: Zero stdlib `errors` import aliases required in files that use both packages
- **SC-005**: Old `errors/` directory removed from repository
- **SC-006**: Package resides at `pkg/errs/` following project layout conventions

## Assumptions

- The 99 files identified in the initial research represent the complete set of files requiring import updates
- No external projects depend on `github.com/c360/semstreams/errors` (internal package only)
- The `pkg/retry` package dependency from the errs package will continue to work (relative path unchanged)
- Standard Go tooling (go fmt, goimports) will be used to validate import correctness
