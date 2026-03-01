# The KV Twofer

How SemStreams uses a single NATS KV bucket as both state store and event bus, and why predicates are event channels.

## The Core Insight

Most event-driven systems require two separate infrastructures: somewhere to store current state, and
somewhere to publish events about state changes. Keeping these in sync is a persistent source of bugs.

SemStreams doesn't have this problem. Every NATS KV bucket is backed by a JetStream stream — this is
not an implementation detail, it's the architecture. A single bucket gives you three interfaces
simultaneously, with no extra infrastructure and no dual-write discipline required.

```text
┌─────────────────────────────────────────────────────────────┐
│                    Any SemStreams KV Bucket                  │
│               (e.g., ENTITY_STATES, PREDICATE_INDEX)        │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  Interface A: State                                         │
│  ─────────────────                                          │
│  kv.Get("acme.ops.robotics.gcs.drone.001")                  │
│  → Current entity state, right now                          │
│                                                             │
│  kv.Keys("acme.ops.robotics.gcs.drone.*")                   │
│  → All drone entity IDs                                     │
│                                                             │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  Interface B: Events (Watch)                                │
│  ────────────────────────────                               │
│  kv.Watch("acme.ops.robotics.gcs.drone.*")                  │
│  → Fires on every change to any drone entity                │
│                                                             │
│  Each entry carries: key, value, revision, operation        │
│  Processor reacts to state changes — no separate event bus  │
│                                                             │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  Interface C: History (Replay)                              │
│  ─────────────────────────────                              │
│  Replay from any revision                                   │
│  → Reconstruct entity state at any point in time            │
│                                                             │
│  Configurable history depth per bucket                      │
│  → Full audit trail with zero extra infrastructure          │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

**The twofer:** Writing entity state publishes an event. Watching entity state subscribes to events.
Replaying revision history replays events. One bucket, three capabilities.

## What This Replaces

In conventional stream processing, you maintain a separate event log alongside a state store:

```text
Conventional approach:

  Producer ──publish──► Kafka Topic    (event log)
      │
      └──write───────► Database        (state store)

  Consumer ─subscribe─► Kafka Topic
                            and
           ──query────► Database

  Problem: Two writes per update. Two systems to keep consistent.
           Missed events = stale state. Missed writes = silent data loss.
```

SemStreams collapses this to one operation:

```text
SemStreams:

  Producer ──Put──► ENTITY_STATES KV
                         │
                         ├─── Get → current state (Interface A)
                         ├─── Watch → event notification (Interface B)
                         └─── History → replay (Interface C)

  One write. Three capabilities. Nothing to keep in sync.
```

## How It Flows Through SemStreams

Every graph processor in SemStreams uses this pattern:

```text
External data                           Internal processing
─────────────                           ──────────────────

  Sensor         graph.entity.upsert
  Adapter  ─────────────────────────► graph-ingest
                                            │
                                            │ kv.Put()
                                            ▼
                                      ENTITY_STATES
                                            │
                               ┌────────────┼────────────┐
                               │            │            │
                           kv.Watch     kv.Watch     kv.Watch
                               │            │            │
                               ▼            ▼            ▼
                          graph-index  graph-rules  graph-embedding
                               │
                           kv.Put()
                               ▼
                          PREDICATE_INDEX
                          OUTGOING_INDEX
                          INCOMING_INDEX
```

`graph-ingest` writes to `ENTITY_STATES`. Every downstream processor — indexing, rule evaluation,
embeddings, clustering — reacts to that write via KV watch. No event routing configuration. No message
schemas for internal state changes. The write IS the event.

## Three Watch Levels

Because entity IDs are 6-part hierarchical keys, the same watch mechanism supports three levels of
subscription specificity without any routing configuration:

```text
6-part entity ID:  org . platform . domain . system . type . instance
                   acme . ops     . robotics . gcs  . drone . 001
```

| Watch Pattern | Subscribes To | Example Use |
|--------------|--------------|-------------|
| Full key | One specific entity | Track a particular drone's state |
| Type wildcard (`...type.*`) | All entities of a type | React to any drone change |
| Subtree wildcard (`...system.>`) | All entities in a subsystem | React to anything in GCS |

```go
// One drone
watcher, _ := entityStates.Watch("acme.ops.robotics.gcs.drone.001")

// All drones on this platform
watcher, _ := entityStates.Watch("acme.ops.robotics.gcs.drone.*")

