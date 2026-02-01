# Context Memory Management Technical Specification

**Version**: 1.0
**Status**: Implementation Ready
**ADR**: [ADR-015: Context Memory Management](../adr-015-context-memory-management.md)
**Component**: `processor/agentic-loop`

---

## Overview

Context memory management brings operating system-style memory management principles to LLM context windows. This
specification defines how `processor/agentic-loop` will track, allocate, compact, and garbage-collect context memory
to ensure optimal performance and prevent context overflow.

**Core principle**: Treat the LLM context window as managed virtual memory with explicit regions, utilization
tracking, and automatic compaction.

**Key insight from ADR-015**: LLM performance degrades non-linearly with context utilization. Research shows
attention thrashing begins at 60% utilization, making proactive compaction essential for maintaining quality.

### Design Goals

1. **Predictable Performance**: Operate in the optimal utilization range (0-60%)
2. **Avoid Crashes**: Never exceed model context limits
3. **Transparency**: Full visibility into token usage per region
4. **Automatic Management**: No manual intervention required for context overflow
5. **Cost Control**: Reduce token usage through intelligent eviction and compaction

### Relationship to ADR-015

This specification implements all technical decisions from ADR-015:

- Memory region model with six distinct zones
- 60% compaction threshold based on attention thrashing research
- Per-model context limit configuration
- Structured compaction preserving key facts
- Garbage collection for tool results
- NATS event publishing for observability

---

## Memory Region Model

The context window is divided into six distinct regions, each with specific characteristics, priorities, and
management strategies.

```text
┌─────────────────────────────────────────────────────┐
│ SYSTEM PROMPT (fixed, ~2-4K tokens)                 │ ← Never evicted
├─────────────────────────────────────────────────────┤
│ COMPACTED HISTORY (summary, ~5-20K tokens)          │ ← Grows via compaction
├─────────────────────────────────────────────────────┤
│ RECENT HISTORY (rolling, ~10-40K tokens)            │ ← Oldest evicted first
├─────────────────────────────────────────────────────┤
│ TOOL RESULTS (temporary, ~20-60K tokens)            │ ← Aged out after N iterations
├─────────────────────────────────────────────────────┤
│ RETRIEVED DOCS (pinned, ~10-20K tokens)             │ ← Released after use
├─────────────────────────────────────────────────────┤
│ HEADROOM (reserved, ~5-10K tokens)                  │ ← Never allocated
└─────────────────────────────────────────────────────┘
```

### Region Definitions

#### 1. System Prompt Region

**Purpose**: Static role instructions, tool definitions, and task context.

**Characteristics**:

- Fixed content set at loop initialization
- Pinned permanently (never evicted)
- Typically 2,000-4,000 tokens
- Priority: 1 (highest, never evicted)

**Contents**:

- Agent role definition (architect/editor/general)
- Tool function signatures and descriptions
- Task-specific instructions
- Output format requirements

#### 2. Compacted History Region

**Purpose**: Structured summaries of evicted conversation history.

**Characteristics**:

- Grows via compaction operations
- Pinned (never evicted)
- Target: 5,000-20,000 tokens (configurable)
- Priority: 2 (never evicted)

**Contents**:

- Key decisions made in earlier iterations
- Files modified with change summaries
- State checkpoints (iteration markers)
- Tool usage statistics
- Critical facts extracted from old messages

**Format**:

```text
Compacted history (iterations 1-5):

Key decisions:
- Used visitor pattern for graph traversal (iteration 2)
- Chose BM25 over TF-IDF for keyword extraction (iteration 3)

Files modified:
- processor/graph/node.go (bug fix, iteration 2)
- processor/keywords/bm25.go (feature add, iteration 3)

State checkpoints:
- Iteration 3: analyzing requirements
- Iteration 5: reviewing implementation

Tool usage:
- graph_query: 12 calls, 10 successful
- file_read: 8 calls, 8 successful
```

#### 3. Recent History Region

**Purpose**: Recent conversation messages (user inputs, assistant responses).

**Characteristics**:

- Rolling window of last N messages
- Oldest messages evicted first during compaction
- Target: 10,000-40,000 tokens (configurable)
- Priority: 3 (evicted to compacted history)

**Contents**:

- Last 10-20 messages (configurable)
- Both user and assistant messages
- Assistant messages without tool calls (tool responses in separate region)

**Management**:

- New messages added to tail
- When compaction triggers, oldest messages moved to compaction buffer
- Only text messages stored here (tool-related messages in Tool Results region)

#### 4. Tool Results Region

**Purpose**: Temporary storage for recent tool execution results.

**Characteristics**:

- Temporary, aged out after N iterations
- Largest region by token allocation
- Target: 20,000-60,000 tokens (configurable)
- Priority: 4 (evicted first during pressure)

**Contents**:

- Tool call messages (role: "assistant" with tool_calls)
- Tool result messages (role: "tool" with results)
- Associated metadata (iteration added, tool name, result size)

**Lifecycle**:

```go
type ToolResultEntry struct {
    ToolCallMessage ChatMessage  // Assistant message with tool_calls
    ToolResultMessage ChatMessage // Tool message with result
    IterationAdded  int           // When this was added
    ToolName        string        // Name of the tool called
    SizeTokens      int           // Token count for these messages
}
```

#### 5. Retrieved Docs Region

**Purpose**: Context retrieved from knowledge graph (future PathRAG integration).

**Characteristics**:

- Pinned during active use
- Released when task phase completes
- Target: 10,000-20,000 tokens (configurable)
- Priority: 5 (evicted when unpinned)

**Contents**:

- Entity summaries from graph queries
- Code snippets from AST queries
- Documentation fragments
- Related context for current task

**Management**:

- Pinned flag controlled by loop state transitions
- Automatically unpinned when moving to next major state
- Can be manually released by model request

#### 6. Headroom Region

**Purpose**: Reserved space for model response generation.

**Characteristics**:

- Never allocated to messages
- Prevents context overflow during response
- Size: 5,000-10,000 tokens (model-dependent)
- Priority: N/A (reservation, not storage)

**Calculation**:

```text
Available for allocation = Model limit - Headroom
Compaction trigger = (Model limit - Headroom) * 0.6
```

### Eviction Priority Order

When compaction is needed, regions are evicted in this order:

1. **Old tool results** (age > N iterations) - evicted first
2. **Old chat history** (moved to compacted summary) - second
3. **Retrieved docs** (after task phase completion) - third
4. **Recent history** (last resort, keep minimum 3 messages) - fourth
5. **Compacted history** - never evicted
6. **System prompt** - never evicted

---

## ContextManager Interface

The `ContextManager` struct provides the primary API for managing context memory within the agentic loop.

### Core Structure

