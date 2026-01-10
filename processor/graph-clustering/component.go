// Package graphclustering provides the graph-clustering component for community detection.
package graphclustering

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/c360/semstreams/component"
	"github.com/c360/semstreams/graph"
	"github.com/c360/semstreams/graph/clustering"
	"github.com/c360/semstreams/graph/llm"
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

// Config holds configuration for graph-clustering component
type Config struct {
	Ports                *component.PortConfig `json:"ports" schema:"type:ports,description:Port configuration,category:basic"`
	DetectionIntervalStr string                `json:"detection_interval" schema:"type:string,description:Interval between community detection runs (e.g. 30s or 5m),category:basic"`
	BatchSize            int                   `json:"batch_size" schema:"type:int,description:Event count threshold for triggering detection,category:basic"`
	EnableLLM            bool                  `json:"enable_llm" schema:"type:bool,description:Enable LLM-based community summarization,category:advanced"`
	LLMEndpoint          string                `json:"llm_endpoint" schema:"type:string,description:URL for LLM endpoint (required if enable_llm is true),category:advanced"`
	LLMModel             string                `json:"llm_model" schema:"type:string,description:Model name for LLM service (e.g. mistral-7b-instruct),category:advanced"`
	EnhancementWorkers   int                   `json:"enhancement_workers" schema:"type:int,description:Number of parallel workers for LLM enhancement (default 5),category:advanced"`
	MinCommunitySize     int                   `json:"min_community_size" schema:"type:int,description:Minimum number of entities to form a community,category:advanced"`
	MaxIterations        int                   `json:"max_iterations" schema:"type:int,description:Maximum iterations for LPA algorithm,category:advanced"`

	// Parsed duration (set by ApplyDefaults)
	detectionInterval time.Duration
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

	// Validate COMMUNITY_INDEX output exists
	hasCommunityIndex := false
	for _, output := range c.Ports.Outputs {
		if output.Subject == graph.BucketCommunityIndex {
			hasCommunityIndex = true
			break
		}
	}
	if !hasCommunityIndex {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", fmt.Sprintf("%s output required", graph.BucketCommunityIndex))
	}

	// Validate detection interval (parsed duration must be positive)
	if c.detectionInterval <= 0 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "detection_interval must be greater than 0")
	}

	// If LLM is enabled, endpoint is required (model defaults to service default)
	if c.EnableLLM && c.LLMEndpoint == "" {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "llm_endpoint required when enable_llm is true")
	}

	// Validate min community size
	if c.MinCommunitySize <= 0 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "min_community_size must be greater than 0")
	}

	// Validate max iterations
	if c.MaxIterations <= 0 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "max_iterations must be greater than 0")
	}

	return nil
}

// DetectionInterval returns the parsed detection interval duration
func (c *Config) DetectionInterval() time.Duration {
	return c.detectionInterval
}

// ApplyDefaults sets default values for configuration
func (c *Config) ApplyDefaults() {
	// Parse detection interval from string
	if c.DetectionIntervalStr != "" {
		if d, err := time.ParseDuration(c.DetectionIntervalStr); err == nil {
			c.detectionInterval = d
		}
	}
	if c.detectionInterval == 0 {
		c.detectionInterval = 30 * time.Second
	}

	if c.BatchSize == 0 {
		c.BatchSize = 100
	}
	// EnableLLM defaults to false (zero value)
	if c.MinCommunitySize == 0 {
		c.MinCommunitySize = 3
	}
	if c.MaxIterations == 0 {
		c.MaxIterations = 100
	}
	if c.EnhancementWorkers == 0 {
		c.EnhancementWorkers = 5 // Increased from default 3 for better parallelism
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
					Name:    "communities",
					Type:    "kv-write",
					Subject: graph.BucketCommunityIndex,
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
					Name:    "communities",
					Type:    "kv-write",
					Subject: graph.BucketCommunityIndex,
				},
			},
		},
		detectionInterval: 30 * time.Second,
		BatchSize:         100,
		EnableLLM:         false,
		MinCommunitySize:  3,
		MaxIterations:     100,
	}
}

