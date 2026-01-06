// Package querymanager provides the QueryManager for high-performance read operations.
package querymanager

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	gtypes "github.com/c360/semstreams/graph"
	"github.com/c360/semstreams/graph/datamanager"
	"github.com/c360/semstreams/graph/indexmanager"
	"github.com/c360/semstreams/graph/llm"
	"github.com/c360/semstreams/metric"
	"github.com/c360/semstreams/pkg/errs"
)

// Ensure Manager implements the Querier interface
var _ Querier = (*Manager)(nil)

// Manager implements the Querier interface as a stateless orchestrator
// that coordinates between EntityStore and IndexManager
type Manager struct {
	// Configuration
	config Config

	// Dependencies - pure orchestration, no caching
	entityReader      datamanager.EntityReader // Read-only entity access with caching
	indexManager      indexmanager.Indexer     // Will handle query result caching
	communityDetector any                      // Optional: CommunityDetector for GraphRAG search (type-erased to avoid import cycle)

	// LLM dependencies for answer generation
	llmClient      llm.Client         // Optional: LLM client for answer generation
	contentFetcher llm.ContentFetcher // Optional: for fetching entity content

	// Metrics (simplified - no cache metrics)
	metrics *Metrics

	// Metrics tracking
	mu           sync.RWMutex
	lastActivity time.Time
	errorCount   int64
	lastError    string

	// Logger
	logger *slog.Logger
}

// Deps holds runtime dependencies for query manager component
type Deps struct {
	Config            Config                   // Business logic configuration
	EntityReader      datamanager.EntityReader // Runtime dependency for read-only entity access
	IndexManager      indexmanager.Indexer     // Runtime dependency
	CommunityDetector any                      // Optional: CommunityDetector for GraphRAG search (type-erased to avoid import cycle)
	LLMClient         llm.Client               // Optional: LLM client for answer generation
	ContentFetcher    llm.ContentFetcher       // Optional: for fetching entity content
	Registry          *metric.MetricsRegistry  // Runtime dependency
	Logger            *slog.Logger             // Runtime dependency
}

// NewManager creates a new Querier instance using idiomatic Go constructor pattern
func NewManager(deps Deps) (Querier, error) {
	// Set defaults first
	deps.Config.SetDefaults()

	// Then validate configuration
	if err := deps.Config.Validate(); err != nil {
		return nil, errs.WrapInvalid(err, "QueryManager", "NewManager",
			"configuration validation failed")
	}

	// Ensure we have a logger
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}

	m := &Manager{
		config:            deps.Config,
		entityReader:      deps.EntityReader,
		indexManager:      deps.IndexManager,
		communityDetector: deps.CommunityDetector, // Optional dependency
		llmClient:         deps.LLMClient,         // Optional LLM client
		contentFetcher:    deps.ContentFetcher,    // Optional for content fetching
		lastActivity:      time.Now(),
		logger:            logger,
	}

	// Initialize metrics if registry provided
	if deps.Registry != nil {
		m.metrics = NewMetrics(deps.Registry, "query_engine")
	}

	return m, nil
}

// GetEntity retrieves a single entity through EntityStore (benefits from its cache)
func (m *Manager) GetEntity(ctx context.Context, id string) (*gtypes.EntityState, error) {
	start := time.Now()
	defer m.recordActivity()

	// Apply timeout
	if m.config.Timeouts.EntityGet > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, m.config.Timeouts.EntityGet)
		defer cancel()
	}

	// Fetch from EntityReader (which has its own L1/L2 cache)
	entity, err := m.entityReader.GetEntity(ctx, id)
	if err != nil {
		m.recordError("GetEntity", err)

		// Record metrics for failure
		if m.metrics != nil {
			m.metrics.RecordEntityGet("query_engine", time.Since(start), "entityreader_error", false)
		}

		// Wrap error appropriately
		if IsEntityNotFound(err) {
			return nil, errs.WrapInvalid(gtypes.ErrEntityNotFound, "QueryManager", "GetEntity",
				fmt.Sprintf("entity ID: %s", id))
		}
		return nil, errs.WrapTransient(err, "QueryManager", "GetEntity",
			fmt.Sprintf("KV operation failed for ID: %s", id))
	}

	// Record metrics for success
	if m.metrics != nil {
		m.metrics.RecordEntityGet("query_engine", time.Since(start), "entityreader", true)
	}

	return entity, nil
}

