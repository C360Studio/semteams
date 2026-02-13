package trustgraph

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/metric"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/c360studio/semstreams/trustgraph"
	vocab "github.com/c360studio/semstreams/vocabulary/trustgraph"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	syncBucketName = "TRUSTGRAPH_SYNC"
)

// Metrics holds Prometheus metrics for the TrustGraph input component.
type Metrics struct {
	triplesImported   prometheus.Counter
	entitiesPublished prometheus.Counter
	pollsTotal        prometheus.Counter
	pollErrors        prometheus.Counter
	pollDuration      prometheus.Histogram
}

func newMetrics(registry *metric.MetricsRegistry, name string) *Metrics {
	if registry == nil {
		return nil
	}

	metrics := &Metrics{
		triplesImported: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "semstreams",
			Subsystem: "trustgraph_input",
			Name:      "triples_imported_total",
			Help:      "Total triples imported from TrustGraph",
		}),
		entitiesPublished: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "semstreams",
			Subsystem: "trustgraph_input",
			Name:      "entities_published_total",
			Help:      "Total entities published to NATS",
		}),
		pollsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "semstreams",
			Subsystem: "trustgraph_input",
			Name:      "polls_total",
			Help:      "Total poll operations",
		}),
		pollErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "semstreams",
			Subsystem: "trustgraph_input",
			Name:      "poll_errors_total",
			Help:      "Total poll errors",
		}),
		pollDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "semstreams",
			Subsystem: "trustgraph_input",
			Name:      "poll_duration_seconds",
			Help:      "Duration of poll operations in seconds",
			Buckets:   prometheus.DefBuckets,
		}),
	}

	serviceName := fmt.Sprintf("trustgraph_input_%s", name)
	registry.RegisterCounter(serviceName, "triples_imported", metrics.triplesImported)
	registry.RegisterCounter(serviceName, "entities_published", metrics.entitiesPublished)
	registry.RegisterCounter(serviceName, "polls_total", metrics.pollsTotal)
	registry.RegisterCounter(serviceName, "poll_errors", metrics.pollErrors)
	registry.RegisterHistogram(serviceName, "poll_duration", metrics.pollDuration)

	return metrics
}

// Component implements the TrustGraph input component.
type Component struct {
	name   string
	config Config

	// Dependencies
	tgClient   *trustgraph.Client
	translator *vocab.Translator
	natsClient *natsclient.Client
	logger     *slog.Logger
	metrics    *Metrics

	// Sync state
	syncBucket jetstream.KeyValue

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
	triplesImported   atomic.Int64
	entitiesPublished atomic.Int64
	pollErrors        atomic.Int64
	lastPollTime      time.Time
}

// Ensure Component implements all required interfaces.
var _ component.Discoverable = (*Component)(nil)
var _ component.LifecycleComponent = (*Component)(nil)

// componentSchema defines the configuration schema.
var componentSchema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// New creates a new TrustGraph input component.
func New(deps ComponentDeps) *Component {
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default().With("component", "trustgraph-input")
	}

	// Create TrustGraph client
	tgClient := trustgraph.New(trustgraph.Config{
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
		name = "trustgraph-input"
	}
	return component.Metadata{
		Name:        name,
		Type:        "input",
		Description: fmt.Sprintf("Imports entities from TrustGraph at %s", c.config.Endpoint),
		Version:     "1.0.0",
	}
}

