# SemStreams Multi-Agent Specification

> **Status**: Planned (not yet implemented)
>
> **Core Integration**: This spec should integrate with the existing `ClassifierChain`
> in `graph/query/classifier_chain.go` rather than duplicating classification logic.
> The chain already supports tiered classification (T0 keyword → T1/T2 embedding).
> Recommended approach: add `Destination` field to `ClassificationResult` and define
> `Agent` interface in core.
>
> **Related**: See also [Component Model Selection](#component-model-selection) below
> for the related but distinct concern of per-component SLM endpoint configuration.

## Overview

Multi-agent is an **optional component** that provides specialized agent routing and
orchestration on top of SemStreams core query capabilities. It enables domain-specific
agents that can be trained, validated, and updated independently.

> **Relationship to Core:** Multi-agent consumes core query APIs and adds agent-level
> abstraction. Core query routing (template matching, PathRAG, GraphRAG) continues to
> function; multi-agent orchestrates which specialized model handles a query.

## Design Principles

1. **Layered on Core:** Uses core query infrastructure, doesn't replace it
2. **Independent Training:** Each agent trains on focused data, updates independently
3. **Graceful Degradation:** Falls back to general-agent or core query if specialized agent unavailable
4. **Tier-Aligned Routing:** Router respects SemStreams tier model (0/1/2)
5. **Registry-Driven:** Agent capabilities declared, not hardcoded

## Architecture

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                         Multi-Agent Architecture                             │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  User Query                                                                  │
│      │                                                                       │
│      ▼                                                                       │
│  ┌─────────────────┐                                                        │
│  │  Agent Router   │  Tier 0: Pattern matching (instant)                    │
│  │                 │  Tier 1: BM25 classifier (statistical)                 │
│  │  "Which agent?" │  Tier 2: LLM routing (semantic, fallback)              │
│  └────────┬────────┘                                                        │
│           │                                                                  │
│           ▼                                                                  │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                        AGENT REGISTRY                                │    │
│  ├─────────────────────────────────────────────────────────────────────┤    │
│  │                                                                      │    │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐              │    │
│  │  │ entity-agent │  │ rules-agent  │  │  flow-agent  │              │    │
│  │  │              │  │              │  │              │              │    │
│  │  │ "What is X?" │  │ "When should │  │ "Create a    │              │    │
│  │  │ "Describe X" │  │  I do Y?"    │  │  workflow"   │              │    │
│  │  └──────────────┘  └──────────────┘  └──────────────┘              │    │
│  │                                                                      │    │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐              │    │
│  │  │ graph-agent  │  │  doc-agent   │  │ general-agent│              │    │
│  │  │              │  │              │  │              │              │    │
│  │  │ "What's      │  │ "Find in     │  │ Fallback for │              │    │
│  │  │  related?"   │  │  manual..."  │  │ unmatched    │              │    │
│  │  └──────────────┘  └──────────────┘  └──────────────┘              │    │
│  │                                                                      │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│           │                                                                  │
│           ▼                                                                  │
│  ┌─────────────────┐                                                        │
│  │  Core Query     │  PathRAG, GraphRAG, template routing                   │
│  │  (unchanged)    │  Agent provides context/model, core executes           │
│  └─────────────────┘                                                        │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Agent Specializations

| Agent | Training Sources | Query Patterns | Output Type |
|-------|------------------|----------------|-------------|
| **entity-agent** | Entity descriptions, aliases | "What is X?", "Describe X", "Tell me about X" | Factual answers |
| **graph-agent** | Relationships, communities | "What's related to X?", "What connects X and Y?" | Lists, paths |
| **rules-agent** | Rule definitions, trigger history | "What should I do when X?", "What's the protocol for Y?" | Procedures, actions |
| **flow-agent** | Flow definitions, workflow patterns | "Create a workflow for X", "Automate Y" | YAML/JSON workflows |
| **doc-agent** | Document chunks, synthetic QA | "Find in the manual...", "What does SOP say about X?" | Cited passages |
| **general-agent** | All sources (sampled) | Fallback for unmatched queries | General responses |

### Agent Capabilities by Tier

Not all agents require the same tier level:

| Agent | Minimum Tier | Notes |
|-------|--------------|-------|
| entity-agent | 0 | Can lookup from graph without LLM |
| graph-agent | 0 | Traversal is deterministic |
| rules-agent | 1 | Benefits from BM25 for rule matching |
| flow-agent | 2 | Generation requires LLM |
| doc-agent | 2 | Synthesis requires LLM |
| general-agent | 1 | Statistical fallback; 2 for best results |

## Router Design

The router determines which agent handles a query. It operates in tiers aligned with SemStreams core tiers.

### Tier 0: Pattern Matching

Instant, deterministic routing based on query patterns.

```go
type PatternRouter struct {
    patterns []PatternRule
}

type PatternRule struct {
    Pattern  string // Glob or regex
    AgentID  string
    Priority int
}

// Example patterns
var defaultPatterns = []PatternRule{
    {Pattern: "what is *", AgentID: "entity-agent", Priority: 100},
    {Pattern: "describe *", AgentID: "entity-agent", Priority: 100},
    {Pattern: "who is *", AgentID: "entity-agent", Priority: 100},
    {Pattern: "* related to *", AgentID: "graph-agent", Priority: 90},
    {Pattern: "what connects *", AgentID: "graph-agent", Priority: 90},
    {Pattern: "when should *", AgentID: "rules-agent", Priority: 90},
    {Pattern: "what * protocol *", AgentID: "rules-agent", Priority: 85},
    {Pattern: "create * workflow *", AgentID: "flow-agent", Priority: 90},
    {Pattern: "automate *", AgentID: "flow-agent", Priority: 80},
    {Pattern: "* manual *", AgentID: "doc-agent", Priority: 70},
    {Pattern: "* documentation *", AgentID: "doc-agent", Priority: 70},
    // No match → Tier 1
}

func (r *PatternRouter) Route(query string) (agentID string, matched bool) {
    query = strings.ToLower(query)
    for _, rule := range r.patterns {
        if matchGlob(rule.Pattern, query) {
            return rule.AgentID, true
        }
    }
    return "", false
}
```

### Tier 1: Statistical Classifier

Uses BM25 against agent descriptions and example queries.

```go
type StatisticalRouter struct {
    registry *AgentRegistry
    index    *BM25Index  // Core index component
}

func (r *StatisticalRouter) Route(ctx context.Context, query string) (agentID string, confidence float64) {
    // Build search corpus from registry
    // Each agent has description + example queries indexed
    results := r.index.Search(ctx, query, 5)
    
    if len(results) == 0 {
        return "", 0
    }
    
    // Score by agent
    scores := make(map[string]float64)
    for _, result := range results {
        agentID := result.Metadata["agent_id"]
        scores[agentID] += result.Score
    }
    
    // Find best
    var bestAgent string
    var bestScore float64
    for agent, score := range scores {
        if score > bestScore {
            bestAgent = agent
            bestScore = score
        }
    }
    
    // Normalize confidence
    confidence = bestScore / (bestScore + 1.0) // Sigmoid-ish
    
    return bestAgent, confidence
}
```

**Threshold:** If confidence > 0.7, route to agent. Otherwise, fall through to Tier 2.

### Tier 2: LLM Routing

Uses SemInstruct for complex disambiguation.

```go
type LLMRouter struct {
    registry  *AgentRegistry
    instruct  *SemInstruct  // Core Tier 2 component
}

func (r *LLMRouter) Route(ctx context.Context, query string) (agentID string, reasoning string, err error) {
    // Build prompt with registry info
    agents := r.registry.ListActive(ctx)
    
    prompt := "Given the following specialized agents and a user query, select the most appropriate agent.\n\n"
    prompt += "AGENTS:\n"
    for _, agent := range agents {
        prompt += fmt.Sprintf("- %s: %s\n", agent.ID, agent.Description)
        prompt += fmt.Sprintf("  Examples: %s\n", strings.Join(agent.Examples[:3], "; "))
    }
    prompt += fmt.Sprintf("\nQUERY: %s\n\n", query)
    prompt += "Respond with JSON: {\"agent_id\": \"...\", \"reasoning\": \"...\"}"
    
    response, err := r.instruct.Complete(ctx, prompt)
    if err != nil {
        return "general-agent", "", err // Fallback
    }
    
    var result struct {
        AgentID   string `json:"agent_id"`
        Reasoning string `json:"reasoning"`
    }
    if err := json.Unmarshal([]byte(response), &result); err != nil {
        return "general-agent", "", err
    }
    
    return result.AgentID, result.Reasoning, nil
}
```

**Side effect:** LLM routing decisions are logged and become training data for Tier 1 classifier (distillation).

### Combined Router

```go
type AgentRouter struct {
    pattern     *PatternRouter
    statistical *StatisticalRouter
    llm         *LLMRouter
    config      RouterConfig
}

type RouterConfig struct {
    StatisticalThreshold float64 // Default 0.7
    EnableLLMFallback    bool    // Default true if Tier 2 available
    LogRoutingDecisions  bool    // For training data collection
}

func (r *AgentRouter) Route(ctx context.Context, query string) (*RoutingDecision, error) {
    decision := &RoutingDecision{
        Query:     query,
        Timestamp: time.Now(),
    }
    
    // Tier 0: Pattern matching
    if agentID, matched := r.pattern.Route(query); matched {
        decision.AgentID = agentID
        decision.Tier = 0
        decision.Confidence = 1.0
        decision.Method = "pattern"
        return decision, nil
    }
    
    // Tier 1: Statistical
    agentID, confidence := r.statistical.Route(ctx, query)
    if confidence >= r.config.StatisticalThreshold {
        decision.AgentID = agentID
        decision.Tier = 1
        decision.Confidence = confidence
        decision.Method = "statistical"
        return decision, nil
    }
    
    // Tier 2: LLM (if available)
    if r.config.EnableLLMFallback && r.llm != nil {
        agentID, reasoning, err := r.llm.Route(ctx, query)
        if err == nil {
            decision.AgentID = agentID
            decision.Tier = 2
            decision.Confidence = 0.9 // LLM decisions assumed high confidence
            decision.Method = "llm"
            decision.Reasoning = reasoning
            
            // Log for distillation to Tier 1
            if r.config.LogRoutingDecisions {
                r.logForTraining(decision)
            }
            
            return decision, nil
        }
    }
    
    // Fallback: general-agent
    decision.AgentID = "general-agent"
    decision.Tier = -1
    decision.Confidence = 0.5
    decision.Method = "fallback"
    return decision, nil
}
```

## Agent Registry

### Data Structure

```go
type AgentRegistry struct {
    store nats.KeyValue // AGENT_REGISTRY bucket
}

type AgentDescriptor struct {
    ID           string            `json:"id"`
    Name         string            `json:"name"`
    Description  string            `json:"description"`
    
    // Routing hints (for Tier 0/1)
    Patterns     []string          `json:"patterns"`       // Glob patterns
    Examples     []string          `json:"examples"`       // Example queries
    Keywords     []string          `json:"keywords"`       // BM25 boost terms
    
    // Capabilities
    MinimumTier  int               `json:"minimum_tier"`   // 0, 1, or 2
    RequiresLLM  bool              `json:"requires_llm"`   // Needs SemInstruct
    OutputTypes  []string          `json:"output_types"`   // "text", "yaml", "list"
    
    // Model configuration
    Model        ModelConfig       `json:"model"`
    
    // Runtime state
    Status       string            `json:"status"`         // "active", "training", "disabled"
    Version      string            `json:"version"`        // Adapter version
    
    // Metrics (updated periodically)
    Metrics      AgentMetrics      `json:"metrics"`
}

type ModelConfig struct {
    BaseModel    string `json:"base_model"`    // e.g., "llama-3.2-8b"
    AdapterPath  string `json:"adapter_path"`  // e.g., "/models/adapters/entity-agent-v3"
    UseAdapter   bool   `json:"use_adapter"`   // false = use base only
}

type AgentMetrics struct {
    TotalQueries     int64         `json:"total_queries"`
    AvgLatency       time.Duration `json:"avg_latency"`
    SuccessRate      float64       `json:"success_rate"`      // Based on feedback
    LastUpdated      time.Time     `json:"last_updated"`
}
```

### Registry Operations

```go
// Register or update an agent
func (r *AgentRegistry) Register(ctx context.Context, agent AgentDescriptor) error

// Get agent by ID
func (r *AgentRegistry) Get(ctx context.Context, id string) (*AgentDescriptor, error)

// List all agents (optionally filter by status)
func (r *AgentRegistry) List(ctx context.Context, status string) ([]AgentDescriptor, error)

// List only active agents
func (r *AgentRegistry) ListActive(ctx context.Context) ([]AgentDescriptor, error)

// Update agent status
func (r *AgentRegistry) SetStatus(ctx context.Context, id string, status string) error

// Update agent metrics
func (r *AgentRegistry) UpdateMetrics(ctx context.Context, id string, metrics AgentMetrics) error

// Watch for registry changes
func (r *AgentRegistry) Watch(ctx context.Context) (<-chan AgentChange, error)
```

### Default Agent Definitions

```yaml
agents:
  - id: entity-agent
    name: Entity Agent
    description: Answers factual questions about entities, equipment, and concepts
    patterns:
      - "what is *"
      - "describe *"
      - "who is *"
      - "tell me about *"
    examples:
      - "What is CTD-07?"
      - "Describe the sampling protocol"
      - "Who is the chief scientist?"
    keywords: ["what", "describe", "entity", "equipment", "sensor"]
    minimum_tier: 0
    requires_llm: false
    output_types: ["text"]
    model:
      base_model: "llama-3.2-8b"
      adapter_path: "/models/adapters/entity-agent"
      use_adapter: true

  - id: graph-agent
    name: Graph Agent  
    description: Answers questions about relationships and connections between entities
    patterns:
      - "* related to *"
      - "what connects *"
      - "what * connected to *"
      - "show relationships *"
    examples:
      - "What sensors are related to station 4?"
      - "What connects CTD-07 and the rosette?"
      - "Show relationships for the water sampling equipment"
    keywords: ["related", "connected", "relationship", "between", "link"]
    minimum_tier: 0
    requires_llm: false
    output_types: ["list", "text"]
    model:
      base_model: "llama-3.2-8b"
      adapter_path: "/models/adapters/graph-agent"
      use_adapter: true

  - id: rules-agent
    name: Rules Agent
    description: Answers questions about procedures, protocols, and what to do in specific situations
    patterns:
      - "when should *"
      - "what * protocol *"
      - "what should I do *"
      - "procedure for *"
    examples:
      - "When should I recalibrate the CTD?"
      - "What's the protocol for temperature anomalies?"
      - "What should I do if salinity drops below threshold?"
    keywords: ["when", "should", "protocol", "procedure", "rule", "action"]
    minimum_tier: 1
    requires_llm: false
    output_types: ["text"]
    model:
      base_model: "llama-3.2-8b"
      adapter_path: "/models/adapters/rules-agent"
      use_adapter: true

  - id: flow-agent
    name: Flow Agent
    description: Creates and explains workflows and automations
    patterns:
      - "create * workflow *"
      - "automate *"
      - "build * flow *"
      - "set up * automation *"
    examples:
      - "Create a workflow for daily sensor checks"
      - "Automate the sampling notification process"
      - "Build a flow to alert on anomalies"
    keywords: ["create", "workflow", "automate", "flow", "automation", "build"]
    minimum_tier: 2
    requires_llm: true
    output_types: ["yaml", "text"]
    model:
      base_model: "llama-3.2-8b"
      adapter_path: "/models/adapters/flow-agent"
      use_adapter: true

  - id: doc-agent
    name: Document Agent
    description: Finds and synthesizes information from documents and manuals
    patterns:
      - "* manual *"
      - "* documentation *"
      - "find in * document *"
      - "what does * say about *"
    examples:
      - "What does the operations manual say about night sampling?"
      - "Find in the equipment documentation how to reset CTD"
      - "What are the safety procedures in the manual?"
    keywords: ["manual", "document", "documentation", "find", "says", "written"]
    minimum_tier: 2
    requires_llm: true
    output_types: ["text"]
    model:
      base_model: "llama-3.2-8b"
      adapter_path: "/models/adapters/doc-agent"
      use_adapter: true

  - id: general-agent
    name: General Agent
    description: Fallback agent for queries that don't match specialized agents
    patterns: []  # No patterns - only reached via fallback
    examples:
      - "Help me with something"
      - "I have a question"
    keywords: []
    minimum_tier: 1
    requires_llm: false
    output_types: ["text"]
    model:
      base_model: "llama-3.2-8b"
      adapter_path: "/models/adapters/general-agent"
      use_adapter: true
```

## Integration with Training

Multi-agent changes how training data is partitioned and used.

### Per-Agent Training Data

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Training Pipeline with Multi-Agent                        │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  EXTRACTION (unchanged - from core)                                         │
│  ──────────────────────────────────                                         │
│  EntityExtractor ────────┐                                                   │
│  RelationshipExtractor ──┤                                                   │
│  RuleExtractor ──────────┼──► Raw QA Pairs                                   │
│  QueryExtractor ─────────┤                                                   │
│  DocumentExtractor ──────┘                                                   │
│                                                                              │
│  PARTITIONING (new - multi-agent aware)                                     │
│  ──────────────────────────────────────                                     │
│                    ┌──────────────────┐                                      │
│  Raw QA Pairs ────►│  AgentPartitioner │                                     │
│                    │                   │                                     │
│                    │  Routes pairs to  │                                     │
│                    │  agent buckets by │                                     │
│                    │  source type      │                                     │
│                    └─────────┬─────────┘                                     │
│                              │                                               │
│         ┌────────────────────┼────────────────────┐                         │
│         ▼                    ▼                    ▼                         │
│  ┌─────────────┐      ┌─────────────┐      ┌─────────────┐                 │
│  │entity-agent │      │ rules-agent │      │ flow-agent  │  ...            │
│  │  /train     │      │  /train     │      │  /train     │                 │
│  └─────────────┘      └─────────────┘      └─────────────┘                 │
│                                                                              │
│  ROUTER TRAINING (new)                                                       │
│  ─────────────────────                                                       │
│                    ┌──────────────────┐                                      │
│  Routing Log ─────►│  RouterTrainer   │                                      │
│  (with outcomes)   │                   │                                     │
│                    │  Builds Tier 1   │                                     │
│                    │  classifier data │                                     │
│                    └─────────┬─────────┘                                     │
│                              ▼                                               │
│                       ┌─────────────┐                                        │
│                       │   router    │                                        │
│                       │  /train     │                                        │
│                       └─────────────┘                                        │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Partitioning Rules

| Source | Target Agent |
|--------|--------------|
| Entity descriptions | entity-agent |
| Entity aliases | entity-agent |
| Relationships | graph-agent |
| Communities | graph-agent |
| Rule definitions | rules-agent |
| Rule triggers | rules-agent |
| Flow definitions | flow-agent |
| Workflow patterns | flow-agent |
| Document chunks | doc-agent |
| Synthetic document QA | doc-agent |
| Query logs (by resolution) | Agent that successfully answered |
| All sources (sampled) | general-agent |

### Router Training Data

The router needs its own training data: query → agent_id mappings.

```go
type RouterTrainingPair struct {
    Query       string  `json:"query"`
    AgentID     string  `json:"agent_id"`
    Confidence  float64 `json:"confidence"`
    Source      string  `json:"source"`
}
```

**Sources for router training:**

| Source | Description | Weight |
|--------|-------------|--------|
| Explicit selection | User explicitly chose agent | 1.0 |
| Successful resolution | Agent returned useful result (feedback) | 0.9 |
| LLM routing | Tier 2 decisions (distillation to Tier 1) | 0.7 |
| Pattern inference | Derived from pattern matches | 0.8 |

### Training Configuration

```yaml
training:
  # Enable multi-agent partitioning
  multi_agent:
    enabled: true
    
  # Per-agent configuration
  agents:
    entity-agent:
      sources:
        - type: entities
          weight: 1.0
        - type: aliases
          weight: 1.0
      min_pairs: 500
      
    graph-agent:
      sources:
        - type: relationships
          weight: 1.0
        - type: communities
          weight: 0.8
      min_pairs: 1000
      
    rules-agent:
      sources:
        - type: rules
          weight: 1.0
        - type: triggers
          weight: 0.8
      min_pairs: 200
      
    flow-agent:
      sources:
        - type: flows
          weight: 1.0
        - type: workflow_patterns
          weight: 0.5
      requires_llm: true
      min_pairs: 100
      
    doc-agent:
      sources:
        - type: documents
          weight: 1.0
      requires_llm: true
      min_pairs: 500
      
    general-agent:
      sources:
        - type: all
          sample_rate: 0.1  # 10% sample from all
      min_pairs: 1000
      
  # Router classifier training
  router:
    enabled: true
    sources:
      - type: routing_log
        require_outcome: true
      - type: llm_routing_log
        weight: 0.7
    model: distilbert-base  # Small classifier
    epochs: 5
```

## DDIL Considerations

Agent availability varies by tier:

```text
┌─────────────────────────────────────────────────────────────────┐
│                 Agent Availability by Tier                       │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  TIER 0 (Always available, no models):                          │
│  ├── Router: Pattern matching only                              │
│  ├── entity-agent: Direct graph lookup                          │
│  └── graph-agent: Direct traversal                              │
│                                                                  │
│  TIER 1 (Requires SLM):                                         │
│  ├── Router: BM25 classifier                                    │
│  ├── rules-agent: Procedure generation                          │
│  └── general-agent: Fallback responses                          │
│                                                                  │
│  TIER 2 (Requires LLM via SemInstruct):                         │
│  ├── Router: LLM disambiguation                                 │
│  ├── flow-agent: Workflow generation                            │
│  └── doc-agent: Document synthesis                              │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### Degradation Strategy

```go
func (r *AgentRouter) RouteWithDegradation(ctx context.Context, query string, availableTier int) (*RoutingDecision, error) {
    decision, err := r.Route(ctx, query)
    if err != nil {
        return nil, err
    }
    
    // Check if selected agent is available at current tier
    agent, _ := r.registry.Get(ctx, decision.AgentID)
    if agent.MinimumTier > availableTier {
        // Agent requires higher tier than available
        // Fall back to general-agent or tier-appropriate alternative
        return r.findFallback(ctx, query, availableTier)
    }
    
    return decision, nil
}

func (r *AgentRouter) findFallback(ctx context.Context, query string, maxTier int) (*RoutingDecision, error) {
    // Find best available agent at or below maxTier
    agents := r.registry.ListActive(ctx)
    
    var candidates []AgentDescriptor
    for _, agent := range agents {
        if agent.MinimumTier <= maxTier {
            candidates = append(candidates, agent)
        }
    }
    
    // Use statistical routing among candidates only
    // ... routing logic among candidates ...
}
```

## Data Model

### Buckets

| Bucket | Purpose |
|--------|---------|
| `AGENT_REGISTRY` | Agent definitions and status |
| `ROUTING_LOG` | Routing decisions for training |

### Streams

| Stream | Purpose |
|--------|---------|
| `agent.routed` | Routing decision events |
| `agent.feedback` | User feedback on agent responses |
| `agent.metrics` | Periodic metrics updates |

### Subjects

| Subject | Purpose |
|---------|---------|
| `agent.route.request` | Request routing decision |
| `agent.route.response` | Routing decision response |
| `agent.query.{agent_id}` | Query specific agent |
| `agent.registry.update` | Registry change notifications |

## API

### Route Query

```bash
# Request routing
nats req agent.route.request '{
  "query": "What sensors are at station 4?",
  "context": {"user": "researcher1"}
}'

