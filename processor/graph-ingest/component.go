// Package graphingest provides the graph-ingest component for entity and triple ingestion.
package graphingest

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/c360/semstreams/component"
	"github.com/c360/semstreams/graph"
	"github.com/c360/semstreams/graph/inference"
	"github.com/c360/semstreams/message"
	"github.com/c360/semstreams/metric"
	"github.com/c360/semstreams/natsclient"
	"github.com/c360/semstreams/pkg/errs"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus"
)

// Ensure Component implements required interfaces
var (
	_ component.Discoverable       = (*Component)(nil)
	_ component.LifecycleComponent = (*Component)(nil)
)

// Package-level prometheus metric (registered once to avoid duplicate registration errors)
var (
	metricsOnce         sync.Once
	entitiesUpdatedOnce prometheus.Counter
)

// entityIDRegex validates entity ID format: org.platform.domain.system.type.instance
// Example: c360.ops.robotics.gcs.drone.001 or c360.logistics.environmental.sensor.humidity.humid-sensor-001
// Each part must start with alphanumeric and can contain alphanumeric, hyphens, or underscores
var entityIDRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*\.[a-zA-Z0-9][a-zA-Z0-9_-]*\.[a-zA-Z0-9][a-zA-Z0-9_-]*\.[a-zA-Z0-9][a-zA-Z0-9_-]*\.[a-zA-Z0-9][a-zA-Z0-9_-]*\.[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

func getEntitiesUpdatedMetric(registry *metric.MetricsRegistry) prometheus.Counter {
	metricsOnce.Do(func() {
		entitiesUpdatedOnce = prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "semstreams",
			Subsystem: "datamanager",
			Name:      "entities_updated_total",
			Help:      "Total entities updated",
		})
		// Register with the metrics registry if available
		if registry != nil {
			_ = registry.RegisterCounter("graph-ingest", "entities_updated_total", entitiesUpdatedOnce)
		} else {
			// Fallback to default prometheus registry for testing
			// Ignore error if already registered (can happen across tests)
			_ = prometheus.DefaultRegisterer.Register(entitiesUpdatedOnce)
		}
	})
	return entitiesUpdatedOnce
}

// Config holds configuration for graph-ingest component
type Config struct {
	Ports              *component.PortConfig `json:"ports" schema:"type:ports,description:Port configuration,category:basic"`
	EnableHierarchy    bool                  `json:"enable_hierarchy" schema:"type:bool,description:Enable hierarchy inference,category:advanced"`
	EnableTypeSiblings *bool                 `json:"enable_type_siblings" schema:"type:bool,description:Enable sibling edges between same-type entities (default true when hierarchy enabled),category:advanced"`
}

// Validate implements component.Validatable interface
func (c *Config) Validate() error {
	if c.Ports == nil {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "ports configuration required")
	}
	if len(c.Ports.Inputs) == 0 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "at least one input port required")
	}
	if len(c.Ports.Outputs) == 0 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "at least one output port required")
	}
	return nil
}

// ApplyDefaults sets default values for configuration
func (c *Config) ApplyDefaults() {
	// EnableHierarchy defaults to false
	if c.Ports == nil {
		c.Ports = &component.PortConfig{}
	}
}

// DefaultConfig returns a valid default configuration
func DefaultConfig() Config {
	return Config{
		Ports: &component.PortConfig{
			Inputs: []component.PortDefinition{
				{
					Name:    "entity_stream",
					Type:    "jetstream",
					Subject: "entity.>",
				},
			},
			Outputs: []component.PortDefinition{
				{
					Name:    "entity_states",
					Type:    "kv-write",
					Subject: graph.BucketEntityStates,
				},
			},
		},
		EnableHierarchy: false,
	}
}

// schema defines the configuration schema for graph-ingest component
var schema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// entityManagerAdapter adapts Component to implement inference.EntityManager interface
type entityManagerAdapter struct {
	component *Component
}

func (a *entityManagerAdapter) ExistsEntity(ctx context.Context, id string) (bool, error) {
	entry, err := a.component.entityBucket.Get(ctx, id)
	if err != nil {
		if err == jetstream.ErrKeyNotFound {
			return false, nil
		}
		return false, err
	}
	return entry != nil, nil
}