// Everything in the robotics subsystem
watcher, _ := entityStates.Watch("acme.ops.robotics.>")
```

No topic registry. No routing tables. The hierarchy falls out of the EntityID structure.

## Handling the Initial Values

When a watcher starts, it first delivers all current values matching the pattern, then transitions to
delivering live updates. The NATS client signals this transition with a `nil` entry.

This matters for processors: they must distinguish bootstrap from live to avoid treating every existing
entity as a "new" event on restart.

```go
watcher, _ := entityStates.Watch("acme.ops.robotics.gcs.drone.*", nats.Context(ctx))

bootstrapping := true

for entry := range watcher.Updates() {
    if entry == nil {
        // Transition point: all current values delivered, live updates follow
        bootstrapping = false
        p.logger.Info("bootstrap complete, processing live updates")
        continue
    }

    if bootstrapping {
        // Hydrate cache from current state — don't treat as new events
        p.cache[entry.Key()] = decode(entry.Value())
    } else {
        // Live update — diff against cache to detect what changed
        p.handleChange(entry)
    }
}
```

Bootstrap is a feature, not a workaround. A processor that restarts automatically recovers full
current state before processing new events. No separate recovery procedure required.

## Audit Trail at No Cost

KV buckets default to `History: 1` (latest value only). For entities where the history of state
transitions matters, increase this:

```go
js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
    Bucket:  "WORKFLOW_EXECUTIONS",
    History: 64,
    TTL:     7 * 24 * time.Hour,
})
```

With `History: 64`, the bucket stores the last 64 values per key. That is a complete event log for
that entity type — no append-only table, no separate event store, no Kafka retention policy to
manage. NATS provides it as a configuration parameter.

---

## Predicates as Event Channels

The twofer applies not just to `ENTITY_STATES` but to every derived index bucket. The `PREDICATE_INDEX`
is where this becomes particularly powerful: because predicate keys are typed in dotted notation, each
predicate is simultaneously a semantic fact category AND an event channel.

### How It Works

`PREDICATE_INDEX` maps predicate keys to the list of entity IDs that currently hold that predicate:

```text
PREDICATE_INDEX

Key: "drone.telemetry.battery"
Value: ["acme.ops.robotics.gcs.drone.001",
        "acme.ops.robotics.gcs.drone.002",
        "acme.ops.robotics.gcs.drone.007"]

Key: "mission.status.current"
Value: ["acme.ops.robotics.gcs.mission.alpha",
        "acme.ops.robotics.gcs.mission.bravo"]
```

Because this bucket is backed by JetStream, you can watch any key — which means you get notified
whenever the set of entities holding that predicate changes.

### Watching a Predicate

```go
// Get a handle to the PREDICATE_INDEX bucket
predicateKV, _ := js.KeyValue(ctx, "PREDICATE_INDEX")

// Watch all battery updates — fires when any entity's battery predicate changes
watcher, _ := predicateKV.Watch("drone.telemetry.battery")

// Watch all mission status changes — fires when any mission status predicate changes
watcher, _ := predicateKV.Watch("mission.status.current")

// Watch everything in the alert namespace
watcher, _ := predicateKV.Watch("alert.>")
```

### Predicates Replace Event Type Registries

In systems with explicit event definitions, you need an event type for every transition you want to
react to. Defining a new event type means adding a schema, a topic, and updating producers and
consumers.

In SemStreams, defining a new predicate automatically creates the event channel:

| Traditional Event Topic | Equivalent PREDICATE_INDEX watch |
|-------------------------|----------------------------------|
| `battery.critical.fired` | `drone.telemetry.battery` |
| `mission.status.changed` | `mission.status.current` |
| `sensor.connectivity.lost` | `sensor.connectivity.status` |
| `alert.rule.triggered` | `alert.active` |
| `task.state.transitioned` | `task.workflow.state` |

No event registration. No schema definition. No separate topic creation. Define the predicate in
your vocabulary, use it in triples, and any processor can watch for it immediately.

### Predicate Prefix Watching

Because predicates use dotted notation, prefix wildcards let you subscribe to families of related
predicates at once:

```go
// All drone telemetry — any predicate under drone.telemetry.*
watcher, _ := predicateKV.Watch("drone.telemetry.*")

// All alert-related predicates
watcher, _ := predicateKV.Watch("alert.>")

