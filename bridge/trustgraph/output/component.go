package output

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

	"github.com/c360studio/semstreams/bridge/trustgraph/client"
	"github.com/c360studio/semstreams/bridge/trustgraph/vocab"
	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/metric"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus"
)

// Metrics holds Prometheus metrics for the TrustGraph output component.
type Metrics struct {
	entitiesReceived prometheus.Counter
	entitiesExported prometheus.Counter
	entitiesFiltered prometheus.Counter
	triplesExported  prometheus.Counter
	batchesSent      prometheus.Counter
	exportErrors     prometheus.Counter
	exportDuration   prometheus.Histogram
}

func newMetrics(registry *metric.MetricsRegistry, name string) *Metrics {
	if registry == nil {
		return nil
	}

	metrics := &Metrics{
		entitiesReceived: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "semstreams",
			Subsystem: "trustgraph_output",
			Name:      "entities_received_total",
			Help:      "Total entities received from NATS",
		}),
		entitiesExported: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "semstreams",
			Subsystem: "trustgraph_output",
			Name:      "entities_exported_total",
			Help:      "Total entities exported to TrustGraph",
		}),
		entitiesFiltered: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "semstreams",
			Subsystem: "trustgraph_output",
			Name:      "entities_filtered_total",
			Help:      "Total entities filtered out",
		}),
		triplesExported: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "semstreams",
			Subsystem: "trustgraph_output",
			Name:      "triples_exported_total",
			Help:      "Total triples exported to TrustGraph",
		}),
		batchesSent: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "semstreams",
			Subsystem: "trustgraph_output",
			Name:      "batches_sent_total",
			Help:      "Total batches sent to TrustGraph",
		}),
		exportErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "semstreams",
			Subsystem: "trustgraph_output",
			Name:      "export_errors_total",
			Help:      "Total export errors",
		}),
		exportDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "semstreams",
			Subsystem: "trustgraph_output",
			Name:      "export_duration_seconds",
			Help:      "Duration of export operations in seconds",
			Buckets:   prometheus.DefBuckets,
		}),
	}

	serviceName := fmt.Sprintf("trustgraph_output_%s", name)
	registry.RegisterCounter(serviceName, "entities_received", metrics.entitiesReceived)
	registry.RegisterCounter(serviceName, "entities_exported", metrics.entitiesExported)
	registry.RegisterCounter(serviceName, "entities_filtered", metrics.entitiesFiltered)
	registry.RegisterCounter(serviceName, "triples_exported", metrics.triplesExported)
	registry.RegisterCounter(serviceName, "batches_sent", metrics.batchesSent)
	registry.RegisterCounter(serviceName, "export_errors", metrics.exportErrors)
	registry.RegisterHistogram(serviceName, "export_duration", metrics.exportDuration)

	return metrics
}

// Component implements the TrustGraph output component.
type Component struct {
	name   string
	config Config

	// Dependencies
	tgClient   *client.Client
	translator *vocab.Translator
	natsClient *natsclient.Client
	logger     *slog.Logger
	metrics    *Metrics

	// Subscriptions
	subscriptions []*natsclient.Subscription

	// Batching
	batchMu     sync.Mutex
	batch       []client.TGTriple
	lastFlush   time.Time
	flushTicker *time.Ticker

	// Lifecycle management
	lifecycleMu       sync.Mutex
	shutdown          chan struct{}
	done              chan struct{}
	running           atomic.Bool
	startTime         time.Time
	mu                sync.RWMutex
	wg                sync.WaitGroup
	lifecycleReporter component.LifecycleReporter

	// Metrics (atomic for thread safety)
	entitiesReceived atomic.Int64
	entitiesExported atomic.Int64
	triplesExported  atomic.Int64
	exportErrors     atomic.Int64
}

// Ensure Component implements all required interfaces.
var _ component.Discoverable = (*Component)(nil)
var _ component.LifecycleComponent = (*Component)(nil)