// InputPorts returns the input ports for this component.
func (c *Component) InputPorts() []component.Port {
	// Input component has no NATS input ports - it polls TrustGraph REST API
	return []component.Port{
		{
			Name:        "trustgraph_api",
			Direction:   component.DirectionInput,
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

// OutputPorts returns the output ports for this component.
func (c *Component) OutputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "entity",
			Direction:   component.DirectionOutput,
			Required:    true,
			Description: "NATS subject for publishing imported entities",
			Config: component.NATSPort{
				Subject: c.config.GetOutputSubject(),
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
	lastPoll := c.lastPollTime
	c.mu.RUnlock()

	errorCount := c.pollErrors.Load()

	// Consider unhealthy if no successful poll in 2x poll interval
	expectedInterval := c.config.GetPollInterval() * 2
	stale := time.Since(lastPoll) > expectedInterval && !lastPoll.IsZero()

	return component.HealthStatus{
		Healthy:    running && !stale,
		LastCheck:  time.Now(),
		ErrorCount: int(errorCount),
		Uptime:     time.Since(c.startTime),
	}
}

// DataFlow returns the current data flow metrics.
func (c *Component) DataFlow() component.FlowMetrics {
	entities := c.entitiesPublished.Load()
	errorCount := c.pollErrors.Load()

	var messagesPerSecond float64
	var errorRate float64

	if uptime := time.Since(c.startTime).Seconds(); uptime > 0 {
		messagesPerSecond = float64(entities) / uptime
	}

	if entities > 0 {
		errorRate = float64(errorCount) / float64(entities)
	}

	c.mu.RLock()
	lastActivity := c.lastPollTime
	c.mu.RUnlock()

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
		return errs.WrapInvalid(errs.ErrMissingConfig, "trustgraph-input", "Initialize", "NATS client is required")
	}

	return nil
}

// Start begins polling TrustGraph and publishing entities.
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

	// Create sync bucket for deduplication
	syncBucket, err := c.natsClient.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{
		Bucket:      syncBucketName,
		Description: "TrustGraph sync state for deduplication",
	})
	if err != nil {
		c.logger.Warn("Failed to create sync bucket, deduplication disabled", slog.Any("error", err))
	} else {
		c.syncBucket = syncBucket
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

	c.shutdown = make(chan struct{})
	c.done = make(chan struct{})

	c.wg.Add(1)
	c.running.Store(true)

	go c.pollLoop(ctx)

	c.logger.Info("TrustGraph input started",
		slog.String("endpoint", c.config.Endpoint),
		slog.Duration("poll_interval", c.config.GetPollInterval()))

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
	close(c.shutdown)
	c.lifecycleMu.Unlock()

	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		c.logger.Info("TrustGraph input stopped gracefully")
	case <-time.After(timeout):
		c.logger.Warn("TrustGraph input stop timed out")
	}

	return nil
}

// pollLoop runs the polling loop.
func (c *Component) pollLoop(ctx context.Context) {
	defer c.wg.Done()
	defer close(c.done)

	ticker := time.NewTicker(c.config.GetPollInterval())
	defer ticker.Stop()

	// Initial poll
	c.poll(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.shutdown:
			return
		case <-ticker.C:
			c.poll(ctx)
		}
	}
}

// poll executes a single poll cycle.
func (c *Component) poll(ctx context.Context) {
	start := time.Now()

	if c.lifecycleReporter != nil {
		_ = c.lifecycleReporter.ReportStage(ctx, "polling")
	}

	// Query triples from TrustGraph
	params := trustgraph.TriplesQueryParams{
		Limit: c.config.Limit,
	}

	if c.config.SubjectFilter != "" {
		params.S = &trustgraph.TGValue{V: c.config.SubjectFilter, E: true}
	}

	triples, err := c.tgClient.QueryTriples(ctx, params)
	if err != nil {
		c.logger.Error("Failed to query TrustGraph", slog.Any("error", err))
		c.pollErrors.Add(1)
		if c.metrics != nil {
			c.metrics.pollErrors.Inc()
		}
		return
	}

	if c.metrics != nil {
		c.metrics.pollsTotal.Inc()
		c.metrics.pollDuration.Observe(time.Since(start).Seconds())
	}

	// Convert to vocab types and group by subject
	vocabTriples := make([]vocab.TGTriple, len(triples))
	for i, t := range triples {
		vocabTriples[i] = vocab.TGTriple{
			S: vocab.TGValue{V: t.S.V, E: t.S.E},
			P: vocab.TGValue{V: t.P.V, E: t.P.E},
			O: vocab.TGValue{V: t.O.V, E: t.O.E},
		}
	}

	// Group triples by subject
	grouped := c.groupBySubject(vocabTriples)

	// Process each entity
	for subjectURI, entityTriples := range grouped {
		if err := c.processEntity(ctx, subjectURI, entityTriples); err != nil {
			c.logger.Warn("Failed to process entity",
				slog.String("subject", subjectURI),
				slog.Any("error", err))
		}
	}

	c.mu.Lock()
	c.lastPollTime = time.Now()
	c.mu.Unlock()

	c.triplesImported.Add(int64(len(triples)))
	if c.metrics != nil {
		c.metrics.triplesImported.Add(float64(len(triples)))
	}

	c.logger.Debug("Poll completed",
		slog.Int("triples", len(triples)),
		slog.Int("entities", len(grouped)),
		slog.Duration("duration", time.Since(start)))
}

