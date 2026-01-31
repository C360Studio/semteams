# CLI Input Component

Interactive command-line interface for the SemStreams agentic system.

## Overview

The CLI input component provides terminal-based interaction with the router and agentic loops. It reads user input from stdin, publishes messages to NATS, displays responses, and handles Ctrl+C signals for loop cancellation.

## Configuration

```json
{
  "user_id": "coby",
  "session_id": "terminal-1",
  "prompt": "> ",
  "stream_name": "USER"
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `user_id` | string | `cli-user` | User identifier for permissions |
| `session_id` | string | `cli-session` | Unique session identifier |
| `prompt` | string | `> ` | Input prompt displayed to user |
| `stream_name` | string | `USER` | JetStream stream name |

## NATS Subjects

| Subject | Direction | Description |
|---------|-----------|-------------|
| `user.message.cli.{session_id}` | Publish | User input messages |
| `user.response.cli.{session_id}` | Subscribe | Responses from router |
| `user.signal.{loop_id}` | Publish | Cancel signals (Ctrl+C) |

## Local Commands

These commands are handled by the CLI component itself:

| Command | Description |
|---------|-------------|
| `/quit`, `/exit` | Exit the CLI session |
| `/clear` | Clear active loop tracking |

All other input (including `/commands`) is forwarded to the router.

## Signal Handling

| Signal | Behavior |
|--------|----------|
| Ctrl+C | Send cancel signal for active loop |
| Ctrl+D | Exit (EOF) |

If no loop is active when Ctrl+C is pressed, a message is displayed. Press Ctrl+C again to exit.

## Response Types

The CLI formats responses based on their type:

| Type | Display Format |
|------|----------------|
| `error` | `[ERROR] message` |
| `status` | `[STATUS] message` |
| `result` | `[RESULT]\nmessage` |
| `prompt` | `[PROMPT] message` with action buttons |
| `stream` | Raw content (for streaming output) |
| `text` | Plain message |

## Example Session

```
> Hello, can you help me review main.go?

[STATUS] Task submitted. Loop: loop_abc123

[RESULT]
I've reviewed main.go and found the following...

> /cancel

Cancel signal sent for loop loop_abc123

> /quit
Goodbye!
```

## Metrics

The component exports Prometheus metrics:

| Metric | Type | Description |
|--------|------|-------------|
| `cli_messages_published_total` | counter | Messages sent to router |
| `cli_responses_received_total` | counter | Responses received by type |
| `cli_signals_sent_total` | counter | Cancel signals sent |

## Integration

The CLI component works with the router component:

1. User types input
2. CLI publishes to `user.message.cli.*`
3. Router processes and publishes task or command response
4. CLI receives response on `user.response.cli.*`
5. CLI displays formatted response

See the [Router documentation](../../processor/router/README.md) for command handling details.
