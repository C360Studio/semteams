# Semspec Command Integration Guide

This document describes how to integrate semspec commands with the SemStreams agentic-dispatch component using the global command registration pattern.

## Overview

The agentic-dispatch component supports global command registration via `init()` functions. This allows the semspec package to register custom commands (like `/spec`, `/propose`, `/review`) that will be automatically loaded when the component starts.

## Quick Start

### 1. Create Command Executor

Each command needs an executor that implements `agenticdispatch.CommandExecutor`:

```go
// semspec/commands/spec.go
package commands

import (
    "context"
    "fmt"
    "time"

    "github.com/c360/semstreams/agentic"
    agenticdispatch "github.com/c360/semstreams/processor/agentic-dispatch"
    "github.com/google/uuid"
)

// SpecCommand implements the /spec command
type SpecCommand struct{}

// Config returns the command configuration
func (c *SpecCommand) Config() agenticdispatch.CommandConfig {
    return agenticdispatch.CommandConfig{
        Pattern:     `^/spec\s*(.*)$`,    // /spec or /spec <name>
        Permission:  "submit_task",        // Required permission
        RequireLoop: false,                // Can run without active loop
        Help:        "/spec [name] - Run spec-driven development workflow",
    }
}

// Execute handles the command
func (c *SpecCommand) Execute(
    ctx context.Context,
    cmdCtx *agenticdispatch.CommandContext,
    msg agentic.UserMessage,
    args []string,
    loopID string,
) (agentic.UserResponse, error) {
    specName := ""
    if len(args) > 0 && args[0] != "" {
        specName = args[0]
    }

    // Your spec workflow logic here...

    return agentic.UserResponse{
        ResponseID:  uuid.New().String(),
        ChannelType: msg.ChannelType,
        ChannelID:   msg.ChannelID,
        UserID:      msg.UserID,
        Type:        agentic.ResponseTypeStatus,
        Content:     fmt.Sprintf("Starting spec workflow: %s", specName),
        Timestamp:   time.Now(),
    }, nil
}
```

### 2. Register via init()

Register the command globally so it's loaded when the agentic-dispatch component starts:

```go
// semspec/commands/register.go
package commands

import agenticdispatch "github.com/c360/semstreams/processor/agentic-dispatch"

func init() {
    // Register all semspec commands
    agenticdispatch.RegisterCommand("spec", &SpecCommand{})
    agenticdispatch.RegisterCommand("propose", &ProposeCommand{})
    agenticdispatch.RegisterCommand("review", &ReviewCommand{})
    agenticdispatch.RegisterCommand("tasks", &TasksCommand{})
}
```

### 3. Import for Side Effects

Import the commands package in your main or initialization code:

```go
// semspec/main.go or cmd/semspec/main.go
package main

import (
    _ "github.com/c360/semspec/commands" // Register commands via init()
)
```

## API Reference

### CommandExecutor Interface

```go
type CommandExecutor interface {
    // Execute handles the command
    Execute(ctx context.Context, cmdCtx *CommandContext, msg agentic.UserMessage, args []string, loopID string) (agentic.UserResponse, error)

    // Config returns the command configuration
    Config() CommandConfig
}
```

### CommandContext

The `CommandContext` provides access to agentic-dispatch services:

```go
type CommandContext struct {
    // NATSClient for publishing messages
    NATSClient *natsclient.Client

    // LoopTracker for tracking active loops
    LoopTracker *LoopTracker

    // Logger for structured logging
    Logger *slog.Logger

    // HasPermission checks if a user has a permission
    HasPermission func(userID, permission string) bool
}
```

### CommandConfig

```go
type CommandConfig struct {
    // Pattern is a regex pattern to match the command
    // Capture groups become args passed to Execute
    Pattern string

    // Permission required to run the command
    // Empty string means no permission required
    Permission string

    // RequireLoop indicates whether an active loop is required
    RequireLoop bool

    // Help is the text shown in /help output
    Help string
}
```

### Registration Function

```go
// RegisterCommand registers a command executor globally via init().
// Returns an error if the command name is empty or already registered.
// Panics if executor is nil (programmer error).
func RegisterCommand(name string, executor CommandExecutor) error
```

## Using CommandContext Services

### Publishing to NATS

