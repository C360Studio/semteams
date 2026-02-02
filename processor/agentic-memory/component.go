// Package agenticmemory provides a graph-backed agent memory processor component
// that manages context hydration, fact extraction, and memory checkpointing for agentic loops.
package agenticmemory

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/nats-io/nats.go/jetstream"
)

// agenticMemorySchema defines the configuration schema
var agenticMemorySchema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// Component implements the agentic-memory processor
type Component struct {
	name       string
	config     Config
	natsClient *natsclient.Client
	logger     *slog.Logger

	hydrator  *Hydrator
	extractor *LLMExtractor

	// Lifecycle management
	running   bool
	startTime time.Time
	mu        sync.RWMutex

	// Metrics
	eventsProcessed int64
	errors          int64
	lastActivity    time.Time
}

// NewComponent creates a new agentic-memory processor component
func NewComponent(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	var config Config
	if err := json.Unmarshal(rawConfig, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Use default config if ports not set
	if config.Ports == nil {
		config = DefaultConfig()
		// Re-unmarshal to get user-provided values
		if err := json.Unmarshal(rawConfig, &config); err != nil {
			return nil, fmt.Errorf("failed to unmarshal config: %w", err)
		}
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Create hydrator (graph client will be provided later during initialization)
	hydrator, err := NewHydrator(config.Hydration, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create hydrator: %w", err)
	}

	// Create LLM extractor (LLM client will be provided later during initialization)
	extractor, err := NewLLMExtractor(config.Extraction, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create extractor: %w", err)
	}

	return &Component{
		name:       "agentic-memory",
		config:     config,
		natsClient: deps.NATSClient,
		logger:     deps.GetLogger(),
		hydrator:   hydrator,
		extractor:  extractor,
	}, nil
}

// Initialize prepares the component
func (c *Component) Initialize() error {
	return nil
}

// Start begins processing memory events
func (c *Component) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return fmt.Errorf("component already running")
	}
	// Mark as running immediately to prevent concurrent Start calls
	c.running = true
	c.mu.Unlock()

	// NATS client is optional for unit tests
	if c.natsClient != nil {
		if err := c.setupInputConsumers(ctx); err != nil {
			// Reset running state on failure
			c.mu.Lock()
			c.running = false
			c.mu.Unlock()
			return fmt.Errorf("failed to setup input consumers: %w", err)
		}
	}

	c.mu.Lock()
	c.startTime = time.Now()
	c.mu.Unlock()

	return nil
}

// setupInputConsumers sets up JetStream consumers for all input ports
func (c *Component) setupInputConsumers(ctx context.Context) error {
	for _, port := range c.config.Ports.Inputs {
		subject := port.Subject
		if subject == "" {
			continue
		}

		var handler func(context.Context, []byte)

		// Route to appropriate handler based on port name
		switch port.Name {
		case "compaction_events":
			handler = c.handleCompactionEvent
		case "hydrate_requests":
			handler = c.handleHydrateRequest
		default:
			c.logger.Debug("Skipping unknown input port", "port", port.Name)
			continue
		}

		if err := c.setupConsumer(ctx, port.Name, subject, handler); err != nil {
			return fmt.Errorf("failed to setup consumer for %s: %w", port.Name, err)
		}
	}

	return nil
}

// setupConsumer sets up a JetStream consumer for an input port
func (c *Component) setupConsumer(ctx context.Context, portName, subject string, handler func(context.Context, []byte)) error {
	// Determine stream name
	streamName := c.config.StreamName
	if streamName == "" {
		streamName = "AGENT"
	}

	// Wait for stream to be available
	if err := c.waitForStream(ctx, streamName); err != nil {
		return fmt.Errorf("stream %s not available: %w", streamName, err)
	}

	// Create durable consumer name
	consumerName := fmt.Sprintf("agentic-memory-%s", sanitizeSubject(subject))
	if c.config.ConsumerNameSuffix != "" {
		consumerName = consumerName + "-" + c.config.ConsumerNameSuffix
	}

	c.logger.Info("Setting up JetStream consumer",
		"stream", streamName,
		"consumer", consumerName,
		"filter_subject", subject,
		"port", portName)

	cfg := natsclient.StreamConsumerConfig{
		StreamName:    streamName,
		ConsumerName:  consumerName,
		FilterSubject: subject,
		DeliverPolicy: "new",
		AckPolicy:     "explicit",
		MaxDeliver:    3,
		AutoCreate:    false,
	}

	err := c.natsClient.ConsumeStreamWithConfig(ctx, cfg, func(msgCtx context.Context, msg jetstream.Msg) {
		handler(msgCtx, msg.Data())
		if ackErr := msg.Ack(); ackErr != nil {
			c.logger.Error("Failed to ack JetStream message", "error", ackErr)
		}
	})
	if err != nil {
		return fmt.Errorf("consumer setup failed for stream %s: %w", streamName, err)
	}

	c.logger.Info("Subscribed (JetStream)",
		"subject", subject,
		"stream", streamName,
		"consumer", consumerName,
		"port", portName)
	return nil
}

