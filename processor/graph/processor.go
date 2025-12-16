// Package graph provides a graph processor component for processing messages into entity states
package graph

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"

	"github.com/c360/semstreams/component"
	"github.com/c360/semstreams/config"
	gtypes "github.com/c360/semstreams/graph"
	"github.com/c360/semstreams/message"
	"github.com/c360/semstreams/metric"
	"github.com/c360/semstreams/natsclient"
	"github.com/c360/semstreams/pkg/cache"
	"github.com/c360/semstreams/pkg/errs"
	"github.com/c360/semstreams/pkg/worker"
	"github.com/c360/semstreams/processor/graph/clustering"
	"github.com/c360/semstreams/processor/graph/datamanager"
	"github.com/c360/semstreams/processor/graph/indexmanager"
	"github.com/c360/semstreams/processor/graph/llm"
	"github.com/c360/semstreams/processor/graph/messagemanager"
	"github.com/c360/semstreams/processor/graph/querymanager"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus"
)

// Clustering configuration defaults
const (
	DefaultMaxIterations        = 100
	DefaultClusteringLevels     = 3
	DefaultEnhancementWorkers   = 3
	DefaultSimilarityThreshold  = 0.6
	DefaultMaxVirtualNeighbors  = 5
	DefaultEntityChangeThreshold = 100
)

// schema defines the configuration schema for graph processor component
// Generated from Config struct tags using reflection
var schema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// Processor orchestrates graph processing services - no business logic, pure coordination
type Processor struct {
	// Component interface implementation
	metadata    component.Metadata
	inputPorts  []component.Port
	outputPorts []component.Port
	health      component.HealthStatus
	startTime   time.Time // Track when component started for uptime calculation

	// Infrastructure dependencies
	natsClient      *natsclient.Client
	logger          *slog.Logger
	metricsRegistry *metric.MetricsRegistry

	// Business service (depends on core modules)
	messageManager messagemanager.MessageHandler // Complex message→entity transformation

	// Core modules (foundational data management)
	dataManager   *datamanager.Manager      // Concrete manager instance (for tests needing full access)
	dataLifecycle datamanager.DataLifecycle // Lifecycle management (Run, FlushPendingWrites, etc.)
	entityManager datamanager.EntityManager // Complete entity operations (passed to sub-components)
	tripleManager datamanager.TripleManager // Semantic triple operations (replaces EdgeManager)
	indexManager  indexmanager.Indexer      // KV watching, secondary indexes
	queryManager  querymanager.Querier      // Query operations with caching

	// Optimization caches
	entityCache cache.Cache[*gtypes.EntityState]
	aliasCache  cache.Cache[string]

	// Worker management
	workerPool *worker.Pool[[]byte]

	// Background modules management
	moduleCancel context.CancelFunc
	moduleDone   chan error

	// Rate limiting for queries (DoS protection)
	queryLimiter *rate.Limiter

	// Synchronization
	mu sync.RWMutex

	// Configuration
	config *Config

	// Clustering components (optional, initialized if config.Clustering.Enabled)
	communityDetector clustering.CommunityDetector
	communityStorage  clustering.CommunityStorage
	enhancementWorker *clustering.EnhancementWorker
	clusteringBuckets map[string]jetstream.KeyValue // For graph provider access
	detectionMu       sync.Mutex
	detectionRunning  bool

	// LLM content fetching (optional, for enriched prompts)
	contentFetcher llm.ContentFetcher

	// Entity change tracking for adaptive clustering
	entityChangeCount atomic.Int64  // Count of new entities since last detection
	detectionTrigger  chan struct{} // Signal to trigger detection from entity callback

	// Enhancement window state - prevents re-detection from overwriting LLM-enhanced communities
	enhancementDeadline time.Time            // When the enhancement window expires
	enhancementMode     EnhancementWindowMode // Mode for enhancement window behavior

	// Inference metrics
	inferredTriples prometheus.Counter
}

// Config holds processor configuration
type Config struct {
	Workers int `json:"workers"       schema:"type:int,description:Number of worker goroutines,default:10,category:basic"`

	QueueSize int `json:"queue_size"    schema:"type:int,description:Worker queue size,default:10000,category:basic"`

	InputSubject string `json:"input_subject" schema:"type:string,description:NATS subject to subscribe for input messages,default:events.graph.entity.*,category:basic"`

	// InputSubjects supports multiple input subjects for multi-stream subscription.
	// Each subject is mapped to its stream using convention: subject "component.action.type" → stream "COMPONENT"
	InputSubjects []string `json:"input_subjects,omitempty" schema:"type:array,description:Multiple NATS subjects to subscribe (derives streams from convention),category:basic"`

	// JetStream configuration for durable message consumption
	// StreamName is deprecated in favor of InputSubjects with convention-derived streams
	StreamName     string   `json:"stream_name,omitempty"     schema:"type:string,description:JetStream stream name for durable consumption (deprecated: use input_subjects),category:advanced"`
	StreamSubjects []string `json:"stream_subjects,omitempty" schema:"type:array,description:JetStream stream subjects (defaults to input_subject pattern),category:advanced"`
	ConsumerName   string   `json:"consumer_name,omitempty"   schema:"type:string,description:JetStream consumer name (durable if set),category:advanced"`

	// Component configurations

	MessageHandler *messagemanager.Config `json:"message_handler,omitempty" schema:"type:object,description:Message handler configuration,category:advanced"`

	DataManager *datamanager.Config `json:"data_manager,omitempty"    schema:"type:object,description:Data manager configuration,category:advanced"`

	Indexer *indexmanager.Config `json:"indexer,omitempty"         schema:"type:object,description:Index manager configuration,category:advanced"`

	Querier *querymanager.Config `json:"querier,omitempty"         schema:"type:object,description:Query manager configuration,category:advanced"`

	// Clustering configures community detection
	Clustering *ClusteringConfig `json:"clustering,omitempty" schema:"type:object,description:Community detection configuration,category:advanced"`
}

// ClusteringConfig configures community detection behavior
type ClusteringConfig struct {
	// Enabled controls whether community detection is active
	Enabled bool `json:"enabled" schema:"type:bool,description:Enable community detection,default:false"`

	// Algorithm configures the detection algorithm
	Algorithm AlgorithmConfig `json:"algorithm,omitempty"`

	// Schedule configures detection timing
	Schedule ScheduleConfig `json:"schedule,omitempty"`

	// Enhancement configures LLM summarization
	Enhancement EnhancementConfig `json:"enhancement,omitempty"`

	// SemanticEdges configures virtual edges based on embedding similarity
	// This enables community detection even when explicit relationship triples don't exist
	SemanticEdges SemanticEdgesConfig `json:"semantic_edges,omitempty"`

	// Inference configures relationship inference from community detection
	Inference InferenceConfig `json:"inference,omitempty"`
}

// SemanticEdgesConfig configures virtual edge creation from embedding similarity
type SemanticEdgesConfig struct {
	// Enabled activates semantic-based virtual edges for clustering
	// When true, entities with similar embeddings are treated as neighbors
	Enabled bool `json:"enabled" schema:"type:bool,description:Enable semantic virtual edges,default:false"`

	// SimilarityThreshold is the minimum cosine similarity for virtual edges (0.0-1.0)
	// Higher values = fewer but stronger virtual connections
	// Default: 0.6 (stricter than search threshold of 0.3)
	SimilarityThreshold float64 `json:"similarity_threshold" schema:"type:float,description:Min similarity for virtual edge,default:0.6"`

	// MaxVirtualNeighbors limits virtual neighbors per entity
	// Controls computation cost during LPA iterations
	// Default: 5
	MaxVirtualNeighbors int `json:"max_virtual_neighbors" schema:"type:int,description:Max virtual neighbors per entity,default:5"`
}

// InferenceConfig configures relationship inference from clustering results
type InferenceConfig struct {
	// Enabled activates triple inference from community detection
	// When true, co-membership in communities creates inferred.clustered_with triples
	Enabled bool `json:"enabled" schema:"type:bool,description:Enable relationship inference,default:false"`

	// MinCommunitySize is the minimum community size for inference
	// Singleton communities (size=1) never produce inferences
	// Default: 2
	MinCommunitySize int `json:"min_community_size" schema:"type:int,description:Min community size for inference,default:2"`

	// MaxInferredPerCommunity limits inferred relationships per community
	// Prevents O(n²) explosion in large communities
	// Default: 50
	MaxInferredPerCommunity int `json:"max_inferred_per_community" schema:"type:int,description:Max inferred relationships per community,default:50"`
}

// AlgorithmConfig configures the LPA detector
type AlgorithmConfig struct {
	// MaxIterations limits LPA iterations
	MaxIterations int `json:"max_iterations" schema:"type:int,description:Max LPA iterations,default:100"`

	// Levels is hierarchical community levels
	Levels int `json:"levels" schema:"type:int,description:Hierarchical levels,default:3"`
}

// EnhancementWindowMode controls detection behavior during LLM enhancement
type EnhancementWindowMode string

const (
	// WindowModeBlocking pauses detection until window expires or all communities reach terminal status
	WindowModeBlocking EnhancementWindowMode = "blocking"
	// WindowModeSoft allows detection if significant entity changes occur during window
	WindowModeSoft EnhancementWindowMode = "soft"
	// WindowModeNone disables enhancement window (original behavior)
	WindowModeNone EnhancementWindowMode = "none"
)

