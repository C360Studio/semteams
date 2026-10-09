#!/usr/bin/env bash
# publish-verify.sh — verify publication evidence for the Publish workflow
# (.github/workflows/publish.yml, #259). Fails closed: any missing,
# unsuccessful or unreadable evidence exits non-zero before a build starts.
#
# Inputs (environment): GITHUB_REPOSITORY, GITHUB_SHA, GITHUB_EVENT_NAME,
# GITHUB_REF_TYPE, GITHUB_REF_NAME, DRY_RUN_INPUT, GITHUB_OUTPUT.
# Uses `git` (run inside the checkout, with an `origin` remote) and `gh`
# (authenticated through GH_TOKEN) from PATH.
#
# Outputs (appended to GITHUB_OUTPUT): version, sha, dry_run, prerelease.
#
# Rules:
#   1. A tag push must be a SemVer 2.0 tag vMAJOR.MINOR.PATCH[-PRERELEASE]
#      (no build metadata); a hyphen marks a prerelease. A workflow_dispatch
#      run is only a dry run: dry_run=false fails, dry_run=true yields
#      version dry-run-<sha7>.
#   2. The peeled commit must be an ancestor of origin/main.
#   3. Every Repository CI (ci.yml) run for the commit must be completed, so
#      a rerun in flight cannot be outvoted by an older success. Its
#      CI Status Check check-run does not exist until its other jobs finish.
#   4. Among GitHub Actions check-runs named "CI Status Check" on the commit,
#      the one with the highest id (the most recently created) decides, and
#      it must have concluded success.

set -euo pipefail

fail() {
  echo "::error::$*"
  exit 1
}

: "${GITHUB_REPOSITORY:?}" "${GITHUB_SHA:?}" "${GITHUB_EVENT_NAME:?}" "${GITHUB_OUTPUT:?}"
GITHUB_REF_TYPE=${GITHUB_REF_TYPE:-}
GITHUB_REF_NAME=${GITHUB_REF_NAME:-}
DRY_RUN_INPUT=${DRY_RUN_INPUT:-}

# SemVer 2.0 core plus optional prerelease, without build metadata.
num='(0|[1-9][0-9]*)'
ident='(0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)'
semver_tag="^v${num}\\.${num}\\.${num}(-${ident}(\\.${ident})*)?\$"

# 1. Resolve the commit, version and mode. Peel an annotated tag so every
#    later check and the build use the one commit it names.
sha=$(git rev-parse --verify --quiet "${GITHUB_SHA}^{commit}") ||
  fail "cannot resolve ${GITHUB_SHA} to a commit"
prerelease=false
case "$GITHUB_EVENT_NAME" in
  push)
    [[ "$GITHUB_REF_TYPE" == "tag" ]] ||
      fail "publication runs only for tag pushes, got ref type '${GITHUB_REF_TYPE}'"
    version=$GITHUB_REF_NAME
    [[ "$version" =~ $semver_tag ]] ||
      fail "publication requires a SemVer tag vMAJOR.MINOR.PATCH[-PRERELEASE], got '${version}'"
    dry_run=false
    if [[ "$version" == *-* ]]; then
      prerelease=true
    fi
    ;;
  workflow_dispatch)
    [[ "$DRY_RUN_INPUT" == "true" ]] ||
      fail "manual publication is not a path; push a tag"
    version="dry-run-${sha:0:7}"
    dry_run=true
    ;;
  *)
    fail "unsupported event ${GITHUB_EVENT_NAME}"
    ;;
esac

# 2. The commit must already be on main.
git fetch --no-tags --quiet origin +refs/heads/main:refs/remotes/origin/main ||
  fail "cannot fetch main"
git merge-base --is-ancestor "$sha" refs/remotes/origin/main ||
  fail "${sha} is not an ancestor of main; only commits already on main publish"

# 3. No Repository CI run for the commit may still be in flight.
statuses=$(gh api --paginate \
  "repos/${GITHUB_REPOSITORY}/actions/workflows/ci.yml/runs?head_sha=${sha}&per_page=100" \
  --jq '.workflow_runs[] | .status') ||
  fail "cannot list Repository CI runs for ${sha}"
[[ -n "$statuses" ]] || fail "no Repository CI run found for ${sha}"
if grep -qvx 'completed' <<<"$statuses"; then
  fail "Repository CI still running for ${sha}; wait for it to finish, then re-run Publish"
fi

# 4. The newest CI Status Check check-run (highest id) must have succeeded.
runs=$(gh api --paginate \
  "repos/${GITHUB_REPOSITORY}/commits/${sha}/check-runs?check_name=CI%20Status%20Check&filter=all&per_page=100" \
  --jq '.check_runs[]
    | select(.name == "CI Status Check" and .app.slug == "github-actions")
    | [.id, (.conclusion // .status)]
    | @tsv') ||
  fail "cannot list check-runs for ${sha}"
[[ -n "$runs" ]] || fail "no CI Status Check found for ${sha}"
newest=$(printf '%s\n' "$runs" | sort -n -k1,1 | tail -n 1)
result=${newest##*$'\t'}
[[ "$result" == "success" ]] ||
  fail "newest CI Status Check for ${sha} is ${result}, not success"

echo "verified ${version} at ${sha}: on main, Repository CI complete, CI Status Check success"
{
  echo "version=${version}"
  echo "sha=${sha}"
  echo "dry_run=${dry_run}"
  echo "prerelease=${prerelease}"
} >> "$GITHUB_OUTPUT"
