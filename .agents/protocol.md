# Shared Work Protocol (Claude, Codex, and Gemini)

State that all agents must see lives in the repository's tools, never in an agent's private memory or a separate
program-status document. Each question has one authoritative home, and each home is discoverable with `gh`, `task`, or
`openspec`.

SemTeams routes design and implementation through its project roles: `architect` owns API and integration contracts;
`go-developer` implements backend work and `go-reviewer` reviews it; `svelte-developer` implements UI work and
`svelte-reviewer` reviews it; cross-stack changes use both reviewer lanes; and `technical-writer` owns durable docs and
conservative OpenSpec task truth. Role contracts and per-platform model routing live in [`.agents/README.md`](README.md).

## Authoritative state homes

| Question | Home | Rule |
|---|---|---|
| What is wanted, what kind, is it decided | GitHub issue + labels (`type:` / `area:` / `class:` / `status:` / `horizon:`) | `status:needs-decision` is the owner's docket; a ruling is posted as an issue comment and the label removed. `status:blocked` names its blocker in a comment. |
| What gates the next tag | GitHub milestone named for the intended version | Membership is the gate: in or out; an unruled item is out. `horizon:pre-v1` means before v1.0.0, not before the next tag. |
| An epic | A tracking issue labeled `type:epic` whose body carries a task list of `#n` children | GitHub renders the progress; there is no separate epic document. |
| Who has claimed what | A **draft PR** opened at the start of the work, with `Closes #n` (or `Addresses #n` for an epic) in its body; the branch prefix names the agent (`claude/...`, `codex/...`, `gemini/...`) | No draft PR, no claim. Design work claims the same way: its OpenSpec proposal is the first content commit. Put the current stop-point in the PR description. |
| Target state and task truth | The OpenSpec change inside that PR; `task openspec:queue` reads its holds | Archive (`openspec archive <id>` plus spec sync) as the landing PR's final content commit, reviewed with the implementation. No task may assert a post-merge fact such as "CI green" or "merge-ready". A hold is written inside the unticked task it stops as `Hold: <reason>`. |
| Why | An ADR, or the owner's ruling comment on the issue | ADRs record durable architectural decisions, not work status or implementation checklists. |

OpenSpec changes are contract deltas, not backlogs. Sequencing, discovery, holds, and future work belong in GitHub
issues. There is no separate program baton document or ticket directory; the legacy `/tickets` YAML records were
reconciled into issues and removed under #255 (history remains in git).

## Work lifecycle

- **Start:** query the milestone's open issues (`gh issue list --state open`), draft PR claims (`gh pr list`),
  `task openspec:queue`, recent `main` runs (`gh run list --branch main --limit 3`), and `status:needs-decision`
  issues before choosing work.
- **Clarify:** before claiming work, mutating state, or spawning role subagents, read any named issue, PR, or file so
  you never ask a question the repository already answers, then verify four boundaries are clear:
  1. **Deliverable** (answer/audit, issue, OpenSpec design, docs-only PR, or code/UI PR),
  2. **Scope and non-goals** (in-bounds packages/files and what must not change),
  3. **Definition of Done and verification** (observable completion criteria and `task` gates), and
  4. **Stop boundary** (pause after inventory/design for an owner decision vs. run through to a ready PR).
  When a prompt leaves any of the four ambiguous, stop first, propose a concise 3–4 bullet recommended Definition of
  Done the owner can approve or adjust in one reply, and ask at most 1–3 targeted questions; once set, run autonomously
  to that stop boundary without pausing on routine steps.
- **Take work:** an unclaimed issue → a dedicated worktree on an agent-prefixed branch (`claude/<topic>`,
  `codex/<topic>`, `gemini/<topic>`; the prefix names who claimed it) → push → draft PR with `Closes #n` (or
  `Addresses #n` for an epic) → then implementation. While concurrent agents have claimed work, each claimed PR has
  exactly one dedicated worktree and the primary checkout is discovery-only. Verify the worktree path and branch before
  every edit, commit, push, or PR mutation; never share a worktree between claimed PRs.
- **Worktree hygiene:** for standalone CLI sessions, the claim's worktree lives at a durable sibling path
  (`git worktree add ../semteams-wt/<branch> -b <branch> origin/main`), never under `/private/tmp`. For Enjoy desktop
  sessions, `move_to_worktree` creates and reuses the branch's worktree under `.worktrees/<branch>` inside the
  repository root; run all commands and file edits inside the path `move_to_worktree` returns. Pushed claim branches
  update by merging `origin/main`, never by rebase or force-push.
- **File:** any defect found in repository code that is not fixed in the active change is filed as a GitHub issue
  (linked without a closing keyword while on a PR branch), never left in comments or a local backlog.
- **Reviewer-pass & cross-agent review:**
  - Every multi-phase or nontrivial change runs the appropriate SemTeams reviewer lane (`go-reviewer` for Go/backend,
    `svelte-reviewer` for `ui/`, or both for cross-stack changes) at each critical boundary, plus the cross-agent
    review round when requested or on code/UI changes across agents.
  - When handing off across agents on a pull request, the implementer posts a PR comment headed `Review request`
    naming the review lane, the commit SHA to read, the diff range, the local verification commands and results on that
    commit, and the issue or ruling answered.
  - The reviewing session answers with a PR comment headed `Review record` containing the reviewer's report on that
    named commit, what was run, and what could not be run. The reviewer remains read-only on the branch.
  - A review record covers the commit SHA it names; later content commits require re-review.
  - State `implemented-by: <agent> (<persona/model>)` in the PR body (`claude`, `codex` / `Sol`, or `gemini`), and
    `reviewed-by: <agent> (<persona/model>)` when a cross-agent or role review record approves the head.
- **Land:** undraft → appropriate SemTeams reviewer lane (`go-reviewer`, `svelte-reviewer`, or both), plus the
  cross-agent round when requested → all applicable local and hosted gates green (`Repository CI` / `CI Status Check`),
  with no known unfixed flake (`class:flake`) in a required job → squash merge closes the issue. A fresh green over a
  known flake is rerun-to-green: fix it, or file it and obtain an explicit owner waiver in a PR comment.
- **Close:** no issue closes without the owner's explicit `CONFIRM-CLOSE` or the squash merge of an approved PR that
  declared `Closes #n`.

## CI baseline

`.github/workflows/ci.yml` runs the Go, UI, and Governance/OpenSpec jobs for every pull request to `main`; all three
feed the stable `CI Status Check` aggregate. Required mock E2E and a main-branch ruleset remain future work.
Publication is tag-only through `.github/workflows/publish.yml`; see [`docs/release.md`](../docs/release.md).
