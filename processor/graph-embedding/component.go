// Package graphembedding provides the graph-embedding component for generating entity embeddings.
package graphembedding

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/c360/semstreams/component"
	"github.com/c360/semstreams/graph/embedding"
	"github.com/c360/semstreams/natsclient"
	"github.com/c360/semstreams/pkg/errs"
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
	CacheTTL     time.Duration         `json:"cache_ttl" schema:"type:string,description:Cache TTL for embeddings,category:advanced"`
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
		if output.Subject == "EMBEDDINGS_CACHE" {
			hasEmbeddingsCache = true
			break
		}
	}
	if !hasEmbeddingsCache {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "EMBEDDINGS_CACHE output required")
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

	// Validate cache TTL
	if c.CacheTTL < 0 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "cache_ttl cannot be negative")
	}

	return nil
}

// ApplyDefaults sets default values for configuration
func (c *Config) ApplyDefaults() {
	if c.EmbedderType == "" {
		c.EmbedderType = "bm25"
	}
	if c.BatchSize == 0 {
		c.BatchSize = 50
	}
	if c.CacheTTL == 0 {
		c.CacheTTL = 15 * time.Minute
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
					Subject: "ENTITY_STATES",
				},
			}
		}
		if len(c.Ports.Outputs) == 0 {
			c.Ports.Outputs = []component.PortDefinition{
				{
					Name:    "embeddings",
					Type:    "kv-write",
					Subject: "EMBEDDINGS_CACHE",
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
					Subject: "ENTITY_STATES",
				},
			},
			Outputs: []component.PortDefinition{
				{
					Name:    "embeddings",
					Type:    "kv-write",
					Subject: "EMBEDDINGS_CACHE",
				},
			},
		},
		EmbedderType: "bm25",
		BatchSize:    50,
		CacheTTL:     15 * time.Minute,
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

	// Metrics (atomic)
	messagesProcessed int64
	bytesProcessed    int64
	errors            int64
	lastActivity      atomic.Value // stores time.Time

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
		natsClient: deps.NATSClient,
		logger:     logger,
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

	// Initialize JetStream
	js, err := c.natsClient.JetStream()
	if err != nil {
		cancel()
		return errs.Wrap(err, "Component", "Start", "JetStream connection")
	}

	// Get embedding bucket
	embeddingBucket, err := js.KeyValue(ctx, "EMBEDDINGS_CACHE")
	if err != nil {
		cancel()
		return errs.Wrap(err, "Component", "Start", "KV bucket access: EMBEDDINGS_CACHE")
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

	// Get EMBEDDING_INDEX bucket for storage (for dedup bucket, use simplified approach)
	embeddingIndexBucket, err := js.KeyValue(ctx, "EMBEDDING_INDEX")
	if err != nil {
		cancel()
		return errs.Wrap(err, "Component", "Start", "KV bucket access: EMBEDDING_INDEX")
	}

	embeddingDedupBucket, err := js.KeyValue(ctx, "EMBEDDING_DEDUP")
	if err != nil {
		cancel()
		return errs.Wrap(err, "Component", "Start", "KV bucket access: EMBEDDING_DEDUP")
	}

	// Create storage
	c.storage = embedding.NewStorage(embeddingIndexBucket, embeddingDedupBucket)

	// Create worker
	c.worker = embedding.NewWorker(c.storage, c.embedder, embeddingIndexBucket, c.logger).
		WithWorkers(c.config.BatchSize / 10) // Scale workers based on batch size

	// Start worker
	if err := c.worker.Start(ctx); err != nil {
		cancel()
		return errs.Wrap(err, "Component", "Start", "worker start")
	}

	// Mark as running
	c.running = true
	c.startTime = time.Now()

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
