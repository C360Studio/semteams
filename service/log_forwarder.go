// Package service provides the LogForwarder service for forwarding logs to NATS
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/c360/semstreams/natsclient"
)

// natsPublisher defines the minimal interface needed for publishing to NATS.
// This allows for easier testing with mocks.
type natsPublisher interface {
	Publish(ctx context.Context, subject string, data []byte) error
}

// NewLogForwarderService creates a new log forwarder service using the standard constructor pattern
func NewLogForwarderService(rawConfig json.RawMessage, deps *Dependencies) (Service, error) {
	// Parse config - handle empty or invalid JSON properly
	var cfg LogForwarderConfig
	if len(rawConfig) > 0 {
		if err := json.Unmarshal(rawConfig, &cfg); err != nil {
			return nil, fmt.Errorf("parse log-forwarder config: %w", err)
		}
	}

	// Apply defaults
	if cfg.MinLevel == "" {
		cfg.MinLevel = "INFO"
	}

	// Normalize level to uppercase
	cfg.MinLevel = strings.ToUpper(cfg.MinLevel)

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate log-forwarder config: %w", err)
	}

	// Check if NATS client is available
	if deps.NATSClient == nil {
		return nil, fmt.Errorf("log-forwarder requires NATS client")
	}

	// Create the LogForwarder with dependencies
	var opts []Option
	if deps.Logger != nil {
		opts = append(opts, WithLogger(deps.Logger))
	}
	if deps.MetricsRegistry != nil {
		opts = append(opts, WithMetrics(deps.MetricsRegistry))
	}

	// The NATS client should implement the publisher interface
	// This works for both *natsclient.Client and test mocks
	publisher, ok := interface{}(deps.NATSClient).(natsPublisher)
	if !ok {
		return nil, fmt.Errorf("NATS client does not implement Publish method")
	}

	return newLogForwarderWithPublisher(&cfg, publisher, opts...)
}

// LogForwarderConfig holds configuration for the LogForwarder service
type LogForwarderConfig struct {
	// Enable or disable log forwarding
	Enabled bool `json:"enabled"`

	// Minimum log level to forward (DEBUG, INFO, WARN, ERROR)
	MinLevel string `json:"min_level"`
}

// Validate checks if the configuration is valid
func (c LogForwarderConfig) Validate() error {
	// Validate log level
	validLevels := map[string]bool{
		"DEBUG": true,
		"INFO":  true,
		"WARN":  true,
		"ERROR": true,
	}

	if !validLevels[c.MinLevel] {
		return fmt.Errorf("invalid log level: %s (must be DEBUG, INFO, WARN, or ERROR)", c.MinLevel)
	}

	return nil
}

// LogForwarder implements slog.Handler and forwards logs to NATS
type LogForwarder struct {
	*BaseService

	config LogForwarderConfig

	// NATS publisher for publishing (interface for testability)
	publisher natsPublisher

	// Wrapped handler (preserves stdout/stderr output)
	wrappedHandler slog.Handler

	// Minimum level as slog.Level
	minLevel slog.Level

	// Accumulated attributes and groups
	attrs  []slog.Attr
	groups []string
	mu     sync.RWMutex

	// Internal logger (separate from wrapped handler)
	logger *slog.Logger
}

// NewLogForwarder creates a new LogForwarder service
func NewLogForwarder(
	config *LogForwarderConfig,
	natsClient *natsclient.Client,
	opts ...Option,
) (*LogForwarder, error) {
	return newLogForwarderWithPublisher(config, natsClient, opts...)
}

// newLogForwarderWithPublisher creates a new LogForwarder with a custom publisher (for testing)
func newLogForwarderWithPublisher(
	config *LogForwarderConfig,
	publisher natsPublisher,
	opts ...Option,
) (*LogForwarder, error) {
	if config == nil {
		config = &LogForwarderConfig{
			Enabled:  false,
			MinLevel: "INFO",
		}
	}

	// Create base service
	baseService := NewBaseServiceWithOptions("log-forwarder", nil, opts...)

	// Convert string level to slog.Level
	minLevel := stringToSlogLevel(config.MinLevel)

	// Create a default wrapped handler that writes to a no-op writer
	// This will be replaced when the handler is actually used in a logger
	wrappedHandler := slog.NewJSONHandler(nopWriter{}, &slog.HandlerOptions{
		Level: slog.LevelDebug, // Allow all levels through to wrapped handler
	})

	lf := &LogForwarder{
		BaseService:    baseService,
		config:         *config,
		publisher:      publisher, // Store as interface
		wrappedHandler: wrappedHandler,
		minLevel:       minLevel,
		attrs:          make([]slog.Attr, 0),
		groups:         make([]string, 0),
		logger:         slog.Default().With("component", "log-forwarder"),
	}

	return lf, nil
}

// Start begins log forwarding
func (lf *LogForwarder) Start(ctx context.Context) error {
	if err := lf.BaseService.Start(ctx); err != nil {
		return err
	}

	lf.logger.Info("LogForwarder started",
		"enabled", lf.config.Enabled,
		"min_level", lf.config.MinLevel)

	return nil
}

// Stop gracefully stops the LogForwarder
func (lf *LogForwarder) Stop(timeout time.Duration) error {
	lf.logger.Info("LogForwarder stopping")
	return lf.BaseService.Stop(timeout)
}

// Enabled reports whether the handler handles records at the given level.
// This implements slog.Handler interface.
func (lf *LogForwarder) Enabled(_ context.Context, level slog.Level) bool {
	// If service is disabled, we still pass through to wrapped handler
	// but we check level for our own publishing
	return level >= lf.minLevel
}

