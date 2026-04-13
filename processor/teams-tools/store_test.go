package teamtools

import (
	"context"
	"testing"
	"time"

	"github.com/c360studio/semteams/teams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInMemoryToolCallStore_Store(t *testing.T) {
	store := NewInMemoryToolCallStore()

	record := ToolCallRecord{
		Call: teams.ToolCall{
			ID:      "call-1",
			Name:    "test_tool",
			LoopID:  "loop-123",
			TraceID: "trace-abc",
		},
		Result: teams.ToolResult{
			CallID:  "call-1",
			Content: "result",
		},
		StartTime: time.Now(),
		EndTime:   time.Now().Add(100 * time.Millisecond),
		Duration:  100 * time.Millisecond,
	}

	err := store.Store(context.Background(), record)
	require.NoError(t, err)

	records := store.Records()
	require.Len(t, records, 1)
	assert.Equal(t, "call-1", records[0].Call.ID)
	assert.Equal(t, "loop-123", records[0].Call.LoopID)
	assert.Equal(t, "trace-abc", records[0].Call.TraceID)
}

func TestInMemoryToolCallStore_Close(t *testing.T) {
	store := NewInMemoryToolCallStore()

	err := store.Close()
	require.NoError(t, err)

	// Store should fail after close
	err = store.Store(context.Background(), ToolCallRecord{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "closed")
}

func TestInMemoryToolCallStore_Reset(t *testing.T) {
	store := NewInMemoryToolCallStore()

	// Add a record
	err := store.Store(context.Background(), ToolCallRecord{
		Call: teams.ToolCall{ID: "call-1"},
	})
	require.NoError(t, err)
	require.Len(t, store.Records(), 1)

	// Close and reset
	err = store.Close()
	require.NoError(t, err)

	store.Reset()

	// Should be able to store again
	err = store.Store(context.Background(), ToolCallRecord{
		Call: teams.ToolCall{ID: "call-2"},
	})
	require.NoError(t, err)

	records := store.Records()
	require.Len(t, records, 1)
	assert.Equal(t, "call-2", records[0].Call.ID)
}

func TestKVToolCallStore_Reset(t *testing.T) {
	// Test Reset state transitions without actual NATS connection
	store := NewKVToolCallStore(nil, "test-bucket")

	// Close the store
	err := store.Close()
	require.NoError(t, err)

	// Verify closed flag is set
	store.mu.RLock()
	isClosed := store.closed
	store.mu.RUnlock()
	assert.True(t, isClosed, "store should be closed")

	// Reset should clear closed flag
	store.Reset()

	store.mu.RLock()
	isClosed = store.closed
	kvIsNil := store.kv == nil
	store.mu.RUnlock()
	assert.False(t, isClosed, "store should not be closed after reset")
	assert.True(t, kvIsNil, "kv should be nil after reset")
}
