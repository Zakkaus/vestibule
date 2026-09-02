#!/bin/sh
# Phase two's acceptance, from docs/PLAN-v5.md, as executable checks.
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
  log=$(mktemp "${TMPDIR:-/tmp}/accept-phase2.XXXXXX") || {
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

# 1. The package-boundary gate passes.
run "package-boundary gate" scripts/lint.sh

# 2. Every committed spam-evasion fixture normalizes to its asserted match.
run "spam-evasion fixtures normalize to asserted matches" \
  go test ./internal/rules -run '^TestNormalizeSpamFixtures$' -count=1

# 3. Rule evaluation remains a pure, directly callable function package.
run "rule evaluation remains directly callable pure behavior" \
  go test ./internal/rules \
  -run '^(TestRuleRejectsBeforeAccept|TestVersionRangeAcceptsNormalizedProcVersionOutput|TestOneOfAnswerNormalization)$' \
  -count=1
exempt "console test-answer directly calls the same rule entry point" \
  "POST /api/chats/{id}/rules/test is unimplemented; route ownership is pending at PLAN-v5.md:1062"

# This was explicitly excluded from phase two because no source implementation exists.
exempt "structural-signal samples and assertions" \
  "the feature was not written; PLAN-v5.md:292-303 and :1064 defer the decision"

[ "$fail" -eq 0 ] && echo "phase 2 acceptance: passed" || echo "phase 2 acceptance: FAILED" >&2
exit "$fail"
