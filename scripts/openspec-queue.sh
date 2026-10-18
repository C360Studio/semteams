#!/usr/bin/env bash
# openspec-queue.sh — surface WHY each in-flight change is still open.
#
# `openspec list` renders a change as a bare progress fraction ("12/14
# tasks"). That fraction actively misleads when the remaining work is not
# work at all but a HALT condition, a deliberate not-done, or a red gate:
# it reads "almost done, just finish it" when the honest reading is
# "decide whether this change should still exist".
#
# This script reads caveats from the source every run, so a program baton
# or handoff note does not have to carry them and cannot drift from them.
#
# A task is read whole: its checkbox line and every line wrapped or nested
# under it, up to the next task or the next line indented no deeper than
# the task. Labels come from the checkbox line alone. A `Hold:` (matched
# with its case) anywhere in an open task's block gets one BLOCKED line,
# numbered where the hold is written, with the task number and the text
# from `Hold:` on; the checkbox line is not shown as BLOCKED a second time.
#
# Exit status is advisory-by-default and deliberately so: this is a
# reporting aid for humans and session startup, not a merge gate. Use
# --strict to exit non-zero when any caveat is found (for CI or a
# pre-archive hook). Exit 2 means the queue could not be read.
#
# --check prints no queue; `task spec:check` / `task openspec:validate` runs it.
# A `Hold:` below a tasks.md's first "## " heading that lies in no task's block,
# ticked tasks included, is one the queue cannot show: each is printed as
# <path>:<line>: and the exit is 1. With none it prints "holds: ok (<n> tasks.md read)"
# and exits 0. An unreadable change list or tasks.md exits 2, as for the queue.
#
# Run from repo root:
#   scripts/openspec-queue.sh [--strict] [--stale-days N]
#   scripts/openspec-queue.sh --check

set -uo pipefail

STRICT=0
CHECK=0
STALE_DAYS=7
MAX_STALE_DAYS=36500

while [ $# -gt 0 ]; do
  case "$1" in
    --strict) STRICT=1; shift ;;
    --check) CHECK=1; shift ;;
    --stale-days)
      if [ $# -lt 2 ] \
        || ! printf '%s\n' "$2" | grep -qE '^[1-9][0-9]*$' \
        || [ "${#2}" -gt "${#MAX_STALE_DAYS}" ] \
        || [ "$2" -gt "$MAX_STALE_DAYS" ]; then
        echo "--stale-days requires an integer from 1 to $MAX_STALE_DAYS" >&2
        exit 2
      fi
      STALE_DAYS=$2
      shift 2
      ;;
    -h|--help) sed -n '2,33p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

# Prefer repo-pinned CLI in node_modules/.bin if present, falling back to PATH.
[ -d "$PWD/node_modules/.bin" ] && PATH="$PWD/node_modules/.bin:$PATH"
if ! command -v openspec >/dev/null 2>&1; then
  echo "queue unavailable: openspec CLI not found on PATH" >&2
  exit 2
fi

CHANGES_DIR="openspec/changes"
[ -d "$CHANGES_DIR" ] || { echo "queue unavailable: no $CHANGES_DIR (run from repo root)" >&2; exit 2; }

# Caveat classes, most severe first. A line matching an earlier pattern is
# reported under that class and not re-reported under a later one.
#
# Match ONLY unchecked/partial task lines. A completed task mentioning a
# block describes history, not a live condition. Word boundaries avoid
# false positives such as classifying "boot failure" prose as a red gate.
label_for() {
  local t=$1
  printf '%s' "$t" | grep -qiE '\bhalt(s|ed|ing)?\b'                    && { echo "HALT";    return; }
  printf '%s' "$t" | grep -qiE '\bred\b|\bfailed\b|\bfailing\b'         && { echo "RED";     return; }
  printf '%s' "$t" | grep -qiE '\bhold\b|\bblocked\b|\bblocking\b'      && { echo "BLOCKED"; return; }
  printf '%s' "$t" | grep -qiE '\bdeliberate|\bnot done\b|\bwont ?do\b' && { echo "WONTDO";  return; }
  printf '%s' "$t" | grep -qiE 'still open'                             && { echo "OPEN-Q";  return; }
  echo ""
}

