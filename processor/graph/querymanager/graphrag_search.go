package querymanager

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	gtypes "github.com/c360/semstreams/graph"
	"github.com/c360/semstreams/message"
	"github.com/c360/semstreams/pkg/errs"
	"github.com/c360/semstreams/processor/graph/clustering"
	"github.com/c360/semstreams/processor/graph/indexmanager"
	"github.com/c360/semstreams/processor/graph/llm"
)

const (
	// DefaultMaxCommunities is the default number of communities to search in GlobalSearch
	DefaultMaxCommunities = 5

	// MaxTotalEntitiesInSearch limits the total number of entities that can be loaded
	// across all communities in GlobalSearch to prevent unbounded memory usage
	MaxTotalEntitiesInSearch = 10000

	// ScoreWeightSummary is the weight for summary text matches in community scoring
	ScoreWeightSummary = 2.0

	// ScoreWeightKeyword is the weight for keyword matches in community scoring
	ScoreWeightKeyword = 1.5
)

// communityDetectorInterface defines the minimal interface needed for GraphRAG search
// Uses *clustering.Community to avoid import cycles
type communityDetectorInterface interface {
	GetCommunity(ctx context.Context, communityID string) (*clustering.Community, error)
	GetEntityCommunity(ctx context.Context, entityID string, level int) (*clustering.Community, error)
	GetCommunitiesByLevel(ctx context.Context, level int) ([]*clustering.Community, error)
}

// LocalSearch performs a search within an entity's community
// This implements the GraphRAG "local search" pattern:
// 1. Find the entity's community at the specified level
// 2. Search only within that community's members
// 3. Return results annotated with community information
func (m *Manager) LocalSearch(
	ctx context.Context,
	entityID string,
	query string,
	level int,
) (*LocalSearchResult, error) {
	startTime := time.Now()
	defer m.recordActivity()

	// Validate inputs
	if entityID == "" {
		return nil, errs.WrapInvalid(errs.ErrMissingConfig, "QueryManager",
			"LocalSearch", "entityID is empty")
	}

	if query == "" {
		return nil, errs.WrapInvalid(errs.ErrMissingConfig, "QueryManager",
			"LocalSearch", "query is empty")
	}

	// Check if community detector is available
	if m.communityDetector == nil {
		return nil, errs.WrapTransient(errs.ErrMissingConfig, "QueryManager",
			"LocalSearch", "community detector not available")
	}

	// Type assert to community detector interface
	detector, ok := m.communityDetector.(communityDetectorInterface)
	if !ok {
		return nil, errs.WrapTransient(errs.ErrMissingConfig, "QueryManager",
			"LocalSearch", "community detector does not implement required interface")
	}

	// Find the entity's community
	community, err := detector.GetEntityCommunity(ctx, entityID, level)
	if err != nil {
		return nil, errs.WrapTransient(err, "QueryManager", "LocalSearch",
			fmt.Sprintf("failed to get community for entity %s at level %d", entityID, level))
	}

	if community == nil {
		return nil, errs.WrapTransient(errs.ErrInvalidData, "QueryManager",
			"LocalSearch", fmt.Sprintf("entity %s not in any community at level %d", entityID, level))
	}

	// Get all entities in the community
	entities, err := m.GetEntities(ctx, community.Members)
	if err != nil {
		return nil, errs.WrapTransient(err, "QueryManager", "LocalSearch",
			"failed to load community members")
	}

	// Filter entities based on query
	matchedEntities := m.filterEntitiesByQuery(entities, query)

	return &LocalSearchResult{
		Entities:    matchedEntities,
		CommunityID: community.ID,
		Count:       len(matchedEntities),
		Duration:    time.Since(startTime),
	}, nil
}

