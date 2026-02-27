# Boids Coordination for SemSpec

**Status**: Proposal
**Author**: SemStreams Team
**Date**: 2026-02-27

## Executive Summary

This proposal describes how to integrate SemStreams' Boids coordination system into SemSpec's multi-agent workflows. Boids enables emergent coordination alongside explicit choreography, allowing multiple developers and reviewers to self-organize based on local rules rather than centralized dispatch.

## Background

### What is Boids?

Craig Reynolds' Boids (1986) demonstrates how complex flocking behavior emerges from three simple local rules:

| Rule | Behavior |
|------|----------|
| **Separation** | Avoid crowding neighbors |
| **Cohesion** | Steer toward average position of neighbors |
| **Alignment** | Match velocity with neighbors |

### SemStreams Implementation

We've implemented graph-native Boids rules in SemStreams:

| Component | Location | Purpose |
|-----------|----------|---------|
| `AgentPosition` | `processor/rule/boid/types.go` | Track agent focus entities, traversal patterns, velocity |
| `SteeringSignal` | `processor/rule/boid/types.go` | Publish avoid/suggest/align signals |
| Separation Rule | `processor/rule/boid/separation.go` | Detect k-hop proximity conflicts |
| Cohesion Rule | `processor/rule/boid/cohesion.go` | Steer toward high-PageRank nodes |
| Alignment Rule | `processor/rule/boid/alignment.go` | Match same-role traversal patterns |
| Position Tracking | `processor/agentic-loop/boid_handler.go` | Update positions on tool execution |

Enable with config:
```json
{
  "boid_enabled": true,
  "positions_bucket": "AGENT_POSITIONS"
}
```

## SemSpec Context

SemSpec is spec-driven agentic development with:

- **Adversarial loop**: `developer ↔ reviewer` (max 3 retries)
- **Workflow phases**: plan → plan-review → phase-gen → task-gen → task-execution
- **Entities**: Code AST nodes, specs, SOPs, plans, tasks in knowledge graph
- **Current coordination**: Explicit reactive workflows with fixed routing

## How Boids Apply to SemSpec

### Scenario 1: Multi-Developer Task Execution

**Current**: Tasks dispatched by `task-dispatcher` with dependency awareness only.

**With Boids**:
```
Developer A working on: auth/login.go, auth/types.go
Developer B receives SEPARATION signal: "avoid auth/* (within 2-hop)"
Developer B picks: api/handlers.go instead
```

The knowledge graph connects code entities. Separation prevents conflicts.

### Scenario 2: Reviewer Load Balancing

**Current**: Fixed developer→reviewer pairing, linear retry.

**With Boids**:
```
Reviewer pool: [R1 (queue=5), R2 (queue=1), R3 (queue=3)]
New review comes in
R2 has highest COHESION toward idle state → gets task
```

Reviewers self-organize based on queue depth perception.

### Scenario 3: Context-Aware Task Selection

**Current**: Tasks dispatched based on DAG dependencies.

**With Boids**:
```
Developer just completed: auth/middleware.go
Next available tasks: [auth/session.go, api/routes.go, db/migrations.go]

COHESION signal: auth/session.go has high PageRank + shares imports
Developer gravitates toward auth/session.go (context already loaded)
```

### Scenario 4: Aligned Code Patterns

**Current**: Each developer works independently, reviewer catches style issues.

**With Boids**:
```
Developers D1, D2, D3 all working on "add logging" feature
D1's TraversalVector: [uses_logger, wraps_error, returns_error]
ALIGNMENT signal to D2, D3: "align with [uses_logger, wraps_error]"
```

Same-role agents converge on consistent patterns proactively.

## Proposed Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         SemSpec                                  │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  task-dispatcher                                                 │
│  ├── Subscribe to: agent.boid.* (steering signals)              │
│  ├── Query: AGENT_POSITIONS KV (current positions)              │
│  └── Apply: separation-aware task assignment                    │
│                                                                  │
│  developer (uses agentic-loop)                                   │
│  ├── Config: boid_enabled=true                                   │
│  ├── Position updates: on tool execution                         │
│  └── Consumes: steering signals for context hints               │
│                                                                  │
│  context-builder                                                 │
│  ├── Query: AGENT_POSITIONS for focus entities                  │
│  └── Apply: cohesion (prioritize high-centrality context)       │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                        SemStreams                                │
├─────────────────────────────────────────────────────────────────┤
│  AGENT_POSITIONS KV ──► rule-processor (boid rules)             │
│                              │                                   │
│                              ▼                                   │
│                         agent.boid.* (steering signals)          │
│                              │                                   │
│                              ▼                                   │
│                    agentic-loop (position tracking)              │
└─────────────────────────────────────────────────────────────────┘
```

## Integration Points

| SemSpec Component | Integration | Purpose |
|-------------------|-------------|---------|
| `task-dispatcher` | Subscribe `agent.boid.separation.*` | Avoid assigning tasks that conflict with active developers |
| `task-dispatcher` | Query `AGENT_POSITIONS` | Know what each developer is working on |
| `developer` | Enable `boid_enabled: true` | Track position in AGENT_POSITIONS |
| `context-builder` | Use cohesion signals | Prioritize entities near high-centrality nodes |
| `developer` prompt | Include alignment hints | "Other developers are using: [pattern1, pattern2]" |

## End-to-End Flow

```
1. Task Dispatch (separation-aware)
   ├── task-dispatcher receives: [task-A (auth/login.go), task-B (api/routes.go)]
   ├── Query AGENT_POSITIONS: Developer-1 focused on [auth/session.go, auth/types.go]
   ├── Separation rule: task-A is within 2-hop of Developer-1's focus
   └── Assign: task-A → Developer-2, task-B → Developer-1 (or queue)

