# Releases

SemTeams publishes one artifact: the container image `ghcr.io/c360studio/semteams`. The only publication path is
the `Publish` workflow in `.github/workflows/publish.yml`, and it publishes only for `v*` tags (owner ruling on #259).

## What a tag does

Pushing a tag such as `v0.1.0` starts `Publish`, which runs two jobs.

1. **Verify publication evidence.** The tag must read `vMAJOR.MINOR.PATCH`, optionally with a `-PRERELEASE`
   suffix. Two gates must then pass, or the run fails before anything is built:
   - the tagged commit is an ancestor of `main`;
   - the most recent `CI Status Check` check-run on that commit concluded `success`.
2. **Build and publish image.** Builds `linux/amd64` and `linux/arm64` from `docker/Dockerfile` at the verified
   commit, pushes the image tags, and creates the GitHub release.

## What is published

| Git tag | Image tags | GitHub release |
| --- | --- | --- |
| `v0.1.0` | `0.1.0`, `0.1` | marked latest |
| `v0.1.0-rc.1` | `0.1.0-rc.1` | marked prerelease, never latest |

The release body is GitHub's generated notes, preceded by the image reference, its digest and a `docker pull` line.

Not published: binaries or tarballs, a `latest` image tag, any image from pushes to `main`, and the
`semteams-sandbox:dev` image, which stays build-local.

## Version reporting

The workflow passes the tag, commit and build date as Docker build args, and the Dockerfile stamps them into the
binary with `-ldflags`. A tagged image's `--version` ends with
`semteams version v0.1.0 (commit <sha>, built <date>)`. A plain `go build` reports `dev`.

## Dry run

Once the workflow is on `main`, start `Publish` from the Actions tab or with
`gh workflow run publish.yml --ref main`, leaving `dry_run` set. It runs the same verification, then builds both
platforms as `dry-run-<short sha>` without logging in to GHCR, pushing, or creating a release. Clearing `dry_run`
fails verification: manual publication is not a path; push a tag.

## Tag protection

A repository ruleset restricting `v*` tag creation to admins is an owner setting in the repository's GitHub
settings, not part of this workflow. Until it is enabled, the two evidence gates are the only guard.

## v0.1.0

`v0.1.0` is gated on milestone v0.1.0: the tag is cut only after the milestone's member issues are done.