```go
// ContextManager manages context window utilization and compaction.
type ContextManager struct {
    // Configuration
    modelName         string
    maxTokens         int     // Model's total context window
    headroomTokens    int     // Reserved for response generation
    compactThreshold  float64 // Utilization percentage to trigger compaction

    // Region management
    regions           map[RegionType]*RegionState

    // Token counting
    tokenizer         Tokenizer

    // State tracking
    currentIteration  int
    mu                sync.RWMutex
}

// RegionType identifies different context regions.
type RegionType string

const (
    RegionSystemPrompt   RegionType = "system_prompt"
    RegionCompactedHist  RegionType = "compacted_history"
    RegionRecentHist     RegionType = "recent_history"
    RegionToolResults    RegionType = "tool_results"
    RegionRetrievedDocs  RegionType = "retrieved_docs"
)

// RegionState tracks the state of a single memory region.
type RegionState struct {
    Type         RegionType
    MaxTokens    int                // Region-specific token limit
    CurrentTokens int               // Actual tokens currently allocated
    Priority     int                // Eviction priority (1 = never evict)
    Pinned       bool               // If true, never evict
    Messages     []ChatMessage      // Messages in this region
    ToolResults  []ToolResultEntry  // Tool results (for RegionToolResults only)
    LastUpdated  time.Time          // Last modification time
}
```

### Public Methods

#### Initialization

```go
// NewContextManager creates a new context manager for the specified model.
func NewContextManager(modelName string, config ContextConfig) (*ContextManager, error)
```

**Purpose**: Initialize context manager with model-specific limits and region configuration.

**Parameters**:

- `modelName`: LLM identifier (e.g., "gpt-4o", "claude-sonnet-4.5")
- `config`: Model limits and region allocations from configuration

**Returns**: Configured ContextManager ready for use.

**Errors**: Returns error if model not found in config or invalid region allocations.

#### Utilization Tracking

```go
// Utilization returns current context utilization as a ratio (0.0-1.0).
func (cm *ContextManager) Utilization() float64

// ShouldCompact returns true if utilization exceeds compaction threshold.
func (cm *ContextManager) ShouldCompact() bool

// UtilizationByRegion returns token counts for each region.
func (cm *ContextManager) UtilizationByRegion() map[RegionType]int
```

**Utilization calculation**:

```text
utilization = (sum of all region tokens) / (maxTokens - headroomTokens)
```

**Compaction trigger**:

```text
shouldCompact = (utilization >= compactThreshold)
```

#### Message Management

```go
// AddMessage adds a message to the appropriate region based on its role and content.
func (cm *ContextManager) AddMessage(msg ChatMessage) error

// AddToolResult adds a tool call and its result to the tool results region.
func (cm *ContextManager) AddToolResult(callMsg, resultMsg ChatMessage) error

// GetContext returns all messages to include in the next model request, ordered correctly.
func (cm *ContextManager) GetContext() []ChatMessage
```

**AddMessage logic**:

- System messages → `RegionSystemPrompt`
- User/assistant text messages → `RegionRecentHist`
- Messages are tokenized and counted
- Region `CurrentTokens` updated
- Error if region exceeds `MaxTokens`

**GetContext ordering**:

1. System prompt messages
2. Compacted history summary
3. Recent history messages (chronological)
4. Tool results (chronological)
5. Retrieved docs

#### Compaction Operations

```go
// Compact executes context compaction to reduce utilization.
// Returns CompactionResult with metrics and evicted content.
func (cm *ContextManager) Compact(ctx context.Context, modelClient ModelClient) (*CompactionResult, error)

// CompactionResult holds the outcome of a compaction operation.
type CompactionResult struct {
    BeforeUtilization float64
    AfterUtilization  float64
    TokensReclaimed   int
    MessagesEvicted   int
    ToolResultsEvicted int
    Summary           string // The compacted summary text
    DurationMs        int64
}
```

**Compaction process** (detailed in Compaction Flow section):

1. Identify eviction candidates by priority
2. Extract messages to summarize
3. Generate structured summary via model call
4. Replace old messages with summary
5. Update region states
6. Return metrics

#### Garbage Collection

```go
// GarbageCollect removes old tool results based on age and size policies.
func (cm *ContextManager) GarbageCollect() *GCResult

// GCResult holds the outcome of garbage collection.
type GCResult struct {
    ToolResultsRemoved int
    TokensReclaimed    int
    EvictedByAge       int
    EvictedBySize      int
}
```

**GC policies**:

- Age-based: Remove tool results older than `maxAgeIterations` (default: 3)
- Size-based: If tool results region exceeds limit, evict oldest first
- Always keep last result for each tool type

#### Iteration Tracking

```go
// IncrementIteration updates the current iteration counter.
// Used by GC to determine tool result age.
func (cm *ContextManager) IncrementIteration()

// CurrentIteration returns the current iteration number.
func (cm *ContextManager) CurrentIteration() int
```

---

## Compaction Flow

Compaction is triggered when utilization exceeds the configured threshold (default: 60%). The process reclaims
context space while preserving critical information.

### Compaction Trigger Conditions

Any of the following triggers compaction:

1. **Utilization threshold**: `Utilization() >= compactThreshold`
2. **Iteration threshold**: `currentIteration >= iterationCompactThreshold` (default: 10)
3. **Explicit request**: Manual compaction call (for testing or special cases)

### Compaction Algorithm

```text
┌─────────────────────────────────────────────────────────────────┐
│                   Compaction Workflow                            │
│                                                                  │
│  1. MEASURE                                                      │
│     - Count tokens in each region                               │
│     - Calculate current utilization                             │
│     - Identify eviction candidates by priority                  │
│     - If utilization < threshold, return early                  │
│                                                                  │
│  2. EVICT TOOL RESULTS (Priority 4)                             │
│     - Run GarbageCollect() to age out old tool results         │
│     - Remove tool results older than maxAgeIterations          │
│     - If still over threshold, evict largest results first      │
│                                                                  │
│  3. EVICT RETRIEVED DOCS (Priority 5, if unpinned)              │
│     - Check if retrieved docs region is pinned                  │
│     - If unpinned and utilization still high, clear region      │
│                                                                  │
│  4. COMPACT RECENT HISTORY (Priority 3)                         │
│     - Extract oldest N messages from recent history             │
│     - Generate structured summary via model call                │
│     - Append summary to compacted history region                │
│     - Remove evicted messages from recent history               │
│                                                                  │
│  5. VERIFY                                                       │
│     - Re-measure token utilization                              │
│     - Ensure utilization below safe threshold (< 50%)           │
│     - If still high, repeat steps 2-4 more aggressively         │
│     - Log warning if can't reclaim sufficient space             │
│                                                                  │
│  6. EMIT EVENT                                                   │
│     - Publish compaction event to NATS with metrics             │
│     - Update Prometheus metrics                                 │
│     - Return CompactionResult                                   │
└─────────────────────────────────────────────────────────────────┘
```

### Compaction Message Selection

When compacting recent history, messages are selected using this strategy:

**Selection criteria**:

- Keep minimum 3 most recent messages (never compact away current context)
- Target: Evict enough messages to bring utilization below 40%
- Select oldest messages first (FIFO eviction)
- Group related messages (user question + assistant answer as atomic pair)