func (a *entityManagerAdapter) CreateEntity(ctx context.Context, entity *graph.EntityState) (*graph.EntityState, error) {
	err := a.component.CreateEntity(ctx, entity)
	if err != nil {
		return nil, err
	}
	return entity, nil
}

func (a *entityManagerAdapter) ListWithPrefix(ctx context.Context, prefix string) ([]string, error) {
	// Get all keys from KV bucket
	keys, err := a.component.entityBucket.Keys(ctx)
	if err != nil {
		if err == jetstream.ErrNoKeysFound {
			return nil, nil
		}
		return nil, err
	}

	// Filter by prefix (prefix + "." to ensure we match the exact level)
	var matched []string
	prefixDot := prefix + "."

	for _, key := range keys {
		if strings.HasPrefix(key, prefixDot) {
			matched = append(matched, key)
		}
	}

	return matched, nil
}

// tripleAdderAdapter adapts Component to implement inference.TripleAdder interface
type tripleAdderAdapter struct {
	component *Component
}

func (a *tripleAdderAdapter) AddTriple(ctx context.Context, triple message.Triple) error {
	return a.component.AddTriple(ctx, triple)
}

// Component implements the graph-ingest processor
type Component struct {
	// Component metadata
	name   string
	config Config

	// Dependencies
	natsClient *natsclient.Client
	logger     *slog.Logger

	// Domain resources
	entityBucket jetstream.KeyValue

	// Inference components
	hierarchyInference *inference.HierarchyInference

	// Lifecycle state
	mu          sync.RWMutex
	running     bool
	initialized bool
	startTime   time.Time
	wg          sync.WaitGroup
	cancel      context.CancelFunc

	// Metrics (atomic)
	messagesProcessed int64
	bytesProcessed    int64
	errors            int64
	lastActivity      atomic.Value // stores time.Time

	// Prometheus metrics (for e2e test compatibility with datamanager metrics)
	entitiesUpdated prometheus.Counter

	// Lifecycle reporting
	lifecycleReporter component.LifecycleReporter

	// Port definitions
	inputPorts  []component.Port
	outputPorts []component.Port
}

// CreateGraphIngest is the factory function for creating graph-ingest components
func CreateGraphIngest(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	// Validate dependencies
	if deps.NATSClient == nil {
		return nil, errs.WrapInvalid(errs.ErrInvalidConfig, "CreateGraphIngest", "factory", "NATSClient required")
	}
	natsClient := deps.NATSClient

	// Parse configuration
	var config Config
	if len(rawConfig) > 0 {
		if err := json.Unmarshal(rawConfig, &config); err != nil {
			return nil, errs.Wrap(err, "CreateGraphIngest", "factory", "config unmarshal")
		}
	} else {
		config = DefaultConfig()
	}

	// Apply defaults and validate
	config.ApplyDefaults()
	if err := config.Validate(); err != nil {
		return nil, errs.Wrap(err, "CreateGraphIngest", "factory", "config validation")
	}

	// Create logger with component context
	logger := deps.GetLoggerWithComponent("graph-ingest")

	// Create component
	comp := &Component{
		name:            "graph-ingest",
		config:          config,
		natsClient:      natsClient,
		logger:          logger,
		entitiesUpdated: getEntitiesUpdatedMetric(deps.MetricsRegistry),
	}

	// Initialize last activity
	comp.lastActivity.Store(time.Now())

	return comp, nil
}

// Register registers the graph-ingest factory with the component registry
func Register(registry *component.Registry) error {
	return registry.RegisterFactory("graph-ingest", &component.Registration{
		Name:        "graph-ingest",
		Type:        "processor",
		Protocol:    "nats",
		Domain:      "graph",
		Description: "Entity and triple ingestion processor",
		Version:     "1.0.0",
		Schema:      schema,
		Factory:     CreateGraphIngest,
	})
}

// ============================================================================
// Discoverable Interface (6 methods)
// ============================================================================

// Meta returns component metadata
func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        "graph-ingest",
		Type:        "processor",
		Description: "Entity and triple ingestion processor for graph system",
		Version:     "1.0.0",
	}
}

// InputPorts returns input port definitions
func (c *Component) InputPorts() []component.Port {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.inputPorts
}

// OutputPorts returns output port definitions
func (c *Component) OutputPorts() []component.Port {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.outputPorts
}

// ConfigSchema returns the configuration schema
func (c *Component) ConfigSchema() component.ConfigSchema {
	return schema
}

