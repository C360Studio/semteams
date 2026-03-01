# Query Access

Why structured query access matters for knowledge graphs and when to use each pattern.

## The Access Problem

Knowledge graphs differ from traditional databases:

```text
Traditional Database:
┌─────────────────────────────────────────────────────────────┐
│                      Single Query Path                       │
│                                                              │
│   Application ────► SQL Engine ────► Tables ────► Response   │
│                                                              │
└─────────────────────────────────────────────────────────────┘
                          │
                    One interface
                    One protocol
```

Knowledge graphs serve diverse clients with different needs:

```text
SemStreams:
┌─────────────────────────────────────────────────────────────┐
│                    Multiple Access Paths                     │
│                                                              │
│   Web Apps ─────────┐                                        │
│                     │                                        │
│   AI Agents ────────┼──────► Graph ────► Knowledge           │
│                     │                                        │
│   Internal Services ┘                                        │
│                                                              │
└─────────────────────────────────────────────────────────────┘
                          │
                    Different needs:
                    - Latency
                    - Schema control
                    - Auditability
```

No single protocol serves all clients optimally. SemStreams provides three access patterns, each optimized for specific use cases.

## Three Access Patterns

### GraphQL: Schema-First Access

GraphQL provides schema-validated queries with field selection.

```text
┌─────────────┐        ┌─────────────┐        ┌─────────────┐
│   Client    │───────►│   Gateway   │───────►│   Graph     │
│             │        │             │        │             │
│  "Give me   │        │  Validates  │        │  Returns    │
│   these     │        │  against    │        │  requested  │
│   fields"   │        │  schema     │        │  fields     │
└─────────────┘        └─────────────┘        └─────────────┘
      │                      │                      │
   Field                 Schema               Bounded
   Selection            Enforcement           Response
```

**Key characteristics:**

- Clients request specific fields (no over-fetching)
- Schema defines available operations
- Introspection enables tooling and exploration
- Single HTTP endpoint for all operations
- Natural language query classification extracts search intents automatically

**Natural language support:** The GraphQL gateway includes a query classifier that extracts temporal, spatial, and intent information from natural language queries. For example, "What sensors were active yesterday?" automatically populates temporal filters and routes to the appropriate search strategy. See [ADR-004](../architecture/adr-004-search-query-classification.md) for details.

**Best for:** External applications, web frontends, interactive exploration, natural language queries.

### MCP: AI Agent Access

Model Context Protocol wraps GraphQL for AI assistants.

```text
┌─────────────┐        ┌─────────────┐        ┌─────────────┐
│  AI Agent   │───────►│ MCP Gateway │───────►│   GraphQL   │
│             │        │             │        │             │
│  Structured │        │  Validates  │        │  Executes   │
│  tool call  │        │  + audits   │        │  query      │
└─────────────┘        └─────────────┘        └─────────────┘
      │                      │                      │
   Bounded               Full                  Same
   Capabilities          Audit Trail           Semantics
```

**Why not give AI agents direct database access?**

| Concern | Direct Access | MCP Gateway |
|---------|--------------|-------------|
| Capabilities | Unbounded | Schema-defined |
| Auditability | Manual logging | Automatic, structured |
| Errors | Arbitrary failures | Structured responses |
| Token usage | Full responses | Field selection |

MCP trades flexibility for safety—appropriate for production AI deployments.

**Natural language handling:** AI agents typically issue natural language queries. The underlying GraphQL gateway's query classifier automatically extracts temporal, spatial, and intent information, routing queries to appropriate search strategies without requiring agents to construct structured parameters.

**Best for:** Claude, LLM agents, automated reasoning systems, conversational interfaces.

### NATS Direct: Service-to-Service

Internal services can bypass gateways entirely.

