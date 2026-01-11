// Package graphclustering query handlers
package graphclustering

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/c360/semstreams/graph/clustering"
	"github.com/nats-io/nats.go/jetstream"
)

// setupQueryHandlers sets up NATS request/reply subscriptions for query handlers
func (c *Component) setupQueryHandlers(ctx context.Context) error {
	// Subscribe to community query
	if err := c.natsClient.SubscribeForRequests(ctx, "graph.clustering.query.community", c.handleQueryCommunityNATS); err != nil {
		return fmt.Errorf("subscribe community query: %w", err)
	}

	// Subscribe to members query
	if err := c.natsClient.SubscribeForRequests(ctx, "graph.clustering.query.members", c.handleQueryMembersNATS); err != nil {
		return fmt.Errorf("subscribe members query: %w", err)
	}

	// Subscribe to entity community query
	if err := c.natsClient.SubscribeForRequests(ctx, "graph.clustering.query.entity", c.handleQueryEntityNATS); err != nil {
		return fmt.Errorf("subscribe entity query: %w", err)
	}

	// Subscribe to level query
	if err := c.natsClient.SubscribeForRequests(ctx, "graph.clustering.query.level", c.handleQueryLevelNATS); err != nil {
		return fmt.Errorf("subscribe level query: %w", err)
	}

	c.logger.Info("query handlers registered",
		"subjects", []string{
			"graph.clustering.query.community",
			"graph.clustering.query.members",
			"graph.clustering.query.entity",
			"graph.clustering.query.level",
		})

	return nil
}

// CommunityRequest is the request format for community query
type CommunityRequest struct {
	ID string `json:"id"`
}

// CommunityResponse is the response format for community query
type CommunityResponse struct {
	Community *clustering.Community `json:"community"`
}

// MembersRequest is the request format for members query
type MembersRequest struct {
	CommunityID string `json:"community_id"`
}

// MembersResponse is the response format for members query
type MembersResponse struct {
	CommunityID string   `json:"community_id"`
	Members     []string `json:"members"`
	Count       int      `json:"count"`
}

// EntityRequest is the request format for entity community query
type EntityRequest struct {
	EntityID string `json:"entity_id"`
	Level    int    `json:"level"`
}

// EntityResponse is the response format for entity community query
type EntityResponse struct {
	EntityID  string                `json:"entity_id"`
	Level     int                   `json:"level"`
	Community *clustering.Community `json:"community"`
}

// LevelRequest is the request format for level query
type LevelRequest struct {
	Level int `json:"level"`
}

// LevelResponse is the response format for level query
type LevelResponse struct {
	Level       int                     `json:"level"`
	Communities []*clustering.Community `json:"communities"`
	Count       int                     `json:"count"`
}

// handleQueryCommunityNATS handles community query requests via NATS request/reply
func (c *Component) handleQueryCommunityNATS(_ context.Context, data []byte) ([]byte, error) {
	// Create context with timeout for KV operations
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Parse request
	var req CommunityRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	// Validate request
	if req.ID == "" {
		return nil, fmt.Errorf("invalid request: empty id")
	}

	// Get community from bucket
	community, err := c.getCommunity(ctx, req.ID)
	if err != nil {
		return nil, err
	}

	response := CommunityResponse{
		Community: community,
	}

	return json.Marshal(response)
}

// handleQueryMembersNATS handles members query requests via NATS request/reply
func (c *Component) handleQueryMembersNATS(_ context.Context, data []byte) ([]byte, error) {
	// Create context with timeout for KV operations
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Parse request
	var req MembersRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	// Validate request
	if req.CommunityID == "" {
		return nil, fmt.Errorf("invalid request: empty community_id")
	}

	// Get community
	community, err := c.getCommunity(ctx, req.CommunityID)
	if err != nil {
		return nil, err
	}

	response := MembersResponse{
		CommunityID: req.CommunityID,
		Members:     community.Members,
		Count:       len(community.Members),
	}

	return json.Marshal(response)
}

