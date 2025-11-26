<!--
Sync Impact Report - Constitution Update
Version: 2.2.1 → 3.0.0
Date: 2025-11-26

MAJOR VERSION BUMP - Removed Svelte/frontend standards (Go-only project)

Changes:
- Renamed project from "SemMem" to "SemStreams"
- Removed Svelte 5 Standards section entirely
- Removed svelte-developer and svelte-reviewer agent references
- Updated documentation paths from docs/agent/ to docs/agents/
- Updated Go version reference to 1.25.3
- Simplified test types (removed frontend component tests)
- Updated governance docs list to reflect Go-only structure

Rationale:
SemStreams is a Go-only backend/infrastructure project. Svelte standards are not
applicable and were inherited from a different project (SemMem). This major version
bump reflects the breaking change of removing an entire language standard section.

Modified Sections:
- Header: SemMem → SemStreams
- Core Principles > II. TDD: Removed "Component Tests" from test types
- Core Principles > III. Quality Gate Compliance: Removed Svelte-specific patterns
- Core Principles > IV. Code Review Standards: Removed frontend review criteria
- Language Standards: Removed entire Svelte 5 section
- Governance > Runtime Development Guidance: Updated file paths, removed svelte.md reference

Removed Sections:
- Language Standards > Svelte 5 Standards (entire section)

Templates Requiring Updates:
⚠ docs/agents/delegation.md - Should remove Svelte references
✅ plan-template.md - Generic, no changes needed
✅ spec-template.md - Generic, no changes needed
✅ tasks-template.md - Generic, no changes needed

Follow-up TODOs:
- Update docs/agents/delegation.md to remove Svelte agent references
-->

# SemStreams Development Constitution

## Core Principles

### I. Spec-First Development

Every feature begins with a specification using the spec-kit workflow. Implementation
follows the defined phases with formal handoffs between stages.

**Workflow Integration:**

- **Feature Planning**: Use spec-kit phases (spec → plan → research → design → tasks)
- **Implementation**: Apply delegation quality gates within each task
- **Traceability**: Maintain spec → task → issue → PR → code relationships

**Requirements:**

- Specifications MUST define success criteria, dependencies, and constraints
- Plan MUST pass Constitution Check before Phase 0 research begins
- Tasks MUST map to user stories with independent test validation
- No implementation without approved specification and architecture design

**Rationale:** Spec-driven development ensures clear requirements, reduces rework,
and enables semantic traceability across the entire development lifecycle.

### II. Test-Driven Development (TDD) (NON-NEGOTIABLE)

TDD is mandatory for all implementation work. Tests MUST be written first, verified
to fail, then implementation proceeds to make tests pass (Red-Green-Refactor cycle).

**Requirements:**

- Write tests BEFORE implementation (verify RED state)
- Tests MUST pass before task completion (GREEN state)
- Refactor with passing tests as safety net
- Focus on testing behavior and outcomes, not implementation details
- Prioritize critical paths and edge cases over coverage metrics
- Unit tests for all functionality exposing public APIs
- Integration tests for cross-component interactions and external dependencies

**Test Types:**

- **Unit Tests**: Table-driven tests with `-race` flag (see docs/agents/go.md)
- **Integration Tests**: Cross-service, database, NATS JetStream, external APIs
- **Contract Tests**: API endpoint and GraphQL schema validation

**Rationale:** TDD catches bugs early, improves design, provides living documentation,
and ensures every line of code has a test justifying its existence.

### III. Quality Gate Compliance

All code MUST pass through six quality gates before integration. Each gate has
specific checklist requirements and formal handoff procedures.

**The Six Gates:**

1. **Specification Completeness** (architect → developer)
   - API contracts, data models, integration points defined
   - Success criteria, dependencies, security considerations documented

2. **Implementation Readiness** (developer preparation)
   - Specifications reviewed, dependencies available
   - Testing strategy defined, linting tools configured

3. **Code Completion** (developer → reviewer handoff REQUIRED)
   - All functional requirements implemented following TDD
   - Go linting passes: `go fmt` + `revive` (zero errors)
   - Unit and integration tests written and passing with `-race` flag
   - Self-review completed with inline documentation
   - **Developer MUST immediately handoff to reviewer - NO exceptions**

