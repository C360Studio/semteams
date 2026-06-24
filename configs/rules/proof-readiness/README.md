# Proof-Readiness Rule Pack

This pack consumes the `formal_claims.*` envelope emitted by
`analyze_proof_readiness` and records the next flow-native route on the run
entity.

It does not subscribe to raw NATS subjects and it does not inspect
`formal_claims.finding.<id>.*` with wildcard predicate logic. The analyzer emits
exact route summary predicates because the SemStreams rule engine matches
literal graph predicates:

```text
formal_claims.route.implementation = "present"
formal_claims.route.test_harness   = "present"
formal_claims.route.coordinator    = "present"
```

The pack writes:

```text
proof_readiness.route                   = "implementation" | "test_harness" | "coordinator" | "pause"
proof_readiness.implementation_ready    = "true"
proof_readiness.test_harness_required   = "true"
proof_readiness.requires_clarification  = "true"
proof_readiness.pause_required          = "true"
proof_readiness.routed                  = "true"
```

`proof_readiness.routed` is a v1 duplicate-fire guard. The analyzer is currently
a one-shot gate in the spec-driven run. A future rerun/upsert slice should replace
the route summary and `proof_readiness.*` facts as an owned projection before the
test-harness team re-analyzes the same run.

Downstream ownership:

- `implementation` is the entry marker for the future `dev-from-task` pack.
- `test_harness` is the entry marker for the future test-harness category pack.
- `coordinator` and `pause` are UI/coordinator surfaces until those lifecycle
  actions are expanded.
