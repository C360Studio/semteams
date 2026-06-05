# Emit contract — Karpathy guidelines as schema

The `emit_dev_via_test_plan` tool rejects payloads that omit the
fields below. The schema is the discipline — you cannot ship a
spec without explicitly surfacing the things Karpathy's guidelines
say a serious software task needs.

## Required at plan level

| Field | Source | What to write |
|---|---|---|
| `goal` | (overall framing) | One sentence in the user's own words. Concrete and verifiable. Not "make it better" — name the capability. |
| `assumptions` | **Karpathy Rule 1 — think before coding** | Plan-level assumptions: environment, deps, semantics. Array of strings. Empty `[]` is legal but you must surface it explicitly. |
| `non_goals` | **Karpathy Rule 2 — simplicity first** | What this work excludes. Array of strings. Empty `[]` is legal — but think about it. |
| `integration_test_command` | **Karpathy Rule 4 (plan scope) — goal-driven execution** | CBG's chain-end full acceptance gate. One shell command. Runs across all task scope. |
| `tasks` | (decomposition) | One or more task objects. See below. |
| `revision` | (re-planning) | Integer ≥ 1, **required** (no default). First emit = 1; bump on coordinator-requested re-plan. The schema rejects absent or `0` so a re-plan accident can't silently look like a first emit. |

## Required per task

| Field | Source | What to write |
|---|---|---|
| `id` | (key) | Stable identifier. Lowercase alphanumeric + hyphens, 1-32 chars. `t1`, `t2`, `parse-iso8601`, `add-tests`. |
| `goal` | (task framing) | One-sentence task-level goal. Concrete and verifiable. |
| `assumptions` | **Karpathy Rule 1 (task-local)** | Per-task assumptions on top of plan-level. Empty `[]` is normal when there's nothing new to add. |
| `non_goals` | **Karpathy Rule 2 (task-local)** | Per-task anti-scope. Empty `[]` is normal. |
| `target_files` | **Karpathy Rule 3 — surgical changes** | File globs Ralph may modify. **At least one required.** The smaller the better. For cross-cutting work, pick the narrowest accurate set; if you truly need broader scope, that's a sign the task should split. |
| `test_command` | **Karpathy Rule 4 — goal-driven execution** | One shell command Ralph iterates against. Exits 0 ⇒ task done. Specific to this task; not the whole suite (that's `integration_test_command`). |

Optional per task:

- `depends_on` — task IDs that must complete before this is ready.
  v1 walker is linear (ignores this); v2 will topo-walk. Emit `[]`
  for first task; emit the prior task's ID if expressing intent.
- `expected_outcome` — human-readable "done looks like". Helps CBG's
  diff review and operator-facing logs. Not load-bearing for Ralph.

## Quality bar (per [[real-llm-smoke-surfaces-engine-gaps]] + [[encode-principles-structurally]])

The schema enforces *presence*. *Substance* is on you:

- **Specific assumptions.** "Go 1.25 in PATH, network access to
  proxy.golang.org" beats "modern Go toolchain available."
- **Narrow target_files.** If you list `**/*.go`, you're not
  thinking. Pick the file or two that the change actually touches.
- **Test commands that fail before, pass after.** Ralph iterates
  against the command's exit code. If the command already passes
  before any code change, you have no convergence signal.
- **Plan-level test ≠ task-level test.** `integration_test_command`
  is the full suite (`go test ./...` + lint + fuzz, whatever
  proves the plan). Per-task `test_command` is the narrow slice
  Ralph iterates against (`go test -run TestX ./pkg/foo`).

## Anti-patterns

- **Bundling unrelated changes into one task.** If a task's
  `target_files` spans three independent surfaces, it's three tasks.
- **Test commands that hide the truth.** `go test ./... || true`,
  `go vet ./... 2>/dev/null` — these make Ralph "converge" instantly
  while shipping nothing. Tests must fail loudly when they fail.
- **`integration_test_command` = single-task test.** The chain-end
  gate must cover all task scope. If the plan is one task, the
  integration command equals the task's test command; that's fine.
- **Speculative non_goals.** "we won't rewrite the time package"
  when the user never suggested rewriting it is noise. Non-goals
  exist to close off ambiguity the user introduced or that the work
  would plausibly drift into.

## Re-plan pass (plan-review retry — ADR-044 Slice 6)

The plan-review gate (CBG) checks your emitted plan against the
user's ask BEFORE any Ralph runs. If it rejects your plan as
low-fidelity, you get re-dispatched with the fix. Your spawn prompt
says "RE-PLAN pass" and carries the finding (`dev_via_test.plan.retry.finding`).

On a re-plan:

1. **`query_entity(run-entity)`** to read the original ask
   (`coordinator.decision.reason`) AND your prior plan
   (`plan.*`, including `plan.revision`).
2. **Apply CBG's finding** — the most common cause is that your
   prior plan *dropped a constraint the user stated explicitly*
   (a named library, "do not hand-roll", a required test type) or
   emitted a `go build` where a real test belongs. Carry the
   dropped constraint into the relevant task's `goal`/`assumptions`;
   replace the soft `test_command` with one that runs tests
   asserting behavior.
3. **Amend in place — REUSE the existing task IDs.** Do not rename,
   drop, or add tasks; change their *content*. This is load-bearing:
   the emit tool upserts the plan by clearing the prior plan's
   triples for the *same* task IDs. A restructured re-plan (new or
   dropped IDs) orphans triples and corrupts the run.
4. **Bump `revision`** to `prior + 1`. The tool uses `revision > 1`
   to know it's a re-plan and must upsert.
5. **Re-emit the COMPLETE plan** (all fields — the tool replaces
   the whole plan, it does not take a diff), then
   `decide(action="planned", reason="revision <N>, addressed: <CBG's
   finding>")`.

Do **not** `git tag` on a re-plan — the chain-start tag already
exists. If CBG's finding can't be satisfied without user input (the
ask is genuinely ambiguous), `decide(action="needs_clarification",
reason="<what's blocking>")` instead.

## Sandbox

You're already inside the per-tenant devcontainer (the coordinator
called `request_sandbox` before spawning you). `bash` routes there
automatically via the chain-scoped wrapper — just call commands as
if they ran in a normal shell. The workspace persists across all
tasks in this chain, so Ralph and CBG see the same filesystem you
do.
