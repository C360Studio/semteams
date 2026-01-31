# ADR-017: Graph-Backed Agent Memory

## Status

Proposed

## Context

Traditional agentic frameworks rely on context windows for agent memory, with context compaction (summarization)
as the primary strategy for managing token limits. This approach has fundamental flaws:

### The Context Compaction Problem

**Current industry practice:**

1. Agent accumulates chat history until context window fills
2. Summarize history into text summary
3. Replace detailed history with summary
4. Continue execution with compressed context

**Why this fails:**

- **Lossy**: Critical facts lost in summarization (file paths, variable names, decision rationale)
- **Unstructured**: No way to query "what files did we modify in iteration 3?"
- **One-shot**: Irreversible compression, can't selectively retrieve details
- **Agent confusion**: Post-compaction agents fail because summaries don't capture semantic relationships

**Real-world pain:** Claude Code agents consistently fail after context compaction because text summaries lose the
structured relationships between decisions, file modifications, and task state. An agent that modified
`processor/graph/node.go` to fix a bug can't remember which file or why after compaction replaces detailed history
with "Fixed graph processor bug."

### SemStreams' Unique Advantage

SemStreams has a **knowledge graph** that no other agentic framework has. This enables structured external memory:

- Every decision, file modification, and state change can be stored as triples
- Graph queries can retrieve relevant context on-demand
- Semantic relationships preserved across iterations
- Context hydration from graph can reconstruct task state

**Example triples from agent execution:**

```text
# Decision tracking
("loop.agentic.task.analyze_code", "agentic.decision.architecture", "Use visitor pattern for traversal")
("loop.agentic.task.analyze_code", "agentic.decision.rationale", "Enables extensibility without modifying core")

# File modification tracking
("loop.agentic.task.analyze_code", "agentic.file.modified", "file.semstreams.processor.graph.node.go")
("file.semstreams.processor.graph.node.go", "agentic.modification.type", "bug_fix")
("file.semstreams.processor.graph.node.go", "agentic.modification.reason", "Nil pointer in visitor pattern")

# State checkpoints
("loop.agentic.task.analyze_code", "agentic.checkpoint.iteration", 3)
("loop.agentic.task.analyze_code", "agentic.checkpoint.state", "reviewing")
```

Post-compaction, the graph can answer:

- "What files did I modify?" → Query for `agentic.file.modified` predicates
- "Why did I choose visitor pattern?" → Retrieve `agentic.decision.*` triples
- "What was the state before compaction?" → Query checkpoint triples

### Hybrid Extraction Strategy

Pure rule-based extraction is brittle (misses nuanced decisions). Pure LLM extraction is expensive (requires
additional LLM call per iteration). **Recommendation: Hybrid approach:**

```text
┌─────────────────────────────────────────────────────────────────┐
│                   Hybrid Fact Extraction                         │
│                                                                  │
│  1. RULE-BASED (Fast, Deterministic)                            │
│     - File modifications from tool results                      │
│     - State transitions from LoopEntity                         │
│     - Iteration counts, timestamps                              │
│     - Tool call patterns (frequency, success/failure)           │
│                                                                  │
│  2. LLM-ASSISTED (Threshold-Based)                              │
│     - Trigger: Every N iterations OR context size threshold     │
│     - Extract: Decisions, rationale, key findings               │
│     - Pattern: "Extract structured facts from this response"    │
│     - Cost control: Only run when context nearing limit         │
│                                                                  │
│  3. ON-DEMAND HYDRATION                                         │
│     - Pre-task: Load relevant facts for new task                │
│     - Post-compaction: Reconstruct critical context             │
│     - Query-driven: "What files relate to X?"                   │
└─────────────────────────────────────────────────────────────────┘
```

### Rules Engine for Extraction

The existing rules engine (`processor/rule/`) provides all capabilities needed for rule-based fact extraction:

