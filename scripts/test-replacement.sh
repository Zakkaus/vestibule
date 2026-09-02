#!/bin/sh
# Exercise the host replacement unit with fake native and container dependencies.
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
replacer=${REPLACER_UNDER_TEST:-${ROOT}/deploy/vestibule-replace}
path_unit=${REPLACEMENT_PATH_UNDER_TEST:-${ROOT}/deploy/vestibule-replace.path}
service_unit=${REPLACEMENT_SERVICE_UNDER_TEST:-${ROOT}/deploy/vestibule-replace.service}
compose_under_test=${COMPOSE_UNDER_TEST:-${ROOT}/deploy/compose.yaml}
selected=all

if [ "$#" -gt 0 ]; then
	[ "$#" -eq 2 ] && [ "$1" = --case ] || {
		echo "usage: test-replacement.sh [--case unit-shape|native|request-preservation|container|health-endpoints|request-url|request-single-line-url|request-extra-line|automatic-rollback|rollback-required|socket-rejection]" >&2
		exit 2
	}
	selected=$2
fi

fail() {
	printf 'FAIL test-replacement: %s\n' "$*" >&2
	exit 1
}

pass() {
	printf 'ok test-replacement: %s\n' "$1"
}

assert_file() {
	[ -f "$1" ] || fail "expected file is missing: $1"
}

assert_absent() {
	[ ! -e "$1" ] || fail "path should be absent: $1"
}

assert_line() {
	grep -Fqx -- "$1" "$2" || fail "$2 is missing: $1"
}

assert_mode() {
	actual=$(stat -c '%a' "$1")
	[ "$actual" = "$2" ] || fail "$1 mode is $actual, want $2"
}

assert_no_docker_socket() {
	if grep -q 'docker\.sock' "$1"; then
		printf 'docker socket mount is forbidden: %s\n' "$1" >&2
		return 1
	fi
}

[ -x "$replacer" ] || fail "replacement executor is missing or not executable: $replacer"
assert_file "$path_unit"
assert_file "$service_unit"
assert_file "$compose_under_test"

tmp=$(mktemp -d "${TMPDIR:-/tmp}/vestibule-replacement-test.XXXXXX") || fail "cannot create temporary directory"
trap 'rm -rf -- "$tmp"' EXIT HUP INT TERM

fake_installer=${tmp}/installer
cat > "$fake_installer" <<'EOF'
#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$INSTALLER_LOG"
case $1 in
	--upgrade | --rollback) ;;
	*) echo "fake installer: unexpected action $1" >&2; exit 64 ;;
esac
if [ -n "${NEXT_REQUEST_FILE:-}" ]; then
	printf '%s\n' "$NEXT_REQUEST_VERSION" > "$NEXT_REQUEST_FILE"
fi
EOF
chmod 755 "$fake_installer"

fake_compose=${tmp}/compose
cat > "$fake_compose" <<'EOF'
#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$COMPOSE_LOG"
case " $* " in
	*' pull app '*) exit 0 ;;
	*' up -d --no-deps app '*)
		count=0
		[ ! -f "$COMPOSE_UP_COUNT" ] || count=$(cat "$COMPOSE_UP_COUNT")
		count=$((count + 1))
		printf '%s\n' "$count" > "$COMPOSE_UP_COUNT"
		if [ "${FAIL_ROLLBACK:-no}" = yes ] && [ "$count" -eq 2 ]; then
			exit 42
		fi
		exit 0
		;;
	*) echo "fake compose: unexpected command: $*" >&2; exit 64 ;;
esac
EOF
chmod 755 "$fake_compose"

fake_health=${tmp}/health
cat > "$fake_health" <<'EOF'
#!/bin/sh
set -eu
count=0
[ ! -f "$HEALTH_COUNT" ] || count=$(cat "$HEALTH_COUNT")
count=$((count + 1))
printf '%s\n' "$count" > "$HEALTH_COUNT"
[ "$count" -gt "${HEALTH_FAILURES:-0}" ]
EOF
chmod 755 "$fake_health"

