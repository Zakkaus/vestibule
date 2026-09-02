#!/bin/sh
# Phase eight's acceptance, from docs/PLAN-v5.md, as executable checks.
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
  log=$(mktemp "${TMPDIR:-/tmp}/accept-phase8.XXXXXX") || {
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

# 1. An empty community group list still starts the bot and console service graph.
run "empty community configuration leaves bot and console service graph working" \
  go test ./internal/app -run '^TestNewServicesAllowsEmptyGroups$' -count=1

# 2. With every optional module disabled, startup succeeds and commands disappear.
run "all optional modules disabled still starts without their command entries" \
  go test ./internal/app -run '^TestNewServicesAllowsAllOptionalModulesDisabled$' -count=1

# 3. No legacy main-community branch remains in production code.
if grep -r -n -E '主群|mainGroup|isMainChat' internal cmd >/dev/null 2>&1; then
  bad "no legacy main-community business branch in internal or cmd"
  grep -r -n -E '主群|mainGroup|isMainChat' internal cmd || true
else
  ok "no legacy main-community business branch in internal or cmd"
fi

[ "$fail" -eq 0 ] && echo "phase 8 acceptance: passed" || echo "phase 8 acceptance: FAILED" >&2
exit "$fail"