// Health returns current health status
func (c *Component) Health() component.HealthStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()

	uptime := time.Duration(0)
	if c.running && !c.startTime.IsZero() {
		uptime = time.Since(c.startTime)
	}

	errorCount := int(atomic.LoadInt64(&c.errors))
	lastErr := ""
	status := "stopped"

	if c.running {
		status = "running"
		if errorCount > 0 {
			lastErr = "errors occurred during processing"
		}
	}

	return component.HealthStatus{
		Healthy:    c.running && errorCount == 0,
		LastCheck:  time.Now(),
		ErrorCount: errorCount,
		LastError:  lastErr,
		Uptime:     uptime,
		Status:     status,
	}
}

// DataFlow returns current data flow metrics
func (c *Component) DataFlow() component.FlowMetrics {
	messages := atomic.LoadInt64(&c.messagesProcessed)
	bytes := atomic.LoadInt64(&c.bytesProcessed)
	errorCount := atomic.LoadInt64(&c.errors)

	c.mu.RLock()
	uptime := time.Duration(0)
	if c.running && !c.startTime.IsZero() {
		uptime = time.Since(c.startTime)
	}
	c.mu.RUnlock()

	// Calculate rates
	var messagesPerSec, bytesPerSec, errorRate float64
	if uptime > 0 {
		seconds := uptime.Seconds()
		messagesPerSec = float64(messages) / seconds
		bytesPerSec = float64(bytes) / seconds
		if messages > 0 {
			errorRate = float64(errorCount) / float64(messages)
		}
	}

	lastAct := time.Now()
	if stored := c.lastActivity.Load(); stored != nil {
		if t, ok := stored.(time.Time); ok {
			lastAct = t
		}
	}

	return component.FlowMetrics{
		MessagesPerSecond: messagesPerSec,
		BytesPerSecond:    bytesPerSec,
		ErrorRate:         errorRate,
		LastActivity:      lastAct,
	}
}

// ============================================================================
// LifecycleComponent Interface (3 methods)
// ============================================================================

// Initialize validates configuration and sets up ports (no I/O)
func (c *Component) Initialize() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.initialized {
		return nil // Idempotent
	}

	// Validate configuration
	if err := c.config.Validate(); err != nil {
		return errs.Wrap(err, "Component", "Initialize", "config validation")
	}

	// Build input ports from config
	c.inputPorts = make([]component.Port, len(c.config.Ports.Inputs))
	for i, portDef := range c.config.Ports.Inputs {
		c.inputPorts[i] = component.BuildPortFromDefinition(portDef, component.DirectionInput)
	}

	// Build output ports from config
	c.outputPorts = make([]component.Port, len(c.config.Ports.Outputs))
	for i, portDef := range c.config.Ports.Outputs {
		c.outputPorts[i] = component.BuildPortFromDefinition(portDef, component.DirectionOutput)
	}

	c.initialized = true
	c.logger.Info("component initialized", slog.String("component", "graph-ingest"))

	return nil
}

