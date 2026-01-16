package component

// LogLevel represents the severity level of a log entry
type LogLevel string

const (
	// LogLevelDebug represents debug-level logs
	LogLevelDebug LogLevel = "DEBUG"
	// LogLevelInfo represents informational logs
	LogLevelInfo LogLevel = "INFO"
	// LogLevelWarn represents warning logs
	LogLevelWarn LogLevel = "WARN"
	// LogLevelError represents error logs
	LogLevelError LogLevel = "ERROR"
)

// LogEntry represents a structured log entry that can be published to NATS
// and consumed by the Flow Builder SSE endpoint.
type LogEntry struct {
	Timestamp string   `json:"timestamp"` // RFC3339 format
	Level     LogLevel `json:"level"`
	Component string   `json:"component"`
	FlowID    string   `json:"flow_id"`
	Message   string   `json:"message"`
	Stack     string   `json:"stack,omitempty"` // Stack trace for errors
}
