package component

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogEntry_JSONMarshaling(t *testing.T) {
	entry := LogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Level:     LogLevelInfo,
		Component: "test-component",
		FlowID:    "test-flow",
		Message:   "test message",
		Stack:     "optional stack trace",
	}

	data, err := json.Marshal(entry)
	require.NoError(t, err)

	var decoded LogEntry
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, entry.Timestamp, decoded.Timestamp)
	assert.Equal(t, entry.Level, decoded.Level)
	assert.Equal(t, entry.Component, decoded.Component)
	assert.Equal(t, entry.FlowID, decoded.FlowID)
	assert.Equal(t, entry.Message, decoded.Message)
	assert.Equal(t, entry.Stack, decoded.Stack)
}

func TestLogEntry_JSONMarshaling_NoStack(t *testing.T) {
	entry := LogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Level:     LogLevelInfo,
		Component: "test-component",
		FlowID:    "test-flow",
		Message:   "test message",
		// Stack omitted
	}

	data, err := json.Marshal(entry)
	require.NoError(t, err)

	// Verify stack is omitted in JSON
	var raw map[string]interface{}
	err = json.Unmarshal(data, &raw)
	require.NoError(t, err)

	_, hasStack := raw["stack"]
	assert.False(t, hasStack, "Empty stack should be omitted from JSON")
}

func TestLogLevel_Constants(t *testing.T) {
	// Verify log level constants are defined correctly
	assert.Equal(t, LogLevel("DEBUG"), LogLevelDebug)
	assert.Equal(t, LogLevel("INFO"), LogLevelInfo)
	assert.Equal(t, LogLevel("WARN"), LogLevelWarn)
	assert.Equal(t, LogLevel("ERROR"), LogLevelError)
}
