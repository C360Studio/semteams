# SemStreams Development Constitution

## Core Principles

### 1. Spec Before Code

No implementation without an approved specification. Spec-kit defines requirements
(`spec.md`), architecture (`plan.md`), and task breakdown (`tasks.md`). Code
implements specs; specs don't document code.

### 2. Quality Is Built In

Quality is structural, not inspected. Enforced through:

- **Separation**: Tester writes unit tests, Builder implements (can't modify unit tests)
- **Verification**: Main Agent runs checks, doesn't trust claims
- **Review + Attack**: Reviewer examines code and tries to break it before acceptance

Code isn't "done" until tests pass, lints are clean, and Reviewer approves.

### 3. TDD Is Required

Red → Green → Refactor. Enforced structurally through two-agent TDD:

- **Tester** writes unit tests before implementation exists (Red)
- **Builder** makes unit tests pass without modifying them (Green)
- Builder refactors with tests as safety net (Refactor)

Unit tests are written by one agent, implementation by another. Builder cannot
edit Tester's unit test files. This separation makes TDD unavoidable, not optional.

### 4. Adversarial Review

Every substantive implementation is reviewed and attacked before acceptance.
Reviewer performs code review against Go standards, then writes attack tests
targeting gaps: nil inputs, concurrency, cancellation, edge cases.

Reviewer's attack tests are permanent — they run in CI forever, with the same
locked status as Tester's unit tests. Passing review means surviving scrutiny
and attack, not satisfying a checklist.

Rejection returns to Builder with specific issues (which Builder cannot modify
if they're in locked test files). Maximum 3 rejection cycles before escalation
to human.

### 5. Traceability

Every artifact links to its parent: Spec → Plan → Task → Test → Code.
Decisions are recorded. Changes reference their justification.

---

## Test Ownership

| Test Type | Owner | Locked? | Build Flag |
| ----------- | ------- | --------- | ------------ |
| Unit tests | Tester | Yes | (none) |
| Integration tests | Builder | No | `//go:build integration` |
| Attack tests | Reviewer | Yes | (none) |

**Unit tests** define the behavioral contract. Locked because they're the spec.

**Integration tests** verify real infrastructure (NATS, DB). Unlocked because
infra setup varies and Builder needs to adapt. Tester provides requirements,
Builder implements.

**Attack tests** find gaps in coverage. Locked because they're adversarial
verification that Builder shouldn't be able to weaken.

---

## Workflow

Spec-kit handles requirements and design. Agents handle implementation and verification.

### Spec-kit Phase (Main Agent)

```text
/speckit.specify → specs/[feature]/spec.md      (requirements)
/speckit.plan    → specs/[feature]/plan.md      (architecture)
/speckit.tasks   → specs/[feature]/tasks.md     (task breakdown)
```

### Agent Phase (Two-Agent TDD)

| Agent | Owns | Constraint |
| ------- | ------ | ------------ |
| **Tester** | Unit tests, integration requirements | Unit tests locked |
| **Builder** | Implementation, integration tests | Cannot modify unit tests |
| **Reviewer** | Code review, attack tests | Review + attack, blocks on failure |

```text
For each task:
    Tester → unit tests (locked) + integration requirements
        ↓
    Builder → implementation + integration tests
        ↓
    Main Agent → verifies (task test, task test:int, task lint)
        ↓
    Reviewer → review + attack (skip for docs/config/trivial)
        ↓
    Accept or reject (max 3 cycles, then escalate)
```

### Test Disputes

If Builder claims a test is wrong:

1. Main Agent reviews test vs spec
2. If test is wrong → Tester fixes (unit) or Builder fixes (integration)
3. If test is right → Builder implements

Builder cannot modify unit tests or attack tests. They can only dispute through Main Agent.

---

## Quality Standards

### Go

- Lint passes with no warnings
- All unit and integration tests pass with race checking
- All packages have a go.doc and README that are up to date

### Required Tooling

All commands defined in `Taskfile.yml`:

Agents must discover and use Taskfile commands.

---

## Task Status

Four states only:

| Status | Meaning |
| -------- | --------- |
| `todo` | Not started |
| `in_progress` | Builder working |
| `review` | Reviewer testing |
| `done` | Reviewer approved |

Tasks are marked `done` only after Reviewer approval.

---

## Documentation

| Location | Content |
| ---------- | --------- |
| `specs/[feature]/` | Specifications, plans, task breakdowns |
| `docs/agents/go-patterns.md` | Shared Go patterns and examples |
| `docs/` | ADRs and system design |
| `Taskfile.yml` | All build/test/lint commands |

Commit format: `<type>(scope): subject`

---

## Governance

### Authority

This constitution defines principles. Agent prompts define enforcement.
When conflicts arise, principles take precedence.

### Amendments

Changes require:

1. Proposal with justification
2. Review by architect
3. Update to affected agent prompts
4. Version increment (MAJOR.MINOR.PATCH)

### Exceptions

Principle violations must be documented in `plan.md` with:

- What principle is violated
- Why it's necessary
- Architect approval
