package service

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/c360/semstreams/component"
	"github.com/c360/semstreams/config"
	"github.com/c360/semstreams/metric"
	"github.com/c360/semstreams/natsclient"
	"github.com/c360/semstreams/types"
)

// natsPublisher defines the minimal interface needed for publishing to NATS.
// This allows for easier testing with mocks.
type natsPublisher interface {
	Publish(ctx context.Context, subject string, data []byte) error
}

// Dependencies provides the standard dependencies that all services receive.
// This replaces the old Dependencies struct and provides consistent injection.
// Services should use HTTP or NATS RPC for inter-service communication.
type Dependencies struct {
	NATSClient        *natsclient.Client
	MetricsRegistry   *metric.MetricsRegistry
	Logger            *slog.Logger
	Platform          types.PlatformMeta  // Platform identity
	Manager           *config.Manager     // Centralized configuration management
	ComponentRegistry *component.Registry // Component registry for ComponentManager
	ServiceManager    *Manager            // Service manager for accessing other services
}

// Constructor defines the standard constructor signature for all services.
// Every service must have a constructor that follows this pattern.
// The constructor receives raw JSON config and must handle its own parsing.
type Constructor func(rawConfig json.RawMessage, deps *Dependencies) (Service, error)