# scan_tasks FILE prints one tab-separated row per line of interest, tasks
# in line order:
#   open  <line> <checkbox line>  an open task ("- [ ]" or "- [~]") with no hold
#   held  <line> <checkbox line>  an open task with a hold; its hold row follows
#   hold  <line> <task number> <the block's text from its first Hold: on>
#   stray <line>                  a Hold: below the first "## " heading, in no block
scan_tasks() {
  python3 - "$1" <<'PY'
import re, sys

TASK = re.compile(r"\s*- \[(.)\]")
tasks, strays = [], []
task, below = None, False
with open(sys.argv[1], encoding="utf-8") as f:
    for n, line in enumerate(f, 1):
        line = line.rstrip("\n")
        depth = len(line) - len(line.lstrip())
        m = TASK.match(line)
        if m:
            words = line[m.end():].split()
            task = {"line": n, "open": m.group(1) in " ~", "first": line, "depth": depth,
                    "number": words[0] if words else "", "hold": 0, "text": []}
            tasks.append(task)
        elif task and line.strip() and depth <= task["depth"]:
            task = None
        if task is None:
            if below and "Hold:" in line:
                strays.append(n)
        elif task["hold"]:
            task["text"].append(line)
        elif "Hold:" in line:
            task["hold"] = n
            task["text"].append(line[line.index("Hold:"):])
        if line.startswith("## "):
            below = True
for t in tasks:
    if not t["open"]:
        continue
    if t["hold"]:
        print("held\t%d\t%s" % (t["line"], t["first"]))
        print("hold\t%d\t%s %s" % (t["hold"], t["number"], " ".join(t["text"])))
    else:
        print("open\t%d\t%s" % (t["line"], t["first"]))
for n in strays:
    print("stray\t%d" % n)
PY
}

now_epoch=$(date -u +%s)
found_any=0
change_count=0
input_error=0

if [ "$CHECK" -eq 0 ]; then
  printf '\n%s\n' "openspec queue — why each in-flight change is still open"
  printf '%s\n\n' "-------------------------------------------------------"
fi

if json=$(openspec list --json 2>&1); then status=0; else status=$?; fi
if [ "$status" -ne 0 ]; then
  printf 'queue unavailable: openspec list --json exited %s\n%s\n' "$status" "$json" >&2
  exit 2
fi

# Parse with python3 so a missing jq does not silently degrade this report
# to nothing.
if ! rows=$(printf '%s' "$json" | python3 -c '
import json,re,sys

def invalid(message):
    print(f"queue unavailable: invalid openspec JSON: {message}", file=sys.stderr)
    sys.exit(2)

try:
    d = json.load(sys.stdin)
except Exception as error:
    invalid(str(error))

if not isinstance(d, dict):
    invalid("root must be an object")
if "changes" not in d:
    invalid("root is missing required changes field")
if not isinstance(d["changes"], list):
    invalid("changes must be a list")

for index, c in enumerate(d["changes"]):
    if not isinstance(c, dict):
        invalid(f"changes[{index}] must be an object")

    name = c.get("name")
    if not isinstance(name, str) or not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._-]*", name):
        invalid(f"changes[{index}].name must be a nonempty safe path component")

    for field in ("completedTasks", "totalTasks"):
        if field not in c:
            invalid(f"changes[{index}] is missing required {field} field")
        if type(c[field]) is not int or c[field] < 0:
            invalid(f"changes[{index}].{field} must be a nonnegative integer")
    if c["completedTasks"] > c["totalTasks"]:
        invalid(f"changes[{index}].completedTasks exceeds totalTasks")

    print("\t".join([
        name,
        str(c["completedTasks"]),
        str(c["totalTasks"]),
        str(c.get("lastModified","")),
    ]))
'); then
  exit 2
fi