```text
┌─────────────┐                              ┌─────────────┐
│   Service   │─────────────────────────────►│  Component  │
│             │                              │             │
│  Request/   │      No gateway overhead     │  Owns data  │
│  Reply      │                              │             │
└─────────────┘                              └─────────────┘
      │                                            │
   Lowest                                     Direct
   Latency                                    Access
```

**When gateway overhead matters:**

- High-frequency internal queries
- Latency-sensitive operations
- Component-to-component communication
- Operations not exposed through GraphQL

**Best for:** Internal services, microservice communication, performance-critical paths.

## Choosing an Access Pattern

```text
                    ┌─────────────────────┐
                    │   Who is calling?   │
                    └──────────┬──────────┘
                               │
          ┌────────────────────┼────────────────────┐
          │                    │                    │
          ▼                    ▼                    ▼
    ┌──────────┐        ┌──────────┐        ┌──────────┐
    │ External │        │ AI Agent │        │ Internal │
    │   App    │        │          │        │ Service  │
    └────┬─────┘        └────┬─────┘        └────┬─────┘
         │                   │                   │
         ▼                   ▼                   ▼
    ┌──────────┐        ┌──────────┐        ┌──────────┐
    │ GraphQL  │        │   MCP    │        │  NATS    │
    │          │        │          │        │  Direct  │
    └──────────┘        └──────────┘        └──────────┘
```

### Decision Matrix

| Factor | GraphQL | MCP | NATS Direct |
|--------|---------|-----|-------------|
| **Latency** | Higher | Higher | Lowest |
| **Schema control** | Strong | Strong | Per-component |
| **Auditability** | Good | Excellent | Manual |
| **Field selection** | Yes | Yes | No |
| **External access** | Yes | Yes | No (internal) |
| **Tooling** | Rich | Growing | Minimal |
| **Discovery** | Introspection | Tool list | Capability queries |

### Common Patterns

**Web application backend:**
- GraphQL for user-facing queries
- NATS direct for background processing

**AI-powered system:**
- MCP for agent reasoning queries
- GraphQL for human-facing dashboard

**Microservice mesh:**
- NATS direct between services
- GraphQL for external API

## Trade-offs

### Schema Enforcement vs Flexibility

GraphQL and MCP enforce schema at the gateway. Invalid queries fail before reaching components. NATS direct relies on component-level validation—faster but less protective.

### Latency vs Features

```text
Feature richness ────────────────────────────────► Latency

   NATS Direct          GraphQL              MCP
       │                   │                  │
       │                   │                  │
   Minimal              Field              Full
   overhead            selection           audit
```

### Discovery Mechanisms

Each pattern has different discovery:

| Pattern | Discovery Method | Granularity |
|---------|-----------------|-------------|
| GraphQL | Schema introspection | All operations |
| MCP | Tool listing | Exposed tools |
| NATS Direct | Capability queries | Per-component |

GraphQL introspection provides the most complete picture. NATS capability queries require knowing which components exist.

## Consistency Considerations

All access patterns read from the same underlying graph:

```text
                    ┌─────────────────────┐
                    │                     │
   GraphQL ────────►│                     │
                    │    Knowledge        │
   MCP ────────────►│      Graph          │
                    │                     │
   NATS Direct ────►│                     │
                    │                     │
                    └─────────────────────┘
                              │
                         Eventually
                         Consistent
```

Regardless of access pattern:

- Entity updates: Milliseconds to queryable
- Index updates: Slightly behind entity state
- Community updates: Periodic (seconds)

The access pattern affects latency, not consistency guarantees.

## Related

**Concepts**
- [Knowledge Graphs](04-knowledge-graphs.md) - The data model being queried
- [GraphRAG Pattern](09-graphrag-pattern.md) - Community-based search operations
- [PathRAG Pattern](10-pathrag-pattern.md) - Graph traversal operations
- [Event-Driven Basics](01-event-driven-basics.md) - NATS fundamentals

**Architecture**
- [ADR-004: Search Query Classification](../architecture/adr-004-search-query-classification.md) - NL query classification design