// componentSchema defines the configuration schema.
var componentSchema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// New creates a new TrustGraph output component.
func New(deps ComponentDeps) *Component {
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default().With("component", "trustgraph-output")
	}

	// Create TrustGraph client
	tgClient := client.New(client.Config{
		Endpoint:   deps.Config.Endpoint,
		APIKey:     deps.Config.GetAPIKey(),
		Timeout:    deps.Config.GetTimeout(),
		MaxRetries: 3,
	})

	// Create translator
	translator := vocab.NewTranslator(deps.Config.Vocab.ToTranslatorConfig())

	var metrics *Metrics
	if deps.MetricsRegistry != nil {
		metrics = newMetrics(deps.MetricsRegistry, deps.Name)
	}

	return &Component{
		name:       deps.Name,
		config:     deps.Config,
		tgClient:   tgClient,
		translator: translator,
		natsClient: deps.NATSClient,
		logger:     logger,
		metrics:    metrics,
		startTime:  time.Now(),
		batch:      make([]client.TGTriple, 0, deps.Config.BatchSize),
	}
}

// ComponentDeps holds runtime dependencies for the component.
type ComponentDeps struct {
	Name            string
	Config          Config
	NATSClient      *natsclient.Client
	MetricsRegistry *metric.MetricsRegistry
	Logger          *slog.Logger
}

// Meta returns the component metadata.
func (c *Component) Meta() component.Metadata {
	name := c.name
	if name == "" {
		name = "trustgraph-output"
	}
	return component.Metadata{
		Name:        name,
		Type:        "output",
		Description: fmt.Sprintf("Exports entities to TrustGraph at %s", c.config.Endpoint),
		Version:     "1.0.0",
	}
}

// InputPorts returns the input ports for this component.
func (c *Component) InputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "entity",
			Direction:   component.DirectionInput,
			Required:    true,
			Description: "NATS subject for subscribing to entity changes",
			Config: component.NATSPort{
				Subject: c.config.GetInputSubject(),
			},
		},
	}
}

// OutputPorts returns the output ports for this component.
func (c *Component) OutputPorts() []component.Port {
	// Output component has no NATS output ports - it sends to TrustGraph REST API
	return []component.Port{
		{
			Name:        "trustgraph_api",
			Direction:   component.DirectionOutput,
			Required:    true,
			Description: fmt.Sprintf("TrustGraph API at %s", c.config.Endpoint),
			Config: component.NetworkPort{
				Protocol: "http",
				Host:     c.config.Endpoint,
				Port:     8088,
			},
		},
	}
}

// ConfigSchema returns the configuration schema.
func (c *Component) ConfigSchema() component.ConfigSchema {
	return componentSchema
}

// Health returns the current health status.
func (c *Component) Health() component.HealthStatus {
	c.mu.RLock()
	running := c.running.Load()
	c.mu.RUnlock()

	errorCount := c.exportErrors.Load()

	return component.HealthStatus{
		Healthy:    running,
		LastCheck:  time.Now(),
		ErrorCount: int(errorCount),
		Uptime:     time.Since(c.startTime),
	}
}

// DataFlow returns the current data flow metrics.
func (c *Component) DataFlow() component.FlowMetrics {
	entities := c.entitiesExported.Load()
	errorCount := c.exportErrors.Load()

	var messagesPerSecond float64
	var errorRate float64

	if uptime := time.Since(c.startTime).Seconds(); uptime > 0 {
		messagesPerSecond = float64(entities) / uptime
	}

	if entities > 0 {
		errorRate = float64(errorCount) / float64(entities)
	}

	c.batchMu.Lock()
	lastActivity := c.lastFlush
	c.batchMu.Unlock()

	return component.FlowMetrics{
		MessagesPerSecond: messagesPerSecond,
		ErrorRate:         errorRate,
		LastActivity:      lastActivity,
	}
}

// Initialize prepares the component.
func (c *Component) Initialize() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.natsClient == nil {
		return errs.WrapInvalid(errs.ErrMissingConfig, "trustgraph-output", "Initialize", "NATS client is required")
	}

	return nil
}

