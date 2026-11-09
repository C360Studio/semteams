## 1. Fixture Design & Guardrails

- [x] 1.1 Record `C360Studio/semsource#228` / `C360Studio/semengine` coordination check and per-file usage inventory in `openspec/changes/natstest-fixture/notes/fixture.md`, and add unit + contract guard tests for `internal/natstest` (`nats-image` digest validation, no production imports of `internal/natstest`, and zero `natsclient.NewTestClient` / `natsclient.TestClient` sites across repository `.go` files) with `task mutate:check` proof.

## 2. Implementation & Test Migration

- [x] 2.1 Implement `internal/natstest` (`nats-image` pinned to `nats:2.15.0-alpine@sha256:ac8f88a6494bffc2c2a5289a0ca61cb28a9145c11ba5677cf24265d07f46d8d4`, `testcontainers-go` dynamic port startup with JetStream readiness, `*natsclient.Client` connection, `CreateKVBucket` / stream helpers, and bounded `context.WithTimeout` `t.Cleanup`) and promote direct `go.mod` dependencies (`testcontainers-go`) cleanly via `go mod tidy`.
- [x] 2.2 Migrate all 11 `_test.go` files (20 `NewTestClient` call sites + 1 `*natsclient.TestClient` parameter) in `cmd/semteams/**` and `test/contract/autoresearch_reply_integration_test.go` onto `internal/natstest`, adding valid `// spec:` citations to every touched test function.

## 3. Ledger Sync & Verification

- [x] 3.1 Update and regenerate `scripts/measure-semengine-port-ledger.go` / `docs/migrations/semengine-port-ledger.md` if `natsclient` test-import counts shift, and verify `task lint` (`cleanup-roots:check`, `lint-test-ports.sh`), `task test:race`, `task test:integration` (`-p 1`), and full `task validate` pass cleanly.
