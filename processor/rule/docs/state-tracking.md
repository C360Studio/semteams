# State Tracking

Rules are stateful. They remember whether each entity matched the condition previously and fire actions only on state transitions.

## Why State Tracking?

Without state tracking, rules would fire on every evaluation:

```
Entity update: battery = 15%
Rule evaluates: 15 < 20 → true → fire alert
Entity update: battery = 14%
Rule evaluates: 14 < 20 → true → fire alert (again!)
Entity update: battery = 13%
Rule evaluates: 13 < 20 → true → fire alert (again!)
```

With state tracking:

```
Entity update: battery = 15%
Rule evaluates: 15 < 20 → true
Previous state: false (no record)
Transition: false → true = ENTERED
Fire on_enter actions

Entity update: battery = 14%
Rule evaluates: 14 < 20 → true
Previous state: true
Transition: true → true = NONE (no action)

Entity update: battery = 25%
Rule evaluates: 25 < 20 → false
Previous state: true
Transition: true → false = EXITED
Fire on_exit actions
```

## State Transitions

```go
type Transition string

const (
    TransitionNone    Transition = ""        // No state change
    TransitionEntered Transition = "entered" // false → true
    TransitionExited  Transition = "exited"  // true → false
)
```

| Previous | Current | Transition | Actions Fired |
|----------|---------|------------|---------------|
| false | true | `entered` | `on_enter` |
| true | false | `exited` | `on_exit` |
| true | true | `none` | `while_true` |
| false | false | `none` | (none) |

## Transition Detection

```go
func DetectTransition(wasMatching, nowMatching bool) Transition {
    if !wasMatching && nowMatching {
        return TransitionEntered
    }
    if wasMatching && !nowMatching {
        return TransitionExited
    }
    return TransitionNone
}
```

## OnEnter

Fires once when condition changes from false to true.

```json
{
  "on_enter": [
    {"type": "add_triple", "predicate": "alert.status", "object": "active"},
    {"type": "publish", "subject": "alerts.battery.low"}
  ]
}
```

**Use cases:**
- Create alert status
- Publish notification
- Establish relationship
- Start monitoring state

## OnExit

Fires once when condition changes from true to false.

```json
{
  "on_exit": [
    {"type": "remove_triple", "predicate": "alert.status"},
    {"type": "publish", "subject": "alerts.battery.recovered"}
  ]
}
```

**Use cases:**
- Remove alert status
- Publish recovery notification
- Remove relationship
- Clean up monitoring state

## WhileTrue

Fires on every evaluation while condition remains true (no transition).

```json
{
  "while_true": [
    {"type": "publish", "subject": "monitoring.battery.status"}
  ]
}
```

**Use cases:**
- Continuous monitoring messages
- Periodic status updates
- Heartbeat while in state

**Warning:** WhileTrue fires frequently. Consider cooldown to limit rate.

## State Persistence

State is stored in `RULE_STATE` NATS KV bucket.

### MatchState Structure

```go
type MatchState struct {
    RuleID         string            // Rule identifier
    EntityKey      string            // Entity ID or canonical pair key
    IsMatching     bool              // Current match state
    LastTransition string            // ""|"entered"|"exited"
    TransitionAt   time.Time         // When last transition occurred
    SourceRevision uint64            // Entity version that caused state
    LastChecked    time.Time         // When rule was last evaluated
    FieldValues    map[string]string // Previous field values (used by transition operator)
}
```

`FieldValues` is populated automatically when a rule uses the `transition` operator. On every
evaluation the current value of each `transition`-operator field is written back into `FieldValues`
so the *next* evaluation can compare against it. This is why a `transition` condition always returns
false on first evaluation — there is no recorded history yet.

### Key Format

Keys in `RULE_STATE` use the format: `{ruleID}.{entityKey}`

**Single entity:**
```
battery-low-alert.acme.logistics.robotics.fleet.drone.drone-007
```

**Entity pair (for relationship rules):**
```
proximity-alert.drone-007_drone-008
```

Pair keys are sorted alphabetically to ensure the same key regardless of which entity triggered evaluation.

### Bucket Configuration

```go
// Created by RuleProcessor.initializeStateTracker()
cfg := jetstream.KeyValueConfig{
    Bucket:      "RULE_STATE",
    Description: "Rule match state tracking",
    TTL:         24 * time.Hour, // Optional: auto-cleanup
}
```

