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
	graphqlgateway "github.com/c360/semstreams/processor/graph/gateway/graphql"
	mcpgateway "github.com/c360/semstreams/processor/graph/gateway/mcp"
	"github.com/c360/semstreams/processor/graph/indexmanager"
	"github.com/c360/semstreams/processor/graph/inference"
	"github.com/c360/semstreams/processor/graph/llm"
	"github.com/c360/semstreams/processor/graph/messagemanager"
	"github.com/c360/semstreams/processor/graph/querymanager"
	"github.com/c360/semstreams/processor/graph/structuralindex"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus"
)

// Clustering configuration defaults
const (
	DefaultMaxIterations         = 100
	DefaultClusteringLevels      = 3
	DefaultEnhancementWorkers    = 3
	DefaultSimilarityThreshold   = 0.6
	DefaultMaxVirtualNeighbors   = 5
	DefaultEntityChangeThreshold = 100
)

// Health tracking constants
const (
	healthyThreshold   = 3 // Failures before degraded
	unhealthyThreshold = 8 // Failures before unhealthy
	recoveryThreshold  = 2 // Consecutive successes for full recovery
	baseBackoffDelay   = 1 * time.Second
	maxBackoffDelay    = 5 * time.Minute
)

// ProcessWork carries message data and the JetStream message for ACK/NAK after processing.
// This enables proper "at least once" delivery semantics by ACKing only after successful KV write.
type ProcessWork struct {
	Data []byte        // Raw message data to process
	Msg  jetstream.Msg // JetStream message for ACK/NAK (nil for non-JetStream messages)
}

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
	workerPool *worker.Pool[*ProcessWork]

	// Background modules management
	moduleCancel context.CancelFunc
	moduleDone   chan error

	// Rate limiting for queries (DoS protection)
	queryLimiter *rate.Limiter

	// Synchronization
	mu sync.RWMutex

	// Configuration
	config *Config

	// Community detection components (optional, initialized if config.GraphAnalysis.CommunityDetection.Enabled)
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
	enhancementDeadline time.Time // When the enhancement window expires

	// Hierarchy inference component (creates sibling edges at entity creation time)
	hierarchyInference *inference.HierarchyInference
	enhancementMode    EnhancementWindowMode // Mode for enhancement window behavior

	// Health tracking fields
	consecutiveFailures  int       // Count of consecutive detection failures
	consecutiveSuccesses int       // Count of consecutive detection successes
	currentHealth        string    // "healthy", "degraded", "unhealthy"
	lastBackoffUntil     time.Time // Time until which detection should be skipped (backoff)

	// Inference metrics
	inferredTriples prometheus.Counter

	// Clustering observability metrics (US3)
	clusteringEmbeddingCoverage prometheus.Gauge   // Current embedding coverage ratio (0-1)
	clusteringWindowActive      prometheus.Gauge   // 1 if enhancement window is active, 0 otherwise
	clusteringFailuresTotal     prometheus.Counter // Total detection failures

	// Structural index components (optional, initialized if config.GraphAnalysis.StructuralIndex.Enabled)
	structuralMu       sync.RWMutex                       // Protects structuralIndices and previousKCore
	structuralIndices  *structuralindex.StructuralIndices // Current k-core and pivot indices
	previousKCore      *structuralindex.KCoreIndex        // Previous k-core for demotion detection
	structuralComputer *structuralIndexComputer           // Helper for computing indices
	graphProvider      clustering.GraphProvider           // Cached graph provider for structural computation

	// Anomaly detection components (optional, initialized if config.GraphAnalysis.AnomalyDetection.Enabled)
	inferenceOrchestrator *inference.Orchestrator
	anomalyStorage        inference.Storage
	reviewWorker          *inference.ReviewWorker

	// Gateway servers (output ports for HTTP access to graph queries)
	graphqlServer *graphqlgateway.Server
	mcpServer     *mcpgateway.Server
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
		health: component.HealthStatus{
			Healthy:    true,
			LastCheck:  time.Now(),
			ErrorCount: 0,
		},
		currentHealth:        "healthy",
		consecutiveFailures:  0,
		consecutiveSuccesses: 0,
	}

	// Build ports from config (standard pattern) or use defaults
	p.inputPorts, p.outputPorts = p.buildPortsFromConfig()

	return p, nil
}

// buildPortsFromConfig creates input/output ports from configuration.
// If Ports is configured, builds dynamic ports from it.
// Otherwise falls back to legacy InputSubjects or hardcoded defaults.
func (p *Processor) buildPortsFromConfig() ([]component.Port, []component.Port) {
	var inputPorts []component.Port
	var outputPorts []component.Port

	// Standard ports pattern: build from Ports config
	if p.config.Ports != nil && len(p.config.Ports.Inputs) > 0 {
		p.logger.Info("Building ports from Ports config",
			"input_count", len(p.config.Ports.Inputs),
			"output_count", len(p.config.Ports.Outputs))
		inputPorts = p.buildInputPortsFromConfig()
		outputPorts = p.buildOutputPortsFromConfig()
		return inputPorts, outputPorts
	}

	// Log why we're using fallback
	if p.config.Ports != nil {
		p.logger.Warn("Ports configured but Inputs empty, using legacy fallback",
			"ports_not_nil", true,
			"inputs_nil", p.config.Ports.Inputs == nil,
			"inputs_len", len(p.config.Ports.Inputs))
	}

	// Legacy: build from InputSubjects if configured
	if len(p.config.InputSubjects) > 0 {
		for i, subject := range p.config.InputSubjects {
			port := component.Port{
				Name:        fmt.Sprintf("input_%d", i),
				Direction:   component.DirectionInput,
				Description: fmt.Sprintf("Entity events from %s", subject),
				Required:    i == 0, // First input is required
				Config: component.JetStreamPort{
					Subjects: []string{subject},
				},
			}
			inputPorts = append(inputPorts, port)
		}
	} else {
		// Hardcoded default fallback
		inputPorts = []component.Port{
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
		}
	}

	// Always include mutations API as input
	inputPorts = append(inputPorts, component.Port{
		Name:        "mutations_api",
		Direction:   component.DirectionInput,
		Description: "Request/Reply API for graph mutations",
		Required:    false,
		Config: component.NATSRequestPort{
			Subject: "graph.mutations",
			Timeout: "500ms",
		},
	})

	// Default output ports (KV buckets)
	outputPorts = []component.Port{
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
	}

	return inputPorts, outputPorts
}

// buildInputPortsFromConfig creates input ports from Ports.Inputs configuration
func (p *Processor) buildInputPortsFromConfig() []component.Port {
	var ports []component.Port

	for _, def := range p.config.Ports.Inputs {
		port := component.Port{
			Name:        def.Name,
			Direction:   component.DirectionInput,
			Description: def.Description,
			Required:    def.Required,
		}

		// Build appropriate port config based on type
		switch def.Type {
		case "jetstream":
			port.Config = component.JetStreamPort{
				Subjects:   []string{def.Subject},
				StreamName: def.StreamName,
				Interface: func() *component.InterfaceContract {
					if def.Interface != "" {
						return &component.InterfaceContract{Type: def.Interface, Version: "v1"}
					}
					return nil
				}(),
			}
		case "nats":
			port.Config = component.NATSPort{
				Subject: def.Subject,
				Interface: func() *component.InterfaceContract {
					if def.Interface != "" {
						return &component.InterfaceContract{Type: def.Interface, Version: "v1"}
					}
					return nil
				}(),
			}
		case "nats-request":
			timeout := def.Timeout
			if timeout == "" {
				timeout = "500ms"
			}
			port.Config = component.NATSRequestPort{
				Subject: def.Subject,
				Timeout: timeout,
			}
		case "kv-watch":
			port.Config = component.KVWatchPort{
				Bucket: def.Subject, // Subject field used for bucket name in KV context
				Interface: func() *component.InterfaceContract {
					if def.Interface != "" {
						return &component.InterfaceContract{Type: def.Interface, Version: "v1"}
					}
					return nil
				}(),
			}
		default:
			// Default to NATS port
			port.Config = component.NATSPort{
				Subject: def.Subject,
			}
		}

		ports = append(ports, port)
	}

	// Always include mutations API as input (standard for graph processor)
	ports = append(ports, component.Port{
		Name:        "mutations_api",
		Direction:   component.DirectionInput,
		Description: "Request/Reply API for graph mutations",
		Required:    false,
		Config: component.NATSRequestPort{
			Subject: "graph.mutations",
			Timeout: "500ms",
		},
	})

	return ports
}

// buildOutputPortsFromConfig creates output ports from Ports.Outputs configuration
func (p *Processor) buildOutputPortsFromConfig() []component.Port {
	var ports []component.Port

	for _, def := range p.config.Ports.Outputs {
		port := component.Port{
			Name:        def.Name,
			Direction:   component.DirectionOutput,
			Description: def.Description,
			Required:    def.Required,
		}

		// Build appropriate port config based on type
		switch def.Type {
		case "jetstream":
			port.Config = component.JetStreamPort{
				Subjects:   []string{def.Subject},
				StreamName: def.StreamName,
				Interface: func() *component.InterfaceContract {
					if def.Interface != "" {
						return &component.InterfaceContract{Type: def.Interface, Version: "v1"}
					}
					return nil
				}(),
			}
		case "nats":
			port.Config = component.NATSPort{
				Subject: def.Subject,
				Interface: func() *component.InterfaceContract {
					if def.Interface != "" {
						return &component.InterfaceContract{Type: def.Interface, Version: "v1"}
					}
					return nil
				}(),
			}
		case "kv-write":
			port.Config = component.KVWritePort{
				Bucket: def.Subject, // Subject field used for bucket name in KV context
				Interface: func() *component.InterfaceContract {
					if def.Interface != "" {
						return &component.InterfaceContract{Type: def.Interface, Version: "v1"}
					}
					return nil
				}(),
			}
		default:
			// Default to NATS port
			port.Config = component.NATSPort{
				Subject: def.Subject,
			}
		}

		ports = append(ports, port)
	}

	// Always include KV bucket outputs (standard for graph processor)
	ports = append(ports, component.Port{
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
	})
	ports = append(ports, component.Port{
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
	})

	return ports
}