// schema defines the configuration schema for graph-clustering component
var schema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// Component implements the graph-clustering processor
type Component struct {
	// Component metadata
	name   string
	config Config

	// Dependencies
	natsClient *natsclient.Client
	logger     *slog.Logger

	// Domain resources
	communityBucket jetstream.KeyValue
	entityBucket    jetstream.KeyValue
	outgoingBucket  jetstream.KeyValue
	incomingBucket  jetstream.KeyValue

	// Community detection
	detector clustering.CommunityDetector
	storage  *clustering.NATSCommunityStorage

	// LLM enhancement (optional)
	enhancementWorker *clustering.EnhancementWorker
	llmClient         llm.Client

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

// CreateGraphClustering is the factory function for creating graph-clustering components
func CreateGraphClustering(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	// Validate dependencies
	if deps.NATSClient == nil {
		return nil, errs.WrapInvalid(errs.ErrInvalidConfig, "CreateGraphClustering", "factory", "NATSClient required")
	}
	natsClient := deps.NATSClient

	// Parse configuration
	var config Config
	if len(rawConfig) > 0 {
		if err := json.Unmarshal(rawConfig, &config); err != nil {
			return nil, errs.Wrap(err, "CreateGraphClustering", "factory", "config unmarshal")
		}
	} else {
		config = DefaultConfig()
	}

	// Apply defaults and validate
	config.ApplyDefaults()
	if err := config.Validate(); err != nil {
		return nil, errs.Wrap(err, "CreateGraphClustering", "factory", "config validation")
	}

	// Create logger with component context
	logger := deps.GetLoggerWithComponent("graph-clustering")

	// Create component
	comp := &Component{
		name:       "graph-clustering",
		config:     config,
		natsClient: natsClient,
		logger:     logger,
	}

	// Initialize last activity
	comp.lastActivity.Store(time.Now())

	return comp, nil
}

// Register registers the graph-clustering factory with the component registry
func Register(registry *component.Registry) error {
	return registry.RegisterFactory("graph-clustering", &component.Registration{
		Name:        "graph-clustering",
		Type:        "processor",
		Protocol:    "nats",
		Domain:      "graph",
		Description: "Graph community detection and clustering processor",
		Version:     "1.0.0",
		Schema:      schema,
		Factory:     CreateGraphClustering,
	})
}

// ============================================================================
// Discoverable Interface (6 methods)
// ============================================================================

// Meta returns component metadata
func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        "graph-clustering",
		Type:        "processor",
		Description: "Graph community detection and clustering processor",
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
	c.logger.Info("component initialized", slog.String("component", "graph-clustering"))

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

	// Create COMMUNITY_INDEX bucket (we are the WRITER)
	communityBucket, err := c.natsClient.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{
		Bucket:      graph.BucketCommunityIndex,
		Description: "Community detection index",
	})
	if err != nil {
		cancel()
		if ctx.Err() != nil {
			return errs.Wrap(ctx.Err(), "Component", "Start", "context cancelled during bucket creation")
		}
		return errs.Wrap(err, "Component", "Start", fmt.Sprintf("KV bucket creation: %s", graph.BucketCommunityIndex))
	}
	c.communityBucket = communityBucket

	// Create community storage for the detector
	c.storage = clustering.NewNATSCommunityStorage(communityBucket)

	// Set up query handlers
	if err := c.setupQueryHandlers(ctx); err != nil {
		cancel()
		return errs.Wrap(err, "Component", "Start", "setup query handlers")
	}

	// Get JetStream for bucket access
	js, err := c.natsClient.JetStream()
	if err != nil {
		cancel()
		return errs.Wrap(err, "Component", "Start", "JetStream connection")
	}

	// Wait for input buckets with retries (we are the READER)
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
	c.entityBucket = entityBucket

	outgoingBucket, err := retry.DoWithResult(ctx, retry.Persistent(), func() (jetstream.KeyValue, error) {
		bucket, err := js.KeyValue(ctx, graph.BucketOutgoingIndex)
		if err != nil {
			c.logger.Debug("waiting for input bucket", slog.String("bucket", graph.BucketOutgoingIndex), slog.Any("error", err))
		}
		return bucket, err
	})
	if err != nil {
		cancel()
		return errs.Wrap(err, "Component", "Start", fmt.Sprintf("%s bucket not available after retries", graph.BucketOutgoingIndex))
	}
	c.outgoingBucket = outgoingBucket

	incomingBucket, err := retry.DoWithResult(ctx, retry.Persistent(), func() (jetstream.KeyValue, error) {
		bucket, err := js.KeyValue(ctx, graph.BucketIncomingIndex)
		if err != nil {
			c.logger.Debug("waiting for input bucket", slog.String("bucket", graph.BucketIncomingIndex), slog.Any("error", err))
		}
		return bucket, err
	})
	if err != nil {
		cancel()
		return errs.Wrap(err, "Component", "Start", fmt.Sprintf("%s bucket not available after retries", graph.BucketIncomingIndex))
	}
	c.incomingBucket = incomingBucket

	// Create graph provider and detector
	provider := newKVGraphProvider(c.entityBucket, c.outgoingBucket, c.incomingBucket, c.logger)

	// Optionally wrap with EntityID-based sibling edges for better clustering
	entityIDProvider := clustering.NewEntityIDGraphProvider(
		provider,
		clustering.DefaultEntityIDProviderConfig(),
		c.logger,
	)

	// Create entity querier for summarization and enhancement
	entityQuerier := &kvEntityQuerier{entityBucket: c.entityBucket, logger: c.logger}

	// Create statistical summarizer for immediate summaries
	// This sets SummaryStatus="statistical" which triggers EnhancementWorker
	summarizer := clustering.NewStatisticalSummarizer()

	// Create LPA detector with summarizer and entity provider
	detector := clustering.NewLPADetector(entityIDProvider, c.storage).
		WithLogger(c.logger).
		WithMaxIterations(c.config.MaxIterations).
		WithLevels(3).
		WithSummarizer(summarizer)

	// Set entity provider for summarization (must be called on concrete type)
	detector.SetEntityProvider(entityQuerier)
	c.detector = detector

	// Start LLM enhancement worker if enabled
	if c.config.EnableLLM {
		if err := c.startEnhancementWorker(ctx, provider); err != nil {
			c.logger.Warn("failed to start enhancement worker, continuing without LLM",
				slog.Any("error", err))
		}
	}

	// Mark as running
	c.running = true
	c.startTime = time.Now()

	// Start detection loop goroutine
	c.wg.Add(1)
	go c.runDetectionLoop(ctx)

	c.logger.Info("component started",
		slog.String("component", "graph-clustering"),
		slog.Time("start_time", c.startTime),
		slog.Duration("detection_interval", c.config.DetectionInterval()),
		slog.Bool("enable_llm", c.config.EnableLLM))

	return nil
}