// GlobalSearch performs a cross-community search using community summaries
// This implements the GraphRAG "global search" pattern:
// 1. Retrieve all communities at the specified level
// 2. Score community summaries against the query
// 3. Select top-N most relevant communities
// 4. Search within selected communities
// 5. Re-rank and deduplicate results
func (m *Manager) GlobalSearch(
	ctx context.Context,
	query string,
	level int,
	maxCommunities int,
) (*GlobalSearchResult, error) {
	startTime := time.Now()
	defer m.recordActivity()

	// Validate inputs
	if query == "" {
		return nil, errs.WrapInvalid(errs.ErrMissingConfig, "QueryManager",
			"GlobalSearch", "query is empty")
	}

	if maxCommunities <= 0 {
		maxCommunities = DefaultMaxCommunities
	}

	// Check if community detector is available
	if m.communityDetector == nil {
		return nil, errs.WrapTransient(errs.ErrMissingConfig, "QueryManager",
			"GlobalSearch", "community detector not available")
	}

	// Type assert to community detector interface
	detector, ok := m.communityDetector.(communityDetectorInterface)
	if !ok {
		return nil, errs.WrapTransient(errs.ErrMissingConfig, "QueryManager",
			"GlobalSearch", "community detector does not implement required interface")
	}

	// Get all communities at the specified level
	communities, err := detector.GetCommunitiesByLevel(ctx, level)
	if err != nil {
		return nil, errs.WrapTransient(err, "QueryManager", "GlobalSearch",
			fmt.Sprintf("failed to get communities at level %d", level))
	}

	if len(communities) == 0 {
		return &GlobalSearchResult{
			Entities:           []*gtypes.EntityState{},
			CommunitySummaries: []CommunitySummary{},
			Count:              0,
			Duration:           time.Since(startTime),
		}, nil
	}

	// Score communities based on their summaries
	scoredCommunities := m.scoreCommunitySummaries(communities, query)

	// Select top-N communities
	selectedCount := maxCommunities
	if len(scoredCommunities) < selectedCount {
		selectedCount = len(scoredCommunities)
	}
	topCommunities := scoredCommunities[:selectedCount]

	// Collect all entity IDs from selected communities
	entityIDSet := make(map[string]bool)
	for _, comm := range topCommunities {
		for _, memberID := range comm.Members {
			entityIDSet[memberID] = true
		}
	}

	// Convert to slice
	entityIDs := make([]string, 0, len(entityIDSet))
	for id := range entityIDSet {
		entityIDs = append(entityIDs, id)
	}

	// Enforce resource limit to prevent unbounded memory usage
	if len(entityIDs) > MaxTotalEntitiesInSearch {
		entityIDs = entityIDs[:MaxTotalEntitiesInSearch]
	}

	// Load entities
	entities, err := m.GetEntities(ctx, entityIDs)
	if err != nil {
		return nil, errs.WrapTransient(err, "QueryManager", "GlobalSearch",
			"failed to load entities from communities")
	}

	// Filter entities based on query
	matchedEntities := m.filterEntitiesByQuery(entities, query)

	// Build community summaries for response
	summaries := make([]CommunitySummary, len(topCommunities))
	for i, comm := range topCommunities {
		// Find relevance score from scored list
		var relevance float64
		for _, scored := range scoredCommunities {
			if scored.ID == comm.ID {
				// Calculate relevance from position (simple approach)
				relevance = 1.0 - (float64(i) / float64(len(scoredCommunities)))
				break
			}
		}

		// Prefer LLM summary if available, fallback to statistical
		summary := comm.LLMSummary
		if summary == "" {
			summary = comm.StatisticalSummary
		}

		summaries[i] = CommunitySummary{
			CommunityID: comm.ID,
			Summary:     summary,
			Keywords:    comm.Keywords,
			Level:       comm.Level,
			Relevance:   relevance,
		}
	}

	return &GlobalSearchResult{
		Entities:           matchedEntities,
		CommunitySummaries: summaries,
		Count:              len(matchedEntities),
		Duration:           time.Since(startTime),
	}, nil
}

// filterEntitiesByQuery filters entities based on simple text matching
// This is a basic implementation using keyword matching
// Future: Could use semantic search with embeddings
func (m *Manager) filterEntitiesByQuery(entities []*gtypes.EntityState, query string) []*gtypes.EntityState {
	query = strings.ToLower(query)
	queryTerms := strings.Fields(query)

	if len(queryTerms) == 0 {
		return entities // No filtering if query is empty
	}

	matched := make([]*gtypes.EntityState, 0)

	for _, entity := range entities {
		if m.entityMatchesQuery(entity, queryTerms) {
			matched = append(matched, entity)
		}
	}

	return matched
}

