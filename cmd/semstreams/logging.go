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

// setupLogger creates the initial logger (stdout only, before NATS is connected).
func setupLogger(level, format string) *slog.Logger {
	logLevel := parseLogLevel(level)

	opts := &slog.HandlerOptions{
		Level:     logLevel,
		AddSource: level == "debug",
	}

	// Create handler based on format
	var handler slog.Handler
	switch strings.ToLower(format) {
	case "json":
		handler = slog.NewJSONHandler(os.Stdout, opts)
	case "text":
		handler = slog.NewTextHandler(os.Stdout, opts)
	default:
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	return slog.New(handler).With(
		"service", "semstreams",
		"version", Version,
		"pid", os.Getpid(),
	)
}

// setupLoggerWithNATS upgrades the logger to also publish to NATS.
// This should be called after NATS is connected and streams are created.
// Returns the new logger that should be set as default and used for deps.
func setupLoggerWithNATS(
	level, format string,
	natsClient *natsclient.Client,
	cfg *config.Config,
) *slog.Logger {
	logLevel := parseLogLevel(level)

	opts := &slog.HandlerOptions{
		Level:     logLevel,
		AddSource: level == "debug",
	}

	// Create stdout handler (existing behavior)
	var stdoutHandler slog.Handler
	switch strings.ToLower(format) {
	case "json":
		stdoutHandler = slog.NewJSONHandler(os.Stdout, opts)
	case "text":
		stdoutHandler = slog.NewTextHandler(os.Stdout, opts)
	default:
		stdoutHandler = slog.NewJSONHandler(os.Stdout, opts)
	}

	// Parse exclude_sources from log-forwarder config
	var excludeSources []string
	if cfg != nil && cfg.Services != nil {
		if logFwdCfg, ok := cfg.Services["log-forwarder"]; ok && logFwdCfg.Enabled {
			var lfCfg struct {
				ExcludeSources []string `json:"exclude_sources"`
			}
			if err := json.Unmarshal(logFwdCfg.Config, &lfCfg); err == nil {
				excludeSources = lfCfg.ExcludeSources
			}
		}
	}

	// Create NATS handler for out-of-band logging
	natsHandler := logging.NewNATSLogHandler(natsClient, logging.NATSLogHandlerConfig{
		MinLevel:       logLevel,
		ExcludeSources: excludeSources,
	})

	// Compose handlers using MultiHandler
	multiHandler := logging.NewMultiHandler(stdoutHandler, natsHandler)

	return slog.New(multiHandler).With(
		"service", "semstreams",
		"version", Version,
		"pid", os.Getpid(),
	)
}