2. Developer Execution (position tracking)
   ├── Developer-2 starts task-A
   ├── Tool call: file_read("auth/login.go")
   ├── BoidHandler extracts entities: [auth.login, auth.LoginRequest, ...]
   ├── Position update: AGENT_POSITIONS["loop-xyz"] = {focus: [auth.login, ...]}
   └── Steering signals published if other agents nearby

3. Context Building (cohesion-aware)
   ├── Developer-2 needs context for auth/login.go
   ├── Query graph: PageRank identifies auth/middleware.go as high-centrality
   ├── Cohesion: include auth/middleware.go in context (it's a hub)
   └── Build context with cohesion-prioritized entities

4. Pattern Alignment
   ├── Developer-1 completed auth/session.go using: [structured_errors, context_propagation]
   ├── Alignment rule: Developer-2 working on same feature
   ├── Prompt includes: "Align with patterns: structured_errors, context_propagation"
   └── Developer-2 naturally follows same patterns
```

## Implementation Phases

### Phase 0: Enable Position Tracking (Config only)

- Update SemSpec developer config: `boid_enabled: true`
- Ensure AGENT_POSITIONS bucket created in flow setup
- Verify positions are tracked when developers run

**Effort**: Config change only

### Phase 1: Separation-Aware Task Dispatch

- Modify `task-dispatcher` to query AGENT_POSITIONS before assignment
- Implement separation check: "is task's target file within k-hop of any active developer's focus?"
- Route tasks to developers with non-overlapping focus areas

**Files**: `semspec/processor/task-dispatcher/component.go`

### Phase 2: Cohesion in Context Building

- Add PageRank/centrality query to context-builder
- Prioritize high-centrality entities when assembling context
- Especially useful for "related code" strategy

**Files**: `semspec/processor/context-builder/strategies/`

### Phase 3: Alignment Prompt Injection

- Subscribe developer to alignment signals
- Extract TraversalVector patterns from same-role agents
- Inject as hints in system prompt: "Follow patterns: [X, Y, Z]"

**Files**: `semspec/processor/developer/component.go`

### Phase 4: Signal Consumption in agentic-loop

- Add subscription to `agent.boid.<loopID>` in agentic-loop
- Store active steering signals per-loop
- Use signals to influence context slicing (avoid entities, prefer entities)

**Files**: `semstreams/processor/agentic-loop/component.go`

### Phase 5: Metrics & Experimentation

- Add metrics: task_reassignment_count, separation_signal_count, alignment_convergence
- A/B test: Boids-enabled vs disabled
- Compare: conflict rate, retry rate, time-to-completion

## Configuration Examples

### SemSpec Flow Config

```json
{
  "components": {
    "developer": {
      "config": {
        "boid_enabled": true,
        "positions_bucket": "AGENT_POSITIONS"
      }
    }
  }
}
```

### Boid Rules

```json
{
  "id": "semspec_separation",
  "type": "boid",
  "entity": {
    "watch_buckets": ["AGENT_POSITIONS"]
  },
  "metadata": {
    "boid_rule": "separation",
    "role_filter": "developer",
    "role_thresholds": {
      "developer": 2,
      "reviewer": 3
    },
    "steering_strength": 0.8
  }
}
```

## Verification

### Phase 0 (Position Tracking)
```bash
nats kv get AGENT_POSITIONS <loop-id>
# Expected: {"loop_id": "...", "focus_entities": ["auth.login", ...], "velocity": 0.5}
```

### Phase 1 (Separation)
```bash
# Start two developers on overlapping tasks
# Expected log: "Separation: task-A reassigned from Dev-1 to Dev-2 (conflict with auth/*)"
```

### Full E2E Test
```bash
task e2e:agentic -- -run TestBoidCoordination
# 1. Start 3 developers in parallel
# 2. Dispatch 6 tasks across auth/, api/, db/ packages
# 3. Verify: no file conflicts, patterns aligned, high-centrality context used
```

## Metrics to Track

| Metric | Description |
|--------|-------------|
| `semspec_task_dispatch_separation_applied` | Count of separation-influenced assignments |
| `semspec_context_cohesion_entities` | Entities added via cohesion |
| `semspec_alignment_patterns_injected` | Alignment hints added to prompts |
| `semspec_developer_conflict_rate` | File conflicts (should decrease) |

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Performance overhead | Cache PageRank results, compute only on significant position changes |
| Oscillation between states | Add damping factor and cooldown between signal publications |
| High event volume | Debounce position updates if KV write rate becomes problematic |

## Questions for SemSpec Team

1. How are multiple developers currently orchestrated? Is there existing conflict detection?
2. Which phase would provide the most immediate value?
3. Are there specific task types where Boids would be most beneficial (e.g., large refactors)?
4. Should Boids be opt-in per-workflow or globally enabled?

## References

- [Craig Reynolds' Boids](https://www.red3d.com/cwr/boids/)
- SemStreams Boid implementation: `processor/rule/boid/`
- SemStreams agentic-loop: `processor/agentic-loop/boid_handler.go`