if [ "$CHECK" -eq 1 ]; then
  read_count=0
  misplaced=0
  while IFS=$'\t' read -r name _; do
    [ -n "$name" ] || continue
    tasks_file="$CHANGES_DIR/$name/tasks.md"
    [ -f "$tasks_file" ] || continue
    scan=$(scan_tasks "$tasks_file") || { echo "queue unavailable: cannot read $tasks_file" >&2; exit 2; }
    read_count=$((read_count + 1))
    while IFS=$'\t' read -r kind lineno _; do
      [ "$kind" = stray ] || continue
      printf '%s:%s: Hold: outside every task; task spec:queue cannot show it. Put it in the task it stops.\n' \
        "$tasks_file" "$lineno"
      misplaced=1
    done <<< "$scan"
  done <<< "$rows"
  [ "$misplaced" -eq 0 ] || exit 1
  printf 'holds: ok (%d tasks.md read)\n' "$read_count"
  exit 0
fi

if [ -z "$rows" ]; then
  printf '  (queue is empty)\n\n'
  exit 0
fi

while IFS=$'\t' read -r name done total modified; do
  [ -n "$name" ] || continue
  change_count=$((change_count + 1))

  age_note=""
  if [ -n "$modified" ]; then
    mod_epoch=$(python3 -c "
import datetime
try:
    s='$modified'.replace('Z','+00:00')
    print(int(datetime.datetime.fromisoformat(s).timestamp()))
except Exception:
    print(0)
" 2>/dev/null)
    if [ "${mod_epoch:-0}" -gt 0 ]; then
      age_days=$(( (now_epoch - mod_epoch) / 86400 ))
      [ "$age_days" -ge "$STALE_DAYS" ] && age_note="   [stale: ${age_days}d]"
    fi
  fi

  printf '  %-42s %s/%s%s\n' "$name" "$done" "$total" "$age_note"

  tasks_file="$CHANGES_DIR/$name/tasks.md"
  if [ ! -f "$tasks_file" ]; then
    printf '      ERROR    no tasks.md\n\n'
    input_error=1
    continue
  fi

  scan=$(scan_tasks "$tasks_file") || { echo "queue unavailable: cannot read $tasks_file" >&2; exit 2; }

  # Unchecked "- [ ]" and partial "- [~]" tasks only. [~] is ALWAYS a
  # caveat regardless of wording — it means a deliberate decision was
  # recorded, and that decision has to be propagated into the spec delta
  # before this change can be archived.
  caveats=0
  while IFS=$'\t' read -r kind lineno text; do
    marker=""
    case "$kind" in
      open|held)
        case "$text" in
          *'- [~]'*) marker="WONTDO" ;;
        esac
        [ -z "$marker" ] && marker="$(label_for "$text")"
        # The task's hold row says BLOCKED; its checkbox line does not say it again.
        [ "$kind" = held ] && [ "$marker" = BLOCKED ] && marker=""
        ;;
      hold) marker="BLOCKED" ;;
    esac
    [ -z "$marker" ] && continue

    # Trim leading list syntax and squeeze whitespace for a compact line.
    clean=$(printf '%s' "$text" \
      | sed -e 's/^[[:space:]]*- \[[^]]*\][[:space:]]*//' \
            -e 's/\*\*//g' \
            -e 's/[[:space:]][[:space:]]*/ /g')
    printf '      %-8s L%-5s %.104s\n' "$marker" "$lineno" "$clean"
    caveats=$((caveats + 1))
    found_any=1
  done <<< "$scan"

  if [ "$caveats" -eq 0 ]; then
    printf '      %-8s no halt/hold/deliberate marker in the open tasks\n' "ok"
  fi
  printf '\n'
done <<< "$rows"

printf -- '-------------------------------------------------------\n'
printf '  %d change(s) in flight.\n' "$change_count"
if [ "$found_any" -eq 1 ]; then
  printf '  Read the flagged lines before treating any fraction above as "almost done".\n'
fi
printf '\n'

if [ "$input_error" -eq 1 ]; then
  exit 2
fi
if [ "$STRICT" -eq 1 ] && [ "$found_any" -eq 1 ]; then
  exit 1
fi
exit 0
