#!/bin/sh
# Install and manage the native Vestibule service from a GitHub release.
#
#   sh install.sh [VERSION]              install, or upgrade when already installed
#   sh install.sh --upgrade [VERSION]    require an existing installation and upgrade it
#   sh install.sh --rollback             swap back to the retained release
#   sh install.sh --uninstall            remove programs and units, preserving config and data
#   sh install.sh --status               print machine-readable installation state
set -eu
repo=Zakkaus/vestibule
name=vestibule
action=deploy
action_set=no
version=
purge_data=ask
root=${VESTIBULE_ROOT:-}
fetch_override=${VESTIBULE_FETCH:-}
systemctl_override=${VESTIBULE_SYSTEMCTL:-}
claim_base_url=${VESTIBULE_CLAIM_BASE_URL:-http://127.0.0.1:8080}
usage() {
	cat <<'EOF'
Usage: install.sh [--upgrade] [VERSION]
       install.sh --rollback
       install.sh --uninstall [--purge-data|--keep-data]
       install.sh --status
The default action installs Vestibule. When Vestibule is already installed, the
same command upgrades it. Uninstall preserves configuration and state unless
--purge-data is explicitly selected.
EOF
}
fail() { printf 'install.sh: %s\n' "$*" >&2; exit 1; }
warn() { printf 'install.sh: WARNING: %s\n' "$*" >&2; }
select_action() {
	requested=$1
	if [ "$action_set" = yes ] && [ "$action" != "$requested" ]; then
		fail "choose only one lifecycle action"
	fi
	action=$requested
	action_set=yes
}
while [ "$#" -gt 0 ]; do
	case $1 in
		-h | --help) usage; exit 0 ;;
		--upgrade) select_action upgrade ;;
		--rollback) select_action rollback ;;
		--uninstall) select_action uninstall ;;
		--status) select_action status ;;
		--purge-data) purge_data=yes ;;
		--keep-data) purge_data=no ;;
		-*) fail "unknown option $1" ;;
		*)
			[ -z "$version" ] || fail "only one release version may be specified"
			version=$1
			;;
	esac
	shift
