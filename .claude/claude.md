# SemStreams UI — Project Context

Graph explorer, flow builder, and runtime monitoring UI for the SemStreams platform. Svelte 5 + SvelteKit 2 + TypeScript frontend that connects to any running SemStreams app via a Caddy reverse proxy.

## Tech Stack

- Svelte 5 (runes: `$state`, `$derived`, `$effect`, `$props`)
- SvelteKit 2, Vite 7, TypeScript
- Sigma.js + Graphology (WebGL graph visualization)
- Lightweight CSS (custom design tokens, no framework)
- Vitest + @testing-library/svelte (3099+ unit/component tests)
- Playwright (E2E tests)

## Architecture

```
localhost:3001 (Caddy) --+-- /components/* ----> backend:8080
                         +-- /flowbuilder/* ---> backend:8080
                         +-- /health ----------> backend:8080
                         +-- /graphql ---------> graphql-gateway
                         +-- /* ---------------> Vite:5173
```

The homepage is a **graph explorer** (DataView) with chat. Users explore the knowledge graph, search entities, and interact with an AI assistant. Flow management is available at `/flows`.

### Pages

| Route         | Purpose                                                                  |
| ------------- | ------------------------------------------------------------------------ |
| `/`           | Graph explorer (DataView) — default homepage, auto-discovers active flow |
| `/flows`      | Flow list — create and manage flows                                      |
| `/flows/[id]` | Flow editor — visual canvas, chat, runtime monitoring                    |

## Key Directories

| Path                             | Purpose                                                                   |
| -------------------------------- | ------------------------------------------------------------------------- |
| `src/lib/components/`            | 51 Svelte components                                                      |
| `src/lib/components/chat/`       | Chat system (ChatPanel, SlashCommandMenu, ContextChips, attachment cards) |
| `src/lib/components/runtime/`    | Runtime tabs (Health, Logs, Metrics, Messages, SigmaCanvas)               |
| `src/lib/services/`              | API clients (chatApi, graphApi, slashCommands)                            |
| `src/lib/server/`                | SvelteKit server (AI providers, tool registry, MCP, GraphQL client)       |
| `src/lib/stores/*.svelte.ts`     | Runes-based stores (graphStore, chatStore, runtimeStore)                  |
| `src/lib/types/`                 | TypeScript types (chat, graph, flow, slashCommand)                        |
| `src/routes/`                    | SvelteKit pages                                                           |
| `e2e/`                           | Playwright E2E tests                                                      |
| `docs/agents/svelte-patterns.md` | Test patterns, code standards, review checklists                          |

## Commands

```bash
npm run test        # Unit/component tests (Vitest)
npm run test:e2e    # E2E tests (Playwright)
npm run lint        # ESLint
npm run format      # Prettier
npm run check       # svelte-check (TypeScript)
npm run build       # Production build
```

## Running the App

```bash
# Connect to any running SemStreams app (recommended)
task dev:connect                                    # localhost:8080
BACKEND_HOST=myapp:8080 task dev:connect            # remote host

# Full stack from source (requires Docker + ../semstreams)
task dev:full                                       # localhost:3001

# Individual control
task dev:backend:start      # Start backend in background
task dev                    # Start Vite dev server only
task dev:backend:stop       # Stop backend
```

## Backend API

```bash
curl -s http://localhost:3001/components/types | jq   # Component catalog
curl http://localhost:3001/health                       # Health check
curl http://localhost:3001/flowbuilder/flows             # Flow operations
curl -X POST http://localhost:3001/graphql \
  -H 'Content-Type: application/json' \
  -d '{"query":"{ entitiesByPrefix(prefix: \"\", limit: 5) { id } }"}'  # GraphQL
```

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

## Chat System

The contextual AI assistant supports:

- **Slash commands** — `/search`, `/flow`, `/explain`, `/debug`, `/health`, `/query`
- **Context chips** — Pin entities/components to chat via "+Chat" buttons
- **Page-aware tools** — Different tools on flow-builder vs data-view pages
- **Attachment-based messages** — `MessageAttachment` union: flow, search-result, entity-detail, error, health, flow-status
- **Multi-provider AI** — Anthropic or OpenAI-compatible (env: `AI_PROVIDER`)

Key types: `ChatPageContext`, `ContextChip`, `ChatIntent`, `SlashCommand`, `MessageAttachment`

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
