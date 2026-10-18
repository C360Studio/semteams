# SemTeams rule index

The full enforcement audit matrix behind the domain-grouped summary in `AGENTS.md`. Each numbered section below has
the same title and the same number of rows, in the same order, as its section in `AGENTS.md`;
`TestAgentSetupParity` (`test/contract/shared_work_protocol_contract_test.go`) checks the titles and row counts, and
the one-to-one meaning of each row is review only. "Enforced by" names the command, test, or contract section that
catches a break; "Gaps" names what stays review only. A rule whose only enforcement is review says so: a planned check
is listed as a gap until it exists.

## 1. Go, composition root, and runtime ownership

| Rule | Enforced by | Gaps |
| --- | --- | --- |
| `cmd/semteams/` independently wires Pattern-A/B/C primitives per ADR-029; no imports from `cmd/semstreams/` | `go build ./...`; contract tests under `test/contract/`; review: `go-developer`, `go-reviewer` | No dedicated import-list guard for `cmd/semstreams/` beyond Go module visibility and review |
| Rule key family registers before config startup; rule-config service starts after components in `StartAll` and stops before them; initial rule reconciliation runs inside the readiness barrier; `StopAll` uses a fresh bounded shutdown context | Contract and integration tests (`test/contract/rule_family_consumer_test.go`); review: `go-developer`, `go-reviewer` | Exact call-order mutations inside `cmd/semteams/main.go` rely on contract tests and `go-reviewer` |
| Canonical IDs are 6 parts (`org.platform.system.domain.type.instance`); never reconstruct the platform segment from configured base alone — read effective platform ID after config starts | Review: `go-developer`, `go-reviewer`; ADR-029; #308 | Review only outside tested shell writers |
| `internal/runtimecatalog` explicitly registers the frozen component and payload surface; importing a package registers nothing, and `task schema:generate` keeps `schemas/` and `specs/openapi.v3.yaml` drift-free | `task validate` (`cmd/openapi-generator` diff against tracked `schemas/` and `specs/openapi.v3.yaml`); `test/contract/` | `init()` registration check is review only |
| Before adding a tool in `cmd/semteams/tools/`, rule action shape, local payload type, KV bucket, or stream, complete a framework-alignment review (`cmd/semteams/tools/README.md`) and classify as substrate (SemEngine), agent runtime (SemTeams), or pack | Review: `architect`, `go-developer`, `go-reviewer`; ADR-061 | Review only |
| Never build a competing generic execution journal, effect ledger, or workflow engine while SemEngine #24 durability contracts are pending | Review: `architect`, `go-developer`, `go-reviewer`; ADR-061 | Review only |
| Production structs never retain `context.Context`; failure paths fail closed and any skip, drop, or degrade is declared and tested | Review: `go-developer`, `go-reviewer` | No AST stored-context guard yet (SemEngine `TestNoStoredContext` is the model) |
| Pinned `revive` (`revive.toml`), worktree-safe `gofmt`, `go mod tidy -diff`, and `go vet` pass with zero errors | `task lint` (in `task validate` and `Repository CI`) | `revive.toml` advisory rules (`function-length`, `unused-parameter`) emit warnings without failing |

## 2. Packs, rules, predicates, and personas

