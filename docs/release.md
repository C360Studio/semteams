# Releases

SemTeams publishes one artifact: the container image `ghcr.io/c360studio/semteams`. The only publication path is
the `Publish` workflow in `.github/workflows/publish.yml`, and it publishes only for `v*` tags (owner ruling on #259).

## What a tag does

Pushing a tag such as `v0.1.0` starts `Publish`. Its first job runs `scripts/publish-verify.sh`, which fails the run
before anything is built unless the tag is SemVer 2.0 (`vMAJOR.MINOR.PATCH`, optionally `-PRERELEASE`, no build
metadata) and the tagged commit:

- is an ancestor of `main`;
- has no `Repository CI` run still queued or in progress;
- has a newest `CI Status Check` check-run (highest id) that concluded `success`.

The second job builds `linux/amd64` and `linux/arm64` from `docker/Dockerfile` at the verified commit, pushes the
image tags, and creates the GitHub release. It refuses a tag that already has a release.

## Which commits may be tagged

GitHub runs the `Publish` file of the tagged commit, not the one on `main`. A tag on a commit older than the merge of
PR #307 runs the deleted SemStreams workflows from history with no gate, and a tag on a commit with an edited
`publish.yml` runs that edit. Tag only commits at or after that merge. The real guard is a repository ruleset that
restricts `v*` tag creation to admins; the owner enables it in repository settings before the first tag.

## What is published

| Git tag | Image tags | GitHub release |
| --- | --- | --- |
| `v0.1.0` | `0.1.0`, `0.1` | latest, unless a higher version exists |
| `v0.1.0-rc.1` | `0.1.0-rc.1` | prerelease, never latest |

The release body is GitHub's generated notes, preceded by the image reference, its digest and a `docker pull` line.
Not published: binaries or tarballs, a `latest` image tag, any image from pushes to `main`, and the
`semteams-sandbox:dev` image, which stays build-local.

## Version reporting

The workflow passes the tag, commit and build date as Docker build args, and the Dockerfile stamps them into the
binary with `-ldflags`. A tagged image's `--version` ends with
`semteams version v0.1.0 (commit <sha>, built <date>)`. A plain `go build` reports `dev`.

## Dry run

Once the workflow is on `main`, run `gh workflow run publish.yml --ref main` (or use the Actions tab) with `dry_run`
left set. It runs the same verification, then builds both platforms as `dry-run-<short sha>` without logging in,
pushing or creating a release. Clearing `dry_run` fails verification: manual publication is not a path; push a tag.

## Recovery

A tag pushed before `main`'s CI finishes fails verification with `Repository CI still running` or
`no CI Status Check found`. Wait for `Repository CI` to finish on that commit, then re-run the failed `Publish` run;
do not move or re-push the tag. Runs publish one at a time: a second tag waits for the first, and GitHub keeps only
one waiting run, so a third tag pushed meanwhile cancels the waiting one, which then needs a re-run.

## v0.1.0

`v0.1.0` is gated on milestone v0.1.0: the tag is cut only after the milestone's member issues are done.
