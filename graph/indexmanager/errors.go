package indexmanager

import (
	stderrors "errors"
	"fmt"

	gtypes "github.com/c360/semstreams/graph"
	"github.com/c360/semstreams/pkg/errs"
)

// Local error types specific to index manager
// (Using shared sentinel errors from types/graph package for common cases)
var (
	// Index-specific errors (ones not in shared types/graph package)
	ErrIndexDisabled = fmt.Errorf("index type is disabled")

	// KV Watch errors
	ErrWatchFailed   = fmt.Errorf("KV watch operation failed")
	ErrWatchStopped  = fmt.Errorf("KV watch was stopped")
	ErrBucketMissing = fmt.Errorf("KV bucket not found")

	// Processing errors
	ErrProcessingTimeout = fmt.Errorf("processing timeout exceeded")
	ErrEventBufferFull   = fmt.Errorf("event buffer is full")
	ErrInvalidEvent      = fmt.Errorf("invalid event data")

	// Bounds errors
	ErrInvalidBounds    = fmt.Errorf("invalid spatial bounds")
	ErrInvalidTimeRange = fmt.Errorf("invalid time range")
)

// All wrapper functions removed - use direct errs.Wrap* calls instead
// This ensures consistency across all managers and reduces code bloat

// IsRetryable checks if an error is retryable
func IsRetryable(err error) bool {
	return errs.IsTransient(err) ||
		IsWatchError(err) ||
		IsProcessingError(err)
}

// IsWatchError checks if an error is related to KV watching
func IsWatchError(err error) bool {
	return stderrors.Is(err, ErrWatchFailed) ||
		stderrors.Is(err, ErrWatchStopped) ||
		stderrors.Is(err, ErrBucketMissing)
}

// IsProcessingError checks if an error is related to event processing
func IsProcessingError(err error) bool {
	return stderrors.Is(err, ErrProcessingTimeout) ||
		stderrors.Is(err, ErrEventBufferFull) ||
		stderrors.Is(err, ErrInvalidEvent)
}

// IsQueryError checks if an error is related to queries
func IsQueryError(err error) bool {
	return stderrors.Is(err, gtypes.ErrQueryTimeout) ||
		stderrors.Is(err, gtypes.ErrInvalidQueryParams) ||
		stderrors.Is(err, gtypes.ErrIndexNotFound) ||
		stderrors.Is(err, gtypes.ErrAliasNotFound) ||
		stderrors.Is(err, ErrInvalidBounds) ||
		stderrors.Is(err, ErrInvalidTimeRange)
}

// IsConfigError checks if an error is related to configuration
func IsConfigError(err error) bool {
	return stderrors.Is(err, ErrIndexDisabled) ||
		errs.IsInvalid(err)
}

// IsLifecycleError checks if an error is related to service lifecycle
func IsLifecycleError(err error) bool {
	return stderrors.Is(err, gtypes.ErrNotStarted) ||
		stderrors.Is(err, gtypes.ErrAlreadyStarted) ||
		stderrors.Is(err, gtypes.ErrShuttingDown)
}

// GetErrorSeverity returns the severity level of an error
func GetErrorSeverity(err error) string {
	if err == nil {
		return "none"
	}

	// Fatal errors that require service restart
	if errs.IsFatal(err) {
		return "fatal"
	}

	// Critical errors that affect core functionality
	if IsWatchError(err) || stderrors.Is(err, ErrEventBufferFull) {
		return "critical"
	}

	// Warning-level errors that are recoverable
	if IsQueryError(err) || IsProcessingError(err) {
		return "warning"
	}

	// Info-level errors for normal operations
	if IsLifecycleError(err) {
		return "info"
	}

	// Default to warning for unknown errors
	return "warning"
}
