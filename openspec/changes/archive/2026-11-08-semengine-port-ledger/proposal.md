# SemEngine port import crosswalk and runtime-absorption ledger

Approved-by: Coby Leuschke (2026-11-08)

Track: quick. Proposal-only migration inventory and regenerable measurement tool; changes no spec Requirement, boot path, durability behavior, or gate file.

## Why

Epic `#412` sequences SemTeams's migration off the frozen SemStreams pin (`v1.0.0-beta.162.0.20260930150212-8b99efe9c66a`, `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`) onto SemEngine (`C360Studio/semengine`) while SemEngine `setup-04a-06` (`processor/rule`) is in flight. SemTeams currently imports 56 distinct `github.com/c360studio/semstreams/...` package paths across repository `.go` files (58 including `scripts/` measurement tools). Without a regenerable, row-by-row crosswalk mapping each path to SemEngine (`main` or `setup-04a-06`), Milestone 2 absorption into SemTeams under ADR-061, or Milestone 1 pruning (`#399`–`#401`), downstream port work risks missing consumer ADR obligations or filing duplicate upstream asks.

## What changes

- Add `docs/migrations/semengine-port-ledger.md` mapping every SemStreams import path in SemTeams (`production`, `_test.go`, and `scripts/` counts) to its SemEngine disposition (`carry`, `adapt`, `defer-exclude`, or unported), upstream issue links (`semengine#25`–`#30`, `#77`, `#155`, `#361`), and SemTeams action across Milestones 1 and 2.
- Document the five consumer-facing SemEngine ADRs SemTeams must adopt on the Milestone 2 branch: ADR-108 (processor shell), ADR-109 (readiness on replies), ADR-110 (write seam), ADR-112 (`natsclient` consumer ownership), and ADR-113 (`StartAll` context bounds `Start` only).
- Audit every substrate row against `C360Studio/semengine` `main` and open issues, filing upstream SemEngine issues only if a genuine substrate gap exists outside packages already ruled `defer-exclude` in SemEngine D4.
- Add a stdlib-only Go measurement script (`scripts/measure-semengine-port-ledger.go`) with unit tests (`test/contract/semengine_port_ledger_test.go`) so the import counts and category breakdown remain regenerable as `#399`–`#401` land.

## Non-goals

- Changing runtime wiring, `go.mod`, live category packs (`research`, `autoresearch`), or parked dev packs (`create-change`, `proof-readiness`, `dev-from-task`, `dev-via-test`, which remain unwired under ADR-058 and ADR-061).
- Executing the catalog trim (`#399`), NATS test-fixture replacement (`#400`), `pkg/logging` / `vocabulary/builtins` decoupling (`#401`), or Milestone 2 agent-runtime absorption (`#404`–`#407`).
- Building any local execution journal, effect ledger, or substrate shim while `C360Studio/semengine#24` and `setup-04a-06` are in flight.

## Impact

- Gives Milestones 1–3 (`#399`–`#408`) one regenerable source of truth aligned with `C360Studio/semsource#226` and the 2026-11-08 owner ruling on `C360Studio/semengine#9`.

Closes #398; addresses #412 (claimed on branch `gemini/semengine-port-ledger`, PR #415).
