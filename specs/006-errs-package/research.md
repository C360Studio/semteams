# Research: Move and Rename Errors Package

**Feature**: 006-errs-package
**Date**: 2025-11-29

## Overview

This research validates the migration strategy for moving `errors/` to `pkg/errs/` and renaming the package from `errors` to `errs`.

## Research Areas

### 1. Go Package Naming Conventions

**Decision**: Rename package from `errors` to `errs`

**Rationale**:
- Standard Go practice recommends avoiding package names that shadow stdlib packages
- `errs` is a common abbreviation used in Go ecosystem (e.g., `github.com/pkg/errors` vs custom packages)
- Short, descriptive names align with Go naming conventions
- Eliminates need for import aliases when using both stdlib `errors` and project utilities

**Alternatives Considered**:
- `semstreamserrors` - Too verbose, violates Go naming conventions
- `serrors` - Ambiguous, could be confused with "structured errors" or "stream errors"
- `errutil` - Less intuitive than `errs`
- Keep `errors` name - Forces import aliases, causes confusion

### 2. Import Path Migration Strategy

**Decision**: Use find-and-replace across all 99 files

**Rationale**:
- Simple, atomic operation that can be verified with `go build`
- No intermediate state where code partially works
- Standard refactoring approach for Go packages

**Migration Pattern**:
```text
OLD: "github.com/c360/semstreams/errors"
NEW: "github.com/c360/semstreams/pkg/errs"
```

**Alternatives Considered**:
- Gradual migration with aliasing - Increases complexity, extends migration window
- Deprecation period - Unnecessary for internal-only package
- Go module replace directive - Overkill for simple package move

### 3. File Naming Within Package

**Decision**: Rename `errors.go` to `errs.go` and `errors_test.go` to `errs_test.go`

**Rationale**:
- Go convention: primary file often matches package name
- Maintains consistency with sibling packages in `pkg/`
- Clear indication that package name has changed

**Alternatives Considered**:
- Keep original filenames - Creates confusion (file named `errors.go` in package `errs`)
- Different naming scheme - Unnecessary complexity

### 4. Package Declaration Updates

**Decision**: Update all `package errors` declarations to `package errs`

**Files Requiring Update**:
- `doc.go` - Package documentation
- `errors.go` → `errs.go` - Main implementation
- `errors_test.go` → `errs_test.go` - Test file

**Validation**: `go build ./pkg/errs` must succeed after changes

### 5. Consumer Code Updates

**Decision**: Update all 99 consumer files with new import path

**Change Pattern**:
```go
// Before
import (
    "github.com/c360/semstreams/errors"
)

// After
import (
    "github.com/c360/semstreams/pkg/errs"
)
```

**Usage Pattern Updates**:
```go
// Before
errors.Wrap(err, "component", "method", "action")
errors.IsTransient(err)

// After
errs.Wrap(err, "component", "method", "action")
errs.IsTransient(err)
```

**Validation**: `go build ./...` must succeed after all updates

### 6. Internal Dependency on pkg/retry

**Decision**: No changes required - import path remains valid

**Current Import** (in errors/errors.go):
```go
import "github.com/c360/semstreams/pkg/retry"
```

**After Move** (in pkg/errs/errs.go):
```go
import "github.com/c360/semstreams/pkg/retry"  // Same path, still valid
```

**Rationale**: The `pkg/retry` import is an absolute module path, not a relative path, so moving the consuming package doesn't affect it.

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Missed import update | Low | Medium | `go build ./...` validates all imports |
| Broken test | Low | Low | `go test -race ./...` validates functionality |
| Merge conflicts | Low | Low | Complete migration in single commit |
| External consumers | None | N/A | Internal package only (verified) |

## Conclusion

No NEEDS CLARIFICATION items remain. The migration strategy is straightforward:

1. Create `pkg/errs/` directory
2. Copy files with renamed filenames
3. Update package declarations in all files
4. Update doc.go documentation
5. Update all 99 consumer imports
6. Verify with `go build ./...` and `go test -race ./...`
7. Delete old `errors/` directory

**Ready for Phase 1: Design & Contracts**
