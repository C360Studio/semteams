# Authentication Options for SemStreams UI

## Current State

- **No authentication** - UI is completely open
- **Backend-agnostic** - Must work with any SemStreams-based backend
- **Container-first** - Deployed via Docker + Caddy

## Requirements Analysis

### Enterprise Expectations (2025)

- ✅ OAuth 2.0 / OpenID Connect (OIDC) - Industry standard
- ✅ SSO integration (Google Workspace, Azure AD, Okta)
- ✅ Passwordless options (passkeys, magic links, biometrics)
- ✅ Multi-factor authentication (MFA)
- ⚠️ Session-based auth - Legacy but still common

### Architecture Constraints

- **Backend-agnostic**: Auth solution shouldn't assume specific backend
- **Optional deployment**: Some users may not need auth (internal networks)
- **Multiple deployment models**: Docker, npm, standalone
- **Reverse proxy setup**: Caddy is already in the stack

---

## Authentication Patterns

### Pattern 1: Reverse Proxy Authentication ⭐ (RECOMMENDED)

**How it works:**

```
Browser → Caddy (auth) → SvelteKit UI → Backend API
                ↓
           Auth Provider (Authelia, OAuth2-Proxy, etc.)
```

**Implementation with Caddy:**

```caddyfile
:3000 {
    # Forward auth to authentication service
    forward_auth authelia:9091 {
        uri /api/verify?rd=https://auth.example.com
        copy_headers Remote-User Remote-Groups Remote-Name Remote-Email
    }

    # API routes → backend (auth headers passed through)
    reverse_proxy /flowbuilder/* backend:8080
    reverse_proxy /components/* backend:8080

    # UI routes → SvelteKit
    reverse_proxy * ui:5173
}
```

**Auth services that integrate with Caddy:**

1. **Authelia** (Open source, feature-rich)
   - OIDC, LDAP, file-based users
   - 2FA support (TOTP, U2F, WebAuthn)
   - Session management
   - Works with: Google, GitHub, Azure AD, custom OIDC

2. **OAuth2-Proxy** (Simple, OAuth-focused)
   - Pure OAuth 2.0 / OIDC
   - Minimal setup
   - Works with: Google, GitHub, GitLab, Keycloak, etc.

3. **Caddy-Security Plugin** (Native Caddy integration)
   - Built into Caddy
   - Multiple provider support
   - JWT validation

**Pros:**

- ✅ **Zero UI code changes** - Auth handled at infrastructure level
- ✅ **Backend-agnostic** - Works with any backend
- ✅ **Enterprise SSO support** - OIDC integrates with Azure AD, Okta, etc.
- ✅ **Optional** - Users can deploy without auth (skip Caddy auth)
- ✅ **Centralized** - One auth point protects entire application
- ✅ **Headers passed to backend** - Backend sees user identity

**Cons:**

- ⚠️ Requires external auth service (Authelia, OAuth2-Proxy)
- ⚠️ More infrastructure to manage
- ⚠️ Harder for local development (can disable auth locally)

**Best for:**

- Production enterprise deployments
- Organizations with existing SSO
- Docker-based deployments

---

### Pattern 2: Backend-Managed Authentication

**How it works:**

```
Browser → SvelteKit UI → Backend API (validates auth)
                           ↓
                      Auth Provider / Session Store
```

**Implementation:**

- Backend provides `/auth/login`, `/auth/logout`, `/auth/session` endpoints
- UI calls backend auth endpoints
- Backend issues session cookies or JWT tokens
- UI includes tokens in all API requests

**Example (Backend provides auth):**

```yaml
# Backend exposes auth endpoints
POST /auth/login        # Login with credentials/OAuth
GET  /auth/session      # Get current user
POST /auth/logout       # Logout
GET  /auth/providers    # List available OAuth providers

# Backend validates requests
GET /components/types   # Requires valid session/token
```

**SvelteKit implementation:**

```typescript
// hooks.server.ts
export async function handle({ event, resolve }) {
  // Check session cookie or token
  const session = await event.cookies.get("session");

  if (!session && !isPublicRoute(event.url.pathname)) {
    return Response.redirect("/login");
  }

  // Pass user info to backend
  event.fetch("/api/...", {
    headers: {
      Authorization: `Bearer ${session}`,
    },
  });

  return resolve(event);
}
```

