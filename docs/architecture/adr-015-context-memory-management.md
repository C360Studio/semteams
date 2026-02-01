# ADR-015: Context Memory Management

## Status

Proposed

## Context

LLMs operate with finite context windows (e.g., GPT-4: 128K tokens, Claude: 200K tokens), but current agentic
systems treat them as unlimited text buffers. This leads to performance degradation, context overflow, and
unpredictable behavior as context fills.

### The Context Window Problem

**Current approach in agentic systems:**

1. Accumulate all messages, tool results, and retrieved documents in context
2. No tracking of token utilization vs model limits
3. No awareness of context regions (system prompt, history, tools, retrieval)
4. Performance degrades at ~60% capacity due to attention thrashing
5. Hard failures when context exceeds model limit

**Real-world symptoms:**

- Models become slower and less accurate as context fills (attention dilution)
- Hard crashes when context exceeds token limit
- No visibility into which context regions are consuming tokens
- Tool results accumulate indefinitely, consuming valuable context space
- No automatic compaction or summarization strategies

### Operating System Memory Management Analogy

Modern operating systems manage physical memory with:

- **Memory regions**: Stack, heap, data, code segments
- **Utilization tracking**: Free memory, allocations, fragmentation
- **Compaction triggers**: Garbage collection, swap when approaching limits
- **Reclamation**: Age out unused pages, compress old data

LLM context windows need the same discipline: treat context as **managed virtual memory**.

### Attention Thrashing at High Utilization

Research shows LLM performance degrades non-linearly with context utilization:

| Utilization | Performance | Behavior |
|------------|-------------|----------|
| 0-40% | Optimal | Full attention across context |
| 40-60% | Good | Slight degradation in recall |
| 60-80% | Degraded | Attention thrashing begins |
| 80-100% | Poor | Significant quality loss |
| 100%+ | Failure | Context overflow, API errors |

**Compaction threshold of 60%** ensures operation in the optimal/good range, with headroom for response generation.

### SemStreams' Context Challenge

Agentic loops in SemStreams accumulate:

- **System prompts**: Role instructions, tool definitions (static, ~2K tokens)
- **Chat history**: User messages, assistant responses (growing, ~10K tokens)
- **Tool results**: Graph queries, file reads, code analysis (temporary, ~20K tokens)
- **Retrieved docs**: PathRAG context, entity summaries (pinned, ~10K tokens)

Without management, a 20-iteration loop exceeds 128K context within ~8 iterations.

### Industry Approaches and Failures

Common strategies and their problems:

| Strategy | Approach | Failure Mode |
|----------|----------|--------------|
| **Unlimited accumulation** | Keep all messages in context | Context overflow crash |
| **Fixed window** | Keep last N messages | Loses critical early decisions |
| **Naive summarization** | Summarize all history to text | Loses structured relationships (see ADR-017) |
| **Random eviction** | Drop oldest tool results | May evict still-relevant data |

**No existing framework applies OS memory management principles to LLM context.**

## Decision

Treat LLM context windows as **managed memory regions** with explicit allocation, tracking, compaction, and garbage
collection. Implement in `processor/agentic-loop` as context utilization increases.

### Memory Region Model

```text
┌─────────────────────────────────────────────────────┐
│ SYSTEM PROMPT (fixed, ~2K tokens)                   │ ← Never evicted
├─────────────────────────────────────────────────────┤
│ COMPACTED HISTORY (summary, ~5K tokens)             │ ← Grows via compaction
├─────────────────────────────────────────────────────┤
│ RECENT HISTORY (rolling, ~10K tokens)               │ ← Oldest evicted first
├─────────────────────────────────────────────────────┤
│ TOOL RESULTS (temporary, ~20K tokens)               │ ← Aged out after N iterations
├─────────────────────────────────────────────────────┤
│ RETRIEVED DOCS (pinned, ~10K tokens)                │ ← Released after use
├─────────────────────────────────────────────────────┤
│ HEADROOM (reserved for response, ~5K tokens)        │ ← Never allocated
└─────────────────────────────────────────────────────┘
```

**Region priorities (eviction order):**

1. Old tool results (age > N iterations)
2. Old chat history (moved to compacted summary)
3. Retrieved docs (after task completion)
4. Recent history (last resort)
5. Compacted history (never evicted)
6. System prompt (never evicted)

### Compaction Strategy

**Trigger conditions:**

- Context utilization exceeds 60% of model limit
- OR explicit compaction request
- OR iteration count exceeds threshold (configurable, default: 10)

**Compaction process:**

