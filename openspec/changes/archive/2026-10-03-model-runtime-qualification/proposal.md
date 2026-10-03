# Qualify the configured model retry boundary

## Why

[Issue #288](https://github.com/C360Studio/semteams/issues/288) owns qualification of the existing model boundary
through SemTeams composition. The production bootstrap permits three total HTTP attempts, while the deterministic
mock bootstrap permits one and explicitly defers retry-path coverage. Current catalog and browser evidence does not
prove that production retry behavior through the product boundary.

This change starts from foundation #283 at SemTeams `042193463e3b27fb1996ed4e99bf2b0cd4024a06`, with frozen SemStreams
`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`. [Draft PR #289](https://github.com/C360Studio/semteams/pull/289), on
`codex/model-boundary-qualification`, owns the claim. Extraction-design PR #286 remains a separate review unit.

## What changes

- Add an integration-tagged consumer test using actual product component and payload setup plus the public component
  manager, bootstrap-derived model configuration, isolated real NATS and a local non-streaming HTTP fixture. Mark only
  the test copy's required request input as external for the fixture; retain retry settings and all other port fields.
- Prove three total production attempts for HTTP 503, 503, 200, followed by a concrete canonical `AgentResponse` with
  matching request identity/response subject, successful content and `RetryCount: 2`. Prove the mock configuration
  stops after one failed attempt and publishes the correlated typed error response.
- Synchronize response consumption and running state before publishing; bound waits and stop/join owned work before
  asserting final HTTP counts. Preserve meaningful failing mutations for a reduced production retry budget and a
  missing model factory, then restore source before qualification.
- Record exact provenance and test limits; run relevant race/integration, lint/build, generated-interface and OpenSpec
  checks, with independent architecture and Go review.

## Non-goals

No production behavior/config change, runtime extraction, copied shared types/sentinels/rollback/lane policy, SemEngine
API or new durability promise. This isolated composed model with an explicitly external fixture input does not
re-prove unchanged full-bootstrap admission. It also does not qualify restarts, exactly-once response publication,
streaming or live-provider quality. No paid runs, UI change, SemDev activation, parked-pack revival or
retained-state conversion. #287 continues to track the deferred shared rollback integration.

## Impact

Integration tests, their qualification evidence and a test-only `runtime-composition` requirement. Runtime
implementations, dependency pins, shipped configurations and generated interfaces remain unchanged. No release
milestone membership or issue closure is implied; closure requires owner `CONFIRM-CLOSE`.
