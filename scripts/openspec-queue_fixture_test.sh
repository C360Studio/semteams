#!/usr/bin/env bash
# openspec-queue_fixture_test.sh — verifies scripts/openspec-queue.sh
# reports the caveat shapes it claims to and suppresses historical/noisy
# shapes. Run from repo root.

set -uo pipefail

SCRIPT="$(pwd)/scripts/openspec-queue.sh"
[ -x "$SCRIPT" ] || { echo "FATAL: $SCRIPT not executable"; exit 2; }

PASS=0
FAIL=0
FIXTURE_ROOT=$(mktemp -d)

cleanup() {
  if [ -n "${FIXTURE_ROOT:-}" ] && [ -d "$FIXTURE_ROOT" ]; then
    rm -rf -- "$FIXTURE_ROOT"
  fi
}
trap cleanup EXIT

# run_fixture <tasks.md content|MISSING> <CLI mode> [script args...]
# Prints script output and returns its exact status. A subprocess timeout keeps
# a malformed argument parser from hanging the fixture suite indefinitely.
run_fixture() {
  local content=$1
  local cli_mode=${2:-valid}
  shift 2
  local work
  work=$(mktemp -d "$FIXTURE_ROOT/case.XXXXXX")

  mkdir -p "$work/openspec/changes/fixture-change"
  if [ "$content" != "MISSING" ]; then
    printf '%s\n' "$content" > "$work/openspec/changes/fixture-change/tasks.md"
  fi

  # Stub the CLI so the fixture drives the report, not the real repo.
  mkdir -p "$work/bin"
  cat > "$work/bin/openspec" <<'STUB'
#!/usr/bin/env bash
if [ "${1:-}" = "list" ]; then
  case "${OPENSPEC_FIXTURE_MODE:-valid}" in
    valid) cat <<'JSON'
{"changes":[{"name":"fixture-change","completedTasks":1,"totalTasks":3,"lastModified":"2026-08-01T00:00:00.000Z","status":"in-progress"}]}
JSON
      ;;
    cli-error)
      echo "fixture openspec failure" >&2
      exit 7
      ;;
    malformed-json)
      printf '{not-json}\n'
      ;;
    missing-changes)
      printf '{}\n'
      ;;
    nonlist-changes)
      printf '{"changes":{}}\n'
      ;;
    nonobject-root)
      printf '[]\n'
      ;;
    empty-list)
      printf '{"changes":[]}\n'
      ;;
    empty-name)
      printf '{"changes":[{"name":"","completedTasks":0,"totalTasks":1}]}\n'
      ;;
    unsafe-name)
      printf '{"changes":[{"name":"../escape","completedTasks":0,"totalTasks":1}]}\n'
      ;;
    missing-task-count)
      printf '{"changes":[{"name":"fixture-change","totalTasks":1}]}\n'
      ;;
    invalid-task-count)
      printf '{"changes":[{"name":"fixture-change","completedTasks":"0","totalTasks":1}]}\n'
      ;;
  esac
fi
STUB
  chmod +x "$work/bin/openspec"

  (
    cd "$work" || exit 99
    OPENSPEC_FIXTURE_MODE="$cli_mode" PATH="$work/bin:$PATH" \
      python3 - "$SCRIPT" "$@" <<'PY'
import subprocess
import sys

try:
    completed = subprocess.run(
        ["bash", *sys.argv[1:]],
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        timeout=10,
        check=False,
    )
except subprocess.TimeoutExpired as error:
    if error.stdout:
        sys.stdout.buffer.write(error.stdout)
    print("fixture command timed out")
    sys.exit(124)

sys.stdout.buffer.write(completed.stdout)
sys.exit(completed.returncode)
PY
  )
}

check() {
  local description=$1
  local expect=$2
  local needle=$3
  local content=$4

  local out
  out=$(run_fixture "$content" valid)

  local got="noreport"
  printf '%s' "$out" | grep -q "$needle" && got="report"

  if [ "$got" = "$expect" ]; then
    printf 'PASS  %s\n' "$description"
    PASS=$((PASS + 1))
  else
    printf 'FAIL  %s (expected %s, got %s)\n' "$description" "$expect" "$got"
    printf '      --- output ---\n%s\n      --------------\n' "$out"
    FAIL=$((FAIL + 1))
  fi
}

check_status() {
  local description=$1
  local expected=$2
  local content=$3
  local cli_mode=$4
  shift 4

  local out
  local got
  out=$(run_fixture "$content" "$cli_mode" "$@")
  got=$?

  if [ "$got" -eq "$expected" ]; then
    printf 'PASS  %s (exit %d)\n' "$description" "$got"
    PASS=$((PASS + 1))
  else
    printf 'FAIL  %s (expected exit %d, got %d)\n' "$description" "$expected" "$got"
    printf '      --- output ---\n%s\n      --------------\n' "$out"
    FAIL=$((FAIL + 1))
  fi
}

