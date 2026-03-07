---
name: backend-integration
description: Connect the UI to semstreams backend APIs. Covers proxy routing, fetch patterns, SSR considerations, and real-time data. Use when adding new backend integrations or debugging API issues.
argument-hint: [API endpoint or integration being added]
---

# Backend Integration Guide

## What are you integrating?

$ARGUMENTS

## Architecture

The UI never talks to the backend directly. All requests go through Caddy at `localhost:3001`:

```
Browser --> localhost:3001 (Caddy) --> backend:8080 (Docker)
                                  --> host.docker.internal:5173 (Vite, for UI assets)
```

### Caddy Routes (from `Caddyfile.dev`)

| Route            | Destination     | Purpose                 |
| ---------------- | --------------- | ----------------------- |
| `/components/*`  | backend:8080    | Component type registry |
| `/flowbuilder/*` | backend:8080    | Flow CRUD operations    |
| `/health`        | backend:8080    | System health           |
| `/*`             | Vite dev server | UI assets and pages     |

### SSR Fetch Transform

`src/hooks.server.ts` transforms fetch URLs during SSR so that server-side rendering can reach the backend via Docker networking (not `localhost`).

When adding new backend routes:

1. Add the route to `Caddyfile.dev`
2. Update `src/hooks.server.ts` if SSR needs to fetch from this route

## Available Backend APIs

### Component Types

```bash
curl -s http://localhost:3001/components/types | jq
```

Returns array of:

```json
{
  "id": "udp-input",
  "name": "UDP Input",
  "type": "input",
  "protocol": "udp",
  "category": "input",
  "description": "Receives UDP packets",
  "schema": {}
}
```

`type`/`category` values: `input`, `processor`, `output`, `storage`, `gateway`

### Flows

```bash
# List flows
curl http://localhost:3001/flowbuilder/flows

# Get flow by ID
curl http://localhost:3001/flowbuilder/flows/{id}

# Create/update flow
curl -X POST http://localhost:3001/flowbuilder/flows -d '...'
```

### Health

```bash
curl http://localhost:3001/health
```

## Fetch Patterns

### From a SvelteKit Load Function (Preferred)

```typescript
// src/routes/my-page/+page.ts
import type { PageLoad } from "./$types";

export const load: PageLoad = async ({ fetch }) => {
  const response = await fetch("/components/types");
  if (!response.ok) {
    throw error(response.status, "Failed to load component types");
  }
  const types = await response.json();
  return { types };
};
```

Use the `fetch` provided by SvelteKit — it handles SSR URL transforms via `hooks.server.ts`.

### From a Component (Client-Side Only)

```svelte
<script lang="ts">
  import { browser } from '$app/environment';

  let data = $state<MyType | null>(null);
  let error = $state<string | null>(null);
  let loading = $state(false);

  async function loadData() {
    loading = true;
    error = null;
    try {
      const response = await fetch('/flowbuilder/flows');
      if (!response.ok) throw new Error(`HTTP ${response.status}`);
      data = await response.json();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Unknown error';
    } finally {
      loading = false;
    }
  }

  // Only fetch in browser (not SSR)
  $effect(() => {
    if (browser) loadData();
  });
</script>
```

### Polling for Real-Time Data

```svelte
<script lang="ts">
  import { browser } from '$app/environment';

  let health = $state<HealthData | null>(null);

  $effect(() => {
    if (!browser) return;

    const fetchHealth = async () => {
      try {
        const res = await fetch('/health');
        health = await res.json();
      } catch { /* swallow — will retry on next interval */ }
    };

    fetchHealth();
    const interval = setInterval(fetchHealth, 5000);
    return () => clearInterval(interval);  // Cleanup
  });
</script>
```

## Adding a New Backend Integration

### Checklist

1. **Verify the endpoint exists**

   ```bash
   curl -v http://localhost:3001/your/endpoint
   ```

2. **Check if Caddy routes it**
   - Read `Caddyfile.dev` — does it match?
   - If not, add a route

3. **Check SSR needs**
   - Will this be called in a `load` function?
   - If yes, check `src/hooks.server.ts` handles the URL

4. **Choose fetch pattern**
   - Page-level data → SvelteKit `load`
   - Component-level, one-time → `$effect` with `browser` guard
   - Real-time → `$effect` with `setInterval` and cleanup
   - User-triggered → event handler with async fetch

5. **Type the response**

   ```typescript
   interface MyApiResponse {
     items: Item[];
     total: number;
   }
   ```

6. **Handle states**
   - Loading state
   - Error state (network error, HTTP error)
   - Empty state (valid response, no data)

## Debugging API Issues

```bash
# Is the backend running?
docker compose -f docker-compose.dev.yml ps

# Can we reach it through Caddy?
curl -v http://localhost:3001/health

# Backend logs
task dev:backend:logs

# Check Caddy logs
docker compose -f docker-compose.dev.yml logs caddy

# Check SSR fetch transforms
grep -n "handle" src/hooks.server.ts
```

Common issues:

- **404**: Route not in `Caddyfile.dev`
- **CORS error**: Hitting backend directly (use `:3001` not backend port)
- **Empty response in SSR**: `hooks.server.ts` not transforming the URL
- **Connection refused**: Backend not started (`task dev:backend:start`)
