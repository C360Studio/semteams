# Testing Documentation

## Quick Reference

### Running E2E Tests

```bash
# RECOMMENDED: Use Taskfile
task test:e2e:semstreams

# Manual (requires BACKEND_CONTEXT)
BACKEND_CONTEXT=../semstreams npm run test:e2e
```

### Running Unit Tests

```bash
npm run test:unit
```

## Documentation

- **[E2E_INFRASTRUCTURE.md](./E2E_INFRASTRUCTURE.md)** - Complete E2E testing guide
  - Docker stack architecture
  - How to run tests
  - How to write tests
  - Troubleshooting
  - **READ THIS FIRST if working on E2E tests**

## Key Points for Developers

1. **E2E tests use REAL backend** via Docker - don't mock API calls
2. **Use Taskfile** - `task test:e2e:semstreams` handles all setup
3. **BACKEND_CONTEXT required** - environment variable pointing to backend directory
4. **Playwright manages Docker** - automatically starts/stops stack
5. **Port 3000** - all E2E tests hit http://localhost:3000

## Common Issues

### "BACKEND_CONTEXT must be set"

```bash
# Use Taskfile (sets it automatically)
task test:e2e:semstreams
```

### Tests timeout waiting for backend

```bash
# Check backend logs
docker compose -f docker-compose.e2e.yml logs backend

# Cleanup and retry
task clean
task test:e2e:semstreams
```

### Port conflicts

```bash
# Cleanup E2E stack
npm run test:e2e:cleanup
```

## Test Structure

```
semstreams-ui/
├── e2e/                          # E2E tests (Playwright)
│   ├── helpers/                  # Test utilities
│   │   ├── flow-helpers.ts       # Flow creation helpers
│   │   └── runtime-helpers.ts    # Runtime panel helpers
│   ├── runtime-viz-basic.spec.ts # Panel open/close/resize
│   ├── runtime-viz-logs.spec.ts  # Logs tab with SSE
│   ├── runtime-viz-metrics.spec.ts # Metrics tab polling
│   └── runtime-viz-health.spec.ts  # Health tab polling
├── src/
│   └── lib/
│       └── components/
│           └── *.test.ts         # Unit tests (Vitest)
├── docker-compose.e2e.yml        # E2E Docker stack
├── playwright.config.ts          # Playwright configuration
└── Taskfile.yml                  # Task automation
```

## For More Information

See [E2E_INFRASTRUCTURE.md](./E2E_INFRASTRUCTURE.md) for complete documentation.