check_status_output() {
  local description=$1
  local expected=$2
  local needle=$3
  local content=$4
  local cli_mode=$5
  shift 5

  local out
  local got
  out=$(run_fixture "$content" "$cli_mode" "$@")
  got=$?

  if [ "$got" -eq "$expected" ] && printf '%s' "$out" | grep -q "$needle"; then
    printf 'PASS  %s (exit %d, diagnostic present)\n' "$description" "$got"
    PASS=$((PASS + 1))
  else
    printf 'FAIL  %s (expected exit %d and diagnostic %q, got %d)\n' \
      "$description" "$expected" "$needle" "$got"
    printf '      --- output ---\n%s\n      --------------\n' "$out"
    FAIL=$((FAIL + 1))
  fi
}

echo "=== positive cases: caveats that MUST surface ==="

check "lowercase 'halt:' mid-sentence in an open task" report "HALT" \
'- [ ] 4.3 If the pre-v1 wipe window closed before 3.1, halt: record the missed window.'

check "[~] partial marker regardless of wording" report "WONTDO" \
'- [~] 4.2 Verifying Workflow.Name equals the Schema own Workflow().'

check "explicit RED gate" report "RED" \
'- [ ] 4.2 RED — semantic e2e is failing, see gh#830.'

check "HOLD wording" report "BLOCKED" \
'- [ ] 8.3 On HOLD pending the owner ruling.'

check "STILL OPEN wording" report "OPEN-Q" \
'- [ ] 8.3 **STILL OPEN** — decide whether it rides the sister replay.'

check "deliberate not-done wording" report "WONTDO" \
'- [ ] 4.2 Not enforced, deliberately, because it converts a posture into a boot failure.'

echo
echo "=== negative cases: shapes that MUST NOT surface ==="

check "COMPLETED task mentioning halt is history, not a live condition" noreport "HALT" \
'- [x] 4.3 The wipe window halt condition was evaluated and did not fire.'

check "ordinary open task with no caveat" noreport "HALT" \
'- [ ] 2.1 Run one comparative benchmark and record it as ADR evidence.'

echo
echo "=== structural case: a clean change must SAY so, not be silent ==="

check "clean change emits an explicit no-marker line" report "no halt/hold/deliberate marker" \
'- [ ] 2.1 Run one comparative benchmark and record it as ADR evidence.
- [x] 2.2 Enumerate the consumers.'

echo
echo "=== exit cases: reporter failures and strict mode MUST be trustworthy ==="

check_status "openspec CLI nonzero fails closed" 2 \
  '- [ ] 2.1 Clean task.' cli-error

check_status "malformed openspec JSON fails closed" 2 \
  '- [ ] 2.1 Clean task.' malformed-json

check_status_output "JSON object missing changes fails closed" 2 "invalid openspec JSON" \
  '- [ ] 2.1 Clean task.' missing-changes

check_status_output "non-list changes field fails closed" 2 "invalid openspec JSON" \
  '- [ ] 2.1 Clean task.' nonlist-changes

check_status_output "non-object JSON root fails closed" 2 "invalid openspec JSON" \
  '- [ ] 2.1 Clean task.' nonobject-root

check_status "an explicit empty changes list is a valid empty queue" 0 \
  '- [ ] 2.1 Clean task.' empty-list

check_status_output "empty change name fails closed" 2 "invalid openspec JSON" \
  '- [ ] 2.1 Clean task.' empty-name

check_status_output "unsafe change name fails closed" 2 "invalid openspec JSON" \
  '- [ ] 2.1 Clean task.' unsafe-name

check_status_output "missing task-count field fails closed" 2 "invalid openspec JSON" \
  '- [ ] 2.1 Clean task.' missing-task-count

check_status_output "non-integer task-count field fails closed" 2 "invalid openspec JSON" \
  '- [ ] 2.1 Clean task.' invalid-task-count

check_status "listed change without tasks.md fails closed" 2 \
  MISSING valid

check_status "strict mode rejects a live caveat" 1 \
  '- [ ] 4.3 HALT pending an owner ruling.' valid --strict

check_status "strict mode accepts a clean queue" 0 \
  '- [ ] 2.1 Clean task.' valid --strict

echo
echo "=== argument cases: --stale-days MUST be a positive integer ==="

check_status "missing --stale-days value is rejected" 2 \
  '- [ ] 2.1 Clean task.' valid --stale-days

check_status "nonnumeric --stale-days value is rejected" 2 \
  '- [ ] 2.1 Clean task.' valid --stale-days banana

check_status "negative --stale-days value is rejected" 2 \
  '- [ ] 2.1 Clean task.' valid --stale-days -1

check_status "oversized --stale-days value is rejected" 2 \
  '- [ ] 2.1 Clean task.' valid --stale-days 99999999999999999999999

echo
printf 'passed=%d failed=%d\n' "$PASS" "$FAIL"
[ "$FAIL" -eq 0 ] || exit 1
