package graphql

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/99designs/gqlgen/graphql"
	"github.com/c360/semstreams/pkg/errs"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

// mapContextError converts context errors to GraphQL errors.
func mapContextError(err error, operation string) error {
	if err == nil {
		return nil
	}

	switch {
	case err == context.DeadlineExceeded:
		return &gqlerror.Error{
			Message: "Query timeout exceeded",
			Extensions: map[string]interface{}{
				"code":      "DEADLINE_EXCEEDED",
				"operation": operation,
			},
		}

	case err == context.Canceled:
		return &gqlerror.Error{
			Message: "Query cancelled",
			Extensions: map[string]interface{}{
				"code":      "CANCELLED",
				"operation": operation,
			},
		}
	}

	return nil
}

// mapJSONError converts JSON unmarshaling errors to GraphQL errors.
func mapJSONError(err error, operation string) error {
	if err == nil {
		return nil
	}

	switch e := err.(type) {
	case *json.SyntaxError:
		return &gqlerror.Error{
			Message: "Invalid response format",
			Extensions: map[string]interface{}{
				"code":      "INVALID_RESPONSE",
				"operation": operation,
				"offset":    e.Offset,
			},
		}

	case *json.UnmarshalTypeError:
		return &gqlerror.Error{
			Message: fmt.Sprintf("Invalid response type: expected %s, got %s", e.Type, e.Value),
			Extensions: map[string]interface{}{
				"code":      "INVALID_RESPONSE_TYPE",
				"operation": operation,
				"field":     e.Field,
			},
		}
	}

	return nil
}

// wrapError wraps an error with GraphQL error format, preserving context.
func wrapError(err error, operation string) error {
	if err == nil {
		return nil
	}

	// If already a GraphQL error, return as-is
	if _, ok := err.(*gqlerror.Error); ok {
		return err
	}

	// Try context error mapping
	if gqlErr := mapContextError(err, operation); gqlErr != nil {
		return gqlErr
	}

	// Try JSON error mapping
	if gqlErr := mapJSONError(err, operation); gqlErr != nil {
		return gqlErr
	}

	// Map SemStreams error classifications
	if errs.IsTransient(err) {
		return &gqlerror.Error{
			Message: fmt.Sprintf("Temporary error: %s", err.Error()),
			Extensions: map[string]interface{}{
				"code":      "TRANSIENT_ERROR",
				"operation": operation,
				"retryable": true,
			},
		}
	}

	if errs.IsInvalid(err) {
		return &gqlerror.Error{
			Message: fmt.Sprintf("Invalid input: %s", err.Error()),
			Extensions: map[string]interface{}{
				"code":      "INVALID_INPUT",
				"operation": operation,
			},
		}
	}

	if errs.IsFatal(err) {
		return &gqlerror.Error{
			Message: "Internal server error",
			Extensions: map[string]interface{}{
				"code":      "INTERNAL_ERROR",
				"operation": operation,
			},
		}
	}

	// Generic error
	return &gqlerror.Error{
		Message: fmt.Sprintf("Query failed: %s", err.Error()),
		Extensions: map[string]interface{}{
			"code":      "QUERY_ERROR",
			"operation": operation,
		},
	}
}

// addErrorToContext adds an error to the GraphQL context for error reporting.
func addErrorToContext(ctx context.Context, err error) {
	if err == nil {
		return
	}

	// Get GraphQL error list from context
	errList := graphql.GetErrors(ctx)
	if errList == nil {
		errList = gqlerror.List{}
	}

	// Add error to list
	if gqlErr, ok := err.(*gqlerror.Error); ok {
		errList = append(errList, gqlErr)
	} else {
		errList = append(errList, &gqlerror.Error{
			Message: err.Error(),
		})
	}

	// Store back in context
	graphql.AddError(ctx, errList[len(errList)-1])
}
