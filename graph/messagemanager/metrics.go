// Package messagemanager provides the MessageHandler interface and Manager implementation
package messagemanager

import (
	"github.com/c360studio/semstreams/metric"
	"github.com/prometheus/client_golang/prometheus"
)

// Metrics holds Prometheus metrics for message manager operations
type Metrics struct {
	// Entity extraction metrics
	EntitiesExtracted prometheus.Counter

	// Entity update metrics (StoredMessage path + generic path)
	EntitiesUpdateAttempts prometheus.Counter
	EntitiesUpdateSuccess  prometheus.Counter
	EntitiesUpdateFailed   prometheus.Counter

	// Entity create metrics
	EntitiesCreateAttempts prometheus.Counter
	EntitiesCreateSuccess  prometheus.Counter
	EntitiesCreateFailed   prometheus.Counter

	// Message processing metrics
	MessagesProcessed prometheus.Counter
	MessagesFailed    prometheus.Counter
}

// NewMetrics creates and registers metrics with the provided registry.
func NewMetrics(registry *metric.MetricsRegistry) *Metrics {
	if registry == nil {
		return nil
	}

	const (
		namespace   = "semstreams"
		subsystem   = "messagemanager"
		serviceName = "messagemanager"
	)

	// Create metrics with standard namespace/subsystem pattern
	entitiesExtracted := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "entities_extracted_total",
		Help:      "Total entities extracted from messages",
	})

	entitiesUpdateAttempts := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "entities_update_attempts_total",
		Help:      "Total entity update attempts",
	})

	entitiesUpdateSuccess := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "entities_update_success_total",
		Help:      "Total successful entity updates",
	})

	entitiesUpdateFailed := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "entities_update_failed_total",
		Help:      "Total failed entity updates (silent loss detection)",
	})

	entitiesCreateAttempts := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "entities_create_attempts_total",
		Help:      "Total entity create attempts",
	})

	entitiesCreateSuccess := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "entities_create_success_total",
		Help:      "Total successful entity creates",
	})

	entitiesCreateFailed := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "entities_create_failed_total",
		Help:      "Total failed entity creates",
	})

	messagesProcessed := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "messages_processed_total",
		Help:      "Total messages processed by message manager",
	})

	messagesFailed := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "messages_failed_total",
		Help:      "Total messages that failed processing",
	})

	// Register with framework's registry
	registry.RegisterCounter(serviceName, "entities_extracted_total", entitiesExtracted)
	registry.RegisterCounter(serviceName, "entities_update_attempts_total", entitiesUpdateAttempts)
	registry.RegisterCounter(serviceName, "entities_update_success_total", entitiesUpdateSuccess)
	registry.RegisterCounter(serviceName, "entities_update_failed_total", entitiesUpdateFailed)
	registry.RegisterCounter(serviceName, "entities_create_attempts_total", entitiesCreateAttempts)
	registry.RegisterCounter(serviceName, "entities_create_success_total", entitiesCreateSuccess)
	registry.RegisterCounter(serviceName, "entities_create_failed_total", entitiesCreateFailed)
	registry.RegisterCounter(serviceName, "messages_processed_total", messagesProcessed)
	registry.RegisterCounter(serviceName, "messages_failed_total", messagesFailed)

	return &Metrics{
		EntitiesExtracted:      entitiesExtracted,
		EntitiesUpdateAttempts: entitiesUpdateAttempts,
		EntitiesUpdateSuccess:  entitiesUpdateSuccess,
		EntitiesUpdateFailed:   entitiesUpdateFailed,
		EntitiesCreateAttempts: entitiesCreateAttempts,
		EntitiesCreateSuccess:  entitiesCreateSuccess,
		EntitiesCreateFailed:   entitiesCreateFailed,
		MessagesProcessed:      messagesProcessed,
		MessagesFailed:         messagesFailed,
	}
}
