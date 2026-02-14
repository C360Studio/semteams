// Package context provides building blocks for context construction in agentic systems.
// Consumers use these utilities to build ConstructedContext before dispatching agents,
// enabling "embed context, don't make agents discover it" pattern.
package context

import (
	"time"

	"github.com/c360studio/semstreams/pkg/types"
)

// ConstructedContext is an alias for types.ConstructedContext.
// The canonical type is defined in pkg/types/context.go.
type ConstructedContext = types.ConstructedContext

// Source is an alias for types.ContextSource, tracking where context came from.
// The canonical type is defined in pkg/types/context.go.
type Source = types.ContextSource

// NewConstructedContext creates a new ConstructedContext from parts.
func NewConstructedContext(content string, entities []string, sources []Source) *ConstructedContext {
	return &ConstructedContext{
		Content:       content,
		TokenCount:    EstimateTokens(content),
		Entities:      entities,
		Sources:       sources,
		ConstructedAt: time.Now(),
	}
}

// EntitySource creates a Source for a graph entity
func EntitySource(entityID string) Source {
	return Source{
		Type: types.SourceTypeGraphEntity,
		ID:   entityID,
	}
}

// RelationshipSource creates a Source for a graph relationship
func RelationshipSource(relationshipID string) Source {
	return Source{
		Type: types.SourceTypeGraphRelationship,
		ID:   relationshipID,
	}
}

// DocumentSource creates a Source for a document
func DocumentSource(docID string) Source {
	return Source{
		Type: types.SourceTypeDocument,
		ID:   docID,
	}
}
