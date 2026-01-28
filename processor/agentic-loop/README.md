# agentic-loop

Loop orchestrator component for the agentic processing system.

## Overview

The `agentic-loop` component orchestrates autonomous agent execution by managing the lifecycle of agentic loops. It coordinates communication between the model processor (LLM calls) and tools processor (tool execution), tracks state through a 7-state machine, and captures complete execution trajectories for observability.

## Architecture

```
                         ┌─────────────────┐
    agent.task.*    ────►│                 │────► agent.request.*
                         │  agentic-loop   │
    agent.response.>◄────│                 │◄──── (from model)
                         │                 │
    tool.result.>   ────►│                 │────► tool.execute.*
                         │                 │
                         │                 │────► agent.complete.*
                         └────────┬────────┘
                                  │
                         ┌────────┴────────┐
                         │    NATS KV      │
                         │  AGENT_LOOPS    │
                         │  AGENT_TRAJ...  │
                         └─────────────────┘
```

## Features

- **State Machine**: 7-state lifecycle (exploring → complete/failed)
- **Tool Coordination**: Tracks pending tool calls, aggregates results
- **Trajectory Capture**: Records complete execution paths for debugging
- **Iteration Guards**: Configurable max iterations to prevent runaway loops
- **Architect/Editor Split**: Automatic spawning of editor from architect

## Configuration

```json
{
  "type": "processor",
  "name": "agentic-loop",
  "enabled": true,
  "config": {
    "max_iterations": 20,
    "timeout": "120s",
    "stream_name": "AGENT",
    "loops_bucket": "AGENT_LOOPS",
    "trajectories_bucket": "AGENT_TRAJECTORIES",
    "ports": {
      "inputs": [
        {"name": "agent.task", "type": "jetstream", "subject": "agent.task.*", "stream_name": "AGENT"},
        {"name": "agent.response", "type": "jetstream", "subject": "agent.response.>", "stream_name": "AGENT"},
        {"name": "tool.result", "type": "jetstream", "subject": "tool.result.>", "stream_name": "AGENT"}
      ],
      "outputs": [
        {"name": "agent.request", "type": "jetstream", "subject": "agent.request.*", "stream_name": "AGENT"},
        {"name": "tool.execute", "type": "jetstream", "subject": "tool.execute.*", "stream_name": "AGENT"},
        {"name": "agent.complete", "type": "jetstream", "subject": "agent.complete.*", "stream_name": "AGENT"}
      ]
    }
  }
}
```

### Configuration Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `max_iterations` | int | 20 | Maximum loop iterations before failure (1-1000) |
| `timeout` | string | "120s" | Loop execution timeout |
| `stream_name` | string | "AGENT" | JetStream stream name |
| `consumer_name_suffix` | string | "" | Suffix for consumer names (for testing) |
| `loops_bucket` | string | "AGENT_LOOPS" | KV bucket for loop state |
| `trajectories_bucket` | string | "AGENT_TRAJECTORIES" | KV bucket for trajectories |
| `ports` | object | (defaults) | Port configuration |

## Ports

### Inputs

| Name | Type | Subject | Description |
|------|------|---------|-------------|
| agent.task | jetstream | agent.task.* | Task requests from external systems |
| agent.response | jetstream | agent.response.> | Model responses from agentic-model |
| tool.result | jetstream | tool.result.> | Tool results from agentic-tools |

### Outputs

| Name | Type | Subject | Description |
|------|------|---------|-------------|
| agent.request | jetstream | agent.request.* | Model requests to agentic-model |
| tool.execute | jetstream | tool.execute.* | Tool calls to agentic-tools |
| agent.complete | jetstream | agent.complete.* | Loop completion events |

### KV Write

| Name | Bucket | Description |
|------|--------|-------------|
| loops | AGENT_LOOPS | Loop entity state |
| trajectories | AGENT_TRAJECTORIES | Execution trajectories |

## State Machine

```
exploring → planning → architecting → executing → reviewing → complete
                                                            ↘ failed
```

States are fluid checkpoints - loops can transition backward except from terminal states.

## KV Storage

### AGENT_LOOPS

Stores `LoopEntity` as JSON:

```json
{
  "id": "loop_123",
  "task_id": "task_456",
  "state": "executing",
  "role": "general",
  "model": "gpt-4",
  "iterations": 3,
  "max_iterations": 20
}
```

### AGENT_TRAJECTORIES

Stores `Trajectory` as JSON:

```json
{
  "loop_id": "loop_123",
  "start_time": "2024-01-15T10:30:00Z",
  "end_time": "2024-01-15T10:31:45Z",
  "steps": [...],
  "outcome": "complete",
  "total_tokens_in": 1500,
  "total_tokens_out": 800,
  "duration": 105000
}
```

## Message Formats

### TaskMessage (Input)

```json
{
  "loop_id": "optional-custom-id",
  "task_id": "task_123",
  "role": "general",
  "model": "gpt-4",
  "prompt": "Analyze this code for bugs"
}
```

### Completion Event (Output)

```json
{
  "loop_id": "loop_123",
  "task_id": "task_456",
  "outcome": "success"
}
```

## Troubleshooting

### Loop stuck waiting for response

- Check that agentic-model is running and subscribed
- Verify AGENT stream exists with correct subjects
- Check model endpoint is accessible

### Max iterations reached

- Increase `max_iterations` for complex tasks
- Check if agent is stuck in tool call loop
- Review trajectory for repeated patterns

### Missing tool results

- Verify agentic-tools is running
- Check tool executor is registered
- Ensure tool name matches exactly

## Related Components

- [agentic-model](../agentic-model/) - LLM endpoint integration
- [agentic-tools](../agentic-tools/) - Tool execution
- [agentic types](../../agentic/) - Shared type definitions
