package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/c360studio/semstreams/config"
)

const natsLogPublishTimeout = 5 * time.Second

// logStreamPublisher is the narrow NATS JetStream publish contract needed by
// setupLogger (*natsclient.Client satisfies it directly).
type logStreamPublisher interface {
	PublishToStream(ctx context.Context, subject string, data []byte) error
}

// parseLogLevel converts a string log level to slog.Level.
func parseLogLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// setupLogger creates the application logger writing to os.Stdout and the NATS
// JetStream LOGS stream via stdlib slog.NewMultiHandler.
// Call this only AFTER NATS is connected and configured streams exist.
func setupLogger(level, format string, publisher logStreamPublisher, cfg *config.Config) *slog.Logger {
	return newLogger(os.Stdout, level, format, publisher, cfg)
}

func newLogger(w io.Writer, level, format string, publisher logStreamPublisher, cfg *config.Config) *slog.Logger {
	logLevel := parseLogLevel(level)

	opts := &slog.HandlerOptions{
		Level:     logLevel,
		AddSource: logLevel == slog.LevelDebug,
	}

	var stdoutHandler slog.Handler
	switch strings.ToLower(format) {
	case "json":
		stdoutHandler = slog.NewJSONHandler(w, opts)
	case "text":
		stdoutHandler = slog.NewTextHandler(w, opts)
	default:
		stdoutHandler = slog.NewJSONHandler(w, opts)
	}

	excludeSources := getExcludeSources(cfg)
	natsHandler := newNATSLogHandler(publisher, logLevel, excludeSources)
	multiHandler := slog.NewMultiHandler(stdoutHandler, natsHandler)

	return slog.New(multiHandler).With(
		"service", appName,
		"version", version,
		"pid", os.Getpid(),
	)
}

// getExcludeSources extracts exclude_sources from log-forwarder config.
func getExcludeSources(cfg *config.Config) []string {
	// Default: exclude WebSocket worker logs to prevent feedback loops.
	excludeSources := []string{"flow-service.websocket"}

	if cfg == nil || cfg.Services == nil {
		return excludeSources
	}

	logFwdCfg, ok := cfg.Services["log-forwarder"]
	if !ok || !logFwdCfg.Enabled {
		return excludeSources
	}

	var lfCfg struct {
		ExcludeSources []string `json:"exclude_sources"`
	}
	if err := json.Unmarshal(logFwdCfg.Config, &lfCfg); err == nil && len(lfCfg.ExcludeSources) > 0 {
		return lfCfg.ExcludeSources
	}

	return excludeSources
}

// natsLogHandler publishes structured log records to NATS JetStream subjects
// shaped logs.<LEVEL>.<source>, preserving frozen semstreams/pkg/logging/nats_handler.go
// wire and filtering semantics without importing unported pkg/logging.
type natsLogHandler struct {
	publisher      logStreamPublisher
	minLevel       slog.Level
	excludeSources []string
	attrs          []slog.Attr
	groups         []string
}

func newNATSLogHandler(publisher logStreamPublisher, minLevel slog.Level, excludeSources []string) *natsLogHandler {
	if publisher == nil {
		panic("natsLogHandler requires a non-nil publisher - create handler after NATS is connected")
	}
	return &natsLogHandler{
		publisher:      publisher,
		minLevel:       minLevel,
		excludeSources: slices.Clone(excludeSources),
		attrs:          make([]slog.Attr, 0),
		groups:         make([]string, 0),
	}
}

func (h *natsLogHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.minLevel
}

func (h *natsLogHandler) Handle(_ context.Context, r slog.Record) error {
	if r.Level < h.minLevel {
		return nil
	}

	source := h.extractSource(r)
	if h.isExcluded(source) {
		return nil
	}

	entry := h.buildLogEntry(r, source)
	data, err := json.Marshal(entry)
	if err != nil {
		return nil
	}

	subject := fmt.Sprintf("logs.%s.%s", r.Level.String(), source)

	// shortcut: fire-and-forget goroutine with bounded 5s timeout matches frozen semstreams/pkg/logging/nats_handler.go while bounding stalled publishes; errors are dropped to prevent recursive slog loops
	go func() {
		pubCtx, cancel := context.WithTimeout(context.Background(), natsLogPublishTimeout)
		defer cancel()
		_ = h.publisher.PublishToStream(pubCtx, subject, data)
	}()

	return nil
}

func (h *natsLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	newAttrs := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	copy(newAttrs[len(h.attrs):], attrs)

	return &natsLogHandler{
		publisher:      h.publisher,
		minLevel:       h.minLevel,
		excludeSources: h.excludeSources,
		attrs:          newAttrs,
		groups:         h.groups,
	}
}

func (h *natsLogHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	newGroups := make([]string, len(h.groups)+1)
	copy(newGroups, h.groups)
	newGroups[len(h.groups)] = name

	return &natsLogHandler{
		publisher:      h.publisher,
		minLevel:       h.minLevel,
		excludeSources: h.excludeSources,
		attrs:          h.attrs,
		groups:         newGroups,
	}
}

func (h *natsLogHandler) isExcluded(source string) bool {
	for _, prefix := range h.excludeSources {
		if source == prefix || strings.HasPrefix(source, prefix+".") {
			return true
		}
	}
	return false
}

func (h *natsLogHandler) extractSource(r slog.Record) string {
	var source, component, service string

	for _, attr := range h.attrs {
		applySourceAttr(attr, &source, &component, &service)
	}
	r.Attrs(func(attr slog.Attr) bool {
		applySourceAttr(attr, &source, &component, &service)
		return true
	})

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

func applySourceAttr(attr slog.Attr, source, component, service *string) {
	val := attr.Value.Resolve()
	switch attr.Key {
	case "source":
		*source = val.String()
	case "component":
		*component = val.String()
	case "service":
		*service = val.String()
	}
}

func (h *natsLogHandler) buildLogEntry(r slog.Record, source string) map[string]any {
	fields := make(map[string]any)

	for _, attr := range h.attrs {
		if isRoutingSourceKey(attr.Key) {
			continue
		}
		fields[attr.Key] = attr.Value.Resolve().Any()
	}
	r.Attrs(func(attr slog.Attr) bool {
		if isRoutingSourceKey(attr.Key) {
			return true
		}
		fields[attr.Key] = attr.Value.Resolve().Any()
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

func isRoutingSourceKey(key string) bool {
	return key == "source" || key == "component" || key == "service"
}
