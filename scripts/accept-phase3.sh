#!/bin/sh
# Phase three's acceptance, from docs/PLAN-v5.md, as executable checks.
set -u

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT" || exit 1

fail=0
say() { printf '%-72s %s\n' "$1" "$2"; }
bad() { say "$1" "FAIL"; fail=1; }
ok() { say "$1" "ok"; }
exempt() { say "$1" "EXEMPT: $2"; }
run() {
  label=$1
  shift
  log=$(mktemp "${TMPDIR:-/tmp}/accept-phase3.XXXXXX") || {
    bad "$label (cannot create log)"
    return
  }
  if "$@" >"$log" 2>&1; then
    ok "$label"
  else
    bad "$label"
    sed 's/^/    /' "$log"
  fi
  rm -f "$log"
}

# 1 and 3. One focused test replays the legacy snapshot and validates every imported record.
import_log=$(mktemp "${TMPDIR:-/tmp}/accept-phase3-import.XXXXXX") || exit 1
if go test ./internal/database -run '^TestImportLegacyStateReplay$' -count=1 >"$import_log" 2>&1; then
  import_ok=1
else
  import_ok=0
fi
if [ "$import_ok" -eq 1 ]; then
  ok "legacy migration can replay"
else
  bad "legacy migration can replay"
  sed 's/^/    /' "$import_log"
fi

# 2. A database newer than this binary's schema refuses startup.
run "old binary rejects a newer database schema at startup" \
  go test ./internal/database -run '^TestOpenRejectsNewerSchema$' -count=1

if [ "$import_ok" -eq 1 ]; then
  ok "legacy import keeps waiting-queue and operation records validated"
else
  bad "legacy import keeps waiting-queue and operation records validated"
fi
rm -f "$import_log"

# The test group needs a live Telegram admission event and a deployed test database.
exempt "test-group join request, expiry scanner, and conditional settlement" \
  "requires live Telegram and the isolated test deployment; unit tests cannot observe it"

[ "$fail" -eq 0 ] && echo "phase 3 acceptance: passed" || echo "phase 3 acceptance: FAILED" >&2
exit "$fail"