// ScheduleConfig configures detection timing
type ScheduleConfig struct {
	// InitialDelay before first detection
	InitialDelay string `json:"initial_delay" schema:"type:string,description:Delay before first detection,default:10s"`

	// DetectionInterval is the maximum time between detection runs (triggers even if no new entities)
	DetectionInterval string `json:"detection_interval" schema:"type:string,description:Max interval between detection runs,default:30s"`

	// MinDetectionInterval is the minimum time between detection runs (burst protection)
	MinDetectionInterval string `json:"min_detection_interval" schema:"type:string,description:Min interval between detection runs,default:5s"`

	// EntityChangeThreshold triggers detection after N new entities arrive (0 disables)
	EntityChangeThreshold int `json:"entity_change_threshold" schema:"type:int,description:Trigger detection after N new entities,default:100"`

	// MinEntities threshold for triggering detection
	MinEntities int `json:"min_entities" schema:"type:int,description:Min entities for detection,default:10"`

	// MinEmbeddingCoverage is the minimum ratio of embeddings to entities required for semantic clustering.
	// When semantic_edges is enabled, clustering will be skipped until this coverage threshold is met.
	// This prevents clustering from running before embeddings are generated.
	// Range: 0.0-1.0, Default: 0.5 (50%)
	MinEmbeddingCoverage float64 `json:"min_embedding_coverage" schema:"type:float,description:Min embedding coverage for semantic clustering (0.0-1.0),default:0.5"`

	// EnhancementWindow is the duration to pause detection after clustering to allow LLM enhancement.
	// During this window, re-detection is paused to prevent overwriting LLM-enhanced communities.
	// Set to "0" or empty to disable (original behavior).
	// Default: 0 (disabled)
	EnhancementWindow string `json:"enhancement_window" schema:"type:string,description:Pause detection duration for LLM enhancement,default:0"`

	// EnhancementWindowMode controls how the enhancement window behaves:
	// - "blocking": Hard pause until window expires or all communities reach terminal status (llm-enhanced/llm-failed)
	// - "soft": Allow detection if entity changes exceed threshold during window
	// - "none": Disable enhancement window (original behavior)
	// Default: "blocking"
	EnhancementWindowMode EnhancementWindowMode `json:"enhancement_window_mode" schema:"type:string,description:Enhancement window mode (blocking|soft|none),default:blocking"`
}

// EnhancementConfig configures LLM summary enhancement
type EnhancementConfig struct {
	// Enabled activates the enhancement worker
	Enabled bool `json:"enabled" schema:"type:bool,description:Enable LLM enhancement,default:false"`

	// LLM configures the LLM client for summarization
	LLM llm.Config `json:"llm,omitempty" schema:"type:object,description:LLM configuration"`

	// Workers is concurrent enhancement workers
	Workers int `json:"workers" schema:"type:int,description:Concurrent workers,default:3"`

	// Domain for prompt selection (e.g., "iot", "default")
	Domain string `json:"domain,omitempty" schema:"type:string,description:Prompt domain,default:default"`
}

// ProcessorDeps holds processor dependencies
type ProcessorDeps struct {
	Config          *Config
	NATSClient      *natsclient.Client
	MetricsRegistry *metric.MetricsRegistry
	Logger          *slog.Logger
}

// NewProcessor creates a new graph processor instance
func NewProcessor(deps ProcessorDeps) (*Processor, error) {
	if deps.NATSClient == nil {
		return nil, errs.WrapFatal(errs.ErrNoConnection, "graph processor", "NewProcessor", "NATS client required")
	}

	if deps.Config == nil {
		deps.Config = DefaultConfig()
	}

	p := &Processor{
		natsClient:      deps.NATSClient,
		logger:          deps.Logger,
		metricsRegistry: deps.MetricsRegistry,
		config:          deps.Config,
		queryLimiter:    rate.NewLimiter(rate.Limit(100), 10), // 100 queries/sec with burst of 10
		metadata: component.Metadata{
			Name:    "graph",
			Type:    "semantic-processor",
			Version: "1.0.0",
		},
		inputPorts: []component.Port{
			{
				Name:        "entities_input",
				Direction:   component.DirectionInput,
				Description: "Entity events from upstream processors",
				Required:    true,
				Config: component.NATSPort{
					Subject: "events.graph.entity.*",
					Interface: &component.InterfaceContract{
						Type:    "graph.Entity",
						Version: "v1",
					},
				},
			},
			{
				Name:        "mutations_api",
				Direction:   component.DirectionInput,
				Description: "Request/Reply API for graph mutations",
				Required:    false,
				Config: component.NATSRequestPort{
					Subject: "graph.mutations",
					Timeout: "500ms",
				},
			},
		},
		outputPorts: []component.Port{
			{
				Name:        "entity_states",
				Direction:   component.DirectionOutput,
				Description: "Writes entity states to ENTITY_STATES KV bucket",
				Required:    false,
				Config: component.KVWritePort{
					Bucket: "ENTITY_STATES",
					Interface: &component.InterfaceContract{
						Type:    "graph.EntityState",
						Version: "v1",
					},
				},
			},
			{
				Name:        "predicate_index",
				Direction:   component.DirectionOutput,
				Description: "Writes predicate indexes to PREDICATE_INDEX KV bucket",
				Required:    false,
				Config: component.KVWritePort{
					Bucket: "PREDICATE_INDEX",
					Interface: &component.InterfaceContract{
						Type:    "graph.PredicateEntry",
						Version: "v1",
					},
				},
			},
			{
				Name:        "entities_output",
				Direction:   component.DirectionOutput,
				Description: "Processed entity events for downstream consumers",
				Required:    false,
				Config: component.NATSPort{
					Subject: "events.graph.processed",
					Interface: &component.InterfaceContract{
						Type:    "graph.Entity",
						Version: "v1",
					},
				},
			},
		},
		health: component.HealthStatus{
			Healthy:    true,
			LastCheck:  time.Now(),
			ErrorCount: 0,
		},
	}

	return p, nil
}

// DefaultConfig returns default processor configuration
func DefaultConfig() *Config {
	return &Config{
		Workers:      10,
		QueueSize:    10000,
		InputSubject: "storage.*.events", // Subscribe to ObjectStore events

		// Enable sophisticated components by default
		DataManager: func() *datamanager.Config {
			config := datamanager.DefaultConfig()
			return &config
		}(),

		Querier: func() *querymanager.Config {
			config := querymanager.Config{}
			config.SetDefaults()
			return &config
		}(),
	}
}

// Component Interface Implementation

// Meta returns the component metadata.
func (p *Processor) Meta() component.Metadata {
	return p.metadata
}

// InputPorts returns the component's input ports.
func (p *Processor) InputPorts() []component.Port {
	return p.inputPorts
}

// OutputPorts returns the component's output ports.
func (p *Processor) OutputPorts() []component.Port {
	return p.outputPorts
}

// Health returns the current health status of the processor.
func (p *Processor) Health() component.HealthStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Calculate uptime if component has started
	health := p.health
	if !p.startTime.IsZero() {
		health.Uptime = time.Since(p.startTime)
	}

	return health
}

// IsReady checks if all services are ready to handle requests
// This is essential for both production monitoring and testing
func (p *Processor) IsReady() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Check if services are initialized (protected by RLock)
	if p.entityManager == nil {
		return false
	}

	if p.indexManager == nil {
		return false
	}

	if p.queryManager == nil {
		return false
	}

	// Worker pool must exist
	if p.workerPool == nil {
		return false
	}

	// Check health status
	return p.health.Healthy
}

// WaitForReady waits for all services to be ready with timeout
func (p *Processor) WaitForReady(timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	checkTimer := time.NewTimer(100 * time.Millisecond)
	defer checkTimer.Stop()

	for {
		if p.IsReady() {
			return nil
		}

		select {
		case <-timer.C:
			status := p.GetReadinessDetails()
			msg := fmt.Errorf("processor not ready after %v: %s", timeout, status)
			return errs.WrapFatal(msg, "Processor", "waitReady", "ready timeout exceeded")
		case <-checkTimer.C:
			checkTimer.Reset(100 * time.Millisecond)
		}
	}
}

// GetReadinessDetails returns detailed status of all components
func (p *Processor) GetReadinessDetails() string {
	details := []string{}

	if p.entityManager != nil {
		details = append(details, "DataManager: initialized")
	} else {
		details = append(details, "DataManager: not initialized")
	}

	if p.indexManager != nil {
		details = append(details, "IndexManager: initialized")
	} else {
		details = append(details, "IndexManager: not initialized")
	}

	if p.queryManager != nil {
		details = append(details, "QueryManager: initialized")
	} else {
		details = append(details, "QueryManager: not initialized")
	}

	if p.workerPool != nil {
		details = append(details, "WorkerPool: initialized")
	} else {
		details = append(details, "WorkerPool: not initialized")
	}

	p.mu.RLock()
	details = append(details, fmt.Sprintf("Health: %v", p.health.Healthy))
	p.mu.RUnlock()

	return strings.Join(details, ", ")
}

// GetQueryManager returns the query manager instance for external use
// Returns nil if the query manager is not initialized
func (p *Processor) GetQueryManager() querymanager.Querier {
	return p.queryManager
}

// ConfigSchema returns the configuration schema for this component.
func (p *Processor) ConfigSchema() component.ConfigSchema {
	return schema
}

// DataFlow returns flow metrics for the component.
func (p *Processor) DataFlow() component.FlowMetrics {
	// Return basic flow metrics - could be enhanced with real data
	return component.FlowMetrics{
		MessagesPerSecond: 0.0, // TODO: implement real metrics
		BytesPerSecond:    0.0,
		ErrorRate:         0.0,
		LastActivity:      time.Now(),
	}
}