// Start begins subscribing to entity changes and exporting to TrustGraph.
func (c *Component) Start(ctx context.Context) error {
	if ctx == nil {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Component", "Start", "context cannot be nil")
	}
	if err := ctx.Err(); err != nil {
		return errs.WrapInvalid(err, "Component", "Start", "context already cancelled")
	}

	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()

	if c.running.Load() {
		return nil
	}

	// Initialize lifecycle reporter
	statusBucket, err := c.natsClient.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{
		Bucket:      "COMPONENT_STATUS",
		Description: "Component lifecycle status tracking",
	})
	if err != nil {
		c.logger.Warn("Failed to create COMPONENT_STATUS bucket", slog.Any("error", err))
		c.lifecycleReporter = component.NewNoOpLifecycleReporter()
	} else {
		c.lifecycleReporter = component.NewLifecycleReporterFromConfig(component.LifecycleReporterConfig{
			KV:               statusBucket,
			ComponentName:    c.name,
			Logger:           c.logger,
			EnableThrottling: true,
		})
	}

	// Setup subscriptions
	if err := c.setupSubscriptions(ctx); err != nil {
		return errs.WrapTransient(err, "trustgraph-output", "Start", "setup subscriptions")
	}

	c.shutdown = make(chan struct{})
	c.done = make(chan struct{})

	// Start flush ticker
	c.flushTicker = time.NewTicker(c.config.GetFlushInterval())

	c.wg.Add(1)
	c.running.Store(true)

	go c.flushLoop(ctx)

	c.logger.Info("TrustGraph output started",
		slog.String("endpoint", c.config.Endpoint),
		slog.String("kg_core_id", c.config.KGCoreID),
		slog.Int("batch_size", c.config.BatchSize))

	return nil
}

// Stop gracefully shuts down the component.
func (c *Component) Stop(timeout time.Duration) error {
	c.lifecycleMu.Lock()

	if !c.running.Load() {
		c.lifecycleMu.Unlock()
		return nil
	}

	c.running.Store(false)

	// Stop flush ticker
	if c.flushTicker != nil {
		c.flushTicker.Stop()
	}

	close(c.shutdown)
	c.lifecycleMu.Unlock()

	// Unsubscribe
	for _, sub := range c.subscriptions {
		if err := sub.Unsubscribe(); err != nil {
			c.logger.Warn("Failed to unsubscribe", slog.Any("error", err))
		}
	}

	// Final flush
	ctx, cancel := context.WithTimeout(context.Background(), timeout/2)
	defer cancel()
	c.flush(ctx)

	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		c.logger.Info("TrustGraph output stopped gracefully")
	case <-time.After(timeout):
		c.logger.Warn("TrustGraph output stop timed out")
	}

	return nil
}

// setupSubscriptions creates NATS subscriptions for entity subjects.
func (c *Component) setupSubscriptions(ctx context.Context) error {
	subject := c.config.GetInputSubject()

	sub, err := c.natsClient.Subscribe(ctx, subject, func(ctx context.Context, msg *nats.Msg) {
		c.handleMessage(ctx, msg.Data)
	})
	if err != nil {
		return err
	}

	c.subscriptions = append(c.subscriptions, sub)
	c.logger.Debug("Subscribed to entity subject", slog.String("subject", subject))

	return nil
}

// flushLoop runs the periodic flush loop.
func (c *Component) flushLoop(ctx context.Context) {
	defer c.wg.Done()
	defer close(c.done)

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.shutdown:
			return
		case <-c.flushTicker.C:
			c.flush(ctx)
		}
	}
}

// handleMessage processes an incoming entity message.
func (c *Component) handleMessage(ctx context.Context, data []byte) {
	c.entitiesReceived.Add(1)
	if c.metrics != nil {
		c.metrics.entitiesReceived.Inc()
	}

	// Parse entity message
	var entityMsg EntityMessage
	if err := json.Unmarshal(data, &entityMsg); err != nil {
		c.logger.Debug("Failed to parse entity message", slog.Any("error", err))
		return
	}

	// Filter by entity prefix
	if !c.matchesPrefix(entityMsg.EntityID) {
		if c.metrics != nil {
			c.metrics.entitiesFiltered.Inc()
		}
		return
	}

	// Filter by source (loop prevention)
	if c.shouldExclude(entityMsg.Triples) {
		if c.metrics != nil {
			c.metrics.entitiesFiltered.Inc()
		}
		return
	}

	// Translate triples to RDF format
	tgTriples := c.translateTriples(entityMsg.Triples)

	// Add to batch
	c.batchMu.Lock()
	c.batch = append(c.batch, tgTriples...)
	shouldFlush := len(c.batch) >= c.config.BatchSize
	c.batchMu.Unlock()

	if shouldFlush {
		c.flush(ctx)
	}
}

