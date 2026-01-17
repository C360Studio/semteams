// Package config provides configuration management for SemStreams.
package config

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360/semstreams/natsclient"
)

// StreamConfig defines configuration for a JetStream stream.
type StreamConfig struct {
	Subjects  []string `json:"subjects"`            // Subjects captured by this stream
	Storage   string   `json:"storage,omitempty"`   // "file" or "memory" (default: file)
	MaxAge    string   `json:"max_age,omitempty"`   // TTL for messages (e.g., "168h", "7d")
	MaxBytes  int64    `json:"max_bytes,omitempty"` // Max storage size in bytes (0 = unlimited)
	Retention string   `json:"retention,omitempty"` // "limits", "interest", "workqueue" (default: limits)
	Replicas  int      `json:"replicas,omitempty"`  // Replication factor (default: 1)
}

// StreamConfigs is a map of stream name to configuration.
type StreamConfigs map[string]StreamConfig

// DeriveStreamName extracts stream name from subject convention.
// Convention: subject "component.action.type" → stream "COMPONENT"
// Examples:
//
//	"objectstore.stored.entity" → "OBJECTSTORE"
//	"sensor.processed.entity"   → "SENSOR"
//	"rule.triggered.alert"      → "RULE"
func DeriveStreamName(subject string) string {
	// Handle wildcard subjects by extracting the first segment
	subject = strings.TrimPrefix(subject, "*.")
	subject = strings.TrimSuffix(subject, ".>")
	subject = strings.TrimSuffix(subject, ".*")

	parts := strings.Split(subject, ".")
	if len(parts) == 0 || parts[0] == "" || parts[0] == "*" || parts[0] == ">" {
		return ""
	}
	return strings.ToUpper(parts[0])
}

// DeriveStreamSubjects creates wildcard pattern for stream capture.
// Convention: subject "component.action.type" → ["component.>"]
// Examples:
//
//	"objectstore.stored.entity" → ["objectstore.>"]
//	"sensor.processed.entity"   → ["sensor.>"]
func DeriveStreamSubjects(subject string) []string {
	streamName := DeriveStreamName(subject)
	if streamName == "" {
		return nil
	}
	return []string{strings.ToLower(streamName) + ".>"}
}

// StreamsManager handles JetStream stream creation and management.
type StreamsManager struct {
	natsClient *natsclient.Client
	logger     *slog.Logger
}

// NewStreamsManager creates a new StreamsManager.
func NewStreamsManager(natsClient *natsclient.Client, logger *slog.Logger) *StreamsManager {
	return &StreamsManager{
		natsClient: natsClient,
		logger:     logger,
	}
}

// logsStreamConfig defines the configuration for the LOGS stream.
// This stream captures all application logs with automatic expiration.
var logsStreamConfig = StreamConfig{
	Subjects: []string{"logs.>"},
	Storage:  "file",
	MaxAge:   "1h",              // TTL: expire after 1 hour
	MaxBytes: 100 * 1024 * 1024, // 100MB max storage
	Replicas: 1,
}

// EnsureStreams creates all required JetStream streams based on:
// 1. System streams (LOGS for out-of-band logging)
// 2. Explicit streams defined in config.Streams (highest priority)
// 3. Streams derived from component JetStream output ports
func (sm *StreamsManager) EnsureStreams(ctx context.Context, cfg *Config) error {
	streams := make(map[string]StreamConfig)

	// 1. Always create LOGS stream for out-of-band logging
	streams["LOGS"] = logsStreamConfig
	sm.logger.Debug("Adding system LOGS stream", "subjects", logsStreamConfig.Subjects)

	// 2. Explicit streams from config (can override system streams)
	for name, sc := range cfg.Streams {
		streams[name] = sc
		sm.logger.Debug("Found explicit stream config", "stream", name, "subjects", sc.Subjects)
	}

	// 3. Derive streams from component JetStream output ports
	for compName, compCfg := range cfg.Components {
		if !compCfg.Enabled {
			continue
		}

		// Parse component config to extract port definitions
		ports, err := sm.extractPortsFromConfig(compCfg.Config)
		if err != nil {
			sm.logger.Debug("Could not parse ports from component config",
				"component", compName, "error", err)
			continue
		}

		for _, port := range ports.Outputs {
			if port.Type != "jetstream" {
				continue
			}

			streamName := DeriveStreamName(port.Subject)
			if streamName == "" {
				sm.logger.Warn("Could not derive stream name from subject",
					"component", compName, "subject", port.Subject)
				continue
			}

			// Only add if not already explicitly configured
			if _, exists := streams[streamName]; !exists {
				streams[streamName] = StreamConfig{
					Subjects: DeriveStreamSubjects(port.Subject),
					// Defaults will be applied in createStream
				}
				sm.logger.Debug("Derived stream from component port",
					"stream", streamName,
					"component", compName,
					"subject", port.Subject)
			}
		}
	}

	// 4. Create all streams
	for name, streamCfg := range streams {
		if err := sm.createStream(ctx, name, streamCfg); err != nil {
			return fmt.Errorf("create stream %s: %w", name, err)
		}
	}

	sm.logger.Info("Ensured JetStream streams", "count", len(streams))
	return nil
}

