package teamsmemory

import (
	"context"
	"fmt"

	"github.com/c360studio/semstreams/pkg/errs"
)

// GraphClient defines the interface for graph operations
type GraphClient interface {
	Query(ctx context.Context, query string) (string, error)
}

// Hydrator provides context hydration from the knowledge graph
type Hydrator struct {
	config      HydrationConfig
	graphClient GraphClient
}

// HydratedContext represents hydrated context with token count
type HydratedContext struct {
	Context    string
	TokenCount int
}

// NewHydrator creates a new Hydrator instance
func NewHydrator(config HydrationConfig, graphClient GraphClient) (*Hydrator, error) {
	return &Hydrator{
		config:      config,
		graphClient: graphClient,
	}, nil
}

// HydratePostCompaction reconstructs context after compaction events
func (h *Hydrator) HydratePostCompaction(ctx context.Context, loopID string) (*HydratedContext, error) {
	// Validate input
	if loopID == "" {
		return nil, errs.WrapInvalid(
			fmt.Errorf("loopID cannot be empty"),
			"Hydrator",
			"HydratePostCompaction",
			"validate loopID",
		)
	}

	// Check context cancellation
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Query graph for post-compaction context
	var contextStr string
	if h.graphClient != nil {
		query := fmt.Sprintf("reconstruct_context_for_loop:%s", loopID)
		result, err := h.graphClient.Query(ctx, query)
		if err != nil {
			// Don't fail on graph query errors, just log and continue
			contextStr = ""
		} else {
			contextStr = result
		}
	}

	// Estimate token count (rough approximation: 4 chars per token)
	tokenCount := len(contextStr) / 4

	// Respect max recovery tokens if configured
	if h.config.PostCompaction.MaxRecoveryTokens > 0 && tokenCount > h.config.PostCompaction.MaxRecoveryTokens {
		// Truncate to max tokens
		maxChars := h.config.PostCompaction.MaxRecoveryTokens * 4
		if len(contextStr) > maxChars {
			contextStr = contextStr[:maxChars]
			tokenCount = h.config.PostCompaction.MaxRecoveryTokens
		}
	}

	return &HydratedContext{
		Context:    contextStr,
		TokenCount: tokenCount,
	}, nil
}

// HydratePreTask injects context before task execution
func (h *Hydrator) HydratePreTask(ctx context.Context, loopID, taskDescription string) (*HydratedContext, error) {
	// Validate inputs
	if loopID == "" {
		return nil, errs.WrapInvalid(
			fmt.Errorf("loopID cannot be empty"),
			"Hydrator",
			"HydratePreTask",
			"validate loopID",
		)
	}
	if taskDescription == "" {
		return nil, errs.WrapInvalid(
			fmt.Errorf("taskDescription cannot be empty"),
			"Hydrator",
			"HydratePreTask",
			"validate taskDescription",
		)
	}

	// Check context cancellation
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Query graph for pre-task context
	var contextStr string
	if h.graphClient != nil {
		query := fmt.Sprintf("pre_task_context:%s:%s", loopID, taskDescription)
		result, err := h.graphClient.Query(ctx, query)
		if err != nil {
			// Don't fail on graph query errors, just log and continue
			contextStr = ""
		} else {
			contextStr = result
		}
	}

	// Estimate token count (rough approximation: 4 chars per token)
	tokenCount := len(contextStr) / 4

	// Respect max context tokens if configured
	if h.config.PreTask.MaxContextTokens > 0 && tokenCount > h.config.PreTask.MaxContextTokens {
		// Truncate to max tokens
		maxChars := h.config.PreTask.MaxContextTokens * 4
		if len(contextStr) > maxChars {
			contextStr = contextStr[:maxChars]
			tokenCount = h.config.PreTask.MaxContextTokens
		}
	}

	return &HydratedContext{
		Context:    contextStr,
		TokenCount: tokenCount,
	}, nil
}

// HydrateForEntities hydrates context for specific entity IDs.
// This supports explicit entity-based hydration for the embedded context pattern.
func (h *Hydrator) HydrateForEntities(ctx context.Context, loopID string, entityIDs []string, depth int) (*HydratedContext, error) {
	// Validate inputs
	if loopID == "" {
		return nil, errs.WrapInvalid(
			fmt.Errorf("loopID cannot be empty"),
			"Hydrator",
			"HydrateForEntities",
			"validate loopID",
		)
	}
	if len(entityIDs) == 0 {
		return nil, errs.WrapInvalid(
			fmt.Errorf("entityIDs cannot be empty"),
			"Hydrator",
			"HydrateForEntities",
			"validate entityIDs",
		)
	}

	// Check context cancellation
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Build query for entities
	var contextStr string
	if h.graphClient != nil {
		// Query each entity and concatenate results
		var parts []string
		for _, entityID := range entityIDs {
			query := fmt.Sprintf("entity:%s:depth:%d", entityID, depth)
			result, err := h.graphClient.Query(ctx, query)
			if err != nil {
				// Don't fail on individual query errors
				continue
			}
			if result != "" {
				parts = append(parts, result)
			}
		}

		// Join entity contexts
		for i, part := range parts {
			if i > 0 {
				contextStr += "\n---\n"
			}
			contextStr += part
		}
	}

	// Estimate token count (rough approximation: 4 chars per token)
	tokenCount := len(contextStr) / 4

	// Respect max context tokens if configured
	if h.config.PreTask.MaxContextTokens > 0 && tokenCount > h.config.PreTask.MaxContextTokens {
		maxChars := h.config.PreTask.MaxContextTokens * 4
		if len(contextStr) > maxChars {
			contextStr = contextStr[:maxChars]
			tokenCount = h.config.PreTask.MaxContextTokens
		}
	}

	return &HydratedContext{
		Context:    contextStr,
		TokenCount: tokenCount,
	}, nil
}

// FormatContext formats context from graph query results
func (h *Hydrator) FormatContext(ctx context.Context, decisionsJSON, filesJSON, toolsJSON string, maxTokens int) (string, error) {
	// Check context cancellation
	if err := ctx.Err(); err != nil {
		return "", err
	}

	// Build formatted context from components
	formatted := ""

	if decisionsJSON != "" {
		formatted += "## Decisions\n" + decisionsJSON + "\n\n"
	}

	if filesJSON != "" {
		formatted += "## Files\n" + filesJSON + "\n\n"
	}

	if toolsJSON != "" {
		formatted += "## Tools\n" + toolsJSON + "\n\n"
	}

	// Respect max tokens if configured
	if maxTokens > 0 {
		// Estimate token count (rough approximation: 4 chars per token)
		tokenCount := len(formatted) / 4
		if tokenCount > maxTokens {
			// Truncate to max tokens
			maxChars := maxTokens * 4
			if len(formatted) > maxChars {
				formatted = formatted[:maxChars]
			}
		}
	}

	return formatted, nil
}