| Rule | Enforced by | Gaps |
| --- | --- | --- |
| All predicates in live configs and rules follow the 3-segment lower-kebab canonical contract (`domain.category.property`); fail closed with no alias mode | `test/contract/predicate_contract_test.go` (`TestPredicateContract` in `task test:race` and `task validate`) | Dynamic predicates constructed outside tracked config/code literals are review only |
| Parked dev-side packs (`create-change`, `proof-readiness`, `dev-from-task`, `dev-via-test`) remain unwired in `configs/flow-bootstrap.json` and `configs/e2e-flow-bootstrap.json` until qualified under ADR-061 | `test/contract/predicate_contract_test.go`; `test/contract/config_dispatch_test.go`; review: `architect`, `go-developer`, `go-reviewer` | None for tracked bootstrap configs |
| New prompt classes are added as category rule packs (`configs/rules/<category>/`) + persona bundles (`configs/personas/fragments/<role>-<category>-<phase?>/`) + coordinator `decide(action=<category>)`, not new runtime components; shared coordinator personas stay domain-neutral | `test/contract/persona_fragment_loading_test.go`; `test/contract/config_dispatch_test.go`; review: `architect`, `go-developer`, `go-reviewer` | Domain-neutral prose quality in shared coordinator fragments is review only |
| Every `publish_agent` rule action sets an explicit `loop_max_iterations` ceiling and respects the `research` `1..4` fan-out / loop containment budget (#272) | `test/contract/research_pack_test.go`; review: `go-developer`, `go-reviewer` | Review for new pack rule files not yet covered by pack-specific contract tests |
| Rule cadence is set by trigger scope, never expression-rule `fire_every_n_events` (semstreams#1007) or `cooldown` | `test/contract/ops_agent_pack_test.go`; `test/contract/research_pack_test.go`; review: `go-developer`, `go-reviewer` | Review for new rule files |
| `configs/e2e-flow-bootstrap.json` stays a structural clone of `configs/flow-bootstrap.json` with `mock-llm` and disabled compaction | `test/contract/config_dispatch_test.go` (`TestConfigDispatch`) | None |

## 3. Svelte UI and observable work

| Rule | Enforced by | Gaps |
| --- | --- | --- |
| Browser reads of portfolio and work state go through scoped SvelteKit server routes (`/api/work/*`), never direct browser NATS connections or admin credentials | Vitest route/store tests under `ui/src/`; review: `svelte-developer`, `svelte-reviewer` | No AST import guard in `ui/src/routes/` yet |
| Graph-derived overlays carry explicit `known \| none \| unknown` states; never collapse a failed, partial, or unsupported lookup into `none` or simulate unshipped pulse/attention state | Vitest unit tests (`ui/src/lib/`); `observable-work` spec; review: `svelte-developer`, `svelte-reviewer` | Semantic UI rendering edge cases are review + E2E |
| Svelte 5 runes (`$state`, `$derived`, `$effect`, `$props`) avoid subscription/SSE reconnect loops and preserve keyboard focus order, `aria-busy`/`aria-disabled`, and WCAG 2.5.3 compliance | `task ui:lint`, `task ui:check`, `task ui:test`; review: `svelte-developer`, `svelte-reviewer` | Reconnect-loop and focus-restoration subtleties are review + component/E2E tests |
| Frontend `npm run lint`, `npm run check`, `npm run test:unit`, `npm run generate-types:check`, and `npm run build` pass without overwriting tracked generated types | `task validate` (`scripts/validate.sh`); `test/contract/validate_task_contract_test.go` | None |

## 4. Testing, E2E, and verification

| Rule | Enforced by | Gaps |
| --- | --- | --- |
| `task validate` (and `task verify`) runs the read-only local parity gate (Go, UI, Governance) without mutating tracked, staged, unstaged, or untracked worktree files | `scripts/validate.sh`, `scripts/verify.sh`; `test/contract/validate_task_contract_test.go` | None |
| Tests use an independent oracle and are shown able to fail; structural guards carry paired sensitivity tests | `TestAgentSetupParitySensitivity`, `TestCleanupRootsCheckSensitivity`, `TestSpecCheckWiring`, `scripts/*_fixture_test.sh`; review for product tests | Product-test oracle independence is review only |
| Tests bind ephemeral TCP ports (`net.Listen("tcp", "127.0.0.1:0")`) and bound `.Stop`/`.Close`/`.Terminate` cleanup with `context.WithTimeout`, never `context.Background()`/`context.TODO()` | `task lint` (`scripts/lint-test-ports.sh`, `scripts/lint-test-ports_fixture_test.sh`, `scripts/cleanup-roots-check.sh`); `test/contract/cleanuproots_test.go` | Multi-line or indirect port/cleanup expressions outside single-line regex patterns remain review only |
| Integration tests run sequentially (`-p 1`) under `task test:integration`; serialize heavy Docker/testcontainers runs across concurrent sessions on a shared host | `taskfiles/test.yml` (`-p 1`); review and `semteams-preflight` skill for cross-session serialization | Host-level cross-session serialization is operational / review only |
| Playwright E2E journeys (`task ui:test:e2e`) run in background and are monitored every 20–30s via test output, backend Docker logs, and `/message-logger/entries`, aborting early on loops with quoted log evidence | Review: `svelte-developer`, `svelte-reviewer`; `semteams-preflight` skill | Operational discipline; review only |
| Never run a git command that can discard working-tree state (`checkout --`, `restore`, `stash`, `clean`, `reset --hard`); temporary in-tree fault checks use `cp` backup + verified `shasum -a 256` restore | Review: all role contracts (`go-developer`, `svelte-developer`, `go-reviewer`, `svelte-reviewer`, `technical-writer`) | Review only |
| Added or edited statements on how frozen SemStreams, SemEngine, NATS, or SvelteKit behaves cite `file:line` at the pinned commit or a named test/probe, or state it is not promised | Review: `architect`, `go-developer`, `svelte-developer`, `go-reviewer`, `svelte-reviewer`, `technical-writer` | Review only |

## 5. Workflow, review, and merge gates

| Rule | Enforced by | Gaps |
| --- | --- | --- |
| OpenSpec changes and specs are well formed (`task openspec:validate` / `task spec:check`) and truthful against code | `task openspec:validate` / `task spec:check` (`test/contract/specqueue_test.go`); review for truth against code | Spec truth against code is review only |
| Every OpenSpec task can be ticked in or before the archive commit; holds are written as `Hold:` inside the unticked task they stop (`scripts/openspec-queue.sh --check`, `TestSpecQueueCheck`) | `scripts/openspec-queue.sh --check` (in `task openspec:validate`, `task spec:check`, and `task validate`); `test/contract/specqueue_test.go` (`TestSpecQueueHolds`, `TestSpecQueueCheck`, `TestSpecCheckWiring`) | Post-merge task wording is review only |
| Steps end when `task validate` passes locally on a clean tree; reviewer `APPROVE` cites commit SHA + local verification run, never CI (#171) | `scripts/validate.sh`; review: `go-developer`, `svelte-developer`, `go-reviewer`, `svelte-reviewer` | Reviewer citation format is review only |
| A merge needs `Repository CI` (`CI Status Check`) green on a head up to date with `main` (`task merge:check -- <pr>`) | `.github/workflows/ci.yml` (`CI Status Check`); `scripts/merge-check.sh` (`task merge:check -- <pr>`, `TestMergeCheckUpToDateRule`); `scripts/publish-verify.sh` at release time | GitHub `main` branch ruleset activation is an admin repo setting checked by `task merge:check` |
| No merge while a known flake (`class:flake` issue) is open in a required job unless the PR closes every open one with before/after reproduction evidence (`task merge:check -- <pr>`) | `scripts/merge-check.sh` (`task merge:check -- <pr>`, `TestMergeCheckKnownFlake`); review: `.agents/protocol.md` § Known flakes | `merge-check.sh` runs via `task merge:check -- <pr>` before merge; before/after reproduction evidence is review only |
| Code/UI PRs require review by the appropriate SemTeams reviewer lane (`go-reviewer`, `svelte-reviewer`, or both) and cross-agent review (`implemented-by:` and `reviewed-by:` naming model family `claude`, `codex`, or `gemini`; `task merge:check -- <pr>`); docs-only PRs take any agent's reviewer | `scripts/merge-check.sh` (`task merge:check -- <pr>`, `TestMergeCheckReview`); review: `.agents/protocol.md` § Cross-agent review | Reviewer lane selection (`go-reviewer` vs `svelte-reviewer`) is review only |
| Any defect found in repo code not fixed in the active change is filed as a GitHub issue (`status:blocked` while its code is only on a PR branch), never left in comments | Review: `.agents/protocol.md` § Work lifecycle "File"; `go-reviewer`, `svelte-reviewer` | Review only |
| A finding needing an unchosen condition (library timing/ordering, OS state, mid-op fault, test-only call sequence) is `BLOCKING`/`HIGH` only with a named routine real-world trigger; else `MEDIUM` at most (#185) | Review: `go-reviewer`, `svelte-reviewer` § Finding and verdict format | Review only |
| A new repo-wide or agent-conduct rule names what enforces it and adds its row to both `AGENTS.md` and `.agents/rules-index.md` | `TestAgentSetupParity` (domain headings and row counts); review for 1-to-1 row meaning | Semantic correspondence of individual rows is review only |
| Before claiming work, mutating state, or spawning role subagents on an underspecified prompt, check named issues/PRs/files first, then confirm Deliverable, Scope/non-goals, Definition of Done/verification, and Stop boundary | Review: `.agents/protocol.md` § Work lifecycle "Clarify"; `.agents/README.md` | Review only |
| Role-agent briefs carry the owner's intent and the artifact itself, not a paraphrase; resume for continuity, start fresh for mechanics; designs cap at 3 review rounds before escalating to the owner | Review: `.agents/README.md` § Orchestrating role agents; `architect` contract | Review only |
| `architect`, `go-developer`, `go-reviewer`, `svelte-developer`, and `svelte-reviewer` run on the frontier model tier (`opus` / `gpt-6-astra` or `gpt-6-sol` / Gemini `inherit` or `pro`); `technical-writer` runs on `sonnet` / `gpt-6-sol` / Gemini `inherit` (or `flash` when requested); effort defaults match `.agents/README.md` | `TestAgentSetupParity` (Claude model, Codex model and reasoning effort) | Gemini runtime `define_subagent` / `invoke_subagent` invocation parameters are review only |
| `CLAUDE.md` only imports `@AGENTS.md`, `AGENTS.md` stays strictly `< 24,000` bytes and matches `.agents/rules-index.md` in numbered domains and row counts, and all role/skill adapters match their canonical contracts | `TestAgentSetupParity` and `TestAgentSetupParitySensitivity` (`test/contract/shared_work_protocol_contract_test.go`) | Semantic correspondence of individual rule rows is review only |
| Pushed claim branches live in dedicated worktrees (`../semteams-wt/<branch>` or `.worktrees/<branch>`) and update by merging `origin/main`, never by rebase or force-push | Review: `.agents/protocol.md` § Work lifecycle | Review only |
| Evidence cited in a PR or task can be opened on another machine; git-ignored paths are named as local only, and task choices not in the design go in `openspec/changes/<id>/notes/<task>.md` | Review: `semteams-handoff` and `semteams-preflight` skills; `.agents/protocol.md` | Review only |
| Docs are written for a working developer: shipped vs. target behavior kept distinct (`docs/demo-mvp-claims.md`), coined terms defined at first use, no marketing register | Review: `technical-writer`, `go-reviewer`, `svelte-reviewer` | Review only |
