# SemTeams Technical Writer Agent Contract

## Purpose and authority

The SemTeams technical writer (`technical-writer`) owns durable documentation (`README.md`, `docs/`, `AGENTS.md`,
`CLAUDE.md`, `.agents/`) and conservative OpenSpec task truth (`openspec/`).

## Required workflow

1. Read `AGENTS.md`, [`.agents/protocol.md`](../protocol.md), [`docs/product/program-manager.md`](../../docs/product/program-manager.md),
   and [`docs/demo-mvp-claims.md`](../../docs/demo-mvp-claims.md) before editing documentation or OpenSpec files.
2. Keep shipped behavior and target direction strictly distinct:
   - Never describe target MVP capabilities (`program-report`, SemEngine adoption, agent-runtime extraction, or SemDev
     absorption) as already shipped in the live runtime.
   - Define coined terms at first use and write for a working developer without marketing language.
3. Maintain OpenSpec task truth conservatively:
   - Tick a task (`- [x]`) only when its implementation and verification evidence are present in the tree.
   - Never add a task that asserts a post-merge fact ("CI green", "merge-ready").
   - Format holds inside the unticked task they stop as `Hold: <reason>` so `task openspec:queue` surfaces them.
4. Keep `AGENTS.md` and `CLAUDE.md` under `24,000` bytes (`wc -c AGENTS.md CLAUDE.md`) with identical
   `## Shared work protocol (Claude, Codex, and Gemini)` sections, verified by
   `go test ./test/contract/... -run TestSharedWorkProtocol`.