**Example**:

```text
Recent history before compaction (15 messages, 35K tokens):
- Messages 1-5: Early exploration (10K tokens)
- Messages 6-10: Architecture decisions (15K tokens)
- Messages 11-15: Current implementation (10K tokens)

Compaction decision:
- Evict messages 1-10 (25K tokens)
- Keep messages 11-15 (current context)
- Generate summary of messages 1-10
```

### Summarization Prompt

The model is asked to generate a structured summary of evicted messages:

```text
You are summarizing conversation history for context management. Extract key information from these messages:

[Messages to summarize]

Generate a structured summary with these sections:

1. Key decisions: Important choices made and their rationale
2. Files modified: List of files changed with brief description
3. State checkpoints: Progress markers at specific iterations
4. Tool usage: Summary of tools used and success rates

Be concise but preserve critical details. This summary will replace the full messages in context.
```

### Compaction Cost

Each compaction requires an LLM call to generate the summary:

**Token cost**:

- Input: Messages to summarize (~10-20K tokens)
- Output: Structured summary (~1-2K tokens)
- Cost ratio: ~10:1 compression

**Mitigation**:

- Compaction threshold (60%) means it only happens when necessary
- Summaries are reused across iterations (cumulative compaction)
- Alternative: Can use cheaper model for summarization (future enhancement)

---

## Token Counting

Accurate token counting is essential for effective context management. The tokenizer interface abstracts
model-specific token counting implementations.

### Tokenizer Interface

```go
// Tokenizer provides model-specific token counting.
type Tokenizer interface {
    // CountTokens returns the token count for the given text.
    CountTokens(text string) int

    // CountMessage returns the token count for a ChatMessage,
    // accounting for role tokens and formatting overhead.
    CountMessage(msg ChatMessage) int

    // CountMessages returns the total token count for multiple messages.
    CountMessages(messages []ChatMessage) int
}
```

### Implementation Strategies

#### 1. Tiktoken (OpenAI Models)

For GPT-4, GPT-3.5, and OpenAI-compatible models:

```go
import "github.com/pkoukk/tiktoken-go"

type TiktokenTokenizer struct {
    encoding *tiktoken.Tiktoken
}

func NewTiktokenTokenizer(model string) (*TiktokenTokenizer, error) {
    encoding, err := tiktoken.EncodingForModel(model)
    if err != nil {
        return nil, fmt.Errorf("failed to get encoding for model %s: %w", model, err)
    }
    return &TiktokenTokenizer{encoding: encoding}, nil
}

func (t *TiktokenTokenizer) CountTokens(text string) int {
    tokens := t.encoding.Encode(text, nil, nil)
    return len(tokens)
}

func (t *TiktokenTokenizer) CountMessage(msg ChatMessage) int {
    // OpenAI message overhead: ~4 tokens per message
    tokens := 4
    tokens += t.CountTokens(msg.Role)
    tokens += t.CountTokens(msg.Content)

    // Tool calls have additional overhead
    for _, tc := range msg.ToolCalls {
        tokens += t.CountTokens(tc.Function.Name)
        tokens += t.CountTokens(tc.Function.Arguments)
    }

    return tokens
}
```

#### 2. Anthropic Claude Models

For Claude models, use Anthropic's token counting API or estimator:

```go
type ClaudeTokenizer struct {
    client *anthropic.Client
}

func (t *ClaudeTokenizer) CountTokens(text string) int {
    // Option 1: Use Anthropic's count_tokens API (requires network call)
    // Option 2: Use local estimator (~3.5 chars per token for English)
    return len(text) / 3.5 // Rough estimation
}
```

**Note**: Claude tokenization is less critical since Claude models have larger context windows (200K tokens) and
can tolerate more estimation error.

#### 3. Local/Ollama Models

For local models served via Ollama:

```go
type OllamaTokenizer struct {
    model string
}

func (t *OllamaTokenizer) CountTokens(text string) int {
    // Many local models use similar tokenizers to GPT models
    // Fall back to tiktoken with appropriate encoding
    // Or use character-based estimation
    return len(text) / 4 // Conservative estimate
}
```

### Token Counting Caching

To reduce CPU overhead, implement token count caching:

```go
type CachedTokenizer struct {
    underlying Tokenizer
    cache      map[string]int
    maxSize    int
    mu         sync.RWMutex
}

func (c *CachedTokenizer) CountTokens(text string) int {
    c.mu.RLock()
    if count, ok := c.cache[text]; ok {
        c.mu.RUnlock()
        return count
    }
    c.mu.RUnlock()

    count := c.underlying.CountTokens(text)

    c.mu.Lock()
    if len(c.cache) < c.maxSize {
        c.cache[text] = count
    }
    c.mu.Unlock()

    return count
}
```

**Cache policy**:

- LRU eviction when cache exceeds max size (default: 1000 entries)
- Cache keyed by message content hash
- Thread-safe with RWMutex

---

## Tool Result Garbage Collection

Tool results consume the most context space and have natural expiration (results become stale after N iterations).
GC reclaims this space automatically.

### GC Strategy

Two complementary policies:

#### 1. Age-Based Eviction

**Policy**: Remove tool results older than `maxAgeIterations`.

**Rationale**: Tool results are most relevant immediately after execution. After several iterations, the model has
likely processed the results and no longer needs the raw data.

**Default**: 3 iterations

**Example**:

```text
Current iteration: 8
Tool result added at iteration 3
Age: 8 - 3 = 5 iterations
Action: Evict (age > 3)
```

#### 2. Size-Based Eviction

**Policy**: If tool results region exceeds `MaxTokens`, evict largest results first until under limit.

**Rationale**: Some tool results are very large (e.g., file reads). When region is full, prioritize keeping smaller,
more recent results.

**Example**:

```text
Tool results region: 65K tokens (limit: 60K)
Results by size:
- file_read "large_file.go": 30K tokens, iteration 5
- graph_query "find_nodes": 5K tokens, iteration 6
- file_read "config.json": 25K tokens, iteration 7

Action: Evict "large_file.go" (largest and oldest)
Result: 35K tokens remaining
```

### GC Exceptions

Always keep:

- Results from the last iteration (age = 0)
- The most recent result for each unique tool name
- Results referenced in compacted history summary

### GC Timing

GC runs automatically:

1. **Before each compaction**: Ensures tool results don't prevent successful compaction
2. **After each iteration**: Proactive cleanup before utilization grows
3. **On explicit request**: For testing or manual cleanup

### GC Implementation

