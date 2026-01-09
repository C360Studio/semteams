// Package graphquery community cache implementation
package graphquery

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/c360/semstreams/graph/clustering"
	"github.com/nats-io/nats.go/jetstream"
)

// CommunityCache maintains an in-memory cache of communities from COMMUNITY_INDEX KV.
// It watches the KV bucket for changes and updates the cache in real-time.
// This is a consumer-owned cache - graph-query owns and manages its own view of community data.
type CommunityCache struct {
	mu sync.RWMutex

	// communities maps community ID → Community
	communities map[string]*clustering.Community

	// entityCommunity maps entityID → level → communityID for fast LocalSearch lookups
	entityCommunity map[string]map[int]string

	// byLevel maps level → communities for GlobalSearch
	byLevel map[int][]*clustering.Community

	// Lifecycle
	logger  *slog.Logger
	ready   bool
	watcher jetstream.KeyWatcher
}

// NewCommunityCache creates a new community cache.
func NewCommunityCache(logger *slog.Logger) *CommunityCache {
	return &CommunityCache{
		communities:     make(map[string]*clustering.Community),
		entityCommunity: make(map[string]map[int]string),
		byLevel:         make(map[int][]*clustering.Community),
		logger:          logger,
	}
}

// WatchAndSync starts watching the COMMUNITY_INDEX KV bucket and syncs changes to the cache.
// This method blocks until the context is cancelled.
func (c *CommunityCache) WatchAndSync(ctx context.Context, bucket jetstream.KeyValue) error {
	watcher, err := bucket.WatchAll(ctx)
	if err != nil {
		return err
	}
	c.watcher = watcher

	c.logger.Info("community cache watcher started")

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("community cache watcher stopping", "reason", "context cancelled")
			watcher.Stop()
			return ctx.Err()

		case entry := <-watcher.Updates():
			if entry == nil {
				// nil entry indicates initial state enumeration complete
				c.mu.Lock()
				c.ready = true
				c.mu.Unlock()
				c.logger.Info("community cache initial sync complete",
					"communities", len(c.communities))
				continue
			}

			if entry.Operation() == jetstream.KeyValueDelete {
				c.handleDelete(entry.Key())
				continue
			}

			c.handleUpdate(entry.Key(), entry.Value())
		}
	}
}

// handleUpdate processes a community create/update from KV watch.
func (c *CommunityCache) handleUpdate(key string, data []byte) {
	var community clustering.Community
	if err := json.Unmarshal(data, &community); err != nil {
		c.logger.Warn("failed to unmarshal community",
			"key", key,
			"error", err)
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Remove old membership mappings if this community existed
	if old, exists := c.communities[key]; exists {
		c.removeMembershipMappings(old)
	}

	// Store the community
	c.communities[key] = &community

	// Update entity→community mappings
	for _, entityID := range community.Members {
		if c.entityCommunity[entityID] == nil {
			c.entityCommunity[entityID] = make(map[int]string)
		}
		c.entityCommunity[entityID][community.Level] = community.ID
	}

	// Rebuild byLevel index for this level
	c.rebuildLevelIndex(community.Level)

	c.logger.Debug("community cache updated",
		"id", community.ID,
		"level", community.Level,
		"members", len(community.Members))
}

// handleDelete processes a community deletion from KV watch.
func (c *CommunityCache) handleDelete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	community, exists := c.communities[key]
	if !exists {
		return
	}

	// Remove membership mappings
	c.removeMembershipMappings(community)

	// Remove from communities map
	delete(c.communities, key)

	// Rebuild byLevel index for this level
	c.rebuildLevelIndex(community.Level)

	c.logger.Debug("community cache deleted", "id", key)
}

// removeMembershipMappings removes entity→community mappings for a community.
// Must be called with mu held.
func (c *CommunityCache) removeMembershipMappings(community *clustering.Community) {
	for _, entityID := range community.Members {
		if levels, exists := c.entityCommunity[entityID]; exists {
			delete(levels, community.Level)
			if len(levels) == 0 {
				delete(c.entityCommunity, entityID)
			}
		}
	}
}

// rebuildLevelIndex rebuilds the byLevel index for a specific level.
// Must be called with mu held.
func (c *CommunityCache) rebuildLevelIndex(level int) {
	communities := make([]*clustering.Community, 0)
	for _, comm := range c.communities {
		if comm.Level == level {
			communities = append(communities, comm)
		}
	}
	c.byLevel[level] = communities
}

// GetCommunity retrieves a community by ID.
// Returns nil if not found.
func (c *CommunityCache) GetCommunity(id string) *clustering.Community {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.communities[id]
}

// GetEntityCommunity retrieves the community containing an entity at a specific level.
// Returns nil if the entity is not in any community at that level.
func (c *CommunityCache) GetEntityCommunity(entityID string, level int) *clustering.Community {
	c.mu.RLock()
	defer c.mu.RUnlock()

	levels, exists := c.entityCommunity[entityID]
	if !exists {
		return nil
	}

	communityID, exists := levels[level]
	if !exists {
		return nil
	}

	return c.communities[communityID]
}

// GetCommunitiesByLevel retrieves all communities at a specific level.
// Returns empty slice if no communities exist at that level.
func (c *CommunityCache) GetCommunitiesByLevel(level int) []*clustering.Community {
	c.mu.RLock()
	defer c.mu.RUnlock()

	communities := c.byLevel[level]
	if communities == nil {
		return []*clustering.Community{}
	}

	// Return a copy to avoid race conditions
	result := make([]*clustering.Community, len(communities))
	copy(result, communities)
	return result
}

// GetAllCommunities retrieves all communities regardless of level.
// Returns empty slice if no communities exist.
func (c *CommunityCache) GetAllCommunities() []*clustering.Community {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]*clustering.Community, 0, len(c.communities))
	for _, comm := range c.communities {
		result = append(result, comm)
	}
	return result
}

// IsReady returns true if the initial sync from KV is complete.
func (c *CommunityCache) IsReady() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ready
}

// Stats returns cache statistics.
func (c *CommunityCache) Stats() CommunityStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	levelCounts := make(map[int]int)
	for level, communities := range c.byLevel {
		levelCounts[level] = len(communities)
	}

	return CommunityStats{
		TotalCommunities: len(c.communities),
		TotalEntities:    len(c.entityCommunity),
		ByLevel:          levelCounts,
		Ready:            c.ready,
	}
}

// CommunityStats provides cache statistics.
type CommunityStats struct {
	TotalCommunities int         `json:"total_communities"`
	TotalEntities    int         `json:"total_entities"`
	ByLevel          map[int]int `json:"by_level"`
	Ready            bool        `json:"ready"`
}

// Stop stops the watcher if running.
func (c *CommunityCache) Stop() {
	if c.watcher != nil {
		c.watcher.Stop()
	}
}
