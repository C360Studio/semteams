# Semspec Tool Integration Guide

This document describes how to integrate semspec tools with the SemStreams agentic-tools component using the global tool registration pattern.

## Overview

The agentic-tools component supports global tool registration via `init()` functions. This allows the semspec package to register custom tools (like `spec_query`, `proposal_create`, `task_list`) that will be automatically loaded when the agentic-tools component starts.

## Quick Start

### 1. Create Tool Executor

Each tool needs an executor that implements `agentictools.ToolExecutor`:

```go
// semspec/tools/spec_query.go
package tools

import (
    "context"
    "encoding/json"
    "fmt"

    "github.com/c360/semstreams/agentic"
)

// SpecQueryExecutor implements the spec_query tool
type SpecQueryExecutor struct {
    // Dependencies injected at creation
    store SpecStore
}

// NewSpecQueryExecutor creates a new spec query executor
func NewSpecQueryExecutor(store SpecStore) *SpecQueryExecutor {
    return &SpecQueryExecutor{store: store}
}

// Execute handles the tool call
func (e *SpecQueryExecutor) Execute(ctx context.Context, call agentic.ToolCall) (agentic.ToolResult, error) {
    // Parse arguments
    var args struct {
        SpecID string `json:"spec_id"`
        Query  string `json:"query"`
    }

    argsJSON, _ := json.Marshal(call.Arguments)
    if err := json.Unmarshal(argsJSON, &args); err != nil {
        return agentic.ToolResult{
            CallID: call.ID,
            Error:  fmt.Sprintf("invalid arguments: %v", err),
        }, nil
    }

    // Respect context cancellation
    select {
    case <-ctx.Done():
        return agentic.ToolResult{
            CallID: call.ID,
            Error:  "execution cancelled",
        }, ctx.Err()
    default:
    }

    // Execute the query
    result, err := e.store.Query(ctx, args.SpecID, args.Query)
    if err != nil {
        return agentic.ToolResult{
            CallID: call.ID,
            Error:  err.Error(),
        }, nil
    }

    return agentic.ToolResult{
        CallID:  call.ID,
        Content: result,
    }, nil
}

// ListTools returns the tool definitions
func (e *SpecQueryExecutor) ListTools() []agentic.ToolDefinition {
    return []agentic.ToolDefinition{
        {
            Name:        "spec_query",
            Description: "Query a specification by ID or search terms",
            Parameters: map[string]any{
                "type": "object",
                "properties": map[string]any{
                    "spec_id": map[string]any{
                        "type":        "string",
                        "description": "Specification ID to query",
                    },
                    "query": map[string]any{
                        "type":        "string",
                        "description": "Search query",
                    },
                },
            },
        },
    }
}
```

### 2. Register via init()

Register the tool globally so it's loaded when agentic-tools starts:

```go
// semspec/tools/register.go
package tools

import agentictools "github.com/c360/semstreams/processor/agentic-tools"

// Default store instance (or use dependency injection)
var defaultStore SpecStore

// SetStore sets the default store for tools
func SetStore(store SpecStore) {
    defaultStore = store
}

func init() {
    // Register all semspec tools
    agentictools.RegisterTool("spec_query", NewSpecQueryExecutor(nil))
    agentictools.RegisterTool("proposal_create", NewProposalCreateExecutor(nil))
    agentictools.RegisterTool("task_list", NewTaskListExecutor(nil))
    agentictools.RegisterTool("constitution_get", NewConstitutionGetExecutor(nil))
}
```

### 3. Import for Side Effects

Import the tools package in your main or initialization code:

```go
// semspec/main.go or cmd/semspec/main.go
package main

import (
    _ "github.com/c360/semspec/tools" // Register tools via init()
)
```

## API Reference

### ToolExecutor Interface

```go
type ToolExecutor interface {
    // Execute handles the tool call
    Execute(ctx context.Context, call agentic.ToolCall) (agentic.ToolResult, error)

    // ListTools returns the tool definitions this executor provides
    ListTools() []agentic.ToolDefinition
}
```

### ToolCall (Input)

