#!/usr/bin/env bash
# openspec-queue_fixture_test.sh — verifies scripts/openspec-queue.sh
# reports the caveat shapes it claims to and suppresses historical/noisy
# shapes. Run from repo root.

set -uo pipefail

SCRIPT="$(pwd)/scripts/openspec-queue.sh"
[ -x "$SCRIPT" ] || { echo "FATAL: $SCRIPT not executable"; exit 2; }

PASS=0
FAIL=0

# run_fixture <tasks.md content> -> prints script output
run_fixture() {
  local content=$1
  local work
  work=$(mktemp -d)

  mkdir -p "$work/openspec/changes/fixture-change"
  printf '%s\n' "$content" > "$work/openspec/changes/fixture-change/tasks.md"

  # Stub the CLI so the fixture drives the report, not the real repo.
  mkdir -p "$work/bin"
  cat > "$work/bin/openspec" <<'STUB'
#!/usr/bin/env bash
if [ "${1:-}" = "list" ]; then
  cat <<'JSON'
{"changes":[{"name":"fixture-change","completedTasks":1,"totalTasks":3,"lastModified":"2026-08-01T00:00:00.000Z","status":"in-progress"}]}
JSON
fi
STUB
  chmod +x "$work/bin/openspec"

  ( cd "$work" && PATH="$work/bin:$PATH" bash "$SCRIPT" 2>&1 )
  rm -rf "$work"
}

check() {
  local description=$1
  local expect=$2
  local needle=$3
  local content=$4

  local out
  out=$(run_fixture "$content")

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
printf 'passed=%d failed=%d\n' "$PASS" "$FAIL"
[ "$FAIL" -eq 0 ] || exit 1
