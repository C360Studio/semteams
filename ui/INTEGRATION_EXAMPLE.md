# Integration Example: Using SemStreams UI with Your Application

This guide shows how to integrate SemStreams UI with your own SemStreams-based application.

## Example: "MyStreamApp" - Custom Stream Processing Application

Let's say you're building **MyStreamApp**, a custom data processing application using the SemStreams framework.

### Step 1: Build Your SemStreams Application

```go
// myapp/main.go
package main

import (
    "github.com/c360/semstreams/engine"
    "github.com/c360/semstreams/service"
    // Your custom components
    "github.com/yourorg/myapp/components/customprocessor"
)

func main() {
    // Register your custom components
    customprocessor.Register()

    // Start SemStreams with flow builder API
    cfg := &service.Config{
        FlowBuilderEnabled: true,
        HTTPPort: 8080,
    }

    svc, err := service.New(cfg)
    if err != nil {
        log.Fatal(err)
    }

    svc.Start()
}
```

### Step 2: Implement Required API Endpoints

Your application must expose these endpoints for the UI to work:

#### 1. Health Check

```
GET /health
→ {"healthy": true, "status": "healthy"}
```

#### 2. Component Types (Discovery)

```
GET /components/types
→ [
    {
      "id": "my-custom-processor",
      "name": "Custom Processor",
      "type": "processor",
      "protocol": "custom",
      "domain": "processing",
      "description": "My custom data processor",
      "version": "1.0.0",
      "schema": { ... }
    }
  ]
```

#### 3. Flow Management

```
GET    /flowbuilder/flows           # List flows
POST   /flowbuilder/flows           # Create flow
GET    /flowbuilder/flows/:id       # Get flow
PUT    /flowbuilder/flows/:id       # Update flow
DELETE /flowbuilder/flows/:id       # Delete flow
```

#### 4. OpenAPI Spec (for type generation)

```
GET /openapi.yaml
→ OpenAPI 3.0 spec with all endpoints and schemas
```

### Step 3: Generate OpenAPI Spec

Use the semstreams schema exporter to generate your spec:

```bash
# In your myapp directory
go run github.com/c360/semstreams/cmd/schema-exporter \
  -output specs/openapi.v3.yaml

# This generates:
# myapp/specs/openapi.v3.yaml
# myapp/schemas/*.v1.json
```

### Step 4: Set Up SemStreams UI

**Option A: Use UI from separate clone**

```bash
# Clone semstreams-ui
git clone https://github.com/c360/semstreams-ui.git
cd semstreams-ui

# Point to your OpenAPI spec
export OPENAPI_SPEC_PATH=../myapp/specs/openapi.v3.yaml

# Generate TypeScript types
npm run generate-types

# Start UI (expects your backend at localhost:8080)
npm run dev
```

**Option B: Copy UI into your repo**

```bash
# Copy UI into your project
cp -r /path/to/semstreams-ui myapp/ui

cd myapp/ui

# Configure for your backend
cat > .env <<EOF
BACKEND_URL=http://localhost:8080
OPENAPI_SPEC_PATH=../specs/openapi.v3.yaml
EOF

# Generate types
npm run generate-types

# Start UI
npm run dev
```

**Option C: Docker Compose (Recommended)**

```yaml
# myapp/docker-compose.yml
version: "3.8"

services:
  # NATS message broker
  nats:
    image: nats:2.10-alpine
    command: ["-js", "-sd", "/data"]
    ports:
      - "4222:4222"
    volumes:
      - nats-data:/data

  # Your backend application
  myapp:
    build: .
    depends_on:
      - nats
    environment:
      - NATS_URL=nats://nats:4222
      - HTTP_PORT=8080
    ports:
      - "8080:8080"
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/health"]
      interval: 10s

  # SemStreams UI
  ui:
    image: semstreams/ui:latest # Or build from local
    environment:
      - BACKEND_HOST=myapp:8080
    depends_on:
      myapp:
        condition: service_healthy
    volumes:
      - ./ui/src:/app/src # For development hot-reload

  # Reverse proxy
  caddy:
    image: caddy:2-alpine
    depends_on:
      - myapp
      - ui
    ports:
      - "3000:3000"
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile
```

```caddyfile
# myapp/Caddyfile
:3000 {
    # API routes → backend
    reverse_proxy /flowbuilder/* myapp:8080
    reverse_proxy /components/* myapp:8080
    reverse_proxy /health myapp:8080

    # Everything else → UI
    reverse_proxy * ui:5173
}
```

### Step 5: Run Your Application with UI

```bash
# Start everything
docker compose up

# Access UI at http://localhost:3000
# The UI automatically discovers your custom components!
```

## What Happens at Runtime

1. **User opens UI** at http://localhost:3000
2. **UI fetches** `GET /components/types` from your backend
3. **UI discovers** your custom components dynamically
4. **User builds flows** using your custom components
5. **UI saves flows** via `POST /flowbuilder/flows`
6. **Your backend** executes the flows using SemStreams engine

## Component Schema Example

Your custom component should expose its configuration schema:

```go
// myapp/components/customprocessor/register.go
package customprocessor

import "github.com/c360/semstreams/component"

type Config struct {
    InputPattern  string `schema:"type:string,desc:Input message pattern,default:my.input.*"`
    OutputPattern string `schema:"type:string,desc:Output message pattern,default:my.output"`
    BufferSize    int    `schema:"type:int,desc:Buffer size,default:100,min:1,max:10000"`
}

func Register() {
    component.RegisterFactory("my-custom-processor", &component.Registration{
        Type:        "processor",
        Protocol:    "custom",
        Domain:      "processing",
        Description: "My custom data processor",
        Version:     "1.0.0",
        Schema:      Config{},  // Schema auto-extracted from struct tags
        Factory:     newProcessor,
    })
}
```

The schema exporter reads the struct tags and generates:

```yaml
# specs/openapi.v3.yaml
components:
  schemas:
    CustomProcessorConfig:
      type: object
      properties:
        InputPattern:
          type: string
          description: Input message pattern
          default: "my.input.*"
        OutputPattern:
          type: string
          description: Output message pattern
          default: "my.output"
        BufferSize:
          type: integer
          description: Buffer size
          default: 100
          minimum: 1
          maximum: 10000
```

The UI automatically renders a form based on this schema!

## TypeScript Type Generation

The UI generates TypeScript types from your OpenAPI spec:

```bash
# In semstreams-ui
OPENAPI_SPEC_PATH=../myapp/specs/openapi.v3.yaml task generate-types

# Generates: src/lib/types/api.generated.ts
# Contains type-safe definitions for all your API endpoints and schemas
```

Now your UI code has full TypeScript autocomplete for your custom components:

```typescript
// UI code automatically knows about your custom processor!
import type { ComponentType } from "$lib/types/api.generated";

const components: ComponentType[] = await fetch("/components/types").then((r) =>
  r.json(),
);

// TypeScript knows the exact shape of your config
const myProcessor = components.find((c) => c.id === "my-custom-processor");
if (myProcessor.schema) {
  // Full autocomplete for InputPattern, OutputPattern, BufferSize
}
```

## Testing Your Integration

### 1. Verify Backend Endpoints

```bash
# Component discovery
curl http://localhost:8080/components/types | jq

# Health check
curl http://localhost:8080/health

# OpenAPI spec
curl http://localhost:8080/openapi.yaml
```

### 2. Test UI Connection

```bash
# Start UI with your backend
BACKEND_URL=http://localhost:8080 npm run dev

# UI should show your custom components in the palette
```

### 3. Create a Test Flow

1. Open http://localhost:5173
2. Drag your custom component onto the canvas
3. Configure it using the auto-generated form
4. Save the flow
5. Verify it appears in `GET /flowbuilder/flows`

## Distribution Options

### Option 1: Standalone Docker Image

```dockerfile
# Dockerfile
FROM node:22-alpine AS ui-builder
WORKDIR /ui
COPY semstreams-ui/package*.json ./
RUN npm ci
COPY semstreams-ui/ ./
RUN npm run build

FROM myapp-base:latest
COPY --from=ui-builder /ui/build /app/ui
# Your backend serves the UI at /
```

### Option 2: Separate Services

Keep UI and backend as separate Docker services (shown in docker-compose above).

### Option 3: Embedded UI

Copy semstreams-ui source into your repo and build together.

## Customization

### Branding

The UI uses "SemStreams" branding by default. You can customize:

```typescript
// src/routes/+page.svelte
<title>MyStreamApp - Flow Builder</title>

// src/lib/components/StatusBar.svelte
<span>MyStreamApp v1.0.0</span>
```

### Custom Color Themes

Override CSS custom properties:

```css
/* src/styles/custom-theme.css */
:root {
  --ui-interactive-primary: #your-brand-color;
  --domain-custom: #your-domain-color;
}
```

### Additional Components

Add your own UI components in `src/lib/components/custom/`.

## Troubleshooting

### "Component types not loading"

```bash
# Check backend is exposing the endpoint
curl http://localhost:8080/components/types

# Check CORS if running UI separately
# Add CORS headers to your backend
```

### "Type generation fails"

```bash
# Verify OpenAPI spec is valid
npx openapi-typescript ../myapp/specs/openapi.v3.yaml --output /tmp/test.ts

# Check spec exists and is readable
ls -la ../myapp/specs/openapi.v3.yaml
```

### "UI can't connect to backend"

```bash
# Check BACKEND_URL environment variable
echo $BACKEND_URL

# Verify backend is running
curl http://localhost:8080/health

# Check network connectivity (Docker)
docker network inspect myapp_default
```

## Next Steps

- Add custom validation rules to your components
- Implement runtime metrics collection
- Create custom visualizations for your data types
- Add authentication/authorization
- Deploy to production with HTTPS

## Support

- SemStreams Framework: https://github.com/c360/semstreams
- SemStreams UI: https://github.com/c360/semstreams-ui
- OpenAPI Spec: See `/docs/OPENAPI_INTEGRATION.md`
- Schema Generation: See `/docs/SCHEMA_GENERATION.md`
