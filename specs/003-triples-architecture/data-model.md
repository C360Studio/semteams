# Data Model: Triples Architecture Evolution

**Feature**: 003-triples-architecture
**Date**: 2025-11-27
**Status**: Draft

## Entity Overview

This feature modifies core data structures to establish Triples as the single source of truth.

```text
┌─────────────────────────────────────────────────────────────────┐
│                        ENTITY_STATES KV                         │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ EntityState                                              │   │
│  │  ├── ID: string                                         │   │
│  │  ├── Type: string                                       │   │
│  │  ├── Triples: []Triple  ← SINGLE SOURCE OF TRUTH        │   │
│  │  ├── ObjectRef: string                                  │   │
│  │  ├── Version: uint64                                    │   │
│  │  └── UpdatedAt: time.Time                               │   │
│  └─────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
                              │
              ┌───────────────┼───────────────┐
              ▼               ▼               ▼
┌─────────────────┐ ┌─────────────────┐ ┌─────────────────┐
│ PREDICATE_INDEX │ │ OUTGOING_INDEX  │ │ INCOMING_INDEX  │
│ (existing)      │ │ (NEW)           │ │ (existing)      │
└─────────────────┘ └─────────────────┘ └─────────────────┘
```

---

## New Structures

### Triple (Modified)

**Location**: `message/triple.go`

```go
type Triple struct {
    Subject    string     `json:"subject"`
    Predicate  string     `json:"predicate"`
    Object     any        `json:"object"`
    Source     string     `json:"source"`
    Timestamp  time.Time  `json:"timestamp"`
    Confidence float64    `json:"confidence"`
    Context    string     `json:"context,omitempty"`
    Datatype   string     `json:"datatype,omitempty"`

    // NEW: Optional expiration time for TTL-based cleanup
    ExpiresAt  *time.Time `json:"expires_at,omitempty"`
}
```

**Validation Rules**:

- Subject: Must be valid 6-part EntityID format
- Predicate: Non-empty, dotted notation (e.g., "robotics.battery.level")
- ExpiresAt: If set, must be in the future at creation time

---

### OutgoingEntry (New)

**Location**: `processor/graph/indexmanager/indexes.go`

```go
type OutgoingEntry struct {
    Predicate  string `json:"predicate"`
    ToEntityID string `json:"to_entity_id"`
}
```

**Purpose**: Represents a single outgoing relationship in OUTGOING_INDEX

**Validation Rules**:

- Predicate: Non-empty string
- ToEntityID: Must be valid 6-part EntityID format

---

### RuleMatchState (New)

**Location**: `processor/rule/state_tracker.go`

```go
type RuleMatchState struct {
    RuleID         string    `json:"rule_id"`
    EntityKey      string    `json:"entity_key"`
    IsMatching     bool      `json:"is_matching"`
    LastTransition string    `json:"last_transition"` // "entered"|"exited"|""
    TransitionAt   time.Time `json:"transition_at,omitempty"`
    SourceRevision uint64    `json:"source_revision"`
    LastChecked    time.Time `json:"last_checked"`
}
```

**Purpose**: Tracks rule match state per entity or entity pair

**Key Format**: `{ruleID}:{entityKey}` where entityKey is:

- Single entity rules: `{entityID}`
- Pair rules: `{entityID1}:{entityID2}` (sorted for consistency)

**Validation Rules**:

- RuleID: Non-empty, matches defined rule
- EntityKey: Valid entity ID(s)
- LastTransition: One of "", "entered", "exited"

---

### Transition (New)

**Location**: `processor/rule/state_tracker.go`

```go
type Transition string

const (
    TransitionNone    Transition = ""
    TransitionEntered Transition = "entered"
    TransitionExited  Transition = "exited"
)
```

---

## Modified Structures

### EntityState (Target - Phase 5)

**Location**: `graph/types.go`

**Current**:

```go
type EntityState struct {
    Node        NodeProperties   `json:"node"`
    Edges       []Edge           `json:"edges"`        // REMOVE
    Triples     []message.Triple `json:"triples"`
    ObjectRef   string           `json:"object_ref"`
    MessageType string           `json:"message_type"` // REMOVE
    Version     uint64           `json:"version"`
    UpdatedAt   time.Time        `json:"updated_at"`
}
```