```text
┌─────────────────────────────────────────────────────────────────┐
│                   Compaction Workflow                            │
│                                                                  │
│  1. MEASURE                                                      │
│     - Count tokens in each region                               │
│     - Calculate utilization percentage                           │
│     - Identify eviction candidates                              │
│                                                                  │
│  2. EVICT (If utilization > 60%)                                │
│     - Remove tool results older than N iterations               │
│     - Archive chat history messages to compaction buffer        │
│     - Release retrieved docs for completed tasks                │
│                                                                  │
│  3. COMPACT (If history accumulated)                            │
│     - Summarize archived history into structured summary        │
│     - Preserve key facts via graph extraction (ADR-017)         │
│     - Replace detailed history with summary                     │
│                                                                  │
│  4. VERIFY                                                       │
│     - Re-measure token utilization                              │
│     - Ensure utilization back to safe range (<50%)              │
│     - Emit compaction event with metrics                        │
└─────────────────────────────────────────────────────────────────┘
```

**Compaction output:**

Structured summary preserving critical information:

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

### Token Counting

Accurate token counting per request using model-specific tokenizers:

```go
// processor/agentic-loop/context_manager.go
type ContextManager struct {
    modelLimit      int             // Model's context window size
    tokenizer       Tokenizer       // Model-specific tokenizer
    regions         map[string]*Region
    compactionThreshold float64     // Default: 0.60
}

type Region struct {
    Name         string
    Messages     []agentic.ChatMessage
    Tokens       int
    Priority     int  // Eviction priority (lower = evict first)
    Pinned       bool // Never evict
}

// Calculate current utilization
func (cm *ContextManager) Utilization() float64 {
    totalTokens := 0
    for _, region := range cm.regions {
        totalTokens += region.Tokens
    }
    return float64(totalTokens) / float64(cm.modelLimit)
}
```

### Model Context Limits

Per-model configuration for context limits:

| Model | Context Limit | Compaction Threshold (60%) | Headroom Reserved |
|-------|--------------|---------------------------|------------------|
| gpt-4o | 128,000 | 76,800 | 6,400 |
| gpt-4o-mini | 128,000 | 76,800 | 6,400 |
| claude-sonnet-4.5 | 200,000 | 120,000 | 10,000 |
| claude-opus-4.5 | 200,000 | 120,000 | 10,000 |

Configuration per model:

```json
{
  "context_management": {
    "enabled": true,
    "models": {
      "gpt-4o": {
        "context_limit": 128000,
        "compaction_threshold": 0.60,
        "headroom_tokens": 6400
      },
      "claude-sonnet-4.5": {
        "context_limit": 200000,
        "compaction_threshold": 0.60,
        "headroom_tokens": 10000
      }
    },
    "default": {
      "context_limit": 128000,
      "compaction_threshold": 0.60,
      "headroom_tokens": 6400
    }
  }
}
```

### Garbage Collection for Tool Results

Age out old tool results to prevent indefinite accumulation:

```go
// processor/agentic-loop/context_gc.go
type ToolResultGC struct {
    maxAge          int  // Iterations after which to evict
    currentIteration int
}

func (gc *ToolResultGC) ShouldEvict(result ToolResultEntry) bool {
    age := gc.currentIteration - result.IterationAdded
    return age > gc.maxAge
}
```

**Default policy:**

- Keep tool results for **3 iterations** (covers recent context needs)
- After 3 iterations, evict unless:
  - Result referenced in recent messages
  - Result pinned by user request
  - Result part of retrieved docs (managed separately)

### Integration with Agentic Loop

**Lifecycle integration:**

```text
┌─────────────────────────────────────────────────────┐
│           Agentic Loop with Context Management       │
│                                                      │
│  1. Task Start                                       │
│     - Initialize ContextManager for loop            │
│     - Set model limits based on task.Model          │
│     - Allocate SYSTEM_PROMPT region                 │
│                                                      │
│  2. Each Iteration                                   │
│     - Add messages to RECENT_HISTORY region         │
│     - Add tool results to TOOL_RESULTS region       │
│     - Count tokens in all regions                   │
│     - Check utilization threshold                   │
│     - Trigger compaction if needed                  │
│                                                      │
│  3. Model Call                                       │
│     - Verify headroom available for response        │
│     - Serialize context from all regions            │
│     - Track TokenUsage from response                │
│     - Update region token counts                    │
│                                                      │
│  4. Compaction Event                                 │
│     - Evict old tool results (GC)                   │
│     - Archive old history to compaction buffer      │
│     - Generate structured summary                   │
│     - Update COMPACTED_HISTORY region               │
│     - Publish compaction event to NATS              │
│                                                      │
│  5. Task Complete                                    │
│     - Release all regions                           │
│     - Emit final utilization metrics                │
└─────────────────────────────────────────────────────┘
```

**Handler modification in `processor/agentic-loop/handlers.go`:**

```go
type MessageHandler struct {
    // ... existing fields ...
    contextManager *ContextManager
}

func (h *MessageHandler) HandleModelResponse(ctx context.Context, loopID string, response agentic.AgentResponse) (HandlerResult, error) {
    // Check if compaction needed
    if h.contextManager.Utilization() > h.contextManager.compactionThreshold {
        if err := h.compactContext(ctx, loopID); err != nil {
            h.logger.Warn("Compaction failed", "error", err)
        }
    }

    // ... existing handler logic ...
}
```