// Start begins processing (must be initialized first)
func (c *Component) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check initialization
	if !c.initialized {
		return errs.WrapFatal(fmt.Errorf("component not initialized"), "Component", "Start", "initialization check")
	}

	// Idempotent - already running
	if c.running {
		return nil
	}

	// Create cancellable context
	ctx, cancel := context.WithCancel(ctx)
	c.cancel = cancel

	// Check context before proceeding
	if err := ctx.Err(); err != nil {
		cancel()
		return errs.Wrap(err, "Component", "Start", "context cancelled")
	}

	// Ensure NATS client is connected
	if c.natsClient.Status() != natsclient.StatusConnected {
		if err := c.natsClient.Connect(ctx); err != nil {
			cancel()
			// Check if this is a context-related error
			if ctx.Err() != nil {
				return errs.Wrap(ctx.Err(), "Component", "Start", "context cancelled during NATS connection")
			}
			return errs.Wrap(err, "Component", "Start", "NATS connection failed")
		}
		if err := c.natsClient.WaitForConnection(ctx); err != nil {
			cancel()
			if ctx.Err() != nil {
				return errs.Wrap(ctx.Err(), "Component", "Start", "context cancelled waiting for NATS")
			}
			return errs.Wrap(err, "Component", "Start", "wait for NATS connection")
		}
	}

	// Initialize KV bucket (create if not exists) - we are the WRITER
	bucket, err := c.natsClient.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{
		Bucket:      graph.BucketEntityStates,
		Description: "Entity state storage for graph-ingest",
	})
	if err != nil {
		cancel()
		return errs.Wrap(err, "Component", "Start", "KV bucket creation")
	}
	c.entityBucket = bucket

	// Initialize lifecycle reporter (throttled for high-throughput ingestion)
	c.initLifecycleReporter(ctx)

	// Initialize hierarchy inference if enabled (synchronous - no Start/Stop)
	c.initHierarchyInference()

	// Set up subscriptions for input ports
	if err := c.setupSubscriptions(ctx); err != nil {
		cancel()
		return errs.Wrap(err, "Component", "Start", "subscription setup")
	}

	// Set up query handler subscriptions
	if err := c.setupQueryHandlers(ctx); err != nil {
		cancel()
		return errs.Wrap(err, "Component", "Start", "query handler setup")
	}

	// Set up mutation handler subscriptions (for rule processor actions)
	if err := c.setupMutationHandlers(ctx); err != nil {
		cancel()
		return errs.Wrap(err, "Component", "Start", "mutation handler setup")
	}

	// Mark as running
	c.running = true
	c.startTime = time.Now()

	// Report initial idle state
	if err := c.lifecycleReporter.ReportStage(ctx, "idle"); err != nil {
		c.logger.Debug("failed to report lifecycle stage", slog.String("stage", "idle"), slog.Any("error", err))
	}

	c.logger.Info("component started",
		slog.String("component", "graph-ingest"),
		slog.Time("start_time", c.startTime))

	return nil
}

// Stop gracefully shuts down the component
func (c *Component) Stop(timeout time.Duration) error {
	c.mu.Lock()

	if !c.running {
		c.mu.Unlock()
		return nil // Already stopped
	}

	// Cancel context
	if c.cancel != nil {
		c.cancel()
	}

	c.running = false
	c.mu.Unlock()

	// Wait for goroutines with timeout
	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		c.logger.Info("component stopped gracefully", slog.String("component", "graph-ingest"))
		return nil
	case <-time.After(timeout):
		c.logger.Warn("component stop timed out", slog.String("component", "graph-ingest"))
		return fmt.Errorf("stop timeout after %v", timeout)
	}
}

// initLifecycleReporter initializes the lifecycle reporter for component status tracking.
func (c *Component) initLifecycleReporter(ctx context.Context) {
	statusBucket, err := c.natsClient.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{
		Bucket:      "COMPONENT_STATUS",
		Description: "Component lifecycle status tracking",
	})
	if err != nil {
		c.logger.Warn("Failed to create COMPONENT_STATUS bucket, lifecycle reporting disabled",
			slog.Any("error", err))
		c.lifecycleReporter = component.NewNoOpLifecycleReporter()
		return
	}
	c.lifecycleReporter = component.NewLifecycleReporterFromConfig(component.LifecycleReporterConfig{
		KV:               statusBucket,
		ComponentName:    "graph-ingest",
		Logger:           c.logger,
		EnableThrottling: true,
	})
}

// initHierarchyInference initializes hierarchy inference if enabled.
func (c *Component) initHierarchyInference() {
	if !c.config.EnableHierarchy {
		return
	}

	// Enable sibling edges by default, can be disabled via config
	enableTypeSiblings := true
	if c.config.EnableTypeSiblings != nil {
		enableTypeSiblings = *c.config.EnableTypeSiblings
	}

	hierarchyConfig := inference.HierarchyConfig{
		Enabled:            true,
		CreateTypeEdges:    true,
		CreateSystemEdges:  true,
		CreateDomainEdges:  true,
		CreateTypeSiblings: enableTypeSiblings,
	}

	c.hierarchyInference = inference.NewHierarchyInference(
		&entityManagerAdapter{component: c},
		&tripleAdderAdapter{component: c},
		hierarchyConfig,
		c.logger,
	)
}

// ============================================================================
// Subscription Management
// ============================================================================