// Initialize sets up in-memory structures only - no external resources
// Per component lifecycle contract: external resources go in Start(ctx)
func (p *Processor) Initialize() error {
	// Initialize only in-memory caches
	if err := p.initializeCaches(); err != nil {
		return errs.WrapFatal(err, "Processor", "Initialize", "cache initialization failed")
	}

	// Do NOT initialize engines here - they need KV buckets which require context
	// Services will be initialized in Start() where we have context

	p.mu.Lock()
	p.health.Healthy = false // Not healthy until Start() completes
	p.health.LastCheck = time.Now()
	p.mu.Unlock()

	return nil
}

// Start blocks until shutdown - required by component interface
func (p *Processor) Start(ctx context.Context) error {
	p.logger.Info("Starting graph processor")
	p.logger.Info("==== CRITICAL: Entered Start() method ====")

	// Wait for JetStream streams to exist (created by StreamsManager or publisher components)
	// Multi-stream mode: wait for all convention-derived streams
	// Legacy mode: wait for single configured stream
	if len(p.config.InputSubjects) > 0 {
		if err := p.waitForInputSubjectStreams(ctx); err != nil {
			return err
		}
	} else if p.config.StreamName != "" {
		if err := p.waitForStream(ctx, p.config.StreamName); err != nil {
			p.logger.Error("Stream not available", "stream", p.config.StreamName, "error", err)
			return errs.WrapFatal(err, "Processor", "Start", "JetStream stream not available")
		}
	}

	// Initialize core modules with context (DataManager, IndexManager, QueryManager)
	p.logger.Info("Initializing core modules (DataManager, IndexManager, QueryManager)")
	if err := p.initializeModules(ctx); err != nil {
		p.logger.Error("Failed to initialize modules", "error", err)
		return errs.WrapFatal(err, "Processor", "Start", "module initialization failed")
	}
	p.logger.Debug("Core modules initialized successfully")

	// Initialize business services (MessageManager depends on core modules)
	p.logger.Debug("Initializing business services (MessageManager)")
	if err := p.initializeBusinessServices(); err != nil {
		p.logger.Error("Failed to initialize business services", "error", err)
		return errs.WrapFatal(err, "Processor", "Start", "business services initialization failed")
	}
	p.logger.Debug("Business services initialized successfully")

	// Setup worker pool and NATS handlers
	if err := p.setupWorkerPoolAndHandlers(ctx); err != nil {
		return err
	}

	// Start background modules FIRST (DataManager and IndexManager)
	// They must be running before we subscribe to NATS to avoid data loss
	p.logger.Debug("Starting background modules (DataManager and IndexManager)")
	if err := p.startBackgroundModules(ctx); err != nil {
		return errs.WrapFatal(err, "Processor", "Start", "subsystems not ready")
	}
	p.logger.Debug("Background modules started and ready")

	// NOW setup NATS subscriptions - subsystems ready to handle data
	if err := p.setupNATSSubscriptions(ctx); err != nil {
		return err
	}

	// Mark component as healthy LAST
	p.markComponentHealthy()

	p.logger.Info("Graph processor started successfully - all subsystems ready")

	return nil
}

// setupWorkerPoolAndHandlers creates worker pool and sets up NATS handlers
func (p *Processor) setupWorkerPoolAndHandlers(ctx context.Context) error {
	// Initialize worker pool (always create a fresh instance)
	p.logger.Debug("Creating worker pool", "workers", p.config.Workers, "queue_size", p.config.QueueSize)
	workerPool := worker.NewPool(
		p.config.Workers,
		p.config.QueueSize,
		p.messageManager.ProcessWork,
		worker.WithMetricsRegistry[[]byte](p.metricsRegistry, "graph_processor"),
	)

	// Assign with proper locking
	p.mu.Lock()
	p.workerPool = workerPool
	p.mu.Unlock()
	p.logger.Debug("Worker pool created and assigned")

	// Setup NATS mutation handlers before starting services
	p.logger.Debug("Setting up NATS mutation handlers")
	if err := p.setupMutationHandlers(ctx); err != nil {
		p.logger.Error("Failed to setup mutation handlers", "error", err)
		return errs.WrapFatal(err, "Processor", "Start", "NATS mutation handlers setup failed")
	}
	p.logger.Debug("NATS mutation handlers setup complete")

	// Setup NATS query handlers for request/reply pattern
	p.logger.Debug("Setting up NATS query handlers")
	if err := p.setupQueryHandlers(ctx); err != nil {
		p.logger.Error("Failed to setup query handlers", "error", err)
		return errs.WrapFatal(err, "Processor", "Start", "NATS query handlers setup failed")
	}
	p.logger.Debug("NATS query handlers setup complete")

	// Start worker pool
	p.logger.Debug("Starting worker pool")
	if err := p.workerPool.Start(ctx); err != nil {
		p.logger.Error("Failed to start worker pool", "error", err)
		return errs.WrapFatal(err, "Processor", "Start", "worker pool startup failed")
	}
	p.logger.Debug("Worker pool started successfully")

	return nil
}

// setupNATSSubscriptions sets up NATS subscriptions with cleanup on failure
func (p *Processor) setupNATSSubscriptions(ctx context.Context) error {
	p.logger.Debug("Setting up NATS subscriptions")
	if err := p.setupSubscriptions(ctx); err != nil {
		p.logger.Error("Failed to setup subscriptions", "error", err)
		// Stop worker pool before returning
		p.workerPool.Stop(5 * time.Second)
		return errs.WrapFatal(err, "Processor", "Start", "NATS subscriptions setup failed")
	}
	p.logger.Debug("NATS subscriptions setup complete")
	return nil
}

// markComponentHealthy marks the component as healthy and records start time
func (p *Processor) markComponentHealthy() {
	p.logger.Debug("Marking component as healthy")
	p.mu.Lock()
	p.health.Healthy = true
	p.health.LastCheck = time.Now()
	p.startTime = time.Now() // Record when component became healthy
	p.mu.Unlock()
	p.logger.Debug("Component marked as healthy", "start_time", p.startTime)
}

