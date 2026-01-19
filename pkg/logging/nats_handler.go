package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// NATSPublisher defines the interface needed for publishing to NATS JetStream.
// Uses PublishToStream for durability - logs are persisted to JetStream LOGS stream.
// This allows for easier testing with mocks.
type NATSPublisher interface {
	PublishToStream(ctx context.Context, subject string, data []byte) error
}

// NATSLogHandler is an slog.Handler that publishes log records to NATS JetStream.
// Logs are published to subjects in the format: logs.{level}.{source}
// This enables NATS wildcard filtering:
//   - logs.WARN.> (all WARN and above)
//   - logs.*.graph-processor (one component, all levels)
//
// The handler requires a non-nil publisher at construction time.
// Create the handler AFTER NATS is connected and streams are created.
type NATSLogHandler struct {
	publisher      NATSPublisher
	minLevel       slog.Level
	excludeSources []string
	attrs          []slog.Attr
	groups         []string
}

// NATSLogHandlerConfig holds configuration for NATSLogHandler.
type NATSLogHandlerConfig struct {
	MinLevel       slog.Level
	ExcludeSources []string
}

// NewNATSLogHandler creates a new NATSLogHandler.
// The publisher must be non-nil - create this handler AFTER NATS is connected.
func NewNATSLogHandler(publisher NATSPublisher, cfg NATSLogHandlerConfig) *NATSLogHandler {
	if publisher == nil {
		panic("NATSLogHandler requires a non-nil publisher - create handler after NATS is connected")
	}
	return &NATSLogHandler{
		publisher:      publisher,
		minLevel:       cfg.MinLevel,
		excludeSources: cfg.ExcludeSources,
		attrs:          make([]slog.Attr, 0),
		groups:         make([]string, 0),
	}
}

// Enabled reports whether the handler handles records at the given level.
func (h *NATSLogHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.minLevel
}

// Handle publishes the log record to NATS.
func (h *NATSLogHandler) Handle(_ context.Context, r slog.Record) error {
	// Check level
	if r.Level < h.minLevel {
		return nil
	}

	// Extract source from attributes
	source := h.extractSource(r)

	// Check exclude list (prefix matching)
	if h.isExcluded(source) {
		return nil
	}

	// Build log entry
	entry := h.buildLogEntry(r, source)

	// Marshal to JSON
	data, err := json.Marshal(entry)
	if err != nil {
		// Can't log the error (would cause recursion), just drop it
		return nil
	}

	// Build subject: logs.{level}.{source}
	// This enables NATS wildcard filtering (e.g., logs.WARN.>, logs.*.graph-processor)
	subject := fmt.Sprintf("logs.%s.%s", r.Level.String(), source)

	// Async publish to JetStream - don't block logging
	// JetStream provides durability - messages are persisted to the LOGS stream
	go func() {
		// Use background context since original ctx may be cancelled
		_ = h.publisher.PublishToStream(context.Background(), subject, data)
	}()

	return nil
}

// WithAttrs returns a new handler with the given attributes added.
func (h *NATSLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	copy(newAttrs[len(h.attrs):], attrs)

	return &NATSLogHandler{
		publisher:      h.publisher,
		minLevel:       h.minLevel,
		excludeSources: h.excludeSources,
		attrs:          newAttrs,
		groups:         h.groups,
	}
}

// WithGroup returns a new handler with the given group added.
func (h *NATSLogHandler) WithGroup(name string) slog.Handler {
	newGroups := make([]string, len(h.groups)+1)
	copy(newGroups, h.groups)
	newGroups[len(h.groups)] = name

	return &NATSLogHandler{
		publisher:      h.publisher,
		minLevel:       h.minLevel,
		excludeSources: h.excludeSources,
		attrs:          h.attrs,
		groups:         newGroups,
	}
}

// isExcluded checks if a source should be excluded from NATS publishing.
// Uses prefix matching: excluding "flow-service.websocket" also excludes
// "flow-service.websocket.health" but NOT "flow-service".
func (h *NATSLogHandler) isExcluded(source string) bool {
	for _, prefix := range h.excludeSources {
		if source == prefix || strings.HasPrefix(source, prefix+".") {
			return true
		}
	}
	return false
}

// extractSource extracts the source identifier from the log record.
// Priority: source > component > service > "system"
func (h *NATSLogHandler) extractSource(r slog.Record) string {
	var source, component, service string

	// Check handler's accumulated attributes first
	for _, attr := range h.attrs {
		switch attr.Key {
		case "source":
			source = attr.Value.String()
		case "component":
			component = attr.Value.String()
		case "service":
			service = attr.Value.String()
		}
	}

	// Check record's attributes (override accumulated ones)
	r.Attrs(func(attr slog.Attr) bool {
		switch attr.Key {
		case "source":
			source = attr.Value.String()
		case "component":
			component = attr.Value.String()
		case "service":
			service = attr.Value.String()
		}
		return true
	})

	// Priority: source > component > service > "system"
	if source != "" {
		return source
	}
	if component != "" {
		return component
	}
	if service != "" {
		return service
	}
	return "system"
}

// buildLogEntry creates the JSON structure for a log entry.
func (h *NATSLogHandler) buildLogEntry(r slog.Record, source string) map[string]any {
	// Collect all fields
	fields := make(map[string]any)

	// Add accumulated attributes
	for _, attr := range h.attrs {
		// Skip source/component/service as they're top-level
		if attr.Key == "source" || attr.Key == "component" || attr.Key == "service" {
			continue
		}
		fields[attr.Key] = attr.Value.Any()
	}

	// Add record attributes
	r.Attrs(func(attr slog.Attr) bool {
		// Skip source/component/service as they're top-level
		if attr.Key == "source" || attr.Key == "component" || attr.Key == "service" {
			return true
		}
		fields[attr.Key] = attr.Value.Any()
		return true
	})

	entry := map[string]any{
		"timestamp": r.Time.Format(time.RFC3339Nano),
		"level":     r.Level.String(),
		"source":    source,
		"message":   r.Message,
	}

	if len(fields) > 0 {
		entry["fields"] = fields
	}

	return entry
}
