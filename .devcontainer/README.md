# Canonical sandbox profiles

Hand-authored `devcontainer.json` profiles consumed by
`cmd/semteams/sandboxmanager` (the Layer-2 sandbox manager from
[ADR-043](../docs/adr/043-devcontainer-as-sandbox-spec.md)). One
profile per directory; each is a self-contained
[containers.dev](https://containers.dev) specification the manager
shells to `@devcontainers/cli` to materialize.

| Profile | Path | Use case |
|---|---|---|
| `go-backend@v1` | [`go-backend/devcontainer.json`](go-backend/devcontainer.json) | Pure backend (Go test/build/lint). No docker socket. Restricted network. |
| `svelte-ui@v1` | [`svelte-ui/devcontainer.json`](svelte-ui/devcontainer.json) | Frontend (npm, vite, playwright). Public network for browser installs + staging endpoints. |
| `full-stack-e2e@v1` | [`full-stack-e2e/devcontainer.json`](full-stack-e2e/devcontainer.json) | agentic-e2e stack (both toolchains + docker-outside-of-docker). The only profile that allows docker-socket privilege. |

## Network posture (MVP) — IMPORTANT

The `customizations.semteams.advertises` arrays describe what the
catalog *promises*. The actual realization layer (`@devcontainers/cli`
bringing up a bridge-networked container) does NOT firewall egress
in MVP. "Restricted" in the catalog means **the profile does not
explicitly opt into `network: public` in admission** — it does NOT
mean the host firewall blocks outbound traffic.

What this means in practice:

- A coordinator that emits `network: restricted` and gets routed
  to `go-backend` will be admitted. The container has bridge
  network with default egress.
- A coordinator that emits `network: public` against `go-backend`
  will be **denied at admission** (the profile's `AllowedNetwork`
  list is `[restricted, none]`).
- The actual egress restriction is a follow-up: either an egress
  proxy (operator-managed) or `runArgs: ["--network=…"]` per
  profile. Tracked under ADR-043 §Risks #6 follow-up.

Per the [structural-over-LLM-judgment](../docs/adr/043-devcontainer-as-sandbox-spec.md#decision)
principle, the catalog's contract is between Coordinator and the
manager — admission is the gate, not the network stack. If a
workload requires actual egress sandboxing, file an ADR addendum
+ wire the proxy or runArgs.

## Pinning posture

Per [[web-research-before-version-pins]] and ADR-043 §Risks:

- **Base images**: pinned to patch version
  (`mcr.microsoft.com/devcontainers/go:2.1.2-1.25-bookworm`).
  Operators bump per security advisory; the catalog `Version`
  ticks alongside.
- **devcontainer features**: major-version-pinned
  (`ghcr.io/devcontainers/features/github-cli:1`). Major bumps
  are spec-breaking and rare; we accept patch-level mobility on
  features. Specific tool versions (e.g. `gh` cli version) are
  passed as feature options.
- **Runtime-installed binaries** (Task, Playwright browsers): pinned
  via `containerEnv` constants the `postCreateCommand` references.
  Update these in lockstep with the catalog `Version` bump in
  [`cmd/semteams/sandboxmanager/catalog_builtin.go`](../cmd/semteams/sandboxmanager/catalog_builtin.go).

## When to add a profile

Adding a profile is high-friction by design (see
ADR-043 §Decision "Why catalog beats render-per-request for MVP").
Add one when:

- An existing real-LLM smoke surfaces a structured `MatchProfile`
  no-match against a Coordinator-emitted `SandboxRequirements`
  shape, AND
- The unmet capability list points to a coherent new role
  (e.g. python ML workload + jupyter) that won't be a one-off.

The PR adds the file under `.devcontainer/<name>/`, the catalog
metadata to `catalog_builtin.go`, contract-test coverage to
`test/integration/sandbox_profile_contract_test.go`, and a
`customizations.semteams.notes` field explaining the
trust/network/privilege rationale.

## When to bump a profile version

Bump `customizations.semteams.profile` and the catalog `Version`
in lockstep when:

- The base image upstream-tag changes (Bookworm → Trixie).
- A feature major version bumps in a contract-breaking way.
- The runtime-installed Task / Playwright versions bump and the
  bump invalidates prior cached attestations (e.g. a Playwright
  major).

Patch-level updates (jq, sub-minor Task) don't need a version
bump — they ride the postCreate refresh on next attestation TTL
expiry.

## Validation

```bash
# JSON syntax
for d in .devcontainer/*/devcontainer.json; do
  python3 -m json.tool "$d" >/dev/null && echo "$d OK"
done

# Contract tests (integration build tag; requires devcontainer CLI + Docker)
SANDBOX_PROFILE_SMOKE=1 task test:integration -- \
  -tags=integration -run TestSandboxProfileContract ./test/integration/...
```

The contract test brings each profile up, runs its advertised
capabilities as probes, and tears down. CI gates merges on it
per ADR-043 §Risks #6 ("Profile drift").