**Target (Phase 5)**:

```go
type EntityState struct {
    ID        string           `json:"id"`
    Type      string           `json:"type"`
    Triples   []message.Triple `json:"triples"`
    ObjectRef string           `json:"object_ref"`
    Version   uint64           `json:"version"`
    UpdatedAt time.Time        `json:"updated_at"`
}
```

**Migration Path**:

1. Phase 4: Add deprecation warnings to Edges, Node.Properties
2. Phase 4: Add helper methods GetTriple(), GetPropertyValue()
3. Phase 5: Remove deprecated fields

---

### Rule Definition (Modified)

**Location**: `processor/rule/config.go`

**Current**:

```go
type Definition struct {
    ID             string   `json:"id"`
    EntityPatterns []string `json:"entity_patterns"`
    Condition      string   `json:"condition"`
    Actions        []Action `json:"actions"`
    // ... other fields
}
```

**Extended (Phase 2)**:

```go
type Definition struct {
    ID              string   `json:"id"`
    EntityPatterns  []string `json:"entity_patterns"`
    RelatedPatterns []string `json:"related_patterns,omitempty"` // NEW: For pair rules
    Condition       string   `json:"condition"`

    // NEW: Stateful actions
    OnEnter   []Action `json:"on_enter,omitempty"`   // Fires on false→true
    OnExit    []Action `json:"on_exit,omitempty"`    // Fires on true→false
    WhileTrue []Action `json:"while_true,omitempty"` // Fires on every update while true

    // Existing (deprecated for stateful rules)
    Actions []Action `json:"actions,omitempty"`
    // ... other fields
}
```

---

### Action (Modified)

**Location**: `processor/rule/config.go`

**Extended (Phase 2)**:

```go
type Action struct {
    Type      string `json:"type"`      // "publish", "add_triple", "remove_triple", etc.
    Subject   string `json:"subject"`   // NATS subject for publish
    Predicate string `json:"predicate"` // NEW: For triple actions
    Object    string `json:"object"`    // NEW: For triple actions (supports $related.id)
    TTL       string `json:"ttl"`       // NEW: Optional TTL (e.g., "5m")
    // ... other fields
}
```

**New Action Types**:

- `add_triple`: Creates relationship triple via mutation API
- `remove_triple`: Removes relationship triple via mutation API
- `update_triple`: Updates triple properties (while_true use case)

---

## Index Structures

### OUTGOING_INDEX (New KV Bucket)

**Bucket Name**: `OUTGOING_INDEX`
**Key Format**: `{sourceEntityID}`
**Value Format**: `[]OutgoingEntry` (JSON array)

**Example**:

```json
// Key: "acme.telemetry.robotics.gcs1.drone.001"
// Value:
[
  {"predicate": "ops.fleet.member_of", "to_entity_id": "acme.ops.logistics.hq.fleet.rescue"},
  {"predicate": "robotics.operator.controlled_by", "to_entity_id": "acme.platform.auth.main.user.alice"}
]
```

**Operations**:

- Create: Add all relationship triples from new entity
- Update: Diff old vs new triples, add/remove entries
- Delete: Remove entire key

---

### RULE_STATE (New KV Bucket)

**Bucket Name**: `RULE_STATE`
**Key Format**: `{ruleID}:{entityKey}`
**Value Format**: `RuleMatchState` (JSON object)

**Example**:

```json
// Key: "proximity-tracking:acme.telemetry.robotics.gcs1.drone.001:acme.telemetry.robotics.gcs1.drone.002"
// Value:
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

---

## State Transitions

### Rule State Machine

```text
                    ┌──────────────┐
                    │  No State    │
                    │  (initial)   │
                    └──────┬───────┘
                           │ First evaluation
                           ▼
              ┌────────────────────────┐
              │                        │
    ┌─────────┴─────────┐    ┌────────┴─────────┐
    │  IsMatching=false │    │  IsMatching=true │
    │  (condition false)│    │  (condition true)│
    └─────────┬─────────┘    └────────┬─────────┘
              │                        │
              │ Condition becomes true │ Condition becomes false
              │ → on_enter fires       │ → on_exit fires
              │ → TransitionEntered    │ → TransitionExited
              │                        │
              └────────────────────────┘