// PortsConfig represents the ports section of a component config.
type PortsConfig struct {
	Inputs  []PortDefinition `json:"inputs,omitempty"`
	Outputs []PortDefinition `json:"outputs,omitempty"`
}

// PortDefinition represents a single port definition.
type PortDefinition struct {
	Name    string `json:"name"`
	Subject string `json:"subject"`
	Type    string `json:"type"` // "nats", "jetstream", etc.
}

// extractPortsFromConfig parses port definitions from raw component config.
func (sm *StreamsManager) extractPortsFromConfig(rawConfig json.RawMessage) (*PortsConfig, error) {
	var cfg struct {
		Ports PortsConfig `json:"ports"`
	}
	if err := json.Unmarshal(rawConfig, &cfg); err != nil {
		return nil, err
	}
	return &cfg.Ports, nil
}

// createStream creates or updates a JetStream stream.
func (sm *StreamsManager) createStream(ctx context.Context, name string, cfg StreamConfig) error {
	js, err := sm.natsClient.JetStream()
	if err != nil {
		return fmt.Errorf("get JetStream context: %w", err)
	}

	// Parse storage type
	storage := jetstream.FileStorage
	if cfg.Storage == "memory" {
		storage = jetstream.MemoryStorage
	}

	// Parse retention policy
	retention := jetstream.LimitsPolicy
	switch cfg.Retention {
	case "interest":
		retention = jetstream.InterestPolicy
	case "workqueue":
		retention = jetstream.WorkQueuePolicy
	}

	// Parse max age
	var maxAge time.Duration
	if cfg.MaxAge != "" {
		var err error
		maxAge, err = parseDurationWithDays(cfg.MaxAge)
		if err != nil {
			sm.logger.Warn("Invalid max_age, using default",
				"stream", name, "max_age", cfg.MaxAge, "error", err)
			maxAge = 7 * 24 * time.Hour // Default: 7 days
		}
	} else {
		maxAge = 7 * 24 * time.Hour // Default: 7 days
	}

	// Replicas default
	replicas := cfg.Replicas
	if replicas <= 0 {
		replicas = 1
	}

	streamCfg := jetstream.StreamConfig{
		Name:      name,
		Subjects:  cfg.Subjects,
		Storage:   storage,
		Retention: retention,
		MaxAge:    maxAge,
		MaxBytes:  cfg.MaxBytes, // 0 means unlimited
		Discard:   jetstream.DiscardOld,
		Replicas:  replicas,
	}

	// Try to get existing stream
	existingStream, err := js.Stream(ctx, name)
	if err == nil {
		// Stream exists - check if subjects match
		existingCfg := existingStream.CachedInfo().Config
		if !subjectsEqual(existingCfg.Subjects, cfg.Subjects) {
			sm.logger.Info("Updating stream subjects",
				"stream", name,
				"old_subjects", existingCfg.Subjects,
				"new_subjects", cfg.Subjects)
			_, err = js.UpdateStream(ctx, streamCfg)
			if err != nil {
				return fmt.Errorf("update stream: %w", err)
			}
		} else {
			sm.logger.Debug("Stream already exists with correct config", "stream", name)
		}
		return nil
	}

	// Stream doesn't exist - create it
	_, err = js.CreateStream(ctx, streamCfg)
	if err != nil {
		return fmt.Errorf("create stream: %w", err)
	}

	sm.logger.Info("Created JetStream stream",
		"stream", name,
		"subjects", cfg.Subjects,
		"storage", cfg.Storage,
		"max_age", maxAge)

	return nil
}

// subjectsEqual checks if two subject lists are equal (order-independent).
func subjectsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	aSet := make(map[string]bool)
	for _, s := range a {
		aSet[s] = true
	}
	for _, s := range b {
		if !aSet[s] {
			return false
		}
	}
	return true
}
