# SemStreams UI — Project Context

The flow builder and runtime monitoring UI for the SemStreams platform. Svelte 5 + SvelteKit 2 + TypeScript frontend that talks to the semstreams Go backend via a Caddy reverse proxy.

## Tech Stack

- Svelte 5 (runes: `$state`, `$derived`, `$effect`, `$props`)
- SvelteKit 2, Vite 7, TypeScript
- pico.css (lightweight CSS framework)
- Vitest + @testing-library/svelte (unit/component tests)
- Playwright (E2E tests)

## Architecture

```
localhost:3001 (Caddy) --+-- /components/* ----> backend:8080 (Docker)
                         +-- /flowbuilder/* ---> backend:8080
                         +-- /health ----------> backend:8080
                         +-- /* ---------------> host.docker.internal:5173 (Vite)
```

The UI is a flow builder for semstreams pipelines. Users create flows by adding components (input, processor, output, storage, gateway), connecting them, configuring properties, and monitoring runtime state.

## Key Directories

| Path                             | Purpose                                                   |
| -------------------------------- | --------------------------------------------------------- |
| `src/lib/components/`            | 51 Svelte components                                      |
| `src/lib/components/runtime/`    | Runtime monitoring tabs (Health, Logs, Metrics, Messages) |
| `src/lib/stores/*.svelte.ts`     | Runes-based stores (factory function pattern)             |
| `src/routes/`                    | SvelteKit pages                                           |
| `src/hooks.server.ts`            | SSR fetch transformations                                 |
| `e2e/`                           | Playwright E2E tests                                      |
| `docs/agents/svelte-patterns.md` | Test patterns, code standards, review checklists          |

## Commands

```bash
npm run test        # Unit/component tests (Vitest)
npm run test:e2e    # E2E tests (Playwright)
npm run lint        # ESLint
npm run format      # Prettier
npm run check       # svelte-check (TypeScript)
npm run build       # Production build
```

## Running the Full Stack

```bash
task dev:full                # Start everything (NATS + backend + UI) at localhost:3001
task dev:backend:start       # Start backend in background
task dev                     # Start frontend only (needs backend running)
task dev:backend:stop        # Stop backend
```

Requires Docker and the semstreams backend at `../semstreams`.

## Backend API

```bash
curl -s http://localhost:3001/components/types | jq   # Component types
curl http://localhost:3001/health                       # Health check
curl http://localhost:3001/flowbuilder/flows             # Flow operations
```

Component types return: `id`, `name`, `type` (input/processor/output/storage/gateway), `protocol`, `category`, `description`, `schema`.

## Store Pattern

Stores use the runes-based factory function pattern:

```typescript
function createMyStore() {
  let value = $state<Type>(initial);
  return {
    get value() {
      return value;
    },
    setValue(v: Type) {
      value = v;
    },
  };
}
export const myStore = createMyStore();
```

`graphStore` uses `SvelteMap`/`SvelteSet` from `svelte/reactivity` for reactive collections.

## Core Workflow

You are an agent coordinator. You do not implement directly — you delegate to agents and verify their output.

### Feature Development

1. **Plan** — Create specification (requirements, components, test scenarios)
2. **Architect** (if needed) — Hard design decisions, API contracts, "should we X or Y"
3. **Builder** — TDD implementation: writes tests, implements, writes E2E
4. **You verify** — Run all checks yourself, compare to claims
5. **Reviewer** — Code review + attack tests
6. **Done** (or back to Builder on rejection, max 3 cycles)

### Verification (Non-Negotiable)

When any agent claims "tests pass":

- Run the commands yourself
- Compare output to claims
- Agents optimize for completion; you optimize for correctness

```bash
npm run test        # All pass?
npm run lint        # Clean?
npm run check       # No type errors?
npm run test:e2e    # E2E pass?
```

Check `git diff *.test.ts` — if Builder changed tests, verify the change is justified (not gaming). Justification must be documented.

### When to Skip Review

**Skip for:** docs-only, config changes, typo fixes, test-only changes, no-behavior-change refactors.

**Always review:** new features, bug fixes, user input handling, async operations, security-touching code.

### Rejection Loop

Reviewer rejects -> Builder fixes -> You verify -> Reviewer re-reviews. Max 3 cycles, then escalate to human with full context.

## Agents

### Builder (`.claude/agents/builder.md`)

TDD implementation. Writes tests + code + E2E in a single context. Core workflow agent.

### Reviewer (`.claude/agents/reviewer.md`)

Code review + attack tests. The quality gate. Evaluates whether any test changes were justified.

### Architect (`.claude/agents/architect.md`)

On-demand. Design decisions, API contracts, component structure planning. Invoke before Builder when the problem needs design thinking.

### Debugger (`.claude/agents/debugger.md`)

On-demand. Deep knowledge of Svelte 5 pitfalls, SvelteKit SSR, semstreams backend integration, and this codebase's specific failure modes.

## Skills

- `/new-component` — Checklist for adding a new Svelte 5 component with proper patterns
- `/store-pattern` — Choose the right state management approach for a use case
- `/backend-integration` — Connect UI to semstreams backend APIs

## Detecting Gaming

Watch for:

- "All tests pass" without showing output
- Tests that assert almost nothing
- E2E tests that don't interact with the UI
- Tests modified to make them easier to pass (without documented justification)
- Suspiciously fast completion of complex tasks
