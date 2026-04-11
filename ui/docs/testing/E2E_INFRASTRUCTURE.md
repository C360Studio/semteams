# E2E Test Infrastructure

## Overview

The SemTeams UI E2E tests run against a **full Docker stack** including:

- NATS server (message broker)
- semteams backend (built from the repo root)
- UI (SvelteKit in dev mode)
- Caddy (reverse proxy routing)

**IMPORTANT**: Do NOT mock the backend. The E2E environment provides a real
semteams backend running in Docker.

## Quick Start

### Option 1: Using Taskfile (RECOMMENDED)

From the semteams repo root:

```bash
task ui:test:e2e
```

Or from `semteams/ui/`:

```bash
task test:e2e
```

Both automatically:

- Start the Docker stack via `docker-compose.e2e.yml` (builds the semteams
  backend from `../..` — the semteams repo root)
- Run Playwright tests
- Clean up the Docker stack via `playwright.teardown.ts`

### Option 2: Manual (for debugging)

```bash
# Start the E2E stack manually
docker compose -f docker-compose.e2e.yml up --build

# In another terminal, run tests
npm run test:e2e

# Cleanup when done
npm run test:e2e:cleanup
```

## Architecture

```
┌─────────────────────────────────────────────┐
│ Host Machine (where Playwright runs)       │
│                                             │
│  Playwright Tests                          │
│       │                                     │
│       │ http://localhost:3000              │
│       ↓                                     │
├─────────────────────────────────────────────┤
│ Docker Network (e2e-net)                   │
│                                             │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐ │
│  │  Caddy   │→ │    UI    │→ │ Backend  │ │
│  │  :3000   │  │  :5173   │  │  :8080   │ │
│  └──────────┘  └──────────┘  └────┬─────┘ │
│                                    │        │
│                              ┌─────┴─────┐  │
│                              │   NATS    │  │
│                              │   :4222   │  │
│                              └───────────┘  │
└─────────────────────────────────────────────┘
```

### Service Details

| Service     | Purpose                       | Health Check                  | Exposed Port    |
| ----------- | ----------------------------- | ----------------------------- | --------------- |
| **nats**    | Message broker with JetStream | http://localhost:8222/healthz | (internal only) |
| **backend** | semteams backend              | http://localhost:8080/health  | (internal only) |
| **ui**      | SvelteKit dev server          | N/A                           | (internal only) |
| **caddy**   | Reverse proxy                 | http://localhost:3000/health  | **3000** (host) |

**Key Point**: Only Caddy is exposed to the host. It routes:

- `/flowbuilder/*` → backend:8080
- `/*` → ui:5173

## Configuration

### Defaults

The E2E stack builds the semteams backend from `../..` (the semteams repo root
relative to `ui/`) using `docker/Dockerfile`. No configuration is needed for
the default case:

```bash
task ui:test:e2e   # from semteams/ repo root
```

### Overrides (for debugging or alternate configs)

You can override the build context and config via environment variables:

```bash
# Different config file
BACKEND_CONFIG=hello-world.json docker compose -f docker-compose.e2e.yml up

# Different backend path (e.g., a sibling checkout of semteams)
BACKEND_CONTEXT=/path/to/alt/semteams docker compose -f docker-compose.e2e.yml up
```

### Environment variable defaults

| Variable             | Default          | Purpose                                      |
| -------------------- | ---------------- | -------------------------------------------- |
| `BACKEND_CONTEXT`    | `../..`          | Docker build context (semteams repo root)    |
| `BACKEND_DOCKERFILE` | `docker/Dockerfile` | Dockerfile path relative to BACKEND_CONTEXT |
| `BACKEND_CONFIG`     | `protocol-flow.json` | Config file under `configs/`              |

## Playwright Configuration

### playwright.config.ts

```typescript
webServer: {
  command: 'docker compose -f docker-compose.e2e.yml up --build',
  url: 'http://localhost:3000/health',  // Wait for Caddy
  timeout: 120000,  // 2 minutes for Docker build + startup
  reuseExistingServer: !process.env.CI,  // Reuse in dev, rebuild in CI
}
```

**Important**: Playwright automatically manages the Docker stack:

- **Before tests**: Starts `docker compose up`
- **During tests**: Keeps stack running
- **After tests**: Runs `playwright.teardown.ts` to cleanup

### playwright.teardown.ts

```typescript
export default async function globalTeardown() {
  execSync("docker compose -f docker-compose.e2e.yml down", {
    stdio: "inherit",
  });
}
```

## Writing E2E Tests

### ✅ DO: Test Against Real Backend

```typescript
test("should create and run flow", async ({ page }) => {
  await page.goto("/");

  // Real backend API call happens here
  await page.click('[data-testid="create-flow"]');

  // Backend creates flow in NATS JetStream
  await expect(page.locator('[data-testid="flow-canvas"]')).toBeVisible();

  // Backend starts flow components
  await page.click('[data-testid="run-flow"]');

  // Backend reports status via real endpoint
  await expect(page.locator('[data-testid="status"]')).toHaveText("Running");
});
```

### ❌ DON'T: Mock Backend API

```typescript
// ❌ WRONG - Backend is REAL, don't mock it!
test('should create flow', async ({ page }) => {
  await page.route('**/flowbuilder/flows', route => {
    route.fulfill({ status: 200, body: JSON.stringify({...}) });
  });
  // ...
});
```

### Testing Runtime Endpoints

All runtime endpoints are REAL and functional:

