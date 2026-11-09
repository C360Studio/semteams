# Developer Notes: `internal/natstest` Fixture & Migration (`#400`)

## 1. Upstream Coordination Check (`C360Studio/semsource#228` & `C360Studio/semengine`)

- **`C360Studio/semengine` admission & harness boundaries:**
  - `docs/admission-ledger.yaml:32-95`: `natsclient/test_client.go` at `8b99efe9c66a4faa4fa509f9f62cc6bad8392128` is disposition `adapt` into SemEngine's private `internal/harness/natsfixture` (without `natsclient.Client`), while `natsclient/test_options.go`, `natsclient/test_client_factory_test.go`, and `natsclient/test_client_integration_test.go` are `defer-exclude` per the owner ruling on `C360Studio/semengine#9` (`comment 5941920346`, Q3).
  - `openspec/specs/harness-boundaries/spec.md:12-64`: SemEngine keeps `internal/harness/natsfixture` internal and forbids importing ported runtime packages (`natsclient`) inside `internal/harness/natsfixture`. No consumer-importable `pkg/natstest` package is planned.
  - `internal/harness/natsfixture/nats-image:1-6`: SemEngine pins integration NATS by multi-arch manifest digest to `nats:2.15.0-alpine@sha256:ac8f88a6494bffc2c2a5289a0ca61cb28a9145c11ba5677cf24265d07f46d8d4` (`NATS_SERVER=2.15.0`).
- **`C360Studio/semsource#228`:**
  - Tracks the identical consumer gap across 38 `_test.go` files (48 `NewTestClient` sites + 3 `TestClient` uses) and specifies building a consumer-local `internal/natstest` over `nats.go` + `testcontainers-go` pinned to SemEngine's digest when SemEngine provides no `pkg/natstest`.
- **SemTeams resolution (`#400`):**
  - SemTeams owns `internal/natstest` locally, embedding `internal/natstest/nats-image` (`nats:2.15.0-alpine@sha256:ac8f88a6494bffc2c2a5289a0ca61cb28a9145c11ba5677cf24265d07f46d8d4`), returning `*natstest.Broker` (`Client *natsclient.Client`, `URL string`, `CreateKVBucket`, `CreateStream`), and keeping all `natsclient` test-broker construction in one file (`internal/natstest/natstest.go`) whose `natsclient` methods (`NewClient`, `WithTimeout`, `WithMaxReconnects`, `WithHealthInterval`, `Connect`, `WaitForConnection`, `Close`, `CreateKeyValueBucket`, `EnsureStream`) match `C360Studio/semengine/natsclient` 1-for-1 for Milestone 2 (`#407`).

## 2. Per-File Inventory (11 `_test.go` Files: 20 `NewTestClient` Call Sites + 1 `*TestClient` Helper)

Note on file count: issue `#400`'s title read `"20 natsclient.NewTestClient sites in 10 files"`, while the issue body's bullet list enumerated **11** `_test.go` files (`10` under `cmd/semteams/**` plus `test/contract/autoresearch_reply_integration_test.go`). All 11 files (20 `NewTestClient` call sites and 1 `*natsclient.TestClient` parameter) are migrated:

| # | File | Prior `NewTestClient` / `TestClient` usage | Prior NATS image pin | Fixture surface used | Post-`#400` `natsclient` import |
| --- | --- | --- | --- | --- | --- |
| 1 | `cmd/semteams/approvalpause/projection_integration_test.go` | 1 `NewTestClient` | `2.14.4` | `Client` | Removed |
| 2 | `cmd/semteams/chain/entity_reader_integration_test.go` | 1 `NewTestClient` | default (`2.14-alpine`) | `Client` | Removed |
| 3 | `cmd/semteams/chainpause/decision_handler_integration_test.go` | 2 `NewTestClient` | default (`2.14-alpine`) | `Client` | Removed |
| 4 | `cmd/semteams/loop_cost_stamp_integration_test.go` | 1 `NewTestClient` (`WithJetStream`, `WithKV`) | `2.14.4-alpine` | `Client`, `URL` | Removed |
| 5 | `cmd/semteams/loop_runtime_integration_test.go` | 1 `NewTestClient` (`WithJetStream`, `WithKV`) | `2.14.4-alpine` | `Client`, `URL` | Removed |
| 6 | `cmd/semteams/model_runtime_integration_test.go` | 1 `NewTestClient` (`WithJetStream`, `WithKV`) | `2.14.4-alpine` | `Client`, `URL` | Removed |
| 7 | `cmd/semteams/platform_identity_integration_test.go` | 1 `NewTestClient` (`WithJetStream`, `WithKV`) | default (`2.14-alpine`) | `Client`, `URL` | Removed |
| 8 | `cmd/semteams/research_loop_budget_integration_test.go` | 3 `NewTestClient` (`WithJetStream`, `WithKV`) | `2.14.4-alpine` | `Client`, `URL` | Removed |
| 9 | `cmd/semteams/rule_config_service_integration_test.go` | 1 `NewTestClient` (`WithJetStream`, `WithKV`) | default (`2.14-alpine`) | `Client`, `URL` | Removed |
| 10 | `cmd/semteams/rule_hot_reload_integration_test.go` | 1 `NewTestClient` (`WithJetStream`, `WithKV`) | default (`2.14-alpine`) | `Client`, `URL` | Kept (`*natsclient.Client` in `startedRuleProcessor`) |
| 11 | `test/contract/autoresearch_reply_integration_test.go` | 7 `NewTestClient` (`WithJetStream`) + 1 `tc *natsclient.TestClient` at `newReplyHarness` | `2.14.4` | `CreateKVBucket` | Removed |

## 3. Non-Destructive Mutation Evidence (`task mutate:check` & `-overlay`)

Mutant copies were written outside the repository under `/tmp/semteams-mutate-400/` (local temporary path, removed after run):

1. **Digest-pin enforcement (`internal/natstest/natstest.go` → `TestPinnedImage`):**
   - Plausible wrong behavior: `pinnedImage` skips `digestImage.MatchString` and accepts mutable tag-only or non-`nats` image strings.
   - Command:
     ```sh
     task mutate:check -- -pkg ./internal/natstest -test TestPinnedImage -file internal/natstest/natstest.go -mutant /tmp/semteams-mutate-400/natstest_mutant.go -expect-text "expected error containing"
     ```
   - Result: `verdict: detection (each of the 1 mutant run(s) failed TestPinnedImage with an expected assertion, and the after-run passed)` (`natstest_test.go:105`).
2. **Option bounds enforcement (`internal/natstest/natstest.go` → `TestValidateOptions`):**
   - Plausible wrong behavior: `validateOptions` accepts non-positive `WithMaxPayload(0)` before Docker startup.
   - Command:
     ```sh
     task mutate:check -- -pkg ./internal/natstest -test TestValidateOptions -file internal/natstest/natstest.go -mutant /tmp/semteams-mutate-400/natstest_options_mutant.go -expect-text "want non-positive payload error"
     ```
   - Result: `verdict: detection (each of the 1 mutant run(s) failed TestValidateOptions with an expected assertion, and the after-run passed)` (`natstest_test.go:127`).
3. **Contract AST boundary guard (`test/contract/natstest_boundary_test.go` → `TestNATSTestBoundary`):**
   - Note: `ateam mutate` refuses `-file` targeting `*_test.go` files (`-file must be a non-test .go source file`), so non-destructive `go test -overlay` was run with `/tmp/semteams-mutate-400/overlay.json` setting `"NewTestClient": false` in `forbiddenNATSClientSymbols`.
   - Result: `detection` — mutant failed `TestNATSTestBoundary/sensitivity_catches_testclient_references_across_default_aliased_and_dot_imports` at `natstest_boundary_test.go:210` (`expected 5 forbidden TestClient violations, got 2`), and the clean after-run passed (`ok github.com/c360studio/semteams/test/contract`).