| Capability | Rules Engine Feature | Agent Memory Use |
|------------|---------------------|------------------|
| Pattern matching | `conditions` with `regex`, `contains` | Detect file paths, decisions |
| State transitions | `on_enter`, `on_exit`, `while_true` | Track fact changes |
| Graph mutations | `add_triple`, `remove_triple` actions | Store extracted facts |
| Entity watching | KV bucket patterns | Watch `AGENT_LOOPS` |
| Deduplication | Cooldown + state tracking | Avoid duplicate facts |

**Implementation approach:**

1. **Rule-based extraction** via JSON rule definitions (no custom code)
2. **LLM-assisted extraction** via minimal component (triggers LLM extraction)
3. **Context hydration** via component that queries graph, formats prompts

**Extraction Rules Location:**

```text
config/rules/agentic-memory/
├── file-modifications.json    # Extract file modification facts
├── tool-usage.json            # Extract tool call patterns
├── state-checkpoints.json     # Extract iteration state
└── decision-patterns.json     # Pattern-match decisions (flags for LLM)
```

## Decision

Create a **lean** `processor/agentic-memory` component that focuses on LLM extraction and hydration, while rule-based
extraction is handled by the existing rules engine via JSON rule definitions.

**Component responsibilities:**

1. **LLM-assisted extraction** of decisions and rationale (threshold-triggered)
2. **Context hydration** from graph for pre-task and post-compaction recovery
3. **Checkpoint management** for compaction coordination

**Delegated to rules engine:**

- File modification tracking (via `file-modifications.json`)
- Tool usage patterns (via `tool-usage.json`)
- State checkpoints (via `state-checkpoints.json`)
- Decision pattern detection (via `decision-patterns.json`)

### Architecture

