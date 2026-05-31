# Canonicalization rigor — cache correctness is on you

The registry signature is the **cache key** for tenant reuse. Two
users asking for the same target two different ways must hit the
same tenant (no waste). Two users asking for materially different
targets must hit distinct tenants (no silent collision /
measurement corruption).

The `emit_bootstrap_plan` tool's Go canonicalizer is the
deterministic hash machine; your job is to **fill in clean
canonical fields** so the canonicalizer can do its work.

## What you canonicalize

- **Command**: lowercase the whole string. Collapse whitespace to
  single spaces. Drop trailing punctuation. Examples:
  - `"Task Test:Integration"` → `task test:integration`
  - `"  pytest    -q"` → `pytest -q`
  - `"npm test;"` → `npm test`
- **Source URL**: canonical https form, no `.git`, no trailing
  slash, lowercase host:
  - `git@github.com:Owner/Repo.git` → `https://github.com/owner/repo`
  - `https://GitHub.com/Owner/Repo/` → `https://github.com/owner/repo`
  - `https://gitlab.example.com/team/proj` → unchanged (already
    canonical)
- **Source ref**: prefer commit SHA when resolvable. If the user
  named a tag (`v1.2.3`), keep it as a tag (the canonicalizer
  pins refs at signature computation time). If the user named a
  moving ref (`main`, `master`, `HEAD`), keep it — the
  canonicalizer treats it as a movable target and the registry's
  freshness check will catch upstream drift.
- **Toolchain versions**: full semantic version strings. `1.26`
  → `1.26.0`. `v1.26` → `1.26.0`. Lowercase keys (`go`, not
  `Go`).
- **Base image**: full `image:tag`. Never `:latest` (changes over
  time; breaks cache correctness). If the user said "ubuntu",
  resolve to `ubuntu:24.04` (current LTS). Same for language
  images.

## What you do NOT canonicalize (composer does it)

- **Shell commands** for the recipe (clone, install, mounts).
  You write structured intent (`source.kind`, `dependencies[]`
  with typed kinds, `mounts[]` with volume_suffix + path); the
  Go composer turns it into the actual `git clone …`, `apt-get
  install -y …`, `<volume>:<path>` strings. CLI grammar is not
  your responsibility — it is deterministic given the intent,
  so it lives in code.
- **Smoke grading shape**: write `smoke.expects.exit_code` and
  `smoke.expects.stdout_contains` as structured fields; the
  composer derives the legacy expected_smoke_signature string.

## Common mistakes that fragment the cache

- Variable command framing: `task t:i` vs `task test:integration`
  → two signatures for the same intent. Solution: canonicalize
  in step 2; don't accept abbreviations.
- Floating refs as immutable: user says `main` but the
  canonicalizer can't resolve it → signature includes literal
  `main`. Two users on different days = different signatures
  even if they fetched the same code. Acceptable but flag in
  scratchpad.
- Toolchain optionality: user didn't name a Go version. You
  default to "latest" → cache thrash. Solution: ask
  `needs_clarification` OR pin to a project default if the
  source identifies one (semteams → `1.26` from go.mod).

## When in doubt, ask

The user's framing is what you have to work with. If two
plausible canonicalizations give different signatures, that's a
sign the framing is ambiguous. Better to `needs_clarification`
once than to fragment the registry across two near-duplicate
tenants.
