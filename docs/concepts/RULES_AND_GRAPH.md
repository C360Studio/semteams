# Rules and Graph: How Rules Build Communities

Rules are not just for alerts. They can BUILD the graph by adding and removing triples, which directly affects community detection.

## Rule Actions

| Action | Effect | Graph Impact |
|--------|--------|--------------|
| `add_triple` | Creates relationship/property | Adds edge for LPA to traverse |
| `remove_triple` | Removes relationship | Removes edge, may split communities |
| `update_triple` | Modifies property value | May change edge weight |
| `publish` | Sends event | No direct graph impact |

## How Rules Shape Communities

```
1. Rule condition evaluates to true (OnEnter)
2. add_triple creates: sensor.fleet_membership → fleet-123
3. Triple stored in ENTITY_STATES
4. IndexManager updates OUTGOING_INDEX
5. Next LPA detection: GetNeighbors() returns fleet-123
6. Sensor now clusters with fleet-123's community
```

## Example: Fleet Assignment

This rule automatically groups drones by zone:

```json
{
  "name": "assign_fleet_by_zone",
  "condition": "entity.type == 'drone' && entity.zone != ''",
  "on_enter": [
    {
      "type": "add_triple",
      "predicate": "fleet.membership",
      "object": "fleet.${entity.zone}"
    }
  ],
  "on_exit": [
    {
      "type": "remove_triple",
      "predicate": "fleet.membership"
    }
  ]
}
```

**What happens:**

1. Drone enters zone "warehouse-7"
2. Rule fires `add_triple`: `drone-007.fleet.membership → fleet.warehouse-7`
3. Next community detection: drone-007 clusters with other warehouse-7 equipment
4. Drone leaves zone (or zone field clears)
5. Rule fires `remove_triple`: relationship removed
6. Next community detection: drone-007 no longer clusters with warehouse-7

## Example: Equipment Hierarchy

Create parent-child relationships for hierarchical communities:

```json
{
  "name": "sensor_to_equipment",
  "condition": "entity.type == 'sensor' && entity.equipment_id != ''",
  "on_enter": [
    {
      "type": "add_triple",
      "predicate": "equipment.attachment",
      "object": "${entity.equipment_id}"
    }
  ]
}
```

Now sensors cluster with their parent equipment, and equipment clusters with other related equipment.

## Example: Anomaly-Based Grouping

Group entities that share anomaly patterns:

```json
{
  "name": "anomaly_cluster",
  "condition": "entity.status == 'anomaly' && entity.anomaly_signature != ''",
  "on_enter": [
    {
      "type": "add_triple",
      "predicate": "anomaly.signature_group",
      "object": "anomaly.${entity.anomaly_signature}"
    }
  ],
  "on_exit": [
    {
      "type": "remove_triple",
      "predicate": "anomaly.signature_group"
    }
  ]
}
```

Entities with the same anomaly signature will cluster together, making it easy to identify correlated failures.

## OnEnter vs OnExit

| Trigger | When | Use For |
|---------|------|---------|
| `on_enter` | Condition becomes true | Creating relationships |
| `on_exit` | Condition becomes false | Removing relationships |

Both are essential for dynamic community membership. Without `on_exit`, entities accumulate stale relationships.

## Rule vs Triple Design

**Use rules when:**
- Relationship depends on entity state (conditional)
- Relationship is temporary (comes and goes)
- You want to enforce business logic

**Use static triples when:**
- Relationship is inherent to the data
- Relationship comes from the source system
- No business logic needed

## Impact on Community Detection

### Creating Communities

```
Before rules:
  drone-007 (isolated)
  drone-008 (isolated)
  sensor-alpha (isolated)

After fleet.membership rule:
  Community: [drone-007, drone-008, sensor-alpha, fleet.warehouse-7]
```

### Splitting Communities

```
Before removal:
  Community: [drone-007, drone-008, sensor-alpha]

After drone-007 leaves zone (remove_triple):
  Community A: [drone-008, sensor-alpha]
  Community B: [drone-007] (isolated)
```

### Merging Communities

```
Before rule:
  Community A: [drone-007, drone-008]
  Community B: [sensor-alpha, sensor-beta]

After sensor-alpha.equipment.parent → drone-007:
  Community: [drone-007, drone-008, sensor-alpha, sensor-beta]
```

## Rule Evaluation and Timing

Rules are evaluated when entities change:

1. New message arrives
2. Entity updated in ENTITY_STATES
3. Rules engine evaluates conditions
4. Actions execute (add/remove triples)
5. Indexes updated
6. (Later) Community detection runs

**Important:** Community detection doesn't run immediately. It's triggered by:
- Entity change threshold exceeded
- Scheduled interval
- Manual trigger

## Best Practices

### 1. Use Predictable Object Values

```json
// Good: Predictable, deterministic
"object": "fleet.${entity.zone}"

// Risky: May create many unique values
"object": "alert.${timestamp}"
```

### 2. Balance Enter/Exit

Every `on_enter` that adds a triple should have a corresponding `on_exit` that removes it. Otherwise, relationships accumulate.

### 3. Avoid Cycles

Be careful with rules that reference each other's outputs:

```
Rule A: If X → add relationship to Y
Rule B: If Y → add relationship to X
```

This can create infinite loops or unexpected clustering.

### 4. Test With Small Data

Rules can have surprising effects on community structure. Test with a small dataset before production.

## Debugging Rules

Check if rules are firing:

```bash
# View rule state
nats kv get RULE_STATE "rule.assign_fleet_by_zone.drone-007"

# Check entity triples
nats kv get ENTITY_STATES "drone-007" | jq '.triples'

# Verify index updated
nats kv get OUTGOING_INDEX "drone-007"
```

## Known Gaps

**Currently not supported:**
- Rules cannot query other entities (only current entity state)
- Rules cannot access community membership (chicken-egg problem)
- No bulk rule operations (each entity evaluated individually)

## Next Steps

- [Index Usage](INDEX_USAGE.md) - How triples become edges
- [What is a Community?](WHAT_IS_A_COMMUNITY.md) - Community detection explained
- [processor/rule/README.md](../../processor/rule/README.md) - Complete rules reference
