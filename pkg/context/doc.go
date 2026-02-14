// Package context provides building blocks for context construction in agentic systems.
//
// # Overview
//
// This package implements the "embed context, don't make agents discover it" pattern.
// Consumers use these utilities to build [ConstructedContext] before dispatching agents,
// enabling precise token budget management and eliminating runtime context discovery.
//
// The key insight is that SemStreams provides the HOW (building blocks), while
// consumers decide the WHAT (what's relevant for their domain). This separation
// allows domain-specific context construction while providing reusable utilities
// for token management, batch queries, and LLM-friendly formatting.
//
// # Core Types
//
// [ConstructedContext] wraps formatted context with token count and source tracking.
// It contains everything needed to embed context in an agent task:
//
//   - Content: The formatted string ready for LLM consumption
//   - TokenCount: Exact token count for budget management
//   - Entities: Entity IDs included in the context
//   - Sources: Provenance tracking (where context came from)
//   - ConstructedAt: Timestamp for cache management
//
// [Source] tracks where context originated. Source types include:
//
//   - graph_entity: Context from a knowledge graph entity
//   - graph_relationship: Context from graph relationships
//   - document: Context from a document or chunk
//
// # Building Block Functions
//
// Token estimation functions help manage context budgets:
//
//   - [EstimateTokens]: Estimates tokens using 4-char-per-token heuristic
//   - [EstimateTokensForModel]: Model-specific token estimation
//   - [FitsInBudget]: Checks if content fits within token budget
//   - [TruncateToBudget]: Truncates at word boundaries to fit budget
//   - [BudgetAllocation]: Tracks token budget allocation across sections
//
// Batch graph query functions fetch entities efficiently:
//
//   - [BatchQueryEntities]: Batch entity lookup with default options
//   - [BatchQueryEntitiesWithOptions]: Configurable batch queries with relationships
//   - [ExpandWithNeighbors]: Expands entity IDs to include N-hop neighbors
//   - [CollectEntityIDs]: Extracts unique entity IDs from relationships
//
// Context formatting functions prepare content for LLMs:
//
//   - [FormatEntitiesForContext]: Formats entities for LLM consumption
//   - [FormatRelationshipsForContext]: Formats relationships for LLM
//   - [FormatBatchResultForContext]: Formats complete batch results
//   - [BuildContextFromBatch]: Creates ConstructedContext from batch result
//
// # Example Usage
//
// Building context for an agent task:
//
//	// Query entities from the graph
//	result, err := context.BatchQueryEntitiesWithOptions(ctx, client, entityIDs,
//	    context.BatchQueryOptions{
//	        IncludeRelationships: true,
//	        Depth:                1,
//	    })
//	if err != nil {
//	    return err
//	}
//
//	// Build constructed context with token tracking
//	opts := context.FormatOptions{
//	    MaxTokens:      8000,
//	    PrettyPrint:    true,
//	    SectionHeaders: true,
//	}
//	constructed, err := context.BuildContextFromBatch(result, opts)
//	if err != nil {
//	    return err
//	}
//
//	// Embed in TaskMessage - token count is exact
//	task.Context = constructed
//
// Token budget management:
//
//	budget := context.NewBudgetAllocation(10000)
//	budget.Allocate("system_prompt", 500)
//	budget.Allocate("entities", 4000)
//	remaining := budget.Remaining() // 5500 for other content
//
// # Integration with Workflows
//
// When using [ConstructedContext] with the workflow processor's publish_agent action,
// the context is embedded directly in the TaskMessage. This enables the pattern:
//
//  1. Consumer builds context using domain-specific logic
//  2. Exact token count known before agent dispatch
//  3. Agent loop receives pre-built context (no discovery needed)
//  4. Fresh context per task (no pollution from prior work)
//
// # Design Rationale
//
// This package follows the principle that "what's relevant" is domain knowledge.
// A code review system has different relevance criteria than a logistics system.
// Rather than embedding domain-specific heuristics, SemStreams provides utilities
// that any domain can use:
//
//   - Token counting and budget management
//   - Efficient batch graph queries
//   - LLM-friendly formatting
//   - Source tracking for provenance
//
// The consumer (e.g., SemSpec) implements the relevance logic and uses these
// building blocks to construct the final context.
package context