**Pros:**

- ✅ **Backend controls auth** - Each backend can have its own auth strategy
- ✅ **Flexible** - Supports various auth methods per backend
- ✅ **Unified API** - Auth and business logic in same service
- ✅ **Works in all deployments** - Docker, npm, standalone

**Cons:**

- ⚠️ **Backend-specific** - Each backend must implement auth
- ⚠️ **UI code changes needed** - Auth logic in SvelteKit
- ⚠️ **Breaks backend-agnostic design** - UI assumes auth endpoints exist
- ⚠️ **More complex** - Auth distributed across UI + backend

**Best for:**

- Backends with existing auth systems
- Microservices with per-service auth
- Custom authentication requirements

---

### Pattern 3: SvelteKit-Managed Authentication (Auth.js)

**How it works:**

```
Browser → SvelteKit (Auth.js) → Backend API
              ↓
         Auth Providers (Google, GitHub, etc.)
```

**Implementation with Auth.js:**

```typescript
// src/hooks.server.ts
import { SvelteKitAuth } from "@auth/sveltekit";
import Google from "@auth/sveltekit/providers/google";
import GitHub from "@auth/sveltekit/providers/github";

export const handle = SvelteKitAuth({
  providers: [
    Google({
      clientId: process.env.GOOGLE_CLIENT_ID,
      clientSecret: process.env.GOOGLE_CLIENT_SECRET,
    }),
    GitHub({
      clientId: process.env.GITHUB_CLIENT_ID,
      clientSecret: process.env.GITHUB_CLIENT_SECRET,
    }),
  ],
  callbacks: {
    async jwt({ token, user }) {
      // Pass user info to backend API calls
      return token;
    },
  },
});
```

**Pros:**

- ✅ **Easy to implement** - Auth.js handles OAuth flow
- ✅ **68+ providers** - Google, Microsoft, GitHub, etc.
- ✅ **SvelteKit-native** - Designed for SvelteKit
- ✅ **Passwordless support** - Magic links, passkeys
- ✅ **CSRF protection** - Built-in security

**Cons:**

- ⚠️ **UI code changes** - Auth logic in SvelteKit app
- ⚠️ **Breaks backend-agnostic design** - Auth becomes UI responsibility
- ⚠️ **Backend needs to trust UI tokens** - Requires JWT validation
- ⚠️ **Harder to configure** - Each deployment needs OAuth credentials

**Best for:**

- SaaS deployments with known auth providers
- Apps where UI manages user identity
- Rapid prototyping

---

### Pattern 4: Hybrid - API Gateway Authentication

**How it works:**

```
Browser → API Gateway (auth) → SvelteKit UI
              ↓                      ↓
         Auth Provider          Backend API
```

**Example with Kong, Traefik, or AWS API Gateway:**

- API Gateway sits in front of everything
- Validates OAuth tokens, API keys, or sessions
- Passes user identity headers downstream
- Works like Pattern 1 but at infrastructure level

**Pros:**

- ✅ **Enterprise-grade** - Mature API gateway solutions
- ✅ **Centralized auth** - One place for all auth
- ✅ **Rate limiting, monitoring** - Additional features
- ✅ **Works with existing infrastructure** - Many orgs have API gateways

**Cons:**

- ⚠️ **Heavy infrastructure** - Requires API gateway deployment
- ⚠️ **Complexity** - More moving parts
- ⚠️ **Vendor lock-in** - If using cloud API gateways

**Best for:**

- Large enterprises with existing API gateways
- Multi-service deployments
- Cloud-native architectures (AWS, Azure, GCP)

---

## Recommended Approach

### For semstreams-ui: **Pattern 1 (Reverse Proxy Auth)** + **Optional Backend Validation**

**Why:**

1. **Maintains backend-agnostic design** - Auth is infrastructure, not code
2. **Optional by default** - Users can deploy without auth for internal use
3. **Enterprise-ready** - OIDC/SSO support via Authelia or OAuth2-Proxy
4. **No UI code changes** - Keep UI clean and focused
5. **Flexible** - Different deployments can use different auth

