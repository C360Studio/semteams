# Change: Publish The SemTeams Image On Tags Only

## Why

The repository's publication workflows were 2025-11 copies of the SemStreams workflows, and none of them published a
correct SemTeams artifact:

- `release.yml` named SemStreams in its archive names, release heading, image pull line, documentation links and
  changelog URL. It has never run because the repository has no tags.
- `container.yml` listened for a completed workflow named `CI`. PR #262 renamed the merge workflow `Repository CI`, so
  the listener was severed and the main-branch image has not been built since. The `latest` image on GHCR predates
  ADR-061.
- Both build paths passed `-X main.version`, `-X main.commit` and `-X main.buildDate` ldflags, but
  `cmd/semteams/main.go` declared its build information as constants with different names, so version stamping has
  been inert since the copy.
- `docker/Dockerfile` labels, source URL, runtime user and binary path still say SemStreams.
- No tag, ruleset or protection exists, so any `v*` push would have published mislabeled artifacts.

Work authority is GitHub issue #259 (owner ruling 2026-10-09) and draft PR #307. This change touches repository
publication and build identity only; it does not change live product packs or activate any parked surface.

## What Changes

- Publish one artifact: the container image `ghcr.io/c360studio/semteams`. The copied `release.yml` and
  `container.yml` are deleted.
- Add one `Publish` workflow triggered only by `v*` tags. No floating `latest` image is built from `main`. A
  prerelease version (a hyphen suffix such as `v0.1.0-rc.1`) marks the GitHub release as prerelease.
- Before building, `scripts/publish-verify.sh` verifies that the tag is SemVer 2.0, that the tagged commit is an
  ancestor of `main`, that no `Repository CI` run for it is still in flight, and that its newest `CI Status Check`
  check-run concluded `success`. Missing, incomplete or unsuccessful evidence fails the run before any build. A
  fixture test in the Governance CI job proves the gate. No new E2E gate is added.
- Because GitHub runs the workflow file of the tagged commit, only commits at or after this change's merge may be
  tagged; the owner-enabled admin-only `v*` tag ruleset is the guard that enforces who can tag.
- `workflow_dispatch` is a dry run: it verifies the same evidence, builds without pushing and creates no release.
- The image identity becomes SemTeams: Dockerfile labels, source URL, runtime user and binary path. The build
  information becomes ldflags-settable variables, so a tagged image reports its tag from `--version`.
- The GitHub release is created through `gh api` with generated notes plus the image reference and digest. There is
  no third-party release action, no hand-maintained changelog template and no attached binary; a tag that already has
  a release is refused.
- The image build reads `docker/Dockerfile.dockerignore`, so `.git` and other non-build files stay out of the context.

## Non-goals

- Binary tarballs or a cross-platform binary matrix.
- A floating `latest` image tag, or any image publication from pushes to `main`.
- A required mock E2E publication or merge gate; that stays deferred as #254 left it.
- Activating the `v*` tag ruleset. The owner enables it in repository settings; this change does not.
- Renaming the `/etc/semstreams/config.json` path or the `SEMSTREAMS_*` environment prefix. They are a shared live
  contract between the compose files and the product shell, and renaming them is a separate ADR-029 contract change.
- Publishing the `semteams-sandbox:dev` image, which stays build-local.
- Pinning the `alpine:latest` runtime base image.
- Cutting `v0.1.0`, which remains gated on the rest of milestone v0.1.0.
