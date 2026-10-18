# Configured model-boundary qualification

Issue [#288](https://github.com/C360Studio/semteams/issues/288), draft
[PR #289](https://github.com/C360Studio/semteams/pull/289), based on foundation #283 at SemTeams
`042193463e3b27fb1996ed4e99bf2b0cd4024a06`. SemStreams remains frozen at
`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`
(`v1.0.0-beta.162.0.20260930150212-8b99efe9c66a`).

## Result and boundary

Both configured-model cases pass under the race detector: the `flow-bootstrap.json` ("production") retry config makes
three total HTTP attempts for 503, 503, 200, then emits the canonical upstream `AgentResponse` with matching
`RequestID`/response subject, expected content and `RetryCount: 2`; the `e2e-flow-bootstrap.json` ("mock") retry config
makes one attempt and emits the correlated canonical typed error. The [focused run](focused-race.txt) and
[full integration run](integration-race.txt) retain the actual outputs.

The test uses product `setupRegistriesAndManager`, `buildPayloadRegistry` and the public component manager. It derives
`agentic-model` component configuration from each shipped bootstrap, asserts the request input is required and
non-external, and changes only that input's `External` marker in the test copy. Reversing the marker reproduces the
original component JSON. Subjects, requiredness, stream, retry settings and output fields on the `agentic-model`
component remain intact. Response consumption and running state precede publication; component stop and HTTP shutdown
join before final provider-call assertions.

Two scope limits apply to the `"production"` subtest (`loadModelBoundaryConfig`, lines 202–207):

1. **Provider replacement:** `loadModelBoundaryConfig` replaces the bootstrap `model_registry` with a single local
   OpenAI non-streaming HTTP endpoint (`Provider: "openai"`, `Stream: false`). Shipped `flow-bootstrap.json` routes
   Gemini through the wire backend plus streaming Anthropic. Because all three backends share `Client.runRetryLoop`,
   this fixture proves the shipped retry budget in `runRetryLoop`, not how the Gemini wire path or streaming Anthropic
   adapter classifies a 503 response.
2. **Production retry count equals component default:** `flow-bootstrap.json` declares `retry.max_attempts = 3`, which
   matches `agentic-model`'s zero-value fallback (`MaxAttempts == 0 → 3`). Deleting the `retry` block from
   `flow-bootstrap.json` alone would still produce 3 attempts; the `e2e-flow-bootstrap.json` subtest (`max_attempts = 1`)
   is the case that proves the component's `retry` configuration is read rather than defaulted.

The fixture uses isolated real NATS with image tag `nats:2.14.4-alpine` and a local non-streaming HTTP endpoint. This
qualifies an isolated composed model with explicit external fixture input, not a full product boot or unchanged
full-bootstrap admission. Existing composition tests retain that separate proof. Shipped configs, runtime code,
dependency pins and generated interfaces remain unchanged.

The [initial admission failure](initial-composition-rejection.txt) reports `orphaned_port` / `no_publishers`: the
fixture publisher sits outside the component graph. Independent architecture review approved the public external
marker (`component/ports.go:53–64`, `composition/analyze.go:71–76`) instead of a fake publisher component. That rejected
development run remains distinct from the accepted results.

## Regression and verification evidence

Both controlled mutations failed their intended assertions and were restored before the successful unmutated run:

| Mutation | Observed failure |
|---|---|
| [Reduce production retry budget](reduced-retry-budget.txt) | `shipped retry setting must govern total provider attempts` |
| [Remove model factory](missing-model-factory.txt) | `configured model was not admitted by the product registry` |

[Mutation commands and restored hashes](mutations.json) record those failures. [Qualification metadata](qualification.json)
records source/evidence hashes, Go `1.26.4 darwin/arm64`, NATS image tag and passing commands:

- focused configured-model integration with `-race -count=1`;
- `task test:integration` and `task test:race`;
- `task lint` (six inherited warnings, zero errors);
- `go build ./...` and integration-tag vet;
- `task schema:generate` and `task schema:check-changes`.

Default-tag tests do not exercise the new integration-tagged test; its focused and full integration logs provide that
proof. Additional command logs remain under `/tmp/semteams-model-runtime-qualification/`. The qualification record
also retains cleanup verification: all six recorded fixture containers were removed; unrelated pre-existing
containers were left alone.

Independent architecture plan review and Go implementation review passed. The reviewed integration-test SHA-256 is
`1877e71807a80372d4bfdae74b16b7348e54c0f53feea1dc1ab4c9a0b349945d`; config, catalog and module hashes are in the
qualification record. The [archived change](../../../openspec/changes/archive/2026-10-03-model-runtime-qualification/)
synchronizes only the tested qualification requirement. Strict OpenSpec validation passed after archive/spec sync.
Independent final archive/spec/evidence review passed with no remaining findings. Strict post-archive OpenSpec validation passed all four living specifications; queue reporter checks passed all 26 fixtures and the active queue is empty.

## Reproduction

From this checkout with Docker available:

```sh
TESTCONTAINERS_RYUK_DISABLED=true go test -race -count=1 -tags=integration ./cmd/semteams -run '^TestConfiguredModelRetryBoundary$' -v
```

Run the two mutations separately using the recorded commands, preserve their failing assertions, restore both, then
accept only a fresh successful unmutated run. The test owns and cleans up its isolated fixture resources. No paid
credentials or live provider endpoint is required.

## Limits

This fixture does not establish streaming, live-provider reliability or quality, process-restart recovery,
retained-state migration, exactly-once response publication, new durability or SemEngine compatibility. Upstream
publication remains at least once. No shared types, sentinels, rollback policy or delivery-lane helper are copied;
#287 remains the deferred shared rollback integration. No UI capability, SemDev activation or parked-pack revival is
introduced.