// entityMatchesQuery checks if an entity matches the query terms
func (m *Manager) entityMatchesQuery(entity *gtypes.EntityState, queryTerms []string) bool {
	// Build searchable text from entity
	var searchText strings.Builder

	// Add entity ID
	searchText.WriteString(strings.ToLower(entity.ID))
	searchText.WriteString(" ")

	// Extract type from ID and add it
	if eid, err := message.ParseEntityID(entity.ID); err == nil {
		searchText.WriteString(strings.ToLower(eid.Type))
		searchText.WriteString(" ")
	}

	// Add properties from triples
	for _, triple := range entity.Triples {
		if !triple.IsRelationship() {
			// Add predicate (property name)
			searchText.WriteString(strings.ToLower(triple.Predicate))
			searchText.WriteString(" ")

			// Add object value if it's a string
			if strVal, ok := triple.Object.(string); ok {
				searchText.WriteString(strings.ToLower(strVal))
				searchText.WriteString(" ")
			}
		}
	}

	searchableText := searchText.String()

	// Check if all query terms appear in searchable text
	matchCount := 0
	for _, term := range queryTerms {
		termLower := strings.ToLower(term)
		if strings.Contains(searchableText, termLower) {
			matchCount++
		}
	}

	// Require at least one term to match
	return matchCount > 0
}

// scoreCommunitySummaries scores communities based on query relevance
// Returns communities sorted by relevance (highest first)
func (m *Manager) scoreCommunitySummaries(
	communities []*clustering.Community,
	query string,
) []*clustering.Community {
	type scoredCommunity struct {
		community *clustering.Community
		score     float64
	}

	query = strings.ToLower(query)
	queryTerms := strings.Fields(query)

	scored := make([]scoredCommunity, 0, len(communities))

	for _, comm := range communities {
		score := 0.0

		// Score based on summary text (prefer LLM if available)
		summary := comm.LLMSummary
		if summary == "" {
			summary = comm.StatisticalSummary
		}
		if summary != "" {
			summaryLower := strings.ToLower(summary)
			for _, term := range queryTerms {
				if strings.Contains(summaryLower, term) {
					score += ScoreWeightSummary
				}
			}
		}

		// Score based on keywords
		for _, keyword := range comm.Keywords {
			keywordLower := strings.ToLower(keyword)
			for _, term := range queryTerms {
				if strings.Contains(keywordLower, term) || strings.Contains(term, keywordLower) {
					score += ScoreWeightKeyword
				}
			}
		}

		scored = append(scored, scoredCommunity{
			community: comm,
			score:     score,
		})
	}

	// Sort by score descending - O(n log n) using Go's built-in sort
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score // Descending order
	})

	// Extract sorted communities
	result := make([]*clustering.Community, len(scored))
	for i, sc := range scored {
		result[i] = sc.community
	}

	return result
}

// GetCommunity retrieves a community by ID
func (m *Manager) GetCommunity(ctx context.Context, communityID string) (*clustering.Community, error) {
	defer m.recordActivity()

	// Check if community detector is available
	if m.communityDetector == nil {
		return nil, errs.WrapTransient(errs.ErrMissingConfig, "QueryManager",
			"GetCommunity", "community detector not available")
	}

	// Type assert to community detector interface
	detector, ok := m.communityDetector.(communityDetectorInterface)
	if !ok {
		return nil, errs.WrapTransient(errs.ErrMissingConfig, "QueryManager",
			"GetCommunity", "community detector does not implement required interface")
	}

	// Get community
	community, err := detector.GetCommunity(ctx, communityID)
	if err != nil {
		return nil, errs.WrapTransient(err, "QueryManager", "GetCommunity",
			fmt.Sprintf("failed to get community %s", communityID))
	}

	return community, nil
}

// GlobalSearchWithAnswer performs a global search and generates an LLM answer
// This extends GlobalSearch by:
// 1. Performing the standard global search
// 2. Using LLM to generate a natural language answer based on the results
// If LLM is unavailable, returns results without an answer
func (m *Manager) GlobalSearchWithAnswer(
	ctx context.Context,
	query string,
	level int,
	maxCommunities int,
) (*GlobalSearchResult, error) {
	// Perform standard global search
	result, err := m.GlobalSearch(ctx, query, level, maxCommunities)
	if err != nil {
		return nil, err
	}

	// If LLM client is available, generate an answer
	if m.llmClient != nil && len(result.CommunitySummaries) > 0 {
		answer, model, answerErr := m.generateAnswer(ctx, query, result)
		if answerErr != nil {
			// Log error but don't fail the search
			m.logger.Warn("Failed to generate LLM answer",
				"error", answerErr,
				"query", query)
		} else {
			result.Answer = answer
			result.AnswerModel = model
		}
	}

	return result, nil
}