// matchesPrefix checks if the entity ID matches any configured prefix.
func (c *Component) matchesPrefix(entityID string) bool {
	if len(c.config.EntityPrefixes) == 0 {
		return true // No filter means accept all
	}

	for _, prefix := range c.config.EntityPrefixes {
		if strings.HasPrefix(entityID, prefix) {
			return true
		}
	}
	return false
}

// shouldExclude checks if the entity should be excluded based on source.
func (c *Component) shouldExclude(triples []message.Triple) bool {
	if len(c.config.ExcludeSources) == 0 {
		return false
	}

	// Check if ALL triples have an excluded source
	for _, triple := range triples {
		excluded := false
		for _, excludeSource := range c.config.ExcludeSources {
			if triple.Source == excludeSource {
				excluded = true
				break
			}
		}
		if !excluded {
			return false // At least one triple has non-excluded source
		}
	}
	return true // All triples have excluded source
}

// translateTriples converts SemStreams triples to TrustGraph format.
func (c *Component) translateTriples(triples []message.Triple) []client.TGTriple {
	vocabTriples := c.translator.TriplesToRDF(triples)

	// Convert vocab.TGTriple to client.TGTriple
	result := make([]client.TGTriple, len(vocabTriples))
	for i, vt := range vocabTriples {
		result[i] = client.TGTriple{
			S: client.TGValue{V: vt.S.V, E: vt.S.E},
			P: client.TGValue{V: vt.P.V, E: vt.P.E},
			O: client.TGValue{V: vt.O.V, E: vt.O.E},
		}
	}
	return result
}

// flush sends the current batch to TrustGraph.
func (c *Component) flush(ctx context.Context) {
	c.batchMu.Lock()
	if len(c.batch) == 0 {
		c.batchMu.Unlock()
		return
	}

	// Take the batch
	batch := c.batch
	c.batch = make([]client.TGTriple, 0, c.config.BatchSize)
	c.lastFlush = time.Now()
	c.batchMu.Unlock()

	// Report lifecycle stage
	if c.lifecycleReporter != nil {
		_ = c.lifecycleReporter.ReportStage(ctx, "exporting")
	}

	start := time.Now()

	// Send to TrustGraph
	err := c.tgClient.PutKGCoreTriples(ctx, c.config.KGCoreID, c.config.User, c.config.Collection, batch)
	if err != nil {
		c.logger.Error("Failed to export triples to TrustGraph",
			slog.Int("triples", len(batch)),
			slog.Any("error", err))
		c.exportErrors.Add(1)
		if c.metrics != nil {
			c.metrics.exportErrors.Inc()
		}

		// Put batch back for retry (best effort)
		c.batchMu.Lock()
		c.batch = append(batch, c.batch...)
		if len(c.batch) > c.config.BatchSize*2 {
			// Prevent unbounded growth - drop oldest
			c.batch = c.batch[len(c.batch)-c.config.BatchSize:]
		}
		c.batchMu.Unlock()
		return
	}

	// Update metrics
	c.triplesExported.Add(int64(len(batch)))
	c.entitiesExported.Add(1)

	if c.metrics != nil {
		c.metrics.triplesExported.Add(float64(len(batch)))
		c.metrics.batchesSent.Inc()
		c.metrics.entitiesExported.Inc()
		c.metrics.exportDuration.Observe(time.Since(start).Seconds())
	}

	c.logger.Debug("Exported triples to TrustGraph",
		slog.Int("triples", len(batch)),
		slog.Duration("duration", time.Since(start)))
}

// EntityMessage represents an entity message from NATS.
type EntityMessage struct {
	EntityID  string           `json:"entity_id"`
	Triples   []message.Triple `json:"triples"`
	Source    string           `json:"source"`
	Timestamp time.Time        `json:"timestamp"`
}
