# SemStreams Core: Agentic Components Specification

**Version**: Draft v2  
**Status**: Planning  
**Location**: `semstreams/component/optional/`

---

## Overview

Optional components enabling any SemStreams app to run agentic loops with LLMs/SLMs.

**Key insight from research**: Aider's architect/editor split produces SOTA results. Incorporate this pattern.

---

## Components

### 1. output/model

**Purpose**: Call OpenAI-compatible model endpoints

**Location**: `semstreams/component/optional/output/model/`

**Subscribes To**: `agent.request.{request_id}`

**Publishes To**: `agent.response.{request_id}`

**Request Schema**:
```json
{
  "request_id": "req_123",
  "model": "implementer",
  "role": "architect | editor | general",
  
  "messages": [
    { "role": "system", "content": "..." },
    { "role": "user", "content": "..." }
  ],
  
  "tools": [
    {
      "type": "function",
      "function": {
        "name": "file_read",
        "description": "Read file contents",
        "parameters": { ... }
      }
    }
  ],
  
  "allowed_tools": ["file_read", "graph_query"],
  
  "config": {
    "temperature": 0.2,
    "max_tokens": 4096
  }
}
```

**Response Schema**:
```json
{
  "request_id": "req_123",
  "status": "complete | tool_call | error",
  
  "message": {
    "role": "assistant",
    "content": "..."
  },
  
  "tool_calls": [
    {
      "id": "call_456",
      "type": "function",
      "function": {
        "name": "file_read",
        "arguments": "{\"path\": \"auth/token.go\"}"
      }
    }
  ],
  
  "usage": {
    "prompt_tokens": 1523,
    "completion_tokens": 892
  }
}
```

**Configuration**:
```json
{
  "component": {
    "type": "output",
    "name": "model",
    "config": {
      "endpoints": {
        "qwen-32b": {
          "url": "http://localhost:11434/v1/chat/completions",
          "provider": "openai-compatible",
          "model": "qwen2.5-coder:32b"
        },
        "deepseek-16b": {
          "url": "http://localhost:11434/v1/chat/completions",
          "provider": "openai-compatible",
          "model": "deepseek-coder-v2:16b"
        },
        "litellm": {
          "url": "http://localhost:4000/v1/chat/completions",
          "provider": "openai-compatible",
          "api_key_env": "LITELLM_API_KEY"
        }
      },
      "timeout": "120s",
      "retry": {
        "max_attempts": 3,
        "backoff": "exponential"
      }
    }
  }
}
```

**Implementation Notes**:
- All providers use OpenAI-compatible API (unified client)
- Ollama serves OpenAI-compatible endpoint at /v1
- LiteLLM can proxy to multiple backends
- No provider-specific code needed

---

### 2. rules/agentic

**Purpose**: Orchestrate agentic loop state machine

**Location**: `semstreams/component/optional/rules/agentic/`

**Key insight**: Fluid states, not rigid phases. (From OpenSpec OPSX research)

**State Machine**:

```
                           ┌──────────────────┐
                           │                  │
      ┌───────────────────►│    EXPLORING     │◄──────┐
      │                    │   (optional)     │       │
      │                    └────────┬─────────┘       │
      │                             │                 │
      │                             ▼                 │
      │                    ┌──────────────────┐       │
      │                    │                  │       │
      │         ┌─────────►│    PLANNING      │───────┤
      │         │          │                  │       │
      │         │          └────────┬─────────┘       │
      │         │                   │                 │
      │         │                   ▼                 │
      │         │          ┌──────────────────┐       │
      │         │          │                  │       │
      │         └──────────│   ARCHITECTING   │───────┤
      │                    │   (optional)     │       │
      │                    └────────┬─────────┘       │
      │                             │                 │
      │                             ▼                 │
      │                    ┌──────────────────┐       │
      │                    │                  │       │
      │ (rethink)          │    EXECUTING     │───────┘
      │                    │                  │  (blocked)
      │                    └────────┬─────────┘
      │                             │
      │                             ▼
      │                    ┌──────────────────┐
      └────────────────────│    REVIEWING     │
           (needs work)    │                  │
                           └────────┬─────────┘
                                    │
                                    ▼
                           ┌──────────────────┐
                           │                  │
                           │    COMPLETE      │
                           │                  │
                           └──────────────────┘
```

**Entity Schema**:
```json
{
  "id": "agent_loop:{id}",
  "type": "agent_loop",
  
  "task": "task:{id}",
  "role": "planner | architect | implementer",
  
  "state": "exploring | planning | architecting | executing | reviewing | complete | failed",
  
  "messages": [...],
  "tool_calls_pending": [...],
  "tool_results": [...],
  
  "iterations": 0,
  "max_iterations": 20,
  
  "allowed_tools": ["graph_query", "file_read"],
  
  "created_at": "...",
  "updated_at": "..."
}
```

**Fluid Transitions**:
- No enforced sequence
- Can move backward (executing → exploring)
- Can skip phases (exploring → executing for simple tasks)
- Checkpoints, not gates

**Rules**:

```yaml
# Model returned tool call
- name: handle-tool-call
  when:
    event: agent.response.*
    condition: "status == 'tool_call'"
  then:
    - validate: "all tools in allowed_tools"
    - for_each: tool_calls
      publish:
        subject: "tool.execute.{name}"
    - update_entity:
        status: executing

# Tool complete - continue
- name: handle-tool-result
  when:
    event: tool.result.*
    condition: "all_tools_complete"
  then:
    - append_messages: tool_results
    - publish: agent.request.{id}
    
# Model complete - check if architect/editor split
- name: handle-architect-complete
  when:
    event: agent.response.*
    condition: "role == 'architect' AND status == 'complete'"
  then:
    - create: new agent_loop for editor
    - update: state = architecting_complete

# Final completion
- name: handle-final-complete
  when:
    event: agent.response.*
    condition: "role != 'architect' AND status == 'complete'"
  then:
    - update: state = complete
    - publish: agent.complete.{id}

# Backward transition (rethink)
- name: allow-backward
  when:
    event: agent.rethink.*
  then:
    - update: state = exploring
    - clear: pending tool calls
```

