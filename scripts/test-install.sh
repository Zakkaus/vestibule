#!/bin/sh
# Exercise deploy/install.sh in an isolated root with fake release and systemctl adapters.
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
installer=${INSTALLER_UNDER_TEST:-${ROOT}/deploy/install.sh}
unit_under_test=${UNIT_UNDER_TEST:-${ROOT}/deploy/vestibule.service}
selected=all

if [ "$#" -gt 0 ]; then
	[ "$#" -eq 2 ] && [ "$1" = --case ] || {
		echo "usage: test-install.sh [--case hardening|lifecycle|bot-env|failure-cleanup|rollback-preflight|checksums]" >&2
		exit 2
	}
	selected=$2
fi

fail() {
	printf 'FAIL test-install: %s\n' "$*" >&2
	exit 1
}

pass() {
	printf 'ok test-install: %s\n' "$1"
}

[ -f "$installer" ] || fail "installer is missing: $installer"
[ -f "$unit_under_test" ] || fail "unit is missing: $unit_under_test"

tmp=$(mktemp -d "${TMPDIR:-/tmp}/vestibule-install-test.XXXXXX") || fail "cannot create temporary directory"
trap 'rm -rf -- "$tmp"' EXIT HUP INT TERM
fixtures=${tmp}/releases
mkdir -p "$fixtures"

case $(uname -m) in
	x86_64) arch=amd64 ;;
	aarch64 | arm64) arch=arm64 ;;
	*) fail "test has no release fixture architecture for $(uname -m)" ;;
esac

make_release() {
	tag=$1
	target=$2
	minimum=$3
	release=${fixtures}/${tag}
	mkdir -p "$release"
	cat > "${release}/vestibule-linux-${arch}" <<EOF
#!/bin/sh
printf '%s\\n' '${tag}'
EOF
	chmod 755 "${release}/vestibule-linux-${arch}"
	cp "$unit_under_test" "${release}/vestibule.service"
	cat > "${release}/vestibule-schema-manifest" <<EOF
target_schema_version=${target}
minimum_rollback_schema_version=${minimum}
EOF
	(
		cd "$release"
		sha256sum "vestibule-linux-${arch}" vestibule.service vestibule-schema-manifest > SHA256SUMS
	)
}

make_release v1.0.0 1 1
make_release v2.0.0 2 1
make_release v3.0.0 3 2

fake_fetch=${tmp}/fetch
cat > "$fake_fetch" <<'EOF'
#!/bin/sh
set -eu
url=$1
destination=$2
printf '%s\n' "$url" >> "$FETCH_LOG"
case $url in
	*/releases/latest)
		printf '{"tag_name":"v2.0.0"}\n' > "$destination"
		;;
	*/releases/download/*/*)
		rest=${url#*/releases/download/}
		tag=${rest%%/*}
		asset=${rest#*/}
		cp "$FIXTURE_RELEASES/$tag/$asset" "$destination"
		;;
	*)
		echo "fake fetch: unexpected URL: $url" >&2
		exit 64
		;;
esac
EOF
chmod 755 "$fake_fetch"

fake_systemctl=${tmp}/systemctl
cat > "$fake_systemctl" <<'EOF'
#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$SYSTEMCTL_LOG"
case $1 in
	restart)
		setup_env=${VESTIBULE_ROOT}/etc/vestibule/setup.env
		if [ -f "$setup_env" ]; then
			setup_token=$(sed -n 's/^SETUP_TOKEN=//p' "$setup_env")
			setup_hash=$(printf '%s' "$setup_token" | sha256sum)
			setup_hash=${setup_hash%% *}
			mkdir -p "${VESTIBULE_ROOT}/var/lib/vestibule"
			printf '{"setup_token_hash":"%s"}\n' "$setup_hash" > "${VESTIBULE_ROOT}/var/lib/vestibule/claim.json"
			chmod 600 "${VESTIBULE_ROOT}/var/lib/vestibule/claim.json"
		fi
		;;
	is-enabled) printf 'enabled\n' ;;
	is-active) printf 'active\n' ;;
esac
[ "${FAIL_SYSTEMCTL:-}" != "$1" ] || exit 42
EOF
chmod 755 "$fake_systemctl"

new_sandbox() {
	case_name=$1
	sandbox=${tmp}/${case_name}
	test_root=${sandbox}/root
	fetch_log=${sandbox}/fetch.log
	systemctl_log=${sandbox}/systemctl.log
	mkdir -p "${test_root}/usr/local/bin" "${test_root}/etc/systemd/system" \
		"${test_root}/etc" "${test_root}/var/lib" "${sandbox}/tmp"
	: > "$fetch_log"
	: > "$systemctl_log"
	fail_systemctl=
}

run_installer() {
	FETCH_LOG=$fetch_log \
	SYSTEMCTL_LOG=$systemctl_log \
	FAIL_SYSTEMCTL=$fail_systemctl \
	FIXTURE_RELEASES=$fixtures \
	VESTIBULE_FETCH=$fake_fetch \
	VESTIBULE_SYSTEMCTL=$fake_systemctl \
	VESTIBULE_ROOT=$test_root \
	TMPDIR=${sandbox}/tmp \
		sh "$installer" "$@"
}

