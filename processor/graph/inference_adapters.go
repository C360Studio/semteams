package graph

import (
	"context"

	"github.com/c360/semstreams/processor/graph/indexmanager"
	"github.com/c360/semstreams/processor/graph/inference"
)

// similarityFinderAdapter adapts IndexManager to inference.SimilarityFinder interface.
// This enables the inference detectors to query semantic similarity without
// depending directly on the IndexManager implementation.
type similarityFinderAdapter struct {
	indexManager indexmanager.Indexer
}

// newSimilarityFinderAdapter creates a new SimilarityFinder adapter.
func newSimilarityFinderAdapter(im indexmanager.Indexer) *similarityFinderAdapter {
	return &similarityFinderAdapter{indexManager: im}
}

// FindSimilar returns entity IDs semantically similar to the given entity.
// Implements inference.SimilarityFinder interface.
func (a *similarityFinderAdapter) FindSimilar(
	ctx context.Context,
	entityID string,
	threshold float64,
	limit int,
) ([]inference.SimilarityResult, error) {
	if a.indexManager == nil {
		return nil, nil
	}

	hits, err := a.indexManager.FindSimilarEntities(ctx, entityID, threshold, limit)
	if err != nil {
		return nil, err
	}

	results := make([]inference.SimilarityResult, len(hits))
	for i, hit := range hits {
		results[i] = inference.SimilarityResult{
			EntityID:   hit.EntityID,
			Similarity: hit.Similarity,
		}
	}
	return results, nil
}

// relationshipQuerierAdapter adapts IndexManager to inference.RelationshipQuerier interface.
// This enables the inference detectors to query relationships without
// depending directly on the IndexManager implementation.
type relationshipQuerierAdapter struct {
	indexManager indexmanager.Indexer
}

// newRelationshipQuerierAdapter creates a new RelationshipQuerier adapter.
func newRelationshipQuerierAdapter(im indexmanager.Indexer) *relationshipQuerierAdapter {
	return &relationshipQuerierAdapter{indexManager: im}
}

// GetOutgoingRelationships returns all outgoing relationships from an entity.
// Implements inference.RelationshipQuerier interface.
func (a *relationshipQuerierAdapter) GetOutgoingRelationships(
	ctx context.Context,
	entityID string,
) ([]inference.RelationshipInfo, error) {
	if a.indexManager == nil {
		return nil, nil
	}

	rels, err := a.indexManager.GetOutgoingRelationships(ctx, entityID)
	if err != nil {
		return nil, err
	}

	results := make([]inference.RelationshipInfo, len(rels))
	for i, rel := range rels {
		results[i] = inference.RelationshipInfo{
			FromEntityID: entityID,
			ToEntityID:   rel.ToEntityID,
			Predicate:    rel.EdgeType,
		}
	}
	return results, nil
}

// GetIncomingRelationships returns all incoming relationships to an entity.
// Implements inference.RelationshipQuerier interface.
func (a *relationshipQuerierAdapter) GetIncomingRelationships(
	ctx context.Context,
	entityID string,
) ([]inference.RelationshipInfo, error) {
	if a.indexManager == nil {
		return nil, nil
	}

	rels, err := a.indexManager.GetIncomingRelationships(ctx, entityID)
	if err != nil {
		return nil, err
	}

	results := make([]inference.RelationshipInfo, len(rels))
	for i, rel := range rels {
		results[i] = inference.RelationshipInfo{
			FromEntityID: rel.FromEntityID,
			ToEntityID:   entityID,
			Predicate:    rel.EdgeType,
		}
	}
	return results, nil
}