// generateAnswer uses LLM to generate a natural language answer from search results
func (m *Manager) generateAnswer(
	ctx context.Context,
	query string,
	searchResult *GlobalSearchResult,
) (string, string, error) {
	// Set specific timeout for answer generation (LLM calls can be slow)
	answerCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Build prompt data
	communities := make([]llm.CommunitySummaryInfo, 0, len(searchResult.CommunitySummaries))
	for _, cs := range searchResult.CommunitySummaries {
		communities = append(communities, llm.CommunitySummaryInfo{
			Summary:     cs.Summary,
			EntityCount: 0, // We don't have entity count per community in this context
			Keywords:    strings.Join(cs.Keywords, ", "),
		})
	}

	// Build entity samples (limit to top 10)
	maxEntities := 10
	if len(searchResult.Entities) < maxEntities {
		maxEntities = len(searchResult.Entities)
	}

	entities := make([]llm.EntitySample, 0, maxEntities)
	for i := 0; i < maxEntities; i++ {
		entity := searchResult.Entities[i]
		eid, err := message.ParseEntityID(entity.ID)
		entityType := ""
		if err == nil {
			entityType = eid.Type
		}

		// Try to extract name from triples
		name := ""
		for _, triple := range entity.Triples {
			if triple.Predicate == "name" || triple.Predicate == "dc.title" {
				if strVal, ok := triple.Object.(string); ok {
					name = strVal
					break
				}
			}
		}

		entities = append(entities, llm.EntitySample{
			ID:   entity.ID,
			Type: entityType,
			Name: name,
		})
	}

	// Build prompt data
	promptData := llm.SearchAnswerData{
		Query:       query,
		Communities: communities,
		Entities:    entities,
	}

	// Render the prompt using direct package variable
	rendered, err := llm.SearchPrompt.Render(promptData)
	if err != nil {
		return "", "", errs.WrapTransient(err, "QueryManager",
			"generateAnswer", "failed to render prompt template")
	}

	// Call LLM with timeout context
	temperature := 0.3 // Lower temperature for more focused answers
	resp, err := m.llmClient.ChatCompletion(answerCtx, llm.ChatRequest{
		SystemPrompt: rendered.System,
		UserPrompt:   rendered.User,
		MaxTokens:    500,
		Temperature:  &temperature,
	})
	if err != nil {
		return "", "", errs.WrapTransient(err, "QueryManager",
			"generateAnswer", "LLM chat completion failed")
	}

	return resp.Content, m.llmClient.Model(), nil
}

// GetEntityCommunity retrieves the community containing a specific entity at a given level
func (m *Manager) GetEntityCommunity(ctx context.Context, entityID string, level int) (*clustering.Community, error) {
	defer m.recordActivity()

	// Check if community detector is available
	if m.communityDetector == nil {
		return nil, errs.WrapTransient(errs.ErrMissingConfig, "QueryManager",
			"GetEntityCommunity", "community detector not available")
	}

	// Type assert to community detector interface
	detector, ok := m.communityDetector.(communityDetectorInterface)
	if !ok {
		return nil, errs.WrapTransient(errs.ErrMissingConfig, "QueryManager",
			"GetEntityCommunity", "community detector does not implement required interface")
	}

	// Get entity's community
	community, err := detector.GetEntityCommunity(ctx, entityID, level)
	if err != nil {
		return nil, errs.WrapTransient(err, "QueryManager", "GetEntityCommunity",
			fmt.Sprintf("failed to get community for entity %s at level %d", entityID, level))
	}

	return community, nil
}

// GetCommunitiesByLevel retrieves all communities at a specific hierarchical level
func (m *Manager) GetCommunitiesByLevel(ctx context.Context, level int) ([]*clustering.Community, error) {
	defer m.recordActivity()

	// Check if community detector is available
	if m.communityDetector == nil {
		return nil, errs.WrapTransient(errs.ErrMissingConfig, "QueryManager",
			"GetCommunitiesByLevel", "community detector not available")
	}

	// Type assert to community detector interface
	detector, ok := m.communityDetector.(communityDetectorInterface)
	if !ok {
		return nil, errs.WrapTransient(errs.ErrMissingConfig, "QueryManager",
			"GetCommunitiesByLevel", "community detector does not implement required interface")
	}

	// Get communities by level
	communities, err := detector.GetCommunitiesByLevel(ctx, level)
	if err != nil {
		return nil, errs.WrapTransient(err, "QueryManager", "GetCommunitiesByLevel",
			fmt.Sprintf("failed to get communities at level %d", level))
	}

	return communities, nil
}

