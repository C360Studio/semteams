# Boids Coordination: Honest Assessment

**Status**: Review
**Date**: 2026-02-27

Assessment of the Boids theory applied to SemStreams and the proposed A/B test plan. Based on full codebase exploration of rule implementations, position tracking, signal generation, agentic-loop integration, and the proposal doc.

## What's Genuinely Good

**Architectural fit is excellent.** SemStreams is already event-driven with reactive rules, KV state, and message passing. Boids fits naturally as another rule type — no new infrastructure primitives. The types, factory, and rule implementations are clean Go code following existing patterns.

**Knowledge graph as "space" is a real insight.** Entity IDs become coordinates, k-hop distance becomes spatial proximity, PageRank becomes gravitational pull. This isn't a forced metaphor — agents really do navigate a graph topology.

**Observability is built-in.** Positions in KV and signals on NATS subjects give you `nats kv watch AGENT_POSITIONS` and `nats sub "agent.boid.>"` visibility for free.

**Decentralized coordination scales.** At 10+ concurrent agents, local rules creating global behavior beats centralized dispatch. The theoretical promise is real.

## What Concerns Me

### 1. The feedback loop is loose, not tight

Original Boids works because birds **mechanically** follow rules at every timestep with deterministic physics. LLMs are stochastic text generators. A steering signal saying "avoid auth/types.go" becomes a system message that the model may or may not follow.

**This is the fundamental open question: can you get emergent coordination from agents that only probabilistically follow local rules?** Maybe. But it's unproven and the emergent behavior would be unreliable.

### 2. The Boids metaphor wraps conventional coordination primitives

| Boid Rule | What it really is | Simpler alternative |
|-----------|-------------------|---------------------|
| Separation | Don't work on what others are working on | Entity claiming / KV locks |
| Cohesion | Work on high-value things near you | Priority queue + PageRank |
| Alignment | Follow same patterns as peers | Shared conventions in system prompt |

These are **useful** mechanisms. But does the Boids framework (velocity, steering strength, k-hop thresholds, cooldowns) provide value over simpler implementations?

### 3. Value depends on scale that doesn't exist yet

Boids creates emergent behavior from **many** agents following local rules. With 2-3 agents, you get a few coordination signals — not emergence. The payoff comes at 10+ concurrent agents.

### 4. The A/B test as designed won't be conclusive

The proposed architect -> editor sequential handoff has minimal opportunity for Boids:

- Two sequential agents rarely conflict (editor starts after architect finishes)
- Without concurrent agents, separation has nothing to separate
- Without multiple same-role agents, alignment has nothing to align
- You need **3+ concurrent agents on overlapping work** to see meaningful differences

### 5. Token cost is unknown

Every steering signal consumed adds context to the model prompt. Is the coordination benefit worth the token cost? Can't know without measurement.

## Implementation Gaps Are Larger Than Acknowledged

The A/B test plan shows 2 missing items. The codebase has **7+ significant gaps**:

| Gap | Severity | Detail |
|-----|----------|--------|
| **PositionProvider not injected** | Critical | Rules log "No position provider" and return false — **they cannot fire** |
| **No NATS subscription for signals** | Critical | `setupSubscriptions()` doesn't route `agent.boid.*` — `HandleSteeringSignalMessage` is dead code |
| **Entity state format mismatch** | Critical | Rules expect triples (`boid.role`, etc), positions stored as flat JSON — `extractAgentPosition()` won't work |
| **Signal consumption stubbed** | High | `ProcessSteeringSignal()` only logs — zero effect on context |
| **PivotIndex not injected** | High | No k-hop distance — separation only matches exact entity IDs |
| **CentralityProvider not injected** | High | No PageRank — cohesion sorts by ID |
| **TraversalVector never populated** | Medium | Alignment rules have no data to align on |
| **RegionGraphEntities missing from GetContext()** | Medium | Entity region exists but isn't in output ordering |

The rules processor, boid rules, and agentic-loop handler are **structurally complete but not wired end-to-end**. An A/B test now would test infrastructure plumbing, not the Boids concept.

### Gap Details

**PositionProvider (`processor/rule/boid/factory.go`)**: The `Create()` method builds rules but never calls `SetPositionProvider()`. Without it, separation's `EvaluateEntityState` hits `"No position provider configured"` and returns false. Same for alignment.

**Entity state format (`processor/rule/boid/separation.go:extractAgentPosition`)**: Expects triples with predicates like `boid.role`, `boid.focus_entities`. But `BoidHandler.UpdatePosition()` writes flat `AgentPosition` JSON to KV. The rule processor's entity watcher unmarshals as `graph.EntityState` with triples — the formats don't match.

**Signal subscription (`processor/agentic-loop/component.go:setupSubscriptions`)**: Routes `agent.task`, `agent.response`, `tool.result`, `agent.signal`. No route for `agent.boid.*`. The `HandleSteeringSignalMessage` method exists but has zero callers.

## Honest Bottom Line

The Boids concept is **intellectually appealing and architecturally sound**. It's not "too good to be true" — it's a reasonable coordination mechanism on a real theoretical foundation.

But it's not a **killer feature** in its current form:

- At small scale (2-3 agents), simpler mechanisms achieve the same results
- At large scale (10+), LLM stochasticity means you can't guarantee the emergent coordination that makes Boids special
- The gap between concept and working system is larger than acknowledged

**It could become a killer feature if:**
1. You reach scale (5+ concurrent agents on overlapping work)
2. LLMs reliably follow steering signals (measurable compliance rate)
3. It demonstrably outperforms simpler coordination (entity locks + priority queues)

## Recommended Path Forward

Instead of the proposed A/B test:

### Phase 1: Close the wiring gaps (prerequisite for any test)

Fix the 7 critical/high gaps so the system works end-to-end.

| File | Change |
|------|--------|
| `processor/rule/boid/factory.go` | Inject PositionProvider, PivotIndex, CentralityProvider |
| `processor/rule/boid/` (watcher or types) | Resolve entity state format (triples vs flat JSON) |
| `processor/agentic-loop/component.go` | Add `agent.boid.*` subscription, route to BoidHandler |
| `processor/agentic-loop/boid_handler.go` | Implement actual context modification in ProcessSteeringSignal |
| `processor/agentic-loop/context_manager.go` | Include RegionGraphEntities in GetContext() |
| `processor/agentic-loop/component.go` | Populate TraversalVector from tool results |

### Phase 2: Separation only, end-to-end

Get one rule working fully before adding complexity. Separation is the most concrete and verifiable.

### Phase 3: Test with 3+ concurrent agents

Design a scenario with genuine concurrency and overlap:
- 3 developer agents simultaneously on overlapping file areas
- Measure: entity conflicts, total time, total tokens
- Compare: separation on vs off

### Phase 4: Measure the feedback loop

The fundamental experiment: **does the model actually follow steering signals?**
- Inject "avoid entity X" — does the model avoid it in tool calls?
- What % compliance? Is it reliable enough for emergent coordination?

### Phase 5: Compare against simple baselines

Not just "Boids off" vs "Boids on", but also:
- "Entity claiming via KV lock" (simple separation)
- "PageRank priority queue" (simple cohesion)
- "Shared conventions in system prompt" (simple alignment)

This tells you whether Boids adds value **over simpler alternatives**, not just over nothing.

## References

- [Craig Reynolds' Boids](https://www.red3d.com/cwr/boids/)
- [Boids SemSpec Integration Proposal](boids-semspec-integration.md)
- Implementation: `processor/rule/boid/`
- Loop integration: `processor/agentic-loop/boid_handler.go`
