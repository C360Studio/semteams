// Package graphquery PathRAG algorithm implementation
package graphquery

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/c360/semstreams/graph"
)

// PathSearchRequest defines the request schema for path search queries
type PathSearchRequest struct {
	StartEntity     string `json:"start_entity"`
	MaxDepth        int    `json:"max_depth"`
	MaxNodes        int    `json:"max_nodes"`
	IncludeSiblings bool   `json:"include_siblings"`
}

// PathSearchResponse defines the response for path search
type PathSearchResponse struct {
	Entities  []PathEntity `json:"entities"`
	Paths     [][]PathStep `json:"paths"` // Each path is a sequence of steps from start to entity
	Truncated bool         `json:"truncated"`
}

// PathEntity represents a discovered entity with relevance score
type PathEntity struct {
	ID    string  `json:"id"`
	Type  string  `json:"type"`
	Score float64 `json:"score"`
}

// PathStep represents a single edge traversal
type PathStep struct {
	From      string `json:"from"`
	Predicate string `json:"predicate"`
	To        string `json:"to"`
}

// parentInfo tracks how we reached an entity during BFS
type parentInfo struct {
	parentID  string
	predicate string
}

// RelationshipEntry represents an outgoing relationship from graph-index
type RelationshipEntry struct {
	ToEntityID string `json:"to_entity_id"`
	Predicate  string `json:"predicate"`
}

// PathSearcher executes PathRAG traversal with proper path tracking
type PathSearcher struct {
	nats        natsRequester
	timeout     time.Duration
	maxDepth    int
	decayFactor float64
	logger      *slog.Logger
}

// NewPathSearcher creates a new PathSearcher instance
func NewPathSearcher(nats natsRequester, timeout time.Duration, maxDepth int, logger *slog.Logger) *PathSearcher {
	return &PathSearcher{
		nats:        nats,
		timeout:     timeout,
		maxDepth:    maxDepth,
		decayFactor: 0.8, // Score decreases by 20% per hop
		logger:      logger,
	}
}

// Search performs BFS traversal with path tracking
func (p *PathSearcher) Search(ctx context.Context, req PathSearchRequest) (*PathSearchResponse, error) {
	if req.StartEntity == "" {
		return nil, fmt.Errorf("invalid request: empty start_entity")
	}

	// Apply max depth limit
	maxDepth := req.MaxDepth
	if maxDepth == 0 || maxDepth > p.maxDepth {
		maxDepth = p.maxDepth
	}

	// Apply max nodes limit
	maxNodes := req.MaxNodes
	if maxNodes == 0 {
		maxNodes = 100 // Default limit
	}

	// Verify start entity exists
	entityReq := map[string]string{"id": req.StartEntity}
	entityReqData, _ := json.Marshal(entityReq)
	_, err := p.nats.Request(ctx, "graph.ingest.query.entity", entityReqData, p.timeout)
	if err != nil {
		return nil, fmt.Errorf("entity not found: %w", err)
	}

	// BFS with parent tracking for path reconstruction
	type queueItem struct {
		entityID string
		depth    int
	}

	visited := make(map[string]bool)
	parentMap := make(map[string]parentInfo) // Track how we reached each entity
	visited[req.StartEntity] = true

	queue := []queueItem{{entityID: req.StartEntity, depth: 0}}

	// Track entities with their scores (decay by depth)
	entities := []PathEntity{{
		ID:    req.StartEntity,
		Type:  "entity",
		Score: 1.0,
	}}

	paths := make([][]PathStep, 0)
	// Start entity has no path (empty path)
	paths = append(paths, []PathStep{})

	nodesDiscovered := 0

	for len(queue) > 0 && nodesDiscovered < maxNodes {
		// Check context cancellation
		if ctx.Err() != nil {
			return nil, fmt.Errorf("context cancelled: %w", ctx.Err())
		}

		current := queue[0]
		queue = queue[1:]

		// Stop if we've reached max depth
		if current.depth >= maxDepth {
			continue
		}

		// Get outgoing relationships from graph-index
		relsReq := map[string]string{"entity_id": current.entityID}
		relsReqData, _ := json.Marshal(relsReq)

		relsResponse, err := p.nats.Request(ctx, "graph.index.query.outgoing", relsReqData, p.timeout)
		if err != nil {
			// Skip unreachable nodes
			continue
		}

		// Parse relationships from envelope response
		var envelope graph.OutgoingQueryResponse
		if err := json.Unmarshal(relsResponse, &envelope); err != nil {
			continue
		}
		if envelope.Error != nil {
			continue
		}

		// Convert to local type
		rels := make([]RelationshipEntry, len(envelope.Data.Relationships))
		for i, r := range envelope.Data.Relationships {
			rels[i] = RelationshipEntry{
				ToEntityID: r.ToEntityID,
				Predicate:  r.Predicate,
			}
		}

		// Add neighbors to queue and entities
		for _, rel := range rels {
			if nodesDiscovered >= maxNodes {
				break
			}

			if !visited[rel.ToEntityID] {
				visited[rel.ToEntityID] = true
				nodesDiscovered++

				// Track parent for path reconstruction
				parentMap[rel.ToEntityID] = parentInfo{
					parentID:  current.entityID,
					predicate: rel.Predicate,
				}

				// Calculate score based on depth (decay)
				newDepth := current.depth + 1
				score := 1.0
				for i := 0; i < newDepth; i++ {
					score *= p.decayFactor
				}

				entities = append(entities, PathEntity{
					ID:    rel.ToEntityID,
					Type:  "entity",
					Score: score,
				})

				// Reconstruct full path from start to this entity
				path := p.reconstructPath(rel.ToEntityID, req.StartEntity, parentMap)
				paths = append(paths, path)

				// Add to queue for further traversal
				queue = append(queue, queueItem{entityID: rel.ToEntityID, depth: newDepth})
			}
		}
	}

	return &PathSearchResponse{
		Entities:  entities,
		Paths:     paths,
		Truncated: nodesDiscovered >= maxNodes,
	}, nil
}

// reconstructPath builds the full path from start to target entity
func (p *PathSearcher) reconstructPath(target, start string, parentMap map[string]parentInfo) []PathStep {
	if target == start {
		return []PathStep{} // Start entity has empty path
	}

	// Walk backwards from target to start
	path := []PathStep{}
	current := target

	for current != start {
		info, ok := parentMap[current]
		if !ok {
			// Should not happen if BFS is correct
			p.logger.Warn("missing parent info during path reconstruction", "entity", current)
			break
		}

		path = append(path, PathStep{
			From:      info.parentID,
			Predicate: info.predicate,
			To:        current,
		})

		current = info.parentID
	}

	// Reverse path to get start → target order
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}

	return path
}
