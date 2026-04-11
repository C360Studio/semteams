# E2E Testing Setup

## Overview

SemStreams UI uses Playwright for end-to-end testing with a full Docker stack (NATS + Backend + UI + Caddy).

## Important: Port Management

**The E2E tests use these ports:**

- **3000**: Caddy (UI access point)
- **4222**: NATS (message broker)
- **5173**: Vite (UI dev server, inside Docker)
- **8080**: Backend API (inside Docker, not exposed to host)

**⚠️ Conflict Warning**: If another process (like backend semantic E2E tests) is using ports 4222 or 8080, the UI E2E tests will fail. Always check for conflicts before running tests.

## Quick Start

```bash
# Run all E2E tests (Playwright manages Docker automatically)
npm run test:e2e

# UI mode (interactive)
npm run test:e2e:ui

# Debug mode
npm run test:e2e:debug

# Cleanup (if tests crash)
task clean
```

## How It Works

### Automated Stack Management

Playwright automatically:

1. **Starts** `docker-compose.e2e.yml` when tests begin
2. **Waits** for health checks (backend, NATS, Caddy)
3. **Runs** tests against http://localhost:3000
4. **Tears down** Docker stack when tests complete

### Docker Compose Stack

`docker-compose.e2e.yml` defines:

```yaml
services:
  nats: # Message broker (port 4222)
  backend: # SemStreams backend (configurable)
  ui: # SvelteKit dev server (port 5173)
  caddy: # Reverse proxy (port 3000)
```

### Backend Selection

The E2E setup supports testing against different backends:

```bash
# Test with semstreams backend (default)
task test:e2e:semstreams

# Test with semmem backend
task test:e2e:semmem

# Custom backend
BACKEND_CONTEXT=../myapp BACKEND_CONFIG=myapp.json npm run test:e2e
```

## Manual Testing

### Start Stack Manually

```bash
# Start full E2E stack
docker compose -f docker-compose.e2e.yml up

# Stack available at http://localhost:3000
# Backend at http://localhost:3000/components/types
# Health at http://localhost:3000/health
```

### Stop Stack

```bash
# Stop and remove volumes
docker compose -f docker-compose.e2e.yml down -v

# Or use task
task clean
```

### Debug Container Logs

```bash
# All services
docker compose -f docker-compose.e2e.yml logs -f

# Specific service
docker compose -f docker-compose.e2e.yml logs backend
docker compose -f docker-compose.e2e.yml logs ui
docker compose -f docker-compose.e2e.yml logs caddy
docker compose -f docker-compose.e2e.yml logs nats
```

## Troubleshooting

### Port Conflicts

```bash
# Check what's using port 3000
lsof -iTCP:3000 -sTCP:LISTEN

# Check what's using port 4222 (NATS)
lsof -iTCP:4222 -sTCP:LISTEN

# List all Docker containers
docker ps

# If backend E2E tests are running (in semstreams repo)
docker compose -f /path/to/semstreams/docker-compose.e2e.yml down
```

### Health Check Failures

```bash
# Check backend health
curl http://localhost:3000/health

# Check component discovery
curl http://localhost:3000/components/types

# Check Caddy routing
curl -I http://localhost:3000/

# Check inside Docker network
docker compose -f docker-compose.e2e.yml exec backend curl http://localhost:8080/health
```

### Container Won't Start

```bash
# Check build logs
docker compose -f docker-compose.e2e.yml build backend

# Force rebuild
docker compose -f docker-compose.e2e.yml build --no-cache backend

# Check volume mounts
docker compose -f docker-compose.e2e.yml config | grep volumes
```

### Tests Timeout

Increase Playwright timeout in `playwright.config.ts`:

```typescript
webServer: {
    command: 'docker compose -f docker-compose.e2e.yml up --build',
    url: 'http://localhost:3000/health',
    timeout: 180000,  // 3 minutes instead of 2
}
```

### Network Issues

```bash
# Inspect Docker network
docker network inspect semstreams-ui-e2e_e2e-net

# Check connectivity between services
docker compose -f docker-compose.e2e.yml exec ui curl http://backend:8080/health
docker compose -f docker-compose.e2e.yml exec caddy curl http://backend:8080/health
```

## Configuration

### Environment Variables

