# agentic-tools

Tool execution component for the agentic processing system.

## Overview

The `agentic-tools` component executes tool calls from the agentic loop orchestrator. It receives `ToolCall` messages, dispatches them to registered tool executors, and publishes `ToolResult` messages back. Supports tool registration, allowlist filtering, per-execution timeouts, and concurrent execution.

## Architecture

```
┌───────────────┐     ┌────────────────┐     ┌──────────────────┐
│ agentic-loop  │────►│ agentic-tools  │────►│ Tool Executors   │
│               │     │                │     │ (your code)      │
│               │◄────│                │◄────│                  │
└───────────────┘     └────────────────┘     └──────────────────┘
  tool.execute.*        Execute()           read_file, query_db,
  tool.result.*                             call_api, etc.
```

## Features

- **Tool Registration**: Register custom tool executors at runtime
- **Allowlist Filtering**: Restrict which tools can execute
- **Timeout Handling**: Per-execution timeout with context cancellation
- **Concurrent Execution**: Multiple tools can run in parallel

## Configuration

```json
{
  "type": "processor",
  "name": "agentic-tools",
  "enabled": true,
  "config": {
    "stream_name": "AGENT",
    "timeout": "60s",
    "allowed_tools": null,
    "ports": {
      "inputs": [
        {"name": "tool_calls", "type": "jetstream", "subject": "tool.execute.>", "stream_name": "AGENT"}
      ],
      "outputs": [
        {"name": "tool_results", "type": "jetstream", "subject": "tool.result.*", "stream_name": "AGENT"}
      ]
    }
  }
}
```

### Configuration Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `allowed_tools` | []string | null | Tool allowlist (null/empty = allow all) |
| `timeout` | string | "60s" | Per-tool execution timeout |
| `stream_name` | string | "AGENT" | JetStream stream name |
| `consumer_name_suffix` | string | "" | Suffix for consumer names (for testing) |
| `ports` | object | (defaults) | Port configuration |

### JetStream Integration

All messaging uses JetStream for durability. Tool call subjects require the AGENT stream to exist with subjects matching `tool.execute.>` and `tool.result.>`.

Consumer naming: `agentic-tools-{subject-pattern}`

## Ports

### Inputs

| Name | Type | Subject | Description |
|------|------|---------|-------------|
| tool_calls | jetstream | tool.execute.> | Tool calls from agentic-loop |

### Outputs

| Name | Type | Subject | Description |
|------|------|---------|-------------|
| tool_results | jetstream | tool.result.* | Tool results to agentic-loop |

## Tool Registration

Tools implement the `ToolExecutor` interface:

```go
type ToolExecutor interface {
    Execute(ctx context.Context, call agentic.ToolCall) (agentic.ToolResult, error)
    ListTools() []agentic.ToolDefinition
}
```

### Global Registration (Preferred)

Register tools globally via `init()` so they're available to all agentic-tools components:

```go
package mytools

import agentictools "github.com/c360/semstreams/processor/agentic-tools"

func init() {
    agentictools.RegisterTool("read_file", &FileReader{})
    agentictools.RegisterTool("query_graph", &GraphQueryExecutor{})
}
```

This pattern matches how components and rules are registered in SemStreams. Globally registered tools are automatically available to all agentic-tools component instances.

### Per-Component Registration

For component-specific tools, register after creating the component:

```go
comp, _ := agentictools.NewComponent(rawConfig, deps)
toolsComp := comp.(*agentictools.Component)

// Register component-specific executors
toolsComp.RegisterToolExecutor(&CustomExecutor{})

// Start component
lc := comp.(component.LifecycleComponent)
lc.Initialize()
lc.Start(ctx)
```

Local tools take precedence over global tools with the same name.

### Example Implementation

```go
type FileReader struct{}

func (f *FileReader) Execute(ctx context.Context, call agentic.ToolCall) (agentic.ToolResult, error) {
    var args struct {
        Path string `json:"path"`
    }
    if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
        return agentic.ToolResult{CallID: call.ID, Error: err.Error()}, nil
    }

    // Respect context cancellation
    select {
    case <-ctx.Done():
        return agentic.ToolResult{CallID: call.ID, Error: "cancelled"}, ctx.Err()
    default:
    }

    content, err := os.ReadFile(args.Path)
    if err != nil {
        return agentic.ToolResult{CallID: call.ID, Error: err.Error()}, nil
    }

    return agentic.ToolResult{CallID: call.ID, Content: string(content)}, nil
}

func (f *FileReader) ListTools() []agentic.ToolDefinition {
    return []agentic.ToolDefinition{{
        Name:        "read_file",
        Description: "Read the contents of a file",
        Parameters: map[string]any{
            "type": "object",
            "properties": map[string]any{
                "path": map[string]any{"type": "string", "description": "File path"},
            },
            "required": []string{"path"},
        },
    }}
}
```

## Tool Allowlist

Control which tools can execute:

```json
{
  "allowed_tools": ["read_file", "list_dir", "query_graph"]
}
```

| Config | Behavior |
|--------|----------|
| `null` or `[]` | All registered tools allowed |
| `["tool1", "tool2"]` | Only listed tools allowed |

Blocked tools return an error result (not a Go error):

```json
{
  "call_id": "call_001",
  "error": "tool 'delete_file' is not allowed"
}
```

## Message Formats

### ToolCall (Input)

```json
{
  "id": "call_001",
  "name": "read_file",
  "arguments": {
    "path": "/etc/hosts"
  }
}
```

### ToolResult (Output)

```json
{
  "call_id": "call_001",
  "content": "127.0.0.1 localhost\n...",
  "error": "",
  "metadata": {}
}
```

## Common Tools to Implement

| Tool | Description |
|------|-------------|
| `read_file` | Read file contents |
| `write_file` | Write content to file |
| `list_dir` | List directory contents |
| `fetch_url` | HTTP GET request |
| `call_api` | Generic HTTP request |
| `query_graph` | Query knowledge graph |
| `run_command` | Execute shell command |

## Troubleshooting

### Tool not found

- Verify tool executor is registered before Start()
- Check tool name matches exactly (case-sensitive)
- Ensure ListTools() returns the correct name

### Tool timeout

- Increase `timeout` for long-running operations
- Implement context cancellation in executor
- Check for blocking operations

### Tool blocked by allowlist

- Add tool name to `allowed_tools` array
- Set `allowed_tools: null` to allow all
- Verify tool name spelling

### Concurrent execution issues

- Ensure tool executor is thread-safe
- Don't share mutable state between calls
- Use proper synchronization if needed

## Related Components

- [agentic-loop](../agentic-loop/) - Loop orchestration
- [agentic-model](../agentic-model/) - LLM endpoint integration
- [agentic types](../../agentic/) - Shared type definitions
