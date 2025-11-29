package querymanager

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/c360/semstreams/errors"
	gtypes "github.com/c360/semstreams/graph"
	"github.com/c360/semstreams/message"
	"github.com/c360/semstreams/processor/graph/clustering"
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
	defer func() {
		m.lastActivity = time.Now()
	}()

	// Validate inputs
	if entityID == "" {
		return nil, errors.WrapInvalid(errors.ErrMissingConfig, "QueryManager",
			"LocalSearch", "entityID is empty")
	}

	if query == "" {
		return nil, errors.WrapInvalid(errors.ErrMissingConfig, "QueryManager",
			"LocalSearch", "query is empty")
	}

	// Check if community detector is available
	if m.communityDetector == nil {
		return nil, errors.WrapTransient(errors.ErrMissingConfig, "QueryManager",
			"LocalSearch", "community detector not available")
	}

	// Type assert to community detector interface
	detector, ok := m.communityDetector.(communityDetectorInterface)
	if !ok {
		return nil, errors.WrapTransient(errors.ErrMissingConfig, "QueryManager",
			"LocalSearch", "community detector does not implement required interface")
	}

	// Find the entity's community
	community, err := detector.GetEntityCommunity(ctx, entityID, level)
	if err != nil {
		return nil, errors.WrapTransient(err, "QueryManager", "LocalSearch",
			fmt.Sprintf("failed to get community for entity %s at level %d", entityID, level))
	}

	if community == nil {
		return nil, errors.WrapTransient(errors.ErrInvalidData, "QueryManager",
			"LocalSearch", fmt.Sprintf("entity %s not in any community at level %d", entityID, level))
	}

	// Get all entities in the community
	entities, err := m.GetEntities(ctx, community.Members)
	if err != nil {
		return nil, errors.WrapTransient(err, "QueryManager", "LocalSearch",
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
	defer func() {
		m.lastActivity = time.Now()
	}()

	// Validate inputs
	if query == "" {
		return nil, errors.WrapInvalid(errors.ErrMissingConfig, "QueryManager",
			"GlobalSearch", "query is empty")
	}

	if maxCommunities <= 0 {
		maxCommunities = DefaultMaxCommunities
	}

	// Check if community detector is available
	if m.communityDetector == nil {
		return nil, errors.WrapTransient(errors.ErrMissingConfig, "QueryManager",
			"GlobalSearch", "community detector not available")
	}

	// Type assert to community detector interface
	detector, ok := m.communityDetector.(communityDetectorInterface)
	if !ok {
		return nil, errors.WrapTransient(errors.ErrMissingConfig, "QueryManager",
			"GlobalSearch", "community detector does not implement required interface")
	}

	// Get all communities at the specified level
	communities, err := detector.GetCommunitiesByLevel(ctx, level)
	if err != nil {
		return nil, errors.WrapTransient(err, "QueryManager", "GlobalSearch",
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
		return nil, errors.WrapTransient(err, "QueryManager", "GlobalSearch",
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
	defer func() {
		m.lastActivity = time.Now()
	}()

	// Check if community detector is available
	if m.communityDetector == nil {
		return nil, errors.WrapTransient(errors.ErrMissingConfig, "QueryManager",
			"GetCommunity", "community detector not available")
	}

	// Type assert to community detector interface
	detector, ok := m.communityDetector.(communityDetectorInterface)
	if !ok {
		return nil, errors.WrapTransient(errors.ErrMissingConfig, "QueryManager",
			"GetCommunity", "community detector does not implement required interface")
	}

	// Get community
	community, err := detector.GetCommunity(ctx, communityID)
	if err != nil {
		return nil, errors.WrapTransient(err, "QueryManager", "GetCommunity",
			fmt.Sprintf("failed to get community %s", communityID))
	}

	return community, nil
}

// GetEntityCommunity retrieves the community containing a specific entity at a given level
func (m *Manager) GetEntityCommunity(ctx context.Context, entityID string, level int) (*clustering.Community, error) {
	defer func() {
		m.lastActivity = time.Now()
	}()

	// Check if community detector is available
	if m.communityDetector == nil {
		return nil, errors.WrapTransient(errors.ErrMissingConfig, "QueryManager",
			"GetEntityCommunity", "community detector not available")
	}

	// Type assert to community detector interface
	detector, ok := m.communityDetector.(communityDetectorInterface)
	if !ok {
		return nil, errors.WrapTransient(errors.ErrMissingConfig, "QueryManager",
			"GetEntityCommunity", "community detector does not implement required interface")
	}

	// Get entity's community
	community, err := detector.GetEntityCommunity(ctx, entityID, level)
	if err != nil {
		return nil, errors.WrapTransient(err, "QueryManager", "GetEntityCommunity",
			fmt.Sprintf("failed to get community for entity %s at level %d", entityID, level))
	}

	return community, nil
}

// GetCommunitiesByLevel retrieves all communities at a specific hierarchical level
func (m *Manager) GetCommunitiesByLevel(ctx context.Context, level int) ([]*clustering.Community, error) {
	defer func() {
		m.lastActivity = time.Now()
	}()

	// Check if community detector is available
	if m.communityDetector == nil {
		return nil, errors.WrapTransient(errors.ErrMissingConfig, "QueryManager",
			"GetCommunitiesByLevel", "community detector not available")
	}

	// Type assert to community detector interface
	detector, ok := m.communityDetector.(communityDetectorInterface)
	if !ok {
		return nil, errors.WrapTransient(errors.ErrMissingConfig, "QueryManager",
			"GetCommunitiesByLevel", "community detector does not implement required interface")
	}

	// Get communities by level
	communities, err := detector.GetCommunitiesByLevel(ctx, level)
	if err != nil {
		return nil, errors.WrapTransient(err, "QueryManager", "GetCommunitiesByLevel",
			fmt.Sprintf("failed to get communities at level %d", level))
	}

	return communities, nil
}