# Response
{
  "agent_id": "graph-agent",
  "tier": 1,
  "confidence": 0.85,
  "method": "statistical"
}
```

### Query Agent Directly

```bash
# Query specific agent
nats req agent.query.entity-agent '{
  "query": "What is CTD-07?",
  "context": {}
}'

# Response
{
  "response": "CTD-07 is a conductivity-temperature-depth sensor...",
  "sources": ["entity:ctd-07"],
  "confidence": 0.95
}
```

### Registry Management

```bash
# List agents
nats req agent.registry.list '{}'

# Get agent details
nats req agent.registry.get '{"id": "entity-agent"}'

# Update agent status
nats req agent.registry.status '{
  "id": "flow-agent",
  "status": "disabled"
}'
```

### Feedback

```bash
# Positive feedback
nats pub agent.feedback '{
  "routing_id": "r-12345",
  "agent_id": "graph-agent", 
  "feedback": "positive"
}'

# Negative feedback (triggers potential reroute logging)
nats pub agent.feedback '{
  "routing_id": "r-12345",
  "agent_id": "graph-agent",
  "feedback": "negative",
  "correct_agent": "entity-agent"
}'
```

## Metrics

| Metric | Description |
|--------|-------------|
| `agent_routing_total` | Total routing decisions by tier and method |
| `agent_routing_latency_seconds` | Routing decision latency by tier |
| `agent_query_total` | Queries per agent |
| `agent_query_latency_seconds` | Query latency per agent |
| `agent_feedback_total` | Feedback events by type |
| `agent_fallback_total` | Fallback events (agent unavailable) |

## Configuration

```yaml
multi_agent:
  enabled: true
  
  router:
    # Tier 0: Pattern matching
    patterns:
      enabled: true
      # Additional patterns loaded from registry
      
    # Tier 1: Statistical
    statistical:
      enabled: true
      threshold: 0.7
      
    # Tier 2: LLM routing
    llm:
      enabled: true  # Only if Tier 2 available
      log_for_training: true
      
  registry:
    bucket: AGENT_REGISTRY
    watch_changes: true
    
  fallback:
    agent: general-agent
    strategy: tier_appropriate  # or "always_general"
    
  logging:
    routing_decisions: true
    feedback: true
