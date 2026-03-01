# Event-Driven Basics

For ML/LLM developers unfamiliar with streaming architectures, this explains the event-driven patterns SemStreams uses.

## Stream-First, Batch When Needed

SemStreams is a hybrid system: events flow continuously, but some operations run periodically.

**Streaming (continuous):**
- Entity updates arrive and persist immediately
- Index updates propagate in milliseconds
- Rules evaluate on each entity change

**Batch (periodic):**
- Community detection runs on intervals (e.g., every 30s)
- LLM summarization queues and processes asynchronously
- Write buffering flushes in micro-batches (e.g., 50ms)

If you're coming from ML, you know batch processing well. SemStreams adds streaming for real-time updates while keeping batch patterns where they make sense.

### Streaming vs Batch Mental Models

```text
Batch (ML Training):
┌─────────┐    ┌─────────┐    ┌─────────┐
│ Dataset │───►│ Process │───►│ Model   │
│ (all)   │    │ (once)  │    │ (done)  │
└─────────┘    └─────────┘    └─────────┘

Stream (Event-Driven):
┌─────────┐    ┌─────────┐    ┌─────────┐
│ Event 1 │───►│         │───►│ Updated │
│ Event 2 │───►│ Process │───►│ State   │
│ Event 3 │───►│ (cont.) │───►│ (live)  │
│   ...   │    └─────────┘    └─────────┘
```

**Key difference**: In streaming, data arrives continuously; processing never "finishes."

## Why Events?

| Concern | Batch | Events |
|---------|-------|--------|
| Latency | Minutes to hours | Milliseconds to seconds |
| Freshness | Stale until next run | Always current |
| Coupling | Tight (knows source) | Loose (just topics) |
| Replay | Re-run entire job | Replay specific events |
| Scale | Vertical (bigger machine) | Horizontal (more consumers) |

SemStreams processes IoT sensors, log streams, and real-time updates where batch windows are too slow.

**When batch makes sense:** Aggregations, ML training, expensive computations that benefit from batching (like LLM calls). SemStreams uses batch for community detection and summarization while keeping entity updates streaming.

## Publish-Subscribe (Pub/Sub)

The core pattern connecting components:

```text
Publisher                    Subscribers
┌─────────┐                  ┌─────────┐
│ Sensor  │──publish──┐ ┌───►│ Graph   │
│ Reader  │           │ │    │Processor│
└─────────┘           │ │    └─────────┘
              ┌───────▼─┴─────┐
              │    Subject    │
              │ "sensors.temp"│
              └───────┬───────┘
                      │      ┌─────────┐
                      └─────►│ Alert   │
                             │ Service │
                             └─────────┘
```

**Publishers** send messages to **subjects** (topics). **Subscribers** receive messages from subjects they're interested in.

### Decoupling

Publishers don't know who subscribes. Subscribers don't know who publishes. They only share the subject name.

**Benefit**: Add new consumers without changing producers. Remove components without breaking others.

### Subject Patterns

Subjects are hierarchical, supporting wildcards:

```text
sensors.temperature.zone-a     # Specific
sensors.temperature.*          # All temperature sensors
sensors.>                      # All sensor messages
```

## NATS: The Message Broker

SemStreams uses NATS as its message backbone. Think of it as the nervous system connecting components.

### Core NATS (Pub/Sub)

Fire-and-forget messaging. Publishers send to a subject; subscribers receive from subjects they registered interest in.