// setupSubscriptions sets up JetStream consumers for input ports
func (c *Component) setupSubscriptions(ctx context.Context) error {
	for _, port := range c.config.Ports.Inputs {
		if port.Type != "jetstream" {
			c.logger.Debug("skipping non-jetstream port", slog.String("port", port.Name), slog.String("type", port.Type))
			continue
		}

		if err := c.setupJetStreamConsumer(ctx, port); err != nil {
			return errs.Wrap(err, "Component", "setupSubscriptions",
				fmt.Sprintf("JetStream consumer for %s", port.Subject))
		}
	}
	return nil
}

// setupJetStreamConsumer creates a JetStream consumer for an input port
func (c *Component) setupJetStreamConsumer(ctx context.Context, port component.PortDefinition) error {
	// Derive stream name from subject
	streamName := port.StreamName
	if streamName == "" {
		streamName = c.deriveStreamName(port.Subject)
	}
	if streamName == "" {
		return fmt.Errorf("could not derive stream name for subject %s", port.Subject)
	}

	// Wait for stream to be available
	if err := c.waitForStream(ctx, streamName); err != nil {
		return fmt.Errorf("stream %s not available: %w", streamName, err)
	}

	// Generate unique consumer name
	sanitizedSubject := strings.ReplaceAll(port.Subject, ".", "-")
	sanitizedSubject = strings.ReplaceAll(sanitizedSubject, "*", "all")
	sanitizedSubject = strings.ReplaceAll(sanitizedSubject, ">", "wildcard")
	consumerName := fmt.Sprintf("graph-ingest-%s", sanitizedSubject)

	c.logger.Info("Setting up JetStream consumer",
		slog.String("stream", streamName),
		slog.String("consumer", consumerName),
		slog.String("filter_subject", port.Subject))

	cfg := natsclient.StreamConsumerConfig{
		StreamName:    streamName,
		ConsumerName:  consumerName,
		FilterSubject: port.Subject,
		DeliverPolicy: "all",
		AckPolicy:     "explicit",
		MaxDeliver:    5,
		AutoCreate:    false,
	}

	subject := port.Subject // capture for closure
	err := c.natsClient.ConsumeStreamWithConfig(ctx, cfg, func(msgCtx context.Context, msg jetstream.Msg) {
		c.handleMessage(msgCtx, subject, msg.Data())
		if ackErr := msg.Ack(); ackErr != nil {
			c.logger.Error("Failed to ack JetStream message", slog.Any("error", ackErr))
		}
	})
	if err != nil {
		return fmt.Errorf("consumer setup failed for stream %s: %w", streamName, err)
	}

	c.logger.Info("graph-ingest subscribed (JetStream)",
		slog.String("subject", subject),
		slog.String("stream", streamName))
	return nil
}

// deriveStreamName derives a stream name from a subject pattern
func (c *Component) deriveStreamName(subject string) string {
	// Common mappings based on subject prefix
	prefixToStream := map[string]string{
		"sensor.":      "SENSOR",
		"objectstore.": "OBJECTSTORE",
		"entity.":      "ENTITY",
		"events.":      "EVENTS",
	}

	for prefix, stream := range prefixToStream {
		if strings.HasPrefix(subject, prefix) {
			return stream
		}
	}

	// Default: use first segment uppercased
	parts := strings.Split(subject, ".")
	if len(parts) > 0 {
		return strings.ToUpper(parts[0])
	}
	return ""
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
			c.logger.Debug("Stream available", slog.String("stream", streamName))
			return nil
		}

		// Exponential backoff
		c.logger.Debug("Waiting for stream",
			slog.String("stream", streamName),
			slog.Int("attempt", i+1),
			slog.Duration("interval", retryInterval))

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(retryInterval):
			retryInterval = time.Duration(float64(retryInterval) * 1.5)
			if retryInterval > maxInterval {
				retryInterval = maxInterval
			}
		}
	}

	return fmt.Errorf("stream %s not available after %d retries", streamName, maxRetries)
}

