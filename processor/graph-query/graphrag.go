// Package graphquery GraphRAG search handlers
package graphquery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	gtypes "github.com/c360/semstreams/graph"
	"github.com/c360/semstreams/graph/clustering"
	"github.com/c360/semstreams/message"
)

// GraphRAG search constants
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

// LocalSearchRequest is the request format for local search
type LocalSearchRequest struct {
	EntityID string `json:"entity_id"`
	Query    string `json:"query"`
	Level    int    `json:"level"`
}

// LocalSearchResponse is the response format for local search
type LocalSearchResponse struct {
	Entities    []*gtypes.EntityState `json:"entities"`
	CommunityID string                `json:"community_id"`
	Count       int                   `json:"count"`
	DurationMs  int64                 `json:"duration_ms"`
}

// GlobalSearchRequest is the request format for global search
type GlobalSearchRequest struct {
	Query          string `json:"query"`
	Level          int    `json:"level"`
	MaxCommunities int    `json:"max_communities"`
}

// GlobalSearchResponse is the response format for global search
type GlobalSearchResponse struct {
	Entities           []*gtypes.EntityState `json:"entities"`
	CommunitySummaries []CommunitySummary    `json:"community_summaries"`
	Count              int                   `json:"count"`
	DurationMs         int64                 `json:"duration_ms"`
	Answer             string                `json:"answer,omitempty"`
	AnswerModel        string                `json:"answer_model,omitempty"`
}

// CommunitySummary represents a community's summary used in global search
type CommunitySummary struct {
	CommunityID string   `json:"community_id"`
	Summary     string   `json:"summary"`
	Keywords    []string `json:"keywords"`
	Level       int      `json:"level"`
	Relevance   float64  `json:"relevance"`
}

// setupGraphRAGHandlers registers the GraphRAG NATS request handlers
func (c *Component) setupGraphRAGHandlers() error {
	// Subscribe to local search
	if err := c.natsClient.SubscribeForRequests(c.ctx, "graph.query.localSearch", c.handleLocalSearch); err != nil {
		return fmt.Errorf("subscribe to localSearch: %w", err)
	}

	// Subscribe to global search
	if err := c.natsClient.SubscribeForRequests(c.ctx, "graph.query.globalSearch", c.handleGlobalSearch); err != nil {
		return fmt.Errorf("subscribe to globalSearch: %w", err)
	}

	c.logger.Info("GraphRAG handlers registered",
		"subjects", []string{"graph.query.localSearch", "graph.query.globalSearch"})

	return nil
}

// handleLocalSearch handles local search requests via NATS request/reply
func (c *Component) handleLocalSearch(ctx context.Context, data []byte) ([]byte, error) {
	startTime := time.Now()

	// Parse request
	var req LocalSearchRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	// Validate request
	if req.EntityID == "" {
		return nil, fmt.Errorf("invalid request: empty entity_id")
	}
	if req.Query == "" {
		return nil, fmt.Errorf("invalid request: empty query")
	}

	// Check if community cache is available
	if c.communityCache == nil {
		return nil, fmt.Errorf("community cache not available")
	}

	// Find the entity's community from cache
	community := c.communityCache.GetEntityCommunity(req.EntityID, req.Level)
	if community == nil {
		return nil, fmt.Errorf("entity %s not in any community at level %d", req.EntityID, req.Level)
	}

	// Load entities from community via graph-ingest
	entities, err := c.loadEntities(ctx, community.Members)
	if err != nil {
		return nil, fmt.Errorf("load entities: %w", err)
	}

	// Filter entities based on query
	matchedEntities := filterEntitiesByQuery(entities, req.Query)

	// Build response
	response := LocalSearchResponse{
		Entities:    matchedEntities,
		CommunityID: community.ID,
		Count:       len(matchedEntities),
		DurationMs:  time.Since(startTime).Milliseconds(),
	}

	c.recordSuccess(len(data), 0)
	return json.Marshal(response)
}

