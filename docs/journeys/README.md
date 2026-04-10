# User Journey Specs

This directory contains **user journey specifications** for the semteams
agentic superpowers surface. Each journey is a single markdown file describing
a user-facing capability the product claims to support, written as a
reproducible sequence of user action → expected backend state → expected UI
state.

Journeys are the **source of truth** for what "agentic superpowers" actually
means in semteams. They drive three things:

1. **Documentation** — anyone can read a journey file and understand the
   capability end-to-end without reading code.
2. **Deterministic fixtures** — each journey points at a fixture under
   `test/fixtures/journeys/` that pins mock-llm responses, so the same
   sequence of LLM calls happens every run.
3. **Cross-layer E2E tests** — both Go observer tests (`test/e2e/scenarios/`)
   and Playwright browser tests (`ui/e2e/agentic/`) consume the same fixture
   and assert against the same checklist.

## Format

Every journey file is a markdown document with a YAML frontmatter block. The
frontmatter is machine-checked by `task journeys:validate`; the body is human
prose.

### Required frontmatter fields

```yaml
---
id: tool-approval-gate              # Unique slug, matches filename
version: 1                           # Schema version of THIS document
backend_capabilities:                # Components that must be enabled
  - agentic-dispatch
  - agentic-loop
  - agentic-tools
  - agentic-governance
fixture: tool-approval-gate.yaml     # File under test/fixtures/journeys/
ui_components:                       # Paths to Svelte components exercised
  - ui/src/lib/components/chat/ApprovalPrompt.svelte
  - ui/src/lib/components/chat/AgentLoopCard.svelte
endpoints:                           # HTTP/SSE endpoints touched
  - POST /agentic-dispatch/message
  - GET /agentic-dispatch/activity
  - POST /agentic-dispatch/loops/{id}/signal
---
```

### Required body sections

A journey body must contain these H2 sections, in order:

1. **`## Goal`** — one paragraph stating the user-facing outcome.
2. **`## Preconditions`** — the minimum stack state required (components
   enabled, KV seeded, fixture loaded, user session present).
3. **`## Steps`** — numbered H3 subsections. Each step is one user action
   and its observable consequences.
4. **`## Assertions`** — an explicit checklist the tests verify. Split into
   backend-state and UI-state groups.

### Step format

Each step is an H3 with three bullet sections:

```markdown
### 1. User sends message triggering tool proposal

- **Action:** `POST /agentic-dispatch/message { text: "..." }`
- **Expected backend:** agentic-dispatch creates loop, agentic-loop starts
  executing, SSE emits `loop_update` with `state: executing`.
- **Expected UI:** `AgentLoopCard` appears in the chat stream showing
  `state: executing` and the loop ID.
```

The three bullets are load-bearing — Go observer tests read "Expected
backend" claims, Playwright tests read "Expected UI" claims, and the fixture
is what makes "Action" deterministic.

### Assertion checklist format

```markdown
## Assertions

### Backend state

- [ ] `GET /agentic-dispatch/loops/{id}` returns `state=complete`
- [ ] NATS KV `AGENT_LOOPS/{id}.state == "complete"`
- [ ] NATS KV `RULE_ENGINE` contains the proposed rule

### UI state

- [ ] `AgentLoopCard` with matching `loop_id` visible
- [ ] `ApprovalPrompt` rendered and interactable
- [ ] Post-approval: `ApprovalPrompt` shows `approved` state
- [ ] Post-completion: `AgentLoopCard` shows `complete` state
```

Each checkbox item is a falsifiable claim. If a claim is hard to assert,
it doesn't belong in the checklist — move it to prose in the step body.

## File naming

- Journey files: `docs/journeys/<id>.md` where `<id>` matches the `id:`
  frontmatter field (kebab-case).
- Fixture files: `test/fixtures/journeys/<id>.yaml` — identical stem.

Example: `docs/journeys/tool-approval-gate.md` pairs with
`test/fixtures/journeys/tool-approval-gate.yaml`.

## Validation

```bash
task journeys:validate
```

This target runs on every `task lint` invocation (once Phase C.2 lands and
there are journeys to validate). It checks:

- Every `*.md` file (except `README.md`) has YAML frontmatter
- Required frontmatter fields are present
- The `fixture:` path exists under `test/fixtures/journeys/`
- The `ui_components:` paths exist under the semteams tree
- The `id:` matches the filename stem

It does **not** check that the journey actually passes — that's what the
Playwright and Go observer tests do.

## Planned journeys

The initial superpower set the semteams product claims to support, per
`docs/proposals/agentic-superpowers.md`. This list is aspirational — journeys
land as the backend + UI capabilities stabilize.

| Journey                         | Superpower                            | Status  |
| ------------------------------- | ------------------------------------- | ------- |
| `tool-approval-gate`            | Human-in-the-loop approval gates      | planned |
| `real-time-activity-stream`     | Real-time agent activity streaming    | planned |
| `self-programming-rule-creation`| Self-programming agents (rule writes) | planned |
| `multi-agent-hierarchy`         | Parent/child loop chains              | planned |
| `graph-backed-memory`           | Semantic memory recall                | planned |
| `stock-workflow-deep-research`  | Pre-built deep-research flow          | planned |

## Adding a new journey

1. Pick a slug and create `docs/journeys/<slug>.md` with the frontmatter
   and body sections above.
2. Create the paired fixture at `test/fixtures/journeys/<slug>.yaml` —
   start from an existing fixture as a template.
3. Run `task journeys:validate` to confirm the doc is well-formed.
4. Add the Go observer test under `test/e2e/scenarios/journeys/<slug>/`
   (if the journey has meaningful backend-only assertions).
5. Add the Playwright spec under `ui/e2e/agentic/<slug>.spec.ts`.
6. Update the "Planned journeys" table above, marking the status.

## Rationale

This format exists because the original question — *"who owns Playwright
E2E for the agentic superpowers?"* — turned out to have the wrong shape.
The right question was *"where does the definition of a superpower live?"*

Answer: it lives here, in a repo-root doc, co-located with the code that
implements it and the fixtures that make it reproducible. The journey spec
is the contract. The Go and Playwright tests are independent assertions
against that contract. When a capability evolves, the journey file evolves
in the same commit as the code that changed it, so the contract never drifts
from reality.

See the plan at `.claude/plans/linked-finding-starlight.md` (local-only) for
the full history of how this directory came to be.