## State Recovery

When the rule processor starts:

1. Loads existing state from `RULE_STATE` bucket
2. No previous state = entity treated as `wasMatching = false`
3. First evaluation determines initial state

```go
prevState, err := e.stateTracker.Get(ctx, ruleDef.ID, entityKey)
wasMatching := false

if err != nil {
    if errors.Is(err, ErrStateNotFound) {
        wasMatching = false // No previous state
    } else {
        return TransitionNone, err // Real error
    }
} else {
    wasMatching = prevState.IsMatching
}
```

This means:
- New entities: First evaluation that matches will fire `on_enter`
- Processor restart: Existing states are preserved
- New rules: All entities treated as "not matching" initially

## State Cleanup

### On Entity Deletion

When an entity is deleted, its rule states are cleaned up:

```go
func (st *StateTracker) DeleteAllForEntity(ctx context.Context, entityID string) error {
    keys, err := st.bucket.Keys(ctx)
    if err != nil {
        return err
    }

    for _, key := range keys {
        if containsEntityID(key, entityID) {
            st.bucket.Delete(ctx, key)
        }
    }
    return nil
}
```

### TTL-Based Cleanup

If configured with TTL, stale state entries expire automatically:

```go
cfg := jetstream.KeyValueConfig{
    Bucket: "RULE_STATE",
    TTL:    24 * time.Hour,
}
```

## Evaluation Flow

```
1. Entity update arrives
        │
        ▼
2. Load previous state from RULE_STATE
        │
        ▼
3. Evaluate conditions against entity
        │
        ▼
4. Detect transition (entered/exited/none)
        │
        ▼
5. Execute actions based on transition:
   - entered → on_enter actions
   - exited → on_exit actions
   - none + matching → while_true actions
        │
        ▼
6. Persist new state to RULE_STATE
```

## Example: Complete Lifecycle

```
Timeline for drone-007 with battery-low rule (threshold < 20):

T1: battery = 85%
    Evaluate: 85 < 20 = false
    Previous: none → false
    Transition: false → false = NONE
    State saved: {isMatching: false}

T2: battery = 15%
    Evaluate: 15 < 20 = true
    Previous: false
    Transition: false → true = ENTERED
    Execute on_enter: add_triple, publish alert
    State saved: {isMatching: true, lastTransition: "entered"}

T3: battery = 10%
    Evaluate: 10 < 20 = true
    Previous: true
    Transition: true → true = NONE
    Execute while_true: publish monitoring
    State saved: {isMatching: true}

T4: battery = 25%
    Evaluate: 25 < 20 = false
    Previous: true
    Transition: true → false = EXITED
    Execute on_exit: remove_triple, publish recovery
    State saved: {isMatching: false, lastTransition: "exited"}
```

## Debugging State

### Check Current State

```bash
# View specific entity state
nats kv get RULE_STATE "battery-low-alert.drone-007"

# List all states for a rule
nats kv ls RULE_STATE | grep battery-low-alert

# List all states for an entity
nats kv ls RULE_STATE | grep drone-007
```

### Expected Output

```json
{
  "rule_id": "battery-low-alert",
  "entity_key": "acme.logistics.robotics.fleet.drone.drone-007",
  "is_matching": true,
  "last_transition": "entered",
  "transition_at": "2024-01-15T10:30:00Z",
  "last_checked": "2024-01-15T10:35:00Z"
}
```

## Cooldown Interaction

Cooldown prevents rapid re-triggering, complementing state tracking:

```json
{
  "id": "battery-low-alert",
  "cooldown": "5m",
  "on_enter": [...]
}
```

| Scenario | State Tracking | Cooldown |
|----------|----------------|----------|
| Condition stays true | Only fires once (entered) | N/A |
| Condition flaps (true/false/true) | Fires each enter | Blocks re-enter within 5m |
| Condition stays false | No fire | N/A |

State tracking handles normal transitions. Cooldown handles rapid flapping.

## Limitations

- State is per-rule, per-entity (no cross-entity state)
- No conditional action execution within a transition
- State recovery requires NATS KV availability
- No manual state manipulation API (intentional)

## Next Steps

- [Entity Watching](06-entity-watching.md) - How entities trigger evaluation
- [Configuration](07-configuration.md) - Rule processor settings
- [Examples](10-examples.md) - Complete stateful rule examples