```go
type ToolCall struct {
    ID        string         `json:"id"`        // Unique call ID
    Name      string         `json:"name"`      // Tool name
    Arguments map[string]any `json:"arguments"` // Tool arguments
}
```

### ToolResult (Output)

```go
type ToolResult struct {
    CallID   string         `json:"call_id"`           // Matches ToolCall.ID
    Content  string         `json:"content,omitempty"` // Success content
    Error    string         `json:"error,omitempty"`   // Error message
    Metadata map[string]any `json:"metadata,omitempty"`// Optional metadata
}
```

### ToolDefinition

```go
type ToolDefinition struct {
    Name        string         `json:"name"`        // Tool name
    Description string         `json:"description"` // What the tool does
    Parameters  map[string]any `json:"parameters"`  // JSON Schema for arguments
}
```

### Registration Function

```go
// RegisterTool registers a tool executor globally via init().
// Thread-safe and can be called from any package's init() function.
func RegisterTool(name string, executor ToolExecutor) error
```

## Argument Parsing

### Simple Arguments

```go
func (e *MyExecutor) Execute(ctx context.Context, call agentic.ToolCall) (agentic.ToolResult, error) {
    // Direct access for simple types
    name, _ := call.Arguments["name"].(string)
    count, _ := call.Arguments["count"].(float64) // JSON numbers are float64

    // ...
}
```

### Struct Arguments

```go
func (e *MyExecutor) Execute(ctx context.Context, call agentic.ToolCall) (agentic.ToolResult, error) {
    var args struct {
        SpecID   string   `json:"spec_id"`
        Query    string   `json:"query"`
        Limit    int      `json:"limit"`
        Tags     []string `json:"tags"`
    }

    // Marshal and unmarshal for proper type conversion
    argsJSON, _ := json.Marshal(call.Arguments)
    if err := json.Unmarshal(argsJSON, &args); err != nil {
        return agentic.ToolResult{
            CallID: call.ID,
            Error:  fmt.Sprintf("invalid arguments: %v", err),
        }, nil
    }

    // Use args.SpecID, args.Query, etc.
}
```

## Context Handling

Always respect context cancellation for proper timeout handling:

```go
func (e *MyExecutor) Execute(ctx context.Context, call agentic.ToolCall) (agentic.ToolResult, error) {
    // Check cancellation before expensive operations
    select {
    case <-ctx.Done():
        return agentic.ToolResult{
            CallID: call.ID,
            Error:  "execution cancelled",
        }, ctx.Err()
    default:
    }

    // For long-running operations, pass context down
    result, err := e.longOperation(ctx)

    // Check cancellation after operations
    if ctx.Err() != nil {
        return agentic.ToolResult{
            CallID: call.ID,
            Error:  "execution cancelled",
        }, ctx.Err()
    }

    return agentic.ToolResult{
        CallID:  call.ID,
        Content: result,
    }, nil
}
```

## Error Handling

### User-Facing Errors (ToolResult.Error)

Return errors that the agent can understand and act on:

```go
func (e *SpecQueryExecutor) Execute(ctx context.Context, call agentic.ToolCall) (agentic.ToolResult, error) {
    // Missing required argument
    specID, ok := call.Arguments["spec_id"].(string)
    if !ok || specID == "" {
        return agentic.ToolResult{
            CallID: call.ID,
            Error:  "spec_id is required",
        }, nil
    }

    // Resource not found
    spec, err := e.store.Get(ctx, specID)
    if err == ErrNotFound {
        return agentic.ToolResult{
            CallID: call.ID,
            Error:  fmt.Sprintf("specification %q not found", specID),
        }, nil
    }

    // Permission denied
    if !e.hasAccess(spec) {
        return agentic.ToolResult{
            CallID: call.ID,
            Error:  "access denied to this specification",
        }, nil
    }

    // ...
}
```

### System Errors (Go error return)

Return Go errors for system-level failures:

```go
func (e *MyExecutor) Execute(ctx context.Context, call agentic.ToolCall) (agentic.ToolResult, error) {
    // Context cancellation
    if ctx.Err() != nil {
        return agentic.ToolResult{
            CallID: call.ID,
            Error:  "execution cancelled",
        }, ctx.Err()
    }

    // Critical system failure
    if e.store == nil {
        return agentic.ToolResult{}, fmt.Errorf("store not initialized")
    }
}
```

## Parameter Schema Examples

### Required String Parameter

```go
Parameters: map[string]any{
    "type": "object",
    "properties": map[string]any{
        "spec_id": map[string]any{
            "type":        "string",
            "description": "The specification ID",
        },
    },
    "required": []string{"spec_id"},
}
```

### Optional Parameters with Defaults

```go
Parameters: map[string]any{
    "type": "object",
    "properties": map[string]any{
        "query": map[string]any{
            "type":        "string",
            "description": "Search query",
        },
        "limit": map[string]any{
            "type":        "integer",
            "description": "Maximum results (default: 10)",
            "default":     10,
        },
    },
    "required": []string{"query"},
}
```

### Enum Parameter

```go
Parameters: map[string]any{
    "type": "object",
    "properties": map[string]any{
        "status": map[string]any{
            "type":        "string",
            "description": "Task status filter",
            "enum":        []string{"pending", "in_progress", "completed", "failed"},
        },
    },
}
```

### Array Parameter

```go
Parameters: map[string]any{
    "type": "object",
    "properties": map[string]any{
        "tags": map[string]any{
            "type":        "array",
            "items":       map[string]any{"type": "string"},
            "description": "Tags to filter by",
        },
    },
}
```

### Nested Object Parameter

```go
Parameters: map[string]any{
    "type": "object",
    "properties": map[string]any{
        "config": map[string]any{
            "type":        "object",
            "description": "Configuration options",
            "properties": map[string]any{
                "verbose": map[string]any{"type": "boolean"},
                "format":  map[string]any{"type": "string"},
            },
        },
    },
}
```

## Suggested Semspec Tools

Based on the semspec workflow, here are recommended tools:

| Tool | Description | Key Parameters |
|------|-------------|----------------|
| `spec_query` | Query specifications | `spec_id`, `query` |
| `spec_create` | Create new specification | `name`, `description`, `requirements` |
| `spec_update` | Update specification | `spec_id`, `changes` |
| `proposal_create` | Create new proposal | `title`, `description` |
| `proposal_get` | Get proposal details | `proposal_id` |
| `task_list` | List tasks | `status`, `assigned_to` |
| `task_create` | Create new task | `title`, `spec_id`, `priority` |
| `task_update` | Update task status | `task_id`, `status`, `notes` |
| `constitution_get` | Get project constitution | (none) |
| `graph_query` | Query knowledge graph | `query`, `entity_type` |

## Testing Tools

```go
func TestSpecQueryExecutor(t *testing.T) {
    // Create mock store
    store := &MockSpecStore{
        specs: map[string]*Spec{
            "spec-001": {ID: "spec-001", Name: "Auth API"},
        },
    }

    // Create executor
    executor := NewSpecQueryExecutor(store)

    // Test ListTools
    tools := executor.ListTools()
    require.Len(t, tools, 1)
    assert.Equal(t, "spec_query", tools[0].Name)

    // Test Execute - success
    result, err := executor.Execute(context.Background(), agentic.ToolCall{
        ID:   "call-001",
        Name: "spec_query",
        Arguments: map[string]any{
            "spec_id": "spec-001",
        },
    })
    require.NoError(t, err)
    assert.Empty(t, result.Error)
    assert.Contains(t, result.Content, "Auth API")

    // Test Execute - not found
    result, err = executor.Execute(context.Background(), agentic.ToolCall{
        ID:   "call-002",
        Name: "spec_query",
        Arguments: map[string]any{
            "spec_id": "nonexistent",
        },
    })
    require.NoError(t, err)
    assert.Contains(t, result.Error, "not found")

    // Test Execute - context cancellation
    ctx, cancel := context.WithCancel(context.Background())
    cancel()
    result, err = executor.Execute(ctx, agentic.ToolCall{
        ID:   "call-003",
        Name: "spec_query",
        Arguments: map[string]any{
            "spec_id": "spec-001",
        },
    })
    assert.Error(t, err)
    assert.Contains(t, result.Error, "cancelled")
}
```

