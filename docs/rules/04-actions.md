# Actions

Actions execute when rule conditions match. They can modify the graph, publish messages, or both.

## Action Structure

```go
type Action struct {
    Type      string  // Action type: add_triple, remove_triple, publish
    Subject   string  // NATS subject for publish actions
    Predicate string  // Relationship type for triple actions
    Object    string  // Target entity or value for triple actions
    TTL       string  // Optional expiration for triples
}
```

## Action Types

| Action | Effect | Graph Impact |
|--------|--------|--------------|
| `add_triple` | Creates relationship/property | Adds edge, affects clustering |
| `remove_triple` | Removes relationship | Removes edge, may split communities |
| `publish` | Sends NATS message | No direct graph impact |

## add_triple

Creates a triple on the entity:

```json
{
  "type": "add_triple",
  "predicate": "alert.status",
  "object": "battery_low"
}
```

**Result:** `entity.alert.status = "battery_low"`

### Creating Relationships

When `object` is an entity ID, it creates a graph edge:

```json
{
  "type": "add_triple",
  "predicate": "fleet.membership",
  "object": "acme.ops.fleet.warehouse-7"
}
```

**Result:**
- Triple: `drone-007.fleet.membership → acme.ops.fleet.warehouse-7`
- Edge indexed in OUTGOING_INDEX (drone-007) and INCOMING_INDEX (fleet)
- Community detection will traverse this edge

### Template Variables

Use entity data in object values:

```json
{
  "type": "add_triple",
  "predicate": "zone.membership",
  "object": "zone.${entity.zone}"
}
```

If entity has `entity.zone = "warehouse-7"`, result is:
`entity.zone.membership → zone.warehouse-7`

### TTL (Time to Live)

Create temporary triples that expire:

```json
{
  "type": "add_triple",
  "predicate": "alert.active",
  "object": "true",
  "ttl": "5m"
}
```

Triple will be automatically removed after 5 minutes.

TTL format: `"30s"`, `"5m"`, `"1h"`, `"24h"`

## remove_triple

Removes a triple from the entity:

```json
{
  "type": "remove_triple",
  "predicate": "alert.status"
}
```

**Result:** Any triple with predicate `alert.status` is removed from the entity.

### Removing Relationships

When removing relationship triples:

```json
{
  "type": "remove_triple",
  "predicate": "fleet.membership"
}
```

**Result:**
- Triple removed from entity
- Edge removed from OUTGOING_INDEX and INCOMING_INDEX
- Next community detection: entity may move to different community

## publish

Publishes a message to a NATS subject:

```json
{
  "type": "publish",
  "subject": "alerts.battery.low"
}
```

**Message payload includes:**
- Entity ID
- Rule ID
- Timestamp
- Entity triples snapshot

### Subject Templates

```json
{
  "type": "publish",
  "subject": "alerts.${entity.type}.critical"
}
```

## How Actions Shape Communities

### Creating Communities

```
Before rule:
  drone-007 (isolated)
  drone-008 (isolated)

Rule fires add_triple: fleet.membership → fleet.rescue

After rule:
  drone-007 ──fleet.membership──> fleet.rescue
  drone-008 ──fleet.membership──> fleet.rescue

Next community detection:
  Community: [drone-007, drone-008, fleet.rescue]
```

### Splitting Communities

```
Before rule exit:
  Community: [drone-007, drone-008, sensor-alpha]

drone-007 leaves zone (rule fires remove_triple)

After rule:
  drone-008 ──fleet.membership──> fleet.rescue
  sensor-alpha ──equipment.parent──> drone-008
  drone-007 (no fleet relationship)

Next community detection:
  Community A: [drone-008, sensor-alpha]
  Community B: [drone-007] (isolated)
```

### Merging Communities

```
Before rule:
  Community A: [drone-007, drone-008]
  Community B: [sensor-alpha, sensor-beta]

Rule fires: sensor-alpha.equipment.parent → drone-007

After rule:
  sensor-alpha ──equipment.parent──> drone-007

Next community detection:
  Community: [drone-007, drone-008, sensor-alpha, sensor-beta]
```

## OnEnter vs OnExit

| Trigger | When | Typical Actions |
|---------|------|-----------------|
| `on_enter` | Condition becomes true | add_triple, publish alert |
| `on_exit` | Condition becomes false | remove_triple, publish recovery |
| `while_true` | Condition remains true | publish monitoring data |

### Balanced Enter/Exit

Every `on_enter` that adds a triple should have a corresponding `on_exit`:

```json
{
  "on_enter": [
    {"type": "add_triple", "predicate": "alert.status", "object": "active"}
  ],
  "on_exit": [
    {"type": "remove_triple", "predicate": "alert.status"}
  ]
}
```

Without `on_exit`, relationships accumulate and never clean up.

## Best Practices

### 1. Use Predictable Object Values

```json
// Good: Deterministic
{"object": "fleet.${entity.zone}"}

// Risky: Creates many unique values
{"object": "alert.${timestamp}"}
```

### 2. Balance Enter/Exit

Always pair add_triple with remove_triple.

### 3. Avoid Cycles

Don't create rules that trigger each other:

```
Rule A: If X → add relationship to Y
Rule B: If Y → add relationship to X
```

This can create infinite loops.

### 4. Use Meaningful Predicates

```json
// Good: Domain-specific
{"predicate": "fleet.membership"}
{"predicate": "equipment.parent"}

// Bad: Generic
{"predicate": "relationship"}
{"predicate": "link"}
```

### 5. Consider TTL for Temporary States

```json
{
  "type": "add_triple",
  "predicate": "alert.suppressed",
  "object": "true",
  "ttl": "1h"
}
```

## Debugging Actions

Check if actions executed:

```bash
# View entity triples
nats kv get ENTITY_STATES "drone-007" | jq '.triples'

# Check relationship index
nats kv get OUTGOING_INDEX "drone-007"
nats kv get INCOMING_INDEX "fleet.rescue"

# View rule state
nats kv get RULE_STATE "battery-low.drone-007"
```

## Limitations

- Cannot query other entities in action templates
- Cannot conditionally execute actions (all or nothing)
- Cannot chain actions (action A triggers action B)
- Object templates only support simple field references

## Next Steps

- [State Tracking](05-state-tracking.md) - When actions fire
- [Entity Watching](06-entity-watching.md) - How entities are monitored
- [Examples](10-examples.md) - Complete working rules
