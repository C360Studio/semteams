#!/usr/bin/env bash
# publish-verify_fixture_test.sh — verifies scripts/publish-verify.sh fails
# closed on every evidence shape it claims to reject and passes only verified
# tags and dry runs. A fake `gh` serves JSON fixtures; a temporary git repo
# with a bare origin drives the main-ancestry check. Run from repo root.

set -uo pipefail

SCRIPT="$(pwd)/scripts/publish-verify.sh"
[ -x "$SCRIPT" ] || { echo "FATAL: $SCRIPT not executable"; exit 2; }
command -v jq >/dev/null 2>&1 || { echo "FATAL: jq not found on PATH"; exit 2; }

PASS=0
FAIL=0
ROOT=$(mktemp -d)

cleanup() {
  if [ -n "${ROOT:-}" ] && [ -d "$ROOT" ]; then
    rm -rf -- "$ROOT"
  fi
}
trap cleanup EXIT

# --- fake gh -----------------------------------------------------------------
# Emulates `gh api [--paginate] URL --jq FILTER` by running jq over the fixture
# chosen by URL. FAKE_GH_FAIL=<workflow-runs|check-runs> simulates an API error;
# FAKE_EXPECT_SHA proves the script queried the peeled commit.
mkdir -p "$ROOT/bin" "$ROOT/fx"
cat > "$ROOT/bin/gh" <<'STUB'
#!/usr/bin/env bash
url="" filter=""
[ "${1:-}" = "api" ] || { echo "fake gh: unsupported command $*" >&2; exit 2; }
shift
while [ $# -gt 0 ]; do
  case "$1" in
    --jq) filter=$2; shift 2 ;;
    --paginate) shift ;;
    -*) echo "fake gh: unexpected flag $1" >&2; exit 2 ;;
    *) url=$1; shift ;;
  esac
done
if [ -n "${FAKE_EXPECT_SHA:-}" ] && [[ "$url" != *"$FAKE_EXPECT_SHA"* ]]; then
  echo "fake gh: $url does not name $FAKE_EXPECT_SHA" >&2
  exit 3
fi
case "$url" in
  */actions/workflows/ci.yml/runs*)
    [[ "$url" == *"?head_sha="*"&per_page=100" ]] ||
      { echo "fake gh: workflow-runs URL must filter by head_sha and page by 100: $url" >&2; exit 2; }
    kind=workflow-runs fixture=$FAKE_WORKFLOW_RUNS ;;
  */check-runs*)
    [[ "$url" == *"?check_name=CI%20Status%20Check&filter=all&per_page=100" ]] ||
      { echo "fake gh: check-runs URL must name the check, use filter=all and page by 100: $url" >&2; exit 2; }
    kind=check-runs fixture=$FAKE_CHECK_RUNS ;;
  *) echo "fake gh: unexpected URL $url" >&2; exit 2 ;;
esac
if [ "${FAKE_GH_FAIL:-}" = "$kind" ]; then
  echo "gh: HTTP 403: Resource not accessible by integration" >&2
  exit 1
fi
jq -r "$filter" "$fixture"
STUB
chmod +x "$ROOT/bin/gh"

fx() { printf '%s\n' "$2" > "$ROOT/fx/$1.json"; }
cr() { # cr <id> <status> <conclusion|null> [app]
  local conclusion=null
  [ "$3" = "null" ] || conclusion="\"$3\""
  printf '{"id":%s,"name":"CI Status Check","status":"%s","conclusion":%s,"started_at":"2026-08-20T18:27:44Z","app":{"slug":"%s"}}' \
    "$1" "$2" "$conclusion" "${4:-github-actions}"
}

fx runs_completed '{"workflow_runs":[{"status":"completed"},{"status":"completed"}]}'
fx runs_in_progress '{"workflow_runs":[{"status":"in_progress"}]}'
fx runs_queued '{"workflow_runs":[{"status":"queued"}]}'
fx runs_mixed '{"workflow_runs":[{"status":"completed"},{"status":"in_progress"}]}'
fx runs_empty '{"workflow_runs":[]}'

fx checks_empty '{"check_runs":[]}'
fx checks_success "{\"check_runs\":[$(cr 101 completed success)]}"
fx checks_cancelled "{\"check_runs\":[$(cr 401 completed cancelled)]}"
fx checks_other_app "{\"check_runs\":[$(cr 501 completed success some-other-app)]}"
# Same started_at second; the failure is newer (higher id) but listed first.
fx checks_tie "{\"check_runs\":[$(cr 202 completed failure),$(cr 201 completed success)]}"
# A queued rerun with null started_at and conclusion is the newest.
fx checks_null_started \
  "{\"check_runs\":[$(cr 301 completed success),{\"id\":302,\"name\":\"CI Status Check\",\"status\":\"queued\",\"conclusion\":null,\"started_at\":null,\"app\":{\"slug\":\"github-actions\"}}]}"