// handleMessage processes an incoming message and creates/updates entity state
func (c *Component) handleMessage(ctx context.Context, subject string, data []byte) {
	// Report processing stage (throttled to avoid KV spam)
	if err := c.lifecycleReporter.ReportStage(ctx, "processing"); err != nil {
		c.logger.Debug("failed to report lifecycle stage", slog.String("stage", "processing"), slog.Any("error", err))
	}

	c.logger.Debug("Received message",
		slog.String("subject", subject),
		slog.Int("size", len(data)))

	// Try to unmarshal as a BaseMessage containing a Graphable payload
	var baseMsg message.BaseMessage
	if err := json.Unmarshal(data, &baseMsg); err != nil {
		c.logger.Warn("Failed to unmarshal base message",
			slog.String("subject", subject),
			slog.Any("error", err))
		atomic.AddInt64(&c.errors, 1)
		return
	}

	// Extract entity from BaseMessage payload
	entity, err := c.extractEntityFromMessage(&baseMsg)
	if err != nil {
		c.logger.Warn("Failed to extract entity from message",
			slog.String("subject", subject),
			slog.Any("error", err))
		atomic.AddInt64(&c.errors, 1)
		return
	}

	// Store entity in KV bucket
	if err := c.CreateEntity(ctx, entity); err != nil {
		c.logger.Error("Failed to create entity",
			slog.String("entity_id", entity.ID),
			slog.Any("error", err))
		return
	}

	c.logger.Debug("Entity ingested",
		slog.String("entity_id", entity.ID),
		slog.Int("triples", len(entity.Triples)))
}

// extractEntityFromMessage extracts an EntityState from a BaseMessage
func (c *Component) extractEntityFromMessage(msg *message.BaseMessage) (*graph.EntityState, error) {
	if msg == nil {
		return nil, fmt.Errorf("nil message")
	}

	payload := msg.Payload()
	if payload == nil {
		return nil, fmt.Errorf("message has no payload")
	}

	// Check if payload implements Graphable
	graphable, ok := payload.(graph.Graphable)
	if !ok {
		return nil, fmt.Errorf("payload does not implement Graphable interface")
	}

	// Get entity ID and triples from Graphable
	entityID := graphable.EntityID()
	if entityID == "" {
		return nil, fmt.Errorf("graphable payload returned empty entity ID")
	}

	triples := graphable.Triples()

	// Build EntityState
	entity := &graph.EntityState{
		ID:          entityID,
		Triples:     triples,
		MessageType: msg.Type(),
		Version:     1,
	}

	return entity, nil
}

// ============================================================================
// Entity Operations
// ============================================================================

// validateEntityID validates that an entity ID follows the expected format
func validateEntityID(id string) error {
	if id == "" {
		return errs.WrapInvalid(errs.ErrInvalidData, "Component", "validateEntityID", "entity ID cannot be empty")
	}

	if len(id) > 255 {
		return errs.WrapInvalid(errs.ErrInvalidData, "Component", "validateEntityID", "entity ID too long (max 255 chars)")
	}

	if !entityIDRegex.MatchString(id) {
		parts := strings.Split(id, ".")
		msg := fmt.Sprintf(
			"invalid entity ID format: expected 6 ASCII alphanumeric parts (org.platform.domain.system.type.instance), got %d parts or non-ASCII characters",
			len(parts))
		return errs.WrapInvalid(errs.ErrInvalidData, "Component", "validateEntityID", msg)
	}

	return nil
}

// CreateEntity creates a new entity in the graph
func (c *Component) CreateEntity(ctx context.Context, entity *graph.EntityState) error {
	if entity == nil {
		return errs.WrapInvalid(errs.ErrInvalidData, "Component", "CreateEntity", "entity cannot be nil")
	}

	// Validate entity ID format
	if err := validateEntityID(entity.ID); err != nil {
		return err
	}

	// Check context
	if err := ctx.Err(); err != nil {
		return errs.Wrap(err, "Component", "CreateEntity", "context cancelled")
	}

	// SYNCHRONOUS HIERARCHY INFERENCE:
	// Get hierarchy triples BEFORE writing entity to storage
	// This ensures entity is written once with all triples included (no cascade)
	if c.config.EnableHierarchy && c.hierarchyInference != nil {
		hierarchyTriples, err := c.hierarchyInference.GetHierarchyTriples(ctx, entity.ID)
		if err != nil {
			c.logger.Warn("Failed to get hierarchy triples",
				slog.String("entity_id", entity.ID),
				slog.Any("error", err))
			// Don't fail entity creation if hierarchy fails - just log warning
		} else if len(hierarchyTriples) > 0 {
			// Add hierarchy triples to entity before writing
			entity.Triples = append(entity.Triples, hierarchyTriples...)
		}
	}

	// Serialize entity (now includes hierarchy triples if enabled)
	data, err := json.Marshal(entity)
	if err != nil {
		atomic.AddInt64(&c.errors, 1)
		return errs.Wrap(err, "Component", "CreateEntity", "entity serialization")
	}

	// Store in KV bucket (single write with all triples)
	if _, err := c.entityBucket.Put(ctx, entity.ID, data); err != nil {
		atomic.AddInt64(&c.errors, 1)
		return errs.Wrap(err, "Component", "CreateEntity", "KV store")
	}

	// Update metrics
	atomic.AddInt64(&c.messagesProcessed, 1)
	atomic.AddInt64(&c.bytesProcessed, int64(len(data)))
	c.lastActivity.Store(time.Now())
	c.entitiesUpdated.Inc()

	c.logger.Debug("entity created",
		slog.String("entity_id", entity.ID),
		slog.Int("triples", len(entity.Triples)))

	// Ensure referenced entities exist (fallback for referential integrity)
	// This creates stub entities for any entity IDs referenced in relationship triples
	// that don't already exist, guaranteeing graph consistency.
	for _, triple := range entity.Triples {
		if triple.IsRelationship() {
			targetID, ok := triple.Object.(string)
			if ok && targetID != "" && targetID != entity.ID {
				if err := c.ensureReferencedEntityExists(ctx, targetID, entity.ID); err != nil {
					c.logger.Debug("failed to ensure referenced entity exists",
						slog.String("target", targetID),
						slog.String("referenced_by", entity.ID),
						slog.Any("error", err))
					// Don't fail entity creation - this is a best-effort fallback
				}
			}
		}
	}

	return nil
}

