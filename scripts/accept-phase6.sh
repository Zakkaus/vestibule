#!/bin/sh
# Phase six's acceptance, from docs/PLAN-v5.md, as executable checks.
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
  log=$(mktemp "${TMPDIR:-/tmp}/accept-phase6.XXXXXX") || {
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
run_web() {
  label=$1
  shift
  log=$(mktemp "${TMPDIR:-/tmp}/accept-phase6-web.XXXXXX") || {
    bad "$label (cannot create log)"
    return
  }
  if (cd web && "$@") >"$log" 2>&1; then
    ok "$label"
  else
    bad "$label"
    sed 's/^/    /' "$log"
  fi
  rm -f "$log"
}

if [ ! -x web/node_modules/.bin/playwright ]; then
  if ! (cd web && npm ci --silent); then
    bad "frontend test dependencies install"
  fi
fi

# 1. The implemented subset has a row in the architecture route table. The
# checker prints the named, deferred table rows rather than claiming equality.
run "architecture route table matches the implemented subset" scripts/check-console-routes.py

# 2. The frontend can start from fixture data without an API.
run_web "frontend starts against fixture data without a backend" \
  npm run e2e -- --grep '^journeys-dev phase-six\.spec\.ts entry retains its fixture fallback without an API$'

# 3. A real-browser journey reaches a successful settlement after session exchange.
run_web "browser end-to-end session exchange reaches successful release" \
  npm run e2e -- --grep '^journeys-dev phase-six\.spec\.ts Mini App session exchange reaches a successful release$'

# 4. The render gate exercises every route at both widths, themes, widest locale, and keyboard path.
run_web "render gate covers widths, themes, widest locale, and keyboard navigation" \
  npm run e2e -- e2e/render-gate.spec.ts

# 5. The installed health check answers after a deployment.
# Phase nine delivered this, so the clause that once waited on it now runs the same
# case phase nine's own acceptance runs.
run "post-deployment health-check command" \
  scripts/test-replacement.sh --case health-endpoints

# Phase nine is complete and did not deliver an isolated deployment holding a real bot
# account, which is phase ten's to arrange. Naming the blocker rather than a phase keeps
# this reason from expiring the way the line above did.
exempt "test console settles a real test-group join request" \
  "needs a live Telegram bot account in an isolated deployment; no unattended authority covers that"

[ "$fail" -eq 0 ] && echo "phase 6 acceptance: passed" || echo "phase 6 acceptance: FAILED" >&2
exit "$fail"
