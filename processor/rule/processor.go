// Package rule provides a rule processing component that implements
// the Discoverable interface for processing message streams through rules
package rule

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/c360/semstreams/component"
	message "github.com/c360/semstreams/message"
	"github.com/c360/semstreams/metric"
	"github.com/c360/semstreams/natsclient"
	"github.com/c360/semstreams/pkg/cache"
	"github.com/c360/semstreams/pkg/errs"
	"github.com/nats-io/nats.go/jetstream"
)

// Static interface checks - compile-time verification
var _ component.Discoverable = (*Processor)(nil)

// schema defines the configuration schema for rule processor component
// Generated from Config struct tags using reflection
var schema = func() component.ConfigSchema {
	s := component.GenerateConfigSchema(reflect.TypeOf(Config{}))

	// Add "rules" property for runtime-only dynamic rule definitions
	// This field is not in Config struct since it's only used for runtime updates
	s.Properties["rules"] = component.PropertySchema{
		Type:        "object",
		Description: "Dynamic rule definitions (rules.{rule_id} pattern)",
		Default:     map[string]interface{}{},
		Category:    "advanced",
	}

	return s
}()

// RuleMetrics and newRuleMetrics are in metrics.go
// Config and DefaultConfig are in config.go

// Processor is a component that processes messages through rules
type Processor struct {
	// Component interface implementation
	metadata    component.Metadata
	inputPorts  []component.Port
	outputPorts []component.Port
	health      component.HealthStatus
	flowMetrics component.FlowMetrics

	// Rule processing resources
	natsClient      *natsclient.Client
	rules           map[string]Rule           // Self-loaded rules
	ruleDefinitions map[string]Definition     // Rule definitions for stateful evaluation
	ruleConfigs     map[string]map[string]any // Original rule configurations for GetRuntimeConfig

	// Message cache
	messageCache cache.Cache[message.Message]

	// Configuration
	config *Config

	// Dependencies
	metricsRegistry *metric.MetricsRegistry

	// Runtime state
	running           bool          // Tracks if processor is running (protected by mu)
	shutdown          chan struct{} // Closed to signal shutdown, never set to nil while running
	done              chan struct{}
	startTime         time.Time
	messagesEvaluated int64
	rulesTriggered    int64
	eventsPublished   int64 // New metric for event publishing
	errorCount        int64
	lastError         string
	lastActivity      time.Time
	mu                sync.RWMutex

	// Active subscriptions flag
	isSubscribed bool

	// JetStream consumer for entity events
	entityConsumer jetstream.Consumer

	// KV watchers for entity state changes
	entityWatchers []jetstream.KeyWatcher

	// Prometheus metrics
	metrics *Metrics

	// Stateful rule support
	stateTracker      *StateTracker
	statefulEvaluator *StatefulEvaluator

	// Logger
	logger *slog.Logger
}

// NewProcessor creates a new rule processor
func NewProcessor(natsClient *natsclient.Client, config *Config) (*Processor, error) {
	return NewProcessorWithMetrics(natsClient, config, nil)
}

// NewProcessorWithMetrics creates a new rule processor with optional metrics
func NewProcessorWithMetrics(natsClient *natsclient.Client, config *Config, metricsRegistry *metric.MetricsRegistry) (*Processor, error) {
	if config == nil {
		defaultConfig := DefaultConfig()
		config = &defaultConfig
	}

	// Validate required configuration
	if config.Ports == nil {
		return nil, fmt.Errorf("rule processor config missing required Ports configuration")
	}

	// Create message cache - will be initialized with context in Start()
	msgCache := cache.NewNoop[message.Message]()

	rp := &Processor{
		metadata: component.Metadata{
			Name:        "rule-processor",
			Type:        "processor",
			Description: "Processes messages through configurable rules and generates alerts",
			Version:     "1.0.0",
		},
		natsClient:      natsClient,
		rules:           make(map[string]Rule),
		ruleDefinitions: make(map[string]Definition),
		ruleConfigs:     make(map[string]map[string]any),
		messageCache:    msgCache,
		config:          config,
		metricsRegistry: metricsRegistry,
		entityWatchers:  make([]jetstream.KeyWatcher, 0),
		health: component.HealthStatus{
			Healthy:    true,
			LastCheck:  time.Now(),
			ErrorCount: 0,
			Uptime:     0,
		},
		flowMetrics: component.FlowMetrics{
			MessagesPerSecond: 0,
			BytesPerSecond:    0,
			ErrorRate:         0,
			LastActivity:      time.Now(),
		},
		isSubscribed: false,
		metrics:      newRuleMetrics(metricsRegistry, "rule"),
		logger:       slog.Default().With("component", "rule-processor"),
	}

	// Set up input and output ports
	rp.setupPorts()

	return rp, nil
}

