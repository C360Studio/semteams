// Package router provides message routing between users and agentic loops.
//
// The router component handles command parsing, permission checking, loop
// tracking, and message dispatch. It bridges input components (CLI, Slack,
// Discord, Web) with the agentic processing system.
//
// # Architecture
//
//	┌─────────────┐                           ┌─────────────┐
//	│  CLI/Slack/ │     user.message.*        │             │
//	│  Discord/   │ ─────────────────────────▶│   Router    │
//	│  Web Input  │                           │             │
//	│             │◀───────────────────────── │  • Commands │
//	└─────────────┘     user.response.*       │  • Perms    │
//	                                          │  • Loops    │
//	                                          └──────┬──────┘
//	                                                 │
//	                                                 │ agent.task.*
//	                                                 │ agent.signal.*
//	                                                 ▼
//	                                          ┌─────────────┐
//	                                          │ agentic-    │
//	                                          │ loop        │
//	                                          └─────────────┘
//
// # Command Registration
//
// Commands can be registered in two ways:
//
// 1. Global registration via init() - preferred for reusable commands:
//
//	package mycommands
//
//	import "github.com/c360/semstreams/processor/router"
//
//	func init() {
//	    router.RegisterCommand("mycommand", &MyCommandExecutor{})
//	}
//
// 2. Per-component registration - for component-specific commands:
//
//	registry := routerComponent.CommandRegistry()
//	registry.Register("local", config, handler)
//
// # CommandExecutor Interface
//
// External commands implement the CommandExecutor interface:
//
//	type CommandExecutor interface {
//	    Execute(ctx context.Context, cmdCtx *CommandContext, msg agentic.UserMessage, args []string, loopID string) (agentic.UserResponse, error)
//	    Config() CommandConfig
//	}
//
// The CommandContext provides access to router services:
//
//	type CommandContext struct {
//	    NATSClient    *natsclient.Client       // For publishing messages
//	    LoopTracker   *LoopTracker             // For tracking loops
//	    Logger        *slog.Logger             // For logging
//	    HasPermission func(userID, permission string) bool  // For permission checks
//	}
//
// # Built-in Commands
//
// The router provides these built-in commands:
//
//   - /cancel [loop_id] - Cancel current or specified loop
//   - /status [loop_id] - Show loop status
//   - /loops - List active loops
//   - /help - Show available commands
//
// # Permissions
//
// Commands can require permissions:
//
//   - view - View status, loops, history
//   - submit_task - Submit new tasks
//   - cancel_own - Cancel own loops
//   - cancel_any - Cancel any loop (admin)
//   - approve - Approve/reject results
//
// # NATS Subjects
//
// The router uses these subject patterns:
//
//   - user.message.{channel}.{id} - Incoming user messages
//   - user.response.{channel}.{id} - Outgoing responses
//   - agent.task.{task_id} - Task dispatch to agentic-loop
//   - agent.signal.{loop_id} - Signals to agentic-loop
//   - agent.complete.{loop_id} - Completion events from agentic-loop
package router
