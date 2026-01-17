package main

import (
	"encoding/json"
	"log/slog"
	"os"
	"strings"

	"github.com/c360/semstreams/config"
	"github.com/c360/semstreams/natsclient"
	"github.com/c360/semstreams/pkg/logging"
)

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

// LoggerComponents holds the components needed for logger setup.
// This allows setting up the handler early and wiring NATS later.
type LoggerComponents struct {
	Logger      *slog.Logger
	NATSHandler *logging.NATSLogHandler
}

// setupLoggerEarly creates the logger with MultiHandler immediately at startup.
// The NATSLogHandler starts with nil publisher - call WireNATS() after NATS connects.
// This ensures all components that call slog.Default() get the correct handler.
func setupLoggerEarly(level, format string) *LoggerComponents {
	logLevel := parseLogLevel(level)

	opts := &slog.HandlerOptions{
		Level:     logLevel,
		AddSource: level == "debug",
	}

	// Create stdout handler
	var stdoutHandler slog.Handler
	switch strings.ToLower(format) {
	case "json":
		stdoutHandler = slog.NewJSONHandler(os.Stdout, opts)
	case "text":
		stdoutHandler = slog.NewTextHandler(os.Stdout, opts)
	default:
		stdoutHandler = slog.NewJSONHandler(os.Stdout, opts)
	}

	// Create NATS handler with nil publisher - will be set later
	// Default exclude_sources - can be updated when config is loaded
	natsHandler := logging.NewNATSLogHandler(nil, logging.NATSLogHandlerConfig{
		MinLevel:       logLevel,
		ExcludeSources: []string{"flow-service.websocket"},
	})

	// Compose handlers using MultiHandler
	multiHandler := logging.NewMultiHandler(stdoutHandler, natsHandler)

	logger := slog.New(multiHandler).With(
		"service", "semstreams",
		"version", Version,
		"pid", os.Getpid(),
	)

	return &LoggerComponents{
		Logger:      logger,
		NATSHandler: natsHandler,
	}
}

// WireNATS connects the NATSLogHandler to the NATS client.
// Call this after NATS is connected and streams are created.
func (lc *LoggerComponents) WireNATS(natsClient *natsclient.Client, cfg *config.Config) {
	// Update exclude_sources from config if available
	excludeSources := []string{"flow-service.websocket"}

	if cfg != nil && cfg.Services != nil {
		if logFwdCfg, ok := cfg.Services["log-forwarder"]; ok && logFwdCfg.Enabled {
			var lfCfg struct {
				ExcludeSources []string `json:"exclude_sources"`
			}
			if err := json.Unmarshal(logFwdCfg.Config, &lfCfg); err == nil && len(lfCfg.ExcludeSources) > 0 {
				excludeSources = lfCfg.ExcludeSources
				slog.Info("Loaded exclude_sources from log-forwarder config", "exclude_sources", excludeSources)
			}
		}
	}

	// Update the handler's exclude sources and set the publisher
	lc.NATSHandler.SetExcludeSources(excludeSources)
	lc.NATSHandler.SetPublisher(natsClient)

	slog.Info("Logger wired to NATS", "exclude_sources", excludeSources)
}