// setupPorts initializes input and output port definitions.
// Ports configuration is validated in the constructor, so config.Ports is guaranteed non-nil.
func (rp *Processor) setupPorts() {
	rp.inputPorts = make([]component.Port, len(rp.config.Ports.Inputs))
	for i, portDef := range rp.config.Ports.Inputs {
		rp.inputPorts[i] = convertDefinitionToPort(portDef, component.DirectionInput)
	}

	rp.outputPorts = make([]component.Port, len(rp.config.Ports.Outputs))
	for i, portDef := range rp.config.Ports.Outputs {
		rp.outputPorts[i] = convertDefinitionToPort(portDef, component.DirectionOutput)
	}
}

// Meta returns component metadata
func (rp *Processor) Meta() component.Metadata {
	return rp.metadata
}

// InputPorts returns declared input ports
func (rp *Processor) InputPorts() []component.Port {
	return rp.inputPorts
}

// OutputPorts returns declared output ports
func (rp *Processor) OutputPorts() []component.Port {
	return rp.outputPorts
}

// ConfigSchema returns configuration schema for component interface
func (rp *Processor) ConfigSchema() component.ConfigSchema {
	return schema
}

// Health returns current health status
func (rp *Processor) Health() component.HealthStatus {
	rp.mu.RLock()
	defer rp.mu.RUnlock()

	rp.health.LastCheck = time.Now()
	rp.health.ErrorCount = int(atomic.LoadInt64(&rp.errorCount))
	if !rp.startTime.IsZero() {
		rp.health.Uptime = time.Since(rp.startTime)
	}

	return rp.health
}

// DataFlow returns current data flow metrics
func (rp *Processor) DataFlow() component.FlowMetrics {
	rp.mu.RLock()
	defer rp.mu.RUnlock()

	// Calculate messages per second based on recent activity
	evaluated := atomic.LoadInt64(&rp.messagesEvaluated)
	if !rp.startTime.IsZero() && evaluated > 0 {
		duration := time.Since(rp.startTime).Seconds()
		if duration > 0 {
			rp.flowMetrics.MessagesPerSecond = float64(evaluated) / duration
		}
	}

	// Error rate calculation
	if evaluated > 0 {
		rp.flowMetrics.ErrorRate = float64(atomic.LoadInt64(&rp.errorCount)) / float64(evaluated)
	}

	rp.flowMetrics.LastActivity = rp.lastActivity

	return rp.flowMetrics
}

// Initialize loads rules and prepares the processor
func (rp *Processor) Initialize() error {
	rp.mu.Lock()
	defer rp.mu.Unlock()

	// Load rules based on configuration
	if err := rp.loadRules(); err != nil {
		return errs.Wrap(err, "RuleProcessor", "initialize", "load rules")
	}

	rp.logger.Info("Rule processor initialized", "rule_count", len(rp.rules))
	return nil
}

// watchEntityStates and handleEntityUpdates are in entity_watcher.go
// loadRuleDefinitionsFromFiles and loadRules are in rule_loader.go

