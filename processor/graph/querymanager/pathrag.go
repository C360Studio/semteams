// Package querymanager provides PathRAG (Path-based Retrieval Augmented Generation) queries.
// This file implements graph traversal with bounded depth, direction support, and decay factors.
package querymanager

import (
	"context"
	"fmt"
	"math"
	"time"

	gtypes "github.com/c360/semstreams/graph"
	"github.com/c360/semstreams/message"
	"github.com/c360/semstreams/pkg/errs"
)

// ExecutePath executes a graph path traversal starting from a given entity.
// It supports bidirectional traversal, edge/node type filtering, and relevance decay.
func (qe *Manager) ExecutePath(ctx context.Context, start string, pattern PathPattern) (*QueryResult, error) {
	startTime := time.Now()
	defer qe.recordActivity()

	// Apply timeout from pattern or config
	pathCtx := ctx
	timeout := pattern.MaxTime
	if timeout == 0 {
		timeout = qe.config.Query.PathTimeout
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		pathCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// Validate pattern
	if err := qe.validatePathPattern(pattern); err != nil {
		return nil, errs.WrapInvalid(err, "QueryManager", "ExecutePath",
			"pattern validation failed")
	}

	// Execute path traversal
	result, err := qe.executePathTraversal(pathCtx, start, pattern)
	if err != nil {
		qe.recordError("ExecutePath", err)
		if qe.metrics != nil {
			qe.metrics.RecordQuery("query_engine", "path", time.Since(startTime), 0, false)
		}
		return nil, err
	}

	result.Duration = time.Since(startTime)
	result.Cached = false

	// Record metrics
	if qe.metrics != nil {
		qe.metrics.RecordQuery("query_engine", "path", time.Since(startTime), len(result.Entities), true)
		if len(result.Paths) > 0 {
			qe.metrics.pathExecutionTotal.Inc()
			qe.metrics.pathExecutionDuration.Observe(time.Since(startTime).Seconds())
			for _, path := range result.Paths {
				qe.metrics.pathLengthHistogram.Observe(float64(path.Length))
			}
		}
	}

	return result, nil
}

// pathTraversalState tracks state during bounded graph traversal
type pathTraversalState struct {
	visited          map[string]bool
	entities         map[string]*gtypes.EntityState
	pathTo           map[string][]string    // Full path to each node
	edgesTo          map[string][]GraphEdge // Edges along path to each node
	scores           map[string]float64
	nodesVisited     int
	truncated        bool
	truncationReason string // "timeout", "max_nodes", "cancelled"
}

// executePathTraversal performs bounded graph traversal with full PathPattern support.
// It uses DFS with cycle detection and respects all resource limits.
func (qe *Manager) executePathTraversal(ctx context.Context, start string, pattern PathPattern) (*QueryResult, error) {
	// Initialize traversal state
	state := &pathTraversalState{
		visited:  make(map[string]bool),
		entities: make(map[string]*gtypes.EntityState),
		pathTo:   make(map[string][]string),
		edgesTo:  make(map[string][]GraphEdge),
		scores:   make(map[string]float64),
	}

	// Get start entity
	startEntity, err := qe.GetEntity(ctx, start)
	if err != nil {
		return nil, errs.WrapTransient(err, "QueryManager", "ExecutePath",
			fmt.Sprintf("failed to get start entity: %s", start))
	}
	if startEntity == nil {
		return nil, errs.WrapInvalid(fmt.Errorf("entity not found: %s", start),
			"QueryManager", "ExecutePath", "start entity not found")
	}

	// Initialize with start entity - use entity.ID as key for consistency
	// This ensures the map key matches entity.ID for correct score lookup in resolver
	state.visited[startEntity.ID] = true
	state.entities[startEntity.ID] = startEntity
	state.pathTo[startEntity.ID] = []string{startEntity.ID}
	state.edgesTo[startEntity.ID] = []GraphEdge{}
	state.scores[startEntity.ID] = 1.0
	state.nodesVisited = 1

	// Perform traversal (DFS respecting MaxDepth)
	if err := qe.traverseGraph(ctx, pattern, state, startEntity.ID, 0); err != nil {
		// Context cancellation is not an error, just truncation
		if err == context.DeadlineExceeded {
			state.truncated = true
			state.truncationReason = TruncationReasonTimeout
		} else if err == context.Canceled {
			state.truncated = true
			state.truncationReason = TruncationReasonCancelled
		} else {
			return nil, err
		}
	}

	// Build result with truncation info
	result := qe.buildPathResult(state, pattern)
	result.Truncated = state.truncated
	result.TruncationReason = state.truncationReason
	return result, nil
}

// traverseGraph performs DFS traversal with Direction support
func (qe *Manager) traverseGraph(ctx context.Context, pattern PathPattern, state *pathTraversalState, current string, depth int) error {
	// Check context cancellation
	select {
	case <-ctx.Done():
		state.truncated = true
		if ctx.Err() == context.DeadlineExceeded {
			state.truncationReason = TruncationReasonTimeout
		} else {
			state.truncationReason = TruncationReasonCancelled
		}
		return ctx.Err()
	default:
	}

	// Check depth limit
	if depth >= pattern.MaxDepth {
		return nil
	}

	// Check node limit
	if pattern.MaxNodes > 0 && state.nodesVisited >= pattern.MaxNodes {
		state.truncated = true
		state.truncationReason = TruncationReasonMaxNodes
		return nil
	}

	// Get explicit relationships based on Direction
	relationships, err := qe.getRelationshipsForTraversal(ctx, current, pattern.Direction)
	if err != nil {
		// Log error but continue - don't fail entire traversal for one node
		if qe.logger != nil {
			qe.logger.Debug("failed to get explicit relationships", "entity", current, "error", err)
		}
	}

	// Add inferred sibling relationships if enabled
	if pattern.IncludeSiblings {
		siblings, err := qe.getSiblingRelationships(ctx, current)
		if err != nil {
			if qe.logger != nil {
				qe.logger.Debug("failed to get sibling relationships", "entity", current, "error", err)
			}
		} else {
			relationships = append(relationships, siblings...)
		}
	}

	for _, rel := range relationships {
		if err := qe.processNeighbor(ctx, pattern, state, current, rel, depth); err != nil {
			return err
		}
	}

	return nil
}

// processNeighbor handles visiting a single neighbor during traversal
func (qe *Manager) processNeighbor(ctx context.Context, pattern PathPattern, state *pathTraversalState, current string, rel *Relationship, depth int) error {
	// Apply EdgeTypes filter
	if !qe.matchesEdgeTypes(rel.EdgeType, pattern.EdgeTypes) {
		return nil
	}

	// Determine target based on relationship direction
	target := rel.ToEntityID
	if target == current {
		target = rel.FromEntityID
	}

	// Skip if already visited (cycle detection)
	if state.visited[target] {
		return nil
	}

	// Apply NodeTypes filter
	if !qe.matchesNodeTypes(target, pattern.NodeTypes) {
		return nil
	}

	// Check node limit before adding
	if pattern.MaxNodes > 0 && state.nodesVisited >= pattern.MaxNodes {
		state.truncated = true
		state.truncationReason = TruncationReasonMaxNodes
		return nil
	}

	// Load target entity
	targetEntity, err := qe.GetEntity(ctx, target)
	if err != nil || targetEntity == nil {
		return nil // Skip entities that can't be loaded
	}

	// Add to state
	state.visited[target] = true
	state.entities[target] = targetEntity
	state.nodesVisited++

	// Update path and edge state
	qe.updateTraversalState(state, current, target, rel, pattern, depth)

	// Recurse
	return qe.traverseGraph(ctx, pattern, state, target, depth+1)
}

// updateTraversalState updates path and edge tracking for a visited node
func (qe *Manager) updateTraversalState(state *pathTraversalState, current, target string, rel *Relationship, pattern PathPattern, depth int) {
	// Build path - copy parent path and append target
	parentPath := state.pathTo[current]
	newPath := make([]string, len(parentPath)+1)
	copy(newPath, parentPath)
	newPath[len(parentPath)] = target
	state.pathTo[target] = newPath

	// Calculate decayed weight
	weight := rel.Weight
	if weight == 0 {
		weight = 1.0
	}
	decayFactor := pattern.DecayFactor
	if decayFactor == 0 {
		decayFactor = 1.0 // No decay if not specified
	}
	if decayFactor > 0 && decayFactor < 1.0 {
		weight *= math.Pow(decayFactor, float64(depth+1))
	}

	// Build edge
	edge := GraphEdge{
		From:     rel.FromEntityID,
		To:       rel.ToEntityID,
		EdgeType: rel.EdgeType,
		Weight:   weight,
	}

	// Copy parent edges and append new edge
	parentEdges := state.edgesTo[current]
	newEdges := make([]GraphEdge, len(parentEdges)+1)
	copy(newEdges, parentEdges)
	newEdges[len(parentEdges)] = edge
	state.edgesTo[target] = newEdges

	// Calculate score with decay
	state.scores[target] = state.scores[current] * decayFactor
}

// getRelationshipsForTraversal gets relationships based on Direction.
// Returns error only if ALL queries fail. Partial results are returned with logging
// if some queries succeed while others fail.
func (qe *Manager) getRelationshipsForTraversal(ctx context.Context, entityID string, direction Direction) ([]*Relationship, error) {
	var relationships []*Relationship
	var queryErrors []error
	successCount := 0

	// Default to outgoing if no direction specified
	if direction == "" {
		direction = DirectionOutgoing
	}

	if direction == DirectionOutgoing || direction == DirectionBoth {
		outgoing, err := qe.queryOutgoingRelationships(ctx, entityID)
		if err != nil {
			queryErrors = append(queryErrors, fmt.Errorf("outgoing: %w", err))
			if qe.logger != nil {
				qe.logger.Warn("failed to get outgoing relationships",
					"entity", entityID, "error", err)
			}
		} else {
			successCount++
			relationships = append(relationships, outgoing...)
		}
	}

	if direction == DirectionIncoming || direction == DirectionBoth {
		incoming, err := qe.queryIncomingRelationships(ctx, entityID)
		if err != nil {
			queryErrors = append(queryErrors, fmt.Errorf("incoming: %w", err))
			if qe.logger != nil {
				qe.logger.Warn("failed to get incoming relationships",
					"entity", entityID, "error", err)
			}
		} else {
			successCount++
			relationships = append(relationships, incoming...)
		}
	}

	// If we have errors but also got some relationships, log partial success
	if len(queryErrors) > 0 && successCount > 0 {
		if qe.logger != nil {
			qe.logger.Info("partial relationship query success",
				"entity", entityID,
				"successes", successCount,
				"failures", len(queryErrors),
				"relationships_found", len(relationships))
		}
	}

	// Return error only if ALL queries failed
	if successCount == 0 && len(queryErrors) > 0 {
		return nil, fmt.Errorf("all relationship queries failed for entity %s: %v", entityID, queryErrors)
	}

	return relationships, nil
}

// getSiblingRelationships returns inferred sibling relationships based on EntityID hierarchy.
// Siblings are entities that share the same type-level prefix (org.platform.domain.system.type).
// These relationships are inferred at query time, not stored explicitly.
func (qe *Manager) getSiblingRelationships(ctx context.Context, entityID string) ([]*Relationship, error) {
	// Parse the entity ID to get the type prefix
	parsed, err := message.ParseEntityID(entityID)
	if err != nil {
		// Not a valid 6-part EntityID, can't infer siblings
		if qe.logger != nil {
			qe.logger.Debug("cannot infer siblings for non-standard EntityID",
				"entity", entityID, "error", err)
		}
		return nil, nil
	}

	// Get the type prefix (5-part: org.platform.domain.system.type)
	typePrefix := parsed.TypePrefix()

	// Find all entities with the same type prefix
	siblingIDs, err := qe.entityReader.ListWithPrefix(ctx, typePrefix)
	if err != nil {
		if qe.logger != nil {
			qe.logger.Warn("failed to list sibling entities",
				"entity", entityID, "prefix", typePrefix, "error", err)
		}
		return nil, err
	}

	// Build sibling relationships
	var siblings []*Relationship
	for _, siblingID := range siblingIDs {
		// Skip self
		if siblingID == entityID {
			continue
		}

		// Create an inferred sibling relationship
		siblings = append(siblings, &Relationship{
			FromEntityID: entityID,
			ToEntityID:   siblingID,
			EdgeType:     "graph.rel.sibling", // Inferred relationship type
			Weight:       0.7,                 // Lower weight than explicit relationships
			Properties: map[string]interface{}{
				"inferred":    true,
				"inference":   "type_prefix",
				"type_prefix": typePrefix,
			},
		})
	}

	if qe.logger != nil && len(siblings) > 0 {
		qe.logger.Debug("inferred sibling relationships",
			"entity", entityID,
			"prefix", typePrefix,
			"siblings_found", len(siblings))
	}

	return siblings, nil
}

// matchesEdgeTypes checks if edge type is in allowed list (empty = all)
func (qe *Manager) matchesEdgeTypes(edgeType string, allowedTypes []string) bool {
	if len(allowedTypes) == 0 {
		return true // Empty = allow all
	}
	for _, allowed := range allowedTypes {
		if edgeType == allowed {
			return true
		}
	}
	return false
}

// matchesNodeTypes checks if entity type matches filter (empty = all)
// Entity type is extracted from the 5th part of the 6-part entity ID
func (qe *Manager) matchesNodeTypes(entityID string, allowedTypes []string) bool {
	if len(allowedTypes) == 0 {
		return true // Empty = allow all
	}
	eid, err := message.ParseEntityID(entityID)
	if err != nil {
		return false // Can't parse = doesn't match
	}
	for _, allowed := range allowedTypes {
		if eid.Type == allowed {
			return true
		}
	}
	return false
}

// buildPathResult constructs QueryResult from traversal state
func (qe *Manager) buildPathResult(state *pathTraversalState, pattern PathPattern) *QueryResult {
	// Collect entities (order determined by subsequent sort in resolver)
	entities := make([]*gtypes.EntityState, 0, len(state.entities))
	for mapKey, entity := range state.entities {
		entities = append(entities, entity)
		// DEBUG: Log any mismatch between map key and entity.ID
		if mapKey != entity.ID && qe.logger != nil {
			qe.logger.Warn("PathRAG score key mismatch",
				"map_key", mapKey,
				"entity_id", entity.ID,
				"score", state.scores[mapKey])
		}
	}

	// Build GraphPath objects for each discovered path
	paths := make([]GraphPath, 0)
	for id, pathIDs := range state.pathTo {
		// Skip start-only paths if IncludeSelf is false
		if len(pathIDs) == 1 && !pattern.IncludeSelf {
			continue
		}

		edges := state.edgesTo[id]

		// Calculate total weight (product of edge weights)
		totalWeight := 1.0
		for _, e := range edges {
			if e.Weight > 0 {
				totalWeight *= e.Weight
			}
		}

		paths = append(paths, GraphPath{
			Entities: pathIDs,
			Edges:    edges,
			Length:   len(edges),
			Weight:   totalWeight,
		})
	}

	return &QueryResult{
		Entities: entities,
		Paths:    paths,
		Count:    len(entities),
		Scores:   state.scores,
	}
}

// validatePathPattern validates a path pattern
func (qe *Manager) validatePathPattern(pattern PathPattern) error {
	if pattern.MaxDepth <= 0 {
		msg := fmt.Sprintf("max depth must be positive, got %d", pattern.MaxDepth)
		return errs.WrapInvalid(errs.ErrInvalidData, "query manager",
			"validatePathPattern", msg)
	}
	if pattern.MaxDepth > qe.config.Query.MaxPathLength {
		msg := fmt.Sprintf("max depth %d exceeds limit %d",
			pattern.MaxDepth, qe.config.Query.MaxPathLength)
		return errs.WrapInvalid(errs.ErrInvalidData, "query manager",
			"validatePathPattern", msg)
	}
	if pattern.DecayFactor < 0 || pattern.DecayFactor > 1 {
		msg := fmt.Sprintf("decay factor must be between 0 and 1, got %f", pattern.DecayFactor)
		return errs.WrapInvalid(errs.ErrInvalidData, "query manager",
			"validatePathPattern", msg)
	}
	return nil
}

// generatePathQueryKey generates a cache key for path queries
func (qe *Manager) generatePathQueryKey(start string, pattern PathPattern) string {
	return fmt.Sprintf("path:%s:%d:%v:%v", start, pattern.MaxDepth, pattern.EdgeTypes, pattern.Direction)
}