```

## Summary

| Component | Purpose | Depends On |
|-----------|---------|------------|
| **Agent Router** | Query → agent routing | Core query, registry |
| **Agent Registry** | Agent definitions, status | NATS KV |
| **Agent Partitioner** | Training data routing | Training pipeline |
| **Router Trainer** | Tier 1 classifier training | Training pipeline, routing log |

**Key insight:** Multi-agent is orchestration *over* core capabilities, not replacement.
Each agent is a specialized adapter that improves quality for its domain while core query
infrastructure does the actual work.

## Component Model Selection

A related but distinct concern is **component-level model selection**: allowing individual
components to specify which SLM/agent endpoint handles their LLM tasks.

### Distinction from Query Routing

| Concern | What It Does | Example |
|---------|--------------|---------|
| Query Routing (this spec) | Route user queries to agents | "What is CTD-07?" → entity-agent |
| Component Model Selection | Configure component's model | graph-clustering → summarization-SLM |

### Why This Matters

At the edge with limited compute, a stable of well-trained SLMs is better than one LLM.
Different components have different needs:

- `graph-clustering` needs summarization capability
- `graph-embedding` needs embedding capability
- `processor/rule/` might need reasoning capability

### Proposed Approach

This should be a **core capability** (not part of multi-agent) because:

- Affects how all Tier 2 components get LLM access
- Is infrastructure, not optional feature
- Needed before multi-agent query routing

Example component config:

```yaml
components:
  graph-clustering:
    model: "summarization"  # capability-based selection
    # OR
    model_endpoint: "http://localhost:8080/summarization"  # direct endpoint
```

Core would need:

1. Model/agent registry (what endpoints are available)
2. Capability mapping (what each model is good at)
3. Component config field for model requirements
4. Resolution logic: capability → available endpoint

### Relationship to Multi-Agent

Once component model selection exists in core:

- Multi-agent query routing becomes simpler (routes to agents, doesn't manage endpoints)
- Agents can declare their model requirements using the same mechanism
- Training pipeline can target specific model endpoints
