# Agent Delegation

Dev → Review → Fix cycle with quality gates.

## Workflow

```text
architect → developer → reviewer → integration
                ↺ fix cycle
```

## Agents

| Agent | Role | Handoff To |
|-------|------|------------|
| **architect** | API contracts, schemas, ADRs | developer |
| **go-developer** | Go services, TDD, tests | go-reviewer |
| **go-reviewer** | Code review, approval | integration OR back to dev |
| **technical-writer** | Docs, ADRs | — |

## Quality Gates

All 6 gates required before task completion:

| Gate | Owner | Key Checks |
|------|-------|------------|
| **1. Spec** | architect | API contracts, schemas, ADR, success criteria |
| **2. Ready** | developer | Specs reviewed, test strategy, env configured |
| **3. Code** | developer | TDD complete, tests pass, lint clean, self-reviewed |
| **4. Review** | reviewer | Architecture, security, coverage, BLOCKING approval |
| **5. Quality** | automated | Lint, tests, coverage ≥80% critical paths, no races |
| **6. Integration** | tech-writer | Docs updated, deployment ready |

**Status flow**: `not_started → in_progress → pending_review → approved → integrating → complete`

## Handoff Context

Every handoff must include:

- Task ID and description
- Specs/contracts (file paths)
- Dependencies and blockers
- Success criteria
- Files with line references

## Language Requirements

### Go

- `go fmt` + `revive` clean
- `go test -race` passes
- Context as first param
- Error wrapping: `fmt.Errorf("...: %w", err)`
- Table-driven tests

## Review Cycle

1. Developer → Reviewer (Gate 3 complete)
2. Reviewer feedback → Developer fixes
3. Max 3 cycles, then escalate

**Escalate when**: 3+ cycles, spec ambiguity, blocked dependencies, persistent failures

## Handoff Template

```yaml
task_id: FEAT-001
from: go-developer
to: go-reviewer
files:
  - internal/auth/service.go:1-150
  - internal/auth/service_test.go:1-200
gate_3: { tests: pass, lint: pass, coverage: 87% }
notes: OAuth2 flow, context throughout
```
