// Package graphembedding provides the graph-embedding component for generating entity embeddings.
package graphembedding

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

	"github.com/c360/semstreams/component"
	"github.com/c360/semstreams/graph"
	"github.com/c360/semstreams/graph/embedding"
	"github.com/c360/semstreams/natsclient"
	"github.com/c360/semstreams/pkg/errs"
	"github.com/c360/semstreams/pkg/retry"
	"github.com/nats-io/nats.go/jetstream"
)

// Ensure Component implements required interfaces
var (
	_ component.Discoverable       = (*Component)(nil)
	_ component.LifecycleComponent = (*Component)(nil)
)

// Config holds configuration for graph-embedding component
type Config struct {
	Ports        *component.PortConfig `json:"ports" schema:"type:ports,description:Port configuration,category:basic"`
	EmbedderType string                `json:"embedder_type" schema:"type:string,description:Embedder type (bm25 or http),category:basic"`
	EmbedderURL  string                `json:"embedder_url" schema:"type:string,description:URL for HTTP embedder (required if embedder_type is http),category:basic"`
	BatchSize    int                   `json:"batch_size" schema:"type:int,description:Batch size for embedding generation,category:advanced"`
	CacheTTLStr  string                `json:"cache_ttl" schema:"type:string,description:Cache TTL for embeddings (e.g. 15m or 1h),category:advanced"`

	// Parsed duration (set by ApplyDefaults)
	cacheTTL time.Duration
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

	// Validate EMBEDDINGS_CACHE output exists
	hasEmbeddingsCache := false
	for _, output := range c.Ports.Outputs {
		if output.Subject == graph.BucketEmbeddingsCache {
			hasEmbeddingsCache = true
			break
		}
	}
	if !hasEmbeddingsCache {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", fmt.Sprintf("%s output required", graph.BucketEmbeddingsCache))
	}

	// Validate embedder type
	if c.EmbedderType == "" {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "embedder_type required")
	}
	if c.EmbedderType != "bm25" && c.EmbedderType != "http" {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "embedder_type must be 'bm25' or 'http'")
	}

	// If HTTP embedder, URL is required
	if c.EmbedderType == "http" && c.EmbedderURL == "" {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "embedder_url required for http embedder_type")
	}

	// Validate batch size
	if c.BatchSize < 0 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "batch_size cannot be negative")
	}

	// Validate cache TTL (parsed duration must be positive)
	if c.cacheTTL < 0 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "cache_ttl cannot be negative")
	}

	return nil
}

// CacheTTL returns the parsed cache TTL duration
func (c *Config) CacheTTL() time.Duration {
	return c.cacheTTL
}

// ApplyDefaults sets default values for configuration
func (c *Config) ApplyDefaults() {
	if c.EmbedderType == "" {
		c.EmbedderType = "bm25"
	}
	if c.BatchSize == 0 {
		c.BatchSize = 50
	}

	// Parse cache TTL from string
	if c.CacheTTLStr != "" {
		if d, err := time.ParseDuration(c.CacheTTLStr); err == nil {
			c.cacheTTL = d
		}
	}
	if c.cacheTTL == 0 {
		c.cacheTTL = 15 * time.Minute
	}

	if c.Ports == nil {
		// Apply full default port config
		defaultConf := DefaultConfig()
		c.Ports = defaultConf.Ports
	} else {
		// If ports exist but are empty, populate with defaults
		if len(c.Ports.Inputs) == 0 {
			c.Ports.Inputs = []component.PortDefinition{
				{
					Name:    "entity_watch",
					Type:    "kv-watch",
					Subject: graph.BucketEntityStates,
				},
			}
		}
		if len(c.Ports.Outputs) == 0 {
			c.Ports.Outputs = []component.PortDefinition{
				{
					Name:    "embeddings",
					Type:    "kv-write",
					Subject: graph.BucketEmbeddingsCache,
				},
			}
		}
	}
}

// DefaultConfig returns a valid default configuration
func DefaultConfig() Config {
	return Config{
		Ports: &component.PortConfig{
			Inputs: []component.PortDefinition{
				{
					Name:    "entity_watch",
					Type:    "kv-watch",
					Subject: graph.BucketEntityStates,
				},
			},
			Outputs: []component.PortDefinition{
				{
					Name:    "embeddings",
					Type:    "kv-write",
					Subject: graph.BucketEmbeddingsCache,
				},
			},
		},
		EmbedderType: "bm25",
		BatchSize:    50,
		cacheTTL:     15 * time.Minute,
	}
}

