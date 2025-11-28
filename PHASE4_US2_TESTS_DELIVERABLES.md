# Phase 4 - User Story 2 Tests (TDD RED Phase) - Deliverables

**Feature**: 003-triples-architecture
**User Story**: US2 - Automatic Relationship Retraction
**Task Range**: T032-T041
**Date**: 2025-11-27
**Developer**: go-developer
**Status**: Gate 2 Complete (Implementation Readiness)

## Executive Summary

All tests for User Story 2 (Automatic Relationship Retraction) have been written following TDD protocol. Tests are currently in the RED state (failing with compilation errors) as expected, since the implementation types do not yet exist. This verifies proper TDD workflow.

## Test Files Created

### 1. `/processor/rule/state_tracker_test.go` (271 lines)
**Purpose**: Tests for RuleMatchState struct and StateTracker operations

**Test Functions** (6):
- `TestRuleMatchState` - T032: Validates RuleMatchState struct fields
- `TestStateTracker_Get` - T033: Tests retrieving rule state from KV store
- `TestStateTracker_Set` - T034: Tests setting rule state in KV store
- `TestDetectTransition` - T035: Tests transition detection logic (false→true, true→false, none)
- `TestTransitionConstants` - T035a: Verifies Transition type constants
- `TestStateTracker_Delete` - T036: Tests deleting rule state

**Key Coverage**:
- Single entity state tracking
- Entity pair state tracking (proximity rules)
- Transition detection for all combinations
- CRUD operations on RULE_STATE KV bucket
- Empty/invalid input validation

### 2. `/processor/rule/stateful_rule_test.go` (275 lines)
**Purpose**: Tests for stateful ECA rule behavior

**Test Functions** (5):
- `TestStatefulRule_OnEnter` - T036: on_enter fires exactly once on false→true transition
- `TestStatefulRule_OnExit` - T037: on_exit fires exactly once on true→false transition
- `TestStatefulRule_NoDuplicateOnEnter` - T038: on_enter doesn't fire repeatedly when condition stays true
- `TestStatefulRule_WhileTrue` - T038a: while_true fires on every update while condition holds
- `TestStateTracker_EntityPairKey` - T038b: Entity pair key generation and sorting

**Key Coverage**:
- on_enter action triggering (FR-008)
- on_exit action triggering (FR-009)
- while_true action triggering (FR-010)
- No duplicate on_enter when condition remains true
- Multiple evaluation sequences over time
- Alphabetical sorting for entity pair keys

### 3. `/processor/rule/actions_test.go` (284 lines)
**Purpose**: Tests for rule actions (add_triple, remove_triple)

**Test Functions** (7):
- `TestAction_AddTriple` - T039: Creates relationship triples via mutation API
- `TestAction_RemoveTriple` - T040: Removes relationship triples
- `TestAction` - T040a: Action struct validation
- `TestActionConstants` - T040b: Action type constants verification
- `TestAction_TTLParsing` - T040c: TTL duration parsing (5m, 1h, 30s formats)
- `TestAction_VariableSubstitution` - T040d: Variable substitution ($entity.id, $related.id)
- `TestActionExecutor` - T040e: Action executor initialization

**Key Coverage**:
- add_triple action with TTL support (FR-011, FR-013)
- remove_triple action (FR-012)
- Variable substitution in action templates
- TTL parsing and validation
- Action type constants
- Invalid action handling

### 4. `/processor/graph/cleanup_test.go` (410 lines)
**Purpose**: Tests for triple cleanup worker (TTL expiration)

**Test Functions** (6):
- `TestExpiredTripleCleanup` - T041: Removes expired triples from entity state
- `TestTripleCleanupWorker_Init` - T041a: Worker initialization with interval
- `TestTripleCleanupWorker_BackgroundExecution` - T041b: Background worker start/stop
- `TestTriple_IsExpired` - T041c: Triple expiration check (uses existing Triple.IsExpired())
- `TestTripleCleanupWorker_Metrics` - T041d: Cleanup metrics tracking
- `TestTripleCleanupWorker_BatchCleanup` - T041e: Batch cleanup across multiple entities

**Key Coverage**:
- Expired triple removal (FR-014)
- Non-expired triple retention
- Triples without expiration (nil ExpiresAt)
- Mixed expired/valid triples
- Worker lifecycle (start/stop)
- Cleanup metrics
- Batch operations

## Test Statistics

- **Total Test Files**: 4
- **Total Test Functions**: 24
- **Total Lines of Test Code**: ~1,240 lines
- **Test Coverage**: 8 Functional Requirements (FR-007 through FR-014)

## Types Required for Implementation (Not Yet Implemented)

### processor/rule package:
```go
type RuleMatchState struct {
    RuleID         string
    EntityKey      string
    IsMatching     bool
    LastTransition string
    TransitionAt   time.Time
    SourceRevision uint64
    LastChecked    time.Time
}

type Transition string
const (
    TransitionNone    Transition = ""
    TransitionEntered Transition = "entered"
    TransitionExited  Transition = "exited"
)

type StateTracker struct {
    // KV backend for state persistence
}

type Action struct {
    Type       string
    Subject    string
    Predicate  string
    Object     string
    TTL        string
    Properties map[string]any
}

type ActionExecutor struct {
    // Mutation API client
}

const (
    ActionTypePublish      = "publish"
    ActionTypeAddTriple    = "add_triple"
    ActionTypeRemoveTriple = "remove_triple"
    ActionTypeUpdateTriple = "update_triple"
)

func DetectTransition(wasMatching, nowMatching bool) Transition
func buildPairKey(entity1, entity2 string) string
func substituteVariables(template, entityID, relatedID string) string
```

