package indexmanager

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360/semstreams/pkg/cache"
	"github.com/c360/semstreams/pkg/errs"
	"github.com/c360/semstreams/processor/graph/embedding"
	"github.com/c360/semstreams/storage/objectstore"
)

// scoredHit represents a search result with its similarity score
type scoredHit struct {
	entityID string
	score    float64
}

// initializeSemanticSearch initializes semantic search components if enabled.
//
// This is called during NewManager initialization. If embedding is disabled,
// this is a no-op and semantic search methods will return errors.
//
// Provider selection with automatic fallback:
//   - "http": Try HTTP service, fall back to BM25 if unavailable
//   - "bm25": Pure Go BM25 embedder (default)
//   - "disabled" or Enabled=false: Disable semantic search
//   - Empty/unspecified: Defaults to BM25
func (m *Manager) initializeSemanticSearch(buckets map[string]jetstream.KeyValue) error {
	// Check if semantic search is explicitly disabled
	if !m.config.Embedding.Enabled || m.config.Embedding.Provider == "disabled" {
		m.logger.Info("Semantic search disabled", "provider", m.config.Embedding.Provider)
		// Caches remain nil when disabled
		return nil
	}

	// Default to BM25 if no provider specified
	provider := m.config.Embedding.Provider
	if provider == "" {
		provider = "bm25"
		m.logger.Info("No embedding provider specified, defaulting to BM25", "provider", "bm25")
	}

	m.logger.Info("Initializing semantic search", "provider", provider)

	// Create embedding cache if configured
	embeddingCache := m.createEmbeddingCache(buckets)

	// Create embedder based on provider with automatic fallback
	if err := m.createEmbedder(provider, embeddingCache); err != nil {
		return err
	}

	// Initialize TTL caches and storage
	return m.initializeCachesAndStorage(buckets)
}

// createEmbeddingCache creates the embedding cache from configured bucket
func (m *Manager) createEmbeddingCache(buckets map[string]jetstream.KeyValue) embedding.Cache {
	if m.config.Embedding.CacheBucket == "" {
		return nil
	}

	cacheBucket, ok := buckets[m.config.Embedding.CacheBucket]
	if ok && cacheBucket != nil {
		m.logger.Info("Embedding cache enabled", "bucket", m.config.Embedding.CacheBucket)
		return embedding.NewNATSCache(cacheBucket)
	}

	m.logger.Warn("Embedding cache bucket not found, caching disabled", "bucket", m.config.Embedding.CacheBucket)
	return nil
}

// createEmbedder creates the embedder based on provider with automatic fallback
func (m *Manager) createEmbedder(provider string, embeddingCache embedding.Cache) error {
	switch provider {
	case "http":
		return m.createHTTPEmbedder(embeddingCache)
	case "bm25":
		m.createBM25Embedder()
		return nil
	default:
		return errs.WrapInvalid(
			fmt.Errorf("unknown embedding provider: %s", provider),
			"IndexManager", "initializeSemanticSearch",
			"supported providers: http, bm25, disabled")
	}
}

// createHTTPEmbedder creates HTTP embedder and tests connectivity with BM25 fallback
func (m *Manager) createHTTPEmbedder(embeddingCache embedding.Cache) error {
	httpConfig := embedding.HTTPConfig{
		BaseURL: m.config.Embedding.HTTPEndpoint,
		Model:   m.config.Embedding.HTTPModel,
		Cache:   embeddingCache,
		Logger:  m.logger,
	}

	embedder, err := embedding.NewHTTPEmbedder(httpConfig)
	if err != nil {
		return errs.WrapTransient(
			err,
			"IndexManager",
			"initializeSemanticSearch",
			"failed to create HTTP embedder",
		)
	}
	m.embedder = embedder

	// Test connectivity with a simple embedding request
	testCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, testErr := m.embedder.Generate(testCtx, []string{"connectivity test"})
	if testErr != nil {
		m.handleHTTPEmbedderFallback(testErr)
	} else {
		// HTTP embedder is working
		if m.promMetrics != nil {
			m.promMetrics.embeddingProvider.Set(2) // 2 = HTTP
		}
	}

	return nil
}