4. **Code Review** (reviewer validation - BLOCKING)
   - Architecture compliance, security, performance verified
   - Test coverage adequate for critical paths (minimum 80%)
   - Go patterns validated: context management, error handling, concurrency
   - **NO task may proceed to Gate 5 without explicit reviewer approval**
   - Reviewer MUST provide feedback or approval (cannot be skipped)

5. **Quality Validation** (automated checks - AFTER Gate 4 approval)
   - All linters pass with zero errors (`go fmt`, `revive`)
   - All tests pass with `-race` flag (unit + integration)
   - Test coverage meets minimums (80% critical paths)
   - No race conditions detected

6. **Integration Readiness** (final deployment)
   - Documentation updated, deployment procedures documented
   - Feature flags configured, monitoring instrumented

**Fix Cycle:** Developer → Reviewer → Fix → Re-review (maximum 3 cycles before
escalation to architect or program manager)

**Task Completion:** Tasks in tasks.md are ONLY marked complete [x] after ALL
six gates pass. Checking task box without reviewer approval (Gate 4) is a
violation of this constitution.

**Rationale:** Structured quality gates catch different classes of issues at
appropriate stages, reducing defects and technical debt. Immediate review handoff
prevents shortcuts and ensures quality is built in, not inspected in later.

### IV. Code Review Standards

All code MUST be reviewed and approved before integration. Reviews verify
architecture compliance, security, performance, and quality standards.

**Reviewer Responsibilities:**

- Verify all quality gate checklist items from Gate 3
- Check architecture patterns and design consistency
- Validate security (OWASP Top 10), performance, error handling
- Ensure test coverage adequate for critical paths
- Confirm Go patterns: context as first param, error wrapping, channel usage
- Provide specific feedback with file:line references

**Review Outcomes:**

- **APPROVED**: Proceed to automated quality validation (Gate 5)
- **CHANGES REQUESTED**: Return to developer with specific feedback
- **BLOCKED**: Escalate to architect or program manager with reasoning

**Batched Feedback:** Provide ALL feedback in first review to avoid multiple cycles

**Rationale:** Code review is the primary quality control mechanism, catching issues
that automated tools miss and ensuring knowledge transfer across the team.

### V. Documentation & Traceability

Documentation MUST be maintained alongside code. All decisions, designs, and
implementations MUST be traceable from spec to deployment.

**Requirements:**

- **API Documentation**: All public APIs and GraphQL schemas documented
- **ADRs**: Architecture decisions recorded for significant choices
- **README**: Project overview, quick start, and contribution guide current
- **Inline Comments**: Explain "why" for complex logic, not "what"
- **Traceability**: Maintain spec → task → issue → PR → code links
- **Commit Messages**: Conventional commits format: `<type>(scope): subject`

**Documentation Hierarchy:**

- `/docs/` - Architecture, integration guides, strategic vision
- `/docs/agents/` - Agent delegation and handoff templates
- `.specify/` - Spec-kit templates, constitution, memory
- `specs/[feature]/` - Feature specifications, plans, designs

**Rationale:** Documentation enables onboarding, debugging, and maintenance.
Traceability ensures every code change has clear justification and context.

## Development Workflow

The development workflow integrates spec-kit's feature-driven approach with
delegation's quality gate implementation process.

### Feature Workflow (Spec-Kit)

1. **Specification** (`/speckit.specify`) - Define feature requirements
2. **Planning** (`/speckit.plan`) - Architecture and technical design
3. **Task Generation** (`/speckit.tasks`) - Break down into actionable tasks
4. **Implementation** (`/speckit.implement`) - Execute tasks with quality gates
5. **Analysis** (`/speckit.analyze`) - Cross-artifact consistency validation

### Implementation Workflow (Delegation Gates)

Within each task from spec-kit, apply the six quality gates with explicit status tracking:

**Task Status Progression:**

```text
not_started
    ↓
in_progress (Gates 1-2: Spec complete, ready to code)
    ↓
pending_review (Gate 3: Code complete, MUST handoff to reviewer)
    ↓
in_rework (Gate 4: Reviewer requested changes) ──┐
    ↓                                              │
pending_review (Fixes submitted for re-review) ───┘
    ↓
approved (Gate 4: Reviewer approved)
    ↓
integrating (Gates 5-6: Automated checks, deployment prep)
    ↓
complete (All 6 gates passed, task checkbox marked [x])
```

