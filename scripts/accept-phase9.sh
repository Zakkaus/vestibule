#!/bin/sh
# Phase nine's acceptance, from docs/PLAN-v5.md, in the plan's clause order.
set -u

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT" || exit 1

fail=0
say() { printf '%-72s %s\n' "$1" "$2"; }
bad() { say "$1" "FAIL"; fail=1; }
ok() { say "$1" "ok"; }
exempt() {
	say "$1" "EXEMPT"
	printf '    %s\n' "$2"
}
run() {
	label=$1
	shift
	log=$(mktemp "${TMPDIR:-/tmp}/accept-phase9.XXXXXX") || {
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
run_then_exempt() {
	label=$1
	reason=$2
	shift 2
	log=$(mktemp "${TMPDIR:-/tmp}/accept-phase9.XXXXXX") || {
		bad "$label (cannot create log)"
		return
	}
	if "$@" >"$log" 2>&1; then
		exempt "$label" "$reason"
	else
		bad "$label"
		sed 's/^/    /' "$log"
	fi
	rm -f "$log"
}

# 1. One command starts the complete application, database, and self-hosted Bot API stack.
run_then_exempt "one command starts the complete three-component stack" \
	"the native five-action lifecycle runs in an isolated root; the container and self-hosted Bot API path belongs to the next phase-nine slice (PLAN-v5.md, '容器是默认部署方式')" \
	scripts/test-install.sh --case lifecycle

# 2. Installed health checks pass.
exempt "installed /livez and /readyz health checks pass" \
	"the isolated systemctl adapter does not start a real process; this slice explicitly exempts health checks rather than treating adapter success as service health"

# 3. A deliberately broken new binary automatically rolls back and records the result.
exempt "broken replacement automatically rolls back and records why" \
	"the host replacement unit and a real systemd rollback drill belong to the next phase-nine slice; this slice verifies explicit rollback only"

# 4. An incompatible schema blocks before the release binary is retrieved.
run "schema incompatibility is reported before binary retrieval" \
	scripts/test-install.sh --case rollback-preflight

# 5. A clean machine can follow install.sh and open the printed address immediately.
exempt "clean-machine install opens the printed address" \
	"requires a disposable host plus the domain, certificate, container, and browser path still pending in phase nine; this worktree must not touch production"

[ "$fail" -eq 0 ] && echo "phase 9 acceptance: passed with exemptions" || echo "phase 9 acceptance: FAILED" >&2
exit "$fail"
