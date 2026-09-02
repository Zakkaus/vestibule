#!/bin/sh
# Phase zero's acceptance, from docs/PLAN-v5.md, as executable checks.
set -u

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT" || exit 1

fail=0
say() { printf '%-72s %s\n' "$1" "$2"; }
bad() { say "$1" "FAIL"; fail=1; }
ok() { say "$1" "ok"; }
exempt() { say "$1" "EXEMPT: $2"; }

# 1. scripts/lint.sh is executable.
if scripts/lint.sh; then
  ok "scripts/lint.sh executes"
else
  bad "scripts/lint.sh executes"
fi

# 2. A 601-line file fails specifically at the file-line assertion.
probe="scripts/.accept-phase0-$$.go"
if [ -e "$probe" ]; then
  bad "601-line file fails at the file-line check (probe already exists)"
else
  trap 'rm -f "$probe"' 0 HUP INT TERM
  {
    printf 'package main\n'
    line=1
    while [ "$line" -le 600 ]; do
      printf '// acceptance probe line %s\n' "$line"
      line=$((line + 1))
    done
  } >"$probe"

  if output=$(scripts/lint.sh 2>&1); then
    bad "601-line file fails at the file-line check"
    printf '%s\n' "$output" | sed 's/^/    /'
  else
    case "$output" in
      *"FAIL file-lines: new $probe has 601 lines (limit 600)"*)
        ok "601-line file fails at the file-line check"
        ;;
      *)
        bad "601-line file fails at the file-line check"
        printf '%s\n' "$output" | sed 's/^/    /'
        ;;
    esac
  fi
  rm -f "$probe"
  trap - 0 HUP INT TERM
fi

# 3. The baseline is a phase-zero snapshot, not a present-day invariant.
exempt "frozen baseline rows still resolve to their original source lines" \
  "historical-only: PLAN-v5.md:161-174 records moved and deleted sources"

[ "$fail" -eq 0 ] && echo "phase 0 acceptance: passed" || echo "phase 0 acceptance: FAILED" >&2
exit "$fail"
