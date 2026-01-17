package logging

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockNATSPublisher implements NATSPublisher for testing
type mockNATSPublisher struct {
	mu          sync.Mutex
	publishFunc func(ctx context.Context, subject string, data []byte) error
	calls       []publishCall
}

type publishCall struct {
	Subject string
	Data    []byte
}

func (m *mockNATSPublisher) Publish(ctx context.Context, subject string, data []byte) error {
	m.mu.Lock()
	m.calls = append(m.calls, publishCall{Subject: subject, Data: data})
	m.mu.Unlock()

	if m.publishFunc != nil {
		return m.publishFunc(ctx, subject, data)
	}
	return nil
}

func (m *mockNATSPublisher) getCalls() []publishCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]publishCall{}, m.calls...)
}

func TestNATSLogHandler_Enabled(t *testing.T) {
	tests := []struct {
		name      string
		minLevel  slog.Level
		testLevel slog.Level
		want      bool
	}{
		// MinLevel = DEBUG (should allow all)
		{"DEBUG min allows DEBUG", slog.LevelDebug, slog.LevelDebug, true},
		{"DEBUG min allows INFO", slog.LevelDebug, slog.LevelInfo, true},
		{"DEBUG min allows WARN", slog.LevelDebug, slog.LevelWarn, true},
		{"DEBUG min allows ERROR", slog.LevelDebug, slog.LevelError, true},

		// MinLevel = INFO (should filter DEBUG)
		{"INFO min filters DEBUG", slog.LevelInfo, slog.LevelDebug, false},
		{"INFO min allows INFO", slog.LevelInfo, slog.LevelInfo, true},
		{"INFO min allows WARN", slog.LevelInfo, slog.LevelWarn, true},
		{"INFO min allows ERROR", slog.LevelInfo, slog.LevelError, true},

		// MinLevel = WARN (should filter DEBUG and INFO)
		{"WARN min filters DEBUG", slog.LevelWarn, slog.LevelDebug, false},
		{"WARN min filters INFO", slog.LevelWarn, slog.LevelInfo, false},
		{"WARN min allows WARN", slog.LevelWarn, slog.LevelWarn, true},
		{"WARN min allows ERROR", slog.LevelWarn, slog.LevelError, true},

		// MinLevel = ERROR (should filter all except ERROR)
		{"ERROR min filters DEBUG", slog.LevelError, slog.LevelDebug, false},
		{"ERROR min filters INFO", slog.LevelError, slog.LevelInfo, false},
		{"ERROR min filters WARN", slog.LevelError, slog.LevelWarn, false},
		{"ERROR min allows ERROR", slog.LevelError, slog.LevelError, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewNATSLogHandler(&mockNATSPublisher{}, NATSLogHandlerConfig{
				MinLevel: tt.minLevel,
			})

			got := handler.Enabled(context.Background(), tt.testLevel)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestMultiHandler_WithLogger_ExcludedSource tests the full production scenario:
// slog.Logger -> MultiHandler -> NATSLogHandler with excluded source
// This simulates: wsLogger := fs.logger.With("source", "flow-service.websocket")
func TestMultiHandler_WithLogger_ExcludedSource(t *testing.T) {
	mock := &mockNATSPublisher{}

	// Create NATSLogHandler with exclude_sources
	natsHandler := NewNATSLogHandler(mock, NATSLogHandlerConfig{
		MinLevel:       slog.LevelDebug,
		ExcludeSources: []string{"flow-service.websocket"},
	})

	// Create stdout handler (discard output for test)
	stdoutHandler := slog.NewTextHandler(&discardWriter{}, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	// Create MultiHandler (same as production)
	multiHandler := NewMultiHandler(stdoutHandler, natsHandler)

	// Create logger with base attrs (simulating setupLoggerWithNATS)
	baseLogger := slog.New(multiHandler).With("service", "semstreams")

	// Create WebSocket worker logger (simulating wsLogger := fs.logger.With("source", "..."))
	wsLogger := baseLogger.With("source", "flow-service.websocket")

	// Log a message (simulating logger.Debug("Failed to send log envelope"))
	wsLogger.Debug("Failed to send log envelope", "error", "connection closed")

	// Wait for async publish
	time.Sleep(50 * time.Millisecond)

	// Should NOT have published - source is excluded
	calls := mock.getCalls()
	assert.Empty(t, calls, "Logs from excluded source via slog.Logger should not be published to NATS")

	// Now test that non-excluded sources DO publish
	otherLogger := baseLogger.With("source", "graph-processor")
	otherLogger.Info("Processing complete")

	time.Sleep(50 * time.Millisecond)

	calls = mock.getCalls()
	assert.Len(t, calls, 1, "Logs from non-excluded source should be published")
	assert.Equal(t, "logs.graph-processor.INFO", calls[0].Subject)
}

// discardWriter is an io.Writer that discards all writes
type discardWriter struct{}

func (d *discardWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func TestNATSLogHandler_Handle_PublishesToNATS(t *testing.T) {
	mock := &mockNATSPublisher{}
	handler := NewNATSLogHandler(mock, NATSLogHandlerConfig{
		MinLevel: slog.LevelInfo,
	})

	// Create and handle a log record
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test message", 0)
	err := handler.Handle(context.Background(), record)
	require.NoError(t, err)

	// Wait for async publish
	time.Sleep(50 * time.Millisecond)

	// Verify publish was called
	calls := mock.getCalls()
	require.Len(t, calls, 1)

	// Verify subject format: logs.{source}.{level}
	assert.Equal(t, "logs.system.INFO", calls[0].Subject)

	// Verify payload structure
	var entry map[string]any
	err = json.Unmarshal(calls[0].Data, &entry)
	require.NoError(t, err)

	assert.Contains(t, entry, "timestamp")
	assert.Equal(t, "INFO", entry["level"])
	assert.Equal(t, "system", entry["source"])
	assert.Equal(t, "test message", entry["message"])
}

func TestNATSLogHandler_Handle_BelowMinLevel_DoesNotPublish(t *testing.T) {
	mock := &mockNATSPublisher{}
	handler := NewNATSLogHandler(mock, NATSLogHandlerConfig{
		MinLevel: slog.LevelInfo,
	})

	// Create DEBUG record (below INFO threshold)
	record := slog.NewRecord(time.Now(), slog.LevelDebug, "debug message", 0)
	err := handler.Handle(context.Background(), record)
	require.NoError(t, err)

	// Wait for any async operations
	time.Sleep(50 * time.Millisecond)

	// Verify no publish
	calls := mock.getCalls()
	assert.Empty(t, calls)
}

func TestNATSLogHandler_Handle_ExtractsSource(t *testing.T) {
	tests := []struct {
		name       string
		attrs      []slog.Attr
		wantSource string
	}{
		{
			name:       "extracts source from source attribute",
			attrs:      []slog.Attr{slog.String("source", "flow-service.websocket")},
			wantSource: "flow-service.websocket",
		},
		{
			name:       "extracts source from component attribute",
			attrs:      []slog.Attr{slog.String("component", "processor")},
			wantSource: "processor",
		},
		{
			name:       "extracts source from service attribute",
			attrs:      []slog.Attr{slog.String("service", "gateway")},
			wantSource: "gateway",
		},
		{
			name: "source takes priority over component",
			attrs: []slog.Attr{
				slog.String("source", "explicit-source"),
				slog.String("component", "some-component"),
			},
			wantSource: "explicit-source",
		},
		{
			name: "component takes priority over service",
			attrs: []slog.Attr{
				slog.String("component", "some-component"),
				slog.String("service", "some-service"),
			},
			wantSource: "some-component",
		},
		{
			name:       "defaults to system when no source attrs",
			attrs:      []slog.Attr{slog.String("other", "value")},
			wantSource: "system",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockNATSPublisher{}
			handler := NewNATSLogHandler(mock, NATSLogHandlerConfig{
				MinLevel: slog.LevelInfo,
			})

			// Create record with attributes
			record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
			record.AddAttrs(tt.attrs...)

			err := handler.Handle(context.Background(), record)
			require.NoError(t, err)

			// Wait for async publish
			time.Sleep(50 * time.Millisecond)

			calls := mock.getCalls()
			require.Len(t, calls, 1)

			// Verify source in subject
			expectedSubject := "logs." + tt.wantSource + ".INFO"
			assert.Equal(t, expectedSubject, calls[0].Subject)

			// Verify source in payload
			var entry map[string]any
			err = json.Unmarshal(calls[0].Data, &entry)
			require.NoError(t, err)
			assert.Equal(t, tt.wantSource, entry["source"])
		})
	}
}

func TestNATSLogHandler_Handle_ExcludedSources(t *testing.T) {
	tests := []struct {
		name           string
		excludeSources []string
		source         string
		wantPublish    bool
	}{
		{
			name:           "empty exclude list allows all",
			excludeSources: []string{},
			source:         "flow-service.websocket",
			wantPublish:    true,
		},
		{
			name:           "exact match excludes",
			excludeSources: []string{"flow-service.websocket"},
			source:         "flow-service.websocket",
			wantPublish:    false,
		},
		{
			name:           "prefix match excludes child",
			excludeSources: []string{"flow-service.websocket"},
			source:         "flow-service.websocket.health",
			wantPublish:    false,
		},
		{
			name:           "prefix does not match parent",
			excludeSources: []string{"flow-service.websocket"},
			source:         "flow-service",
			wantPublish:    true,
		},
		{
			name:           "prefix does not match sibling",
			excludeSources: []string{"flow-service.websocket"},
			source:         "flow-service.api",
			wantPublish:    true,
		},
		{
			name:           "partial string match not allowed",
			excludeSources: []string{"flow"},
			source:         "flow-service",
			wantPublish:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockNATSPublisher{}
			handler := NewNATSLogHandler(mock, NATSLogHandlerConfig{
				MinLevel:       slog.LevelInfo,
				ExcludeSources: tt.excludeSources,
			})

			// Create record with source attribute
			record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
			record.AddAttrs(slog.String("source", tt.source))

			err := handler.Handle(context.Background(), record)
			require.NoError(t, err)

			// Wait for async publish
			time.Sleep(50 * time.Millisecond)

			calls := mock.getCalls()
			if tt.wantPublish {
				assert.Len(t, calls, 1)
			} else {
				assert.Empty(t, calls)
			}
		})
	}
}

func TestNATSLogHandler_WithAttrs(t *testing.T) {
	mock := &mockNATSPublisher{}
	handler := NewNATSLogHandler(mock, NATSLogHandlerConfig{
		MinLevel: slog.LevelInfo,
	})

	// Add component attribute
	handlerWithAttrs := handler.WithAttrs([]slog.Attr{
		slog.String("component", "test-component"),
	})

	// Create record
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
	err := handlerWithAttrs.Handle(context.Background(), record)
	require.NoError(t, err)

	// Wait for async publish
	time.Sleep(50 * time.Millisecond)

	calls := mock.getCalls()
	require.Len(t, calls, 1)

	// Source should be extracted from accumulated attrs
	assert.Equal(t, "logs.test-component.INFO", calls[0].Subject)
}

// TestNATSLogHandler_WithAttrs_ExcludedSource verifies that source attributes
// added via WithAttrs are correctly excluded based on exclude_sources config.
// This simulates the WebSocket worker scenario: logger.With("source", "flow-service.websocket")
func TestNATSLogHandler_WithAttrs_ExcludedSource(t *testing.T) {
	mock := &mockNATSPublisher{}
	handler := NewNATSLogHandler(mock, NATSLogHandlerConfig{
		MinLevel:       slog.LevelInfo,
		ExcludeSources: []string{"flow-service.websocket"},
	})

	// Simulate wsLogger := fs.logger.With("source", "flow-service.websocket")
	handlerWithSource := handler.WithAttrs([]slog.Attr{
		slog.String("source", "flow-service.websocket"),
	})

	// Create record (simulating logger.Debug("Failed to send log envelope"))
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "Failed to send log envelope", 0)
	record.AddAttrs(slog.String("error", "connection closed"))

	err := handlerWithSource.Handle(context.Background(), record)
	require.NoError(t, err)

	// Wait for async publish (if any)
	time.Sleep(50 * time.Millisecond)

	// Should NOT have published - source is excluded
	calls := mock.getCalls()
	assert.Empty(t, calls, "Logs from excluded source should not be published to NATS")
}

func TestNATSLogHandler_WithGroup(t *testing.T) {
	mock := &mockNATSPublisher{}
	handler := NewNATSLogHandler(mock, NATSLogHandlerConfig{
		MinLevel: slog.LevelInfo,
	})

	// Add group
	handlerWithGroup := handler.WithGroup("test-group")
	require.NotNil(t, handlerWithGroup)

	// Should still be a NATSLogHandler
	_, ok := handlerWithGroup.(*NATSLogHandler)
	assert.True(t, ok)
}

func TestNATSLogHandler_ConcurrentPublish(t *testing.T) {
	mock := &mockNATSPublisher{}
	handler := NewNATSLogHandler(mock, NATSLogHandlerConfig{
		MinLevel: slog.LevelInfo,
	})

	// Concurrent log publishing
	numGoroutines := 50
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			record := slog.NewRecord(time.Now(), slog.LevelInfo, "concurrent message", 0)
			err := handler.Handle(context.Background(), record)
			assert.NoError(t, err)
		}()
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond)

	// Verify all publishes occurred
	calls := mock.getCalls()
	assert.Equal(t, numGoroutines, len(calls))
}

func TestNATSLogHandler_isExcluded(t *testing.T) {
	handler := &NATSLogHandler{
		excludeSources: []string{"flow-service.websocket", "metrics-forwarder"},
	}

	tests := []struct {
		source   string
		excluded bool
	}{
		{"flow-service.websocket", true},
		{"flow-service.websocket.health", true},
		{"metrics-forwarder", true},
		{"metrics-forwarder.internal", true},
		{"flow-service", false},
		{"flow-service.api", false},
		{"graph-processor", false},
	}

	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			got := handler.isExcluded(tt.source)
			assert.Equal(t, tt.excluded, got)
		})
	}
}
