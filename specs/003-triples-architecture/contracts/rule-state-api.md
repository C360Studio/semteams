# Rule State API Contract: Stateful ECA Rules

**Feature**: 003-triples-architecture
**Component**: processor/rule
**Date**: 2025-11-27

## Overview

The Rule State API enables stateful Event-Condition-Action rules that can detect transitions (condition becoming true/false) and fire appropriate actions.

## Interface

### StateTracker

```go
// StateTracker manages rule match state in NATS KV
type StateTracker struct {
    bucket    jetstream.KeyValue
    kvStore   *natsclient.KVStore
    cache     *lru.Cache[string, RuleMatchState] // Optional LRU cache
    logger    *slog.Logger
}

// NewStateTracker creates a new state tracker
func NewStateTracker(natsClient *natsclient.Client, logger *slog.Logger) (*StateTracker, error)

// Core operations
func (st *StateTracker) Get(ctx context.Context, ruleID, entityKey string) (RuleMatchState, error)
func (st *StateTracker) Set(ctx context.Context, state RuleMatchState) error
func (st *StateTracker) Delete(ctx context.Context, ruleID, entityKey string) error

// Bulk operations
func (st *StateTracker) GetAllForRule(ctx context.Context, ruleID string) ([]RuleMatchState, error)
func (st *StateTracker) DeleteAllForEntity(ctx context.Context, entityID string) error
```

### RuleMatchState

```go
type RuleMatchState struct {
    RuleID         string    `json:"rule_id"`
    EntityKey      string    `json:"entity_key"`
    IsMatching     bool      `json:"is_matching"`
    LastTransition string    `json:"last_transition"` // ""|"entered"|"exited"
    TransitionAt   time.Time `json:"transition_at,omitempty"`
    SourceRevision uint64    `json:"source_revision"`
    LastChecked    time.Time `json:"last_checked"`
}
```

### Transition

```go
type Transition string

const (
    TransitionNone    Transition = ""
    TransitionEntered Transition = "entered"
    TransitionExited  Transition = "exited"
)

// DetectTransition compares previous and current match state
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

## Rule Definition Schema

### Extended Rule Definition

```go
type Definition struct {
    ID              string   `json:"id"`
    EntityPatterns  []string `json:"entity_patterns"`
    RelatedPatterns []string `json:"related_patterns,omitempty"` // For pair rules
    Condition       string   `json:"condition"`

    // Stateful actions (NEW)
    OnEnter   []Action `json:"on_enter,omitempty"`   // Fires on false→true transition
    OnExit    []Action `json:"on_exit,omitempty"`    // Fires on true→false transition
    WhileTrue []Action `json:"while_true,omitempty"` // Fires on every update while true

    // Legacy actions (still supported, fires when condition is true)
    Actions []Action `json:"actions,omitempty"`
}
```

### Action Types

```go
type Action struct {
    Type      string            `json:"type"`
    Subject   string            `json:"subject,omitempty"`   // For "publish" type
    Predicate string            `json:"predicate,omitempty"` // For triple actions
    Object    string            `json:"object,omitempty"`    // For triple actions
    TTL       string            `json:"ttl,omitempty"`       // Optional expiration
    Properties map[string]any   `json:"properties,omitempty"` // Additional properties
}

// Action types
const (
    ActionTypePublish      = "publish"       // Publish to NATS subject
    ActionTypeAddTriple    = "add_triple"    // Create relationship triple
    ActionTypeRemoveTriple = "remove_triple" // Remove relationship triple
    ActionTypeUpdateTriple = "update_triple" // Update triple metadata
)
```

## YAML Configuration Example

```yaml
rules:
  - id: "proximity-tracking"
    entity_patterns: ["*.*.robotics.*.drone.*"]
    related_patterns: ["*.*.robotics.*.drone.*"]
    condition: "distance(entity.position, related.position) < 100"

    on_enter:
      - type: "add_triple"
        predicate: "proximity.near"
        object: "$related.id"
        ttl: "5m"

    on_exit:
      - type: "remove_triple"
        predicate: "proximity.near"
        object: "$related.id"

    while_true:
      - type: "update_triple"
        predicate: "proximity.near"
        object: "$related.id"
        properties:
          distance: "$calculated_distance"