// schema defines the configuration schema for graph-embedding component
var schema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// Component implements the graph-embedding processor
type Component struct {
	// Component metadata
	name   string
	config Config

	// Dependencies
	natsClient *natsclient.Client
	logger     *slog.Logger

	// Domain resources
	embedder        embedding.Embedder
	storage         *embedding.Storage
	worker          *embedding.Worker
	embeddingBucket jetstream.KeyValue

	// Lifecycle state
	mu          sync.RWMutex
	running     bool
	initialized bool
	startTime   time.Time
	wg          sync.WaitGroup
	cancel      context.CancelFunc

	// Metrics (atomic for internal tracking)
	messagesProcessed int64
	bytesProcessed    int64
	errors            int64
	lastActivity      atomic.Value // stores time.Time

	// Prometheus metrics
	metrics *embeddingMetrics

	// Lifecycle reporting
	lifecycleReporter component.LifecycleReporter

	// Port definitions
	inputPorts  []component.Port
	outputPorts []component.Port
}

// CreateGraphEmbedding is the factory function for creating graph-embedding components
func CreateGraphEmbedding(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	// Validate dependencies
	if deps.NATSClient == nil {
		return nil, errs.WrapInvalid(errs.ErrInvalidConfig, "CreateGraphEmbedding", "factory", "NATSClient required")
	}
	natsClient := deps.NATSClient

	// Parse configuration
	var config Config
	if len(rawConfig) > 0 {
		if err := json.Unmarshal(rawConfig, &config); err != nil {
			return nil, errs.Wrap(err, "CreateGraphEmbedding", "factory", "config unmarshal")
		}
	} else {
		config = DefaultConfig()
	}

	// Apply defaults and validate
	config.ApplyDefaults()
	if err := config.Validate(); err != nil {
		return nil, errs.Wrap(err, "CreateGraphEmbedding", "factory", "config validation")
	}

	// Create logger with component context
	logger := deps.GetLoggerWithComponent("graph-embedding")

	// Create component
	comp := &Component{
		name:       "graph-embedding",
		config:     config,
		natsClient: natsClient,
		logger:     logger,
		metrics:    getMetrics(deps.MetricsRegistry),
	}

	// Initialize last activity
	comp.lastActivity.Store(time.Now())

	return comp, nil
}

// Register registers the graph-embedding factory with the component registry
func Register(registry *component.Registry) error {
	return registry.RegisterFactory("graph-embedding", &component.Registration{
		Name:        "graph-embedding",
		Type:        "processor",
		Protocol:    "nats",
		Domain:      "graph",
		Description: "Graph entity embedding generation processor",
		Version:     "1.0.0",
		Schema:      schema,
		Factory:     CreateGraphEmbedding,
	})
}

// ============================================================================
// Discoverable Interface (6 methods)
// ============================================================================