fx checks_success_then_in_progress "{\"check_runs\":[$(cr 701 completed success),$(cr 702 in_progress null)]}"
fx checks_success_then_failure "{\"check_runs\":[$(cr 601 completed success),$(cr 602 completed failure)]}"
fx checks_failure_then_success "{\"check_runs\":[$(cr 801 completed failure),$(cr 802 completed success)]}"
# Numeric, not lexical, id order: 10 is newer than 9.
fx checks_numeric_success_newest "{\"check_runs\":[$(cr 9 completed failure),$(cr 10 completed success)]}"
fx checks_numeric_failure_newest "{\"check_runs\":[$(cr 9 completed success),$(cr 10 completed failure)]}"

# --- git fixture -------------------------------------------------------------
# origin.git/main holds MAIN_SHA; SIDE_SHA exists only on a local branch.
G() { git -c user.name=fixture -c user.email=fixture@example.invalid -c init.defaultBranch=main \
  -c commit.gpgsign=false -c tag.gpgsign=false "$@"; }
G init --quiet --bare "$ROOT/origin.git"
G clone --quiet "$ROOT/origin.git" "$ROOT/work" 2>/dev/null
WORK="$ROOT/work"
G -C "$WORK" commit --quiet --allow-empty -m "main commit"
G -C "$WORK" push --quiet origin HEAD:refs/heads/main
MAIN_SHA=$(git -C "$WORK" rev-parse HEAD)
G -C "$WORK" tag -a v0.1.0 -m "annotated tag"
TAG_OBJECT=$(git -C "$WORK" rev-parse v0.1.0)
G -C "$WORK" checkout --quiet -b side
G -C "$WORK" commit --quiet --allow-empty -m "side commit"
SIDE_SHA=$(git -C "$WORK" rev-parse HEAD)

OUT="$ROOT/github_output"

# run_verify [KEY=VALUE...] — defaults describe a valid v0.1.0 tag on main with
# completed CI and a successful CI Status Check; arguments override them.
run_verify() {
  : > "$OUT"
  (
    cd "$WORK" && env \
      PATH="$ROOT/bin:$PATH" \
      GITHUB_REPOSITORY=c360studio/semteams \
      GITHUB_SHA="$MAIN_SHA" \
      GITHUB_EVENT_NAME=push \
      GITHUB_REF_TYPE=tag \
      GITHUB_REF_NAME=v0.1.0 \
      DRY_RUN_INPUT= \
      GITHUB_OUTPUT="$OUT" \
      FAKE_WORKFLOW_RUNS="$ROOT/fx/runs_completed.json" \
      FAKE_CHECK_RUNS="$ROOT/fx/checks_success.json" \
      FAKE_GH_FAIL= \
      FAKE_EXPECT_SHA="$MAIN_SHA" \
      "$@" bash "$SCRIPT"
  )
}

# expect_fail <name> <message substring> [KEY=VALUE...]
# A failure must exit 1, name its reason, and write no outputs.
expect_fail() {
  local name=$1 want=$2 log status
  shift 2
  log=$(run_verify "$@" 2>&1)
  status=$?
  if [ "$status" -eq 1 ] && [[ "$log" == *"$want"* ]] && [ ! -s "$OUT" ]; then
    PASS=$((PASS + 1))
    printf '  ok    %s\n' "$name"
  else
    FAIL=$((FAIL + 1))
    printf '  FAIL  %s (status=%s, outputs=%s bytes)\n%s\n' "$name" "$status" "$(wc -c < "$OUT" | tr -d ' ')" "$log"
  fi
}

# expect_pass <name> <expected GITHUB_OUTPUT content> [KEY=VALUE...]
expect_pass() {
  local name=$1 want=$2 log status got
  shift 2
  log=$(run_verify "$@" 2>&1)
  status=$?
  got=$(cat "$OUT")
  if [ "$status" -eq 0 ] && [ "$got" = "$want" ]; then
    PASS=$((PASS + 1))
    printf '  ok    %s\n' "$name"
  else
    FAIL=$((FAIL + 1))
    printf '  FAIL  %s (status=%s)\n%s\n--- want outputs\n%s\n--- got outputs\n%s\n' \
      "$name" "$status" "$log" "$want" "$got"
  fi
}

tag_outputs() { # tag_outputs <version> <prerelease>
  printf 'version=%s\nsha=%s\ndry_run=false\nprerelease=%s' "$1" "$MAIN_SHA" "$2"
}

echo "=== CI evidence MUST fail closed ==="
expect_fail "first run: CI running, no CI Status Check yet" "Repository CI still running" \
  FAKE_WORKFLOW_RUNS="$ROOT/fx/runs_in_progress.json" FAKE_CHECK_RUNS="$ROOT/fx/checks_empty.json"
expect_fail "rerun in progress does not reuse an older success" "Repository CI still running" \
  FAKE_WORKFLOW_RUNS="$ROOT/fx/runs_in_progress.json"
expect_fail "one incomplete Repository CI run among several" "Repository CI still running" \
  FAKE_WORKFLOW_RUNS="$ROOT/fx/runs_mixed.json"
expect_fail "queued Repository CI run" "Repository CI still running" \
  FAKE_WORKFLOW_RUNS="$ROOT/fx/runs_queued.json"
