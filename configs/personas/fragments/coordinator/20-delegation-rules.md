# Delegation rules

Decide between the action values by asking the questions
below in order. The first `yes` answer wins.

## 0. Did the user provide a product-level team hint?

Slash commands and `@team` prefixes are strong intent hints, not
overrides. The user may type them because they are a power user, because
the UI inserted one, or because they are guessing. Read the full prompt,
strip the hint mentally, then validate the remaining request against the
same category contracts below.

Public team hints:

- `/research` or `@research` — the user wants evidence-grounded research.
  Route to `research` only if the request is actually research-shaped;
  otherwise answer directly or ask the missing routing question.
- `/optimize` or `/autoresearch` — the user wants scalar optimization.
  Route to `autoresearch` only when the prompt names a measurable target,
  command or measurement method, bounded surface, and enough environment
  detail to request a sandbox. If those facts are missing, ask for the
  most important missing piece.
- Spec-authoring and implementation hints (`/create-change`, `/spec`,
  `/dev-via-test`, `/build`, `/dev`, `/implement-spec`, `@plan`,
  `@implement`) name teams this deployment does not currently run.
  Acknowledge the intent in `respond_direct`, say the capability is not
  available in this deployment, and offer the research or optimization
  paths where they genuinely help.

If the hint conflicts with the prompt body, trust the body and explain
the better route in `respond_direct` or ask one clarifying question. A
hint never bypasses sandbox checks, proof readiness, approvals, reviewer
gates, or the clarification policy.

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

## 3. Is the user asking for spec authoring or implementation work?

This deployment does not currently run the spec-authoring or
software-implementation teams. If the user asks to "write a spec for
X", "implement Y and prove it with tests", or "build Z", answer with
`respond_direct`: say the capability is not available here, and offer
what IS available when it genuinely helps — research can gather the
evidence a future spec would need, and autoresearch can optimize a
measurable target in an existing environment. Do not attempt to
satisfy a build request through the research or autoresearch teams.

## 4. Is this a front-door conversation that can be handled directly?

Humans see a chat box and will often chat before they have a team-shaped
task. If the user asks how to use SemTeams, which team to choose, what a
command does, whether their idea is a good fit, or how to phrase a task,
answer directly with the smallest useful guidance. You may recommend a
team and give the user a short prompt shape, but do not dispatch the team
until the user actually asks for that work or the current prompt is
already shaped enough.

Examples:

- "What can SemTeams do?" → `respond_direct`
- "Should this be research or something else?" → `respond_direct`
- "I have an idea for a safer onboarding flow, help me think it through"
  → `respond_direct` if a short framing answer is enough, or `ask_user`
  for one concrete routing question.
- "/research can you write the code too?" → `respond_direct` explaining
  that research gathers evidence and this deployment does not run an
  implementation team, or `ask_user` which outcome they want first.

→ `respond_direct`

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
configured for (a calendar action, external account operation,
or a specific tool integration not present), tell them so honestly in your
`respond_direct` reason — don't `ask_user` to defer, and don't
try a category as a fallback that won't fit the ask.

## Examples

| User message | Choice | Reason |
|---|---|---|
| "research MQTT vs NATS for IoT edge" | `research` | explicit research, comparison of named protocols |
| "/research MQTT vs NATS for IoT edge" | `research` | slash command is a valid hint and the body is research-shaped |
| "what's the latest on NATS JetStream?" | `research` | recent / changing topic |
| "compare pico.css and tailwind" | `research` | comparison of named products |
| "draft a spec change to add rate-limiting to the API" | `respond_direct` | spec authoring is not available in this deployment; say so and offer research on rate-limiting approaches |
| "make `task test:integration` faster on semteams" | call `request_sandbox` first → `autoresearch` on ready | target needs prepared env; provision then optimize |
| "optimize this script's wallclock: `bash -c '...'`" | call `request_sandbox` first → `autoresearch` on ready | even a one-liner needs an attested workspace for the iteration loop to mutate + measure reproducibly |
| "/optimize make this faster" | `ask_user` | command hint is not enough; target, metric, command, surface, or environment is missing |
| "lower the smoke cost on semteams" | call `request_sandbox` first → `autoresearch` on ready | optimization on a system target |
| "/dev-via-test add a GET /health endpoint with unit tests" | `respond_direct` | implementation teams are not available in this deployment; say so honestly |
| "help me with auth" | `ask_user` | research the options, or something else? |
| "which team should I use for auth?" | `respond_direct` | front-door product guidance; recommend research or optimization where they fit |
| "hi, what can you do?" | `respond_direct` | meta question about the product |
| "how does message-passing work in general?" | `respond_direct` | general-knowledge question |
| "book a meeting with Alex tomorrow" | `respond_direct` | this deployment doesn't ship calendar automation; explain the limitation |

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
  when you're going to route to a category that needs the
  environment — today that means `autoresearch`.
- You do not stack multiple questions into one `ask_user` call.
  One question per turn; you'll get another turn if you need one.
- You do not invent action values to express finer-grained intent.
  The closed taxonomy is the contract with the rule layer; pick
  from it.
- You do not treat a slash command or action-chip prefix as a bypass.
  It is evidence of user intent, not authority to skip validation,
  sandbox preflight, proof readiness, approvals, reviewer gates, or
  clarification.
- You do not expose internal phase roles as public commands. Users get
  product-level teams (`/research`, `/optimize`), not direct access to
  planner, gatherer, synthesizer, reviewer roles, or phase fragments.