### processor/graph package:
```go
type TripleCleanupWorker struct {
    interval time.Duration
    // ...
}

func NewTripleCleanupWorker(interval time.Duration) (*TripleCleanupWorker, error)
func (w *TripleCleanupWorker) CleanupExpiredTriples(ctx context.Context, entity *EntityState) (int, error)
func (w *TripleCleanupWorker) CleanupBatch(ctx context.Context, entities []*EntityState) (int, error)
func (w *TripleCleanupWorker) Start(ctx context.Context) error
func (w *TripleCleanupWorker) Stop() error
func (w *TripleCleanupWorker) GetMetrics() *CleanupMetrics
```

## Verification of RED State

All tests currently FAIL with compilation errors (expected TDD RED state):

```bash
$ go test -c ./processor/rule/... 2>&1 | head -15
processor/rule/actions_test.go:22:13: undefined: Action
processor/rule/actions_test.go:30:12: undefined: Action
processor/rule/actions_test.go:31:16: undefined: ActionTypeAddTriple
processor/rule/state_tracker_test.go:19:9: undefined: RuleMatchState
processor/rule/state_tracker_test.go:74:13: undefined: StateTracker
processor/rule/stateful_rule_test.go:68:17: undefined: RuleMatchState
processor/rule/stateful_rule_test.go:79:29: undefined: DetectTransition
...
FAIL	github.com/c360/semstreams/processor/rule [build failed]
```

```bash
$ go test -c ./processor/graph/... 2>&1 | grep cleanup
processor/graph/cleanup_test.go:120:15: undefined: TripleCleanupWorker
processor/graph/cleanup_test.go:171:19: undefined: NewTripleCleanupWorker
processor/graph/cleanup_test.go:209:17: undefined: NewTripleCleanupWorker
FAIL	github.com/c360/semstreams/processor/graph [build failed]
```

This is the correct TDD RED state - tests fail because implementation doesn't exist yet.

## Test Quality Standards Met

1. **Table-Driven Tests**: All tests use table-driven patterns for comprehensive coverage
2. **Parallel Execution**: All tests marked with `t.Parallel()` for concurrent execution
3. **Context-Aware**: All operations use `context.Context` as first parameter
4. **Explicit Synchronization**: No arbitrary `time.Sleep()` calls; explicit assertions
5. **Error Handling**: All error cases tested (empty values, nil values, invalid inputs)
6. **Edge Cases**: Boundary conditions tested (expired vs non-expired, nil ExpiresAt, etc.)
7. **Idempotency**: Tests verify idempotent operations (delete non-existent, etc.)
8. **Type Safety**: Strong typing with constants for transitions and action types

## Functional Requirements Coverage

| FR     | Description                           | Test Coverage                              |
|--------|---------------------------------------|--------------------------------------------|
| FR-007 | RULE_STATE KV bucket                  | StateTracker tests (Get, Set, Delete)      |
| FR-008 | on_enter actions (false→true)         | TestStatefulRule_OnEnter                   |
| FR-009 | on_exit actions (true→false)          | TestStatefulRule_OnExit                    |
| FR-010 | while_true actions                    | TestStatefulRule_WhileTrue                 |
| FR-011 | add_triple action type                | TestAction_AddTriple                       |
| FR-012 | remove_triple action type             | TestAction_RemoveTriple                    |
| FR-013 | Triple TTL support                    | TestAction_TTLParsing, TestAction_AddTriple|
| FR-014 | Cleanup worker for expired triples    | TestExpiredTripleCleanup, worker tests     |

## Next Steps (Implementation Phase - Gate 3)

1. **Implement types** - Create all struct and type definitions
2. **Implement StateTracker** - NATS KV backend for rule state persistence
3. **Implement DetectTransition** - Simple boolean comparison logic
4. **Implement Action types** - Action struct and executor
5. **Implement TripleCleanupWorker** - Background cleanup process
6. **Run tests** - Verify tests transition from RED → GREEN
7. **Refactor** - Optimize while keeping tests green
8. **Integration tests** - End-to-end testing with NATS KV

## Contract Alignment

All tests align with the contract specification in:
- `/specs/003-triples-architecture/contracts/rule-state-api.md`

Test entity IDs use the canonical 6-part format:
- `c360.platform1.robotics.mav1.drone.001`

Test predicates follow three-level dotted notation:
- `proximity.near`
- `fleet.member_of`

## Files Modified

**New Files**:
- `/processor/rule/state_tracker_test.go`
- `/processor/rule/stateful_rule_test.go`
- `/processor/rule/actions_test.go`
- `/processor/graph/cleanup_test.go`

**No Existing Files Modified** - Pure additive change (TDD tests only)

## Gate 2 Completion Checklist

- [x] Tests written following TDD protocol
- [x] Tests FAIL with compilation errors (RED state verified)
- [x] Table-driven test patterns used
- [x] All tests marked with `t.Parallel()`
- [x] Context-aware function signatures
- [x] Error cases tested
- [x] Edge cases tested
- [x] Contract alignment verified
- [x] FR coverage documented
- [x] No implementation code written (tests only)

## Handoff to go-reviewer

**Status**: Ready for Gate 3 (Code Completion)
**Blocker**: None
**Next Agent**: go-developer (implementation phase)
**Estimated Implementation**: ~4 hours for all types and logic

The test suite is comprehensive and ready to guide implementation. All tests follow Go best practices and idiomatic patterns. The RED state is verified - implementation can now proceed to make tests GREEN.