```text
┌─────────────────────────────────────────────────────────────────┐
│                      Agentic Memory Flow                         │
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │  RULES ENGINE (processor/rule)                            │   │
│  │                                                           │   │
│  │  AGENT_LOOPS KV  ──────►  Rule Evaluator                 │   │
│  │  (entity watch)             │                            │   │
│  │                             ├─► file-modifications.json  │   │
│  │                             ├─► tool-usage.json          │   │
│  │                             ├─► state-checkpoints.json   │   │
│  │                             └─► decision-patterns.json   │   │
│  │                                    │                     │   │
│  │                                    └─► Triples ─► GRAPH  │   │
│  └──────────────────────────────────────────────────────────┘   │
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │  AGENTIC-MEMORY COMPONENT (lean)                          │   │
│  │                                                           │   │
│  │  agent.context.compaction.* ──► LLM Extractor            │   │
│  │  (threshold-triggered)            │                      │   │
│  │                                   └─► Decision Triples   │   │
│  │                                                           │   │
│  │  agent.context.compaction.complete ──► Context Hydrator  │   │
│  │  memory.hydrate.request.*              │                 │   │
│  │                                        ├─► Query Graph   │   │
│  │                                        └─► Inject Context│   │
│  └──────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

### Component Structure (Lean)

```text
processor/agentic-memory/
├── component.go       # Lifecycle, NATS subscriptions
├── llm_extractor.go   # Threshold-triggered LLM extraction
├── hydrator.go        # Query graph → format context
└── config.go          # Configuration types
```

### NATS Subjects

**Input Ports:**

- `agent.response.>` - Model responses for rule-based extraction
- `tool.result.>` - Tool results for file/state tracking
- `agent.memory.extract.*` - Explicit extraction requests (LLM-assisted)
- `agent.task.*` - New tasks requiring context hydration
- `agent.compact.*` - Compaction events requiring state reconstruction

**Output Ports:**

- `graph.mutation.*` - Triples to be added to knowledge graph
- `agent.context.injected.*` - Hydrated context for injection into prompts
- `agent.memory.checkpoint.*` - Checkpoint events for observability

**KV Buckets:**

- `AGENT_LOOPS` - Existing loop state (read)
- `AGENT_TRAJECTORIES` - Existing trajectory data (read)
- `AGENT_MEMORY_CHECKPOINTS` - Iteration checkpoints for recovery

### Graph Schema for Agent Facts

**Entity Types:**

```go
// Entity ID format: loop.agentic.task.{task_id}
// Example: loop.agentic.task.analyze_code_123
const (
    EntityTypeAgenticTask = "task"
)
```

**Predicates (Vocabulary):**

```go
const (
    // Decision tracking
    PredicateDecisionArchitecture = "agentic.decision.architecture"
    PredicateDecisionRationale    = "agentic.decision.rationale"
    PredicateDecisionAlternative  = "agentic.decision.alternative_considered"

    // File operations
    PredicateFileModified  = "agentic.file.modified"
    PredicateFileCreated   = "agentic.file.created"
    PredicateFileDeleted   = "agentic.file.deleted"
    PredicateFileRead      = "agentic.file.read"

    // File modification metadata (for file entities)
    PredicateModificationType   = "agentic.modification.type"    // bug_fix, feature, refactor
    PredicateModificationReason = "agentic.modification.reason"

    // State tracking
    PredicateCheckpointIteration = "agentic.checkpoint.iteration"
    PredicateCheckpointState     = "agentic.checkpoint.state"
    PredicateCheckpointTimestamp = "agentic.checkpoint.timestamp"

    // Tool usage patterns
    PredicateToolUsed       = "agentic.tool.used"
    PredicateToolSuccess    = "agentic.tool.success_count"
    PredicateToolFailure    = "agentic.tool.failure_count"

    // Task outcomes
    PredicateTaskOutcome    = "agentic.task.outcome"    // complete, failed
    PredicateTaskDuration   = "agentic.task.duration_ms"
    PredicateTaskIterations = "agentic.task.iterations"

    // Context management
    PredicateContextCompactedAt = "agentic.context.compacted_at"
    PredicateContextTokensUsed  = "agentic.context.tokens_used"
)
```

**File Entity Format:**

```go
// Entity ID format: file.{org}.{path_components}
// Example: file.semstreams.processor.graph.node.go
const (
    EntityTypeFile = "file"
)
```

### Extraction Rules

**Rule-Based Extraction (Every Iteration):**

1. **From ToolResult:**
   - Tool name → `(task, agentic.tool.used, tool_name)`
   - Success/failure → Update `agentic.tool.success_count` or `failure_count`
   - File modifications → `(task, agentic.file.modified, file_entity_id)`

2. **From AgentResponse:**
   - Iteration number → `(task, agentic.checkpoint.iteration, N)`
   - Current state → `(task, agentic.checkpoint.state, state)`
   - Timestamp → `(task, agentic.checkpoint.timestamp, time)`

3. **From Trajectory:**
   - Token usage → `(task, agentic.context.tokens_used, total)`
   - Duration → `(task, agentic.task.duration_ms, ms)`

**LLM-Assisted Extraction (Threshold-Based):**

Trigger conditions:

- Every N iterations (configurable, default: 5)
- OR context tokens exceed threshold (configurable, default: 80% of limit)
- OR explicit extraction request

Extraction prompt pattern:

```text
Extract structured facts from this agent response. Return JSON array of triples:

Response:
{agent_response_content}

Extract:
1. Key decisions made (architecture, design patterns, approach)
2. Rationale for decisions (why this choice vs alternatives)
3. Alternatives considered (what was rejected and why)
4. Critical findings or discoveries

Format as triples: [{"subject": "task_id", "predicate": "agentic.decision.X", "object": "value"}]
```

### Context Hydration Strategy

**Pre-Task Hydration:**

When new task arrives, query graph for relevant context:

```go
// Get recent tasks in same project
recentTasks := graph.Query(EntityCriteria{Type: "task"}, limit=10, sort="desc_by_timestamp")

// Get file modification history
fileHistory := graph.Query(RelationshipCriteria{
    Type: "agentic.file.modified",
    Direction: QueryDirectionOutgoing,
})