```go
func (cm *ContextManager) GarbageCollect() *GCResult {
    region := cm.regions[RegionToolResults]
    result := &GCResult{}

    evicted := []ToolResultEntry{}
    kept := []ToolResultEntry{}

    // Age-based eviction
    for _, entry := range region.ToolResults {
        age := cm.currentIteration - entry.IterationAdded
        if age > cm.config.MaxToolResultAge {
            evicted = append(evicted, entry)
            result.EvictedByAge++
            result.TokensReclaimed += entry.SizeTokens
        } else {
            kept = append(kept, entry)
        }
    }

    // Size-based eviction (if still over limit)
    if cm.sumTokens(kept) > region.MaxTokens {
        // Sort by size descending, then by age descending
        sort.Slice(kept, func(i, j int) bool {
            if kept[i].SizeTokens != kept[j].SizeTokens {
                return kept[i].SizeTokens > kept[j].SizeTokens
            }
            return kept[i].IterationAdded < kept[j].IterationAdded
        })

        // Keep evicting largest until under limit
        final := []ToolResultEntry{}
        tokens := 0
        for _, entry := range kept {
            if tokens + entry.SizeTokens <= region.MaxTokens {
                final = append(final, entry)
                tokens += entry.SizeTokens
            } else {
                evicted = append(evicted, entry)
                result.EvictedBySize++
                result.TokensReclaimed += entry.SizeTokens
            }
        }
        kept = final
    }

    // Update region
    region.ToolResults = kept
    region.CurrentTokens = cm.sumTokens(kept)
    result.ToolResultsRemoved = len(evicted)

    return result
}
```

---

## Model Configuration

Context management is highly model-specific. Configuration defines limits, thresholds, and region allocations for
each supported model.

### Configuration Structure

```yaml
context_memory:
  enabled: true
  default_compact_threshold: 0.6

  models:
    gpt-4o:
      max_tokens: 128000
      headroom: 8000
      compact_threshold: 0.6
      regions:
        system_prompt:
          max_tokens: 4000
        compacted_history:
          max_tokens: 20000
        recent_history:
          max_tokens: 40000
          messages_to_keep: 10
        tool_results:
          max_tokens: 60000
          max_age_iterations: 3
        retrieved_docs:
          max_tokens: 20000

    claude-3-sonnet:
      max_tokens: 200000
      headroom: 10000
      compact_threshold: 0.6
      regions:
        system_prompt:
          max_tokens: 4000
        compacted_history:
          max_tokens: 30000
        recent_history:
          max_tokens: 60000
          messages_to_keep: 15
        tool_results:
          max_tokens: 80000
          max_age_iterations: 5
        retrieved_docs:
          max_tokens: 30000

    ollama/qwen2.5-coder:
      max_tokens: 32768
      headroom: 4000
      compact_threshold: 0.5
      regions:
        system_prompt:
          max_tokens: 2000
        compacted_history:
          max_tokens: 8000
        recent_history:
          max_tokens: 10000
          messages_to_keep: 5
        tool_results:
          max_tokens: 12000
          max_age_iterations: 2
        retrieved_docs:
          max_tokens: 4000

  # GC configuration
  garbage_collection:
    run_before_compaction: true
    run_after_iteration: true

  # Tokenization
  tokenization:
    cache_size: 1000
    estimation_fallback: true
```

### Configuration Validation

During initialization, validate:

- All region `max_tokens` sum to <= `(max_tokens - headroom)`
- `compact_threshold` is between 0.1 and 0.9
- `max_age_iterations` is positive
- `messages_to_keep` is at least 3
- Model name matches supported tokenizer

### Default Fallback

If model not found in configuration, use conservative defaults:

```go
func DefaultModelConfig() ModelConfig {
    return ModelConfig{
        MaxTokens:        128000,
        Headroom:         8000,
        CompactThreshold: 0.6,
        Regions: map[RegionType]RegionConfig{
            RegionSystemPrompt:   {MaxTokens: 4000},
            RegionCompactedHist:  {MaxTokens: 20000},
            RegionRecentHist:     {MaxTokens: 40000, MessagesToKeep: 10},
            RegionToolResults:    {MaxTokens: 60000, MaxAgeIterations: 3},
            RegionRetrievedDocs:  {MaxTokens: 20000},
        },
    }
}
```

---

## Integration with agentic-loop

Context management integrates seamlessly into the existing agentic-loop component lifecycle.

### Integration Points

#### 1. Loop Initialization

When a new loop is created via `HandleTask`:

```go
func (h *MessageHandler) HandleTask(ctx context.Context, taskMsg TaskMessage) (HandlerResult, error) {
    // Create loop entity (existing code)
    loopID, err := h.loopMgr.CreateLoop(taskMsg.TaskID, taskMsg.Role, taskMsg.Model, h.config.MaxIterations)

    // NEW: Initialize context manager for this loop
    contextMgr, err := NewContextManager(taskMsg.Model, h.config.ContextConfig)
    h.contextManagers[loopID] = contextMgr

    // Add system prompt to context
    systemPrompt := h.buildSystemPrompt(taskMsg.Role)
    contextMgr.AddMessage(ChatMessage{
        Role:    "system",
        Content: systemPrompt,
    })

    // Continue with existing task handling...
}
```

#### 2. Pre-Request Check

Before sending each AgentRequest to the model:

```go
func (h *MessageHandler) prepareModelRequest(ctx context.Context, loopID string) (*AgentRequest, error) {
    contextMgr := h.contextManagers[loopID]

    // NEW: Check if compaction needed
    if contextMgr.ShouldCompact() {
        h.logger.Info("Context utilization high, triggering compaction",
            "loop_id", loopID,
            "utilization", contextMgr.Utilization())

        result, err := contextMgr.Compact(ctx, h.modelClient)
        if err != nil {
            h.logger.Warn("Compaction failed", "error", err)
        } else {
            h.emitCompactionEvent(loopID, result)
            h.metrics.recordCompaction(result)
        }
    }

    // Get current context from all regions
    messages := contextMgr.GetContext()

    // Build request (existing logic)
    return &AgentRequest{
        RequestID: generateRequestID(),
        LoopID:    loopID,
        Messages:  messages,
        // ...
    }
}
```

#### 3. Post-Response Update

After receiving an AgentResponse:

```go
func (h *MessageHandler) HandleModelResponse(ctx context.Context, loopID string, response AgentResponse) (HandlerResult, error) {
    contextMgr := h.contextManagers[loopID]

    // Add assistant response to recent history
    contextMgr.AddMessage(response.Message)

    // Update token usage metrics
    h.metrics.recordTokenUsage(response.TokenUsage)

    // If tool calls, add to tool results region
    if response.Status == "tool_call" {
        contextMgr.AddMessage(response.Message) // Tool call message
        // Tool results will be added when HandleToolResult is called
    }

    // Continue with existing response handling...
}
```

#### 4. Tool Result Handling

When tool results arrive:

```go
func (h *MessageHandler) HandleToolResult(ctx context.Context, loopID string, result ToolResult) (HandlerResult, error) {
    contextMgr := h.contextManagers[loopID]

    // Find the tool call message
    toolCallMsg := h.findToolCallMessage(loopID, result.ToolCallID)

    // Add tool result to context
    resultMsg := ChatMessage{
        Role:       "tool",
        Content:    result.Result,
        ToolCallID: result.ToolCallID,
    }
    contextMgr.AddToolResult(toolCallMsg, resultMsg)

    // Continue with existing tool result handling...
}
```