// handleQueryEntityNATS handles entity community query requests via NATS request/reply
func (c *Component) handleQueryEntityNATS(_ context.Context, data []byte) ([]byte, error) {
	// Create context with timeout for KV operations
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Parse request
	var req EntityRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	// Validate request
	if req.EntityID == "" {
		return nil, fmt.Errorf("invalid request: empty entity_id")
	}

	// Apply defaults
	level := req.Level
	if level < 0 {
		level = 0
	}

	// Get entity community by scanning communities at level
	community, err := c.getEntityCommunity(ctx, req.EntityID, level)
	if err != nil {
		return nil, err
	}

	response := EntityResponse{
		EntityID:  req.EntityID,
		Level:     level,
		Community: community,
	}

	return json.Marshal(response)
}

// handleQueryLevelNATS handles level query requests via NATS request/reply
func (c *Component) handleQueryLevelNATS(_ context.Context, data []byte) ([]byte, error) {
	// Create context with timeout for KV operations
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Parse request
	var req LevelRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	// Apply defaults
	level := req.Level
	if level < 0 {
		level = 0
	}

	// Get communities at level
	communities, err := c.getCommunitiesByLevel(ctx, level)
	if err != nil {
		return nil, err
	}

	response := LevelResponse{
		Level:       level,
		Communities: communities,
		Count:       len(communities),
	}

	return json.Marshal(response)
}

// getCommunity retrieves a community by ID from the KV bucket
func (c *Component) getCommunity(ctx context.Context, id string) (*clustering.Community, error) {
	if c.communityBucket == nil {
		return nil, fmt.Errorf("community bucket not initialized")
	}

	entry, err := c.communityBucket.Get(ctx, id)
	if err != nil {
		if err == jetstream.ErrKeyNotFound {
			return nil, fmt.Errorf("not found: %s", id)
		}
		return nil, fmt.Errorf("get community: %w", err)
	}

	var community clustering.Community
	if err := json.Unmarshal(entry.Value(), &community); err != nil {
		return nil, fmt.Errorf("unmarshal community: %w", err)
	}

	return &community, nil
}

// getEntityCommunity finds the community containing the given entity at the specified level
// Uses the indexed entity mapping (entity.{level}.{entity_id} -> community_id) for O(1) lookup
func (c *Component) getEntityCommunity(ctx context.Context, entityID string, level int) (*clustering.Community, error) {
	if c.communityBucket == nil {
		return nil, fmt.Errorf("community bucket not initialized")
	}

	// Use indexed entity -> community mapping for O(1) lookup
	entityKey := fmt.Sprintf("entity.%d.%s", level, entityID)
	entry, err := c.communityBucket.Get(ctx, entityKey)
	if err != nil {
		if err == jetstream.ErrKeyNotFound {
			return nil, nil // Entity not in any community at this level
		}
		return nil, fmt.Errorf("get entity mapping: %w", err)
	}

	communityID := string(entry.Value())

	// Fetch the community data
	communityKey := fmt.Sprintf("%d.%s", level, communityID)
	communityEntry, err := c.communityBucket.Get(ctx, communityKey)
	if err != nil {
		if err == jetstream.ErrKeyNotFound {
			return nil, nil // Mapping exists but community was deleted
		}
		return nil, fmt.Errorf("get community: %w", err)
	}

	var community clustering.Community
	if err := json.Unmarshal(communityEntry.Value(), &community); err != nil {
		return nil, fmt.Errorf("unmarshal community: %w", err)
	}

	return &community, nil
}

// getCommunitiesByLevel retrieves all communities at a given level
func (c *Component) getCommunitiesByLevel(ctx context.Context, level int) ([]*clustering.Community, error) {
	if c.communityBucket == nil {
		return nil, fmt.Errorf("community bucket not initialized")
	}

	// List all keys and filter by level
	keys, err := c.communityBucket.Keys(ctx)
	if err != nil {
		return nil, fmt.Errorf("list keys: %w", err)
	}

	var communities []*clustering.Community

	for _, key := range keys {
		entry, err := c.communityBucket.Get(ctx, key)
		if err != nil {
			continue
		}

		var community clustering.Community
		if err := json.Unmarshal(entry.Value(), &community); err != nil {
			continue
		}

		if community.Level == level {
			communities = append(communities, &community)
		}
	}

	return communities, nil
}