expect_fail "no Repository CI run for the commit" "no Repository CI run found" \
  FAKE_WORKFLOW_RUNS="$ROOT/fx/runs_empty.json"
expect_fail "completed CI without a CI Status Check" "no CI Status Check found" \
  FAKE_CHECK_RUNS="$ROOT/fx/checks_empty.json"
expect_fail "same-second tie: newer failure beats older success" "is failure, not success" \
  FAKE_CHECK_RUNS="$ROOT/fx/checks_tie.json"
expect_fail "newest check-run queued with null started_at" "is queued, not success" \
  FAKE_CHECK_RUNS="$ROOT/fx/checks_null_started.json"
expect_fail "newest check-run still in progress" "is in_progress, not success" \
  FAKE_CHECK_RUNS="$ROOT/fx/checks_success_then_in_progress.json"
expect_fail "newer failure after success" "is failure, not success" \
  FAKE_CHECK_RUNS="$ROOT/fx/checks_success_then_failure.json"
expect_fail "numeric id order: newest (10) failed" "is failure, not success" \
  FAKE_CHECK_RUNS="$ROOT/fx/checks_numeric_failure_newest.json"
expect_fail "cancelled CI Status Check" "is cancelled, not success" \
  FAKE_CHECK_RUNS="$ROOT/fx/checks_cancelled.json"
expect_fail "CI Status Check from another app is ignored" "no CI Status Check found" \
  FAKE_CHECK_RUNS="$ROOT/fx/checks_other_app.json"
expect_fail "gh error listing Repository CI runs" "cannot list Repository CI runs" \
  FAKE_GH_FAIL=workflow-runs
expect_fail "gh error listing check-runs" "cannot list check-runs" \
  FAKE_GH_FAIL=check-runs

echo
echo "=== main ancestry and mode MUST fail closed ==="
expect_fail "commit off main" "is not an ancestor of main" \
  GITHUB_SHA="$SIDE_SHA" FAKE_EXPECT_SHA="$SIDE_SHA"
expect_fail "unresolvable commit" "cannot resolve" \
  GITHUB_SHA=0000000000000000000000000000000000000000
expect_fail "dispatch with dry_run=false" "manual publication is not a path; push a tag" \
  GITHUB_EVENT_NAME=workflow_dispatch GITHUB_REF_TYPE=branch GITHUB_REF_NAME=main DRY_RUN_INPUT=false
expect_fail "dispatch with empty dry_run" "manual publication is not a path; push a tag" \
  GITHUB_EVENT_NAME=workflow_dispatch GITHUB_REF_TYPE=branch GITHUB_REF_NAME=main
expect_fail "branch push" "only for tag pushes" \
  GITHUB_REF_TYPE=branch GITHUB_REF_NAME=main
expect_fail "unsupported event" "unsupported event pull_request" \
  GITHUB_EVENT_NAME=pull_request

echo
echo "=== tags MUST follow SemVer 2.0 without build metadata ==="
for bad in v01.2.3 v1.02.3 v1.2.03 v1.2.3-rc..1 v1.2.3-01 v1.2.3- v1.2.3+meta v1.2 1.2.3 'v1.2.3$(id)'; do
  expect_fail "tag $bad is rejected" "requires a SemVer tag" GITHUB_REF_NAME="$bad"
done

echo
echo "=== verified evidence passes ==="
expect_pass "v0.1.0 on main with complete, successful CI" "$(tag_outputs v0.1.0 false)"
expect_pass "v0.1.0-rc.1 is a prerelease" "$(tag_outputs v0.1.0-rc.1 true)" GITHUB_REF_NAME=v0.1.0-rc.1
expect_pass "v1.2.3-beta.67 is a prerelease" "$(tag_outputs v1.2.3-beta.67 true)" GITHUB_REF_NAME=v1.2.3-beta.67
# SemVer 2.0 allows a hyphen-only alphanumeric identifier, so 1.2.3-- is valid.
expect_pass "v1.2.3-- is a SemVer prerelease" "$(tag_outputs v1.2.3-- true)" GITHUB_REF_NAME=v1.2.3--
expect_pass "rerun success after an earlier failure" "$(tag_outputs v0.1.0 false)" \
  FAKE_CHECK_RUNS="$ROOT/fx/checks_failure_then_success.json"
expect_pass "numeric id order: newest (10) succeeded" "$(tag_outputs v0.1.0 false)" \
  FAKE_CHECK_RUNS="$ROOT/fx/checks_numeric_success_newest.json"
expect_pass "annotated tag object peels to its commit" "$(tag_outputs v0.1.0 false)" \
  GITHUB_SHA="$TAG_OBJECT"
expect_pass "dispatch dry run" \
  "$(printf 'version=dry-run-%s\nsha=%s\ndry_run=true\nprerelease=false' "${MAIN_SHA:0:7}" "$MAIN_SHA")" \
  GITHUB_EVENT_NAME=workflow_dispatch GITHUB_REF_TYPE=branch GITHUB_REF_NAME=main DRY_RUN_INPUT=true

echo
printf 'passed=%d failed=%d\n' "$PASS" "$FAIL"
[ "$FAIL" -eq 0 ] || exit 1