**Characteristics:**
- At-most-once delivery (no retries)
- No persistence (miss it, it's gone)
- Extremely fast (millions of msgs/sec)

### JetStream (Persistence)

Adds durability on top of Core NATS. Messages persist to streams; consumers acknowledge receipt.

**What JetStream adds:**
- At-least-once delivery (consumers ack messages)
- Persistent storage (survives restarts)
- Replay from any point (new consumers catch up)
- Consumer groups for horizontal scaling

### KV Buckets (State)

Key-value storage built on JetStream. Store, retrieve, and *watch* for changes reactively.

```text
┌─────────┐     ┌──────────┐     ┌──────────┐
│ Writer  │────►│ KV Bucket│────►│ Watcher  │
│         │ put │          │watch│ (reacts) │
└─────────┘     └──────────┘     └──────────┘
```

**Key capability:** Watchers receive updates when keys change, enabling reactive architectures without polling.

SemStreams stores all entity state in KV buckets.

## SemStreams Data Flow

```text
                              ┌───────────┐
                         ┌───►│  Output   │
┌─────────┐              │    │ Component │
│  Input  │──┬───────────┤    └───────────┘
│Component│  │           │
└─────────┘  │    ┌──────▼──────┐
             │    │  Processor  │───► KV Buckets
             │    │  Component  │    (state & indexes)
             │    └─────────────┘
             │           ▲
             │           │ (storage ref)
             │    ┌──────┴──────┐
             └───►│   Storage   │───► ObjectStore
                  │  (optional) │    (large data)
                  └─────────────┘
```

All components connect via NATS subjects (not shown for clarity).

### Input Components

Ingest data from external sources:

- **UDP**: Network sensors, syslog
- **File**: Log files, CSV imports
- **WebSocket**: Real-time feeds
- **HTTP**: Webhooks, APIs

### Processor Components

Transform data into knowledge graph entities. Your domain types implement the `Graphable` interface, defining how raw data becomes entities with triples.

See [Implementing Graphable](../basics/03-graphable-interface.md) for code examples.

### State (KV Buckets)

The Graph processors manage entity state in NATS KV buckets:

| Bucket | Purpose |
|--------|---------|
| `ENTITY_STATES` | Entity records with triples |
| `OUTGOING_INDEX` | Forward relationship lookup |
| `INCOMING_INDEX` | Reverse relationship lookup |
| `ALIAS_INDEX` | Entity alias resolution |
| `PREDICATE_INDEX` | Predicate → entity lookup |

Additional indexes (spatial, temporal, embedding, community, structural, anomaly) are created by their respective processors at higher tiers.

See [Index and Bucket Reference](../advanced/05-index-reference.md#kv-bucket-reference) for complete bucket documentation.

KV buckets are **not** separate components—they're internal state managed by the graph processors.

### Storage Components (Optional)

For large data (documents, images, binary), optional Storage components handle persistence:

1. Receive data from Input component
2. Store content to ObjectStore (or other backend)
3. Create a storage reference
4. Pass the reference forward to the Processor
5. Processor creates an entity pointing to the stored object

Use Storage components when:
- Payloads exceed KV bucket limits (~1MB)
- You need to store raw documents alongside graph entities
- Large binary data shouldn't flow through the message bus

### Output Components

Export or expose data:

- **File**: Logs, exports
- **WebSocket**: Real-time dashboards
- **GraphQL**: Query API
- **HTTP**: Webhooks, notifications

### HTTP Request/Reply Gateway

For services that need HTTP access to NATS, SemStreams provides an HTTP Request/Reply gateway that maps HTTP routes to NATS subjects. This enables REST-style access to any NATS service.

See [Configuration Guide](../basics/06-configuration.md) for HTTP gateway setup.

> **Note**: For knowledge graph queries, use the [GraphQL gateway](11-query-access.md) instead. The HTTP Request/Reply gateway is for generic NATS service integration, not graph queries.

## Eventual Consistency

In event-driven systems, not everything updates simultaneously.

```text
Time ──────────────────────────────────────►

Entity saved:     ✓
Predicate index:        ✓  (milliseconds later)
Community:                      ✓  (seconds later)
LLM summary:                              ✓  (async)
```

**What this means:**
- Query entity by ID: **Immediately consistent**
- Query by predicate: **Eventually consistent** (brief lag)
- Query community membership: **Batch consistent** (seconds to minutes)

For most applications, this is transparent. For latency-sensitive queries, understand the consistency tier you're querying.

## Consumers and Delivery

### Consumer Groups

Multiple instances share work:

```text
                    ┌─────────────┐
                    │  Consumer 1 │◄── msg 1, 4, 7
       ┌───────────►│  (replica)  │
       │            └─────────────┘
┌──────┴──────┐     ┌─────────────┐
│   Stream    │────►│  Consumer 2 │◄── msg 2, 5, 8
│ (messages)  │     │  (replica)  │
└──────┬──────┘     └─────────────┘
       │            ┌─────────────┐
       └───────────►│  Consumer 3 │◄── msg 3, 6, 9
                    │  (replica)  │
                    └─────────────┘
```

Messages are distributed; each is processed once across the group.

### Delivery Guarantees

| Mode | Guarantee | Use Case |
|------|-----------|----------|
| Core NATS | At-most-once | Fire-and-forget, metrics |
| JetStream | At-least-once | Critical events |
| JetStream + dedup | Exactly-once | Financial, audit |

SemStreams defaults to at-least-once with idempotent entity updates.

## Backpressure

When consumers can't keep up, the system needs a strategy:

```text
Fast Producer ──────► Buffer ──────► Slow Consumer
                        │
                   What happens
                   when full?
```

**Strategies:**
- **Drop oldest**: Lose stale data (metrics)
- **Drop newest**: Reject new data (backpressure to source)
- **Block**: Producer waits (synchronous)
- **Buffer to disk**: Persist overflow (JetStream)

SemStreams uses JetStream persistence—overflow goes to disk, not memory.

## Component Wiring

Components connect through subjects, not direct calls:

```text
┌─────────┐  subject   ┌───────────┐  subject   ┌─────────┐
│  Input  │───────────►│ Processor │───────────►│ Output  │
│Component│ "raw.data" │ Component │ "entities" │Component│
└─────────┘            └───────────┘            └─────────┘
```

**Key points:**
- Components only share subject names (loose coupling)
- Add/remove components without code changes
- Same component type can have multiple instances

See [Configuration](../basics/06-configuration.md) for setup details.

## Key Differences from Batch ML

| ML Pattern | Streaming Equivalent |
|------------|---------------------|
| Load dataset | Subscribe to stream |
| Transform all | Transform each event |
| Save model | Update state incrementally |
| Batch inference | Real-time inference |
| Retrain periodically | Continuous learning |

## Related

**Basics (how to implement):**
- [Architecture](../basics/02-architecture.md) - Full system overview
- [Implementing Graphable](../basics/03-graphable-interface.md) - Writing domain processors
- [Configuration](../basics/06-configuration.md) - Component setup

**Reference (API details):**
- [Index and Bucket Reference](../advanced/05-index-reference.md) - Storage and index schema

**Concepts (mental models):**
- [Real-Time Inference](00-real-time-inference.md) - Tiers and inference modes
- [Knowledge Graphs](04-knowledge-graphs.md) - Entity and triple fundamentals