// Inject into system prompt
systemPrompt = fmt.Sprintf(`You are continuing work on this project.

Recent context:
- Modified files: %s
- Previous decisions: %s
- Tool usage patterns: %s

Current task: %s`, fileHistory, decisions, toolPatterns, task.Prompt)
```

**Post-Compaction Hydration:**

When context compaction occurs, reconstruct critical state:

```go
// Get checkpoint before compaction
checkpoint := graph.Query(Subject: taskID, Predicate: "agentic.checkpoint.*")

// Get all file modifications
files := graph.Query(Subject: taskID, Predicate: "agentic.file.modified")

// Get key decisions
decisions := graph.Query(Subject: taskID, Predicate: "agentic.decision.*")

// Reconstruct compressed context
compactContext = fmt.Sprintf(`Resuming from iteration %d (state: %s).

Files modified:
%s

Key decisions:
%s`, checkpoint.Iteration, checkpoint.State, formatFiles(files), formatDecisions(decisions))
```

### Integration with Agentic Loop

**Component Collaboration:**

```text
processor/agentic-loop (existing)
    │
    ├─► Publishes agent.response.* ─────┐
    ├─► Publishes tool.result.* ────────┼─► processor/agentic-memory (NEW)
    ├─► Publishes agent.compact.* ──────┤        │
    │                                    │        ├─► Extracts triples
    └─► Receives agent.context.injected.*        ├─► Stores to graph
                                                  └─► Hydrates on demand
```

**Trajectory Enhancement:**

Enhance `agentic.Trajectory` to reference memory checkpoints:

```go
type Trajectory struct {
    // ... existing fields ...
    MemoryCheckpoints []MemoryCheckpoint `json:"memory_checkpoints,omitempty"`
}

