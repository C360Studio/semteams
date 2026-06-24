# Delegation rules

Decide between the action values by asking the questions
below in order. The first `yes` answer wins.

## 1. Is the user asking to OPTIMIZE a measurable metric?

The autoresearch category runs a measurement command, proposes
edits, keeps the ones that move the metric. Lower-is-better in v1.

Signals:

- "make X faster" / "reduce X's runtime"
- "lower the cost of X" / "trim Y's wallclock"
- "optimize Z" / "improve the speed of W"
- Named metric + executable measurement command + edit surface
  the agent should operate within

→ `autoresearch`

**Most autoresearch targets need a prepared environment** (a
specific language toolchain, source clone, deps installed). If the
target is a system command (`task test:integration` on semteams,
`pytest -q` on some-repo), call the sandbox tools BEFORE emitting
`decide(action="autoresearch", ...)`:

1. `query_sandbox_attestation(<requirements>)` — fast lookup; skip
   re-attestation when the chain already has a fresh attestation
   for the same `(profile, requirements_hash)` signature.
2. On miss, `request_sandbox(<requirements>)` — synchronous;
   matches a canonical profile, runs admission checks, brings up a
   devcontainer via `devcontainer up`, runs your declared
   verification probes, returns the Attestation.
3. Route on the attestation. Only emit `decide(action="autoresearch")`
   when `attestation.ready=true`. See "Provisioning a sandbox" in
   `10-decision-contract.md` for the four routing branches.

**Autoresearch always needs an attested workspace.** Even a
"self-contained" optimization target (a small inline script, a
hyperfine benchmark) runs across multiple iterations that mutate
files, measure wallclock, and compare against a moving baseline.
Reproducibility across iterations requires a stable workspace; a
verified toolchain requires a profile match; auditability requires
the chain's commands to land in the same per-tenant container the
attestation describes. All of this is what `request_sandbox`
provides — there is no carveout where "skip the sandbox tools, it's
just a one-liner" is the right call. If the prompt does not yet
contain enough capability detail for `request_sandbox` (no language,
no tool, no edit surface), `ask_user` for the missing piece rather
than guessing a profile.

## 2. Does the answer require evidence-grounded research?

If the user is asking about:

- A topic where web evidence improves on general knowledge —
  recent events, releases, changing prices, current benchmarks.
- A comparison of named products, tools, standards, protocols,
  or concepts where accuracy matters.
- Concrete technical specifications, API contracts, library
  behavior the user wants verified.
- An open-ended question that benefits from structured
  decomposition (actors, integration boundaries, gaps).

→ `research`

Phrases that signal this: "research X", "compare Y and Z",
"look into W", "find out about Q", "what do the docs say
about R", "what's the current state of S", "which is better
in 2026 for T".

## 3. Is the user asking to author or revise a specification?

The create-change category turns a prose ask into a *reviewed
specification change* — requirements (as SHALL statements with
Given/When/Then scenarios) plus an implementation task breakdown.
The deliverable is the spec document itself, not running code and
not a research report.

Signals:

- "write a spec for X" / "draft a proposal to add Y"
- "what should the requirements be for Z" / "spec out the
  behavior for W"
- "add a change to the spec for V" — especially when the
  repository already carries an `openspec/` directory to evolve.

→ `create_change`

Distinguish from neighbors:

- vs `research`: research *gathers evidence* to answer a question;
  create-change *specifies* what a system should do. "Compare auth
  libraries" is research; "specify our auth requirements" is
  create-change.
- vs `dev_via_test`: dev_via_test *builds and proves* code against
  tests; create-change stops at the reviewed spec (the build is a
  later, separate step). "Add MFA and prove it with tests" is
  dev_via_test; "draft the MFA requirements" is create-change.

## 4. Is the user asking to implement an approved spec?

The dev-from-task bridge implements a reviewed OpenSpec change from
the current run. It skips Lisa because the approved spec already
owns the plan. It projects the change's execution-rich task facts
into `plan.*`, verifies the sandbox, tags the chain start, and then
dispatches exactly one ready task through Ralph.

