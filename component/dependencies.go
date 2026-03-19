package component

import (
	"log/slog"

	"github.com/c360studio/semstreams/metric"
	"github.com/c360studio/semstreams/model"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/security"
	"github.com/c360studio/semstreams/types"
)

// PlatformMeta provides platform identity to components.
// Type alias to avoid import cycles while maintaining compatibility.
type PlatformMeta = types.PlatformMeta

// Lookup provides read-only access to sibling components at call time.
// Lazy lookup avoids stale pointers when ComponentManager restarts components.
type Lookup interface {
	Component(name string) Discoverable
}

// Dependencies provides all external dependencies needed by components.
type Dependencies struct {
	NATSClient        *natsclient.Client      // NATS client for messaging
	KVWatchClient     *natsclient.Client      // Dedicated client for heavy KV watchers (can be nil, falls back to NATSClient)
	MetricsRegistry   *metric.MetricsRegistry // Metrics registry for Prometheus (can be nil)
	Logger            *slog.Logger            // Structured logger (can be nil, defaults to slog.Default())
	Platform          PlatformMeta            // Platform identity (organization and platform)
	Security          security.Config         // Platform-wide security configuration
	ModelRegistry     model.RegistryReader    // Unified model registry (can be nil)
	ComponentRegistry Lookup                  // Sibling component lookup (can be nil)
}

// GetLogger returns the configured logger or a default logger if none is provided
func (d *Dependencies) GetLogger() *slog.Logger {
	if d.Logger != nil {
		return d.Logger
	}
	return slog.Default()
}

// GetLoggerWithComponent returns a logger configured with component context
func (d *Dependencies) GetLoggerWithComponent(componentName string) *slog.Logger {
	return d.GetLogger().With("component", componentName)
}
