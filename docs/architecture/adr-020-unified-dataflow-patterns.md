# ADR-020: Unified Dataflow Patterns

## Status

Accepted

## Context

SemStreams has two configuration systems for defining data pipelines:

| System | Unit | Data Routing | Type Declaration |
|--------|------|--------------|------------------|
| **Flows** | Components | Ports with subjects | `interface` on port |
| **Workflows** | Steps | String path interpolation | `input_type`/`output_type` on step |

A workflow is conceptually a "mini-flow" - steps are components, data flows between them. But the patterns diverge significantly:

### Flow Pattern (Consistent)

```json
{
  "components": {
    "parser": {
      "config": {
        "ports": {
          "inputs": [{"name": "raw", "subject": "raw.data", "type": "jetstream"}],
          "outputs": [{"name": "parsed", "subject": "parsed.data", "interface": "data.parsed.v1"}]
        }
      }
    },
    "enricher": {
      "config": {
        "ports": {
          "inputs": [{"name": "in", "subject": "parsed.data", "interface": "data.parsed.v1"}],
          "outputs": [{"name": "out", "subject": "enriched.data", "interface": "data.enriched.v1"}]
        }
      }
    }
  }
}
```

- Ports have names, subjects, and optional interface types
- Connections are implicit via matching subjects
- Type info travels in BaseMessage envelope

### Workflow Pattern (Inconsistent)

```json
{
  "steps": [
    {
      "name": "fetch",
      "output_type": "data.response.v1",
      "action": {"type": "call", "subject": "data.fetch"}
    },
    {
      "name": "process",
      "input_type": "processor.request.v1",
      "action": {
        "type": "call",
        "subject": "processor.run",
        "payload_mapping": {
          "data": "steps.fetch.output.result",
          "user_id": "trigger.payload.user_id",
          "exec_id": "execution.id"
        }
      }
    }
  ]
}
```

- Magic string paths (`steps.X.output.Y`, `trigger.payload.Z`, `execution.id`)
- Embedded query language for data access
- Type declarations separate from data wiring
- `payload_mapping` has no equivalent in flows

### The Problem

1. **Cognitive overhead**: Two mental models for the same concept (data flowing through typed stages)
2. **String paths are fragile**: Typos in `steps.fetch.output.result` caught only at runtime (or load-time with recent validation)
3. **Inconsistent type handling**: Flows use `interface` on ports; workflows use `input_type`/`output_type` on steps
4. **Hidden query language**: `payload_mapping` paths are a mini-DSL that doesn't exist elsewhere

### Type Safety Reality Check

Both systems ultimately rely on the same "cheat":

```
Go types → JSON + type metadata → NATS → JSON + type metadata → Go types
           (serialize)                      (deserialize via PayloadRegistry)
```

True compile-time type safety across process boundaries is impossible. The best we can do is:
- Schema validation at load time
- Type metadata in message envelopes (BaseMessage)
- Consistent patterns for type reconstruction (PayloadRegistry)

## Decision

Unify the dataflow patterns so workflows feel like mini-flows:

### Unified Model

| Concept | Flows | Workflows (Proposed) |
|---------|-------|---------------------|
| Processing unit | Component | Step |
| Data contract | Port | Port |
| Input declaration | `inputs: [{subject, interface}]` | `inputs: [{from, interface}]` |
| Output declaration | `outputs: [{subject, interface}]` | `outputs: [{name, interface}]` |
| Connections | Implicit via subject match | Explicit via `from` reference |
| Type metadata | `interface` field | `interface` field |

### Proposed Workflow Schema

```json
{
  "name": "process-data",
  "trigger": {"subject": "workflow.trigger.process"},
  "steps": [
    {
      "name": "fetch",
      "action": {"type": "call", "subject": "data.fetch"},
      "outputs": {
        "result": {"interface": "data.response.v1"}
      }
    },
    {
      "name": "process",
      "inputs": {
        "data": {"from": "fetch.result"},
        "user_id": {"from": "trigger.payload.user_id"},
        "exec_id": {"from": "execution.id"}
      },
      "action": {"type": "call", "subject": "processor.run"},
      "outputs": {
        "processed": {"interface": "processor.result.v1"}
      }
    },
    {
      "name": "notify",
      "inputs": {
        "message": {"from": "process.processed.summary"}
      },
      "action": {"type": "publish", "subject": "notifications.send"}
    }
  ]
}
```

