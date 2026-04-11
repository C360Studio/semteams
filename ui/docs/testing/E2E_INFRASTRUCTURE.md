# E2E Test Infrastructure

## Overview

The semstreams-ui E2E tests run against a **full Docker stack** including:

- NATS server (message broker)
- Backend (semstreams or semmem - configurable)
- UI (SvelteKit in dev mode)
- Caddy (reverse proxy routing)

**IMPORTANT**: Do NOT mock the backend. The E2E environment provides a real backend running in Docker.

## Quick Start

### Option 1: Using Taskfile (RECOMMENDED)

```bash
# Run E2E tests against semstreams backend
task test:e2e:semstreams
```

This automatically:

- Sets BACKEND_CONTEXT=../semstreams
- Sets BACKEND_CONFIG=protocol-flow.json
- Starts Docker stack via docker-compose.e2e.yml
- Runs Playwright tests
- Cleans up Docker stack

### Option 2: Manual (for debugging)

```bash
# Start the E2E stack manually
BACKEND_CONTEXT=../semstreams \
BACKEND_CONFIG=protocol-flow.json \
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
| **backend** | SemStreams/semmem backend     | http://localhost:8080/health  | (internal only) |
| **ui**      | SvelteKit dev server          | N/A                           | (internal only) |
| **caddy**   | Reverse proxy                 | http://localhost:3000/health  | **3000** (host) |

**Key Point**: Only Caddy is exposed to the host. It routes:

- `/flowbuilder/*` → backend:8080
- `/*` → ui:5173

## Configuration

### Backend Selection

The E2E stack is **backend-agnostic**. Configure via environment variables:

**For semstreams backend:**

```bash
export BACKEND_CONTEXT=../semstreams
export BACKEND_CONFIG=protocol-flow.json
```

**For semmem backend:**

```bash
export BACKEND_CONTEXT=../semstreams  # Still semstreams repo
export BACKEND_DOCKERFILE=semmem/Dockerfile
export BACKEND_CONFIG=semmem-flow.json
```

**For custom backend:**

```bash
export BACKEND_CONTEXT=/path/to/your-backend
export BACKEND_CONFIG=your-config.json
```

### Required Backend Structure

Your backend must provide:

```
your-backend/
├── Dockerfile (or custom via BACKEND_DOCKERFILE)
│   └── Must have 'production' target
├── configs/
│   └── ${BACKEND_CONFIG} (referenced in STREAMKIT_CONFIG)
└── Must expose:
    ├── GET /health (for healthcheck)
    └── GET /flowbuilder/* (API endpoints)
```

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

### Required (via Taskfile or manual)

| Variable          | Purpose                         | Example              |
| ----------------- | ------------------------------- | -------------------- |
| `BACKEND_CONTEXT` | Path to backend directory       | `../semstreams`      |
| `BACKEND_CONFIG`  | Config file in backend/configs/ | `protocol-flow.json` |

### Optional

| Variable             | Purpose                   | Default      |
| -------------------- | ------------------------- | ------------ |
| `BACKEND_DOCKERFILE` | Custom Dockerfile path    | `Dockerfile` |
| `CI`                 | Running in CI environment | (unset)      |

## Troubleshooting

### "BACKEND_CONTEXT must be set" Error

```bash
# ❌ WRONG: Missing BACKEND_CONTEXT
npm run test:e2e

# ✅ CORRECT: Use Taskfile
task test:e2e:semstreams

# ✅ CORRECT: Set manually
BACKEND_CONTEXT=../semstreams npm run test:e2e
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
  env:
    BACKEND_CONTEXT: ${{ github.workspace }}/semstreams
    BACKEND_CONFIG: protocol-flow.json
  run: |
    npm install
    task test:e2e:semstreams
```

**Important**: CI always rebuilds the stack (reuseExistingServer: false).

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

1. **Backend is REAL** - Don't mock `/flowbuilder/*` endpoints
2. **Use Taskfile** - `task test:e2e:semstreams` handles all setup
3. **Services start automatically** - Playwright manages docker-compose
4. **BACKEND_CONTEXT required** - Points to backend directory
5. **Port 3000** - All tests hit http://localhost:3000 (Caddy)
6. **Cleanup automatic** - playwright.teardown.ts runs `docker compose down`

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

**The semstreams-ui E2E infrastructure provides a COMPLETE, REAL backend environment via Docker.**

- ✅ Use `task test:e2e:semstreams` to run tests
- ✅ Test against real backend endpoints
- ✅ Let Playwright manage Docker lifecycle
- ❌ Don't mock backend API calls
- ❌ Don't start Docker manually (Playwright does it)
- ❌ Don't forget BACKEND_CONTEXT environment variable

For questions, see:

- `docker-compose.e2e.yml` - Docker stack definition
- `playwright.config.ts` - Playwright configuration
- `Taskfile.yml` - Task automation
