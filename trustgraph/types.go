package trustgraph

// APIError represents an error from the TrustGraph API.
type APIError struct {
	StatusCode int
	Message    string
	RetryAfter int // Seconds to wait before retrying (for 429 responses)
}

// Error implements the error interface.
func (e *APIError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "TrustGraph API error"
}

// IsRetryable returns true if the error suggests the request should be retried.
func (e *APIError) IsRetryable() bool {
	return e.StatusCode >= 500 || e.StatusCode == 429
}
