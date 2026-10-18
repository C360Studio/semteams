---
name: semteams-handoff
description: >-
  Checkpoint a SemTeams PR or issue for continued work or transfer between Claude, Codex, and Gemini sessions.
  Reconcile shared records and return a short pickup prompt at a meaningful work boundary.
---

# Checkpoint SemTeams work

Read [the shared protocol](../../protocol.md) first. It owns claims, state locations, filing, and landing gates.
Use this workflow within the user's authorized effort. It grants no extra publication, merge, closure, or
cross-repository authority. Preserve authorization already given without asking for it again.

## Reconcile the checkpoint

Identify the effort by PR or issue. Verify its worktree path (`../semteams-wt/<branch>` or `.worktrees/<branch>`),
branch, HEAD, upstream divergence, and dirty state. Read the current PR, applicable issue decisions, and active
OpenSpec change. Run `task openspec:queue` in the claim's worktree: an empty queue in the primary checkout does not
describe unmerged work elsewhere.

Record shared facts in the protocol's existing homes: the PR stop-point description, OpenSpec tasks and `Hold:` lines,
issue decisions, or ADRs (`docs/adr/`). Re-read a shared record before updating it and preserve other authors' content.
Do not turn a reviewer recommendation or inferred decision into an owner ruling. Do not create a separate prose handoff
or baton document competing with those homes.

Keep test evidence attached to the revision it tested: command (`task validate`, lane commands, or E2E journey),
result, timestamp or run link, and artifact path. Cite evidence a reader on another machine can open: output pasted
into the PR or a CI run. A path git ignores (such as `.enjoy-logs/` or `/tmp/`) exists only on the host that wrote it;
when one is cited, say it is local only, and never make it a task's only proof. Identify tests on a dirty tree and the
snapshot or diff needed to reproduce them. Distinguish failed, skipped, and unrun checks. Historical green does not
prove later edits. Run inexpensive state checks now; do not repeat expensive suites solely to write a handoff.

Record pending commands, approvals, background E2E stacks, or dev servers with their identifiers and owners, including
whether they will keep running, so the next session does not duplicate them. Do not silently cancel or duplicate
another session's work.

## Preserve work and ownership

When a push is already authorized and applicable gates pass, verify the branch still matches the PR head and publish
the checkpoint through the protocol. Do not force a push, weaken a gate, or include someone else's edits to satisfy
handoff wording. If publication is blocked, preserve local work and report what is unpushed or uncommitted, why, and
where it lives. The receiver must verify access before relying on a local-only checkpoint.

For a transfer, identify the writer relinquishing ownership and the intended receiver. A draft PR identifies the effort
and its branch prefix (`claude/`, `codex/`, `gemini/`) names the claimant; neither shows that the claimant's worktree
is idle. The receiver may inspect but must not edit until the old writer has stopped or released ownership.

## Return the pickup prompt

Use these five fields in the conversation. Include useful addresses, constraints, and checks; write `none` for an empty
field. Shared decisions, holds, and ordering are pointers to their homes. Machine-local details belong here only when
the receiver cannot obtain them from shared state.

```text
Goal: PR/issue and the authorized outcome.
Addresses: PR, change ID, worktree, branch, checkpoint SHA, and evidence locations.
Constraints: Decision/hold pointers, local access limits, and write-ownership transfer status.
Done when: Existing acceptance criteria and the remaining boundary.
Checks to run: Cheap reconciliation commands; necessary unrun validation distinguished from prior evidence.
```

End with a request to use [semteams-pickup](../semteams-pickup/SKILL.md) for the named effort. Shared records are
authoritative: a private memory file is optional and cannot be required for another agent.

## Platform behavior

In Codex, invoke `$semteams-handoff` or ask to checkpoint the effort. In Claude, use `/semteams-handoff`. In
Antigravity (Gemini), follow `.agents/skills/semteams-handoff/SKILL.md` directly. The names deliberately differ from
personal `handoff` and `pickup` skills.

Checkpoint after material decisions, review rounds, or verified work slices. Target a handoff at about 400K tokens of
context: finish the current step, run this skill, and start a fresh session so auto-compaction is not reached. A
checkpoint need not end the task; continue unless a transfer, the threshold, or a real hold applies.