// Handle handles the Record.
// This implements slog.Handler interface.
func (lf *LogForwarder) Handle(ctx context.Context, record slog.Record) error {
	// Always delegate to wrapped handler first (preserves stdout/stderr)
	if err := lf.wrappedHandler.Handle(ctx, record); err != nil {
		// Don't fail the logging chain if wrapped handler fails
		lf.logger.Debug("wrapped handler error", "error", err)
	}

	// If disabled or below min level, we're done
	if !lf.config.Enabled || record.Level < lf.minLevel {
		return nil
	}

	// Extract source from attributes
	source := lf.extractSource(record)

	// Convert level to string
	levelStr := record.Level.String()

	// Build NATS subject: logs.{source}.{level}
	subject := fmt.Sprintf("logs.%s.%s", source, levelStr)

	// Build log entry
	entry := map[string]interface{}{
		"timestamp": record.Time.Format(time.RFC3339Nano),
		"level":     levelStr,
		"source":    source,
		"message":   record.Message,
		"fields":    lf.extractFields(record),
	}

	// Marshal to JSON
	data, err := json.Marshal(entry)
	if err != nil {
		// Log error but don't break logging chain
		lf.logger.Debug("failed to marshal log entry", "error", err)
		return nil
	}

	// Publish to NATS (async, don't block logging)
	go func() {
		if err := lf.publisher.Publish(ctx, subject, data); err != nil {
			// Log error but don't break logging chain
			lf.logger.Debug("failed to publish log to NATS",
				"subject", subject,
				"error", err)
		}
	}()

	return nil
}

// WithAttrs returns a new Handler whose attributes consist of
// both the receiver's attributes and the arguments.
// This implements slog.Handler interface.
func (lf *LogForwarder) WithAttrs(attrs []slog.Attr) slog.Handler {
	lf.mu.RLock()
	defer lf.mu.RUnlock()

	// Create a new handler with accumulated attributes
	newHandler := &LogForwarder{
		BaseService:    lf.BaseService,
		config:         lf.config,
		publisher:      lf.publisher,
		wrappedHandler: lf.wrappedHandler.WithAttrs(attrs),
		minLevel:       lf.minLevel,
		attrs:          append(append([]slog.Attr{}, lf.attrs...), attrs...),
		groups:         append([]string{}, lf.groups...),
		logger:         lf.logger,
	}

	return newHandler
}

// WithGroup returns a new Handler with the given group appended to
// the receiver's existing groups.
// This implements slog.Handler interface.
func (lf *LogForwarder) WithGroup(name string) slog.Handler {
	lf.mu.RLock()
	defer lf.mu.RUnlock()

	// Create a new handler with the group
	newHandler := &LogForwarder{
		BaseService:    lf.BaseService,
		config:         lf.config,
		publisher:      lf.publisher,
		wrappedHandler: lf.wrappedHandler.WithGroup(name),
		minLevel:       lf.minLevel,
		attrs:          append([]slog.Attr{}, lf.attrs...),
		groups:         append(append([]string{}, lf.groups...), name),
		logger:         lf.logger,
	}

	return newHandler
}

// extractSource extracts the source identifier from log attributes
// Priority: component > service > "system"
func (lf *LogForwarder) extractSource(record slog.Record) string {
	source := "system"

	// Check accumulated attributes first
	lf.mu.RLock()
	for _, attr := range lf.attrs {
		if attr.Key == "component" {
			source = attr.Value.String()
			lf.mu.RUnlock()
			return source
		}
	}
	for _, attr := range lf.attrs {
		if attr.Key == "service" {
			source = attr.Value.String()
			lf.mu.RUnlock()
			return source
		}
	}
	lf.mu.RUnlock()

	// Check record attributes
	record.Attrs(func(attr slog.Attr) bool {
		if attr.Key == "component" {
			source = attr.Value.String()
			return false // Stop iteration
		}
		return true
	})

	if source != "system" {
		return source
	}

	// Check for service attribute
	record.Attrs(func(attr slog.Attr) bool {
		if attr.Key == "service" {
			source = attr.Value.String()
			return false // Stop iteration
		}
		return true
	})

	return source
}

// extractFields extracts all attributes from the record as a map
func (lf *LogForwarder) extractFields(record slog.Record) map[string]interface{} {
	fields := make(map[string]interface{})

	// Add accumulated attributes
	lf.mu.RLock()
	for _, attr := range lf.attrs {
		fields[attr.Key] = attr.Value.Any()
	}
	lf.mu.RUnlock()

	// Add record attributes (may override accumulated)
	record.Attrs(func(attr slog.Attr) bool {
		fields[attr.Key] = attr.Value.Any()
		return true
	})

	return fields
}

// stringToSlogLevel converts a string level to slog.Level
func stringToSlogLevel(level string) slog.Level {
	switch strings.ToUpper(level) {
	case "DEBUG":
		return slog.LevelDebug
	case "INFO":
		return slog.LevelInfo
	case "WARN":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// nopWriter is a no-op writer that discards all writes
type nopWriter struct{}

func (nopWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}

// newLogForwarderForTest creates a LogForwarder for testing with a mock publisher.
// This is used by test helpers to bypass the Dependencies type constraint.
func newLogForwarderForTest(config *LogForwarderConfig, publisher natsPublisher, logger *slog.Logger) (*LogForwarder, error) {
	opts := []Option{}
	if logger != nil {
		opts = append(opts, WithLogger(logger))
	}
	return newLogForwarderWithPublisher(config, publisher, opts...)
}

// SetWrappedHandler sets a custom wrapped handler (for testing).
// This allows tests to verify that logs are delegated to the wrapped handler.
func (lf *LogForwarder) SetWrappedHandler(handler slog.Handler) {
	lf.wrappedHandler = handler
}