// GlobalSearchWithOptions performs a cross-community search with optional index pre-filtering.
// This extends GlobalSearch by:
// 1. Pre-filtering candidate entities using spatial/temporal/predicate indexes
// 2. Filtering communities to only those containing candidates
// 3. Performing standard GraphRAG search on filtered communities
// 4. Optionally using semantic similarity instead of keyword matching
//
// Progressive enhancement: If indexes are unavailable, falls back to standard search.
func (m *Manager) GlobalSearchWithOptions(
	ctx context.Context,
	opts *SearchOptions,
) (*GlobalSearchResult, error) {
	startTime := time.Now()
	defer m.recordActivity()

	// Apply defaults and validate
	opts.SetDefaults()
	if err := opts.Validate(); err != nil {
		return nil, errs.WrapInvalid(err, "QueryManager",
			"GlobalSearchWithOptions", "invalid search options")
	}

	// Check if community detector is available
	if m.communityDetector == nil {
		return nil, errs.WrapTransient(errs.ErrMissingConfig, "QueryManager",
			"GlobalSearchWithOptions", "community detector not available")
	}

	// Type assert to community detector interface
	detector, ok := m.communityDetector.(communityDetectorInterface)
	if !ok {
		return nil, errs.WrapTransient(errs.ErrMissingConfig, "QueryManager",
			"GlobalSearchWithOptions", "community detector does not implement required interface")
	}

	// 1. If index filters present, collect candidate entities FIRST
	var candidateIDs map[string]bool
	if opts.HasIndexFilters() {
		candidates, err := m.collectCandidatesFromIndexes(ctx, opts)
		if err != nil {
			// Log warning but don't fail - fall back to full search
			m.logger.Warn("Index pre-filter failed, falling back to full search",
				"error", err,
				"strategy", opts.InferStrategy())
		} else if len(candidates) > 0 {
			candidateIDs = candidates
			m.logger.Debug("Index pre-filter applied",
				"candidate_count", len(candidates),
				"strategy", opts.InferStrategy())
		}
	}

	// 2. Get all communities at the specified level
	communities, err := detector.GetCommunitiesByLevel(ctx, opts.Level)
	if err != nil {
		return nil, errs.WrapTransient(err, "QueryManager", "GlobalSearchWithOptions",
			fmt.Sprintf("failed to get communities at level %d", opts.Level))
	}

	if len(communities) == 0 {
		return &GlobalSearchResult{
			Entities:           []*gtypes.EntityState{},
			CommunitySummaries: []CommunitySummary{},
			Count:              0,
			Duration:           time.Since(startTime),
		}, nil
	}

	// 3. If we have candidates, filter communities to only those containing candidates
	if len(candidateIDs) > 0 {
		communities = m.filterCommunitiesByMembers(communities, candidateIDs)
		m.logger.Debug("Filtered communities by candidates",
			"remaining_communities", len(communities),
			"candidate_count", len(candidateIDs))

		if len(communities) == 0 {
			// No communities contain any candidates
			return &GlobalSearchResult{
				Entities:           []*gtypes.EntityState{},
				CommunitySummaries: []CommunitySummary{},
				Count:              0,
				Duration:           time.Since(startTime),
			}, nil
		}
	}

	// 4. Score communities based on their summaries
	scoredCommunities := m.scoreCommunitySummaries(communities, opts.Query)

	// 5. Select top-N communities
	selectedCount := opts.MaxCommunities
	if len(scoredCommunities) < selectedCount {
		selectedCount = len(scoredCommunities)
	}
	topCommunities := scoredCommunities[:selectedCount]

	// 6. Collect entity IDs from selected communities
	entityIDSet := make(map[string]bool)
	for _, comm := range topCommunities {
		for _, memberID := range comm.Members {
			// If we have candidates, only include members that are candidates
			if len(candidateIDs) > 0 && !candidateIDs[memberID] {
				continue
			}
			entityIDSet[memberID] = true
		}
	}

	// Convert to slice
	entityIDs := make([]string, 0, len(entityIDSet))
	for id := range entityIDSet {
		entityIDs = append(entityIDs, id)
	}

	// Apply limits
	if len(entityIDs) > MaxTotalEntitiesInSearch {
		entityIDs = entityIDs[:MaxTotalEntitiesInSearch]
	}
	if opts.Limit > 0 && len(entityIDs) > opts.Limit {
		entityIDs = entityIDs[:opts.Limit]
	}

	// 7. Load entities
	entities, err := m.GetEntities(ctx, entityIDs)
	if err != nil {
		return nil, errs.WrapTransient(err, "QueryManager", "GlobalSearchWithOptions",
			"failed to load entities from communities")
	}

	// 8. Filter entities based on query (keyword or semantic)
	matchedEntities := m.filterEntitiesByQueryWithOptions(ctx, entities, opts)

	// 9. Build community summaries for response
	summaries := make([]CommunitySummary, len(topCommunities))
	for i, comm := range topCommunities {
		var relevance float64
		for _, scored := range scoredCommunities {
			if scored.ID == comm.ID {
				relevance = 1.0 - (float64(i) / float64(len(scoredCommunities)))
				break
			}
		}

		summary := comm.LLMSummary
		if summary == "" {
			summary = comm.StatisticalSummary
		}

		summaries[i] = CommunitySummary{
			CommunityID: comm.ID,
			Summary:     summary,
			Keywords:    comm.Keywords,
			Level:       comm.Level,
			Relevance:   relevance,
		}
	}

	return &GlobalSearchResult{
		Entities:           matchedEntities,
		CommunitySummaries: summaries,
		Count:              len(matchedEntities),
		Duration:           time.Since(startTime),
	}, nil
}