fake_curl=${tmp}/curl
cat > "$fake_curl" <<'EOF'
#!/bin/sh
set -eu
url=
for argument in "$@"; do
	url=$argument
done
printf '%s\n' "$url" >> "$CURL_LOG"
EOF
chmod 755 "$fake_curl"

new_sandbox() {
	case_name=$1
	sandbox=${tmp}/${case_name}
	test_root=${sandbox}/root
	state_dir=${test_root}/var/lib/vestibule
	confdir=${test_root}/etc/vestibule
	installer_log=${sandbox}/installer.log
	compose_log=${sandbox}/compose.log
	compose_up_count=${sandbox}/compose-up-count
	health_count=${sandbox}/health-count
	curl_log=${sandbox}/curl.log
	mkdir -p "$state_dir" "$confdir" "${sandbox}/tmp"
	cp "$compose_under_test" "${confdir}/compose.yaml"
	printf 'deployment=container\n' > "${confdir}/deployment.env"
	cat > "${confdir}/container.env" <<EOF
VESTIBULE_APP_IMAGE=ghcr.io/zakkaus/vestibule:v1.0.0
VESTIBULE_STATE_DIRECTORY=${state_dir}
VESTIBULE_CONFIG_PATH=${confdir}/config.json
VESTIBULE_BOT_API_IMAGE=ghcr.io/zakkaus/vestibule-bot-api:v1.0.0
VESTIBULE_BOT_API_STATE_DIRECTORY=${test_root}/var/lib/vestibule-bot-api
VESTIBULE_POSTGRES_PASSWORD=fixture
VESTIBULE_DATABASE_URI=postgres://vestibule:fixture@database:5432/vestibule?sslmode=disable
EOF
	: > "$installer_log"
	: > "$compose_log"
	: > "$curl_log"
	health_failures=0
	fail_rollback=no
	next_request_file=
	next_request_version=
}

run_replace() {
	INSTALLER_LOG=$installer_log \
	COMPOSE_LOG=$compose_log \
	COMPOSE_UP_COUNT=$compose_up_count \
	HEALTH_COUNT=$health_count \
	HEALTH_FAILURES=$health_failures \
	FAIL_ROLLBACK=$fail_rollback \
	NEXT_REQUEST_FILE=$next_request_file \
	NEXT_REQUEST_VERSION=$next_request_version \
	VESTIBULE_ROOT=$test_root \
	VESTIBULE_REPLACE_INSTALLER=$fake_installer \
	VESTIBULE_REPLACE_COMPOSE=$fake_compose \
	VESTIBULE_REPLACE_HEALTH_COMMAND=$fake_health \
	VESTIBULE_REPLACE_HEALTH_ATTEMPTS=1 \
	TMPDIR=${sandbox}/tmp \
		"$replacer"
}

run_replace_default_probe() {
	INSTALLER_LOG=$installer_log \
	COMPOSE_LOG=$compose_log \
	COMPOSE_UP_COUNT=$compose_up_count \
	CURL_LOG=$curl_log \
	VESTIBULE_ROOT=$test_root \
	VESTIBULE_REPLACE_INSTALLER=$fake_installer \
	VESTIBULE_REPLACE_COMPOSE=$fake_compose \
	VESTIBULE_REPLACE_HEALTH_COMMAND= \
	VESTIBULE_REPLACE_HEALTH_URL= \
	VESTIBULE_REPLACE_LIVE_URL= \
	VESTIBULE_REPLACE_HEALTH_ATTEMPTS=1 \
	PATH=${tmp}:$PATH \
	TMPDIR=${sandbox}/tmp \
		"$replacer"
}

run_success() {
	log=$1
	if ! run_replace > "$log" 2>&1; then
		sed 's/^/    /' "$log" >&2
		fail "replacement unexpectedly failed"
	fi
}

run_failure() {
	log=$1
	if run_replace > "$log" 2>&1; then
		fail "replacement unexpectedly succeeded"
	fi
}

