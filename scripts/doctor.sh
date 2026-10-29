#!/usr/bin/env bash
# Report tool versions and compare each against its single pin. Starts no workloads.
set -uo pipefail
cd "$(dirname "$0")/.."

fail=0
ok()   { printf '  ok    %-15s %s\n' "$1" "$2"; }
bad()  { printf '  FAIL  %-15s %s\n' "$1" "$2"; fail=1; }
warn() { printf '  warn  %-15s %s\n' "$1" "$2"; }

# version_ge A B: true when A >= B (dotted numeric versions).
version_ge() { [ "$(printf '%s\n%s\n' "$2" "$1" | sort -V | head -n1)" = "$2" ]; }

echo "SemTeams doctor"

# go: go.mod's go directive is the pin; the toolchain must be at least that.
if command -v go >/dev/null; then
  want=$(awk '$1=="go"{print $2; exit}' go.mod)
  have=$(go env GOVERSION | sed 's/^go//')
  if version_ge "$have" "$want"; then ok go "$have (go.mod: $want)"; else bad go "$have < go.mod $want"; fi
else bad go "not found"; fi

# task: .task-version is the pin (CI installs it from the same pin).
if command -v task >/dev/null; then
  want=$(tr -d '[:space:]' < .task-version)
  have=$(task --version | grep -Eo '[0-9]+\.[0-9]+\.[0-9]+' | head -n1)
  if [ "$have" = "$want" ]; then ok task "$have"; else bad task "$have != .task-version $want"; fi
else bad task "not found"; fi

# node: .nvmrc is the pin, compared by major version.
if command -v node >/dev/null; then
  want=$(tr -d '[:space:]v' < .nvmrc)
  have=$(node --version | sed 's/^v//')
  if [ "${have%%.*}" = "${want%%.*}" ]; then ok node "$have (.nvmrc: $want)"; else bad node "$have != .nvmrc $want"; fi
else bad node "not found"; fi

if command -v npm >/dev/null; then ok npm "$(npm --version)"; else bad npm "not found"; fi

# python3: scripts/qualify-migration-browser.py and its unittest (task test:browser-runner) use it.
if command -v python3 >/dev/null; then ok python3 "$(python3 --version 2>&1 | sed 's/^Python //')"
else bad python3 "not found (task test:browser-runner and task validate need it)"; fi

# jq: scripts/publish-verify.sh and scripts/publish-verify_fixture_test.sh use it.
if command -v jq >/dev/null; then ok jq "$(jq --version 2>&1)"
else bad jq "not found (task publish:verify-test and task validate need it)"; fi

# revive: pinned in .github/workflows/ci.yml (v1.15.0).
if command -v revive >/dev/null; then
  want_revive="1.15.0"
  have_revive=$(revive --version 2>&1 | grep -Eo '[0-9]+\.[0-9]+\.[0-9]+' | head -n1 || true)
  if [ "$have_revive" = "$want_revive" ]; then ok revive "$have_revive"; else bad revive "${have_revive:-unknown} != $want_revive"; fi
else bad revive "not found (go install github.com/mgechev/revive@v1.15.0)"; fi

# openspec: pinned in .github/workflows/ci.yml (1.7.0).
[ -d "$PWD/node_modules/.bin" ] && PATH="$PWD/node_modules/.bin:$PATH"
if command -v openspec >/dev/null; then
  want_openspec="1.7.0"
  have_openspec=$(openspec --version 2>&1 | grep -Eo '[0-9]+\.[0-9]+\.[0-9]+' | head -n1 || true)
  if [ "$have_openspec" = "$want_openspec" ]; then ok openspec "$have_openspec"; else bad openspec "${have_openspec:-unknown} != $want_openspec"; fi
else bad openspec "not found (npm install --global @fission-ai/openspec@1.7.0)"; fi

# ateam: pinned in .github/workflows/ci.yml (v0.3.4).
if command -v ateam >/dev/null; then
  want_ateam="v0.3.4"
  have_ateam=$(ateam version 2>&1 | grep -Eo 'v[0-9]+\.[0-9]+\.[0-9]+' | head -n1 || true)
  if [ "$have_ateam" = "$want_ateam" ]; then ok ateam "$have_ateam"; else bad ateam "${have_ateam:-unknown} != $want_ateam"; fi
else bad ateam "not found (GOTOOLCHAIN=auto go install github.com/c360studio/ateam/cmd/ateam@v0.3.4)"; fi

# ui/node_modules: required for UI checks and task validate.
if [ -d "ui/node_modules" ]; then
  ok ui/node_modules "present"
else
  warn ui/node_modules "missing (run 'npm --prefix ui ci' before UI checks or task validate)"
fi

# docker: version and daemon reachability only; no containers are started.
daemon=false
if command -v docker >/dev/null; then
  ok docker "client $(docker --version | sed 's/^Docker version //; s/,.*//')"
  if docker info >/dev/null 2>&1; then ok docker-daemon "reachable"; daemon=true
  else warn docker-daemon "not reachable (task test:integration and task validate need it)"; fi
else bad docker "not found"; fi

if $daemon; then
  context=$(docker context show 2>/dev/null || echo unknown)
  endpoint=$(docker context inspect --format '{{.Endpoints.docker.Host}}' 2>/dev/null || echo unknown)
  ok docker-context "$context ($endpoint)"
  if [ -n "${DOCKER_HOST:-}" ]; then warn docker-host "DOCKER_HOST=$DOCKER_HOST overrides the context"; fi
fi

if [ "$fail" -ne 0 ]; then echo "doctor: FAILED"; exit 1; fi
echo "doctor: ok"
