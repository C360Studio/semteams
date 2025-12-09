# Rules Engine Overview

The rules engine evaluates conditions against entities and executes actions when conditions match. Rules can add or remove triples, publish messages, and build dynamic relationships that affect community detection.

## What Rules Do

Rules are stateful evaluators that:
- Watch entity state changes in NATS KV
- Evaluate conditions against entity triples
- Execute actions on state transitions (enter/exit/while)
- Create and remove relationships dynamically

## Quick Start Example

A simple battery alert rule:

```json
{
  "id": "battery-low",
  "name": "Battery Low Alert",
  "enabled": true,
  "conditions": [
    {"field": "drone.telemetry.battery", "operator": "lt", "value": 20}
  ],
  "on_enter": [
    {"type": "add_triple", "predicate": "alert.status", "object": "battery_low"},
    {"type": "publish", "subject": "alerts.battery"}
  ],
  "on_exit": [
    {"type": "remove_triple", "predicate": "alert.status"}
  ]
}
```

**What happens:**

1. Entity `drone-007` updates with `drone.telemetry.battery: 15`
2. Condition evaluates: `15 < 20` = true
3. State transition: none → entered
4. Actions execute:
   - Triple added: `drone-007.alert.status = battery_low`
   - Message published to `alerts.battery`
5. Later, battery becomes 25
6. Condition evaluates: `25 < 20` = false
7. State transition: entered → exited
8. Exit action: `alert.status` triple removed

## Two Evaluation Modes

### Message-Based Evaluation

Rules evaluate incoming messages from NATS subjects:

```go
type Rule interface {
    Subscribe() []string                           // NATS subjects to listen to
    Evaluate(messages []message.Message) bool      // Evaluate conditions
    ExecuteEvents(messages []message.Message) ([]Event, error)
}
```

### Entity State Evaluation

Rules evaluate directly against NATS KV entity state:

```go
type EntityStateEvaluator interface {
    EvaluateEntityState(entityState *gtypes.EntityState) bool
}
```

This is more efficient for rules that need to access triple predicates directly.

## State Transitions

Rules track state per entity:

| Transition | Trigger | Actions Executed |
|------------|---------|------------------|
| **Entered** | false → true | `on_enter` |
| **Exited** | true → false | `on_exit` |
| **While True** | true → true | `while_true` |
| **None** | false → false | (none) |

State is persisted in `RULE_STATE` KV bucket.

## Integration with Graph

When `EnableGraphIntegration` is true (default), rule actions directly affect the graph:

```
Rule Action: add_triple(predicate: "fleet.membership", object: "fleet-123")
     │
     ▼
Entity Triple Created: drone-007.fleet.membership → fleet-123
     │
     ▼
Index Updated: OUTGOING_INDEX, INCOMING_INDEX
     │
     ▼
Community Detection: drone-007 now clusters with fleet-123
```

Rules don't just alert - they build graph structure.

## Use Cases

### Alerting

```json
{
  "conditions": [{"field": "sensor.measurement.celsius", "operator": "gt", "value": 100}],
  "on_enter": [{"type": "publish", "subject": "alerts.temperature"}]
}
```

### Dynamic Relationships

```json
{
  "conditions": [{"field": "drone.zone", "operator": "ne", "value": ""}],
  "on_enter": [{"type": "add_triple", "predicate": "zone.membership", "object": "zone.${entity.zone}"}],
  "on_exit": [{"type": "remove_triple", "predicate": "zone.membership"}]
}
```

### State Machines

```json
{
  "conditions": [{"field": "equipment.status", "operator": "eq", "value": "maintenance"}],
  "on_enter": [{"type": "add_triple", "predicate": "ops.state", "object": "offline"}],
  "on_exit": [{"type": "add_triple", "predicate": "ops.state", "object": "online"}]
}
```

### Hierarchical Grouping

```json
{
  "conditions": [{"field": "sensor.equipment_id", "operator": "ne", "value": ""}],
  "on_enter": [{"type": "add_triple", "predicate": "equipment.parent", "object": "${entity.equipment_id}"}]
}
```

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        RuleProcessor                            │
├─────────────────────────────────────────────────────────────────┤
│  EntityWatcher ──────┬──────> MessageHandler                    │
│  (KV Watch)          │        (Message Processing)              │
│                      │                                          │
│                      ▼                                          │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │                   StatefulEvaluator                       │  │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────────┐   │  │
│  │  │ ExprRule 1  │  │ ExprRule 2  │  │ ExprRule N      │   │  │
│  │  │ (battery)   │  │ (zone)      │  │ (...)           │   │  │
│  │  └─────────────┘  └─────────────┘  └─────────────────┘   │  │
│  │                                                           │  │
│  │  RULE_STATE bucket: tracks entered/exited per entity      │  │
│  └──────────────────────────────────────────────────────────┘  │
│                      │                                          │
│                      ▼                                          │
│  ActionExecutor ─────┬──────> GraphIntegration (add_triple)    │
│                      └──────> Publisher (publish)               │
└─────────────────────────────────────────────────────────────────┘
```

## Key Components

| Component | Responsibility |
|-----------|---------------|
| `EntityWatcher` | Watches ENTITY_STATES KV for changes |
| `MessageHandler` | Processes messages from NATS subjects |
| `StatefulEvaluator` | Tracks state transitions, fires actions |
| `ExpressionRule` | Evaluates conditions using DSL |
| `ActionExecutor` | Executes add_triple, remove_triple, publish |

## Limitations

- Rules cannot query other entities (only current entity state)
- Rules cannot access community membership
- No bulk rule operations (each entity evaluated individually)
- No cross-entity conditions (A depends on B)

## Next Steps

- [Rule Syntax](02-rule-syntax.md) - JSON definition structure
- [Conditions](03-conditions.md) - Expression DSL and operators
- [Actions](04-actions.md) - Available actions and graph impact
- [State Tracking](05-state-tracking.md) - OnEnter/OnExit/WhileTrue
