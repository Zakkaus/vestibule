#!/bin/sh
# Phase five's acceptance, from docs/PLAN-v5.md, as executable checks.
set -u

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT" || exit 1

fail=0
say() { printf '%-72s %s\n' "$1" "$2"; }
bad() { say "$1" "FAIL"; fail=1; }
ok() { say "$1" "ok"; }
run() {
  label=$1
  shift
  log=$(mktemp "${TMPDIR:-/tmp}/accept-phase5.XXXXXX") || {
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

# 1. A valid Mini App payload creates a session; expiry and replay are rejected.
run "Mini App initData creates a session; expiry and replay are rejected" \
  go test ./internal/console/api \
  -run '^(TestPostSessionRejectsReplayedInitData|TestGetSessionRejectsExpiredCookie)$' -count=1

# 2. Writes without the CSRF header return csrf_invalid.
run "write without X-CSRF-Token returns csrf_invalid" \
  go test ./internal/console/api -run '^TestPostSettlementRequiresCSRF$' -count=1

# 3. A chat outside the authorized list returns chat_not_found, not an empty result.
run "unauthorized chat ID returns chat_not_found, not an empty collection" \
  go test ./internal/console/api \
  -run '^TestZeroConfiguredGroupsReturnEmptyListAndChatNotFound$' -count=1

# 4. The HTTP adapter reaches the verification service and has no direct SQL path.
log=$(mktemp "${TMPDIR:-/tmp}/accept-phase5-boundary.XXXXXX") || exit 1
if go test ./internal/console/api \
  -run '^TestOperatorCanSettleAfterReadingCurrentSession$' -count=1 >"$log" 2>&1; then
  if python3 - >>"$log" 2>&1 <<'PY'
from pathlib import Path
import re

api = Path("internal/console/api")
server = (api / "server.go").read_text(encoding="utf-8")
if "s.verification.SettleConsole(" not in server:
    raise SystemExit("settlement no longer calls verification.Service")

for source in api.glob("*.go"):
    if source.name.endswith("_test.go"):
        continue
    text = source.read_text(encoding="utf-8")
    if '"database/sql"' in text or '"github.com/Zakkaus/vestibule/internal/database"' in text:
        raise SystemExit(f"{source} imports a database package")
    if re.search(r"\.(?:Exec(?:Context)?|QueryRow(?:Context)?|QueryContext)\s*\(", text):
        raise SystemExit(f"{source} executes a database query directly")
PY
  then
    ok "settlement goes through verification.Service without direct SQL"
  else
    bad "settlement goes through verification.Service without direct SQL"
    sed 's/^/    /' "$log"
  fi
else
  bad "settlement goes through verification.Service without direct SQL"
  sed 's/^/    /' "$log"
fi
rm -f "$log"

[ "$fail" -eq 0 ] && echo "phase 5 acceptance: passed" || echo "phase 5 acceptance: FAILED" >&2
exit "$fail"
