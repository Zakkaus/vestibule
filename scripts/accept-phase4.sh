#!/bin/sh
# Phase four's acceptance, from docs/PLAN-v5.md, as executable checks.
set -u

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT" || exit 1

fail=0
say() { printf '%-72s %s\n' "$1" "$2"; }
bad() { say "$1" "FAIL"; fail=1; }
ok() { say "$1" "ok"; }

log=$(mktemp "${TMPDIR:-/tmp}/accept-phase4.XXXXXX") || exit 1
if go test ./internal/settings \
  -run '^TestUpgradeSettingsVersion(Zero|One|Two|Three)$' -count=1 >"$log" 2>&1; then
  ok "all four configuration versions upgrade, preserve overrides, and default new fields"
else
  bad "all four configuration versions upgrade, preserve overrides, and default new fields"
  sed 's/^/    /' "$log"
fi
rm -f "$log"

[ "$fail" -eq 0 ] && echo "phase 4 acceptance: passed" || echo "phase 4 acceptance: FAILED" >&2
exit "$fail"
