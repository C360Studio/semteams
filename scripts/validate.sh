#!/usr/bin/env bash
# validate.sh — authoritative read-only local validation mirroring Repository CI
# (.github/workflows/ci.yml: go, ui, and governance lanes, #310 / #344).
# Invoked by both `task validate` and `task verify`.
#
# Guarantees:
#   1. Fails explicitly before running lanes when any prerequisite tool or
#      daemon is missing (go, gofmt, task, revive, python3, node, npm, docker,
#      openspec, ateam, jq, diff, git, or ui/node_modules).
#   2. Writes Go build artifacts and generated schemas/OpenAPI specs only to a
#      temporary directory outside the repository, then compares generated
#      output against tracked schemas/ and specs/openapi.v3.yaml.
#   3. Preserves all tracked, staged, unstaged, and untracked worktree content
#      whether validation passes or fails.
#   4. Reports per-lane wall-clock timings using bash's builtin SECONDS.
#   5. Never invokes paid LLM calls, Playwright journeys, or mock E2E stacks.

set -euo pipefail

fail() {
  echo "::error::validate: $*" >&2
  exit 1
}

REPO_ROOT="${VALIDATE_REPO_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
cd "$REPO_ROOT"

HAD_UI_BUILD=0
if [ -e "$REPO_ROOT/ui/build" ]; then
  HAD_UI_BUILD=1
fi
HAD_UI_SVELTE_KIT=0
if [ -e "$REPO_ROOT/ui/.svelte-kit" ]; then
  HAD_UI_SVELTE_KIT=1
fi

TMP_DIR=""
cleanup() {
  if [ -n "${TMP_DIR:-}" ] && [ -d "$TMP_DIR" ]; then
    rm -rf -- "$TMP_DIR"
  fi
  if [ "$HAD_UI_BUILD" -eq 0 ] && [ -e "$REPO_ROOT/ui/build" ]; then
    rm -rf -- "$REPO_ROOT/ui/build"
  fi
  if [ "$HAD_UI_SVELTE_KIT" -eq 0 ] && [ -e "$REPO_ROOT/ui/.svelte-kit" ]; then
    rm -rf -- "$REPO_ROOT/ui/.svelte-kit"
  fi
}
trap cleanup EXIT

check_prereqs() {
  local missing=()
  local tool
  for tool in go gofmt task revive python3 node npm docker openspec ateam jq diff git; do
    if ! command -v "$tool" >/dev/null 2>&1; then
      missing+=("$tool")
    fi
  done
  if [ "${#missing[@]}" -gt 0 ]; then
    fail "missing required prerequisite(s): ${missing[*]}"
  fi
  if ! docker info >/dev/null 2>&1; then
    fail "prerequisite check failed: docker daemon is not reachable (docker info failed)"
  fi
  if [ ! -d "$REPO_ROOT/ui/node_modules" ]; then
    fail "missing required prerequisite: ui/node_modules (run 'npm --prefix ui ci' first)"
  fi
}

check_prereqs

TMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/semteams-validate.XXXXXX")
export PYTHONDONTWRITEBYTECODE=1
SECONDS=0

echo "=== [1/3] Go lane ==="
go_start=$SECONDS
task lint
task test:race
task test:browser-runner
task test:integration

mkdir -p "$TMP_DIR/bin"
go build -o "$TMP_DIR/bin/" ./...

mkdir -p "$TMP_DIR/schemas" "$TMP_DIR/specs"
go run ./cmd/openapi-generator -out "$TMP_DIR/schemas" -openapi "$TMP_DIR/specs/openapi.v3.yaml"
if ! diff -ru schemas "$TMP_DIR/schemas" || ! diff -u specs/openapi.v3.yaml "$TMP_DIR/specs/openapi.v3.yaml"; then
  fail "generated schemas/OpenAPI drifted from tracked schemas/ or specs/openapi.v3.yaml; run 'task schema:generate' and commit"
fi
go_elapsed=$((SECONDS - go_start))

echo "=== [2/3] UI lane ==="
ui_start=$SECONDS
(
  cd "$REPO_ROOT/ui"
  export OPENAPI_SPEC_PATH="../specs/openapi.v3.yaml"
  export SCHEMAS_DIR="../schemas"
  npm run lint
  npm run check
  npm run test:unit -- --run
  npm run generate-types:check
  npm run build
)
ui_elapsed=$((SECONDS - ui_start))

echo "=== [3/3] Governance lane ==="
gov_start=$SECONDS
task ateam:doctor
task openspec:validate
task openspec:queue-test
task publish:verify-test
gov_elapsed=$((SECONDS - gov_start))

echo
echo "lane timings:"
echo "  go          ${go_elapsed}s"
echo "  ui          ${ui_elapsed}s"
echo "  governance  ${gov_elapsed}s"
echo "✅ validate: all Repository CI lanes passed (Go, UI, Governance)"