// Stop gracefully shuts down the component
func (c *Component) Stop(timeout time.Duration) error {
	c.mu.Lock()

	if !c.running {
		c.mu.Unlock()
		return nil // Already stopped
	}

	// Stop enhancement worker if running
	if c.enhancementWorker != nil {
		if err := c.enhancementWorker.Stop(); err != nil {
			c.logger.Warn("enhancement worker stop error", slog.Any("error", err))
		}
	}

	// Close LLM client if present
	if c.llmClient != nil {
		if err := c.llmClient.Close(); err != nil {
			c.logger.Warn("LLM client close error", slog.Any("error", err))
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
		c.logger.Info("component stopped gracefully", slog.String("component", "graph-clustering"))
		return nil
	case <-time.After(timeout):
		c.logger.Warn("component stop timed out", slog.String("component", "graph-clustering"))
		return fmt.Errorf("stop timeout after %v", timeout)
	}
}

// runDetectionLoop runs community detection on a timer
func (c *Component) runDetectionLoop(ctx context.Context) {
	defer c.wg.Done()

	ticker := time.NewTicker(c.config.DetectionInterval())
	defer ticker.Stop()

	c.logger.Info("detection loop started",
		slog.Duration("interval", c.config.DetectionInterval()))

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("detection loop stopping")
			return
		case <-ticker.C:
			// Double-check context before starting new detection
			// This prevents starting a new cycle if shutdown just began
			if ctx.Err() != nil {
				c.logger.Info("detection loop stopping - context cancelled")
				return
			}
			c.runCommunityDetection(ctx)
		}
	}
}

