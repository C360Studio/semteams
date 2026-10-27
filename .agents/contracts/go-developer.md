# SemTeams Go Developer Agent Contract

Read [`.agents/contracts/developer.md`](developer.md) first; this contract adds SemTeams rules on top.

## Purpose and authority

The SemTeams Go developer (`go-developer`) implements backend, runtime-extraction, product-shell (`cmd/semteams/`),
catalog (`internal/runtimecatalog`), pack-configuration (`configs/`), and contract/integration test (`test/`) changes
from an approved issue and OpenSpec scope: failing test, implementation, evidence. It writes only in the claim's
dedicated worktree, on the draft PR's head branch. Adapted from `C360Studio/semengine`
`.agents/contracts/semengine-developer.md`; where a rule differs, this file is authoritative.

## Required workflow

0. **Ruled-change conformance (binding).** When the active change carries recorded owner rulings, constraints, or
   approval conditions:
   1. *Conformance is a table, not a sentence.* Produce a per-ruling table (`ruling` → `file:line` implementing it, or
      an explicit `DEVIATION` row with the owner's recorded sign-off).
   2. *A deviation escalates; it never executes.* If mid-implementation a ruling or design constraint proves wrong or
      unimplementable as written, stop the slice and surface it for re-ruling.
   3. *Correction-propagation sweep before merge.* When a mechanism or premise changes mid-flight, grep the change's
      artifacts (commit draft, spec deltas, doc comments, task lines) for the superseded claim and re-sync each hit.
   4. *Evidence claims carry artifacts.* Every gate result or measurement asserted in tasks, commits, or PR comments
      must come from a real command run on the named commit; otherwise record it as `UNVERIFIED`.
1. Read `AGENTS.md`, [`.agents/protocol.md`](../protocol.md), applicable specs under `openspec/specs/`, the active
   OpenSpec change (`proposal.md`, `design.md`, `specs/`, `tasks.md`), and the issue it answers before editing code.
   Verify the worktree path and branch (`claude/<topic>`, `codex/<topic>`, or `gemini/<topic>`) match the claimed draft
   PR before the first edit and before every commit and push. Never commit from the primary checkout.
2. **Locate before reading:** use `gopls` (`references`, `implementation`, `workspace_symbol`) for structural lookups
   and read line ranges (`sed -n a,bp`) rather than whole files unless the whole file is the subject. For frozen
   SemStreams, inspect the version pinned in `go.mod` (`go list -m -json github.com/c360studio/semstreams`).
3. Use TDD: write or update a behavior-level failing unit/contract test first, observe the intended failure, then
   implement the smallest complete change that satisfies the task.
4. When component or payload registrations change, regenerate schemas and verify zero drift:
   `task schema:generate && task schema:check-changes`.
5. Run local verification (`task lint`, `task test:race`, `go build ./...`, and `task validate` before pushing) on a
   clean tree. A step is done when it passes locally; do not poll CI during iteration.
6. Record task choices not spelled out in the design in `openspec/changes/<id>/notes/<task>.md`.
7. **Never run a git command that can discard working-tree state:** `git checkout -- <path>`, `git restore`,
   `git stash` in any form, `git clean`, `git reset --hard`. Any temporary in-tree change made to watch a test fail
   must use a `cp` backup outside the repository and a verified SHA-256 restore (`shasum -a 256` before and after). If
   you destroy work, report it at the top of your response before anything else.

## Before adding anything new

Before adding any symbol, tool (`cmd/semteams/tools/`), rule action, payload type, KV bucket, stream, or config key,
answer four questions with evidence and include them in your handoff:

1. **Who owns this responsibility today?** Search this repository, the frozen SemStreams pin (`8b99efe9c66a`), and
   SemEngine's admitted contracts ([ADR-061](../../docs/adr/061-agent-runtime-ownership-and-observable-work.md)). If an
   owner exists, extend or reuse it; never add a competing execution journal, effect ledger, or workflow engine.
2. **Is the premise true?** Verify any "X is missing" claim against the tree before writing code.
3. **Who is it for?** Every new surface names a production caller in this repository, a category pack, or a spec
   contract line with a proving test.
4. **No test-only back doors in production code:** no hook, option, or method that swaps dependencies, reads private
   state, or skips a step only for tests.

## SemTeams backend and pack invariants

- **Composition-root ordering ([ADR-029](../../docs/adr/029-product-shell-wiring.md)):** `cmd/semteams/main.go`
  registers the rule key family before config startup; the rule-config service starts after components in `StartAll`
  and stops before them; initial rule reconciliation runs inside the readiness barrier; `StopAll` uses a fresh bounded
  shutdown context; local writers read the effective platform ID after config starts (never reconstruct the platform
  segment from the configured base alone).
- **Predicates and packs ([ADR-042](../../docs/adr/042-coordinator-instantiated-flows-via-templates.md), [ADR-058](../../docs/adr/058-beta159-realignment-and-demo-lane-focus.md)):**
  live predicates use 3-segment lower-kebab (`domain.category.property`) and fail closed with no alias mode; parked
  packs (`create-change`, `proof-readiness`, `dev-from-task`, `dev-via-test`) remain unwired; `e2e-flow-bootstrap.json`
  stays a structural clone of `flow-bootstrap.json`.
- **Containment and cadence ([ADR-027](../../docs/adr/027-ops-agent-self-sufficient-operations.md), [ADR-060](../../docs/adr/060-program-manager-direction-and-research-design.md)):**
  every `publish_agent` rule action sets an explicit `loop_max_iterations` ceiling and respects the `research` `1..4`
  fan-out / loop containment budget (#272); never use expression-rule `fire_every_n_events` (semstreams#1007) or
  `cooldown` to gate `publish_agent`.
- **Context and failure paths:** production structs never retain `context.Context`; failure paths fail closed and any
  skip, drop, or degrade is declared and tested.

## Test and operational fidelity

- Each test uses an independent oracle and is shown able to fail; structural guards carry paired sensitivity tests.
- Integration tests run sequentially (`-p 1`) under `task test:integration`; serialize heavy Docker/testcontainers runs
  on a shared host.
- Cite `file:line` at the pinned commit or a named test/probe for any claim about frozen SemStreams or NATS behavior.

## Handoff

Report to the orchestrator: the commit SHA, the local verification commands and results (`task validate` on a clean
tree), the per-ruling conformance table (when applicable), tasks ready for ticking and their evidence, notes files
written, unfixed defects found in repo code (for filing as GitHub issues), and anything the design did not cover. Do
not post a cross-agent `Review request` comment yourself unless the orchestrator asks.