### Implementation Strategy

**Phase 1: Document reverse proxy auth (no code changes)**

- Create example docker-compose with Authelia
- Create example docker-compose with OAuth2-Proxy
- Document Caddy forward_auth configuration
- Provide examples for: Google OAuth, Azure AD, Okta

**Phase 2: Optional - Backend auth headers**

- Document how backends can read auth headers
- Provide optional middleware for user validation
- Examples: `Remote-User`, `Remote-Email`, `Remote-Groups`

**Phase 3: Optional - SvelteKit auth hooks (if needed)**

- Add optional Auth.js integration for users who want it
- Keep as optional plugin, not core feature
- Document how to enable/disable

---

## Example Configurations

### Option A: Authelia + Caddy (Full SSO)

**docker-compose.auth.yml:**

```yaml
services:
  authelia:
    image: authelia/authelia:latest
    volumes:
      - ./authelia:/config
    environment:
      - AUTHELIA_JWT_SECRET=${AUTHELIA_JWT_SECRET}
      - AUTHELIA_SESSION_SECRET=${AUTHELIA_SESSION_SECRET}
    ports:
      - "9091:9091"

  caddy:
    image: caddy:2-alpine
    volumes:
      - ./Caddyfile.auth:/etc/caddy/Caddyfile
    depends_on:
      - authelia
      - backend
      - ui
    ports:
      - "3000:3000"
```

**Caddyfile.auth:**

```caddyfile
:3000 {
    # Protect entire site with Authelia
    forward_auth authelia:9091 {
        uri /api/verify?rd=https://auth.example.com
        copy_headers Remote-User Remote-Email Remote-Groups
    }

    # Routes
    reverse_proxy /flowbuilder/* backend:8080
    reverse_proxy /components/* backend:8080
    reverse_proxy * ui:5173
}
```

**authelia/configuration.yml:**

```yaml
authentication_backend:
  file:
    path: /config/users_database.yml

access_control:
  default_policy: deny
  rules:
    - domain: semstreams.example.com
      policy: two_factor

session:
  secret: ${AUTHELIA_SESSION_SECRET}
  domain: example.com
  expiration: 1h
  inactivity: 5m

identity_providers:
  oidc:
    clients:
      - id: semstreams-ui
        description: SemStreams Flow Builder
        secret: ${OIDC_CLIENT_SECRET}
        redirect_uris:
          - https://semstreams.example.com/oauth2/callback
```

**Estimated setup time:** 30 minutes
**User experience:** SSO login page, 2FA support, enterprise-grade

---

### Option B: OAuth2-Proxy + Google (Simple OAuth)

**docker-compose.oauth.yml:**

```yaml
services:
  oauth2-proxy:
    image: quay.io/oauth2-proxy/oauth2-proxy:latest
    command:
      - --provider=google
      - --email-domain=example.com # Allow only your domain
      - --upstream=http://caddy:3000
      - --http-address=0.0.0.0:4180
      - --cookie-secret=${OAUTH2_PROXY_COOKIE_SECRET}
      - --client-id=${GOOGLE_CLIENT_ID}
      - --client-secret=${GOOGLE_CLIENT_SECRET}
    ports:
      - "4180:4180"
    depends_on:
      - caddy

  caddy:
    # ... (standard setup, no auth here)
```

**Access:** http://localhost:4180 (OAuth2-Proxy in front)

**Estimated setup time:** 15 minutes (with Google OAuth app)
**User experience:** "Sign in with Google" button

---

### Option C: No Auth (Development / Internal Networks)

**docker-compose.yml:**

```yaml
services:
  caddy:
    # No forward_auth directive
    # Direct access to UI
  ui:
    # ... standard setup
  backend:
    # ... standard setup
```

**Access:** http://localhost:3000 (direct, no auth)

**Estimated setup time:** 0 minutes
**User experience:** Immediate access

---

## Security Considerations

### Reverse Proxy Auth (Pattern 1)