// Exact match — only this specific predicate
watcher, _ := predicateKV.Watch("mission.status.current")
```

This mirrors how NATS subject wildcards work generally — the predicate vocabulary and the watch
mechanism share the same structural logic, which is not coincidental. See
[Vocabulary](../basics/04-vocabulary.md) for predicate naming conventions.

### Example: Reacting to Predicate Changes

A rules-adjacent processor that wants to react whenever any entity enters a critical battery state:

```go
type BatteryMonitor struct {
    predicateKV nats.KeyValue
    entityKV    nats.KeyValue
    threshold   int
}

func (m *BatteryMonitor) Run(ctx context.Context) error {
    watcher, err := m.predicateKV.Watch("drone.telemetry.battery", nats.Context(ctx))
    if err != nil {
        return err
    }
    defer watcher.Stop()

    for entry := range watcher.Updates() {
        if entry == nil {
            continue // bootstrap complete
        }

        // entry.Value() is the updated list of entity IDs with this predicate
        entityIDs := decodeEntityList(entry.Value())

        for _, entityID := range entityIDs {
            // Fetch the actual entity to read the battery value
            entityEntry, _ := m.entityKV.Get(ctx, entityID)
            entity := decodeEntity(entityEntry.Value())

            battery := entity.GetTripleValue("drone.telemetry.battery")
            if battery < m.threshold {
                m.handleCritical(ctx, entityID, battery)
            }
        }
    }
    return ctx.Err()
}
```

Note that this fetches the entity to read the value after detecting a predicate change. The predicate
watch tells you *which entities changed*; you then read `ENTITY_STATES` to get *what they changed to*.
This two-step pattern is intentional — it keeps each bucket's responsibility clean.

---

## Both Patterns Together

Entity-level watching and predicate-level watching are complementary, not competing. The right choice
depends on what you need to react to:

| If you want to react to... | Use |
|---------------------------|-----|
| Any change to a specific entity | `ENTITY_STATES` watch on full key |
| Changes to all entities of a type | `ENTITY_STATES` watch with type wildcard |
| Changes to a specific fact type across all entities | `PREDICATE_INDEX` watch |
| Changes within a predicate namespace | `PREDICATE_INDEX` watch with wildcard |

A processor can hold watchers on both simultaneously, combining them in a select loop:

```go
for {
    select {
    case entry := <-entityWatcher.Updates():
        // React to entity-level changes
    case entry := <-predicateWatcher.Updates():
        // React to predicate-level changes
    case <-ctx.Done():
        return ctx.Err()
    }
}
```

---

## In Context: The Full Event Model

To be precise about where the twofer applies and where it doesn't:

```text
                          ┌──────────────────────────────┐
                          │  External boundary           │
                          │  (pub/sub subjects)          │
                          │                              │
  External systems  ─────►│  graph.entity.upsert         │
  Input processors        │  (JetStream, at-least-once,  │
  Standards adapters      │   dedup, backpressure)       │
                          └──────────────┬───────────────┘
                                         │
                                    graph-ingest
                                         │
                                         ▼
                          ┌──────────────────────────────┐
                          │  Internal graph              │
                          │  (KV twofer)                 │
                          │                              │
                          │  ENTITY_STATES               │
                          │  PREDICATE_INDEX             │◄── All processors
                          │  OUTGOING_INDEX              │    watch here
                          │  INCOMING_INDEX              │
                          │  [other derived indexes]     │
                          │                              │
                          └──────────────────────────────┘
```

The external boundary uses JetStream pub/sub subjects because they provide at-least-once delivery,
per-message deduplication, and durable consumer backpressure — properties important for the ingestion
edge, especially in DDIL environments. Once data is inside the graph, the KV twofer takes over and
no further pub/sub coordination is needed for internal processing.

---

## Related

**Concepts**
- [Streams vs KV Watches](03-streams-vs-kv-watches.md) — when to use the twofer vs JetStream
  streams, and why agentic components use both
- [Event-Driven Basics](01-event-driven-basics.md) — pub/sub, JetStream, and KV fundamentals
- [Knowledge Graphs](04-knowledge-graphs.md) — how triples create the predicates being watched
- [Real-Time Inference](00-real-time-inference.md) — how the twofer enables each inference tier

**Reference**
- [Index and Bucket Reference](../advanced/05-index-reference.md) — complete bucket inventory,
  key formats, and ownership table
- [Vocabulary](../basics/04-vocabulary.md) — predicate naming conventions that make prefix
  watching useful
- [Graph Components](../advanced/07-graph-components.md) — how each processor's `kv-watch` input
  port implements this pattern
