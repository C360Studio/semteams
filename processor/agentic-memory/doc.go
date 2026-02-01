// Package agenticmemory provides a graph-backed agent memory processor component
// that manages context hydration, fact extraction, and memory checkpointing for
// agentic loops.
//
// # Overview
//
// The agentic-memory processor bridges the agentic loop system with the knowledge
// graph, providing persistent memory capabilities for agents. It responds to context
// events from agentic-loop, extracts facts for long-term storage, and hydrates
// context when needed.
//
// Key capabilities:
//
//   - Context hydration from knowledge graph
//   - LLM-assisted fact extraction
//   - Memory checkpointing for recovery
//   - Integration with agentic-loop context events
//
// # Architecture
//
// The memory processor sits between the agentic loop and the knowledge graph:
//
//	┌────────────────┐     ┌─────────────────┐     ┌──────────────┐
//	│  agentic-loop  │────▶│ agentic-memory  │────▶│   Graph      │
//	│                │     │   (this pkg)    │     │  Processor   │
//	│                │◀────│                 │     │              │
//	└────────────────┘     └─────────────────┘     └──────────────┘
//	  context.compaction.*   graph.mutation.*
//	  hydrate.request.*      context.injected.*
//
// # Message Flow
//
// The processor handles two main flows:
//
// **Compaction Flow** (from agentic-loop):
//
//  1. agentic-loop publishes compaction_starting event
//  2. agentic-memory extracts facts from context using LLM
//  3. Facts published to graph.mutation.* for storage
//  4. agentic-loop publishes compaction_complete event
//  5. agentic-memory queries graph for relevant context
//  6. Hydrated context published to agent.context.injected.*
//
// **Hydration Request Flow** (explicit):
//
//  1. External system publishes hydrate request to memory.hydrate.request.*
//  2. agentic-memory queries graph based on request type
//  3. Hydrated context published to agent.context.injected.*
//
// # Key Types
//
// **Hydrator** - Retrieves context from knowledge graph:
//
//	hydrator, err := NewHydrator(config.Hydration, graphClient)
//
//	// Pre-task hydration (before loop starts)
//	context, err := hydrator.HydratePreTask(ctx, loopID, taskDescription)
//
//	// Post-compaction hydration (after context compressed)
//	context, err := hydrator.HydratePostCompaction(ctx, loopID)
//
// **LLMExtractor** - Extracts facts using language models:
//
//	extractor, err := NewLLMExtractor(config.Extraction, llmClient)
//
//	// Extract facts from conversation content
//	triples, err := extractor.ExtractFacts(ctx, loopID, content)
//
// **Publisher** - Publishes results to NATS:
//
//	// Published to agent.context.injected.{loopID}
//	err := c.publishInjectedContext(ctx, loopID, "post_compaction", hydrated)
//
//	// Published to graph.mutation.{loopID}
//	err := c.publishGraphMutations(ctx, loopID, "add_triples", triples)
//
// # Configuration
//
// The processor is configured via JSON:
//
//	{
//	    "extraction": {
//	        "llm_assisted": {
//	            "enabled": true,
//	            "model": "fast",
//	            "trigger_iteration_interval": 5,
//	            "trigger_context_threshold": 0.8,
//	            "max_tokens": 1000
//	        }
//	    },
//	    "hydration": {
//	        "pre_task": {
//	            "enabled": true,
//	            "max_context_tokens": 2000,
//	            "include_decisions": true,
//	            "include_files": true
//	        },
//	        "post_compaction": {
//	            "enabled": true,
//	            "reconstruct_from_checkpoint": true,
//	            "max_recovery_tokens": 1500
//	        }
//	    },
//	    "checkpoint": {
//	        "enabled": true,
//	        "storage_bucket": "AGENT_MEMORY_CHECKPOINTS",
//	        "retention_days": 7
//	    },
//	    "stream_name": "AGENT"
//	}
//
// Configuration fields:
//
//   - extraction.llm_assisted.enabled: Enable LLM-assisted fact extraction
//   - extraction.llm_assisted.model: Model alias for extraction (from agentic-model)
//   - extraction.llm_assisted.trigger_iteration_interval: Extract every N iterations
//   - extraction.llm_assisted.trigger_context_threshold: Extract at N% utilization
//   - extraction.llm_assisted.max_tokens: Max tokens for extraction request
//   - hydration.pre_task.enabled: Enable pre-task context hydration
//   - hydration.pre_task.max_context_tokens: Max tokens for pre-task context
//   - hydration.pre_task.include_decisions: Include past decisions
//   - hydration.pre_task.include_files: Include file context
//   - hydration.post_compaction.enabled: Enable post-compaction reconstruction
//   - hydration.post_compaction.reconstruct_from_checkpoint: Use checkpoints
//   - hydration.post_compaction.max_recovery_tokens: Max tokens for recovery
//   - checkpoint.enabled: Enable memory checkpointing
//   - checkpoint.storage_bucket: KV bucket for checkpoints
//   - checkpoint.retention_days: Days to retain checkpoints
//   - stream_name: JetStream stream name (default: "AGENT")
//
// # Ports
//
// Input ports (JetStream consumers):
//
//   - compaction_events: Context compaction events from agentic-loop
//     (subject: agent.context.compaction.>)
//   - hydrate_requests: Explicit hydration requests
//     (subject: memory.hydrate.request.*)
//   - entity_states: Entity state changes for reactive hydration
//     (type: kv-watch, bucket: ENTITY_STATES)
//
// Output ports (publishers):
//
//   - injected_context: Hydrated context for agentic-loop
//     (subject: agent.context.injected.*)
//   - graph_mutations: Fact triples for graph processor
//     (subject: graph.mutation.*)
//   - checkpoint_events: Checkpoint creation notifications
//     (subject: memory.checkpoint.created.*)
//
// # Context Events
//
// The processor consumes context events from agentic-loop:
//
//	// Compaction starting - extract facts before content is lost
//	{
//	    "type": "compaction_starting",
//	    "loop_id": "loop_123",
//	    "iteration": 5,
//	    "utilization": 0.65
//	}
//
//	// Compaction complete - hydrate recovered context
//	{
//	    "type": "compaction_complete",
//	    "loop_id": "loop_123",
//	    "iteration": 5,
//	    "tokens_saved": 2500,
//	    "summary": "Discussed authentication..."
//	}
//
//	// GC complete - logged for observability
//	{
//	    "type": "gc_complete",
//	    "loop_id": "loop_123",
//	    "iteration": 6
//	}
//
// # Hydration Requests
//
// Explicit hydration can be requested:
//
//	// Pre-task hydration
//	{
//	    "loop_id": "loop_123",
//	    "type": "pre_task",
//	    "task_description": "Implement user authentication"
//	}
//
//	// Post-compaction hydration
//	{
//	    "loop_id": "loop_123",
//	    "type": "post_compaction"
//	}
//
// # Memory Lifecycle
//
// A typical memory lifecycle for an agentic loop:
//
//  1. Loop starts, pre-task hydration injects relevant context
//  2. Loop executes, context grows with each iteration
//  3. Context approaches limit, compaction_starting published
//  4. agentic-memory extracts key facts before compaction
//  5. agentic-loop compresses context, publishes compaction_complete
//  6. agentic-memory hydrates recovered context from graph
//  7. Process repeats as needed until loop completes
//  8. Final checkpoint created for future reference
//
// # Quick Start
//
// Create and start the component:
//
//	config := agenticmemory.DefaultConfig()
//
//	rawConfig, _ := json.Marshal(config)
//	comp, err := agenticmemory.NewComponent(rawConfig, deps)
//
//	lc := comp.(component.LifecycleComponent)
//	lc.Initialize()
//	lc.Start(ctx)
//	defer lc.Stop(5 * time.Second)
//
// # Thread Safety
//
// The Component is safe for concurrent use after Start() is called:
//
//   - Event handlers run in separate goroutines
//   - Internal state protected by RWMutex
//   - Atomic counters for metrics
//
// # Error Handling
//
// Errors are categorized:
//
//   - Graph query errors: Logged, hydration returns empty context
//   - LLM extraction errors: Logged, facts not extracted
//   - Publish errors: Logged, counted as errors in metrics
//   - Invalid events: Logged with error counter increment
//
// Errors don't fail the agentic loop - memory is supplementary.
//
// # Integration with agentic-loop
//
// agentic-memory integrates with agentic-loop through context events:
//
//  1. agentic-loop publishes to agent.context.compaction.*
//  2. agentic-memory consumes these events
//  3. agentic-memory publishes to agent.context.injected.*
//  4. agentic-loop can consume injected context for enhancement
//
// # Testing
//
// For testing, use the ConsumerNameSuffix config option:
//
//	config := agenticmemory.Config{
//	    ConsumerNameSuffix: "test-" + t.Name(),
//	    // ...
//	}
//
// # Limitations
//
// Current limitations:
//
//   - Hydration quality depends on graph content
//   - LLM extraction has cost implications
//   - No streaming support for large contexts
//   - Checkpoint size limited by KV bucket limits
//
// # See Also
//
// Related packages:
//
//   - processor/agentic-loop: Loop orchestration (publishes context events)
//   - processor/agentic-model: LLM endpoint integration (for extraction)
//   - graph: Knowledge graph operations
//   - agentic: Shared types
package agenticmemory