- ✅ Tokens never in browser localStorage (more secure)
- ✅ HTTPS enforced by reverse proxy
- ✅ CSRF protection via SameSite cookies
- ✅ Session timeout configurable
- ⚠️ Ensure auth service is secure
- ⚠️ Use secure cookie flags (HttpOnly, Secure, SameSite)

### Backend-Managed Auth (Pattern 2)

- ✅ Backend controls authorization
- ✅ Can integrate with backend's user database
- ⚠️ CSRF protection needed in UI
- ⚠️ Token storage in browser (prefer cookies over localStorage)
- ⚠️ Logout must invalidate tokens

### SvelteKit Auth.js (Pattern 3)

- ✅ Built-in CSRF protection
- ✅ Server-only cookies (secure)
- ⚠️ OAuth credentials must be protected
- ⚠️ Callback URLs must be whitelisted
- ⚠️ Token refresh logic needed

---

## Decision Matrix

| Factor                                | Reverse Proxy                    | Backend-Managed          | SvelteKit Auth.js               |
| ------------------------------------- | -------------------------------- | ------------------------ | ------------------------------- |
| **Maintains backend-agnostic design** | ✅ Yes                           | ⚠️ No (assumes auth API) | ⚠️ No (UI owns auth)            |
| **No UI code changes**                | ✅ Yes                           | ❌ No                    | ❌ No                           |
| **Enterprise SSO**                    | ✅ Easy (Authelia, OAuth2-Proxy) | ✅ Possible              | ✅ Possible (68+ providers)     |
| **Optional deployment**               | ✅ Yes (skip Caddy auth)         | ⚠️ Harder                | ⚠️ Harder                       |
| **Setup complexity**                  | ⚠️ Medium (infrastructure)       | ⚠️ Medium (backend code) | ⚠️ Medium (UI code + OAuth app) |
| **Best for**                          | Docker deployments               | Custom auth backends     | SaaS apps                       |
| **2025 best practice**                | ✅ Yes (OIDC at edge)            | ✅ Yes (BFF pattern)     | ✅ Yes (Auth.js)                |

---

## Next Steps (No Code Changes Yet)

### Immediate: Documentation

1. Create `docs/auth/` directory with examples
2. Document Authelia integration
3. Document OAuth2-Proxy integration
4. Document "no auth" deployment
5. Add auth examples to INTEGRATION_EXAMPLE.md

### Future: Optional Implementation

1. Add optional Auth.js integration (feature flag)
2. Create example backends with auth endpoints
3. Add auth header documentation for backends
4. Create Caddy plugin examples

### For Users to Decide

- Which auth pattern fits their deployment?
- Do they need enterprise SSO?
- Is auth required or optional?
- What identity providers do they use?

---

## Recommendations by Use Case

### Internal Tool (No Auth Needed)

→ **Option C**: Deploy without auth, rely on network security

### Small Team (Simple OAuth)

→ **Option B**: OAuth2-Proxy + Google/GitHub OAuth
→ Setup time: 15 minutes

### Enterprise (SSO Required)

→ **Option A**: Authelia + Azure AD / Okta OIDC
→ Setup time: 30 minutes, full 2FA + session management

### Custom Backend with Auth

→ **Pattern 2**: Let backend handle auth
→ UI calls backend auth endpoints

### Multi-tenant SaaS

→ **Pattern 3**: SvelteKit Auth.js with per-tenant providers
→ Or **Pattern 4**: API Gateway with tenant isolation

---

## Summary

**For semstreams-ui, Pattern 1 (Reverse Proxy Auth) is recommended** because:

1. ✅ No code changes needed - infrastructure-level
2. ✅ Maintains backend-agnostic architecture
3. ✅ Optional - users choose their auth strategy
4. ✅ Enterprise-ready - full SSO/OIDC support
5. ✅ Flexible - works with any auth provider
6. ✅ 2025 best practice - OAuth at edge, OIDC federation

**Implementation path:**

- Phase 1: Document reverse proxy auth (this document)
- Phase 2: Provide docker-compose examples
- Phase 3: Create tutorials for common providers
- Future: Optional SvelteKit Auth.js for users who want it

This approach lets users of semstreams-ui choose their auth strategy without forcing any particular solution into the codebase.
