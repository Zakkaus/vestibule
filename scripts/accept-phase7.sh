#!/bin/sh
# Phase seven's acceptance, from docs/PLAN-v5.md, as executable checks.
set -u

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT" || exit 1

fail=0
say() { printf '%-72s %s\n' "$1" "$2"; }
bad() { say "$1" "FAIL"; fail=1; }
ok() { say "$1" "ok"; }
exempt() { say "$1" "EXEMPT: $2"; }

if [ ! -x web/node_modules/.bin/playwright ]; then
  if ! (cd web && npm ci --silent); then
    bad "frontend test dependencies install"
  fi
fi

# The literal three-locale matrix conflicts with the implemented acceptance.
exempt "every screen runs separately in all three locales" \
  "render-gate measures all locale widths then runs the widest; phase eleven runs every locale on a lower frequency (decided 2026-09-02)"

# The implemented render gate discovers routes and checks the visual invariants in Chromium.
log=$(mktemp "${TMPDIR:-/tmp}/accept-phase7.XXXXXX") || exit 1
if (cd web && npm run e2e -- e2e/render-gate.spec.ts) >"$log" 2>&1; then
  ok "widest locale has no truncation, horizontal overflow, or displaced controls"
else
  bad "widest locale has no truncation, horizontal overflow, or displaced controls"
  sed 's/^/    /' "$log"
fi
rm -f "$log"

[ "$fail" -eq 0 ] && echo "phase 7 acceptance: passed" || echo "phase 7 acceptance: FAILED" >&2
exit "$fail"
