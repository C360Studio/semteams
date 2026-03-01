# SemStreams Documentation

## Start Here

**New to SemStreams?** Start with the basics:

1. [What is SemStreams?](basics/01-what-is-semstreams.md) - Overview and capabilities
2. [Architecture](basics/02-architecture.md) - System components and data flow
3. [Graphable Interface](basics/03-graphable-interface.md) - Core interface for graph entities

## Documentation Structure

| Folder | Purpose | Audience |
|--------|---------|----------|
| [basics/](basics/) | Getting started, core interfaces, first processor | New users |
| [concepts/](concepts/) | Background knowledge, algorithms, theory | Learning fundamentals |
| [advanced/](advanced/) | Clustering, LLM, performance, GraphQL, rules engine | Production tuning |
| [operations/](operations/) | Local monitoring, deployment, observability | Operators |
| [contributing/](contributing/) | Development, testing, CI | Contributors |

## Learning Paths

**Go/streaming developers** (familiar with NATS, event-driven):
1. [Basics](basics/) - Core interfaces and processors
2. [Embeddings](concepts/03-embeddings.md) - Vectors and semantic similarity
3. [GraphRAG Pattern](concepts/07-graphrag-pattern.md) - Community-based retrieval

**ML/LLM developers** (familiar with models and vectors):
1. [Event-Driven Basics](concepts/01-event-driven-basics.md) - Pub/sub, streams
2. [Knowledge Graphs](concepts/02-knowledge-graphs.md) - Triples, SPO model
3. [Basics](basics/) - Implementation patterns

**Production operators**:
1. [Local Monitoring](operations/01-local-monitoring.md) - Prometheus + Grafana setup
2. [Configuration](basics/06-configuration.md) - Capability tiers
3. [Clustering](advanced/01-clustering.md) - LPA tuning
4. [Performance](advanced/03-performance.md) - Optimization strategies

## Quick Reference

- [Index Reference](advanced/05-index-reference.md) - The seven indexes
- [Rules Engine](advanced/06-rules-engine.md) - Condition-based actions
- [Community Detection](concepts/05-community-detection.md) - LPA algorithm
- [Query Access](concepts/09-query-access.md) - GraphQL and MCP gateway

## Vocabulary & Standards

- [Vocabulary Guide](basics/04-vocabulary.md) - Predicate design, registration, alias resolution
- [Agentic Vocabulary](vocabulary/agentic.md) - W3C S-Agent-Comm predicates for agent interoperability
- [RDF Export](../vocabulary/export/doc.go) - Serialize triples to Turtle, N-Triples, JSON-LD
- [Vocabulary Package](../vocabulary/README.md) - Full API reference, IRI mappings, ontology subpackages

## Agentic Systems

Build LLM-powered autonomous agents with tool use:

- [Agentic Quickstart](basics/07-agentic-quickstart.md) - Get started with agents
- [Agentic Systems](concepts/11-agentic-systems.md) - Concepts: loops, state machine, tools, trajectories
- [Agentic Components](advanced/08-agentic-components.md) - Reference: loop, model, and tools processors
- [Payload Registry](concepts/13-payload-registry.md) - Polymorphic JSON serialization pattern
- [JetStream Tuning](advanced/11-jetstream-tuning.md) - AckWait, backpressure, and heartbeats for agentic consumers

## Workflow Orchestration

Multi-step processes with loops, timeouts, and retries:

- [Workflow Quickstart](basics/08-workflow-quickstart.md) - Get started with workflows
- [Workflow Configuration](advanced/09-workflow-configuration.md) - Complete schema reference
- [Orchestration Layers](concepts/12-orchestration-layers.md) - When to use rules vs. workflows

## Operations

- [Local Monitoring](operations/01-local-monitoring.md) - Prometheus + Grafana setup
- [Troubleshooting](operations/02-troubleshooting.md) - Common issues and solutions

## External Integrations

Optional bridges for connecting SemStreams to external systems:

| Integration | Purpose | Documentation |
|-------------|---------|---------------|
| **AGNTCY** | Agent discovery, A2A protocol, OTEL export | [Concepts Guide](concepts/20-agntcy-integration.md) |
| **TrustGraph** | Document knowledge import/export, GraphRAG queries | [Integration Guide](integration/trustgraph-integration.md) |

These integrations are optional components that can be enabled based on deployment needs. See individual guides for configuration and deployment patterns.
