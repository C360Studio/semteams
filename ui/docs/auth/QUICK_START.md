# Authentication Quick Start Guide

Choose your authentication approach based on your needs:

## Option 1: No Auth (Development / Internal Networks)

**When to use:**

- Local development
- Internal networks with network-level security
- Testing and prototyping

**Setup time:** 0 minutes

**Steps:**

```bash
# Use standard docker-compose (no auth)
docker compose up
```

Access: http://localhost:3000 (no login required)

---

## Option 2: OAuth2-Proxy + Google OAuth (Simple)

**When to use:**

- Small teams
- "Sign in with Google" is acceptable
- Quick enterprise auth needed

**Setup time:** 15 minutes

**Steps:**

1. **Create Google OAuth App:**
   - Go to https://console.cloud.google.com/apis/credentials
   - Create OAuth 2.0 Client ID
   - Add redirect URI: `http://localhost:4180/oauth2/callback`
   - Copy Client ID and Client Secret

2. **Create `.env.oauth` file:**

```bash
GOOGLE_CLIENT_ID=your-client-id.apps.googleusercontent.com
GOOGLE_CLIENT_SECRET=your-client-secret
OAUTH2_PROXY_COOKIE_SECRET=$(openssl rand -base64 32)
```

3. **Start services:**

```bash
cd docs/auth/examples
docker compose -f oauth2-proxy-docker-compose.yml --env-file .env.oauth up
```

4. **Access:**
   - Open: http://localhost:4180
   - Click "Sign in with Google"
   - Only users with @example.com domain can access

**Customize:**

- Change `--email-domain=example.com` to your domain
- Add `--github-org=your-org` for GitHub OAuth
- Add `--azure-tenant=your-tenant` for Azure AD

---

## Option 3: Authelia (Full SSO with 2FA)

**When to use:**

- Enterprise deployments
- Need SSO (Azure AD, Okta, etc.)
- Want 2FA (TOTP, WebAuthn)
- Session management required

**Setup time:** 30 minutes

**Steps:**

1. **Create Authelia configuration directory:**

```bash
mkdir -p authelia
cd authelia
```

2. **Create `authelia/configuration.yml`:**

```yaml
server:
  host: 0.0.0.0
  port: 9091

log:
  level: info

authentication_backend:
  file:
    path: /config/users_database.yml

access_control:
  default_policy: deny
  rules:
    - domain: localhost
      policy: two_factor

session:
  name: authelia_session
  secret: ${AUTHELIA_SESSION_SECRET}
  expiration: 1h
  inactivity: 5m
  domain: localhost

storage:
  encryption_key: ${AUTHELIA_STORAGE_ENCRYPTION_KEY}
  local:
    path: /config/db.sqlite3

notifier:
  filesystem:
    filename: /config/notification.txt
```

3. **Create `authelia/users_database.yml`:**

```yaml
users:
  admin:
    displayname: "Admin User"
    password: "$argon2id$v=19$m=65536,t=3,p=4$<hash>" # Generate with: authelia crypto hash generate argon2
    email: admin@example.com
    groups:
      - admins
      - users
```

4. **Generate secrets:**

```bash
# Generate Authelia secrets
export AUTHELIA_JWT_SECRET=$(openssl rand -base64 32)
export AUTHELIA_SESSION_SECRET=$(openssl rand -base64 32)
export AUTHELIA_STORAGE_ENCRYPTION_KEY=$(openssl rand -base64 32)

# Save to .env.authelia
cat > .env.authelia <<EOF
AUTHELIA_JWT_SECRET=$AUTHELIA_JWT_SECRET
AUTHELIA_SESSION_SECRET=$AUTHELIA_SESSION_SECRET
AUTHELIA_STORAGE_ENCRYPTION_KEY=$AUTHELIA_STORAGE_ENCRYPTION_KEY
EOF
```

5. **Start services:**

```bash
cd docs/auth/examples
docker compose -f authelia-docker-compose.yml --env-file .env.authelia up
```

6. **Access:**
   - Open: http://localhost:3000
   - Login with credentials from users_database.yml
   - Setup 2FA (TOTP app like Google Authenticator)

**Enterprise SSO:**
To integrate with Azure AD, Okta, or Google Workspace, update `configuration.yml`:

```yaml
identity_providers:
  oidc:
    clients:
      - id: semstreams-ui
        description: SemStreams Flow Builder
        secret: ${OIDC_CLIENT_SECRET}
        redirect_uris:
          - https://your-domain.com/oauth2/callback
        authorization_policy: two_factor
```

Then configure your OIDC provider (Azure AD, Okta, etc.) and point it to Authelia.

