# Decouple cmd/semteams from pkg/logging and vocabulary/builtins

Approved-by: Coby Leuschke (2026-11-08)

Track: quick. Updates one existing Requirement (`Product-owned registration boundary`) in `openspec/specs/runtime-composition/spec.md` to cover explicit boot vocabulary registration (`vocabulary/agentic.Register()` + `cmd/semteams/vocab.Register()` via `vocabulary.Register`) and product-owned `log/slog` + NATS logger composition without `pkg/logging` or `vocabulary/builtins` (`#401`); adds no new Requirement, exported surface, durability change, or gate-file edit.

## Why

`cmd/semteams/logging.go` imports `"github.com/c360studio/semstreams/pkg/logging"` (`NewNATSLogHandler`, `NewMultiHandler`), which SemEngine cuts (`C360Studio/semengine#8`). `cmd/semteams/main.go` imports `"github.com/c360studio/semstreams/vocabulary/builtins"` (`vocabbuiltins.Register()`), which SemEngine drops (`C360Studio/semengine#3`), and `cmd/semteams/vocab/vocab.go` calls `vocabulary.RegisterPredicate(meta)`, which SemEngine removes in favor of `vocabulary.Register(name, opts...)` (`C360Studio/semengine#72`).

In frozen SemStreams (`8b99efe9c66a`), `vocabbuiltins.Register()` calls only `agentic.Register()` and `rulepacks.Register()`, and zero of `rulepacks`'s 13 legacy predicates appear in SemTeams's 49 wired rules or projection contracts. Decoupling both helpers and migrating `RegisterPredicate` now brings Group 4 of `docs/migrations/semengine-port-ledger.md` to 23/23 pruned (`0` active imports) on `main`.

## What changes

- Replace `semstreams/pkg/logging` in `cmd/semteams/logging.go` with Go stdlib `slog.NewMultiHandler` and a product-owned NATS JetStream `logs.<LEVEL>.<source>` handler preserving JSON/text stdout formatting, `AddSource` on debug, `source > component > service > "system"` precedence, and `log-forwarder` `exclude_sources` prefix filtering.
- Replace `vocabbuiltins.Register()` in `cmd/semteams/main.go` with direct `agvocab.Register()` (`semstreams/vocabulary/agentic`), and replace `vocabulary.RegisterPredicate(meta)` in `cmd/semteams/vocab/vocab.go` with `vocabulary.Register(name, vocabulary.WithDescription(...), vocabulary.WithDataType(...))`.
- Document the exact predicate inventory across SemTeams's 49 bootstrap-wired rules and both flow configs' projection contracts (SemEngine `vocabulary` core vs. `agentic/agentrun` vs. `vocabulary/agentic` vs. `cmd/semteams/vocab`, and 0 hits on `vocabulary/rulepacks`) and add logger + `RequireDeclaredPredicate` contract tests with `task mutate:check` detections.
- Update `scripts/measure-semengine-port-ledger.go` Milestone 1 status notes and regenerate `docs/migrations/semengine-port-ledger.md` (Group 4: `23` of `23` pruned).

## Non-goals

- Changing runtime log wire subjects (`logs.<LEVEL>.<source>`), JSON entry fields, `configs/flow-bootstrap.json`, `configs/e2e-flow-bootstrap.json`, or any rule/persona pack.
- Absorbing `vocabulary/agentic` or `agentic/agentrun` into SemTeams packages on `main` (tracked under `#402` / `#404`).

## Impact

- Prunes the final 2 Group 4 SemStreams imports (`pkg/logging`, `vocabulary/builtins`) and `vocabulary/rulepacks` transitive closure from `cmd/semteams`, leaving `0` active Group 4 imports ahead of `#402` and `#403`.

Closes #401; addresses #412.