// startBackgroundModules starts DataManager and IndexManager in background goroutines
// and waits for them to signal ready (or returns error on timeout/failure).
func (p *Processor) startBackgroundModules(ctx context.Context) error {
	// Create cancellable context for background modules
	moduleCtx, moduleCancel := context.WithCancel(ctx)

	p.mu.Lock()
	p.moduleCancel = moduleCancel
	p.moduleDone = make(chan error, 1)
	p.mu.Unlock()

	// Track startup readiness
	var wg sync.WaitGroup
	var startErr atomic.Pointer[error]

	wg.Add(2) // DataManager + IndexManager

	// Start background modules in goroutine
	go func() {
		defer func() {
			// Ensure we clean up on exit
			p.mu.Lock()
			p.health.Healthy = false
			p.health.LastCheck = time.Now()
			p.startTime = time.Time{} // Reset start time when stopping
			p.mu.Unlock()
		}()

		// Create error group for modules
		g, gctx := errgroup.WithContext(moduleCtx)

		// Launch DataManager with onReady callback
		g.Go(func() error {
			err := p.dataLifecycle.Run(gctx, func() {
				p.logger.Debug("DataManager ready")
				wg.Done()
			})
			if err != nil {
				// If we fail before signaling ready, decrement WaitGroup
				startErr.CompareAndSwap(nil, &err)
			}
			return err
		})

		// Launch IndexManager with onReady callback
		g.Go(func() error {
			err := p.indexManager.Run(gctx, func() {
				p.logger.Debug("IndexManager ready")
				wg.Done()
			})
			if err != nil {
				startErr.CompareAndSwap(nil, &err)
			}
			return err
		})

		// Launch clustering loop if enabled
		if p.config.Clustering != nil && p.config.Clustering.Enabled && p.communityDetector != nil {
			g.Go(func() error {
				return p.runClusteringLoop(gctx)
			})

			// Launch enhancement worker if configured
			if p.enhancementWorker != nil {
				g.Go(func() error {
					return p.runEnhancementWorker(gctx)
				})
			}
		}

		// Wait for modules to complete or error
		err := g.Wait()
		if err != nil && !stderrors.Is(err, context.Canceled) {
			p.logger.Error("Background module error", "error", err)
		}

		// Signal completion
		p.mu.Lock()
		if p.moduleDone != nil {
			select {
			case p.moduleDone <- err:
			default:
				// Channel full or closed, ignore
			}
		}
		p.mu.Unlock()
	}()

	// Wait for both subsystems to signal ready (with timeout)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	readyTimeout := 30 * time.Second
	select {
	case <-done:
		// Check if any startup error occurred
		if errPtr := startErr.Load(); errPtr != nil {
			return fmt.Errorf("subsystem startup failed: %w", *errPtr)
		}
		p.logger.Info("All subsystems ready")
		return nil
	case <-time.After(readyTimeout):
		moduleCancel() // Cancel the modules if timeout
		return fmt.Errorf("subsystems failed to start within %v", readyTimeout)
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Stop waits for graceful cleanup
func (p *Processor) Stop(timeout time.Duration) error {
	p.logger.Info("Waiting for graceful shutdown", "timeout", timeout)

	// Stop background modules first
	p.mu.Lock()
	moduleCancel := p.moduleCancel
	moduleDone := p.moduleDone
	p.mu.Unlock()

	if moduleCancel != nil {
		// Cancel background modules
		moduleCancel()

		// Wait for modules to stop with timeout
		if moduleDone != nil {
			select {
			case <-moduleDone:
				p.logger.Info("Background modules stopped")
			case <-time.After(timeout):
				p.logger.Warn("Background modules stop timeout", "timeout", timeout)
			}
		}

		// Clean up
		p.mu.Lock()
		p.moduleCancel = nil
		if p.moduleDone != nil {
			close(p.moduleDone)
			p.moduleDone = nil
		}
		p.mu.Unlock()
	}

	// Stop worker pool
	if p.workerPool != nil {
		if err := p.workerPool.Stop(timeout); err != nil {
			p.logger.Warn("Worker pool stop timeout", "error", err, "timeout", timeout)
		}
		// Clear the reference to ensure clean state for next test
		p.workerPool = nil
	}

	return nil
}

// Private initialization methods

func (p *Processor) initializeCaches() error {
	// Create entity cache with LRU eviction
	var err error
	p.entityCache, err = cache.NewLRU[*gtypes.EntityState](10000)
	if err != nil {
		return errs.WrapTransient(err, "GraphProcessor", "initializeCaches", "entity cache creation")
	}

	// Create alias cache with LRU eviction
	p.aliasCache, err = cache.NewLRU[string](1000)
	if err != nil {
		return errs.WrapTransient(err, "GraphProcessor", "initializeCaches", "alias cache creation")
	}

	return nil
}

func (p *Processor) initializeModules(ctx context.Context) error {
	p.logger.Debug("Starting module initialization")

	// Get entity states bucket and initialize DataManager
	kvBucket, err := p.createKVBucket(ctx, "ENTITY_STATES", "Entity state storage")
	if err != nil {
		return err
	}

	dataHandler, err := p.initializeDataManager(kvBucket)
	if err != nil {
		return err
	}

	// Create all index buckets and initialize IndexManager
	buckets, err := p.createIndexBuckets(ctx, kvBucket)
	if err != nil {
		return err
	}

	indexer, err := p.initializeIndexManager(buckets)
	if err != nil {
		return err
	}

	// Initialize QueryManager
	querier, err := p.initializeQueryManager(dataHandler, indexer)
	if err != nil {
		return err
	}

	// Assign all managers atomically
	p.assignManagers(dataHandler, indexer, querier)

	// Initialize clustering if enabled (after managers are ready)
	if err := p.initializeClusteringIfEnabled(ctx, buckets); err != nil {
		return err
	}

	p.logger.Info("All managers initialized successfully")
	return nil
}

// createKVBucket creates a single KV bucket with logging
func (p *Processor) createKVBucket(ctx context.Context, name, description string) (jetstream.KeyValue, error) {
	p.logger.Debug("Getting KV bucket", "bucket", name)

	bucket, err := p.natsClient.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{
		Bucket:      name,
		Description: description,
		History:     10,
	})

	if err != nil {
		p.logger.Error("Failed to get KV bucket", "bucket", name, "error", err)
		errMsg := fmt.Sprintf("%s KV bucket unavailable", name)
		return nil, errs.WrapFatal(err, "Processor", "createKVBucket", errMsg)
	}

	p.logger.Debug("KV bucket retrieved successfully", "bucket", name)
	return bucket, nil
}

// initializeDataManager creates and configures the DataManager
// Returns the manager which implements both DataLifecycle and EntityManager interfaces
func (p *Processor) initializeDataManager(kvBucket jetstream.KeyValue) (*datamanager.Manager, error) {
	p.logger.Debug("Preparing DataManager configuration")

	dataConfig := datamanager.DefaultConfig()
	if p.config.DataManager != nil {
		dataConfig = *p.config.DataManager
	}

	dataDeps := datamanager.Dependencies{
		KVBucket:        kvBucket,
		MetricsRegistry: p.metricsRegistry,
		Logger:          p.logger,
		Config:          dataConfig,
	}

	p.logger.Debug("Creating DataManager instance")
	dataManager, err := datamanager.NewDataManager(dataDeps)
	if err != nil {
		p.logger.Error("Failed to create DataManager", "error", err)
		return nil, errs.WrapFatal(err, "Processor", "initializeDataManager", "DataManager creation failed")
	}

	p.logger.Debug("DataManager created successfully")
	return dataManager, nil
}

// createIndexBuckets creates all required index buckets
func (p *Processor) createIndexBuckets(ctx context.Context, entityBucket jetstream.KeyValue) (map[string]jetstream.KeyValue, error) {
	p.logger.Debug("Creating KV buckets map for IndexManager")

	buckets := map[string]jetstream.KeyValue{
		"ENTITY_STATES": entityBucket,
	}

	indexBucketConfigs := []struct {
		name        string
		description string
	}{
		{"ALIAS_INDEX", "Alias index for entity resolution"},
		{"PREDICATE_INDEX", "Predicate index for property queries"},
		{"INCOMING_INDEX", "Incoming edge index for relationship queries"},
		{"SPATIAL_INDEX", "Spatial index for geospatial queries"},
		{"TEMPORAL_INDEX", "Temporal index for time-based queries"},
		{"EMBEDDING_INDEX", "Vector embeddings with metadata and status"},
		{"EMBEDDING_DEDUP", "Content-addressed embedding deduplication"},
		{"COMMUNITY_INDEX", "Graph community detection and clustering"},
	}

	for _, cfg := range indexBucketConfigs {
		bucket, err := p.createKVBucket(ctx, cfg.name, cfg.description)
		if err != nil {
			return nil, err
		}
		buckets[cfg.name] = bucket
	}

	return buckets, nil
}

// initializeIndexManager creates and configures the IndexManager
func (p *Processor) initializeIndexManager(buckets map[string]jetstream.KeyValue) (indexmanager.Indexer, error) {
	p.logger.Debug("Preparing IndexManager configuration")

	indexConfig := indexmanager.DefaultConfig()
	if p.config.Indexer != nil {
		indexConfig = *p.config.Indexer
	}

	p.logger.Debug("Creating IndexManager instance", "bucket_count", len(buckets))
	indexManager, err := indexmanager.NewManager(indexConfig, buckets, p.natsClient, p.metricsRegistry, p.logger)
	if err != nil {
		p.logger.Error("Failed to create IndexManager", "error", err)
		return nil, errs.WrapFatal(err, "Processor", "initializeIndexManager", "IndexManager creation failed")
	}

	p.logger.Debug("IndexManager created successfully")
	return indexManager, nil
}

// initializeQueryManager creates and configures the QueryManager
func (p *Processor) initializeQueryManager(
	entityReader datamanager.EntityReader,
	indexer indexmanager.Indexer,
) (querymanager.Querier, error) {
	p.logger.Debug("Preparing QueryManager configuration")

	queryConfig := querymanager.Config{}
	queryConfig.SetDefaults()
	if p.config.Querier != nil {
		queryConfig = *p.config.Querier
	}

	// Create ContentFetcher if NATS client is available (optional, for enriched LLM prompts)
	if p.natsClient != nil && p.contentFetcher == nil {
		fetcher, err := llm.NewNATSContentFetcher(
			p.natsClient,
			llm.WithContentSubject("storage.objectstore.api"),
			llm.WithContentLogger(p.logger),
		)
		if err != nil {
			p.logger.Warn("Failed to create content fetcher, LLM prompts will not include entity content",
				"error", err)
		} else {
			p.contentFetcher = fetcher
		}
	}

	queryDeps := querymanager.Deps{
		Config:         queryConfig,
		EntityReader:   entityReader,
		IndexManager:   indexer,
		ContentFetcher: p.contentFetcher,
		Registry:       p.metricsRegistry,
		Logger:         p.logger,
	}

	p.logger.Debug("Creating QueryManager instance")
	queryManager, err := querymanager.NewManager(queryDeps)
	if err != nil {
		p.logger.Error("Failed to create QueryManager", "error", err)
		return nil, errs.WrapFatal(err, "Processor", "initializeQueryManager", "QueryManager creation failed")
	}

	p.logger.Debug("QueryManager created successfully")
	return queryManager, nil
}

// assignManagers assigns all managers atomically with proper locking
func (p *Processor) assignManagers(
	dataManager *datamanager.Manager,
	indexer indexmanager.Indexer,
	querier querymanager.Querier,
) {
	p.logger.Debug("Assigning all managers atomically")
	p.mu.Lock()
	p.dataManager = dataManager
	p.dataLifecycle = dataManager
	p.entityManager = dataManager
	p.tripleManager = dataManager
	p.indexManager = indexer
	p.queryManager = querier
	p.mu.Unlock()
	p.logger.Debug("All managers assigned successfully")
}

func (p *Processor) initializeBusinessServices() error {
	// Initialize message processor (business service that depends on modules)
	msgConfig := messagemanager.DefaultConfig()
	if p.config.MessageHandler != nil {
		msgConfig = *p.config.MessageHandler
	}

	msgDeps := messagemanager.Dependencies{
		EntityManager:   p.entityManager,
		IndexManager:    p.indexManager,
		Logger:          p.logger,
		MetricsRegistry: p.metricsRegistry,
	}

	p.messageManager = messagemanager.NewManager(msgConfig, msgDeps, p.recordError)

	// EdgeManager functionality now consolidated into DataManager

	return nil
}

// getStreamSubjects returns configured stream subjects or derives from input subject
func (p *Processor) getStreamSubjects() []string {
	if len(p.config.StreamSubjects) > 0 {
		return p.config.StreamSubjects
	}
	// Derive from input subject - convert * to > for stream wildcard
	subject := p.config.InputSubject
	if subject == "" {
		subject = "events.graph.entity.*"
	}
	// Convert single-level wildcard (*) to multi-level (>) for stream capture
	if subject[len(subject)-1] == '*' {
		subject = subject[:len(subject)-1] + ">"
	}
	return []string{subject}
}

func (p *Processor) setupSubscriptions(ctx context.Context) error {
	// Check for new multi-stream subscription mode (InputSubjects)
	if len(p.config.InputSubjects) > 0 {
		return p.setupMultiStreamSubscriptions(ctx)
	}

	// Legacy mode: single subject/stream configuration
	subject := p.config.InputSubject
	if subject == "" {
		subject = "events.graph.entity.*" // Fallback to default
	}

	// Use JetStream if stream name is configured, otherwise fall back to core NATS
	if p.config.StreamName != "" {
		return p.setupJetStreamConsumer(ctx, subject)
	}

	// Fall back to core NATS subscription (fire-and-forget, no persistence)
	p.logger.Warn("Using core NATS subscription (no persistence) - configure stream_name or input_subjects for durable consumption",
		"subject", subject)

	err := p.natsClient.Subscribe(ctx, subject, func(msgCtx context.Context, data []byte) {
		p.handleMessage(msgCtx, data)
	})
	if err != nil {
		return errs.WrapFatal(err, "Processor", "setupSubscriptions", "NATS subscription failed for "+subject)
	}

	return nil
}

// setupMultiStreamSubscriptions handles multiple input subjects with convention-derived streams.
// Each input subject is mapped to its stream using convention: "component.action.type" → "COMPONENT" stream.
// This enables Graph to consume from multiple independent component streams (e.g., OBJECTSTORE, SENSOR).
func (p *Processor) setupMultiStreamSubscriptions(ctx context.Context) error {
	// Derive unique streams from input subjects using naming convention
	// Convention: subject "objectstore.stored.entity" → stream "OBJECTSTORE"
	streamSubjects := make(map[string][]string) // stream name → filter subjects
	for _, subject := range p.config.InputSubjects {
		streamName := config.DeriveStreamName(subject)
		if streamName == "" {
			p.logger.Warn("Could not derive stream name from subject, skipping",
				"subject", subject)
			continue
		}
		streamSubjects[streamName] = append(streamSubjects[streamName], subject)
	}

	if len(streamSubjects) == 0 {
		return errs.WrapInvalid(nil, "Processor", "setupMultiStreamSubscriptions",
			"no valid streams derived from input_subjects")
	}

	p.logger.Info("Setting up multi-stream subscriptions",
		"input_subjects", p.config.InputSubjects,
		"derived_streams", len(streamSubjects))

	// Wait for all streams to be available, then set up consumers
	for streamName, subjects := range streamSubjects {
		// Wait for stream to exist (created by publishing component)
		if err := p.waitForStream(ctx, streamName); err != nil {
			return errs.WrapFatal(err, "Processor", "setupMultiStreamSubscriptions",
				"stream "+streamName+" not available")
		}

		// Set up consumer for this stream with filter subjects
		for _, subject := range subjects {
			if err := p.setupStreamConsumer(ctx, streamName, subject); err != nil {
				return err
			}
		}
	}

	return nil
}

// setupStreamConsumer creates a JetStream consumer for a specific stream and filter subject.
func (p *Processor) setupStreamConsumer(ctx context.Context, streamName, filterSubject string) error {
	// Generate unique consumer name from stream and filter
	consumerName := fmt.Sprintf("graph-%s-%s",
		strings.ToLower(streamName),
		strings.ReplaceAll(strings.ReplaceAll(filterSubject, ".", "-"), "*", "all"))

	p.logger.Info("Setting up stream consumer",
		"stream", streamName,
		"consumer", consumerName,
		"filter_subject", filterSubject)

	cfg := natsclient.StreamConsumerConfig{
		StreamName:    streamName,
		ConsumerName:  consumerName,
		FilterSubject: filterSubject,
		DeliverPolicy: "all",      // Process all messages including historical
		AckPolicy:     "explicit", // Explicit ack required
		MaxDeliver:    5,          // Retry up to 5 times before giving up
		AutoCreate:    false,      // Stream should already exist (created by StreamsManager)
	}

	err := p.natsClient.ConsumeStreamWithConfig(ctx, cfg, func(msgCtx context.Context, msg jetstream.Msg) {
		p.handleJetStreamMessage(msgCtx, msg)
	})
	if err != nil {
		return errs.WrapFatal(err, "Processor", "setupStreamConsumer",
			fmt.Sprintf("consumer failed for stream %s filter %s", streamName, filterSubject))
	}

	return nil
}

// waitForInputSubjectStreams waits for all streams derived from InputSubjects to exist.
// Uses the naming convention to derive stream names from subjects.
func (p *Processor) waitForInputSubjectStreams(ctx context.Context) error {
	// Derive unique streams from input subjects
	streamSet := make(map[string]bool)
	for _, subject := range p.config.InputSubjects {
		streamName := config.DeriveStreamName(subject)
		if streamName != "" {
			streamSet[streamName] = true
		}
	}

	if len(streamSet) == 0 {
		p.logger.Warn("No streams derived from input_subjects")
		return nil
	}

	p.logger.Info("Waiting for input subject streams",
		"input_subjects", p.config.InputSubjects,
		"derived_streams", len(streamSet))

	// Wait for each stream
	for streamName := range streamSet {
		if err := p.waitForStream(ctx, streamName); err != nil {
			p.logger.Error("Stream not available", "stream", streamName, "error", err)
			return errs.WrapFatal(err, "Processor", "waitForInputSubjectStreams",
				"stream "+streamName+" not available")
		}
	}

	return nil
}

// waitForStream waits for a JetStream stream to exist with exponential backoff.
// The stream should be created by the publishing component (e.g., ObjectStore).
// This implements the subscriber retry policy for loosely-coupled stream ownership.
func (p *Processor) waitForStream(ctx context.Context, streamName string) error {
	backoff := []time.Duration{
		500 * time.Millisecond,
		1 * time.Second,
		2 * time.Second,
		5 * time.Second,
		10 * time.Second,
	}

	js, err := p.natsClient.JetStream()
	if err != nil {
		return errs.WrapTransient(err, "Processor", "waitForStream", "failed to get JetStream context")
	}

	p.logger.Info("Waiting for JetStream stream", "stream", streamName)

	for i, delay := range backoff {
		_, err := js.Stream(ctx, streamName)
		if err == nil {
			p.logger.Info("JetStream stream found", "stream", streamName, "attempts", i+1)
			return nil
		}

		if !stderrors.Is(err, jetstream.ErrStreamNotFound) {
			return errs.WrapTransient(err, "Processor", "waitForStream",
				"failed to check stream "+streamName)
		}

		p.logger.Info("Stream not found, waiting...",
			"stream", streamName,
			"attempt", i+1,
			"retry_in", delay)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}

	return errs.WrapTransient(
		stderrors.New("stream not found after retries"),
		"Processor", "waitForStream",
		"stream "+streamName+" not available after "+fmt.Sprint(len(backoff))+" attempts")
}

// setupJetStreamConsumer creates a durable JetStream consumer for reliable message processing.
func (p *Processor) setupJetStreamConsumer(ctx context.Context, subject string) error {
	p.logger.Info("Setting up JetStream consumer",
		"stream", p.config.StreamName,
		"consumer", p.config.ConsumerName,
		"filter_subject", subject,
		"workers", p.config.Workers)

	cfg := natsclient.StreamConsumerConfig{
		StreamName:    p.config.StreamName,
		ConsumerName:  p.config.ConsumerName,
		FilterSubject: subject,
		DeliverPolicy: "all",      // Process all messages including historical
		AckPolicy:     "explicit", // Explicit ack required
		MaxDeliver:    5,          // Retry up to 5 times before giving up
		AutoCreate:    true,       // Auto-create stream if it doesn't exist
		AutoCreateConfig: &natsclient.StreamAutoCreateConfig{
			Subjects:  p.getStreamSubjects(),
			Storage:   "file",
			Retention: "limits",
		},
	}

	err := p.natsClient.ConsumeStreamWithConfig(ctx, cfg, func(msgCtx context.Context, msg jetstream.Msg) {
		p.handleJetStreamMessage(msgCtx, msg)
	})
	if err != nil {
		return errs.WrapFatal(err, "Processor", "setupJetStreamConsumer",
			"JetStream consumer failed for stream "+p.config.StreamName)
	}

	return nil
}

// handleJetStreamMessage processes a JetStream message with explicit acknowledgment.
// Uses async worker pool for throughput. Messages are ACK'd after successful submission
// to the worker pool. The write buffer provides durability for in-flight messages.
func (p *Processor) handleJetStreamMessage(ctx context.Context, msg jetstream.Msg) {
	// Log received message subject for debugging
	p.logger.Debug("JetStream message received",
		"subject", msg.Subject(),
		"data_len", len(msg.Data()))

	// Check context before processing
	select {
	case <-ctx.Done():
		p.logger.Debug("Context cancelled, nak-ing message for redelivery")
		_ = msg.Nak() // Redelivery on context cancellation
		return
	default:
	}

	// Check if processor is started
	if p.workerPool == nil {
		p.recordError("Worker pool is nil - processor not started")
		_ = msg.Nak() // Redelivery - processor may recover
		return
	}

	data := msg.Data()

	// Submit to worker pool for async processing
	if err := p.workerPool.Submit(data); err != nil {
		// Check error type to determine acknowledgment strategy
		if stderrors.Is(err, worker.ErrPoolStopped) || stderrors.Is(err, worker.ErrPoolNotStarted) {
			// Worker pool stopped unexpectedly - nak for redelivery
			p.recordError("Worker pool stopped unexpectedly")
			p.logger.Error("Worker pool no longer running",
				"error", err,
				"data_len", len(data))
			_ = msg.Nak()
		} else if stderrors.Is(err, worker.ErrQueueFull) {
			// Queue full is transient - nak with delay for backpressure
			p.logger.Debug("Worker queue full, nak-ing message",
				"data_len", len(data))
			_ = msg.NakWithDelay(time.Second) // Delay redelivery by 1s
		} else {
			// Unknown error - nak for redelivery
			p.recordError(fmt.Sprintf("Unexpected worker pool error: %v", err))
			p.logger.Warn("Failed to submit message to worker pool",
				"data_len", len(data),
				"error", err)
			_ = msg.Nak()
		}
		return
	}

	// Successfully submitted to worker pool - acknowledge
	// Note: Processing happens async. The write buffer provides durability for in-flight data.
	// If processing fails after ACK, the data is still in the buffer and will be written.
	if err := msg.Ack(); err != nil {
		p.logger.Warn("Failed to ack message", "error", err)
	}
}

// Message handling - pure orchestration

func (p *Processor) handleMessage(ctx context.Context, data []byte) {
	// Check context before processing
	select {
	case <-ctx.Done():
		p.logger.Debug("Context cancelled, skipping message processing")
		return
	default:
	}

	// Check if processor is started
	if p.workerPool == nil {
		p.recordError("Worker pool is nil - processor not started")
		return
	}

	// Delegate to worker pool which uses messageProcessor
	if err := p.workerPool.Submit(data); err != nil {
		// Check error type to determine severity
		if stderrors.Is(err, worker.ErrPoolStopped) || stderrors.Is(err, worker.ErrPoolNotStarted) {
			// Worker pool stopped unexpectedly - this is critical
			p.recordError("Worker pool stopped unexpectedly")
			p.logger.Error("Worker pool no longer running",
				"error", err,
				"data_len", len(data))
			// Consider setting health status to unhealthy here
		} else if stderrors.Is(err, worker.ErrQueueFull) {
			// Queue full is transient - log at debug level
			p.logger.Debug("Worker queue full, message dropped",
				"data_len", len(data))
			// Metrics would track dropped messages here
		} else {
			// Unknown error - log as warning
			p.recordError(fmt.Sprintf("Unexpected worker pool error: %v", err))
			p.logger.Warn("Failed to submit message to worker pool",
				"data_len", len(data),
				"error", err)
		}
	}
}

// API endpoints - direct engine calls (no wrappers)

// GetEntity retrieves an entity by its ID.
func (p *Processor) GetEntity(ctx context.Context, id string) (*gtypes.EntityState, error) {
	return p.entityManager.GetEntity(ctx, id)
}

// GetEntityByAlias retrieves an entity by its alias or ID.
func (p *Processor) GetEntityByAlias(ctx context.Context, aliasOrID string) (*gtypes.EntityState, error) {
	// Check if processor is initialized
	if p.aliasCache == nil || p.entityManager == nil || p.indexManager == nil {
		return nil, errs.WrapTransient(nil, "Processor", "GetEntityByAlias", "processor not initialized")
	}

	// Try alias cache first
	if entityID, ok := p.aliasCache.Get(aliasOrID); ok {
		return p.entityManager.GetEntity(ctx, entityID)
	}

	// Resolve via index engine
	entityID, err := p.indexManager.ResolveAlias(ctx, aliasOrID)
	if err != nil {
		return nil, err
	}

	// Cache the result
	p.aliasCache.Set(aliasOrID, entityID)

	return p.entityManager.GetEntity(ctx, entityID)
}

// QueryByPredicate queries entities by a predicate expression.
func (p *Processor) QueryByPredicate(ctx context.Context, predicate string) ([]string, error) {
	return p.indexManager.GetPredicateIndex(ctx, predicate)
}

// Error handling

func (p *Processor) recordError(errorMsg string) {
	p.logger.Error("Graph processor error", "error", errorMsg)

	p.mu.Lock()
	p.health.ErrorCount++
	p.health.LastError = errorMsg
	p.health.Healthy = false
	p.health.LastCheck = time.Now()
	p.mu.Unlock()
}

// Clustering Integration

// processorGraphProvider implements clustering.GraphProvider using processor components
type processorGraphProvider struct {
	entityReader datamanager.EntityReader
	kvBucket     jetstream.KeyValue
}

// GetAllEntityIDs returns all entity IDs from ENTITY_STATES bucket
func (p *processorGraphProvider) GetAllEntityIDs(ctx context.Context) ([]string, error) {
	keys, err := p.kvBucket.ListKeys(ctx)
	if err != nil {
		return nil, errs.WrapTransient(err, "processorGraphProvider", "GetAllEntityIDs", "failed to list keys")
	}

	ids := make([]string, 0)
	for key := range keys.Keys() {
		ids = append(ids, key)
	}
	return ids, nil
}

// GetNeighbors returns entity IDs connected to the given entity
func (p *processorGraphProvider) GetNeighbors(ctx context.Context, entityID string, direction string) ([]string, error) {
	entity, err := p.entityReader.GetEntity(ctx, entityID)
	if err != nil {
		return []string{}, nil // Entity not found, return empty
	}

	neighborSet := make(map[string]bool)

	if direction == "outgoing" || direction == "both" {
		for _, triple := range entity.Triples {
			if triple.IsRelationship() {
				neighborSet[triple.Object.(string)] = true
			}
		}
	}

	// For incoming direction, we'd need INCOMING_INDEX
	// For now, just return outgoing neighbors
	if direction == "incoming" {
		return []string{}, nil
	}

	neighbors := make([]string, 0, len(neighborSet))
	for id := range neighborSet {
		neighbors = append(neighbors, id)
	}
	return neighbors, nil
}

// GetEdgeWeight returns the weight of an edge between two entities
func (p *processorGraphProvider) GetEdgeWeight(ctx context.Context, fromID, toID string) (float64, error) {
	entity, err := p.entityReader.GetEntity(ctx, fromID)
	if err != nil {
		return 0.0, nil
	}

	for _, triple := range entity.Triples {
		if triple.IsRelationship() && triple.Object.(string) == toID {
			if triple.Confidence > 0 {
				return triple.Confidence, nil
			}
			return 1.0, nil
		}
	}
	return 0.0, nil
}

// initializeClusteringIfEnabled sets up clustering components if enabled in config
func (p *Processor) initializeClusteringIfEnabled(ctx context.Context, buckets map[string]jetstream.KeyValue) error {
	if !p.isClusteringEnabled() {
		return nil
	}

	cfg := p.config.Clustering
	p.logger.Info("Initializing clustering",
		"max_iterations", cfg.Algorithm.MaxIterations,
		"levels", cfg.Algorithm.Levels,
		"enhancement_enabled", cfg.Enhancement.Enabled)

	p.clusteringBuckets = buckets

	communityBucket, graphProvider, err := p.setupGraphProvider(ctx, buckets, cfg)
	if err != nil {
		return err
	}

	p.communityDetector = p.createCommunityDetector(graphProvider, cfg)
	p.initializeInferenceMetrics()

	if err := p.setupEnhancementWorker(ctx, cfg, communityBucket, graphProvider); err != nil {
		return err
	}

	p.setupEntityChangeCallback(cfg)
	return nil
}

// isClusteringEnabled checks if clustering is configured and enabled.
func (p *Processor) isClusteringEnabled() bool {
	clusteringNil := p.config.Clustering == nil
	enabled := !clusteringNil && p.config.Clustering.Enabled
	p.logger.Info("Checking clustering configuration",
		"clustering_config_nil", clusteringNil,
		"enabled", enabled)

	if clusteringNil || !enabled {
		p.logger.Info("Clustering disabled, skipping initialization")
		return false
	}
	return true
}

// setupGraphProvider creates the graph provider and community storage from buckets.
func (p *Processor) setupGraphProvider(ctx context.Context, buckets map[string]jetstream.KeyValue, cfg *ClusteringConfig) (jetstream.KeyValue, clustering.GraphProvider, error) {
	// Check context before bucket operations
	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	default:
	}

	communityBucket, ok := buckets["COMMUNITY_INDEX"]
	if !ok {
		return nil, nil, errs.WrapFatal(errs.ErrMissingConfig, "Processor",
			"initializeClusteringIfEnabled", "COMMUNITY_INDEX bucket not found")
	}
	p.communityStorage = clustering.NewNATSCommunityStorage(communityBucket)

	entityBucket, ok := buckets["ENTITY_STATES"]
	if !ok {
		return nil, nil, errs.WrapFatal(errs.ErrMissingConfig, "Processor",
			"initializeClusteringIfEnabled", "ENTITY_STATES bucket not found")
	}

	baseProvider := &processorGraphProvider{
		entityReader: p.dataManager,
		kvBucket:     entityBucket,
	}

	graphProvider := p.wrapWithSemanticProvider(baseProvider, cfg)
	return communityBucket, graphProvider, nil
}

