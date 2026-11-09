## 1. Approval

- [x] 1.1 Owner approves `proposal.md` (`Approved-by: Coby Leuschke (2026-11-09)`) in Enjoy conversation (`quick` track, full mock-LLM Playwright + boundary matrix scope, concise JSON/Markdown baseline in `docs/migrations/semengine-port-baseline/` + Go contract test, run straight through to cross-reviewed merge-ready PR).

## 2. Empirical acceptance-floor measurement on unmodified `main`

- [x] 2.1 Run and capture exact commit SHA (`e6cd847005833733a53343741a30248c45b25acf`), NATS broker versions and content-addressed digests (`internal/natstest` `2.15.0` vs. E2E compose `2.14.4`), `task validate` lane timings/counts, Go unit/contract `-race` counts (including `#293` rule-family consumer tests), Go `-tags=integration -p 1` counts (including `#289` model retry and `#295` loop first-request/cost/budget suites), and UI Vitest counts.
- [x] 2.2 Run and capture the post-`#399` 9-factory startup composition diagnostic ledger (offline `composition.Analyze` and live Docker boot), cold-start `/teams-dispatch/loops` `503 -> 200` readiness transition while backend `/readyz` is `200`, all 23 `task ui:test:e2e` subtasks, opt-in `autoresearch` + `autoresearch-guardrails`, all 7 `scripts/qualify-migration-browser.py` boundary scenarios, wait-gated API-first rechecks, and parked donor scaffolds (`tool-approval-gate`, `clarification-resume-ui`), filing `#420` for pre-existing `main` E2E harness/spec defects and cleaning up all temporary containers/volumes/images.

## 3. Baseline artifacts, ledger cross-reference, and contract verification

- [x] 3.1 Commit `docs/migrations/semengine-port-baseline/baseline.json` and `docs/migrations/semengine-port-baseline/README.md`, and update `docs/migrations/semengine-port-ledger.md` to cross-reference them as the Milestone 3/4 non-regression floor while preserving `TestSemEnginePortLedger` generated-crosswalk equality.
- [x] 3.2 Add `test/contract/semengine_port_baseline_test.go` (`// spec:` citation) verifying `baseline.json` pins, `internal/natstest` digest parity, live `composition.Analyze` 9-finding parity across `configs/flow-bootstrap.json` and `configs/e2e-flow-bootstrap.json`, and ledger/README cross-references; verify sensitivity via `task mutate:check` (or synthetic drift fixture) and pass `task spec:check` + read-only `task validate`.
