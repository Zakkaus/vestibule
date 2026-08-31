#!/bin/sh
# Phase one's acceptance, from docs/PLAN-v5.md, as something that runs.
#
# Written as prose it read fine and proved nothing. Each clause below is the
# clause from the plan, in the same order, with the command that decides it.
# Run from the repository root. Non-zero on any failure.
set -u
fail=0
say() { printf '%-52s %s\n' "$1" "$2"; }
bad() { say "$1" "FAIL"; fail=1; }
ok()  { say "$1" "ok"; }

# 1. Both build tags compile.
go build ./... >/dev/null 2>&1 && ok "builds, default tag" || bad "builds, default tag"
go build -tags gentoo ./... >/dev/null 2>&1 && ok "builds, gentoo tag" || bad "builds, gentoo tag"

# 2. Both test suites are green.
go test -race ./... >/tmp/p1-test.log 2>&1 && ok "tests, default tag" || bad "tests, default tag"
go test -race -tags gentoo ./... >/tmp/p1-test-gentoo.log 2>&1 \
  && ok "tests, gentoo tag" || bad "tests, gentoo tag"

# 3. The core carries no platform type. The package need not exist yet; a
#    slice that has not reached it passes this trivially, which is correct.
if [ -d internal/verification ]; then
  if grep -rq telego internal/verification 2>/dev/null; then
    bad "no telego in internal/verification"
  else
    ok "no telego in internal/verification"
  fi
else
  say "no telego in internal/verification" "n/a, package not created yet"
fi

# 4. Every target package that exists pins its interfaces at compile time, or
#    has tests. An empty package is scaffolding, which the limits forbid.
for p in app verification rules telegram console settings database status; do
  d="internal/$p"
  [ -d "$d" ] || continue
  if find "$d" -name '*.go' | head -1 | grep -q . ; then
    if grep -rqE '^var _ [A-Za-z]+\.[A-Za-z]+ = ' "$d" 2>/dev/null \
       || find "$d" -name '*_test.go' | head -1 | grep -q .; then
      ok "internal/$p is pinned or tested"
    else
      bad "internal/$p is pinned or tested"
    fi
  else
    bad "internal/$p has no Go files"
  fi
done

# 5. Phase zero's measurements gained nothing. The gate prints how many
#    baselined violations it is holding; that number may fall, never rise.
if [ -f scripts/lint.sh ]; then
  out=$(sh scripts/lint.sh 2>&1)
  if printf '%s' "$out" | grep -q FAIL; then
    bad "phase-zero gate"
    printf '%s\n' "$out" | grep FAIL | sed 's/^/    /'
  else
    held=$(printf '%s' "$out" | grep -oE 'holding [0-9]+' | grep -oE '[0-9]+')
    base=${PHASE0_HELD:-26}
    if [ -n "$held" ] && [ "$held" -gt "$base" ]; then
      bad "held violations $held, was $base"
    else
      ok "phase-zero gate, holding ${held:-0} (was $base)"
    fi
  fi
else
  bad "scripts/lint.sh is missing"
fi

[ "$fail" -eq 0 ] && echo "phase 1 acceptance: passed" || echo "phase 1 acceptance: FAILED" >&2
exit "$fail"