**Phase 1: Specification → Implementation**

- Gate 1: Specification Completeness (architect → developer handoff)
- Gate 2: Implementation Readiness (developer preparation)
- **Status**: not_started → in_progress

**Phase 2: Implementation → Review (CRITICAL HANDOFF)**

- Gate 3: Code Completion (developer → reviewer handoff with TDD artifacts)
  - **Status**: in_progress → pending_review
  - **REQUIRED**: Developer MUST immediately handoff to reviewer
  - **FORBIDDEN**: Proceeding to Gate 5 without Gate 4 approval

- Gate 4: Code Review (reviewer approval or change requests)
  - **Status**: pending_review → approved OR in_rework
  - **If changes requested**: in_rework → pending_review (after fixes)
  - **Maximum**: 3 review cycles before escalation
  - **BLOCKING**: Nothing proceeds until reviewer approval

**Phase 3: Review → Integration (AFTER Gate 4 approval)**

- Gate 5: Quality Validation (automated linting and testing)
  - **Status**: approved → integrating
  - **Prerequisite**: Gate 4 approval MUST be received

- Gate 6: Integration Readiness (documentation and deployment prep)
  - **Status**: integrating → complete
  - **Task checkbox**: NOW marked [x] in tasks.md

### Context Passing Between Agents

All agent handoffs MUST include:

- Task context (ID, description, priority, complexity)
- Specifications (API contracts, data models, business rules)
- Dependencies (completed tasks, deliverables, blockers)
- Success criteria (functional and non-functional requirements)
- Go-specific context (reference docs/agents/ for standards)
- **Current status**: To enable proper status tracking

See `docs/agents/delegation.md` for detailed handoff templates and checklists.

## Language Standards

Language-specific standards are enforced at Quality Gate 3 (Code Completion),
Gate 4 (Code Review), and Gate 5 (Quality Validation). This section defines
WHAT must be validated; see `docs/agents/` for HOW to implement patterns.

### Go Standards

**Version**: Go 1.25+

**Required Quality Checks:**

- Code MUST pass `go fmt` (zero changes)
- Code MUST pass `revive` linting (zero errors)
- All tests MUST pass with `-race` flag (zero data races)
- Critical path test coverage MUST be ≥80%

**Required Patterns:**

- **Context Management**: Context as first parameter for I/O operations
- **Concurrency Safety**: Proper use of channels, mutexes, atomics
- **Error Handling**: Return errors with context wrapping (`fmt.Errorf("...: %w", err)`)
- **Testing**: Table-driven tests, explicit synchronization

**Rationale:** Go standards ensure code quality, concurrency safety, and
maintainability across the NATS JetStream and GraphQL service architecture.

### Adding New Languages

When adding a new language to the project:

1. Create `docs/agents/[language].md` with comprehensive patterns and examples
2. Add language section here with required quality checks and pattern references
3. Update quality gate checklists in `docs/agents/delegation.md`
4. Configure linting tools and CI/CD checks

## Governance

### Constitution Authority

This constitution supersedes all other development practices, guidelines, and
conventions. When conflicts arise, constitution principles take precedence.

### Amendment Process

Constitution amendments require:

1. Documented proposal with justification and impact analysis
2. Review by architect and program manager
3. Approval from project maintainers
4. Migration plan for affected code and documentation
5. Update to all dependent templates and guidance documents
6. Version increment following semantic versioning

### Version Semantics

- **MAJOR**: Backward incompatible principle changes, removals, or redefinitions
- **MINOR**: New principles added or materially expanded guidance
- **PATCH**: Clarifications, wording improvements, typo fixes

### Compliance Review

- All PRs MUST verify compliance with constitution principles
- Code reviews MUST check adherence to quality gates
- Quarterly constitution review to assess effectiveness
- Update guidance documents when patterns emerge

### Complexity Justification

Any violation of constitution principles MUST be justified in `plan.md` Complexity
Tracking section with:

- Specific violation description
- Why the violation is necessary
- Why simpler alternatives were rejected
- Approval from architect

### Runtime Development Guidance

For detailed implementation guidance, see:

- `docs/agents/delegation.md` - Quality gate checklists and handoff templates

**Version**: 3.0.0 | **Ratified**: 2025-01-23 | **Last Amended**: 2025-11-26
