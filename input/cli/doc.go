// Package cli provides a CLI input component for interactive user sessions.
//
// The CLI input component reads from stdin, publishes user messages to the router
// via NATS, displays responses, and handles Ctrl+C signals for loop cancellation.
//
// # Architecture
//
// The CLI component is an input component that bridges terminal interaction with
// the agentic system:
//
//	┌─────────────┐     user.message.cli.*     ┌────────────┐
//	│             │ ──────────────────────────▶│            │
//	│  CLI Input  │                            │   Router   │
//	│             │◀────────────────────────── │            │
//	└─────────────┘     user.response.cli.*    └────────────┘
//	      │
//	      │ Ctrl+C
//	      ▼
//	user.signal.{loop_id}
//
// # NATS Subjects
//
// The component uses three subject patterns:
//
//   - user.message.cli.{session_id} - Published when user enters text
//   - user.response.cli.{session_id} - Subscribed for router responses
//   - user.signal.{loop_id} - Published on Ctrl+C to cancel active loop
//
// # Local Commands
//
// Some commands are handled locally without routing to the server:
//
//   - /quit, /exit - Exit the CLI session
//   - /clear - Clear the active loop tracking
//
// All other input (including /commands) is published to the router.
//
// # Configuration
//
// The component is configured via JSON:
//
//	{
//	  "user_id": "coby",
//	  "session_id": "terminal-1",
//	  "prompt": "> ",
//	  "stream_name": "USER"
//	}
//
// # Signal Handling
//
// Pressing Ctrl+C sends a cancel signal for the currently active loop. If no
// loop is active, it displays a message. Press Ctrl+C twice to exit when no
// loop is running.
//
// # Example Usage
//
// The CLI component is typically created via the component registry:
//
//	config := json.RawMessage(`{
//	  "user_id": "coby",
//	  "session_id": "session-001"
//	}`)
//	comp, err := cli.NewComponent(config, deps)
//	if err != nil {
//	  log.Fatal(err)
//	}
//	comp.Start(ctx)
package cli