// GetEntities retrieves multiple entities efficiently through EntityStore
func (m *Manager) GetEntities(ctx context.Context, ids []string) ([]*gtypes.EntityState, error) {
	start := time.Now()
	defer m.recordActivity()

	// Apply timeout
	if m.config.Timeouts.EntityBatch > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, m.config.Timeouts.EntityBatch)
		defer cancel()
	}

	// Fetch all entities from EntityReader (which has its own cache)
	results, err := m.entityReader.BatchGet(ctx, ids)
	if err != nil {
		m.recordError("GetEntities", err)
		return nil, err
	}

	// Record metrics
	if m.metrics != nil {
		sizeRange := m.getSizeRange(len(ids))
		m.metrics.entityBatchTotal.WithLabelValues("query_engine", "success").Inc()
		m.metrics.entityBatchDuration.WithLabelValues("query_engine", sizeRange).Observe(time.Since(start).Seconds())
	}

	return results, nil
}

// GetEntityByAlias retrieves an entity by alias or ID (extracted from graph processor)
func (m *Manager) GetEntityByAlias(ctx context.Context, aliasOrID string) (*gtypes.EntityState, error) {
	start := time.Now()
	defer m.recordActivity()

	// First, try to resolve the alias using index manager
	var entityID string
	if m.indexManager != nil {
		if resolvedID, err := m.indexManager.ResolveAlias(ctx, aliasOrID); err == nil {
			entityID = resolvedID
		} else {
			// If alias resolution fails, assume it's already an entity ID
			entityID = aliasOrID
		}
	} else {
		// No index manager available, assume it's an entity ID
		entityID = aliasOrID
	}

	// Get entity using the resolved ID
	entity, err := m.GetEntity(ctx, entityID)
	if err != nil {
		// If entity not found and we tried alias resolution, return alias not found error
		if IsEntityNotFound(err) && entityID != aliasOrID {
			return nil, errs.WrapInvalid(gtypes.ErrAliasNotFound, "QueryManager", "GetEntityByAlias",
				fmt.Sprintf("alias: %s", aliasOrID))
		}
		return nil, err
	}

	// Record metrics for alias resolution
	if m.metrics != nil {
		m.metrics.RecordQuery("query_engine", "alias_resolution", time.Since(start), 1, true)
	}

	return entity, nil
}

// Query operations that delegate to index manager

// QueryByPredicate queries entities by predicate (delegated to index manager)
func (m *Manager) QueryByPredicate(ctx context.Context, predicate string) ([]string, error) {
	start := time.Now()
	defer m.recordActivity()

	if m.indexManager == nil {
		return nil, errs.WrapTransient(ErrIndexManagerUnavailable, "QueryManager", "QueryByPredicate",
			"index manager dependency unavailable")
	}

	// Direct delegation to index manager (will handle its own caching in Step 2)
	entityIDs, err := m.indexManager.GetPredicateIndex(ctx, predicate)
	if err != nil {
		m.recordError("QueryByPredicate", err)
		if m.metrics != nil {
			m.metrics.RecordQuery("query_engine", "predicate", time.Since(start), 0, false)
		}
		return nil, errs.WrapTransient(err, "QueryManager", "QueryByPredicate",
			fmt.Sprintf("index manager operation failed for predicate: %s", predicate))
	}

	// Record metrics
	if m.metrics != nil {
		m.metrics.RecordQuery("query_engine", "predicate", time.Since(start), len(entityIDs), false)
	}

	return entityIDs, nil
}

// QuerySpatial queries entities by spatial bounds (delegated to index manager)
func (m *Manager) QuerySpatial(ctx context.Context, bounds SpatialBounds) ([]string, error) {
	start := time.Now()
	defer m.recordActivity()

	if m.indexManager == nil {
		return nil, errs.WrapTransient(ErrIndexManagerUnavailable, "QueryManager", "QuerySpatial",
			"index manager dependency unavailable")
	}

	// Convert bounds
	indexBounds := indexmanager.Bounds{
		North: bounds.North,
		South: bounds.South,
		East:  bounds.East,
		West:  bounds.West,
	}

	// Delegate to index manager
	entityIDs, err := m.indexManager.QuerySpatial(ctx, indexBounds)
	if err != nil {
		m.recordError("QuerySpatial", err)
		if m.metrics != nil {
			m.metrics.RecordQuery("query_engine", "spatial", time.Since(start), 0, false)
		}
		return nil, errs.WrapTransient(err, "QueryManager", "QuerySpatial",
			"index manager spatial query failed")
	}

	// Record metrics
	if m.metrics != nil {
		m.metrics.RecordQuery("query_engine", "spatial", time.Since(start), len(entityIDs), true)
	}

	return entityIDs, nil
}

