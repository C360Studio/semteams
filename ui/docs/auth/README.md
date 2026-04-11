# Authentication Documentation

This directory contains authentication options and examples for the SemTeams UI.

> **History note (2026-04):** These docs were written when this tree targeted
> "any SemStreams-based backend" as its origin. That framing is stale —
> `semteams/ui/` is now the dedicated UI for the semteams backend. The
> architectural analysis (Pattern 1 reverse-proxy auth > Pattern 2
> backend-managed auth) still applies. References to "backend-agnostic"
> below should be read as "UI is decoupled from backend auth logic" — the
> semteams backend doesn't implement auth endpoints, so infrastructure-level
> auth (Pattern 1) remains the recommended approach.

## Quick Navigation

- **[AUTH_OPTIONS.md](../AUTH_OPTIONS.md)** - Complete analysis of auth patterns and architecture
- **[QUICK_START.md](./QUICK_START.md)** - Step-by-step setup guides for each auth approach
- **[examples/](./examples/)** - Ready-to-use docker-compose configurations

## TL;DR - What Should I Use?

### For Development

→ **No auth** - Just use standard docker-compose.yml

### For Small Teams (< 50 users)

→ **OAuth2-Proxy + Google OAuth**

- 15 minute setup
- Sign in with Google
- Email domain restrictions
- See: [QUICK_START.md](./QUICK_START.md#option-2-oauth2-proxy--google-oauth-simple)

### For Enterprise (SSO Required)

→ **Authelia + OIDC**

- 30 minute setup
- Full SSO (Azure AD, Okta, Google Workspace)
- Two-factor authentication
- Session management
- See: [QUICK_START.md](./QUICK_START.md#option-3-authelia-full-sso-with-2fa)

### If semteams ever implements auth endpoints

→ **Backend-managed auth**

- semteams backend would provide auth endpoints
- UI calls the backend for login/session
- Requires UI code changes
- Not currently recommended — semteams does not implement auth
- See: [AUTH_OPTIONS.md](../AUTH_OPTIONS.md#pattern-2-backend-managed-authentication)

## Architecture Overview

The SemTeams UI supports multiple authentication patterns:

```
Pattern 1: Reverse Proxy Auth (RECOMMENDED)
Browser → Caddy (auth) → SvelteKit → Backend
               ↓
          Auth Service (Authelia/OAuth2-Proxy)

Pattern 2: Backend-Managed Auth
Browser → SvelteKit → Backend (validates auth)

Pattern 3: SvelteKit Auth
Browser → SvelteKit (Auth.js) → Backend
               ↓
          OAuth Providers
```

**Recommendation:** Pattern 1 (Reverse Proxy) keeps the UI decoupled from auth logic and requires no code changes.

## Files

```
docs/auth/
├── README.md                           # This file
├── QUICK_START.md                      # Setup guides
├── examples/
│   ├── authelia-docker-compose.yml     # Full SSO setup
│   ├── Caddyfile.authelia              # Caddy config for Authelia
│   └── oauth2-proxy-docker-compose.yml # Simple OAuth setup
└── ../AUTH_OPTIONS.md                  # Detailed architecture analysis
```

## Getting Started

1. Read [QUICK_START.md](./QUICK_START.md) to choose your auth approach
2. Copy the relevant docker-compose file from `examples/`
3. Follow the setup steps (15-30 minutes)
4. Deploy!

## Key Benefits

### No Code Changes Required ✅

- Auth handled at infrastructure level (Caddy + auth service)
- Keeps the UI decoupled from auth concerns
- Works whether or not the semteams backend gains auth endpoints later

### Optional by Default ✅

- Deploy without auth for internal networks
- Add auth when needed
- No forced auth solution

### Enterprise Ready ✅

- OAuth 2.0 / OpenID Connect
- SSO with Azure AD, Okta, Google
- Two-factor authentication (Authelia)
- Session management

### 2025 Best Practices ✅

- OAuth at the edge (reverse proxy)
- OIDC federation
- Passwordless options (future)
- Zero trust architecture

## Security Considerations

All examples follow security best practices:

- ✅ HTTPS recommended (use Let's Encrypt with Caddy)
- ✅ HttpOnly, Secure cookies
- ✅ CSRF protection
- ✅ Session timeout
- ✅ Strong secrets (generated, not hardcoded)

See [AUTH_OPTIONS.md - Security Considerations](../AUTH_OPTIONS.md#security-considerations) for details.

## Common Questions

### Q: Do I need authentication?

**A:** Only if your deployment is internet-facing or you need user tracking. Internal tools on private networks may not need auth.

### Q: Can I use my existing SSO?

**A:** Yes! Authelia supports OIDC, so you can integrate with Azure AD, Okta, Google Workspace, Keycloak, etc.

### Q: Can I customize the login page?

**A:** Yes with Authelia (fully customizable). OAuth2-Proxy uses provider's login page (Google, GitHub, etc.).

### Q: What about API keys for automation?

**A:** Authelia supports API tokens. OAuth2-Proxy can be configured for service accounts. Backend-managed auth can implement custom token systems.

### Q: Can I mix auth and no-auth deployments?

**A:** Yes! Use Caddy access rules to protect only certain paths, or deploy separate instances (one with auth, one without).

## Next Steps

1. **Choose your approach** - See QUICK_START.md
2. **Test locally** - Use provided docker-compose examples
3. **Configure production** - Set up HTTPS, strong secrets, etc.
4. **Document for users** - Add auth setup to your deployment docs

## Support

For detailed architecture analysis and decision-making guidance, see:

- [AUTH_OPTIONS.md](../AUTH_OPTIONS.md) - Full pattern analysis
- [Decision Matrix](../AUTH_OPTIONS.md#decision-matrix) - Compare approaches
- [Security Considerations](../AUTH_OPTIONS.md#security-considerations)

## Contributing

Have a better auth pattern? Want to add examples for other providers?
See [CONTRIBUTING.md](../../CONTRIBUTING.md) for how to contribute.

---

**Summary:** For most users, start with OAuth2-Proxy (15 min setup). Upgrade to Authelia when you need 2FA and enterprise SSO. Both require zero code changes to the SemTeams UI.