// handleHTTPEmbedderFallback handles fallback from HTTP to BM25 embedder
func (m *Manager) handleHTTPEmbedderFallback(testErr error) {
	m.logger.Warn("HTTP embedding service unavailable - falling back to BM25",
		"endpoint", m.config.Embedding.HTTPEndpoint,
		"error", testErr.Error(),
		"fallback", "bm25",
		"hint", "Start embedding service with: task services:start:embedding")

	// Automatic fallback to BM25 instead of disabling
	m.createBM25Embedder()

	m.logger.Info("Fallback to BM25 embedder successful",
		"dimensions", m.embedder.Dimensions(),
		"model", m.embedder.Model())

	// Record fallback event
	if m.promMetrics != nil {
		m.promMetrics.embeddingFallbacks.Inc()
		m.promMetrics.embeddingProvider.Set(1) // 1 = BM25
	}
}

// createBM25Embedder creates pure Go BM25 embedder with standard parameters
func (m *Manager) createBM25Embedder() {
	m.embedder = embedding.NewBM25Embedder(embedding.BM25Config{
		Dimensions: 384,  // Match neural embedding dimensions for compatibility
		K1:         1.5,  // Standard BM25 parameter
		B:          0.75, // Standard BM25 parameter
	})

	m.logger.Info("BM25 embedder initialized (pure Go lexical search)",
		"dimensions", m.embedder.Dimensions(),
		"model", m.embedder.Model())

	// Record provider
	if m.promMetrics != nil {
		m.promMetrics.embeddingProvider.Set(1) // 1 = BM25
	}
}

// initializeCachesAndStorage initializes TTL caches and persistent storage
func (m *Manager) initializeCachesAndStorage(buckets map[string]jetstream.KeyValue) error {
	ctx := context.Background()

	// Create vector cache with configured retention window
	vectorCache, err := cache.NewTTL[[]float32](
		ctx,
		m.config.Embedding.RetentionWindow,
		5*time.Minute, // cleanup interval
		cache.WithEvictionCallback(func(entityID string, _ []float32) {
			m.logger.Debug("Evicted embedding vector", "entity_id", entityID, "reason", "ttl_expired")
		}),
	)
	if err != nil {
		return errs.WrapTransient(err, "IndexManager", "initializeSemanticSearch", "failed to create vector cache")
	}
	m.vectorCache = vectorCache

	// Create metadata cache with same TTL
	metadataCache, err := cache.NewTTL[*EntityMetadata](
		ctx,
		m.config.Embedding.RetentionWindow,
		5*time.Minute, // cleanup interval
		cache.WithEvictionCallback(func(entityID string, _ *EntityMetadata) {
			m.logger.Debug("Evicted entity metadata", "entity_id", entityID, "reason", "ttl_expired")
		}),
	)
	if err != nil {
		// Clean up vector cache if metadata cache fails
		vectorCache.Close()
		return errs.WrapTransient(err, "IndexManager", "initializeSemanticSearch", "failed to create metadata cache")
	}
	m.metadataCache = metadataCache

	// Initialize embedding storage (persistent KV buckets)
	embeddingIndexBucket, hasIndexBucket := buckets["EMBEDDING_INDEX"]
	embeddingDedupBucket, hasDedupBucket := buckets["EMBEDDING_DEDUP"]

	if hasIndexBucket && hasDedupBucket {
		m.embeddingStorage = embedding.NewStorage(embeddingIndexBucket, embeddingDedupBucket)

		// Initialize embedding worker for async generation with cache callback
		m.embeddingWorker = embedding.NewWorker(
			m.embeddingStorage,
			m.embedder,
			embeddingIndexBucket,
			m.logger,
		).WithWorkers(m.config.Workers). // Use same worker count as index manager
							WithOnGenerated(func(entityID string, vector []float32) {
				// Populate vector cache for search when embedding is generated
				if m.vectorCache != nil {
					m.vectorCache.Set(entityID, vector)
					m.logger.Debug("Vector cache populated", "entity_id", entityID, "dimensions", len(vector))
				}
				// Increment embedding metrics (Fix: these metrics were defined but never incremented)
				if m.promMetrics != nil {
					m.promMetrics.embeddingsGenerated.Inc()
				}
			})

		// Wire up embedding worker metrics for observability
		if metricsAdapter := NewEmbeddingWorkerMetricsAdapter(m.promMetrics); metricsAdapter != nil {
			m.embeddingWorker.WithMetrics(metricsAdapter)
		}

		// Configure content store for ContentStorable pattern (if bucket specified)
		if m.config.Embedding.ContentStoreBucket != "" && m.natsClient != nil {
			contentStoreCfg := objectstore.Config{
				BucketName: m.config.Embedding.ContentStoreBucket,
			}
			contentStore, err := objectstore.NewStoreWithConfig(ctx, m.natsClient, contentStoreCfg)
			if err != nil {
				m.logger.Warn("Failed to create content store - StorageRef embedding disabled",
					"bucket", m.config.Embedding.ContentStoreBucket,
					"error", err)
			} else {
				m.embeddingWorker.WithContentStore(contentStore)
				m.logger.Info("Content store configured for ContentStorable embedding",
					"bucket", m.config.Embedding.ContentStoreBucket)
			}
		}

		m.logger.Info("Embedding storage and worker initialized",
			"workers", m.config.Workers)
	} else {
		m.logger.Warn("Embedding buckets not found - async embeddings disabled",
			"has_index", hasIndexBucket,
			"has_dedup", hasDedupBucket)
	}

	m.logger.Info("Semantic search initialized successfully",
		"provider", m.config.Embedding.Provider,
		"model", m.embedder.Model(),
		"dimensions", m.embedder.Dimensions(),
		"retention_window", m.config.Embedding.RetentionWindow)

	return nil
}