---

## Option 4: Backend-Managed Auth (Custom)

**When to use:**

- Backend already has authentication
- Custom auth requirements
- Existing user database

**Setup time:** Varies (depends on backend)

**Steps:**

1. **Backend implements auth endpoints:**

```go
// Example backend endpoints
POST /auth/login       // Login with credentials
GET  /auth/session     // Get current user
POST /auth/logout      // Logout
GET  /auth/providers   // List OAuth providers (optional)
```

2. **UI calls backend for auth:**

```typescript
// SvelteKit hooks.server.ts
export async function handle({ event, resolve }) {
  const session = event.cookies.get("session");

  if (!session) {
    return Response.redirect("/login");
  }

  // Validate session with backend
  const user = await fetch(`${BACKEND_URL}/auth/session`, {
    headers: { Cookie: `session=${session}` },
  });

  if (!user.ok) {
    return Response.redirect("/login");
  }

  event.locals.user = await user.json();
  return resolve(event);
}
```

3. **Deploy:**

```bash
# Build UI with auth code
npm run build
docker build -t semstreams-ui-auth .
docker compose up
```

**Note:** This requires UI code changes and breaks backend-agnostic design.

---

## Comparison

| Approach         | Setup  | Enterprise SSO | 2FA | Code Changes |
| ---------------- | ------ | -------------- | --- | ------------ |
| **No Auth**      | 0 min  | ❌             | ❌  | None         |
| **OAuth2-Proxy** | 15 min | ✅ (OIDC)      | ❌  | None         |
| **Authelia**     | 30 min | ✅ (OIDC)      | ✅  | None         |
| **Backend Auth** | Varies | ✅             | ✅  | UI + Backend |

---

## Testing Authentication

### Test OAuth2-Proxy:

```bash
# Check if protected
curl http://localhost:4180
# Should redirect to Google OAuth

# Check headers passed to upstream
docker compose logs oauth2-proxy | grep X-Forwarded
```

### Test Authelia:

```bash
# Check if protected
curl http://localhost:3000
# Should redirect to Authelia login

# Check session
curl -v http://localhost:3000 \
  -H "Cookie: authelia_session=<your-session-cookie>"
```

### Check auth headers received by backend:

```bash
# See what headers backend receives
docker compose exec backend env | grep -i remote
# Should show: Remote-User, Remote-Email, Remote-Groups
```

---

## Troubleshooting

### OAuth2-Proxy not redirecting

- Check Google OAuth redirect URI matches exactly
- Verify `--email-domain` matches your email
- Check logs: `docker compose logs oauth2-proxy`

### Authelia login fails

- Verify password hash: `docker compose exec authelia authelia crypto hash generate argon2`
- Check configuration: `docker compose exec authelia cat /config/configuration.yml`
- Review logs: `docker compose logs authelia`

### Headers not passed to backend

- Verify Caddy `forward_auth` with `copy_headers`
- Check backend receives headers: Add debug logging
- Test with curl: Include auth cookies

---

## Next Steps

After choosing an auth approach:

1. **Production deployment:**
   - Use HTTPS (Let's Encrypt with Caddy)
   - Set secure cookie flags
   - Use strong secrets (not example values)

2. **Configure your provider:**
   - Set up OAuth app in Google/Azure/GitHub
   - Configure OIDC in Authelia
   - Test with real users

3. **Backend integration:**
   - Read auth headers: `Remote-User`, `Remote-Email`, `Remote-Groups`
   - Implement authorization logic
   - Audit log with user identity

4. **Documentation:**
   - Document auth setup for your users
   - Provide example credentials
   - Create onboarding guide

---

## Recommended: Start with OAuth2-Proxy

For most teams, we recommend starting with **OAuth2-Proxy + Google OAuth**:

- Quick setup (15 minutes)
- No code changes
- Works with existing Google Workspace
- Easy to understand
- Can upgrade to Authelia later for 2FA

Then upgrade to **Authelia** when you need:

- Two-factor authentication
- More complex access rules
- Session management
- Multiple auth backends (LDAP + OIDC)

---

## Resources

- **OAuth2-Proxy**: https://oauth2-proxy.github.io/oauth2-proxy/
- **Authelia**: https://www.authelia.com/
- **Caddy forward_auth**: https://caddyserver.com/docs/caddyfile/directives/forward_auth
- **Auth.js (SvelteKit)**: https://authjs.dev/reference/sveltekit
- **OIDC providers**: Azure AD, Okta, Google Workspace, Keycloak

For questions, see `docs/AUTH_OPTIONS.md` for detailed architecture discussion.