```go
func (c *SpecCommand) Execute(ctx context.Context, cmdCtx *agenticdispatch.CommandContext, msg agentic.UserMessage, args []string, loopID string) (agentic.UserResponse, error) {
    // Publish a task to the agentic loop
    task := agentic.TaskMessage{
        LoopID: "loop_" + uuid.New().String()[:8],
        TaskID: uuid.New().String(),
        Role:   "architect",
        Prompt: "Design the spec for: " + args[0],
    }

    data, _ := json.Marshal(task)
    err := cmdCtx.NATSClient.Publish(ctx, "agent.task."+task.TaskID, data)
    if err != nil {
        return agentic.UserResponse{}, fmt.Errorf("failed to publish task: %w", err)
    }

    return agentic.UserResponse{
        Type:    agentic.ResponseTypeStatus,
        Content: fmt.Sprintf("Task submitted: %s", task.LoopID),
        // ...
    }, nil
}
```

### Tracking Loops

```go
func (c *SpecCommand) Execute(ctx context.Context, cmdCtx *agenticdispatch.CommandContext, msg agentic.UserMessage, args []string, loopID string) (agentic.UserResponse, error) {
    // Get user's active loops
    loops := cmdCtx.LoopTracker.GetUserLoops(msg.UserID)

    // Get specific loop info
    if loopID != "" {
        info := cmdCtx.LoopTracker.Get(loopID)
        if info != nil {
            // Use loop info...
        }
    }

    // Track a new loop
    cmdCtx.LoopTracker.Track(&agenticdispatch.LoopInfo{
        LoopID:      newLoopID,
        UserID:      msg.UserID,
        ChannelType: msg.ChannelType,
        ChannelID:   msg.ChannelID,
        State:       "pending",
        CreatedAt:   time.Now(),
    })

    return agentic.UserResponse{...}, nil
}
```

### Checking Permissions

```go
func (c *AdminCommand) Execute(ctx context.Context, cmdCtx *agenticdispatch.CommandContext, msg agentic.UserMessage, args []string, loopID string) (agentic.UserResponse, error) {
    // Additional permission check beyond Config().Permission
    if !cmdCtx.HasPermission(msg.UserID, "admin") {
        return agentic.UserResponse{
            Type:    agentic.ResponseTypeError,
            Content: "Admin permission required",
        }, nil
    }

    // Proceed with admin operation...
    return agentic.UserResponse{...}, nil
}
```

### Logging

```go
func (c *SpecCommand) Execute(ctx context.Context, cmdCtx *agenticdispatch.CommandContext, msg agentic.UserMessage, args []string, loopID string) (agentic.UserResponse, error) {
    cmdCtx.Logger.Info("Executing spec command",
        slog.String("user_id", msg.UserID),
        slog.String("spec_name", args[0]),
        slog.String("loop_id", loopID))

    // ...

    return agentic.UserResponse{...}, nil
}
```

## Pattern Examples

### Simple Command

```go
// /constitution - No arguments
func (c *ConstitutionCommand) Config() agenticdispatch.CommandConfig {
    return agenticdispatch.CommandConfig{
        Pattern:     `^/constitution$`,
        Permission:  "view",
        RequireLoop: false,
        Help:        "/constitution - Show project constitution",
    }
}
```

### Command with Optional Argument

```go
// /spec or /spec feature-name
func (c *SpecCommand) Config() agenticdispatch.CommandConfig {
    return agenticdispatch.CommandConfig{
        Pattern:     `^/spec\s*(.*)$`,  // Captures everything after /spec
        Permission:  "submit_task",
        RequireLoop: false,
        Help:        "/spec [name] - Run spec-driven development",
    }
}

func (c *SpecCommand) Execute(..., args []string, ...) {
    specName := ""
    if len(args) > 0 && args[0] != "" {
        specName = strings.TrimSpace(args[0])
    }
    // ...
}
```

### Command with Required Argument

```go
// /propose <description> - Requires description
func (c *ProposeCommand) Config() agenticdispatch.CommandConfig {
    return agenticdispatch.CommandConfig{
        Pattern:     `^/propose\s+(.+)$`,  // Requires at least one character after /propose
        Permission:  "submit_task",
        RequireLoop: false,
        Help:        "/propose <description> - Create new proposal",
    }
}
```

### Command with Multiple Arguments