// wrapWithSemanticProvider wraps base provider with semantic edges if enabled.
func (p *Processor) wrapWithSemanticProvider(baseProvider clustering.GraphProvider, cfg *ClusteringConfig) clustering.GraphProvider {
	if !cfg.SemanticEdges.Enabled || p.indexManager == nil {
		return baseProvider
	}

	threshold := cfg.SemanticEdges.SimilarityThreshold
	if threshold <= 0 {
		threshold = DefaultSimilarityThreshold
	}
	maxNeighbors := cfg.SemanticEdges.MaxVirtualNeighbors
	if maxNeighbors <= 0 {
		maxNeighbors = DefaultMaxVirtualNeighbors
	}

	semanticConfig := clustering.SemanticProviderConfig{
		SimilarityThreshold: threshold,
		MaxVirtualNeighbors: maxNeighbors,
	}

	p.logger.Info("Semantic edges enabled for clustering",
		"similarity_threshold", threshold,
		"max_virtual_neighbors", maxNeighbors)

	return clustering.NewSemanticGraphProvider(baseProvider, p.indexManager, semanticConfig, p.logger)
}

// createCommunityDetector creates the LPA detector with configuration.
func (p *Processor) createCommunityDetector(graphProvider clustering.GraphProvider, cfg *ClusteringConfig) clustering.CommunityDetector {
	maxIterations := cfg.Algorithm.MaxIterations
	if maxIterations <= 0 {
		maxIterations = DefaultMaxIterations
	}
	levels := cfg.Algorithm.Levels
	if levels <= 0 {
		levels = DefaultClusteringLevels
	}

	detector := clustering.NewLPADetector(graphProvider, p.communityStorage).
		WithMaxIterations(maxIterations).
		WithLevels(levels)

	summarizer := clustering.NewProgressiveSummarizer()
	return detector.WithProgressiveSummarization(summarizer, p.queryManager)
}