run_success() {
	log=$1
	shift
	if ! run_installer "$@" > "$log" 2>&1; then
		sed 's/^/    /' "$log" >&2
		fail "installer command failed: $*"
	fi
}

assert_file() {
	[ -f "$1" ] || fail "expected file is missing: $1"
}

assert_absent() {
	[ ! -e "$1" ] || fail "path should not exist: $1"
}

assert_mode() {
	actual=$(stat -c '%a' "$1")
	[ "$actual" = "$2" ] || fail "$1 mode is $actual, want $2"
}

assert_line() {
	line=$1
	file=$2
	grep -Fqx "$line" "$file" || fail "$file is missing: $line"
}

assert_same() {
	cmp -s "$1" "$2" || fail "$1 and $2 differ"
}

case_hardening() {
	for line in \
		'Type=notify' \
		'WatchdogSec=120s' \
		'DynamicUser=yes' \
		'ProtectSystem=strict' \
		'CapabilityBoundingSet=' \
		'SystemCallFilter=@system-service' \
		'RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6' \
		'UMask=0077' \
		'StateDirectoryMode=0700' \
		'EnvironmentFile=-/etc/vestibule/setup.env'; do
		assert_line "$line" "$unit_under_test"
	done
	if grep -q 'BOT_TOKEN' "$installer"; then
		fail "$installer still reads or names the bot token"
	fi
	if grep -q '/etc/os-release' "$installer"; then
		fail "$installer branches on a distribution name instead of capabilities"
	fi
	pass "systemd hardening and installer boundaries"
}