---

### 3. Architect/Editor Pattern

**Insight from Aider research**: Separating "describe solution" from "write edits" produces better results.

**Implementation**:

```
┌─────────────────────────────────────────────────────────────────────┐
│                                                                      │
│  ARCHITECT PHASE                                                    │
│                                                                      │
│  Model: Higher capability (Claude)                                  │
│  Output: Description of changes needed                              │
│  Tools: Read-only (graph_query, ast_query, file_read)              │
│                                                                      │
│  Prompt:                                                            │
│  "Analyze this task and describe exactly what changes are needed.  │
│   Do not write code. Describe files to modify, functions to add,   │
│   logic to implement."                                              │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
                              │
                              │ architecture description
                              ▼
┌─────────────────────────────────────────────────────────────────────┐
│                                                                      │
│  EDITOR PHASE                                                       │
│                                                                      │
│  Model: Can be smaller (tuned SLM)                                 │
│  Input: Architecture description + file contents                    │
│  Output: Actual file edits                                          │
│  Tools: file_read, file_write, git_*                               │
│                                                                      │
│  Prompt:                                                            │
│  "Implement the following changes as described by the architect:   │
│   [architecture description]                                        │
│   Write the actual code changes."                                  │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

**When to use**:
- Complex changes spanning multiple files
- When precision matters more than speed
- For training data (architect description + editor output = good pairs)

**When to skip**:
- Simple single-file changes
- Quick fixes
- Exploration mode

**Configuration**:
```json
{
  "architect_editor": {
    "enabled": true,
    "threshold": "multi_file",
    "architect_model": "qwen2.5-coder:32b",
    "editor_model": "qwen2.5-coder:32b"
  }
}
```

**Note**: With local models, both roles can use the same model. The split is about *prompting strategy*, not model capability. Architect prompt focuses on planning, editor prompt focuses on code generation.

---

### 4. Trajectory Capture

**Purpose**: Store everything for training

**Location**: Integrated into rules/agentic

**Captured per loop**:
```json
{
  "loop_id": "agent_loop:123",
  "task_id": "task:456",
  "role": "implementer",
  
  "trajectory": [
    {
      "step": 1,
      "type": "model_call",
      "model": "implementer-7b",
      "input_tokens": 1523,
      "output_tokens": 892,
      "messages_in": [...],
      "message_out": {...}
    },
    {
      "step": 2,
      "type": "tool_call",
      "tool": "file_read",
      "args": {"path": "auth/token.go"},
      "result_size": 2341
    },
    {
      "step": 3,
      "type": "model_call",
      "model": "implementer-7b",
      "input_tokens": 3864,
      "output_tokens": 1247,
      "messages_in": [...],
      "message_out": {...}
    }
  ],
  
  "total_tokens_in": 5387,
  "total_tokens_out": 2139,
  "duration_ms": 12453,
  
  "outcome": "complete | failed",
  "error": null
}
```

**Storage**: ObjectStore (can be large)

---

## App Integration

### What App Provides

```go
// App implements tool executor
type ToolExecutor interface {
    Execute(call ToolCall) (ToolResult, error)
    ListTools() []ToolDefinition
}

// App registers tools
func (app *Semspec) RegisterTools(executor *agentic.Executor) {
    executor.Register("graph_query", app.graphQuery)
    executor.Register("ast_query", app.astQuery)
    executor.Register("file_read", app.fileRead)
    executor.Register("file_write", app.fileWrite)
    executor.Register("git_commit", app.gitCommit)
}
```

### What Core Provides

```go
// Core provides loop orchestration
type AgenticLoop struct {
    modelOutput *output.Model
    rules       *rules.Agentic
    entityStore *EntityStore
}

func (a *AgenticLoop) Run(request AgentRequest) (*AgentResult, error) {
    // 1. Create loop entity
    // 2. Call model
    // 3. Handle tool calls (via app's executor)
    // 4. Continue until complete
    // 5. Return result with trajectory
}
```

---

## Configuration Reference

```json
{
  "agentic": {
    "max_iterations": 20,
    "tool_timeout": "60s",
    "parallel_tools": true,
    
    "architect_editor": {
      "enabled": true,
      "threshold": "multi_file",
      "architect_model": "claude-sonnet",
      "editor_model": "implementer"
    },
    
    "trajectory_capture": {
      "enabled": true,
      "store": "objectstore",
      "retention_days": 90
    },
    
    "fluid_transitions": {
      "allow_backward": true,
      "require_checkpoints": ["review"]
    }
  }
}
```

---

## Testing

### Unit Tests
- output/model: Mock HTTP, verify request/response
- rules/agentic: State transitions, tool validation

### Integration Tests
- Full loop with mock model
- Architect/editor split
- Trajectory capture
- Backward transitions

### End-to-End Tests
- Real model (Ollama) with simple task
- Multi-step with tool calls
- Error handling and recovery

---

## Comparison to v1 Spec

| v1 | v2 |
|----|-----|
| Rigid state machine | Fluid transitions |
| Single model | Architect/editor split |
| Basic trajectory | Full capture with tokens |
| Phase gates | Checkpoints |