// initializeInferenceMetrics creates the inference metrics counter.
func (p *Processor) initializeInferenceMetrics() {
	if p.metricsRegistry == nil {
		return
	}
	p.inferredTriples = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "semstreams_graph_inferred_triples_total",
		Help: "Total inferred relationship triples from community detection",
	})
	p.metricsRegistry.RegisterCounter("graph", "semstreams_graph_inferred_triples_total", p.inferredTriples)
}

// setupEnhancementWorker creates the LLM enhancement worker if enabled.
func (p *Processor) setupEnhancementWorker(ctx context.Context, cfg *ClusteringConfig, communityBucket jetstream.KeyValue, graphProvider clustering.GraphProvider) error {
	if !cfg.Enhancement.Enabled || !cfg.Enhancement.LLM.IsEnabled() {
		return nil
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	llmClient, err := llm.NewOpenAIClient(cfg.Enhancement.LLM.ToOpenAIConfig())
	if err != nil {
		return errs.WrapFatal(err, "Processor", "setupEnhancementWorker", "failed to create LLM client")
	}

	// Create LLM summarizer with optional content fetcher (reuse from QueryManager initialization)
	summarizerOpts := []clustering.LLMSummarizerOption{}
	if p.contentFetcher != nil {
		summarizerOpts = append(summarizerOpts, clustering.WithContentFetcher(p.contentFetcher))
	}

	llmSummarizer, err := clustering.NewLLMSummarizer(
		clustering.LLMSummarizerConfig{Client: llmClient},
		summarizerOpts...,
	)
	if err != nil {
		return errs.WrapFatal(err, "Processor", "setupEnhancementWorker", "failed to create LLM summarizer")
	}

	workers := cfg.Enhancement.Workers
	if workers <= 0 {
		workers = DefaultEnhancementWorkers
	}

	workerConfig := &clustering.EnhancementWorkerConfig{
		LLMSummarizer:   llmSummarizer,
		Storage:         p.communityStorage,
		GraphProvider:   graphProvider,
		Querier:         p.queryManager,
		CommunityBucket: communityBucket,
		Logger:          p.logger,
		Registry:        p.metricsRegistry,
	}

	worker, err := clustering.NewEnhancementWorker(workerConfig)
	if err != nil {
		return errs.WrapFatal(err, "Processor", "setupEnhancementWorker", "failed to create enhancement worker")
	}
	p.enhancementWorker = worker.WithWorkers(workers)

	p.logger.Info("Enhancement worker configured",
		"base_url", cfg.Enhancement.LLM.BaseURL,
		"model", cfg.Enhancement.LLM.Model,
		"workers", workers)
	return nil
}

// setupEntityChangeCallback configures the entity creation callback for adaptive clustering.
func (p *Processor) setupEntityChangeCallback(cfg *ClusteringConfig) {
	p.detectionTrigger = make(chan struct{}, 1)

	threshold := cfg.Schedule.EntityChangeThreshold
	if threshold <= 0 {
		threshold = DefaultEntityChangeThreshold
	}

	p.dataManager.SetEntityCreatedCallback(func(_ string) {
		newCount := p.entityChangeCount.Add(1)
		if int(newCount) >= threshold {
			select {
			case p.detectionTrigger <- struct{}{}:
			default:
			}
		}
	})

	p.logger.Info("Clustering initialized successfully", "entity_change_threshold", threshold)
}

// runClusteringLoop runs periodic community detection with hybrid triggers.
// Detection is triggered by:
// 1. Max interval timer (detection_interval, default 30s) - ensures detection even in quiet periods
// 2. Entity change threshold (entity_change_threshold, default 100) - triggers immediately when threshold reached
// 3. Min interval protection (min_detection_interval, default 5s) - prevents hammering during bursts
func (p *Processor) runClusteringLoop(ctx context.Context) error {
	if p.config.Clustering == nil {
		return nil
	}

	cfg := p.config.Clustering

	// Parse timing configuration with error logging
	initialDelay := 10 * time.Second
	if cfg.Schedule.InitialDelay != "" {
		if d, err := time.ParseDuration(cfg.Schedule.InitialDelay); err == nil {
			initialDelay = d
		} else {
			p.logger.Warn("Invalid initial_delay config, using default",
				"value", cfg.Schedule.InitialDelay, "default", initialDelay, "error", err)
		}
	}

	// Max interval between detection runs (triggers even if no new entities)
	maxInterval := 30 * time.Second
	if cfg.Schedule.DetectionInterval != "" {
		if d, err := time.ParseDuration(cfg.Schedule.DetectionInterval); err == nil {
			maxInterval = d
		} else {
			p.logger.Warn("Invalid detection_interval config, using default",
				"value", cfg.Schedule.DetectionInterval, "default", maxInterval, "error", err)
		}
	}

	// Min interval between detection runs (burst protection)
	minInterval := 5 * time.Second
	if cfg.Schedule.MinDetectionInterval != "" {
		if d, err := time.ParseDuration(cfg.Schedule.MinDetectionInterval); err == nil {
			minInterval = d
		} else {
			p.logger.Warn("Invalid min_detection_interval config, using default",
				"value", cfg.Schedule.MinDetectionInterval, "default", minInterval, "error", err)
		}
	}

	minEntities := cfg.Schedule.MinEntities
	if minEntities <= 0 {
		minEntities = 10
	}

	entityThreshold := cfg.Schedule.EntityChangeThreshold
	if entityThreshold <= 0 {
		entityThreshold = 100
	}

	// Parse enhancement window configuration
	var enhancementWindow time.Duration
	if cfg.Schedule.EnhancementWindow != "" {
		if d, err := time.ParseDuration(cfg.Schedule.EnhancementWindow); err == nil {
			enhancementWindow = d
		} else {
			p.logger.Warn("Invalid enhancement_window config, using default (disabled)",
				"value", cfg.Schedule.EnhancementWindow, "error", err)
		}
	}

	// Set enhancement window mode (default: blocking)
	p.enhancementMode = cfg.Schedule.EnhancementWindowMode
	if p.enhancementMode == "" {
		p.enhancementMode = WindowModeBlocking
	}

	p.logger.Info("Starting clustering loop (hybrid trigger)",
		"initial_delay", initialDelay,
		"max_interval", maxInterval,
		"min_interval", minInterval,
		"entity_threshold", entityThreshold,
		"min_entities", minEntities,
		"enhancement_window", enhancementWindow,
		"enhancement_window_mode", p.enhancementMode)

	// Wait for initial delay
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(initialDelay):
	}

	// Run initial detection
	p.entityChangeCount.Store(0)
	p.runDetectionIfReady(ctx, minEntities, enhancementWindow, entityThreshold)
	lastRun := time.Now()

	// Start max interval timer
	ticker := time.NewTicker(maxInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("Clustering loop stopped")
			return ctx.Err()

		case <-ticker.C:
			// Max interval reached - run detection regardless of entity count
			p.logger.Debug("Max interval reached, triggering detection",
				"entities_since_last", p.entityChangeCount.Load())
			p.entityChangeCount.Store(0)
			p.runDetectionIfReady(ctx, minEntities, enhancementWindow, entityThreshold)
			lastRun = time.Now()

		case <-p.detectionTrigger:
			// Entity threshold reached - check min interval for burst protection
			timeSinceLast := time.Since(lastRun)
			if timeSinceLast >= minInterval {
				p.logger.Debug("Entity threshold reached, triggering detection",
					"entities", p.entityChangeCount.Load(),
					"time_since_last", timeSinceLast)
				p.entityChangeCount.Store(0)
				p.runDetectionIfReady(ctx, minEntities, enhancementWindow, entityThreshold)
				lastRun = time.Now()
				ticker.Reset(maxInterval) // Reset max interval timer
			} else {
				p.logger.Debug("Entity threshold reached but min interval not elapsed",
					"entities", p.entityChangeCount.Load(),
					"time_since_last", timeSinceLast,
					"min_interval", minInterval)
			}
		}
	}
}