type MemoryCheckpoint struct {
    Iteration     int       `json:"iteration"`
    TriplesCount  int       `json:"triples_count"`
    ContextTokens int       `json:"context_tokens"`
    CheckpointID  string    `json:"checkpoint_id"` // KV key for recovery
    Timestamp     time.Time `json:"timestamp"`
}
```

### Configuration

```json
{
  "enabled": true,
  "extraction": {
    "rule_based": {
      "enabled": true,
      "track_files": true,
      "track_tools": true,
      "track_state": true
    },
    "llm_assisted": {
      "enabled": true,
      "trigger_interval": 5,
      "trigger_context_threshold_percent": 80,
      "model": "gpt-4o-mini",
      "max_tokens": 1000
    }
  },
  "hydration": {
    "pre_task": {
      "enabled": true,
      "max_recent_tasks": 10,
      "include_file_history": true,
      "include_decisions": true
    },
    "post_compaction": {
      "enabled": true,
      "reconstruct_from_checkpoint": true,
      "include_recent_iterations": 3
    }
  },
  "storage": {
    "checkpoint_bucket": "AGENT_MEMORY_CHECKPOINTS",
    "retention_days": 30
  }
}
```

### Integration with ADR-015: Context Memory Management

ADR-017 and ADR-015 form a **two-tier memory hierarchy**:

| Layer | ADR | Responsibility |
|-------|-----|----------------|
| Working Memory | ADR-015 | Manage context window, trigger compaction |
| External Memory | ADR-017 | Extract facts, enable recovery |

**Integration Points:**

1. **Compaction Signal**: ADR-017 subscribes to `agent.context.compaction.*` from ADR-015
2. **Extraction Trigger**: When compaction starts, ADR-017 extracts facts before history is lost
3. **Hydration Response**: After compaction completes, ADR-017 publishes hydrated context
4. **Threshold Monitoring**: ADR-017 can also trigger extraction at utilization thresholds

**Why this architecture:**

Without ADR-015, ADR-017 doesn't know *when* to extract or hydrate:

- No utilization tracking → no trigger signal
- No compaction events → no recovery opportunity

Without ADR-017, ADR-015 compaction is lossy:

- Text summaries lose structured relationships
- No way to answer "what files did I modify?"
- Agent confusion after compaction

**NATS Subject Integration:**

| Subscribed by ADR-017 | Published by ADR-015 |
|----------------------|---------------------|
| `agent.context.compaction.starting` | Compaction begins |
| `agent.context.compaction.complete` | Compaction finished |
| `agent.context.utilization.*` | Utilization updates |

| Published by ADR-017 | Subscribed by ADR-015 |
|---------------------|----------------------|
| `agent.context.injected.*` | Hydrated context ready |

**Standalone Operation:**

ADR-017 can operate without ADR-015 (manual extraction/hydration), but the integration provides:

- Automatic extraction at compaction boundaries
- Automatic hydration post-compaction
- Threshold-based extraction before utilization limits

## Consequences

### Positive

- **Unique Differentiator**: First agentic framework with graph-backed memory
- **Survives Compaction**: Agents don't lose critical context when summarized
- **Queryable History**: Can ask "what files did I modify?" across all iterations
- **Structured Recovery**: Post-compaction state reconstruction from graph
- **Cost Efficient**: Hybrid extraction balances accuracy with LLM cost
- **Selective Retrieval**: On-demand context loading instead of full history replay
- **Audit Trail**: Complete decision provenance in knowledge graph
- **Cross-Task Learning**: Future tasks can learn from past task patterns

### Negative

- **Extraction Overhead**: LLM-assisted extraction adds latency (mitigated by threshold triggering)
- **Graph Storage**: Long-running tasks accumulate many triples (mitigated by retention policy)
- **Query Complexity**: Hydration queries must be fast to not block loop (mitigated by graph indexing)
- **Extraction Accuracy**: LLM extraction may miss facts or hallucinate (mitigated by hybrid approach)
- **Configuration Complexity**: Multiple thresholds and toggles to tune

### Neutral

- **Opt-In**: Memory component optional, agentic-loop works standalone
- **Gradual Rollout**: Can enable rule-based first, add LLM-assisted later
- **Schema Evolution**: Predicate vocabulary will grow as use cases emerge
- **Performance Tuning**: Extraction/hydration thresholds will require tuning per use case

## Key Files

**Lean Component (NEW):**

| File | Purpose |
|------|---------|
| `processor/agentic-memory/component.go` | Lifecycle, NATS subscriptions |
| `processor/agentic-memory/llm_extractor.go` | Threshold-triggered LLM extraction |
| `processor/agentic-memory/hydrator.go` | Context hydration from graph |
| `processor/agentic-memory/config.go` | Configuration types |

**Extraction Rules (NEW):**

| File | Purpose |
|------|---------|
| `config/rules/agentic-memory/file-modifications.json` | Extract file modification facts |
| `config/rules/agentic-memory/tool-usage.json` | Extract tool call patterns |
| `config/rules/agentic-memory/state-checkpoints.json` | Extract iteration state |
| `config/rules/agentic-memory/decision-patterns.json` | Pattern-match decisions |

**Existing Files (Modified):**

| File | Purpose |
|------|---------|
| `processor/agentic-loop/trajectory.go` | Enhanced with MemoryCheckpoint |
| `agentic/types.go` | AgentRequest, AgentResponse |
| `graph/mutation_requests.go` | Graph triple insertion |
| `graph/query_types.go` | Graph query interfaces |

## References

- [Agentic Loop README](../../processor/agentic-loop/doc.go) - Loop orchestrator documentation
- [ADR-015: Context Memory Management](./adr-015-context-memory-management.md) - Complementary context window management
- [Knowledge Graph Package](../../graph/) - Graph storage and query operations
- [Triple Format](../../message/triple.go) - Semantic triple structure
- [ADR-013: Content Enrichment Pattern](./adr-013-content-enrichment-pattern.md) - Similar async worker pattern
- [Context Compaction Research](https://arxiv.org/abs/2309.XXXXX) - Industry approaches and failures
