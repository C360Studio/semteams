# Quickstart: errs Package Migration

**Feature**: 006-errs-package
**Date**: 2025-11-29

## Overview

This guide provides step-by-step instructions for migrating from `errors` to `errs` package.

## Prerequisites

- Go 1.25+
- Access to SemStreams repository
- All local changes committed or stashed

## Migration Steps

### Step 1: Create the New Package Directory

```bash
mkdir -p pkg/errs
```

### Step 2: Copy and Rename Files

```bash
# Copy files with new names
cp errors/errors.go pkg/errs/errs.go
cp errors/errors_test.go pkg/errs/errs_test.go
cp errors/doc.go pkg/errs/doc.go
```

### Step 3: Update Package Declarations

In each file under `pkg/errs/`, change the package declaration:

```go
// Before
package errors

// After
package errs
```

Files to update:
- `pkg/errs/doc.go`
- `pkg/errs/errs.go`
- `pkg/errs/errs_test.go`

### Step 4: Update doc.go Documentation

Update references in `pkg/errs/doc.go`:

```go
// Before
// Package errors provides standardized error handling...
import "github.com/c360/semstreams/errors"

// After
// Package errs provides standardized error handling...
import "github.com/c360/semstreams/pkg/errs"
```

### Step 5: Update All Consumer Imports

Find and replace across the codebase:

```bash
# Using sed (macOS)
find . -name "*.go" -not -path "./errors/*" -not -path "./pkg/errs/*" \
  -exec sed -i '' 's|"github.com/c360/semstreams/errors"|"github.com/c360/semstreams/pkg/errs"|g' {} +

# Using sed (Linux)
find . -name "*.go" -not -path "./errors/*" -not -path "./pkg/errs/*" \
  -exec sed -i 's|"github.com/c360/semstreams/errors"|"github.com/c360/semstreams/pkg/errs"|g' {} +
```

### Step 6: Update Usage References

Find and replace package qualifier:

```bash
# Using sed (macOS)
find . -name "*.go" -not -path "./errors/*" -not -path "./pkg/errs/*" \
  -exec sed -i '' 's/errors\.\(Wrap\|WrapTransient\|WrapFatal\|WrapInvalid\|IsTransient\|IsFatal\|IsInvalid\|Classify\|DefaultRetryConfig\|ErrorClass\|ErrorTransient\|ErrorInvalid\|ErrorFatal\|ClassifiedError\|RetryConfig\|Err[A-Z][a-zA-Z]*\)/errs.\1/g' {} +
```

**Note**: This regex handles most cases but may need manual review for edge cases.

### Step 7: Verify Build

```bash
go build ./...
```

Fix any remaining import or usage issues.

### Step 8: Run Tests

```bash
# Run package tests
go test -race ./pkg/errs/...

# Run all tests
go test -race ./...
```

### Step 9: Run Linters

```bash
go fmt ./...
revive ./...
```

### Step 10: Delete Old Package

```bash
rm -rf errors/
```

### Step 11: Final Verification

```bash
# Ensure no references to old package remain
grep -r '"github.com/c360/semstreams/errors"' --include="*.go" .

# Should return no results

# Build and test one more time
go build ./...
go test -race ./...
```

## Usage After Migration

### Importing

```go
import "github.com/c360/semstreams/pkg/errs"
```

### Using with stdlib errors

```go
import (
    "errors"  // stdlib

    "github.com/c360/semstreams/pkg/errs"  // project utilities
)

func example() error {
    // stdlib errors
    if errors.Is(err, someStdlibError) {
        // ...
    }

    // project utilities - no alias needed!
    if errs.IsTransient(err) {
        // ...
    }

    return errs.Wrap(err, "Component", "Method", "action")
}
```

## Common Migration Issues

### Issue: Import alias conflicts

**Before**: Required alias when using both stdlib and project errors
```go
import (
    "errors"

    semerrors "github.com/c360/semstreams/errors"  // needed alias
)
```

**After**: No alias needed
```go
import (
    "errors"

    "github.com/c360/semstreams/pkg/errs"  // different name, no conflict
)
```

### Issue: Partial migration

**Symptom**: Build errors about undefined `errors.Wrap` etc.

**Fix**: Ensure all usage sites updated from `errors.` to `errs.`

### Issue: Test imports

**Remember**: Test files also need import updates
```go
// errors/errors_test.go uses internal references
// pkg/errs/errs_test.go should use package errs
```

## Rollback

If issues arise, restore from git:

```bash
git checkout -- .
```