// run is the main background goroutine that handles processor lifecycle
func (rp *Processor) run(ctx context.Context) {
	defer close(rp.done)

	// Start KV watchers for entity state changes FIRST
	if err := rp.watchEntityStates(ctx); err != nil {
		rp.logger.Warn("Failed to start entity state watching", "error", err)
		// Don't fail - rules can still process semantic messages
	}

	// Subscribe to input subjects
	if err := rp.setupSubscriptions(ctx); err != nil {
		rp.logger.Error("Failed to setup subscriptions", "error", err)
		return
	}

	// NOW mark healthy - watchers established, subscriptions ready
	rp.mu.Lock()
	rp.health.Healthy = true
	rp.health.LastCheck = time.Now()
	rp.mu.Unlock()
	rp.logger.Info("Rule processor ready - watchers and subscriptions established")

	// Wait for shutdown signal or context cancellation
	select {
	case <-rp.shutdown:
		rp.logger.Info("Rule processor shutdown requested")
	case <-ctx.Done():
		rp.logger.Info("Rule processor context cancelled", "error", ctx.Err())
	}
}

// initializeStateTracker creates the RULE_STATE KV bucket and initializes state tracking components.
// This enables stateful ECA rules with OnEnter/OnExit/WhileTrue actions.
func (rp *Processor) initializeStateTracker(ctx context.Context) error {
	// Get or create the RULE_STATE KV bucket
	const bucketName = "RULE_STATE"

	js, err := rp.natsClient.JetStream()
	if err != nil {
		return fmt.Errorf("get JetStream context: %w", err)
	}

	// Try to get existing bucket first
	bucket, err := js.KeyValue(ctx, bucketName)
	if err != nil {
		// Bucket doesn't exist - create it
		kvConfig := jetstream.KeyValueConfig{
			Bucket:      bucketName,
			Description: "Rule match state tracking for stateful ECA rules",
			TTL:         0,  // No expiration by default
			MaxBytes:    -1, // No size limit
			History:     1,  // Keep only current state
		}

		bucket, err = js.CreateKeyValue(ctx, kvConfig)
		if err != nil {
			return fmt.Errorf("create RULE_STATE bucket: %w", err)
		}

		rp.logger.Info("Created RULE_STATE KV bucket for stateful rules")
	} else {
		rp.logger.Info("Using existing RULE_STATE KV bucket")
	}

	// Create StateTracker
	rp.stateTracker = NewStateTracker(bucket, rp.logger)

	// Create ActionExecutor
	actionExecutor := NewActionExecutor(rp.logger)

	// Create StatefulEvaluator
	rp.statefulEvaluator = NewStatefulEvaluator(rp.stateTracker, actionExecutor, rp.logger)

	rp.logger.Info("State tracker initialized for stateful ECA rules")
	return nil
}

// Start begins processing messages through rules
func (rp *Processor) Start(ctx context.Context) error {
	rp.mu.Lock()
	defer rp.mu.Unlock()

	if rp.running {
		return errs.WrapInvalid(errs.ErrAlreadyStarted, "RuleProcessor", "Start", "check processor state")
	}

	// Initialize message cache with context and metrics
	msgCache, err := cache.NewFromConfig[message.Message](ctx, rp.config.MessageCache,
		cache.WithMetrics[message.Message](rp.metricsRegistry, "rule_processor"),
	)
	if err != nil {
		rp.logger.Warn("Failed to create message cache, using noop cache", "error", err)
		msgCache = cache.NewNoop[message.Message]()
	}
	rp.messageCache = msgCache

	// Initialize StateTracker for stateful ECA rules
	if err := rp.initializeStateTracker(ctx); err != nil {
		rp.logger.Warn("Failed to initialize state tracker, stateful rules will be disabled", "error", err)
		// Don't fail - processor can still work with stateless rules
	}

	// Create shutdown and done channels for coordination
	rp.shutdown = make(chan struct{})
	rp.done = make(chan struct{})
	rp.running = true
	rp.startTime = time.Now()
	// Note: health.Healthy is set in run() after watchers and subscriptions are established

	// Start background goroutine with context
	go rp.run(ctx)

	rp.isSubscribed = true

	// Count subjects for logging
	subjectCount := 0
	for _, port := range rp.config.Ports.Inputs {
		if (port.Type == "nats" || port.Type == "jetstream") && port.Subject != "" {
			subjectCount++
		}
	}

	rp.logger.Info("Rule processor started", "subject_count", subjectCount)
	return nil
}