case_unit_shape() {
	assert_line 'PathExists=/var/lib/vestibule/replacement-request' "$path_unit"
	assert_line 'PathChanged=/var/lib/vestibule/replacement-request' "$path_unit"
	assert_line 'Unit=vestibule-replace.service' "$path_unit"
	assert_line 'ExecStart=/usr/local/libexec/vestibule-replace' "$service_unit"
	assert_no_docker_socket "$compose_under_test" || fail "published compose exposes the Docker socket"
	grep -Fq '  app:' "$compose_under_test" || fail "compose has no app service"
	grep -Fq '  database:' "$compose_under_test" || fail "compose has no database service"
	grep -Fq '  bot-api:' "$compose_under_test" || fail "compose has no Bot API service"
	pass "path unit watches only the request and compose has no Docker socket"
}

case_native() {
	new_sandbox native
	printf 'deployment=native\n' > "${confdir}/deployment.env"
	printf 'v2.0.0\n' > "${state_dir}/replacement-request"
	run_success "${sandbox}/native.out"
	result=${state_dir}/replacement-result.env
	assert_mode "$result" 600
	assert_line 'status=applied' "$result"
	assert_line 'requested_version=v2.0.0' "$result"
	assert_line 'reason=complete' "$result"
	assert_line '--upgrade v2.0.0' "$installer_log"
	assert_absent "${state_dir}/replacement-request"
	pass "one host executor applies a native version request"
}

case_request_preservation() {
	new_sandbox request-preservation
	printf 'deployment=native\n' > "${confdir}/deployment.env"
	printf 'v2.0.0\n' > "${state_dir}/replacement-request"
	next_request_file=${state_dir}/replacement-request
	next_request_version=v3.0.0
	run_success "${sandbox}/preservation.out"
	result=${state_dir}/replacement-result.env
	assert_line 'requested_version=v2.0.0' "$result"
	assert_line 'v3.0.0' "${state_dir}/replacement-request"
	pass "a request written during replacement remains for the next unit run"
}

case_health_endpoints() {
	new_sandbox health-endpoints
	printf 'deployment=native\n' > "${confdir}/deployment.env"
	printf 'v2.0.0\n' > "${state_dir}/replacement-request"
	if ! run_replace_default_probe > "${sandbox}/health.out" 2>&1; then
		sed 's/^/    /' "${sandbox}/health.out" >&2
		fail "replacement default health probes unexpectedly failed"
	fi
	assert_line 'http://127.0.0.1:8080/livez' "$curl_log"
	assert_line 'http://127.0.0.1:8080/readyz' "$curl_log"
	pass "host replacement probes liveness and readiness before recording success"
}

case_container() {
	new_sandbox container
	printf 'v2.0.0\n' > "${state_dir}/replacement-request"
	run_success "${sandbox}/container.out"
	result=${state_dir}/replacement-result.env
	assert_mode "$result" 600
	assert_line 'status=applied' "$result"
	assert_line 'VESTIBULE_APP_IMAGE=ghcr.io/zakkaus/vestibule:v2.0.0' "${confdir}/container.env"
	grep -Fq ' pull app' "$compose_log" || fail "container replacement did not pull app"
	grep -Fq ' up -d --no-deps app' "$compose_log" || fail "container replacement did not recreate app"
	pass "the same host executor applies a container version request"
}

case_request_url() {
	new_sandbox request-url
	cat > "${state_dir}/replacement-request" <<'EOF'
v2.0.0
https://attacker.example/vestibule
EOF
	run_failure "${sandbox}/request-url.out"
	result=${state_dir}/replacement-result.env
	assert_line 'status=rejected' "$result"
	assert_line 'requested_version=invalid' "$result"
	assert_line 'reason=invalid_request' "$result"
	[ ! -s "$compose_log" ] || fail "URL-bearing request reached the container runtime"
	pass "a request containing a download address is rejected"
}