// groupBySubject groups triples by their subject URI.
func (c *Component) groupBySubject(triples []vocab.TGTriple) map[string][]vocab.TGTriple {
	grouped := make(map[string][]vocab.TGTriple)
	for _, t := range triples {
		grouped[t.S.V] = append(grouped[t.S.V], t)
	}
	return grouped
}

// processEntity processes a single entity (subject with its triples).
func (c *Component) processEntity(ctx context.Context, subjectURI string, tgTriples []vocab.TGTriple) error {
	// Translate to SemStreams triples
	entityID := c.translator.URIToEntityID(subjectURI)
	triples := make([]message.Triple, len(tgTriples))
	for i, tg := range tgTriples {
		triples[i] = c.translator.RDFToTriple(tg, c.config.Source)
		// Override subject with the translated entity ID
		triples[i].Subject = entityID
	}

	// Compute hash for deduplication
	hash := c.computeTripleHash(triples)

	// Check if entity has changed
	if c.syncBucket != nil {
		key := "input:hash:" + entityID
		entry, err := c.syncBucket.Get(ctx, key)
		if err == nil && string(entry.Value()) == hash {
			// Entity hasn't changed, skip
			return nil
		}

		// Update hash
		_, err = c.syncBucket.Put(ctx, key, []byte(hash))
		if err != nil {
			c.logger.Debug("Failed to update sync hash", slog.Any("error", err))
		}
	}

	// Publish entity to NATS
	return c.publishEntity(ctx, entityID, triples)
}

// computeTripleHash computes a hash of the triple set for deduplication.
func (c *Component) computeTripleHash(triples []message.Triple) string {
	// Sort triples for consistent hashing
	sorted := make([]string, len(triples))
	for i, t := range triples {
		sorted[i] = fmt.Sprintf("%s|%s|%v", t.Subject, t.Predicate, t.Object)
	}
	sort.Strings(sorted)

	h := sha256.New()
	for _, s := range sorted {
		h.Write([]byte(s))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// publishEntity publishes an entity to NATS.
func (c *Component) publishEntity(ctx context.Context, entityID string, triples []message.Triple) error {
	// Create entity message
	entityMsg := EntityMessage{
		EntityID:  entityID,
		Triples:   triples,
		Source:    c.config.Source,
		Timestamp: time.Now(),
	}

	data, err := json.Marshal(entityMsg)
	if err != nil {
		return errs.Wrap(err, "trustgraph-input", "publishEntity", "marshal entity")
	}

	// Determine subject from entity ID
	// Entity ID format: org.platform.domain.system.type.instance
	// Publish to: entity.{type}.{instance} or configured subject
	subject := c.config.GetOutputSubject()
	if strings.HasSuffix(subject, ">") {
		// Wildcard subject - construct specific subject from entity ID
		parts := strings.Split(entityID, ".")
		if len(parts) >= 6 {
			subject = strings.TrimSuffix(subject, ">") + parts[4] + "." + parts[5]
		} else {
			subject = strings.TrimSuffix(subject, ">") + entityID
		}
	}

	// Check if we should use JetStream
	if c.isJetStreamPort() {
		if err := c.natsClient.PublishToStream(ctx, subject, data); err != nil {
			return errs.WrapTransient(err, "trustgraph-input", "publishEntity", "JetStream publish")
		}
	} else {
		nc := c.natsClient.GetConnection()
		if nc == nil {
			return errs.WrapTransient(errs.ErrNoConnection, "trustgraph-input", "publishEntity", "NATS connection check")
		}
		if err := nc.Publish(subject, data); err != nil {
			return errs.WrapTransient(err, "trustgraph-input", "publishEntity", "NATS publish")
		}
	}

	c.entitiesPublished.Add(1)
	if c.metrics != nil {
		c.metrics.entitiesPublished.Inc()
	}

	return nil
}

// isJetStreamPort checks if the output port is configured for JetStream.
func (c *Component) isJetStreamPort() bool {
	if c.config.Ports == nil {
		return false
	}
	for _, port := range c.config.Ports.Outputs {
		if port.Type == "jetstream" {
			return true
		}
	}
	return false
}

// EntityMessage represents an imported entity message.
type EntityMessage struct {
	EntityID  string           `json:"entity_id"`
	Triples   []message.Triple `json:"triples"`
	Source    string           `json:"source"`
	Timestamp time.Time        `json:"timestamp"`
}
