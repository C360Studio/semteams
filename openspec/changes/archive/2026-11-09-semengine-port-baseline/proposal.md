# Record post-v0.3.0 SemEngine port acceptance floor at frozen 8b99efe9

Approved-by: Coby Leuschke (2026-11-09)

Track: quick. Updates one existing Requirement (`Frozen framework adoption preserves qualified product behavior`) in `openspec/specs/framework-adoption/spec.md` to record the post-`v0.3.0` Milestone 1 SemEngine port acceptance floor (`#403`); adds no new Requirement, exported surface, boot/durability change, or gate-file edit.

## Why

`docs/migrations/semstreams-8b99efe/` captured SemTeams's pre-`v0.2.0` October 1 qualification snapshot (`edf87d31`). Since then, `main` shipped `/work`, scheduled Program Pulse (`/pulse`), research containment budgets (`#272`), per-loop cost accounting, `ateam` governance gates, runtime boundary suites (`#289`, `#293`, `#295`), and Milestone 1 preparation (`#398`–`#402`, including the 9-factory `internal/runtimecatalog` cut and `internal/natstest` `2.15.0` digest pin). Milestones 3 (`#408`) and 4 need an honest, reproducible acceptance floor measured on current `main` (`e6cd847005833733a53343741a30248c45b25acf`) so post-`v0.2.0` features are protected and pre-existing E2E harness/spec defects (`#420`) are never misattributed to SemEngine.

## What changes

- Record concise machine-readable and human-readable acceptance-floor artifacts in `docs/migrations/semengine-port-baseline/{baseline.json,README.md}` measured on unmodified `main` (`e6cd847005833733a53343741a30248c45b25acf`, SemStreams `v1.0.0-beta.162.0.20260930150212-8b99efe9c66a`) covering `task validate`, `task test:integration` (`-p 1`, including `#289`, `#293`, `#295`), UI Vitest, NATS broker versions/digests (`2.15.0` Go `internal/natstest` vs. `2.14.4` E2E compose), the 9-warning startup composition diagnostic ledger after the 9-factory catalog cut, cold-start `/teams-dispatch/loops` `503 -> 200` readiness behavior, and the mock-LLM Playwright E2E + boundary matrix (explicitly attributing `#420` harness/spec defects and parked donor exclusions).
- Reference `docs/migrations/semengine-port-baseline/` from `docs/migrations/semengine-port-ledger.md` as the authoritative Milestone 3/4 non-regression floor without mutating the generated 56-path crosswalk block.
- Add Go contract test `test/contract/semengine_port_baseline_test.go` verifying `baseline.json` schema/pin invariants, live 9-finding `composition.Analyze` parity across both shipped bootstrap configs, `internal/natstest` image parity, and `README.md` / `semengine-port-ledger.md` cross-references, proven with `task mutate:check`.

## Non-goals

- Modifying `ui/e2e/**`, `ui/Taskfile.yml`, `configs/**`, or runtime Go packages (pre-existing E2E harness/spec defects on `main` are filed separately in `#420` so `#403` measures `main` unaltered).
- Running paid real-LLM `smoke*` tasks or starting Milestone 2 SemEngine code changes (`#404`–`#407`, `#287`).

## Impact

- Completes Milestone 1 (`semengine-port 1: ready before the pin`) and establishes the exact non-regression floor gating Milestone 3 (`#408`).

Closes #403; addresses #412.