```typescript
test("should stream logs via SSE", async ({ page }) => {
  await page.goto("/flows/test-flow-id");

  // Open runtime panel
  await page.click('[data-testid="runtime-panel-toggle"]');

  // Click Logs tab
  await page.click('[data-testid="tab-logs"]');

  // SSE connection to backend:8080/flowbuilder/flows/{id}/runtime/logs
  await expect(page.locator('[data-testid="logs-status"]')).toHaveText(
    "Connected",
  );

  // Real logs stream from backend
  await expect(page.locator('[data-testid="log-entry"]')).toHaveCount(
    greaterThan(0),
  );
});
```

## Environment Variables

All environment variables have sensible defaults for the semteams stack — no
configuration required for the standard case. See the Configuration section
above for available overrides.

| Variable             | Default              | Purpose                         |
| -------------------- | -------------------- | ------------------------------- |
| `BACKEND_CONTEXT`    | `../..`              | semteams repo root              |
| `BACKEND_DOCKERFILE` | `docker/Dockerfile`  | Dockerfile under BACKEND_CONTEXT |
| `BACKEND_CONFIG`     | `protocol-flow.json` | Config file under `configs/`    |
| `E2E_UI_PORT`        | `3000`               | Host port for Caddy             |
| `CI`                 | (unset)              | Running in CI environment       |

## Troubleshooting

### Docker build fails for the backend

```bash
# Check Docker logs
docker compose -f docker-compose.e2e.yml logs backend

# Common issues:
# - Backend Dockerfile path wrong (verify docker/Dockerfile exists at repo root)
# - Config file not found at BACKEND_CONTEXT/configs/BACKEND_CONFIG
```

### Tests Timeout Waiting for Services

```bash
# Check Docker logs
docker compose -f docker-compose.e2e.yml logs backend

# Common issues:
# 1. Backend failed to start - check config file exists
# 2. NATS not healthy - check NATS logs
# 3. Build failed - check Dockerfile
```

### Port 3000 Already in Use

```bash
# Find what's using port 3000
lsof -i :3000

# Stop it or use cleanup
npm run test:e2e:cleanup
```

### Stale Containers from Previous Run

```bash
# Full cleanup
task clean

# Or manually
docker compose -f docker-compose.e2e.yml down -v
```

## CI Integration

### GitHub Actions Example

```yaml
- name: Run E2E Tests
  working-directory: ui
  run: |
    npm ci
    npm run test:e2e
```

**Important**: CI always rebuilds the stack (reuseExistingServer: false). The
semteams backend is built from the repo root as part of the docker-compose
stack — no separate backend checkout required.

## Common Patterns

### Test with Flow Creation

```typescript
test("should test runtime panel with real flow", async ({ page }) => {
  // Navigate to app
  await page.goto("/");

  // Create flow (real backend API)
  await page.click('[data-testid="create-flow"]');
  await page.fill('[data-testid="flow-name"]', "Test Flow");
  await page.click('[data-testid="save-flow"]');

  // Add components (real backend API)
  await page.click('[data-testid="add-udp-source"]');
  await page.click('[data-testid="add-processor"]');

  // Run flow (real backend starts components)
  await page.click('[data-testid="run-flow"]');

  // Wait for real backend to report running
  await expect(page.locator('[data-testid="status"]')).toHaveText("Running");

  // Open runtime panel (real backend endpoints)
  await page.click('[data-testid="runtime-panel-toggle"]');

  // Verify real metrics endpoint responds
  await expect(page.locator('[data-testid="metrics-table"]')).toBeVisible();
});
```

### Cleanup Test Data

```typescript
test.afterEach(async ({ page }) => {
  // Backend may persist flows in NATS - cleanup if needed
  // Usually handled by docker compose down between test runs
});
```

## Performance Expectations

- **Stack startup**: ~30-60 seconds (first time with build)
- **Stack startup**: ~10-20 seconds (with cache)
- **Per test**: 2-10 seconds
- **Full suite**: < 5 minutes target

## For Agent Developers

### Key Points

1. **Backend is REAL** - Don't mock `/flowbuilder/*`, `/agentic-dispatch/*`, or other semteams endpoints
2. **Use Taskfile** - `task ui:test:e2e` (from repo root) or `task test:e2e` (from `ui/`) handles all setup
3. **Services start automatically** - Playwright manages docker-compose
4. **No env vars required** - Defaults build semteams from `../..` automatically
5. **Port 3000** - All tests hit http://localhost:3000 (Caddy)
6. **Cleanup automatic** - `playwright.teardown.ts` runs `docker compose down`

### Test Writing Guidelines

```typescript
// ✅ Good test - uses real backend
test('should poll metrics', async ({ page }) => {
  await page.goto('/flows/test-id');
  await page.click('[data-testid="runtime-panel-toggle"]');
  await page.click('[data-testid="tab-metrics"]');

  // Real backend /runtime/metrics endpoint is polled
  await expect(page.locator('[data-testid="throughput"]')).toBeVisible();
});

// ❌ Bad test - mocks what's real
test('should poll metrics', async ({ page }) => {
  await page.route('**/runtime/metrics', route => {...});  // DON'T!
  // ...
});
```

## Summary

**The SemTeams UI E2E infrastructure provides a COMPLETE, REAL semteams
backend environment via Docker.**

- ✅ Use `task ui:test:e2e` (from repo root) or `task test:e2e` (from `ui/`)
- ✅ Test against real semteams endpoints
- ✅ Let Playwright manage Docker lifecycle
- ❌ Don't mock backend API calls
- ❌ Don't start Docker manually (Playwright does it)

For questions, see:

- `docker-compose.e2e.yml` - Docker stack definition
- `playwright.config.ts` - Playwright configuration
- `Taskfile.yml` - Task automation