```bash
# Backend context (path to your SemStreams-based application)
export BACKEND_CONTEXT=/path/to/your-backend

# Backend Dockerfile (relative to BACKEND_CONTEXT)
export BACKEND_DOCKERFILE=Dockerfile

# Backend config file (default: protocol-flow.json)
export BACKEND_CONFIG=myapp-flow.json
```

### Playwright Config

Key settings in `playwright.config.ts`:

```typescript
{
    testDir: 'e2e',                  // Test location
    fullyParallel: true,             // Run tests in parallel
    retries: process.env.CI ? 2 : 0, // Retry on CI
    workers: process.env.CI ? 1 : undefined,

    webServer: {
        command: 'docker compose -f docker-compose.e2e.yml up --build',
        url: 'http://localhost:3000/health',
        timeout: 120000,
        reuseExistingServer: !process.env.CI,  // Reuse in dev
    },

    use: {
        baseURL: 'http://localhost:3000',
        trace: 'on-first-retry',
        screenshot: 'only-on-failure',
        video: 'retain-on-failure',
    },
}
```

## CI/CD Integration

### GitHub Actions

`.github/workflows/e2e-tests.yml`:

```yaml
name: E2E Tests

on:
  pull_request:
  push:
    branches: [main]

jobs:
  e2e:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Setup Node.js
        uses: actions/setup-node@v4
        with:
          node-version: "22"

      - name: Install dependencies
        run: npm ci

      - name: Install Playwright browsers
        run: npx playwright install chromium

      - name: Run E2E tests
        run: npm run test:e2e

      - name: Upload artifacts
        if: failure()
        uses: actions/upload-artifact@v4
        with:
          name: playwright-report
          path: playwright-report/
```

### Local CI Simulation

```bash
# Run as CI would (no reuse, clean state)
CI=1 npm run test:e2e

# Force cleanup before tests
task clean && npm run test:e2e
```

## Test Organization

```
e2e/
├── basic-navigation.spec.ts       # Basic routing, page loads
├── component-palette.spec.ts      # Component discovery, dragging
├── flow-creation.spec.ts          # Creating and saving flows
├── component-config.spec.ts       # Configuring components
├── validation.spec.ts             # Flow validation
└── helpers/
    └── flow-setup.ts              # Shared test utilities
```

## Best Practices

1. **Always check ports** before running E2E tests
2. **Use task commands** instead of manual docker compose
3. **Clean up after failures** to avoid stale containers
4. **Check health endpoints** when debugging
5. **Use `--headed` mode** for visual debugging: `npx playwright test --headed`
6. **Inspect network tab** in Playwright UI mode for API debugging
7. **Keep E2E tests focused** - test user workflows, not implementation details

## Performance Tips

### Faster Test Runs

```bash
# Skip build if code hasn't changed
docker compose -f docker-compose.e2e.yml up  # No --build flag

# Run specific test file
npx playwright test flow-creation.spec.ts

# Run tests matching pattern
npx playwright test --grep "create flow"
```

### Reuse Stack During Development

```bash
# Start stack once
docker compose -f docker-compose.e2e.yml up -d

# Run tests (reuses existing stack)
npx playwright test

# Stop when done
docker compose -f docker-compose.e2e.yml down -v
```

## Debugging Failed Tests

1. **Check Playwright trace**: Open `playwright-report/index.html`
2. **View screenshots**: Saved in `test-results/` on failure
3. **Watch video**: Saved in `test-results/` for failed tests
4. **Run in debug mode**: `npm run test:e2e:debug`
5. **Use Playwright Inspector**: Pause on `await page.pause()`

## Backend-Specific E2E Tests

### Semstreams Backend

```bash
# Uses protocol-flow.json config
task test:e2e:semstreams
```

### Semmem Backend

```bash
# Uses semmem-specific config
task test:e2e:semmem
```

### Custom Backend

```yaml
# docker-compose.e2e.yml
services:
  backend:
    build:
      context: ${BACKEND_CONTEXT} # Path to your backend repo
      dockerfile: ${BACKEND_DOCKERFILE:-Dockerfile}
    environment:
      - STREAMKIT_CONFIG=/app/configs/${BACKEND_CONFIG:-protocol-flow.json}
```

## Future Enhancements

- [ ] Add visual regression testing
- [ ] Test accessibility with axe-core
- [ ] Add performance benchmarks
- [ ] Test mobile responsive layouts
- [ ] Add cross-browser testing (Firefox, Safari)
- [ ] Implement E2E test parallelization across multiple backends
