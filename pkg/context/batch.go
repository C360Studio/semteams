package context

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/c360studio/semstreams/pkg/types"
)

// Relationship is an alias for the shared type
type Relationship = types.Relationship

// GraphClient defines the interface for batch graph query operations.
// This mirrors the interface in workflow actions but is defined here
// for use by context construction utilities.
type GraphClient interface {
	// QueryEntities fetches multiple entities by their IDs
	QueryEntities(ctx context.Context, entityIDs []string) (map[string]json.RawMessage, error)

	// QueryRelationships fetches relationships for an entity with depth
	QueryRelationships(ctx context.Context, entityID string, depth int) ([]Relationship, error)
}

// BatchQueryOptions configures batch query behavior
type BatchQueryOptions struct {
	IncludeRelationships bool
	Depth                int
	MaxConcurrent        int // Max concurrent queries (default: 10)
}

// BatchQueryResult contains results from a batch query
type BatchQueryResult struct {
	Entities      map[string]json.RawMessage
	Relationships []Relationship
	NotFound      []string
	Errors        map[string]error
}

// BatchQueryEntities performs batch entity lookups efficiently.
// Returns all found entities and tracks which were not found.
func BatchQueryEntities(ctx context.Context, client GraphClient, entityIDs []string) (*BatchQueryResult, error) {
	return BatchQueryEntitiesWithOptions(ctx, client, entityIDs, BatchQueryOptions{})
}

// BatchQueryEntitiesWithOptions performs batch entity lookups with options.
func BatchQueryEntitiesWithOptions(ctx context.Context, client GraphClient, entityIDs []string, opts BatchQueryOptions) (*BatchQueryResult, error) {
	if client == nil {
		return nil, fmt.Errorf("graph client is required")
	}

	if len(entityIDs) == 0 {
		return &BatchQueryResult{
			Entities: make(map[string]json.RawMessage),
		}, nil
	}

	// Check for context cancellation before starting
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled before query: %w", err)
	}

	result := &BatchQueryResult{
		Entities: make(map[string]json.RawMessage),
		Errors:   make(map[string]error),
	}

	// Query entities
	entities, err := client.QueryEntities(ctx, entityIDs)
	if err != nil {
		return nil, fmt.Errorf("batch query failed: %w", err)
	}

	result.Entities = entities

	// Track not found
	for _, id := range entityIDs {
		if _, found := entities[id]; !found {
			result.NotFound = append(result.NotFound, id)
		}
	}

	// Query relationships if requested
	if opts.IncludeRelationships && opts.Depth > 0 {
		maxConcurrent := opts.MaxConcurrent
		if maxConcurrent <= 0 {
			maxConcurrent = 10
		}

		sem := make(chan struct{}, maxConcurrent)
		var mu sync.Mutex
		var wg sync.WaitGroup

		for _, entityID := range entityIDs {
			if _, found := entities[entityID]; !found {
				continue
			}

			// Check for context cancellation before spawning goroutine
			if err := ctx.Err(); err != nil {
				break
			}

			wg.Add(1)
			go func(id string) {
				defer wg.Done()

				// Check for context cancellation before acquiring semaphore
				select {
				case <-ctx.Done():
					mu.Lock()
					result.Errors[id] = ctx.Err()
					mu.Unlock()
					return
				case sem <- struct{}{}:
					defer func() { <-sem }()
				}

				// Check again after acquiring semaphore
				if ctx.Err() != nil {
					mu.Lock()
					result.Errors[id] = ctx.Err()
					mu.Unlock()
					return
				}

				rels, err := client.QueryRelationships(ctx, id, opts.Depth)
				if err != nil {
					mu.Lock()
					result.Errors[id] = err
					mu.Unlock()
					return
				}

				mu.Lock()
				result.Relationships = append(result.Relationships, rels...)
				mu.Unlock()
			}(entityID)
		}

		wg.Wait()
	}

	return result, nil
}

// CollectEntityIDs extracts unique entity IDs from relationships
func CollectEntityIDs(relationships []Relationship) []string {
	seen := make(map[string]bool)
	var ids []string

	for _, rel := range relationships {
		if !seen[rel.Subject] {
			seen[rel.Subject] = true
			ids = append(ids, rel.Subject)
		}
		if !seen[rel.Object] {
			seen[rel.Object] = true
			ids = append(ids, rel.Object)
		}
	}

	return ids
}

// ExpandWithNeighbors expands entity IDs to include their neighbors
func ExpandWithNeighbors(ctx context.Context, client GraphClient, entityIDs []string, depth int) ([]string, error) {
	if depth <= 0 {
		return entityIDs, nil
	}

	// Check for context cancellation
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	seen := make(map[string]bool)
	for _, id := range entityIDs {
		seen[id] = true
	}

	current := entityIDs
	for d := 0; d < depth; d++ {
		// Check for context cancellation each depth level
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("context cancelled at depth %d: %w", d, err)
		}

		var nextLevel []string

		for _, id := range current {
			rels, err := client.QueryRelationships(ctx, id, 1)
			if err != nil {
				continue
			}

			for _, rel := range rels {
				if !seen[rel.Subject] {
					seen[rel.Subject] = true
					nextLevel = append(nextLevel, rel.Subject)
				}
				if !seen[rel.Object] {
					seen[rel.Object] = true
					nextLevel = append(nextLevel, rel.Object)
				}
			}
		}

		current = nextLevel
	}

	// Collect all seen IDs
	result := make([]string, 0, len(seen))
	for id := range seen {
		result = append(result, id)
	}

	return result, nil
}