```

### Entity Lifecycle with Indexes

```text
Entity Created
     │
     ▼
┌─────────────┐
│ Store in    │
│ENTITY_STATES│
└─────┬───────┘
      │
      ├──► Update PREDICATE_INDEX (all triples)
      ├──► Update OUTGOING_INDEX (relationship triples)
      └──► Update INCOMING_INDEX (relationship triples)

Entity Updated
     │
     ▼
┌─────────────┐
│ Diff old vs │
│ new triples │
└─────┬───────┘
      │
      ├──► Add/remove PREDICATE_INDEX entries
      ├──► Add/remove OUTGOING_INDEX entries
      └──► Add/remove INCOMING_INDEX entries

Entity Deleted
     │
     ▼
┌─────────────┐
│ Remove from │
│ENTITY_STATES│
└─────┬───────┘
      │
      ├──► Remove from PREDICATE_INDEX
      │
      │    ┌─────────────────────────────────────────┐
      │    │ OUTGOING_INDEX Cleanup (FR-005a/b/c)    │
      │    │ 1. Read OUTGOING_INDEX for deleted ID   │
      │    │ 2. For each target entity:              │
      │    │    RemoveIncomingReference(target, id)  │
      │    │ 3. Delete OUTGOING_INDEX entry          │
      │    └─────────────────────────────────────────┘
      ├──► Remove from OUTGOING_INDEX (with cleanup above)
      └──► Remove from INCOMING_INDEX (entity's own entry)
```

---

## Relationships

```text
Triple ──────────────────────────────────────────┐
  │                                               │
  │ contains                                      │
  ▼                                               │
EntityState ◄─────────────────────────────────────┘
  │
  │ indexed by
  ▼
┌─────────────────────────────────────────────────┐
│                    Indexes                       │
├─────────────────────────────────────────────────┤
│ PREDICATE_INDEX  (predicate:value → entityIDs) │
│ OUTGOING_INDEX   (entityID → [OutgoingEntry])  │
│ INCOMING_INDEX   (entityID → [IncomingEntry])  │
│ ALIAS_INDEX      (alias → entityID)            │
│ SPATIAL_INDEX    (geohash → entityIDs)         │
│ TEMPORAL_INDEX   (timeBucket → entityIDs)      │
└─────────────────────────────────────────────────┘

Rule ────────────────────────────────────────────┐
  │                                               │
  │ tracks state in                               │
  ▼                                               │
RuleMatchState ◄──────────────────────────────────┘
  │
  │ triggers
  ▼
Action (add_triple, remove_triple, publish)
  │
  │ mutates
  ▼
EntityState (via mutation API)
```

---

## Relationship Detection (FR-006a/b)

A triple represents a relationship when its Object is a valid 6-part EntityID.

**Correct Method** (`message/triple.go`):
```go
func (t Triple) IsRelationship() bool {
    if str, ok := t.Object.(string); ok {
        return IsValidEntityID(str)
    }
    return false
}
```

**Do NOT use hardcoded predicate lists**:
```go
// WRONG - DO NOT USE
func isRelationshipPredicate(predicate string) bool {
    relationshipPredicates := map[string]bool{
        "POWERED_BY": true,  // Hardcoded = unmaintainable
    }
    return relationshipPredicates[predicate]
}
```

**Rationale**: The EntityID format (6-part dotted notation) is the authoritative indicator of an entity reference. Any predicate can potentially point to an entity; the vocabulary system does not restrict this.

---

## Validation Summary

| Structure | Field | Validation |
|-----------|-------|------------|
| Triple | Subject | Valid 6-part EntityID |
| Triple | Predicate | Non-empty, dotted notation |
| Triple | Object | Use IsRelationship() to detect entity refs |
| Triple | ExpiresAt | If set, must be future time |
| OutgoingEntry | ToEntityID | Valid 6-part EntityID |
| RuleMatchState | RuleID | Must match defined rule |
| RuleMatchState | EntityKey | Valid EntityID(s) |
| RuleMatchState | LastTransition | "", "entered", or "exited" |
| Action | Type | One of defined action types |
| Action | TTL | Valid duration string (e.g., "5m") |