#### 5. Iteration Increment

After each model request completes:

```go
func (h *MessageHandler) incrementIteration(loopID string) error {
    // Existing iteration increment
    err := h.loopMgr.IncrementIteration(loopID)

    // NEW: Update context manager iteration (for GC)
    contextMgr := h.contextManagers[loopID]
    contextMgr.IncrementIteration()

    // NEW: Run garbage collection
    gcResult := contextMgr.GarbageCollect()
    if gcResult.ToolResultsRemoved > 0 {
        h.logger.Debug("Garbage collected tool results",
            "loop_id", loopID,
            "removed", gcResult.ToolResultsRemoved,
            "tokens_reclaimed", gcResult.TokensReclaimed)
        h.metrics.recordGC(gcResult)
    }

    return err
}
```

#### 6. Loop Cleanup

When loop completes or fails:

```go
func (h *MessageHandler) cleanupLoop(loopID string) {
    // Remove context manager
    delete(h.contextManagers, loopID)

    // Continue with existing cleanup...
}
```

### Message Building

The existing message building logic is replaced with:

```go
// OLD:
messages := append([]ChatMessage{systemPrompt}, loop.Messages...)

// NEW:
messages := contextMgr.GetContext()
```

This ensures messages are correctly ordered and filtered based on region priorities.

---

## NATS Events

Context management publishes events to NATS for observability and monitoring.

### Event Types

#### 1. Compaction Events

**Subject**: `agent.context.compaction.{loop_id}`

**Payload**:

```json
{
  "loop_id": "loop_123",
  "iteration": 6,
  "timestamp": "2026-01-31T12:00:00Z",
  "before": {
    "total_tokens": 82000,
    "utilization": 0.64,
    "regions": {
      "system_prompt": 2000,
      "compacted_history": 0,
      "recent_history": 15000,
      "tool_results": 50000,
      "retrieved_docs": 15000
    }
  },
  "after": {
    "total_tokens": 45000,
    "utilization": 0.35,
    "regions": {
      "system_prompt": 2000,
      "compacted_history": 5000,
      "recent_history": 10000,
      "tool_results": 20000,
      "retrieved_docs": 8000
    }
  },
  "evicted": {
    "tool_results": 30,
    "history_messages": 12,
    "retrieved_docs": 3
  },
  "tokens_reclaimed": 37000,
  "duration_ms": 850
}
```

#### 2. Utilization Events

**Subject**: `agent.context.utilization.{loop_id}`

**Frequency**: Every iteration

**Payload**:

```json
{
  "loop_id": "loop_123",
  "iteration": 8,
  "timestamp": "2026-01-31T12:01:00Z",
  "utilization": 0.42,
  "total_tokens": 54000,
  "available_tokens": 74000,
  "regions": {
    "system_prompt": 2000,
    "compacted_history": 5000,
    "recent_history": 15000,
    "tool_results": 25000,
    "retrieved_docs": 7000
  },
  "model": "gpt-4o",
  "max_tokens": 128000,
  "headroom": 8000
}
```

#### 3. Garbage Collection Events

**Subject**: `agent.context.gc.{loop_id}`

**Payload**:

```json
{
  "loop_id": "loop_123",
  "iteration": 9,
  "timestamp": "2026-01-31T12:01:15Z",
  "tool_results_removed": 5,
  "tokens_reclaimed": 12000,
  "evicted_by_age": 3,
  "evicted_by_size": 2,
  "duration_ms": 5
}
```

### Event Publishing

```go
func (h *MessageHandler) emitCompactionEvent(loopID string, result *CompactionResult) {
    event := CompactionEvent{
        LoopID:    loopID,
        Iteration: h.loopMgr.GetIteration(loopID),
        Timestamp: time.Now(),
        Before:    result.Before,
        After:     result.After,
        Evicted:   result.Evicted,
        // ...
    }

    data, _ := json.Marshal(event)
    subject := fmt.Sprintf("agent.context.compaction.%s", loopID)
    h.natsClient.PublishToStream(context.Background(), subject, data)
}
```

---

## Metrics

Context management extends the existing Prometheus metrics in `processor/agentic-loop/metrics.go`.

### New Metrics

Add to `loopMetrics` struct:

```go
type loopMetrics struct {
    // ... existing metrics ...

    // Context management
    contextUtilization    *prometheus.GaugeVec     // Current utilization by loop
    contextRegionTokens   *prometheus.GaugeVec     // Tokens per region
    contextCompactionTotal prometheus.Counter      // Compaction events
    contextCompactionDuration prometheus.Histogram // Compaction duration
    contextEvictionTotal  *prometheus.CounterVec   // Evictions by region
    contextGCTotal        prometheus.Counter       // GC events
    contextGCTokensReclaimed prometheus.Counter    // Tokens reclaimed by GC
}
```

### Metric Definitions

```go
contextUtilization: prometheus.NewGaugeVec(prometheus.GaugeOpts{
    Namespace: "semstreams",
    Subsystem: "agentic_loop",
    Name:      "context_utilization_ratio",
    Help:      "Current context utilization ratio (0.0-1.0) by loop",
}, []string{"loop_id", "model"}),

contextRegionTokens: prometheus.NewGaugeVec(prometheus.GaugeOpts{
    Namespace: "semstreams",
    Subsystem: "agentic_loop",
    Name:      "context_region_tokens",
    Help:      "Token count per context region",
}, []string{"loop_id", "region"}),

contextCompactionTotal: prometheus.NewCounter(prometheus.CounterOpts{
    Namespace: "semstreams",
    Subsystem: "agentic_loop",
    Name:      "context_compaction_total",
    Help:      "Total number of context compaction events",
}),

contextCompactionDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
    Namespace: "semstreams",
    Subsystem: "agentic_loop",
    Name:      "context_compaction_duration_seconds",
    Help:      "Duration of context compaction operations",
    Buckets:   []float64{0.1, 0.5, 1.0, 2.0, 5.0, 10.0},
}),

contextEvictionTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
    Namespace: "semstreams",
    Subsystem: "agentic_loop",
    Name:      "context_eviction_total",
    Help:      "Total context evictions by region",
}, []string{"region"}),

contextGCTotal: prometheus.NewCounter(prometheus.CounterOpts{
    Namespace: "semstreams",
    Subsystem: "agentic_loop",
    Name:      "context_gc_total",
    Help:      "Total context garbage collection events",
}),

contextGCTokensReclaimed: prometheus.NewCounter(prometheus.CounterOpts{
    Namespace: "semstreams",
    Subsystem: "agentic_loop",
    Name:      "context_gc_tokens_reclaimed_total",
    Help:      "Total tokens reclaimed by garbage collection",
}),
```

### Metric Recording

