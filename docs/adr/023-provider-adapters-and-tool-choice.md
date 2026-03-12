# ADR-023: Provider Adapters and ToolChoice Threading

## Status

Proposed

## Context

The `agentic-model` component uses the go-openai SDK as a universal client for all LLM providers. Each provider exposes an OpenAI-compatible endpoint, but none are truly compatible. Provider quirks cause 400 errors, dropped tool calls, and silent data loss.

### Current State (alpha.35)

Provider quirks are handled as inline fixes scattered across `client.go` and `handlers.go`:

| Quirk | Provider | Location | Fix Version |
|-------|----------|----------|-------------|
| Empty content on assistant tool_call messages | Gemini | client.go:142-146 | alpha.25 |
| Tool result `name` field required | Gemini | client.go:116-124 | alpha.31 |
| `reasoning_content` rejected in requests | Gemini | client.go:103-109 | alpha.25 |
| Empty-name tool calls | Gemini | handlers.go:532-554 | alpha.32 |
| Empty context after tool pair repair | Gemini | handlers.go:853 | alpha.35 |
| `reasoning_effort` parameter | OpenAI o1 | client.go:164-166 | alpha.23 |
| Missing stream delta index | Gemini | stream.go | alpha.25 |

These are hard to find, test in isolation, or extend.

### ToolChoice Gap

The OpenAI API supports `tool_choice` to control tool calling behavior (`"auto"`, `"required"`, `"none"`, or a specific function). The go-openai SDK has `ChatCompletionRequest.ToolChoice any`. Our `TaskMessage` and `AgentRequest` types do not carry this field, so callers cannot control tool calling strategy.

This matters for:
- **Workflow orchestration**: Forcing a specific tool on the first iteration (e.g., `tool_choice: {type: "function", function: {name: "read_file"}}`)
- **Structured output**: Using `"required"` to guarantee tool use instead of free text
- **Budget control**: Using `"none"` to prevent tool calls on final iterations
- **Provider workarounds**: Gemini Flash ignores `tool_choice` — the adapter can rewrite it to a prompt-level instruction

## Decision

### Part 1: Provider Adapter Interface

Define a `ProviderAdapter` interface that normalizes requests and responses per provider:

```go
// ProviderAdapter normalizes request/response payloads for a specific
// LLM provider's OpenAI-compatible endpoint.
type ProviderAdapter interface {
    Name() string
    NormalizeRequest(req *openai.ChatCompletionRequest)
    NormalizeMessages(msgs []openai.ChatCompletionMessage) []openai.ChatCompletionMessage
    NormalizeStreamDelta(delta openai.ToolCall, lastIndex int) int
    NormalizeResponse(resp *openai.ChatCompletionResponse)
}
```

Adapters are resolved from the endpoint's `provider` field in the model registry (already present). Falls back to `GenericAdapter` for unknown providers.

### Part 2: ToolChoice Threading

Add `ToolChoice` to the message chain:

```go
// agentic/types.go
type AgentRequest struct {
    // ... existing fields ...
    Tools      []ToolDefinition `json:"tools,omitempty"`
    ToolChoice *ToolChoice      `json:"tool_choice,omitempty"` // NEW
}

// agentic/user_types.go
type TaskMessage struct {
    // ... existing fields ...
    Tools      []ToolDefinition `json:"tools,omitempty"`
    ToolChoice *ToolChoice      `json:"tool_choice,omitempty"` // NEW
}
```

Define our own `ToolChoice` type (not leaking go-openai types into the agentic package):

```go
// agentic/types.go

// ToolChoice controls how the model selects tools.
// Mode is one of: "auto" (default), "required", "none", or "function".
// When Mode is "function", FunctionName specifies which function to call.
type ToolChoice struct {
    Mode         string `json:"mode"`                    // "auto", "required", "none", "function"
    FunctionName string `json:"function_name,omitempty"` // required when Mode is "function"
}
```