// waitForStream waits for a JetStream stream to be available
func (c *Component) waitForStream(ctx context.Context, streamName string) error {
	js, err := c.natsClient.JetStream()
	if err != nil {
		return fmt.Errorf("failed to get JetStream context: %w", err)
	}

	maxRetries := 30
	retryInterval := 100 * time.Millisecond
	maxInterval := 2 * time.Second

	for i := 0; i < maxRetries; i++ {
		_, err := js.Stream(ctx, streamName)
		if err == nil {
			return nil
		}
		if i < maxRetries-1 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(retryInterval):
				retryInterval = min(retryInterval*2, maxInterval)
			}
		}
	}

	return fmt.Errorf("stream %s not found after %d retries", streamName, maxRetries)
}

// sanitizeSubject converts a subject pattern to a valid consumer name suffix
func sanitizeSubject(subject string) string {
	s := strings.ReplaceAll(subject, ".", "-")
	s = strings.ReplaceAll(s, ">", "all")
	s = strings.ReplaceAll(s, "*", "any")
	return s
}

// Stop gracefully stops the component within the given timeout
func (c *Component) Stop(_ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return nil
	}

	// Cleanup operations would happen here in integration mode
	// For now, just mark as stopped
	c.running = false
	return nil
}

// Discoverable interface implementation

// Meta returns component metadata
func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        "agentic-memory",
		Type:        "processor",
		Description: "Graph-backed agent memory with context hydration and fact extraction",
		Version:     "0.1.0",
	}
}

// InputPorts returns configured input port definitions
func (c *Component) InputPorts() []component.Port {
	if c.config.Ports == nil {
		return []component.Port{}
	}

	ports := make([]component.Port, len(c.config.Ports.Inputs))
	for i, portDef := range c.config.Ports.Inputs {
		ports[i] = buildInputPort(portDef)
	}
	return ports
}

// OutputPorts returns configured output port definitions
func (c *Component) OutputPorts() []component.Port {
	if c.config.Ports == nil {
		return []component.Port{}
	}

	ports := make([]component.Port, len(c.config.Ports.Outputs))
	for i, portDef := range c.config.Ports.Outputs {
		ports[i] = buildOutputPort(portDef)
	}
	return ports
}

// buildInputPort creates a Port from a PortDefinition for input
func buildInputPort(portDef component.PortDefinition) component.Port {
	port := component.Port{
		Name:        portDef.Name,
		Direction:   component.DirectionInput,
		Required:    portDef.Required,
		Description: portDef.Description,
	}

	// Create appropriate port config based on type
	switch portDef.Type {
	case "jetstream":
		port.Config = component.JetStreamPort{
			StreamName: portDef.StreamName,
			Subjects:   []string{portDef.Subject},
		}
	case "kv-watch", "kvwatch":
		port.Config = component.KVWatchPort{
			Bucket: portDef.Bucket,
		}
	default:
		port.Config = component.NATSPort{
			Subject: portDef.Subject,
		}
	}

	return port
}

// buildOutputPort creates a Port from a PortDefinition for output
func buildOutputPort(portDef component.PortDefinition) component.Port {
	port := component.Port{
		Name:        portDef.Name,
		Direction:   component.DirectionOutput,
		Required:    portDef.Required,
		Description: portDef.Description,
	}

	// Create appropriate port config based on type
	switch portDef.Type {
	case "jetstream":
		port.Config = component.JetStreamPort{
			StreamName: portDef.StreamName,
			Subjects:   []string{portDef.Subject},
		}
	default:
		port.Config = component.NATSPort{
			Subject: portDef.Subject,
		}
	}

	return port
}

// ConfigSchema returns the configuration schema
func (c *Component) ConfigSchema() component.ConfigSchema {
	return agenticMemorySchema
}

// Health returns the current health status
func (c *Component) Health() component.HealthStatus {
	// Read atomic counters without lock
	errors := atomic.LoadInt64(&c.errors)

	// Lock only for non-atomic fields
	c.mu.RLock()
	running := c.running
	startTime := c.startTime
	status := c.getStatus()
	c.mu.RUnlock()

	return component.HealthStatus{
		Healthy:    running,
		LastCheck:  time.Now(),
		ErrorCount: int(errors),
		Uptime:     time.Since(startTime),
		Status:     status,
	}
}

// getStatus returns a status string
func (c *Component) getStatus() string {
	if c.running {
		return "running"
	}
	return "stopped"
}

// DataFlow returns current data flow metrics
func (c *Component) DataFlow() component.FlowMetrics {
	// Read atomic counters without lock
	eventsProcessed := atomic.LoadInt64(&c.eventsProcessed)
	errors := atomic.LoadInt64(&c.errors)

	// Lock only for non-atomic fields
	c.mu.RLock()
	lastActivity := c.lastActivity
	c.mu.RUnlock()

	var errorRate float64
	total := eventsProcessed + errors
	if total > 0 {
		errorRate = float64(errors) / float64(total)
	}

	return component.FlowMetrics{
		MessagesPerSecond: 0,
		BytesPerSecond:    0,
		ErrorRate:         errorRate,
		LastActivity:      lastActivity,
	}
}