// setupSubscriptions creates subscriptions for input subjects based on port type
func (rp *Processor) setupSubscriptions(ctx context.Context) error {
	if !rp.natsClient.IsHealthy() {
		return errs.WrapFatal(errs.ErrNoConnection, "RuleProcessor", "Start", "check NATS health")
	}

	for _, port := range rp.config.Ports.Inputs {
		if port.Subject == "" {
			continue
		}

		// Skip entity.events subjects since we use KV watch for entity states
		if strings.HasPrefix(port.Subject, "events.graph.entity") {
			rp.logger.Info("Skipping subscription - using KV watch for entity states", "subject", port.Subject)
			continue
		}

		switch port.Type {
		case "jetstream":
			// JetStream subscription - use durable consumer
			if err := rp.setupJetStreamConsumer(ctx, port); err != nil {
				return errs.Wrap(err, "RuleProcessor", "setupSubscriptions",
					fmt.Sprintf("JetStream consumer for %s", port.Subject))
			}

		case "nats":
			// Core NATS subscription
			subject := port.Subject // capture for closure
			err := rp.natsClient.Subscribe(ctx, subject, func(msgCtx context.Context, data []byte) {
				rp.handleMessage(msgCtx, subject, data)
			})
			if err != nil {
				return errs.Wrap(err, "RuleProcessor", "Start", fmt.Sprintf("subscribe to %s", subject))
			}
			rp.logger.Info("Rule processor subscribed (NATS)", "subject", subject)

		default:
			rp.logger.Warn("Unknown port type, skipping", "port", port.Name, "type", port.Type)
		}
	}

	return nil
}

// setupJetStreamConsumer creates a JetStream consumer for an input port
func (rp *Processor) setupJetStreamConsumer(ctx context.Context, port component.PortDefinition) error {
	// Derive stream name from subject or use explicit stream name
	streamName := port.StreamName
	if streamName == "" {
		streamName = deriveStreamName(port.Subject)
	}
	if streamName == "" {
		return fmt.Errorf("could not derive stream name for subject %s", port.Subject)
	}

	// Wait for stream to be available
	if err := rp.waitForStream(ctx, streamName); err != nil {
		return fmt.Errorf("stream %s not available: %w", streamName, err)
	}

	// Generate unique consumer name
	sanitizedSubject := strings.ReplaceAll(port.Subject, ".", "-")
	sanitizedSubject = strings.ReplaceAll(sanitizedSubject, "*", "all")
	sanitizedSubject = strings.ReplaceAll(sanitizedSubject, ">", "wildcard")
	consumerName := fmt.Sprintf("rule-processor-%s", sanitizedSubject)

	rp.logger.Info("Setting up JetStream consumer",
		"stream", streamName,
		"consumer", consumerName,
		"filter_subject", port.Subject)

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
	err := rp.natsClient.ConsumeStreamWithConfig(ctx, cfg, func(msgCtx context.Context, msg jetstream.Msg) {
		rp.handleMessage(msgCtx, subject, msg.Data())
		if ackErr := msg.Ack(); ackErr != nil {
			rp.logger.Error("Failed to ack JetStream message", "error", ackErr)
		}
	})
	if err != nil {
		return fmt.Errorf("consumer setup failed for stream %s: %w", streamName, err)
	}

	rp.logger.Info("Rule processor subscribed (JetStream)", "subject", subject, "stream", streamName)
	return nil
}

