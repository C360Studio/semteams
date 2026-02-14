package actions

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ToolExecutor defines the interface for executing tools
type ToolExecutor interface {
	Execute(ctx context.Context, toolName string, args map[string]any) (json.RawMessage, error)
}

// ToolBatchAction executes multiple tools concurrently with graph optimization
type ToolBatchAction struct {
	Tools    []string // Tool calls in format "tool_name:arg" or just "tool_name"
	FailFast bool     // Stop on first failure

	// Injected at execution time
	ToolExecutor ToolExecutor
}

// NewToolBatchAction creates a new tool batch action
func NewToolBatchAction(tools []string, failFast bool) *ToolBatchAction {
	return &ToolBatchAction{
		Tools:    tools,
		FailFast: failFast,
	}
}

// ToolResult represents the result of a single tool execution
type ToolResult struct {
	Tool    string          `json:"tool"`
	Success bool            `json:"success"`
	Output  json.RawMessage `json:"output,omitempty"`
	Error   string          `json:"error,omitempty"`
}

// Execute runs all tools concurrently, with graph query optimization
func (a *ToolBatchAction) Execute(ctx context.Context, _ *Context) Result {
	start := time.Now()

	if len(a.Tools) == 0 {
		return Result{
			Success:  false,
			Error:    "tool_batch action requires at least one tool",
			Duration: time.Since(start),
		}
	}

	if a.ToolExecutor == nil {
		return Result{
			Success:  false,
			Error:    "tool executor not available",
			Duration: time.Since(start),
		}
	}

	// Parse and optimize tool calls
	parsedTools := a.parseTools()
	optimizedTools := a.optimizeGraphQueries(parsedTools)

	// Execute tools concurrently
	results := a.executeTools(ctx, optimizedTools)

	// Check for failures
	allSuccess := true
	var firstError string
	for _, r := range results {
		if !r.Success {
			allSuccess = false
			if firstError == "" {
				firstError = fmt.Sprintf("%s: %s", r.Tool, r.Error)
			}
			if a.FailFast {
				break
			}
		}
	}

	// Build output
	output, err := json.Marshal(map[string]any{
		"results": results,
		"count":   len(results),
		"success": allSuccess,
	})
	if err != nil {
		return Result{
			Success:  false,
			Error:    fmt.Sprintf("failed to marshal results: %v", err),
			Duration: time.Since(start),
		}
	}

	result := Result{
		Success:  allSuccess,
		Output:   output,
		Duration: time.Since(start),
	}
	if !allSuccess {
		result.Error = firstError
	}

	return result
}

// parsedTool represents a parsed tool call
type parsedTool struct {
	Name string
	Arg  string
	Args map[string]any
}

// parseTools parses tool strings into structured calls
func (a *ToolBatchAction) parseTools() []parsedTool {
	var result []parsedTool

	for _, tool := range a.Tools {
		parts := strings.SplitN(tool, ":", 2)
		pt := parsedTool{
			Name: parts[0],
			Args: make(map[string]any),
		}
		if len(parts) > 1 {
			pt.Arg = parts[1]
			// Common argument mapping
			switch pt.Name {
			case "query_entity":
				pt.Args["entity_id"] = pt.Arg
			case "query_entities":
				// Multiple entities comma-separated
				pt.Args["entity_ids"] = strings.Split(pt.Arg, ",")
			default:
				pt.Args["arg"] = pt.Arg
			}
		}
		result = append(result, pt)
	}

	return result
}

// optimizeGraphQueries combines multiple query_entity calls into query_entities
func (a *ToolBatchAction) optimizeGraphQueries(tools []parsedTool) []parsedTool {
	var entityIDs []string
	var otherTools []parsedTool

	// Collect query_entity calls
	for _, tool := range tools {
		if tool.Name == "query_entity" {
			if entityID, ok := tool.Args["entity_id"].(string); ok {
				entityIDs = append(entityIDs, entityID)
			} else {
				otherTools = append(otherTools, tool)
			}
		} else {
			otherTools = append(otherTools, tool)
		}
	}

	// If we have multiple query_entity calls, batch them
	if len(entityIDs) > 1 {
		batchedTool := parsedTool{
			Name: "query_entities",
			Args: map[string]any{
				"entity_ids": entityIDs,
			},
		}
		return append([]parsedTool{batchedTool}, otherTools...)
	}

	// Return original if no optimization possible
	return tools
}

// executeTools runs all tools concurrently
func (a *ToolBatchAction) executeTools(ctx context.Context, tools []parsedTool) []ToolResult {
	results := make([]ToolResult, len(tools))

	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstFailure bool

	for i, tool := range tools {
		wg.Add(1)
		go func(idx int, t parsedTool) {
			defer wg.Done()

			// Check for fail-fast cancellation
			if a.FailFast {
				mu.Lock()
				if firstFailure {
					mu.Unlock()
					results[idx] = ToolResult{
						Tool:    t.Name,
						Success: false,
						Error:   "skipped due to fail-fast",
					}
					return
				}
				mu.Unlock()
			}

			// Execute the tool
			output, err := a.ToolExecutor.Execute(ctx, t.Name, t.Args)

			tr := ToolResult{
				Tool:    t.Name,
				Success: err == nil,
				Output:  output,
			}
			if err != nil {
				tr.Error = err.Error()
				if a.FailFast {
					mu.Lock()
					firstFailure = true
					mu.Unlock()
				}
			}

			results[idx] = tr
		}(i, tool)
	}

	wg.Wait()
	return results
}
