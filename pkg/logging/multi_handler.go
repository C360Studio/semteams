// Package logging provides slog handlers for structured logging across the application.
package logging

import (
	"context"
	"log/slog"
)

// MultiHandler composes multiple slog.Handler instances, dispatching log records
// to all of them. This allows logs to be written to multiple destinations
// (e.g., stdout and NATS) simultaneously.
type MultiHandler struct {
	handlers []slog.Handler
}

// NewMultiHandler creates a new MultiHandler that dispatches to all provided handlers.
func NewMultiHandler(handlers ...slog.Handler) *MultiHandler {
	return &MultiHandler{handlers: handlers}
}

// Enabled reports whether any handler handles records at the given level.
func (m *MultiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

// Handle dispatches the record to all handlers.
// If a handler fails, we continue to the next handler (don't fail the logging chain).
func (m *MultiHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, h := range m.handlers {
		if h.Enabled(ctx, r.Level) {
			// Ignore errors from individual handlers - don't break logging chain
			_ = h.Handle(ctx, r)
		}
	}
	return nil
}

// WithAttrs returns a new MultiHandler with the given attributes added to all handlers.
func (m *MultiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		handlers[i] = h.WithAttrs(attrs)
	}
	return &MultiHandler{handlers: handlers}
}

// WithGroup returns a new MultiHandler with the given group added to all handlers.
func (m *MultiHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		handlers[i] = h.WithGroup(name)
	}
	return &MultiHandler{handlers: handlers}
}