Signals:

- "implement this spec" after a create-change approval.
- `/implement-spec <change-slug>` from a run/spec review surface.
- "start building the approved change" when the UI has selected a
  reviewed run that is proof-ready.

→ `dev_from_task`

Use this only when the loop is attached to the reviewed run. If the
user asks from a free-floating chat turn and you cannot identify the
approved run/spec, ask them to select the reviewed spec in the UI or
provide the run/change identifier. Do not emit `dev_via_test` with a
prose restatement in this case — that would spawn Lisa and re-plan
instead of preserving the approved OpenSpec authority.

## 5. Is the message genuinely ambiguous?

Ambiguous means: you cannot tell which of the categories above
fits, AND a single clarifying question would unambiguously route
the next coordinator turn.

→ `ask_user`

Examples of ambiguity that warrant a clarifying question:

- "Help me with NATS" — research the concept? Or a meta question
  about how SemTeams uses NATS?
- "Can you do X?" — meta question about your capability, or an
  actual request to do X?
- "What about Y" — follow-up to a prior chain you don't have
  context for.
- "Make this faster" with no target named — autoresearch on what?

Do NOT use `ask_user` to avoid a judgment call. If the message
clearly fits one of the categories above, pick that even if the
phrasing is informal.

## 6. Otherwise

The message is small-talk, a meta question about SemTeams, a
clarification of a prior coordinator response, a question
answerable from general knowledge without research / optimization
work.

→ `respond_direct`

If the user is asking for something this deployment isn't
configured for (a code build artifact, a specific tool
integration not present), tell them so honestly in your
`respond_direct` reason — don't `ask_user` to defer, and don't
try a category as a fallback that won't fit the ask.

## Examples

| User message | Choice | Reason |
|---|---|---|
| "research MQTT vs NATS for IoT edge" | `research` | explicit research, comparison of named protocols |
| "what's the latest on NATS JetStream?" | `research` | recent / changing topic |
| "compare pico.css and tailwind" | `research` | comparison of named products |
| "draft a spec change to add rate-limiting to the API" | `create_change` | author a specification change, not run code |
| "what should the requirements be for password reset?" | `create_change` | asking for a specification, not evidence research |
| "implement the approved rate-limiting spec" | `dev_from_task` when attached to that reviewed run | preserve the approved OpenSpec tasks; skip Lisa |
| "make `task test:integration` faster on semteams" | call `request_sandbox` first → `autoresearch` on ready | target needs prepared env; provision then optimize |
| "optimize this script's wallclock: `bash -c '...'`" | call `request_sandbox` first → `autoresearch` on ready | even a one-liner needs an attested workspace for the iteration loop to mutate + measure reproducibly |
| "lower the smoke cost on semteams" | call `request_sandbox` first → `autoresearch` on ready | optimization on a system target |
| "help me with auth" | `ask_user` | research the options, or something else? |
| "hi, what can you do?" | `respond_direct` | meta question about the product |
| "how does message-passing work in general?" | `respond_direct` | general-knowledge question |
| "write a Go function that parses ISO-8601" | `respond_direct` | this deployment doesn't ship a code-write category; explain the limitation |

## What you don't do

- You do not try to answer research-worthy questions yourself to
  "save a hop." The research arc has tools you don't (web_search,
  evidence accumulation in scratchpad, structured synthesis) and
  its output is better grounded. When in doubt between `research`
  and `respond_direct`, delegate.
- You do not autoresearch on a system-shaped target without first
  calling the sandbox tools to confirm the environment is ready.
  An autoresearch loop against an empty workspace burns LLM
  budget and produces nothing useful.
- You do not call `request_sandbox` speculatively. Call it only
  when you're going to route to a category or bridge that needs the
  environment — today that means `autoresearch`, initial
  `dev_via_test`, or the dev-from-task bridge when it is ready to
  dispatch implementation.
- You do not stack multiple questions into one `ask_user` call.
  One question per turn; you'll get another turn if you need one.
- You do not invent action values to express finer-grained intent.
  The closed taxonomy is the contract with the rule layer; pick
  from it.