# The URL case above is rejected for having two lines. A single line that is a
# URL has to be rejected on its own, because the executor must not trust a file
# anyone who can write the data directory could have written.
case_request_single_line_url() {
	new_sandbox request-single-line-url
	printf 'https://attacker.example/vestibule\n' > "${state_dir}/replacement-request"
	run_failure "${sandbox}/request-single-line-url.out"
	result=${state_dir}/replacement-result.env
	assert_line 'status=rejected' "$result"
	assert_line 'requested_version=invalid' "$result"
	assert_line 'reason=invalid_request' "$result"
	[ ! -s "$compose_log" ] || fail "single-line URL request reached the container runtime"
	pass "a one-line request that is a download address is rejected"
}

case_request_extra_line() {
	new_sandbox request-extra-line
	printf 'v2.0.0\n\n' > "${state_dir}/replacement-request"
	run_failure "${sandbox}/request-extra-line.out"
	result=${state_dir}/replacement-result.env
	assert_line 'status=rejected' "$result"
	assert_line 'reason=invalid_request' "$result"
	[ ! -s "$compose_log" ] || fail "multi-line request reached the container runtime"
	pass "a request with an extra line is rejected"
}

case_automatic_rollback() {
	new_sandbox automatic-rollback
	printf 'v2.0.0\n' > "${state_dir}/replacement-request"
	health_failures=1
	run_success "${sandbox}/automatic-rollback.out"
	result=${state_dir}/replacement-result.env
	assert_mode "$result" 600
	assert_line 'status=rolled_back' "$result"
	assert_line 'requested_version=v2.0.0' "$result"
	assert_line 'reason=healthcheck_failed' "$result"
	assert_line 'VESTIBULE_APP_IMAGE=ghcr.io/zakkaus/vestibule:v1.0.0' "${confdir}/container.env"
	up_count=$(cat "$compose_up_count")
	[ "$up_count" = 2 ] || fail "replacement did not recreate the prior container"
	pass "a failed health check restores the prior container and records why"
}

case_rollback_required() {
	new_sandbox rollback-required
	printf 'v2.0.0\n' > "${state_dir}/replacement-request"
	health_failures=99
	fail_rollback=yes
	run_failure "${sandbox}/rollback-required.out"
	result=${state_dir}/replacement-result.env
	assert_line 'status=rollback_failed' "$result"
	assert_line 'reason=healthcheck_failed' "$result"
	assert_line 'VESTIBULE_APP_IMAGE=ghcr.io/zakkaus/vestibule:v1.0.0' "${confdir}/container.env"
	up_count=$(cat "$compose_up_count")
	[ "$up_count" = 2 ] || fail "replacement did not attempt the required rollback"
	pass "a failed replacement without a working rollback is rejected"
}

case_socket_rejection() {
	new_sandbox socket-rejection
	assert_no_docker_socket "${confdir}/compose.yaml" || fail "published compose exposes the Docker socket"
	cat > "${confdir}/compose.yaml" <<'EOF'
services:
  app:
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
EOF
	if assert_no_docker_socket "${confdir}/compose.yaml" > "${sandbox}/socket.out" 2>&1; then
		fail "socket injection was not rejected"
	fi
	pass "a Docker socket mount in the application deployment is rejected"
}

run_case() {
	case $1 in
		unit-shape) case_unit_shape ;;
		native) case_native ;;
		request-preservation) case_request_preservation ;;
		container) case_container ;;
		health-endpoints) case_health_endpoints ;;
		request-url) case_request_url ;;
		request-single-line-url) case_request_single_line_url ;;
		request-extra-line) case_request_extra_line ;;
		automatic-rollback) case_automatic_rollback ;;
		rollback-required) case_rollback_required ;;
		socket-rejection) case_socket_rejection ;;
		*) fail "unknown test case: $1" ;;
	esac
}

if [ "$selected" = all ]; then
	for test_case in unit-shape native request-preservation container health-endpoints request-url request-single-line-url request-extra-line automatic-rollback rollback-required socket-rejection; do
		run_case "$test_case"
	done
else
	run_case "$selected"
fi

printf 'test-replacement: passed\n'
