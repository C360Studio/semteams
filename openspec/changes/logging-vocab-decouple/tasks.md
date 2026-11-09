## 1. Approval

- [x] 1.1 Owner approves `proposal.md` (`Approved-by: Coby Leuschke (2026-11-08)`) in Enjoy conversation `Pickup Next Milestone 1` (`quick` track, drop `pkg/logging` + `vocabulary/builtins` + `RegisterPredicate`, run through to cross-reviewed merge-ready PR).

## 2. Product-owned logger and explicit boot vocabulary registration

- [ ] 2.1 Replace `semstreams/pkg/logging` in `cmd/semteams/logging.go` with product-owned `log/slog` multi-handler and NATS `logs.<LEVEL>.<source>` log handler types preserving JSON/text stdout output, debug `AddSource`, `source > component > service > "system"` extraction, and `log-forwarder` `exclude_sources` prefix filtering, with unit tests and `task mutate:check` detections (`// spec:` citations).
- [ ] 2.2 Replace `vocabbuiltins.Register()` in `cmd/semteams/main.go` with direct `agvocab.Register()` (`semstreams/vocabulary/agentic`), migrate `cmd/semteams/vocab/vocab.go` from `vocabulary.RegisterPredicate` to `vocabulary.Register(name, opts...)`, and add unit/contract tests verifying `PredicateMetadata` preservation and `vocabulary.RequireDeclaredPredicate` across all 49 bootstrap-wired rules and both flow configs' projection contracts without `vocabulary/rulepacks` (`// spec:` citations + `task mutate:check` detections).

## 3. Predicate audit note, crosswalk sync, and verification

- [ ] 3.1 Record the complete wired-rule + projection-contract predicate ownership inventory and `./cmd/semteams` package closure delta in `openspec/changes/logging-vocab-decouple/notes/audit-and-closure.md`, update Milestone 1 `#401` status notes in `scripts/measure-semengine-port-ledger.go`, and regenerate `docs/migrations/semengine-port-ledger.md` (Group 4: `23` of `23` pruned).
- [ ] 3.2 Run `task spec:check` and read-only `task validate` on a clean worktree before requesting cross-provider `go-reviewer` review.