```go
// Update utilization after each change
func (m *loopMetrics) recordContextUtilization(loopID, model string, utilization float64) {
    m.contextUtilization.WithLabelValues(loopID, model).Set(utilization)
}

// Update region tokens
func (m *loopMetrics) recordRegionTokens(loopID string, region RegionType, tokens int) {
    m.contextRegionTokens.WithLabelValues(loopID, string(region)).Set(float64(tokens))
}

// Record compaction
func (m *loopMetrics) recordCompaction(result *CompactionResult) {
    m.contextCompactionTotal.Inc()
    m.contextCompactionDuration.Observe(float64(result.DurationMs) / 1000.0)

    // Record evictions by region
    if result.Evicted.ToolResults > 0 {
        m.contextEvictionTotal.WithLabelValues("tool_results").Add(float64(result.Evicted.ToolResults))
    }
    if result.Evicted.HistoryMessages > 0 {
        m.contextEvictionTotal.WithLabelValues("recent_history").Add(float64(result.Evicted.HistoryMessages))
    }
}

// Record GC
func (m *loopMetrics) recordGC(result *GCResult) {
    m.contextGCTotal.Inc()
    m.contextGCTokensReclaimed.Add(float64(result.TokensReclaimed))
}
```

### Metric Usage

Dashboard queries:

```promql
# Current utilization across all loops
semstreams_agentic_loop_context_utilization_ratio

# Compaction frequency (events per hour)
rate(semstreams_agentic_loop_context_compaction_total[1h])

# Average compaction duration
histogram_quantile(0.95, semstreams_agentic_loop_context_compaction_duration_seconds)

# Tokens reclaimed by GC (rate)
rate(semstreams_agentic_loop_context_gc_tokens_reclaimed_total[1h])

# Most evicted region
sum by (region) (semstreams_agentic_loop_context_eviction_total)
```

---

## Testing Strategy

Comprehensive testing ensures context management works correctly under various conditions.

### Unit Tests

#### 1. ContextManager Tests

**File**: `processor/agentic-loop/context_manager_test.go`

**Test cases**:

- `TestNewContextManager`: Initialization with valid config
- `TestUtilizationCalculation`: Correct utilization percentage
- `TestShouldCompactThreshold`: Trigger logic at various utilizations
- `TestAddMessageToCorrectRegion`: Message routing to regions
- `TestRegionTokenLimits`: Error when region exceeds max tokens
- `TestGetContextOrdering`: Messages returned in correct order

#### 2. Compaction Tests

**File**: `processor/agentic-loop/context_compaction_test.go`

**Test cases**:

- `TestCompactionTriggering`: Compaction runs when utilization > threshold
- `TestCompactionSummarization`: Summary generated correctly
- `TestCompactionTokenReclamation`: Tokens actually reclaimed
- `TestCompactionIdempotency`: Multiple compactions don't corrupt state
- `TestCompactionFailureHandling`: Graceful handling when summarization fails

#### 3. Garbage Collection Tests

**File**: `processor/agentic-loop/context_gc_test.go`

**Test cases**:

- `TestGCAgeBasedEviction`: Old tool results evicted
- `TestGCSizeBasedEviction`: Largest results evicted when over limit
- `TestGCKeepRecentResults`: Last iteration results always kept
- `TestGCKeepOnePerTool`: Most recent result per tool preserved

#### 4. Tokenizer Tests

**File**: `processor/agentic-loop/tokenizer_test.go`

**Test cases**:

- `TestTiktokenAccuracy`: Token counts match OpenAI's API
- `TestMessageTokenCounting`: Overhead calculated correctly
- `TestTokenCaching`: Cache reduces redundant counting
- `TestTokenizerSelection`: Correct tokenizer chosen per model

### Integration Tests

**File**: `processor/agentic-loop/context_integration_test.go`

**Test cases**:

#### TestContextLifecycle

End-to-end test with mock model:

1. Create loop with context manager
2. Add system prompt
3. Simulate 20 iterations with messages and tool results
4. Verify compaction triggers at correct thresholds
5. Verify GC runs after each iteration
6. Verify final utilization remains healthy
7. Check NATS events published

#### TestCompactionWithRealTokenizer

Uses actual tiktoken library:

1. Generate realistic conversation (code review task)
2. Add large tool results (file reads)
3. Trigger compaction
4. Verify token counts accurate within 5%
5. Verify summary preserves key information

#### TestRegionPriorities

Test eviction order:

1. Fill all regions to capacity
2. Trigger compaction
3. Verify tool results evicted before history
4. Verify system prompt never evicted
5. Verify compacted history never evicted

### End-to-End Tests

Add to existing E2E test suite:

#### TestE2EContextManagement

**File**: `e2e/context_test.go`

**Requirements**: Docker (NATS), local model (Ollama)

**Scenario**:

1. Start full agentic system (loop + model + tools)
2. Submit complex task requiring 15+ iterations
3. Monitor context utilization via NATS events
4. Verify compaction occurs at expected iteration
5. Verify loop completes successfully despite high context usage
6. Check Prometheus metrics for accurate reporting

### Test Data

**File**: `processor/agentic-loop/testdata/context_messages.json`

Realistic test data:

```json
{
  "system_prompt": "You are a code review assistant...",
  "conversation": [
    {"role": "user", "content": "Review auth/token.go for security issues"},
    {"role": "assistant", "content": "I'll analyze the file...", "tool_calls": [...]},
    {"role": "tool", "tool_call_id": "call_1", "content": "package auth\n\n..."}
  ],
  "large_tool_result": {
    "file_content": "... 5000 lines of code ..."
  }
}
```

---

## Relationship to ADR-017

Context memory management (ADR-015) and graph-backed agent memory (ADR-017) are complementary systems serving
different purposes.

### Division of Responsibility

| Aspect | ADR-015 (Context Memory) | ADR-017 (Graph Memory) |
|--------|--------------------------|------------------------|
| **Storage** | Volatile (in-memory context) | Persistent (knowledge graph) |
| **Scope** | Current loop execution | Cross-loop, long-term |
| **Lifetime** | Loop duration only | Permanent |
| **Size limit** | Model context window | Effectively unlimited |
| **Access speed** | Immediate (in context) | Query-based retrieval |
| **Purpose** | Working memory for reasoning | External knowledge base |

### Integration Points

#### 1. Compaction Triggers Fact Extraction

When compaction evicts messages from recent history, optionally extract facts to the graph:

```go
func (cm *ContextManager) Compact(ctx context.Context, modelClient ModelClient) (*CompactionResult, error) {
    // ... existing compaction logic ...

    // Optional: Extract facts from evicted messages to graph
    if cm.config.ExtractFactsOnCompaction {
        facts := cm.extractFacts(evictedMessages)
        cm.graphClient.StoreFacts(ctx, facts)
    }

    // Continue with compaction...
}
```

**Benefit**: Evicted working memory becomes permanent knowledge, preventing information loss.

#### 2. Retrieved Docs Region for PathRAG

The retrieved docs region is designed for ADR-017's PathRAG implementation:

```go
// When PathRAG retrieves context from graph
func (h *MessageHandler) retrieveGraphContext(ctx context.Context, loopID string, query string) error {
    contextMgr := h.contextManagers[loopID]

    // Query graph for relevant entities/relationships
    results := h.graphClient.PathRAG(ctx, query)

    // Add to retrieved docs region
    contextMgr.PinRetrievedDocs(results.Entities)

    return nil
}

// When loop transitions to new state, release docs
func (h *MessageHandler) onStateTransition(loopID string, newState LoopState) {
    contextMgr := h.contextManagers[loopID]

    if newState == StateReviewing {
        // Unpin retrieved docs from exploration phase
        contextMgr.UnpinRetrievedDocs()
    }
}
```

#### 3. Compacted History Queryable via Graph

Future enhancement: Store compacted summaries as graph entities:

```text
(Loop:loop_123)-[:HAS_SUMMARY]->(Summary:iteration_1_5)
(Summary:iteration_1_5)-[:CONTAINS_DECISION]->(Decision:visitor_pattern)
(Summary:iteration_1_5)-[:MODIFIED_FILE]->(File:processor/graph/node.go)
```

**Benefit**: Later loops can query previous loop summaries via graph traversal.

### Example Workflow

**Scenario**: Multi-day code refactoring with multiple loops

**Loop 1 (Day 1)**:

1. Context manager handles working memory (file reads, code analysis)
2. At iteration 6, compaction triggers
3. Facts extracted to graph: "Refactored auth system to use JWT"
4. Loop completes, context discarded

**Loop 2 (Day 2)**:

1. New loop starts with fresh context
2. User asks: "What changed in the auth system?"
3. PathRAG (ADR-017) queries graph for auth-related facts
4. Retrieved fact: "Refactored auth system to use JWT (Loop 1, iteration 4)"
5. Fact added to retrieved docs region (ADR-015)
6. Model has context from previous loop without re-reading all files

**Synergy**: Working memory (ADR-015) stays focused and efficient. Long-term memory (ADR-017) provides historical
context on demand.

---

## Implementation Roadmap

Phased implementation to minimize risk and enable early validation.

### Phase 1: Foundation (Week 1)

**Goal**: Token counting and utilization tracking

**Deliverables**:

- `Tokenizer` interface and implementations
- `ContextManager` struct with region management
- `Utilization()` and `UtilizationByRegion()` methods
- Basic `AddMessage()` and `GetContext()` methods
- Unit tests for token counting accuracy

**Integration**: None yet (non-functional)

**Validation**: Unit tests verify token counts within 5% of actual API usage

### Phase 2: Garbage Collection (Week 1)

**Goal**: Tool result GC to reclaim space

**Deliverables**:

- `GarbageCollect()` method with age and size policies
- `ToolResultEntry` tracking
- Integration with `IncrementIteration()`
- Unit tests for GC policies

**Integration**: GC runs after each iteration

**Validation**: Integration test shows tool results evicted after 3 iterations

### Phase 3: Compaction (Week 2)

**Goal**: Automated compaction when utilization high

**Deliverables**:

- `Compact()` method with eviction and summarization
- Summarization prompt template
- `CompactionResult` metrics tracking
- Integration with pre-request check

**Integration**: Compaction triggers before model requests when utilization > 60%

**Validation**: Integration test shows successful compaction reduces utilization to < 40%

### Phase 4: Events and Metrics (Week 2)

**Goal**: Observability for context management

**Deliverables**:

- NATS event publishing (compaction, utilization, GC)
- Prometheus metrics (utilization gauge, compaction counter, etc.)
- Event schemas documented

**Integration**: Events published on compaction and iteration

**Validation**: E2E test monitors NATS and Prometheus for correct metrics

### Phase 5: Configuration (Week 3)

**Goal**: Per-model configuration system

**Deliverables**:

- YAML configuration schema
- Model-specific region allocations
- Configuration validation
- Documentation for tuning thresholds

**Integration**: Configuration loaded from `config.yml`

**Validation**: Multiple models tested with different limits

### Phase 6: Documentation and Tuning (Week 3)

**Goal**: Production-ready documentation and optimized defaults

**Deliverables**:

- This specification document
- Inline code documentation (godoc)
- Configuration tuning guide
- Performance benchmarks

**Validation**: Documentation reviewed by team, benchmarks show < 5% overhead

### Success Criteria

Context management is production-ready when:

- No context overflow crashes in 100-iteration test loops
- Compaction reduces utilization by at least 30%
- Token counting accuracy within 10% of actual API usage
- All metrics publish correctly to Prometheus
- Configuration supports GPT-4, Claude, and Qwen models
- E2E tests pass with all models

---

## Open Questions and Future Work

### Open Questions

1. **Summarization model**: Use same model or cheaper alternative for compaction?
   - **Tradeoff**: Same model = better quality, cheaper model = lower cost
   - **Recommendation**: Start with same model, add config option later

2. **Compaction frequency**: Should compaction be less aggressive for large models (Claude 200K)?
   - **Tradeoff**: Less frequent = simpler but wastes capacity
   - **Recommendation**: Scale threshold with model size (70% for 200K models)

3. **Retrieved docs pinning**: Should docs auto-unpin or require explicit release?
   - **Tradeoff**: Auto = convenient but risky, explicit = verbose but safe
   - **Recommendation**: Auto-unpin on state transitions, allow explicit override

### Future Enhancements

#### 1. Streaming Compaction

Current design: Compaction is synchronous and blocks loop iteration.

**Future**: Stream compaction in background, continue loop with current context.

**Benefit**: No latency impact on loop execution.

#### 2. Multi-Level Compaction

Current design: Single compacted history region.

**Future**: Hierarchical summaries (recent, mid-term, long-term).

**Benefit**: Better preservation of detail at different time scales.

#### 3. Selective Eviction

Current design: FIFO eviction of messages.

**Future**: Semantic importance scoring to keep high-value messages longer.

**Benefit**: Critical information stays in context even if old.

#### 4. Context Prefetching

Current design: Retrieved docs added on demand.

**Future**: Predict needed context and prefetch from graph.

**Benefit**: Reduce query latency during execution.

#### 5. Cross-Loop Context Sharing

Current design: Each loop has independent context.

**Future**: Share compacted summaries across related loops.

**Benefit**: Multi-loop tasks maintain continuity.

---

## Appendix A: Configuration Examples

### Development Configuration

Optimized for rapid iteration with local models:

```yaml
context_memory:
  enabled: true
  models:
    ollama/qwen2.5-coder:
      max_tokens: 32768
      headroom: 2000
      compact_threshold: 0.5
      regions:
        system_prompt:
          max_tokens: 1000
        compacted_history:
          max_tokens: 5000
        recent_history:
          max_tokens: 10000
          messages_to_keep: 5
        tool_results:
          max_tokens: 15000
          max_age_iterations: 2
        retrieved_docs:
          max_tokens: 2000
  garbage_collection:
    run_before_compaction: true
    run_after_iteration: true
  tokenization:
    cache_size: 500
    estimation_fallback: true
```