// SearchSemantic performs semantic similarity search using embeddings.
//
// Returns an error if semantic search is not enabled in the configuration.
func (m *Manager) SearchSemantic(
	ctx context.Context,
	query string,
	opts *SemanticSearchOptions,
) (*SearchResults, error) {
	startTime := time.Now()

	// Validate inputs
	if err := m.validateSemanticSearch(query); err != nil {
		return nil, err
	}

	// Generate query embedding
	queryEmbedding, err := m.generateQueryEmbedding(ctx, query)
	if err != nil {
		return nil, err
	}

	// Default options
	if opts == nil {
		opts = &SemanticSearchOptions{
			Threshold: 0.3, // Reasonable default for all-MiniLM-L6-v2
			Limit:     10,
		}
	}

	// Compute similarity scores and filter
	hits := m.computeSimilarityScores(queryEmbedding, opts)

	// Sort and limit results
	hits = m.sortAndLimitHits(hits, opts)

	// Build final results
	results := m.buildSearchResults(hits, startTime)

	return results, nil
}

// validateSemanticSearch checks if semantic search is enabled and query is valid
func (m *Manager) validateSemanticSearch(query string) error {
	if m.embedder == nil {
		return errs.WrapInvalid(
			fmt.Errorf("semantic search not enabled"),
			"IndexManager", "SearchSemantic",
			"configure Embedding.Enabled=true and Embedding.Provider to enable semantic search")
	}

	if query == "" {
		return errs.WrapInvalid(
			fmt.Errorf("query is empty"),
			"IndexManager", "SearchSemantic",
			"query string cannot be empty")
	}

	return nil
}

// generateQueryEmbedding generates the embedding vector for the search query
func (m *Manager) generateQueryEmbedding(ctx context.Context, query string) ([]float32, error) {
	embeddings, err := m.embedder.Generate(ctx, []string{query})
	if err != nil {
		return nil, errs.WrapTransient(err, "IndexManager", "SearchSemantic", "failed to generate query embedding")
	}
	if len(embeddings) == 0 {
		return nil, errs.WrapTransient(
			fmt.Errorf("no embedding generated for query"),
			"IndexManager", "SearchSemantic", "empty embedding response")
	}
	return embeddings[0], nil
}