### NATS Events for Observability

**Published events:**

- `agent.context.compaction.*` - Compaction events with metrics
- `agent.context.utilization.*` - Periodic utilization snapshots
- `agent.context.eviction.*` - Tool result eviction events

**Example compaction event:**

```json
{
  "loop_id": "loop-123",
  "iteration": 6,
  "timestamp": "2026-01-31T12:00:00Z",
  "before": {
    "total_tokens": 82000,
    "utilization": 0.64,
    "regions": {
      "system_prompt": 2000,
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
  }
}
```

### Integration with ADR-017: Graph-Backed Agent Memory

ADR-015 and ADR-017 form a **two-tier memory hierarchy** (working memory + external memory):

| Layer | ADR | Responsibility |
|-------|-----|----------------|
| Working Memory | ADR-015 | Manage context window, trigger compaction |
| External Memory | ADR-017 | Extract facts, enable recovery |

**Compaction Flow:**

1. ADR-015 detects `Utilization() > compactionThreshold`
2. ADR-015 publishes `agent.context.compaction.starting`
3. ADR-017 extracts structured facts to graph (files, decisions, state)
4. ADR-015 generates text summary, evicts old messages
5. ADR-015 publishes `agent.context.compaction.complete`
6. ADR-017 hydrates critical context from graph
7. ADR-017 publishes `agent.context.injected.*`
8. ADR-015 adds hydrated facts to COMPACTED_HISTORY region

**Why both are needed:**

- ADR-015 alone: Text summaries are lossy, agents lose file paths and decision rationale
- ADR-017 alone: No awareness of when to extract or hydrate
- Together: Efficient context management + reliable recovery from compaction

**NATS Subject Integration:**

| Published by ADR-015 | Consumed by ADR-017 |
|---------------------|---------------------|
| `agent.context.compaction.starting` | Triggers fact extraction |
| `agent.context.compaction.complete` | Triggers context hydration |
| `agent.context.utilization.*` | Monitors for threshold extraction |

| Published by ADR-017 | Consumed by ADR-015 |
|---------------------|---------------------|
| `agent.context.injected.*` | Adds to COMPACTED_HISTORY |

## Consequences

### Positive

- **Predictable Performance**: Operate in optimal utilization range (0-60%)
- **Avoid Crashes**: Never exceed model context limits
- **Transparency**: Full visibility into token usage per region
- **Automatic Management**: No manual intervention for context overflow
- **Configurable**: Per-model limits and thresholds
- **Observability**: Metrics for utilization, compaction frequency, evictions
- **Structured Compaction**: Preserves key facts in summary (complements ADR-017)
- **Cost Control**: Reduces token usage via eviction and compaction

### Negative

- **Compaction Overhead**: Summarization requires additional LLM call (mitigated by threshold triggering)
- **Token Counting Cost**: Per-message tokenization adds CPU overhead (mitigated by caching)
- **Configuration Complexity**: Per-model limits and thresholds require tuning
- **Information Loss**: Compaction is lossy (mitigated by graph extraction in ADR-017)
- **Implementation Complexity**: Memory region management adds code complexity

### Neutral

- **Opt-In**: Can disable context management for development/testing
- **Gradual Rollout**: Can enable token tracking first, compaction later
- **Tokenizer Dependency**: Requires model-specific tokenizer (tiktoken for OpenAI, claude-tokenizer for Anthropic)
- **Metrics Volume**: High-frequency utilization metrics may require downsampling

## Key Files

| File | Purpose |
|------|---------|
| `processor/agentic-loop/context_manager.go` | Context region management (NEW) |
| `processor/agentic-loop/context_gc.go` | Tool result garbage collection (NEW) |
| `processor/agentic-loop/context_compaction.go` | History compaction logic (NEW) |
| `processor/agentic-loop/tokenizer.go` | Model-specific token counting (NEW) |
| `processor/agentic-loop/handlers.go` | Integration with message handling |
| `processor/agentic-loop/config.go` | Context management configuration |
| `processor/agentic-loop/metrics.go` | Context utilization metrics |
| `agentic/types.go` | TokenUsage tracking (existing) |

## References

- [Agentic Loop README](../../processor/agentic-loop/doc.go) - Loop orchestrator documentation
- [ADR-017: Graph-Backed Agent Memory](./adr-017-graph-backed-agent-memory.md) - Complementary external memory
- [TokenUsage Tracking](../../agentic/types.go) - Existing token metrics
- [Context Window Attention Research](https://arxiv.org/abs/2307.03172) - Performance degradation studies
- [OS Memory Management](https://en.wikipedia.org/wiki/Memory_management) - Analogy source