// runCommunityDetection executes the community detection algorithm
func (c *Component) runCommunityDetection(ctx context.Context) {
	// Check if context is already cancelled (shutdown in progress)
	if ctx.Err() != nil {
		c.logger.Debug("skipping detection - shutdown in progress")
		return
	}

	c.logger.Info("running community detection")
	start := time.Now()

	communities, err := c.detector.DetectCommunities(ctx)
	if err != nil {
		// Context cancellation during shutdown is expected, not an error
		if errors.Is(err, context.Canceled) {
			c.logger.Info("detection interrupted by shutdown")
			return
		}
		c.logger.Error("community detection failed", slog.Any("error", err))
		atomic.AddInt64(&c.errors, 1)
		return
	}

	// Count total communities across all levels
	totalCommunities := 0
	for _, levelCommunities := range communities {
		totalCommunities += len(levelCommunities)
	}

	atomic.AddInt64(&c.messagesProcessed, int64(totalCommunities))
	c.lastActivity.Store(time.Now())

	c.logger.Info("community detection complete",
		slog.Int("communities_found", totalCommunities),
		slog.Int("levels", len(communities)),
		slog.Duration("duration", time.Since(start)))
}

// ============================================================================
// KV-based Graph Provider for Community Detection
// ============================================================================

// kvGraphProvider implements clustering.GraphProvider using NATS KV buckets
type kvGraphProvider struct {
	entityBucket   jetstream.KeyValue
	outgoingBucket jetstream.KeyValue
	incomingBucket jetstream.KeyValue
	logger         *slog.Logger
}

// newKVGraphProvider creates a graph provider that reads from KV buckets
func newKVGraphProvider(
	entityBucket jetstream.KeyValue,
	outgoingBucket jetstream.KeyValue,
	incomingBucket jetstream.KeyValue,
	logger *slog.Logger,
) *kvGraphProvider {
	return &kvGraphProvider{
		entityBucket:   entityBucket,
		outgoingBucket: outgoingBucket,
		incomingBucket: incomingBucket,
		logger:         logger,
	}
}

// GetAllEntityIDs returns all entity IDs from the ENTITY_STATES bucket
func (p *kvGraphProvider) GetAllEntityIDs(ctx context.Context) ([]string, error) {
	keys, err := p.entityBucket.Keys(ctx)
	if err != nil {
		// Empty bucket returns an error in some cases
		if err == jetstream.ErrNoKeysFound {
			return nil, nil
		}
		return nil, errs.WrapTransient(err, "kvGraphProvider", "GetAllEntityIDs", "list keys")
	}
	return keys, nil
}

// GetNeighbors returns entity IDs connected to the given entity
func (p *kvGraphProvider) GetNeighbors(ctx context.Context, entityID string, direction string) ([]string, error) {
	if entityID == "" {
		return nil, errs.WrapInvalid(errs.ErrInvalidConfig, "kvGraphProvider", "GetNeighbors", "entityID is empty")
	}

	neighbors := make(map[string]bool)

	// Get outgoing neighbors
	if direction == "outgoing" || direction == "both" {
		outgoing, err := p.getNeighborsFromBucket(ctx, p.outgoingBucket, entityID)
		if err != nil {
			p.logger.Debug("failed to get outgoing neighbors", slog.String("entity", entityID), slog.Any("error", err))
		}
		for _, n := range outgoing {
			neighbors[n] = true
		}
	}

	// Get incoming neighbors
	if direction == "incoming" || direction == "both" {
		incoming, err := p.getNeighborsFromBucket(ctx, p.incomingBucket, entityID)
		if err != nil {
			p.logger.Debug("failed to get incoming neighbors", slog.String("entity", entityID), slog.Any("error", err))
		}
		for _, n := range incoming {
			neighbors[n] = true
		}
	}

	result := make([]string, 0, len(neighbors))
	for n := range neighbors {
		result = append(result, n)
	}
	return result, nil
}

// relationshipEntry represents a relationship in the index buckets
type relationshipEntry struct {
	Predicate    string `json:"predicate"`
	ToEntityID   string `json:"to_entity_id,omitempty"`   // For OUTGOING_INDEX
	FromEntityID string `json:"from_entity_id,omitempty"` // For INCOMING_INDEX
}

// getNeighborsFromBucket reads neighbor entity IDs from a relationship index bucket
func (p *kvGraphProvider) getNeighborsFromBucket(ctx context.Context, bucket jetstream.KeyValue, entityID string) ([]string, error) {
	entry, err := bucket.Get(ctx, entityID)
	if err != nil {
		if err == jetstream.ErrKeyNotFound {
			return nil, nil // No neighbors found
		}
		return nil, err
	}

	// Parse the index entry - format is a list of relationship entries
	var relationships []relationshipEntry
	if err := json.Unmarshal(entry.Value(), &relationships); err != nil {
		return nil, err
	}

	neighbors := make([]string, 0, len(relationships))
	for _, rel := range relationships {
		// Use whichever ID field is populated
		if rel.ToEntityID != "" {
			neighbors = append(neighbors, rel.ToEntityID)
		} else if rel.FromEntityID != "" {
			neighbors = append(neighbors, rel.FromEntityID)
		}
	}
	return neighbors, nil
}