// computeSimilarityScores computes similarity scores for all vectors and applies filters
func (m *Manager) computeSimilarityScores(queryEmbedding []float32, opts *SemanticSearchOptions) []scoredHit {
	var hits []scoredHit
	for _, entityID := range m.vectorCache.Keys() {
		vec, ok := m.vectorCache.Get(entityID)
		if !ok {
			continue // Entry evicted between Keys() and Get()
		}

		score := embedding.CosineSimilarity(queryEmbedding, vec)

		// Apply user-specified threshold (default 0.3 set in handleQuerySemantic)
		if score < opts.Threshold {
			continue
		}

		// Apply type filter if specified
		if len(opts.Types) > 0 {
			if !m.matchesTypeFilter(entityID, opts.Types) {
				continue
			}
		}

		// Apply k-core filter if specified
		if opts.MinCoreFilter > 0 {
			if !m.passesKCoreFilter(entityID, opts.MinCoreFilter) {
				continue
			}
		}

		hits = append(hits, scoredHit{entityID: entityID, score: score})
	}
	return hits
}

// passesKCoreFilter checks if entity has core number >= minCore
func (m *Manager) passesKCoreFilter(entityID string, minCore int) bool {
	if m.structuralIndices == nil {
		return true // No index available, include all
	}
	idx := m.structuralIndices.GetKCoreIndex()
	if idx == nil {
		return true // No index computed yet, include all
	}
	return idx.GetCore(entityID) >= minCore
}

// matchesTypeFilter checks if entity matches any of the specified types
func (m *Manager) matchesTypeFilter(entityID string, types []string) bool {
	meta, ok := m.metadataCache.Get(entityID)
	if !ok {
		return false
	}
	for _, t := range types {
		if meta.EntityType == t {
			return true
		}
	}
	return false
}

// sortAndLimitHits sorts hits by score descending and applies limit
func (m *Manager) sortAndLimitHits(hits []scoredHit, opts *SemanticSearchOptions) []scoredHit {
	// Sort by score descending
	sort.Slice(hits, func(i, j int) bool {
		return hits[i].score > hits[j].score
	})

	// Apply limit
	limit := opts.Limit
	if limit <= 0 {
		limit = 10
	}
	if len(hits) > limit {
		hits = hits[:limit]
	}

	return hits
}

// buildSearchResults constructs the final SearchResults from scored hits
func (m *Manager) buildSearchResults(hits []scoredHit, startTime time.Time) *SearchResults {
	results := &SearchResults{
		Hits:      make([]*SearchHit, 0, len(hits)),
		Total:     len(hits),
		QueryTime: time.Since(startTime),
	}

	for _, hit := range hits {
		meta, ok := m.metadataCache.Get(hit.entityID)
		if !ok {
			continue // Entry evicted between scoring and result building
		}

		// Extract text for snippet
		text := m.extractText(meta.Properties)
		snippet := text
		if len(snippet) > 150 {
			snippet = snippet[:150] + "..."
		}

		// Get location if available from spatial index
		location := m.extractLocation(meta.Properties)

		results.Hits = append(results.Hits, &SearchHit{
			EntityID:   hit.entityID,
			Score:      hit.score,
			Snippet:    snippet,
			Properties: meta.Properties,
			Timestamp:  meta.Updated,
			Location:   location,
		})
	}

	return results
}

// extractLocation extracts location from entity properties if available
func (m *Manager) extractLocation(properties map[string]interface{}) *GeoPoint {
	if !m.config.Indexes.Spatial {
		return nil
	}

	// Try "latitude"/"longitude" first
	if lat, ok := properties["latitude"].(float64); ok {
		if lon, ok := properties["longitude"].(float64); ok {
			return &GeoPoint{Lat: lat, Lon: lon}
		}
	}

	// Try "lat"/"lon" as fallback
	if lat, ok := properties["lat"].(float64); ok {
		if lon, ok := properties["lon"].(float64); ok {
			return &GeoPoint{Lat: lat, Lon: lon}
		}
	}

	return nil
}

