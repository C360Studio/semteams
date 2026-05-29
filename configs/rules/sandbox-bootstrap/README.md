# Sandbox-bootstrap category rule pack

**ADR-042 §addendum 2026-05-29 — third category pack, foundation for
multi-category coordinator orchestration.** Provisions or reuses
**tenant containers** in the sandbox fleet, idempotently. Routed to by
the coordinator when a target requires a prepared execution environment
(typically as the precursor to an `autoresearch` arc, but composable
with any future category that needs a tenant).

The pack runs through the same substrate singletons as the research
and autoresearch packs — `configs/flow-bootstrap.json` wires the agentic
stack; this pack adds category-keyed rules + role-scoped persona
bundles.

## What this pack proves about the substrate

The research pack runs ONE arc with an LLM reviewer judging substance.
The autoresearch pack runs N arcs with an empirical reviewer (the
executor) routing per-iteration outcomes. This pack proves the substrate
can:

1. **Read and write a cross-run registry.** Each tenant container is
   identified by a stable target signature; the registry caches
   signature → container info so subsequent bootstrap invocations on
   the same target reuse rather than re-provision. The query primitive
   (`query_sandbox_tenant`) is read-only registry access; emit tools
   commit registry state.
2. **Chain to downstream categories.** Rule 07's wake-up coordinator
   has `action_allowlist: ["respond_direct", "ask_user", "autoresearch",
   "research"]` — NOT just the user-facing pair. This is the first
   pack to exercise the multi-category orchestration extension from
   ADR-042 §addendum §C.
3. **Manage long-lived infrastructure idempotently.** The plan persona's
   registry-check-first pattern means a bootstrap arc on an already-ready
   tenant completes in ~3 loops (plan-skip → reviewer-bootstrap →
   coordinator wake-up), not a full provision sequence. The skip path is
   the common case once a tenant has been provisioned once.

If this pack runs, the substrate-plus-overlays claim from ADR-042 is
backed by three structurally-distinct category contracts (substance
review, empirical review, infrastructure idempotency) AND the
multi-category orchestration pattern that makes them composable.

## Naming convention

Per ADR-042 open question #2, role tokens follow
`<cognitive-role>-<category>-<phase?>`:

| Role token | Phase | Persona dir |
|---|---|---|
| `provisioner-bootstrap-plan` | plan | `configs/personas/fragments/provisioner-bootstrap-plan/` |
| `provisioner-bootstrap-execute` | execute | `configs/personas/fragments/provisioner-bootstrap-execute/` |
| `provisioner-bootstrap-verify` | verify | `configs/personas/fragments/provisioner-bootstrap-verify/` |
| `reviewer-bootstrap` | (single-phase) | `configs/personas/fragments/reviewer-bootstrap/` |

`provisioner-bootstrap-` prefix lets `ls configs/personas/fragments/ |
grep ^provisioner-bootstrap-` enumerate the provisioning roles in
phase order. `reviewer-bootstrap` follows the `reviewer-<category>`
convention.

## Rules

| File | Trigger | Spawn / Stamp |
|---|---|---|
| `01-coordinator-bootstrap-spawn.json` | coordinator decide(bootstrap_sandbox) | provisioner-bootstrap-plan |
| `02a-plan-skip-to-verify.json` | provisioner-bootstrap-plan decide(skip) | provisioner-bootstrap-verify (fast path; tenant exists ready, skip provisioning, still verify) |
| `02b-plan-to-execute.json` | provisioner-bootstrap-plan decide(execute) | provisioner-bootstrap-execute (full provision or re-provision per plan) |
| `03-execute-to-verify.json` | provisioner-bootstrap-execute decide(verify) | provisioner-bootstrap-verify |
| `04-verify-to-reviewer.json` | provisioner-bootstrap-verify decide(emit) | reviewer-bootstrap |
| `05-reviewer-rejected-retry.json` | reviewer-bootstrap decide(insufficient) | provisioner-bootstrap-plan (re-plan, max_iterations=2) |
| `06-needs-clarification-replan.json` | any pack role decide(needs_clarification) | coordinator (max_iterations=3) |
| `07-reviewer-approved-to-coordinator.json` | reviewer-bootstrap decide(approved) | coordinator wake-up — **chained allowlist** (respond_direct + ask_user + autoresearch + research; NOT bootstrap_sandbox per §addendum §C loop-protection) |
| `08-loop-failed-pause.json` | any pack role outcome=failed | stamp chain.paused.marker |

### Iteration shape (rules 01-07)

The coordinator parses the user's target intent and routes via
`decide(bootstrap_sandbox, reason="<target description>")`. The reason
field carries the full target intent (command, repo, deps) — the plan
persona is responsible for canonicalizing and signing.

Rule 01 spawns `provisioner-bootstrap-plan`. Plan:
1. Reads the coordinator's reason via `read_loop_result`
2. Canonicalizes the target and computes the signature
3. Calls `query_sandbox_tenant(signature)` for registry state
4. If hit + fresh + ready → `decide(skip)` (rule 02a path)
5. If hit + stale OR miss → `decide(execute)` (rule 02b path) with
   provision/re-provision intent in the plan