### Production Configuration

Optimized for quality with cloud models:

```yaml
context_memory:
  enabled: true
  models:
    gpt-4o:
      max_tokens: 128000
      headroom: 8000
      compact_threshold: 0.6
      regions:
        system_prompt:
          max_tokens: 4000
        compacted_history:
          max_tokens: 20000
        recent_history:
          max_tokens: 40000
          messages_to_keep: 10
        tool_results:
          max_tokens: 60000
          max_age_iterations: 3
        retrieved_docs:
          max_tokens: 20000

    claude-sonnet-4.5:
      max_tokens: 200000
      headroom: 10000
      compact_threshold: 0.7
      regions:
        system_prompt:
          max_tokens: 4000
        compacted_history:
          max_tokens: 30000
        recent_history:
          max_tokens: 60000
          messages_to_keep: 15
        tool_results:
          max_tokens: 80000
          max_age_iterations: 5
        retrieved_docs:
          max_tokens: 30000

  garbage_collection:
    run_before_compaction: true
    run_after_iteration: false

  tokenization:
    cache_size: 2000
    estimation_fallback: false
```

---

## Appendix B: Metric Dashboard Queries

### Utilization Overview

```promql
# Current utilization per loop
semstreams_agentic_loop_context_utilization_ratio

# Utilization histogram
histogram_quantile(0.95,
  rate(semstreams_agentic_loop_context_utilization_ratio[5m]))

# Loops approaching threshold
count(semstreams_agentic_loop_context_utilization_ratio > 0.55)
```

### Compaction Analysis

```promql
# Compaction frequency
rate(semstreams_agentic_loop_context_compaction_total[1h])

# Compaction duration P95
histogram_quantile(0.95,
  rate(semstreams_agentic_loop_context_compaction_duration_seconds_bucket[5m]))

# Failed compactions (if tracked)
rate(semstreams_agentic_loop_context_compaction_errors_total[5m])
```

### Region Analysis

```promql
# Tokens per region (stacked)
sum by (region) (semstreams_agentic_loop_context_region_tokens)

# Most evicted region
topk(3,
  sum by (region) (
    rate(semstreams_agentic_loop_context_eviction_total[1h])))

# Tool results eviction rate
rate(semstreams_agentic_loop_context_eviction_total{region="tool_results"}[5m])
```

### Garbage Collection

```promql
# GC events per hour
rate(semstreams_agentic_loop_context_gc_total[1h])

# Tokens reclaimed rate
rate(semstreams_agentic_loop_context_gc_tokens_reclaimed_total[5m])

# GC efficiency (tokens reclaimed per event)
rate(semstreams_agentic_loop_context_gc_tokens_reclaimed_total[5m]) /
rate(semstreams_agentic_loop_context_gc_total[5m])
```

---

## Appendix C: Example Messages JSON

Realistic test data for unit and integration tests.

**File**: `processor/agentic-loop/testdata/context_messages.json`

```json
{
  "system_prompt": {
    "role": "system",
    "content": "You are a code review assistant. Analyze code for security issues, bugs, and best practices. Use the available tools to read files and query the codebase knowledge graph."
  },

  "conversation": [
    {
      "role": "user",
      "content": "Review auth/token.go for security issues"
    },
    {
      "role": "assistant",
      "content": "I'll analyze the authentication token implementation. Let me read the file first.",
      "tool_calls": [
        {
          "id": "call_001",
          "type": "function",
          "function": {
            "name": "file_read",
            "arguments": "{\"path\": \"auth/token.go\"}"
          }
        }
      ]
    },
    {
      "role": "tool",
      "tool_call_id": "call_001",
      "content": "package auth\n\nimport (\n\t\"crypto/rand\"\n\t\"encoding/base64\"\n\t\"time\"\n)\n\ntype Token struct {\n\tValue     string\n\tExpiresAt time.Time\n}\n\nfunc GenerateToken() (*Token, error) {\n\tb := make([]byte, 32)\n\t_, err := rand.Read(b)\n\tif err != nil {\n\t\treturn nil, err\n\t}\n\treturn &Token{\n\t\tValue:     base64.URLEncoding.EncodeToString(b),\n\t\tExpiresAt: time.Now().Add(24 * time.Hour),\n\t}, nil\n}"
    },
    {
      "role": "assistant",
      "content": "I found a potential issue: tokens are generated with cryptographically secure randomness (good), but there's no mechanism to revoke tokens before expiration. Let me check if there's a revocation system in the codebase.",
      "tool_calls": [
        {
          "id": "call_002",
          "type": "function",
          "function": {
            "name": "graph_query",
            "arguments": "{\"query\": \"MATCH (f:File)-[:CONTAINS]->(fn:Function) WHERE fn.name CONTAINS 'Revoke' RETURN f.path, fn.name LIMIT 5\"}"
          }
        }
      ]
    },
    {
      "role": "tool",
      "tool_call_id": "call_002",
      "content": "{\"results\": []}"
    },
    {
      "role": "assistant",
      "content": "Confirmed: no revocation mechanism exists. Here's my security review:\n\n**Findings:**\n\n1. **CRITICAL - Missing Token Revocation**: Tokens cannot be invalidated before expiration. If a token is compromised, it remains valid for 24 hours.\n\n   **Recommendation**: Implement a token blacklist using Redis or a database table. Add a `RevokeToken(value string)` function and check the blacklist in `ValidateToken()`.\n\n2. **GOOD - Secure Random Generation**: Using `crypto/rand` is appropriate for token generation.\n\n3. **MEDIUM - Fixed Expiration**: 24-hour expiration is hardcoded. Consider making this configurable based on token type (session vs. API key).\n\n**Next Steps:**\n- Implement token revocation system\n- Add configurable expiration durations\n- Add tests for token lifecycle"
    }
  ],

  "large_file_content": {
    "role": "tool",
    "tool_call_id": "call_003",
    "content": "package main\n\n// This is a very large file with 5000+ lines of code...\n// [Simulated content would be much longer in real test data]\n\nfunc main() {\n\t// Application entry point\n\tapp := NewApp()\n\tapp.Run()\n}\n\n// ... many more functions ..."
  }
}
```

---

## References

- [ADR-015: Context Memory Management](../adr-015-context-memory-management.md) - Design rationale
- [ADR-017: Graph-Backed Agent Memory](../adr-017-graph-backed-agent-memory.md) - Complementary system
- [Agentic Loop Documentation](../../processor/agentic-loop/doc.go) - Component overview
- [Agentic Types](../../agentic/types.go) - Message and request types
- [Context Window Attention Research](https://arxiv.org/abs/2307.03172) - Performance degradation studies
- [Tiktoken Library](https://github.com/pkoukk/tiktoken-go) - OpenAI token counting
- [OpenAI Token Limits](https://platform.openai.com/docs/models) - Model specifications
- [Anthropic Token Limits](https://docs.anthropic.com/en/docs/about-claude/models) - Claude specifications