// GetEdgeWeight returns the weight of the edge between two entities
func (p *kvGraphProvider) GetEdgeWeight(_ context.Context, _, _ string) (float64, error) {
	// For now, return 1.0 for all edges (equal weight)
	// Could be enhanced to read confidence from the relationship data
	return 1.0, nil
}

// ============================================================================
// LLM Enhancement Support
// ============================================================================

// startEnhancementWorker initializes and starts the LLM enhancement worker
func (c *Component) startEnhancementWorker(ctx context.Context, provider clustering.GraphProvider) error {
	// Use default model if not specified (LLM service provides its own default)
	model := c.config.LLMModel
	if model == "" {
		model = "default" // LLM service will use its configured default
	}

	// Create LLM client
	llmClient, err := llm.NewOpenAIClient(llm.OpenAIConfig{
		BaseURL: c.config.LLMEndpoint,
		Model:   model,
		Logger:  c.logger,
	})
	if err != nil {
		return errs.Wrap(err, "Component", "startEnhancementWorker", "create LLM client")
	}
	c.llmClient = llmClient

	// Create LLM summarizer
	llmSummarizer, err := clustering.NewLLMSummarizer(clustering.LLMSummarizerConfig{
		Client:    llmClient,
		MaxTokens: 200,
	})
	if err != nil {
		llmClient.Close()
		return errs.Wrap(err, "Component", "startEnhancementWorker", "create LLM summarizer")
	}

	// Create entity querier from entity bucket
	querier := newKVEntityQuerier(c.entityBucket, c.logger)

	// Create enhancement worker
	worker, err := clustering.NewEnhancementWorker(&clustering.EnhancementWorkerConfig{
		LLMSummarizer:   llmSummarizer,
		Storage:         c.storage,
		GraphProvider:   provider,
		Querier:         querier,
		CommunityBucket: c.communityBucket,
		Logger:          c.logger,
	})
	if err != nil {
		llmClient.Close()
		return errs.Wrap(err, "Component", "startEnhancementWorker", "create enhancement worker")
	}

	// Configure worker parallelism
	worker.WithWorkers(c.config.EnhancementWorkers)

	// Start the worker
	if err := worker.Start(ctx); err != nil {
		llmClient.Close()
		return errs.Wrap(err, "Component", "startEnhancementWorker", "start enhancement worker")
	}

	c.enhancementWorker = worker
	c.logger.Info("LLM enhancement worker started",
		slog.String("endpoint", c.config.LLMEndpoint),
		slog.String("model", c.config.LLMModel),
		slog.Int("workers", c.config.EnhancementWorkers))

	return nil
}

// ============================================================================
// KV-based Entity Querier for Enhancement Worker
// ============================================================================

// kvEntityQuerier implements clustering.EntityQuerier using NATS KV
type kvEntityQuerier struct {
	entityBucket jetstream.KeyValue
	logger       *slog.Logger
}

// newKVEntityQuerier creates an entity querier that reads from ENTITY_STATES
func newKVEntityQuerier(entityBucket jetstream.KeyValue, logger *slog.Logger) *kvEntityQuerier {
	return &kvEntityQuerier{
		entityBucket: entityBucket,
		logger:       logger,
	}
}

// GetEntities retrieves entities by their IDs from ENTITY_STATES bucket
func (q *kvEntityQuerier) GetEntities(ctx context.Context, ids []string) ([]*graph.EntityState, error) {
	entities := make([]*graph.EntityState, 0, len(ids))

	for _, id := range ids {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		entry, err := q.entityBucket.Get(ctx, id)
		if err != nil {
			if err == jetstream.ErrKeyNotFound {
				q.logger.Debug("entity not found", slog.String("id", id))
				continue
			}
			return nil, errs.WrapTransient(err, "kvEntityQuerier", "GetEntities", "get entity")
		}

		var entity graph.EntityState
		if err := json.Unmarshal(entry.Value(), &entity); err != nil {
			q.logger.Warn("failed to unmarshal entity", slog.String("id", id), slog.Any("error", err))
			continue
		}

		entities = append(entities, &entity)
	}

	return entities, nil
}