// collectCandidatesFromIndexes queries indexes to pre-filter candidate entities.
// Returns a set of entity IDs that match the specified filters.
// Progressive enhancement: unavailable indexes are silently skipped.
func (m *Manager) collectCandidatesFromIndexes(ctx context.Context, opts *SearchOptions) (map[string]bool, error) {
	var indexResults []map[string]bool

	// Check indexManager exists
	if m.indexManager == nil {
		m.logger.Debug("indexManager not available, skipping index pre-filter")
		return nil, nil
	}

	// Check context before starting work
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Spatial pre-filter
	if opts.GeoBounds != nil {
		bounds := indexmanager.Bounds{
			North: opts.GeoBounds.North,
			South: opts.GeoBounds.South,
			East:  opts.GeoBounds.East,
			West:  opts.GeoBounds.West,
		}
		entityIDs, err := m.indexManager.QuerySpatial(ctx, bounds)
		if err != nil {
			m.logger.Warn("spatial index query failed, skipping", "error", err)
		} else if len(entityIDs) > 0 {
			indexResults = append(indexResults, toSet(entityIDs))
			m.logger.Debug("spatial index pre-filter", "count", len(entityIDs))
		}
	}

	// Check context between operations
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Temporal pre-filter
	if opts.TimeRange != nil {
		entityIDs, err := m.indexManager.QueryTemporal(ctx, opts.TimeRange.Start, opts.TimeRange.End)
		if err != nil {
			m.logger.Warn("temporal index query failed, skipping", "error", err)
		} else if len(entityIDs) > 0 {
			indexResults = append(indexResults, toSet(entityIDs))
			m.logger.Debug("temporal index pre-filter", "count", len(entityIDs))
		}
	}

	// Check context between operations
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Predicate pre-filter
	if len(opts.Predicates) > 0 {
		predicateResults := make(map[string]bool)
		for _, pred := range opts.Predicates {
			entityIDs, err := m.indexManager.GetPredicateIndex(ctx, pred)
			if err != nil {
				m.logger.Warn("predicate index query failed", "predicate", pred, "error", err)
				continue
			}
			for _, id := range entityIDs {
				predicateResults[id] = true
			}
		}
		if len(predicateResults) > 0 {
			indexResults = append(indexResults, predicateResults)
			m.logger.Debug("predicate index pre-filter", "count", len(predicateResults))
		}
	}

	// Type pre-filter (uses predicate index with type)
	if len(opts.Types) > 0 {
		typeResults := make(map[string]bool)
		for _, entityType := range opts.Types {
			// Types are stored as predicate "type" with the entity type as value
			// This requires entities to be loaded and filtered by type from their ID
			entityIDs, err := m.indexManager.GetPredicateIndex(ctx, "type:"+entityType)
			if err != nil {
				// Type may not be indexed as predicate - this is expected
				m.logger.Debug("type predicate not found", "type", entityType)
				continue
			}
			for _, id := range entityIDs {
				typeResults[id] = true
			}
		}
		if len(typeResults) > 0 {
			indexResults = append(indexResults, typeResults)
			m.logger.Debug("type index pre-filter", "count", len(typeResults))
		}
	}

	// If no index results, return nil (no pre-filter)
	if len(indexResults) == 0 {
		return nil, nil
	}

	return combineResults(indexResults, opts.RequireAllFilters), nil
}

