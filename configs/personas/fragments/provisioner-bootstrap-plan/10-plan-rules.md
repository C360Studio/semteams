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

## Step 4 — Compose the plan

Three shapes, by path:

**SKIP plan** (registry FRESH): all plan fields carry the cached
values so they thread through verify. Set `plan.action=skip`.

**PROVISION plan** (registry MISS): write the full shape per
step 1's extracted parameters PLUS:

- `clone_command`: full `git clone --branch <ref-or-branch> <url> /workspace`
  (or `none` if no clone needed). `--branch` BEFORE the URL; git's
  positional grammar is `git clone [options] <url> [<dir>]` — putting
  the branch token after the URL makes git treat it as the directory
  arg and fail with "Too many arguments" when /workspace follows.
- `install_steps`: ordered list of single-line shell commands.
  Batch idempotent installs (`apt-get install -y A B C D` is one
  step, not 4). Order matters: apt first, then toolchain installers,
  then app deps (`go mod download`, `npm ci`, `pip install`).
- `volume_mounts`: `semteams-tenant-<sig>-workspace:/workspace` +
  `semteams-tenant-<sig>-deps:/root/.cache` for dep caches that
  speed up re-provisioning later.
- `verify_command`: a fast smoke (< 60s). NOT the full
  measurement command. Examples: `which go && go version`,
  `task lint:setup`, `node -e 'console.log(1)'`, `ls /workspace`.
- `expected_smoke_signature`: what the verify persona will match
  against. Exit code + a specific stdout substring or regex.
  Example: `{exit_code: 0, stdout_contains: "go version go1.26"}`.

**RE-PROVISION plan** (registry STALE): same shape as PROVISION,
plus `plan.action=reprovision` so execute knows to `docker rm -f`
the existing container first.

## Step 5 — Emit + decide

Call `emit_bootstrap_plan` with the full plan shape. The tool
stamps `sandbox.tenant.*` triples on the run entity + updates
registry state to `provisioning` (for execute paths) or keeps it
at the current ready state (for skip).

Then `decide(action="execute" | "skip" | "needs_clarification",
reason="<one-line summary of plan path + key params>")`.

`reason` is the handoff to execute (or to reviewer on skip path);
keep the substance in it so downstream phases don't have to
re-read every triple.