// runDetectionIfReady runs community detection if not already running and entity threshold met.
// The enhancementWindow parameter controls how long to pause detection after clustering
// to allow LLM enhancement to complete without being overwritten.
func (p *Processor) runDetectionIfReady(ctx context.Context, minEntities int, enhancementWindow time.Duration, entityThreshold int) {
	// Check enhancement window - prevents re-detection from overwriting LLM-enhanced communities
	if !p.enhancementDeadline.IsZero() && time.Now().Before(p.enhancementDeadline) {
		switch p.enhancementMode {
		case WindowModeBlocking:
			// Check if all communities have reached terminal status
			allTerminal, _ := p.checkAllCommunitiesTerminal(ctx)
			if allTerminal {
				p.logger.Info("Enhancement window: all communities terminal, allowing detection")
				p.enhancementDeadline = time.Time{} // Clear window
			} else {
				p.logger.Debug("Enhancement window active, skipping detection",
					"deadline", p.enhancementDeadline,
					"remaining", time.Until(p.enhancementDeadline))
				return
			}
		case WindowModeSoft:
			// Allow detection only if significant entity changes occurred
			if int(p.entityChangeCount.Load()) < entityThreshold {
				p.logger.Debug("Enhancement window (soft) active, skipping detection",
					"deadline", p.enhancementDeadline,
					"entity_changes", p.entityChangeCount.Load(),
					"threshold", entityThreshold)
				return
			}
			p.logger.Info("Enhancement window soft override: significant entity changes",
				"entity_changes", p.entityChangeCount.Load())
		case WindowModeNone:
			// No enhancement window - continue with detection
		}
	}

	// Skip if previous detection still running
	p.detectionMu.Lock()
	if p.detectionRunning {
		p.detectionMu.Unlock()
		p.logger.Debug("Skipping detection - previous run still in progress")
		return
	}
	p.detectionRunning = true
	p.detectionMu.Unlock()

	defer func() {
		p.detectionMu.Lock()
		p.detectionRunning = false
		p.detectionMu.Unlock()
	}()

	// Check entity count threshold
	var entityCount int
	if p.clusteringBuckets != nil {
		if entityBucket, ok := p.clusteringBuckets["ENTITY_STATES"]; ok {
			keys, err := entityBucket.ListKeys(ctx)
			if err == nil {
				for range keys.Keys() {
					entityCount++
				}
				if entityCount < minEntities {
					p.logger.Debug("Skipping detection - not enough entities",
						"current", entityCount, "required", minEntities)
					return
				}
			}
		}
	}

	// Check embedding coverage for semantic clustering
	if p.isSemanticClusteringEnabled() && p.indexManager != nil && entityCount > 0 {
		embeddingCount := p.indexManager.GetEmbeddingCount()
		coverage := float64(embeddingCount) / float64(entityCount)
		minCoverage := p.getMinEmbeddingCoverage()

		if coverage < minCoverage {
			p.logger.Info("Skipping detection - waiting for embeddings",
				"entities", entityCount,
				"embeddings", embeddingCount,
				"coverage", fmt.Sprintf("%.1f%%", coverage*100),
				"min_coverage", fmt.Sprintf("%.1f%%", minCoverage*100))
			return
		}

		p.logger.Info("Embedding coverage sufficient for semantic clustering",
			"entities", entityCount,
			"embeddings", embeddingCount,
			"coverage", fmt.Sprintf("%.1f%%", coverage*100))
	}

	// Run community detection
	p.logger.Info("Running community detection")
	startTime := time.Now()

	// Pause enhancement worker during detection to prevent races
	// with GetAllCommunities() and SaveCommunity()
	if p.enhancementWorker != nil {
		p.enhancementWorker.Pause()
		defer p.enhancementWorker.Resume()
	}

	communities, err := p.communityDetector.DetectCommunities(ctx)
	if err != nil {
		p.logger.Error("Community detection failed", "error", err)
		return
	}

	// Count total communities across all levels
	totalCommunities := 0
	for _, levelCommunities := range communities {
		totalCommunities += len(levelCommunities)
	}

	p.logger.Info("Community detection completed",
		"duration", time.Since(startTime),
		"levels", len(communities),
		"total_communities", totalCommunities)

	// Set enhancement window deadline if configured
	// This prevents re-detection from overwriting LLM-enhanced communities
	if enhancementWindow > 0 && totalCommunities > 0 {
		p.enhancementDeadline = time.Now().Add(enhancementWindow)
		p.logger.Info("Enhancement window started",
			"deadline", p.enhancementDeadline,
			"duration", enhancementWindow,
			"mode", p.enhancementMode)
	}

	// Run statistical inference if enabled
	if p.config.Clustering.Inference.Enabled {
		p.runInference(ctx, communities)
	}
}

