// Package inference provides structural anomaly detection and inference
// for identifying potential missing relationships in the knowledge graph.
package inference

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"

	gtypes "github.com/c360/semstreams/graph"
	"github.com/c360/semstreams/message"
	"github.com/c360/semstreams/vocabulary"
	"github.com/nats-io/nats.go/jetstream"
)

// HierarchyInference creates membership edges to container entities based on
// the 6-part entity ID structure. When an entity is created, it automatically
// creates edges to type, system, and domain container entities.
//
// Entity ID format: org.platform.domain.system.type.instance
// Example: acme.iot.sensors.hvac.temperature.001
//
// Container entities are auto-created with the following pattern:
//   - Type container:   org.platform.domain.system.type.group (6-part)
//   - System container: org.platform.domain.system.group.container (6-part)
//   - Domain container: org.platform.domain.group.container.level (6-part)
//
// Graph distances via containers:
//   - Same type siblings: 2 hops (entity → type.group ← entity)
//   - Same system, different type: 4 hops
//   - Same domain, different system: 6 hops
type HierarchyInference struct {
	// entityManager handles entity existence checks and creation
	entityManager EntityManager

	// tripleAdder adds triples to entities
	tripleAdder TripleAdder

	// Configuration
	config HierarchyConfig

	// Logger for observability
	logger *slog.Logger

	// Cache of known container entities to avoid repeated existence checks
	containerCache   map[string]bool
	containerCacheMu sync.RWMutex

	// Metrics
	containersCreated atomic.Int64
	edgesCreated      atomic.Int64
	edgesFailed       atomic.Int64

	// KV Watcher lifecycle (follows ReviewWorker pattern)
	entityBucket jetstream.KeyValue // ENTITY_STATES bucket
	watcher      jetstream.KeyWatcher
	workerCount  int
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	mu           sync.RWMutex
	started      bool
}

// EntityManager provides entity existence checks and creation.
// Typically implemented by datamanager.Manager.
type EntityManager interface {
	ExistsEntity(ctx context.Context, id string) (bool, error)
	CreateEntity(ctx context.Context, entity *gtypes.EntityState) (*gtypes.EntityState, error)
}

// HierarchyConfig configures hierarchy container inference.
type HierarchyConfig struct {
	// Enabled activates hierarchy inference on entity creation
	Enabled bool `json:"enabled"`

	// CreateTypeEdges enables type membership edges (5-part prefix → type container)
	// StandardIRI: skos:broader
	CreateTypeEdges bool `json:"create_type_edges"`

	// CreateSystemEdges enables system membership edges (4-part prefix → system container)
	// StandardIRI: skos:broader
	CreateSystemEdges bool `json:"create_system_edges"`

	// CreateDomainEdges enables domain membership edges (3-part prefix → domain container)
	// StandardIRI: skos:broader
	CreateDomainEdges bool `json:"create_domain_edges"`
}

// DefaultHierarchyConfig returns sensible defaults for hierarchy inference.
func DefaultHierarchyConfig() HierarchyConfig {
	return HierarchyConfig{
		Enabled:           false, // Opt-in feature
		CreateTypeEdges:   true,
		CreateSystemEdges: true,
		CreateDomainEdges: true,
	}
}

// NewHierarchyInference creates a new hierarchy inference component.
//
// Parameters:
//   - entityManager: Component for entity existence checks and creation
//   - tripleAdder: Component that can add triples (typically DataManager)
//   - entityBucket: ENTITY_STATES KV bucket to watch for new entities
//   - config: Configuration for edge creation
//   - logger: Logger for observability (can be nil)
func NewHierarchyInference(
	entityManager EntityManager,
	tripleAdder TripleAdder,
	entityBucket jetstream.KeyValue,
	config HierarchyConfig,
	logger *slog.Logger,
) *HierarchyInference {
	if logger == nil {
		logger = slog.Default()
	}

	return &HierarchyInference{
		entityManager:  entityManager,
		tripleAdder:    tripleAdder,
		entityBucket:   entityBucket,
		config:         config,
		logger:         logger,
		containerCache: make(map[string]bool),
		workerCount:    2, // Default: 2 workers for hierarchy inference
	}
}