// QueryTemporal queries entities by temporal bounds (delegated to index manager)
func (m *Manager) QueryTemporal(ctx context.Context, start, end time.Time) ([]string, error) {
	startTime := time.Now()
	defer m.recordActivity()

	if m.indexManager == nil {
		return nil, errs.WrapTransient(ErrIndexManagerUnavailable, "QueryManager", "QueryTemporal",
			"index manager dependency unavailable")
	}

	// Delegate to index manager
	entityIDs, err := m.indexManager.QueryTemporal(ctx, start, end)
	if err != nil {
		m.recordError("QueryTemporal", err)
		if m.metrics != nil {
			m.metrics.RecordQuery("query_engine", "temporal", time.Since(startTime), 0, false)
		}
		return nil, errs.WrapTransient(err, "QueryManager", "QueryTemporal",
			fmt.Sprintf("index manager temporal query failed: %v to %v", start, end))
	}

	// Record metrics
	if m.metrics != nil {
		m.metrics.RecordQuery("query_engine", "temporal", time.Since(startTime), len(entityIDs), true)
	}

	return entityIDs, nil
}

// Complex query operations implemented in query.go

// Cache management operations

// InvalidateEntity - no-op since query manager is now stateless (index manager will handle its own cache invalidation)
func (m *Manager) InvalidateEntity(_ string) error {
	// No cache to invalidate at query manager level
	return nil
}

// WarmCache pre-loads entities into EntityReader's cache
func (m *Manager) WarmCache(ctx context.Context, entityIDs []string) error {
	// Simply fetch entities, which will warm EntityReader's cache
	_, err := m.GetEntities(ctx, entityIDs)
	if err != nil {
		return errs.WrapTransient(err, "QueryManager", "WarmCache",
			"batch cache warming failed")
	}

	// Entity caching is now handled by EntityReader
	// Query result caching happens automatically when queries are executed
	return nil
}

// Health and metrics

// GetCacheStats returns empty stats since query manager is now stateless
func (m *Manager) GetCacheStats() CacheStats {
	// No cache at query manager level - EntityReader and index manager manage their own caches
	return CacheStats{}
}

// EntityID Hierarchy Navigation

// ListWithPrefix returns entity IDs matching the given prefix.
// Used for EntityID hierarchy navigation (e.g., "c360.logistics" returns all logistics entities).
func (m *Manager) ListWithPrefix(ctx context.Context, prefix string) ([]string, error) {
	defer m.recordActivity()
	return m.entityReader.ListWithPrefix(ctx, prefix)
}

// GetHierarchyStats returns entity counts grouped by next hierarchy level.
// Used by MCP tools to understand graph structure at each EntityID level.
func (m *Manager) GetHierarchyStats(ctx context.Context, prefix string) (*HierarchyStats, error) {
	defer m.recordActivity()

	ids, err := m.entityReader.ListWithPrefix(ctx, prefix)
	if err != nil {
		m.recordError("GetHierarchyStats", err)
		return nil, err
	}

	// Group by next hierarchy level
	childCounts := make(map[string]int)
	for _, id := range ids {
		nextLevel := extractNextLevel(id, prefix)
		if nextLevel != "" {
			childCounts[nextLevel]++
		}
	}

	// Build result with sorted children for deterministic output
	stats := &HierarchyStats{
		Prefix:        prefix,
		TotalEntities: len(ids),
		Children:      make([]HierarchyLevel, 0, len(childCounts)),
	}

	for childPrefix, count := range childCounts {
		stats.Children = append(stats.Children, HierarchyLevel{
			Prefix: childPrefix,
			Name:   extractLastSegment(childPrefix),
			Count:  count,
		})
	}

	// Sort children alphabetically by prefix for deterministic output
	sortHierarchyLevels(stats.Children)

	return stats, nil
}