// SearchHybrid combines semantic, temporal, and spatial filters.
//
// Returns an error if semantic search is not enabled when SemanticQuery is provided.
func (m *Manager) SearchHybrid(ctx context.Context, query *HybridQuery) (*SearchResults, error) {
	startTime := time.Now()

	if err := m.validateHybridQuery(query); err != nil {
		return nil, err
	}

	// Start with all entities and apply filters
	candidateIDs := m.initializeCandidates()
	candidateIDs = m.applyTemporalFilter(candidateIDs, query.TimeRange)
	candidateIDs = m.applySpatialFilter(candidateIDs, query.GeoBounds)
	candidateIDs = m.applyTypeFilter(candidateIDs, query.Types)

	// Apply semantic search if query provided
	if query.SemanticQuery != "" {
		return m.executeSemanticSearch(ctx, query, candidateIDs, startTime)
	}

	// No semantic query - return all candidates
	return m.buildNonSemanticResults(candidateIDs, startTime), nil
}

// validateHybridQuery validates the hybrid query parameters
func (m *Manager) validateHybridQuery(query *HybridQuery) error {
	if query == nil {
		return errs.WrapInvalid(fmt.Errorf("query is nil"), "IndexManager", "SearchHybrid", "query cannot be nil")
	}

	if query.SemanticQuery != "" && m.embedder == nil {
		return errs.WrapInvalid(
			fmt.Errorf("semantic search not enabled"),
			"IndexManager", "SearchHybrid",
			"configure Embedding.Enabled=true to use semantic queries")
	}

	return nil
}

// initializeCandidates creates initial candidate set from all entities
func (m *Manager) initializeCandidates() map[string]bool {
	candidateIDs := make(map[string]bool)
	for _, entityID := range m.metadataCache.Keys() {
		candidateIDs[entityID] = true
	}
	return candidateIDs
}

// applyTemporalFilter filters candidates by time range
func (m *Manager) applyTemporalFilter(candidates map[string]bool, timeRange *TimeRange) map[string]bool {
	if timeRange == nil {
		return candidates
	}

	filtered := make(map[string]bool)
	for entityID := range candidates {
		meta, ok := m.metadataCache.Get(entityID)
		if ok && !meta.Updated.Before(timeRange.Start) && !meta.Updated.After(timeRange.End) {
			filtered[entityID] = true
		}
	}
	return filtered
}

// applySpatialFilter filters candidates by geographic bounds
func (m *Manager) applySpatialFilter(candidates map[string]bool, bounds *GeoBounds) map[string]bool {
	if bounds == nil {
		return candidates
	}

	filtered := make(map[string]bool)
	for entityID := range candidates {
		meta, ok := m.metadataCache.Get(entityID)
		if !ok {
			continue
		}

		lat, lon, hasLocation := extractLatLon(meta.Properties)
		if hasLocation && isWithinBounds(lat, lon, bounds) {
			filtered[entityID] = true
		}
	}
	return filtered
}

// extractLatLon extracts latitude and longitude from properties
func extractLatLon(properties map[string]interface{}) (float64, float64, bool) {
	if lat, ok := properties["latitude"].(float64); ok {
		if lon, ok := properties["longitude"].(float64); ok {
			return lat, lon, true
		}
	}

	if lat, ok := properties["lat"].(float64); ok {
		if lon, ok := properties["lon"].(float64); ok {
			return lat, lon, true
		}
	}

	return 0, 0, false
}

// isWithinBounds checks if coordinates are within geographic bounds
func isWithinBounds(lat, lon float64, bounds *GeoBounds) bool {
	return lat >= bounds.SouthWest.Lat &&
		lat <= bounds.NorthEast.Lat &&
		lon >= bounds.SouthWest.Lon &&
		lon <= bounds.NorthEast.Lon
}

// applyTypeFilter filters candidates by entity type
func (m *Manager) applyTypeFilter(candidates map[string]bool, types []string) map[string]bool {
	if len(types) == 0 {
		return candidates
	}

	filtered := make(map[string]bool)
	for entityID := range candidates {
		meta, ok := m.metadataCache.Get(entityID)
		if !ok {
			continue
		}
		for _, t := range types {
			if meta.EntityType == t {
				filtered[entityID] = true
				break
			}
		}
	}
	return filtered
}