// OnEntityCreated is called when a new entity is added to the graph.
// It creates membership edges to container entities based on the entity's 6-part ID.
//
// For each enabled level (type, system, domain), it:
// 1. Computes the container entity ID
// 2. Auto-creates the container if it doesn't exist
// 3. Creates a membership edge from entity to container
//
// This is O(1) per entity - only the new entity is modified, plus up to 3
// container creations (which are idempotent and cached).
func (h *HierarchyInference) OnEntityCreated(ctx context.Context, entityID string) error {
	if !h.config.Enabled {
		return nil
	}

	// Parse entity ID to validate 6-part structure
	parts := strings.Split(entityID, ".")
	if len(parts) != 6 {
		// Not a valid 6-part EntityID, skip silently
		return nil
	}

	var errs []error

	// Create type membership: entity → type.group
	if h.config.CreateTypeEdges {
		typeContainerID := h.buildTypeContainerID(parts)
		if err := h.ensureContainerAndEdge(ctx, entityID, typeContainerID, vocabulary.HierarchyTypeMember); err != nil {
			errs = append(errs, err)
		}
	}

	// Create system membership: entity → system.group.container
	if h.config.CreateSystemEdges {
		systemContainerID := h.buildSystemContainerID(parts)
		if err := h.ensureContainerAndEdge(ctx, entityID, systemContainerID, vocabulary.HierarchySystemMember); err != nil {
			errs = append(errs, err)
		}
	}

	// Create domain membership: entity → domain.group.container.level
	if h.config.CreateDomainEdges {
		domainContainerID := h.buildDomainContainerID(parts)
		if err := h.ensureContainerAndEdge(ctx, entityID, domainContainerID, vocabulary.HierarchyDomainMember); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// buildTypeContainerID creates a 6-part type container ID.
// Input: [org, platform, domain, system, type, instance]
// Output: org.platform.domain.system.type.group
func (h *HierarchyInference) buildTypeContainerID(parts []string) string {
	return strings.Join(parts[:5], ".") + ".group"
}

// buildSystemContainerID creates a 6-part system container ID.
// Input: [org, platform, domain, system, type, instance]
// Output: org.platform.domain.system.group.container
func (h *HierarchyInference) buildSystemContainerID(parts []string) string {
	return strings.Join(parts[:4], ".") + ".group.container"
}

// buildDomainContainerID creates a 6-part domain container ID.
// Input: [org, platform, domain, system, type, instance]
// Output: org.platform.domain.group.container.level
func (h *HierarchyInference) buildDomainContainerID(parts []string) string {
	return strings.Join(parts[:3], ".") + ".group.container.level"
}

// ensureContainerAndEdge creates the container entity if needed, then creates the membership edge
// and its inverse (container → contains → entity) for bidirectional graph traversal.
func (h *HierarchyInference) ensureContainerAndEdge(ctx context.Context, entityID, containerID, predicate string) error {
	// Ensure container exists (with caching)
	if err := h.ensureContainerExists(ctx, containerID); err != nil {
		h.edgesFailed.Add(1)
		return err
	}

	// Create forward membership edge: entity → predicate → container
	forwardTriple := message.Triple{
		Subject:    entityID,
		Predicate:  predicate,
		Object:     containerID, // Real 6-part entity ID - IsRelationship() returns true
		Context:    "inference.hierarchy",
		Confidence: 1.0, // Structural inference has perfect confidence
	}

	if err := h.tripleAdder.AddTriple(ctx, forwardTriple); err != nil {
		h.edgesFailed.Add(1)
		return err
	}

	h.edgesCreated.Add(1)

	// Create inverse edge: container → contains → entity
	// This enables direct traversal from container to its members without using IncomingIndex
	inversePredicate := vocabulary.GetInversePredicate(predicate)
	if inversePredicate != "" {
		inverseTriple := message.Triple{
			Subject:    containerID,
			Predicate:  inversePredicate, // e.g., hierarchy.type.contains
			Object:     entityID,
			Context:    "inference.hierarchy",
			Confidence: 1.0,
		}

		if err := h.tripleAdder.AddTriple(ctx, inverseTriple); err != nil {
			// Log warning but don't fail - forward edge already created
			h.logger.Warn("Failed to create inverse edge",
				"container_id", containerID,
				"entity_id", entityID,
				"inverse_predicate", inversePredicate,
				"error", err)
		} else {
			h.edgesCreated.Add(1)
		}
	}

	return nil
}

// ensureContainerExists creates a minimal container entity if it doesn't exist.
// Uses an in-memory cache to avoid repeated existence checks.
func (h *HierarchyInference) ensureContainerExists(ctx context.Context, containerID string) error {
	// Check cache first
	h.containerCacheMu.RLock()
	exists := h.containerCache[containerID]
	h.containerCacheMu.RUnlock()

	if exists {
		return nil
	}

	// Check if container exists in storage
	existsInStorage, err := h.entityManager.ExistsEntity(ctx, containerID)
	if err != nil {
		return err
	}

	if existsInStorage {
		// Update cache and return
		h.containerCacheMu.Lock()
		h.containerCache[containerID] = true
		h.containerCacheMu.Unlock()
		return nil
	}

	// Create minimal container entity
	containerEntity := &gtypes.EntityState{
		ID: containerID,
		Triples: []message.Triple{
			{
				Subject:    containerID,
				Predicate:  "entity.type.class",
				Object:     "hierarchy.container",
				Context:    "inference.hierarchy",
				Confidence: 1.0,
			},
		},
	}

	_, err = h.entityManager.CreateEntity(ctx, containerEntity)
	if err != nil {
		// Check if it's a "already exists" error (race condition with another goroutine)
		// In that case, it's not a failure - the container exists
		if strings.Contains(err.Error(), "exists") {
			h.containerCacheMu.Lock()
			h.containerCache[containerID] = true
			h.containerCacheMu.Unlock()
			return nil
		}
		return err
	}

	// Update cache and metrics
	h.containerCacheMu.Lock()
	h.containerCache[containerID] = true
	h.containerCacheMu.Unlock()

	h.containersCreated.Add(1)

	h.logger.Debug("Created hierarchy container entity",
		"container_id", containerID)

	return nil
}

// ClearCache resets the container cache. Call when containers might be deleted.
func (h *HierarchyInference) ClearCache() {
	h.containerCacheMu.Lock()
	defer h.containerCacheMu.Unlock()

	h.containerCache = make(map[string]bool)
}

// GetMetrics returns metrics for hierarchy inference operations.
func (h *HierarchyInference) GetMetrics() (containersCreated, edgesCreated, edgesFailed int64) {
	return h.containersCreated.Load(), h.edgesCreated.Load(), h.edgesFailed.Load()
}

// GetCacheStats returns statistics about the container cache.
func (h *HierarchyInference) GetCacheStats() int {
	h.containerCacheMu.RLock()
	defer h.containerCacheMu.RUnlock()

	return len(h.containerCache)
}

// Start begins watching ENTITY_STATES for new entities and creating hierarchy edges.
// Follows the ReviewWorker pattern for independent KV watching.
func (h *HierarchyInference) Start(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.started {
		return nil // Idempotent
	}

	if !h.config.Enabled {
		h.logger.Debug("Hierarchy inference disabled, not starting watcher")
		return nil
	}

	if h.entityBucket == nil {
		h.logger.Warn("No entity bucket provided, hierarchy inference cannot start")
		return nil
	}

	h.ctx, h.cancel = context.WithCancel(ctx)

	// Create KV watcher on ENTITY_STATES
	watcher, err := h.entityBucket.WatchAll(h.ctx)
	if err != nil {
		h.cancel()
		return errors.Join(errors.New("failed to create entity watcher"), err)
	}
	h.watcher = watcher

	// Start worker goroutines
	for i := 0; i < h.workerCount; i++ {
		h.wg.Add(1)
		go h.processEntities(i)
	}

	h.started = true
	h.logger.Info("Hierarchy inference started with KV watcher",
		"workers", h.workerCount,
		"create_type_edges", h.config.CreateTypeEdges,
		"create_system_edges", h.config.CreateSystemEdges,
		"create_domain_edges", h.config.CreateDomainEdges)

	return nil
}

// Stop gracefully shuts down the hierarchy inference worker.
func (h *HierarchyInference) Stop() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.started {
		return nil // Idempotent
	}

	if h.cancel != nil {
		h.cancel()
	}

	if h.watcher != nil {
		_ = h.watcher.Stop()
	}

	h.wg.Wait()
	h.started = false

	containersCreated, edgesCreated, edgesFailed := h.GetMetrics()
	h.logger.Info("Hierarchy inference stopped",
		"containers_created", containersCreated,
		"edges_created", edgesCreated,
		"edges_failed", edgesFailed)

	return nil
}

// processEntities is the worker loop that watches for new entities.
// Follows the ReviewWorker pattern for independent KV watching.
//
// On startup, WatchAll() delivers all existing entries first (backfill),
// then continues with live updates. The container cache ensures idempotency -
// entities that already have hierarchy edges will be skipped efficiently.
func (h *HierarchyInference) processEntities(workerID int) {
	defer h.wg.Done()

	h.logger.Debug("Hierarchy inference worker started", "worker_id", workerID)

	for {
		select {
		case <-h.ctx.Done():
			h.logger.Debug("Hierarchy inference worker stopping", "worker_id", workerID)
			return

		case entry, ok := <-h.watcher.Updates():
			if !ok {
				h.logger.Debug("Hierarchy inference watcher closed", "worker_id", workerID)
				return
			}

			// Skip nil entries (initial watcher marker)
			if entry == nil {
				continue
			}

			// Skip deleted entries
			if entry.Operation() == jetstream.KeyValueDelete {
				continue
			}

			entityID := entry.Key()

			// Process entity - the container cache ensures idempotency.
			// Existing entities will short-circuit if containers already exist.
			// This handles both:
			// 1. Initial backfill (existing entities when watcher starts)
			// 2. Live creates (new entities added after watcher starts)
			if err := h.OnEntityCreated(h.ctx, entityID); err != nil {
				h.logger.Debug("Hierarchy inference skipped entity",
					"worker_id", workerID,
					"entity_id", entityID,
					"error", err)
			}
		}
	}
}