## Multi-Tool Executors

A single executor can provide multiple related tools:

```go
type SpecToolsExecutor struct {
    store SpecStore
}

func (e *SpecToolsExecutor) ListTools() []agentic.ToolDefinition {
    return []agentic.ToolDefinition{
        {
            Name:        "spec_get",
            Description: "Get a specification by ID",
            Parameters:  specGetParams,
        },
        {
            Name:        "spec_list",
            Description: "List all specifications",
            Parameters:  specListParams,
        },
        {
            Name:        "spec_search",
            Description: "Search specifications",
            Parameters:  specSearchParams,
        },
    }
}

func (e *SpecToolsExecutor) Execute(ctx context.Context, call agentic.ToolCall) (agentic.ToolResult, error) {
    switch call.Name {
    case "spec_get":
        return e.executeGet(ctx, call)
    case "spec_list":
        return e.executeList(ctx, call)
    case "spec_search":
        return e.executeSearch(ctx, call)
    default:
        return agentic.ToolResult{
            CallID: call.ID,
            Error:  fmt.Sprintf("unknown tool: %s", call.Name),
        }, nil
    }
}
```

Register multi-tool executors by their primary name:

```go
func init() {
    agentictools.RegisterTool("spec_tools", NewSpecToolsExecutor(nil))
}
```

## Dependency Injection

For tools that need runtime dependencies:

```go
// semspec/tools/registry.go
package tools

import agentictools "github.com/c360/semstreams/processor/agentic-tools"

// Registry holds tool executors with dependencies
type Registry struct {
    specQuery    *SpecQueryExecutor
    proposalTool *ProposalToolExecutor
}

// NewRegistry creates a registry with injected dependencies
func NewRegistry(store SpecStore, natsClient *natsclient.Client) *Registry {
    return &Registry{
        specQuery:    NewSpecQueryExecutor(store),
        proposalTool: NewProposalToolExecutor(store, natsClient),
    }
}

// Register registers all tools globally
func (r *Registry) Register() error {
    if err := agentictools.RegisterTool("spec_query", r.specQuery); err != nil {
        return err
    }
    if err := agentictools.RegisterTool("proposal_create", r.proposalTool); err != nil {
        return err
    }
    return nil
}
```

Usage:

```go
// In main.go
func main() {
    store := NewSpecStore(...)
    natsClient := natsclient.New(...)

    registry := tools.NewRegistry(store, natsClient)
    if err := registry.Register(); err != nil {
        log.Fatal(err)
    }

    // Start components...
}
```

## File Organization

Suggested file structure for semspec tools:

```
semspec/
├── tools/
│   ├── register.go       # init() registration or Registry
│   ├── spec_query.go     # spec_query tool
│   ├── spec_create.go    # spec_create tool
│   ├── proposal.go       # proposal tools
│   ├── task.go           # task tools
│   ├── constitution.go   # constitution_get tool
│   └── graph.go          # graph_query tool
└── main.go               # imports tools package
```

## Comparison: Commands vs Tools

| Aspect | Commands | Tools |
|--------|----------|-------|
| Triggered by | User input (`/spec`) | Agent decision |
| Interface | `CommandExecutor` | `ToolExecutor` |
| Context | `CommandContext` | `context.Context` |
| Registration | `router.RegisterCommand()` | `agentictools.RegisterTool()` |
| Input | `UserMessage` + args | `ToolCall` |
| Output | `UserResponse` | `ToolResult` |
| Use case | User-initiated actions | Agent-initiated actions |

## See Also

- [agentic-tools README](../../processor/agentic-tools/README.md) - Component documentation
- [agentic-tools doc.go](../../processor/agentic-tools/doc.go) - Package documentation
- [Agentic Components](../advanced/08-agentic-components.md) - System overview
- [Semspec Command Integration](./semspec-command-integration.md) - Command registration
