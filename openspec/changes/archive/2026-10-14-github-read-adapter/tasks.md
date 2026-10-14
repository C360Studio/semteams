# Tasks: `github-read-adapter` (#273)

- [x] 1. Record the framework-alignment review addendum in `docs/adr/060-program-manager-direction-and-research-design.md` and register `github_read` in `cmd/semteams/tools/README.md`.
- [x] 2. Implement the Go portfolio reader and validator in `cmd/semteams/portfolio/` matching `configs/portfolio/portfolio.schema.json`.
- [x] 3. Implement the shared `research.EvidenceRecord` schema, origin-ID normalizer, and validator in `cmd/semteams/research/evidence.go` with unit tests.
- [x] 4. Implement the read-only `github_read` executor, GitHub REST client, and cursor store in `cmd/semteams/tools/githubread/` and wire registration in `cmd/semteams/product_tools.go`.
- [x] 5. Add contract tests in `test/contract/github_read_contract_test.go` verifying evidence-record validation, portfolio allowlist enforcement, and second-run cursor suppression.
