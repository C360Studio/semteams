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

	// Parse exclude_sources from log-forwarder config.
	// Default excludes WebSocket worker logs to prevent feedback loops where
	// debug logs from WebSocket workers get sent back over the same WebSocket.
	excludeSources := []string{"flow-service.websocket"}

	if cfg != nil && cfg.Services != nil {
		slog.Debug("Checking log-forwarder config", "services_count", len(cfg.Services))
		if logFwdCfg, ok := cfg.Services["log-forwarder"]; ok {
			slog.Debug("Found log-forwarder service config",
				"enabled", logFwdCfg.Enabled,
				"config_raw", string(logFwdCfg.Config))
			if logFwdCfg.Enabled {
				var lfCfg struct {
					ExcludeSources []string `json:"exclude_sources"`
				}
				if err := json.Unmarshal(logFwdCfg.Config, &lfCfg); err != nil {
					slog.Warn("Failed to parse log-forwarder config", "error", err)
				} else if len(lfCfg.ExcludeSources) > 0 {
					// Config overrides default if explicitly set
					excludeSources = lfCfg.ExcludeSources
					slog.Info("Loaded exclude_sources from log-forwarder config", "exclude_sources", excludeSources)
				} else {
					slog.Debug("No exclude_sources in config, using default")
				}
			}
		} else {
			slog.Debug("log-forwarder service not found in config")
		}
	} else {
		slog.Debug("No services in config", "cfg_nil", cfg == nil)
	}
	slog.Info("NATSLogHandler exclude_sources configured", "exclude_sources", excludeSources)

	// Create NATS handler for out-of-band logging
	// Log the configuration for debugging
	slog.Debug("Creating NATSLogHandler",
		"min_level", logLevel.String(),
		"exclude_sources", excludeSources)

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