// extractNextLevel extracts the next hierarchy level from an entity ID given a prefix.
// For example: extractNextLevel("c360.logistics.sensor.temp.001", "c360") returns "c360.logistics"
func extractNextLevel(entityID, prefix string) string {
	// Handle empty prefix (root level)
	if prefix == "" {
		parts := splitEntityID(entityID)
		if len(parts) > 0 {
			return parts[0]
		}
		return ""
	}

	// Entity ID must start with prefix
	if !hasPrefix(entityID, prefix) {
		return ""
	}

	// Get the part after the prefix
	remainder := entityID[len(prefix):]
	if len(remainder) == 0 {
		return "" // Entity ID equals prefix exactly
	}

	// Skip the dot separator
	if remainder[0] == '.' {
		remainder = remainder[1:]
	}

	// Find the next segment
	parts := splitEntityID(remainder)
	if len(parts) > 0 {
		return prefix + "." + parts[0]
	}

	return ""
}

// extractLastSegment returns the last segment of a dotted prefix
func extractLastSegment(prefix string) string {
	parts := splitEntityID(prefix)
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return prefix
}

// splitEntityID splits an entity ID by dots
func splitEntityID(id string) []string {
	if id == "" {
		return nil
	}
	var parts []string
	start := 0
	for i := 0; i <= len(id); i++ {
		if i == len(id) || id[i] == '.' {
			if i > start {
				parts = append(parts, id[start:i])
			}
			start = i + 1
		}
	}
	return parts
}

// hasPrefix checks if entityID starts with prefix (with proper dot boundary)
func hasPrefix(entityID, prefix string) bool {
	if len(entityID) < len(prefix) {
		return false
	}
	if entityID[:len(prefix)] != prefix {
		return false
	}
	// Must be exact match or followed by a dot
	if len(entityID) == len(prefix) {
		return true
	}
	return entityID[len(prefix)] == '.'
}

// sortHierarchyLevels sorts hierarchy levels alphabetically by prefix
func sortHierarchyLevels(levels []HierarchyLevel) {
	for i := 0; i < len(levels)-1; i++ {
		for j := i + 1; j < len(levels); j++ {
			if levels[i].Prefix > levels[j].Prefix {
				levels[i], levels[j] = levels[j], levels[i]
			}
		}
	}
}

// SearchSimilar performs similarity search using embeddings (BM25 or neural).
// Returns entities ranked by cosine similarity score to the query text.
// Works on both statistical (BM25) and semantic (neural) tiers.
func (m *Manager) SearchSimilar(ctx context.Context, query string, limit int) (*SimilaritySearchResult, error) {
	start := time.Now()
	defer m.recordActivity()

	if m.indexManager == nil {
		return nil, errs.WrapTransient(ErrIndexManagerUnavailable, "QueryManager", "SearchSimilar",
			"index manager dependency unavailable")
	}

	// Set default limit
	if limit <= 0 {
		limit = 10
	}

	// Configure similarity search options
	opts := &indexmanager.SemanticSearchOptions{
		Threshold: 0.1, // Low threshold to get more results, ranked by score
		Limit:     limit,
	}

	// Delegate to index manager's semantic search (internal name unchanged)
	results, err := m.indexManager.SearchSemantic(ctx, query, opts)
	if err != nil {
		m.recordError("SearchSimilar", err)
		if m.metrics != nil {
			m.metrics.RecordQuery("query_engine", "similarity", time.Since(start), 0, false)
		}
		return nil, errs.WrapTransient(err, "QueryManager", "SearchSimilar",
			"index manager similarity search failed")
	}

	// Convert to querymanager types
	hits := make([]SimilarityHit, len(results.Hits))
	for i, hit := range results.Hits {
		hits[i] = SimilarityHit{
			EntityID: hit.EntityID,
			Score:    hit.Score,
		}
	}

	// Record metrics
	if m.metrics != nil {
		m.metrics.RecordQuery("query_engine", "similarity", time.Since(start), len(hits), true)
	}

	return &SimilaritySearchResult{
		Hits:  hits,
		Total: results.Total,
	}, nil
}

// Helper methods

// getSizeRange returns a size range label for metrics
func (m *Manager) getSizeRange(size int) string {
	if size <= 10 {
		return "small"
	}
	if size <= 100 {
		return "medium"
	}
	return "large"
}

// recordError records an error for health monitoring
func (m *Manager) recordError(operation string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.errorCount++
	m.lastError = fmt.Sprintf("%s: %v", operation, err)
	m.logger.Error("Query manager error", "operation", operation, "error", err)
}

// recordActivity updates last activity time with proper synchronization
func (m *Manager) recordActivity() {
	m.mu.Lock()
	m.lastActivity = time.Now()
	m.mu.Unlock()
}

// Background monitoring methods removed - query manager is now stateless
// Health is determined on-demand via IsReady() and HealthStatus()
