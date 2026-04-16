// Package teamsdispatch provides message routing between users and agentic loops.
//
// The agentic-dispatch component handles command parsing, permission checking, loop
// tracking, and message dispatch. It bridges input components (CLI, Slack,
// Discord, Web) with the agentic processing system.
//
// # Architecture
//
//	┌─────────────┐                              ┌─────────────────┐
//	│  CLI/Slack/ │  teams.user.message.*        │                 │
//	│  Discord/   │ ────────────────────────────▶│ teams-dispatch  │
//	│  Web Input  │                              │                 │
//	│             │◀──────────────────────────── │  • Commands     │
//	└─────────────┘  teams.user.response.*       │  • Perms        │
//	                                             │  • Loops        │
//	                                             └────────┬────────┘
//	                                                      │
//	                                                      │ teams.task.*
//	                                                      │ teams.signal.*
//	                                                      ▼
//	                                             ┌─────────────┐
//	                                             │ teams-loop  │
//	                                             └─────────────┘
//
// # Command Registration
//
// Commands can be registered in two ways:
//
// 1. Global registration via init() - preferred for reusable commands:
//
//	package mycommands
//
//	import teamsdispatch "github.com/c360studio/semteams/processor/teams-dispatch"
//
//	func init() {
//	    teamsdispatch.RegisterCommand("mycommand", &MyCommandExecutor{})
//	}
//
// 2. Per-component registration - for component-specific commands:
//
//	registry := dispatchComponent.CommandRegistry()
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
// The CommandContext provides access to dispatch services:
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
// The agentic-dispatch component provides these built-in commands:
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
// Default subject patterns (resolved from port config — overridable per deployment):
//
//   - teams.user.message.> - Incoming user messages
//   - teams.user.response.{channel}.{id} - Outgoing responses
//   - teams.task.{task_id} - Task dispatch to teams-loop
//   - teams.signal.{loop_id} - Control signals to teams-loop
//   - teams.complete.{loop_id} - Completion events from teams-loop
//   - teams.created.{loop_id} - Loop creation events (workflow context sync)
//   - teams.failed.{loop_id} - Loop failure events
//
// # JetStream Integration
//
// All messaging uses JetStream for durability. Subject namespace and stream name
// are driven by the port configuration, not hardcoded. This allows multiple
// dispatch instances with different subject namespaces to coexist in one deployment.
//
// Default stream is TEAMS. Consumer names follow the pattern: teams-dispatch-{port-name}.
package teamsdispatch