```go
// /assign <task_id> <user_id>
func (c *AssignCommand) Config() agenticdispatch.CommandConfig {
    return agenticdispatch.CommandConfig{
        Pattern:     `^/assign\s+(\S+)\s+(\S+)$`,
        Permission:  "approve",
        RequireLoop: false,
        Help:        "/assign <task_id> <user_id> - Assign task to user",
    }
}

func (c *AssignCommand) Execute(..., args []string, ...) {
    taskID := args[0]  // First capture group
    userID := args[1]  // Second capture group
    // ...
}
```

### Command Requiring Active Loop

```go
// /feedback <text> - Requires active loop
func (c *FeedbackCommand) Config() agenticdispatch.CommandConfig {
    return agenticdispatch.CommandConfig{
        Pattern:     `^/feedback\s+(.+)$`,
        Permission:  "view",
        RequireLoop: true,  // Router will reject if no active loop
        Help:        "/feedback <text> - Add feedback to current loop",
    }
}
```

## Suggested Semspec Commands

Based on the agentic-dispatch specification, here are recommended commands for semspec:

| Command | Pattern | Permission | Description |
|---------|---------|------------|-------------|
| `/spec` | `^/spec\s*(.*)$` | `submit_task` | Start spec workflow |
| `/propose` | `^/propose\s+(.+)$` | `submit_task` | Create new proposal |
| `/review` | `^/review\s*(\S*)$` | `approve` | Review pending result |
| `/tasks` | `^/tasks(?:\s+(\w+))?$` | `view` | List tasks (optional status filter) |
| `/constitution` | `^/constitution$` | `view` | Show project constitution |
| `/explore` | `^/explore\s+(.+)$` | `submit_task` | Free exploration mode |
| `/plan` | `^/plan\s*(\S*)$` | `submit_task` | Create plan from proposal |
| `/graph` | `^/graph\s+(.+)$` | `view` | Query knowledge graph |

## Testing Commands

Commands can be tested without the full agentic-dispatch component:

```go
func TestSpecCommand(t *testing.T) {
    cmd := &SpecCommand{}

    // Test config
    config := cmd.Config()
    assert.Equal(t, "submit_task", config.Permission)
    assert.Contains(t, config.Pattern, "spec")

    // Test pattern matching
    re := regexp.MustCompile(config.Pattern)
    matches := re.FindStringSubmatch("/spec my-feature")
    assert.Equal(t, "my-feature", matches[1])

    // Test execute
    cmdCtx := &agenticdispatch.CommandContext{
        LoopTracker: agenticdispatch.NewLoopTracker(),
        Logger:      slog.Default(),
        HasPermission: func(_, _ string) bool { return true },
    }

    msg := agentic.UserMessage{
        UserID:      "test-user",
        ChannelType: "cli",
        ChannelID:   "test-session",
    }

    resp, err := cmd.Execute(context.Background(), cmdCtx, msg, []string{"my-feature"}, "")
    require.NoError(t, err)
    assert.Contains(t, resp.Content, "my-feature")
}
```

## Error Handling

Commands should return errors appropriately:

```go
func (c *SpecCommand) Execute(...) (agentic.UserResponse, error) {
    // Return error response for user-facing errors
    if args[0] == "" {
        return agentic.UserResponse{
            Type:    agentic.ResponseTypeError,
            Content: "Spec name is required. Usage: /spec <name>",
        }, nil
    }

    // Return error for system errors (will be wrapped by agentic-dispatch)
    result, err := c.doSomething()
    if err != nil {
        return agentic.UserResponse{}, fmt.Errorf("spec command failed: %w", err)
    }

    return agentic.UserResponse{...}, nil
}
```

## File Organization

Suggested file structure for semspec commands:

```
semspec/
├── commands/
│   ├── register.go      # init() registration
│   ├── spec.go          # /spec command
│   ├── propose.go       # /propose command
│   ├── review.go        # /review command
│   ├── tasks.go         # /tasks command
│   ├── constitution.go  # /constitution command
│   └── graph.go         # /graph command
└── main.go              # imports _ "semspec/commands"
```

## See Also

- [Agentic Dispatch README](../../processor/agentic-dispatch/README.md) - Component documentation
- [Agentic Dispatch doc.go](../../processor/agentic-dispatch/doc.go) - Package documentation
- [Agentic Dispatch Spec](../architecture/specs/semstreams-input-router-spec.md) - Full specification
