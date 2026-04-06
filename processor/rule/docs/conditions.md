# Conditions

Conditions are expressions that evaluate against entity triples. This document covers all supported operators and field types.

## Condition Structure

```go
type ConditionExpression struct {
    Field    string      // Predicate path (e.g., "drone.telemetry.battery")
    Operator string      // Comparison operator
    Value    interface{} // Comparison value
    Required bool        // If false, missing field doesn't fail evaluation
}
```

## Field Paths

Fields use predicate paths from entity triples:

```json
{"field": "drone.telemetry.battery"}
{"field": "sensor.measurement.celsius"}
{"field": "entity.type"}
{"field": "geo.location.zone"}
```

The evaluator looks up the predicate in the entity's triples and extracts the object value.

## Field Types

The evaluator detects field type automatically:

| Type | Examples | Applicable Operators |
|------|----------|---------------------|
| `float64` | `23.5`, `100`, `-5.2` | eq, ne, lt, lte, gt, gte, between |
| `string` | `"active"`, `"warehouse-7"` | eq, ne, contains, starts_with, ends_with, regex, transition |
| `bool` | `true`, `false` | eq, ne |
| `array` | `["a", "b"]`, `[1, 2, 3]` | in, not_in, length_eq, length_gt, length_lt, array_contains |

## Numeric Operators

### eq (Equal)

```json
{"field": "sensor.measurement.celsius", "operator": "eq", "value": 25.0}
```

### ne (Not Equal)

```json
{"field": "drone.telemetry.battery", "operator": "ne", "value": 0}
```

### lt (Less Than)

```json
{"field": "drone.telemetry.battery", "operator": "lt", "value": 20}
```

### lte (Less Than or Equal)

```json
{"field": "sensor.measurement.celsius", "operator": "lte", "value": 100}
```

### gt (Greater Than)

```json
{"field": "sensor.measurement.celsius", "operator": "gt", "value": 50}
```

### gte (Greater Than or Equal)

```json
{"field": "drone.telemetry.altitude", "operator": "gte", "value": 100}
```

### between (Range)

```json
{"field": "sensor.measurement.celsius", "operator": "between", "value": [10, 30]}
```

Value must be a two-element array `[min, max]`. Returns true if `min <= field <= max`.

## String Operators

### eq (Equal)

```json
{"field": "entity.type", "operator": "eq", "value": "drone"}
```

### ne (Not Equal)

```json
{"field": "entity.status", "operator": "ne", "value": "offline"}
```

### contains

```json
{"field": "entity.description", "operator": "contains", "value": "warehouse"}
```

### starts_with

```json
{"field": "entity.id", "operator": "starts_with", "value": "acme.logistics"}
```

### ends_with

```json
{"field": "entity.id", "operator": "ends_with", "value": ".sensor.temperature"}
```

### regex

```json
{"field": "entity.id", "operator": "regex", "value": "^acme\\..*\\.drone\\.\\d+$"}
```

Uses Go regular expression syntax.

## Boolean Operators

### eq

```json
{"field": "entity.active", "operator": "eq", "value": true}
```

### ne

```json
{"field": "entity.maintenance_mode", "operator": "ne", "value": true}
```

## Array Operators

> **Note**: The operators in this section (`in`, `not_in`, `between`, `length_eq`, `length_gt`,
> `length_lt`, `array_contains`) were previously missing from the operator validation list and would
> be rejected at rule load time despite being fully implemented. This was corrected in Phase 1 of the
> rules engine KV twofer work.

### in (Value in Array)

Checks if field value is in the provided array:

```json
{"field": "entity.status", "operator": "in", "value": ["active", "standby", "ready"]}
```

### not_in (Value Not in Array)

```json
{"field": "entity.status", "operator": "not_in", "value": ["offline", "error", "maintenance"]}
```

### length_eq (Array Length Equal)

```json
{"field": "entity.tags", "operator": "length_eq", "value": 3}
```

### length_gt (Array Length Greater Than)

```json
{"field": "entity.sensors", "operator": "length_gt", "value": 0}
```

### length_lt (Array Length Less Than)

```json
{"field": "entity.errors", "operator": "length_lt", "value": 5}
```

### array_contains (Array Contains Value)

```json
{"field": "entity.tags", "operator": "array_contains", "value": "critical"}
```

## Logic Operators

Combine multiple conditions:

### AND (default)

All conditions must be true:

```json
{
  "conditions": [
    {"field": "entity.type", "operator": "eq", "value": "drone"},
    {"field": "drone.telemetry.battery", "operator": "lt", "value": 20}
  ],
  "logic": "and"
}
```

### OR

Any condition must be true:

```json
{
  "conditions": [
    {"field": "drone.telemetry.battery", "operator": "lt", "value": 10},
    {"field": "entity.status", "operator": "eq", "value": "critical"}
  ],
  "logic": "or"
}
```

## Required vs Optional Fields

### Required (default: false)

When `required: true`, the condition fails if the field doesn't exist:

```json
{
  "field": "drone.telemetry.battery",
  "operator": "lt",
  "value": 20,
  "required": true
}
```

If entity has no `drone.telemetry.battery` triple, condition returns false.

### Optional (required: false)

When `required: false`, missing fields don't cause failure:

```json
{
  "field": "optional.metadata.priority",
  "operator": "eq",
  "value": "high",
  "required": false
}
```

If field is missing, condition returns false but evaluation continues.

## Type Coercion

The evaluator attempts type coercion when comparing:

| Field Type | Value Type | Behavior |
|------------|------------|----------|
| float64 | int | Converts int to float64 |
| string | number | Converts number to string |
| bool | string | "true"/"false" converted |

If coercion fails, the condition returns false with an error.

## Common Patterns

### Threshold Alert

```json
{
  "conditions": [
    {"field": "sensor.measurement.celsius", "operator": "gt", "value": 100}
  ]
}
```

### Entity Type Filter

```json
{
  "conditions": [
    {"field": "entity.type", "operator": "eq", "value": "drone"},
    {"field": "drone.telemetry.battery", "operator": "lt", "value": 20}
  ]
}
```

### Status Change Detection

```json
{
  "conditions": [
    {"field": "entity.status", "operator": "in", "value": ["error", "critical", "offline"]}
  ]
}
```

### Range Check

```json
{
  "conditions": [
    {"field": "sensor.measurement.celsius", "operator": "between", "value": [15, 25]}
  ]
}
```

### Non-Empty Field

```json
{
  "conditions": [
    {"field": "entity.zone", "operator": "ne", "value": ""}
  ]
}
```

## Transition Operator

The `transition` operator validates that a field is moving from a set of allowed previous values to a
specific target value. It enables state machine rules that only fire on valid state progressions —
invalid transitions simply don't match.

```json
{
  "field": "workflow.plan.status",
  "operator": "transition",
  "from": ["created", "rejected"],
  "value": "drafting"
}
```

| Field | Required | Description |
|-------|----------|-------------|
| `field` | yes | Predicate path to evaluate |
| `operator` | yes | `"transition"` |
| `value` | yes | The required target value |
| `from` | no | Allowed previous values (single string or array) |

**Behavior rules:**

- The current field value must equal `value`.
- If `from` is specified, the previous field value must be one of those values.
- If `from` is omitted, any change to `value` counts as a valid transition.
- **First evaluation returns false** — a transition requires a recorded previous value.
  The very first time a rule sees an entity, there is no history, so the condition never
  matches on that evaluation.

Previous field values are stored per-rule per-entity in the `RULE_STATE` KV bucket alongside
the standard match state. See [State Tracking](05-state-tracking.md) for details.

### Example: Workflow Status Gate

Only allow a plan to enter `drafting` from `created` or `rejected`:

```json
{
  "field": "workflow.plan.status",
  "operator": "transition",
  "from": ["created", "rejected"],
  "value": "drafting"
}
```

If the plan jumps directly from `created` to `approved` (skipping `drafting`), the `drafting`
transition rule never fires. Invalid progressions are silently skipped.

### Example: Any Change to a Value

Detect whenever an entity's status becomes `offline`, regardless of prior state:

```json
{
  "field": "entity.status",
  "operator": "transition",
  "value": "offline"
}
```

This fires once each time the field transitions *to* `offline` from any other value.

## Evaluation Errors

| Error | Cause |
|-------|-------|
| Field not found | Required field doesn't exist |
| Type mismatch | Operator not supported for field type |
| Invalid value | Value format doesn't match operator |
| Invalid regex | Regex pattern is malformed |

Errors are logged but don't crash the rule processor.

## Next Steps

- [Actions](04-actions.md) - What happens when conditions match
- [State Tracking](05-state-tracking.md) - OnEnter/OnExit transitions
- [Examples](10-examples.md) - Complete working rules