### Key Changes

1. **`inputs` replaces `payload_mapping`**
   - Explicit input declarations with `from` references
   - Same pattern as flow port connections
   - `interface` optional but enables validation

2. **`outputs` declares what a step produces**
   - Named outputs with optional interface types
   - Enables downstream validation of `from` references
   - Maps to step result fields

3. **`from` syntax is structured, not string interpolation**
   - `"from": "step_name.output_name"` - reference another step's output
   - `"from": "trigger.payload.field"` - reference trigger data
   - `"from": "execution.id"` - reference execution context
   - Parsed and validated at load time, not runtime string interpolation

4. **Remove `payload_mapping`, `pass_through`, `input_type`, `output_type`**
   - `inputs`/`outputs` with `interface` replaces all of these
   - Single consistent pattern

### Validation at Load Time

```go
func validateWorkflow(def *Definition) []error {
    var errs []error

    // Build output registry from all steps
    outputs := map[string]OutputDef{}
    for _, step := range def.Steps {
        for name, out := range step.Outputs {
            outputs[step.Name+"."+name] = out
        }
    }

    // Validate all input references
    for _, step := range def.Steps {
        for inputName, input := range step.Inputs {
            if !isValidFrom(input.From, outputs, def.Trigger) {
                errs = append(errs, fmt.Errorf(
                    "step %q input %q: invalid reference %q",
                    step.Name, inputName, input.From,
                ))
            }
            // Optional: validate interface compatibility
            if input.Interface != "" {
                // Check PayloadRegistry for type existence
            }
        }
    }

    return errs
}
```

### Runtime Behavior

The workflow executor:

1. **On step execution**: Resolves all `inputs` by looking up `from` references
2. **Builds payload**: Assembles `map[string]any` from resolved inputs
3. **Type reconstruction**: If step has `interface` on action, uses PayloadRegistry.BuildPayload()
4. **Publishes**: BaseMessage with type metadata
5. **Records output**: Stores step result keyed by output names

Same JSON round-trip as today, but with consistent patterns.

## Implementation

This is a greenfield implementation. Remove the old pattern entirely:

1. **Update schema**: Replace `payload_mapping`, `pass_through`, `input_type`, `output_type` with `inputs`/`outputs`
2. **Update interpolator**: Resolve `from` references instead of string path interpolation
3. **Update validation**: Validate `from` references and `interface` types at load time
4. **Update tests**: All workflow tests use new pattern
5. **Update documentation**: Examples use unified pattern

## Consequences

### Positive

- **Unified mental model**: Flows and workflows use same patterns
- **Explicit over implicit**: Input/output declarations are clear
- **Better validation**: Structured `from` references validated at load time
- **Self-documenting**: Step inputs/outputs visible in definition
- **Tooling friendly**: Easier to build visual editors, analyzers
- **Clean slate**: No legacy patterns to support

### Negative

- **More verbose**: Explicit inputs/outputs vs terse string paths
- **Implementation effort**: Rework existing workflow internals

### Neutral

- **Same runtime behavior**: Still JSON round-trip under the hood
- **Same type safety**: Load-time validation, runtime reconstruction
- **Same BaseMessage pattern**: Type metadata in envelope unchanged

## Alternatives Considered

### Keep Separate Patterns

Status quo. Workflows keep `payload_mapping` with string paths.

Rejected because:
- Cognitive overhead for users and AI agents
- Inconsistent validation approaches
- String interpolation is fragile

### Full Port Semantics in Workflows

Make workflow steps exactly like flow components with full port definitions.

Rejected because:
- Over-engineering for sequential workflows
- Workflows don't need subject-based routing between steps
- Steps share execution context (trigger, previous outputs)

### GraphQL-style Field Selection

```json
{
  "inputs": {
    "data": "fetch { result { items } }",
    "user": "trigger { payload { user_id, name } }"
  }
}
```

Rejected because:
- Adds another query language
- More complex parsing
- Overkill for simple field references

## References

- [ADR-011: Workflow Processor](./adr-011-workflow-processor.md)
- [ADR-018: Agentic Workflow Orchestration](./adr-018-agentic-workflow-orchestration.md)
- [Orchestration Layers](../concepts/12-orchestration-layers.md)