// executeSemanticSearch performs semantic search on filtered candidates
func (m *Manager) executeSemanticSearch(
	ctx context.Context,
	query *HybridQuery,
	candidates map[string]bool,
	startTime time.Time,
) (*SearchResults, error) {
	queryEmbedding, err := m.generateHybridQueryEmbedding(ctx, query.SemanticQuery)
	if err != nil {
		return nil, err
	}

	hits := m.scoreAndFilterCandidates(candidates, queryEmbedding, query.MinScore)
	hits = m.sortAndLimitHybridHits(hits, query.Limit)

	return m.buildSemanticResults(hits, startTime), nil
}

// generateHybridQueryEmbedding generates embedding for the hybrid query text
func (m *Manager) generateHybridQueryEmbedding(ctx context.Context, queryText string) ([]float32, error) {
	embeddings, err := m.embedder.Generate(ctx, []string{queryText})
	if err != nil {
		return nil, errs.WrapTransient(err, "IndexManager", "SearchHybrid", "failed to generate query embedding")
	}
	if len(embeddings) == 0 {
		return nil, errs.WrapTransient(
			fmt.Errorf("no embedding generated for query"),
			"IndexManager", "SearchHybrid", "empty embedding response")
	}
	return embeddings[0], nil
}

// scoreAndFilterCandidates scores candidates and filters by threshold
func (m *Manager) scoreAndFilterCandidates(
	candidates map[string]bool,
	queryEmbedding []float32,
	minScore float64,
) []scoredHit {
	var hits []scoredHit

	for entityID := range candidates {
		vec, ok := m.vectorCache.Get(entityID)
		if !ok {
			continue
		}

		score := embedding.CosineSimilarity(queryEmbedding, vec)
		if score < minScore {
			continue
		}

		hits = append(hits, scoredHit{entityID: entityID, score: score})
	}

	return hits
}

// sortAndLimitHybridHits sorts hits by score and applies limit
func (m *Manager) sortAndLimitHybridHits(hits []scoredHit, limit int) []scoredHit {
	sort.Slice(hits, func(i, j int) bool {
		return hits[i].score > hits[j].score
	})

	if limit == 0 {
		limit = 10
	}
	if len(hits) > limit {
		hits = hits[:limit]
	}

	return hits
}

// buildSemanticResults builds search results from scored hits
func (m *Manager) buildSemanticResults(hits []scoredHit, startTime time.Time) *SearchResults {
	results := &SearchResults{
		Hits:      make([]*SearchHit, 0, len(hits)),
		Total:     len(hits),
		QueryTime: time.Since(startTime),
	}

	for _, hit := range hits {
		meta, ok := m.metadataCache.Get(hit.entityID)
		if !ok {
			continue
		}

		text := m.extractText(meta.Properties)
		snippet := text
		if len(snippet) > 150 {
			snippet = snippet[:150] + "..."
		}

		results.Hits = append(results.Hits, &SearchHit{
			EntityID:   hit.entityID,
			Score:      hit.score,
			Snippet:    snippet,
			Properties: meta.Properties,
			Timestamp:  meta.Updated,
			Location:   extractGeoPoint(meta.Properties),
		})
	}

	return results
}

// buildNonSemanticResults builds results for non-semantic queries
func (m *Manager) buildNonSemanticResults(candidates map[string]bool, startTime time.Time) *SearchResults {
	results := &SearchResults{
		Hits:      make([]*SearchHit, 0, len(candidates)),
		Total:     len(candidates),
		QueryTime: time.Since(startTime),
	}

	for entityID := range candidates {
		meta, ok := m.metadataCache.Get(entityID)
		if !ok {
			continue
		}

		results.Hits = append(results.Hits, &SearchHit{
			EntityID:   entityID,
			Score:      1.0,
			Snippet:    "",
			Properties: meta.Properties,
			Timestamp:  meta.Updated,
			Location:   extractGeoPoint(meta.Properties),
		})
	}

	return results
}

// extractGeoPoint extracts GeoPoint from properties
func extractGeoPoint(properties map[string]interface{}) *GeoPoint {
	lat, lon, hasLocation := extractLatLon(properties)
	if hasLocation {
		return &GeoPoint{Lat: lat, Lon: lon}
	}
	return nil
}