// ensureReferencedEntityExists creates a stub entity if the referenced entity doesn't exist.
// This is a fallback mechanism to guarantee referential integrity - if an entity references
// another entity by ID, that entity must exist in the graph.
func (c *Component) ensureReferencedEntityExists(ctx context.Context, entityID, referencedBy string) error {
	// Check if entity already exists
	_, err := c.entityBucket.Get(ctx, entityID)
	if err == nil {
		return nil // Entity exists, nothing to do
	}

	// Entity doesn't exist - create a stub
	now := time.Now()
	stub := &graph.EntityState{
		ID:        entityID,
		UpdatedAt: now,
		Triples: []message.Triple{
			{
				Subject:    entityID,
				Predicate:  "core.identity.stub",
				Object:     true,
				Source:     "graph-ingest-referential-integrity",
				Timestamp:  now,
				Confidence: 1.0,
			},
			{
				Subject:    entityID,
				Predicate:  "core.identity.referenced_by",
				Object:     referencedBy,
				Source:     "graph-ingest-referential-integrity",
				Timestamp:  now,
				Confidence: 1.0,
			},
		},
	}

	data, err := json.Marshal(stub)
	if err != nil {
		return fmt.Errorf("marshal stub entity: %w", err)
	}

	if _, err := c.entityBucket.Put(ctx, entityID, data); err != nil {
		return fmt.Errorf("store stub entity: %w", err)
	}

	c.logger.Debug("created stub entity for referential integrity",
		slog.String("entity_id", entityID),
		slog.String("referenced_by", referencedBy))

	return nil
}

// UpdateEntity updates an existing entity
func (c *Component) UpdateEntity(ctx context.Context, entity *graph.EntityState) error {
	if entity == nil {
		return errs.WrapInvalid(errs.ErrInvalidData, "Component", "UpdateEntity", "entity cannot be nil")
	}

	// Validate entity ID format
	if err := validateEntityID(entity.ID); err != nil {
		return err
	}

	// Check context
	if err := ctx.Err(); err != nil {
		return errs.Wrap(err, "Component", "UpdateEntity", "context cancelled")
	}

	// Serialize entity
	data, err := json.Marshal(entity)
	if err != nil {
		atomic.AddInt64(&c.errors, 1)
		return errs.Wrap(err, "Component", "UpdateEntity", "entity serialization")
	}

	// Update in KV bucket
	if _, err := c.entityBucket.Put(ctx, entity.ID, data); err != nil {
		atomic.AddInt64(&c.errors, 1)
		return errs.Wrap(err, "Component", "UpdateEntity", "KV store")
	}

	// Update metrics
	atomic.AddInt64(&c.messagesProcessed, 1)
	atomic.AddInt64(&c.bytesProcessed, int64(len(data)))
	c.lastActivity.Store(time.Now())
	c.entitiesUpdated.Inc()

	c.logger.Debug("entity updated",
		slog.String("entity_id", entity.ID),
		slog.Uint64("version", entity.Version))

	return nil
}