// Meta returns component metadata
func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        "graph-embedding",
		Type:        "processor",
		Description: "Graph entity embedding generation processor",
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
	c.logger.Info("component initialized", slog.String("component", "graph-embedding"))

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

	// Create embedding bucket (we are the WRITER)
	embeddingBucket, err := c.natsClient.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{
		Bucket:      graph.BucketEmbeddingsCache,
		Description: "Entity embedding cache",
	})
	if err != nil {
		cancel()
		if ctx.Err() != nil {
			return errs.Wrap(ctx.Err(), "Component", "Start", "context cancelled during bucket creation")
		}
		return errs.Wrap(err, "Component", "Start", fmt.Sprintf("KV bucket creation: %s", graph.BucketEmbeddingsCache))
	}
	c.embeddingBucket = embeddingBucket

	// Create embedder based on config
	switch c.config.EmbedderType {
	case "bm25":
		c.embedder = embedding.NewBM25Embedder(embedding.BM25Config{
			Dimensions: 384,
			K1:         1.5,
			B:          0.75,
		})
		c.logger.Info("using BM25 embedder", slog.Int("dimensions", 384))

	case "http":
		httpEmbedder, err := embedding.NewHTTPEmbedder(embedding.HTTPConfig{
			BaseURL: c.config.EmbedderURL,
			Model:   "all-MiniLM-L6-v2",
			Timeout: 30 * time.Second,
			Logger:  c.logger,
		})
		if err != nil {
			cancel()
			return errs.Wrap(err, "Component", "Start", "HTTP embedder creation")
		}
		c.embedder = httpEmbedder
		c.logger.Info("using HTTP embedder", slog.String("url", c.config.EmbedderURL))

	default:
		cancel()
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Component", "Start", fmt.Sprintf("unknown embedder type: %s", c.config.EmbedderType))
	}

	// Set embedder type metric for E2E detection
	if c.metrics != nil {
		c.metrics.setEmbedderType(c.config.EmbedderType)
	}

	// Create EMBEDDING_INDEX bucket for storage (we are the WRITER)
	embeddingIndexBucket, err := c.natsClient.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{
		Bucket:      graph.BucketEmbeddingIndex,
		Description: "Entity embedding index",
	})
	if err != nil {
		cancel()
		return errs.Wrap(err, "Component", "Start", fmt.Sprintf("KV bucket creation: %s", graph.BucketEmbeddingIndex))
	}

	// Create EMBEDDING_DEDUP bucket (we are the WRITER)
	embeddingDedupBucket, err := c.natsClient.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{
		Bucket:      graph.BucketEmbeddingDedup,
		Description: "Entity embedding deduplication",
	})
	if err != nil {
		cancel()
		return errs.Wrap(err, "Component", "Start", fmt.Sprintf("KV bucket creation: %s", graph.BucketEmbeddingDedup))
	}

	// Initialize lifecycle reporter (throttled for high-throughput embedding)
	statusBucket, err := c.natsClient.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{
		Bucket:      "COMPONENT_STATUS",
		Description: "Component lifecycle status tracking",
	})
	if err != nil {
		c.logger.Warn("Failed to create COMPONENT_STATUS bucket, lifecycle reporting disabled",
			slog.Any("error", err))
		c.lifecycleReporter = component.NewNoOpLifecycleReporter()
	} else {
		c.lifecycleReporter = component.NewLifecycleReporterFromConfig(component.LifecycleReporterConfig{
			KV:               statusBucket,
			ComponentName:    "graph-embedding",
			Logger:           c.logger,
			EnableThrottling: true,
		})
	}

	// Create storage
	c.storage = embedding.NewStorage(embeddingIndexBucket, embeddingDedupBucket)

	// Create worker with metrics and generation callback
	c.worker = embedding.NewWorker(c.storage, c.embedder, embeddingIndexBucket, c.logger).
		WithWorkers(c.config.BatchSize / 10). // Scale workers based on batch size
		WithMetrics(newWorkerMetricsAdapter(c.metrics)).
		WithOnGenerated(func(entityID string, _ []float32) {
			// Record successful embedding generation
			if c.metrics != nil {
				c.metrics.recordEmbeddingGenerated()
			}
			c.logger.Debug("embedding generated", "entity_id", entityID)
		})

	// Start worker
	if err := c.worker.Start(ctx); err != nil {
		cancel()
		return errs.Wrap(err, "Component", "Start", "worker start")
	}

	// Wait for input KV bucket (ENTITY_STATES) with retries - we are the reader/watcher
	js, err := c.natsClient.JetStream()
	if err != nil {
		cancel()
		return errs.Wrap(err, "Component", "Start", "JetStream connection")
	}

	entityBucket, err := retry.DoWithResult(ctx, retry.Persistent(), func() (jetstream.KeyValue, error) {
		bucket, err := js.KeyValue(ctx, graph.BucketEntityStates)
		if err != nil {
			c.logger.Debug("waiting for input bucket", slog.String("bucket", graph.BucketEntityStates), slog.Any("error", err))
		}
		return bucket, err
	})
	if err != nil {
		cancel()
		return errs.Wrap(err, "Component", "Start", fmt.Sprintf("%s bucket not available after retries", graph.BucketEntityStates))
	}

	// Start entity watcher goroutine to queue entities for embedding
	c.wg.Add(1)
	go c.watchEntityStates(ctx, entityBucket)

	// Set up query handlers
	if err := c.setupQueryHandlers(ctx); err != nil {
		cancel()
		return errs.Wrap(err, "Component", "Start", "setup query handlers")
	}

	// Mark as running
	c.running = true
	c.startTime = time.Now()

	// Report initial idle state
	_ = c.lifecycleReporter.ReportStage(ctx, "idle")

	c.logger.Info("component started",
		slog.String("component", "graph-embedding"),
		slog.Time("start_time", c.startTime),
		slog.String("embedder_type", c.config.EmbedderType))

	return nil
}

// Stop gracefully shuts down the component
func (c *Component) Stop(timeout time.Duration) error {
	c.mu.Lock()

	if !c.running {
		c.mu.Unlock()
		return nil // Already stopped
	}

	// Stop worker first
	if c.worker != nil {
		if err := c.worker.Stop(); err != nil {
			c.logger.Warn("worker stop error", slog.Any("error", err))
		}
	}

	// Close embedder
	if c.embedder != nil {
		if err := c.embedder.Close(); err != nil {
			c.logger.Warn("embedder close error", slog.Any("error", err))
		}
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
		c.logger.Info("component stopped gracefully", slog.String("component", "graph-embedding"))
		return nil
	case <-time.After(timeout):
		c.logger.Warn("component stop timed out", slog.String("component", "graph-embedding"))
		return fmt.Errorf("stop timeout after %v", timeout)
	}
}