**Skip path (rules 02a → 04 → 07):** plan → verify (re-runs smoke
checks against the existing tenant; if smoke passes, `decide(emit)`;
if smoke fails, plan got stale read → reviewer rejects, recovery
re-plans with `force_refresh=true`) → reviewer → coordinator wake-up.

**Provision path (rules 02b → 03 → 04 → 07):** plan → execute
(`docker run` + clone + install) → verify (smoke checks) → reviewer →
coordinator wake-up.

**Re-provision path:** same as provision but with `docker rm -f` of
the stale tenant container first; same downstream rules.

### Chained wake-up allowlist (rule 07)

Rule 07 is the first pack to exercise the per-pack-configurable
wake-up allowlist from §addendum §C. The bootstrap wake-up's
allowlist is:

```json
"action_allowlist": ["respond_direct", "ask_user", "autoresearch", "research"]
```

The wake-up coordinator decides:

- `decide(autoresearch, reason="<original intent + tenant_signature ref>")`
  — typical path when bootstrap was a precursor to autoresearch.
  The autoresearch arc spawns; its execute role reads the run entity's
  tenant_ref and routes `docker exec` to the right container.
- `decide(research, reason=...)` — possible but uncommon; research
  doesn't currently consume tenant containers (uses the always-warm
  sandbox). Surfaced in the allowlist for future packs that might.
- `decide(respond_direct, reason=...)` — bootstrap was the user's full
  ask ("provision a tenant for X"); deliver the result.
- `decide(ask_user, reason=...)` — bootstrap completed but the original
  intent was ambiguous about the downstream category.

**`bootstrap_sandbox` is explicitly excluded** from the allowlist —
loop protection per §addendum §C. A wake-up coordinator that wants to
re-route to bootstrap_sandbox must terminate via `respond_direct` /
`ask_user` and surface the contestation.

### Registry primitives (provided by §A foundation PR)

This pack assumes the following product-shell tools exist (ship in
the §A foundation PR per §G PR sequence):

- `query_sandbox_tenant(signature) → tenant_record` — read-only
  registry lookup. Returns `{state, container_name, image, workspace,
  ready_at, plan_hash, last_used}` or `null` on miss.
- `emit_bootstrap_plan(signature, base_image, clone_command,
  install_steps, volume_mounts, docker_socket_mount, verify_command,
  expected_smoke_signature, plan_hash, force_refresh)` — stamps the
  plan triples on the plan loop entity AND updates registry state
  to `provisioning`.
- `emit_bootstrap_verify(smoke_exit_code, smoke_stdout_tail,
  smoke_matches_expected, container_name, workspace_path)` — stamps
  verify-result triples on the verify loop entity. Does NOT commit
  registry ready state; reviewer-bootstrap approval drives that.
- `emit_bootstrap_committed(signature, container_name, image,
  workspace, plan_hash)` — called by reviewer-bootstrap on approval;
  updates registry state to `ready_running` + `ready_at=$now` +
  `last_used=$now`.

The exact tool surface is the §A foundation PR's design space; the
pack files reference them by these names as the canonical contract.

### No chain.mode / phasevalidator / chainstall

Same posture as research and autoresearch packs: this pack omits the
legacy chain.mode machinery (retired in MVP-7). Direct role+decision
matches; no chain-entity mode or phase-validator sentinels.

## Open substrate questions (filed as follow-ups)

These items inherit from the ADR-042 §addendum 2026-05-29 §F. Listed
here for pack-local reference:

1. **Freshness window detection.** v1: TTL (24h default) + plan-hash.
   Smarter detection is post-v1.
2. **Cross-tenant concurrency limits.** v1 has no host-level cap.
3. **Auto-GC.** v1 ships manual `task sandbox:gc`.
4. **Multi-tenant primitives in framework, not product shell.** Today
   product-local; graduate upstream if cross-product need surfaces.

Pack-specific open questions:

5. **Registry storage shape.** §addendum captures `SANDBOX_REGISTRY` as
   a new KV bucket. Alternative considered: virtual namespace within
   `ENTITY_STATES` keyed by entity ID `c360.sandbox.tenant.<signature>`.
   Tradeoffs documented in §A foundation PR design. The rules in this
   pack are storage-agnostic — they call `query_sandbox_tenant` and
   the emit tools; the foundation PR picks the storage shape.
6. **Tenant container name shape.** Pack assumes `semteams-tenant-<signature>`.
   §A foundation PR may pin a different convention; rules use
   `$entity.triple.sandbox.tenant.container_name` substitution after
   plan stamps it.

## Migration posture

Net-new pack on the post-MVP-7 substrate. No legacy predecessor.
Future revisions may move registry primitives upstream if semspec /
semdragons gain similar bootstrap needs. The pack file layout stays
stable; only the tool implementations would move.
