# Data Model: errs Package Public API

**Feature**: 006-errs-package
**Date**: 2025-11-29

## Overview

This document inventories all public types, constants, variables, and functions that must be preserved during the package migration from `errors` to `errs`. The API surface remains unchanged; only the package name and import path change.

## Types

### ErrorClass

```go
type ErrorClass int
```

Enumeration type representing error classification for handling purposes.

**Methods**:
- `func (ec ErrorClass) String() string` - Returns string representation ("transient", "invalid", "fatal", "unknown")

### ClassifiedError

```go
type ClassifiedError struct {
    Class     ErrorClass
    Err       error
    Message   string
    Component string
    Operation string
}
```

Wrapper type that attaches classification metadata to underlying errors.

**Methods**:
- `func (ce *ClassifiedError) Error() string` - Implements error interface
- `func (ce *ClassifiedError) Unwrap() error` - Returns underlying error for error chain support

### RetryConfig

```go
type RetryConfig struct {
    MaxRetries      int
    InitialDelay    time.Duration
    MaxDelay        time.Duration
    BackoffFactor   float64
    RetryableErrors []error
}
```

Configuration for retry operations with exponential backoff.

**Methods**:
- `func (rc RetryConfig) ShouldRetry(err error, attempt int) bool` - Determines if error should be retried
- `func (rc RetryConfig) ToRetryConfig() retry.Config` - Converts to pkg/retry Config type
- `func (rc RetryConfig) BackoffDelay(attempt int) time.Duration` - Calculates delay for retry attempt

## Constants

### ErrorClass Values

```go
const (
    ErrorTransient ErrorClass = iota  // Temporary errors that may be retried
    ErrorInvalid                       // Errors due to invalid input or configuration
    ErrorFatal                         // Unrecoverable errors that should stop processing
)
```

## Sentinel Error Variables

### Component Lifecycle Errors

| Variable | Value |
|----------|-------|
| `ErrAlreadyStarted` | "component already started" |
| `ErrNotStarted` | "component not started" |
| `ErrAlreadyStopped` | "component already stopped" |
| `ErrShuttingDown` | "component is shutting down" |

### Connection and Networking Errors

| Variable | Value |
|----------|-------|
| `ErrNoConnection` | "no connection available" |
| `ErrConnectionLost` | "connection lost" |
| `ErrConnectionTimeout` | "connection timeout" |
| `ErrSubscriptionFailed` | "subscription failed" |

### Data Processing Errors

| Variable | Value |
|----------|-------|
| `ErrInvalidData` | "invalid data format" |
| `ErrDataCorrupted` | "data corrupted" |
| `ErrChecksumFailed` | "checksum validation failed" |
| `ErrParsingFailed` | "parsing failed" |

### Storage and Persistence Errors

| Variable | Value |
|----------|-------|
| `ErrStorageFull` | "storage full" |
| `ErrStorageUnavailable` | "storage unavailable" |
| `ErrBucketNotFound` | "bucket not found" |
| `ErrKeyNotFound` | "key not found" |

### Configuration Errors

| Variable | Value |
|----------|-------|
| `ErrInvalidConfig` | "invalid configuration" |
| `ErrMissingConfig` | "missing required configuration" |
| `ErrConfigNotFound` | "configuration not found" |

### Resource Errors

| Variable | Value |
|----------|-------|
| `ErrResourceExhausted` | "resource exhausted" |
| `ErrRateLimited` | "rate limited" |
| `ErrQuotaExceeded` | "quota exceeded" |

### Circuit Breaker and Retry Errors

| Variable | Value |
|----------|-------|
| `ErrCircuitOpen` | "circuit breaker open" |
| `ErrMaxRetriesExceeded` | "maximum retries exceeded" |
| `ErrRetryTimeout` | "retry timeout exceeded" |

## Functions

### Classification Functions

| Function | Signature | Description |
|----------|-----------|-------------|
| `IsTransient` | `func IsTransient(err error) bool` | Checks if error is transient (retryable) |
| `IsFatal` | `func IsFatal(err error) bool` | Checks if error is fatal (stop processing) |
| `IsInvalid` | `func IsInvalid(err error) bool` | Checks if error is due to invalid input |
| `Classify` | `func Classify(err error) ErrorClass` | Returns error classification |

### Wrapping Functions

| Function | Signature | Description |
|----------|-----------|-------------|
| `Wrap` | `func Wrap(err error, component, method, action string) error` | Wraps error with context |
| `WrapTransient` | `func WrapTransient(err error, component, method, action string) error` | Wraps as transient |
| `WrapFatal` | `func WrapFatal(err error, component, method, action string) error` | Wraps as fatal |
| `WrapInvalid` | `func WrapInvalid(err error, component, method, action string) error` | Wraps as invalid |

### Configuration Functions

| Function | Signature | Description |
|----------|-----------|-------------|
| `DefaultRetryConfig` | `func DefaultRetryConfig() RetryConfig` | Returns default retry configuration |

## Internal Functions (Not Exported)

| Function | Notes |
|----------|-------|
| `newClassified` | Internal helper - use `WrapTransient`, `WrapFatal`, `WrapInvalid` instead |

## Dependencies

### Internal Dependencies

- `github.com/c360/semstreams/pkg/retry` - For `retry.Config` type in `ToRetryConfig()` method

### Standard Library Dependencies

- `context` - For `context.DeadlineExceeded` and `context.Canceled` classification
- `errors` - For stdlib `errors.New`, `errors.Is`, `errors.As`
- `fmt` - For error formatting
- `strings` - For error message pattern matching
- `time` - For `time.Duration` in retry configuration

## Migration Impact

### Import Path Change

```go
// Before
import "github.com/c360/semstreams/errors"

// After
import "github.com/c360/semstreams/pkg/errs"
```

### Usage Pattern Change

```go
// Before
errors.Wrap(err, "Component", "Method", "action")
errors.IsTransient(err)
errors.ErrConnectionTimeout

// After
errs.Wrap(err, "Component", "Method", "action")
errs.IsTransient(err)
errs.ErrConnectionTimeout
```

## Validation Criteria

After migration, all of the following must hold true:

1. All types compile with identical field signatures
2. All constants have identical values
3. All sentinel errors have identical string values
4. All functions have identical signatures
5. All methods have identical receivers and signatures
6. `go test -race ./pkg/errs/...` passes
7. `go build ./...` succeeds with updated imports