done
case $root in
	/) root= ;;
	'' | /*) ;;
	*) fail "VESTIBULE_ROOT must be an absolute path" ;;
esac
case $root in
	*'|'*) fail "VESTIBULE_ROOT may not contain a pipe character" ;;
esac
case $claim_base_url in
	http://* | https://*) claim_base_url=${claim_base_url%/} ;;
	*) fail "VESTIBULE_CLAIM_BASE_URL must be an absolute HTTP or HTTPS URL" ;;
esac
prefix=${root}/usr/local/bin
confdir=${root}/etc/${name}
state_dir=${root}/var/lib/${name}
binary=${prefix}/${name}
previous_binary=${prefix}/${name}.previous
unit=${root}/etc/systemd/system/${name}.service
previous_unit=${unit}.previous
bot_env=${confdir}/bot.env
setup_env=${confdir}/setup.env
current_manifest=${confdir}/schema-manifest
previous_manifest=${confdir}/schema-manifest.previous
current_version_file=${confdir}/release-version
previous_version_file=${confdir}/release-version.previous
claim_state=${state_dir}/claim.json
result_file=${confdir}/install-result.env
work=
txn_active=no
txn_count=0
journal=
dir_journal=
manager=foreground
systemctl_bin=
fetcher=
service_changed=no
had_binary_before=no
had_unit_before=no
claim_url=
setup_hash=
new_claim_expected=no
service_control() { [ "$manager" = systemd ] || return 0; "$systemctl_bin" "$@"; }
restore_transaction() {
	set +e
	if [ -n "$journal" ] && [ -f "$journal" ]; then
		while IFS='|' read -r disposition destination backup; do
			[ -n "$disposition" ] || continue
			case $disposition in
				restore) cp -p -- "$backup" "$destination" ;;
				remove) rm -f -- "$destination" ;;
			esac
		done < "$journal"
	fi
	if [ "$service_changed" = yes ] && [ "$manager" = systemd ]; then
		service_control daemon-reload >/dev/null 2>&1 || :
		if [ "$had_unit_before" = yes ] && [ "$had_binary_before" = yes ]; then
			service_control restart "$name" >/dev/null 2>&1 || :
		else
			service_control disable --now "$name" >/dev/null 2>&1 || :
		fi
	fi
	if [ "$new_claim_expected" = yes ] && [ -f "$claim_state" ]; then
		actual_claim=$(cat "$claim_state")
		[ "$actual_claim" != "{\"setup_token_hash\":\"${setup_hash}\"}" ] || rm -f -- "$claim_state"
	fi
	if [ -n "$dir_journal" ] && [ -f "$dir_journal" ]; then
		while IFS= read -r directory; do
			[ -n "$directory" ] && rmdir -- "$directory" >/dev/null 2>&1 || :
		done < "$dir_journal"
	fi
	set -e
}
on_exit() {
	status=$?
	trap - EXIT HUP INT TERM
	if [ "$status" -ne 0 ] && [ "$txn_active" = yes ]; then
		warn "operation failed; restoring existing files and removing only files created by this run"
		restore_transaction
	fi
	[ -z "$work" ] || rm -rf -- "$work"
	exit "$status"
}
trap on_exit EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM
make_workdir() {
	if [ -n "$work" ]; then return 0; fi
	work=$(mktemp -d "${TMPDIR:-/tmp}/vestibule-install.XXXXXX") || fail "cannot create temporary directory"
	journal=${work}/files.journal
	dir_journal=${work}/dirs.journal
	: > "$journal"
	: > "$dir_journal"
}
begin_transaction() {
	make_workdir
	txn_active=yes
	if [ -e "$binary" ]; then
		had_binary_before=yes
	fi
	if [ -e "$unit" ]; then
		had_unit_before=yes
	fi
}
commit_transaction() { txn_active=no; }
ensure_directory() {
	directory=$1
	if [ -d "$directory" ]; then
		return
	fi
	[ ! -e "$directory" ] || fail "$directory exists and is not a directory"
	mkdir -p -- "$directory"
	printf '%s\n' "$directory" >> "$dir_journal"
}
txn_replace() {
	source_file=$1
	mode=$2
	destination=$3
	ensure_directory "${destination%/*}"
	txn_count=$((txn_count + 1))
	backup=${work}/backup.${txn_count}
	if [ -e "$destination" ] || [ -L "$destination" ]; then
		cp -p -- "$destination" "$backup"
		printf 'restore|%s|%s\n' "$destination" "$backup" >> "$journal"
	else
		printf 'remove|%s|\n' "$destination" >> "$journal"
	fi
	staged=${destination}.vestibule-new.$$
	rm -f -- "$staged"
	cp -- "$source_file" "$staged"
	chmod "$mode" "$staged"
	mv -f -- "$staged" "$destination"
}
txn_remove() {
	destination=$1
	if [ ! -e "$destination" ] && [ ! -L "$destination" ]; then
		return
	fi
	txn_count=$((txn_count + 1))
	backup=${work}/backup.${txn_count}
	cp -p -- "$destination" "$backup"
	printf 'restore|%s|%s\n' "$destination" "$backup" >> "$journal"
	rm -f -- "$destination"
}
require_mutation_access() {
	if [ -z "$root" ] && [ "$(id -u)" -ne 0 ]; then
		fail "install, upgrade, rollback, and uninstall require root; rerun this command through sudo"
	fi
}
detect_fetcher() {
	if [ -n "$fetch_override" ]; then
		command -v "$fetch_override" >/dev/null 2>&1 || fail "VESTIBULE_FETCH is not executable: $fetch_override"
		fetcher=override
	elif command -v curl >/dev/null 2>&1; then
		fetcher=curl
	elif command -v wget >/dev/null 2>&1; then
		fetcher=wget
	else
		fail "neither curl nor wget is available"
	fi
}
fetch_url() {
	url=$1
	destination=$2
	case $fetcher in
		override) "$fetch_override" "$url" "$destination" ;;
		curl) curl --fail --silent --show-error --location --output "$destination" "$url" ;;
		wget) wget --quiet --output-document="$destination" "$url" ;;
	esac
}
detect_service_manager() {
	if [ -n "$systemctl_override" ]; then
		command -v "$systemctl_override" >/dev/null 2>&1 || fail "VESTIBULE_SYSTEMCTL is not executable: $systemctl_override"
		systemctl_bin=$systemctl_override
		manager=systemd
	elif [ -z "$root" ] && command -v systemctl >/dev/null 2>&1; then
		systemctl_bin=$(command -v systemctl)
		manager=systemd
	else
		manager=foreground
	fi
}
detect_architecture() {
	case $(uname -m) in
		x86_64) arch=amd64 ;;
		aarch64 | arm64) arch=arm64 ;;
		*) fail "no release binary for $(uname -m)" ;;
	esac
}
validate_version() {
	case $version in
		'' | *[!A-Za-z0-9._-]*) fail "invalid release version: $version" ;;
	esac
}
resolve_version() {
	[ -n "$version" ] && { validate_version; return; }
	fetch_url "https://api.github.com/repos/${repo}/releases/latest" "${work}/latest.json"
	version=$(sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' "${work}/latest.json" | sed -n '1p')
	[ -n "$version" ] || fail "could not resolve the latest release"
	validate_version
}
verify_asset() {
	asset=$1
	expected=$(awk -v name="$asset" '$2 == name { count++; digest=$1 } END { if (count != 1) exit 1; print digest }' "${work}/SHA256SUMS") \
		|| fail "SHA256SUMS must contain exactly one entry for $asset"
	[ "${#expected}" -eq 64 ] || fail "SHA256SUMS has an invalid digest for $asset"
	case $expected in *[!0-9a-fA-F]*) fail "SHA256SUMS has an invalid digest for $asset" ;; esac
	actual=$(sha256sum "${work}/${asset}")
	actual=${actual%% *}
	[ "$actual" = "$expected" ] || fail "checksum mismatch for $asset"
	printf 'verified SHA-256: %s\n' "$asset"
}
parse_manifest() {
	manifest_path=$1
	parsed_target=
	parsed_minimum=
	seen_target=no
	seen_minimum=no
	while IFS= read -r line || [ -n "$line" ]; do
		case $line in
			target_schema_version=*)
				[ "$seen_target" = no ] || fail "$manifest_path repeats target_schema_version"
				parsed_target=${line#*=}
				seen_target=yes
				;;
			minimum_rollback_schema_version=*)
				[ "$seen_minimum" = no ] || fail "$manifest_path repeats minimum_rollback_schema_version"
				parsed_minimum=${line#*=}
				seen_minimum=yes
				;;
			*) fail "$manifest_path contains an unknown or malformed field" ;;
		esac
	done < "$manifest_path"
	[ "$seen_target" = yes ] && [ "$seen_minimum" = yes ] || fail "$manifest_path is incomplete"
	case $parsed_target in '' | *[!0-9]*) fail "$manifest_path has an invalid target schema version" ;; esac
	case $parsed_minimum in '' | *[!0-9]*) fail "$manifest_path has an invalid rollback floor" ;; esac
	[ "$parsed_minimum" -le "$parsed_target" ] || fail "$manifest_path has a rollback floor above its target"
}
fetch_release_manifest() {
	base="https://github.com/${repo}/releases/download/${version}"
	fetch_url "${base}/SHA256SUMS" "${work}/SHA256SUMS"
	fetch_url "${base}/${name}-schema-manifest" "${work}/${name}-schema-manifest"
	warn "SHA256SUMS comes from the same GitHub release as the assets; it verifies integrity against that file, not publisher authenticity, because no detached signature is published"
	verify_asset "${name}-schema-manifest"
	parse_manifest "${work}/${name}-schema-manifest"
	target_schema=$parsed_target
	target_minimum=$parsed_minimum
}
assess_upgrade_rollback() {
	[ -f "$current_manifest" ] || fail "existing installation has no stored schema manifest; rollback safety is unknown, so the binary will not be downloaded"
	parse_manifest "$current_manifest"
	current_schema=$parsed_target
	if [ "$target_schema" -lt "$current_schema" ]; then
		fail "target schema v${target_schema} is older than installed schema v${current_schema}; use --rollback with the retained release"
	fi
	if [ "$target_schema" -eq "$current_schema" ]; then
		printf 'rollback preflight: safe; target and retained release both use schema v%s\n' "$current_schema"
		return
	fi
	if [ "$current_schema" -lt "$target_minimum" ]; then
		fail "rollback preflight: unsafe; target schema v${target_schema} requires schema v${target_minimum} or newer, but the retained release uses v${current_schema}; the binary was not downloaded"
	fi
	printf 'rollback preflight: safe from target schema v%s to retained schema v%s (minimum v%s)\n' \
		"$target_schema" "$current_schema" "$target_minimum"
}
fetch_release_payload() {
	binary_asset=${name}-linux-${arch}
	fetch_url "${base}/${binary_asset}" "${work}/${binary_asset}"
	fetch_url "${base}/${name}.service" "${work}/${name}.service"
	verify_asset "$binary_asset"
	verify_asset "${name}.service"
}
random_setup_token() {
	if command -v openssl >/dev/null 2>&1; then
		openssl rand -hex 32
	elif command -v od >/dev/null 2>&1; then
		od -An -N32 -tx1 /dev/urandom | tr -d ' \n'
	else
		fail "openssl or od is required to generate the one-use claim link"
	fi
}
result_value() {
	key=$1
	[ -f "$result_file" ] || return 0
	awk -F= -v key="$key" '$1 == key { sub(/^[^=]*=/, ""); print; exit }' "$result_file"
}
write_result() {
	result_operation=$1
	result_version=$2
	rollback_available=$3
	[ -n "$claim_url" ] || claim_url=$(result_value claim_url)
	cat > "${work}/install-result.env" <<EOF
operation=${result_operation}
version=${result_version}
deployment=native-${manager}
claim_url=${claim_url}
rollback_available=${rollback_available}
checksum_integrity=sha256
checksum_authenticity=unverified_same_release
EOF
	txn_replace "${work}/install-result.env" 600 "$result_file"
}
prepare_first_claim() {
	claim_url=$(result_value claim_url)
	if [ -e "$claim_state" ]; then
		rm -f -- "$setup_env"
		return
	fi
	if [ -n "$claim_url" ]; then
		setup_token=${claim_url##*/}
		[ "${#setup_token}" -eq 64 ] || fail "$result_file contains an invalid claim URL"
		case $setup_token in *[!0-9a-f]*) fail "$result_file contains an invalid claim URL" ;; esac
	else
		setup_token=$(random_setup_token)
		claim_url=${claim_base_url}/setup/${setup_token}
	fi
	setup_hash=$(printf '%s' "$setup_token" | sha256sum)
	setup_hash=${setup_hash%% *}
	if [ "$manager" = systemd ]; then
		printf 'SETUP_TOKEN=%s\n' "$setup_token" > "${work}/setup.env"
		txn_replace "${work}/setup.env" 600 "$setup_env"
		new_claim_expected=yes
	else
		ensure_directory "$state_dir"
		printf '{"setup_token_hash":"%s"}\n' "$setup_hash" > "${work}/claim.json"
		txn_replace "${work}/claim.json" 600 "$claim_state"
	fi
}
stage_previous_release() {
	cp -p -- "$binary" "${work}/retained-binary"
	cp -p -- "$current_manifest" "${work}/retained-manifest"
	cp -p -- "$current_version_file" "${work}/retained-version"
	txn_replace "${work}/retained-binary" 755 "$previous_binary"
	txn_replace "${work}/retained-manifest" 644 "$previous_manifest"
	txn_replace "${work}/retained-version" 644 "$previous_version_file"
	if [ -f "$unit" ]; then
		cp -p -- "$unit" "${work}/retained-unit"
		txn_replace "${work}/retained-unit" 644 "$previous_unit"
	fi
}
install_or_upgrade() {
	require_mutation_access
	make_workdir
	detect_fetcher
	detect_service_manager
	detect_architecture
	operation=install
	if [ -e "$binary" ]; then
		operation=upgrade
	fi
	if [ "$action" = upgrade ] && [ "$operation" != upgrade ]; then
		fail "--upgrade requires an existing $binary"
	fi
	resolve_version
	printf '%s %s %s (%s)\n' "$operation" "$name" "$version" "$arch"
	fetch_release_manifest
	if [ "$operation" = upgrade ]; then
		assess_upgrade_rollback
	fi
	fetch_release_payload
	begin_transaction
	ensure_directory "$prefix"
	ensure_directory "$confdir"
	if [ ! -e "$bot_env" ]; then
		: > "${work}/bot.env"
		txn_replace "${work}/bot.env" 600 "$bot_env"
	fi
	prepare_first_claim
	if [ "$operation" = upgrade ]; then
		[ -f "$current_version_file" ] || fail "existing installation has no stored release version"
		stage_previous_release
	fi
	txn_replace "${work}/${name}-linux-${arch}" 755 "$binary"
	txn_replace "${work}/${name}-schema-manifest" 644 "$current_manifest"
	printf '%s\n' "$version" > "${work}/release-version"
	txn_replace "${work}/release-version" 644 "$current_version_file"
	if [ "$manager" = systemd ]; then
		txn_replace "${work}/${name}.service" 644 "$unit"
		service_changed=yes
		service_control daemon-reload
		service_control enable "$name"
		service_control restart "$name"
		if [ "$new_claim_expected" = yes ]; then
			[ -f "$claim_state" ] || fail "service started without persisting the one-use claim"
			rm -f -- "$setup_env"
		fi
	else
		warn "systemctl was not found; files are installed, but no service was started or enabled"
		printf 'start manually: STATE_DIRECTORY=%s %s --config %s/config.json\n' "$state_dir" "$binary" "$confdir"
	fi
	if [ "$operation" = upgrade ]; then
		rollback_available=yes
	else
		rollback_available=no
	fi
	write_result "$operation" "$version" "$rollback_available"
	commit_transaction
	printf '%s %s complete\n' "$name" "$operation"
	claim_url=$(result_value claim_url)
	[ -z "$claim_url" ] || printf 'claim URL: %s\n' "$claim_url"
	printf 'result file: %s\n' "$result_file"
}
check_manual_rollback() {
	parse_manifest "$current_manifest"
	rollback_floor=$parsed_minimum
	current_schema=$parsed_target
	parse_manifest "$previous_manifest"
	rollback_schema=$parsed_target
	if [ "$rollback_schema" -lt "$rollback_floor" ]; then
		fail "retained release schema v${rollback_schema} is below current release rollback floor v${rollback_floor}"
	fi
	printf 'rollback preflight: safe from schema v%s to retained schema v%s\n' "$current_schema" "$rollback_schema"
}
swap_release_file() {
	current=$1
	previous=$2
	mode=$3
	label=$4
	cp -p -- "$current" "${work}/${label}.current"
	cp -p -- "$previous" "${work}/${label}.previous"
	txn_replace "${work}/${label}.previous" "$mode" "$current"
	txn_replace "${work}/${label}.current" "$mode" "$previous"
}
rollback_release() {
	require_mutation_access
	for required in "$binary" "$previous_binary" "$current_manifest" "$previous_manifest" \
		"$current_version_file" "$previous_version_file"; do
		[ -f "$required" ] || fail "cannot roll back; retained file is missing: $required"
	done
	make_workdir
	detect_service_manager
	check_manual_rollback
	begin_transaction
	swap_release_file "$binary" "$previous_binary" 755 binary
	swap_release_file "$current_manifest" "$previous_manifest" 644 manifest
	swap_release_file "$current_version_file" "$previous_version_file" 644 version
	if [ -f "$unit" ] && [ -f "$previous_unit" ]; then
		swap_release_file "$unit" "$previous_unit" 644 unit
	fi
	if [ "$manager" = systemd ]; then
		service_changed=yes
		service_control daemon-reload
		service_control restart "$name"
	else
		warn "systemctl was not found; release files were swapped, but no service was restarted"
	fi
	rolled_back_version=$(sed -n '1p' "$current_version_file")
	write_result rollback "$rolled_back_version" yes
	commit_transaction
	printf '%s rolled back to %s\n' "$name" "$rolled_back_version"
}
show_status() {
	detect_service_manager
	if [ ! -f "$binary" ]; then
		printf 'installed=no\n'
		return 1
	fi
	installed_version=unknown
	[ ! -f "$current_version_file" ] || installed_version=$(sed -n '1p' "$current_version_file")
	if [ -f "$previous_binary" ] && [ -f "$previous_manifest" ]; then
		rollback_available=yes
	else
		rollback_available=no
	fi
	printf 'installed=yes\nversion=%s\ndeployment=native-%s\nrollback_available=%s\n' \
		"$installed_version" "$manager" "$rollback_available"
	if [ "$manager" = systemd ]; then
		service_enabled=$(service_control is-enabled "$name" 2>/dev/null || :)
		service_active=$(service_control is-active "$name" 2>/dev/null || :)
		[ -n "$service_enabled" ] || service_enabled=unknown
		[ -n "$service_active" ] || service_active=unknown
		printf 'service_enabled=%s\nservice_active=%s\n' "$service_enabled" "$service_active"
	fi
	last_operation=$(result_value operation)
	claim_url=$(result_value claim_url)
	[ -z "$last_operation" ] || printf 'last_operation=%s\n' "$last_operation"
	[ -z "$claim_url" ] || printf 'claim_url=%s\n' "$claim_url"
	printf 'result_file=%s\n' "$result_file"
}
choose_purge_data() {
	if [ "$purge_data" != ask ]; then
		return
	fi
	purge_data=no
	if [ -t 0 ]; then
		printf 'Remove %s and %s, including credentials and database state? [y/N] ' "$confdir" "$state_dir"
		read -r answer || answer=
		case $answer in y | Y | yes | YES) purge_data=yes ;; esac
	fi
}
uninstall_release() {
	require_mutation_access
	make_workdir
	detect_service_manager
	choose_purge_data
	if [ "$manager" = systemd ] && [ -e "$unit" ]; then
		service_control disable --now "$name"
		service_changed=yes
	fi
	begin_transaction
	for installed_file in "$binary" "$previous_binary" "$unit" "$previous_unit" \
		"$current_manifest" "$previous_manifest" "$current_version_file" "$previous_version_file"; do
		txn_remove "$installed_file"
	done
	if [ -d "$state_dir" ]; then
		uninstalled_version=$(result_value version)
		[ -n "$uninstalled_version" ] || uninstalled_version=unknown
		write_result uninstall "$uninstalled_version" no
	fi
	if [ "$manager" = systemd ]; then
		service_control daemon-reload
	fi
	commit_transaction
	if [ "$purge_data" = yes ]; then
		rm -rf -- "$confdir" "$state_dir"
		printf 'removed programs, units, configuration, and state\n'
	else
		printf 'removed programs and units; preserved %s and %s\n' "$confdir" "$state_dir"
	fi
}
case $action in
	deploy | upgrade)
		[ "$purge_data" = ask ] || fail "--purge-data and --keep-data apply only to --uninstall"
		install_or_upgrade
		;;
	rollback)
		[ -z "$version" ] || fail "--rollback does not accept a release version"
		[ "$purge_data" = ask ] || fail "data options apply only to --uninstall"
		rollback_release
		;;
	status)
		[ -z "$version" ] || fail "--status does not accept a release version"
		[ "$purge_data" = ask ] || fail "data options apply only to --uninstall"
		show_status
		;;
	uninstall)
		[ -z "$version" ] || fail "--uninstall does not accept a release version"
		uninstall_release
		;;
esac
