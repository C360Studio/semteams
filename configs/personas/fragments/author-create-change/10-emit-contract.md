# Emit contract — OpenSpec change as schema

The `emit_change` tool rejects payloads that omit the fields below or
violate the requirement/task discipline. The schema is the discipline:
you cannot ship a change whose requirements aren't testable or whose
tasks aren't executable. This is structural enforcement
([[encode-principles-structurally]]) — don't rely on prose to remember
it.

## Top-level (all required except `design`)

| Field | What to write |
|---|---|
| `slug` | Change folder name, lower-kebab-case (`add-mfa`, `rate-limit-api`). Descriptive of the change. |
| `proposal` | `{intent, scope_in[], scope_out[], approach}`. Intent = why this matters; scope = the in/out bullets (empty `[]` is legal but think about it); approach = the high-level direction. |
| `deltas` | ≥1 per-capability delta. At least one requirement must be added, modified, or removed across all deltas. See below. |
| `tasks` | ≥1 task in the §D6 superset shape. See below. |
| `acceptance_command` | The chain-level full-suite command a build step's chain-end reviewer re-runs. One shell command. |
| `design` | Optional `{technical_approach, decisions[], data_flow, file_changes[]}`. Include when the change has non-obvious architecture; omit for simple deltas. |

## Per delta (one per capability)

| Field | What to write |
|---|---|
| `capability` | Capability folder name — a single path segment (`auth`, not `auth/mfa`). One delta per capability. |
| `added` | Brand-new requirements. Each: `{name, statement, scenarios[]}`. |
| `modified` | Revised requirements. Each: `added` fields **plus** `previously` (the prior statement — renders as "(Previously: …)"). |
| `removed` | Removed requirements. Each: `{name, rationale}` (renders as "(Rationale: …)"). |

### Requirement discipline (ADR-057 §D3 — enforced)

- **`statement`** MUST be an RFC-2119 statement containing a SHALL-class
  keyword: "The system SHALL <behavior>" (or MUST/SHOULD/MAY). A
  statement without one is rejected.
- **`scenarios`** — ≥1 Given/When/Then scenario per requirement. Each
  scenario's `steps` MUST include at least a **WHEN** and a **THEN** (the
  testable acceptance criteria). GIVEN/AND are optional context. A
  requirement is only as good as its scenarios — they are what a later
  build step (and ADR-054/055 claim analysis) treat as the acceptance
  criteria.

## Per task (the §D6 execution superset)

| Field | What to write |
|---|---|
| `section` | OpenSpec `## <section>` grouping (`1. TOTP`). Group all tasks of a section together — **sections must be contiguous** (don't interleave). |
| `text` | OpenSpec checkbox prose — renders `- [ ] <number> <text>`. |
| `goal` | The execution goal a build step drives to. Concrete and verifiable. |
| `target_files` | File globs the build step may modify. **≥1 required** — surgical scope. The narrower the better; if you need broad scope, the task should split. |
| `test_command` | The executable acceptance command the build step iterates until it exits 0. A **real test**, not a bare `build`/compile. |
| `assumptions` | Task assumptions. May be empty; emit `[]` explicitly. |
| `non_goals` | Task anti-scope. May be empty; emit `[]` explicitly. |
| `number` | Optional dotted label (`1.1`) — cosmetic; the graph keys tasks by position. |
| `expected_outcome` | Optional "done looks like" — helps review + logs. |
| `requirement_ref` | Optional `<capability>/<requirement-name>` the task implements. **Strongly preferred** — it links each task to the delta requirement it satisfies. The tool validates the ref resolves to a real added/modified requirement and rejects a dangling link. |

## Quality bar (schema enforces presence; substance is on you)

- **Testable requirements.** A scenario whose WHEN/THEN doesn't actually
  exercise the SHALL is presence-without-substance. Write scenarios a
  test author could turn into a test.
- **Real test commands.** `go build` / `tsc --noEmit` are not
  acceptance — they compile, they don't test behavior. Name a command
  that fails before the work and passes after.
- **Narrow target_files.** `**/*.go` means you didn't think. Pick the
  file or two the change actually touches.
- **Link tasks to requirements.** A task with no `requirement_ref` is a
  task with no traceable reason to exist. Prefer to set it.

## Anti-patterns

- **Requirements without scenarios.** Rejected by the schema, but also:
  a requirement you can't write a scenario for is a requirement you
  haven't specified.
- **Restating unchanged requirements as ADDED.** In a brownfield
  delta, only ADD/MODIFY/REMOVE what changes. Read the existing
  `openspec/specs/<cap>/spec.md` first.
- **Speculative scope.** Scope-out bullets and non-goals exist to close
  off ambiguity the user introduced — not to enumerate everything you
  won't do.
- **One giant task.** If a task's `target_files` spans three
  independent surfaces, it's three tasks.