// checkAllCommunitiesTerminal checks if all communities have reached terminal enhancement status.
// Terminal statuses are "llm-enhanced" or "llm-failed" - these won't be re-enhanced.
// Returns true if all communities are terminal, false if any are still pending ("statistical").
func (p *Processor) checkAllCommunitiesTerminal(ctx context.Context) (bool, error) {
	if p.communityStorage == nil {
		// No storage means no communities to check
		return true, nil
	}

	// Get all communities from storage
	// We need to iterate through levels 0-2 (typical 3 levels)
	for level := 0; level < 3; level++ {
		communities, err := p.communityStorage.GetCommunitiesByLevel(ctx, level)
		if err != nil {
			p.logger.Debug("Failed to get communities for level", "level", level, "error", err)
			continue
		}

		for _, comm := range communities {
			// Check if community is in terminal status
			if comm.SummaryStatus != "llm-enhanced" && comm.SummaryStatus != "llm-failed" {
				// Found a non-terminal community (still "statistical" or empty)
				return false, nil
			}
		}
	}

	return true, nil
}

// runInference generates and persists inferred relationships from community detection.
// For each level's communities, it creates "inferred.clustered_with" triples between
// co-members, then persists them via the data manager.
func (p *Processor) runInference(ctx context.Context, communities map[int][]*clustering.Community) {
	startTime := time.Now()
	totalInferred := 0

	// Convert processor config to clustering config
	inferConfig := clustering.InferenceConfig{
		MinCommunitySize:        p.config.Clustering.Inference.MinCommunitySize,
		MaxInferredPerCommunity: p.config.Clustering.Inference.MaxInferredPerCommunity,
	}

	// Process each level
	for level := range communities {
		inferred, err := p.communityDetector.InferRelationshipsFromCommunities(ctx, level, inferConfig)
		if err != nil {
			p.logger.Warn("Inference failed for level",
				"level", level,
				"error", err)
			continue
		}

		// Persist inferred triples
		for _, triple := range inferred {
			msgTriple := message.Triple{
				Subject:    triple.Subject,
				Predicate:  triple.Predicate,
				Object:     triple.Object,
				Source:     triple.Source,
				Confidence: triple.Confidence,
				Timestamp:  triple.Timestamp,
				Context:    triple.CommunityID, // Store community ID as context
			}

			if err := p.dataManager.AddTriple(ctx, msgTriple); err != nil {
				p.logger.Debug("Failed to persist inferred triple",
					"subject", triple.Subject,
					"object", triple.Object,
					"error", err)
				continue
			}
			totalInferred++
		}
	}

	// Update metrics
	if p.inferredTriples != nil {
		p.inferredTriples.Add(float64(totalInferred))
	}

	p.logger.Info("Statistical inference completed",
		"duration", time.Since(startTime),
		"inferred_triples", totalInferred)
}

// runEnhancementWorker runs the LLM enhancement worker
func (p *Processor) runEnhancementWorker(ctx context.Context) error {
	if p.enhancementWorker == nil {
		return nil
	}

	p.logger.Info("Starting enhancement worker")
	err := p.enhancementWorker.Start(ctx)
	if err != nil && err != context.Canceled {
		p.logger.Error("Enhancement worker error", "error", err)
	}
	p.logger.Info("Enhancement worker stopped")
	return err
}

// Register registers the graph processor component with the given registry.
func Register(registry *component.Registry) error {
	return registry.RegisterWithConfig(component.RegistrationConfig{
		Name:        "graph-processor",
		Factory:     CreateGraphProcessor,
		Schema:      schema,
		Type:        "processor",
		Protocol:    "graph",
		Domain:      "semantic",
		Description: "Graph processor for entity extraction and storage",
		Version:     "1.0.0",
	})
}

// CreateGraphProcessor creates a new graph processor instance with the given configuration.
func CreateGraphProcessor(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	var config Config
	if err := json.Unmarshal(rawConfig, &config); err != nil {
		return nil, errs.WrapInvalid(err, "Processor", "ConfigFromJSON", "invalid JSON configuration")
	}

	processorDeps := ProcessorDeps{
		Config:          &config,
		NATSClient:      deps.NATSClient,
		MetricsRegistry: deps.MetricsRegistry,
		Logger:          deps.Logger,
	}

	return NewProcessor(processorDeps)
}

// isSemanticClusteringEnabled checks if semantic edge clustering is enabled
func (p *Processor) isSemanticClusteringEnabled() bool {
	if p.config == nil || p.config.Clustering == nil {
		return false
	}
	return p.config.Clustering.SemanticEdges.Enabled
}

// getMinEmbeddingCoverage returns the minimum embedding coverage threshold for semantic clustering.
// Returns 0.5 (50%) as default if not configured.
func (p *Processor) getMinEmbeddingCoverage() float64 {
	if p.config != nil && p.config.Clustering != nil &&
		p.config.Clustering.Schedule.MinEmbeddingCoverage > 0 {
		return p.config.Clustering.Schedule.MinEmbeddingCoverage
	}
	return 0.5 // Default: 50% coverage
}
