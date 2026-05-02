#!/bin/bash
#
# agentic-chain-watcher.sh
#
# Active monitor for an agentic-loop chain running on the local
# semteams stack. Polls /teams-dispatch/loops every 5s, logs role
# transitions, and auto-approves any pending tool gate (add_source_repo
# is the canonical case; the script is tool-agnostic — it approves
# whatever pending_approval.tool_name appears).
#
# Designed for manual smoke runs of approval-gated journeys (R2.5
# source-acquisition, R3.4 OSH demo) where Playwright-driven approval
# is not appropriate. Mock-llm Playwright specs already drive
# approvals via the spec itself; this script is for real-LLM smokes
# invoked via `task test:e2e:agentic:dev:*` tasks.
#
# Bash 3.2 compatible (macOS default). Uses no associative arrays;
# de-dups call_ids via append-only files in /tmp so a stale list
# endpoint can't trigger HTTP-409 spam from the same call_id.
#
# Usage:
#   ui/scripts/agentic-chain-watcher.sh [<run_id>] [<port>]
#
# - run_id  : prefix for the trajectory and events files in /tmp.
#             Default: "agentic". Example: "r34-osh".
# - port    : Caddy port the dispatch endpoint listens on.
#             Default: 3100.
#
# Outputs (in /tmp, prefixed by <run_id>):
#   <run_id>-trajectory.jsonl  — one JSON snapshot per poll
#   <run_id>-events.log        — human-readable timeline
#   <run_id>-seen.txt          — internal: loop IDs already reported
#   <run_id>-states.txt        — internal: latest state per loop
#   <run_id>-approved.txt      — internal: call_ids already approved
#
# Terminal condition: every loop is in state=complete or state=failed,
# AND no new loops have appeared in the last ~120s. Exits 0 on
# terminal. On stack teardown the curl polls return empty `[]` and
# the loop continues until you Ctrl-C.

set -u

PREFIX="${1:-agentic}"
PORT="${2:-3100}"

LOG=/tmp/${PREFIX}-trajectory.jsonl
EVENTS=/tmp/${PREFIX}-events.log
SEEN=/tmp/${PREFIX}-seen.txt
STATES=/tmp/${PREFIX}-states.txt
APPROVED=/tmp/${PREFIX}-approved.txt
LOOPS_URL="http://localhost:${PORT}/teams-dispatch/loops"
APPROVE_URL_BASE="http://localhost:${PORT}/teams-dispatch/loops"

> "$LOG"; > "$SEEN"; > "$STATES"; > "$APPROVED"

QUIET=0
LAST_COUNT=-1

emit() {
  echo "[$(date -u +%H:%M:%S)] $1" | tee -a "$EVENTS"
}

emit "watcher start (bash $BASH_VERSION; prefix=$PREFIX; port=$PORT)"

while true; do
  CUR=$(curl -sf "$LOOPS_URL" 2>/dev/null || echo '[]')
  COUNT=$(echo "$CUR" | jq 'length')
  echo "$CUR" | jq -c "{ts:\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\",count:length,loops:[.[]|{loop_id,role,state,iterations,outcome,pending:(.pending_approval//{})|(.tool_name//null)}]}" >> "$LOG"

  echo "$CUR" | jq -c '.[]' | while read -r row; do
    LOOP_ID=$(echo "$row" | jq -r '.loop_id')
    STATE=$(echo "$row" | jq -r '.state')
    ROLE=$(echo "$row" | jq -r '.role // "(dispatch)"')
    PENDING=$(echo "$row" | jq -r '.pending_approval.tool_name // ""')
    CALL_ID=$(echo "$row" | jq -r '.pending_approval.call_id // ""')
    OUTCOME=$(echo "$row" | jq -r '.outcome // ""')
    [ -z "$LOOP_ID" ] && continue

    # First sighting of this loop?
    if ! grep -q "^$LOOP_ID$" "$SEEN" 2>/dev/null; then
      echo "$LOOP_ID" >> "$SEEN"
      emit "NEW LOOP ${LOOP_ID:0:12} role=$ROLE state=$STATE"
    fi

    # State change since last seen?
    PREV=$(grep "^$LOOP_ID:" "$STATES" 2>/dev/null | tail -1 | cut -d: -f2-)
    if [ "$PREV" != "$STATE" ]; then
      emit "  → ${LOOP_ID:0:12} state '$PREV' → '$STATE'${PENDING:+ pending=$PENDING}${OUTCOME:+ outcome=$OUTCOME}"
      echo "$LOOP_ID:$STATE" >> "$STATES"
    fi

    # Auto-approve, but skip if this exact call_id has already been
    # approved (list endpoint can be stale and re-show pending after
    # we've cleared it — would cause HTTP-409 noise without the dedup).
    if [ -n "$PENDING" ] && [ -n "$CALL_ID" ]; then
      if ! grep -q "^$CALL_ID$" "$APPROVED" 2>/dev/null; then
        echo "$CALL_ID" >> "$APPROVED"
        emit "  ⊕ AUTO-APPROVING ${LOOP_ID:0:12} call_id=${CALL_ID:0:14} tool=$PENDING"
        RESP=$(curl -sf -X POST "${APPROVE_URL_BASE}/${LOOP_ID}/approval" \
          -H 'Content-Type: application/json' \
          -H "X-User-Id: ${PREFIX}-watcher" \
          -d "{\"decision\":\"approve\",\"user_id\":\"${PREFIX}-watcher\"}" 2>&1)
        emit "  ⊕ approve resp: ${RESP:0:140}"
      fi
    fi
  done

  # Terminal condition: all loops terminal + count stable for ~120s.
  ALL_TERMINAL=$(echo "$CUR" | jq 'if length==0 then false else all(.[]; .state=="complete" or .state=="failed") end')
  if [ "$ALL_TERMINAL" = "true" ] && [ "$COUNT" = "$LAST_COUNT" ]; then
    QUIET=$((QUIET+1))
    if [ $QUIET -ge 24 ]; then
      emit "All $COUNT loops terminal + count stable for ~120s; watcher exit."
      break
    fi
  else
    QUIET=0
  fi
  LAST_COUNT=$COUNT
  sleep 5
done
