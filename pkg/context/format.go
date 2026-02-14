package context

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// FormatOptions configures context formatting
type FormatOptions struct {
	MaxTokens       int      // Max tokens for output
	PrettyPrint     bool     // Pretty print JSON
	IncludeMetadata bool     // Include entity metadata
	EntityOrder     []string // Explicit order for entities (if empty, uses map order)
	SectionHeaders  bool     // Add section headers
}

// DefaultFormatOptions returns sensible defaults for formatting
func DefaultFormatOptions() FormatOptions {
	return FormatOptions{
		MaxTokens:       4000,
		PrettyPrint:     true,
		IncludeMetadata: false,
		SectionHeaders:  true,
	}
}

// FormatEntitiesForContext formats entity data for LLM context.
// Returns the formatted string, token count, and any error.
func FormatEntitiesForContext(entities map[string]json.RawMessage, opts FormatOptions) (string, int, error) {
	if len(entities) == 0 {
		return "", 0, nil
	}

	var builder strings.Builder

	// Determine entity order
	entityOrder := opts.EntityOrder
	if len(entityOrder) == 0 {
		entityOrder = make([]string, 0, len(entities))
		for id := range entities {
			entityOrder = append(entityOrder, id)
		}
		sort.Strings(entityOrder) // Consistent ordering
	}

	// Track budget
	budget := NewBudgetAllocation(opts.MaxTokens)
	headerTokens := 50 // Reserve for headers
	budget.Allocate("headers", headerTokens)

	// Allocate remaining budget equally among entities
	perEntity := budget.Remaining() / len(entityOrder)

	for i, entityID := range entityOrder {
		data, exists := entities[entityID]
		if !exists {
			continue
		}

		// Format entity header
		if opts.SectionHeaders {
			if i > 0 {
				builder.WriteString("\n")
			}
			builder.WriteString(fmt.Sprintf("### Entity: %s\n", entityID))
		}

		// Format entity data
		var formatted string
		if opts.PrettyPrint {
			var parsed any
			if err := json.Unmarshal(data, &parsed); err == nil {
				if pretty, err := json.MarshalIndent(parsed, "", "  "); err == nil {
					formatted = string(pretty)
				}
			}
		}
		if formatted == "" {
			formatted = string(data)
		}

		// Truncate if needed
		if perEntity > 0 {
			formatted = TruncateToBudget(formatted, perEntity)
		}

		builder.WriteString(formatted)
		builder.WriteString("\n")
	}

	result := builder.String()
	tokenCount := EstimateTokens(result)

	// Final truncation if still over budget
	if opts.MaxTokens > 0 && tokenCount > opts.MaxTokens {
		result = TruncateToBudget(result, opts.MaxTokens)
		tokenCount = EstimateTokens(result)
	}

	return result, tokenCount, nil
}

// FormatRelationshipsForContext formats relationships for LLM context.
func FormatRelationshipsForContext(relationships []Relationship, opts FormatOptions) (string, int, error) {
	if len(relationships) == 0 {
		return "", 0, nil
	}

	var builder strings.Builder

	if opts.SectionHeaders {
		builder.WriteString("### Relationships\n")
	}

	for _, rel := range relationships {
		builder.WriteString(fmt.Sprintf("- %s -[%s]-> %s\n",
			rel.Subject, rel.Predicate, rel.Object))
	}

	result := builder.String()
	tokenCount := EstimateTokens(result)

	if opts.MaxTokens > 0 && tokenCount > opts.MaxTokens {
		result = TruncateToBudget(result, opts.MaxTokens)
		tokenCount = EstimateTokens(result)
	}

	return result, tokenCount, nil
}

// FormatBatchResultForContext formats a BatchQueryResult for LLM context.
func FormatBatchResultForContext(result *BatchQueryResult, opts FormatOptions) (string, int, error) {
	if result == nil {
		return "", 0, nil
	}

	var builder strings.Builder

	// Format entities
	if len(result.Entities) > 0 {
		entityContent, _, err := FormatEntitiesForContext(result.Entities, FormatOptions{
			MaxTokens:      opts.MaxTokens / 2, // Half for entities
			PrettyPrint:    opts.PrettyPrint,
			SectionHeaders: opts.SectionHeaders,
		})
		if err != nil {
			return "", 0, err
		}
		builder.WriteString(entityContent)
	}

	// Format relationships
	if len(result.Relationships) > 0 {
		relContent, _, err := FormatRelationshipsForContext(result.Relationships, FormatOptions{
			MaxTokens:      opts.MaxTokens / 2, // Half for relationships
			SectionHeaders: opts.SectionHeaders,
		})
		if err != nil {
			return "", 0, err
		}
		if builder.Len() > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString(relContent)
	}

	// Note not found entities
	if len(result.NotFound) > 0 && opts.IncludeMetadata {
		builder.WriteString("\n### Not Found\n")
		for _, id := range result.NotFound {
			builder.WriteString(fmt.Sprintf("- %s\n", id))
		}
	}

	resultStr := builder.String()
	tokenCount := EstimateTokens(resultStr)

	if opts.MaxTokens > 0 && tokenCount > opts.MaxTokens {
		resultStr = TruncateToBudget(resultStr, opts.MaxTokens)
		tokenCount = EstimateTokens(resultStr)
	}

	return resultStr, tokenCount, nil
}

// BuildContextFromBatch creates a ConstructedContext from a BatchQueryResult.
func BuildContextFromBatch(result *BatchQueryResult, opts FormatOptions) (*ConstructedContext, error) {
	content, tokenCount, err := FormatBatchResultForContext(result, opts)
	if err != nil {
		return nil, err
	}

	// Build sources
	var sources []Source
	for id := range result.Entities {
		sources = append(sources, EntitySource(id))
	}
	for _, rel := range result.Relationships {
		sources = append(sources, RelationshipSource(
			fmt.Sprintf("%s-%s-%s", rel.Subject, rel.Predicate, rel.Object)))
	}

	// Collect entity IDs
	entityIDs := make([]string, 0, len(result.Entities))
	for id := range result.Entities {
		entityIDs = append(entityIDs, id)
	}

	return &ConstructedContext{
		Content:    content,
		TokenCount: tokenCount,
		Entities:   entityIDs,
		Sources:    sources,
	}, nil
}
