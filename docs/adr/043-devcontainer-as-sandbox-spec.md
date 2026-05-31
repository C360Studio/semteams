# ADR-043: Sandbox-requirements contract, canonical-profile catalog, and devcontainer.json as the realization layer

## Status

**Accepted (2026-05-31).** Pivots the sandbox-bootstrap pack from
a SemTeams-bespoke composer pipeline to a **three-layer model**:

1. Coordinator reasons over a typed `sandbox_requirements` contract
   (capabilities needed, not shell to produce them).
2. A **sandbox manager** resolves requirements → canonical
   profile (a hand-authored devcontainer.json) → runs preflight
   probes → emits an **attestation** of verified capabilities.
3. Routing to downstream agents (`go-developer`, `svelte-developer`,
   etc.) happens **only after attestation** — coordinator consumes
   `ready/failed/degraded + verified capabilities`, never container
   internals.

Realization layer is the multi-vendor
[`containers.dev`](https://containers.dev) spec, executed via the
[`@devcontainers/cli`](https://github.com/devcontainers/cli)
reference implementation (shell-out, not library import).
[DevPod](https://github.com/loft-sh/devpod) is the documented
multi-backend upgrade path.

This ADR **supersedes the sandbox-bootstrap pack mechanics** in
[ADR-042 §addendum 2026-05-29 §A/§B](042-coordinator-instantiated-flows-via-templates.md)
and **retires the PR 3.1 / 3.2 / 3.3 / 3.4 forward-stamping chain**.
The orchestration substrate ADR-042 introduced (coordinator routes
to category packs; rule pack with recovery cycles; reviewer-as-gate
discipline; chain-pause + needs-clarification recovery) **stays
load-bearing** — but the bootstrap rule pack itself shrinks
significantly because execute is no longer a chain hop.

It **does not change** ADR-029 (product-shell wiring), ADR-037
(chain-pause), ADR-038 (chain entity), ADR-039
(needs-clarification recovery), ADR-041 (MVP role compression), or
ADR-042's substrate-plus-overlays MVP.

## Why this exists

### Smoke #11 + #12 surfaced two findings in two days

**Smoke #11** (2026-05-31, [[smoke11-findings]]) wedged at 12
identical plan calls because our `toolchain` field had a thin
`type: object` schema with no inner shape. Schema-description
tightening (commit 45509ef) closed it; gemini-flash subsequently
emitted the correct intent shape on first call in smoke #12.

**Smoke #12** (2026-05-31, [[smoke12-findings]]) reached the
6-loop happy-ish path BUT reviewer terminated with
`needs_clarification, reason="run state structurally malformed:
plan_hash, signature, and workspace_path not available from run
entity or verify loop entity"`. The reviewer's spawn-prompt
substitution against the verify loop entity resolved to empty
because verify never forward-stamped `sandbox.tenant.*`. This is
the exact pattern PR 3.2 closed for the execute→verify hop one
month earlier. PR 3.4 was being drafted to close the same shape
for verify→reviewer — i.e. extending an accumulating reinvention.

### The pause-and-think moment

Mid-PR-3.4, an honest architectural review surfaced that we had
been steadily reinventing **devcontainer.json + docker-compose
+ `devcontainer up`** badly. The structured-intent layer (PR 3.3)
mapped roughly 1:1 to the devcontainer spec:

| Our concept | devcontainer.json equivalent |
|---|---|
| `source.kind=git` + `repo_url/ref` | git clone (or workspace mount) |
| `dependencies[{kind:toolchain_go}]` | `ghcr.io/devcontainers/features/go:1` |
| `dependencies[{kind:toolchain_node}]` | `ghcr.io/devcontainers/features/node:1` |
| `dependencies[{kind:apt, packages:[]}]` | features `common-utils` + apt feature |
| `dependencies[{kind:go_mod_download}]` | `postCreateCommand` |
| `mounts[{volume_suffix, path}]` | devcontainer `mounts` (same name) |
| `docker_socket_mount: true` | `features/docker-outside-of-docker` |
| `smoke.command` + `expects` | `postStartCommand` + health probe |
| Cache reuse via plan_hash | docker layer cache + content-addressed labels |
| `semteams-tenant-<sig>` container name | container labels + `--project-name` |

Our `emit_bootstrap_{plan,execute,verify,committed}` chain — and
the forward-stamping pattern that grew across PR 3.1 → 3.2 → 3.4 to
bridge per-hop substitution gaps — was mostly machinery for
executing what `devcontainer up` does in one command. The
substitution-chain work was solving a problem that disappears
under the right substrate.

### The landscape scan

Before pivoting we surveyed the current sandbox / ephemeral-dev-env
landscape ([[sandbox-landscape-2026-05-31]] for the full report).
Constraints applied:

1. **STRONG preference for spec/DSL over library/project as
   dependency** — "I am not a fan of us having a big project as a
   direct dep as there are always ideological and velocity
   mismatches that will haunt us as soon as we import."
2. **Agentic-aware solutions in scope** — the 2025-2026 wave of
   agent-purpose-built sandboxes (E2B, Modal, Daytona, etc.).
3. **Self-hosted preferred** — current architecture uses local
   Docker socket (DooD).
4. **Reinvention isn't a great answer either.**

Verdict: **devcontainer.json + `@devcontainers/cli`** wins on
every constraint. Spec-only dep + reference-CLI shell-out is
exactly Coby's preferred dep shape. Multi-vendor governance
(Microsoft + GitHub + JetBrains + loft-sh) — no single-org
velocity risk. Consumed by Anthropic Managed Agents, Cursor,
GitHub Codespaces, Docker AI Sandboxes → LLM training corpora
know the schema cold. Self-hosted, local-Docker-native.

### The architectural insight (Gemini feedback, 2026-05-31)

A reviewer correctly identified that the first cut of this ADR
conflated layers and risked making Coordinator the provisioning
engine. The sharper framing — three layers with attestation as the
bridge — is what this ADR adopts. Specifically:

> **Do not let Coordinator become a provisioning engine.** Using
> the orchestration-check lens: Coordinator/workflow owns
> sequencing and readiness decisions; a sandbox component owns
> execution mechanics; rules can trigger state transitions but
> should not accumulate multi-step provisioning logic.

That principle is the architectural anchor for everything below.

## Three-layer model

```
┌────────────────────────────────────────────────────────────────┐
│ LAYER 1 — Coordinator reasons over typed requirements          │
│                                                                │
│   sandbox_requirements:                                        │
│     languages: [go, node]                                      │
│     tools:     [task, docker, playwright]                      │
│     services:  [nats, postgres]                                │
│     network:   restricted | public | none                      │
│     secrets:   [openai_api_key]                                │
│     mounts:    [workspace-write]                               │
│     privileges:[docker-socket?]                                │
│     verification:                                              │
│       - task build                                             │
│       - task ui:check                                          │
└────────────────────────────┬───────────────────────────────────┘
                             │  request
                             ▼
┌────────────────────────────────────────────────────────────────┐
│ LAYER 2 — Sandbox manager resolves + admits + materializes     │
│                                                                │
│   1. profile_match(requirements) → one of N canonical profiles │
│      (or "no match — escalate to human / new profile request") │
│   2. admission_check(requirements)                             │
│      → reject if privileged + not approved                     │
│      → reject if secrets requested + not pre-provisioned       │
│      → reject if public network + not approved for chain       │
│   3. devcontainer up --workspace-folder ${TENANT_ROOT}/<sig>   │
│   4. preflight_probes() — execute the requirements.verification │
│      list inside the container; capture exit codes + stdout    │
│   5. emit attestation:                                         │
│        sandbox.attestation.ready: bool                         │
│        sandbox.attestation.profile: go-backend@v1              │
│        sandbox.attestation.image_digest: sha256:…              │
│        sandbox.attestation.verified.{go,node,task,…}: ok|fail  │
│        sandbox.attestation.degraded_reasons: [...]             │
└────────────────────────────┬───────────────────────────────────┘
                             │  attestation triples
                             ▼
┌────────────────────────────────────────────────────────────────┐
│ LAYER 3 — Coordinator routes on attestation, never internals   │
│                                                                │
│   if attestation.ready and matches needs → route to            │
│      go-developer / svelte-developer / etc.                    │
│   if attestation.degraded → route to remediation persona       │
│   if attestation.failed → route to admission-review or         │
│      respond_direct(failure surfaced to user)                  │
└────────────────────────────────────────────────────────────────┘
```

### Why the layers matter

**Layer 1 keeps Coordinator domain-shaped.** Coordinator persona
prose says "this task needs Go 1.25 and docker socket access" —
NOT "compose a `docker run -v /var/run/docker.sock:…
golang:1.25-bookworm sleep infinity`." That's exactly the
[[personas-should-not-author-shell]] rule applied to the
Coordinator level (PR 3.3 applied it at the plan persona).

**Layer 2 is one component, not a chain.** Pre-pivot we had
plan → execute → verify → reviewer → committed as a 5-loop
state machine. Under this model the manager owns
*profile-match + admission + up + probe + attest* as ONE
operation. Rule packs trigger it ("requirements emitted →
manager invoked"); they don't accumulate the mechanics.

**Layer 3 makes routing verifiable.** Coordinator can say "I
won't route to svelte-developer because the attestation says
node is NOT ready" — backed by a probe result, not a hope. The
reviewer-as-gate seam from ADR-042 becomes the
**admission-approval seam**: privileged requirements wait at
admission for human/policy approval, mirroring the existing
tool-approval pattern.

### Operational state vs domain truth

Attestation triples are **operational state**, not domain graph
truth. They live in a `sandbox.attestation.*` namespace (separate
from `sandbox.tenant.*` which captures the long-lived registry)
and they age out — an attestation from 24 hours ago doesn't say
the env is ready *now*. The freshness check is the manager's job;
Coordinator just reads "is there a fresh-enough attestation for
this profile?"

## Decision

### Adopt the three-layer model

The boundaries above are normative. We do NOT let the manager's
internals leak into Coordinator prose; we do NOT let Coordinator
emit container-level intent (image, mounts, privileges as
first-class fields — those live inside profiles or admission
policy, not in the requirements contract).

### Canonical-profile catalog (MVP), LLM-rendered (v3 deferred)

We ship 2-3 hand-authored canonical profiles as
`devcontainer.json` files in the repo:

| Profile | `.devcontainer/<profile>/devcontainer.json` | Use case |
|---|---|---|
| `go-backend` | `golang:1.25-bookworm` + features for task, gh, jq | Most semteams backend work; `task test`, `task lint` |
| `svelte-ui` | `node:22-bookworm` + features for playwright browsers | UI work, Playwright e2e |
| `full-stack-e2e` | both toolchains + docker-outside-of-docker | The agentic-e2e stack itself |

Profile-match is a Go function: given `sandbox_requirements`, score
each profile by capability overlap, pick the best (or fail-fast
with "no match — needs new profile"). For MVP the matcher is a
flat lookup table; replaceable with an LLM-driven
catalog-extension step in v3 when the catalog stops covering new
asks.

**Why catalog beats render-per-request for MVP:**
- Smaller attack surface — auditable hand-authored specs vs.
  LLM-emitted JSON
- Profiles capture *reviewed* admission policy (e.g.
  `docker-outside-of-docker` lives in `full-stack-e2e` only because
  the reviewer signed off once, not every request)
- Image-layer cache reuse is automatic when most teams pick from
  the same N images
- LLM fidelity is moot when the LLM doesn't emit the spec at all
- Re-introducing render-per-request later is straightforward when
  the catalog hits a real gap; the inverse (deleting a renderer
  nobody trusts) is harder

The PR 3.3 `RecipeIntent` struct **becomes `SandboxRequirements`** —
a typed capability contract, not a recipe. Field shapes overlap
significantly; the rename + reframe is the load-bearing change.

### Admission checks as first-class preflight

Before `devcontainer up`, the manager runs admission against the
declared `sandbox_requirements`. Hard policy gates:

| Requirement | Gate | Approval path |
|---|---|---|
| `privileges: [docker-socket]` | Requires explicit chain-level approval | Existing tool-approval seam (reviewer or human) |
| `privileges: [privileged]` (full --privileged) | Forbidden by default | Operator-level config flag + chain approval |
| `mounts: [host-path:...]` | Requires explicit chain-level approval | Tool-approval seam |
| `network: public` | Requires explicit chain-level approval | Tool-approval seam |
| `secrets: [...]` | Each secret must be pre-provisioned in operator config | Boot-time policy (no per-request) |
| `image_digest` not in allowlist | Forbidden if profile is unrecognized | Profile catalog acts as image allowlist |

Admission failures route via `decide(needs_clarification,
reason="admission denied: <specific gate>")` so the existing
recovery rule + reviewer pattern surface the gap to the operator.
No silent fail-shut; the requirement-vs-policy mismatch is
human-visible.

### Attestation triple shape

The manager emits attestation triples on a `sandbox.attestation.*`
namespace stamped on the requesting chain entity (NOT on a tenant
loop — operational state, not loop history):

```
sandbox.attestation.profile        = "go-backend@v1"
sandbox.attestation.image_digest   = "sha256:..."
sandbox.attestation.ready          = true
sandbox.attestation.attested_at    = "2026-05-31T..."
sandbox.attestation.degraded       = false
sandbox.attestation.degraded_reasons = []
sandbox.attestation.verified.go    = "1.25.4"
sandbox.attestation.verified.task  = "v3.34.1"
sandbox.attestation.verified.docker= "27.3"
sandbox.attestation.failed_probes  = []   (or list of probe-name strings)
```

Coordinator's routing decision substitutes these via
`$entity.triple.sandbox.attestation.*`. Freshness check: if
`attested_at` is older than the profile's TTL (24h default, per-
profile configurable), the manager re-runs `up + probe + attest`
before returning ready.

### Tool surface

| Tool | Status | Notes |
|---|---|---|
| `request_sandbox` | NEW | Coordinator calls with `sandbox_requirements`. Tool invokes the manager: profile-match → admission → up → probe → attest → return `(ready, attestation, container_ref)`. Synchronous from the coordinator's POV; manager internals are not exposed. |
| `query_sandbox_attestation` | NEW | Read-only: look up a fresh attestation for `(profile, requirements_hash)`. Used by coordinator before deciding to invoke a fresh `request_sandbox`. |
| `query_sandbox_tenant` | KEEP (refactored) | Registry lookup for the long-lived tenant record (signature → container_ref). Manager-side, not coordinator-side. |
| `emit_bootstrap_plan` | RETIRE | Composer + per-request render disappear. The catalog IS the plan. |
| `emit_bootstrap_execute` | RETIRE | Execute is the manager calling `devcontainer up`. |
| `emit_bootstrap_verify` | RETIRE | Verify is the manager's probe step, emitting attestation. |
| `emit_bootstrap_committed` | RETIRE | Registry commit happens inside the manager on successful `up + probe`; not a chain hop. |

The four `emit_bootstrap_*` tools collapse into one synchronous
`request_sandbox` call. The 5-loop chain becomes a 1-loop call.
Per-hop substitution disappears.

### Persona + rule pack consequences

The sandbox-bootstrap rule pack from ADR-042 §addendum **shrinks
dramatically**:

- `provisioner-bootstrap-{plan,execute,verify}` personas — RETIRE.
  They were the chain hops for the multi-step provisioning; gone
  under the manager model.
- `reviewer-bootstrap` persona — REPURPOSE as `admission-reviewer`:
  human-/policy-approval gate for privileged requirements. Fires
  only when admission flags a requirement that needs explicit
  sign-off.
- Rules 01-07 — RETIRE. Replaced by ONE rule that fires when
  `request_sandbox` returns `attestation.ready=false AND
  admission_pending=true` → spawn admission-reviewer.

The orchestration substrate (rule engine, recovery loops,
chain-pause, reviewer-as-gate seam) is *used* by this design but
the bootstrap-specific rules collapse.

## v2 design sketch — implementation

### Layer 1: requirements contract (Go)

```go
package sandboxfleet

// SandboxRequirements is the typed capability contract Coordinator
// emits. Replaces PR 3.3's RecipeIntent. Field names are domain-
// shaped (languages, tools, services) — NOT container-shaped
// (image, mounts, privileges). The latter live in profiles or
// admission policy.
type SandboxRequirements struct {
    Languages    []string          // ["go", "node"]
    Tools        []string          // ["task", "gh", "docker", "playwright"]
    Services     []string          // ["nats", "postgres"]
    Network      NetworkPolicy     // restricted | public | none
    Secrets      []string          // ["openai_api_key"]
    Mounts       []MountClass      // workspace-write | workspace-read
    Privileges   []Privilege       // docker-socket
    Verification []VerifyProbe     // [{name: "task build", expect_exit: 0}]
}
```

Validation invariants from PR 3.3 reviewer pass transfer where
applicable (REC-1 path defense becomes "Mounts class enum, no
arbitrary paths"; REC-7 whitespace rejection becomes
"verification probe commands trimmed and validated").

### Layer 2: sandbox manager

```go
package sandboxmanager

func Request(ctx, req SandboxRequirements) (Attestation, error) {
    profile, err := MatchProfile(req)          // catalog lookup
    if err != nil { return failed(err), nil }

    if err := AdmissionCheck(req, profile); err != nil {
        return failed(err), nil   // needs approval — coordinator routes
    }

    ref, err := DevcontainerUp(profile, signatureFor(req, profile))
    if err != nil { return failed(err), nil }

    probeResults := RunProbes(ctx, ref, req.Verification)
    att := Attest(profile, ref, probeResults)
    StampAttestation(ctx, callerChainID, att)
    return att, nil
}
```

Devcontainer `up` and probes shell out via the chain-scoped bash
tool. No Go library import; `@devcontainers/cli` is invoked as
`devcontainer up --workspace-folder ${SEMTEAMS_TENANT_ROOT}/<sig>
--config <profile>` and the manager parses its JSON output.

### Layer 3: coordinator persona prose

```markdown
# Coordinator (sandbox-aware routing)

When a task needs a prepared environment:

1. Sketch the SandboxRequirements (languages, tools, network, etc.).
2. Call `query_sandbox_attestation(profile_hint?, requirements)`
   to check if a fresh attestation already covers it.
3. If no fresh attestation: call `request_sandbox(requirements)`.
   The tool returns an Attestation synchronously.
4. Branch on the attestation:
   - `ready=true` + verified capabilities match needs →
     decide(route, target=<go-developer|svelte-developer|...>)
   - `degraded=true` → decide(route, target=<remediation-persona>,
     reason=<degraded_reasons>)
   - `ready=false, admission_pending=true` → recovery rule fires
     admission-reviewer
   - `ready=false, terminal=true` → decide(respond_direct, reason=
     <user-facing failure>)
```

Coordinator never authors `devcontainer up`, never reads container
internals, never decides whether docker socket is OK to mount.

## What we keep, what we drop

**Keep (load-bearing):**
- Orchestration substrate from ADR-042 (coordinator routes, rule
  engine, recovery cycles, chain-pause, reviewer-as-approval gate)
- Canonical signature + content-addressed hash semantics from
  PR 3.3 — re-keyed to `(profile, requirements_hash)` instead of
  `(canonical inputs, recipe hash)`
- Schema-shape-for-cross-field-constraints rule
  ([[schema-shape-for-cross-field-constraints]])
- Personas-should-not-author-shell rule
  ([[personas-should-not-author-shell]]) — reinforced and lifted
  to apply to Coordinator-level provisioning prose too
- `sandboxfleet.TenantRegistry` (long-lived tenant records;
  separate namespace from per-request attestations)
- Tool-approval seam (becomes admission-approval seam)
- The structured-validation invariants from PR 3.3 reviewer pass
  (REC-1 path defense, REC-3 toolchain version interpolation,
  REC-7 URL whitespace rejection, determinism) transfer to
  Layer 1's requirements validation + Layer 2's profile renderer

**Drop (machinery that disappears under three-layer + catalog):**
- `sandboxfleet.Compose` (the bespoke shell renderer)
- All four `emit_bootstrap_*` tools and executors (plan, execute,
  verify, committed)
- PR 3.4 `emit_bootstrap_verify` forward-stamp extension
  (uncommitted, stashed at `stash@{0}`)
- Per-hop `$entity.triple.sandbox.tenant.*` substitution in rules
  02b/03/04
- `provisioner-bootstrap-{plan,execute,verify}` personas
- Rules 01-07 from the sandbox-bootstrap pack (replaced by one
  admission-spawn rule)
- The bash-driven `docker run`, `docker exec`, `docker inspect`
  dance — devcontainer CLI owns these
- `DependencyKind` enum (replaced by profile catalog + devcontainer
  features ecosystem)

**Net code change estimate (revised):** ~900 LoC retired
(composer + 4 tools + 3 personas + 7 rules), ~300 LoC added
(requirements contract + sandbox manager + 1 admission rule + 1
admission-reviewer persona + 3 canonical profile devcontainer.json
files). Composer tests (~260 LoC) get partially repurposed as
requirements-validation + admission tests; most invariants
transfer.

## Risks

1. **Admission policy is now a first-class security property.** If
   admission checks are wrong (too lax: privileged escalation; too
   strict: legitimate work blocked), the impact is real. Mitigation:
   default-deny on privileges; reviewer-approval-required gates for
   every flag in the privileged set; admission denials are
   loud (`needs_clarification` to coordinator, surfaced to user).
2. **Profile catalog coverage.** 2-3 profiles will not cover every
   future task class. Mitigation: profile-match returns "no match
   — needs new profile" as a structured failure, coordinator
   surfaces it via `decide(respond_direct, reason=
   "needs new sandbox profile: <gap>")`. Operator adds a profile
   manually. v3 considers LLM-rendered profile extension when this
   becomes the bottleneck.
3. **`@devcontainers/cli` velocity.** Pinned in the sandbox
   Dockerfile; DevPod is documented multi-backend fallback.
4. **Features ecosystem dep.** Runtime references only
   (`ghcr.io/devcontainers/features/go:1`), same shape as image
   deps. Acceptable.
5. **Attestation staleness.** A "ready" attestation 24 hours old
   doesn't mean ready now. Mitigation: per-profile TTL +
   `query_sandbox_attestation` always returns the staleness flag
   so coordinator can decide to re-attest.
6. **Profile drift.** The 3 hand-authored devcontainer.jsons need
   maintenance. Mitigation: contract tests in CI that exercise
   each profile with `devcontainer up + probe` against a known
   workload; broken profiles fail CI loudly.

## PR sequence (revised per three-layer model)

1. **PR 4.1 — canonical profiles + preflight + attestation**
   - Hand-author `.devcontainer/go-backend/devcontainer.json`,
     `.devcontainer/svelte-ui/devcontainer.json`,
     `.devcontainer/full-stack-e2e/devcontainer.json`
   - Add `@devcontainers/cli` to sandbox Dockerfile (pinned)
   - New package `cmd/semteams/sandboxmanager/` with
     `Request(ctx, req) → Attestation` orchestrating
     profile-match + admission + up + probe + attest
   - Unit tests for: profile-match scoring, admission rules,
     attestation rendering (probe-result aggregation)
   - Contract tests: each canonical profile passes its own
     preflight probes (CI smoke against a known workload)
2. **PR 4.2 — coordinator-facing tools**
   - `request_sandbox` tool: wraps `sandboxmanager.Request`,
     stamps attestation triples on the chain entity
   - `query_sandbox_attestation` tool: read-only lookup with
     staleness flag
   - Update product_tools.go registrations
   - Retire `emit_bootstrap_{plan,execute,verify,committed}`
3. **PR 4.3 — persona + rule pack slim-down**
   - Retire `provisioner-bootstrap-{plan,execute,verify}` personas
   - Repurpose `reviewer-bootstrap` → `admission-reviewer`
   - Retire rules 01-07 from `configs/rules/sandbox-bootstrap/`
   - Add ONE rule: `admission_pending_to_reviewer` (fires on
     `request_sandbox` returning `admission_pending=true` →
     spawn admission-reviewer)
   - Update coordinator persona to read attestations + branch
     accordingly
4. **PR 4.4 — mock journey + real-LLM smoke #13**
   - Rewrite mock journey: 2-3 loops happy path
     (coordinator → request_sandbox → route-to-developer)
   - Optional 4th loop for admission-pending → admission-reviewer
   - Real-LLM smoke #13: full-stack-e2e profile against the
     semteams repo, expect ~$0.10–$0.30 (much faster than #12 —
     no 5-loop chain)
5. **PR 4.5 — symmetric tenant-root bind + per-tenant profile materialization**
   Closes smoke #13 probe-2 findings F5 + F6 (the production gaps
   that blocked the real `devcontainer up` path after probes 0 + 1
   proved the wiring).
   - F5: `SandboxAPIRunner.Up` stages the catalog profile JSON into
     `<workspaceFolder>/.devcontainer/devcontainer.json` via a
     leading `mkdir+cp` /exec call so devcontainer-cli's
     `devcontainer-lock.json` sibling write lands in tenant-writable
     space (the catalog mount stays `:ro` per operator trust posture).
   - F6: docker-compose binds `${SEMTEAMS_TENANT_ROOT}:${SEMTEAMS_TENANT_ROOT}`
     symmetrically (host source path == sandbox container target path)
     so devcontainer-cli's DooD-spawned sibling containers resolve
     `--mount source=<wsf>` against a real host path on the docker
     daemon. The pre-PR-4.5 `sandbox-agentic-workspaces` named volume
     was sandbox-internal-only and broke DooD path translation.
   - Runner uses the manager-supplied `workspaceFolder` verbatim for
     `--workspace-folder` (drop the `/workspace/<task_id>`
     substitution that was the F6 bug).
   - Manager default `TenantRoot`: `/tenants` → `/var/lib/semteams-tenants`
     (production-canonical). `SEMTEAMS_TENANT_ROOT` env overrides;
     Taskfile `stack:up` pre-mkdir's the e2e default
     (`{{.ROOT_DIR}}/.tenant-workspaces`) and resolves it via
     `pwd -P` so the bind source is absolute.
   - Operator's gate: Probe 2 retry + real-LLM smoke #13.

## Memory cross-links

- [[sandbox-landscape-2026-05-31]] — landscape research this ADR is based on
- [[smoke11-findings]] — schema-shape gap that motivated the schema fix
- [[smoke12-findings]] — verify→reviewer gap that triggered the architectural pause
- [[pr3-3-intent-restructure]] — composer side retired; intent struct survives renamed as `SandboxRequirements`
- [[personas-should-not-author-shell]] — survives, reinforced, lifted to Coordinator level
- [[schema-shape-for-cross-field-constraints]] — survives; applies to requirements + attestation schemas
- [[structural-over-llm-judgment]] — this ADR IS the principle applied at architecture level
- [[feedback_framework_alignment_review]] — discipline applied at addendum-time, not commit-time

## References

- [containers.dev spec](https://containers.dev/implementors/spec/)
- [@devcontainers/cli](https://github.com/devcontainers/cli)
- [DevPod (multi-backend devcontainer impl)](https://github.com/loft-sh/devpod)
- ADR-042 §addendum 2026-05-29 (the sandbox-bootstrap pack this ADR refactors)
- PR #441e342 (PR 3.3 structured intent + composer — composer retired by this ADR; intent struct repurposed)
- PR #45509ef (toolchain schema tightening — survives unchanged conceptually; the lesson informs requirements-contract design)
- Smoke #11 + #12 evidence under `/tmp/smoke11-pr3-3/` + `/tmp/smoke12-toolchain-schema/`
- Stash: `stash@{0}` "PR 3.4 sandbox-bootstrap verify forward-stamp (superseded by ADR-043 pivot to devcontainers)"