// DeleteEntity removes an entity from the graph
func (c *Component) DeleteEntity(ctx context.Context, entityID string) error {
	// Validate entity ID format
	if err := validateEntityID(entityID); err != nil {
		return err
	}

	// Check context
	if err := ctx.Err(); err != nil {
		return errs.Wrap(err, "Component", "DeleteEntity", "context cancelled")
	}

	// Delete from KV bucket
	if err := c.entityBucket.Delete(ctx, entityID); err != nil {
		atomic.AddInt64(&c.errors, 1)
		return errs.Wrap(err, "Component", "DeleteEntity", "KV delete")
	}

	// Update metrics
	atomic.AddInt64(&c.messagesProcessed, 1)
	c.lastActivity.Store(time.Now())

	c.logger.Debug("entity deleted", slog.String("entity_id", entityID))

	return nil
}

// ============================================================================
// Triple Operations
// ============================================================================

// AddTriple adds a triple to an entity
func (c *Component) AddTriple(ctx context.Context, triple message.Triple) error {
	if triple.Subject == "" {
		return errs.WrapInvalid(errs.ErrInvalidData, "Component", "AddTriple", "triple subject cannot be empty")
	}
	if triple.Predicate == "" {
		return errs.WrapInvalid(errs.ErrInvalidData, "Component", "AddTriple", "triple predicate cannot be empty")
	}

	// Check context
	if err := ctx.Err(); err != nil {
		return errs.Wrap(err, "Component", "AddTriple", "context cancelled")
	}

	// Get existing entity
	entry, err := c.entityBucket.Get(ctx, triple.Subject)
	if err != nil {
		if err == jetstream.ErrKeyNotFound {
			// Create new entity with this triple
			entity := &graph.EntityState{
				ID:        triple.Subject,
				Triples:   []message.Triple{triple},
				Version:   1,
				UpdatedAt: time.Now(),
			}
			return c.CreateEntity(ctx, entity)
		}
		atomic.AddInt64(&c.errors, 1)
		return errs.Wrap(err, "Component", "AddTriple", "entity lookup")
	}

	// Deserialize existing entity
	var entity graph.EntityState
	if err := json.Unmarshal(entry.Value(), &entity); err != nil {
		atomic.AddInt64(&c.errors, 1)
		return errs.Wrap(err, "Component", "AddTriple", "entity deserialization")
	}

	// Add triple
	entity.Triples = append(entity.Triples, triple)
	entity.Version++
	entity.UpdatedAt = time.Now()

	// Update entity
	return c.UpdateEntity(ctx, &entity)
}

// RemoveTriple removes a triple from an entity
func (c *Component) RemoveTriple(ctx context.Context, subject, predicate string) error {
	if subject == "" {
		return errs.WrapInvalid(errs.ErrInvalidData, "Component", "RemoveTriple", "subject cannot be empty")
	}
	if predicate == "" {
		return errs.WrapInvalid(errs.ErrInvalidData, "Component", "RemoveTriple", "predicate cannot be empty")
	}

	// Check context
	if err := ctx.Err(); err != nil {
		return errs.Wrap(err, "Component", "RemoveTriple", "context cancelled")
	}

	// Get existing entity
	entry, err := c.entityBucket.Get(ctx, subject)
	if err != nil {
		if err == jetstream.ErrKeyNotFound {
			return nil // Entity doesn't exist, nothing to remove
		}
		atomic.AddInt64(&c.errors, 1)
		return errs.Wrap(err, "Component", "RemoveTriple", "entity lookup")
	}

	// Deserialize existing entity
	var entity graph.EntityState
	if err := json.Unmarshal(entry.Value(), &entity); err != nil {
		atomic.AddInt64(&c.errors, 1)
		return errs.Wrap(err, "Component", "RemoveTriple", "entity deserialization")
	}

	// Remove matching triples
	filtered := make([]message.Triple, 0, len(entity.Triples))
	for _, t := range entity.Triples {
		if t.Predicate != predicate {
			filtered = append(filtered, t)
		}
	}

	entity.Triples = filtered
	entity.Version++
	entity.UpdatedAt = time.Now()

	// Update entity
	return c.UpdateEntity(ctx, &entity)
}