```

## Evaluation Flow

```go
func (e *RuleEvaluator) evaluateWithState(
    ctx context.Context,
    rule Definition,
    entity EntityState,
    related *EntityState, // nil for single-entity rules
) error {
    // 1. Evaluate condition
    matches, err := e.evaluateCondition(rule.Condition, entity, related)
    if err != nil {
        return fmt.Errorf("evaluate condition: %w", err)
    }

    // 2. Build entity key
    entityKey := entity.ID
    if related != nil {
        entityKey = buildPairKey(entity.ID, related.ID)
    }

    // 3. Get previous state
    prevState, _ := e.stateTracker.Get(ctx, rule.ID, entityKey)

    // 4. Detect transition
    transition := DetectTransition(prevState.IsMatching, matches)

    // 5. Execute appropriate actions
    switch transition {
    case TransitionEntered:
        if err := e.executeActions(ctx, rule.OnEnter, entity, related); err != nil {
            return fmt.Errorf("execute on_enter: %w", err)
        }
    case TransitionExited:
        if err := e.executeActions(ctx, rule.OnExit, entity, related); err != nil {
            return fmt.Errorf("execute on_exit: %w", err)
        }
    case TransitionNone:
        if matches && len(rule.WhileTrue) > 0 {
            if err := e.executeActions(ctx, rule.WhileTrue, entity, related); err != nil {
                return fmt.Errorf("execute while_true: %w", err)
            }
        }
    }

    // 6. Update state
    newState := RuleMatchState{
        RuleID:         rule.ID,
        EntityKey:      entityKey,
        IsMatching:     matches,
        LastTransition: string(transition),
        TransitionAt:   time.Now(),
        SourceRevision: entity.Version,
        LastChecked:    time.Now(),
    }

    return e.stateTracker.Set(ctx, newState)
}
```

## KV Storage

### Bucket Configuration

```go
bucketConfig := jetstream.KeyValueConfig{
    Bucket:      "RULE_STATE",
    Description: "Rule match state tracking for stateful ECA rules",
    History:     5,  // Keep history for debugging
    TTL:         24 * time.Hour, // Auto-cleanup old states
    Storage:     jetstream.FileStorage,
}
```

### Key Format

```text
Single entity: {ruleID}:{entityID}
Entity pair:   {ruleID}:{entityID1}:{entityID2}  (IDs sorted alphabetically)

Examples:
- "low-battery:acme.telemetry.robotics.gcs1.drone.001"
- "proximity-tracking:acme.telemetry.robotics.gcs1.drone.001:acme.telemetry.robotics.gcs1.drone.002"
```

### Value Format

```json
{
  "rule_id": "proximity-tracking",
  "entity_key": "acme.telemetry.robotics.gcs1.drone.001:acme.telemetry.robotics.gcs1.drone.002",
  "is_matching": true,
  "last_transition": "entered",
  "transition_at": "2025-11-27T10:30:00Z",
  "source_revision": 42,
  "last_checked": "2025-11-27T10:35:00Z"
}
```

## Expression Functions

New expression functions for rule conditions:

```go
// Check if relationship triple exists
func hasTriple(entityID, predicate, objectID string) bool

// Get outgoing relationships
func getOutgoing(entityID string) []OutgoingEntry

// Get incoming relationships
func getIncoming(entityID string) []IncomingEntry

// Calculate distance between positions
func distance(pos1, pos2 Position) float64

// Get entity by ID
func getEntity(entityID string) EntityState
```

## Error Handling

```go
var (
    ErrRuleNotFound      = errors.New("rule not found")
    ErrStateNotFound     = errors.New("rule state not found")
    ErrInvalidTransition = errors.New("invalid transition")
    ErrActionFailed      = errors.New("action execution failed")
)
```

## Metrics

```go
// State operations
rule_state_operations_total{rule_id,operation="get|set|delete",status="success|error"}

// Transitions
rule_transitions_total{rule_id,transition="entered|exited"}

// Action execution
rule_actions_total{rule_id,action_type,status="success|error"}
rule_action_duration_seconds{rule_id,action_type}
```