// DefaultConfig returns default processor configuration
func DefaultConfig() *Config {
	return &Config{
		Workers:       10,
		QueueSize:     10000,
		MaxAckPending: 20,                 // 2x workers for NATS-level backpressure
		InputSubject:  "storage.*.events", // Subscribe to ObjectStore events

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
	// Standard ports mode: wait for streams from jetstream port definitions
	// Legacy InputSubjects mode: wait for all convention-derived streams
	// Legacy single stream mode: wait for single configured stream
	if p.config.Ports != nil && len(p.config.Ports.Inputs) > 0 {
		if err := p.waitForPortStreams(ctx); err != nil {
			return err
		}
	} else if len(p.config.InputSubjects) > 0 {
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

	// Start gateway output ports (GraphQL/MCP) after QueryManager is ready
	if err := p.startGateways(ctx); err != nil {
		return errs.WrapFatal(err, "Processor", "Start", "gateway startup failed")
	}

	// Mark component as healthy LAST
	p.markComponentHealthy()

	p.logger.Info("Graph processor started successfully - all subsystems ready")

	return nil
}

// setupWorkerPoolAndHandlers creates worker pool and sets up NATS handlers
func (p *Processor) setupWorkerPoolAndHandlers(ctx context.Context) error {
	// Initialize worker pool (always create a fresh instance)
	// Uses processWorkWithAck to ACK messages only after successful KV write
	p.logger.Debug("Creating worker pool", "workers", p.config.Workers, "queue_size", p.config.QueueSize)
	workerPool := worker.NewPool(
		p.config.Workers,
		p.config.QueueSize,
		p.processWorkWithAck,
		worker.WithMetricsRegistry[*ProcessWork](p.metricsRegistry, "graph_processor"),
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

		// Launch community detection loop if enabled
		if p.isCommunityDetectionEnabled() && p.communityDetector != nil {
			g.Go(func() error {
				return p.runClusteringLoop(gctx)
			})

			// Launch enhancement worker if configured
			if p.enhancementWorker != nil {
				g.Go(func() error {
					return p.runEnhancementWorker(gctx)
				})
			}

			// Launch review worker if configured
			if p.reviewWorker != nil {
				g.Go(func() error {
					return p.runReviewWorker(gctx)
				})
			}
		}

		// Launch hierarchy inference worker if configured (independent of community detection)
		if p.hierarchyInference != nil {
			g.Go(func() error {
				return p.runHierarchyInference(gctx)
			})
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

	// Stop gateway output ports first (they depend on QueryManager)
	p.stopGateways(timeout)

	// Stop background modules
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

	// Initialize clustering core BEFORE QueryManager (creates p.communityDetector for DI)
	// Pass indexer explicitly since p.indexManager isn't assigned yet
	if err := p.initializeClusteringCore(ctx, buckets, dataHandler, indexer); err != nil {
		return err
	}

	// Initialize QueryManager with CommunityDetector available
	querier, err := p.initializeQueryManager(dataHandler, indexer)
	if err != nil {
		return err
	}

	// Assign all managers atomically
	p.assignManagers(dataHandler, indexer, querier)

	// Wire up entity provider for community summarization now that queryManager is available.
	// This completes the deferred initialization started in createCommunityDetector.
	p.completeCommunityDetectorSetup()

	// Wire up applier for anomaly detection now that dataManager is available.
	// This completes the deferred initialization started in initializeAnomalyDetectionIfEnabled.
	if err := p.completeAnomalyDetectorSetup(); err != nil {
		return err
	}

	// Complete clustering setup (requires p.dataManager from assignManagers)
	if err := p.initializeClusteringCallbacks(); err != nil {
		return err
	}

	// Initialize hierarchy inference (requires p.dataManager and p.indexManager from assignManagers)
	p.initializeHierarchyInference()

	// Initialize enhancement worker (requires p.queryManager from assignManagers)
	if err := p.initializeEnhancementWorkerIfEnabled(ctx); err != nil {
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
		{"OUTGOING_INDEX", "Outgoing edge index for relationship queries"},
		{"SPATIAL_INDEX", "Spatial index for geospatial queries"},
		{"TEMPORAL_INDEX", "Temporal index for time-based queries"},
		{"CONTEXT_INDEX", "Context/provenance index for triple origin tracking"},
		{"EMBEDDING_INDEX", "Vector embeddings with metadata and status"},
		{"EMBEDDING_DEDUP", "Content-addressed embedding deduplication"},
		{"COMMUNITY_INDEX", "Graph community detection and clustering"},
		{"ANOMALY_INDEX", "Structural anomaly detection results"},
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
		Config:            queryConfig,
		EntityReader:      entityReader,
		IndexManager:      indexer,
		CommunityDetector: p.communityDetector, // Available via initializeClusteringCore
		ContentFetcher:    p.contentFetcher,
		Registry:          p.metricsRegistry,
		Logger:            p.logger,
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
	// Standard ports pattern: use Ports config if available
	if p.config.Ports != nil && len(p.config.Ports.Inputs) > 0 {
		return p.setupPortsSubscriptions(ctx)
	}

	// Legacy: check for multi-stream subscription mode (InputSubjects)
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

// setupPortsSubscriptions sets up subscriptions based on Ports configuration.
// This is the standard pattern - each input port definition specifies type and subject.
func (p *Processor) setupPortsSubscriptions(ctx context.Context) error {
	p.logger.Info("Setting up subscriptions from ports configuration",
		"input_count", len(p.config.Ports.Inputs))

	for _, portDef := range p.config.Ports.Inputs {
		// Skip non-subscribable port types
		if portDef.Type == "nats-request" || portDef.Type == "kv-watch" {
			continue
		}

		subject := portDef.Subject
		if subject == "" {
			p.logger.Warn("Port has no subject, skipping", "port", portDef.Name)
			continue
		}

		switch portDef.Type {
		case "jetstream":
			// JetStream subscription - derive stream name from subject
			streamName := portDef.StreamName
			if streamName == "" {
				streamName = config.DeriveStreamName(subject)
			}
			if streamName == "" {
				p.logger.Warn("Could not derive stream name from subject, using NATS",
					"port", portDef.Name, "subject", subject)
				if err := p.setupNATSSubscription(ctx, subject); err != nil {
					return err
				}
				continue
			}

			// Wait for stream and setup consumer
			if err := p.waitForStream(ctx, streamName); err != nil {
				return errs.WrapFatal(err, "Processor", "setupPortsSubscriptions",
					fmt.Sprintf("stream %s not available for port %s", streamName, portDef.Name))
			}
			if err := p.setupStreamConsumer(ctx, streamName, subject); err != nil {
				return err
			}

		case "nats":
			// Simple NATS subscription
			if err := p.setupNATSSubscription(ctx, subject); err != nil {
				return err
			}

		default:
			p.logger.Warn("Unknown port type, treating as NATS",
				"port", portDef.Name, "type", portDef.Type)
			if err := p.setupNATSSubscription(ctx, subject); err != nil {
				return err
			}
		}
	}

	return nil
}

// setupNATSSubscription creates a simple NATS subscription for a subject
func (p *Processor) setupNATSSubscription(ctx context.Context, subject string) error {
	p.logger.Info("Setting up NATS subscription", "subject", subject)

	err := p.natsClient.Subscribe(ctx, subject, func(msgCtx context.Context, data []byte) {
		p.handleMessage(msgCtx, data)
	})
	if err != nil {
		return errs.WrapFatal(err, "Processor", "setupNATSSubscription",
			"NATS subscription failed for "+subject)
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
	// NATS consumer names cannot contain '.', '*', or '>' characters
	sanitizedSubject := strings.ReplaceAll(filterSubject, ".", "-")
	sanitizedSubject = strings.ReplaceAll(sanitizedSubject, "*", "all")
	sanitizedSubject = strings.ReplaceAll(sanitizedSubject, ">", "wildcard")
	consumerName := fmt.Sprintf("graph-%s-%s",
		strings.ToLower(streamName),
		sanitizedSubject)

	p.logger.Info("Setting up stream consumer",
		"stream", streamName,
		"consumer", consumerName,
		"filter_subject", filterSubject)

	cfg := natsclient.StreamConsumerConfig{
		StreamName:    streamName,
		ConsumerName:  consumerName,
		FilterSubject: filterSubject,
		DeliverPolicy: "all",                  // Process all messages including historical
		AckPolicy:     "explicit",             // Explicit ack required
		MaxDeliver:    5,                      // Retry up to 5 times before giving up
		MaxAckPending: p.config.MaxAckPending, // NATS-level backpressure
		AutoCreate:    false,                  // Stream should already exist (created by StreamsManager)
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

// waitForPortStreams waits for all JetStream streams required by port definitions.
// Only waits for ports with type="jetstream" - NATS ports don't require streams.
func (p *Processor) waitForPortStreams(ctx context.Context) error {
	streamSet := make(map[string]bool)

	for _, portDef := range p.config.Ports.Inputs {
		// Only jetstream ports require waiting for streams
		if portDef.Type != "jetstream" {
			continue
		}

		// Use explicit stream name if provided, otherwise derive from subject
		streamName := portDef.StreamName
		if streamName == "" {
			streamName = config.DeriveStreamName(portDef.Subject)
		}
		if streamName != "" {
			streamSet[streamName] = true
		}
	}

	if len(streamSet) == 0 {
		p.logger.Debug("No JetStream ports configured, skipping stream wait")
		return nil
	}

	p.logger.Info("Waiting for port streams",
		"stream_count", len(streamSet))

	for streamName := range streamSet {
		if err := p.waitForStream(ctx, streamName); err != nil {
			p.logger.Error("Stream not available", "stream", streamName, "error", err)
			return errs.WrapFatal(err, "Processor", "waitForPortStreams",
				"stream "+streamName+" not available")
		}
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
		DeliverPolicy: "all",                  // Process all messages including historical
		AckPolicy:     "explicit",             // Explicit ack required
		MaxDeliver:    5,                      // Retry up to 5 times before giving up
		MaxAckPending: p.config.MaxAckPending, // NATS-level backpressure
		AutoCreate:    true,                   // Auto-create stream if it doesn't exist
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
// Messages are ACK'd only after successful processing (KV write complete), not on queue submission.
// This ensures "at least once" delivery semantics - messages are redelivered on failure.
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

	// Create work item with message for ACK/NAK after processing
	work := &ProcessWork{
		Data: msg.Data(),
		Msg:  msg,
	}

	// Submit to worker pool - ACK/NAK happens in processWorkWithAck after processing completes
	if err := p.workerPool.Submit(work); err != nil {
		// Check error type to determine acknowledgment strategy
		if stderrors.Is(err, worker.ErrPoolStopped) || stderrors.Is(err, worker.ErrPoolNotStarted) {
			// Worker pool stopped unexpectedly - nak for redelivery
			p.recordError("Worker pool stopped unexpectedly")
			p.logger.Error("Worker pool no longer running",
				"error", err,
				"data_len", len(work.Data))
			_ = msg.Nak()
		} else if stderrors.Is(err, worker.ErrQueueFull) {
			// Queue full is transient - nak with delay for backpressure
			p.logger.Debug("Worker queue full, nak-ing message",
				"data_len", len(work.Data))
			_ = msg.NakWithDelay(time.Second) // Delay redelivery by 1s
		} else {
			// Unknown error - nak for redelivery
			p.recordError(fmt.Sprintf("Unexpected worker pool error: %v", err))
			p.logger.Warn("Failed to submit message to worker pool",
				"data_len", len(work.Data),
				"error", err)
			_ = msg.Nak()
		}
		return
	}
	// NOTE: Do NOT ACK here - worker will ACK after successful processing in processWorkWithAck
}

// Message handling - pure orchestration

// processWorkWithAck processes a work item and ACKs/NAKs the JetStream message after completion.
// This ensures "at least once" delivery semantics by only ACKing after successful KV write.
func (p *Processor) processWorkWithAck(ctx context.Context, work *ProcessWork) error {
	// Process the message data through the message manager
	err := p.messageManager.ProcessWork(ctx, work.Data)

	// ACK or NAK the JetStream message based on processing result
	if work.Msg != nil {
		if err != nil {
			// Processing failed - NAK for redelivery
			if nakErr := work.Msg.Nak(); nakErr != nil {
				p.logger.Warn("Failed to NAK message after processing error",
					"error", nakErr, "processing_error", err)
			}
		} else {
			// Processing succeeded - ACK to confirm delivery
			if ackErr := work.Msg.Ack(); ackErr != nil {
				p.logger.Warn("Failed to ACK message after successful processing",
					"error", ackErr)
			}
		}
	}

	return err
}

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

	// Create work item without JetStream message (no ACK needed for non-JetStream)
	work := &ProcessWork{
		Data: data,
		Msg:  nil, // No ACK/NAK for non-JetStream messages
	}

	// Delegate to worker pool which uses processWorkWithAck
	if err := p.workerPool.Submit(work); err != nil {
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
	indexManager indexmanager.Indexer
	logger       *slog.Logger
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
	neighborSet := make(map[string]bool)
	var outgoingCount, incomingCount int

	// Outgoing: from entity's triples (entity → neighbors)
	if direction == "outgoing" || direction == "both" {
		entity, err := p.entityReader.GetEntity(ctx, entityID)
		if err == nil {
			for _, triple := range entity.Triples {
				if triple.IsRelationship() {
					neighborSet[triple.Object.(string)] = true
					outgoingCount++
				}
			}
		}
	}

	// Incoming: from INCOMING_INDEX via indexManager (neighbors → entity)
	if (direction == "incoming" || direction == "both") && p.indexManager != nil {
		incomingRels, err := p.indexManager.GetIncomingRelationships(ctx, entityID)
		if err == nil {
			for _, rel := range incomingRels {
				neighborSet[rel.FromEntityID] = true
				incomingCount++
			}
		}
	}

	neighbors := make([]string, 0, len(neighborSet))
	for id := range neighborSet {
		neighbors = append(neighbors, id)
	}

	// Debug logging for graph traversal diagnostics
	if p.logger != nil && (outgoingCount > 0 || incomingCount > 0) {
		p.logger.Debug("GetNeighbors completed",
			"entity_id", entityID,
			"direction", direction,
			"outgoing", outgoingCount,
			"incoming", incomingCount,
			"total", len(neighbors))
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

// initializeClusteringCore sets up core clustering components needed for QueryManager DI.
// This creates the CommunityDetector but defers callback setup until dataManager is assigned.
// Must be called BEFORE initializeQueryManager so CommunityDetector is available for injection.
// The indexer parameter is passed explicitly since p.indexManager isn't assigned yet.
func (p *Processor) initializeClusteringCore(ctx context.Context, buckets map[string]jetstream.KeyValue, entityReader datamanager.EntityReader, indexer indexmanager.Indexer) error {
	if !p.isCommunityDetectionEnabled() {
		// Even if community detection is disabled, we may still need structural indexing
		return p.initializeStructuralIndexOnly(ctx, buckets, entityReader, indexer)
	}

	cfg := p.config.GraphAnalysis.CommunityDetection
	p.logger.Info("Initializing graph analysis (community detection)",
		"max_iterations", cfg.Algorithm.MaxIterations,
		"levels", cfg.Algorithm.Levels,
		"enhancement_enabled", cfg.Enhancement.Enabled)

	p.clusteringBuckets = buckets

	_, graphProvider, err := p.setupGraphProvider(ctx, buckets, cfg, entityReader, indexer)
	if err != nil {
		return err
	}

	p.communityDetector = p.createCommunityDetector(graphProvider, cfg)
	p.initializeInferenceMetrics()
	p.initializeClusteringMetrics()

	// Cache graph provider for structural index computation
	p.graphProvider = graphProvider

	// Initialize structural index computer if enabled (from GraphAnalysis, not CommunityDetection)
	if err := p.initializeStructuralIndexIfEnabled(graphProvider); err != nil {
		return err
	}

	// Initialize anomaly detection if enabled
	if err := p.initializeAnomalyDetectionIfEnabled(ctx, buckets); err != nil {
		return err
	}

	// NOTE: setupEnhancementWorker is called separately in initializeEnhancementWorkerIfEnabled
	// AFTER assignManagers, because it requires p.queryManager to be set.

	return nil
}

// completeCommunityDetectorSetup wires up the entity provider for community summarization.
// Must be called AFTER assignManagers so p.queryManager is available.
// This completes the deferred initialization started in createCommunityDetector,
// which couldn't set the entity provider because queryManager wasn't available yet.
func (p *Processor) completeCommunityDetectorSetup() {
	if p.communityDetector == nil || p.queryManager == nil {
		return
	}

	// Type assert to LPADetector to call SetEntityProvider
	if lpa, ok := p.communityDetector.(*clustering.LPADetector); ok {
		lpa.SetEntityProvider(p.queryManager)
		p.logger.Debug("Community detector entity provider configured")
	}
}

// initializeClusteringCallbacks sets up entity change callbacks for clustering.
// Must be called AFTER assignManagers so p.dataManager is available.
func (p *Processor) initializeClusteringCallbacks() error {
	if !p.isCommunityDetectionEnabled() {
		return nil
	}

	cfg := p.config.GraphAnalysis.CommunityDetection
	p.setupEntityChangeCallback(cfg)
	return nil
}

// initializeHierarchyInference sets up hierarchy inference for automatic container membership edges.
// Must be called AFTER assignManagers so p.dataManager and p.indexManager are available.
//
// Hierarchy inference creates membership edges to container entities based on 6-part entity IDs:
//   - Type container:   entity → hierarchy.type.member → org.platform.domain.system.type.group
//   - System container: entity → hierarchy.system.member → org.platform.domain.system.group.container
//   - Domain container: entity → hierarchy.domain.member → org.platform.domain.group.container.level
//
// This enables graph traversal between related entities (same type = 2 hops, same system = 4 hops).
func (p *Processor) initializeHierarchyInference() {
	// Check if hierarchy inference is configured and enabled
	if p.config.GraphAnalysis == nil || p.config.GraphAnalysis.HierarchyInference == nil {
		p.logger.Debug("Hierarchy inference not configured, skipping initialization")
		return
	}

	cfg := p.config.GraphAnalysis.HierarchyInference
	if !cfg.Enabled {
		p.logger.Debug("Hierarchy inference disabled, skipping initialization")
		return
	}

	// Get ENTITY_STATES bucket for KV watching
	entityBucket, ok := p.clusteringBuckets["ENTITY_STATES"]
	if !ok {
		p.logger.Warn("ENTITY_STATES bucket not found, hierarchy inference cannot watch for entities")
		entityBucket = nil // Will still create component but Start() will skip watching
	}

	// Create hierarchy inference component with KV watcher support
	// DataManager implements EntityManager (ExistsEntity, CreateEntity) and TripleAdder
	p.hierarchyInference = inference.NewHierarchyInference(
		p.dataManager, // Implements EntityManager (ExistsEntity, CreateEntity)
		p.dataManager, // Implements TripleAdder (AddTriple)
		entityBucket,  // ENTITY_STATES bucket for KV watching
		*cfg,
		p.logger,
	)

	// Note: Start() is called in startBackgroundModules() after all initialization is complete.
	// The KV watcher pattern replaces the callback-based approach for better decoupling.

	p.logger.Info("Hierarchy inference initialized (watcher starts in background)",
		"create_type_edges", cfg.CreateTypeEdges,
		"create_system_edges", cfg.CreateSystemEdges,
		"create_domain_edges", cfg.CreateDomainEdges)
}

// Note: backfillHierarchyEdges removed - HierarchyInference's KV watcher now handles
// backfill automatically. WatchAll() delivers existing entries first before live updates,
// and the container cache ensures idempotency.

// initializeEnhancementWorkerIfEnabled sets up the LLM enhancement worker.
// Must be called AFTER assignManagers so p.queryManager is available.
func (p *Processor) initializeEnhancementWorkerIfEnabled(ctx context.Context) error {
	if !p.isCommunityDetectionEnabled() {
		return nil
	}

	cfg := p.config.GraphAnalysis.CommunityDetection
	if !cfg.Enhancement.Enabled {
		return nil
	}

	// Get the community bucket from cached buckets
	communityBucket, ok := p.clusteringBuckets["COMMUNITY_INDEX"]
	if !ok {
		return errs.WrapFatal(errs.ErrMissingConfig, "Processor",
			"initializeEnhancementWorkerIfEnabled", "COMMUNITY_INDEX bucket not found")
	}

	if p.graphProvider == nil {
		return errs.WrapFatal(errs.ErrMissingConfig, "Processor",
			"initializeEnhancementWorkerIfEnabled", "graph provider not initialized")
	}

	return p.setupEnhancementWorker(ctx, cfg, communityBucket, p.graphProvider)
}

// isCommunityDetectionEnabled checks if community detection is configured and enabled.
func (p *Processor) isCommunityDetectionEnabled() bool {
	if p.config.GraphAnalysis == nil {
		p.logger.Info("GraphAnalysis config nil, community detection disabled")
		return false
	}
	if p.config.GraphAnalysis.CommunityDetection == nil {
		p.logger.Info("CommunityDetection config nil, community detection disabled")
		return false
	}
	enabled := p.config.GraphAnalysis.CommunityDetection.Enabled
	p.logger.Info("Checking community detection configuration", "enabled", enabled)
	if !enabled {
		p.logger.Info("Community detection disabled, skipping initialization")
		return false
	}
	return true
}

// setupGraphProvider creates the graph provider and community storage from buckets.
// The indexer parameter is passed explicitly since p.indexManager may not be assigned yet.
func (p *Processor) setupGraphProvider(ctx context.Context, buckets map[string]jetstream.KeyValue, cfg *CommunityDetectionConfig, entityReader datamanager.EntityReader, indexer indexmanager.Indexer) (jetstream.KeyValue, clustering.GraphProvider, error) {
	// Check context before bucket operations
	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	default:
	}

	communityBucket, ok := buckets["COMMUNITY_INDEX"]
	if !ok {
		return nil, nil, errs.WrapFatal(errs.ErrMissingConfig, "Processor",
			"initializeClusteringCore", "COMMUNITY_INDEX bucket not found")
	}
	p.communityStorage = clustering.NewNATSCommunityStorage(communityBucket)

	entityBucket, ok := buckets["ENTITY_STATES"]
	if !ok {
		return nil, nil, errs.WrapFatal(errs.ErrMissingConfig, "Processor",
			"initializeClusteringCore", "ENTITY_STATES bucket not found")
	}

	baseProvider := &processorGraphProvider{
		entityReader: entityReader,
		kvBucket:     entityBucket,
		indexManager: indexer,
		logger:       p.logger,
	}

	graphProvider := p.buildGraphProviderChain(baseProvider, cfg)
	return communityBucket, graphProvider, nil
}

// buildGraphProviderChain wraps the base provider with configured edge providers.
// Each layer adds virtual edges for clustering/structural analysis:
//   - EntityID edges: siblings based on 6-part ID hierarchy (no ML, zero-cost)
//   - Semantic edges: similarity based on embeddings (requires ML)
//
// Order: base → entityID edges → semantic edges
func (p *Processor) buildGraphProviderChain(base clustering.GraphProvider, cfg *CommunityDetectionConfig) clustering.GraphProvider {
	provider := base

	// Layer 1: EntityID sibling edges (hierarchy-based, no ML required)
	// Skip if hierarchy inference is enabled - real edges are created at ingestion time
	hierarchyInferenceEnabled := p.config.GraphAnalysis != nil &&
		p.config.GraphAnalysis.HierarchyInference != nil &&
		p.config.GraphAnalysis.HierarchyInference.Enabled

	if cfg.EntityIDEdges != nil && cfg.EntityIDEdges.Enabled && !hierarchyInferenceEnabled {
		entityIDConfig := clustering.EntityIDProviderConfig{
			SiblingWeight:   cfg.EntityIDEdges.SiblingWeight,
			MaxSiblings:     cfg.EntityIDEdges.MaxSiblings,
			IncludeSiblings: cfg.EntityIDEdges.IncludeSiblings,
		}
		provider = clustering.NewEntityIDGraphProvider(provider, entityIDConfig, p.logger)
		p.logger.Info("EntityID sibling edges enabled for graph analysis",
			"sibling_weight", cfg.EntityIDEdges.SiblingWeight,
			"max_siblings", cfg.EntityIDEdges.MaxSiblings)
	} else if hierarchyInferenceEnabled {
		p.logger.Info("EntityID virtual sibling edges disabled (hierarchy inference creates real edges)")
	}

	// Layer 2: Semantic similarity edges (embedding-based, requires ML)
	if cfg.SemanticEdges.Enabled && p.indexManager != nil {
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
		provider = clustering.NewSemanticGraphProvider(provider, p.indexManager, semanticConfig, p.logger)
		p.logger.Info("Semantic similarity edges enabled for graph analysis",
			"similarity_threshold", threshold,
			"max_virtual_neighbors", maxNeighbors)
	}

	return provider
}

// buildGraphProviderChainForStructural wraps provider with EntityID edges
// when structural analysis is enabled but community detection is not.
// This allows k-core/pivot to benefit from the 6-part EntityID hierarchy
// without requiring ML-based community detection.
func (p *Processor) buildGraphProviderChainForStructural(base clustering.GraphProvider) clustering.GraphProvider {
	// Check if EntityID edges should be enabled from graph_analysis config
	if p.config.GraphAnalysis == nil || p.config.GraphAnalysis.EntityIDEdges == nil {
		return base
	}
	cfg := p.config.GraphAnalysis.EntityIDEdges
	if !cfg.Enabled {
		return base
	}

	// Skip if hierarchy inference is enabled - real edges are created at ingestion time
	if p.config.GraphAnalysis.HierarchyInference != nil &&
		p.config.GraphAnalysis.HierarchyInference.Enabled {
		p.logger.Info("EntityID virtual sibling edges disabled for structural analysis (hierarchy inference creates real edges)")
		return base
	}

	entityIDConfig := clustering.EntityIDProviderConfig{
		SiblingWeight:   cfg.SiblingWeight,
		MaxSiblings:     cfg.MaxSiblings,
		IncludeSiblings: cfg.IncludeSiblings,
	}
	p.logger.Info("EntityID sibling edges enabled for structural analysis",
		"sibling_weight", cfg.SiblingWeight,
		"max_siblings", cfg.MaxSiblings)
	return clustering.NewEntityIDGraphProvider(base, entityIDConfig, p.logger)
}

// createCommunityDetector creates the LPA detector with configuration.
func (p *Processor) createCommunityDetector(graphProvider clustering.GraphProvider, cfg *CommunityDetectionConfig) clustering.CommunityDetector {
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

	// Use WithSummarizer instead of WithProgressiveSummarization because
	// p.queryManager is not available yet (assigned in assignManagers later).
	// SetEntityProvider is called after assignManagers to complete the setup.
	summarizer := clustering.NewProgressiveSummarizer()
	return detector.WithSummarizer(summarizer)
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

// initializeClusteringMetrics creates metrics for clustering observability (US3).
func (p *Processor) initializeClusteringMetrics() {
	if p.metricsRegistry == nil {
		return
	}

	p.clusteringEmbeddingCoverage = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "semstreams_clustering_embedding_coverage",
		Help: "Current embedding coverage ratio (0-1) for community detection",
	})
	p.metricsRegistry.RegisterGauge("graph", "semstreams_clustering_embedding_coverage", p.clusteringEmbeddingCoverage)

	p.clusteringWindowActive = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "semstreams_clustering_window_active",
		Help: "Enhancement window state (1=active, 0=inactive)",
	})
	p.metricsRegistry.RegisterGauge("graph", "semstreams_clustering_window_active", p.clusteringWindowActive)

	p.clusteringFailuresTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "semstreams_clustering_failures_total",
		Help: "Total community detection failures",
	})
	p.metricsRegistry.RegisterCounter("graph", "semstreams_clustering_failures_total", p.clusteringFailuresTotal)
}

// isStructuralIndexEnabled checks if structural indexing is configured and enabled.
func (p *Processor) isStructuralIndexEnabled() bool {
	if p.config.GraphAnalysis == nil {
		return false
	}
	return p.config.GraphAnalysis.StructuralIndex.Enabled
}

// initializeStructuralIndexIfEnabled sets up the structural index computer if enabled.
func (p *Processor) initializeStructuralIndexIfEnabled(graphProvider clustering.GraphProvider) error {
	if !p.isStructuralIndexEnabled() {
		return nil
	}

	cfg := p.config.GraphAnalysis.StructuralIndex
	p.structuralComputer = newStructuralIndexComputer(graphProvider, cfg, p.logger)
	p.logger.Info("Structural index computation enabled",
		"kcore_enabled", cfg.KCore.Enabled,
		"pivot_enabled", cfg.Pivot.Enabled,
		"pivot_count", cfg.Pivot.PivotCount)
	return nil
}

// initializeStructuralIndexOnly initializes structural indexing when community detection is disabled.
// This allows k-core and pivot indexes to be computed without LPA community detection.
// The indexer parameter is passed explicitly since p.indexManager may not be assigned yet.
func (p *Processor) initializeStructuralIndexOnly(_ context.Context, buckets map[string]jetstream.KeyValue, entityReader datamanager.EntityReader, indexer indexmanager.Indexer) error {
	if !p.isStructuralIndexEnabled() {
		p.logger.Debug("Structural index not enabled, skipping initialization")
		return nil
	}

	p.logger.Info("Initializing structural index (community detection disabled)")
	p.clusteringBuckets = buckets

	// Get entity bucket for graph provider
	entityBucket, ok := buckets["ENTITY_STATES"]
	if !ok {
		return errs.WrapFatal(errs.ErrMissingConfig, "Processor",
			"initializeStructuralIndexOnly", "ENTITY_STATES bucket not found")
	}

	// Create base graph provider for structural indexing
	baseProvider := &processorGraphProvider{
		entityReader: entityReader,
		kvBucket:     entityBucket,
		indexManager: indexer,
		logger:       p.logger,
	}

	// Apply EntityID edges if configured (provides structure from 6-part hierarchy)
	graphProvider := p.buildGraphProviderChainForStructural(baseProvider)

	// Cache graph provider and initialize structural computer
	p.graphProvider = graphProvider
	return p.initializeStructuralIndexIfEnabled(graphProvider)
}

// initializeAnomalyDetectionIfEnabled sets up anomaly detection components if enabled.
func (p *Processor) initializeAnomalyDetectionIfEnabled(_ context.Context, buckets map[string]jetstream.KeyValue) error {
	if p.config.GraphAnalysis == nil {
		p.logger.Debug("Anomaly detection disabled: no GraphAnalysis config")
		return nil
	}
	if p.config.GraphAnalysis.AnomalyDetection == nil {
		p.logger.Debug("Anomaly detection disabled: no AnomalyDetection config section")
		return nil
	}
	if !p.config.GraphAnalysis.AnomalyDetection.Enabled {
		p.logger.Debug("Anomaly detection disabled: enabled=false")
		return nil
	}
	cfg := p.config.GraphAnalysis.AnomalyDetection

	// Apply defaults for any unset values (e.g., DetectionTimeout)
	cfg.ApplyDefaults()

	p.logger.Info("Initializing anomaly detection")

	// Get ANOMALY_INDEX bucket
	anomalyBucket, ok := buckets["ANOMALY_INDEX"]
	if !ok {
		return errs.WrapFatal(errs.ErrMissingConfig, "Processor",
			"initializeAnomalyDetectionIfEnabled", "ANOMALY_INDEX bucket not found")
	}

	// Create storage
	p.anomalyStorage = inference.NewNATSAnomalyStorage(anomalyBucket, p.logger)

	// Create orchestrator
	orchestratorCfg := inference.OrchestratorConfig{
		Config:  *cfg,
		Storage: p.anomalyStorage,
		Logger:  p.logger,
	}

	orchestrator, err := inference.NewOrchestrator(orchestratorCfg)
	if err != nil {
		return errs.WrapFatal(err, "Processor", "initializeAnomalyDetectionIfEnabled",
			"failed to create inference orchestrator")
	}

	// Register detectors
	orchestrator.RegisterDetector(inference.NewSemanticGapDetector(nil))
	orchestrator.RegisterDetector(inference.NewCoreAnomalyDetector(nil))
	orchestrator.RegisterDetector(inference.NewTransitivityDetector(nil))

	// NOTE: Applier setup is deferred to completeAnomalyDetectorSetup() because
	// it requires p.dataManager which isn't available until after assignManagers().
	// Store config for later use.
	if cfg.VirtualEdges.AutoApply.Enabled {
		p.logger.Info("Virtual edge auto-apply enabled (applier will be configured after managers assigned)",
			"min_similarity", cfg.VirtualEdges.AutoApply.MinSimilarity,
			"min_structural_distance", cfg.VirtualEdges.AutoApply.MinStructuralDistance,
			"predicate_template", cfg.VirtualEdges.AutoApply.PredicateTemplate)
	}

	p.inferenceOrchestrator = orchestrator

	p.logger.Info("Anomaly detection initialized",
		"detectors", orchestrator.GetRegisteredDetectors())

	// NOTE: Review worker initialization is also deferred to completeAnomalyDetectorSetup()
	// because it requires p.dataManager for the DirectRelationshipApplier.

	return nil
}

// completeAnomalyDetectorSetup wires up the applier for the anomaly detection orchestrator.
// Must be called AFTER assignManagers so p.dataManager is available.
// This completes the deferred initialization started in initializeAnomalyDetectionIfEnabled.
func (p *Processor) completeAnomalyDetectorSetup() error {
	if p.inferenceOrchestrator == nil || p.dataManager == nil {
		return nil
	}

	cfg := p.config.GraphAnalysis.AnomalyDetection

	// Set up applier for virtual edge creation
	if cfg.VirtualEdges.AutoApply.Enabled {
		applier := inference.NewDirectRelationshipApplier(p.dataManager, p.logger)
		p.inferenceOrchestrator.SetApplier(applier)
		p.logger.Debug("Anomaly detector applier configured with DataManager")
	}

	// Initialize review worker if enabled
	if cfg.Review.Enabled {
		// Create LLM client if configured
		var llmClient llm.Client
		if cfg.Review.LLM.IsEnabled() {
			var llmErr error
			llmClient, llmErr = llm.NewOpenAIClient(cfg.Review.LLM.ToOpenAIConfig())
			if llmErr != nil {
				return errs.WrapFatal(llmErr, "Processor", "completeAnomalyDetectorSetup",
					"failed to create LLM client for review worker")
			}
		}

		// Use DirectRelationshipApplier for intra-processor mutations
		applier := inference.NewDirectRelationshipApplier(p.dataManager, p.logger)

		// Create review metrics if metrics registry is available
		reviewMetrics := inference.NewReviewMetrics("graph_processor", p.metricsRegistry)

		// Get anomaly bucket from the stored buckets
		anomalyBucket := p.clusteringBuckets["ANOMALY_INDEX"]
		if anomalyBucket == nil {
			p.logger.Warn("ANOMALY_INDEX bucket not found, skipping review worker")
			return nil
		}

		reviewWorkerCfg := &inference.ReviewWorkerConfig{
			AnomalyBucket: anomalyBucket,
			Storage:       p.anomalyStorage,
			LLMClient:     llmClient,
			Applier:       applier,
			Config:        cfg.Review,
			Metrics:       reviewMetrics,
			Logger:        p.logger,
		}

		reviewWorker, rwErr := inference.NewReviewWorker(reviewWorkerCfg)
		if rwErr != nil {
			return errs.WrapFatal(rwErr, "Processor", "completeAnomalyDetectorSetup",
				"failed to create review worker")
		}
		p.reviewWorker = reviewWorker

		p.logger.Info("Review worker initialized",
			"workers", cfg.Review.Workers,
			"auto_approve_threshold", cfg.Review.AutoApproveThreshold,
			"auto_reject_threshold", cfg.Review.AutoRejectThreshold)
	}

	return nil
}

// computeStructuralIndices computes k-core and pivot indices.
// Called after community detection in the clustering loop.
func (p *Processor) computeStructuralIndices(ctx context.Context) error {
	if p.structuralComputer == nil {
		return nil
	}

	startTime := time.Now()
	p.logger.Info("Computing structural indices")

	// Preserve previous k-core for demotion detection (read with lock)
	p.structuralMu.RLock()
	if p.structuralIndices != nil && p.structuralIndices.KCore != nil {
		// Store reference to previous k-core before releasing lock
		prevKCore := p.structuralIndices.KCore
		p.structuralMu.RUnlock()

		// Update previousKCore with write lock
		p.structuralMu.Lock()
		p.previousKCore = prevKCore
		p.structuralMu.Unlock()
	} else {
		p.structuralMu.RUnlock()
	}

	// Compute new indices (expensive operation, outside lock)
	indices, err := p.structuralComputer.Compute(ctx)
	if err != nil {
		p.logger.Error("Structural index computation failed", "error", err)
		return err
	}

	// Update structural indices with write lock
	p.structuralMu.Lock()
	p.structuralIndices = indices
	p.structuralMu.Unlock()

	// Update IndexManager's structural index holder for query-time access
	// (IndexManager's holder has its own mutex protection)
	if p.indexManager != nil {
		if mgr, ok := p.indexManager.(*indexmanager.Manager); ok {
			holder := mgr.GetStructuralIndices()
			if holder != nil {
				holder.SetKCoreIndex(indices.KCore)
				holder.SetPivotIndex(indices.Pivot)
			}
		}
	}

	p.logger.Info("Structural indices computed",
		"duration", time.Since(startTime),
		"entities", indices.KCore.EntityCount,
		"max_core", indices.KCore.MaxCore,
		"pivots", len(indices.Pivot.Pivots))

	return nil
}

// runAnomalyDetection runs anomaly detection using the inference orchestrator.
// Called after structural index computation in the clustering loop.
func (p *Processor) runAnomalyDetection(ctx context.Context) {
	if p.inferenceOrchestrator == nil {
		p.logger.Debug("Skipping anomaly detection: orchestrator not initialized")
		return
	}

	// Read structural indices with lock
	p.structuralMu.RLock()
	indices := p.structuralIndices
	prevKCore := p.previousKCore
	p.structuralMu.RUnlock()

	if indices == nil {
		p.logger.Warn("Skipping anomaly detection: structural indices not available")
		return
	}

	// Pause review worker during detection to avoid processing stale anomalies
	if p.reviewWorker != nil {
		p.reviewWorker.Pause()
		defer p.reviewWorker.Resume()
	}

	startTime := time.Now()
	p.logger.Info("Running anomaly detection")

	// Build dependencies for detectors (using local copies from locked read)
	deps := &inference.DetectorDependencies{
		StructuralIndices:   indices,
		PreviousKCore:       prevKCore,
		SimilarityFinder:    newSimilarityFinderAdapter(p.indexManager),
		RelationshipQuerier: newRelationshipQuerierAdapter(p.indexManager),
		AnomalyStorage:      p.anomalyStorage,
		Logger:              p.logger,
	}

	// Set dependencies on orchestrator
	p.inferenceOrchestrator.SetDependencies(deps)

	// Run detection
	result, err := p.inferenceOrchestrator.RunDetection(ctx)
	if err != nil {
		p.logger.Error("Anomaly detection failed", "error", err)
		return
	}

	p.logger.Info("Anomaly detection completed",
		"duration", time.Since(startTime),
		"anomalies", result.AnomalyCount(),
		"truncated", result.Truncated)

	// Log breakdown by type
	for anomalyType, count := range result.CountByType() {
		p.logger.Info("Anomalies detected", "type", anomalyType, "count", count)
	}
}

// setupEnhancementWorker creates the LLM enhancement worker if enabled.
func (p *Processor) setupEnhancementWorker(ctx context.Context, cfg *CommunityDetectionConfig, communityBucket jetstream.KeyValue, graphProvider clustering.GraphProvider) error {
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
func (p *Processor) setupEntityChangeCallback(cfg *CommunityDetectionConfig) {
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
	if p.config.GraphAnalysis == nil || p.config.GraphAnalysis.CommunityDetection == nil {
		return nil
	}

	cfg := p.config.GraphAnalysis.CommunityDetection

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
	if !p.shouldProceedWithDetection(ctx, entityThreshold) {
		return
	}

	// Skip if previous detection still running
	if !p.acquireDetectionLock() {
		return
	}
	defer p.releaseDetectionLock()

	// Check entity count threshold
	entityCount, ok := p.checkEntityCountThreshold(ctx, minEntities)
	if !ok {
		return
	}

	// Check embedding coverage for semantic clustering
	if !p.checkEmbeddingCoverage(ctx, entityCount) {
		return
	}

	// Run community detection
	communities := p.executeCommunityDetection(ctx, enhancementWindow)
	if communities == nil {
		return
	}

	// Run statistical inference if enabled
	if p.config.GraphAnalysis.CommunityDetection.Inference.Enabled {
		p.runInference(ctx, communities)
	}
}

// shouldProceedWithDetection checks if detection should proceed based on enhancement window state.
func (p *Processor) shouldProceedWithDetection(ctx context.Context, entityThreshold int) bool {
	if p.enhancementDeadline.IsZero() || !time.Now().Before(p.enhancementDeadline) {
		return true
	}

	switch p.enhancementMode {
	case WindowModeBlocking:
		allTerminal, _ := p.checkAllCommunitiesTerminal(ctx)
		if allTerminal {
			p.logger.Info("Enhancement window: all communities terminal, allowing detection")
			p.enhancementDeadline = time.Time{}
			// Update window active metric (US3)
			if p.clusteringWindowActive != nil {
				p.clusteringWindowActive.Set(0)
			}
			return true
		}
		p.logger.Debug("Enhancement window active, skipping detection",
			"deadline", p.enhancementDeadline,
			"remaining", time.Until(p.enhancementDeadline))
		return false

	case WindowModeSoft:
		if int(p.entityChangeCount.Load()) < entityThreshold {
			p.logger.Debug("Enhancement window (soft) active, skipping detection",
				"deadline", p.enhancementDeadline,
				"entity_changes", p.entityChangeCount.Load(),
				"threshold", entityThreshold)
			return false
		}
		p.logger.Info("Enhancement window soft override: significant entity changes",
			"entity_changes", p.entityChangeCount.Load())
		return true

	case WindowModeNone:
		return true
	}

	return true
}

// acquireDetectionLock attempts to acquire the detection lock. Returns false if already running.
func (p *Processor) acquireDetectionLock() bool {
	p.detectionMu.Lock()
	defer p.detectionMu.Unlock()

	if p.detectionRunning {
		p.logger.Debug("Skipping detection - previous run still in progress")
		return false
	}
	p.detectionRunning = true
	return true
}

// releaseDetectionLock releases the detection lock.
func (p *Processor) releaseDetectionLock() {
	p.detectionMu.Lock()
	p.detectionRunning = false
	p.detectionMu.Unlock()
}

// checkEntityCountThreshold checks if there are enough entities for detection.
func (p *Processor) checkEntityCountThreshold(ctx context.Context, minEntities int) (int, bool) {
	if p.clusteringBuckets == nil {
		return 0, true
	}

	entityBucket, ok := p.clusteringBuckets["ENTITY_STATES"]
	if !ok {
		return 0, true
	}

	keys, err := entityBucket.ListKeys(ctx)
	if err != nil {
		return 0, true
	}

	var entityCount int
	for range keys.Keys() {
		entityCount++
	}

	if entityCount < minEntities {
		p.logger.Debug("Skipping detection - not enough entities",
			"current", entityCount, "required", minEntities)
		return entityCount, false
	}

	return entityCount, true
}

// checkEmbeddingCoverage checks if embedding coverage is sufficient for semantic clustering.
func (p *Processor) checkEmbeddingCoverage(ctx context.Context, entityCount int) bool {
	if !p.isSemanticClusteringEnabled() || p.indexManager == nil || entityCount == 0 {
		return true
	}

	// Get embeddable entity count (excluding hierarchy containers)
	embeddableCount, err := p.getEmbeddableEntityCount(ctx)
	if err != nil {
		p.logger.Warn("Failed to get embeddable entity count, using total", "error", err)
		embeddableCount = entityCount
	}

	if embeddableCount == 0 {
		return true // No embeddable entities yet
	}

	// First, check cache coverage
	cacheEmbeddingCount := p.indexManager.GetEmbeddingCount()
	cacheCoverage := float64(cacheEmbeddingCount) / float64(embeddableCount)
	minCoverage := p.getMinEmbeddingCoverage()

	coverageSource := "cache"
	embeddingCount := cacheEmbeddingCount
	coverage := cacheCoverage

	// If cache coverage is insufficient, fallback to KV query for accurate count
	if cacheCoverage < minCoverage {
		p.logger.Debug("Cache coverage below threshold, querying KV for accurate count",
			"cache_coverage", fmt.Sprintf("%.1f%%", cacheCoverage*100),
			"min_coverage", fmt.Sprintf("%.1f%%", minCoverage*100))

		kvCount, kvErr := p.indexManager.CountEmbeddingsInKV(ctx)
		if kvErr != nil {
			p.logger.Warn("KV embedding count query failed, using cache value",
				"error", kvErr,
				"cache_count", cacheEmbeddingCount)
			// Continue with cache values on error
		} else {
			// Use KV count for coverage check
			embeddingCount = kvCount
			coverage = float64(kvCount) / float64(embeddableCount)
			coverageSource = "kv"

			p.logger.Debug("KV query completed",
				"kv_count", kvCount,
				"cache_count", cacheEmbeddingCount,
				"kv_coverage", fmt.Sprintf("%.1f%%", coverage*100))
		}
	} else {
		p.logger.Debug("Cache coverage sufficient, no KV query needed",
			"cache_coverage", fmt.Sprintf("%.1f%%", cacheCoverage*100),
			"min_coverage", fmt.Sprintf("%.1f%%", minCoverage*100))
	}

	// Update coverage metric (US3)
	if p.clusteringEmbeddingCoverage != nil {
		p.clusteringEmbeddingCoverage.Set(coverage)
	}

	// Check final coverage
	if coverage < minCoverage {
		p.logger.Info("Skipping detection - waiting for embeddings",
			"total_entities", entityCount,
			"embeddable_entities", embeddableCount,
			"embeddings", embeddingCount,
			"coverage", fmt.Sprintf("%.1f%%", coverage*100),
			"min_coverage", fmt.Sprintf("%.1f%%", minCoverage*100),
			"source", coverageSource)
		return false
	}

	p.logger.Info("Embedding coverage sufficient for semantic clustering",
		"total_entities", entityCount,
		"embeddable_entities", embeddableCount,
		"embeddings", embeddingCount,
		"coverage", fmt.Sprintf("%.1f%%", coverage*100),
		"source", coverageSource)
	return true
}

// isContainerEntity checks if an entity ID represents a hierarchy container.
// Container entities have IDs ending in .group, .group.container, or .group.container.level
func isContainerEntity(entityID string) bool {
	return strings.HasSuffix(entityID, ".group") ||
		strings.HasSuffix(entityID, ".group.container") ||
		strings.HasSuffix(entityID, ".group.container.level")
}

// getEmbeddableEntityCount returns the count of entities that could have embeddings.
// Excludes hierarchy container entities which have no text content.
func (p *Processor) getEmbeddableEntityCount(ctx context.Context) (int, error) {
	allIDs, err := p.indexManager.GetAllEntityIDs(ctx)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, id := range allIDs {
		if !isContainerEntity(id) {
			count++
		}
	}
	return count, nil
}

// executeCommunityDetection runs the detection algorithm and post-processing.
func (p *Processor) executeCommunityDetection(ctx context.Context, enhancementWindow time.Duration) map[int][]*clustering.Community {
	// Check backoff first
	if p.shouldSkipDetection() {
		p.logger.Debug("Skipping detection due to backoff",
			"backoff_until", p.lastBackoffUntil)
		return nil
	}

	p.logger.Info("Running community detection")
	startTime := time.Now()

	// Pause enhancement worker during detection to prevent races
	if p.enhancementWorker != nil {
		p.enhancementWorker.Pause()
		defer p.enhancementWorker.Resume()
	}

	// Clear graph provider caches to ensure fresh data for detection
	if cacheClearer, ok := p.graphProvider.(interface{ ClearCache() }); ok {
		cacheClearer.ClearCache()
		p.logger.Debug("Cleared graph provider cache before detection")
	}

	communities, err := p.communityDetector.DetectCommunities(ctx)
	if err != nil {
		p.logger.Error("Community detection failed", "error", err)
		// Reset enhancement window on detection error to allow retry
		p.enhancementDeadline = time.Time{}
		// Update window active metric (US3)
		if p.clusteringWindowActive != nil {
			p.clusteringWindowActive.Set(0)
		}
		// Record failure for health tracking
		p.recordDetectionFailure()
		return nil
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

	// Record success for health tracking
	p.recordDetectionSuccess()

	// Compute structural indices if enabled (needed for anomaly detection)
	if p.structuralComputer != nil && p.isStructuralIndexEnabled() {
		if err := p.computeStructuralIndices(ctx); err != nil {
			p.logger.Warn("Structural index computation failed", "error", err)
		}
	}

	// Run anomaly detection if enabled
	if p.inferenceOrchestrator != nil && p.config.GraphAnalysis != nil &&
		p.config.GraphAnalysis.AnomalyDetection != nil &&
		p.config.GraphAnalysis.AnomalyDetection.Enabled {
		p.runAnomalyDetection(ctx)
	}

	// Set enhancement window deadline if configured
	if enhancementWindow > 0 && totalCommunities > 0 {
		p.enhancementDeadline = time.Now().Add(enhancementWindow)
		// Update window active metric (US3)
		if p.clusteringWindowActive != nil {
			p.clusteringWindowActive.Set(1)
		}
		p.logger.Info("Enhancement window started",
			"deadline", p.enhancementDeadline,
			"duration", enhancementWindow,
			"mode", p.enhancementMode)
	}

	return communities
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
		MinCommunitySize:        p.config.GraphAnalysis.CommunityDetection.Inference.MinCommunitySize,
		MaxInferredPerCommunity: p.config.GraphAnalysis.CommunityDetection.Inference.MaxInferredPerCommunity,
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

// runReviewWorker runs the anomaly review worker
func (p *Processor) runReviewWorker(ctx context.Context) error {
	if p.reviewWorker == nil {
		return nil
	}

	p.logger.Info("Starting review worker")
	err := p.reviewWorker.Start(ctx)
	if err != nil && err != context.Canceled {
		p.logger.Error("Review worker error", "error", err)
	}

	// Wait for context cancellation
	<-ctx.Done()

	// Stop the worker gracefully
	if stopErr := p.reviewWorker.Stop(); stopErr != nil {
		p.logger.Warn("Review worker stop error", "error", stopErr)
	}

	p.logger.Info("Review worker stopped")
	return err
}

// runHierarchyInference runs the hierarchy inference worker that watches ENTITY_STATES
// and creates container membership edges for new entities.
func (p *Processor) runHierarchyInference(ctx context.Context) error {
	if p.hierarchyInference == nil {
		return nil
	}

	p.logger.Info("Starting hierarchy inference worker")
	err := p.hierarchyInference.Start(ctx)
	if err != nil && err != context.Canceled {
		p.logger.Error("Hierarchy inference worker error", "error", err)
		return err
	}

	// Wait for context cancellation
	<-ctx.Done()

	// Stop the worker gracefully
	if stopErr := p.hierarchyInference.Stop(); stopErr != nil {
		p.logger.Warn("Hierarchy inference worker stop error", "error", stopErr)
	}

	p.logger.Info("Hierarchy inference worker stopped")
	return nil
}

// startGateways starts the GraphQL and MCP gateway servers if configured.
// This must be called after QueryManager is ready.
func (p *Processor) startGateways(ctx context.Context) error {
	if p.config.Gateway == nil {
		return nil
	}

	// Start GraphQL gateway if enabled
	if p.config.Gateway.GraphQL != nil && p.config.Gateway.GraphQL.Enabled {
		if err := p.startGraphQLGateway(ctx); err != nil {
			return fmt.Errorf("graphql gateway: %w", err)
		}
	}

	// Start MCP gateway if enabled
	if p.config.Gateway.MCP != nil && p.config.Gateway.MCP.Enabled {
		if err := p.startMCPGateway(ctx); err != nil {
			return fmt.Errorf("mcp gateway: %w", err)
		}
	}

	return nil
}

// startGraphQLGateway creates and starts the GraphQL gateway server.
func (p *Processor) startGraphQLGateway(ctx context.Context) error {
	config := *p.config.Gateway.GraphQL
	if err := config.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	// Create metrics recorder for GraphQL operations
	metricsRecorder := graphqlgateway.NewMetricsRecorder(p.logger.With("component", "graphql-metrics"))

	// Create resolver with direct QueryManager access
	resolver := graphqlgateway.NewResolver(p.queryManager, metricsRecorder)

	// Create server
	server, err := graphqlgateway.NewServer(config, resolver, p.logger.With("gateway", "graphql"))
	if err != nil {
		return fmt.Errorf("create server: %w", err)
	}

	// Setup routes
	if err := server.Setup(); err != nil {
		return fmt.Errorf("setup server: %w", err)
	}

	p.graphqlServer = server

	// Start server in background goroutine
	ready := make(chan struct{})
	go func() {
		if err := server.Start(ctx, ready); err != nil {
			p.logger.Error("GraphQL gateway error", "error", err)
		}
	}()

	// Wait for server to be ready
	select {
	case <-ready:
		p.logger.Info("GraphQL gateway started",
			"address", config.BindAddress,
			"path", config.Path)
	case <-time.After(5 * time.Second):
		return fmt.Errorf("server failed to start within timeout")
	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}

// startMCPGateway creates and starts the MCP gateway server.
func (p *Processor) startMCPGateway(ctx context.Context) error {
	config := *p.config.Gateway.MCP
	if err := config.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	// Create metrics recorder and GraphQL resolver/executor for MCP to use
	metricsRecorder := graphqlgateway.NewMetricsRecorder(p.logger.With("component", "mcp-metrics"))
	resolver := graphqlgateway.NewResolver(p.queryManager, metricsRecorder)
	executor, err := graphqlgateway.NewExecutor(resolver, p.logger.With("component", "mcp-executor"))
	if err != nil {
		return fmt.Errorf("create executor: %w", err)
	}

	// Create MCP server (no metrics recorder for now)
	server, err := mcpgateway.NewServer(config, executor, nil, p.logger.With("gateway", "mcp"))
	if err != nil {
		return fmt.Errorf("create server: %w", err)
	}

	// Setup routes
	if err := server.Setup(); err != nil {
		return fmt.Errorf("setup server: %w", err)
	}

	p.mcpServer = server

	// Start server in background goroutine
	ready := make(chan struct{})
	go func() {
		if err := server.Start(ctx, ready); err != nil {
			p.logger.Error("MCP gateway error", "error", err)
		}
	}()

	// Wait for server to be ready
	select {
	case <-ready:
		p.logger.Info("MCP gateway started",
			"address", config.BindAddress,
			"path", config.Path)
	case <-time.After(5 * time.Second):
		return fmt.Errorf("server failed to start within timeout")
	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}

// stopGateways stops the GraphQL and MCP gateway servers.
func (p *Processor) stopGateways(timeout time.Duration) {
	// Stop MCP gateway first (it depends on GraphQL executor)
	if p.mcpServer != nil {
		if err := p.mcpServer.Stop(timeout); err != nil {
			p.logger.Warn("Error stopping MCP gateway", "error", err)
		}
		p.mcpServer = nil
		p.logger.Info("MCP gateway stopped")
	}

	// Stop GraphQL gateway
	if p.graphqlServer != nil {
		if err := p.graphqlServer.Stop(timeout); err != nil {
			p.logger.Warn("Error stopping GraphQL gateway", "error", err)
		}
		p.graphqlServer = nil
		p.logger.Info("GraphQL gateway stopped")
	}
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

	// Debug: Log ports config parsing
	if deps.Logger != nil {
		if config.Ports != nil {
			deps.Logger.Info("CreateGraphProcessor: Ports config parsed",
				"inputs_count", len(config.Ports.Inputs),
				"outputs_count", len(config.Ports.Outputs))
			for i, input := range config.Ports.Inputs {
				deps.Logger.Info("CreateGraphProcessor: Input port",
					"index", i,
					"name", input.Name,
					"subject", input.Subject,
					"type", input.Type)
			}
		} else {
			deps.Logger.Warn("CreateGraphProcessor: Ports config is NIL")
		}
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
	if p.config == nil || p.config.GraphAnalysis == nil ||
		p.config.GraphAnalysis.CommunityDetection == nil {
		return false
	}
	return p.config.GraphAnalysis.CommunityDetection.SemanticEdges.Enabled
}

// getMinEmbeddingCoverage returns the minimum embedding coverage threshold for semantic clustering.
// Returns 0.5 (50%) as default if not configured.
func (p *Processor) getMinEmbeddingCoverage() float64 {
	if p.config != nil && p.config.GraphAnalysis != nil &&
		p.config.GraphAnalysis.CommunityDetection != nil &&
		p.config.GraphAnalysis.CommunityDetection.Schedule.MinEmbeddingCoverage > 0 {
		return p.config.GraphAnalysis.CommunityDetection.Schedule.MinEmbeddingCoverage
	}
	return 0.5 // Default: 50% coverage
}

// DebugStatus returns extended debug information for the graph processor.
// Implements component.DebugStatusProvider.
func (p *Processor) DebugStatus() any {
	p.mu.RLock()
	defer p.mu.RUnlock()

	variant := p.getVariant()
	embeddingCoverage, coverageSource := p.getEmbeddingCoverage()

	return ClusteringStatus{
		Variant:             variant,
		LastRunTime:         time.Time{}, // TODO: Track this
		LastRunResult:       "",          // TODO: Track this
		EmbeddingCoverage:   embeddingCoverage,
		MinCoverage:         p.getMinEmbeddingCoverage(),
		CoverageSource:      coverageSource,
		EnhancementWindow:   p.getWindowStatus(),
		Communities:         0,                     // TODO: Track this
		ConsecutiveFailures: p.consecutiveFailures, // Now tracked
		BlockingReason:      p.getBlockingReason(),
		HealthStatus:        p.getHealthStatusString(),
	}
}

// getVariant determines the clustering variant (structural, statistical, semantic)
// based on configuration.
func (p *Processor) getVariant() string {
	if p.config == nil || p.config.GraphAnalysis == nil || p.config.GraphAnalysis.CommunityDetection == nil {
		return "structural" // Default when clustering not configured
	}

	if p.config.GraphAnalysis.CommunityDetection.SemanticEdges.Enabled {
		return "semantic"
	}

	// If EntityIDEdges is configured but semantic edges are not
	if p.config.GraphAnalysis.CommunityDetection.EntityIDEdges != nil &&
		p.config.GraphAnalysis.CommunityDetection.EntityIDEdges.Enabled {
		return "statistical"
	}

	return "structural"
}

// getEmbeddingCoverage returns current embedding coverage and source
func (p *Processor) getEmbeddingCoverage() (float64, string) {
	if !p.isSemanticClusteringEnabled() || p.indexManager == nil {
		return 0.0, ""
	}

	// Get embedding count from cache
	embeddingCount := p.indexManager.GetEmbeddingCount()

	// For simplicity, return 0 coverage if we can't determine entity count
	// This is just for observability; actual coverage checking uses different methods
	if embeddingCount == 0 {
		return 0.0, "cache"
	}

	// Return a placeholder value - we don't have direct access to entity count here
	// The actual coverage checking in checkEmbeddingCoverage uses getEmbeddableEntityCount
	return 0.0, "cache"
}

// getWindowStatus returns the enhancement window state
func (p *Processor) getWindowStatus() WindowStatus {
	now := time.Now()
	active := !p.enhancementDeadline.IsZero() && now.Before(p.enhancementDeadline)

	mode := "none"
	if p.config != nil && p.config.GraphAnalysis != nil &&
		p.config.GraphAnalysis.CommunityDetection != nil {
		mode = string(p.enhancementMode)
	}

	return WindowStatus{
		Active:        active,
		Mode:          mode,
		Deadline:      p.enhancementDeadline,
		EntityChanges: int(p.entityChangeCount.Load()),
		Threshold:     DefaultEntityChangeThreshold,
		AllTerminal:   false, // TODO: Track this if needed
	}
}

// getBlockingReason returns why detection is currently blocked (empty if not blocked)
func (p *Processor) getBlockingReason() string {
	// Check if config is nil first
	if p.config == nil || p.config.GraphAnalysis == nil || p.config.GraphAnalysis.CommunityDetection == nil {
		return ""
	}

	if !p.config.GraphAnalysis.CommunityDetection.Enabled {
		return ""
	}

	if p.isSemanticClusteringEnabled() {
		coverage, _ := p.getEmbeddingCoverage()
		minCoverage := p.getMinEmbeddingCoverage()
		if coverage < minCoverage {
			return fmt.Sprintf("embedding coverage %.2f below minimum %.2f", coverage, minCoverage)
		}
	}

	// Check if enhancement window is blocking
	now := time.Now()
	if !p.enhancementDeadline.IsZero() && now.Before(p.enhancementDeadline) {
		if p.enhancementMode == WindowModeBlocking {
			return "enhancement window active (blocking mode)"
		}
	}

	return ""
}

// getHealthStatusString returns the health status as a string
func (p *Processor) getHealthStatusString() string {
	// Note: mu is already held by DebugStatus() caller
	if p.currentHealth == "" {
		return "healthy" // Default to healthy if not initialized
	}
	return p.currentHealth
}

// recordDetectionFailure increments failure count, resets success count, and updates health status.
// Updates health based on consecutive failure count following thresholds:
// - 3 consecutive failures: degraded
// - 8 consecutive failures: unhealthy
func (p *Processor) recordDetectionFailure() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.consecutiveFailures++
	p.consecutiveSuccesses = 0

	// Update health based on failure count
	if p.consecutiveFailures >= unhealthyThreshold {
		p.currentHealth = "unhealthy"
	} else if p.consecutiveFailures >= healthyThreshold {
		p.currentHealth = "degraded"
	}

	// Set backoff
	backoff := p.calculateBackoffDuration()
	if backoff > 0 {
		p.lastBackoffUntil = time.Now().Add(backoff)
	}

	// Update clustering failures metric (US3)
	if p.clusteringFailuresTotal != nil {
		p.clusteringFailuresTotal.Inc()
	}

	// Log health transition
	p.logger.Warn("Detection failure recorded",
		"consecutive_failures", p.consecutiveFailures,
		"health_status", p.currentHealth,
		"backoff_until", p.lastBackoffUntil)
}

// recordDetectionSuccess increments success count, resets failure count, and updates health status.
// Recovery requires consecutive successes:
// - unhealthy → degraded: after 1 success
// - degraded → healthy: after 2 consecutive successes
func (p *Processor) recordDetectionSuccess() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.consecutiveSuccesses++
	previousHealth := p.currentHealth

	// Clear backoff on success
	p.lastBackoffUntil = time.Time{}
	p.consecutiveFailures = 0

	// Update health based on success count
	if p.consecutiveSuccesses >= recoveryThreshold {
		p.currentHealth = "healthy"
	} else if p.consecutiveSuccesses == 1 && p.currentHealth == "unhealthy" {
		p.currentHealth = "degraded"
	}

	// Log health transition if changed
	if previousHealth != p.currentHealth {
		p.logger.Info("Health status recovered",
			"previous", previousHealth,
			"current", p.currentHealth,
			"consecutive_successes", p.consecutiveSuccesses)
	}
}

// getHealthStatus returns current health status.
// Note: Caller must hold mu if synchronization is needed.
func (p *Processor) getHealthStatus() string {
	return p.currentHealth
}

// calculateBackoffDuration computes exponential backoff based on consecutive failures.
// Formula: min(baseDelay * 2^(failures-1), maxDelay)
// Returns 0 if no failures.
func (p *Processor) calculateBackoffDuration() time.Duration {
	if p.consecutiveFailures <= 0 {
		return 0
	}

	// Cap failures before bit shift to prevent int64 overflow.
	// At 10 failures (2^9 * 1s = 512s), we exceed maxBackoffDelay (5min = 300s),
	// so anything above 10 returns maxBackoffDelay directly.
	failures := p.consecutiveFailures
	if failures > 10 {
		return maxBackoffDelay
	}

	// Calculate 2^(failures-1) * baseDelay, capped at maxDelay
	multiplier := int64(1) << (failures - 1)
	backoff := time.Duration(multiplier) * baseBackoffDelay
	if backoff > maxBackoffDelay {
		backoff = maxBackoffDelay
	}
	return backoff
}

// getBackoffDuration returns the current backoff duration based on failure count.
// This is a public method for testing, delegates to calculateBackoffDuration.
func (p *Processor) getBackoffDuration() time.Duration {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.calculateBackoffDuration()
}

// shouldSkipDetection returns true if currently in backoff period.
func (p *Processor) shouldSkipDetection() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.lastBackoffUntil.IsZero() {
		return false
	}
	return time.Now().Before(p.lastBackoffUntil)
}

// getConsecutiveFailures returns current consecutive failure count.
func (p *Processor) getConsecutiveFailures() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.consecutiveFailures
}