// handleGlobalSearch handles global search requests via NATS request/reply
func (c *Component) handleGlobalSearch(ctx context.Context, data []byte) ([]byte, error) {
	startTime := time.Now()

	// Parse request
	var req GlobalSearchRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	// Validate request
	if req.Query == "" {
		return nil, fmt.Errorf("invalid request: empty query")
	}

	// Apply defaults
	if req.MaxCommunities <= 0 {
		req.MaxCommunities = DefaultMaxCommunities
	}

	// Check if community cache is available
	if c.communityCache == nil {
		return nil, fmt.Errorf("community cache not available")
	}

	// Get all communities at the specified level from cache
	communities := c.communityCache.GetCommunitiesByLevel(req.Level)
	if len(communities) == 0 {
		response := GlobalSearchResponse{
			Entities:           []*gtypes.EntityState{},
			CommunitySummaries: []CommunitySummary{},
			Count:              0,
			DurationMs:         time.Since(startTime).Milliseconds(),
		}
		return json.Marshal(response)
	}

	// Score communities based on their summaries
	scoredCommunities := scoreCommunitySummaries(communities, req.Query)

	// Select top-N communities
	selectedCount := req.MaxCommunities
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

	// Enforce resource limit
	if len(entityIDs) > MaxTotalEntitiesInSearch {
		entityIDs = entityIDs[:MaxTotalEntitiesInSearch]
	}

	// Load entities via graph-ingest
	entities, err := c.loadEntities(ctx, entityIDs)
	if err != nil {
		return nil, fmt.Errorf("load entities: %w", err)
	}

	// Filter entities based on query
	matchedEntities := filterEntitiesByQuery(entities, req.Query)

	// Build community summaries for response
	summaries := make([]CommunitySummary, len(topCommunities))
	for i, comm := range topCommunities {
		// Calculate relevance from position
		relevance := 1.0 - (float64(i) / float64(len(scoredCommunities)))

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

	// Build response
	response := GlobalSearchResponse{
		Entities:           matchedEntities,
		CommunitySummaries: summaries,
		Count:              len(matchedEntities),
		DurationMs:         time.Since(startTime).Milliseconds(),
	}

	c.recordSuccess(len(data), 0)
	return json.Marshal(response)
}

// loadEntities loads entities by ID via graph-ingest request/reply
func (c *Component) loadEntities(ctx context.Context, entityIDs []string) ([]*gtypes.EntityState, error) {
	if len(entityIDs) == 0 {
		return []*gtypes.EntityState{}, nil
	}

	// Build batch request
	reqData, err := json.Marshal(map[string]any{
		"ids": entityIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// Request entities from graph-ingest
	subject := c.router.Route("entityBatch")
	if subject == "" {
		return nil, errors.New("entityBatch query routing not available")
	}
	respData, err := c.natsClient.Request(ctx, subject, reqData, c.config.QueryTimeout)
	if err != nil {
		return nil, fmt.Errorf("request entities: %w", err)
	}

	// Parse response
	var resp struct {
		Entities []*gtypes.EntityState `json:"entities"`
	}
	if err := json.Unmarshal(respData, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return resp.Entities, nil
}

// filterEntitiesByQuery filters entities based on simple text matching
func filterEntitiesByQuery(entities []*gtypes.EntityState, query string) []*gtypes.EntityState {
	query = strings.ToLower(query)
	queryTerms := strings.Fields(query)

	if len(queryTerms) == 0 {
		return entities // No filtering if query is empty
	}

	matched := make([]*gtypes.EntityState, 0)

	for _, entity := range entities {
		if entityMatchesQuery(entity, queryTerms) {
			matched = append(matched, entity)
		}
	}

	return matched
}

// entityMatchesQuery checks if an entity matches the query terms
func entityMatchesQuery(entity *gtypes.EntityState, queryTerms []string) bool {
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

	// Check if any query term matches
	for _, term := range queryTerms {
		if strings.Contains(searchableText, strings.ToLower(term)) {
			return true
		}
	}

	return false
}

// scoreCommunitySummaries scores communities based on query relevance
// Returns communities sorted by relevance (highest first)
func scoreCommunitySummaries(communities []*clustering.Community, query string) []*clustering.Community {
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

	// Sort by score descending
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// Extract sorted communities
	result := make([]*clustering.Community, len(scored))
	for i, sc := range scored {
		result[i] = sc.community
	}

	return result
}