// extractText extracts text from entity properties for embedding generation.
//
// Uses the configured text fields to extract relevant text content.
func (m *Manager) extractText(properties map[string]interface{}) string {
	var parts []string

	textFields := m.config.Embedding.TextFields
	if len(textFields) == 0 {
		// Default text fields
		textFields = []string{"title", "content", "description", "summary", "text", "name"}
	}

	for _, field := range textFields {
		if value, ok := properties[field]; ok {
			if str, ok := value.(string); ok && str != "" {
				parts = append(parts, str)
			}
		}
	}

	return strings.Join(parts, " ")
}

// FindSimilarEntities returns entities similar to the given entity based on embedding similarity.
// This method is used by SemanticGraphProvider to create virtual edges for LPA clustering,
// enabling community detection even when explicit relationship triples don't exist.
//
// Parameters:
//   - entityID: The source entity to find similar entities for
//   - threshold: Minimum cosine similarity score (0.0-1.0), recommended 0.6 for clustering
//   - limit: Maximum number of similar entities to return
//
// Returns an error if semantic search is not enabled or entity has no embedding.
func (m *Manager) FindSimilarEntities(
	ctx context.Context,
	entityID string,
	threshold float64,
	limit int,
) ([]SimilarityHit, error) {
	// Validate semantic search is enabled
	if m.vectorCache == nil || m.embedder == nil {
		return nil, errs.WrapInvalid(
			fmt.Errorf("semantic search not enabled"),
			"IndexManager", "FindSimilarEntities",
			"configure Embedding.Enabled=true to use similarity-based inference")
	}

	// Validate inputs
	if entityID == "" {
		return nil, errs.WrapInvalid(
			fmt.Errorf("entityID is empty"),
			"IndexManager", "FindSimilarEntities",
			"entityID cannot be empty")
	}
	if threshold < 0 || threshold > 1 {
		return nil, errs.WrapInvalid(
			fmt.Errorf("threshold out of range: %f", threshold),
			"IndexManager", "FindSimilarEntities",
			"threshold must be between 0.0 and 1.0")
	}
	if limit <= 0 {
		limit = 5 // Default limit for clustering neighbors
	}

	// Get source entity's embedding
	sourceVec, ok := m.vectorCache.Get(entityID)
	if !ok {
		// No embedding for this entity - may be newly created or non-embeddable type
		return nil, nil
	}

	// Get source entity's type for potential type-batched filtering
	var sourceType string
	if meta, ok := m.metadataCache.Get(entityID); ok {
		sourceType = meta.EntityType
	}

	// Compute similarity against all cached entities
	var hits []SimilarityHit
	for _, candidateID := range m.vectorCache.Keys() {
		// Skip self
		if candidateID == entityID {
			continue
		}

		// Check context cancellation
		select {
		case <-ctx.Done():
			return nil, errs.WrapTransient(ctx.Err(), "IndexManager", "FindSimilarEntities", "context cancelled")
		default:
		}

		candidateVec, ok := m.vectorCache.Get(candidateID)
		if !ok {
			continue // Entry evicted
		}

		score := embedding.CosineSimilarity(sourceVec, candidateVec)

		// Apply threshold filter
		if score < threshold {
			continue
		}

		// Get candidate type for filtering/reporting
		var candidateType string
		if meta, ok := m.metadataCache.Get(candidateID); ok {
			candidateType = meta.EntityType
		}

		// Optional: Type-batched filtering for O(n²) mitigation
		// Only compare within same entity type family when enabled
		// Uncomment when scaling becomes an issue:
		// if sourceType != "" && candidateType != "" && !sameTypeFamily(sourceType, candidateType) {
		//     continue
		// }
		_ = sourceType // Silence unused variable warning (reserved for future type batching)

		hits = append(hits, SimilarityHit{
			EntityID:   candidateID,
			Similarity: score,
			EntityType: candidateType,
		})
	}

	// Sort by similarity descending
	sort.Slice(hits, func(i, j int) bool {
		return hits[i].Similarity > hits[j].Similarity
	})

	// Apply limit
	if len(hits) > limit {
		hits = hits[:limit]
	}

	return hits, nil
}
