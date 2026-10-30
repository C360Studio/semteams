# Shared Work Protocol (Claude, Codex, and Gemini)

Canonical. `CLAUDE.md` imports `AGENTS.md`, which points to this file and carries the four gates (clarify, claim,
merge, close) inline; edit the protocol here only. Read it before taking, landing, or closing work; the
[pickup](skills/pickup/SKILL.md), [handoff](skills/handoff/SKILL.md), [preflight](skills/preflight/SKILL.md),
[review-pr](skills/review-pr/SKILL.md), [ponytail](skills/ponytail/SKILL.md), and [design](skills/design/SKILL.md)
skills read it. Adapted from SemEngine's and ATeam's `.agents/protocol.md` alongside SemVocab and SemSparql.

State that all agents must see lives in the repository's tools, never in an agent's private memory or a separate
program-status document. Each question has one authoritative home, and each home is discoverable with `gh`, `ateam`,
`task`, or `openspec`. Local `.ateam/` snapshots (`.ateam/state.json`, written by `ateam state handoff` and read by
`ateam state pickup`) are git-ignored working caches reconciled against live git and GitHub state, never unverified
truth. There is no `/tickets` directory or program baton file (legacy `/tickets` YAML records were reconciled into
issues and removed under #255).

SemTeams routes design and implementation through its project roles: `architect` owns API and integration contracts;
`go-developer` implements backend/pack work and `go-reviewer` reviews it; `svelte-developer` implements UI work and
`svelte-reviewer` reviews it; cross-stack changes use both reviewer lanes; and `technical-writer` owns durable docs and
conservative OpenSpec task truth. Role contracts, `.agents/review-policy.json`, and per-platform model routing live in
[`.agents/README.md`](README.md).

## Authoritative state homes

| Question | Home | Rule |
|---|---|---|
| What is wanted, what kind, is it decided | GitHub issue + labels (`type:` / `area:` / `class:` / `status:` / `horizon:`) | `status:needs-decision` is the owner's docket; a ruling is posted as an issue comment and the label removed. `status:blocked` names its blocker in a comment. |
| What gates the next tag | GitHub milestone named for the intended version | Membership is the gate: in or out; an unruled item is out. `horizon:pre-v1` means before v1.0.0, not before the next tag. |
| An epic | A tracking issue labeled `type:epic` whose body carries a task list of `#n` children | GitHub renders the progress; there is no separate epic document. |
| Who has claimed what | A **draft PR** opened at the start of the work, with `Closes #n` (or `Addresses #n` for an epic) in its body; the branch prefix names the agent (`claude/...`, `codex/...`, `gemini/...`) | No draft PR, no claim. Design work claims the same way: its OpenSpec proposal is the first content commit. Put the current stop-point in the PR description. |
| Session checkpoint and handoff | PR/issue comment + `.ateam/state.json` via `ateam state handoff` (`ateam_state` MCP `handoff`) and `ateam state pickup` | Checkpoints emit the canonical 5-field pickup prompt (`Goal`, `Addresses`, `Constraints`, `Done when`, `Checks to run`) and clear the writer lease without creating tracked prose baton files. |
| Target state, task truth, holds | The OpenSpec change inside that PR; `task openspec:queue` / `task spec:queue` (`ateam state spec-queue`) reads its holds | Archive (`openspec archive <id>`, moving the change into `openspec/changes/archive/` plus spec sync) as the landing PR's final content commit, reviewed with the implementation. Every task can be ticked in or before the archive commit. No task may assert a post-merge fact ("CI green", "merge-ready") or wait on a step after archive. A hold is written inside the unticked task it stops as `Hold: <reason>`. |
| Upstream dependencies | An issue on the owning repository (`semstreams`, `semengine`, `ateam`, `semmem`, `semsource`), linked from the SemTeams issue or task it blocks, with that task carrying the `Hold:` | Sister repositories are read-only from here; never fork unreviewed upstream mechanics locally. |
| Why | An ADR (`docs/adr/`), or the owner's ruling comment on the issue | ADRs record durable architectural decisions, not work status or implementation checklists. |

OpenSpec changes are contract deltas or approval-gated proposals, not backlogs: a change either carries spec deltas or
is proposal-only (Work lifecycle, Clarify). Sequencing, discovery, holds, and future work belong in GitHub issues.

New and changed tests cite a spec Requirement: every Go or Vitest test a pull request adds or changes carries
`// spec: openspec/specs/<capability>/spec.md#Requirement: <name>` (or `//aip:verifies <scenario-id>`) in the comment
attached to it. `task test:spec` (`ateam test-spec --base`,
[C360Studio/ateam#237](https://github.com/C360Studio/ateam/issues/237)) checks it in the Governance job of
`Repository CI` against the pull request base, and in `task validate` against `origin/main`. Uncited legacy tests the
diff does not touch are not reported; a malformed or dangling citation fails wherever it is. A citation of an in-flight
change's spec moves to `openspec/specs/` in the archive commit (owner ruling on #386, lifting the #374 deferral).

## Bounded port and extraction scope

When extracting agent-runtime packages from the frozen SemStreams pin (`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`) or
adopting SemEngine contracts under ADR-061, a change implements the admitted surface and its accepted contract. A
finding adds required work only when the correction satisfies a binding requirement (the issue's acceptance criteria, a
spec requirement), a verification gate, an explicit owner ruling, an accepted design commitment, or the dependency
change needed to build. Name that obligation and the concrete failure or missing evidence. A maintenance preference
alone (inherited duplication, naming, indirection) is optional: it cannot veto approval or become `BLOCKING`/`HIGH`,
and scheduling it needs an explicit owner decision. This never excuses a broken contract, a newly introduced defect, a
rewired parked pack, or an unproved composition-root invariant.

## Work lifecycle

- **Start:** `ateam state pickup` · `gh issue list --state open` · `gh pr list` (drafts are claims; skip them) ·
  `task spec:queue` · `gh run list --branch main --limit 3` · `gh issue list --label status:needs-decision`.
- **Clarify:** before claiming work, mutating state, or spawning role subagents, read any named issue, PR, or file so
  you never ask a question the repository already answers, then verify four boundaries are clear:
  1. **Deliverable** (answer/audit, issue, OpenSpec design, docs-only PR, or code/UI PR),
  2. **Scope and non-goals** (in-bounds packages/files and what must not change),
  3. **Definition of Done and verification** (observable completion criteria and `task` gates), and
  4. **Stop boundary** (pause after inventory/design for an owner decision vs. run through to a ready PR).
  When a prompt leaves any of the four ambiguous, stop first, propose a concise 3–4 bullet recommended Definition of
  Done the owner can approve or adjust in one reply, and ask at most 1–3 targeted questions; once set, run autonomously
  to that stop boundary without pausing on routine steps.
  - **Proposal approval** (owner ruling on #374): for non-trivial work the default Stop boundary is proposal approval:
    a one-page OpenSpec `proposal.md` (≤ 60 lines; detail goes in `tasks.md` or `design.md`; `task spec:check` warns
    past it) with an empty `Approved-by:` line, and a first task carrying `Hold: OPEN-Q proposal awaits owner approval`
    until the owner fills `Approved-by: <name> (<YYYY-MM-DD>)` and removes the hold in the same edit; the line may link
    the issue comment where the owner ruled. The agent stops at the hold and hands off. Non-trivial work that changes
    no Requirement carries a proposal-only change (`proposal.md` and `tasks.md`, no `specs/`) whose `.openspec.yaml`
    sets `skip_specs: true`, as ATeam's own proposal-only changes do; `openspec validate --strict` rejects a change
    with no delta otherwise.
  - **Track:** a non-trivial change also declares its track: `design` when it adds a spec Requirement, touches more
    than one capability under `openspec/specs/`, adds exported surface reachable from an adopting repository, or
    touches a boot path, durability/ack behavior, or a gate file, otherwise `quick` with no `design.md`. On `design`,
    the [design](skills/design/SKILL.md) skill drafts `design.md` in OpenSpec's `spec-driven` sections (≤ 120 lines;
    `task spec:check` warns past it), and the one approval hold covers `proposal.md` and `design.md`. SemTeams runs no
    `Gate files` check (#372); a gate file here is one that decides whether a pull request may merge:
    `.agents/review-policy.json`, `scripts/merge-check.sh`, and `.github/workflows/`.
- **Take work:** an unclaimed issue → a dedicated worktree on an agent-prefixed branch (`claude/<topic>`,
  `codex/<topic>`, `gemini/<topic>`; the prefix names who claimed it) → push → draft PR with `Closes #n` (or
  `Addresses #n` for an epic) → then implementation. One claimed PR owns one worktree. When multiple agents share a
  host, the primary checkout is discovery-only and no agent commits from it. Verify the worktree path and branch before
  every edit, commit, push, or PR mutation; a mismatch stops the operation. When pausing or transferring across
  sessions, run `ateam state handoff` (see [handoff](skills/handoff/SKILL.md)).
- **Worktree hygiene:** for standalone CLI sessions, the claim's worktree lives at a durable sibling path
  (`git worktree add ../semteams-wt/<branch> -b <branch> origin/main`), never under `/private/tmp`. For Enjoy desktop
  sessions, `move_to_worktree` creates and reuses the branch's worktree under `.worktrees/<branch>` inside the
  repository root (ignored in `.gitignore` alongside `.ateam/` and `.enjoy-logs/`); run all commands and file edits
  inside the path `move_to_worktree` returns. Pushed claim branches update by merging `origin/main`, never by rebase or
  force-push: a review record names the commit SHA it read. Heavy local gates (`task test:integration`,
  `task ui:test:e2e`, `task validate`) run one agent at a time on a shared host.
- **Prove tests can fail:** state the correct behavior and one plausible wrong behavior, and compute expected outcomes
  from an independent oracle, never by repeating production logic. Show non-trivial logic able to fail without a git
  command that discards state: `task mutate:check -- -pkg <pkg> -test <Test> -file <file.go> -mutant <copy> -expect
  <file_test.go:N>` (`ateam mutate`: `go test -overlay` against a mutant kept outside the repository; prefix
  `GOFLAGS=-tags=integration` for an integration-tagged test), or `cp` + `shasum -a 256` before and after for Vitest
  and anything `-overlay` cannot reach. Report `detection`, `survivor`, `invalid`, or `inconclusive`.
- **File:** a defect in code in this repository that the active change does not fix is always filed as a GitHub issue,
  whether the code was carried, extracted, or written here. Check for an existing issue first (`gh issue list --search`).
  When the defect's code exists only on a pull request's branch, the issue is labelled `status:blocked` with that pull
  request named as its blocker, and the pull request body links it without a closing keyword
  (`Follow-up after this pull request merges: #n`). A defect in a sister repository is reported to its owner, not fixed
  here.

  Route every other finding before opening an issue. A residual (a cost of a decision this change made deliberately)
  belongs in a doc comment at the line or in the change's `design.md`. An unmeasured cost becomes a *Declared cost*
  section in the design. Only an architectural finding (it crosses files, would need its evidence re-collected, or
  changes what someone should not do next) becomes an issue. Developer notes on choices a task made that its design does
  not spell out go in `openspec/changes/<id>/notes/<task>.md`, linked from a one-line PR comment; they archive with the
  change.
- **Land:** implementation review (`go-reviewer` for Go/packs, `svelte-reviewer` for `ui/`, or both for cross-stack,
  and another agent's reviewer on code/UI pull requests: "Cross-agent review" below) → fixes and re-review → archive
  (`openspec archive <id>`, moving the change into `openspec/changes/archive/` + spec sync) as the final content commit
  → narrow reviewer check of the archive/spec sync (any agent's reviewer) → undraft → `task validate`,
  `task merge:check -- <pr>`, `Required`, and `Merge check` green on a head up to date with `main` and with **no
  known flake open** → squash merge. A draft PR skips every `Repository CI` job and `Merge check` (no skipped
  check-run carries the name `Required` or `Merge check`), so undrafting starts the first run that checks it. On a non-draft code/UI PR,
  `Merge check` (its own workflow, `merge-check.yml`) is skipped (under a name that is never `Merge check`) while
  the PR has no review record and no `reviewed-by:` line, so the required context is absent and the merge stays
  blocked; once either exists it runs in full and is red until both the approving record and the line exist. The
  reviewer's description edit that adds `reviewed-by:` reruns it on the `edited` event, as
  [review-pr](skills/review-pr/SKILL.md) § 5 says (never update the branch). That red is expected, not a flake, and it
  never turns `Required` red. A correction after archive re-enters reconciliation and final review. State
  `implemented-by: <agent> (<model>, <effort>)` in the PR body naming the implementing model family (`claude`, `codex`,
  or `gemini`), resolved model, and reasoning effort (`implemented-by: gemini (gemini-4-argon, high)`), and
  `reviewed-by: <agent> (<model>, <effort>)` once the reviewing agent's newest record approves. Never write unresolved
  `inherit` in `implemented-by:` or `reviewed-by:`.
  Once the squash merge has landed, the agent that claimed the branch removes its worktree (`git worktree remove
  ../semteams-wt/<branch>` on the CLI, `.worktrees/<branch>` in Enjoy) and deletes the remote branch; a review checkout
  is removed once its review record is posted. `ateam state pickup` labels every merged or PR-less worktree; a
  finished worktree older than a day is the claimer's to explain or remove. Never remove a worktree you do not own,
  and never one that is dirty or unpushed.
- **Cross-agent review:** on a code/UI pull request the implementation review, and the re-review of a fix that changes
  code or UI, are done by an agent that wrote none of the commits under review, enforced by `task merge:check -- <pr>`
  (`scripts/merge-check.sh` → `ateam review gate`) against `.agents/review-policy.json` (`cross_provider: true`,
  reviewer not weaker by model tier, then effort band, than every implementer). The agent named is the **model family**
  that did the work (`claude`, `codex`, `gemini`), not the service or IDE it ran in: a Claude model running in
  Antigravity is `claude`, and a review by a different model family counts as cross-agent even when both ran in the same
  tool (owner ruling of 2026-10-15). A **documents-only** pull request changes only `.md` files or `.yml`/`.yaml`/`.json`
  files under `openspec/`, and none under `.claude/agents/` or `.agents/contracts/`; it keeps any agent's reviewer. Any
  other changed file (`.go`, `.svelte`, `.ts`, `go.mod`, `package.json`, a rule/persona JSON or prompt file, a script
  including `openspec/*.sh`, `Taskfile.yml`, a CI workflow, `.claude/agents/*`, `.codex/agents/*`, `.agents/contracts/*`,
  `.agents/review-policy.json`) makes it a code/UI pull request.
  - The implementer asks with a PR comment headed `Review request`: the review lane (`go-reviewer`, `svelte-reviewer`,
    or both), the commit SHA to read, the diff range, the implementer's local verification result (`task validate`) on
    that commit (clean tree), and the issue or ruling the change answers. It does not say what the reviewer should
    conclude.
  - The reviewing session posts one PR comment in ATeam's commit-bound review record contract
    ([`contracts/reviewer.md`](contracts/reviewer.md) § Record; [review-pr](skills/review-pr/SKILL.md)):
    ```text
    reviewed-by: <agent> (<model>, <effort>)
    Reviewed at <commit-sha>
    Verdict: APPROVE | REQUEST CHANGES
    <SemTeams reviewer-lane findings (BLOCKING, HIGH, MEDIUM, NIT), checks run, unavailable evidence>
    ```
    The comment must designate a single commit of the pull request via `Reviewed at <sha>` (7–40 hex chars; do not add a
    competing `Head: <other-sha>` line) and carry exactly one `Verdict:` line outside code fences; `ateam review gate`
    passes only `Verdict: APPROVE`. Without its own `task validate` / lane run, the implementer's result is recorded as
    reported, never as passed. The record does not rest on CI: the CI run before merge is the check.
  - A record covers the commit SHA it names. A later commit that changes code/UI goes back to the reviewing agent; a
    later documents-only commit (such as `openspec/changes/archive/...`) and a clean merge of `origin/main` touching no
    file the pull request changes carry the record over (`ateam review gate`).
  - The reviewer does not write on the branch. If the agents disagree on a finding, it goes to the owner on the issue,
    labelled `status:needs-decision`.
- **Known flakes:** a known flake is an open issue labelled `class:flake`: a test or check that passes and fails on the
  same tree. A network fetch that did not answer is not one; record it on the pull request and re-run. A red you cannot
  explain by your diff is filed before the next push. An open `class:flake` the PR does not close fails `Merge check`,
  not `Required`. A fresh green over an open `class:flake` in a required job is rerun-to-green, not a fix: fix the flake
  in a PR that closes the `class:flake` issue and shows the reproduction (command and failure rate before and after). A flake that comes back reopens its issue.
- **Close:** no issue closes without the owner's explicit `CONFIRM-CLOSE` on the issue itself or the squash merge of an
  approved PR that declared `Closes #n`. A PR that only `Addresses` an epic closes nothing. Approval of adjacent work
  never widens into a close.

## Verification and CI baseline

`.github/workflows/ci.yml` runs the Go, UI, and Governance/OpenSpec jobs for every non-draft pull request and push to
`main`; all three feed the stable `Required` aggregate check-run. `.github/workflows/merge-check.yml` runs `Merge check`
(`ateam review gate` from the base commit) as a second required check; the `main` ruleset requires both. Locally,
`task validate` (`scripts/validate.sh`, alias `task verify`) mirrors all three lanes (including `task ateam:doctor`) in
read-only mode without mutating tracked or untracked worktree files. Gate selection per diff and evidence recording live in [preflight](skills/preflight/SKILL.md).
Publication is tag-only through `.github/workflows/publish.yml`; see [`docs/release.md`](../docs/release.md).
