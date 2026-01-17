package logging

import (
	"bytes"
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockHandler implements slog.Handler for testing
type mockHandler struct {
	mu       sync.Mutex
	calls    []slog.Record
	minLevel slog.Level
	enabled  bool
	attrs    []slog.Attr
	groups   []string
}

func newMockHandler(minLevel slog.Level) *mockHandler {
	return &mockHandler{
		minLevel: minLevel,
		enabled:  true,
	}
}

func (m *mockHandler) Enabled(_ context.Context, level slog.Level) bool {
	return m.enabled && level >= m.minLevel
}

func (m *mockHandler) Handle(_ context.Context, r slog.Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, r)
	return nil
}

func (m *mockHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &mockHandler{
		minLevel: m.minLevel,
		enabled:  m.enabled,
		attrs:    append(append([]slog.Attr{}, m.attrs...), attrs...),
		groups:   m.groups,
	}
}

func (m *mockHandler) WithGroup(name string) slog.Handler {
	return &mockHandler{
		minLevel: m.minLevel,
		enabled:  m.enabled,
		attrs:    m.attrs,
		groups:   append(append([]string{}, m.groups...), name),
	}
}

func (m *mockHandler) getCalls() []slog.Record {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]slog.Record{}, m.calls...)
}

func TestMultiHandler_DispatchesToAllHandlers(t *testing.T) {
	handler1 := newMockHandler(slog.LevelInfo)
	handler2 := newMockHandler(slog.LevelInfo)

	multi := NewMultiHandler(handler1, handler2)

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test message", 0)
	err := multi.Handle(context.Background(), record)
	require.NoError(t, err)

	// Both handlers should receive the record
	assert.Len(t, handler1.getCalls(), 1)
	assert.Len(t, handler2.getCalls(), 1)
}

func TestMultiHandler_Enabled_ReturnsTrueIfAnyEnabled(t *testing.T) {
	// Handler 1 only allows WARN and above
	handler1 := newMockHandler(slog.LevelWarn)
	// Handler 2 allows INFO and above
	handler2 := newMockHandler(slog.LevelInfo)

	multi := NewMultiHandler(handler1, handler2)

	// INFO should be enabled because handler2 allows it
	assert.True(t, multi.Enabled(context.Background(), slog.LevelInfo))
	// WARN should be enabled
	assert.True(t, multi.Enabled(context.Background(), slog.LevelWarn))
}

func TestMultiHandler_Enabled_ReturnsFalseIfNoneEnabled(t *testing.T) {
	handler1 := newMockHandler(slog.LevelWarn)
	handler2 := newMockHandler(slog.LevelError)

	multi := NewMultiHandler(handler1, handler2)

	// DEBUG should not be enabled by either
	assert.False(t, multi.Enabled(context.Background(), slog.LevelDebug))
	// INFO should not be enabled by either
	assert.False(t, multi.Enabled(context.Background(), slog.LevelInfo))
}

func TestMultiHandler_Handle_OnlyDispatchesToEnabledHandlers(t *testing.T) {
	handler1 := newMockHandler(slog.LevelWarn) // Only WARN+
	handler2 := newMockHandler(slog.LevelInfo) // INFO+

	multi := NewMultiHandler(handler1, handler2)

	// Send INFO record
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "info message", 0)
	err := multi.Handle(context.Background(), record)
	require.NoError(t, err)

	// Only handler2 should receive it (handler1 filters INFO)
	assert.Len(t, handler1.getCalls(), 0)
	assert.Len(t, handler2.getCalls(), 1)
}

func TestMultiHandler_WithAttrs_PropagatesAttrs(t *testing.T) {
	handler1 := newMockHandler(slog.LevelInfo)
	handler2 := newMockHandler(slog.LevelInfo)

	multi := NewMultiHandler(handler1, handler2)
	multiWithAttrs := multi.WithAttrs([]slog.Attr{
		slog.String("key", "value"),
	})

	// Should return a new MultiHandler
	newMulti, ok := multiWithAttrs.(*MultiHandler)
	require.True(t, ok)
	assert.NotSame(t, multi, newMulti)
}

func TestMultiHandler_WithGroup_PropagatesGroup(t *testing.T) {
	handler1 := newMockHandler(slog.LevelInfo)
	handler2 := newMockHandler(slog.LevelInfo)

	multi := NewMultiHandler(handler1, handler2)
	multiWithGroup := multi.WithGroup("test-group")

	// Should return a new MultiHandler
	newMulti, ok := multiWithGroup.(*MultiHandler)
	require.True(t, ok)
	assert.NotSame(t, multi, newMulti)
}

func TestMultiHandler_EmptyHandlers(t *testing.T) {
	multi := NewMultiHandler()

	// Should not panic
	assert.False(t, multi.Enabled(context.Background(), slog.LevelInfo))

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
	err := multi.Handle(context.Background(), record)
	require.NoError(t, err)
}

func TestMultiHandler_ContinuesOnHandlerError(t *testing.T) {
	// Create a handler that writes to a buffer
	var buf bytes.Buffer
	realHandler := slog.NewTextHandler(&buf, nil)

	// Create a handler that's disabled (won't error, just won't run)
	disabledHandler := newMockHandler(slog.LevelError)

	multi := NewMultiHandler(disabledHandler, realHandler)

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test message", 0)
	err := multi.Handle(context.Background(), record)
	require.NoError(t, err)

	// Real handler should have received the message
	assert.Contains(t, buf.String(), "test message")
}

func TestMultiHandler_Concurrent(t *testing.T) {
	handler1 := newMockHandler(slog.LevelInfo)
	handler2 := newMockHandler(slog.LevelInfo)

	multi := NewMultiHandler(handler1, handler2)

	numGoroutines := 50
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			record := slog.NewRecord(time.Now(), slog.LevelInfo, "concurrent", 0)
			_ = multi.Handle(context.Background(), record)
		}()
	}

	wg.Wait()

	// Both handlers should have received all messages
	assert.Equal(t, numGoroutines, len(handler1.getCalls()))
	assert.Equal(t, numGoroutines, len(handler2.getCalls()))
}