// ============================================================================
// Entity State Watcher
// ============================================================================

// watchEntityStates watches the ENTITY_STATES KV bucket and queues entities for embedding
func (c *Component) watchEntityStates(ctx context.Context, bucket jetstream.KeyValue) {
	defer c.wg.Done()

	watcher, err := bucket.WatchAll(ctx)
	if err != nil {
		c.logger.Error("failed to start entity watcher",
			slog.String("bucket", graph.BucketEntityStates),
			slog.Any("error", err))
		return
	}
	// NOTE: watcher.Stop() is called explicitly before each return, not via defer.
	// This avoids a race condition in nats.go where Stop() can race with the
	// internal message handler goroutine when using defer.

	c.logger.Info("entity watcher started", slog.String("bucket", graph.BucketEntityStates))

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("entity watcher stopping", slog.String("reason", "context cancelled"))
			watcher.Stop()
			return
		case entry, ok := <-watcher.Updates():
			if !ok {
				// Channel closed, watcher stopped externally
				watcher.Stop()
				return
			}
			if entry == nil {
				// nil entry indicates initial state enumeration complete
				c.logger.Debug("entity watcher initial sync complete")
				continue
			}

			if entry.Operation() == jetstream.KeyValueDelete {
				// Skip deleted entities
				continue
			}

			c.queueEntityForEmbedding(ctx, entry.Key(), entry.Value())
		}
	}
}

// queueEntityForEmbedding queues an entity for async embedding generation
func (c *Component) queueEntityForEmbedding(ctx context.Context, entityID string, data []byte) {
	// Report embedding stage (throttled to avoid KV spam)
	_ = c.lifecycleReporter.ReportStage(ctx, "embedding")

	// Parse entity state
	var entityState graph.EntityState
	if err := json.Unmarshal(data, &entityState); err != nil {
		c.logger.Warn("failed to unmarshal entity state",
			slog.String("entity", entityID),
			slog.Any("error", err))
		return
	}

	// ContentStorable path: if StorageRef is present, use it
	if entityState.StorageRef != nil {
		c.queueEmbeddingWithStorageRef(ctx, entityID, &entityState)
		return
	}

	// Legacy path: Extract text from triples
	text := c.extractTextForEmbedding(&entityState)
	if text == "" {
		c.logger.Debug("no text content found, skipping embedding", slog.String("entity", entityID))
		return
	}

	// Calculate content hash for deduplication
	contentHash := embedding.ContentHash(text)

	// Queue for embedding generation
	if err := c.storage.SavePending(ctx, entityID, contentHash, text); err != nil {
		c.logger.Error("failed to queue embedding",
			slog.String("entity", entityID),
			slog.Any("error", err))
		return
	}

	c.logger.Debug("queued embedding for generation",
		slog.String("entity", entityID),
		slog.Int("text_length", len(text)))
}

// queueEmbeddingWithStorageRef queues an embedding using ContentStorable pattern
func (c *Component) queueEmbeddingWithStorageRef(ctx context.Context, entityID string, state *graph.EntityState) {
	// Create StorageRef for embedding record
	storageRef := &embedding.StorageRef{
		StorageInstance: state.StorageRef.StorageInstance,
		Key:             state.StorageRef.Key,
	}

	// Calculate content hash from storage key (for deduplication)
	contentHash := embedding.ContentHash(state.StorageRef.Key)

	// Queue for embedding generation with storage reference
	if err := c.storage.SavePendingWithStorageRef(ctx, entityID, contentHash, storageRef, nil); err != nil {
		c.logger.Error("failed to queue embedding with storage ref",
			slog.String("entity", entityID),
			slog.Any("error", err))
		return
	}

	c.logger.Debug("queued embedding with storage reference",
		slog.String("entity", entityID),
		slog.String("storage_key", state.StorageRef.Key))
}

// extractTextForEmbedding extracts text from entity state for embedding generation
func (c *Component) extractTextForEmbedding(state *graph.EntityState) string {
	var parts []string

	// Suffixes to look for in predicates (e.g., dc.terms.title matches ".title")
	textSuffixes := []string{".title", ".content", ".description", ".summary", ".text", ".name", ".body", ".abstract", ".subject"}

	// Look through all triples for text-like predicates
	for _, triple := range state.Triples {
		if triple.IsRelationship() {
			continue
		}

		predicate := strings.ToLower(triple.Predicate)

		// Check if predicate ends with any text suffix
		for _, suffix := range textSuffixes {
			if strings.HasSuffix(predicate, suffix) {
				if str, ok := triple.Object.(string); ok && str != "" {
					parts = append(parts, str)
				}
				break
			}
		}
	}

	return strings.Join(parts, " ")
}
