# Replace natsclient.NewTestClient with digest-pinned internal/natstest

Approved-by: Coby Leuschke (2026-11-08)

Track: quick (`skip_specs: true`). Test-harness debt decoupling integration tests from unported `semstreams/natsclient.NewTestClient`; changes no production runtime behavior, spec Requirement, boot path, durability behavior, or gate file.

## Why

SemTeams's integration tests construct NATS brokers via `natsclient.NewTestClient` across 20 call sites in 11 `_test.go` files (10 under `cmd/semteams/**` plus `test/contract/autoresearch_reply_integration_test.go`, which also passes `*natsclient.TestClient` into a helper). Upstream SemEngine (`docs/admission-ledger.yaml`, `openspec/specs/harness-boundaries/spec.md`) did not port `natsclient/test_client.go`, keeps its own `internal/harness/natsfixture` internal with no consumer-importable `pkg/natstest` planned (`C360Studio/semsource#228` faces the same gap), and pins NATS by digest. SemTeams's call sites also drift across `2.14.4`, `2.14.4-alpine`, and unpinned `2.14-alpine`. Replacing `NewTestClient` on `main` now eliminates 11 Milestone 2 compile breaks and pins integration NATS to SemEngine's digest (`nats:2.15.0-alpine@sha256:ac8f88a6494bffc2c2a5289a0ca61cb28a9145c11ba5677cf24265d07f46d8d4`).

## What changes

- Add `internal/natstest` over `nats.go` and `testcontainers-go`, embedding `internal/natstest/nats-image` (`nats:2.15.0-alpine@sha256:ac8f88a6494bffc2c2a5289a0ca61cb28a9145c11ba5677cf24265d07f46d8d4`), starting an isolated JetStream-enabled NATS container on a dynamic mapped port, returning a connected `*natsclient.Client`, broker URL, KV/stream helpers, and bounded `context.WithTimeout` cleanup (`t.Cleanup`).
- Migrate all 20 `natsclient.NewTestClient` call sites and 1 `*natsclient.TestClient` helper signature across all 11 `_test.go` files onto `internal/natstest`, attaching `// spec:` citations to touched tests.
- Add unit and contract guard tests verifying `nats-image` digest parsing, forbidding `natsclient.NewTestClient` / `natsclient.TestClient` and non-test imports of `internal/natstest`, and proving sensitivity via `task mutate:check`.
- Regenerate `docs/migrations/semengine-port-ledger.md` and record coordination findings with `C360Studio/semsource#228` in `openspec/changes/natstest-fixture/notes/fixture.md`.

## Non-goals

- Modifying production runtime code (`cmd/semteams/*.go`, `internal/runtimecatalog`), `go.mod` framework pin (`8b99efe9`), or Docker Compose files (`ui/docker-compose.*.yml`).
- Wiring parked development packs (`create-change`, `proof-readiness`, `dev-from-task`, `dev-via-test`).

## Impact

- Leaves Milestone 2 with only mechanical `semstreams/natsclient` → `semengine/natsclient` import path rewrites across integration tests while unifying NATS integration runs onto SemEngine's content-addressed `2.15.0-alpine` image.

Closes #400; addresses #412.