// filterCommunitiesByMembers filters communities to only those containing at least one candidate.
func (m *Manager) filterCommunitiesByMembers(communities []*clustering.Community, candidateIDs map[string]bool) []*clustering.Community {
	result := make([]*clustering.Community, 0, len(communities))
	for _, comm := range communities {
		for _, memberID := range comm.Members {
			if candidateIDs[memberID] {
				result = append(result, comm)
				break // At least one member is a candidate
			}
		}
	}
	return result
}

// filterEntitiesByQueryWithOptions filters entities using the specified search options.
// Supports both keyword matching and semantic similarity (progressive enhancement).
func (m *Manager) filterEntitiesByQueryWithOptions(ctx context.Context, entities []*gtypes.EntityState, opts *SearchOptions) []*gtypes.EntityState {
	// If semantic filtering requested and embeddings available, use it
	if opts.UseEmbeddings && m.indexManager != nil {
		results := m.filterByEmbeddingSimilarity(ctx, entities, opts.Query)
		if len(results) > 0 {
			return results
		}
		// Fall through to keyword matching if no semantic hits
		m.logger.Debug("semantic filtering returned no results, falling back to keywords")
	}

	// Keyword matching (default)
	return m.filterEntitiesByQuery(entities, opts.Query)
}

// filterByEmbeddingSimilarity filters entities using embedding-based semantic similarity.
// Returns entities sorted by similarity score, filtered by minimum threshold.
func (m *Manager) filterByEmbeddingSimilarity(ctx context.Context, entities []*gtypes.EntityState, query string) []*gtypes.EntityState {
	if m.indexManager == nil || query == "" {
		return nil
	}

	// Check context before expensive operation
	if err := ctx.Err(); err != nil {
		return nil
	}

	// Use hybrid search if available (combines keyword + semantic)
	results, err := m.indexManager.SearchHybrid(ctx, &indexmanager.HybridQuery{
		SemanticQuery: query,
		MinScore:      0.3, // Lower threshold for initial filtering
	})
	if err != nil {
		m.logger.Warn("hybrid search failed for entity filtering", "error", err)
		return nil
	}

	// Build set of matching entity IDs with scores
	matchScores := make(map[string]float64)
	for _, hit := range results.Hits {
		matchScores[hit.EntityID] = hit.Score
	}

	// Filter and sort entities by score
	type scored struct {
		entity *gtypes.EntityState
		score  float64
	}
	var scoredEntities []scored

	for _, entity := range entities {
		if score, ok := matchScores[entity.ID]; ok {
			scoredEntities = append(scoredEntities, scored{entity, score})
		}
	}

	// Sort by score descending
	sort.Slice(scoredEntities, func(i, j int) bool {
		return scoredEntities[i].score > scoredEntities[j].score
	})

	// Extract sorted entities
	result := make([]*gtypes.EntityState, len(scoredEntities))
	for i, s := range scoredEntities {
		result[i] = s.entity
	}

	return result
}

// toSet converts a slice of strings to a set (map[string]bool)
func toSet(ids []string) map[string]bool {
	result := make(map[string]bool, len(ids))
	for _, id := range ids {
		result[id] = true
	}
	return result
}

// combineResults combines multiple index result sets based on the require-all flag.
// If requireAll is true, returns intersection (AND). Otherwise returns union (OR).
func combineResults(indexResults []map[string]bool, requireAll bool) map[string]bool {
	if len(indexResults) == 0 {
		return nil
	}

	if len(indexResults) == 1 {
		return indexResults[0]
	}

	if requireAll {
		// Intersection (AND) - entity must be in ALL result sets
		result := make(map[string]bool)
		// Start with first set
		for id := range indexResults[0] {
			inAll := true
			for i := 1; i < len(indexResults); i++ {
				if !indexResults[i][id] {
					inAll = false
					break
				}
			}
			if inAll {
				result[id] = true
			}
		}
		return result
	}

	// Union (OR) - entity must be in ANY result set
	result := make(map[string]bool)
	for _, set := range indexResults {
		for id := range set {
			result[id] = true
		}
	}
	return result
}
