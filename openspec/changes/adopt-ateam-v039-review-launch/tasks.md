## 1. Approval

- [x] 1.1 Owner approves `proposal.md` and `design.md` (`Approved-by: Coby Leuschke (2026-11-08)`), ruling on Open Questions 1–2 in Enjoy conversation `Semengine migration plan and issues` ("File the issue in semengine-port 1, move to a worktree, and open + implement the PR now" and "Ship .agents/hosts/enjoy.toml only (matching semsource)").

## 2. Repo-local `.tools/bin` install and `scripts/ateam` shim

- [ ] 2.1 Ignore `.tools/` in `.gitignore` (`:91-95`), add `ATEAM_VERSION: v0.3.9`, `ATEAM_BIN_DIR`, `ATEAM`, `env.PATH`, and `tools:ateam` to `Taskfile.yml`, and update `scripts/doctor.sh` (`:62-67`) and `scripts/validate.sh` (`:51-70`) to prepend existing `.tools/bin` directories while keeping `validate.sh` read-only. Acceptance: `task tools:ateam` installs `v0.3.9` into `<main-checkout>/.tools/bin/ateam` and `task doctor` prints `ok    ateam           v0.3.9`.
- [ ] 2.2 Add `scripts/ateam` (`chmod +x`) and wire all 7 hook/MCP configs (`.mcp.json`, `.agents/plugins/ateam/mcp_config.json`, `.agents/hooks.json`, `.claude/settings.json`, `.codex/config.toml`, `.codex/hooks.json`, `.gemini/settings.json`). Add contract test `TestAteamShimResolution` (`test/contract/ateam_shim_test.go`) with paired sensitivity/mutation proof and `// spec:` citation. Acceptance: `go test ./test/contract/... -run TestAteamShimResolution` passes and fails when resolution order is broken.

## 3. Pin `ateam@v0.3.9` and enable Enjoy agent review launch

- [ ] 3.1 Bump `v0.3.5` to `v0.3.9` in `.github/workflows/ci.yml` (`:40,135`), `.github/workflows/merge-check.yml` (`:3,55,102`), `test/contract/workflow_gate_test.go` (`:19`), and `.agents/rules-index.md` (`:48,49,63`). Acceptance: `go test ./test/contract/... -run 'TestMergeCheckWorkflow|TestCIWorkflowGate'` passes.
- [ ] 3.2 Set `"review_launch": "agent"` in `.agents/review-policy.json`, add `v0.3.9`'s canonical `.agents/hosts/enjoy.toml`, sync `.agents/skills/handoff/SKILL.md` and `.agents/skills/review-pr/SKILL.md` to `v0.3.9`, and update `.agents/protocol.md` (`:150-154`) for `ateam review request` / `ateam review notify`. Acceptance: `task ateam:doctor` (`v0.3.9`) reports zero violations and `scripts/ateam review request --dry-run <pr>` prints cross-provider reviewer runs + Enjoy launch instructions.
- [ ] 3.3 Sync `repository-governance` spec delta and verify full local parity. Acceptance: `task spec:check` and `task validate` pass on a clean worktree.
