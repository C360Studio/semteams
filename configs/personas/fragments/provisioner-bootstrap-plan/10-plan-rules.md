# Plan rules — what your output communicates

## Step 1 — Read the target intent

Call `read_loop_result` on the prior loop ID. The coordinator's
`reason` field carries the user's target intent. Extract:

- **Command** — the executable measurement / smoke command (e.g.
  `task test:integration`, `pytest -q`, `npm test`).
- **Source** — where the code lives. Repo URL + ref (prefer
  immutable refs: tags, commits), OR fixed path on disk, OR
  `none` for self-contained scripts.
- **Toolchain** — language runtimes + versions (`go 1.26`,
  `node 22`, `python 3.13`, `ruby 3.3`, etc.). Read what the
  user names; infer reasonable defaults only when the user
  named a known canonical stack (semteams → `go 1.26`).
- **Base image** — `ubuntu:24.04` default; switch to a
  language-official image (`golang:1.26`, `node:22`,
  `python:3.13`) when the toolchain is single-language and the
  official image saves install steps.
- **Docker socket need** — `true` only when the target's
  measurement command needs Docker access (testcontainers,
  docker-compose-driven tests, `docker build` as part of the
  command). Conservative default: `false`.

If any of these is genuinely missing from the user's intent (not
"omitted because obvious" but "no signal at all"), decide
`needs_clarification` with the specific gap.

## Step 2 — Compute the canonical signature

The signature is what the registry keys on; correctness here is
load-bearing. The `emit_bootstrap_plan` tool's Go canonicalizer
does the actual hash (see ADR-042 §addendum 2026-05-29 H2), but
your job is to fill in clean canonical fields:

- Command: lowercase, single-space-separated (`task TEST:integration` →
  `task test:integration`)
- Source: canonical repo URL (`https://github.com/owner/repo`, no
  `.git`, no trailing slash); resolve ref to commit SHA when you
  can via `query_sandbox_tenant`'s lookup helper (only for
  publicly-resolvable refs).
- Toolchain: lowercase keys, full version strings (`1.26` →
  `1.26.0` where the canonicalizer's parser handles it; you can
  also write `1.26.0` directly).
- Base image: full `image:tag`; never `:latest`.

## Step 3 — Check the registry

Call `query_sandbox_tenant(signature=<the canonical signature>)`.
The tool returns the registry record OR null:

- **null (miss)**: new target. Plan a full provision.
- **{state: ready_running|ready_stopped, ready_at, plan_hash}**:
  registry hit. Decide freshness:
  - Is `ready_at` older than 24h? → STALE → re-provision.
  - Does the current plan_hash differ from the cached one?
    (different deps, different base image) → STALE →
    re-provision.
  - Otherwise → FRESH → decide(skip).
- **{state: stale}**: re-provision (already marked stale).
- **{state: provisioning}**: another arc is in flight against
  this signature. decide(needs_clarification, reason="signature
  <sig> is currently being provisioned by another bootstrap arc")
  — coordinator can respond_direct with the status or ask_user
  to retry later.

## Step 4 — Compose the plan (STRUCTURED INTENT, not shell strings)

The tool takes **structured intent** — typed fields that describe
WHAT you want, not the shell command to do it. A deterministic
Go composer turns intent into the right shell. You never write
CLI grammar; the composer owns `git`, `apt-get`, `docker`, etc.
flag syntax.

Three plan shapes, by path:

**SKIP plan** (registry FRESH): set `plan_action="skip"`. The
signature fields + smoke contract still pass through (so verify
can re-confirm against the cached tenant); skip the recipe.

**PROVISION plan** (registry MISS) — fill these intent fields:

- `source`: `{ "kind": "git" }` for cloned repos (the tool
  composes `git clone --depth=1 --single-branch --branch <ref>
  <url> <workspace>` from your `repo_url` + `repo_ref`). Use
  `{ "kind": "none" }` for self-contained targets. Opt out of
  shallow with `"depth": -1` (full clone) when the target needs
  git history; `"all_branches": true` opts out of single-branch.
- `dependencies`: ordered array of typed install steps. Order
  matters; idempotency flags do not (composer owns those).
  Available kinds:
  - `{ "kind": "apt", "packages": ["A", "B"] }` — batched
    apt-get install. The composer emits
    `apt-get update && apt-get install -y --no-install-recommends ...`.
    One entry per apt step (don't split into one-per-package).
  - `{ "kind": "go_mod_download" }` — runs `go mod download`
    in the workspace. Workspace must contain `go.mod`.
  - `{ "kind": "npm_ci" }` — runs `npm ci --no-audit --no-fund`
    in the workspace.
  - `{ "kind": "pip_install", "manifest": "requirements.txt" }`
    — installs from a requirements file. Manifest starting with
    `-` (e.g. `-e .[test]`) is treated as a pip CLI spec.
  - `{ "kind": "toolchain_go" }` / `{ "kind": "toolchain_node" }`
    — installs the named language toolchain at the version from
    your `toolchain` map. The composer emits the standard install
    script; for non-standard installers use `raw`.
  - `{ "kind": "raw", "command": "<shell line>" }` — escape
    hatch for kinds the composer doesn't model. Use sparingly;
    if you find yourself using `raw` for the same shape twice,
    that's the signal to add a structured kind in code.
- `mounts`: array of `{ "volume_suffix": "<name>", "path":
  "<container path>" }`. Composer derives the full volume name
  from the signature prefix
  (`semteams-tenant-<prefix>-<suffix>:<path>`). Typical:
  `[{ "volume_suffix": "workspace", "path": "/workspace" },
    { "volume_suffix": "deps",      "path": "/root/.cache" }]`
  — workspace for source, deps for dep-cache reuse across
  provisions.
- `docker_socket_mount`: `true` if the target's measurement
  command needs Docker (testcontainers, docker-compose). Default
  `false`.
- `smoke`: `{ "command": "<shell line>", "expects": { "exit_code":
  <int>, "stdout_contains": "<substring>" } }`. The `command` is
  legitimately verbatim shell — it IS the verify-phase unit of
  work, not a wrapper. Keep it fast (<60s); the full measurement
  command is too expensive for the smoke. `expects` encodes the
  grading rule structurally; the composer derives the legacy
  expected_smoke_signature string from it.

**RE-PROVISION plan** (registry STALE): same intent shape as
PROVISION, plus `plan_action="reprovision"` so execute knows to
`docker rm -f` the existing container first.

## Step 5 — Emit + decide

Call `emit_bootstrap_plan` with the full intent. The tool
canonicalizes, composes the recipe, stamps `sandbox.tenant.*`
triples on the run entity + your plan-loop entity, and updates
registry state to `provisioning` (for execute paths) or keeps it
at the current ready state (for skip).

Then `decide(action="execute" | "skip" | "needs_clarification",
reason="<one-line summary of plan path + key params>")`.

`reason` is the handoff to execute (or to reviewer on skip path);
keep the substance in it so downstream phases don't have to
re-read every triple.