case_lifecycle() {
	new_sandbox lifecycle
	run_success "${sandbox}/install.out" v1.0.0
	installed_binary=${test_root}/usr/local/bin/vestibule
	installed_unit=${test_root}/etc/systemd/system/vestibule.service
	result=${test_root}/etc/vestibule/install-result.env
	claim=${test_root}/var/lib/vestibule/claim.json
	bot_env=${test_root}/etc/vestibule/bot.env
	assert_same "${fixtures}/v1.0.0/vestibule-linux-${arch}" "$installed_binary"
	assert_same "$unit_under_test" "$installed_unit"
	assert_file "$bot_env"
	assert_mode "$bot_env" 600
	assert_mode "$claim" 600
	assert_mode "$result" 600
	assert_line 'operation=install' "$result"
	assert_absent "${test_root}/etc/vestibule/setup.env"
	claim_url=$(sed -n 's/^claim_url=//p' "$result")
	case $claim_url in http://127.0.0.1:8080/setup/[0-9a-f][0-9a-f]*) ;; *) fail "result has no claim URL: $claim_url" ;; esac
	setup_token=${claim_url##*/}
	[ "${#setup_token}" -eq 64 ] || fail "claim token is not 32 random bytes"
	expected_hash=$(printf '%s' "$setup_token" | sha256sum)
	expected_hash=${expected_hash%% *}
	assert_line "{\"setup_token_hash\":\"${expected_hash}\"}" "$claim"
	if grep -Fq "$setup_token" "$claim"; then
		fail "claim state stores the raw one-use token instead of its hash"
	fi
	if grep -Eiq '^(bot_token|password|credential|secret)=' "$result"; then
		fail "installation result contains a credential"
	fi
	printf 'SETUP_TOKEN=stale\n' > "${test_root}/etc/vestibule/setup.env"
	printf 'database-state\n' > "${test_root}/var/lib/vestibule/database.keep"

	: > "$fetch_log"
	run_success "${sandbox}/upgrade.out" v2.0.0
	assert_absent "${test_root}/etc/vestibule/setup.env"
	assert_same "${fixtures}/v2.0.0/vestibule-linux-${arch}" "$installed_binary"
	assert_same "${fixtures}/v1.0.0/vestibule-linux-${arch}" "${installed_binary}.previous"
	assert_line 'operation=upgrade' "$result"
	manifest_line=$(grep -n '/vestibule-schema-manifest$' "$fetch_log" | cut -d: -f1)
	binary_line=$(grep -n "/vestibule-linux-${arch}$" "$fetch_log" | cut -d: -f1)
	[ "$manifest_line" -lt "$binary_line" ] || fail "upgrade fetched the binary before checking the schema manifest"

	run_success "${sandbox}/status.out" --status
	assert_line 'installed=yes' "${sandbox}/status.out"
	assert_line 'version=v2.0.0' "${sandbox}/status.out"
	assert_line 'service_active=active' "${sandbox}/status.out"

	run_success "${sandbox}/rollback.out" --rollback
	assert_same "${fixtures}/v1.0.0/vestibule-linux-${arch}" "$installed_binary"
	assert_same "${fixtures}/v2.0.0/vestibule-linux-${arch}" "${installed_binary}.previous"
	assert_line 'operation=rollback' "$result"

	run_success "${sandbox}/uninstall.out" --uninstall --keep-data
	assert_absent "$installed_binary"
	assert_absent "$installed_unit"
	assert_file "$bot_env"
	assert_file "$claim"
	assert_file "${test_root}/var/lib/vestibule/database.keep"
	assert_line 'operation=uninstall' "$result"
	pass "install, same-command upgrade, status, rollback, and uninstall"
}

case_bot_env() {
	new_sandbox bot-env
	mkdir -p "${test_root}/etc/vestibule"
	printf 'operator-owned-bytes\n' > "${test_root}/etc/vestibule/bot.env"
	cp "${test_root}/etc/vestibule/bot.env" "${sandbox}/bot.env.before"
	run_success "${sandbox}/install.out" v1.0.0
	assert_same "${sandbox}/bot.env.before" "${test_root}/etc/vestibule/bot.env"
	pass "existing bot.env remains byte-identical"
}

case_failure_cleanup() {
	new_sandbox failure-cleanup
	mkdir -p "${test_root}/etc/vestibule" "${test_root}/var/lib/vestibule"
	printf 'operator-owned-bytes\n' > "${test_root}/etc/vestibule/bot.env"
	printf 'existing-unit\n' > "${test_root}/etc/systemd/system/vestibule.service"
	printf 'unrelated-binary\n' > "${test_root}/usr/local/bin/unrelated"
	printf 'database-state\n' > "${test_root}/var/lib/vestibule/database.keep"
	cp "${test_root}/etc/vestibule/bot.env" "${sandbox}/bot.env.before"
	cp "${test_root}/etc/systemd/system/vestibule.service" "${sandbox}/unit.before"
	fail_systemctl=restart
	if run_installer v1.0.0 > "${sandbox}/failure.out" 2>&1; then
		fail "installation unexpectedly succeeded when systemctl restart failed"
	fi
	assert_absent "${test_root}/usr/local/bin/vestibule"
	assert_absent "${test_root}/etc/vestibule/schema-manifest"
	assert_absent "${test_root}/etc/vestibule/release-version"
	assert_absent "${test_root}/var/lib/vestibule/claim.json"
	assert_absent "${test_root}/etc/vestibule/install-result.env"
	assert_same "${sandbox}/bot.env.before" "${test_root}/etc/vestibule/bot.env"
	assert_same "${sandbox}/unit.before" "${test_root}/etc/systemd/system/vestibule.service"
	assert_file "${test_root}/usr/local/bin/unrelated"
	assert_file "${test_root}/var/lib/vestibule/database.keep"
	pass "failed installation removes only resources created by that run"
}

case_rollback_preflight() {
	new_sandbox rollback-preflight
	run_success "${sandbox}/install.out" v1.0.0
	: > "$fetch_log"
	if run_installer v3.0.0 > "${sandbox}/blocked.out" 2>&1; then
		fail "schema-incompatible upgrade unexpectedly succeeded"
	fi
	grep -Fq 'rollback preflight: unsafe' "${sandbox}/blocked.out" || fail "blocked upgrade did not explain rollback incompatibility"
	grep -q '/vestibule-schema-manifest$' "$fetch_log" || fail "blocked upgrade did not fetch the schema manifest"
	if grep -q "/vestibule-linux-${arch}$" "$fetch_log"; then
		fail "blocked upgrade downloaded the binary"
	fi
	assert_same "${fixtures}/v1.0.0/vestibule-linux-${arch}" "${test_root}/usr/local/bin/vestibule"
	pass "schema rollback preflight blocks before binary retrieval"
}

case_checksums() {
	new_sandbox checksums
	run_success "${sandbox}/install.out" v1.0.0
	printf 'corrupt\n' >> "${fixtures}/v2.0.0/vestibule-linux-${arch}"
	if run_installer v2.0.0 > "${sandbox}/bad-binary.out" 2>&1; then
		fail "upgrade accepted a binary that did not match SHA256SUMS"
	fi
	assert_same "${fixtures}/v1.0.0/vestibule-linux-${arch}" "${test_root}/usr/local/bin/vestibule"
	make_release v2.0.0 2 1
	printf 'corrupt\n' >> "${fixtures}/v2.0.0/vestibule-schema-manifest"
	: > "$fetch_log"
	if run_installer v2.0.0 > "${sandbox}/bad-manifest.out" 2>&1; then
		fail "upgrade accepted a manifest that did not match SHA256SUMS"
	fi
	if grep -q "/vestibule-linux-${arch}$" "$fetch_log"; then
		fail "manifest checksum failure happened after binary retrieval"
	fi
	pass "manifest and binary must match published SHA256SUMS"
}

run_case() {
	case $1 in
		hardening) case_hardening ;;
		lifecycle) case_lifecycle ;;
		bot-env) case_bot_env ;;
		failure-cleanup) case_failure_cleanup ;;
		rollback-preflight) case_rollback_preflight ;;
		checksums) case_checksums ;;
		*) fail "unknown test case: $1" ;;
	esac
}

if [ "$selected" = all ]; then
	for test_case in hardening lifecycle bot-env failure-cleanup rollback-preflight checksums; do
		run_case "$test_case"
	done
else
	run_case "$selected"
fi

printf 'test-install: passed\n'