Threading path:

1. `HandleTask`: copy `task.ToolChoice` → `request.ToolChoice`
2. `handleToolsComplete`: copy cached ToolChoice → `request.ToolChoice`
3. `buildChatRequest`: convert `ToolChoice` → `openai.ChatCompletionRequest.ToolChoice`

Conversion in `buildChatRequest`:

```go
if req.ToolChoice != nil {
    switch req.ToolChoice.Mode {
    case "auto", "required", "none":
        chatReq.ToolChoice = req.ToolChoice.Mode
    case "function":
        chatReq.ToolChoice = openai.ToolChoice{
            Type:     openai.ToolTypeFunction,
            Function: openai.ToolFunction{Name: req.ToolChoice.FunctionName},
        }
    }
}
```

### Part 3: Adapter-Aware ToolChoice

The `GeminiAdapter` can intercept `tool_choice: "required"` and rewrite it to a prompt-level instruction since Gemini Flash ignores the parameter:

```go
func (a *GeminiAdapter) NormalizeRequest(req *openai.ChatCompletionRequest) {
    // Gemini Flash ignores tool_choice — rewrite to prompt instruction
    if req.ToolChoice == "required" && isFlashModel(req.Model) {
        req.Messages = append([]openai.ChatCompletionMessage{{
            Role:    "system",
            Content: "You MUST call one of the available tools. Do not respond with text.",
        }}, req.Messages...)
        req.ToolChoice = nil
    }
}
```

## Implementation

### Phase 1: ToolChoice Threading (small, no refactoring)

| File | Change |
|------|--------|
| `agentic/types.go` | Add `ToolChoice` struct, add field to `AgentRequest` |
| `agentic/user_types.go` | Add `ToolChoice` field to `TaskMessage` |
| `processor/agentic-loop/handlers.go` | Thread ToolChoice in `HandleTask` and `handleToolsComplete` |
| `processor/agentic-loop/state.go` | Cache ToolChoice per loop (like Tools) |
| `processor/agentic-model/client.go` | Convert ToolChoice in `buildChatRequest` |
| Tests | Unit tests for conversion, threading, nil safety |

### Phase 2: Provider Adapter Extraction (refactoring, no behavior change)

| File | Change |
|------|--------|
| `processor/agentic-model/adapter.go` | Interface + `AdapterFor()` resolver |
| `processor/agentic-model/adapter_generic.go` | Safe defaults (strip reasoning_content) |
| `processor/agentic-model/adapter_gemini.go` | All Gemini quirks consolidated |
| `processor/agentic-model/adapter_openai.go` | OpenAI-specific features |
| `processor/agentic-model/client.go` | Replace inline quirks with adapter calls |
| `processor/agentic-model/component.go` | Resolve adapter during client creation |

### Phase 3: Adapter-Aware Features

- Gemini `tool_choice` workaround (prompt rewriting)
- Schema normalization per provider
- Provider-specific token counting hints
- Anthropic native adapter (when needed)

## Consequences

### Positive

- Callers can control tool selection strategy via `TaskMessage.ToolChoice`
- Provider quirks have a single-file home per provider
- New providers are additive (write adapter, register in `AdapterFor`)
- Existing inline fixes become testable in isolation

### Negative

- Slight indirection for debugging provider-specific issues
- `ToolChoice` adds a field to the wire format (backward-compatible via omitempty)
- Adapter interface may need expansion for new quirk categories

### Mitigations

- Adapters are pure functions: no state, no config, no dependencies beyond SDK types
- `ToolChoice` is nil by default, preserving current "auto" behavior
- Phase 1 (ToolChoice) and Phase 2 (adapters) are independently valuable

## Related

- semdragon ADR-009: Provider Adapters (parallel design, same pattern)
- alpha.31-35: Gemini compatibility fixes (inline quirks this consolidates)
- ADR-022 (superseded): Workflow engine simplification
