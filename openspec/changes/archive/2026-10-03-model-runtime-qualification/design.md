# Configured model-boundary qualification

## Boundary and plan review

This test-only change closes the retry-coverage gap explicitly named by the mock bootstrap. It exercises frozen
upstream model behavior through SemTeams composition, while keeping shared types, sentinels and policy upstream.
Issue [#288](https://github.com/C360Studio/semteams/issues/288) owns the change; shared rollback integration remains
in [#287](https://github.com/C360Studio/semteams/issues/287).

Independent architecture plan review approved this boundary before implementation on 2026-10-03. The reviewed
sources are `cmd/semteams/main.go:654` (`setupRegistriesAndManager`), `:748` (`buildPayloadRegistry`), production model
configuration at `configs/flow-bootstrap.json:524–565`, and mock model configuration at
`configs/e2e-flow-bootstrap.json:503–545`. The approval covers the composition path, three total production attempts,
typed/correlated outcomes, synchronized shutdown before final call counts, and both regression mutations. Independent
Go change review subsequently passed on integration-test SHA-256
`1877e71807a80372d4bfdae74b16b7348e54c0f53feea1dc1ab4c9a0b349945d`. Focused/full integration and the required runtime
checks passed; the qualification record retains results and restored mutations. Final content review remains separate.

## Fixture and assertions

Add integration-tagged tests in the product package so they call the actual `setupRegistriesAndManager` and
`buildPayloadRegistry`. Use the public component manager to load and start the model component derived from each
shipped bootstrap. First assert that its original `agent.request` input is required and non-external; mark only that
input's `External` field true in the test copy. Preserve `Required`, subjects, stream, retry settings and every output
field. Configure the AGENT stream. Isolate NATS and local HTTP resources; direct the test's model endpoint to the local
fixture without changing shipped files or contacting a live provider.

The initial model-only admission failed with `orphaned_port` / `no_publishers`: its request publisher is the external
fixture, not an in-composition component. The
[retained log](../../../../docs/migrations/model-runtime-qualification/initial-composition-rejection.txt) is rejected
development evidence, not an accepted run. Independent architecture review approved the public external-input marker defined by frozen `component/ports.go:53–64` and
`composition/analyze.go:71–76`. This expresses the actual fixture boundary without a fake publisher component.
Qualification is of an isolated composed model with explicit external fixture input. Existing composition tests
retain the separate proof of unchanged full-bootstrap admission; this test is not a full-stack product boot.

The controlled non-streaming HTTP fixture returns 503, 503, then a valid 200 completion if three calls are made.

| Bootstrap | Required observation |
|---|---|
| Production, `max_attempts: 3` | Three total HTTP calls; canonical upstream `*agentic.AgentResponse`, matching request identity and response subject, expected successful content, and `RetryCount: 2` |
| Mock, `max_attempts: 1` | One HTTP call; canonical upstream typed error response correlated to the same request and expected response subject |

Establish the response consumer and observe the configured component's running status before publishing a canonical
upstream request. Use explicit readiness, completion and shutdown synchronization, with bounded contexts rather than
sleeps. Stop and join owned component/fixture work before asserting final HTTP call counts, and clean up owned NATS
resources on every exit path. Decode with the real product payload registry; JSON shape alone is insufficient proof
of concrete type compatibility. Do not assert exactly one response publication: the upstream publish path is at least
once, and this test qualifies correlated content and provider-attempt behavior rather than delivery deduplication.

## Regression evidence and acceptance

Temporarily lower the production retry budget and prove the new behavioral test fails to observe the required
successful three-attempt outcome. Temporarily remove model factory registration and prove the test detects the
missing configured component through the real composition path. Preserve exact failing commands/assertions, restore
both mutations, then qualify the unmodified runtime/config baseline with the new tests.

Run relevant integration/race tests, lint/build, unchanged generated-interface checks and OpenSpec validation. Record
source/dependency identities, fixture/NATS setup, commands, observed outcomes and independent Go review. A successful
local fixture run does not qualify provider reliability, streaming, process restart, retained state, external-effect
durability or exactly-once delivery. No runtime implementation, shared policy, dependency pin or production config is
changed; existing product limits and the development-pack boundary remain intact.

Qualification record: [configured model evidence](../../../../docs/migrations/model-runtime-qualification/README.md).