// waitForStream waits for a JetStream stream to be available
func (rp *Processor) waitForStream(ctx context.Context, streamName string) error {
	js, err := rp.natsClient.JetStream()
	if err != nil {
		return fmt.Errorf("failed to get JetStream context: %w", err)
	}

	maxRetries := 30
	retryInterval := 100 * time.Millisecond
	maxInterval := 2 * time.Second

	for i := 0; i < maxRetries; i++ {
		_, err := js.Stream(ctx, streamName)
		if err == nil {
			rp.logger.Debug("Stream available", "stream", streamName)
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

	return fmt.Errorf("stream %s not available after %d retries", streamName, maxRetries)
}

// deriveStreamName extracts stream name from subject convention.
// Convention: subject "component.action.type" → stream "COMPONENT"
func deriveStreamName(subject string) string {
	// Handle wildcard subjects
	subject = strings.TrimPrefix(subject, "*.")
	subject = strings.TrimSuffix(subject, ".>")
	subject = strings.TrimSuffix(subject, ".*")

	parts := strings.Split(subject, ".")
	if len(parts) == 0 || parts[0] == "" || parts[0] == "*" || parts[0] == ">" {
		return ""
	}
	return strings.ToUpper(parts[0])
}

// Message handling functions (handleMessage, handleSemanticMessage, evaluateRulesForMessage,
// matchesRuleSubject, recordError) are in message_handler.go

// Stop stops the processor and cleans up resources
func (rp *Processor) Stop(_ time.Duration) error {
	rp.mu.Lock()
	if !rp.running {
		rp.mu.Unlock()
		return nil // Already stopped
	}
	close(rp.shutdown)
	rp.mu.Unlock()

	// Wait for graceful shutdown with timeout
	select {
	case <-rp.done:
		// Clean shutdown
	case <-time.After(5 * time.Second):
		rp.logger.Warn("Rule processor shutdown timeout after 5 seconds")
	}

	// Clean up resources
	rp.mu.Lock()
	defer rp.mu.Unlock()

	// Stop all entity watchers
	for _, watcher := range rp.entityWatchers {
		if err := watcher.Stop(); err != nil {
			rp.logger.Error("Error stopping entity watcher", "error", err)
		}
	}
	rp.entityWatchers = nil

	// Clean up all rules
	rp.rules = nil

	// Legacy JetStream consumer cleanup (if still exists)
	if rp.entityConsumer != nil {
		rp.logger.Info("Legacy JetStream consumer stopped")
	}

	// Note: NATS client handles unsubscription during context cancellation
	rp.isSubscribed = false

	// Close message cache
	if rp.messageCache != nil {
		if err := rp.messageCache.Close(); err != nil {
			rp.logger.Warn("Failed to close message cache", "error", err)
		}
	}

	// Mark as stopped - don't nil the channels, goroutines may still reference them
	// A closed channel is sufficient for signaling; setting to nil causes races
	rp.running = false
	rp.health.Healthy = false

	rp.logger.Info("Rule processor stopped")
	return nil
}

// publishGraphEvents and publishRuleEvent are in publisher.go

// GetRuleMetrics returns metrics for all rules
func (rp *Processor) GetRuleMetrics() map[string]any {
	rp.mu.RLock()
	defer rp.mu.RUnlock()

	metrics := make(map[string]any)

	for name, ruleInstance := range rp.rules {
		metrics[name] = map[string]any{
			"subjects": ruleInstance.Subscribe(),
		}
	}

	metrics["total_evaluated"] = atomic.LoadInt64(&rp.messagesEvaluated)
	metrics["total_triggered"] = atomic.LoadInt64(&rp.rulesTriggered)
	metrics["events_published"] = atomic.LoadInt64(&rp.eventsPublished)
	metrics["error_count"] = atomic.LoadInt64(&rp.errorCount)

	return metrics
}

// Register, CreateRuleProcessor, and convertDefinitionToPort are in factory.go

// Validation functions (ValidateConfigUpdate, validateSingleRuleConfig, validateExpressionRule,
// isKnownRuleType, isValidOperator, createRuleFromConfig, and helper functions) are in config_validation.go

// Runtime configuration functions (ApplyConfigUpdate, applyRuleChanges, GetRuntimeConfig,
// extractConditions, RuntimeConfigWrapper, and related methods) are in runtime_config.go

// Variable substitution functions are in variables.go
